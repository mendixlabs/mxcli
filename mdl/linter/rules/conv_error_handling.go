// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// --- CONV013: ErrorHandlingOnCalls ---

// ErrorHandlingOnCallsRule flags external call actions (REST, web service, Java) without custom error handling.
type ErrorHandlingOnCallsRule struct{}

func NewErrorHandlingOnCallsRule() *ErrorHandlingOnCallsRule {
	return &ErrorHandlingOnCallsRule{}
}

func (r *ErrorHandlingOnCallsRule) ID() string                       { return "CONV013" }
func (r *ErrorHandlingOnCallsRule) Name() string                     { return "ErrorHandlingOnCalls" }
func (r *ErrorHandlingOnCallsRule) Category() string                 { return "quality" }
func (r *ErrorHandlingOnCallsRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *ErrorHandlingOnCallsRule) Description() string {
	return "External service calls (REST, web service, Java action) should have custom error handling"
}

func (r *ErrorHandlingOnCallsRule) Check(ctx *linter.LintContext) []linter.Violation {
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

		findUnhandledCalls(fullMF.ObjectCollection.Objects, mf, r, &violations)
	}

	return violations
}

func findUnhandledCalls(objects []microflows.MicroflowObject, mf linter.Microflow, r *ErrorHandlingOnCallsRule, violations *[]linter.Violation) {
	for _, obj := range objects {
		switch act := obj.(type) {
		case *microflows.ActionActivity:
			if act.Action == nil {
				continue
			}
			actionName := ""
			switch act.Action.(type) {
			case *microflows.RestCallAction:
				actionName = "REST call"
			case *microflows.WebServiceCallAction:
				actionName = "Web service call"
			case *microflows.JavaActionCallAction:
				actionName = "Java action call"
			default:
				continue
			}

			// Check if error handling is not custom
			if act.ErrorHandlingType != microflows.ErrorHandlingTypeCustom &&
				act.ErrorHandlingType != microflows.ErrorHandlingTypeCustomWithoutRollback {
				*violations = append(*violations, linter.Violation{
					RuleID:   r.ID(),
					Severity: r.DefaultSeverity(),
					Message: fmt.Sprintf("%s in '%s.%s' uses '%s' error handling instead of Custom.",
						actionName, mf.ModuleName, mf.Name, act.ErrorHandlingType),
					Location: linter.Location{
						Module:       mf.ModuleName,
						DocumentType: "microflow",
						DocumentName: mf.Name,
						DocumentID:   mf.ID,
					},
					Suggestion: "Set error handling to 'Custom with rollback' and add an error handler flow",
				})
			}
		case *microflows.LoopedActivity:
			if act.ObjectCollection != nil {
				findUnhandledCalls(act.ObjectCollection.Objects, mf, r, violations)
			}
		}
	}
}

// --- CONV014: NoContinueErrorHandling ---

// NoContinueErrorHandlingRule flags activities with "Continue" error handling, which silently swallows errors.
type NoContinueErrorHandlingRule struct{}

func NewNoContinueErrorHandlingRule() *NoContinueErrorHandlingRule {
	return &NoContinueErrorHandlingRule{}
}

func (r *NoContinueErrorHandlingRule) ID() string       { return "CONV014" }
func (r *NoContinueErrorHandlingRule) Name() string     { return "NoContinueErrorHandling" }
func (r *NoContinueErrorHandlingRule) Category() string { return "quality" }
func (r *NoContinueErrorHandlingRule) DefaultSeverity() linter.Severity {
	return linter.SeverityWarning
}

func (r *NoContinueErrorHandlingRule) Description() string {
	return "Activities should not use 'Continue' error handling which silently swallows errors"
}

func (r *NoContinueErrorHandlingRule) Check(ctx *linter.LintContext) []linter.Violation {
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

		findContinueErrorHandling(fullMF.ObjectCollection.Objects, mf, r, &violations)
	}

	return violations
}

func findContinueErrorHandling(objects []microflows.MicroflowObject, mf linter.Microflow, r *NoContinueErrorHandlingRule, violations *[]linter.Violation) {
	for _, obj := range objects {
		switch act := obj.(type) {
		case *microflows.ActionActivity:
			if act.ErrorHandlingType == microflows.ErrorHandlingTypeContinue {
				caption := act.Caption
				if caption == "" {
					caption = "(unnamed activity)"
				}
				*violations = append(*violations, linter.Violation{
					RuleID:   r.ID(),
					Severity: r.DefaultSeverity(),
					Message: fmt.Sprintf("Activity '%s' in '%s.%s' uses 'Continue' error handling, which silently swallows errors.",
						caption, mf.ModuleName, mf.Name),
					Location: linter.Location{
						Module:       mf.ModuleName,
						DocumentType: "microflow",
						DocumentName: mf.Name,
						DocumentID:   mf.ID,
					},
					Suggestion: "Change error handling to 'Custom with rollback' or 'Abort' to properly handle errors",
				})
			}
		case *microflows.LoopedActivity:
			if act.ErrorHandlingType == microflows.ErrorHandlingTypeContinue {
				caption := act.Caption
				if caption == "" {
					caption = "(unnamed loop)"
				}
				*violations = append(*violations, linter.Violation{
					RuleID:   r.ID(),
					Severity: r.DefaultSeverity(),
					Message: fmt.Sprintf("Loop '%s' in '%s.%s' uses 'Continue' error handling, which silently swallows errors.",
						caption, mf.ModuleName, mf.Name),
					Location: linter.Location{
						Module:       mf.ModuleName,
						DocumentType: "microflow",
						DocumentName: mf.Name,
						DocumentID:   mf.ID,
					},
					Suggestion: "Change error handling to 'Custom with rollback' or 'Abort' to properly handle errors",
				})
			}
			if act.ObjectCollection != nil {
				findContinueErrorHandling(act.ObjectCollection.Objects, mf, r, violations)
			}
		}
	}
}
