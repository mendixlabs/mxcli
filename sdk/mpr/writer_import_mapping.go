// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// CreateImportMapping creates a new import mapping document.
func (w *Writer) CreateImportMapping(im *model.ImportMapping) error {
	if im.ID == "" {
		im.ID = model.ID(generateUUID())
	}
	im.TypeName = "ImportMappings$ImportMapping"

	contents, err := w.serializeImportMapping(im)
	if err != nil {
		return fmt.Errorf("failed to serialize import mapping: %w", err)
	}

	return w.insertUnit(string(im.ID), string(im.ContainerID), "Documents", "ImportMappings$ImportMapping", contents)
}

// UpdateImportMapping updates an existing import mapping document.
func (w *Writer) UpdateImportMapping(im *model.ImportMapping) error {
	contents, err := w.serializeImportMapping(im)
	if err != nil {
		return fmt.Errorf("failed to serialize import mapping: %w", err)
	}
	return w.updateUnit(string(im.ID), contents)
}

// DeleteImportMapping deletes an import mapping document.
func (w *Writer) DeleteImportMapping(id model.ID) error {
	return w.deleteUnit(string(id))
}

// MoveImportMapping moves an import mapping to a new container.
func (w *Writer) MoveImportMapping(im *model.ImportMapping) error {
	return w.moveUnitByID(string(im.ID), string(im.ContainerID))
}

func (w *Writer) serializeImportMapping(im *model.ImportMapping) ([]byte, error) {
	elements := bson.A{int32(2)}
	for _, elem := range im.Elements {
		elements = append(elements, serializeImportMappingElement(elem, "(Object)"))
	}

	exportLevel := im.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}

	// ParameterType is a required sub-document even when not used (DataTypes$UnknownType).
	// Without it Studio Pro fails to render the schema source and mapping elements correctly.
	parameterType := bson.M{
		"$ID":   idToBsonBinary(generateUUID()),
		"$Type": "DataTypes$UnknownType",
	}

	doc := bson.M{
		"$ID":               idToBsonBinary(string(im.ID)),
		"$Type":             "ImportMappings$ImportMapping",
		"Name":              im.Name,
		"Documentation":     im.Documentation,
		"Excluded":          im.Excluded,
		"ExportLevel":       exportLevel,
		"JsonStructure":     im.JsonStructure,
		"XmlSchema":         im.XmlSchema,
		"MessageDefinition": im.MessageDefinition,
		"Elements":          elements,
		// Required fields with defaults — verified against Studio Pro-created BSON
		"UseSubtransactionsForMicroflows": false,
		"PublicName":                      "", // Studio Pro writes "" not the mapping name
		"XsdRootElementName":              "",
		"MappingSourceReference":          nil,
		"ParameterType":                   parameterType,
		"OperationName":                   "",
		"ServiceName":                     "",
		"WsdlFile":                        "",
	}
	return bson.Marshal(doc)
}

func serializeImportMappingElement(elem *model.ImportMappingElement, parentPath string) bson.M {
	id := string(elem.ID)
	if id == "" {
		id = generateUUID()
	}

	if elem.Kind == "Object" {
		return serializeImportObjectElement(id, elem, parentPath)
	}
	return serializeImportValueElement(id, elem, parentPath)
}

func serializeImportObjectElement(id string, elem *model.ImportMappingElement, parentPath string) bson.M {
	// Compute JsonPath: root element (empty ExposedName) stays at parentPath;
	// named elements append |key to navigate into the nested JSON object.
	// Note: the |(Object) suffix is only used for array item elements (empty ExposedName
	// inside an array container), not for regular named nested objects.
	var jsonPath string
	if elem.ExposedName == "" {
		jsonPath = parentPath
	} else {
		jsonPath = parentPath + "|" + elem.ExposedName
	}

	children := bson.A{int32(2)}
	for _, child := range elem.Children {
		children = append(children, serializeImportMappingElement(child, jsonPath))
	}

	objectHandling := elem.ObjectHandling
	if objectHandling == "" {
		objectHandling = "Create"
	}

	// IMPORTANT: The correct $Type is "ImportMappings$ObjectMappingElement" (no "Import" prefix in the element name).
	// The generated metamodel (ImportMappingsImportObjectMappingElement) is misleading — Studio Pro will throw
	// TypeCacheUnknownTypeException if you use "ImportMappings$ImportObjectMappingElement".
	// Rule: MappingElement $Type names do NOT repeat the namespace prefix (same for ExportMappings).
	return bson.M{
		"$ID":                               idToBsonBinary(id),
		"$Type":                             "ImportMappings$ObjectMappingElement",
		"Entity":                            elem.Entity,
		"ExposedName":                       elem.ExposedName,
		"JsonPath":                          jsonPath,
		"XmlPath":                           "",
		"ObjectHandling":                    objectHandling,
		"ObjectHandlingBackup":              objectHandling,
		"ObjectHandlingBackupAllowOverride": false,
		"Association":                       elem.Association,
		"Children":                          children,
		"MinOccurs":                         int32(0),
		"MaxOccurs":                         int32(1),
		"Nillable":                          true,
		"IsDefaultType":                     false,
		"ElementType":                       "Object",
		"Documentation":                     "",
		"CustomHandlerCall":                 nil,
	}
}

func serializeImportValueElement(id string, elem *model.ImportMappingElement, parentPath string) bson.M {
	dataType := serializeImportValueDataType(elem.DataType)
	jsonPath := parentPath + "|" + elem.ExposedName

	return bson.M{
		"$ID":              idToBsonBinary(id),
		"$Type":            "ImportMappings$ValueMappingElement",
		"Attribute":        elem.Attribute,
		"ExposedName":      elem.ExposedName,
		"JsonPath":         jsonPath,
		"XmlPath":          "",
		"IsKey":            elem.IsKey,
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

func serializeImportValueDataType(typeName string) bson.D {
	typeID := idToBsonBinary(GenerateID())
	switch typeName {
	case "Integer", "Long":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$IntegerType"},
		}
	case "Decimal":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$DecimalType"},
		}
	case "Boolean":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$BooleanType"},
		}
	case "DateTime":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$DateTimeType"},
		}
	case "Binary":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$BinaryType"},
		}
	default: // String
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$StringType"},
		}
	}
}
