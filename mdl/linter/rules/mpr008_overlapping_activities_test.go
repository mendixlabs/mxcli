// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

// NOTE: The full Check() logic requires ctx.Reader().GetMicroflow() to read microflow
// positions from a real MPR file. The overlap detection algorithm (collect positions →
// pairwise distance check) is inline in Check() and cannot be unit-tested without
// building a mock mpr.Reader. This rule currently lacks end-to-end coverage;
// behavioral testing requires a real .mpr project with overlapping activities.

func TestOverlappingActivitiesRule_NilReader(t *testing.T) {
	r := NewOverlappingActivitiesRule()
	ctx := linter.NewLintContextFromDB(nil)
	violations := r.Check(ctx)
	if violations != nil {
		t.Errorf("expected nil with nil reader, got %v", violations)
	}
}

func TestOverlappingActivitiesRule_Metadata(t *testing.T) {
	r := NewOverlappingActivitiesRule()
	if r.ID() != "MPR008" {
		t.Errorf("ID = %q, want MPR008", r.ID())
	}
	if r.Category() != "correctness" {
		t.Errorf("Category = %q, want correctness", r.Category())
	}
	if r.Name() != "OverlappingActivities" {
		t.Errorf("Name = %q, want OverlappingActivities", r.Name())
	}
}

// The Check() method walks microflow positions via ctx.Reader().GetMicroflow() and detects
// overlapping activities using pairwise distance checks against internal heuristic constants
// (activityBoxWidth, activityBoxHeight). Since the collect function is defined inline in
// Check(), behavioral testing requires a real *mpr.Reader with positioned activities.
