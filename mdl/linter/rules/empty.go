// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

// EmptyMicroflowRule checks for microflows with no activities.
type EmptyMicroflowRule struct{}

// NewEmptyMicroflowRule creates a new empty microflow rule.
func NewEmptyMicroflowRule() *EmptyMicroflowRule {
	return &EmptyMicroflowRule{}
}

func (r *EmptyMicroflowRule) ID() string                       { return "MPR002" }
func (r *EmptyMicroflowRule) Name() string                     { return "EmptyMicroflow" }
func (r *EmptyMicroflowRule) Category() string                 { return "quality" }
func (r *EmptyMicroflowRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *EmptyMicroflowRule) Description() string {
	return "Checks for microflows that have no activities"
}

// Check runs the empty microflow check.
func (r *EmptyMicroflowRule) Check(ctx *linter.LintContext) []linter.Violation {
	var violations []linter.Violation

	for mf := range ctx.Microflows() {
		if mf.ActivityCount == 0 {
			violations = append(violations, linter.Violation{
				RuleID:   r.ID(),
				Severity: r.DefaultSeverity(),
				Message:  fmt.Sprintf("Microflow '%s' has no activities", mf.Name),
				Location: linter.Location{
					Module:       mf.ModuleName,
					DocumentType: "microflow",
					DocumentName: mf.Name,
					DocumentID:   mf.ID,
				},
				Suggestion: "Add activities or remove unused microflow",
			})
		}
	}

	return violations
}
