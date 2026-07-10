// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

func init() {
	// Import/export mapping element trees: the Elements list (on the mapping) and
	// every element's Children list serialize with the typed-array marker 2, even
	// when empty (verified against the legacy serializer). The populated marker is
	// keyed by the leading child $Type — both Object and Value element types occur
	// as the first child, so register marker 2 for all four.
	codec.RegisterListMarker("ImportMappings$ObjectMappingElement", 2)
	codec.RegisterListMarker("ImportMappings$ValueMappingElement", 2)
	codec.RegisterListMarker("ExportMappings$ObjectMappingElement", 2)
	codec.RegisterListMarker("ExportMappings$ValueMappingElement", 2)

	// MappingSourceReference is always serialized as BSON null on the mapping
	// document; CustomHandlerCall is always null on object mapping elements.
	codec.RegisterTypeDefaults("ImportMappings$ImportMapping", codec.TypeDefaults{
		NullFields:           []string{"MappingSourceReference"},
		MandatoryListMarkers: map[string]int32{"Elements": 2},
	})
	codec.RegisterTypeDefaults("ExportMappings$ExportMapping", codec.TypeDefaults{
		NullFields:           []string{"MappingSourceReference"},
		MandatoryListMarkers: map[string]int32{"Elements": 2},
	})
	codec.RegisterTypeDefaults("ImportMappings$ObjectMappingElement", codec.TypeDefaults{
		NullFields:           []string{"CustomHandlerCall"},
		MandatoryListMarkers: map[string]int32{"Children": 2},
	})
	codec.RegisterTypeDefaults("ExportMappings$ObjectMappingElement", codec.TypeDefaults{
		NullFields:           []string{"CustomHandlerCall"},
		MandatoryListMarkers: map[string]int32{"Children": 2},
	})
}

// ---------------------------------------------------------------------------
// Import mappings
// ---------------------------------------------------------------------------

// CreateImportMapping inserts a new ImportMappings$ImportMapping document (its
// schema source plus the object/value mapping element tree). Mirrors the legacy
// serializer field-for-field.
func (b *Backend) CreateImportMapping(im *model.ImportMapping) error {
	if im == nil {
		return fmt.Errorf("CreateImportMapping: nil import mapping")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateImportMapping: not connected for writing")
	}
	if im.ID == "" {
		im.ID = model.ID(mmpr.GenerateID())
	}
	im.TypeName = "ImportMappings$ImportMapping"
	contents, err := (&codec.Encoder{}).Encode(importMappingToGen(im))
	if err != nil {
		return fmt.Errorf("CreateImportMapping: encode: %w", err)
	}
	return b.writer.InsertUnit(string(im.ID), string(im.ContainerID), "Documents", "ImportMappings$ImportMapping", contents)
}

// UpdateImportMapping rewrites an existing import mapping in place (CREATE OR MODIFY).
func (b *Backend) UpdateImportMapping(im *model.ImportMapping) error {
	if im == nil {
		return fmt.Errorf("UpdateImportMapping: nil import mapping")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateImportMapping: not connected for writing")
	}
	contents, err := (&codec.Encoder{}).Encode(importMappingToGen(im))
	if err != nil {
		return fmt.Errorf("UpdateImportMapping: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(im.ID), contents)
}

// DeleteImportMapping removes an import mapping unit by ID.
func (b *Backend) DeleteImportMapping(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteImportMapping: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// GetImportMappingByQualifiedName finds an import mapping by module + name (used
// by the CREATE OR MODIFY existence check and DROP).
func (b *Backend) GetImportMappingByQualifiedName(moduleName, name string) (*model.ImportMapping, error) {
	all, err := b.ListImportMappings()
	if err != nil {
		return nil, err
	}
	for _, im := range all {
		if im.Name == name && b.moduleNameFor(im.ID) == moduleName {
			return im, nil
		}
	}
	return nil, fmt.Errorf("import mapping not found: %s.%s", moduleName, name)
}

func importMappingToGen(im *model.ImportMapping) element.Element {
	g := newElem("ImportMappings$ImportMapping", string(im.ID))
	addStr(g, "Name", im.Name)
	addStr(g, "Documentation", im.Documentation)
	addBool(g, "Excluded", im.Excluded)
	addStr(g, "ExportLevel", orDefault(im.ExportLevel, "Hidden"))
	addStr(g, "JsonStructure", im.JsonStructure)
	addStr(g, "XmlSchema", im.XmlSchema)
	addStr(g, "MessageDefinition", im.MessageDefinition)

	elems := make([]element.Element, 0, len(im.Elements))
	for _, e := range im.Elements {
		elems = append(elems, importMappingElementToGen(e, "(Object)"))
	}
	addPartList(g, "Elements", elems)

	addBool(g, "UseSubtransactionsForMicroflows", false)
	addStr(g, "PublicName", "")
	addStr(g, "XsdRootElementName", "")
	// ParameterType is a required sub-document even when unused; without it Studio
	// Pro fails to render the schema source and mapping elements correctly.
	addPart(g, "ParameterType", newElem("DataTypes$UnknownType", ""))
	addStr(g, "OperationName", "")
	addStr(g, "ServiceName", "")
	addStr(g, "WsdlFile", "")
	return g
}

func importMappingElementToGen(elem *model.ImportMappingElement, parentPath string) element.Element {
	id := string(elem.ID)
	if elem.Kind == "Object" || elem.Kind == "Array" {
		return importObjectElementToGen(id, elem, parentPath)
	}
	return importValueElementToGen(id, elem, parentPath)
}

func importObjectElementToGen(id string, elem *model.ImportMappingElement, parentPath string) element.Element {
	jsonPath := elem.JsonPath
	if jsonPath == "" {
		if elem.ExposedName == "" {
			jsonPath = parentPath
		} else {
			jsonPath = parentPath + "|" + elem.ExposedName
		}
	}

	objectHandling := orDefault(elem.ObjectHandling, "Create")
	objectHandlingBackup := objectHandling
	if objectHandling == "FindOrCreate" {
		objectHandling = "Find"
		objectHandlingBackup = "Create"
	}

	// $Type is ObjectMappingElement (no "Import" prefix); the generated metamodel
	// name is misleading and causes TypeCacheUnknownTypeException if used.
	g := newElem("ImportMappings$ObjectMappingElement", id)
	addStr(g, "Entity", elem.Entity)
	addStr(g, "ExposedName", elem.ExposedName)
	addStr(g, "JsonPath", jsonPath)
	addStr(g, "XmlPath", "")
	addStr(g, "ObjectHandling", objectHandling)
	addStr(g, "ObjectHandlingBackup", objectHandlingBackup)
	addBool(g, "ObjectHandlingBackupAllowOverride", false)
	addStr(g, "Association", elem.Association)

	children := make([]element.Element, 0, len(elem.Children))
	for _, c := range elem.Children {
		children = append(children, importMappingElementToGen(c, jsonPath))
	}
	addPartList(g, "Children", children)

	addInt32(g, "MinOccurs", int32(elem.MinOccurs))
	addInt32(g, "MaxOccurs", int32(elem.MaxOccurs))
	addBool(g, "Nillable", elem.Nillable)
	addBool(g, "IsDefaultType", false)
	addStr(g, "ElementType", elementTypeForKind(elem.Kind))
	addStr(g, "Documentation", "")
	return g
}

func importValueElementToGen(id string, elem *model.ImportMappingElement, parentPath string) element.Element {
	jsonPath := elem.JsonPath
	if jsonPath == "" {
		jsonPath = parentPath + "|" + elem.ExposedName
	}

	g := newElem("ImportMappings$ValueMappingElement", id)
	addStr(g, "Attribute", elem.Attribute)
	addStr(g, "ExposedName", elem.ExposedName)
	addStr(g, "JsonPath", jsonPath)
	addStr(g, "XmlPath", "")
	addBool(g, "IsKey", elem.IsKey)
	addPart(g, "Type", mappingValueDataTypeToGen(elem.DataType))
	addInt32(g, "MinOccurs", int32(elem.MinOccurs))
	addInt32(g, "MaxOccurs", int32(elem.MaxOccurs))
	addBool(g, "Nillable", elem.Nillable)
	addBool(g, "IsDefaultType", false)
	addStr(g, "ElementType", "Value")
	addStr(g, "Documentation", "")
	addStr(g, "Converter", "")
	addInt32(g, "FractionDigits", int32(elem.FractionDigits))
	addInt32(g, "TotalDigits", int32(elem.TotalDigits))
	addInt32(g, "MaxLength", int32(elem.MaxLength))
	addBool(g, "IsContent", false)
	addBool(g, "IsXmlAttribute", false)
	addStr(g, "OriginalValue", elem.OriginalValue)
	addStr(g, "XmlPrimitiveType", xmlPrimitiveTypeName(elem.DataType))
	return g
}

// ---------------------------------------------------------------------------
// Export mappings
// ---------------------------------------------------------------------------

// CreateExportMapping inserts a new ExportMappings$ExportMapping document.
func (b *Backend) CreateExportMapping(em *model.ExportMapping) error {
	if em == nil {
		return fmt.Errorf("CreateExportMapping: nil export mapping")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateExportMapping: not connected for writing")
	}
	if em.ID == "" {
		em.ID = model.ID(mmpr.GenerateID())
	}
	em.TypeName = "ExportMappings$ExportMapping"
	contents, err := (&codec.Encoder{}).Encode(exportMappingToGen(em))
	if err != nil {
		return fmt.Errorf("CreateExportMapping: encode: %w", err)
	}
	return b.writer.InsertUnit(string(em.ID), string(em.ContainerID), "Documents", "ExportMappings$ExportMapping", contents)
}

// UpdateExportMapping rewrites an existing export mapping in place (CREATE OR MODIFY).
func (b *Backend) UpdateExportMapping(em *model.ExportMapping) error {
	if em == nil {
		return fmt.Errorf("UpdateExportMapping: nil export mapping")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateExportMapping: not connected for writing")
	}
	contents, err := (&codec.Encoder{}).Encode(exportMappingToGen(em))
	if err != nil {
		return fmt.Errorf("UpdateExportMapping: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(em.ID), contents)
}

// DeleteExportMapping removes an export mapping unit by ID.
func (b *Backend) DeleteExportMapping(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteExportMapping: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// GetExportMappingByQualifiedName finds an export mapping by module + name (used
// by the CREATE OR MODIFY existence check and DROP).
func (b *Backend) GetExportMappingByQualifiedName(moduleName, name string) (*model.ExportMapping, error) {
	all, err := b.ListExportMappings()
	if err != nil {
		return nil, err
	}
	for _, em := range all {
		if em.Name == name && b.moduleNameFor(em.ID) == moduleName {
			return em, nil
		}
	}
	return nil, fmt.Errorf("export mapping not found: %s.%s", moduleName, name)
}

func exportMappingToGen(em *model.ExportMapping) element.Element {
	g := newElem("ExportMappings$ExportMapping", string(em.ID))
	addStr(g, "Name", em.Name)
	addStr(g, "Documentation", em.Documentation)
	addBool(g, "Excluded", em.Excluded)
	addStr(g, "ExportLevel", orDefault(em.ExportLevel, "Hidden"))
	addStr(g, "JsonStructure", em.JsonStructure)
	addStr(g, "XmlSchema", em.XmlSchema)
	addStr(g, "MessageDefinition", em.MessageDefinition)
	addStr(g, "NullValueOption", orDefault(em.NullValueOption, "LeaveOutElement"))

	elems := make([]element.Element, 0, len(em.Elements))
	for _, e := range em.Elements {
		elems = append(elems, exportMappingElementToGen(e, "(Object)"))
	}
	addPartList(g, "Elements", elems)

	addStr(g, "PublicName", "")
	addStr(g, "XsdRootElementName", "")
	addBool(g, "IsHeaderParameter", false)
	addStr(g, "ParameterName", "")
	addStr(g, "OperationName", "")
	addStr(g, "ServiceName", "")
	addStr(g, "WsdlFile", "")
	return g
}

func exportMappingElementToGen(elem *model.ExportMappingElement, parentPath string) element.Element {
	id := string(elem.ID)
	if elem.Kind == "Object" || elem.Kind == "Array" {
		return exportObjectElementToGen(id, elem, parentPath)
	}
	return exportValueElementToGen(id, elem, parentPath)
}

func exportObjectElementToGen(id string, elem *model.ExportMappingElement, parentPath string) element.Element {
	jsonPath := elem.JsonPath
	if jsonPath == "" {
		if elem.ExposedName == "" {
			jsonPath = parentPath
		} else {
			jsonPath = parentPath + "|" + elem.ExposedName
		}
	}

	objectHandling := orDefault(elem.ObjectHandling, "Parameter")

	g := newElem("ExportMappings$ObjectMappingElement", id)
	addStr(g, "Entity", elem.Entity)
	addStr(g, "ExposedName", elem.ExposedName)
	addStr(g, "JsonPath", jsonPath)
	addStr(g, "XmlPath", "")
	addStr(g, "ObjectHandling", objectHandling)
	addStr(g, "ObjectHandlingBackup", objectHandling)
	addBool(g, "ObjectHandlingBackupAllowOverride", false)
	addStr(g, "Association", elem.Association)

	children := make([]element.Element, 0, len(elem.Children))
	for _, c := range elem.Children {
		children = append(children, exportMappingElementToGen(c, jsonPath))
	}
	addPartList(g, "Children", children)

	addInt32(g, "MinOccurs", 0)
	addInt32(g, "MaxOccurs", int32(elem.MaxOccurs))
	addBool(g, "Nillable", true)
	addBool(g, "IsDefaultType", false)
	addStr(g, "ElementType", elementTypeForKind(elem.Kind))
	addStr(g, "Documentation", "")
	return g
}

func exportValueElementToGen(id string, elem *model.ExportMappingElement, parentPath string) element.Element {
	jsonPath := elem.JsonPath
	if jsonPath == "" {
		jsonPath = parentPath + "|" + elem.ExposedName
	}

	g := newElem("ExportMappings$ValueMappingElement", id)
	addStr(g, "Attribute", elem.Attribute)
	addStr(g, "ExposedName", elem.ExposedName)
	addStr(g, "JsonPath", jsonPath)
	addStr(g, "XmlPath", "")
	addPart(g, "Type", mappingValueDataTypeToGen(elem.DataType))
	addInt32(g, "MinOccurs", 0)
	addInt32(g, "MaxOccurs", 0)
	addBool(g, "Nillable", true)
	addBool(g, "IsDefaultType", false)
	addStr(g, "ElementType", "Value")
	addStr(g, "Documentation", "")
	addStr(g, "Converter", "")
	addInt32(g, "FractionDigits", -1)
	addInt32(g, "TotalDigits", -1)
	addInt32(g, "MaxLength", 0)
	addBool(g, "IsContent", false)
	addBool(g, "IsXmlAttribute", false)
	addStr(g, "OriginalValue", "")
	addStr(g, "XmlPrimitiveType", xmlPrimitiveTypeName(elem.DataType))
	return g
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// mappingValueDataTypeToGen builds the DataTypes$* sub-object for a value
// element's Type property. Shared by import and export (identical type names).
func mappingValueDataTypeToGen(dataType string) element.Element {
	var typeName string
	switch dataType {
	case "Integer", "Long":
		typeName = "DataTypes$IntegerType"
	case "Decimal":
		typeName = "DataTypes$DecimalType"
	case "Boolean":
		typeName = "DataTypes$BooleanType"
	case "DateTime":
		typeName = "DataTypes$DateTimeType"
	case "Binary":
		typeName = "DataTypes$BinaryType"
	default:
		typeName = "DataTypes$StringType"
	}
	return newElem(typeName, "")
}

// xmlPrimitiveTypeName maps a value element's data type to its XML primitive name.
func xmlPrimitiveTypeName(dataType string) string {
	switch dataType {
	case "Integer", "Long":
		return "Integer"
	case "Decimal":
		return "Decimal"
	case "Boolean":
		return "Boolean"
	case "DateTime":
		return "DateTime"
	default:
		return "String"
	}
}

// elementTypeForKind maps a model element Kind to its BSON ElementType value.
func elementTypeForKind(kind string) string {
	switch kind {
	case "Array":
		return "Array"
	case "Value":
		return "Value"
	default:
		return "Object"
	}
}
