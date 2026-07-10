// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UpdateQualifiedNameInAllUnits rewrites every reference to oldName across all
// units to newName — the cross-cutting rename primitive (e.g. after renaming a
// microflow, fix every caller). It is a raw-BSON traversal: each unit is decoded
// to an ordered document, every string value that equals oldName or is prefixed
// "oldName." is rewritten, and changed units are written back. Returns the number
// of units updated. Mirrors the legacy writer (but preserves field order via
// bson.D rather than an unordered map).
func (b *Backend) UpdateQualifiedNameInAllUnits(oldName, newName string) (int, error) {
	if b.writer == nil {
		return 0, fmt.Errorf("UpdateQualifiedNameInAllUnits: not connected for writing")
	}
	ids, err := b.reader.ListAllUnitIDs()
	if err != nil {
		return 0, fmt.Errorf("UpdateQualifiedNameInAllUnits: list units: %w", err)
	}
	updated := 0
	for _, id := range ids {
		raw, err := b.reader.GetRawUnitBytes(id)
		if err != nil || len(raw) == 0 {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			continue
		}
		if replaced, ok := replaceQNInValue(doc, oldName, newName); ok {
			contents, err := bson.Marshal(replaced)
			if err != nil {
				return updated, fmt.Errorf("UpdateQualifiedNameInAllUnits: marshal %s: %w", id, err)
			}
			if err := b.writer.UpdateRawUnit(id, contents); err != nil {
				return updated, fmt.Errorf("UpdateQualifiedNameInAllUnits: update %s: %w", id, err)
			}
			updated++
		}
	}
	return updated, nil
}

// RenameReferences scans every unit and replaces qualified-name strings matching
// oldName (exact or "oldName."-prefixed) with newName, reporting one RenameHit per
// affected unit. When dryRun is true no unit is written — only the hit list is
// returned (the `mxcli rename ... --dry-run` / "scan references" path). Mirrors the
// legacy writer.RenameReferences but uses the codec reader's raw primitives.
func (b *Backend) RenameReferences(oldName, newName string, dryRun bool) ([]types.RenameHit, error) {
	if !dryRun && b.writer == nil {
		return nil, fmt.Errorf("RenameReferences: not connected for writing")
	}
	units, err := b.reader.ListUnits()
	if err != nil {
		return nil, fmt.Errorf("RenameReferences: list units: %w", err)
	}
	var hits []types.RenameHit
	for _, u := range units {
		raw, err := b.reader.GetRawUnitBytes(u.ID)
		if err != nil || len(raw) == 0 {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			continue
		}
		count := 0
		updated := replaceQNInDocCounted(doc, oldName, newName, &count)
		if count == 0 {
			continue
		}
		hits = append(hits, types.RenameHit{
			UnitID:   u.ID,
			UnitType: u.Type,
			Name:     docNameOf(updated),
			Count:    count,
		})
		if !dryRun {
			contents, err := bson.Marshal(updated)
			if err != nil {
				return hits, fmt.Errorf("RenameReferences: marshal %s: %w", u.ID, err)
			}
			if err := b.writer.UpdateRawUnit(u.ID, contents); err != nil {
				return hits, fmt.Errorf("RenameReferences: update %s: %w", u.ID, err)
			}
		}
	}
	return hits, nil
}

// RenameDocumentByName finds the document named oldName inside moduleName (directly
// or via a nested folder) and rewrites its top-level "Name" field to newName. Works
// for any document type via a raw BSON scan. Mirrors the legacy writer.
func (b *Backend) RenameDocumentByName(moduleName, oldName, newName string) error {
	if b.writer == nil {
		return fmt.Errorf("RenameDocumentByName: not connected for writing")
	}
	modules, err := b.reader.ListModules()
	if err != nil {
		return fmt.Errorf("RenameDocumentByName: list modules: %w", err)
	}
	var moduleID string
	for _, m := range modules {
		if m.Name == moduleName {
			moduleID = m.ID
			break
		}
	}
	if moduleID == "" {
		return fmt.Errorf("module not found: %s", moduleName)
	}
	containers := b.containerSetForModule(moduleID)

	units, err := b.reader.ListUnits()
	if err != nil {
		return fmt.Errorf("RenameDocumentByName: list units: %w", err)
	}
	for _, u := range units {
		if !containers[u.ContainerID] {
			continue
		}
		raw, err := b.reader.GetRawUnitBytes(u.ID)
		if err != nil || len(raw) == 0 {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			continue
		}
		for i, elem := range doc {
			if elem.Key != "Name" {
				continue
			}
			if s, ok := elem.Value.(string); ok && s == oldName {
				doc[i].Value = newName
				contents, err := bson.Marshal(doc)
				if err != nil {
					return fmt.Errorf("RenameDocumentByName: marshal: %w", err)
				}
				return b.writer.UpdateRawUnit(u.ID, contents)
			}
		}
	}
	return fmt.Errorf("document '%s.%s' not found", moduleName, oldName)
}

// containerSetForModule returns the module ID plus every folder ID nested under it
// (transitively), so a document-in-a-folder is recognised as belonging to the module.
func (b *Backend) containerSetForModule(moduleID string) map[string]bool {
	set := map[string]bool{moduleID: true}
	folders, err := b.reader.ListFolders()
	if err != nil {
		return set
	}
	for changed := true; changed; {
		changed = false
		for _, f := range folders {
			if set[f.ContainerID] && !set[f.ID] {
				set[f.ID] = true
				changed = true
			}
		}
	}
	return set
}

// docNameOf returns the top-level "Name" string of a decoded unit, or "".
func docNameOf(doc bson.D) string {
	for _, elem := range doc {
		if elem.Key == "Name" {
			if s, ok := elem.Value.(string); ok {
				return s
			}
		}
	}
	return ""
}

// replaceQNInDocCounted is replaceQNInValue with a running count of replacements,
// used by RenameReferences to report per-unit hit counts.
func replaceQNInDocCounted(doc bson.D, oldName, newName string, count *int) bson.D {
	for i, elem := range doc {
		doc[i].Value = replaceQNInValueCounted(elem.Value, oldName, newName, count)
	}
	return doc
}

func replaceQNInValueCounted(v any, oldName, newName string, count *int) any {
	switch val := v.(type) {
	case string:
		if val == oldName {
			*count++
			return newName
		}
		if strings.HasPrefix(val, oldName+".") {
			*count++
			return newName + val[len(oldName):]
		}
		return val
	case primitive.D:
		return replaceQNInDocCounted(val, oldName, newName, count)
	case primitive.A:
		for i, elem := range val {
			val[i] = replaceQNInValueCounted(elem, oldName, newName, count)
		}
		return val
	case []any:
		for i, elem := range val {
			val[i] = replaceQNInValueCounted(elem, oldName, newName, count)
		}
		return val
	}
	return v
}

// replaceQNInValue recursively rewrites qualified-name references in a decoded
// BSON value, returning the (possibly mutated) value and whether anything changed.
func replaceQNInValue(v any, oldName, newName string) (any, bool) {
	switch val := v.(type) {
	case string:
		return replaceQualifiedNameRef(val, oldName, newName)
	case primitive.D: // also matches bson.D (alias)
		changed := false
		for i, elem := range val {
			if nv, ok := replaceQNInValue(elem.Value, oldName, newName); ok {
				val[i].Value = nv
				changed = true
			}
		}
		return val, changed
	case primitive.A:
		changed := false
		for i, elem := range val {
			if nv, ok := replaceQNInValue(elem, oldName, newName); ok {
				val[i] = nv
				changed = true
			}
		}
		return val, changed
	case []any:
		changed := false
		for i, elem := range val {
			if nv, ok := replaceQNInValue(elem, oldName, newName); ok {
				val[i] = nv
				changed = true
			}
		}
		return val, changed
	}
	return v, false
}

// replaceQualifiedNameRef rewrites s when it equals oldName or is prefixed
// "oldName." (so "Mod.Old.Param" → "Mod.New.Param"), matching the legacy logic.
func replaceQualifiedNameRef(s, oldName, newName string) (any, bool) {
	if s == oldName {
		return newName, true
	}
	if strings.HasPrefix(s, oldName+".") {
		return newName + s[len(oldName):], true
	}
	return s, false
}
