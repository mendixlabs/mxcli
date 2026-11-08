// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The stored agent, shaped like ICXGraphPilot's GraphExplorer (Studio Pro
// 11.16): a microflow tool, a knowledge base tool, and an entity whose
// documentId Agent Editor filled in.
func storedGraphExplorer(mod *model.Module, mf *microflows.Microflow, kb *agenteditor.KnowledgeBase) *agenteditor.Agent {
	return &agenteditor.Agent{
		BaseElement: model.BaseElement{ID: "agent-1"},
		ContainerID: mod.ID,
		Name:        "GraphExplorer",
		UsageType:   "Chat",
		Tools: []agenteditor.AgentTool{{
			ID: "aed6208e-29df-4f9f-999e-f47b47b25109", Name: "Tool_ExceuteSparql", Enabled: true, ToolType: "Microflow",
			Document: &agenteditor.DocRef{DocumentID: string(mf.ID), QualifiedName: "M.Tool_ExecuteSparql"},
		}},
		KBTools: []agenteditor.AgentKBTool{{
			ID: "cf1c8147-c697-49e8-a9c1-2e313e7abdd6", Name: "WorkInstructionKnowledgeBase", Enabled: true,
			Document: &agenteditor.DocRef{DocumentID: string(kb.ID), QualifiedName: "M.KB_WorkInstructions"},
		}},
		Entity: &agenteditor.DocRef{DocumentID: "a2ab7baa-69bc-47b1-95b6-931a3e693a82", QualifiedName: "M.OnboardingHelper"},
	}
}

// describe → exec of an agent wrote its microflow tool with no document (the
// tool ran nothing), every tool with an empty id, and the entity with an empty
// documentId. The ids are what the write path pairs a tool with its stored
// entry by, so losing them also lost the tool's approval and display title.
func TestCreateOrModifyAgent_KeepsToolTargetsAndIDs(t *testing.T) {
	mod := mkModule("M")
	mf := mkMicroflow(mod.ID, "Tool_ExecuteSparql")
	kb := &agenteditor.KnowledgeBase{BaseElement: model.BaseElement{ID: "kb-1"}, ContainerID: mod.ID, Name: "KB_WorkInstructions"}
	stored := storedGraphExplorer(mod, mf, kb)
	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)

	var written *agenteditor.Agent
	mb := &mock.MockBackend{
		IsConnectedFunc:                   func() bool { return true },
		ListModulesFunc:                   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc:                func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		ListAgentEditorAgentsFunc:         func() ([]*agenteditor.Agent, error) { return []*agenteditor.Agent{stored}, nil },
		ListAgentEditorKnowledgeBasesFunc: func() ([]*agenteditor.KnowledgeBase, error) { return []*agenteditor.KnowledgeBase{kb}, nil },
		UpdateAgentEditorAgentFunc: func(a *agenteditor.Agent) error {
			written = a
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	// What describe prints for it, plus a tool the stored agent does not have.
	prog := parseMDL(t, `create or modify agent M.GraphExplorer (
  UsageType: Chat,
  Entity: M.OnboardingHelper
)
{
  tool Tool_ExceuteSparql (
    ToolType: Microflow,
    Document: M.Tool_ExecuteSparql,
    Enabled: true
  )
  knowledge base WorkInstructionKnowledgeBase (
    Source: M.KB_WorkInstructions,
    Enabled: true
  )
  tool AddedTool (
    ToolType: Microflow,
    Document: M.Tool_ExecuteSparql,
    Enabled: true
  )
};`)
	assertNoError(t, execCreateAgent(ctx, prog.Statements[0].(*ast.CreateAgentStmt)))
	if written == nil {
		t.Fatal("the agent was not written")
	}

	tool := written.Tools[0]
	if tool.ID != stored.Tools[0].ID {
		t.Errorf("tool id %q, want the stored %q", tool.ID, stored.Tools[0].ID)
	}
	if tool.Document == nil || tool.Document.DocumentID != string(mf.ID) || tool.Document.QualifiedName != "M.Tool_ExecuteSparql" {
		t.Errorf("tool document %+v, want the microflow %s", tool.Document, mf.ID)
	}
	if written.KBTools[0].ID != stored.KBTools[0].ID {
		t.Errorf("knowledge base tool id %q, want the stored %q", written.KBTools[0].ID, stored.KBTools[0].ID)
	}
	if written.Entity == nil || written.Entity.DocumentID != stored.Entity.DocumentID {
		t.Errorf("entity %+v, want the stored documentId %q", written.Entity, stored.Entity.DocumentID)
	}
	// Control: a tool the stored agent does not have pairs with nothing and
	// gets an id of its own.
	added := written.Tools[1]
	if added.ID == "" || added.ID == stored.Tools[0].ID {
		t.Errorf("added tool id %q, want a fresh one", added.ID)
	}
}

// A model's Type and Contents name are what Agent Editor picked from the
// Portal resource, with no MDL spelling; a restatement wrote both empty.
func TestCreateOrModifyModel_KeepsPortalModel(t *testing.T) {
	mod := mkModule("M")
	key := mkConstant(mod.ID, "ModelKey", "String", "")
	h := mkHierarchy(mod)
	withContainer(h, key.ContainerID, mod.ID)
	stored := &agenteditor.Model{
		BaseElement: model.BaseElement{ID: "model-1"}, ContainerID: mod.ID, Name: "ClaudeSonnet46",
		Type: "Text generation", InnerName: "eu.anthropic.claude-sonnet-4-6", Provider: "MxCloudGenAI",
	}
	var written *agenteditor.Model
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListModulesFunc:           func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListConstantsFunc:         func() ([]*model.Constant, error) { return []*model.Constant{key}, nil },
		ListAgentEditorModelsFunc: func() ([]*agenteditor.Model, error) { return []*agenteditor.Model{stored}, nil },
		UpdateAgentEditorModelFunc: func(m *agenteditor.Model) error {
			written = m
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	keyRef := ast.QualifiedName{Module: "M", Name: "ModelKey"}
	assertNoError(t, execCreateAgentEditorModel(ctx, &ast.CreateModelStmt{
		Name: ast.QualifiedName{Module: "M", Name: "ClaudeSonnet46"}, Provider: "MxCloudGenAI", Key: &keyRef, CreateOrModify: true,
	}))
	if written == nil || written.Type != "Text generation" || written.InnerName != "eu.anthropic.claude-sonnet-4-6" {
		t.Errorf("written %+v, want the stored Type and model name", written)
	}
}

// The knowledge base's Contents name is the Portal knowledge base; the
// encoder wrote it as "".
func TestCreateOrModifyKnowledgeBase_KeepsPortalName(t *testing.T) {
	mod := mkModule("M")
	h := mkHierarchy(mod)
	stored := &agenteditor.KnowledgeBase{
		BaseElement: model.BaseElement{ID: "kb-1"}, ContainerID: mod.ID, Name: "KB_WorkInstructions",
		InnerName: "WorkInstructions", Provider: "MxCloudGenAI",
	}
	var written *agenteditor.KnowledgeBase
	mb := &mock.MockBackend{
		IsConnectedFunc:                   func() bool { return true },
		ListModulesFunc:                   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListAgentEditorKnowledgeBasesFunc: func() ([]*agenteditor.KnowledgeBase, error) { return []*agenteditor.KnowledgeBase{stored}, nil },
		UpdateAgentEditorKnowledgeBaseFunc: func(k *agenteditor.KnowledgeBase) error {
			written = k
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execCreateKnowledgeBase(ctx, &ast.CreateKnowledgeBaseStmt{
		Name: ast.QualifiedName{Module: "M", Name: "KB_WorkInstructions"}, Provider: "MxCloudGenAI", CreateOrModify: true,
	}))
	if written == nil || written.InnerName != "WorkInstructions" {
		t.Errorf("written %+v, want the stored name WorkInstructions", written)
	}
}
