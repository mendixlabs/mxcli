// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestAlterModel_SetMultiple(t *testing.T) {
	prog, errs := Build(`ALTER MODEL MyModule.GPT4 SET DisplayName = 'GPT-4 Turbo', KeyName = 'OPENAI_KEY';`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.AlterModelStmt)
	if !ok {
		t.Fatalf("Expected AlterModelStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Module != "MyModule" || stmt.Name.Name != "GPT4" {
		t.Errorf("name = %s.%s", stmt.Name.Module, stmt.Name.Name)
	}
	if stmt.Changes["displayname"] != "GPT-4 Turbo" {
		t.Errorf("DisplayName = %q", stmt.Changes["displayname"])
	}
	if stmt.Changes["keyname"] != "OPENAI_KEY" {
		t.Errorf("KeyName = %q", stmt.Changes["keyname"])
	}
}

func TestAlterKnowledgeBase_SetProvider(t *testing.T) {
	prog, errs := Build(`ALTER KNOWLEDGE BASE MyModule.Docs SET Provider = MxCloudGenAI;`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.AlterKnowledgeBaseStmt)
	if !ok {
		t.Fatalf("Expected AlterKnowledgeBaseStmt, got %T", prog.Statements[0])
	}
	if stmt.Changes["provider"] != "MxCloudGenAI" {
		t.Errorf("Provider = %q", stmt.Changes["provider"])
	}
}

func TestAlterConsumedMCPService_SetTimeout(t *testing.T) {
	prog, errs := Build(`ALTER CONSUMED MCP SERVICE MyModule.Weather SET ConnectionTimeoutSeconds = 60, Version = '1.0.1';`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.AlterConsumedMCPServiceStmt)
	if !ok {
		t.Fatalf("Expected AlterConsumedMCPServiceStmt, got %T", prog.Statements[0])
	}
	if stmt.Changes["connectiontimeoutseconds"] != "60" {
		t.Errorf("ConnectionTimeoutSeconds = %q", stmt.Changes["connectiontimeoutseconds"])
	}
	if stmt.Changes["version"] != "1.0.1" {
		t.Errorf("Version = %q", stmt.Changes["version"])
	}
}

func TestAlterAgent_SetOnly(t *testing.T) {
	prog, errs := Build(`ALTER AGENT MyModule.Helper
		SET SystemPrompt = 'New prompt', Temperature = 0.5, MaxTokens = 8192;`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.AlterAgentStmt)
	if !ok {
		t.Fatalf("Expected AlterAgentStmt, got %T", prog.Statements[0])
	}
	if stmt.Sets["systemprompt"] != "New prompt" {
		t.Errorf("SystemPrompt = %q", stmt.Sets["systemprompt"])
	}
	if stmt.Sets["temperature"] != "0.5" {
		t.Errorf("Temperature = %q", stmt.Sets["temperature"])
	}
	if stmt.Sets["maxtokens"] != "8192" {
		t.Errorf("MaxTokens = %q", stmt.Sets["maxtokens"])
	}
}

func TestAlterAgent_AddAndDrop(t *testing.T) {
	prog, errs := Build(`ALTER AGENT MyModule.Helper
		SET Description = 'Assistant'
		ADD TOOL DoSomething { Description: 'd', Enabled: true }
		ADD MCP SERVICE MyModule.Weather { Description: 'w', Enabled: true }
		ADD KNOWLEDGE BASE Docs { Source: MyModule.MyKB, Collection: 'corp', MaxResults: 5 }
		DROP TOOL OldTool
		DROP MCP SERVICE MyModule.OldSvc
		DROP KNOWLEDGE BASE OldKB;`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.AlterAgentStmt)
	if !ok {
		t.Fatalf("Expected AlterAgentStmt, got %T", prog.Statements[0])
	}
	if stmt.Sets["description"] != "Assistant" {
		t.Errorf("Description = %q", stmt.Sets["description"])
	}
	if len(stmt.AddTools) != 2 {
		t.Fatalf("expected 2 AddTools (TOOL + MCP SERVICE), got %d", len(stmt.AddTools))
	}
	if stmt.AddTools[0].ToolType != "Microflow" || stmt.AddTools[0].Name != "DoSomething" {
		t.Errorf("AddTools[0] = %+v", stmt.AddTools[0])
	}
	if stmt.AddTools[1].ToolType != "MCP" || stmt.AddTools[1].Document == nil || stmt.AddTools[1].Document.Name != "Weather" {
		t.Errorf("AddTools[1] = %+v", stmt.AddTools[1])
	}
	if len(stmt.AddKBs) != 1 || stmt.AddKBs[0].Name != "Docs" || stmt.AddKBs[0].MaxResults != 5 {
		t.Errorf("AddKBs = %+v", stmt.AddKBs)
	}
	if len(stmt.DropTools) != 1 || stmt.DropTools[0] != "OldTool" {
		t.Errorf("DropTools = %v", stmt.DropTools)
	}
	if len(stmt.DropMCPServices) != 1 || stmt.DropMCPServices[0].Name != "OldSvc" {
		t.Errorf("DropMCPServices = %v", stmt.DropMCPServices)
	}
	if len(stmt.DropKBs) != 1 || stmt.DropKBs[0] != "OldKB" {
		t.Errorf("DropKBs = %v", stmt.DropKBs)
	}
}

// Regression test for issue #464.
func TestAlterAgent_Issue464Repro(t *testing.T) {
	prog, errs := Build(`CREATE AGENT MyModule.VerifyAgent (SystemPrompt: 'test agent');
ALTER AGENT MyModule.VerifyAgent SET SystemPrompt = 'updated';`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	if len(prog.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Statements))
	}
	if _, ok := prog.Statements[1].(*ast.AlterAgentStmt); !ok {
		t.Fatalf("expected AlterAgentStmt second, got %T", prog.Statements[1])
	}
}
