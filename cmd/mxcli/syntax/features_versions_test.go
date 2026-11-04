// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/versions"
)

// ako/mxcli#1367: `mxcli syntax` and sdk/versions/*.yaml kept separate version
// tables and had drifted apart both ways — workflow.boundary-event said 10.6.0
// where the registry says 10.14.0, view entities 10.18.0 against 10.21.0, and
// data-transformer had no version at all although the registry gates it. A
// topic now names registry features and its versions are read from there;
// Register refuses a hand-written one. These two tests close both directions.

// A topic may only name a feature the registry has: a typo would silently
// print no requirement.
func TestSyntaxFeaturesResolve(t *testing.T) {
	reg, err := versions.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range All() {
		for _, key := range f.Features {
			area, name, ok := cutFeatureKey(key)
			if !ok {
				t.Errorf("%s: feature %q is not area/name", f.Path, key)
				continue
			}
			if _, found := reg.Lookup(area, name); !found {
				t.Errorf("%s: feature %q is not in sdk/versions/*.yaml", f.Path, key)
			}
		}
		if len(f.Features) > 0 && (f.MinVersion == "" || len(f.Requires) != len(f.Features)) {
			t.Errorf("%s: names %v but resolved MinVersion %q, %d requirements", f.Path, f.Features, f.MinVersion, len(f.Requires))
		}
	}
}

// unreferencedFeatures are registry features no syntax topic names. A LEDGER:
// the test fails when a feature is neither referenced nor listed (add it to the
// topic that documents its statement) and when a listed one gets referenced
// (strike it off), so the list only shrinks.
var unreferencedFeatures = map[string]string{
	// Storage-format facts, not statements: no topic documents their syntax.
	"mpr_format/association_storage": "storage format, no statement",
	"mpr_format/mpr_v2":              "storage format, no statement",
	"mpr_format/portable_app":        "storage format, no statement",
}

func TestEveryRegistryFeatureHasATopic(t *testing.T) {
	reg, err := versions.Load()
	if err != nil {
		t.Fatal(err)
	}
	referenced := map[string]bool{}
	for _, f := range All() {
		for _, k := range f.Features {
			referenced[k] = true
		}
	}
	var missing []string
	for _, k := range reg.Keys() {
		_, listed := unreferencedFeatures[k]
		switch {
		case referenced[k] && listed:
			t.Errorf("%s is referenced by a topic now: strike it off unreferencedFeatures", k)
		case !referenced[k] && !listed:
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	for _, k := range missing {
		t.Errorf("registry feature %s is named by no syntax topic: add it to Features of the topic that documents it", k)
	}
	for k := range unreferencedFeatures {
		area, name, _ := cutFeatureKey(k)
		if _, ok := reg.Lookup(area, name); !ok {
			t.Errorf("unreferencedFeatures lists %s, which the registry no longer has", k)
		}
	}
}
