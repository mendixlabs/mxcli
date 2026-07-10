// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// CreateExportMapping creates a new export mapping document.
func (w *Writer) CreateExportMapping(em *model.ExportMapping) error {
	if em.ID == "" {
		em.ID = model.ID(generateUUID())
	}
	em.TypeName = "ExportMappings$ExportMapping"

	contents, err := w.serializeExportMapping(em)
	if err != nil {
		return fmt.Errorf("failed to serialize export mapping: %w", err)
	}

	return w.insertUnit(string(em.ID), string(em.ContainerID), "Documents", "ExportMappings$ExportMapping", contents)
}

// UpdateExportMapping updates an existing export mapping document.
func (w *Writer) UpdateExportMapping(em *model.ExportMapping) error {
	contents, err := w.serializeExportMapping(em)
	if err != nil {
		return fmt.Errorf("failed to serialize export mapping: %w", err)
	}
	return w.updateUnit(string(em.ID), contents)
}

// DeleteExportMapping deletes an export mapping document.
func (w *Writer) DeleteExportMapping(id model.ID) error {
	return w.deleteUnit(string(id))
}

// MoveExportMapping moves an export mapping to a new container.
func (w *Writer) MoveExportMapping(em *model.ExportMapping) error {
	return w.moveUnitByID(string(em.ID), string(em.ContainerID))
}

func (w *Writer) serializeExportMapping(em *model.ExportMapping) ([]byte, error) {
	elements := bson.A{int32(2)}
	for _, elem := range em.Elements {
		elements = append(elements, serializeExportMappingElement(elem, "(Object)"))
	}

	exportLevel := em.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}

	nullValueOption := em.NullValueOption
	if nullValueOption == "" {
		nullValueOption = "LeaveOutElement"
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(em.ID))},
		{Key: "$Type", Value: "ExportMappings$ExportMapping"},
		{Key: "Name", Value: em.Name},
		{Key: "Documentation", Value: em.Documentation},
		{Key: "Excluded", Value: em.Excluded},
		{Key: "ExportLevel", Value: exportLevel},
		{Key: "JsonStructure", Value: em.JsonStructure},
		{Key: "XmlSchema", Value: em.XmlSchema},
		{Key: "MessageDefinition", Value: em.MessageDefinition},
		{Key: "NullValueOption", Value: nullValueOption},
		{Key: "Elements", Value: elements},
		// Required fields with defaults — verified against Studio Pro-created BSON
		{Key: "PublicName", Value: ""}, // Studio Pro writes "" not the mapping name
		{Key: "XsdRootElementName", Value: ""},
		{Key: "IsHeaderParameter", Value: false},
		{Key: "ParameterName", Value: ""},
		{Key: "OperationName", Value: ""},
		{Key: "ServiceName", Value: ""},
		{Key: "WsdlFile", Value: ""},
		{Key: "MappingSourceReference", Value: nil},
	}
	return marshalUnitIDFirst(doc)
}

func serializeExportMappingElement(elem *model.ExportMappingElement, parentPath string) bson.D {
	id := string(elem.ID)
	if id == "" {
		id = generateUUID()
	}

	if elem.Kind == "Object" || elem.Kind == "Array" {
		return serializeExportObjectElement(id, elem, parentPath)
	}
	return serializeExportValueElement(id, elem, parentPath)
}

func serializeExportObjectElement(id string, elem *model.ExportMappingElement, parentPath string) bson.D {
	// Use pre-computed JsonPath from the executor (which knows the JSON structure element types).
	// Fall back to a simple parentPath + "|" + ExposedName only when JsonPath was not set.
	jsonPath := elem.JsonPath
	if jsonPath == "" {
		if elem.ExposedName == "" {
			jsonPath = parentPath
		} else {
			jsonPath = parentPath + "|" + elem.ExposedName
		}
	}

	children := bson.A{int32(2)}
	for _, child := range elem.Children {
		children = append(children, serializeExportMappingElement(child, jsonPath))
	}

	// IMPORTANT: The correct $Type is "ExportMappings$ObjectMappingElement" (no "Export" prefix in the element name).
	// The generated metamodel (ExportMappingsExportObjectMappingElement) is misleading — Studio Pro will throw
	// TypeCacheUnknownTypeException if you use "ExportMappings$ExportObjectMappingElement".
	// Same convention as ImportMappings: element types do NOT repeat the namespace prefix.
	objectHandling := elem.ObjectHandling
	if objectHandling == "" {
		objectHandling = "Parameter"
	}

	maxOccurs := int32(elem.MaxOccurs)

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "ExportMappings$ObjectMappingElement"},
		{Key: "Entity", Value: elem.Entity},
		{Key: "ExposedName", Value: elem.ExposedName},
		{Key: "JsonPath", Value: jsonPath},
		{Key: "XmlPath", Value: ""},
		{Key: "ObjectHandling", Value: objectHandling},
		{Key: "ObjectHandlingBackup", Value: objectHandling},
		{Key: "ObjectHandlingBackupAllowOverride", Value: false},
		{Key: "Association", Value: elem.Association},
		{Key: "Children", Value: children},
		{Key: "MinOccurs", Value: int32(0)},
		{Key: "MaxOccurs", Value: maxOccurs},
		{Key: "Nillable", Value: true},
		{Key: "IsDefaultType", Value: false},
		{Key: "ElementType", Value: elementTypeForKind(elem.Kind)},
		{Key: "Documentation", Value: ""},
		{Key: "CustomHandlerCall", Value: nil},
	}
}

func serializeExportValueElement(id string, elem *model.ExportMappingElement, parentPath string) bson.D {
	dataType := serializeImportValueDataType(elem.DataType) // reuse — same DataTypes$* types
	// Use pre-computed JsonPath when available, otherwise derive from parentPath.
	jsonPath := elem.JsonPath
	if jsonPath == "" {
		jsonPath = parentPath + "|" + elem.ExposedName
	}

	// IMPORTANT: "ExportMappings$ValueMappingElement" — no "Export" prefix. See comment in serializeExportObjectElement.
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "ExportMappings$ValueMappingElement"},
		{Key: "Attribute", Value: elem.Attribute},
		{Key: "ExposedName", Value: elem.ExposedName},
		{Key: "JsonPath", Value: jsonPath},
		{Key: "XmlPath", Value: ""},
		{Key: "Type", Value: dataType},
		{Key: "MinOccurs", Value: int32(0)},
		{Key: "MaxOccurs", Value: int32(0)},
		{Key: "Nillable", Value: true},
		{Key: "IsDefaultType", Value: false},
		{Key: "ElementType", Value: "Value"},
		{Key: "Documentation", Value: ""},
		{Key: "Converter", Value: ""},
		{Key: "FractionDigits", Value: int32(-1)},
		{Key: "TotalDigits", Value: int32(-1)},
		{Key: "MaxLength", Value: int32(0)},
		{Key: "IsContent", Value: false},
		{Key: "IsXmlAttribute", Value: false},
		{Key: "OriginalValue", Value: ""},
		{Key: "XmlPrimitiveType", Value: xmlPrimitiveTypeName(elem.DataType)},
	}
}
