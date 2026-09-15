// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"fmt"
	"sort"
	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Identities maps an element's path within its module to the `GUID` the stored
// model holds for it.
//
// This is the load-bearing safety mechanism of a module update. The database
// keys entities and attributes on the model's `GUID` — measured, see
// PROPOSAL_marketplace_module_upgrade.md §8 — so an update that replaces a
// module's documents must carry every existing `GUID` onto its replacement or
// the next deploy silently destroys that module's data. Studio Pro's own update
// does exactly this transplant (§4: 9 of 9 preserved, while all 94 `$ID`s are
// renumbered).
type Identities map[string][]byte

// Paths returns the recorded paths in a stable order.
func (ids Identities) Paths() []string {
	out := make([]string, 0, len(ids))
	for p := range ids {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// CaptureIdentities records the `GUID` of every element of a module that carries
// one.
//
// Elements are keyed by their **path of names** within the document — an entity
// is `Account`, its attribute is `Account/FullName` — rather than by name alone,
// because names repeat: two entities in one domain model both having a `Name`
// attribute is the normal case, not an edge case. The path is also what survives
// the update, since a replace renumbers every `$ID` and keeps every name.
//
// The walk is deliberately type-agnostic: any BSON node carrying both a `Name`
// and a `GUID` is recorded, wherever it sits. New document types therefore need
// no registration here, which matters because the set of `GUID`-carrying types
// is not something mxcli can enumerate from the metamodel.
func CaptureIdentities(mprPath, moduleName string) (Identities, error) {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", mprPath, err)
	}
	defer reader.Disconnect()

	units, err := reader.ListUnits()
	if err != nil {
		return nil, fmt.Errorf("list units: %w", err)
	}

	// Resolve which units belong to the module. A document nests in folders, so
	// this walks the containment chain rather than reading one level — the same
	// trap that made foldered documents invisible to DESCRIBE (#759).
	inModule, err := unitsOfModule(reader, units, moduleName)
	if err != nil {
		return nil, err
	}
	if len(inModule) == 0 {
		return nil, fmt.Errorf("module %q has no units, so there is nothing to preserve", moduleName)
	}

	ids := Identities{}
	for _, unitID := range inModule {
		raw, err := reader.GetRawUnitBytes(model.ID(unitID))
		if err != nil || len(raw) == 0 {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			continue
		}
		collectIdentities(doc, nil, ids)
	}
	return ids, nil
}

// collectIdentities walks a decoded document, recording every Name+GUID pair
// under its path of enclosing names.
func collectIdentities(v any, trail []string, out Identities) {
	switch t := v.(type) {
	case bson.D:
		name, guid, hasGUID := nameAndGUID(t)
		next := trail
		if name != "" {
			next = append(append([]string{}, trail...), name)
			if hasGUID {
				out[strings.Join(next, "/")] = guid
			}
		}
		for _, e := range t {
			collectIdentities(e.Value, next, out)
		}
	case bson.A:
		for _, e := range t {
			collectIdentities(e, trail, out)
		}
	}
}

func nameAndGUID(d bson.D) (name string, guid []byte, hasGUID bool) {
	for _, e := range d {
		switch e.Key {
		case "Name":
			if s, ok := e.Value.(string); ok {
				name = s
			}
		case "GUID":
			if b, ok := e.Value.(primitive.Binary); ok && len(b.Data) > 0 {
				guid, hasGUID = b.Data, true
			}
		}
	}
	return name, guid, hasGUID
}

// unitsOfModule returns the IDs of every unit contained in the named module,
// directly or through any depth of folders.
func unitsOfModule(reader backend.FullBackend, units []*types.UnitInfo, moduleName string) ([]string, error) {
	parent := make(map[string]string, len(units))
	for _, u := range units {
		parent[string(u.ID)] = string(u.ContainerID)
	}

	mods, err := reader.ListModules()
	if err != nil {
		return nil, err
	}
	var moduleID string
	for _, m := range mods {
		if strings.EqualFold(m.Name, moduleName) {
			moduleID = string(m.ID)
			break
		}
	}
	if moduleID == "" {
		return nil, fmt.Errorf("module %q not found", moduleName)
	}

	var out []string
	for _, u := range units {
		if string(u.ID) == moduleID || descendsFrom(string(u.ID), moduleID, parent) {
			out = append(out, string(u.ID))
		}
	}
	return out, nil
}

// descendsFrom reports whether id sits under ancestor. The walk is bounded
// because the project root is its own container — an unguarded loop hangs there.
func descendsFrom(id, ancestor string, parent map[string]string) bool {
	seen := map[string]bool{}
	for cur := id; cur != "" && !seen[cur]; {
		seen[cur] = true
		p, ok := parent[cur]
		if !ok || p == cur {
			return false
		}
		if p == ancestor {
			return true
		}
		cur = p
	}
	return false
}

// ApplyIdentities writes recorded `GUID`s back onto a module's elements, matched
// by path, and reports what it could not place.
//
// This is the transplant half of the update: after a module's documents are
// replaced, every element that existed before must carry the `GUID` it had
// before, or the runtime treats it as a new entity and drops the old table
// (§8). Studio Pro does the same thing — its update renumbers all 94 `$ID`s and
// preserves all 9 `GUID`s (§4).
//
// Elements with no recorded identity are left exactly as they are, with their
// freshly minted `GUID`s: those are genuinely new in the target version, and a
// new element must not inherit an old one's identity. Recorded paths that no
// longer exist are returned as `missing` — an element the new version removed,
// which is information the caller needs rather than an error here.
//
// The write is per unit and only for units that actually changed, so a module
// whose identities all already match is not rewritten at all (ADR-0008).
func ApplyIdentities(mprPath, moduleName string, ids Identities) (applied int, missing []string, err error) {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return 0, nil, fmt.Errorf("open %s: %w", mprPath, err)
	}
	units, err := reader.ListUnits()
	if err != nil {
		reader.Disconnect()
		return 0, nil, fmt.Errorf("list units: %w", err)
	}
	inModule, err := unitsOfModule(reader, units, moduleName)
	if err != nil {
		reader.Disconnect()
		return 0, nil, err
	}

	placed := map[string]bool{}
	type pending struct {
		id       string
		contents []byte
	}
	var writes []pending

	for _, unitID := range inModule {
		raw, rerr := reader.GetRawUnitBytes(model.ID(unitID))
		if rerr != nil || len(raw) == 0 {
			continue
		}
		var doc bson.D
		if bson.Unmarshal(raw, &doc) != nil {
			continue
		}
		n := applyIdentities(doc, nil, ids, placed)
		if n == 0 {
			continue
		}
		encoded, merr := bson.Marshal(doc)
		if merr != nil {
			reader.Disconnect()
			return 0, nil, fmt.Errorf("re-encode unit %s: %w", unitID, merr)
		}
		writes = append(writes, pending{unitID, encoded})
		applied += n
	}
	reader.Disconnect()

	for _, p := range ids.Paths() {
		if !placed[p] {
			missing = append(missing, p)
		}
	}
	if len(writes) == 0 {
		return applied, missing, nil
	}

	writer, err := modelsdk.OpenForWriting(mprPath)
	if err != nil {
		return 0, nil, fmt.Errorf("open %s for writing: %w", mprPath, err)
	}
	defer writer.Disconnect()
	for _, w := range writes {
		if err := writer.UpdateRawUnit(w.id, w.contents); err != nil {
			return 0, nil, fmt.Errorf("write identities into unit %s: %w", w.id, err)
		}
	}
	return applied, missing, nil
}

// applyIdentities rewrites GUIDs in place, returning how many it changed and
// recording which recorded paths it found.
func applyIdentities(v any, trail []string, ids Identities, placed map[string]bool) int {
	changed := 0
	switch t := v.(type) {
	case bson.D:
		name, _, hasGUID := nameAndGUID(t)
		next := trail
		if name != "" {
			next = append(append([]string{}, trail...), name)
			if hasGUID {
				path := strings.Join(next, "/")
				if want, ok := ids[path]; ok {
					placed[path] = true
					for i, e := range t {
						if e.Key != "GUID" {
							continue
						}
						b := e.Value.(primitive.Binary)
						if !bytesEqual(b.Data, want) {
							t[i].Value = primitive.Binary{Subtype: b.Subtype, Data: append([]byte{}, want...)}
							changed++
						}
						break
					}
				}
			}
		}
		for _, e := range t {
			changed += applyIdentities(e.Value, next, ids, placed)
		}
	case bson.A:
		for _, e := range t {
			changed += applyIdentities(e, trail, ids, placed)
		}
	}
	return changed
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
