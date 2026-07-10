// SPDX-License-Identifier: Apache-2.0

// BSONScanner interface implementation for modelsdk/mpr.Reader.
// Implements types.BSONScanner so callers can use modelsdk/mpr instead of
// sdk/mpr for rename and qualified-name scanning.
package mpr

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// Compile-time assertion: *Reader implements types.BSONScanner.
var _ types.BSONScanner = (*Reader)(nil)

// ScanRenameReferences scans every unit in the project and returns the set of
// patches + hit list produced by replacing oldName with newName (exact or
// "oldName." prefix match). It performs no writes — callers persist patches
// via the modelsdk write transaction.
func (r *Reader) ScanRenameReferences(oldName, newName string) ([]types.UnitPatch, []types.RenameHit, error) {
	units, err := r.listUnitsByType("")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list units: %w", err)
	}

	var (
		patches []types.UnitPatch
		hits    []types.RenameHit
	)

	for _, unit := range units {
		contents, err := r.resolveContents(unit.ID, unit.Contents)
		if err != nil {
			continue
		}
		if len(contents) == 0 {
			continue
		}

		var raw bson.D
		if err := bson.Unmarshal(contents, &raw); err != nil {
			continue
		}

		count := 0
		updated := bscanReplaceStringsInDoc(raw, oldName, newName, &count)
		if count == 0 {
			continue
		}

		docName := ""
		for _, elem := range updated {
			if elem.Key == "Name" {
				if s, ok := elem.Value.(string); ok {
					docName = s
				}
			}
		}

		hits = append(hits, types.RenameHit{
			UnitID:   unit.ID,
			UnitType: unit.Type,
			Name:     docName,
			Count:    count,
		})

		newContents, err := bson.Marshal(updated)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal updated document %s: %w", unit.ID, err)
		}
		patches = append(patches, types.UnitPatch{ID: unit.ID, Contents: newContents})
	}

	return patches, hits, nil
}

// ScanQualifiedNameUpdates scans every unit in the project and returns the set
// of patches needed to replace oldName with newName (exact or "oldName." prefix
// match). It performs no writes — callers persist the returned patches via the
// modelsdk write transaction.
func (r *Reader) ScanQualifiedNameUpdates(oldName, newName string) ([]types.UnitPatch, error) {
	units, err := r.listUnitsByType("")
	if err != nil {
		return nil, err
	}

	var patches []types.UnitPatch
	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil || len(contents) == 0 {
			continue
		}

		var raw map[string]any
		if err := bson.Unmarshal(contents, &raw); err != nil {
			continue
		}

		if bscanReplaceStringsInMap(raw, oldName, newName) {
			newContents, err := bson.Marshal(raw)
			if err != nil {
				continue
			}
			patches = append(patches, types.UnitPatch{ID: u.ID, Contents: newContents})
		}
	}

	return patches, nil
}

// ── Private helpers (prefixed bscan to avoid name collisions) ─────────────────

// bscanReplaceStringsInDoc recursively walks a bson.D document and replaces
// string values that match oldName exactly or start with oldName + ".".
func bscanReplaceStringsInDoc(doc bson.D, oldName, newName string, count *int) bson.D {
	result := make(bson.D, len(doc))
	for i, elem := range doc {
		result[i] = bson.E{
			Key:   elem.Key,
			Value: bscanReplaceStringsInValue(elem.Value, oldName, newName, count),
		}
	}
	return result
}

// bscanReplaceStringsInValue replaces qualified name strings in any BSON value type.
func bscanReplaceStringsInValue(val any, oldName, newName string, count *int) any {
	switch v := val.(type) {
	case string:
		if v == oldName {
			*count++
			return newName
		}
		if strings.HasPrefix(v, oldName+".") {
			*count++
			return newName + v[len(oldName):]
		}
		return v

	case bson.D:
		return bscanReplaceStringsInDoc(v, oldName, newName, count)

	case bson.A:
		result := make(bson.A, len(v))
		for i, item := range v {
			result[i] = bscanReplaceStringsInValue(item, oldName, newName, count)
		}
		return result

	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = bscanReplaceStringsInValue(item, oldName, newName, count)
		}
		return result

	default:
		return v
	}
}

// bscanReplaceStringsInMap recursively walks a map and replaces string values
// that match oldName exactly or have oldName as a prefix (followed by ".").
// Returns true if any replacement was made.
func bscanReplaceStringsInMap(m map[string]any, oldName, newName string) bool {
	changed := false
	for k, v := range m {
		if replaced, ok := bscanReplaceInValue(v, oldName, newName); ok {
			m[k] = replaced
			changed = true
		}
	}
	return changed
}

// bscanReplaceInValue recursively processes a value and returns the replacement
// and whether any change was made.
func bscanReplaceInValue(v any, oldName, newName string) (any, bool) {
	switch val := v.(type) {
	case string:
		if newStr, ok := bscanReplaceQualifiedName(val, oldName, newName); ok {
			return newStr, true
		}
	case map[string]any:
		if bscanReplaceStringsInMap(val, oldName, newName) {
			return val, true
		}
	case bson.M:
		m := map[string]any(val)
		if bscanReplaceStringsInMap(m, oldName, newName) {
			return val, true
		}
	case bson.A:
		changed := false
		for i, elem := range val {
			if replaced, ok := bscanReplaceInValue(elem, oldName, newName); ok {
				val[i] = replaced
				changed = true
			}
		}
		if changed {
			return val, true
		}
	case []any:
		changed := false
		for i, elem := range val {
			if replaced, ok := bscanReplaceInValue(elem, oldName, newName); ok {
				val[i] = replaced
				changed = true
			}
		}
		if changed {
			return val, true
		}
	case bson.D:
		changed := false
		for i, elem := range val {
			if replaced, ok := bscanReplaceInValue(elem.Value, oldName, newName); ok {
				val[i].Value = replaced
				changed = true
			}
		}
		if changed {
			return val, true
		}
	}
	return v, false
}

// bscanReplaceQualifiedName checks if s matches oldName exactly or as a prefix
// (e.g., "OldModule.Microflow.Param") and returns the replacement.
func bscanReplaceQualifiedName(s, oldName, newName string) (string, bool) {
	if s == oldName {
		return newName, true
	}
	if strings.HasPrefix(s, oldName+".") {
		return newName + s[len(oldName):], true
	}
	return "", false
}
