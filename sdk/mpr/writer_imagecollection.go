// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CreateImageCollection creates a new empty image collection unit in the MPR.
func (w *Writer) CreateImageCollection(ic *ImageCollection) error {
	if ic.ID == "" {
		ic.ID = model.ID(generateUUID())
	}
	if ic.ExportLevel == "" {
		ic.ExportLevel = "Hidden"
	}

	contents, err := serializeImageCollection(ic)
	if err != nil {
		return err
	}

	return w.insertUnit(string(ic.ID), string(ic.ContainerID),
		"Documents", "Images$ImageCollection", contents)
}

// UpdateImageCollection re-serializes an existing image collection in-place, preserving its ID.
func (w *Writer) UpdateImageCollection(ic *ImageCollection) error {
	contents, err := serializeImageCollection(ic)
	if err != nil {
		return err
	}
	return w.updateUnit(string(ic.ID), contents)
}

// DeleteImageCollection deletes an image collection by ID.
func (w *Writer) DeleteImageCollection(id string) error {
	return w.deleteUnit(id)
}

func serializeImageCollection(ic *ImageCollection) ([]byte, error) {
	// Images array always starts with the array marker int32(3)
	images := bson.A{int32(3)}
	for i := range ic.Images {
		img := &ic.Images[i]
		if img.ID == "" {
			img.ID = model.ID(generateUUID())
		}
		images = append(images, bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(img.ID))},
			{Key: "$Type", Value: "Images$Image"},
			{Key: "Image", Value: primitive.Binary{Subtype: 0, Data: img.Data}},
			{Key: "ImageFormat", Value: img.Format},
			{Key: "Name", Value: img.Name},
		})
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(ic.ID))},
		{Key: "$Type", Value: "Images$ImageCollection"},
		{Key: "Documentation", Value: ic.Documentation},
		{Key: "Excluded", Value: false},
		{Key: "ExportLevel", Value: ic.ExportLevel},
		{Key: "Images", Value: images},
		{Key: "Name", Value: ic.Name},
	}

	return marshalUnitIDFirst(doc)
}
