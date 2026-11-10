// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2021, #2022: the Agent Editor settings stored on a tool, a
// knowledge base tool and the agent, keyed by their document property names.
func TestAgentSettings_Parse(t *testing.T) {
	prog := mustBuild(t, `create or modify agent M.A (
  UsageType: Chat,
  EnableBackendStreaming: true,
  EnableUIStreaming: false
)
{
  tool T (Document: M.Flow, UserAccessApproval: userconfirmationrequired, DisplayTitle: 'Look it up', DisplayDescription: '')
  mcp service M.Svc (Enabled: true, UserAccessApproval: HiddenForUser)
  knowledge base K (Source: M.KB, MinSimilarity: 0.2, IsVisible: false)
};`)
	s := prog.Statements[0].(*ast.CreateAgentStmt)
	if s.EnableBackendStreaming == nil || !*s.EnableBackendStreaming || s.EnableUIStreaming == nil || *s.EnableUIStreaming {
		t.Errorf("streaming %v %v, want true and false", s.EnableBackendStreaming, s.EnableUIStreaming)
	}
	tool := s.Tools[0]
	if tool.UserAccessApproval != "UserConfirmationRequired" || tool.DisplayTitle == nil || *tool.DisplayTitle != "Look it up" ||
		tool.DisplayDescription == nil || *tool.DisplayDescription != "" {
		t.Errorf("tool %+v", tool)
	}
	if s.Tools[1].UserAccessApproval != "HiddenForUser" {
		t.Errorf("mcp service tool %+v", s.Tools[1])
	}
	kb := s.KBTools[0]
	if kb.MinSimilarity == nil || *kb.MinSimilarity != 0.2 || kb.IsVisible == nil || *kb.IsVisible {
		t.Errorf("knowledge base tool %+v", kb)
	}
}

// Control: a statement that names none of them says nothing about them, so a
// create or modify keeps the stored values.
func TestAgentSettings_LeftOut(t *testing.T) {
	prog := mustBuild(t, `create or modify agent M.A (UsageType: Chat)
{
  tool T (Document: M.Flow)
  knowledge base K (Source: M.KB)
};`)
	s := prog.Statements[0].(*ast.CreateAgentStmt)
	if s.EnableBackendStreaming != nil || s.EnableUIStreaming != nil || s.Tools[0].UserAccessApproval != "" ||
		s.Tools[0].DisplayTitle != nil || s.KBTools[0].MinSimilarity != nil || s.KBTools[0].IsVisible != nil {
		t.Errorf("unstated settings were set: %+v", s)
	}
}

func TestAgentSettings_UnknownApprovalRefused(t *testing.T) {
	_, errs := Build(`create agent M.A (UsageType: Chat) { tool T (UserAccessApproval: Ask) };`)
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "UserAccessApproval takes HiddenForUser, VisibleForUser, UserConfirmationRequired") {
		t.Errorf("errors = %v", errs)
	}
}

// alter agent … add takes the same keys.
func TestAgentSettings_AlterAdd(t *testing.T) {
	prog := mustBuild(t, `alter agent M.A
  add tool T (Document: M.Flow, UserAccessApproval: VisibleForUser)
  add knowledge base K (Source: M.KB, MinSimilarity: 0.5);`)
	s := prog.Statements[0].(*ast.AlterAgentStmt)
	if s.AddTools[0].UserAccessApproval != "VisibleForUser" || s.AddKBs[0].MinSimilarity == nil || *s.AddKBs[0].MinSimilarity != 0.5 {
		t.Errorf("alter add %+v %+v", s.AddTools[0], s.AddKBs[0])
	}
}

// ako/mxcli#2031: the model an agent uses on a multi-model resource.
func TestAgentSettings_SelectedModel(t *testing.T) {
	prog := mustBuild(t, `create agent M.A (
  UsageType: Task,
  Model: M.Model,
  SelectedModel: 'eu.anthropic.claude-sonnet-5',
  SelectedModelName: 'Anthropic Claude Sonnet 5'
);
create or modify agent M.B (UsageType: Task);
alter agent M.A set SelectedModel = 'eu.anthropic.claude-haiku-5';`)
	a := prog.Statements[0].(*ast.CreateAgentStmt)
	if a.SelectedModel == nil || *a.SelectedModel != "eu.anthropic.claude-sonnet-5" ||
		a.SelectedModelName == nil || *a.SelectedModelName != "Anthropic Claude Sonnet 5" {
		t.Errorf("selected model %v / %v", a.SelectedModel, a.SelectedModelName)
	}
	// Control: left out, nothing is said, so a create or modify keeps the stored value.
	if b := prog.Statements[1].(*ast.CreateAgentStmt); b.SelectedModel != nil || b.SelectedModelName != nil {
		t.Errorf("unstated selected model set: %v %v", b.SelectedModel, b.SelectedModelName)
	}
	if alt := prog.Statements[2].(*ast.AlterAgentStmt); alt.Sets["selectedmodel"] != "eu.anthropic.claude-haiku-5" {
		t.Errorf("alter set SelectedModel: %+v", alt.Sets)
	}
}
