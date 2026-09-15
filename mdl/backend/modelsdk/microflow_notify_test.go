// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/TestApp's ZzMxcliExample_Notify (Studio Pro 11.14): one notify action per
// target type, each targeting ZzMxcliExample_EventSubProcesses.
const notifyFixture = "testdata/TestApp.ZzMxcliExample_Notify.bson"

// Every target type and output variable is read off the Studio Pro document. The
// output variable was read under gen's "VariableName" key, where Studio Pro
// stores "OutputVariableName", so it came back empty; the target was not read.
func TestReadNotifyTargetsFromStudioProFixture(t *testing.T) {
	raw, err := os.ReadFile(notifyFixture)
	if err != nil {
		t.Fatal(err)
	}
	root, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(raw))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*microflows.NotifyWorkflowAction{}
	element.Walk(root, func(e element.Element) bool {
		if _, ok := e.(*genMf.NotifyWorkflowAction); ok {
			if a, ok := actionFromGen(e).(*microflows.NotifyWorkflowAction); ok {
				got[a.OutputVariableName] = a
			}
		}
		return true
	})

	const wf = "workflow.ZzMxcliExample_EventSubProcesses."
	want := map[string]microflows.NotifyTarget{
		"NotifiedCancel":    {TypeName: "Workflows$InterruptingNotificationEventSubProcessStartActivityTarget", Name: wf + "espCancelStart"},
		"NotifiedAudit":     {TypeName: "Workflows$NonInterruptingNotificationEventSubProcessStartActivityTarget", Name: wf + "espAuditStart"},
		"NotifiedDocuments": {TypeName: "Workflows$NotifyNotificationActivityTarget", Name: wf + "notifyReceived"},
		"NotifiedWithdrawn": {TypeName: "Workflows$NotifyNotificationBoundaryEventTarget", Name: wf + "beWithdrawn"},
		"NotifiedApproval":  {TypeName: "Workflows$NotifyWaitForNotificationActivityTarget", Name: wf + "waitApproval"},
	}
	if len(got) != len(want) {
		t.Fatalf("read %d notify actions by output variable, want %d: %v", len(got), len(want), got)
	}
	for variable, w := range want {
		a := got[variable]
		if a == nil || a.Target == nil || *a.Target != w || a.WorkflowVariable != "Workflow" {
			t.Errorf("%s read back as %+v, want target %+v", variable, a, w)
		}
	}
}

// A notify action is written the way Studio Pro stores it — the output variable
// under OutputVariableName, the target as a NotifyTarget whose name sits under
// Activity or BoundaryEvent — and reads back unchanged. Before Mendix 11.7 the
// target is the Activity string, which round-trips too.
func TestNotifyWorkflowAction_WritesStudioProKeys(t *testing.T) {
	encode := func(t *testing.T, a microflows.MicroflowAction) bson.Raw {
		t.Helper()
		raw, err := (&codec.Encoder{}).Encode(microflowActionToGen(a))
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		return bson.Raw(raw)
	}
	decode := func(t *testing.T, raw bson.Raw) *microflows.NotifyWorkflowAction {
		t.Helper()
		el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		a, ok := actionFromGen(el).(*microflows.NotifyWorkflowAction)
		if !ok {
			t.Fatalf("read back as %T", actionFromGen(el))
		}
		return a
	}

	for _, target := range []microflows.NotifyTarget{
		{TypeName: "Workflows$NotifyNotificationBoundaryEventTarget", Name: "M.W.withdrawn"},
		{TypeName: "Workflows$InterruptingNotificationEventSubProcessStartActivityTarget", Name: "M.W.cancelStart"},
	} {
		action := &microflows.NotifyWorkflowAction{WorkflowVariable: "Workflow", OutputVariableName: "Notified", Target: &target}
		action.ID = "id-notify"
		raw := encode(t, action)
		if v, err := raw.LookupErr("OutputVariableName"); err != nil || v.StringValue() != "Notified" {
			t.Errorf("OutputVariableName not stored under its Studio Pro key: %v", raw)
		}
		if _, err := raw.LookupErr("VariableName"); err == nil {
			t.Errorf("gen's wrong key VariableName was written: %v", raw)
		}
		key := target.Key()
		if v, err := raw.LookupErr("NotifyTarget", key); err != nil || v.StringValue() != target.Name {
			t.Errorf("NotifyTarget.%s not stored: %v", key, raw)
		}
		got := decode(t, raw)
		if got.OutputVariableName != "Notified" || got.Target == nil || *got.Target != target {
			t.Errorf("read back as %+v, want output Notified and target %+v", got, target)
		}
	}

	legacy := &microflows.NotifyWorkflowAction{WorkflowVariable: "Workflow", Activity: "M.W.waitApproval"}
	legacy.ID = "id-legacy"
	raw := encode(t, legacy)
	if _, err := raw.LookupErr("NotifyTarget"); err == nil {
		t.Errorf("a pre-11.7 notify must not store a NotifyTarget: %v", raw)
	}
	if got := decode(t, raw); got.Activity != "M.W.waitApproval" || got.Target != nil {
		t.Errorf("pre-11.7 notify read back as %+v", got)
	}
}
