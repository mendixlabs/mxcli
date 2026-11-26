// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// uniqueName must not hand out a suffixed name another activity already holds.
// `create or modify` carries the stored names of the ends it cannot state
// (End2 on one boundary path), then deduplicates; an uncarried End was then
// numbered End2 as well, because the suffix was the count and was never checked
// against the names seen — CE0495 "Duplicate name 'End2'", on a workflow the
// same script had just created.
func TestUniqueName_SkipsANameAlreadyTaken(t *testing.T) {
	carried := &workflows.EndWorkflowActivity{}
	carried.Name = "End2"
	uncarried := &workflows.EndWorkflowActivity{}
	uncarried.Name = "End"
	deduplicateActivityNames([]workflows.WorkflowActivity{carried, uncarried}, "Start", "End")
	if carried.Name != "End2" || uncarried.Name == "End2" || uncarried.Name == "End" {
		t.Errorf("names %q and %q; want End2 kept and the other given a free name", carried.Name, uncarried.Name)
	}
	if uncarried.Name != "End3" {
		t.Errorf("uncarried End = %q, want End3, the next free suffix", uncarried.Name)
	}
}
