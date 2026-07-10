// SPDX-License-Identifier: Apache-2.0

// Package bsonutil provides BSON-aware ID conversion and type-extraction utilities
// for model elements.
// It depends on mdl/types (WASM-safe) and the BSON driver (also WASM-safe),
// but does NOT depend on sdk/mpr (which pulls in SQLite/CGO).
package bsonutil

import (
	"fmt"
	"log"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// IDToBsonBinary converts a UUID string to a BSON binary value.
// Panics if id is not a valid UUID — an invalid ID at this layer is always a programming error.
// For user-supplied IDs where invalid input is expected, use IDToBsonBinaryErr.
func IDToBsonBinary(id string) primitive.Binary {
	bin, err := IDToBsonBinaryErr(id)
	if err != nil {
		panic("bsonutil.IDToBsonBinary: " + err.Error())
	}
	return bin
}

// IDToBsonBinaryErr converts a UUID string to a BSON binary value, returning an error
// for invalid input instead of panicking. Use this for user-supplied or untrusted IDs.
func IDToBsonBinaryErr(id string) (primitive.Binary, error) {
	blob := types.UUIDToBlob(id)
	if blob == nil || len(blob) != 16 {
		return primitive.Binary{}, fmt.Errorf("invalid UUID: %q", id)
	}
	return primitive.Binary{
		Subtype: 0x00,
		Data:    blob,
	}, nil
}

// BsonBinaryToID converts a BSON binary value to a hex UUID string.
func BsonBinaryToID(bin primitive.Binary) string {
	return types.BlobToUUID(bin.Data)
}

// NewIDBsonBinary generates a new unique ID and returns it as a BSON binary value.
// Panics if the OS entropy source fails. For callers that can handle failure, use NewIDBsonBinaryErr.
func NewIDBsonBinary() primitive.Binary {
	return IDToBsonBinary(types.GenerateID())
}

// NewIDBsonBinaryErr generates a new unique ID and returns it as a BSON binary value,
// returning an error instead of panicking on failure.
func NewIDBsonBinaryErr() (primitive.Binary, error) {
	id, err := types.GenerateIDErr()
	if err != nil {
		return primitive.Binary{}, fmt.Errorf("generating ID: %w", err)
	}
	return IDToBsonBinaryErr(id)
}

// String extracts a string from an interface value.
// Returns the zero value and logs a diagnostic warning on type mismatch.
func String(v any, field string) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		log.Printf("warning: bsonutil: expected string for %q, got <nil>", field)
	} else {
		log.Printf("warning: bsonutil: expected string for %q, got %T", field, v)
	}
	return ""
}

// Bool extracts a bool from an interface value.
// Returns the zero value and logs a diagnostic warning on type mismatch.
func Bool(v any, field string) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if v == nil {
		log.Printf("warning: bsonutil: expected bool for %q, got <nil>", field)
	} else {
		log.Printf("warning: bsonutil: expected bool for %q, got %T", field, v)
	}
	return false
}
