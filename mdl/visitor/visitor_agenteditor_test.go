// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCreateModel(t *testing.T) {
	input := `CREATE MODEL MyModule.GPT4 (
		Provider: MxCloudGenAI,
		Key: MyModule.APIKey,
		DisplayName: 'GPT-4 Turbo'
	);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateModelStmt)
	if !ok {
		t.Fatalf("Expected CreateModelStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Module != "MyModule" || stmt.Name.Name != "GPT4" {
		t.Errorf("Expected MyModule.GPT4, got %s.%s", stmt.Name.Module, stmt.Name.Name)
	}
	if stmt.Provider != "MxCloudGenAI" {
		t.Errorf("Got Provider %q", stmt.Provider)
	}
	if stmt.Key == nil || stmt.Key.Name != "APIKey" {
		t.Error("Key mismatch")
	}
	if stmt.DisplayName != "GPT-4 Turbo" {
		t.Errorf("Got DisplayName %q", stmt.DisplayName)
	}
}

func TestCreateConsumedMCPService(t *testing.T) {
	input := `CREATE CONSUMED MCP SERVICE MyModule.ToolService (
		ProtocolVersion: v2025_03_26,
		Version: '0.0.1',
		ConnectionTimeoutSeconds: 30,
		Documentation: 'MCP tool service'
	);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateConsumedMCPServiceStmt)
	if !ok {
		t.Fatalf("Expected CreateConsumedMCPServiceStmt, got %T", prog.Statements[0])
	}
	if stmt.ProtocolVersion != "v2025_03_26" {
		t.Errorf("Got ProtocolVersion %q", stmt.ProtocolVersion)
	}
	if stmt.Version != "0.0.1" {
		t.Errorf("Got Version %q", stmt.Version)
	}
	if stmt.ConnectionTimeoutSeconds != 30 {
		t.Errorf("Got ConnectionTimeoutSeconds %d", stmt.ConnectionTimeoutSeconds)
	}
	if stmt.InnerDocumentation != "MCP tool service" {
		t.Errorf("Got InnerDocumentation %q", stmt.InnerDocumentation)
	}
}

func TestCreateKnowledgeBase(t *testing.T) {
	input := `CREATE KNOWLEDGE BASE MyModule.ProductKB (
		Provider: MxCloudGenAI,
		Key: MyModule.KBKey,
		ModelDisplayName: 'Product Knowledge',
		ModelName: 'product-embeddings'
	);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateKnowledgeBaseStmt)
	if !ok {
		t.Fatalf("Expected CreateKnowledgeBaseStmt, got %T", prog.Statements[0])
	}
	if stmt.Provider != "MxCloudGenAI" {
		t.Errorf("Got Provider %q", stmt.Provider)
	}
	if stmt.Key == nil || stmt.Key.Name != "KBKey" {
		t.Error("Key mismatch")
	}
	if stmt.ModelDisplayName != "Product Knowledge" {
		t.Errorf("Got ModelDisplayName %q", stmt.ModelDisplayName)
	}
}

func TestCreateAgent_Basic(t *testing.T) {
	input := `CREATE AGENT MyModule.OrderAgent (
		UsageType: Task,
		Model: MyModule.GPT4,
		Entity: MyModule.OrderContext,
		SystemPrompt: 'You are an order processing assistant.',
		ToolChoice: auto,
		MaxTokens: 4096,
		Temperature: 0.7
	);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateAgentStmt)
	if !ok {
		t.Fatalf("Expected CreateAgentStmt, got %T", prog.Statements[0])
	}
	if stmt.UsageType != "Task" {
		t.Errorf("Got UsageType %q", stmt.UsageType)
	}
	if stmt.Model == nil || stmt.Model.Name != "GPT4" {
		t.Error("Model mismatch")
	}
	if stmt.Entity == nil || stmt.Entity.Name != "OrderContext" {
		t.Error("Entity mismatch")
	}
	if stmt.MaxTokens == nil || *stmt.MaxTokens != 4096 {
		t.Error("MaxTokens mismatch")
	}
	if stmt.Temperature == nil || *stmt.Temperature != 0.7 {
		t.Error("Temperature mismatch")
	}
	if stmt.ToolChoice != "auto" {
		t.Errorf("Got ToolChoice %q", stmt.ToolChoice)
	}
}

func TestCreateModel_OrModify(t *testing.T) {
	prog, errs := Build(`CREATE OR MODIFY MODEL MyModule.GPT4 (Provider: MxCloudGenAI);`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateModelStmt)
	if !stmt.CreateOrModify {
		t.Error("Expected CreateOrModify=true")
	}
}

func TestCreateModel_OrReplace_MapsToModify(t *testing.T) {
	prog, errs := Build(`CREATE OR REPLACE MODEL MyModule.GPT4 (Provider: MxCloudGenAI);`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateModelStmt)
	if !stmt.CreateOrModify {
		t.Error("Expected CreateOrModify=true for OR REPLACE on model")
	}
}

func TestCreateConsumedMCPService_OrModify(t *testing.T) {
	prog, errs := Build(`CREATE OR MODIFY CONSUMED MCP SERVICE MyModule.WebSearch (ProtocolVersion: 'v2025_03_26', Version: '1.0');`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateConsumedMCPServiceStmt)
	if !stmt.CreateOrModify {
		t.Error("Expected CreateOrModify=true")
	}
}

func TestCreateKnowledgeBase_OrModify(t *testing.T) {
	prog, errs := Build(`CREATE OR MODIFY KNOWLEDGE BASE MyModule.Docs (Provider: MxCloudGenAI);`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateKnowledgeBaseStmt)
	if !stmt.CreateOrModify {
		t.Error("Expected CreateOrModify=true")
	}
}

func TestCreateAgent_OrModify(t *testing.T) {
	prog, errs := Build(`CREATE OR MODIFY AGENT MyModule.Assistant (UsageType: 'UserInitiated', Model: MyModule.GPT4);`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateAgentStmt)
	if !stmt.CreateOrModify {
		t.Error("Expected CreateOrModify=true")
	}
}
