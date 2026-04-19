// SPDX-License-Identifier: Apache-2.0

// Package bsonutil provides BSON-aware ID conversion utilities for model elements.
// It depends on mdl/types (CGO-free) and the BSON driver (also CGO-free),
// but does NOT depend on sdk/mpr (which pulls in SQLite/CGO).
package bsonutil

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// IDToBsonBinary converts a hex UUID string to a BSON binary value.
// If the input is not a valid UUID, a new random ID is generated as a fallback.
// This matches the legacy sdk/mpr behavior where callers expect a valid binary
// result without error handling. Consider using ValidateID first if strict
// validation is needed.
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
