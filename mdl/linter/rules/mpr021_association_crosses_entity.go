// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

// AssociationCrossesEntityRule flags a line on a domain model diagram that
// runs through an entity box which is not one of its ends: an association
// (from its stored connection points), a generalization arrow, or a
// cross-module association, which Studio Pro always draws from the entity's
// left side straight to the diagram's left edge (mendixlabs/mxcli#1929, rules 1
// and 2). A line through a box hides members behind it and reads as if it
// connected that entity.
//
// The geometry is dmlayout's, so `mxcli layout`, which avoids exactly these,
// and this rule agree on what a line is. Box sizes are estimates (measured, see
// dmlayout.boxSize), so a line is reported only if it passes through a box
// shrunk by grazeInset on every side: one that merely grazes an estimated edge
// may clear the real one.
type AssociationCrossesEntityRule struct{}

// grazeInset is how far inside an estimated box a line must pass to be reported.
const grazeInset = 8

func NewAssociationCrossesEntityRule() *AssociationCrossesEntityRule {
	return &AssociationCrossesEntityRule{}
}

func (r *AssociationCrossesEntityRule) ID() string       { return "MPR021" }
func (r *AssociationCrossesEntityRule) Name() string     { return "AssociationCrossesEntity" }
func (r *AssociationCrossesEntityRule) Category() string { return "design" }
func (r *AssociationCrossesEntityRule) DefaultSeverity() linter.Severity {
	return linter.SeverityWarning
}
func (r *AssociationCrossesEntityRule) Description() string {
	return "An association, generalization or cross-module line on the domain model diagram runs through an entity it does not connect"
}

func (r *AssociationCrossesEntityRule) Check(ctx *linter.LintContext) []linter.Violation {
	dms := ctx.DomainModels()
	if len(dms) == 0 {
		return nil
	}
	moduleNames := ctx.OwnedModuleNamesByID()

	var violations []linter.Violation
	for _, dm := range dms {
		if dm == nil {
			continue
		}
		module := moduleNames[dm.ContainerID]
		if module == "" || ctx.IsExcluded(module) {
			continue
		}
		pos := map[model.ID]model.Point{}
		names := map[model.ID]string{}
		for _, e := range dm.Entities {
			if e != nil {
				pos[e.ID] = e.Location
				names[e.ID] = e.Name
			}
		}
		cross := dmlayout.CrossLinesFor(module, dm, dms)
		for _, l := range dmlayout.Lines(dm, pos, dmlayout.StoredAnchors(dm), cross) {
			through := l.ThroughInset(dm, pos, grazeInset)
			if len(through) == 0 {
				continue
			}
			var hit []string
			for _, e := range through {
				hit = append(hit, e.Name)
			}
			doc := l.Name
			if doc == "" {
				doc = names[l.Ends[0]]
			}
			if ctx.IsDocumentExcluded(module + "." + doc) {
				continue
			}
			violations = append(violations, linter.Violation{
				RuleID:   r.ID(),
				Severity: r.DefaultSeverity(),
				Message:  crossesMessage(module, l, names, hit),
				Location: linter.Location{Module: module, DocumentType: "association", DocumentName: doc},
				Suggestion: fmt.Sprintf("Run 'mxcli layout --module %s': it keeps cross-module lines clear and orders each column for the fewest crossings. "+
					"A line between entities several columns apart can still need an entity moved by hand.", module),
			})
		}
	}
	return violations
}

func crossesMessage(module string, l dmlayout.Line, names map[model.ID]string, hit []string) string {
	list := strings.Join(hit, ", ")
	switch l.Kind {
	case "cross-module":
		return fmt.Sprintf("The cross-module association line '%s', drawn from %s to the left edge of the %s domain model diagram, runs through %s.",
			l.Name, names[l.Ends[0]], module, list)
	case "generalization":
		return fmt.Sprintf("The generalization arrow from %s to %s on the %s domain model diagram runs through %s.",
			names[l.Ends[0]], names[l.Ends[1]], module, list)
	}
	return fmt.Sprintf("The association line '%s' on the %s domain model diagram runs through %s, which it does not connect.", l.Name, module, list)
}
