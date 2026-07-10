// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"

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
	parameterType := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "DataTypes$UnknownType"},
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(im.ID))},
		{Key: "$Type", Value: "ImportMappings$ImportMapping"},
		{Key: "Name", Value: im.Name},
		{Key: "Documentation", Value: im.Documentation},
		{Key: "Excluded", Value: im.Excluded},
		{Key: "ExportLevel", Value: exportLevel},
		{Key: "JsonStructure", Value: im.JsonStructure},
		{Key: "XmlSchema", Value: im.XmlSchema},
		{Key: "MessageDefinition", Value: im.MessageDefinition},
		{Key: "Elements", Value: elements},
		// Required fields with defaults — verified against Studio Pro-created BSON
		{Key: "UseSubtransactionsForMicroflows", Value: false},
		{Key: "PublicName", Value: ""}, // Studio Pro writes "" not the mapping name
		{Key: "XsdRootElementName", Value: ""},
		{Key: "MappingSourceReference", Value: nil},
		{Key: "ParameterType", Value: parameterType},
		{Key: "OperationName", Value: ""},
		{Key: "ServiceName", Value: ""},
		{Key: "WsdlFile", Value: ""},
	}
	return marshalUnitIDFirst(doc)
}

func serializeImportMappingElement(elem *model.ImportMappingElement, parentPath string) bson.D {
	id := string(elem.ID)
	if id == "" {
		id = generateUUID()
	}

	if elem.Kind == "Object" || elem.Kind == "Array" {
		return serializeImportObjectElement(id, elem, parentPath)
	}
	return serializeImportValueElement(id, elem, parentPath)
}

func serializeImportObjectElement(id string, elem *model.ImportMappingElement, parentPath string) bson.D {
	// Use pre-computed JsonPath from the executor when available.
	// The executor aligns JsonPath with the JSON structure element paths.
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
		children = append(children, serializeImportMappingElement(child, jsonPath))
	}

	objectHandling := elem.ObjectHandling
	if objectHandling == "" {
		objectHandling = "Create"
	}
	objectHandlingBackup := objectHandling
	if objectHandling == "FindOrCreate" {
		objectHandling = "Find"
		objectHandlingBackup = "Create"
	}

	// IMPORTANT: The correct $Type is "ImportMappings$ObjectMappingElement" (no "Import" prefix in the element name).
	// The generated metamodel (ImportMappingsImportObjectMappingElement) is misleading — Studio Pro will throw
	// TypeCacheUnknownTypeException if you use "ImportMappings$ImportObjectMappingElement".
	// Rule: MappingElement $Type names do NOT repeat the namespace prefix (same for ExportMappings).
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "ImportMappings$ObjectMappingElement"},
		{Key: "Entity", Value: elem.Entity},
		{Key: "ExposedName", Value: elem.ExposedName},
		{Key: "JsonPath", Value: jsonPath},
		{Key: "XmlPath", Value: ""},
		{Key: "ObjectHandling", Value: objectHandling},
		{Key: "ObjectHandlingBackup", Value: objectHandlingBackup},
		{Key: "ObjectHandlingBackupAllowOverride", Value: false},
		{Key: "Association", Value: elem.Association},
		{Key: "Children", Value: children},
		{Key: "MinOccurs", Value: int32(elem.MinOccurs)},
		{Key: "MaxOccurs", Value: int32(elem.MaxOccurs)},
		{Key: "Nillable", Value: elem.Nillable},
		{Key: "IsDefaultType", Value: false},
		{Key: "ElementType", Value: elementTypeForKind(elem.Kind)},
		{Key: "Documentation", Value: ""},
		{Key: "CustomHandlerCall", Value: nil},
	}
}

func serializeImportValueElement(id string, elem *model.ImportMappingElement, parentPath string) bson.D {
	dataType := serializeImportValueDataType(elem.DataType)
	jsonPath := elem.JsonPath
	if jsonPath == "" {
		jsonPath = parentPath + "|" + elem.ExposedName
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "ImportMappings$ValueMappingElement"},
		{Key: "Attribute", Value: elem.Attribute},
		{Key: "ExposedName", Value: elem.ExposedName},
		{Key: "JsonPath", Value: jsonPath},
		{Key: "XmlPath", Value: ""},
		{Key: "IsKey", Value: elem.IsKey},
		{Key: "Type", Value: dataType},
		{Key: "MinOccurs", Value: int32(elem.MinOccurs)},
		{Key: "MaxOccurs", Value: int32(elem.MaxOccurs)},
		{Key: "Nillable", Value: elem.Nillable},
		{Key: "IsDefaultType", Value: false},
		{Key: "ElementType", Value: "Value"},
		{Key: "Documentation", Value: ""},
		{Key: "Converter", Value: ""},
		{Key: "FractionDigits", Value: int32(elem.FractionDigits)},
		{Key: "TotalDigits", Value: int32(elem.TotalDigits)},
		{Key: "MaxLength", Value: int32(elem.MaxLength)},
		{Key: "IsContent", Value: false},
		{Key: "IsXmlAttribute", Value: false},
		{Key: "OriginalValue", Value: elem.OriginalValue},
		{Key: "XmlPrimitiveType", Value: xmlPrimitiveTypeName(elem.DataType)},
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

// elementTypeForKind maps model Kind to BSON ElementType.
func elementTypeForKind(kind string) string {
	if kind == "Array" {
		return "Array"
	}
	if kind == "Value" {
		return "Value"
	}
	return "Object"
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
