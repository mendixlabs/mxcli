// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// ako/mxcli#1810: the System module is version-dependent. On Mendix 9 the user
// task's assignee association is WorkflowUserTask_Assignee (a Reference); from
// 10.0 it is WorkflowUserTask_Assignees (a ReferenceSet) — one association,
// renamed with its GUID kept, read from the 9.24.30 / 10.0.0 / 11.12.6 modeler
// assemblies. The virtual System model check --references resolves against was
// the 11.x one for every project, so on 9.24 it accepted Assignees (CE1613 at
// build) and refused Assignee, the name 9.24 has.
func TestSystemModelAssigneeFollowsProjectVersion(t *testing.T) {
	atLeast := func(maj, min int) func(int, int) bool {
		return func(a, b int) bool { return maj > a || (maj == a && min >= b) }
	}
	cases := []struct {
		name      string
		atLeast   func(int, int) bool
		want, not string
		wantType  string
	}{
		{"9.24", atLeast(9, 24), "WorkflowUserTask_Assignee", "WorkflowUserTask_Assignees", "Reference"},
		{"10.0", atLeast(10, 0), "WorkflowUserTask_Assignees", "WorkflowUserTask_Assignee", "ReferenceSet"},
		{"11.12", atLeast(11, 12), "WorkflowUserTask_Assignees", "WorkflowUserTask_Assignee", "ReferenceSet"},
		{"unknown", nil, "WorkflowUserTask_Assignees", "WorkflowUserTask_Assignee", "ReferenceSet"},
	}
	for _, c := range cases {
		dm := buildSystemDomainModel(c.atLeast)
		names := map[string]string{}
		for _, a := range dm.Associations {
			names[a.Name] = string(a.Type)
		}
		if typ, ok := names[c.want]; !ok || typ != c.wantType {
			t.Errorf("%s: want %s (%s), got %q present=%v", c.name, c.want, c.wantType, typ, ok)
		}
		if _, ok := names[c.not]; ok {
			t.Errorf("%s: %s must not exist in this version's System module", c.name, c.not)
		}
	}
}
