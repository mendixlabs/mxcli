// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// PrettyPrintJSON delegates to types.PrettyPrintJSON.
func PrettyPrintJSON(s string) string { return types.PrettyPrintJSON(s) }

// BuildJsonElementsFromSnippet delegates to types.BuildJsonElementsFromSnippet.
func BuildJsonElementsFromSnippet(snippet string, customNameMap map[string]string) ([]*JsonElement, error) {
	return types.BuildJsonElementsFromSnippet(snippet, customNameMap)
}

// CreateJsonStructure creates a new JSON structure unit in the MPR.
func (w *Writer) CreateJsonStructure(js *JsonStructure) error {
	if js.ID == "" {
		js.ID = model.ID(generateUUID())
	}
	if js.ExportLevel == "" {
		js.ExportLevel = "Hidden"
	}

	contents, err := serializeJsonStructure(js)
	if err != nil {
		return err
	}

	return w.insertUnit(string(js.ID), string(js.ContainerID),
		"Documents", "JsonStructures$JsonStructure", contents)
}

// UpdateJsonStructure re-serializes an existing JSON structure in-place, preserving its ID.
func (w *Writer) UpdateJsonStructure(js *JsonStructure) error {
	contents, err := serializeJsonStructure(js)
	if err != nil {
		return err
	}
	return w.updateUnit(string(js.ID), contents)
}

// DeleteJsonStructure deletes a JSON structure by ID.
func (w *Writer) DeleteJsonStructure(id string) error {
	return w.deleteUnit(id)
}

func serializeJsonStructure(js *JsonStructure) ([]byte, error) {
	elements := bson.A{int32(2)}
	for _, elem := range js.Elements {
		elements = append(elements, serializeJsonElement(elem))
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(js.ID))},
		{Key: "$Type", Value: "JsonStructures$JsonStructure"},
		{Key: "Documentation", Value: js.Documentation},
		{Key: "Elements", Value: elements},
		{Key: "Excluded", Value: js.Excluded},
		{Key: "ExportLevel", Value: js.ExportLevel},
		{Key: "JsonSnippet", Value: js.JsonSnippet},
		{Key: "Name", Value: js.Name},
	}

	return marshalUnitIDFirst(doc)
}

// serializeJsonElement serializes a single JsonElement to BSON.
// Note: JsonStructures$JsonElement uses int32 for numeric properties (MinOccurs, MaxOccurs, etc.),
// unlike most other Mendix document types which use int64. Verified against Studio Pro-generated BSON.
func serializeJsonElement(elem *JsonElement) bson.D {
	children := bson.A{int32(2)}
	for _, child := range elem.Children {
		children = append(children, serializeJsonElement(child))
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "JsonStructures$JsonElement"},
		{Key: "Children", Value: children},
		{Key: "ElementType", Value: elem.ElementType},
		{Key: "ErrorMessage", Value: ""},
		{Key: "ExposedItemName", Value: elem.ExposedItemName},
		{Key: "ExposedName", Value: elem.ExposedName},
		{Key: "FractionDigits", Value: int32(elem.FractionDigits)},
		{Key: "IsDefaultType", Value: elem.IsDefaultType},
		{Key: "MaxLength", Value: int32(elem.MaxLength)},
		{Key: "MaxOccurs", Value: int32(elem.MaxOccurs)},
		{Key: "MinOccurs", Value: int32(elem.MinOccurs)},
		{Key: "Nillable", Value: elem.Nillable},
		{Key: "OriginalValue", Value: elem.OriginalValue},
		{Key: "Path", Value: elem.Path},
		{Key: "PrimitiveType", Value: elem.PrimitiveType},
		{Key: "TotalDigits", Value: int32(elem.TotalDigits)},
		{Key: "WarningMessage", Value: ""},
	}
}
