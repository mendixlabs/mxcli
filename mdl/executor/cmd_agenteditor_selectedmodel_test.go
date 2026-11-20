// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2031: Agent Editor 2.2+ imports an agent only with a selected
// model (AgentFromModel_Validate requires SelectedModelID, and the deployed
// model is retrieved by Model = SelectedModelID). mxcli could not write one.
func TestCreateOrModifyAgent_SelectedModel(t *testing.T) {
	run := func(t *testing.T, src string, stored *agenteditor.SelectedModel) *agenteditor.Agent {
		t.Helper()
		mod := mkModule("M")
		mf := mkMicroflow(mod.ID, "Tool_ExecuteSparql")
		kb := &agenteditor.KnowledgeBase{BaseElement: model.BaseElement{ID: "kb-1"}, ContainerID: mod.ID, Name: "KB_WorkInstructions"}
		a := storedGraphExplorer(mod, mf, kb)
		a.SelectedModel = stored
		h := mkHierarchy(mod)
		withContainer(h, mf.ContainerID, mod.ID)
		var written *agenteditor.Agent
		mb := &mock.MockBackend{
			IsConnectedFunc:                   func() bool { return true },
			ListModulesFunc:                   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
			ListMicroflowsFunc:                func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
			ListAgentEditorAgentsFunc:         func() ([]*agenteditor.Agent, error) { return []*agenteditor.Agent{a}, nil },
			ListAgentEditorKnowledgeBasesFunc: func() ([]*agenteditor.KnowledgeBase, error) { return []*agenteditor.KnowledgeBase{kb}, nil },
			UpdateAgentEditorAgentFunc: func(ag *agenteditor.Agent) error {
				written = ag
				return nil
			},
		}
		ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
		prog := parseMDL(t, src)
		assertNoError(t, execCreateAgent(ctx, prog.Statements[0].(*ast.CreateAgentStmt)))
		if written == nil {
			t.Fatal("the agent was not written")
		}
		return written
	}
	sonnet := &agenteditor.SelectedModel{ID: "eu.anthropic.claude-sonnet-5", Name: "Anthropic Claude Sonnet 5"}

	t.Run("stated", func(t *testing.T) {
		w := run(t, `create or modify agent M.GraphExplorer (UsageType: Chat, SelectedModel: 'eu.anthropic.claude-haiku-5')`, sonnet)
		// A new id with no name takes the id as its name: the stored display
		// name belongs to the old model.
		if w.SelectedModel == nil || *w.SelectedModel != (agenteditor.SelectedModel{ID: "eu.anthropic.claude-haiku-5", Name: "eu.anthropic.claude-haiku-5"}) {
			t.Errorf("selected model %+v", w.SelectedModel)
		}
	})
	t.Run("stated with name", func(t *testing.T) {
		w := run(t, `create or modify agent M.GraphExplorer (UsageType: Chat, SelectedModel: 'x', SelectedModelName: 'Model X')`, nil)
		if w.SelectedModel == nil || *w.SelectedModel != (agenteditor.SelectedModel{ID: "x", Name: "Model X"}) {
			t.Errorf("selected model %+v", w.SelectedModel)
		}
	})
	t.Run("same id keeps the stored name", func(t *testing.T) {
		w := run(t, `create or modify agent M.GraphExplorer (UsageType: Chat, SelectedModel: 'eu.anthropic.claude-sonnet-5')`, sonnet)
		if w.SelectedModel == nil || *w.SelectedModel != *sonnet {
			t.Errorf("selected model %+v, want the stored %+v", w.SelectedModel, sonnet)
		}
	})
	t.Run("left out keeps the stored one", func(t *testing.T) {
		w := run(t, `create or modify agent M.GraphExplorer (UsageType: Chat)`, sonnet)
		if w.SelectedModel == nil || *w.SelectedModel != *sonnet {
			t.Errorf("selected model %+v, want the stored %+v", w.SelectedModel, sonnet)
		}
	})
	t.Run("empty clears it", func(t *testing.T) {
		w := run(t, `create or modify agent M.GraphExplorer (UsageType: Chat, SelectedModel: '')`, sonnet)
		if w.SelectedModel != nil {
			t.Errorf("selected model %+v, want none", w.SelectedModel)
		}
	})
}

func TestAlterAgent_SelectedModel(t *testing.T) {
	mod := mkModule("M")
	a := &agenteditor.Agent{BaseElement: model.BaseElement{ID: "agent-1"}, ContainerID: mod.ID, Name: "A", UsageType: "Task",
		SelectedModel: &agenteditor.SelectedModel{ID: "old", Name: "Old Model"}}
	h := mkHierarchy(mod)
	withContainer(h, a.ContainerID, mod.ID)
	var updated *agenteditor.Agent
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListAgentEditorAgentsFunc: func() ([]*agenteditor.Agent, error) { return []*agenteditor.Agent{a}, nil },
		UpdateAgentEditorAgentFunc: func(ag *agenteditor.Agent) error {
			updated = ag
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	// Both in one statement: the name must survive whichever key is applied first.
	assertNoError(t, execAlterAgent(ctx, &ast.AlterAgentStmt{Name: ast.QualifiedName{Module: "M", Name: "A"},
		Sets: map[string]string{"SelectedModelName": "New Model", "SelectedModel": "new"}}))
	if updated == nil || updated.SelectedModel == nil || *updated.SelectedModel != (agenteditor.SelectedModel{ID: "new", Name: "New Model"}) {
		t.Fatalf("selected model %+v", updated.SelectedModel)
	}
	ctx, _ = newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterAgent(ctx, &ast.AlterAgentStmt{Name: ast.QualifiedName{Module: "M", Name: "A"},
		Sets: map[string]string{"SelectedModel": ""}}))
	if updated.SelectedModel != nil {
		t.Errorf("SelectedModel = '' left %+v", updated.SelectedModel)
	}
}

// describe prints the selected model, the name only when it is not the id, and
// what it prints reads back as the stored value.
func TestDescribeAgent_SelectedModel(t *testing.T) {
	mod := mkModule("M")
	a := &agenteditor.Agent{BaseElement: model.BaseElement{ID: "agent-1"}, ContainerID: mod.ID, Name: "A", UsageType: "Task",
		SelectedModel: &agenteditor.SelectedModel{ID: "eu.anthropic.claude-sonnet-5", Name: "Anthropic Claude Sonnet 5"}}
	h := mkHierarchy(mod)
	withContainer(h, a.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListAgentEditorAgentsFunc: func() ([]*agenteditor.Agent, error) { return []*agenteditor.Agent{a}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeAgentEditorAgent(ctx, ast.QualifiedName{Module: "M", Name: "A"}))
	s := parseMDL(t, buf.String()).Statements[0].(*ast.CreateAgentStmt)
	if s.SelectedModel == nil || *s.SelectedModel != "eu.anthropic.claude-sonnet-5" ||
		s.SelectedModelName == nil || *s.SelectedModelName != "Anthropic Claude Sonnet 5" {
		t.Errorf("describe does not read back as the stored selected model:\n%s", buf.String())
	}
	// A name equal to the id is not repeated.
	a.SelectedModel.Name = a.SelectedModel.ID
	buf.Reset()
	assertNoError(t, describeAgentEditorAgent(ctx, ast.QualifiedName{Module: "M", Name: "A"}))
	if strings.Contains(buf.String(), "SelectedModelName") || !strings.Contains(buf.String(), "SelectedModel: 'eu.anthropic.claude-sonnet-5'") {
		t.Errorf("name equal to id:\n%s", buf.String())
	}
	// Control: an agent without one prints none.
	a.SelectedModel = nil
	buf.Reset()
	assertNoError(t, describeAgentEditorAgent(ctx, ast.QualifiedName{Module: "M", Name: "A"}))
	if strings.Contains(buf.String(), "SelectedModel") {
		t.Errorf("an agent without a selected model prints one:\n%s", buf.String())
	}
}
