// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// ValidationFeedbackRule checks for validation feedback actions with empty message templates.
type ValidationFeedbackRule struct{}

// NewValidationFeedbackRule creates a new validation feedback rule.
func NewValidationFeedbackRule() *ValidationFeedbackRule {
	return &ValidationFeedbackRule{}
}

func (r *ValidationFeedbackRule) ID() string                       { return "MPR004" }
func (r *ValidationFeedbackRule) Name() string                     { return "EmptyValidationFeedback" }
func (r *ValidationFeedbackRule) Category() string                 { return "correctness" }
func (r *ValidationFeedbackRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *ValidationFeedbackRule) Description() string {
	return "Checks for validation feedback actions with empty message templates (CE0091)"
}

// Check loads each microflow and inspects validation feedback actions for empty templates.
func (r *ValidationFeedbackRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}

	var violations []linter.Violation

	for mf := range ctx.Microflows() {
		if ctx.IsExcluded(mf.ModuleName) {
			continue
		}

		fullMF, err := ctx.FullMicroflow(model.ID(mf.ID))
		if err != nil || fullMF == nil || fullMF.ObjectCollection == nil {
			continue
		}

		walkObjects(fullMF.ObjectCollection.Objects, mf, r, &violations)
	}

	return violations
}

// walkObjects recursively walks microflow objects looking for empty validation feedback.
func walkObjects(objects []microflows.MicroflowObject, mf linter.Microflow, r *ValidationFeedbackRule, violations *[]linter.Violation) {
	for _, obj := range objects {
		switch act := obj.(type) {
		case *microflows.ActionActivity:
			if act.Action == nil {
				continue
			}
			if vf, ok := act.Action.(*microflows.ValidationFeedbackAction); ok {
				if isEmptyTemplate(vf) {
					*violations = append(*violations, linter.Violation{
						RuleID:   r.ID(),
						Severity: r.DefaultSeverity(),
						Message: fmt.Sprintf("Validation feedback in '%s.%s' has empty message template. "+
							"Mendix requires a non-empty feedback message (CE0091).",
							mf.ModuleName, mf.Name),
						Location: linter.Location{
							Module:       mf.ModuleName,
							DocumentType: "microflow",
							DocumentName: mf.Name,
							DocumentID:   mf.ID,
						},
						Suggestion: "Add a message template to the validation feedback action",
					})
				}
			}
		case *microflows.LoopedActivity:
			if act.ObjectCollection != nil {
				walkObjects(act.ObjectCollection.Objects, mf, r, violations)
			}
		}
	}
}

// isEmptyTemplate checks if a ValidationFeedbackAction has an empty or nil template.
func isEmptyTemplate(vf *microflows.ValidationFeedbackAction) bool {
	if vf.Template == nil {
		return true
	}
	if len(vf.Template.Translations) == 0 {
		return true
	}
	// Check if all translations are empty strings
	for _, text := range vf.Template.Translations {
		if text != "" {
			return false
		}
	}
	return true
}
