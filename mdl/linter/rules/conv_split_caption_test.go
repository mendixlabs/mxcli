// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestFindEmptySplitCaptions_EmptyCaption(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ExclusiveSplit{Caption: ""},
	}

	var violations []linter.Violation
	r := NewExclusiveSplitCaptionRule()
	findEmptySplitCaptions(objects, testMicroflow(), r, &violations)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != "CONV012" {
		t.Errorf("expected CONV012, got %s", violations[0].RuleID)
	}
}

func TestFindEmptySplitCaptions_WithCaption(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ExclusiveSplit{Caption: "Is order valid?"},
	}

	var violations []linter.Violation
	r := NewExclusiveSplitCaptionRule()
	findEmptySplitCaptions(objects, testMicroflow(), r, &violations)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(violations))
	}
}

func TestFindEmptySplitCaptions_WhitespaceOnly(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ExclusiveSplit{Caption: "   "},
	}

	var violations []linter.Violation
	r := NewExclusiveSplitCaptionRule()
	findEmptySplitCaptions(objects, testMicroflow(), r, &violations)

	if len(violations) != 1 {
		t.Errorf("expected 1 violation for whitespace caption, got %d", len(violations))
	}
}

func TestFindEmptySplitCaptions_InsideLoop(t *testing.T) {
	loopBody := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.ExclusiveSplit{Caption: ""},
		},
	}
	objects := []microflows.MicroflowObject{
		&microflows.LoopedActivity{
			ObjectCollection: loopBody,
		},
	}

	var violations []linter.Violation
	r := NewExclusiveSplitCaptionRule()
	findEmptySplitCaptions(objects, testMicroflow(), r, &violations)

	if len(violations) != 1 {
		t.Errorf("expected 1 violation inside loop, got %d", len(violations))
	}
}

func TestExclusiveSplitCaptionRule_Metadata(t *testing.T) {
	r := NewExclusiveSplitCaptionRule()
	if r.ID() != "CONV012" {
		t.Errorf("ID = %q, want CONV012", r.ID())
	}
	if r.Category() != "quality" {
		t.Errorf("Category = %q, want quality", r.Category())
	}
}
