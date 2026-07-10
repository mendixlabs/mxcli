// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/JordtenBulte-OLC/mxcli/mdl/bsonutil"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

// ListImageCollections reads every Images$ImageCollection unit (identity, export
// level, and the embedded images with their binary data). Used by SHOW IMAGE
// COLLECTIONS, the CREATE OR MODIFY existence check, and duplicate validation.
func (b *Backend) ListImageCollections() ([]*types.ImageCollection, error) {
	units, err := b.reader.ListRawUnitsByType("Images$ImageCollection")
	if err != nil {
		return nil, err
	}
	out := make([]*types.ImageCollection, 0, len(units))
	for _, u := range units {
		var doc bson.M
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			return nil, fmt.Errorf("unmarshal image collection %s: %w", u.ID, err)
		}
		ic := &types.ImageCollection{
			ContainerID: model.ID(u.ContainerID),
		}
		ic.ID = model.ID(u.ID)
		ic.TypeName = "Images$ImageCollection"
		ic.Name, _ = doc["Name"].(string)
		ic.Documentation, _ = doc["Documentation"].(string)
		ic.ExportLevel, _ = doc["ExportLevel"].(string)
		if arr, ok := doc["Images"].(bson.A); ok {
			for _, el := range arr {
				imgDoc, ok := el.(bson.M)
				if !ok {
					continue
				}
				img := types.Image{}
				if v, ok := imgDoc["$ID"].(primitive.Binary); ok {
					img.ID = model.ID(bsonutil.BsonBinaryToID(v))
				}
				img.Name, _ = imgDoc["Name"].(string)
				img.Format, _ = imgDoc["ImageFormat"].(string)
				if bin, ok := imgDoc["Image"].(primitive.Binary); ok {
					img.Data = bin.Data
				}
				ic.Images = append(ic.Images, img)
			}
		}
		out = append(out, ic)
	}
	return out, nil
}

// CreateImageCollection inserts a new Images$ImageCollection document. The image
// payloads are stored as binary (subtype 0); the Images array uses the marker-3
// form. Built with raw BSON because of the binary image data.
func (b *Backend) CreateImageCollection(ic *types.ImageCollection) error {
	if ic == nil {
		return fmt.Errorf("CreateImageCollection: nil image collection")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateImageCollection: not connected for writing")
	}
	if ic.ID == "" {
		ic.ID = model.ID(mmpr.GenerateID())
	}
	if ic.ExportLevel == "" {
		ic.ExportLevel = "Hidden"
	}
	contents, err := serializeImageCollection(ic)
	if err != nil {
		return fmt.Errorf("CreateImageCollection: marshal: %w", err)
	}
	return b.writer.InsertUnit(string(ic.ID), string(ic.ContainerID), "Documents", "Images$ImageCollection", contents)
}

// UpdateImageCollection rewrites an existing image collection in place.
func (b *Backend) UpdateImageCollection(ic *types.ImageCollection) error {
	if ic == nil {
		return fmt.Errorf("UpdateImageCollection: nil image collection")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateImageCollection: not connected for writing")
	}
	if ic.ExportLevel == "" {
		ic.ExportLevel = "Hidden"
	}
	contents, err := serializeImageCollection(ic)
	if err != nil {
		return fmt.Errorf("UpdateImageCollection: marshal: %w", err)
	}
	return b.writer.UpdateRawUnit(string(ic.ID), contents)
}

// DeleteImageCollection removes an image collection unit by ID.
func (b *Backend) DeleteImageCollection(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteImageCollection: not connected for writing")
	}
	return b.writer.DeleteUnit(id)
}

func serializeImageCollection(ic *types.ImageCollection) ([]byte, error) {
	images := bson.A{int32(3)} // Images array marker
	for i := range ic.Images {
		img := &ic.Images[i]
		imgID := string(img.ID)
		if imgID == "" {
			imgID = mmpr.GenerateID()
			img.ID = model.ID(imgID)
		}
		images = append(images, bson.D{
			{Key: "$ID", Value: bsonutil.IDToBsonBinary(imgID)},
			{Key: "$Type", Value: "Images$Image"},
			{Key: "Image", Value: primitive.Binary{Subtype: 0, Data: img.Data}},
			{Key: "ImageFormat", Value: img.Format},
			{Key: "Name", Value: img.Name},
		})
	}
	doc := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(string(ic.ID))},
		{Key: "$Type", Value: "Images$ImageCollection"},
		{Key: "Documentation", Value: ic.Documentation},
		{Key: "Excluded", Value: false},
		{Key: "ExportLevel", Value: ic.ExportLevel},
		{Key: "Images", Value: images},
		{Key: "Name", Value: ic.Name},
	}
	return bson.Marshal(doc)
}
