// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// SerializeImportMapping returns BSON bytes for an import mapping unit.
func SerializeImportMapping(im *model.ImportMapping) ([]byte, error) {
	elements := bson.A{int32(2)}
	for _, elem := range im.Elements {
		elements = append(elements, serImportMappingElement(elem, "(Object)"))
	}

	exportLevel := im.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}

	// ParameterType is a required sub-document even when not used (DataTypes$UnknownType).
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
		{Key: "PublicName", Value: ""},
		{Key: "XsdRootElementName", Value: ""},
		{Key: "MappingSourceReference", Value: nil},
		{Key: "ParameterType", Value: parameterType},
		{Key: "OperationName", Value: ""},
		{Key: "ServiceName", Value: ""},
		{Key: "WsdlFile", Value: ""},
	}
	return bson.Marshal(doc)
}

func serImportMappingElement(elem *model.ImportMappingElement, parentPath string) bson.D {
	id := string(elem.ID)
	if id == "" {
		id = generateUUID()
	}

	if elem.Kind == "Object" || elem.Kind == "Array" {
		return serImportObjectElement(id, elem, parentPath)
	}
	return serImportValueElement(id, elem, parentPath)
}

func serImportObjectElement(id string, elem *model.ImportMappingElement, parentPath string) bson.D {
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
		children = append(children, serImportMappingElement(child, jsonPath))
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
		{Key: "ElementType", Value: serElementTypeForKind(elem.Kind)},
		{Key: "Documentation", Value: ""},
		{Key: "CustomHandlerCall", Value: nil},
	}
}

func serImportValueElement(id string, elem *model.ImportMappingElement, parentPath string) bson.D {
	dataType := serMappingValueDataType(elem.DataType)
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
		{Key: "XmlPrimitiveType", Value: serXmlPrimitiveTypeName(elem.DataType)},
	}
}

// SerializeExportMapping returns BSON bytes for an export mapping unit.
func SerializeExportMapping(em *model.ExportMapping) ([]byte, error) {
	elements := bson.A{int32(2)}
	for _, elem := range em.Elements {
		elements = append(elements, serExportMappingElement(elem, "(Object)"))
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
		{Key: "PublicName", Value: ""},
		{Key: "XsdRootElementName", Value: ""},
		{Key: "IsHeaderParameter", Value: false},
		{Key: "ParameterName", Value: ""},
		{Key: "OperationName", Value: ""},
		{Key: "ServiceName", Value: ""},
		{Key: "WsdlFile", Value: ""},
		{Key: "MappingSourceReference", Value: nil},
	}
	return bson.Marshal(doc)
}

func serExportMappingElement(elem *model.ExportMappingElement, parentPath string) bson.D {
	id := string(elem.ID)
	if id == "" {
		id = generateUUID()
	}

	if elem.Kind == "Object" || elem.Kind == "Array" {
		return serExportObjectElement(id, elem, parentPath)
	}
	return serExportValueElement(id, elem, parentPath)
}

func serExportObjectElement(id string, elem *model.ExportMappingElement, parentPath string) bson.D {
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
		children = append(children, serExportMappingElement(child, jsonPath))
	}

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
		{Key: "ElementType", Value: serElementTypeForKind(elem.Kind)},
		{Key: "Documentation", Value: ""},
		{Key: "CustomHandlerCall", Value: nil},
	}
}

func serExportValueElement(id string, elem *model.ExportMappingElement, parentPath string) bson.D {
	dataType := serMappingValueDataType(elem.DataType)
	jsonPath := elem.JsonPath
	if jsonPath == "" {
		jsonPath = parentPath + "|" + elem.ExposedName
	}

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
		{Key: "XmlPrimitiveType", Value: serXmlPrimitiveTypeName(elem.DataType)},
	}
}

// serXmlPrimitiveTypeName maps a Mendix data type name to an XML primitive type name.
func serXmlPrimitiveTypeName(dataType string) string {
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

// serElementTypeForKind maps model Kind to BSON ElementType.
func serElementTypeForKind(kind string) string {
	if kind == "Array" {
		return "Array"
	}
	if kind == "Value" {
		return "Value"
	}
	return "Object"
}

// serMappingValueDataType converts a simple type name to a DataTypes$ BSON object.
func serMappingValueDataType(typeName string) bson.D {
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
