// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
)

// TestAlterModel_Mock verifies execAlterModel patches the existing model
// scalars and calls UpdateAgentEditorModel exactly once.
func TestAlterModel_Mock(t *testing.T) {
	mod := mkModule("M")
	mdl := &agenteditor.Model{
		BaseElement: model.BaseElement{ID: nextID("aem")},
		ContainerID: mod.ID,
		Name:        "GPT4",
		Provider:    "MxCloudGenAI",
	}
	h := mkHierarchy(mod)
	withContainer(h, mdl.ContainerID, mod.ID)

	var updated *agenteditor.Model
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListAgentEditorModelsFunc: func() ([]*agenteditor.Model, error) { return []*agenteditor.Model{mdl}, nil },
		UpdateAgentEditorModelFunc: func(m *agenteditor.Model) error {
			updated = m
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterModel(ctx, &ast.AlterModelStmt{
		Name: ast.QualifiedName{Module: "M", Name: "GPT4"},
		Changes: map[string]string{
			"displayname": "GPT-4 Turbo",
			"keyname":     "OPENAI_KEY",
		},
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Altered model: M.GPT4")
	if updated == nil {
		t.Fatal("UpdateAgentEditorModelFunc was not called")
	}
	if updated.DisplayName != "GPT-4 Turbo" {
		t.Errorf("DisplayName = %q", updated.DisplayName)
	}
	if updated.KeyName != "OPENAI_KEY" {
		t.Errorf("KeyName = %q", updated.KeyName)
	}
	if updated.Provider != "MxCloudGenAI" {
		t.Errorf("Provider lost during alter: %q", updated.Provider)
	}
}

// TestAlterKnowledgeBase_Mock — scalar patch.
func TestAlterKnowledgeBase_Mock(t *testing.T) {
	mod := mkModule("M")
	kb := &agenteditor.KnowledgeBase{
		BaseElement: model.BaseElement{ID: nextID("aekb")},
		ContainerID: mod.ID,
		Name:        "Docs",
		Provider:    "MxCloudGenAI",
	}
	h := mkHierarchy(mod)
	withContainer(h, kb.ContainerID, mod.ID)

	var updated *agenteditor.KnowledgeBase
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListAgentEditorKnowledgeBasesFunc: func() ([]*agenteditor.KnowledgeBase, error) {
			return []*agenteditor.KnowledgeBase{kb}, nil
		},
		UpdateAgentEditorKnowledgeBaseFunc: func(k *agenteditor.KnowledgeBase) error {
			updated = k
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterKnowledgeBase(ctx, &ast.AlterKnowledgeBaseStmt{
		Name:    ast.QualifiedName{Module: "M", Name: "Docs"},
		Changes: map[string]string{"modelname": "text-embedding-3-small"},
	})
	assertNoError(t, err)
	if updated == nil || updated.ModelName != "text-embedding-3-small" {
		t.Errorf("ModelName not patched: %+v", updated)
	}
}

// TestAlterConsumedMCPService_Mock — scalar patch including int field.
func TestAlterConsumedMCPService_Mock(t *testing.T) {
	mod := mkModule("M")
	svc := &agenteditor.ConsumedMCPService{
		BaseElement:              model.BaseElement{ID: nextID("aemcp")},
		ContainerID:              mod.ID,
		Name:                     "Weather",
		ProtocolVersion:          "v2025_03_26",
		ConnectionTimeoutSeconds: 30,
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	var updated *agenteditor.ConsumedMCPService
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListAgentEditorConsumedMCPServicesFunc: func() ([]*agenteditor.ConsumedMCPService, error) {
			return []*agenteditor.ConsumedMCPService{svc}, nil
		},
		UpdateAgentEditorConsumedMCPServiceFunc: func(c *agenteditor.ConsumedMCPService) error {
			updated = c
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterConsumedMCPService(ctx, &ast.AlterConsumedMCPServiceStmt{
		Name: ast.QualifiedName{Module: "M", Name: "Weather"},
		Changes: map[string]string{
			"connectiontimeoutseconds": "60",
			"version":                  "1.0.1",
		},
	})
	assertNoError(t, err)
	if updated == nil {
		t.Fatal("UpdateAgentEditorConsumedMCPServiceFunc not called")
	}
	if updated.ConnectionTimeoutSeconds != 60 {
		t.Errorf("ConnectionTimeoutSeconds = %d", updated.ConnectionTimeoutSeconds)
	}
	if updated.Version != "1.0.1" {
		t.Errorf("Version = %q", updated.Version)
	}
}

// TestAlterAgent_SetAndCollections — covers SET, ADD, DROP across all three
// collections in one call.
func TestAlterAgent_SetAndCollections(t *testing.T) {
	mod := mkModule("M")
	mdl := &agenteditor.Model{
		BaseElement: model.BaseElement{ID: nextID("aem")},
		ContainerID: mod.ID,
		Name:        "GPT4",
	}
	mcp := &agenteditor.ConsumedMCPService{
		BaseElement: model.BaseElement{ID: nextID("aemcp")},
		ContainerID: mod.ID,
		Name:        "Weather",
	}
	kb := &agenteditor.KnowledgeBase{
		BaseElement: model.BaseElement{ID: nextID("aekb")},
		ContainerID: mod.ID,
		Name:        "Corp",
	}
	a := &agenteditor.Agent{
		BaseElement: model.BaseElement{ID: nextID("aea")},
		ContainerID: mod.ID,
		Name:        "Helper",
		Tools: []agenteditor.AgentTool{
			{Name: "OldTool", ToolType: "Microflow", Enabled: true},
		},
		KBTools: []agenteditor.AgentKBTool{{Name: "OldKB"}},
	}
	h := mkHierarchy(mod)
	withContainer(h, a.ContainerID, mod.ID)
	withContainer(h, mdl.ContainerID, mod.ID)
	withContainer(h, mcp.ContainerID, mod.ID)
	withContainer(h, kb.ContainerID, mod.ID)

	var updated *agenteditor.Agent
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListAgentEditorAgentsFunc: func() ([]*agenteditor.Agent, error) { return []*agenteditor.Agent{a}, nil },
		ListAgentEditorModelsFunc: func() ([]*agenteditor.Model, error) { return []*agenteditor.Model{mdl}, nil },
		ListAgentEditorConsumedMCPServicesFunc: func() ([]*agenteditor.ConsumedMCPService, error) {
			return []*agenteditor.ConsumedMCPService{mcp}, nil
		},
		ListAgentEditorKnowledgeBasesFunc: func() ([]*agenteditor.KnowledgeBase, error) {
			return []*agenteditor.KnowledgeBase{kb}, nil
		},
		UpdateAgentEditorAgentFunc: func(ag *agenteditor.Agent) error {
			updated = ag
			return nil
		},
	}

	mcpRef := ast.QualifiedName{Module: "M", Name: "Weather"}
	kbRef := ast.QualifiedName{Module: "M", Name: "Corp"}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterAgent(ctx, &ast.AlterAgentStmt{
		Name: ast.QualifiedName{Module: "M", Name: "Helper"},
		Sets: map[string]string{
			"systemprompt": "New prompt",
			"temperature":  "0.5",
			"maxtokens":    "8192",
		},
		AddTools: []ast.AgentToolDef{
			{ToolType: "Microflow", Name: "DoSomething", Description: "d", Enabled: true},
			{ToolType: "MCP", Name: "M.Weather", Document: &mcpRef, Description: "w", Enabled: true},
		},
		AddKBs: []ast.AgentKBToolDef{
			{Name: "Docs", Source: &kbRef, Collection: "corp", MaxResults: 5, Enabled: true},
		},
		DropTools: []string{"OldTool"},
		DropKBs:   []string{"OldKB"},
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Altered agent: M.Helper")

	if updated == nil {
		t.Fatal("UpdateAgentEditorAgentFunc not called")
	}
	if updated.SystemPrompt != "New prompt" {
		t.Errorf("SystemPrompt = %q", updated.SystemPrompt)
	}
	if updated.Temperature == nil || *updated.Temperature != 0.5 {
		t.Errorf("Temperature = %v", updated.Temperature)
	}
	if updated.MaxTokens == nil || *updated.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %v", updated.MaxTokens)
	}
	if len(updated.Tools) != 2 {
		t.Fatalf("Tools after alter = %+v", updated.Tools)
	}
	if updated.Tools[0].Name != "DoSomething" || updated.Tools[0].ToolType != "Microflow" {
		t.Errorf("Tools[0] = %+v", updated.Tools[0])
	}
	if updated.Tools[1].ToolType != "MCP" || updated.Tools[1].Document == nil || updated.Tools[1].Document.QualifiedName != "M.Weather" {
		t.Errorf("Tools[1] = %+v", updated.Tools[1])
	}
	if len(updated.KBTools) != 1 || updated.KBTools[0].Name != "Docs" {
		t.Errorf("KBTools = %+v", updated.KBTools)
	}
}

// TestAlterAgent_NotFound — agent doesn't exist.
func TestAlterAgent_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListAgentEditorAgentsFunc: func() ([]*agenteditor.Agent, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy()))
	err := execAlterAgent(ctx, &ast.AlterAgentStmt{
		Name: ast.QualifiedName{Module: "M", Name: "Missing"},
		Sets: map[string]string{"description": "x"},
	})
	if err == nil {
		t.Fatal("expected NotFound error")
	}
}
