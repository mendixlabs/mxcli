// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2311: `create or modify` inserted statements and drew the two
// connectors at their ends with the splice's own control vectors (±15),
// where a create of the same script draws them as its @curve states — and
// straight ("0;0") where it states none. The next run of the same script read
// the difference as a connector the script redraws and refused it
// (MDL-V1-REBUILD): a script that had just succeeded could not run again.
// The fragment now carries the curves the script states for both connectors.

func vectors(t *testing.T, m *Mutator, origin, dest string) (string, string) {
	t.Helper()
	for _, f := range m.graph().flows {
		if f.origin == origin && f.dest == dest {
			_, o := flowEnd(f.doc, "Origin")
			_, d := flowEnd(f.doc, "Destination")
			return o, d
		}
	}
	t.Fatalf("no flow %s -> %s", origin, dest)
	return "", ""
}

func TestSplice_InsertDrawsTheStatedCurves_Issue2311(t *testing.T) {
	m, _ := newMutator(t, unit(line(), lineFlows()))
	frag := oneActivity()
	frag.InCurve = &backend.FlowVectors{Origin: "30;0", Destination: "-30;0"}
	frag.OutCurve = &backend.FlowVectors{Origin: "0;0", Destination: "0;0"}
	if err := m.InsertAfter(model.ID(uid("a")), frag); err != nil {
		t.Fatal(err)
	}
	if o, d := vectors(t, m, uid("a"), string(frag.Entry)); o != "30;0" || d != "-30;0" {
		t.Errorf("connector into the fragment = %s / %s, want the stated 30;0 / -30;0", o, d)
	}
	if o, d := vectors(t, m, string(frag.Exit), uid("b")); o != "0;0" || d != "0;0" {
		t.Errorf("connector out of the fragment = %s / %s, want the stated straight line", o, d)
	}
}

// CONTROL: an `alter` insert states no curves, and keeps the splice's own.
func TestSplice_InsertWithoutStatedCurvesKeepsItsOwn_Issue2311(t *testing.T) {
	m, _ := newMutator(t, unit(line(), lineFlows()))
	frag := oneActivity()
	if err := m.InsertAfter(model.ID(uid("a")), frag); err != nil {
		t.Fatal(err)
	}
	if _, d := vectors(t, m, uid("a"), string(frag.Entry)); d != sideVector(sideLeft) {
		t.Errorf("unstated connector into the fragment ends with %s, want the splice's %s", d, sideVector(sideLeft))
	}
}
