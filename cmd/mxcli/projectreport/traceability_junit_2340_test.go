// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

// ako/mxcli#2340 (mendixlabs/mxcli#1877): `report traceability --junit` read
// the results and dropped them — each requirement's tests stayed a list of
// names, so the JSON with and without --junit was byte-identical, and "built,
// test failing" read like "built and proven". A requirement's tests now carry
// their result.
func TestTraceability_TestsCarryTheirResult_Issue2340(t *testing.T) {
	f := newFixture(t)
	f.module("Game", "")
	f.microflow("Game.ACT_RemoveScore", true, "")
	f.microflow("Game.ACT_AddScore", true, "")
	in := TraceabilityInput{
		Requirements: []brain.RequirementResult{
			req("69d67f", "01-scores", "Remove a high score", true, anchor("@Game.ACT_RemoveScore", "microflow", brain.Resolved)),
			req("70a1b2", "01-scores", "Add a high score", true, anchor("@Game.ACT_AddScore", "microflow", brain.Resolved)),
		},
		Evidence: &TestsData{Tests: []TestInfo{
			{Name: "A removed high score is gone", Status: "failed", Message: "expected 0, got 1", Reached: []string{"Game.ACT_RemoveScore"}},
			{Name: "A score is added", Status: "passed", Reached: []string{"Game.ACT_AddScore"}},
		}},
	}
	d, err := CollectTraceability(f.cat, in)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]TraceRequirement{}
	for _, m := range d.Milestones {
		for _, r := range m.Requirements {
			got[r.ID] = r
		}
	}
	if want := []TraceTestResult{{Name: "A removed high score is gone", Status: "failed"}}; !reflect.DeepEqual(got["69d67f"].TestResults, want) {
		t.Errorf("69d67f test results = %+v, want %+v", got["69d67f"].TestResults, want)
	}
	if want := []TraceTestResult{{Name: "A score is added", Status: "passed"}}; !reflect.DeepEqual(got["70a1b2"].TestResults, want) {
		t.Errorf("70a1b2 test results = %+v, want %+v", got["70a1b2"].TestResults, want)
	}
	// The names stay where they were, for readers of the old field.
	if !reflect.DeepEqual(got["69d67f"].Tests, []string{"A removed high score is gone"}) {
		t.Errorf("tests = %v", got["69d67f"].Tests)
	}
	// The rendered table says which test fails.
	env, err := NewEnvelope("traceability", "Game", "2026-11-17", "test", d, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := RenderTraceability(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	var cell string
	for _, s := range doc.Sections {
		for _, b := range s.Blocks {
			tb, ok := b.(Table)
			if !ok {
				continue
			}
			for _, row := range tb.Rows {
				for _, c := range row {
					if strings.Contains(c, "A removed high score is gone") {
						cell = c
					}
				}
			}
		}
	}
	if !strings.Contains(cell, "failed") {
		t.Errorf("rendered tests cell = %q, want the failing result shown", cell)
	}
}

// CONTROL: without results (no --junit) the tests are "not run", and the
// table shows the names alone.
func TestTraceability_TestsWithoutResults_Issue2340(t *testing.T) {
	f := newFixture(t)
	f.module("Game", "")
	f.microflow("Game.ACT_RemoveScore", true, "")
	in := TraceabilityInput{
		Requirements: []brain.RequirementResult{
			req("69d67f", "01-scores", "Remove a high score", true, anchor("@Game.ACT_RemoveScore", "microflow", brain.Resolved)),
		},
		Evidence: &TestsData{Tests: []TestInfo{{Name: "A removed high score is gone", Status: "not run", Reached: []string{"Game.ACT_RemoveScore"}}}},
	}
	d, err := CollectTraceability(f.cat, in)
	if err != nil {
		t.Fatal(err)
	}
	r := d.Milestones[0].Requirements[0]
	if len(r.TestResults) != 1 || r.TestResults[0].Status != "not run" {
		t.Errorf("test results = %+v", r.TestResults)
	}
}
