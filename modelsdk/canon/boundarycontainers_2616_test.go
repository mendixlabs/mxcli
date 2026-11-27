// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// The fixtures are one workflow and one microflow mxcli wrote on Mendix 11.16.1
// — a user task with interrupting and non-interrupting timers and a
// non-interrupting notification event, a call microflow with a timer, a user
// task in a parallel split with a timer, and a notify workflow targeting the
// notification event — and the same units after mx convert to 11.17.0
// (ako/mxcli#2616). Conversion re-mints element $IDs, so the comparison leaves
// $ID out; everything else, PersistentIds included, must match.

func readFixture2616(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "boundary2616", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// shape2616 is a unit as JSON with every $ID removed and keys sorted, so two
// units compare by content.
func shape2616(t *testing.T, raw []byte) string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var strip func(v any) any
	strip = func(v any) any {
		switch n := v.(type) {
		case bson.D:
			m := map[string]any{}
			for _, e := range n {
				if e.Key != "$ID" {
					m[e.Key] = strip(e.Value)
				}
			}
			return m
		case bson.A:
			out := make([]any, len(n))
			for i, e := range n {
				out[i] = strip(e)
			}
			return out
		case bson.Binary:
			return n.Data
		}
		return v
	}
	b, err := json.MarshalIndent(strip(doc), "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var v1117 = &version.Version{Major: 11, Minor: 17, Patch: 0}

func TestRaiseBoundaryEventContainers_MatchesMxConvert_2616(t *testing.T) {
	for _, pair := range [][2]string{
		{"workflow_11.16.1.mxunit", "workflow_11.17.0_converted.mxunit"},
		{"notify_11.16.1.mxunit", "notify_11.17.0_converted.mxunit"},
	} {
		t.Run(pair[0], func(t *testing.T) {
			raised, err := RaiseBoundaryEventContainers(readFixture2616(t, pair[0]), v1117)
			if err != nil {
				t.Fatal(err)
			}
			got, want := shape2616(t, raised), shape2616(t, readFixture2616(t, pair[1]))
			if got != want {
				t.Errorf("raised 11.16.1 unit differs from mx convert's 11.17.0:\n%s", firstDiff(got, want))
			}
		})
	}
}

// Reading a stored 11.17 unit lowers it to the shape the readers decode, and
// writing it back raises it to exactly what was stored: names, PersistentIds
// and every other value survive the round trip.
func TestLowerThenRaise_IsTheStoredUnit_2616(t *testing.T) {
	for _, name := range []string{"workflow_11.17.0_converted.mxunit", "notify_11.17.0_converted.mxunit"} {
		t.Run(name, func(t *testing.T) {
			stored := readFixture2616(t, name)
			lowered := LowerBoundaryEventContainers(stored)
			if bytes.Contains(lowered, []byte(boundaryContainersKey)) || bytes.Contains(lowered, []byte(`"Activity"`)) {
				t.Fatal("lowering left an 11.17 key")
			}
			raised, err := RaiseBoundaryEventContainers(lowered, v1117)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := shape2616(t, raised), shape2616(t, stored); got != want {
				t.Errorf("lower then raise changed the unit:\n%s", firstDiff(got, want))
			}
		})
	}
}

// Below 11.17 nothing is raised: the keys are not in that metamodel.
func TestRaiseBoundaryEventContainers_Below1117_2616(t *testing.T) {
	in := readFixture2616(t, "workflow_11.16.1.mxunit")
	out, err := RaiseBoundaryEventContainers(in, &version.Version{Major: 11, Minor: 16, Patch: 1})
	if err != nil || !bytes.Equal(in, out) {
		t.Errorf("an 11.16.1 unit was changed (err %v)", err)
	}
}

func firstDiff(got, want string) string {
	g, w := []byte(got), []byte(want)
	i := 0
	for i < len(g) && i < len(w) && g[i] == w[i] {
		i++
	}
	lo := i - 300
	if lo < 0 {
		lo = 0
	}
	hiG, hiW := i+300, i+300
	if hiG > len(g) {
		hiG = len(g)
	}
	if hiW > len(w) {
		hiW = len(w)
	}
	return "--- got\n" + string(g[lo:hiG]) + "\n--- want\n" + string(w[lo:hiW])
}

// In the 11.17 shape a boundary event container and the event activity inside
// it share one PersistentId — mx convert gives both the old event's. The
// PersistentId carry refused any result with a shared one and fell back to the
// writer's fresh ids, so every write of an 11.17 workflow changed every id and
// no rewrite was ever elided.
func TestCarryPersistentIDs_KeepsAContainersSharedID_2616(t *testing.T) {
	stored := readFixture2616(t, "workflow_11.17.0_converted.mxunit")
	if hasDuplicatePersistentID(stored) {
		t.Fatal("mx convert's own 11.17 workflow reads as having a duplicate PersistentId")
	}
	// What a write produces: the same document with fresh ids, a container
	// sharing its event activity's as the raise writes it.
	var doc bson.D
	if err := bson.Unmarshal(LowerBoundaryEventContainers(stored), &doc); err != nil {
		t.Fatal(err)
	}
	var refresh func(v any) any
	refresh = func(v any) any {
		switch n := v.(type) {
		case bson.D:
			for i := range n {
				if n[i].Key == "PersistentId" {
					n[i].Value = freshID()
				} else {
					n[i].Value = refresh(n[i].Value)
				}
			}
			return n
		case bson.A:
			for i := range n {
				n[i] = refresh(n[i])
			}
			return n
		}
		return v
	}
	written, err := bson.Marshal(refresh(doc))
	if err != nil {
		t.Fatal(err)
	}
	written, err = RaiseBoundaryEventContainers(written, v1117)
	if err != nil {
		t.Fatal(err)
	}
	got := CarryPersistentIDs(TransplantIDs(written, stored), stored)
	if a, b := shape2616(t, got), shape2616(t, stored); a != b {
		t.Errorf("the stored PersistentIds were not carried:\n%s", firstDiff(a, b))
	}
}
