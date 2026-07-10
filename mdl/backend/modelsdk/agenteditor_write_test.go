// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
)

// TestCreateAgentEditor_RoundTrip creates one of each agent-editor document type
// (Model, KnowledgeBase, ConsumedMCPService, Agent) and confirms each round-trips
// through its List reader, including a representative Contents-JSON field.
func TestCreateAgentEditor_RoundTrip(t *testing.T) {
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
	cid := mod.ID

	if err := b.CreateAgentEditorModel(&agenteditor.Model{ContainerID: cid, Name: "ZzModel", DisplayName: "Zz Display"}); err != nil {
		t.Fatalf("CreateAgentEditorModel: %v", err)
	}
	if err := b.CreateAgentEditorKnowledgeBase(&agenteditor.KnowledgeBase{ContainerID: cid, Name: "ZzKB", ModelName: "text-embed"}); err != nil {
		t.Fatalf("CreateAgentEditorKnowledgeBase: %v", err)
	}
	if err := b.CreateAgentEditorConsumedMCPService(&agenteditor.ConsumedMCPService{ContainerID: cid, Name: "ZzMCP", Version: "1.0", ConnectionTimeoutSeconds: 30}); err != nil {
		t.Fatalf("CreateAgentEditorConsumedMCPService: %v", err)
	}
	if err := b.CreateAgentEditorAgent(&agenteditor.Agent{ContainerID: cid, Name: "ZzAgent", SystemPrompt: "be helpful", UsageType: "task"}); err != nil {
		t.Fatalf("CreateAgentEditorAgent: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	models, err := b2.ListAgentEditorModels()
	if err != nil {
		t.Fatalf("ListAgentEditorModels: %v", err)
	}
	if !hasModel(models, "ZzModel", "Zz Display") {
		t.Errorf("ZzModel (DisplayName) not round-tripped: %+v", models)
	}

	kbs, err := b2.ListAgentEditorKnowledgeBases()
	if err != nil || !hasName(len(kbs), kbNames(kbs), "ZzKB") {
		t.Errorf("ZzKB not round-tripped: %v %+v", err, kbs)
	}

	mcps, err := b2.ListAgentEditorConsumedMCPServices()
	if err != nil || !hasName(len(mcps), mcpNames(mcps), "ZzMCP") {
		t.Errorf("ZzMCP not round-tripped: %v %+v", err, mcps)
	}

	agents, err := b2.ListAgentEditorAgents()
	if err != nil {
		t.Fatalf("ListAgentEditorAgents: %v", err)
	}
	var found bool
	for _, a := range agents {
		if a.Name == "ZzAgent" {
			found = true
			if a.SystemPrompt != "be helpful" || a.UsageType != "task" {
				t.Errorf("ZzAgent Contents not round-tripped: %+v", a)
			}
		}
	}
	if !found {
		t.Errorf("ZzAgent not found after create")
	}
}

func hasModel(models []*agenteditor.Model, name, display string) bool {
	for _, m := range models {
		if m.Name == name && m.DisplayName == display {
			return true
		}
	}
	return false
}

func kbNames(ks []*agenteditor.KnowledgeBase) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = k.Name
	}
	return out
}

func mcpNames(cs []*agenteditor.ConsumedMCPService) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

func hasName(_ int, names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
