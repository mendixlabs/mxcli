// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"

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

	doc := bson.M{
		"$ID":               idToBsonBinary(string(em.ID)),
		"$Type":             "ExportMappings$ExportMapping",
		"Name":              em.Name,
		"Documentation":     em.Documentation,
		"Excluded":          em.Excluded,
		"ExportLevel":       exportLevel,
		"JsonStructure":     em.JsonStructure,
		"XmlSchema":         em.XmlSchema,
		"MessageDefinition": em.MessageDefinition,
		"NullValueOption":   nullValueOption,
		"Elements":          elements,
		// Required fields with defaults — verified against Studio Pro-created BSON
		"PublicName":             "", // Studio Pro writes "" not the mapping name
		"XsdRootElementName":     "",
		"IsHeaderParameter":      false,
		"ParameterName":          "",
		"OperationName":          "",
		"ServiceName":            "",
		"WsdlFile":               "",
		"MappingSourceReference": nil,
	}
	return bson.Marshal(doc)
}

func serializeExportMappingElement(elem *model.ExportMappingElement, parentPath string) bson.M {
	id := string(elem.ID)
	if id == "" {
		id = generateUUID()
	}

	if elem.Kind == "Object" {
		return serializeExportObjectElement(id, elem, parentPath)
	}
	return serializeExportValueElement(id, elem, parentPath)
}

func serializeExportObjectElement(id string, elem *model.ExportMappingElement, parentPath string) bson.M {
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
	return bson.M{
		"$ID":                               idToBsonBinary(id),
		"$Type":                             "ExportMappings$ObjectMappingElement",
		"Entity":                            elem.Entity,
		"ExposedName":                       elem.ExposedName,
		"JsonPath":                          jsonPath,
		"XmlPath":                           "",
		"ObjectHandling":                    "Create",
		"ObjectHandlingBackup":              "Create",
		"ObjectHandlingBackupAllowOverride": false,
		"Association":                       elem.Association,
		"Children":                          children,
		"MinOccurs":                         int32(0),
		"MaxOccurs":                         int32(1),
		"Nillable":                          true,
		"IsDefaultType":                     false,
		"ElementType":                       "Object",
		"Documentation":                     "",
	}
}

func serializeExportValueElement(id string, elem *model.ExportMappingElement, parentPath string) bson.M {
	dataType := serializeImportValueDataType(elem.DataType) // reuse — same DataTypes$* types
	// Use pre-computed JsonPath when available, otherwise derive from parentPath.
	jsonPath := elem.JsonPath
	if jsonPath == "" {
		jsonPath = parentPath + "|" + elem.ExposedName
	}

	// IMPORTANT: "ExportMappings$ValueMappingElement" — no "Export" prefix. See comment in serializeExportObjectElement.
	return bson.M{
		"$ID":              idToBsonBinary(id),
		"$Type":            "ExportMappings$ValueMappingElement",
		"Attribute":        elem.Attribute,
		"ExposedName":      elem.ExposedName,
		"JsonPath":         jsonPath,
		"XmlPath":          "",
		"Type":             dataType,
		"MinOccurs":        int32(0),
		"MaxOccurs":        int32(1),
		"Nillable":         true,
		"IsDefaultType":    false,
		"ElementType":      "Value",
		"Documentation":    "",
		"Converter":        "",
		"FractionDigits":   int32(-1),
		"TotalDigits":      int32(-1),
		"MaxLength":        int32(0),
		"IsContent":        false,
		"IsXmlAttribute":   false,
		"OriginalValue":    "",
		"XmlPrimitiveType": xmlPrimitiveTypeName(elem.DataType),
	}
}
