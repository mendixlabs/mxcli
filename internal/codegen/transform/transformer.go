// SPDX-License-Identifier: Apache-2.0

package transform

import (
	"slices"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/internal/codegen/schema"
)

// GoPackage represents a generated Go package.
type GoPackage struct {
	Name       string          // Package name (e.g., "domainmodels")
	Namespace  string          // Original namespace (e.g., "DomainModels")
	Imports    map[string]bool // Required imports
	Types      []*GoType       // Struct types
	Interfaces []*GoInterface  // Interface types (for abstract types)
	Enums      []*GoEnum       // Enum types
}

// GoType represents a generated Go struct type.
type GoType struct {
	Name             string     // Go type name (e.g., "Entity")
	QualifiedName    string     // Original qualified name (e.g., "DomainModels$Entity")
	Comment          string     // Doc comment
	Fields           []*GoField // Struct fields
	MarkerMethods    []string   // Marker methods for interfaces (e.g., "isAttributeType")
	IsModelUnit      bool       // Is this a MODEL_UNIT?
	IsStructuralUnit bool       // Is this a STRUCTURAL_UNIT?
}

// GoField represents a field in a Go struct.
type GoField struct {
	Name      string // Go field name (e.g., "Name")
	GoType    string // Go type (e.g., "string", "*Entity", "[]*Attribute")
	JSONTag   string // JSON tag value
	OmitEmpty bool   // Include omitempty in tag
	Comment   string // Field comment
}

// GoInterface represents a generated Go interface for abstract types.
type GoInterface struct {
	Name            string   // Interface name
	QualifiedName   string   // Original qualified name
	Comment         string   // Doc comment
	MarkerMethod    string   // Marker method name (e.g., "isAttributeType")
	Implementations []string // List of implementing type names
}

// GoEnum represents a generated Go enum type.
type GoEnum struct {
	Name    string         // Enum type name
	Comment string         // Doc comment
	Values  []*GoEnumValue // Enum values
}

// GoEnumValue represents a single enum constant.
type GoEnumValue struct {
	Name  string // Constant name
	Value string // String value
}

// Transformer transforms reflection data into Go types.
type Transformer struct {
	data            schema.ReflectionData
	abstractTypes   map[string]bool     // Set of abstract type names
	implementations map[string][]string // Abstract type -> concrete implementations
	enumRegistry    map[string]*GoEnum  // Property path -> enum definition
}

// NewTransformer creates a new transformer for the given reflection data.
func NewTransformer(data schema.ReflectionData) *Transformer {
	t := &Transformer{
		data:            data,
		abstractTypes:   make(map[string]bool),
		implementations: make(map[string][]string),
		enumRegistry:    make(map[string]*GoEnum),
	}
	t.analyzeInheritance()
	return t
}

// analyzeInheritance builds the inheritance graph from reflection data.
func (t *Transformer) analyzeInheritance() {
	for qualifiedName, typeDef := range t.data {
		if typeDef.Abstract {
			t.abstractTypes[qualifiedName] = true
			t.implementations[qualifiedName] = typeDef.AllCompatibleTypes
		}
	}
}

// TransformNamespace transforms all types in a namespace to a Go package.
func (t *Transformer) TransformNamespace(namespace string) *GoPackage {
	pkg := &GoPackage{
		Name:      ToGoPackage(namespace),
		Namespace: namespace,
		Imports:   make(map[string]bool),
	}

	// Always import model package
	pkg.Imports["github.com/JordtenBulte-OLC/mxcli/model"] = true

	types := t.data.GetTypesByNamespace(namespace)

	for _, typeDef := range types {
		if typeDef.Abstract {
			// Generate interface for abstract type
			iface := t.transformInterface(typeDef, namespace)
			pkg.Interfaces = append(pkg.Interfaces, iface)
		} else {
			// Generate struct for concrete type
			goType, imports := t.transformType(typeDef, namespace)
			pkg.Types = append(pkg.Types, goType)
			for imp := range imports {
				pkg.Imports[imp] = true
			}
		}
	}

	// Sort for deterministic output
	sort.Slice(pkg.Types, func(i, j int) bool {
		return pkg.Types[i].Name < pkg.Types[j].Name
	})
	sort.Slice(pkg.Interfaces, func(i, j int) bool {
		return pkg.Interfaces[i].Name < pkg.Interfaces[j].Name
	})

	// Collect unique enums
	pkg.Enums = t.collectEnums(namespace)

	return pkg
}

// transformInterface creates a Go interface for an abstract type.
func (t *Transformer) transformInterface(typeDef *schema.TypeDefinition, namespace string) *GoInterface {
	typeName := ExtractTypeName(typeDef.QualifiedName)

	iface := &GoInterface{
		Name:          ToGoTypeName(typeName),
		QualifiedName: typeDef.QualifiedName,
		MarkerMethod:  ToMarkerMethodName(typeName),
	}

	// Collect implementation names
	for _, implName := range typeDef.AllCompatibleTypes {
		implType := ExtractTypeName(implName)
		iface.Implementations = append(iface.Implementations, ToGoTypeName(implType))
	}
	sort.Strings(iface.Implementations)

	iface.Comment = "// " + iface.Name + " is implemented by: " + joinStrings(iface.Implementations, ", ")

	return iface
}

// transformType creates a Go struct for a concrete type.
func (t *Transformer) transformType(typeDef *schema.TypeDefinition, namespace string) (*GoType, map[string]bool) {
	imports := make(map[string]bool)
	typeName := ExtractTypeName(typeDef.QualifiedName)

	goType := &GoType{
		Name:             ToGoTypeName(typeName),
		QualifiedName:    typeDef.QualifiedName,
		Comment:          "// " + ToGoTypeName(typeName) + " represents a " + typeDef.QualifiedName + " element.",
		IsModelUnit:      typeDef.Type == schema.ElementTypeModelUnit,
		IsStructuralUnit: typeDef.Type == schema.ElementTypeStructuralUnit,
	}

	// Find which abstract types this implements
	goType.MarkerMethods = t.findMarkerMethods(typeDef.QualifiedName)

	// Transform properties to fields
	if typeDef.Properties != nil {
		// Sort properties for deterministic output
		propNames := make([]string, 0, len(typeDef.Properties))
		for name := range typeDef.Properties {
			propNames = append(propNames, name)
		}
		sort.Strings(propNames)

		for _, propName := range propNames {
			prop := typeDef.Properties[propName]
			field, fieldImports := t.transformProperty(prop, namespace)
			if field != nil {
				goType.Fields = append(goType.Fields, field)
				for imp := range fieldImports {
					imports[imp] = true
				}
			}
		}
	}

	return goType, imports
}

// findMarkerMethods finds all marker methods this type should implement.
func (t *Transformer) findMarkerMethods(qualifiedName string) []string {
	var markers []string

	for abstractType, implementations := range t.implementations {
		if slices.Contains(implementations, qualifiedName) {
			// This type implements the abstract type
			abstractTypeName := ExtractTypeName(abstractType)
			markers = append(markers, ToMarkerMethodName(abstractTypeName))
		}
	}

	sort.Strings(markers)
	return markers
}

// transformProperty transforms a property definition to a Go field.
func (t *Transformer) transformProperty(prop *schema.PropertyDef, namespace string) (*GoField, map[string]bool) {
	imports := make(map[string]bool)

	field := &GoField{
		Name:      ToGoFieldName(prop.Name),
		JSONTag:   StorageToJSONTag(prop.StorageName),
		OmitEmpty: !prop.Required,
	}

	// Determine Go type based on TypeInfo
	switch prop.TypeInfo.Type {
	case schema.TypeInfoPrimitive:
		field.GoType = t.primitiveToGoType(prop.TypeInfo.PrimitiveType, prop.List)
		if prop.TypeInfo.PrimitiveType == schema.PrimitiveDateTime {
			imports["time"] = true
		}

	case schema.TypeInfoEnumeration:
		// Register enum and use it (will be prefixed with namespace in merged output)
		enumName := t.registerEnum(namespace, prop)
		// Prefix with namespace for single-package mode
		fullEnumName := namespace + enumName
		if prop.List {
			field.GoType = "[]" + fullEnumName
		} else {
			field.GoType = fullEnumName
		}

	case schema.TypeInfoElement:
		field.GoType, imports = t.elementToGoType(prop, namespace)

	case schema.TypeInfoUnit:
		// Unit references are stored as IDs
		if prop.List {
			field.GoType = "[]model.ID"
		} else {
			field.GoType = "model.ID"
		}
	}

	return field, imports
}

// primitiveToGoType converts a primitive type to its Go equivalent.
func (t *Transformer) primitiveToGoType(primType schema.PrimitiveType, isList bool) string {
	var goType string

	switch primType {
	case schema.PrimitiveString:
		goType = "string"
	case schema.PrimitiveInteger:
		goType = "int"
	case schema.PrimitiveLong:
		goType = "int64"
	case schema.PrimitiveDouble:
		goType = "float64"
	case schema.PrimitiveBoolean:
		goType = "bool"
	case schema.PrimitiveGUID:
		goType = "model.ID"
	case schema.PrimitiveDateTime:
		goType = "time.Time"
	case schema.PrimitivePoint:
		goType = "model.Point"
	case schema.PrimitiveSize:
		goType = "model.Size"
	case schema.PrimitiveColor:
		goType = "string" // Colors stored as strings
	case schema.PrimitiveBlob:
		goType = "[]byte"
	default:
		goType = "interface{}" // Unknown types
	}

	if isList && goType != "[]byte" {
		return "[]" + goType
	}
	return goType
}

// elementToGoType converts an element reference to its Go type.
func (t *Transformer) elementToGoType(prop *schema.PropertyDef, currentNamespace string) (string, map[string]bool) {
	imports := make(map[string]bool)

	elementType := prop.TypeInfo.ElementType
	targetNamespace := ExtractNamespace(elementType)
	targetTypeName := ExtractTypeName(elementType)

	// In single-package mode, prefix with namespace for uniqueness
	var goTypeName string
	if targetNamespace != "" {
		goTypeName = targetNamespace + ToGoTypeName(targetTypeName)
	} else {
		goTypeName = ToGoTypeName(targetTypeName)
	}

	// Determine reference style
	switch prop.TypeInfo.Kind {
	case schema.ReferenceKindPart:
		// Contained element - use pointer
		if prop.List {
			return "[]*" + goTypeName, imports
		}
		return "*" + goTypeName, imports

	case schema.ReferenceKindByID:
		// Reference by ID
		if prop.List {
			return "[]model.ID", imports
		}
		return "model.ID", imports

	case schema.ReferenceKindByName, schema.ReferenceKindLocalByName:
		// Reference by name
		if prop.List {
			return "[]model.QualifiedName", imports
		}
		return "model.QualifiedName", imports

	default:
		// Default to pointer for unknown kinds
		if prop.List {
			return "[]*" + goTypeName, imports
		}
		return "*" + goTypeName, imports
	}
}

// registerEnum registers an enumeration type and returns its Go type name.
func (t *Transformer) registerEnum(namespace string, prop *schema.PropertyDef) string {
	enumKey := namespace + "$" + prop.Name

	if existing, ok := t.enumRegistry[enumKey]; ok {
		return existing.Name
	}

	enumName := ToEnumTypeName(namespace, prop.Name)

	enum := &GoEnum{
		Name:    enumName,
		Comment: "// " + enumName + " represents possible values for " + prop.Name + ".",
	}

	for _, value := range prop.TypeInfo.Values {
		enum.Values = append(enum.Values, &GoEnumValue{
			Name:  ToEnumValueName(enumName, value),
			Value: value,
		})
	}

	t.enumRegistry[enumKey] = enum
	return enumName
}

// collectEnums collects all enums registered for a namespace.
func (t *Transformer) collectEnums(namespace string) []*GoEnum {
	var enums []*GoEnum
	prefix := namespace + "$"

	for key, enum := range t.enumRegistry {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			enums = append(enums, enum)
		}
	}

	sort.Slice(enums, func(i, j int) bool {
		return enums[i].Name < enums[j].Name
	})

	return enums
}

// Helper function
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	var result strings.Builder
	result.WriteString(strs[0])
	for i := 1; i < len(strs); i++ {
		result.WriteString(sep + strs[i])
	}
	return result.String()
}
