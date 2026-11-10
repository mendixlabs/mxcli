// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// leftUnsetFixture is a module M holding a stored agent and consumed MCP
// service, each with or without the setting, next to an AgentEditorCommons
// module of the given Marketplace version.
func leftUnsetWarnings(t *testing.T, commonsVersion string, storedHasValue bool, src string) []string {
	t.Helper()
	mod := mkModule("M")
	commons := mkModule("AgentEditorCommons")
	commons.FromAppStore = true
	commons.AppStoreVersion = commonsVersion
	h := mkHierarchy(mod, commons)
	withContainer(h, mod.ID, mod.ID)

	agent := &agenteditor.Agent{BaseElement: model.BaseElement{ID: "agent-1"}, ContainerID: mod.ID, Name: "Stored"}
	svc := &agenteditor.ConsumedMCPService{BaseElement: model.BaseElement{ID: "svc-1"}, ContainerID: mod.ID, Name: "StoredSvc"}
	if storedHasValue {
		agent.SelectedModel = &agenteditor.SelectedModel{ID: "gpt-4o", Name: "gpt-4o"}
		svc.Endpoint = &agenteditor.DocRef{DocumentID: "c-1", QualifiedName: "M.Url"}
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod, commons}, nil },
		ListAgentEditorAgentsFunc: func() ([]*agenteditor.Agent, error) {
			return []*agenteditor.Agent{agent}, nil
		},
		ListAgentEditorConsumedMCPServicesFunc: func() ([]*agenteditor.ConsumedMCPService, error) {
			return []*agenteditor.ConsumedMCPService{svc}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	sc := newScriptContext()
	warnAgentDocumentsLeftUnset(ctx, parseMDL(t, src), sc)
	return sc.warnings
}

func assertLeftUnset(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d warnings, want %d:\n  %s", len(got), len(want), strings.Join(got, "\n  "))
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) {
			t.Errorf("warning %d = %q, want it to contain %q", i, got[i], w)
		}
	}
}

const (
	newAgent = `create or modify agent M.New ( UsageType: Task, SystemPrompt: 'x', UserPrompt: 'y' );`
	newSvc   = `create or modify consumed mcp service M.NewSvc ( ProtocolVersion: v2025_03_26, Version: '1.0' );`
)

// ako/mxcli#2089: MDL-AGENT02/03 skipped every create or modify, which is how
// project scripts and describe create a document, so a new agent with no
// selected model and a new service with no endpoint were never flagged.
func TestLeftUnset_NewDocumentsWarn(t *testing.T) {
	got := leftUnsetWarnings(t, "2.6.0", true, "mdl 1;\n"+newAgent+"\n"+newSvc)
	assertLeftUnset(t, got, "MDL-AGENT02: agent M.New", "MDL-AGENT03: consumed mcp service M.NewSvc")
}

// A stored document that lacks the setting stays without it when the
// statement leaves the key out.
func TestLeftUnset_StoredWithoutValueWarns(t *testing.T) {
	got := leftUnsetWarnings(t, "2.6.0", false, `mdl 1;
create or modify agent M.Stored ( UsageType: Task, SystemPrompt: 'x', UserPrompt: 'y' );
create or modify consumed mcp service M.StoredSvc ( ProtocolVersion: v2025_03_26, Version: '1.0' );`)
	assertLeftUnset(t, got, "MDL-AGENT02: agent M.Stored", "MDL-AGENT03: consumed mcp service M.StoredSvc")
}

// Controls: a stored value is kept, a statement that sets one is fine, and a
// plain create is MDL-AGENT02/03's own (static) case, not reported twice.
func TestLeftUnset_Controls(t *testing.T) {
	assertLeftUnset(t, leftUnsetWarnings(t, "2.6.0", true, `mdl 1;
create or modify agent M.Stored ( UsageType: Task, SystemPrompt: 'x', UserPrompt: 'y' );
create or modify consumed mcp service M.StoredSvc ( ProtocolVersion: v2025_03_26, Version: '1.0' );
create or modify agent M.Set ( UsageType: Task, SelectedModel: 'gpt-4o', SystemPrompt: 'x', UserPrompt: 'y' );
create or modify consumed mcp service M.SetSvc ( Endpoint: @M.Url, ProtocolVersion: v2025_03_26, Version: '1.0' );
create agent M.Plain ( UsageType: Task, SystemPrompt: 'x', UserPrompt: 'y' );`))
}

// The script's own statements decide what is stored by the time it runs: a
// drop leaves nothing to keep, an earlier create or alter that sets the value
// does, and SelectedModel: ” clears it.
func TestLeftUnset_ScriptOrder(t *testing.T) {
	assertLeftUnset(t, leftUnsetWarnings(t, "2.6.0", true, `mdl 1;
drop agent M.Stored;
create or modify agent M.Stored ( UsageType: Task, SystemPrompt: 'x', UserPrompt: 'y' );`),
		"MDL-AGENT02: agent M.Stored")
	assertLeftUnset(t, leftUnsetWarnings(t, "2.6.0", false, `mdl 1;
create agent M.New ( UsageType: Task, SelectedModel: 'gpt-4o', SystemPrompt: 'x', UserPrompt: 'y' );
create or modify agent M.New ( UsageType: Task, SystemPrompt: 'z', UserPrompt: 'y' );
alter agent M.Stored set SelectedModel = 'gpt-4o';
create or modify agent M.Stored ( UsageType: Task, SystemPrompt: 'x', UserPrompt: 'y' );`))
	assertLeftUnset(t, leftUnsetWarnings(t, "2.6.0", true, `mdl 1;
create or modify agent M.Stored ( UsageType: Task, SelectedModel: '', SystemPrompt: 'x', UserPrompt: 'y' );`),
		"MDL-AGENT02: agent M.Stored")
}

// Agent Editor before 2.2 does not read selectedModel (measured in
// AgentEditorCommons 1.1.0), so an agent without one is fine there; an
// endpoint is read by every version.
func TestLeftUnset_AgentEditorBefore22(t *testing.T) {
	got := leftUnsetWarnings(t, "1.1.0", true, "mdl 1;\n"+newAgent+"\n"+newSvc)
	assertLeftUnset(t, got, "MDL-AGENT03: consumed mcp service M.NewSvc")
}
