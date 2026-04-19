// SPDX-License-Identifier: Apache-2.0

// Package bsonutil provides BSON-aware ID conversion utilities for model elements.
// It depends on mdl/types (WASM-safe) and the BSON driver (also WASM-safe),
// but does NOT depend on sdk/mpr (which pulls in SQLite/CGO).
package bsonutil

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// IDToBsonBinary converts a hex UUID string to a BSON binary value.
func IDToBsonBinary(id string) primitive.Binary {
	blob := types.UUIDToBlob(id)
	if blob == nil || len(blob) != 16 {
		blob = types.UUIDToBlob(types.GenerateID())
	}
	return primitive.Binary{
		Subtype: 0x00,
		Data:    blob,
	}
}

// BsonBinaryToID converts a BSON binary value to a hex UUID string.
func BsonBinaryToID(bin primitive.Binary) string {
	return types.BlobToUUID(bin.Data)
}

// NewIDBsonBinary generates a new unique ID and returns it as a BSON binary value.
func NewIDBsonBinary() primitive.Binary {
	return IDToBsonBinary(types.GenerateID())
}
