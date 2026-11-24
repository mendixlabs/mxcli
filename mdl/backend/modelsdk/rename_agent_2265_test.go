// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// ako/mxcli#2265: RENAME and MOVE left an Agent Editor document naming the
// old microflow, because it keeps the reference inside its Contents JSON.
// Studio Pro re-syncs it; nothing headless does, and mx check stays green.
func TestIssue2265_RenameRewritesAgentToolReference(t *testing.T) {
	b := connectAgentFixture(t)

	hits, err := b.RenameReferences("MyFirstModule.Tool_GetOrderStatus", "MyFirstModule.Tool_GetOrderStatus2", true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !hasHit(hits, "ZzAgent", 2) {
		t.Fatalf("dry run does not list the agent's two references: %+v", hits)
	}
	if a := agentNamed(t, b, "ZzAgent"); a.Tools[0].Document.QualifiedName != "MyFirstModule.Tool_GetOrderStatus" {
		t.Fatalf("dry run wrote: %+v", a.Tools[0].Document)
	}

	if _, err := b.RenameReferences("MyFirstModule.Tool_GetOrderStatus", "MyFirstModule.Tool_GetOrderStatus2", false); err != nil {
		t.Fatalf("rename: %v", err)
	}
	a := agentNamed(t, b, "ZzAgent")
	checkAgentRenamed(t, a, "MyFirstModule.Tool_GetOrderStatus2")
}

func TestIssue2265_MoveRewritesAgentToolReference(t *testing.T) {
	b := connectAgentFixture(t)
	n, err := b.UpdateQualifiedNameInAllUnits("MyFirstModule.Tool_GetOrderStatus", "Other.Tool_GetOrderStatus")
	if err != nil {
		t.Fatalf("move sweep: %v", err)
	}
	if n == 0 {
		t.Fatalf("move sweep updated no unit")
	}
	checkAgentRenamed(t, agentNamed(t, b, "ZzAgent"), "Other.Tool_GetOrderStatus")
}

func connectAgentFixture(t *testing.T) *Backend {
	t.Helper()
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
	tool := &agenteditor.DocRef{DocumentID: "dca6f75b", QualifiedName: "MyFirstModule.Tool_GetOrderStatus"}
	if err := b.CreateAgentEditorAgent(&agenteditor.Agent{
		ContainerID:  mod.ID,
		Name:         "ZzAgent",
		SystemPrompt: "Call MyFirstModule.Tool_GetOrderStatus first",
		UsageType:    "task",
		Tools: []agenteditor.AgentTool{
			{Name: "status", ToolType: "Microflow", Enabled: true, Document: tool},
			{Name: "other", ToolType: "Microflow", Enabled: true,
				Document: &agenteditor.DocRef{DocumentID: "bb", QualifiedName: "MyFirstModule.Tool_GetOrderStatusV2"}},
		},
		Entity: &agenteditor.DocRef{DocumentID: "e1", QualifiedName: "MyFirstModule.Tool_GetOrderStatus"},
	}); err != nil {
		t.Fatalf("CreateAgentEditorAgent: %v", err)
	}
	return b
}

func checkAgentRenamed(t *testing.T, a *agenteditor.Agent, want string) {
	t.Helper()
	if got := a.Tools[0].Document; got.QualifiedName != want || got.DocumentID != "dca6f75b" {
		t.Errorf("tool reference = %+v, want %s with its documentId kept", got, want)
	}
	if got := a.Tools[1].Document.QualifiedName; got != "MyFirstModule.Tool_GetOrderStatusV2" {
		t.Errorf("a name that only shares the prefix was rewritten: %s", got)
	}
	if a.Entity == nil || a.Entity.QualifiedName != want {
		t.Errorf("entity reference = %+v, want %s", a.Entity, want)
	}
	if !strings.Contains(a.SystemPrompt, "MyFirstModule.Tool_GetOrderStatus first") {
		t.Errorf("the prompt is the user's text and must not change: %q", a.SystemPrompt)
	}
}

func agentNamed(t *testing.T, b *Backend, name string) *agenteditor.Agent {
	t.Helper()
	agents, err := b.ListAgentEditorAgents()
	if err != nil {
		t.Fatalf("ListAgentEditorAgents: %v", err)
	}
	for _, a := range agents {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("agent %s not found", name)
	return nil
}

func hasHit(hits []types.RenameHit, name string, count int) bool {
	for _, h := range hits {
		if h.Name == name && h.Count == count {
			return true
		}
	}
	return false
}
