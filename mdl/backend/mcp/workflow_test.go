// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

func TestMapWorkflow(t *testing.T) {
	b := &Backend{}
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Do"}
	call.Name = "callDo"
	call.Outcomes = []workflows.ConditionOutcome{&workflows.VoidConditionOutcome{}}
	start := &workflows.StartWorkflowActivity{}
	start.Name = "start"
	end := &workflows.EndWorkflowActivity{}
	end.Name = "end"

	wf := &workflows.Workflow{
		Name:         "Approve",
		WorkflowName: "Approve Order",
		Parameter:    &workflows.WorkflowParameter{EntityRef: "M.OrderCtx"},
		Flow:         &workflows.Flow{Activities: []workflows.WorkflowActivity{start, call, end}},
	}

	content, err := b.mapWorkflow(wf)
	if err != nil {
		t.Fatalf("mapWorkflow: %v", err)
	}
	if content["name"] != "Approve" || content["title"] != "Approve Order" {
		t.Fatalf("workflow shell: %+v", content)
	}
	param, _ := content["parameter"].(map[string]any)
	if param["$Type"] != "Workflows$Parameter" || param["entity"] != "M.OrderCtx" {
		t.Fatalf("parameter: %+v", param)
	}
	flow, _ := content["flow"].(map[string]any)
	acts, _ := flow["activities"].([]any)
	if len(acts) != 3 {
		t.Fatalf("activities: %+v", acts)
	}
	types := make([]string, 3)
	for i, a := range acts {
		types[i] = a.(map[string]any)["$Type"].(string)
	}
	want := []string{"Workflows$StartWorkflowActivity", "Workflows$CallMicroflowActivity", "Workflows$EndWorkflowActivity"}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("activity[%d] = %s, want %s", i, types[i], want[i])
		}
	}
	// CallMicroflow carries its microflow + the Void outcome.
	cm := acts[1].(map[string]any)
	if cm["microflow"] != "M.ACT_Do" {
		t.Fatalf("call microflow: %+v", cm)
	}
	ocs, _ := cm["outcomes"].([]any)
	if len(ocs) != 1 || ocs[0].(map[string]any)["$Type"] != "Workflows$VoidConditionOutcome" {
		t.Fatalf("outcomes: %+v", cm["outcomes"])
	}
}

func TestMapUserTask(t *testing.T) {
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Process"}
	call.Name = "proc"
	approve := &workflows.UserTaskOutcome{Value: "Approve", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{call}}}
	reject := &workflows.UserTaskOutcome{Value: "Reject"}
	ut := &workflows.UserTask{
		Page:       "M.TaskPage",
		UserSource: &workflows.XPathBasedUserSource{XPath: "[Status = 'Draft']"},
		Outcomes:   []*workflows.UserTaskOutcome{approve, reject},
	}
	ut.Name = "Review"
	ut.Caption = "Review it"

	m, err := mapWorkflowActivity(ut)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(UserTask): %v", err)
	}
	if m["$Type"] != "Workflows$SingleUserTaskActivity" || m["name"] != "Review" {
		t.Fatalf("user task: %+v", m)
	}
	if tp, _ := m["taskPage"].(map[string]any); tp["page"] != "M.TaskPage" {
		t.Fatalf("taskPage: %+v", m["taskPage"])
	}
	tg, _ := m["userTargeting"].(map[string]any)
	if tg["$Type"] != "Workflows$XPathUserTargeting" || tg["xPathConstraint"] != "[Status = 'Draft']" {
		t.Fatalf("userTargeting: %+v", tg)
	}
	ocs, _ := m["outcomes"].([]any)
	if len(ocs) != 2 {
		t.Fatalf("outcomes: %+v", ocs)
	}
	o0, _ := ocs[0].(map[string]any)
	if o0["value"] != "Approve" {
		t.Fatalf("outcome[0] value: %+v", o0)
	}
	// The Approve outcome carries a sub-flow with the mapped call-microflow.
	flow, _ := o0["flow"].(map[string]any)
	acts, _ := flow["activities"].([]any)
	if len(acts) != 1 || acts[0].(map[string]any)["$Type"] != "Workflows$CallMicroflowActivity" {
		t.Fatalf("outcome[0] flow: %+v", o0["flow"])
	}
}

func TestMapMultiUserTask(t *testing.T) {
	ut := &workflows.UserTask{
		IsMulti:    true,
		Page:       "M.TaskPage",
		UserSource: &workflows.XPathBasedUserSource{XPath: "[true()]"},
		Outcomes:   []*workflows.UserTaskOutcome{{Value: "Approved"}, {Value: "Denied"}},
	}
	ut.Name = "GroupApproval"
	m, err := mapWorkflowActivity(ut)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(MultiUserTask): %v", err)
	}
	// Multi differs from single: a bare string pageReference, no taskPage element.
	if m["$Type"] != "Workflows$MultiUserTaskActivity" || m["pageReference"] != "M.TaskPage" {
		t.Fatalf("multi user task: %+v", m)
	}
	if _, hasTaskPage := m["taskPage"]; hasTaskPage {
		t.Error("multi user task must not carry a taskPage element")
	}
	if ocs, _ := m["outcomes"].([]any); len(ocs) != 2 {
		t.Fatalf("outcomes: %+v", m["outcomes"])
	}
}

func TestMapUserTaskWithBoundaryEvent(t *testing.T) {
	ut := &workflows.UserTask{
		Page: "M.TaskPage",
		BoundaryEvents: []*workflows.BoundaryEvent{
			{EventType: "InterruptingTimer", TimerDelay: "addHours([%CurrentDateTime%], 2)"},
		},
	}
	ut.Name = "Review"
	m, err := mapWorkflowActivity(ut)
	if err != nil {
		t.Fatalf("mapWorkflowActivity: %v", err)
	}
	be, _ := m["boundaryEvents"].([]any)
	if len(be) != 1 {
		t.Fatalf("boundaryEvents: %+v", m["boundaryEvents"])
	}
	e0, _ := be[0].(map[string]any)
	if e0["$Type"] != "Workflows$InterruptingTimerBoundaryEvent" || e0["firstExecutionTime"] != "addHours([%CurrentDateTime%], 2)" {
		t.Fatalf("boundary event: %+v", e0)
	}
}

func TestMapCallWorkflowAndWaitForNotification(t *testing.T) {
	cw := &workflows.CallWorkflowActivity{Workflow: "M.SubProc"}
	cw.Name = "callSub"
	m, err := mapWorkflowActivity(cw)
	if err != nil {
		t.Fatalf("CallWorkflow: %v", err)
	}
	if m["$Type"] != "Workflows$CallWorkflowActivity" || m["workflow"] != "M.SubProc" {
		t.Fatalf("call workflow: %+v", m)
	}

	wn := &workflows.WaitForNotificationActivity{}
	wn.Name = "wait"
	m2, err := mapWorkflowActivity(wn)
	if err != nil {
		t.Fatalf("WaitForNotification: %v", err)
	}
	if m2["$Type"] != "Workflows$WaitForNotificationActivity" {
		t.Fatalf("wait for notification: %+v", m2)
	}
}

func TestMapDecision(t *testing.T) {
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Process"}
	call.Name = "proc"
	dec := &workflows.ExclusiveSplitActivity{
		Expression: "$ctx/Total > 1000",
		Outcomes: []workflows.ConditionOutcome{
			&workflows.BooleanConditionOutcome{Value: true, Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{call}}},
			&workflows.BooleanConditionOutcome{Value: false},
		},
	}
	dec.Name = "decide"
	m, err := mapWorkflowActivity(dec)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(Decision): %v", err)
	}
	if m["$Type"] != "Workflows$ExclusiveSplitActivity" || m["expression"] != "$ctx/Total > 1000" {
		t.Fatalf("decision: %+v", m)
	}
	ocs, _ := m["outcomes"].([]any)
	if len(ocs) != 2 {
		t.Fatalf("outcomes: %+v", ocs)
	}
	o0, _ := ocs[0].(map[string]any)
	if o0["$Type"] != "Workflows$BooleanConditionOutcome" || o0["value"] != true {
		t.Fatalf("outcome[0]: %+v", o0)
	}
	if _, ok := o0["flow"]; !ok {
		t.Fatalf("outcome[0] missing sub-flow: %+v", o0)
	}
}

func TestMapParallelSplit(t *testing.T) {
	mk := func(mf string) *workflows.Flow {
		c := &workflows.CallMicroflowTask{Microflow: mf}
		c.Name = "c"
		return &workflows.Flow{Activities: []workflows.WorkflowActivity{c}}
	}
	ps := &workflows.ParallelSplitActivity{Outcomes: []*workflows.ParallelSplitOutcome{
		{Flow: mk("M.A")}, {Flow: mk("M.B")},
	}}
	ps.Name = "split"
	m, err := mapWorkflowActivity(ps)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(ParallelSplit): %v", err)
	}
	if m["$Type"] != "Workflows$ParallelSplitActivity" {
		t.Fatalf("parallel split: %+v", m)
	}
	ocs, _ := m["outcomes"].([]any)
	if len(ocs) != 2 {
		t.Fatalf("outcomes: %+v", ocs)
	}
	for i, want := range []string{"M.A", "M.B"} {
		oc := ocs[i].(map[string]any)
		if oc["$Type"] != "Workflows$ParallelSplitOutcome" {
			t.Fatalf("outcome[%d]: %+v", i, oc)
		}
		flow := oc["flow"].(map[string]any)
		act := flow["activities"].([]any)[0].(map[string]any)
		if act["microflow"] != want {
			t.Fatalf("path[%d] microflow = %v, want %s", i, act["microflow"], want)
		}
	}
}

func TestMapJumpTo(t *testing.T) {
	j := &workflows.JumpToActivity{TargetActivity: "ReviewStep"}
	j.Name = "jump"
	m, err := mapWorkflowActivity(j)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(JumpTo): %v", err)
	}
	if m["$Type"] != "Workflows$JumpToActivity" || m["targetActivity"] != "ReviewStep" {
		t.Fatalf("jump to: %+v", m)
	}
}

func TestMapWaitForTimer(t *testing.T) {
	w := &workflows.WaitForTimerActivity{DelayExpression: "addHours([%CurrentDateTime%], 1)"}
	w.Name = "wait"
	m, err := mapWorkflowActivity(w)
	if err != nil {
		t.Fatalf("mapWorkflowActivity(WaitForTimer): %v", err)
	}
	if m["$Type"] != "Workflows$WaitForTimerActivity" || m["delay"] != "addHours([%CurrentDateTime%], 1)" {
		t.Fatalf("wait for timer: %+v", m)
	}
}

func TestMapWorkflowActivity_Unsupported(t *testing.T) {
	// An activity type not yet mapped is rejected, not silently dropped.
	w := &workflows.WorkflowAnnotationActivity{Description: "note"}
	w.Name = "annot"
	if _, err := mapWorkflowActivity(w); err == nil {
		t.Error("unmapped workflow activity should be rejected")
	}
}

func TestWorkflowMutator_SetProperties(t *testing.T) {
	m := &mcpWorkflowMutator{backend: &Backend{}, moduleName: "M", workflowName: "WF"}
	if err := m.SetProperty("display", "New Name"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetProperty("description", "New Desc"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPropertyWithEntity("parameter", "$ctx", "M.NewCtx"); err != nil {
		t.Fatal(err)
	}
	// Expect ops: /workflowName/text, /title, /workflowDescription/text, /parameter/entity.
	got := map[string]any{}
	for _, op := range m.ops {
		got[op.Path] = op.Operation.Value
	}
	want := map[string]any{
		"/workflowName/text":        "New Name",
		"/title":                    "New Name",
		"/workflowDescription/text": "New Desc",
		"/parameter/entity":         "M.NewCtx",
	}
	for path, v := range want {
		if got[path] != v {
			t.Errorf("op %s = %v, want %v", path, got[path], v)
		}
	}
	// Unsupported workflow-level property is rejected.
	if err := m.SetProperty("export_level", "api"); err == nil {
		t.Error("SET export_level should be rejected")
	}
	// An unsupported activity property is rejected before any client call.
	if err := m.SetActivityProperty("x", 0, "bogus_prop", "p"); err == nil {
		t.Error("unsupported activity property should be rejected")
	}
}

// --- ALTER WORKFLOW activity-level ops (outcome/path/branch/boundary/property) ---

// wfMutatorFake returns a fakePED scripting the reads the activity-level ops need:
// /flow/activities (for index resolution) and the nested outcome/targeting arrays.
func wfMutatorFake(t *testing.T) (*fakePED, *mcpWorkflowMutator) {
	t.Helper()
	results := map[string]string{
		"/flow/activities": `[{"$Type":"Workflows$SingleUserTaskActivity","name":"ReviewOrder","caption":"Review the order"},
			{"$Type":"Workflows$ExclusiveSplitActivity","name":"Decide","caption":"Decision"},
			{"$Type":"Workflows$ParallelSplitActivity","name":"Split","caption":"Parallel split"}]`,
		"/flow/activities/0/outcomes": `[{"$Type":"Workflows$UserTaskOutcome","value":"Approve"},
			{"$Type":"Workflows$UserTaskOutcome","value":"Hold"}]`,
		"/flow/activities/1/outcomes": `[{"$Type":"Workflows$BooleanConditionOutcome","value":true},
			{"$Type":"Workflows$BooleanConditionOutcome","value":false}]`,
		"/flow/activities/2/outcomes": `[{"$Type":"Workflows$ParallelSplitOutcome"},
			{"$Type":"Workflows$ParallelSplitOutcome"},{"$Type":"Workflows$ParallelSplitOutcome"}]`,
		"/flow/activities/0/userTargeting":  `{"$Type":"Workflows$XPathUserTargeting"}`,
		"/flow/activities/0/boundaryEvents": `[{"$Type":"Workflows$InterruptingTimerBoundaryEvent"}]`,
	}
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		if name == "ped_check_errors" {
			return "No errors found.", false
		}
		if name != "ped_read_document" {
			return "SUCCESS", false
		}
		paths, _ := args["paths"].([]any)
		p, _ := paths[0].(string)
		v, ok := results[p]
		if !ok {
			v = "null"
		}
		return fmt.Sprintf(`{"results":[{"path":%q,"result":%s}]}`, p, v), false
	})
	b := &Backend{client: f.connectClient(t)}
	return f, &mcpWorkflowMutator{backend: b, moduleName: "M", workflowName: "WF"}
}

func wfUpdateOps(t *testing.T, f *fakePED) string {
	t.Helper()
	call, ok := f.callByName("ped_update_document")
	if !ok {
		t.Fatal("no ped_update_document sent")
	}
	raw, _ := json.Marshal(call.Args["operations"])
	return string(raw)
}

func TestWFInsertOutcome(t *testing.T) {
	f, m := wfMutatorFake(t)
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Review"}
	call.Name = "rev"
	if err := m.InsertOutcome("ReviewOrder", 0, "Escalate", []workflows.WorkflowActivity{call}); err != nil {
		t.Fatal(err)
	}
	ops := wfUpdateOps(t, f)
	for _, want := range []string{
		`"path":"/flow/activities/0/outcomes"`, `"type":"add"`,
		`"$Type":"Workflows$UserTaskOutcome"`, `"value":"Escalate"`,
		`"$Type":"Workflows$CallMicroflowActivity"`, // sub-flow mapped
	} {
		if !strings.Contains(ops, want) {
			t.Errorf("insert outcome op missing %s: %s", want, ops)
		}
	}
}

func TestWFDropOutcome(t *testing.T) {
	f, m := wfMutatorFake(t)
	if err := m.DropOutcome("ReviewOrder", 0, "Hold"); err != nil { // index 1
		t.Fatal(err)
	}
	ops := wfUpdateOps(t, f)
	for _, want := range []string{`"path":"/flow/activities/0/outcomes"`, `"type":"remove"`, `"index":1`} {
		if !strings.Contains(ops, want) {
			t.Errorf("drop outcome op missing %s: %s", want, ops)
		}
	}
	// A missing outcome is an error, not a silent no-op.
	_, m2 := wfMutatorFake(t)
	if err := m2.DropOutcome("ReviewOrder", 0, "Nope"); err == nil {
		t.Error("dropping a missing outcome should error")
	}
}

func TestWFInsertPath(t *testing.T) {
	f, m := wfMutatorFake(t)
	if err := m.InsertPath("Parallel split", 0, "", nil); err != nil {
		t.Fatal(err)
	}
	ops := wfUpdateOps(t, f)
	for _, want := range []string{`"path":"/flow/activities/2/outcomes"`, `"type":"add"`, `"$Type":"Workflows$ParallelSplitOutcome"`} {
		if !strings.Contains(ops, want) {
			t.Errorf("insert path op missing %s: %s", want, ops)
		}
	}
}

func TestWFDropPath(t *testing.T) {
	f, m := wfMutatorFake(t)
	if err := m.DropPath("Parallel split", 0, "Path 2"); err != nil { // index 1
		t.Fatal(err)
	}
	if ops := wfUpdateOps(t, f); !strings.Contains(ops, `"path":"/flow/activities/2/outcomes"`) || !strings.Contains(ops, `"index":1`) {
		t.Errorf("drop path 'Path 2' wrong: %s", ops)
	}
	// Empty caption drops the last path (index 2 of 3).
	f2, m2 := wfMutatorFake(t)
	if err := m2.DropPath("Parallel split", 0, ""); err != nil {
		t.Fatal(err)
	}
	if ops := wfUpdateOps(t, f2); !strings.Contains(ops, `"index":2`) {
		t.Errorf("drop last path wrong: %s", ops)
	}
}

func TestWFInsertBranch(t *testing.T) {
	cases := map[string]string{
		"true":     `"$Type":"Workflows$BooleanConditionOutcome"`,
		"default":  `"$Type":"Workflows$VoidConditionOutcome"`,
		"Approved": `"$Type":"Workflows$EnumerationValueConditionOutcome"`,
	}
	for cond, wantType := range cases {
		f, m := wfMutatorFake(t)
		if err := m.InsertBranch("Decision", 0, cond, nil); err != nil {
			t.Fatalf("%s: %v", cond, err)
		}
		ops := wfUpdateOps(t, f)
		if !strings.Contains(ops, `"path":"/flow/activities/1/outcomes"`) || !strings.Contains(ops, wantType) {
			t.Errorf("insert branch %q: want %s in %s", cond, wantType, ops)
		}
	}
}

func TestWFDropBranch(t *testing.T) {
	f, m := wfMutatorFake(t)
	if err := m.DropBranch("Decision", 0, "false"); err != nil { // index 1 (value:false)
		t.Fatal(err)
	}
	if ops := wfUpdateOps(t, f); !strings.Contains(ops, `"path":"/flow/activities/1/outcomes"`) || !strings.Contains(ops, `"index":1`) {
		t.Errorf("drop branch false wrong: %s", ops)
	}
}

func TestWFBoundaryEvent(t *testing.T) {
	f, m := wfMutatorFake(t)
	if err := m.InsertBoundaryEvent("ReviewOrder", 0, "NonInterruptingTimer", "addHours([%CurrentDateTime%], 1)", nil); err != nil {
		t.Fatal(err)
	}
	ops := wfUpdateOps(t, f)
	for _, want := range []string{
		`"path":"/flow/activities/0/boundaryEvents"`, `"type":"add"`,
		`"$Type":"Workflows$NonInterruptingTimerBoundaryEvent"`,
		`"firstExecutionTime":"addHours([%CurrentDateTime%], 1)"`,
	} {
		if !strings.Contains(ops, want) {
			t.Errorf("insert boundary event missing %s: %s", want, ops)
		}
	}
	// DROP removes index 0.
	f2, m2 := wfMutatorFake(t)
	if err := m2.DropBoundaryEvent("ReviewOrder", 0); err != nil {
		t.Fatal(err)
	}
	if ops := wfUpdateOps(t, f2); !strings.Contains(ops, `"path":"/flow/activities/0/boundaryEvents"`) || !strings.Contains(ops, `"index":0`) {
		t.Errorf("drop boundary event wrong: %s", ops)
	}
}

func TestWFSetActivityProperty(t *testing.T) {
	cases := map[string]string{
		"page":        `"path":"/flow/activities/0/taskPage/page"`,
		"description": `"path":"/flow/activities/0/taskDescription/text"`,
		"due_date":    `"path":"/flow/activities/0/dueDate"`,
	}
	for prop, wantPath := range cases {
		f, m := wfMutatorFake(t)
		if err := m.SetActivityProperty("ReviewOrder", 0, prop, "X"); err != nil {
			t.Fatalf("%s: %v", prop, err)
		}
		if ops := wfUpdateOps(t, f); !strings.Contains(ops, wantPath) || !strings.Contains(ops, `"type":"set"`) {
			t.Errorf("set activity %s: want %s in %s", prop, wantPath, ops)
		}
	}
	// Changing the targeting *kind* (current is XPath) is rejected.
	_, m := wfMutatorFake(t)
	if err := m.SetActivityProperty("ReviewOrder", 0, "targeting_microflow", "M.Pick"); err == nil {
		t.Error("switching targeting kind from XPath to Microflow should be rejected")
	}
}

func TestUpdateWorkflow_ReplacesFlowAndProperties(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		switch name {
		case "ped_check_errors":
			return "No errors found.", false
		case "ped_read_document":
			// The existing flow has 3 activities (Start, X, End).
			return `{"results":[{"path":"/flow/activities","result":[
				{"$Type":"Workflows$StartWorkflowActivity"},
				{"$Type":"Workflows$CallMicroflowActivity"},
				{"$Type":"Workflows$EndWorkflowActivity"}]}]}`, false
		}
		return "SUCCESS", false
	})
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	b := &Backend{client: f.connectClient(t), sessionModules: []*model.Module{mod}}

	start := &workflows.StartWorkflowActivity{}
	start.Name = "Start"
	end := &workflows.EndWorkflowActivity{}
	end.Name = "End"
	y := &workflows.CallMicroflowTask{Microflow: "M.Y"}
	y.Name = "Y"
	wf := &workflows.Workflow{
		Name: "WF", WorkflowName: "Display",
		Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"},
		Flow:      &workflows.Flow{Activities: []workflows.WorkflowActivity{start, y, end}},
	}
	wf.ContainerID = "mod1"
	wf.ID = "wfid"

	if err := b.UpdateWorkflow(wf); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	call, ok := f.callByName("ped_update_document")
	if !ok {
		t.Fatal("no ped_update_document sent")
	}
	ops, _ := call.Args["operations"].([]any)
	var adds, removes int
	for _, o := range ops {
		op, _ := o.(map[string]any)["operation"].(map[string]any)
		switch op["type"] {
		case "add":
			adds++
			// Middles are inserted just after Start (index 1), in reverse order.
			if idx, _ := op["index"].(float64); idx != 1 {
				t.Errorf("flow-replace add must target index 1, got %v", op["index"])
			}
		case "remove":
			removes++
		}
	}
	// New flow [Start, Y, End] and existing [Start, X, End]: only the middle is
	// replaced (Start/End preserved) -> 1 add + 1 remove.
	if adds != 1 || removes != 1 {
		t.Errorf("expected 1 middle add + 1 middle remove, got %d/%d", adds, removes)
	}
	raw, _ := json.Marshal(ops)
	s := string(raw)
	for _, want := range []string{
		`"path":"/workflowName/text"`, `"value":"Display"`,
		`"path":"/title"`,
		`"path":"/parameter/entity"`, `"value":"M.Ctx"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("UpdateWorkflow ops missing %s: %s", want, s)
		}
	}
	if len(b.sessionWorkflows) != 1 || b.sessionWorkflows[0].ID != "wfid" {
		t.Errorf("session workflow not upserted: %+v", b.sessionWorkflows)
	}
}

func TestWFNestedActivityResolution(t *testing.T) {
	// A workflow whose user task "Review" has an Approve outcome containing a
	// nested "NestedCall" activity. Dropping NestedCall must target the nested
	// sub-flow's activities array, not the top level.
	results := map[string]string{
		"/flow/activities": `[{"$Type":"Workflows$StartWorkflowActivity","name":"Start"},
			{"$Type":"Workflows$SingleUserTaskActivity","name":"Review","caption":"Review"},
			{"$Type":"Workflows$EndWorkflowActivity","name":"End"}]`,
		"/flow/activities/1/outcomes":                   `[{"$Type":"Workflows$UserTaskOutcome","value":"Approve","flow":{"$Type":"Workflows$Flow"}}]`,
		"/flow/activities/1/outcomes/0/flow/activities": `[{"$Type":"Workflows$CallMicroflowActivity","name":"NestedCall","caption":"NestedCall"}]`,
	}
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		if name == "ped_check_errors" {
			return "No errors found.", false
		}
		if name != "ped_read_document" {
			return "SUCCESS", false
		}
		paths, _ := args["paths"].([]any)
		p, _ := paths[0].(string)
		v, ok := results[p]
		if !ok {
			v = "null"
		}
		return fmt.Sprintf(`{"results":[{"path":%q,"result":%s}]}`, p, v), false
	})
	m := &mcpWorkflowMutator{backend: &Backend{client: f.connectClient(t)}, moduleName: "M", workflowName: "WF"}

	// Resolve lands in the nested flow.
	arrayPath, index, actPath, err := m.resolve("NestedCall", 0)
	if err != nil {
		t.Fatalf("resolve nested: %v", err)
	}
	if arrayPath != "/flow/activities/1/outcomes/0/flow/activities" || index != 0 {
		t.Fatalf("nested resolve = %q[%d], want /flow/activities/1/outcomes/0/flow/activities[0]", arrayPath, index)
	}
	if actPath != "/flow/activities/1/outcomes/0/flow/activities/0" {
		t.Fatalf("actPath = %q", actPath)
	}

	// DropActivity on the nested ref removes from the nested array.
	if err := m.DropActivity("NestedCall", 0); err != nil {
		t.Fatalf("DropActivity nested: %v", err)
	}
	call, _ := f.callByName("ped_update_document")
	raw, _ := json.Marshal(call.Args["operations"])
	if !strings.Contains(string(raw), `"path":"/flow/activities/1/outcomes/0/flow/activities"`) || !strings.Contains(string(raw), `"index":0`) {
		t.Errorf("nested drop op wrong: %s", raw)
	}
}
