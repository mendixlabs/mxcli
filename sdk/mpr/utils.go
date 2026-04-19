// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
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
func IDToBsonBinary(id string) primitive.Binary {
	return bsonutil.IDToBsonBinary(id)
}

// BsonBinaryToID converts a BSON binary value to a UUID string.
func BsonBinaryToID(bin primitive.Binary) string {
	return bsonutil.BsonBinaryToID(bin)
}

// Hash computes a hash for content (used for content deduplication).
func Hash(content []byte) string {
	return types.Hash(content)
}

// ValidateID checks if an ID is valid.
func ValidateID(id string) bool {
	return types.ValidateID(id)
}
