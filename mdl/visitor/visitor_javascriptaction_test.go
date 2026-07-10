// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestJavaScriptAction_BasicParsing(t *testing.T) {
	input := `CREATE JAVASCRIPT ACTION MyModule.DoThing(
  Input: String NOT NULL,
  Count: Integer
) RETURNS Boolean
EXPOSED AS 'Do Thing' IN 'Demo'
PLATFORM Native
AS $$
return Promise.resolve(true);
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateJavaScriptActionStmt)
	if !ok {
		t.Fatalf("expected CreateJavaScriptActionStmt, got %T", prog.Statements[0])
	}

	if stmt.Name.Module != "MyModule" || stmt.Name.Name != "DoThing" {
		t.Errorf("name = %s.%s", stmt.Name.Module, stmt.Name.Name)
	}
	if len(stmt.Parameters) != 2 {
		t.Fatalf("expected 2 params, got %d", len(stmt.Parameters))
	}
	if stmt.Parameters[0].Name != "Input" || !stmt.Parameters[0].IsRequired {
		t.Errorf("param0 = %+v", stmt.Parameters[0])
	}
	if stmt.Parameters[1].Name != "Count" || stmt.Parameters[1].IsRequired {
		t.Errorf("param1 = %+v", stmt.Parameters[1])
	}
	if stmt.ExposedCaption != "Do Thing" || stmt.ExposedCategory != "Demo" {
		t.Errorf("exposed = %q / %q", stmt.ExposedCaption, stmt.ExposedCategory)
	}
	if stmt.Platform != "Native" {
		t.Errorf("platform = %q, want Native", stmt.Platform)
	}
	if stmt.JavaScriptCode != "return Promise.resolve(true);" {
		t.Errorf("code = %q", stmt.JavaScriptCode)
	}
}

func TestJavaScriptAction_DefaultPlatformWeb(t *testing.T) {
	input := `CREATE JAVASCRIPT ACTION M.A() RETURNS Boolean AS $$ return true; $$;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateJavaScriptActionStmt)
	if stmt.Platform != "Web" {
		t.Errorf("default platform = %q, want Web", stmt.Platform)
	}
}

func TestJavaScriptAction_PlatformCaseNormalized(t *testing.T) {
	input := `CREATE JAVASCRIPT ACTION M.A() RETURNS Boolean PLATFORM all AS $$ x $$;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateJavaScriptActionStmt)
	if stmt.Platform != "All" {
		t.Errorf("platform = %q, want All (normalized)", stmt.Platform)
	}
}

func TestDropJavaScriptAction_Parsing(t *testing.T) {
	prog, errs := Build(`DROP JAVASCRIPT ACTION M.A;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.DropJavaScriptActionStmt)
	if !ok {
		t.Fatalf("expected DropJavaScriptActionStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Module != "M" || stmt.Name.Name != "A" {
		t.Errorf("name = %s.%s", stmt.Name.Module, stmt.Name.Name)
	}
}

// Issue #680: explicit ENUM / Enumeration(...) types must be flagged as enums so
// the executor serializes them as enumeration params (not entity refs); a bare
// Module.Name must NOT be flagged (it can't be told apart from an entity).
func TestJavaAction_EnumParamFlaggedExplicit(t *testing.T) {
	input := `CREATE JAVA ACTION M.A(
  Fmt: Enumeration(Barcode.BarcodeFormat) NOT NULL,
  Node: ENUM M.LogNodes,
  Obj: M.SomeEntity
) RETURNS ENUM M.Status
AS $$ return null; $$;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateJavaActionStmt)
	if len(stmt.Parameters) != 3 {
		t.Fatalf("expected 3 params, got %d", len(stmt.Parameters))
	}
	// Enumeration(...) and ENUM ... → ExplicitEnum
	if !stmt.Parameters[0].Type.ExplicitEnum {
		t.Error("Enumeration(...) param should be ExplicitEnum")
	}
	if !stmt.Parameters[1].Type.ExplicitEnum {
		t.Error("ENUM param should be ExplicitEnum")
	}
	// bare Module.Name → NOT explicit (ambiguous with entity)
	if stmt.Parameters[2].Type.ExplicitEnum {
		t.Error("bare Module.Name param must NOT be ExplicitEnum")
	}
	if !stmt.ReturnType.ExplicitEnum {
		t.Error("ENUM return type should be ExplicitEnum")
	}
}
