// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"sort"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// CreateEnumeration creates a new enumeration.
func (w *Writer) CreateEnumeration(enum *model.Enumeration) error {
	if enum.ID == "" {
		enum.ID = model.ID(generateUUID())
	}
	enum.TypeName = "Enumerations$Enumeration"

	contents, err := w.serializeEnumeration(enum)
	if err != nil {
		return fmt.Errorf("failed to serialize enumeration: %w", err)
	}

	return w.insertUnit(string(enum.ID), string(enum.ContainerID), "Documents", "Enumerations$Enumeration", contents)
}

// UpdateEnumeration updates an existing enumeration.
func (w *Writer) UpdateEnumeration(enum *model.Enumeration) error {
	contents, err := w.serializeEnumeration(enum)
	if err != nil {
		return fmt.Errorf("failed to serialize enumeration: %w", err)
	}

	return w.updateUnit(string(enum.ID), contents)
}

// MoveEnumeration moves an enumeration to a new container (module or folder).
// Only updates the ContainerID in the database, preserving all BSON content as-is.
func (w *Writer) MoveEnumeration(enum *model.Enumeration) error {
	return w.moveUnitByID(string(enum.ID), string(enum.ContainerID))
}

// DeleteEnumeration deletes an enumeration.
func (w *Writer) DeleteEnumeration(id model.ID) error {
	return w.deleteUnit(string(id))
}

// MoveConstant moves a constant to a new container (module or folder).
func (w *Writer) MoveConstant(constant *model.Constant) error {
	return w.moveUnitByID(string(constant.ID), string(constant.ContainerID))
}

// CreateConstant creates a new constant.
func (w *Writer) CreateConstant(constant *model.Constant) error {
	if constant.ID == "" {
		constant.ID = model.ID(generateUUID())
	}
	constant.TypeName = "Constants$Constant"

	contents, err := w.serializeConstant(constant)
	if err != nil {
		return fmt.Errorf("failed to serialize constant: %w", err)
	}

	return w.insertUnit(string(constant.ID), string(constant.ContainerID), "Documents", "Constants$Constant", contents)
}

// UpdateConstant updates an existing constant.
func (w *Writer) UpdateConstant(constant *model.Constant) error {
	contents, err := w.serializeConstant(constant)
	if err != nil {
		return fmt.Errorf("failed to serialize constant: %w", err)
	}

	return w.updateUnit(string(constant.ID), contents)
}

// DeleteConstant deletes a constant.
func (w *Writer) DeleteConstant(id model.ID) error {
	return w.deleteUnit(string(id))
}
func (w *Writer) serializeEnumeration(enum *model.Enumeration) ([]byte, error) {
	values := bson.A{int32(3)} // Version prefix
	for _, v := range enum.Values {
		valueID := string(v.ID)
		if valueID == "" {
			valueID = generateUUID()
		}
		captionID := generateUUID()

		// Build translation items (sorted for deterministic output)
		translationItems := bson.A{int32(3)}
		if v.Caption != nil {
			langs := make([]string, 0, len(v.Caption.Translations))
			for lang := range v.Caption.Translations {
				langs = append(langs, lang)
			}
			sort.Strings(langs)
			for _, langCode := range langs {
				translationItems = append(translationItems, bson.D{
					{Key: "$ID", Value: idToBsonBinary(generateUUID())},
					{Key: "$Type", Value: "Texts$Translation"},
					{Key: "LanguageCode", Value: langCode},
					{Key: "Text", Value: v.Caption.Translations[langCode]},
				})
			}
		}

		// Use bson.D (ordered) so $Type appears first — Mendix requires this for correct parsing
		valueDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(valueID)},
			{Key: "$Type", Value: "Enumerations$EnumerationValue"},
			{Key: "Name", Value: v.Name},
			{Key: "Caption", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(captionID)},
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "Items", Value: translationItems},
			}},
			{Key: "Image", Value: ""},
			{Key: "RemoteValue", Value: nil},
		}
		values = append(values, valueDoc)
	}

	// Use bson.D (ordered) so $Type appears early — Mendix requires this for correct parsing
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(enum.ID))},
		{Key: "$Type", Value: "Enumerations$Enumeration"},
		{Key: "Name", Value: enum.Name},
		{Key: "Documentation", Value: enum.Documentation},
		{Key: "Excluded", Value: false},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "RemoteSource", Value: nil},
		{Key: "Values", Value: values},
	}
	return marshalUnitIDFirst(doc)
}

func (w *Writer) serializeConstant(constant *model.Constant) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(constant.ID))},
		{Key: "$Type", Value: "Constants$Constant"},
		{Key: "Name", Value: constant.Name},
		{Key: "Documentation", Value: constant.Documentation},
		{Key: "Type", Value: serializeConstantDataType(constant.Type)},
		{Key: "DefaultValue", Value: constant.DefaultValue},
		{Key: "ExposedToClient", Value: constant.ExposedToClient},
		{Key: "Excluded", Value: constant.Excluded},
		{Key: "ExportLevel", Value: constant.ExportLevel},
	}
	return marshalUnitIDFirst(doc)
}

// serializeConstantDataType converts a ConstantDataType to BSON.
func serializeConstantDataType(dt model.ConstantDataType) bson.D {
	typeID := idToBsonBinary(GenerateID())

	switch dt.Kind {
	case "String":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$StringType"},
		}
	case "Integer":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$IntegerType"},
		}
	case "Long":
		// Mendix uses IntegerType for both Integer and Long in BSON storage.
		// DataTypes$LongType does not exist in the metamodel type cache.
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
	case "DateTime", "Date":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$DateTimeType"},
		}
	case "Binary":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$BinaryType"},
		}
	case "Float":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$FloatType"},
		}
	case "Enumeration":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$EnumerationType"},
			{Key: "Enumeration", Value: dt.EnumRef},
		}
	case "Object":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$ObjectType"},
			{Key: "Entity", Value: dt.EntityRef},
		}
	case "List":
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$ListType"},
			{Key: "Entity", Value: dt.EntityRef},
		}
	default:
		// Default to string type
		return bson.D{
			{Key: "$ID", Value: typeID},
			{Key: "$Type", Value: "DataTypes$StringType"},
		}
	}
}
