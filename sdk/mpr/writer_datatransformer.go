// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// CreateDataTransformer creates a new DataTransformers$DataTransformer document.
func (w *Writer) CreateDataTransformer(dt *model.DataTransformer) error {
	if dt.ID == "" {
		dt.ID = model.ID(generateUUID())
	}
	dt.TypeName = "DataTransformers$DataTransformer"

	contents, err := serializeDataTransformer(dt)
	if err != nil {
		return fmt.Errorf("failed to serialize data transformer: %w", err)
	}

	return w.insertUnit(string(dt.ID), string(dt.ContainerID), "Documents", "DataTransformers$DataTransformer", contents)
}

// UpdateDataTransformer replaces an existing data transformer unit, preserving its UUID.
func (w *Writer) UpdateDataTransformer(dt *model.DataTransformer) error {
	dt.TypeName = "DataTransformers$DataTransformer"

	contents, err := serializeDataTransformer(dt)
	if err != nil {
		return fmt.Errorf("failed to serialize data transformer: %w", err)
	}

	return w.updateUnit(string(dt.ID), contents)
}

// DeleteDataTransformer deletes a data transformer by ID.
func (w *Writer) DeleteDataTransformer(id model.ID) error {
	return w.deleteUnit(string(id))
}

func serializeDataTransformer(dt *model.DataTransformer) ([]byte, error) {
	// Root element
	rootElemID := generateUUID()
	rootElement := bson.D{
		{Key: "$ID", Value: idToBsonBinary(rootElemID)},
		{Key: "$Type", Value: "DataTransformers$StructureObject"},
		{Key: "Attributes", Value: bson.A{int32(2)}},
	}

	// Source
	var source bson.D
	switch strings.ToUpper(dt.SourceType) {
	case "XML":
		source = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DataTransformers$XmlSource"},
			{Key: "Content", Value: dt.SourceJSON},
		}
	default: // JSON
		source = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DataTransformers$JsonSource"},
			{Key: "Content", Value: dt.SourceJSON},
		}
	}

	// Steps
	steps := bson.A{int32(2)}
	for _, step := range dt.Steps {
		var action bson.D
		switch strings.ToUpper(step.Technology) {
		case "JSLT":
			action = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DataTransformers$JsltAction"},
				{Key: "Jslt", Value: step.Expression},
			}
		case "XSLT":
			action = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DataTransformers$XsltAction"},
				{Key: "Xslt", Value: step.Expression},
			}
		default:
			action = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DataTransformers$JsltAction"},
				{Key: "Jslt", Value: step.Expression},
			}
		}

		steps = append(steps, bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DataTransformers$Step"},
			{Key: "Action", Value: action},
			{Key: "InputElementPointer", Value: idToBsonBinary(rootElemID)},
			{Key: "OutputElementPointer", Value: idToBsonBinary(rootElemID)},
		})
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(dt.ID))},
		{Key: "$Type", Value: "DataTransformers$DataTransformer"},
		{Key: "Name", Value: dt.Name},
		{Key: "Documentation", Value: ""},
		{Key: "Excluded", Value: dt.Excluded},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "Source", Value: source},
		{Key: "Elements", Value: bson.A{int32(2), rootElement}},
		{Key: "RootElementPointer", Value: idToBsonBinary(rootElemID)},
		{Key: "Steps", Value: steps},
	}

	return marshalUnitIDFirst(doc)
}
