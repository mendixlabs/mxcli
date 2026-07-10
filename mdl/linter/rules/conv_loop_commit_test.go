// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestFindCommitsInLoops_NoLoop(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{},
			Action:       &microflows.CommitObjectsAction{},
		},
	}

	var violations []linter.Violation
	r := NewNoCommitInLoopRule()
	findCommitsInLoops(objects, testMicroflow(), r, &violations, false)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations outside loop, got %d", len(violations))
	}
}

func TestFindCommitsInLoops_CommitInsideLoop(t *testing.T) {
	loopBody := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.ActionActivity{
				BaseActivity: microflows.BaseActivity{},
				Action:       &microflows.CommitObjectsAction{},
			},
		},
	}
	objects := []microflows.MicroflowObject{
		&microflows.LoopedActivity{
			ObjectCollection: loopBody,
		},
	}

	var violations []linter.Violation
	r := NewNoCommitInLoopRule()
	findCommitsInLoops(objects, testMicroflow(), r, &violations, false)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != "CONV011" {
		t.Errorf("expected CONV011, got %s", violations[0].RuleID)
	}
}

func TestFindCommitsInLoops_NilAction(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{},
			Action:       nil,
		},
	}

	var violations []linter.Violation
	r := NewNoCommitInLoopRule()
	findCommitsInLoops(objects, testMicroflow(), r, &violations, true)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for nil action, got %d", len(violations))
	}
}

func TestNoCommitInLoopRule_NilReader(t *testing.T) {
	r := NewNoCommitInLoopRule()
	ctx := linter.NewLintContextFromDB(nil)
	// Reader() is nil, should return nil
	violations := r.Check(ctx)
	if violations != nil {
		t.Errorf("expected nil with nil reader, got %v", violations)
	}
}

func TestNoCommitInLoopRule_Metadata(t *testing.T) {
	r := NewNoCommitInLoopRule()
	if r.ID() != "CONV011" {
		t.Errorf("ID = %q, want CONV011", r.ID())
	}
	if r.Category() != "performance" {
		t.Errorf("Category = %q, want performance", r.Category())
	}
}
