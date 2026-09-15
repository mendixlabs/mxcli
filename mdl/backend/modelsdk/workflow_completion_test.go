// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	genWf "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// The workflow Studio Pro 11.14 saved for ako/TestApp's
// ZzMxcliExample_Completion2: one multi-user task per completion rule, built over
// MCP and checked clean before it was saved.
const completionFixture = "testdata/TestApp.ZzMxcliExample_Completion2.bson"

// The reader decodes every rule off the Studio Pro document, turning each outcome
// pointer ($ID) back into the outcome's value.
func TestReadCompletionCriteriaFromStudioProFixture(t *testing.T) {
	raw, err := os.ReadFile(completionFixture)
	if err != nil {
		t.Fatal(err)
	}
	g := genWf.NewWorkflow()
	g.InitFromRaw(bson.Raw(raw))
	wf := workflowFromGen(g, "")

	got := map[string]*workflows.UserTask{}
	for _, a := range wf.Flow.Activities {
		if ut, ok := a.(*workflows.UserTask); ok {
			got[ut.Name] = ut
		}
	}
	want := map[string]struct {
		cc    workflows.CompletionCriteria
		input workflows.TargetUserInput
		await bool
	}{
		"mutConsensus":    {workflows.CompletionCriteria{Kind: "Consensus", FallbackOutcome: "Fast"}, workflows.TargetUserInput{Kind: "All"}, false},
		"mutMajorityHalf": {workflows.CompletionCriteria{Kind: "Majority", CompletionType: "Absolute", FallbackOutcome: "Fast"}, workflows.TargetUserInput{Kind: "All"}, false},
		"mutMajorityMost": {workflows.CompletionCriteria{Kind: "Majority", CompletionType: "Relative", FallbackOutcome: "Fast"}, workflows.TargetUserInput{Kind: "All"}, false},
		"mutThresholdPct": {workflows.CompletionCriteria{Kind: "Threshold", CompletionType: "Relative", Threshold: 60, FallbackOutcome: "Fiurious"}, workflows.TargetUserInput{Kind: "Percentage", Percentage: 80}, false},
		"mutThresholdAbs": {workflows.CompletionCriteria{Kind: "Threshold", CompletionType: "Absolute", Threshold: 2, FallbackOutcome: "Fiurious"}, workflows.TargetUserInput{Kind: "Absolute", Amount: 3}, false},
		"mutVeto":         {workflows.CompletionCriteria{Kind: "Veto", VetoOutcome: "Fiurious"}, workflows.TargetUserInput{Kind: "All"}, true},
		"mutMicroflow":    {workflows.CompletionCriteria{Kind: "Microflow", Microflow: "workflow.ZzMxcliExample_DecideOutcome"}, workflows.TargetUserInput{Kind: "All"}, false},
	}
	for name, w := range want {
		ut := got[name]
		if ut == nil {
			t.Errorf("%s not read", name)
			continue
		}
		if ut.CompletionCriteria == nil || *ut.CompletionCriteria != w.cc {
			t.Errorf("%s criteria = %+v, want %+v", name, ut.CompletionCriteria, w.cc)
		}
		if ut.TargetUserInput == nil || *ut.TargetUserInput != w.input {
			t.Errorf("%s participants = %+v, want %+v", name, ut.TargetUserInput, w.input)
		}
		if ut.AwaitAllUsers != w.await {
			t.Errorf("%s await = %v, want %v", name, ut.AwaitAllUsers, w.await)
		}
	}
}

// Every rule survives a write and a fresh read, and the stored pointer is the
// $ID of the named outcome — Studio Pro's shape, not the first outcome by default.
func TestCreateWorkflow_CompletionCriteriaRoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	type setting struct {
		cc    *workflows.CompletionCriteria
		input *workflows.TargetUserInput
		await bool
	}
	settings := map[string]setting{
		"consensus":      {&workflows.CompletionCriteria{Kind: "Consensus", FallbackOutcome: "Reject"}, nil, false},
		"majorityHalf":   {&workflows.CompletionCriteria{Kind: "Majority", CompletionType: "Absolute", FallbackOutcome: "Reject"}, nil, false},
		"thresholdPct":   {&workflows.CompletionCriteria{Kind: "Threshold", CompletionType: "Relative", Threshold: 60, FallbackOutcome: "Reject"}, &workflows.TargetUserInput{Kind: "Percentage", Percentage: 80}, false},
		"thresholdVotes": {&workflows.CompletionCriteria{Kind: "Threshold", CompletionType: "Absolute", Threshold: 2, FallbackOutcome: "Approve"}, &workflows.TargetUserInput{Kind: "Absolute", Amount: 3}, true},
		"veto":           {&workflows.CompletionCriteria{Kind: "Veto", VetoOutcome: "Reject"}, nil, true},
		"microflow":      {&workflows.CompletionCriteria{Kind: "Microflow", Microflow: "MyFirstModule.Decide"}, nil, false},
		"default":        {nil, nil, false},
	}
	acts := []workflows.WorkflowActivity{&workflows.StartWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Start"}}}
	for name, s := range settings {
		task := &workflows.UserTask{
			BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: name, Caption: name},
			IsMulti:              true,
			Outcomes:             []*workflows.UserTaskOutcome{{Value: "Approve"}, {Value: "Reject"}},
			CompletionCriteria:   s.cc,
			TargetUserInput:      s.input,
			AwaitAllUsers:        s.await,
		}
		acts = append(acts, task)
	}
	acts = append(acts, &workflows.EndWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "End"}})
	wf := &workflows.Workflow{ContainerID: mod.ID, Name: "ZzCompletion", Parameter: &workflows.WorkflowParameter{EntityRef: "MyFirstModule.Ctx"}, Flow: &workflows.Flow{Activities: acts}}
	if err := b.CreateWorkflow(wf); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	all, err := b2.ListWorkflows()
	if err != nil {
		t.Fatal(err)
	}
	var got *workflows.Workflow
	for _, w := range all {
		if w.Name == "ZzCompletion" {
			got = w
		}
	}
	if got == nil {
		t.Fatal("workflow not found after reconnect")
	}
	for _, a := range got.Flow.Activities {
		ut, ok := a.(*workflows.UserTask)
		if !ok {
			continue
		}
		s := settings[ut.Name]
		wantCC := s.cc
		if wantCC == nil {
			wantCC = &workflows.CompletionCriteria{Kind: "Consensus", FallbackOutcome: "Approve"}
		}
		wantInput := s.input
		if wantInput == nil {
			wantInput = &workflows.TargetUserInput{Kind: "All"}
		}
		if !reflect.DeepEqual(ut.CompletionCriteria, wantCC) || !reflect.DeepEqual(ut.TargetUserInput, wantInput) || ut.AwaitAllUsers != s.await {
			t.Errorf("%s read back as %+v %+v %v, want %+v %+v %v", ut.Name, ut.CompletionCriteria, ut.TargetUserInput, ut.AwaitAllUsers, wantCC, wantInput, s.await)
		}
	}

	raw, err := b2.GetRawUnit(got.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range raw["Flow"].(map[string]any)["Activities"].([]any) {
		m, ok := a.(map[string]any)
		if !ok || m["Name"] != "majorityHalf" {
			continue
		}
		outs := m["Outcomes"].([]any)
		reject := outs[2].(map[string]any)["$ID"]
		cc := m["CompletionCriteria"].(map[string]any)
		if fmt.Sprint(cc["FallbackOutcomePointer"]) != fmt.Sprint(reject) || cc["CompletionType"] != "Absolute" {
			t.Errorf("majorityHalf CompletionCriteria = %v, want an Absolute majority pointing at Reject's $ID %v", cc, reject)
		}
	}
}
