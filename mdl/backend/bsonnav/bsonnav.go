// SPDX-License-Identifier: Apache-2.0

// Package bsonnav provides generic bson.D navigation helpers shared by the page
// and workflow mutators. These operate on raw (decoded) Mendix BSON documents —
// bson v1 (go.mongodb.org/mongo-driver/bson), the version both mutators decode
// into — and understand the Mendix array convention (an int32 type marker at
// index 0 of every list).
package bsonnav

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// DGet returns the value for a key in a bson.D, or nil if not found.
func DGet(doc bson.D, key string) any {
	for _, elem := range doc {
		if elem.Key == key {
			return elem.Value
		}
	}
	return nil
}

// DGetDoc returns a nested bson.D field value, or nil.
func DGetDoc(doc bson.D, key string) bson.D {
	v := DGet(doc, key)
	if d, ok := v.(bson.D); ok {
		return d
	}
	return nil
}

// DGetString returns a string field value, or "".
func DGetString(doc bson.D, key string) string {
	v := DGet(doc, key)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// DSet sets a field value in a bson.D in place. Returns true if found.
// NOTE: callers generally do not check the return value because the keys
// are structurally guaranteed by the widgetFinder traversal. If a key
// is absent, the mutation is silently skipped — this is intentional for
// optional fields (e.g. Appearance, DataSource) that may not be present
// on every widget type.
func DSet(doc bson.D, key string, value any) bool {
	for i := range doc {
		if doc[i].Key == key {
			doc[i].Value = value
			return true
		}
	}
	return false
}

// DGetArrayElements extracts Mendix array elements from a bson.D field value.
// Strips the int32 type marker at index 0.
func DGetArrayElements(val any) []any {
	arr := ToBsonA(val)
	if len(arr) == 0 {
		return nil
	}
	if _, ok := arr[0].(int32); ok {
		return arr[1:]
	}
	if _, ok := arr[0].(int); ok {
		return arr[1:]
	}
	return arr
}

// ToBsonA converts various BSON array types to []any.
func ToBsonA(v any) []any {
	switch arr := v.(type) {
	case bson.A:
		return []any(arr)
	case []any:
		return arr
	default:
		return nil
	}
}

// DSetArray sets a Mendix-style BSON array field, preserving the int32 marker.
func DSetArray(doc bson.D, key string, elements []any) {
	existing := ToBsonA(DGet(doc, key))
	var marker any
	if len(existing) > 0 {
		if _, ok := existing[0].(int32); ok {
			marker = existing[0]
		} else if _, ok := existing[0].(int); ok {
			marker = existing[0]
		}
	}
	var result bson.A
	if marker != nil {
		result = make(bson.A, 0, len(elements)+1)
		result = append(result, marker)
		result = append(result, elements...)
	} else {
		result = make(bson.A, len(elements))
		copy(result, elements)
	}
	DSet(doc, key, result)
}

// ExtractBinaryIDFromDoc extracts a binary ID string from a bson.D field.
func ExtractBinaryIDFromDoc(val any) string {
	switch bin := val.(type) {
	case primitive.Binary:
		return types.BlobToUUID(bin.Data)
	case []byte:
		return types.BlobToUUID(bin)
	default:
		return ""
	}
}
