// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/mpr/version"
	mversion "github.com/mendixlabs/mxcli/modelsdk/version"
)

// PropertySetRepair describes what normalizing one stored unit's property sets
// changes: how many property NAMES it adds to (canon.CompletePropertySets) and
// strips from (canon.StripUndeclaredProperties) the elements the unit already
// holds. Keys inside a value the completion adds are not counted — they belong
// to an element the stored unit did not have.
type PropertySetRepair struct {
	UnitID   string
	Type     string // the unit's $Type
	Name     string // the unit's Name, "" when it has none
	Added    int
	Stripped int

	// normalized is the unit as the repair leaves it (the stored bytes when
	// nothing changes), for a PropertySetCensus; nil when it could not be
	// normalized.
	normalized []byte
}

// Changed reports whether the unit's property-name sets differ from what is
// stored — the condition under which Mendix's merge engine refuses to compare
// it with the revision Studio Pro will save.
func (p PropertySetRepair) Changed() bool { return p.Added+p.Stripped > 0 }

// StoredPropertySetGaps reports what CompleteStoredPropertySets would change in
// a stored unit, without writing anything. An error means the unit cannot be
// normalized — typically a key the project's version does not declare that
// holds a value, which StripUndeclaredProperties refuses rather than drop.
func (r *Reader) StoredPropertySetGaps(unitID string) (PropertySetRepair, error) {
	rep, _, _, err := planPropertySetRepair(r, unitID)
	return rep, err
}

// CompleteStoredPropertySets gives a STORED unit the property set Studio Pro
// writes for each of its elements on this project's version, and writes it
// when the property-name sets changed (ako/mxcli#1097).
//
// # Why this is not just another write
//
// Every write already normalizes (normalizeForStorage), so a unit that a
// statement rewrites is completed on the way. The units nothing rewrites are
// not: a project built by an older mxcli keeps every gap in documents no
// later statement touches — a module's Projects$ModuleSettings, a microflow
// whose script was never re-run — and Mendix's merge engine throws on each
// ("Objects with ID … do not have the same properties"). This is the pass that
// reaches them.
//
// # What it writes
//
// The stored bytes plus the keys the table adds and minus the empty keys the
// version does not declare — nothing else. No element $ID moves (the content IS
// the stored content), no GUID is added or changed (the table holds none, and
// the storage-GUID guard still runs on the result), and a nested default value
// gets the derived, stable $IDs CompletePropertySets gives it. The write is
// made only when the NAME sets differ, so a second run writes nothing.
//
// It goes through the ordinary update path — canon.Reconcile and every guard
// of updateUnit, the storage-GUID guard among them — rather than around it.
// Nothing there needs bypassing: Reconcile's carries are the identity on a
// document that IS the stored one, and its elision compares the canonical form
// strictly, so a unit whose property names changed is never judged equal to
// what is stored. (Measured: TestStoredUnitMissingAKeyIsCompletedByAnyRewrite.)
// What this adds is the reach, and the decision — made on property NAMES, so
// a unit with nothing to repair is not offered at all.
func (w *Writer) CompleteStoredPropertySets(unitID string) (PropertySetRepair, error) {
	rep, _, out, err := planPropertySetRepair(w.reader, unitID)
	if err != nil || !rep.Changed() {
		return rep, err
	}
	landed := w.writesLanded
	if err := w.updateUnit(unitID, out); err != nil {
		return rep, err
	}
	if w.writesLanded == landed && w.sessionBuf == nil && w.deferred == nil {
		return rep, fmt.Errorf("unit %s: the property-set repair was elided as unchanged", unitID)
	}
	return rep, nil
}

// planPropertySetRepair reads a stored unit, normalizes it for the project's
// version, and counts the property names that changed on the elements the
// stored unit holds.
func planPropertySetRepair(r *Reader, unitID string) (rep PropertySetRepair, stored, out []byte, err error) {
	rep.UnitID = unitID
	stored, err = r.GetRawUnitBytes(unitID)
	if err != nil {
		return rep, nil, nil, err
	}
	raw := bson.Raw(stored)
	rep.Type, _ = raw.Lookup("$Type").StringValueOK()
	rep.Name, _ = raw.Lookup("Name").StringValueOK()

	out, err = normalizePropertySetsFor(r.ProjectVersion(), unitID, stored)
	if err != nil {
		return rep, stored, nil, err
	}
	rep.normalized = out
	if len(out) == len(stored) && string(out) == string(stored) {
		return rep, stored, out, nil
	}
	before, err := elementKeySets(stored)
	if err != nil {
		return rep, stored, nil, fmt.Errorf("unit %s: %w", unitID, err)
	}
	after, err := elementKeySets(out)
	if err != nil {
		return rep, stored, nil, fmt.Errorf("unit %s: %w", unitID, err)
	}
	for path, keys := range before {
		now, ok := after[path]
		if !ok {
			// Normalization never removes an element; if it did, every key went.
			rep.Stripped += len(keys)
			continue
		}
		for k := range keys {
			if !now[k] {
				rep.Stripped++
			}
		}
		for k := range now {
			if !keys[k] {
				rep.Added++
			}
		}
	}
	return rep, stored, out, nil
}

// lowerToProjectShape is canon.LowerToProjectShape, a variable so a test can
// stub the lowering and show what the checks after it see without it.
var lowerToProjectShape = canon.LowerToProjectShape

// normalizePropertySetsFor is normalizePropertySets for a given project
// version, shared by the write path and the stored-unit repair.
func normalizePropertySetsFor(pv *version.ProjectVersion, unitID string, contents []byte) ([]byte, error) {
	return storageFormFor(pv, unitID, contents, false)
}

// storageFormFor is normalizePropertySetsFor, and with listMarkers the list
// markers Studio Pro writes as well (canon.NormalizeListMarkers) — everything
// the write path does to a unit's bytes before they are stored or compared.
func storageFormFor(pv *version.ProjectVersion, unitID string, contents []byte, listMarkers bool) ([]byte, error) {
	if pv == nil || pv.MajorVersion == 0 {
		if listMarkers {
			return canon.NormalizeListMarkers(contents), nil
		}
		return contents, nil
	}
	v := mversion.Version{Major: pv.MajorVersion, Minor: pv.MinorVersion, Patch: pv.PatchVersion}
	// Properties whose shape changed at a version are written in the project's
	// shape first, so the floor sees the keys this version declares
	// (ako/mxcli#1209). A version that has every shape is not parsed for it.
	lowered, err := lowerToProjectShape(contents, &v)
	if err != nil {
		return nil, fmt.Errorf("unit %s: %w", unitID, err)
	}
	// Then, on one parse of the document (canon.CompleteForStorage):
	//
	//   - keys this version does not declare are stripped, or refused when
	//     they hold a real value;
	//   - keys Studio Pro always writes are completed;
	//   - a GUID-less user role gets its $ID as GUID — absent only, never
	//     changed (canon.FillMissingUserRoleGUIDs). Not a table row: the value
	//     is per role;
	//   - an element whose $Type this version does not have yet makes the
	//     project unloadable (TypeCacheUnknownTypeException). What has an older
	//     shape was lowered above; what is left is refused, on the bytes that
	//     would land (ako/mxcli#1774).
	out, err := canon.CompleteForStorage(lowered, &v, listMarkers)
	if err != nil {
		return nil, fmt.Errorf("unit %s: %w", unitID, err)
	}
	return out, nil
}

// elementKeySets maps every document node in raw, by its containment path, to
// the set of its keys. Normalization adds and removes keys but never reorders a
// list, so the same path names the same node on both sides.
func elementKeySets(raw []byte) (map[string]map[string]bool, error) {
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := map[string]map[string]bool{}
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch x := v.(type) {
		case bson.D:
			keys := make(map[string]bool, len(x))
			for _, e := range x {
				keys[e.Key] = true
				walk(e.Value, path+"/"+e.Key)
			}
			out[path] = keys
		case bson.A:
			for i, e := range x {
				walk(e, path+"["+strconv.Itoa(i)+"]")
			}
		}
	}
	walk(doc, "")
	return out, nil
}
