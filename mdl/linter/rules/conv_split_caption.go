// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// ExclusiveSplitCaptionRule flags exclusive splits without meaningful captions.
type ExclusiveSplitCaptionRule struct{}

func NewExclusiveSplitCaptionRule() *ExclusiveSplitCaptionRule {
	return &ExclusiveSplitCaptionRule{}
}

func (r *ExclusiveSplitCaptionRule) ID() string                       { return "CONV012" }
func (r *ExclusiveSplitCaptionRule) Name() string                     { return "ExclusiveSplitCaption" }
func (r *ExclusiveSplitCaptionRule) Category() string                 { return "quality" }
func (r *ExclusiveSplitCaptionRule) DefaultSeverity() linter.Severity { return linter.SeverityInfo }

func (r *ExclusiveSplitCaptionRule) Description() string {
	return "Exclusive splits should have a meaningful caption describing the decision"
}

func (r *ExclusiveSplitCaptionRule) Check(ctx *linter.LintContext) []linter.Violation {
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

		findEmptySplitCaptions(fullMF.ObjectCollection.Objects, mf, r, &violations)
	}

	return violations
}

func findEmptySplitCaptions(objects []microflows.MicroflowObject, mf linter.Microflow, r *ExclusiveSplitCaptionRule, violations *[]linter.Violation) {
	for _, obj := range objects {
		switch act := obj.(type) {
		case *microflows.ExclusiveSplit:
			caption := strings.TrimSpace(act.Caption)
			if caption == "" {
				*violations = append(*violations, linter.Violation{
					RuleID:   r.ID(),
					Severity: r.DefaultSeverity(),
					Message: fmt.Sprintf("Exclusive split in '%s.%s' has no caption. "+
						"Add a question or description to clarify the decision.",
						mf.ModuleName, mf.Name),
					Location: linter.Location{
						Module:       mf.ModuleName,
						DocumentType: "microflow",
						DocumentName: mf.Name,
						DocumentID:   mf.ID,
					},
					Suggestion: "Set a caption that describes the decision, e.g., 'Is the order valid?'",
				})
			}
		case *microflows.LoopedActivity:
			if act.ObjectCollection != nil {
				findEmptySplitCaptions(act.ObjectCollection.Objects, mf, r, violations)
			}
		}
	}
}
