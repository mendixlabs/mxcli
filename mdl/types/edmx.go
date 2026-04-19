// SPDX-License-Identifier: Apache-2.0

package types

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// EdmxDocument represents a parsed OData $metadata document (EDMX/CSDL).
// Supports both OData v3 (CSDL 2.0/3.0) and OData v4 (CSDL 4.0).
type EdmxDocument struct {
	Version    string          // "1.0" (OData3) or "4.0" (OData4)
	Schemas    []*EdmSchema    // Schema definitions
	EntitySets []*EdmEntitySet // Entity sets from EntityContainer
	Actions    []*EdmAction    // OData4 actions / OData3 function imports
}

// EdmSchema represents an EDM schema namespace.
type EdmSchema struct {
	Namespace   string
	EntityTypes []*EdmEntityType
	EnumTypes   []*EdmEnumType
}

// EdmEntityType represents an entity type definition.
type EdmEntityType struct {
	Name                 string
	BaseType             string // Qualified name of base type (e.g. "Microsoft...PlanItem"), empty if none
	IsAbstract           bool   // True if <EntityType Abstract="true">
	IsOpen               bool   // True if <EntityType OpenType="true">
	KeyProperties        []string
	Properties           []*EdmProperty
	NavigationProperties []*EdmNavigationProperty
	Summary              string
	Description          string
}

// EdmProperty represents a property on an entity type.
type EdmProperty struct {
	Name      string
	Type      string // e.g. "Edm.String", "Edm.Int64"
	Nullable  *bool  // nil = not specified (default true)
	MaxLength string // e.g. "200", "max"
	Scale     string // e.g. "variable"

	// Capability annotations (OData Core V1). When true, the property is not
	// settable by the client:
	//   Computed  = server-computed, not settable on create or update.
	//   Immutable = settable on create, but not on update.
	Computed  bool
	Immutable bool
}

// EdmNavigationProperty represents a navigation property (association).
type EdmNavigationProperty struct {
	Name           string
	Type           string // OData4: "DefaultNamespace.Customer" or "Collection(DefaultNamespace.Part)"
	Partner        string // OData4 partner property name
	TargetType     string // Resolved target entity type name (without namespace/Collection)
	IsMany         bool   // true if Collection()
	ContainsTarget bool   // true if <NavigationProperty ContainsTarget="true">
	// OData3 fields (from Association)
	Relationship string
	FromRole     string
	ToRole       string
}

// EdmEntitySet represents an entity set in the entity container.
type EdmEntitySet struct {
	Name       string
	EntityType string // Qualified name of entity type

	// Capabilities derived from Org.OData.Capabilities.V1 annotations.
	// nil = not specified (treat as default true).
	Insertable *bool // InsertRestrictions/Insertable
	Updatable  *bool // UpdateRestrictions/Updatable
	Deletable  *bool // DeleteRestrictions/Deletable

	// Navigation property names listed under
	// Org.OData.Capabilities.V1.{Insert,Update}Restrictions/Non*NavigationProperties.
	NonInsertableNavigationProperties []string
	NonUpdatableNavigationProperties  []string

	// Property names listed under
	// Org.OData.Capabilities.V1.{Insert,Update}Restrictions/Non*Properties.
	// Structural properties named here cannot be set on insert / update.
	NonInsertableProperties []string
	NonUpdatableProperties  []string
}

// EdmAction represents an OData4 action or OData3 function import.
type EdmAction struct {
	Name       string
	IsBound    bool
	Parameters []*EdmActionParameter
	ReturnType string
}

// EdmActionParameter represents a parameter of an action.
type EdmActionParameter struct {
	Name     string
	Type     string
	Nullable *bool
}

// EdmEnumType represents an enumeration type.
type EdmEnumType struct {
	Name    string
	Members []*EdmEnumMember
}

// EdmEnumMember represents a member of an enum type.
type EdmEnumMember struct {
	Name  string
	Value string
}

// FindEntityType looks up an entity type by name (with or without namespace prefix).
func (d *EdmxDocument) FindEntityType(name string) *EdmEntityType {
	// Strip namespace prefix if present
	shortName := name
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		shortName = name[idx+1:]
	}
	for _, s := range d.Schemas {
		for _, et := range s.EntityTypes {
			if et.Name == shortName {
				return et
			}
		}
	}
	return nil
}

// ParseEdmx parses an OData $metadata XML string into an EdmxDocument.
func ParseEdmx(metadataXML string) (*EdmxDocument, error) {
	if metadataXML == "" {
		return nil, fmt.Errorf("empty metadata XML")
	}

	var edmx xmlEdmx
	if err := xml.Unmarshal([]byte(metadataXML), &edmx); err != nil {
		return nil, fmt.Errorf("failed to parse EDMX XML: %w", err)
	}

	doc := &EdmxDocument{
		Version: edmx.Version,
	}

	for _, ds := range edmx.DataServices {
		for _, s := range ds.Schemas {
			schema := &EdmSchema{
				Namespace: s.Namespace,
			}

			// Parse entity types
			for _, et := range s.EntityTypes {
				entityType := parseXmlEntityType(&et)
				schema.EntityTypes = append(schema.EntityTypes, entityType)
			}

			// Parse enum types
			for _, en := range s.EnumTypes {
				enumType := &EdmEnumType{Name: en.Name}
				for _, m := range en.Members {
					enumType.Members = append(enumType.Members, &EdmEnumMember{
						Name:  m.Name,
						Value: m.Value,
					})
				}
				schema.EnumTypes = append(schema.EnumTypes, enumType)
			}

			doc.Schemas = append(doc.Schemas, schema)

			// Parse entity container
			for _, ec := range s.EntityContainers {
				for _, es := range ec.EntitySets {
					entitySet := &EdmEntitySet{
						Name:       es.Name,
						EntityType: es.EntityType,
					}
					applyCapabilityAnnotations(entitySet, es.Annotations)
					doc.EntitySets = append(doc.EntitySets, entitySet)
				}

				// OData3 function imports
				for _, fi := range ec.FunctionImports {
					action := &EdmAction{
						Name:       fi.Name,
						ReturnType: fi.ReturnType,
					}
					for _, p := range fi.Parameters {
						action.Parameters = append(action.Parameters, &EdmActionParameter{
							Name: p.Name,
							Type: p.Type,
						})
					}
					doc.Actions = append(doc.Actions, action)
				}
			}

			// OData4 actions
			for _, a := range s.Actions {
				action := &EdmAction{
					Name:    a.Name,
					IsBound: a.IsBound == "true",
				}
				if a.ReturnType != nil {
					action.ReturnType = a.ReturnType.Type
				}
				for _, p := range a.Parameters {
					param := &EdmActionParameter{
						Name: p.Name,
						Type: p.Type,
					}
					if p.Nullable != "" {
						v := p.Nullable == "true"
						param.Nullable = &v
					}
					action.Parameters = append(action.Parameters, param)
				}
				doc.Actions = append(doc.Actions, action)
			}

			// OData4 functions (treated same as actions for discovery)
			for _, f := range s.Functions {
				action := &EdmAction{
					Name:    f.Name,
					IsBound: f.IsBound == "true",
				}
				if f.ReturnType != nil {
					action.ReturnType = f.ReturnType.Type
				}
				for _, p := range f.Parameters {
					param := &EdmActionParameter{
						Name: p.Name,
						Type: p.Type,
					}
					action.Parameters = append(action.Parameters, param)
				}
				doc.Actions = append(doc.Actions, action)
			}
		}
	}

	return doc, nil
}

func parseXmlEntityType(et *xmlEntityType) *EdmEntityType {
	entityType := &EdmEntityType{
		Name:       et.Name,
		BaseType:   et.BaseType,
		IsAbstract: et.Abstract == "true",
		IsOpen:     et.OpenType == "true",
	}

	// Parse key
	if et.Key != nil {
		for _, pr := range et.Key.PropertyRefs {
			entityType.KeyProperties = append(entityType.KeyProperties, pr.Name)
		}
	}

	// Parse documentation (OData3 style)
	if et.Documentation != nil {
		entityType.Summary = et.Documentation.Summary
		entityType.Description = et.Documentation.LongDescription
	}

	// Parse annotations (OData4 style)
	for _, ann := range et.Annotations {
		switch ann.Term {
		case "Org.OData.Core.V1.Description":
			entityType.Summary = ann.String
		case "Org.OData.Core.V1.LongDescription":
			entityType.Description = ann.String
		}
	}

	// Parse properties
	for _, p := range et.Properties {
		prop := &EdmProperty{
			Name:      p.Name,
			Type:      p.Type,
			MaxLength: p.MaxLength,
			Scale:     p.Scale,
		}
		if p.Nullable != "" {
			v := p.Nullable != "false"
			prop.Nullable = &v
		}
		for _, ann := range p.Annotations {
			switch ann.Term {
			case "Org.OData.Core.V1.Computed":
				prop.Computed = ann.Bool == "" || ann.Bool == "true"
			case "Org.OData.Core.V1.Immutable":
				prop.Immutable = ann.Bool == "" || ann.Bool == "true"
			}
		}
		entityType.Properties = append(entityType.Properties, prop)
	}

	// Parse navigation properties
	for _, np := range et.NavigationProperties {
		nav := &EdmNavigationProperty{
			Name:           np.Name,
			Type:           np.Type,
			Partner:        np.Partner,
			ContainsTarget: np.ContainsTarget == "true",
			Relationship:   np.Relationship,
			FromRole:       np.FromRole,
			ToRole:         np.ToRole,
		}

		// Resolve target type from OData4 Type field
		if np.Type != "" {
			nav.TargetType, nav.IsMany = ResolveNavType(np.Type)
		}

		entityType.NavigationProperties = append(entityType.NavigationProperties, nav)
	}

	return entityType
}

// applyCapabilityAnnotations reads Org.OData.Capabilities.V1.{Insert,Update,
// Delete}Restrictions annotations on an entity set and stores the relevant
// flags on the EdmEntitySet.
func applyCapabilityAnnotations(es *EdmEntitySet, annotations []xmlCapabilitiesAnnotation) {
	for _, ann := range annotations {
		if ann.Record == nil {
			continue
		}
		switch ann.Term {
		case "Org.OData.Capabilities.V1.InsertRestrictions":
			for _, pv := range ann.Record.PropertyValues {
				switch pv.Property {
				case "Insertable":
					if pv.Bool != "" {
						v := pv.Bool == "true"
						es.Insertable = &v
					}
				case "NonInsertableNavigationProperties":
					if pv.Collection != nil {
						es.NonInsertableNavigationProperties = pv.Collection.NavigationPropertyPaths
					}
				case "NonInsertableProperties":
					if pv.Collection != nil {
						es.NonInsertableProperties = pv.Collection.PropertyPaths
					}
				}
			}
		case "Org.OData.Capabilities.V1.UpdateRestrictions":
			for _, pv := range ann.Record.PropertyValues {
				switch pv.Property {
				case "Updatable":
					if pv.Bool != "" {
						v := pv.Bool == "true"
						es.Updatable = &v
					}
				case "NonUpdatableNavigationProperties":
					if pv.Collection != nil {
						es.NonUpdatableNavigationProperties = pv.Collection.NavigationPropertyPaths
					}
				case "NonUpdatableProperties":
					if pv.Collection != nil {
						es.NonUpdatableProperties = pv.Collection.PropertyPaths
					}
				}
			}
		case "Org.OData.Capabilities.V1.DeleteRestrictions":
			for _, pv := range ann.Record.PropertyValues {
				if pv.Property == "Deletable" && pv.Bool != "" {
					v := pv.Bool == "true"
					es.Deletable = &v
				}
			}
		}
	}
}

// ResolveNavType parses "Collection(Namespace.Type)" or "Namespace.Type" into the short type name.
func ResolveNavType(t string) (typeName string, isMany bool) {
	if strings.HasPrefix(t, "Collection(") && strings.HasSuffix(t, ")") {
		isMany = true
		t = t[len("Collection(") : len(t)-1]
	}
	if idx := strings.LastIndex(t, "."); idx >= 0 {
		typeName = t[idx+1:]
	} else {
		typeName = t
	}
	return
}

// ============================================================================
// XML deserialization types (internal)
// ============================================================================

type xmlEdmx struct {
	XMLName      xml.Name         `xml:"Edmx"`
	Version      string           `xml:"Version,attr"`
	DataServices []xmlDataService `xml:"DataServices"`
}

type xmlDataService struct {
	Schemas []xmlSchema `xml:"Schema"`
}

type xmlSchema struct {
	Namespace        string               `xml:"Namespace,attr"`
	EntityTypes      []xmlEntityType      `xml:"EntityType"`
	EnumTypes        []xmlEnumType        `xml:"EnumType"`
	EntityContainers []xmlEntityContainer `xml:"EntityContainer"`
	Actions          []xmlAction          `xml:"Action"`
	Functions        []xmlAction          `xml:"Function"`
}

type xmlEntityType struct {
	Name                 string                  `xml:"Name,attr"`
	BaseType             string                  `xml:"BaseType,attr"`
	Abstract             string                  `xml:"Abstract,attr"`
	OpenType             string                  `xml:"OpenType,attr"`
	Key                  *xmlKey                 `xml:"Key"`
	Properties           []xmlProperty           `xml:"Property"`
	NavigationProperties []xmlNavigationProperty `xml:"NavigationProperty"`
	Documentation        *xmlDocumentation       `xml:"Documentation"`
	Annotations          []xmlAnnotation         `xml:"Annotation"`
}

type xmlKey struct {
	PropertyRefs []xmlPropertyRef `xml:"PropertyRef"`
}

type xmlPropertyRef struct {
	Name string `xml:"Name,attr"`
}

type xmlProperty struct {
	Name        string          `xml:"Name,attr"`
	Type        string          `xml:"Type,attr"`
	Nullable    string          `xml:"Nullable,attr"`
	MaxLength   string          `xml:"MaxLength,attr"`
	Scale       string          `xml:"Scale,attr"`
	Annotations []xmlAnnotation `xml:"Annotation"`
}

type xmlNavigationProperty struct {
	Name           string `xml:"Name,attr"`
	Type           string `xml:"Type,attr"`           // OData4
	Partner        string `xml:"Partner,attr"`        // OData4
	ContainsTarget string `xml:"ContainsTarget,attr"` // OData4: contained nav target (e.g. Person.Trips)
	Relationship   string `xml:"Relationship,attr"`   // OData3
	FromRole       string `xml:"FromRole,attr"`       // OData3
	ToRole         string `xml:"ToRole,attr"`         // OData3
}

type xmlDocumentation struct {
	Summary         string `xml:"Summary"`
	LongDescription string `xml:"LongDescription"`
}

type xmlAnnotation struct {
	Term   string `xml:"Term,attr"`
	String string `xml:"String,attr"`
	Bool   string `xml:"Bool,attr"`
}

type xmlEntityContainer struct {
	Name            string              `xml:"Name,attr"`
	EntitySets      []xmlEntitySet      `xml:"EntitySet"`
	FunctionImports []xmlFunctionImport `xml:"FunctionImport"`
}

type xmlEntitySet struct {
	Name        string                      `xml:"Name,attr"`
	EntityType  string                      `xml:"EntityType,attr"`
	Annotations []xmlCapabilitiesAnnotation `xml:"Annotation"`
}

// xmlCapabilitiesAnnotation captures the bits of OData V1 Capabilities
// annotations we care about. The wrapping <Record> contains
// <PropertyValue Property="Insertable" Bool="..."/> and (sometimes)
// <PropertyValue Property="NonInsertableNavigationProperties"><Collection>
// <NavigationPropertyPath>Trips</NavigationPropertyPath></Collection></PropertyValue>.
type xmlCapabilitiesAnnotation struct {
	Term   string                 `xml:"Term,attr"`
	Record *xmlCapabilitiesRecord `xml:"Record"`
}

type xmlCapabilitiesRecord struct {
	PropertyValues []xmlCapabilitiesPropertyValue `xml:"PropertyValue"`
}

type xmlCapabilitiesPropertyValue struct {
	Property   string                     `xml:"Property,attr"`
	Bool       string                     `xml:"Bool,attr"`
	Collection *xmlCapabilitiesCollection `xml:"Collection"`
}

type xmlCapabilitiesCollection struct {
	NavigationPropertyPaths []string `xml:"NavigationPropertyPath"`
	PropertyPaths           []string `xml:"PropertyPath"`
}

type xmlFunctionImport struct {
	Name       string           `xml:"Name,attr"`
	ReturnType string           `xml:"ReturnType,attr"`
	Parameters []xmlActionParam `xml:"Parameter"`
}

type xmlAction struct {
	Name       string           `xml:"Name,attr"`
	IsBound    string           `xml:"IsBound,attr"`
	ReturnType *xmlReturnType   `xml:"ReturnType"`
	Parameters []xmlActionParam `xml:"Parameter"`
}

type xmlReturnType struct {
	Type     string `xml:"Type,attr"`
	Nullable string `xml:"Nullable,attr"`
}

type xmlActionParam struct {
	Name     string `xml:"Name,attr"`
	Type     string `xml:"Type,attr"`
	Nullable string `xml:"Nullable,attr"`
}

type xmlEnumType struct {
	Name    string          `xml:"Name,attr"`
	Members []xmlEnumMember `xml:"Member"`
}

type xmlEnumMember struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}
