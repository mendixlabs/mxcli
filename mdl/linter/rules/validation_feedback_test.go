// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestIsEmptyTemplate_Nil(t *testing.T) {
	vf := &microflows.ValidationFeedbackAction{Template: nil}
	if !isEmptyTemplate(vf) {
		t.Error("expected true for nil template")
	}
}

func TestIsEmptyTemplate_EmptyTranslations(t *testing.T) {
	vf := &microflows.ValidationFeedbackAction{
		Template: &model.Text{Translations: map[string]string{}},
	}
	if !isEmptyTemplate(vf) {
		t.Error("expected true for empty translations")
	}
}

func TestIsEmptyTemplate_AllEmpty(t *testing.T) {
	vf := &microflows.ValidationFeedbackAction{
		Template: &model.Text{Translations: map[string]string{"en_US": "", "nl_NL": ""}},
	}
	if !isEmptyTemplate(vf) {
		t.Error("expected true when all translations empty")
	}
}

func TestIsEmptyTemplate_HasContent(t *testing.T) {
	vf := &microflows.ValidationFeedbackAction{
		Template: &model.Text{Translations: map[string]string{"en_US": "Please fill in this field"}},
	}
	if isEmptyTemplate(vf) {
		t.Error("expected false when translation has content")
	}
}

func TestWalkObjects_EmptyValidation(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{},
			Action: &microflows.ValidationFeedbackAction{
				Template: nil,
			},
		},
	}

	var violations []linter.Violation
	r := NewValidationFeedbackRule()
	walkObjects(objects, testMicroflow(), r, &violations)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != "MPR004" {
		t.Errorf("expected MPR004, got %s", violations[0].RuleID)
	}
}

func TestWalkObjects_ValidFeedback(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{},
			Action: &microflows.ValidationFeedbackAction{
				Template: &model.Text{Translations: map[string]string{"en_US": "Required"}},
			},
		},
	}

	var violations []linter.Violation
	r := NewValidationFeedbackRule()
	walkObjects(objects, testMicroflow(), r, &violations)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(violations))
	}
}

func TestWalkObjects_InsideLoop(t *testing.T) {
	loopBody := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.ActionActivity{
				BaseActivity: microflows.BaseActivity{},
				Action: &microflows.ValidationFeedbackAction{
					Template: nil,
				},
			},
		},
	}
	objects := []microflows.MicroflowObject{
		&microflows.LoopedActivity{
			ObjectCollection: loopBody,
		},
	}

	var violations []linter.Violation
	r := NewValidationFeedbackRule()
	walkObjects(objects, testMicroflow(), r, &violations)

	if len(violations) != 1 {
		t.Errorf("expected 1 violation inside loop, got %d", len(violations))
	}
}

func TestValidationFeedbackRule_Metadata(t *testing.T) {
	r := NewValidationFeedbackRule()
	if r.ID() != "MPR004" {
		t.Errorf("ID = %q, want MPR004", r.ID())
	}
	if r.Category() != "correctness" {
		t.Errorf("Category = %q, want correctness", r.Category())
	}
}
