// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
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
	// A custom handler's ParameterMappings list serializes with marker 2, and
	// Studio Pro writes it even when the microflow takes NO parameters — the
	// bare [2], the same MandatoryLists rule as a rule document's Flows.
	// Dropping it is a diff against every stored document that has one (#264).
	codec.RegisterListMarker("Mappings$MicroflowCallParameterMappingImpl", 2)
	codec.RegisterTypeDefaults("Mappings$MappingMicroflowCallImpl", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
	})

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
	// MDL has no export-level spelling; the executor fills in "Hidden" (#816).
	contents, err = b.keepStoredExportLevel(string(im.ID), contents)
	if err != nil {
		return fmt.Errorf("UpdateImportMapping: %w", err)
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
	// A name is not a unique key: a module may hold an excluded twin (#914).
	// Prefer the live one, so DESCRIBE shows what the app runs and CREATE OR
	// MODIFY edits it; fall back to the first excluded match (#1185).
	var excluded *model.ImportMapping
	for _, im := range all {
		if im.Name == name && b.moduleNameFor(im.ID) == moduleName {
			if !im.Excluded {
				return im, nil
			}
			if excluded == nil {
				excluded = im
			}
		}
	}
	if excluded != nil {
		return excluded, nil
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
	// MessageDefinition is carried the same way: Mendix 11.15 removed the key
	// (the source moved to MessageDefinition2), so an 11.15 document must not
	// get it back (ako/mxcli#987). nil means absent.
	if im.MessageDefinition != nil {
		addStr(g, "MessageDefinition", *im.MessageDefinition)
	}
	// MessageDefinition2 is CARRIED, never invented: gen records it as Introduced
	// 11.10.0, so a document written before then does not have it and adding one
	// is the shape CLAUDE.md's overlay rule warns about (mxbuild tolerates an
	// unknown property; Studio Pro refuses to open the document). The executor
	// decides — it carries the stored document's value on an update and applies
	// the version gate only on a create, where there is nothing to read the key
	// off. nil means the key is absent, which is NOT the same as present-and-empty.
	if im.MessageDefinition2 != nil {
		addStr(g, "MessageDefinition2", *im.MessageDefinition2)
	}

	elems := make([]element.Element, 0, len(im.Elements))
	for _, e := range im.Elements {
		elems = append(elems, importMappingElementToGen(e, "(Object)"))
	}
	addPartList(g, "Elements", elems)

	addBool(g, "UseSubtransactionsForMicroflows", false)
	addStr(g, "PublicName", "")
	// The XML schema root is carried, never derived: writing "" deleted it
	// from every rewritten XML-schema mapping (CE0270, mendixlabs/mxcli#1623).
	addStr(g, "XsdRootElementName", im.RootElementName)
	// ParameterType is a required sub-document even when unused; without it Studio
	// Pro fails to render the schema source and mapping elements correctly. A
	// mapping declaring an input object stores a DataTypes$ObjectType naming it
	// instead of the UnknownType marker (#265).
	if im.ParameterEntity != "" {
		pt := newElem("DataTypes$ObjectType", "")
		addStr(pt, "Entity", im.ParameterEntity)
		addPart(g, "ParameterType", pt)
	} else {
		addPart(g, "ParameterType", newElem("DataTypes$UnknownType", ""))
	}
	addStr(g, "OperationName", "")
	addStr(g, "ServiceName", "")
	addStr(g, "WsdlFile", "")
	return g
}

func importMappingElementToGen(elem *model.ImportMappingElement, parentPath string) element.Element {
	id := string(elem.ID)
	if isMappingObjectKind(elem.Kind) {
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
	if objectHandling == "FindOrCreate" {
		objectHandling = "Find"
	}
	objectHandlingBackup := importBackupFor(objectHandling, elem.ObjectHandlingBackup)

	// $Type is ObjectMappingElement (no "Import" prefix); the generated metamodel
	// name is misleading and causes TypeCacheUnknownTypeException if used.
	g := newElem("ImportMappings$ObjectMappingElement", id)
	addStr(g, "Entity", elem.Entity)
	addStr(g, "ExposedName", elem.ExposedName)
	addStr(g, "JsonPath", jsonPath)
	addStr(g, "XmlPath", elem.XmlPath)
	addStr(g, "ObjectHandling", objectHandling)
	addStr(g, "ObjectHandlingBackup", objectHandlingBackup)
	addBool(g, "ObjectHandlingBackupAllowOverride", elem.BackupAllowOverride)
	addStr(g, "Association", elem.Association)
	if ch := customHandlerToGen(elem.CustomHandler); ch != nil {
		addPart(g, "CustomHandlerCall", ch)
	}

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
	addStr(g, "XmlPath", elem.XmlPath)
	addBool(g, "IsKey", elem.IsKey)
	addPart(g, "Type", mappingValueDataTypeToGen(elem.DataType))
	addInt32(g, "MinOccurs", int32(elem.MinOccurs))
	addInt32(g, "MaxOccurs", int32(elem.MaxOccurs))
	addBool(g, "Nillable", elem.Nillable)
	// IsDefaultType is NOT written here: it belongs to the OBJECT element type
	// only. The generated metamodel declares it on Import/ExportObjectMappingElement
	// and on neither ValueMappingElement, and Studio Pro's own mappings in a blank
	// app carry it on the object element alone. A property the type does not own is
	// the shape mxbuild accepts and Studio Pro refuses to open. (issue #882)
	addStr(g, "ElementType", "Value")
	addStr(g, "Documentation", "")
	addStr(g, "Converter", elem.Converter)
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
	// MDL has no export-level spelling; the executor fills in "Hidden" (#816).
	contents, err = b.keepStoredExportLevel(string(em.ID), contents)
	if err != nil {
		return fmt.Errorf("UpdateExportMapping: %w", err)
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
	// A name is not a unique key: a module may hold an excluded twin (#914).
	// Prefer the live one, so DESCRIBE shows what the app runs and CREATE OR
	// MODIFY edits it; fall back to the first excluded match (#1185).
	var excluded *model.ExportMapping
	for _, em := range all {
		if em.Name == name && b.moduleNameFor(em.ID) == moduleName {
			if !em.Excluded {
				return em, nil
			}
			if excluded == nil {
				excluded = em
			}
		}
	}
	if excluded != nil {
		return excluded, nil
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
	// MessageDefinition is carried the same way: Mendix 11.15 removed the key
	// (the source moved to MessageDefinition2), so an 11.15 document must not
	// get it back (ako/mxcli#987). nil means absent.
	if em.MessageDefinition != nil {
		addStr(g, "MessageDefinition", *em.MessageDefinition)
	}
	// MessageDefinition2 is CARRIED, never invented: gen records it as Introduced
	// 11.10.0, so a document written before then does not have it and adding one
	// is the shape CLAUDE.md's overlay rule warns about (mxbuild tolerates an
	// unknown property; Studio Pro refuses to open the document). The executor
	// decides — it carries the stored document's value on an update and applies
	// the version gate only on a create, where there is nothing to read the key
	// off. nil means the key is absent, which is NOT the same as present-and-empty.
	if em.MessageDefinition2 != nil {
		addStr(g, "MessageDefinition2", *em.MessageDefinition2)
	}
	addStr(g, "NullValueOption", orDefault(em.NullValueOption, "LeaveOutElement"))

	elems := make([]element.Element, 0, len(em.Elements))
	for _, e := range em.Elements {
		elems = append(elems, exportMappingElementToGen(e, "(Object)"))
	}
	addPartList(g, "Elements", elems)

	addStr(g, "PublicName", "")
	// The XML schema root is carried, never derived: writing "" deleted it
	// from every rewritten XML-schema mapping (CE0270, mendixlabs/mxcli#1623).
	addStr(g, "XsdRootElementName", em.RootElementName)
	addBool(g, "IsHeaderParameter", false)
	addStr(g, "ParameterName", "")
	addStr(g, "OperationName", "")
	addStr(g, "ServiceName", "")
	addStr(g, "WsdlFile", "")
	return g
}

func exportMappingElementToGen(elem *model.ExportMappingElement, parentPath string) element.Element {
	id := string(elem.ID)
	if isMappingObjectKind(elem.Kind) {
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
	// EVERY export object element in the demo apps stores Error — 537 of 537,
	// whatever its handling — and "Parameter"/"Find"/"Custom" are not in the
	// {Create, Error, Ignore} enum the backup takes (#261).
	objectHandlingBackup := "Error"

	g := newElem("ExportMappings$ObjectMappingElement", id)
	addStr(g, "Entity", elem.Entity)
	addStr(g, "ExposedName", elem.ExposedName)
	addStr(g, "JsonPath", jsonPath)
	addStr(g, "XmlPath", elem.XmlPath)
	addStr(g, "ObjectHandling", objectHandling)
	addStr(g, "ObjectHandlingBackup", objectHandlingBackup)
	addBool(g, "ObjectHandlingBackupAllowOverride", false)
	addStr(g, "Association", elem.Association)
	if ch := customHandlerToGen(elem.CustomHandler); ch != nil {
		addPart(g, "CustomHandlerCall", ch)
	}

	children := make([]element.Element, 0, len(elem.Children))
	for _, c := range elem.Children {
		children = append(children, exportMappingElementToGen(c, jsonPath))
	}
	addPartList(g, "Children", children)

	// A schema ROOT has MinOccurs 1; hardcoding 0 lost that on every export
	// mapping (#279).
	addInt32(g, "MinOccurs", int32(elem.MinOccurs))
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
	addStr(g, "XmlPath", elem.XmlPath)
	addPart(g, "Type", mappingValueDataTypeToGen(elem.DataType))
	addInt32(g, "MinOccurs", int32(elem.MinOccurs))
	// Mirror the bound schema element: Mendix cross-validates the two and
	// reports CE5015 on any mismatch. Hardcoding 0 only worked while the JSON
	// structure also wrote 0 for every element (#841).
	addInt32(g, "MaxOccurs", int32(elem.MaxOccurs))
	addBool(g, "Nillable", true)
	// IsDefaultType is NOT written here: it belongs to the OBJECT element type
	// only. The generated metamodel declares it on Import/ExportObjectMappingElement
	// and on neither ValueMappingElement, and Studio Pro's own mappings in a blank
	// app carry it on the object element alone. A property the type does not own is
	// the shape mxbuild accepts and Studio Pro refuses to open. (issue #882)
	addStr(g, "ElementType", "Value")
	addStr(g, "Documentation", "")
	addStr(g, "Converter", elem.Converter)
	addInt32(g, "FractionDigits", -1)
	addInt32(g, "TotalDigits", -1)
	// Mirrors the bound schema element, like MaxOccurs above: Studio Pro stores 0
	// for a string element and -1 for a numeric one, so a hardcoded 0 was wrong
	// for every non-string value (#277).
	addInt32(g, "MaxLength", int32(elem.MaxLength))
	// Studio Pro writes IsKey on export value elements too; omitting it was a
	// divergence from the import twin rather than a decision (#277).
	addBool(g, "IsKey", elem.IsKey)
	addBool(g, "IsContent", false)
	addBool(g, "IsXmlAttribute", false)
	// Carried, not hardcoded: whether a mapping stores the structure's sample is
	// a per-document property, so a rewrite preserves what was there rather than
	// deleting it (ako/mxcli#379). A newly authored mapping still gets "".
	addStr(g, "OriginalValue", elem.OriginalValue)
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
	case "Integer", "Long", "AutoNumber":
		// An AutoNumber is an integer to its schema element: as String, mx
		// check failed CE0227 + CE5015 against the Integer it maps
		// (ako/mxcli#2603).
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
	case "Integer", "Long", "AutoNumber":
		return "Integer"
	case "Decimal":
		return "Decimal"
	case "Boolean":
		return "Boolean"
	case "DateTime":
		return "DateTime"
	case "Binary":
		// A FileDocument/Image attribute. Falling through to "String" was the
		// last difference between an mxcli rebuild and Studio Pro's
		// Email_Connector.IMM_EmailTemplateMapping (#261's measurement).
		return "Binary"
	default:
		return "String"
	}
}

// elementTypeForKind maps a model element Kind to its BSON ElementType value.
func elementTypeForKind(kind string) string {
	switch kind {
	case "Array":
		return "Array"
	case "Wrapper":
		// An array of PRIMITIVES: one entity per item, with the value on an
		// attribute (#268).
		return "Wrapper"
	case "Value":
		return "Value"
	default:
		return "Object"
	}
}

// customHandlerToGen builds the Mappings$MappingMicroflowCallImpl sub-document
// for an object element whose handling is Custom (#264).
//
// Built with the generic element helpers, like the rest of the mapping writer,
// rather than through gen: the parameter's stored $Type is
// Mappings$MicroflowCallParameterMappingImpl (161 of 161 documents) where gen
// names it MappingMicroflowParameter, and ParameterMappings must be emitted even
// when EMPTY — Studio Pro writes the bare typed-array marker, the same
// MandatoryLists rule as a rule document's Flows. gen's PartList has no way to
// say "present but empty", and a dropped key is a diff against every stored
// document that has one.
//
// ValueElementPath is deliberately not written: gen declares it and Studio Pro
// writes it in 0 of 161 parameter mappings.
func customHandlerToGen(call *model.MappingMicroflowCall) element.Element {
	if call == nil || call.Microflow == "" {
		return nil
	}
	g := newElem("Mappings$MappingMicroflowCallImpl", "")
	addStr(g, "Microflow", call.Microflow)

	params := make([]element.Element, 0, len(call.Parameters))
	for _, p := range call.Parameters {
		jsonPath, xmlPath, level := storedParameterPaths(p)
		pg := newElem("Mappings$MicroflowCallParameterMappingImpl", "")
		addStr(pg, "Parameter", p.Parameter)
		addInt32(pg, "LevelOfParent", int32(level))
		addStr(pg, "JsonValueElementPath", jsonPath)
		addStr(pg, "XmlValueElementPath", xmlPath)
		params = append(params, pg)
	}
	addPartList(g, "ParameterMappings", params)
	return g
}

// storedParameterPaths maps a parameter's source to the (json, xml, level)
// triple Studio Pro stores. The marker sources write the SAME marker into both
// path properties; an explicit value path is JSON-only.
func storedParameterPaths(p *model.MappingMicroflowParameter) (string, string, int) {
	switch p.Source {
	case "parameter":
		return "(parameter)", "(parameter)", -1
	case "ancestor":
		return "", "", p.LevelOfParent
	case "path":
		return p.ValuePath, p.XmlValuePath, -1
	default:
		return "(parent)", "(parent)", -1
	}
}

// isMappingObjectKind reports whether a mapping element serializes as an OBJECT
// mapping element rather than a value one.
//
// "Wrapper" belongs here: an array of primitives is an object element that binds
// an entity, with the primitive on a value child (#268). Leaving it out made the
// element serialize as a Value with no attribute, which mxbuild rejects with
// CE5015 "Element '…/(Wrapper)' is not a Value element."
func isMappingObjectKind(kind string) bool {
	switch kind {
	case "Object", "Array", "Wrapper":
		return true
	default:
		return false
	}
}

// importBackupFor is the ObjectHandlingBackup an import element gets.
//
// The property takes {Create, Error, Ignore} and nothing else. mxcli used to
// copy the HANDLING into it, which wrote "Find" for a plain find, "Parameter"
// for an export root and "Custom" for a microflow handler — none of which occurs
// in the 1,261 object elements of the demo apps (#261).
//
// An explicit choice from the statement wins. Otherwise: `create` and
// `find or create` both mean Create, and a custom handler defaults to Create
// (73 of 77 in the corpus). A bare `find` never reaches here — the executor
// refuses it rather than choosing among three different meanings.
func importBackupFor(handling, requested string) string {
	switch requested {
	case "Create", "Error", "Ignore":
		return requested
	}
	return "Create"
}
