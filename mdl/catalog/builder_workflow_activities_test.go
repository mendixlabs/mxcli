// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"strings"
	"testing"

	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
)

// ako/mxcli#1295 (mendixlabs/mxcli#1269): a lint rule could not ask which kind
// of workflow activity runs a microflow — "a system task's microflow is
// prefixed WF_SYS_", "a user task's targeting microflow WF_TGT_". refs records
// only that the WORKFLOW calls the MICROFLOW, and workflows_data only counts
// activities.

func TestWorkflowActivityRows_KindAndSlots_Issue1295(t *testing.T) {
	var got []string
	for _, r := range workflowActivityRows(everySlotWorkflow()) {
		got = append(got, fmt.Sprintf("%s mf=%s src=%s created=%s done=%s page=%s wf=%s",
			r.ActivityType, r.Microflow, r.UserSourceMicroflow, r.OnCreatedMicroflow,
			r.CompletionMicroflow, r.Page, r.CalledWorkflow))
	}
	want := []string{
		// The main flow, each activity followed by its boundary-event paths.
		"UserTask mf= src=M.TargetUsers created=M.OnCreated done= page=M.TaskPage wf=",
		"CallMicroflow mf=M.InUserTaskBoundary src= created= done= page= wf=",
		"MultiUserTask mf= src=M.TargetGroups created= done=M.Completion page= wf=",
		"CallMicroflow mf=M.MainCall src= created= done= page= wf=",
		"SystemTask mf=M.InCallBoundary src= created= done= page= wf=",
		"CallWorkflow mf= src= created= done= page= wf=M.SubWorkflow",
		"CallMicroflow mf=M.InCallWorkflowBoundary src= created= done= page= wf=",
		"WaitForNotification mf= src= created= done= page= wf=",
		"CallMicroflow mf=M.InWaitBoundary src= created= done= page= wf=",
		// Then the event sub-process.
		"InterruptingNotificationEventSubProcessStart mf= src= created= done= page= wf=",
		"CallMicroflow mf=M.InEventSubProcess src= created= done= page= wf=",
		"UserTask mf= src= created= done= page=M.SubProcessTaskPage wf=",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The table is written by a fast build: model-quality rules run on one.
func TestTestApp_WorkflowActivitiesInFastBuild_Issue1295(t *testing.T) {
	be := modelsdkbackend.New()
	if err := be.Connect(copyTestApp(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { be.Disconnect() })
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	if err := NewBuilder(cat, be).Build(nil); err != nil {
		t.Fatalf("catalog build: %v", err)
	}

	// workflow.UserTaskEventHandle is a user task's on-created microflow
	// (TestTestApp_WorkflowHandlersAreNotDead).
	rows := queryRows(t, cat, `SELECT WorkflowQualifiedName, ActivityType FROM workflow_activities
		WHERE OnCreatedMicroflow = 'workflow.UserTaskEventHandle'`)
	if len(rows) == 0 {
		t.Fatal("no activity has workflow.UserTaskEventHandle as its on-created microflow")
	}
	for _, r := range rows {
		if typ := r[1].(string); typ != "UserTask" && typ != "MultiUserTask" {
			t.Errorf("%v: on-created microflow on a %s", r[0], typ)
		}
	}
	// An empty slot is NULL, not ''.
	if n := queryRows(t, cat, `SELECT COUNT(*) FROM workflow_activities WHERE Microflow = '' OR Page = ''`); n[0][0].(int64) != 0 {
		t.Errorf("%v rows store an empty slot as ''", n[0][0])
	}
	// Every workflow's activity count matches its rows.
	for _, r := range queryRows(t, cat, `SELECT w.QualifiedName, w.ActivityCount,
			(SELECT COUNT(*) FROM workflow_activities a WHERE a.WorkflowId = w.Id)
		FROM workflows w`) {
		if r[1].(int64) != r[2].(int64) {
			t.Errorf("%v: ActivityCount %v but %v activity rows", r[0], r[1], r[2])
		}
	}
}
