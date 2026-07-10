// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// =============================================================================
// Java Action Type Parameters — Parser/Visitor Tests
// =============================================================================

func TestJavaAction_BasicParsing(t *testing.T) {
	input := `CREATE JAVA ACTION MyModule.DoSomething(
  Name: String NOT NULL
) RETURNS Boolean
AS $$
return true;
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.CreateJavaActionStmt)
	if !ok {
		t.Fatalf("Expected CreateJavaActionStmt, got %T", prog.Statements[0])
	}

	if stmt.Name.Module != "MyModule" || stmt.Name.Name != "DoSomething" {
		t.Errorf("Expected MyModule.DoSomething, got %s.%s", stmt.Name.Module, stmt.Name.Name)
	}
	if len(stmt.Parameters) != 1 {
		t.Fatalf("Expected 1 parameter, got %d", len(stmt.Parameters))
	}
	if stmt.Parameters[0].Name != "Name" {
		t.Errorf("Expected param name 'Name', got '%s'", stmt.Parameters[0].Name)
	}
	if !stmt.Parameters[0].IsRequired {
		t.Error("Expected param to be NOT NULL")
	}
	if stmt.ReturnType.Kind != ast.TypeBoolean {
		t.Errorf("Expected Boolean return type, got %d", stmt.ReturnType.Kind)
	}
	if stmt.JavaCode != "return true;" {
		t.Errorf("Expected 'return true;', got '%s'", stmt.JavaCode)
	}
}

func TestJavaAction_SingleTypeParameter(t *testing.T) {
	input := `CREATE JAVA ACTION MyModule.Validate(
  EntityType: ENTITY <pEntity> NOT NULL,
  InputObject: pEntity NOT NULL
) RETURNS Boolean
AS $$
return true;
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt, ok := prog.Statements[0].(*ast.CreateJavaActionStmt)
	if !ok {
		t.Fatalf("Expected CreateJavaActionStmt, got %T", prog.Statements[0])
	}

	// Verify type parameters extracted from ENTITY <pEntity> declaration
	if len(stmt.TypeParameters) != 1 {
		t.Fatalf("Expected 1 type parameter, got %d", len(stmt.TypeParameters))
	}
	if stmt.TypeParameters[0] != "pEntity" {
		t.Errorf("Expected type parameter 'pEntity', got '%s'", stmt.TypeParameters[0])
	}

	// Verify parameters
	if len(stmt.Parameters) != 2 {
		t.Fatalf("Expected 2 parameters, got %d", len(stmt.Parameters))
	}

	// First param: ENTITY <pEntity> — entity type selector
	param0 := stmt.Parameters[0]
	if param0.Name != "EntityType" {
		t.Errorf("Expected param name 'EntityType', got '%s'", param0.Name)
	}
	if param0.Type.Kind != ast.TypeEntityTypeParam {
		t.Errorf("Expected TypeEntityTypeParam, got %v", param0.Type.Kind)
	}
	if param0.Type.TypeParamName != "pEntity" {
		t.Errorf("Expected TypeParamName 'pEntity', got '%s'", param0.Type.TypeParamName)
	}

	// Second param: pEntity — bare name reference
	param1 := stmt.Parameters[1]
	if param1.Name != "InputObject" {
		t.Errorf("Expected param name 'InputObject', got '%s'", param1.Name)
	}
	if param1.Type.EnumRef == nil {
		t.Fatal("Expected EnumRef to be set for bare type parameter name")
	}
	if param1.Type.EnumRef.Name != "pEntity" {
		t.Errorf("Expected type ref 'pEntity', got '%s'", param1.Type.EnumRef.Name)
	}
}

func TestJavaAction_MultipleTypeParameters(t *testing.T) {
	input := `CREATE JAVA ACTION MyModule.Transform(
  SourceType: ENTITY <pSource> NOT NULL,
  TargetType: ENTITY <pTarget> NOT NULL,
  Source: pSource NOT NULL,
  Target: pTarget NOT NULL
) RETURNS Boolean
AS $$
return true;
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateJavaActionStmt)

	if len(stmt.TypeParameters) != 2 {
		t.Fatalf("Expected 2 type parameters, got %d", len(stmt.TypeParameters))
	}
	if stmt.TypeParameters[0] != "pSource" {
		t.Errorf("Expected 'pSource', got '%s'", stmt.TypeParameters[0])
	}
	if stmt.TypeParameters[1] != "pTarget" {
		t.Errorf("Expected 'pTarget', got '%s'", stmt.TypeParameters[1])
	}

	if len(stmt.Parameters) != 4 {
		t.Fatalf("Expected 4 parameters, got %d", len(stmt.Parameters))
	}
	// ENTITY <pSource> params
	if stmt.Parameters[0].Type.Kind != ast.TypeEntityTypeParam {
		t.Error("Expected first param to be TypeEntityTypeParam")
	}
	if stmt.Parameters[1].Type.Kind != ast.TypeEntityTypeParam {
		t.Error("Expected second param to be TypeEntityTypeParam")
	}
	// Bare name reference params
	if stmt.Parameters[2].Name != "Source" {
		t.Errorf("Expected 'Source', got '%s'", stmt.Parameters[2].Name)
	}
	if stmt.Parameters[3].Name != "Target" {
		t.Errorf("Expected 'Target', got '%s'", stmt.Parameters[3].Name)
	}
}

func TestJavaAction_ExposedAsClause(t *testing.T) {
	input := `CREATE JAVA ACTION MyModule.FormatCurrency(
  Amount: Decimal NOT NULL
) RETURNS String
EXPOSED AS 'Format Currency' IN 'Formatting'
AS $$
return "formatted";
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateJavaActionStmt)

	if stmt.ExposedCaption != "Format Currency" {
		t.Errorf("Expected exposed caption 'Format Currency', got '%s'", stmt.ExposedCaption)
	}
	if stmt.ExposedCategory != "Formatting" {
		t.Errorf("Expected exposed category 'Formatting', got '%s'", stmt.ExposedCategory)
	}
}

func TestJavaAction_TypeParamAndExposedCombined(t *testing.T) {
	input := `CREATE JAVA ACTION MyModule.DeepClone(
  EntityType: ENTITY <pEntity> NOT NULL,
  Original: pEntity NOT NULL
) RETURNS Boolean
EXPOSED AS 'Deep Clone' IN 'Object Utils'
AS $$
return true;
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateJavaActionStmt)

	// Type parameter extracted from ENTITY <pEntity>
	if len(stmt.TypeParameters) != 1 || stmt.TypeParameters[0] != "pEntity" {
		t.Errorf("Expected type parameter 'pEntity', got %v", stmt.TypeParameters)
	}

	// Exposed
	if stmt.ExposedCaption != "Deep Clone" {
		t.Errorf("Expected 'Deep Clone', got '%s'", stmt.ExposedCaption)
	}
	if stmt.ExposedCategory != "Object Utils" {
		t.Errorf("Expected 'Object Utils', got '%s'", stmt.ExposedCategory)
	}

	// Parameters
	if len(stmt.Parameters) != 2 {
		t.Fatalf("Expected 2 parameters, got %d", len(stmt.Parameters))
	}
	if stmt.Parameters[0].Name != "EntityType" {
		t.Errorf("Expected 'EntityType', got '%s'", stmt.Parameters[0].Name)
	}
	if stmt.Parameters[0].Type.Kind != ast.TypeEntityTypeParam {
		t.Error("Expected first param to be TypeEntityTypeParam")
	}
	if stmt.Parameters[1].Name != "Original" {
		t.Errorf("Expected 'Original', got '%s'", stmt.Parameters[1].Name)
	}
	if !stmt.Parameters[1].IsRequired {
		t.Error("Expected NOT NULL")
	}
}

func TestJavaAction_NoTypeParams_NoExposed(t *testing.T) {
	// Verify backward compatibility — existing syntax without new features
	input := `CREATE JAVA ACTION MyModule.SimpleAction(
  Value: String NOT NULL
) RETURNS Boolean
AS $$
return true;
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateJavaActionStmt)

	if len(stmt.TypeParameters) != 0 {
		t.Errorf("Expected no type parameters, got %d", len(stmt.TypeParameters))
	}
	if stmt.ExposedCaption != "" {
		t.Errorf("Expected no exposed caption, got '%s'", stmt.ExposedCaption)
	}
	if stmt.ExposedCategory != "" {
		t.Errorf("Expected no exposed category, got '%s'", stmt.ExposedCategory)
	}
}

func TestJavaAction_VoidReturnType(t *testing.T) {
	input := `CREATE JAVA ACTION MyModule.DoStuff()
AS $$
System.out.println("done");
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateJavaActionStmt)

	if len(stmt.Parameters) != 0 {
		t.Errorf("Expected no parameters, got %d", len(stmt.Parameters))
	}
	// No return type specified
	if stmt.ReturnType.Kind != 0 {
		t.Errorf("Expected zero-value return type, got %d", stmt.ReturnType.Kind)
	}
}

func TestJavaAction_ExplicitVoidReturnType(t *testing.T) {
	input := `CREATE JAVA ACTION MyModule.DoStuff()
RETURNS Void
AS $$
System.out.println("done");
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateJavaActionStmt)
	if stmt.ReturnType.Kind != ast.TypeVoid {
		t.Fatalf("ReturnType.Kind = %v, want TypeVoid", stmt.ReturnType.Kind)
	}
}

func TestJavaAction_TypeParamWithMixedParamTypes(t *testing.T) {
	// Mix ENTITY <pEntity> declaration, bare type param ref, and regular typed params
	input := `CREATE JAVA ACTION MyModule.ProcessEntity(
  EntityType: ENTITY <pEntity> NOT NULL,
  InputObject: pEntity NOT NULL,
  Label: String NOT NULL,
  Count: Integer
) RETURNS Boolean
AS $$
return true;
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateJavaActionStmt)

	if len(stmt.TypeParameters) != 1 {
		t.Fatalf("Expected 1 type parameter, got %d", len(stmt.TypeParameters))
	}

	if len(stmt.Parameters) != 4 {
		t.Fatalf("Expected 4 parameters, got %d", len(stmt.Parameters))
	}

	// First param: ENTITY <pEntity> type parameter declaration
	if stmt.Parameters[0].Type.Kind != ast.TypeEntityTypeParam {
		t.Errorf("Expected first param to be TypeEntityTypeParam, got %v", stmt.Parameters[0].Type.Kind)
	}
	if stmt.Parameters[0].Type.TypeParamName != "pEntity" {
		t.Errorf("Expected TypeParamName 'pEntity', got '%s'", stmt.Parameters[0].Type.TypeParamName)
	}

	// Second param: bare type parameter ref
	if stmt.Parameters[1].Type.EnumRef == nil || stmt.Parameters[1].Type.EnumRef.Name != "pEntity" {
		t.Error("Expected second param to reference type parameter 'pEntity'")
	}

	// Third param: regular String type
	if stmt.Parameters[2].Type.Kind != ast.TypeString {
		t.Errorf("Expected third param to be String, got %d", stmt.Parameters[2].Type.Kind)
	}

	// Fourth param: regular Integer type
	if stmt.Parameters[3].Type.Kind != ast.TypeInteger {
		t.Errorf("Expected fourth param to be Integer, got %d", stmt.Parameters[3].Type.Kind)
	}
}

func TestJavaAction_TypeParamWithEntityTypeSelector(t *testing.T) {
	// Tests the new explicit ENTITY <pEntity> syntax for entity type selector.
	input := `CREATE JAVA ACTION MyModule.CopyAttributes(
  EntityType: ENTITY <pEntity> NOT NULL,
  Source: pEntity NOT NULL,
  Target: pEntity NOT NULL,
  AttributeNames: String NOT NULL
) RETURNS Boolean
AS $$
return true;
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateJavaActionStmt)

	if len(stmt.TypeParameters) != 1 || stmt.TypeParameters[0] != "pEntity" {
		t.Errorf("Expected type parameter 'pEntity', got %v", stmt.TypeParameters)
	}

	if len(stmt.Parameters) != 4 {
		t.Fatalf("Expected 4 parameters, got %d", len(stmt.Parameters))
	}

	// First param: ENTITY <pEntity> — entity type selector
	if stmt.Parameters[0].Type.Kind != ast.TypeEntityTypeParam {
		t.Errorf("Expected first param to be TypeEntityTypeParam, got %v", stmt.Parameters[0].Type.Kind)
	}
	if stmt.Parameters[0].Type.TypeParamName != "pEntity" {
		t.Errorf("Expected TypeParamName 'pEntity', got '%s'", stmt.Parameters[0].Type.TypeParamName)
	}

	// Second and third params: bare pEntity refs
	for _, idx := range []int{1, 2} {
		param := stmt.Parameters[idx]
		if param.Type.EnumRef == nil || param.Type.EnumRef.Name != "pEntity" {
			t.Errorf("Expected param %d (%s) to reference type parameter 'pEntity'", idx, param.Name)
		}
	}

	// Fourth param should be String
	if stmt.Parameters[3].Type.Kind != ast.TypeString {
		t.Errorf("Expected fourth param to be String, got %d", stmt.Parameters[3].Type.Kind)
	}

	// All params should be NOT NULL
	for _, idx := range []int{0, 1, 2, 3} {
		if !stmt.Parameters[idx].IsRequired {
			t.Errorf("Expected param %d (%s) to be NOT NULL", idx, stmt.Parameters[idx].Name)
		}
	}
}

func TestJavaAction_ExposedWithSpecialChars(t *testing.T) {
	// Test that special characters in exposed strings are handled
	input := `CREATE JAVA ACTION MyModule.SendNotification(
  Message: String NOT NULL
) RETURNS Boolean
EXPOSED AS 'Send E-Mail Notification' IN 'Communication & Alerts'
AS $$
return true;
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreateJavaActionStmt)

	if stmt.ExposedCaption != "Send E-Mail Notification" {
		t.Errorf("Expected 'Send E-Mail Notification', got '%s'", stmt.ExposedCaption)
	}
	if stmt.ExposedCategory != "Communication & Alerts" {
		t.Errorf("Expected 'Communication & Alerts', got '%s'", stmt.ExposedCategory)
	}
}

// =============================================================================
// Import extraction from $$ block (issue #127)
// =============================================================================

func TestJavaAction_ImportsExtractedFromDollarBlock(t *testing.T) {
	input := `CREATE JAVA ACTION MyModule.ReadCSV(File: System.FileDocument) RETURNS String
AS $$
import com.mendix.core.Core;
import java.io.*;
import java.io.BufferedReader;
IContext context = this.getContext();
InputStream is = Core.getFileDocumentContent(context, File.getMendixObject());
BufferedReader reader = new BufferedReader(new InputStreamReader(is, "UTF-8"));
return reader.readLine();
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.CreateJavaActionStmt)
	if !ok {
		t.Fatalf("Expected CreateJavaActionStmt")
	}

	if len(stmt.Imports) != 3 {
		t.Errorf("Expected 3 imports, got %d: %v", len(stmt.Imports), stmt.Imports)
	}
	for _, imp := range stmt.Imports {
		if !isJavaImportLine(imp) {
			t.Errorf("import line should start with 'import ': %q", imp)
		}
	}

	// import lines must not appear in the method body
	for _, line := range splitLines(stmt.JavaCode) {
		if isJavaImportLine(line) {
			t.Errorf("import line leaked into JavaCode body: %q", line)
		}
	}

	// actual code must be preserved
	if stmt.JavaCode == "" {
		t.Error("JavaCode should not be empty after import extraction")
	}
}

func TestJavaAction_NoImportsInDollarBlock(t *testing.T) {
	input := `CREATE JAVA ACTION MyModule.Simple() RETURNS Boolean
AS $$
return true;
$$;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateJavaActionStmt)
	if len(stmt.Imports) != 0 {
		t.Errorf("Expected no imports, got %v", stmt.Imports)
	}
	if stmt.JavaCode != "return true;" {
		t.Errorf("Unexpected JavaCode: %q", stmt.JavaCode)
	}
}

func isJavaImportLine(s string) bool {
	return len(s) > 7 && s[:7] == "import "
}

func splitLines(s string) []string {
	start := 0
	var lines []string
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func TestJavaAction_OrModify(t *testing.T) {
	prog, errs := Build("CREATE OR MODIFY JAVA ACTION MyModule.DoSomething() RETURNS Boolean AS $$ return true; $$;")
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateJavaActionStmt)
	if !ok {
		t.Fatalf("Expected CreateJavaActionStmt, got %T", prog.Statements[0])
	}
	if !stmt.CreateOrModify {
		t.Error("Expected CreateOrModify=true")
	}
}
