// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCreateDataTransformer(t *testing.T) {
	input := `CREATE DATA TRANSFORMER MyModule.LatExtract SOURCE JSON '{"lat": 51.9}' {
		JSLT '{ "latitude": .lat }';
	};`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateDataTransformerStmt)
	if !ok {
		t.Fatalf("Expected CreateDataTransformerStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Name != "LatExtract" {
		t.Errorf("Got name %s", stmt.Name.Name)
	}
	if stmt.SourceType != "JSON" {
		t.Errorf("Got SourceType %q", stmt.SourceType)
	}
	if len(stmt.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(stmt.Steps))
	}
	if stmt.Steps[0].Technology != "JSLT" {
		t.Errorf("Got Technology %q", stmt.Steps[0].Technology)
	}
}

func TestCreateDataTransformer_OrModify(t *testing.T) {
	prog, errs := Build(`CREATE OR MODIFY DATA TRANSFORMER MyModule.LatExtract SOURCE JSON '{}' {};`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateDataTransformerStmt)
	if !ok {
		t.Fatalf("Expected CreateDataTransformerStmt, got %T", prog.Statements[0])
	}
	if !stmt.CreateOrModify {
		t.Error("Expected CreateOrModify=true")
	}
}
