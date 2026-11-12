// SPDX-License-Identifier: Apache-2.0

package meta

import "testing"

// The workflow sub-process members of the System module — two entities, their
// associations and System.WorkflowSubProcessState — were missing, so a binding
// over System.WorkflowActivityRecord_WorkflowSubProcess could not resolve:
// describe -> exec of TestApp's WorkflowCommons.Snip_WorkflowActivityRecord_Header
// wrote text4 as an expression, mx check CE0117 "Error(s) in expression.".
//
// Measured from the System domain model in the deployment/model/model.mdp
// mxbuild 11.14.0 writes; the version they first appear in from the modeler
// assembly of each Mendix version (the #1810 method).
func atLeastVersion(major, minor int) func(int, int) bool {
	return func(ma, mi int) bool { return major > ma || major == ma && minor >= mi }
}

func TestWorkflowSubProcessMembers(t *testing.T) {
	ent, ok := systemEntityByName("WorkflowSubProcess")
	if !ok {
		t.Fatal("System.WorkflowSubProcess is not declared")
	}
	want := map[string]string{"Caption": "String", "StartTime": "DateTime", "EndTime": "DateTime", "State": "Enumeration", "Reason": "String"}
	for _, a := range ent.Attributes {
		if want[a.Name] != a.Type {
			t.Errorf("WorkflowSubProcess.%s is %s, measured %s", a.Name, a.Type, want[a.Name])
		}
		delete(want, a.Name)
	}
	if len(want) > 0 {
		t.Errorf("WorkflowSubProcess lacks %v", want)
	}
	if _, ok := systemEntityByName("WorkflowSubProcessDefinition"); !ok {
		t.Error("System.WorkflowSubProcessDefinition is not declared")
	}

	has := func(atLeast func(int, int) bool, name string) bool {
		for _, a := range ModelerSystemAssociationsIn(atLeast) {
			if a.Name == name {
				return true
			}
		}
		return false
	}
	hasEntity := func(atLeast func(int, int) bool, name string) bool {
		for _, e := range ModelerSystemEntitiesIn(atLeast) {
			if e.Name == name {
				return true
			}
		}
		return false
	}
	current := atLeastVersion(11, 14)
	if !has(current, "WorkflowActivityRecord_WorkflowSubProcess") || !hasEntity(current, "WorkflowSubProcess") {
		t.Error("11.14 must have WorkflowSubProcess and WorkflowActivityRecord_WorkflowSubProcess")
	}
	if has(nil, "WorkflowActivity_WorkflowSubProcess") {
		t.Error("WorkflowActivity_WorkflowSubProcess ends on a runtime-only entity and must not be modeler-visible")
	}
	// Before the version that introduced them, the names do not exist and a
	// model naming one is CE1613; the association goes with its end entity.
	before := atLeastVersion(subProcessBeforeMajor, subProcessBeforeMinor)
	if hasEntity(before, "WorkflowSubProcess") || has(before, "WorkflowActivityRecord_WorkflowSubProcess") {
		t.Errorf("%d.%d has no workflow sub-processes", subProcessBeforeMajor, subProcessBeforeMinor)
	}
	if !has(before, "WorkflowActivityRecord_Actor") {
		t.Error("control: an unrelated association must stay")
	}
	for _, e := range SystemEnumerationsIn(before) {
		if e.Name == "System.WorkflowSubProcessState" {
			t.Errorf("System.WorkflowSubProcessState listed for %d.%d", subProcessBeforeMajor, subProcessBeforeMinor)
		}
	}
}

// The last version measured WITHOUT the sub-process members.
const subProcessBeforeMajor, subProcessBeforeMinor = 11, 7
