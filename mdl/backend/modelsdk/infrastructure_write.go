// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/v2/bson"
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
	// A path step — `[Src.Assoc/Src.Entity/Attr = 1]` in an access rule,
	// `$O/Src.Assoc/Name` in an expression — names an entity or an association,
	// and only those: an enumeration renamed must not rewrite an association
	// that shares its name. By the time a move or rename sweeps, the element
	// already carries newName, so that is what decides (ako/mxcli#1836).
	paths := b.isDomainElementName(newName)
	rewrite := func(s string) (any, bool) {
		if v, ok := replaceQualifiedNameRef(s, oldName, newName); ok {
			return v, true
		}
		if paths {
			if v, n := types.RewritePathSteps(s, oldName, newName); n > 0 {
				return v, true
			}
		}
		return s, false
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
		if replaced, ok := replaceQNInValueWith(doc, rewrite); ok {
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

// RenameDocument rewrites the top-level "Name" of the document unit id from
// oldName to newName. Works for any document type via a raw BSON rewrite.
//
// The unit is addressed by ID because the caller has already resolved it among
// the documents of the kind it was asked to rename. This replaced a lookup by
// module and bare name over every unit in the module's containers, which took
// the first unit with that Name whatever its $Type: a folder created by a FOLDER
// clause is inserted before the document it holds, so `rename page M.X` on a page
// in folder 'X' renamed the folder (ako/mxcli#1443).
//
// Two refusals keep a wrong ID from doing the same damage. A folder or module is
// never renamed here — they are containers, not documents, and RENAME MODULE has
// its own path. And the stored Name must be oldName, so a stale or mismatched ID
// fails before anything is written instead of renaming whatever it points at.
func (b *Backend) RenameDocument(id model.ID, oldName, newName string) error {
	if b.writer == nil {
		return fmt.Errorf("RenameDocument: not connected for writing")
	}
	if id == "" {
		return fmt.Errorf("RenameDocument: no document to rename")
	}
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		return fmt.Errorf("RenameDocument: read %s: %w", id, err)
	}
	if len(raw) == 0 {
		return fmt.Errorf("RenameDocument: unit %s not found", id)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("RenameDocument: decode %s: %w", id, err)
	}
	unitType, _ := docField(doc, "$Type").(string)
	if unitType == "Projects$Folder" || unitType == "Projects$ModuleImpl" || unitType == "Projects$Module" {
		return fmt.Errorf("RenameDocument: unit %s is a %s, not a document", id, unitType)
	}
	for i, elem := range doc {
		if elem.Key != "Name" {
			continue
		}
		if s, _ := elem.Value.(string); s != oldName {
			return fmt.Errorf("RenameDocument: %s %s is named %q, not %q", unitType, id, s, oldName)
		}
		doc[i].Value = newName
		contents, err := bson.Marshal(doc)
		if err != nil {
			return fmt.Errorf("RenameDocument: marshal: %w", err)
		}
		return b.writer.UpdateRawUnit(string(id), contents)
	}
	return fmt.Errorf("RenameDocument: %s %s has no Name", unitType, id)
}

// docField returns the value of a top-level field of a decoded unit, or nil.
func docField(doc bson.D, key string) any {
	for _, elem := range doc {
		if elem.Key == key {
			return elem.Value
		}
	}
	return nil
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
		// A view entity's OQL is the one place a qualified name is stored inside
		// a sentence rather than as a property of its own, so the whole-string
		// match below never sees it. Without this, renaming an entity a view
		// selects from left the view pointing at a name that no longer exists —
		// CE0174 "Cannot resolve object name" — and reported 0 references
		// updated. See types.RewriteOQLQualifiedName.
		if elem.Key == oqlPropertyKey {
			if q, ok := elem.Value.(string); ok {
				if rewritten, n := types.RewriteOQLQualifiedName(q, oldName, newName); n > 0 {
					*count += n
					doc[i].Value = rewritten
				}
				continue
			}
		}
		doc[i].Value = replaceQNInValueCounted(elem.Value, oldName, newName, count)
	}
	return doc
}

// oqlPropertyKey is where DomainModels$OqlViewEntitySource keeps the query.
const oqlPropertyKey = "Oql"

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
		// A reference inside an expression: @Module.Constant, or an
		// enumeration value in a longer expression (ako/mxcli#1671).
		if rewritten, n := types.RewriteExpressionReferences(val, oldName, newName); n > 0 {
			*count += n
			return rewritten
		}
		return val
	case bson.D:
		return replaceQNInDocCounted(val, oldName, newName, count)
	case bson.A:
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

// isDomainElementName reports whether qn names an entity, an association or a
// cross-module association in the project.
func (b *Backend) isDomainElementName(qn string) bool {
	mod, name, ok := strings.Cut(qn, ".")
	if !ok || name == "" || strings.Contains(name, ".") {
		return false
	}
	modules, err := b.ListModules()
	if err != nil {
		return false
	}
	var modID model.ID
	for _, m := range modules {
		if m.Name == mod {
			modID = m.ID
		}
	}
	if modID == "" {
		return false
	}
	dm, err := b.GetDomainModel(modID)
	if err != nil || dm == nil {
		return false
	}
	for _, e := range dm.Entities {
		if e.Name == name {
			return true
		}
	}
	for _, a := range dm.Associations {
		if a.Name == name {
			return true
		}
	}
	for _, a := range dm.CrossAssociations {
		if a.Name == name {
			return true
		}
	}
	return false
}

// replaceQNInValueWith recursively applies rewrite to every string in a decoded
// BSON value, returning the (possibly mutated) value and whether anything
// changed.
func replaceQNInValueWith(v any, rewrite func(string) (any, bool)) (any, bool) {
	switch val := v.(type) {
	case string:
		return rewrite(val)
	case bson.D:
		changed := false
		for i, elem := range val {
			if nv, ok := replaceQNInValueWith(elem.Value, rewrite); ok {
				val[i].Value = nv
				changed = true
			}
		}
		return val, changed
	case bson.A:
		changed := false
		for i, elem := range val {
			if nv, ok := replaceQNInValueWith(elem, rewrite); ok {
				val[i] = nv
				changed = true
			}
		}
		return val, changed
	case []any:
		changed := false
		for i, elem := range val {
			if nv, ok := replaceQNInValueWith(elem, rewrite); ok {
				val[i] = nv
				changed = true
			}
		}
		return val, changed
	}
	return v, false
}

// replaceQualifiedNameRef rewrites s when it equals oldName or is prefixed
// "oldName." (so "Mod.Old.Param" → "Mod.New.Param"), matching the legacy logic,
// and otherwise rewrites the references to oldName inside it as an expression —
// @Module.Constant, or an enumeration value in a longer expression — which a
// cross-module move of a constant left naming the old module (ako/mxcli#1671).
func replaceQualifiedNameRef(s, oldName, newName string) (any, bool) {
	if s == oldName {
		return newName, true
	}
	if strings.HasPrefix(s, oldName+".") {
		return newName + s[len(oldName):], true
	}
	if rewritten, n := types.RewriteExpressionReferences(s, oldName, newName); n > 0 {
		return rewritten, true
	}
	return s, false
}
