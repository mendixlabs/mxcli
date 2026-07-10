// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// NoCommitInLoopRule flags commit actions inside loops, which cause N+1 database operations.
type NoCommitInLoopRule struct{}

func NewNoCommitInLoopRule() *NoCommitInLoopRule { return &NoCommitInLoopRule{} }

func (r *NoCommitInLoopRule) ID() string                       { return "CONV011" }
func (r *NoCommitInLoopRule) Name() string                     { return "NoCommitInLoop" }
func (r *NoCommitInLoopRule) Category() string                 { return "performance" }
func (r *NoCommitInLoopRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *NoCommitInLoopRule) Description() string {
	return "Commit actions should not be inside loops (N+1 performance issue)"
}

func (r *NoCommitInLoopRule) Check(ctx *linter.LintContext) []linter.Violation {
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

		findCommitsInLoops(fullMF.ObjectCollection.Objects, mf, r, &violations, false)
	}

	return violations
}

func findCommitsInLoops(objects []microflows.MicroflowObject, mf linter.Microflow, r *NoCommitInLoopRule, violations *[]linter.Violation, insideLoop bool) {
	for _, obj := range objects {
		switch act := obj.(type) {
		case *microflows.ActionActivity:
			if !insideLoop || act.Action == nil {
				continue
			}
			if _, ok := act.Action.(*microflows.CommitObjectsAction); ok {
				*violations = append(*violations, linter.Violation{
					RuleID:   r.ID(),
					Severity: r.DefaultSeverity(),
					Message: fmt.Sprintf("Microflow '%s.%s' has a Commit action inside a loop. "+
						"This causes N+1 database operations.",
						mf.ModuleName, mf.Name),
					Location: linter.Location{
						Module:       mf.ModuleName,
						DocumentType: "microflow",
						DocumentName: mf.Name,
						DocumentID:   mf.ID,
					},
					Suggestion: "Move the commit outside the loop, or collect objects in a list and commit once after the loop",
				})
			}
		case *microflows.LoopedActivity:
			if act.ObjectCollection != nil {
				findCommitsInLoops(act.ObjectCollection.Objects, mf, r, violations, true)
			}
		}
	}
}
