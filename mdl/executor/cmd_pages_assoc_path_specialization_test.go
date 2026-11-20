// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestApp WorkflowCommons.Snip_WorkflowUserTaskView_Details, text11: Studio Pro
// stores an association step whose DestinationEntity is a SPECIALIZATION of the
// association's target —
//
//	Attribute "Administration.Account.FullName"
//	Steps [ { Association "System.WorkflowUserTaskOutcome_User",
//	          DestinationEntity "Administration.Account" } ]
//
// describe printed `System.WorkflowUserTaskOutcome_User/FullName`, dropping the
// cast, and exec wrote System.User.FullName: mx check CE1613 "The selected
// attribute 'System.User.FullName' no longer exists." at Text 'text11'.
// `Assoc/Module.Entity/Attr` — Mendix's expression spelling of the cast — keeps it.

// specializationPB extends ruleActionPB: SpecialRule specializes BusinessRule
// and declares Extra, which BusinessRule does not have.
func specializationPB(t *testing.T) *pageBuilder {
	t.Helper()
	pb := ruleActionPB(t)
	dm := pb.execCache.domainModels[0]
	dm.Entities = append(dm.Entities, &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: "e-special"}, Name: "SpecialRule",
		GeneralizationRef: "Rules.BusinessRule",
		Attributes:        []*domainmodel.Attribute{{Name: "Extra"}},
	})
	return pb
}

func TestAssociationPathSpecializationSegment(t *testing.T) {
	pb := specializationPB(t)
	qn, steps := pb.resolveInputAttribute("RuleAction_BusinessRule/Rules.SpecialRule/Extra")
	if qn != "Rules.SpecialRule.Extra" || len(steps) != 1 ||
		steps[0].Association != "Rules.RuleAction_BusinessRule" || steps[0].DestinationEntity != "Rules.SpecialRule" {
		t.Errorf("resolved %q %+v, want Rules.SpecialRule.Extra over Rules.RuleAction_BusinessRule to Rules.SpecialRule", qn, steps)
	}
	// Control: without the cast the step lands on the association's target.
	qn, steps = pb.resolveInputAttribute("RuleAction_BusinessRule/Name")
	if qn != "Rules.BusinessRule.Name" || len(steps) != 1 || steps[0].DestinationEntity != "Rules.BusinessRule" {
		t.Errorf("control resolved %q %+v", qn, steps)
	}
	// An entity that does not specialize the step's target is not a cast.
	if _, steps = pb.resolveInputAttribute("RuleAction_BusinessRule/Rules.RuleAction/ActionType"); len(steps) != 0 {
		t.Errorf("a non-specialization was taken as a cast: %+v", steps)
	}
}

func TestDescribeAssociationPathKeepsSpecialization(t *testing.T) {
	pb := specializationPB(t)
	ref := func(attr, dest string) map[string]any {
		return map[string]any{
			"Attribute": attr,
			"EntityRef": map[string]any{"$Type": "DomainModels$IndirectEntityRef", "Steps": []any{int32(2),
				map[string]any{"Association": "Rules.RuleAction_BusinessRule", "DestinationEntity": dest}}},
		}
	}
	isEnd := pb.associationHasEnd
	if got := associationPathMDL(ref("Rules.SpecialRule.Extra", "Rules.SpecialRule"), false, isEnd); got != "Rules.RuleAction_BusinessRule/Rules.SpecialRule/Extra" {
		t.Errorf("specialization described %q", got)
	}
	// Control: the association's own target is not printed.
	if got := associationPathMDL(ref("Rules.BusinessRule.Name", "Rules.BusinessRule"), false, isEnd); got != "Rules.RuleAction_BusinessRule/Name" {
		t.Errorf("control described %q", got)
	}
	// No lookup (no project): as before.
	if got := associationPathMDL(ref("Rules.SpecialRule.Extra", "Rules.SpecialRule"), true, nil); got != "RuleAction_BusinessRule/Extra" {
		t.Errorf("without a lookup described %q", got)
	}
}
