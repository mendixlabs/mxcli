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
	Namespace    string
	EntityTypes  []*EdmEntityType
	ComplexTypes []*EdmComplexType
	EnumTypes    []*EdmEnumType
}

// EdmComplexType represents a <ComplexType> — a keyless structured value.
//
// The Mendix domain model has no complex types. Studio Pro imports the
// PROPERTIES of one as attributes of the containing entity, named
// `<complexProperty>_<leaf>`; see EdmxDocument.FlattenProperties. Parsing them
// is what makes that possible: with no ComplexType in the model, a property
// typed `Shared.Uom.Quantity` is indistinguishable from a property of a type
// nothing knows about, and every consumer drops it (mendixlabs/mxcli#1118).
type EdmComplexType struct {
	Name       string
	BaseType   string // Qualified name of the base complex type, empty if none
	Properties []*EdmProperty
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

	// RemotePath is how the SERVICE addresses this property when it was reached
	// through a complex-typed property: "MaxQty/UoMNId". Empty on a property the
	// entity type declares directly, where the path is just the name — use
	// Path() rather than reading this field, so the two cases stay one lookup.
	RemotePath string

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
	Countable  *bool // CountRestrictions/Countable
	// TopSupported / SkipSupported are STANDALONE boolean annotations in the
	// OData capabilities vocabulary, not records:
	//   <Annotation Bool="false" Term="Org.OData.Capabilities.V1.TopSupported"/>
	// Mendix compares them against the app's per-entity flags and reports
	// CE6630 on a mismatch, so a generator that stamps them true regardless
	// makes `TopSupported: No` on the publishing side unusable.
	TopSupported  *bool
	SkipSupported *bool

	// Property names the service says cannot be filtered or sorted on, from
	// FilterRestrictions/NonFilterableProperties and
	// SortRestrictions/NonSortableProperties. Mendix compares these against the
	// app's per-attribute flags and reports CE6630 on a mismatch.
	NonFilterableProperties []string
	NonSortableProperties   []string

	// Filterable / Sortable are the WHOLE-SET form of the same two restrictions,
	// carried as the record's own Bool property:
	//
	//	<Annotation Term="…FilterRestrictions"><Record>
	//	  <PropertyValue Bool="false" Property="Filterable"/>
	//	</Record></Annotation>
	//
	// Mendix picks the shape by arithmetic, not by preference: it lists
	// NonFilterableProperties when SOME attributes are filterable, and emits the
	// bare Bool when NONE are, because then there is no list to write. Both
	// appear in one document. nil means unstated, which is OData's default of
	// true (mxcli-formula1 §48).
	Filterable *bool
	Sortable   *bool

	// Use AttrFilterable / AttrSortable rather than reading the four fields
	// above: a consumer that consults only one shape generates an app the
	// publisher's own contract contradicts.

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

// FindEnumType looks up an enum type by name (with or without namespace prefix).
//
// It exists to tell an ENUM apart from a complex type or a type definition when
// classifying an external action's parameter: Mendix builds an enum-typed
// parameter at 0 errors and refuses the other two (CE7255), so a lookup that
// cannot distinguish them refuses something that works.
func (d *EdmxDocument) FindEnumType(name string) *EdmEnumType {
	shortName := name
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		shortName = name[idx+1:]
	}
	for _, s := range d.Schemas {
		for _, et := range s.EnumTypes {
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
	// Every term below is matched by its full namespace; read aliased ones
	// through the document's aliases first (ako/mxcli#1977).
	normalizeAnnotationTerms(&edmx)

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

			// Parse complex types
			for _, ct := range s.ComplexTypes {
				complexType := &EdmComplexType{Name: ct.Name, BaseType: ct.BaseType}
				for i := range ct.Properties {
					complexType.Properties = append(complexType.Properties, parseXmlProperty(&ct.Properties[i]))
				}
				schema.ComplexTypes = append(schema.ComplexTypes, complexType)
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

	// Second pass: apply schema-level external annotations to entity sets.
	// Many OData 4.0 services place capability annotations in <Annotations Target="...">
	// blocks at the schema level rather than inline within <EntitySet>.
	// Target format: "Namespace.ContainerName/EntitySetName" or "ContainerName/EntitySetName".
	if len(doc.EntitySets) > 0 {
		esByName := make(map[string]*EdmEntitySet, len(doc.EntitySets))
		for _, es := range doc.EntitySets {
			esByName[es.Name] = es
		}
		for _, ds := range edmx.DataServices {
			for _, s := range ds.Schemas {
				for _, ext := range s.Annotations {
					// Extract the entity set name: take the part after the last "/".
					target := ext.Target
					if idx := strings.LastIndex(target, "/"); idx >= 0 {
						target = target[idx+1:]
					}
					if es, ok := esByName[target]; ok {
						applyCapabilityAnnotations(es, ext.Annotations)
					}
				}
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
	for i := range et.Properties {
		entityType.Properties = append(entityType.Properties, parseXmlProperty(&et.Properties[i]))
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

// parseXmlProperty converts one <Property> element. Shared by entity types and
// complex types: a complex type's properties are turned into entity attributes
// verbatim, facets and capability annotations included, so a second copy of this
// would drift the two apart (the duplicate-resolver failure class).
func parseXmlProperty(p *xmlProperty) *EdmProperty {
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
	// ConcurrencyMode="Fixed" (OData v3) marks a property as an optimistic
	// concurrency token — the server manages it, the client cannot set it.
	if p.ConcurrencyMode == "Fixed" {
		prop.Computed = true
	}
	for _, ann := range p.Annotations {
		switch ann.Term {
		case "Org.OData.Core.V1.Computed":
			prop.Computed = ann.Bool == "" || ann.Bool == "true"
		case "Org.OData.Core.V1.Immutable":
			prop.Immutable = ann.Bool == "" || ann.Bool == "true"
		}
	}
	return prop
}

// Path returns how the OData service addresses this property — the name for a
// property the entity type declares itself, "MaxQty/UoMNId" for one reached
// through a complex-typed property. It is what belongs in the Mendix attribute's
// RemoteName, and what a capability annotation's PropertyPath names.
func (p *EdmProperty) Path() string {
	if p.RemotePath != "" {
		return p.RemotePath
	}
	return p.Name
}

// FindComplexType resolves a complex type by its QUALIFIED name.
//
// Qualified, not short: one $metadata document may declare `Quantity` in two
// namespaces, and a short-name lookup would hand the entity whichever schema
// happened to parse first — a wrong set of attributes rather than a missing one,
// which is strictly harder to notice. FindEntityType's short-name fallback is
// deliberately not copied here.
func (d *EdmxDocument) FindComplexType(qualifiedName string) *EdmComplexType {
	idx := strings.LastIndex(qualifiedName, ".")
	if idx < 0 {
		return nil
	}
	namespace, name := qualifiedName[:idx], qualifiedName[idx+1:]
	for _, s := range d.Schemas {
		if s.Namespace != namespace {
			continue
		}
		for _, ct := range s.ComplexTypes {
			if ct.Name == name {
				return ct
			}
		}
	}
	return nil
}

// inheritedComplexProperties returns the properties a complex type inherits
// from its BaseType chain — the ones that are NOT importable, so that a caller
// can name them instead of dropping them in silence.
//
// Mendix imports a complex type's OWN properties only. Measured on mxbuild
// 11.12.1 against TripPin, whose `AirportLocation` and `EventLocation` both
// derive from `Location`: flattening the inherited `Address` gives
//
//	CE6615 "Attribute 'Location_Address' of external entity 'Airports' does
//	        not exist in the OData service."
//
// while the same `Address` reached through `Person.HomeAddress`, typed
// `Location` directly, is recognised — and each derived type's OWN property
// (`Loc`, `BuildingInfo`) is recognised too. So the line is inheritance, not
// the path syntax.
//
// The depth guard is for a document whose base types form a cycle — malformed,
// but it arrives over the network and must not hang the import.
func (d *EdmxDocument) inheritedComplexProperties(ct *EdmComplexType) []*EdmProperty {
	var props []*EdmProperty
	seen := map[*EdmComplexType]bool{}
	var walk func(*EdmComplexType)
	walk = func(c *EdmComplexType) {
		if c == nil || seen[c] {
			return
		}
		seen[c] = true
		props = append(props, c.Properties...)
		walk(d.FindComplexType(c.BaseType))
	}
	if ct != nil && ct.BaseType != "" {
		walk(d.FindComplexType(ct.BaseType))
	}
	return props
}

// importableEdmTypes are the primitive types Mendix maps to an attribute —
// Consumed OData Service Requirements' "Supported Attribute Types", which is
// also the list its complex-type paragraph defers to.
//
// It has to be a closed set, not `strings.HasPrefix(t, "Edm.")`: measured on
// 11.12.1, flattening TripPin's `AirportLocation.Loc` (Edm.GeographyPoint) is
//
//	CE6622 "The type of attribute 'Location_Loc' in the OData service is not
//	        supported. Please delete this attribute."
//
// Edm.Duration is absent deliberately — Mendix has no duration type, and the
// import has always skipped it.
var importableEdmTypes = map[string]bool{
	"Edm.String": true, "Edm.Boolean": true, "Edm.Guid": true, "Edm.Binary": true,
	"Edm.Byte": true, "Edm.SByte": true, "Edm.Int16": true, "Edm.Int32": true, "Edm.Int64": true,
	"Edm.Decimal": true, "Edm.Double": true, "Edm.Single": true,
	"Edm.Date": true, "Edm.DateTime": true, "Edm.DateTimeOffset": true,
}

// FlattenProperties expands every complex-typed property into one property per
// leaf, and passes everything else through untouched.
//
// This is what Studio Pro does on import, and the reason it has to exist here:
// "Complex types are not supported by the domain model. However, Studio Pro
// allows you to read external entities that contain attributes of a complex type
// by importing the properties of the complex type as attributes of the
// containing entity […] the attribute names consist of the name of the complex
// attribute and the name of the property that is part of the complex type,
// separated by an underscore" (Consumed OData Service Requirements). So
// `MaxQty` of type `Shared.Uom.Quantity` becomes `MaxQty_UoMNId` and
// `MaxQty_QuantityValue`, addressed over the paths `MaxQty/UoMNId` and
// `MaxQty/QuantityValue`.
//
// Three things are NOT importable, and each is returned in unsupported rather
// than dropped — a caller that says nothing is the defect this whole function
// exists to fix (mendixlabs/mxcli#1118):
//
//   - a leaf whose type is not in importableEdmTypes, which includes a complex
//     type nested in a complex type (flattening is one level deep, matching the
//     same page's "only the properties of the types described in Supported
//     Attribute Types are supported");
//   - a property INHERITED from the complex type's BaseType — see
//     inheritedComplexProperties for the measurement.
//
// A property whose type is not a complex type this document declares is passed
// through unchanged for the caller's own rules to judge.
func (d *EdmxDocument) FlattenProperties(props []*EdmProperty) (flat []*EdmProperty, unsupported []string) {
	for _, p := range props {
		ct := d.FindComplexType(p.Type)
		if ct == nil {
			flat = append(flat, p)
			continue
		}
		for _, leaf := range ct.Properties {
			path := p.Name + "/" + leaf.Name
			if !importableEdmTypes[leaf.Type] {
				reason := leaf.Type
				if d.FindComplexType(leaf.Type) != nil {
					reason = leaf.Type + ", a complex type nested in a complex type"
				}
				unsupported = append(unsupported, fmt.Sprintf("%s (%s)", path, reason))
				continue
			}
			expanded := *leaf
			expanded.Name = p.Name + "_" + leaf.Name
			expanded.RemotePath = path
			// The containing property's own capability annotations apply to
			// every leaf underneath it: a computed complex value has no
			// individually settable parts.
			expanded.Computed = expanded.Computed || p.Computed
			expanded.Immutable = expanded.Immutable || p.Immutable
			flat = append(flat, &expanded)
		}
		// Inherited properties are not importable — name them rather than let
		// them disappear, since a reader looking at the contract will expect
		// them and they are exactly what CE6615 fires on.
		for _, leaf := range d.inheritedComplexProperties(ct) {
			unsupported = append(unsupported, fmt.Sprintf("%s/%s (inherited from %s; Mendix imports a complex type's own properties only)",
				p.Name, leaf.Name, ct.BaseType))
		}
	}
	return flat, unsupported
}

// applyCapabilityAnnotations reads Org.OData.Capabilities.V1.{Insert,Update,
// Delete}Restrictions annotations on an entity set and stores the relevant
// flags on the EdmEntitySet.
func applyCapabilityAnnotations(es *EdmEntitySet, annotations []xmlCapabilitiesAnnotation) {
	for _, ann := range annotations {
		// Standalone boolean terms carry their value on the annotation itself.
		// Handled before the record guard, which skipped them entirely.
		if ann.Record == nil {
			if ann.Bool == "" {
				continue
			}
			v := ann.Bool == "true"
			switch ann.Term {
			case "Org.OData.Capabilities.V1.TopSupported":
				es.TopSupported = &v
			case "Org.OData.Capabilities.V1.SkipSupported":
				es.SkipSupported = &v
			}
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
		case "Org.OData.Capabilities.V1.CountRestrictions":
			for _, pv := range ann.Record.PropertyValues {
				if pv.Property == "Countable" && pv.Bool != "" {
					v := pv.Bool == "true"
					es.Countable = &v
				}
			}
		case "Org.OData.Capabilities.V1.FilterRestrictions":
			for _, pv := range ann.Record.PropertyValues {
				switch pv.Property {
				case "Filterable":
					// The whole-set form. Mendix emits this INSTEAD of a
					// NonFilterableProperties list when NO attribute is
					// filterable — there is nothing to enumerate — so reading
					// only the list makes an entirely unfilterable set look
					// entirely filterable.
					if pv.Bool != "" {
						v := pv.Bool == "true"
						es.Filterable = &v
					}
				case "NonFilterableProperties":
					if pv.Collection != nil {
						es.NonFilterableProperties = pv.Collection.PropertyPaths
					}
				}
			}
		case "Org.OData.Capabilities.V1.SortRestrictions":
			for _, pv := range ann.Record.PropertyValues {
				switch pv.Property {
				case "Sortable":
					if pv.Bool != "" {
						v := pv.Bool == "true"
						es.Sortable = &v
					}
				case "NonSortableProperties":
					if pv.Collection != nil {
						es.NonSortableProperties = pv.Collection.PropertyPaths
					}
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
	References   []xmlReference   `xml:"Reference"`
	DataServices []xmlDataService `xml:"DataServices"`
}

// xmlReference is an <edmx:Reference>: the vocabularies (and other documents)
// this one uses, each Include optionally giving its namespace an alias.
type xmlReference struct {
	Includes []xmlInclude `xml:"Include"`
}

type xmlInclude struct {
	Namespace string `xml:"Namespace,attr"`
	Alias     string `xml:"Alias,attr"`
}

type xmlDataService struct {
	Schemas []xmlSchema `xml:"Schema"`
}

type xmlSchema struct {
	Namespace        string                  `xml:"Namespace,attr"`
	Alias            string                  `xml:"Alias,attr"`
	EntityTypes      []xmlEntityType         `xml:"EntityType"`
	ComplexTypes     []xmlComplexType        `xml:"ComplexType"`
	EnumTypes        []xmlEnumType           `xml:"EnumType"`
	EntityContainers []xmlEntityContainer    `xml:"EntityContainer"`
	Actions          []xmlAction             `xml:"Action"`
	Functions        []xmlAction             `xml:"Function"`
	Annotations      []xmlExternalAnnotation `xml:"Annotations"`
}

// xmlExternalAnnotation represents a schema-level <Annotations Target="..."> block.
// Many OData 4.0 services (including Azure, SAP, and some Oasis reference services)
// place capability annotations here rather than inline within <EntitySet>.
type xmlExternalAnnotation struct {
	Target      string                      `xml:"Target,attr"`
	Annotations []xmlCapabilitiesAnnotation `xml:"Annotation"`
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

type xmlComplexType struct {
	Name       string        `xml:"Name,attr"`
	BaseType   string        `xml:"BaseType,attr"`
	Properties []xmlProperty `xml:"Property"`
}

type xmlKey struct {
	PropertyRefs []xmlPropertyRef `xml:"PropertyRef"`
}

type xmlPropertyRef struct {
	Name string `xml:"Name,attr"`
}

type xmlProperty struct {
	Name            string          `xml:"Name,attr"`
	Type            string          `xml:"Type,attr"`
	Nullable        string          `xml:"Nullable,attr"`
	MaxLength       string          `xml:"MaxLength,attr"`
	Scale           string          `xml:"Scale,attr"`
	ConcurrencyMode string          `xml:"ConcurrencyMode,attr"` // OData v3: "Fixed" = optimistic concurrency token
	Annotations     []xmlAnnotation `xml:"Annotation"`
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
	Term string `xml:"Term,attr"`
	// Bool carries a standalone boolean term (TopSupported, SkipSupported).
	// Empty when the annotation is record-shaped.
	Bool   string                 `xml:"Bool,attr"`
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

// AttrFilterable reports whether a client may filter on the named property.
//
// FilterRestrictions has TWO shapes and a consumer must honour both. Mendix
// chooses between them by arithmetic rather than preference: it lists
// NonFilterableProperties when SOME attributes are filterable, and emits a bare
// `Bool="false" Property="Filterable"` when NONE are, because then there is no
// list to write. Both shapes appear in one document, on different entity sets.
//
// Reading only the list marks every property of a wholly-unfilterable set as
// filterable, and the consuming app then fails to build — one CE6630 per
// property ("'message' is marked Sortable=False in the OData service, but True
// in the app"), 28 of them on one service (mxcli-formula1 §48). This is §42 one
// layer along, so the two live behind one call to keep them from drifting apart
// again.
//
// A nil receiver, or an unstated restriction, means OData's default: allowed.
func (es *EdmEntitySet) AttrFilterable(property string) bool {
	if es == nil {
		return true
	}
	if es.Filterable != nil && !*es.Filterable {
		return false
	}
	return !containsString(es.NonFilterableProperties, property)
}

// AttrSortable reports whether a client may order by the named property. See
// AttrFilterable — SortRestrictions carries the identical pair of shapes.
func (es *EdmEntitySet) AttrSortable(property string) bool {
	if es == nil {
		return true
	}
	if es.Sortable != nil && !*es.Sortable {
		return false
	}
	return !containsString(es.NonSortableProperties, property)
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// normalizeAnnotationTerms rewrites every annotation term written with a
// vocabulary alias to its full namespace, so the term switches below match it.
//
// CSDL lets a document alias a namespace — <edmx:Include
// Namespace="Org.OData.Capabilities.V1" Alias="Capabilities"/>, or a Schema's
// own Alias — and write `Term="Capabilities.UpdateRestrictions"`. Matched by
// the full namespace only, every such term was skipped: an entity set imported
// as not creatable / updatable / deletable, which mxbuild, resolving the alias,
// reports as CE6630 (ako/mxcli#1977). A prefix the document does not declare is
// left as written; it names no vocabulary this parser knows.
func normalizeAnnotationTerms(edmx *xmlEdmx) {
	aliases := map[string]string{}
	for _, ref := range edmx.References {
		for _, inc := range ref.Includes {
			if inc.Alias != "" && inc.Namespace != "" {
				aliases[inc.Alias] = inc.Namespace
			}
		}
	}
	for _, ds := range edmx.DataServices {
		for _, s := range ds.Schemas {
			if s.Alias != "" && s.Namespace != "" {
				aliases[s.Alias] = s.Namespace
			}
		}
	}
	if len(aliases) == 0 {
		return
	}
	canon := func(term string) string {
		name, qualifier, hasQualifier := strings.Cut(term, "#")
		i := strings.LastIndex(name, ".")
		if i <= 0 {
			return term
		}
		ns, ok := aliases[name[:i]]
		if !ok {
			return term
		}
		out := ns + name[i:]
		if hasQualifier {
			out += "#" + qualifier
		}
		return out
	}
	for d := range edmx.DataServices {
		for si := range edmx.DataServices[d].Schemas {
			s := &edmx.DataServices[d].Schemas[si]
			for i := range s.EntityTypes {
				et := &s.EntityTypes[i]
				for j := range et.Annotations {
					et.Annotations[j].Term = canon(et.Annotations[j].Term)
				}
				for j := range et.Properties {
					for k := range et.Properties[j].Annotations {
						et.Properties[j].Annotations[k].Term = canon(et.Properties[j].Annotations[k].Term)
					}
				}
			}
			for i := range s.ComplexTypes {
				for j := range s.ComplexTypes[i].Properties {
					p := &s.ComplexTypes[i].Properties[j]
					for k := range p.Annotations {
						p.Annotations[k].Term = canon(p.Annotations[k].Term)
					}
				}
			}
			for i := range s.EntityContainers {
				for j := range s.EntityContainers[i].EntitySets {
					es := &s.EntityContainers[i].EntitySets[j]
					for k := range es.Annotations {
						es.Annotations[k].Term = canon(es.Annotations[k].Term)
					}
				}
			}
			for i := range s.Annotations {
				for j := range s.Annotations[i].Annotations {
					s.Annotations[i].Annotations[j].Term = canon(s.Annotations[i].Annotations[j].Term)
				}
			}
		}
	}
}
