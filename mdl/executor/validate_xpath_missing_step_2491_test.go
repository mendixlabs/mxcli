// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2491: `[Car_Assignment.ReturnedOn = empty]` on Repro.Car, where
// Car_Assignment leads to Repro.Assignment and ReturnedOn is its attribute.
// MDL047 suggested `[not(Car_Assignment.ReturnedOn/<Module.TargetEntity>)]`
// and the reference check called the whole name an unknown association; both
// kept the dotted name, and neither said a path needs association/entity/
// attribute. An agent followed them for about 40 minutes.

type fakeXPathModel2491 struct{ assoc map[string]string }

func (f fakeXPathModel2491) IsEntity(qn string) bool {
	return qn == "Repro.Car" || qn == "Repro.Assignment"
}
func (f fakeXPathModel2491) AssociationTarget(qn, from string) (string, bool) {
	t, ok := f.assoc[qn]
	return t, ok
}

func resolve2491(entityQN, member string) memberResolution {
	if entityQN == "Repro.Assignment" && member == "ReturnedOn" {
		return memberFound
	}
	return memberMissing
}

func TestMissingEntityStepHint_2491(t *testing.T) {
	m := fakeXPathModel2491{assoc: map[string]string{"Repro.Car_Assignment": "Repro.Assignment"}}
	for _, name := range []string{"Car_Assignment.ReturnedOn", "Repro.Car_Assignment.ReturnedOn"} {
		hint := missingEntityStepHint(m, resolve2491, "Repro.Car", name)
		for _, want := range []string{"entity step", "[Repro.Car_Assignment/Repro.Assignment/ReturnedOn", "[Repro.Car_Assignment/Repro.Assignment[ReturnedOn"} {
			if !strings.Contains(hint, want) {
				t.Errorf("%s: hint %q lacks %q", name, hint, want)
			}
		}
	}
	// Controls: a real unknown association, an attribute the far end does not
	// have, and a name with no dot say nothing extra.
	for _, name := range []string{"Repro.NoSuchAssoc", "Car_Assignment.Missing", "ReturnedOn"} {
		if hint := missingEntityStepHint(m, resolve2491, "Repro.Car", name); hint != "" {
			t.Errorf("%s: unexpected hint %q", name, hint)
		}
	}
}

// Without a project MDL047 cannot tell `Module.Assoc` from `Assoc.Attr`, so
// its fix names the missing-step reading as well as the absence form.
func TestMDL047FixOffersTheMissingStepReading_2491(t *testing.T) {
	hits := xpathAssociationEmptyMatches("[Car_Assignment.ReturnedOn = empty]")
	if len(hits) != 1 {
		t.Fatalf("hits = %+v", hits)
	}
	fix := hits[0].fix()
	if !strings.Contains(fix, "not(Car_Assignment.ReturnedOn/") || !strings.Contains(fix, "<Module>.Car_Assignment/<Module.TargetEntity>/ReturnedOn = empty") {
		t.Errorf("fix = %q", fix)
	}
}
