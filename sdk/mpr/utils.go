// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/JordtenBulte-OLC/mxcli/mdl/bsonutil"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// GenerateID generates a new unique ID for model elements.
func GenerateID() string {
	return types.GenerateID()
}

// GenerateDeterministicID generates a stable UUID from a seed string.
func GenerateDeterministicID(seed string) string {
	return types.GenerateDeterministicID(seed)
}

// BlobToUUID converts a binary ID blob to a UUID string.
func BlobToUUID(data []byte) string {
	return types.BlobToUUID(data)
}

// IDToBsonBinary converts a UUID string to a BSON binary value.
// For invalid or empty UUIDs (e.g. test placeholders), falls back to generating
// a random ID to maintain backward compatibility with existing serialization paths.
// For strict validation, use bsonutil.IDToBsonBinaryErr.
func IDToBsonBinary(id string) primitive.Binary {
	return idToBsonBinary(id)
}

// BsonBinaryToID converts a BSON binary value to a UUID string.
func BsonBinaryToID(bin primitive.Binary) string {
	return bsonutil.BsonBinaryToID(bin)
}

// ValidateID checks if an ID is valid.
func ValidateID(id string) bool {
	return types.ValidateID(id)
}
