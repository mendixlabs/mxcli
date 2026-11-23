// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// DomainModelNotLaidOutRule flags a module whose linked entities still sit on
// the grid slots mxcli's auto-placement gives an entity created without
// @Position: a script made the domain model and `mxcli layout` was never run.
//
// The grid is a default, not a layout. It knows nothing about which entities
// are related, so association lines cross the diagram, and the generate-
// domain-model skill already says to run `mxcli layout` afterwards. Agents
// forget, and nothing told them: MPR016 only fires when boxes overlap, which
// Place now avoids, and it is a warning, which `lint` and `verify` pass.
//
// # Why this can be an error
//
// An error fails the gate, so a false finding would block work. The signature
// (dmlayout.NotLaidOut) is an exact grid slot other than the origin, on an
// entity linked to another one in the module, that Plan would not put there.
// Studio Pro positions are wherever someone dropped a box and practically never
// land on those exact coordinates, so a hand-arranged diagram is not reported;
// a diagram `mxcli layout` produced is excluded by construction. That leaves
// script-placed entities, and the repair is one command.
//
// The repair rewrites every position in the module. That is right for a module
// a script built, and the finding says so, so someone who arranged the rest of
// the module by hand can position the reported entities themselves instead.
type DomainModelNotLaidOutRule struct{}

func NewDomainModelNotLaidOutRule() *DomainModelNotLaidOutRule {
	return &DomainModelNotLaidOutRule{}
}

func (r *DomainModelNotLaidOutRule) ID() string                       { return "MPR020" }
func (r *DomainModelNotLaidOutRule) Name() string                     { return "DomainModelNotLaidOut" }
func (r *DomainModelNotLaidOutRule) Category() string                 { return "design" }
func (r *DomainModelNotLaidOutRule) DefaultSeverity() linter.Severity { return linter.SeverityError }
func (r *DomainModelNotLaidOutRule) Description() string {
	return "Linked entities still on mxcli's default placement grid: the domain model was created by a script and never laid out with 'mxcli layout'"
}

func (r *DomainModelNotLaidOutRule) Check(ctx *linter.LintContext) []linter.Violation {
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
		unlaid := dmlayout.NotLaidOut(dm)
		if len(unlaid) == 0 || ctx.IsDocumentExcluded(module+"."+unlaid[0].Name) {
			continue
		}
		names := make([]string, 0, len(unlaid))
		for _, e := range unlaid {
			names = append(names, e.Name)
		}
		violations = append(violations, linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Message:  notLaidOutMessage(module, names),
			Location: linter.Location{
				Module:       module,
				DocumentType: "entity",
				DocumentName: unlaid[0].Name,
				DocumentID:   string(unlaid[0].ID),
			},
			Suggestion: fmt.Sprintf("Run 'mxcli layout --module %s' (add --dry-run to see the moves first). "+
				"It re-arranges every entity in the module; if the rest was arranged by hand, give these entities explicit positions instead.", module),
		})
	}
	return violations
}

// notLaidOutMessage words one finding, naming at most maxNamedInCluster
// entities for the same reason MPR016 does.
func notLaidOutMessage(module string, names []string) string {
	shown, suffix := names, ""
	if len(shown) > maxNamedInCluster {
		shown = shown[:maxNamedInCluster]
		suffix = fmt.Sprintf(" and %d more", len(names)-maxNamedInCluster)
	}
	noun := "1 entity is"
	if len(names) > 1 {
		noun = fmt.Sprintf("%d entities are", len(names))
	}
	return fmt.Sprintf("The %s domain model was never laid out: %s still on mxcli's default placement grid (%s%s), so its association lines run across the diagram.",
		module, noun, strings.Join(shown, ", "), suffix)
}
