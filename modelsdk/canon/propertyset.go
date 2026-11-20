// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// CompletePropertySets adds to every element in contents the properties Studio
// Pro always serializes for its $Type but the writer left out, each with the
// value Mendix itself gives an absent one. It returns contents unchanged — the
// same slice — when nothing was missing.
//
// # Why
//
// Mendix's merge and diff engine compares two revisions of an element by its
// property NAMES before it looks at a single value, and throws rather than
// falling back to a default when the sets differ:
//
//	System.InvalidOperationException: Objects with ID … of type
//	Microflows$LoopedActivity do not have the same properties.
//	baseNames = …; newNames = Documentation, …
//
// So an element mxcli writes without, say, an empty Documentation is a valid
// model — mx check is clean, Studio Pro opens it, the runtime runs it — and
// still poisons version control: the first time Studio Pro saves that document
// it writes the full set, and every diff, merge and the Version Control status
// panel between the two revisions crashes (mendixlabs/mxcli#1373). It does not
// need an $ID carried from a stored document to happen; a brand-new element
// fails the same way one save later.
//
// # Where the values come from
//
// Not from the metamodel and not from intuition: from Mendix. The table in
// studiopro_property_defaults.json is MEASURED — mxcli writes the doctype corpus
// into a fresh project, `mx convert -p` re-serializes every unit that is not
// canonical for its version (and, as the control shows, no other), and each
// (type, key) Mendix added is recorded with the value it wrote. Since that is
// the value Mendix fills in when it loads the absent property, adding it here
// changes what the model says by nothing; it only changes what it spells.
//
// # The version floor
//
// A key the project's metamodel does not declare is worse than a missing one:
// Studio Pro cannot open the document (see codec.Encoder.OmitKeys). Each entry
// therefore carries the lowest Mendix version it was measured on, and is added
// only to a project at least that new — and, for a key a newer measurement
// showed renamed or removed, only to one older than its Until. A key that
// exists in versions it was not measured on is left out on them: the gap this
// closes, not a new one. A nil version adds nothing.
//
// So Since is only as low as the oldest version anyone measured, and every
// version below it keeps its gaps. Measured from 10.24.24 up, the table gave
// 10.24.0–10.24.23 two of its rows, and a nanoflow re-created on 10.24.18
// broke `mx diff` (ako/mxcli#1505); measure the lowest patch of a version line,
// not the newest. Stripping a row's key from Studio Pro-authored elements and
// letting `mx convert` write it back measures the row without any mxcli writer
// leaving it out.
//
// # Limits
//
// It only ADDS. A key mxcli writes that Mendix's metamodel does not have is a
// writer defect fixed where it is written, not here. And it fills only keys
// whose measured value was the same everywhere; one Mendix derives from context
// is not in the table.
func CompletePropertySets(contents []byte, pv *version.Version) []byte {
	if pv == nil || len(contents) == 0 {
		return contents
	}
	defs := propertyDefaults()
	if len(defs) == 0 {
		return contents
	}
	var doc bson.D
	if err := parseForStorageForm(contents, &doc); err != nil {
		return contents
	}
	if !completeValue(&doc, defs, *pv, elementIDs(doc)) {
		return contents
	}
	out, err := bson.Marshal(doc)
	if err != nil {
		return contents
	}
	return out
}

// propertyDefault is one measured (type, key) entry.
type propertyDefault struct {
	key   string
	since version.Version
	until version.Version // exclusive; zero = no upper bound
	value any             // a bson value; documents are bson.D and are copied with fresh $IDs
}

// appliesTo reports whether the key exists in a project of version pv, as far
// as it was measured.
func (d propertyDefault) appliesTo(pv version.Version) bool {
	if pv.Compare(d.since) < 0 {
		return false
	}
	return d.until.IsZero() || pv.Compare(d.until) < 0
}

//go:embed studiopro_property_defaults.json
var propertyDefaultsJSON []byte

var (
	propertyDefaultsOnce sync.Once
	propertyDefaultsMap  map[string][]propertyDefault
)

// propertyDefaultEntry is the on-disk form of one entry.
type propertyDefaultEntry struct {
	Type  string          `json:"type"`
	Key   string          `json:"key"`
	Since string          `json:"since"`
	Until string          `json:"until,omitempty"` // exclusive
	Seen  string          `json:"seen,omitempty"`  // bookkeeping for internal/propertysets.Merge; not read
	Value json.RawMessage `json:"value"`           // canonical extended JSON of {"v": <value>}
}

func propertyDefaults() map[string][]propertyDefault {
	propertyDefaultsOnce.Do(func() {
		m, err := parsePropertyDefaults(propertyDefaultsJSON)
		if err != nil {
			// The table is embedded and covered by a unit test; a malformed one
			// must not take every write down with it.
			m = nil
		}
		propertyDefaultsMap = m
	})
	return propertyDefaultsMap
}

func parsePropertyDefaults(data []byte) (map[string][]propertyDefault, error) {
	var entries []propertyDefaultEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("property defaults: %w", err)
	}
	m := make(map[string][]propertyDefault, len(entries))
	for _, e := range entries {
		var wrapped bson.D
		if err := bson.UnmarshalExtJSON(e.Value, true, &wrapped); err != nil {
			return nil, fmt.Errorf("property defaults: %s.%s: %w", e.Type, e.Key, err)
		}
		if len(wrapped) != 1 || wrapped[0].Key != "v" {
			return nil, fmt.Errorf("property defaults: %s.%s: value must be {\"v\": …}", e.Type, e.Key)
		}
		since := version.Parse(e.Since)
		if since.IsZero() {
			return nil, fmt.Errorf("property defaults: %s.%s: missing since", e.Type, e.Key)
		}
		var until version.Version
		if e.Until != "" {
			until = version.Parse(e.Until)
			if until.Compare(since) <= 0 {
				return nil, fmt.Errorf("property defaults: %s.%s: until %s is not after since %s", e.Type, e.Key, e.Until, e.Since)
			}
		}
		m[e.Type] = append(m[e.Type], propertyDefault{key: e.Key, since: since, until: until, value: wrapped[0].Value})
	}
	return m, nil
}

// completeValue walks v and completes every element document in it. It reports
// whether anything was added.
func completeValue(v *bson.D, defs map[string][]propertyDefault, pv version.Version, used map[string]bool) bool {
	changed := false
	for i := range *v {
		if walkValue(&(*v)[i].Value, defs, pv, used) {
			changed = true
		}
	}
	typeName, _ := lookupString(*v, "$Type")
	for _, d := range defs[typeName] {
		if !d.appliesTo(pv) || hasKey(*v, d.key) {
			continue
		}
		*v = append(*v, bson.E{Key: d.key, Value: derivedCopy(d.value, elementIDBytes(*v), d.key, used)})
		changed = true
	}
	return changed
}

func walkValue(v *any, defs map[string][]propertyDefault, pv version.Version, used map[string]bool) bool {
	switch x := (*v).(type) {
	case bson.D:
		if completeValue(&x, defs, pv, used) {
			*v = x
			return true
		}
	case bson.A:
		changed := false
		for i := range x {
			if walkValue(&x[i], defs, pv, used) {
				changed = true
			}
		}
		return changed
	}
	return false
}

func hasKey(d bson.D, key string) bool {
	for _, e := range d {
		if e.Key == key {
			return true
		}
	}
	return false
}

func lookupString(d bson.D, key string) (string, bool) {
	for _, e := range d {
		if e.Key == key {
			s, ok := e.Value.(string)
			return s, ok
		}
	}
	return "", false
}

// derivedCopy deep-copies a table value, giving every element in it an $ID
// DERIVED from the element it is added to: the owner's $ID, the property key and
// the element's path inside the value. Two owners never share one (their $IDs
// differ), so the project never holds a duplicate. And the same owner always
// gets the same one, so completing the same element twice produces the same
// bytes. A random $ID made every re-encoding differ, which a byte-level
// comparison (the raw passthrough of `call web service raw`) read as a change
// on every re-run. An owner without a binary $ID (never seen) falls back to
// random ids.
func derivedCopy(v any, owner []byte, path string, used map[string]bool) any {
	switch x := v.(type) {
	case bson.D:
		out := make(bson.D, len(x))
		for i, e := range x {
			if e.Key == "$ID" {
				out[i] = bson.E{Key: "$ID", Value: completionID(owner, path, used)}
				continue
			}
			out[i] = bson.E{Key: e.Key, Value: derivedCopy(e.Value, owner, path+"/"+e.Key, used)}
		}
		return out
	case bson.A:
		out := make(bson.A, len(x))
		for i, e := range x {
			out[i] = derivedCopy(e, owner, fmt.Sprintf("%s[%d]", path, i), used)
		}
		return out
	case bson.Binary:
		return bson.Binary{Subtype: x.Subtype, Data: append([]byte(nil), x.Data...)}
	}
	return v
}

// elementIDBytes returns an element's binary $ID, or nil.
func elementIDBytes(d bson.D) []byte {
	for _, e := range d {
		if e.Key == "$ID" {
			if b, ok := e.Value.(bson.Binary); ok && len(b.Data) == 16 {
				return b.Data
			}
		}
	}
	return nil
}

// completionID is a version-5-style UUID (SHA-256 truncated) of owner and path,
// as binary subtype 0 like every element $ID. As derivedID does for carried
// translations, it steps past an id the document already holds — a collision
// nobody will see, but the one failure (a duplicate $ID makes the project
// unopenable) this must never produce — and records what it hands out.
func completionID(owner []byte, path string, used map[string]bool) bson.Binary {
	if owner == nil {
		id := freshID()
		used[string(id.Data)] = true
		return id
	}
	for n := 0; ; n++ {
		h := sha256.New()
		h.Write([]byte("mxcli/canon.CompletePropertySets\x00"))
		h.Write(owner)
		h.Write([]byte(path))
		h.Write([]byte(strconv.Itoa(n)))
		b := h.Sum(nil)[:16]
		b[6] = (b[6] & 0x0f) | 0x50
		b[8] = (b[8] & 0x3f) | 0x80
		if !used[string(b)] {
			used[string(b)] = true
			return bson.Binary{Subtype: 0, Data: b}
		}
	}
}

// freshID mints an element $ID in the form every writer uses: a random UUID as
// binary subtype 0.
func freshID() bson.Binary {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return bson.Binary{Subtype: 0, Data: b}
}
