// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// pedAppUnits is every unit of the Studio Pro-authored PedApp fixture, as
// stored.
func pedAppUnits(t *testing.T) map[string][]byte {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "pedapp", "mprcontents")
	units := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".mxunit" {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		units[filepath.Base(path)] = raw
		return nil
	})
	if err != nil {
		t.Fatalf("read the PedApp units: %v", err)
	}
	if len(units) < 300 {
		t.Fatalf("found %d PedApp units under %s, expected the whole fixture", len(units), root)
	}
	return units
}

// thinned is raw as a writer that leaves things out, and gets a marker wrong,
// would have built it: every nth property that is not a $-key dropped, and the
// marker of every nth list changed. It gives each pass something to do on a
// document Studio Pro wrote complete.
func thinned(t *testing.T, raw []byte, n int) []byte {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	seen := 0
	var thin func(v any) any
	thin = func(v any) any {
		switch x := v.(type) {
		case bson.D:
			out := make(bson.D, 0, len(x))
			for _, e := range x {
				if len(e.Key) > 0 && e.Key[0] != '$' {
					seen++
					if seen%n == 0 {
						continue
					}
				}
				out = append(out, bson.E{Key: e.Key, Value: thin(e.Value)})
			}
			return out
		case bson.A:
			out := make(bson.A, len(x))
			for i, e := range x {
				out[i] = thin(e)
			}
			if len(x) > 0 {
				if m, ok := x[0].(int32); ok {
					seen++
					if seen%n == 0 {
						out[0] = m%3 + 1
					}
				}
			}
			return out
		}
		return v
	}
	out, err := bson.Marshal(thin(doc))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

// rewritten is raw with every element passed through fn, which may drop keys or
// change values in place.
func rewritten(t *testing.T, raw []byte, fn func(typ string, d bson.D) bson.D) []byte {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var walk func(v any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case bson.D:
			for i := range x {
				x[i].Value = walk(x[i].Value)
			}
			typ, _ := lookupString(x, "$Type")
			return fn(typ, x)
		case bson.A:
			for i := range x {
				x[i] = walk(x[i])
			}
		}
		return v
	}
	out, err := bson.Marshal(walk(doc))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

// withoutUserRoleGUIDs is raw with the GUID of every user role removed, as a
// project older builds wrote has them.
func withoutUserRoleGUIDs(t *testing.T, raw []byte) []byte {
	return rewritten(t, raw, func(typ string, d bson.D) bson.D {
		if typ != "Security$UserRole" {
			return d
		}
		out := d[:0]
		for _, e := range d {
			if e.Key != "GUID" {
				out = append(out, e)
			}
		}
		return out
	})
}

// withOtherListMarkers is raw with the marker of every list changed.
func withOtherListMarkers(t *testing.T, raw []byte) []byte {
	return rewritten(t, raw, func(_ string, d bson.D) bson.D {
		for _, e := range d {
			if a, ok := e.Value.(bson.A); ok && len(a) > 0 {
				if m, ok := a[0].(int32); ok {
					a[0] = m%3 + 1
				}
			}
		}
		return d
	})
}

// registerFixtureListMarkers registers, for the length of the test, the marker
// of every list the units hold, by owner type and key. In the product the
// codec's packages register them; this package's tests import none of those,
// and with no marker registered the list-marker pass has nothing to do.
func registerFixtureListMarkers(t *testing.T, units map[string][]byte) {
	t.Helper()
	saved := propertyListMarkers
	propertyListMarkers = map[string]map[string]int32{}
	t.Cleanup(func() { propertyListMarkers = saved })
	for _, raw := range units {
		rewritten(t, raw, func(typ string, d bson.D) bson.D {
			for _, e := range d {
				if a, ok := e.Value.(bson.A); ok && len(a) > 0 && typ != "" {
					if m, ok := a[0].(int32); ok {
						if _, have := propertyListMarkers[typ][e.Key]; !have {
							RegisterPropertyListMarker(typ, e.Key, m)
						}
					}
				}
			}
			return d
		})
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// CompleteForStorage is defined as its passes run one after the other, each on
// the bytes of the one before. Sharing one parse puts each pass on what the
// earlier ones left in memory instead — a value a pass appended has not been
// through an encode and a decode — so the two are compared byte for byte, on
// every PedApp unit as stored and with properties left out, at versions on
// both sides of what the floors and the measured defaults distinguish.
func TestCompleteForStorage_ReturnsWhatItsPassesReturnInTurn(t *testing.T) {
	units := pedAppUnits(t)
	registerFixtureListMarkers(t, units)
	versions := []version.Version{
		{Major: 9, Minor: 24}, {Major: 10, Minor: 24}, {Major: 11, Minor: 13}, {Major: 12},
	}
	compared, changed, refused := 0, 0, 0
	filled, marked := 0, 0
	for name, stored := range units {
		variants := [][]byte{stored, thinned(t, stored, 5), withoutUserRoleGUIDs(t, stored), withOtherListMarkers(t, stored)}
		if !bytes.Equal(FillMissingUserRoleGUIDs(variants[2]), variants[2]) {
			filled++
		}
		if !bytes.Equal(NormalizeListMarkers(variants[3]), variants[3]) {
			marked++
		}
		for vi, raw := range variants {
			for i := range versions {
				pv := versions[i]
				// Without list markers is the stored-unit repair's form: compared
				// on the units as stored, where it is used.
				for _, markers := range []bool{true, false}[:2-min(vi, 1)] {
					want, wantErr := completeForStorageInPasses(raw, &pv, markers)
					got, gotErr := CompleteForStorage(raw, &pv, markers)
					compared++
					if errText(gotErr) != errText(wantErr) {
						t.Fatalf("%s at %s (markers %v): error %q, the passes in turn give %q", name, pv, markers, errText(gotErr), errText(wantErr))
					}
					if wantErr != nil {
						refused++
						continue
					}
					if !bytes.Equal(got, want) {
						t.Fatalf("%s at %s (markers %v): %d bytes, the passes in turn give %d different ones", name, pv, markers, len(got), len(want))
					}
					if !bytes.Equal(want, raw) {
						changed++
					} else if len(got) > 0 && &got[0] != &raw[0] {
						t.Fatalf("%s at %s: an unchanged document was returned as a copy", name, pv)
					}
				}
			}
		}
	}
	t.Logf("%d comparisons: %d changed the document, %d refused it", compared, changed, refused)
	// Controls: each pass must have had something to do somewhere, and the
	// comparison must have covered all three outcomes.
	if filled == 0 || marked == 0 {
		t.Fatalf("a pass had nothing to do: %d units needed a user role GUID, %d a list marker", filled, marked)
	}
	if changed == 0 || refused == 0 || compared-changed-refused == 0 {
		t.Fatalf("the comparison did not cover every outcome: %d changed, %d refused, %d unchanged", changed, refused, compared-changed-refused)
	}
}

// One parse for a unit, where the passes in turn made one each.
func TestCompleteForStorage_ParsesTheDocumentOnce(t *testing.T) {
	units := pedAppUnits(t)
	pv := version.Version{Major: 11, Minor: 13}
	var raw []byte
	for _, stored := range units {
		if thin := thinned(t, stored, 5); len(thin) > len(raw) {
			raw = thin
		}
	}
	before := StorageFormParses()
	if _, err := completeForStorageInPasses(raw, &pv, true); err != nil {
		t.Fatalf("passes in turn: %v", err)
	}
	inTurn := StorageFormParses() - before
	before = StorageFormParses()
	if _, err := CompleteForStorage(raw, &pv, true); err != nil {
		t.Fatalf("CompleteForStorage: %v", err)
	}
	once := StorageFormParses() - before
	t.Logf("parses: %d by the passes in turn, %d sharing one", inTurn, once)
	if inTurn < 2 {
		t.Fatalf("control: the passes in turn parsed %d times, so the count is not measuring them", inTurn)
	}
	if once != 1 {
		t.Errorf("CompleteForStorage parsed the document %d times, want 1", once)
	}
}
