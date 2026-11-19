// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
)

// ako/mxcli#1640: every write gives a list with a registered owner/key marker
// that marker, the stored lists a rewrite passes through included
// (canon.NormalizeListMarkers). That is only safe if each registration is what
// Studio Pro writes, so this census holds every one of them against the
// Studio Pro-authored fixtures: a document Studio Pro wrote must be a fixed
// point of the normalization.

// studioProFixture is one Studio Pro-authored fixture this census reads.
type studioProFixture struct {
	root string
	// minUnits guards against reading a truncated fixture. Per root, not over
	// the total: a single floor over both made the census fail when the
	// optional submodule was absent, which is the same all-or-nothing the skip
	// below exists to avoid.
	minUnits int
	// skipIfMissing is the remedy printed when an OPTIONAL fixture is absent.
	// Empty for a committed fixture, which must always be present.
	skipIfMissing string
}

// studioProFixtures are the roots the census holds the marker registrations
// against. A document Studio Pro wrote must be a fixed point of the
// normalization, so these have to be Studio Pro-authored, not mxcli-written.
var studioProFixtures = []studioProFixture{
	{
		// ako/TestApp, pinned as the git submodule testdata/testapp. Not
		// committed, so a plain `git clone` does not have it.
		root:     "../../../testdata/testapp/TestApp/mprcontents",
		minUnits: 800,
		skipIfMissing: "the TestApp fixture is a git submodule that is not initialised; " +
			"run `git submodule update --init testdata/testapp` to include it in the census",
	},
	{
		// Committed, so never skipped: it is what keeps this census meaningful
		// when the submodule is absent.
		root:     "../../../testdata/pedapp/mprcontents",
		minUnits: 300,
	},
}

// studioProUnits lists the .mxunit files of the Studio Pro-authored fixtures.
//
// An absent OPTIONAL fixture skips that root rather than failing the test
// (ako/mxcli#2413). It used to walk straight into filepath.WalkDir's lstat
// error and t.Fatal, so on any clone without submodules `make ci-local` could
// not pass — and .claude/skills/steward/SKILL.md tells every contributor to run
// it before each push. A pre-existing failure was then indistinguishable from
// one just caused, which cost a real session its time. mdl/roundtrip already
// skips this same fixture, with this same remedy, so the two now agree.
//
// Under CI it FAILS instead of skipping. A skip that can hide the test
// everywhere is how an integration test came to have only ever skipped
// (ako/mxcli#808), and this census is the only thing holding canon's list
// marker registrations against Studio Pro-authored documents. CI checks the
// submodule out (`submodules: true` in .github/workflows/push-test.yml), so an
// absence there is a regression in the workflow, not a developer's clone.
func studioProUnits(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, f := range studioProFixtures {
		if f.skipIfMissing != "" {
			if _, err := os.Stat(f.root); err != nil {
				if os.Getenv("CI") != "" {
					t.Fatalf("%s is missing under CI, which checks submodules out: %v", f.root, err)
				}
				t.Logf("skipping %s: %s", f.root, f.skipIfMissing)
				continue
			}
		}
		n := 0
		err := filepath.WalkDir(f.root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".mxunit") {
				out = append(out, p)
				n++
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if n < f.minUnits {
			t.Fatalf("found only %d units under %s (want at least %d)", n, f.root, f.minUnits)
		}
	}
	if len(out) == 0 {
		t.Fatal("no fixture units found at all")
	}
	return out
}

// markerCensus counts, per "Owner.Key", the markers stored for every list.
func markerCensus(t *testing.T) map[string]map[int32]int {
	t.Helper()
	census := map[string]map[int32]int{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			owner := ""
			for _, e := range x {
				if e.Key == "$Type" {
					owner, _ = e.Value.(string)
				}
			}
			for _, e := range x {
				if a, ok := e.Value.(bson.A); ok && len(a) > 0 {
					if m, ok := a[0].(int32); ok && owner != "" {
						k := owner + "." + e.Key
						if census[k] == nil {
							census[k] = map[int32]int{}
						}
						census[k][m]++
					}
				}
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	for _, p := range studioProUnits(t) {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		walk(doc)
	}
	return census
}

func TestIssue1640_RegisteredListMarkersAreWhatStudioProStores(t *testing.T) {
	census := markerCensus(t)
	var wrong, measured []string
	for key, byMarker := range census {
		owner, prop, _ := strings.Cut(key, ".")
		want, ok := canon.PropertyListMarker(owner, prop)
		if !ok {
			continue
		}
		total := 0
		for _, n := range byMarker {
			total += n
		}
		if byMarker[want] != total {
			wrong = append(wrong, key)
			t.Errorf("%s is registered as %d, but Studio Pro stores %v", key, want, byMarker)
			continue
		}
		measured = append(measured, key)
	}
	sort.Strings(measured)
	t.Logf("%d registered owner/key markers agree with every stored list: %s", len(measured), strings.Join(measured, ", "))

	// And a Studio Pro document passes through the normalization unchanged.
	changed := 0
	for _, p := range studioProUnits(t) {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if out := canon.NormalizeListMarkers(raw); string(out) != string(raw) {
			changed++
			if changed <= 5 {
				t.Errorf("normalizing changes the Studio Pro unit %s", p)
			}
		}
	}
	if changed > 0 {
		t.Errorf("%d Studio Pro units change under the normalization", changed)
	}
}
