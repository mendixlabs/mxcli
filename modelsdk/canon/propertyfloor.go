// SPDX-License-Identifier: Apache-2.0

package canon

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// StripUndeclaredProperties removes from every element in contents the
// properties the project's Mendix version does not declare yet — introduced in
// a later version — when they hold nothing, and refuses the write when one
// holds a value. It returns contents unchanged (the same slice) when there is
// nothing to strip.
//
// # Why
//
// Writers emit some keys unconditionally — an empty list from a codec default,
// a false flag, an empty string — whatever version they write to. A key the
// project's metamodel does not declare is worse than a missing one: Studio Pro
// resolves every stored property against the type and cannot open the document
// (codec.Encoder.OmitKeys), mxbuild tolerates it so `mx check` passes, and
// `mx convert` silently drops it, so the next Studio Pro save differs from
// mxcli's in property NAMES — which Mendix's merge engine refuses to compare
// (mendixlabs/mxcli#1373). Measured on 10.24.24 and 11.6.8: sixteen such keys,
// among them CustomWidgets$WidgetValueType.AllowUpload on every pluggable
// widget property.
//
// # Empty is dropped, a value is refused
//
// An empty value (null, "", false, 0, an empty list, a zero GUID) is what the
// version would have had without the key, so dropping it is exactly what
// `mx convert` does and loses nothing. A real value is something the statement
// said that this version cannot store; dropping it would silently write a
// different model, so the write is refused with the version it needs.
//
// # Where the versions come from
//
// The generated metamodel's version data (registered by
// modelsdk/gen/versioninfo; looked up by storage $Type and BSON key), and, for
// keys it has no data for, the measured studiopro_property_floors.json. A key
// with neither is left alone — the state before this pass. A nil version
// strips nothing.
func StripUndeclaredProperties(contents []byte, pv *version.Version) ([]byte, error) {
	if pv == nil || len(contents) == 0 {
		return contents, nil
	}
	var doc bson.D
	if err := parseForStorageForm(contents, &doc); err != nil {
		return contents, nil
	}
	s := &stripper{pv: *pv}
	if !s.doc(&doc) {
		if len(s.refused) > 0 {
			return nil, s.err()
		}
		return contents, nil
	}
	if len(s.refused) > 0 {
		return nil, s.err()
	}
	out, err := bson.Marshal(doc)
	if err != nil {
		return contents, nil
	}
	return out, nil
}

type stripper struct {
	pv      version.Version
	refused []string
}

func (s *stripper) err() error {
	seen := map[string]bool{}
	uniq := s.refused[:0]
	for _, r := range s.refused {
		if !seen[r] {
			seen[r] = true
			uniq = append(uniq, r)
		}
	}
	s.refused = uniq
	sort.Strings(s.refused)
	return fmt.Errorf("this Mendix %s project cannot store %s: the propert%s did not exist yet in this version, "+
		"and Studio Pro cannot open a document that holds one. Remove what needs it from the statement, or upgrade the project",
		s.pv, strings.Join(s.refused, "; "), map[bool]string{true: "y", false: "ies"}[len(s.refused) == 1])
}

// doc strips one element and everything under it; it reports a change.
func (s *stripper) doc(d *bson.D) bool {
	changed := false
	for i := range *d {
		if s.value(&(*d)[i].Value) {
			changed = true
		}
	}
	typeName, _ := lookupString(*d, "$Type")
	if typeName == "" {
		return changed
	}
	out := (*d)[:0]
	for _, e := range *d {
		// A key Mendix removed at or before this version: Studio Pro discards
		// it on load whatever it holds, so dropping it loses nothing and
		// removes a name the merge engine does not have (#1097: an older
		// writer's CloseFormAction.NumberOfPages, removed in 8.11). Measured
		// rows only: the generated data's Deleted versions are wrong for keys
		// Studio Pro still writes (it marks LayoutGridRow.Columns deleted in
		// 8.0), and trusting it stripped ~297k live keys from a Studio
		// Pro-authored 11.13 project.
		if gone, ok := measuredRemoved()[typeName+"."+e.Key]; ok && s.pv.Compare(gone) >= 0 {
			changed = true
			continue
		}
		since, ok := propertyFloor(typeName, e.Key)
		if !ok || s.pv.Compare(since) >= 0 {
			out = append(out, e)
			continue
		}
		if !isEmptyValue(e.Value) {
			s.refused = append(s.refused, fmt.Sprintf("%s.%s (Mendix %s+)", typeName, e.Key, since))
			out = append(out, e)
			continue
		}
		changed = true
	}
	*d = out
	return changed
}

func (s *stripper) value(v *any) bool {
	switch x := (*v).(type) {
	case bson.D:
		if s.doc(&x) {
			*v = x
			return true
		}
	case bson.A:
		changed := false
		for i := range x {
			if s.value(&x[i]) {
				changed = true
			}
		}
		return changed
	}
	return false
}

// isEmptyValue reports whether a value is what an absent property means: null,
// the zero scalar, an empty list (its typed-array marker alone), or an
// all-zero GUID.
func isEmptyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case bool:
		return !x
	case int32:
		return x == 0
	case int64:
		return x == 0
	case float64:
		return x == 0
	case bson.Binary:
		for _, b := range x.Data {
			if b != 0 {
				return false
			}
		}
		return true
	case bson.A:
		if len(x) == 0 {
			return true
		}
		if len(x) == 1 {
			_, marker := x[0].(int32)
			return marker
		}
	}
	return false
}

// PropertyDeclared reports whether a project of version pv declares the BSON
// key on the storage $Type typeName, by the same version data
// StripUndeclaredProperties drops and refuses on. A key with no version data is
// declared, as it is to the floor.
//
// It exists for writers that copy a schema authored for a newer Mendix — a
// pluggable widget's Type from an embedded 11.6 template — and must write it in
// the project's shape before the floor sees it, where a value it holds is not
// something the statement said and refusing it would refuse the widget.
func PropertyDeclared(typeName, key string, pv version.Version) bool {
	if gone, ok := measuredRemoved()[typeName+"."+key]; ok && pv.Compare(gone) >= 0 {
		return false
	}
	since, ok := propertyFloor(typeName, key)
	return !ok || pv.Compare(since) >= 0
}

// propertyFloor returns the first Mendix version that declares a type's key:
// the measured table first (it exists for keys the generated data lacks), then
// the generated metamodel's data.
func propertyFloor(typeName, key string) (version.Version, bool) {
	if v, ok := measuredFloors()[typeName+"."+key]; ok {
		return v, true
	}
	return version.PropertyIntroduced(typeName, key)
}

//go:embed studiopro_property_floors.json
var propertyFloorsJSON []byte

var (
	propertyFloorsOnce sync.Once
	propertyFloorsMap  map[string]version.Version
)

// propertyFloorEntry is one measured floor: the oldest Mendix version measured
// to declare the key (`mx convert` keeps it) where an older measured version
// does not (it strips it).
type propertyFloorEntry struct {
	Type  string `json:"type"`
	Key   string `json:"key"`
	Since string `json:"since"`
	// Absent is the newest measured version that does NOT declare the key —
	// evidence, not read.
	Absent string `json:"absent"`
}

func measuredFloors() map[string]version.Version {
	propertyFloorsOnce.Do(func() {
		m, err := parsePropertyFloors(propertyFloorsJSON)
		if err != nil {
			m = nil
		}
		propertyFloorsMap = m
	})
	return propertyFloorsMap
}

func parsePropertyFloors(data []byte) (map[string]version.Version, error) {
	var entries []propertyFloorEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("property floors: %w", err)
	}
	m := make(map[string]version.Version, len(entries))
	for _, e := range entries {
		v := version.Parse(e.Since)
		if v.IsZero() || e.Type == "" || e.Key == "" {
			return nil, fmt.Errorf("property floors: incomplete entry %+v", e)
		}
		if a := version.Parse(e.Absent); !a.IsZero() && a.Compare(v) >= 0 {
			return nil, fmt.Errorf("property floors: %s.%s absent on %s, not before its floor %s", e.Type, e.Key, e.Absent, e.Since)
		}
		m[e.Type+"."+e.Key] = v
	}
	return m, nil
}

//go:embed studiopro_property_removed.json
var propertyRemovedJSON []byte

var (
	propertyRemovedOnce sync.Once
	propertyRemovedMap  map[string]version.Version
)

// propertyRemovedEntry is one measured removal: a key Mendix declared once and
// removed in Since, which `mx convert` on Seen (or later) strips.
type propertyRemovedEntry struct {
	Type  string `json:"type"`
	Key   string `json:"key"`
	Since string `json:"since"`
	// Seen is the version the stripping was measured on — evidence, not read.
	Seen string `json:"seen"`
}

func measuredRemoved() map[string]version.Version {
	propertyRemovedOnce.Do(func() {
		m, err := parsePropertyRemoved(propertyRemovedJSON)
		if err != nil {
			m = nil
		}
		propertyRemovedMap = m
	})
	return propertyRemovedMap
}

func parsePropertyRemoved(data []byte) (map[string]version.Version, error) {
	var entries []propertyRemovedEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("removed properties: %w", err)
	}
	m := make(map[string]version.Version, len(entries))
	for _, e := range entries {
		v := version.Parse(e.Since)
		if v.IsZero() || e.Type == "" || e.Key == "" || version.Parse(e.Seen).IsZero() {
			return nil, fmt.Errorf("removed properties: incomplete entry %+v", e)
		}
		m[e.Type+"."+e.Key] = v
	}
	return m, nil
}
