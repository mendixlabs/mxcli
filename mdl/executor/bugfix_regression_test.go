// SPDX-License-Identifier: Apache-2.0

// Regression tests for GitHub Issues #18, #19, #23, #25, #26, #27, #28.
package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// =============================================================================
// Issue #18: DESCRIBE MICROFLOW emits END WHILE and traverses WHILE loop body
// =============================================================================

// TestLoopEndKeyword_WhileLoop verifies loopEndKeyword returns "END WHILE" for WHILE loops.
func TestLoopEndKeyword_WhileLoop(t *testing.T) {
	loop := &microflows.LoopedActivity{
		LoopSource: &microflows.WhileLoopCondition{WhileExpression: "$Counter < 10"},
	}
	got := loopEndKeyword(loop)
	if got != "END WHILE" {
		t.Errorf("loopEndKeyword(WhileLoop) = %q, want %q", got, "END WHILE")
	}
}

// TestLoopEndKeyword_ForEachLoop verifies loopEndKeyword returns "END LOOP" for FOR-EACH loops.
func TestLoopEndKeyword_ForEachLoop(t *testing.T) {
	loop := &microflows.LoopedActivity{
		LoopSource: &microflows.IterableList{
			VariableName:     "Item",
			ListVariableName: "Items",
		},
	}
	got := loopEndKeyword(loop)
	if got != "END LOOP" {
		t.Errorf("loopEndKeyword(ForEachLoop) = %q, want %q", got, "END LOOP")
	}
}

// TestLoopEndKeyword_NilSource verifies loopEndKeyword returns "END LOOP" when LoopSource is nil.
func TestLoopEndKeyword_NilSource(t *testing.T) {
	loop := &microflows.LoopedActivity{}
	got := loopEndKeyword(loop)
	if got != "END LOOP" {
		t.Errorf("loopEndKeyword(nil) = %q, want %q", got, "END LOOP")
	}
}

// TestFormatActivity_WhileLoop verifies WHILE loop header formatting.
func TestFormatActivity_WhileLoop(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.LoopedActivity{
		BaseMicroflowObject: mkObj("1"),
		LoopSource:          &microflows.WhileLoopCondition{WhileExpression: "$Counter <= $N"},
	}
	got := e.formatActivity(obj, nil, nil)
	if got != "WHILE $Counter <= $N" {
		t.Errorf("got %q, want %q", got, "WHILE $Counter <= $N")
	}
}

// =============================================================================
// Issue #19: Long type must not be downgraded to Integer
// =============================================================================

// TestConvertASTToMicroflowDataType_Long verifies Long maps to LongType, not IntegerType.
func TestConvertASTToMicroflowDataType_Long(t *testing.T) {
	dt := ast.DataType{Kind: ast.TypeLong}
	result := convertASTToMicroflowDataType(dt, nil)
	if _, ok := result.(*microflows.LongType); !ok {
		t.Errorf("expected *microflows.LongType, got %T", result)
	}
}

// TestConvertASTToMicroflowDataType_Integer verifies Integer maps to IntegerType (not affected by Long fix).
func TestConvertASTToMicroflowDataType_Integer(t *testing.T) {
	dt := ast.DataType{Kind: ast.TypeInteger}
	result := convertASTToMicroflowDataType(dt, nil)
	if _, ok := result.(*microflows.IntegerType); !ok {
		t.Errorf("expected *microflows.IntegerType, got %T", result)
	}
}

// TestLongType_GetTypeName verifies LongType.GetTypeName() returns "Long".
func TestLongType_GetTypeName(t *testing.T) {
	lt := &microflows.LongType{}
	if got := lt.GetTypeName(); got != "Long" {
		t.Errorf("LongType.GetTypeName() = %q, want %q", got, "Long")
	}
}

// =============================================================================
// Issue #25: DESCRIBE CONSTANT emits COMMENT field
// =============================================================================

// TestOutputConstantMDL_WithComment verifies DESCRIBE CONSTANT includes COMMENT clause.
func TestOutputConstantMDL_WithComment(t *testing.T) {
	buf := &bytes.Buffer{}
	e := New(buf)
	c := &model.Constant{
		Name:          "MaxRetries",
		Type:          model.ConstantDataType{Kind: "Integer"},
		DefaultValue:  "3",
		Documentation: "Maximum retry attempts",
	}
	if err := e.outputConstantMDL(c, "MyModule"); err != nil {
		t.Fatalf("outputConstantMDL: %v", err)
	}
	gotStr := buf.String()
	if !strings.Contains(gotStr, "COMMENT 'Maximum retry attempts'") {
		t.Errorf("expected COMMENT clause in output, got:\n%s", gotStr)
	}
}

// TestOutputConstantMDL_WithoutComment verifies DESCRIBE CONSTANT without COMMENT omits it.
func TestOutputConstantMDL_WithoutComment(t *testing.T) {
	buf := &bytes.Buffer{}
	e := New(buf)
	c := &model.Constant{
		Name:         "AppName",
		Type:         model.ConstantDataType{Kind: "String"},
		DefaultValue: "'MyApp'",
	}
	if err := e.outputConstantMDL(c, "MyModule"); err != nil {
		t.Fatalf("outputConstantMDL: %v", err)
	}
	gotStr := buf.String()
	if strings.Contains(gotStr, "COMMENT") {
		t.Errorf("expected no COMMENT clause, got:\n%s", gotStr)
	}
}

// TestOutputConstantMDL_CommentEscapesSingleQuotes verifies quotes in COMMENT are escaped.
func TestOutputConstantMDL_CommentEscapesSingleQuotes(t *testing.T) {
	buf := &bytes.Buffer{}
	e := New(buf)
	c := &model.Constant{
		Name:          "Greeting",
		Type:          model.ConstantDataType{Kind: "String"},
		DefaultValue:  "'hello'",
		Documentation: "It's a test",
	}
	if err := e.outputConstantMDL(c, "MyModule"); err != nil {
		t.Fatalf("outputConstantMDL: %v", err)
	}
	gotStr := buf.String()
	if !strings.Contains(gotStr, "COMMENT 'It''s a test'") {
		t.Errorf("expected escaped quote in COMMENT, got:\n%s", gotStr)
	}
}

// =============================================================================
// Issue #26: Date type distinct from DateTime
// =============================================================================

// TestConvertASTToMicroflowDataType_Date verifies Date maps to DateType (not DateTimeType).
func TestConvertASTToMicroflowDataType_Date(t *testing.T) {
	dt := ast.DataType{Kind: ast.TypeDate}
	result := convertASTToMicroflowDataType(dt, nil)
	if _, ok := result.(*microflows.DateType); !ok {
		t.Errorf("expected *microflows.DateType, got %T", result)
	}
}

// TestConvertASTToMicroflowDataType_DateTime verifies DateTime maps to DateTimeType (not affected by Date fix).
func TestConvertASTToMicroflowDataType_DateTime(t *testing.T) {
	dt := ast.DataType{Kind: ast.TypeDateTime}
	result := convertASTToMicroflowDataType(dt, nil)
	if _, ok := result.(*microflows.DateTimeType); !ok {
		t.Errorf("expected *microflows.DateTimeType, got %T", result)
	}
}

// TestDateType_GetTypeName verifies DateType.GetTypeName() returns "Date".
func TestDateType_GetTypeName(t *testing.T) {
	dt := &microflows.DateType{}
	if got := dt.GetTypeName(); got != "Date" {
		t.Errorf("DateType.GetTypeName() = %q, want %q", got, "Date")
	}
}

// TestFormatConstantType_Date verifies Date constant type formatting.
func TestFormatConstantType_Date(t *testing.T) {
	got := formatConstantType(model.ConstantDataType{Kind: "Date"})
	if got != "Date" {
		t.Errorf("formatConstantType(Date) = %q, want %q", got, "Date")
	}
}

// TestFormatConstantType_DateTime verifies DateTime constant type formatting.
func TestFormatConstantType_DateTime(t *testing.T) {
	got := formatConstantType(model.ConstantDataType{Kind: "DateTime"})
	if got != "DateTime" {
		t.Errorf("formatConstantType(DateTime) = %q, want %q", got, "DateTime")
	}
}

// =============================================================================
// Issue #27: DESCRIBE omits incorrect $ prefix on enum literal in RETURN
// =============================================================================

// TestIsQualifiedEnumLiteral verifies enum literal detection.
func TestIsQualifiedEnumLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Module.Status.Active", true},
		{"System.ENUM_Type.Value", true},
		{"MyVar", false},
		{"", false},
		{"empty", false},
		{"true", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isQualifiedEnumLiteral(tt.input)
			if got != tt.want {
				t.Errorf("isQualifiedEnumLiteral(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestFormatActivity_EndEvent_EnumReturn verifies enum RETURN has no $ prefix.
func TestFormatActivity_EndEvent_EnumReturn(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.EndEvent{
		BaseMicroflowObject: mkObj("1"),
		ReturnValue:         "Module.Status.Active",
	}
	got := e.formatActivity(obj, nil, nil)
	want := "RETURN Module.Status.Active;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatActivity_EndEvent_VariableReturn verifies variable RETURN keeps $ prefix.
func TestFormatActivity_EndEvent_VariableReturn(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.EndEvent{
		BaseMicroflowObject: mkObj("1"),
		ReturnValue:         "Result",
	}
	got := e.formatActivity(obj, nil, nil)
	want := "RETURN $Result;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// =============================================================================
// Issue #28: Inline if-then-else expression parsing and serialization
// =============================================================================

// TestParseInlineIfThenElse verifies the parser handles inline if-then-else in SET.
func TestParseInlineIfThenElse(t *testing.T) {
	input := `CREATE MICROFLOW Test.InlineIf ()
BEGIN
  DECLARE $X Integer = 0;
  SET $X = if $X > 10 then 1 else 0;
END;`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse error: %v", errs[0])
	}
	if len(prog.Statements) == 0 {
		t.Fatal("No statements parsed")
	}

	stmt, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("Expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}

	// Find the SET statement which should contain an IfThenElseExpr
	found := false
	for _, action := range stmt.Body {
		if setStmt, ok := action.(*ast.MfSetStmt); ok {
			if _, isIf := setStmt.Value.(*ast.IfThenElseExpr); isIf {
				found = true
			}
		}
	}
	if !found {
		t.Error("Expected SET with IfThenElseExpr, not found in parsed body")
	}
}

// TestExpressionToString_IfThenElse verifies inline if-then-else serialization.
func TestExpressionToString_IfThenElse(t *testing.T) {
	expr := &ast.IfThenElseExpr{
		Condition: &ast.BinaryExpr{
			Left:     &ast.VariableExpr{Name: "X"},
			Operator: ">",
			Right:    &ast.LiteralExpr{Value: int64(10), Kind: ast.LiteralInteger},
		},
		ThenExpr: &ast.VariableExpr{Name: "A"},
		ElseExpr: &ast.VariableExpr{Name: "B"},
	}
	got := expressionToString(expr)
	want := "if $X > 10 then $A else $B"
	if got != want {
		t.Errorf("expressionToString(IfThenElse) = %q, want %q", got, want)
	}
}

// TestExpressionToString_NestedIfThenElse verifies nested inline if-then-else.
func TestExpressionToString_NestedIfThenElse(t *testing.T) {
	expr := &ast.IfThenElseExpr{
		Condition: &ast.BinaryExpr{
			Left:     &ast.VariableExpr{Name: "X"},
			Operator: ">",
			Right:    &ast.LiteralExpr{Value: int64(0), Kind: ast.LiteralInteger},
		},
		ThenExpr: &ast.IfThenElseExpr{
			Condition: &ast.BinaryExpr{
				Left:     &ast.VariableExpr{Name: "X"},
				Operator: ">",
				Right:    &ast.LiteralExpr{Value: int64(100), Kind: ast.LiteralInteger},
			},
			ThenExpr: &ast.LiteralExpr{Value: int64(1), Kind: ast.LiteralInteger},
			ElseExpr: &ast.LiteralExpr{Value: int64(2), Kind: ast.LiteralInteger},
		},
		ElseExpr: &ast.LiteralExpr{Value: int64(3), Kind: ast.LiteralInteger},
	}
	got := expressionToString(expr)
	want := "if $X > 0 then if $X > 100 then 1 else 2 else 3"
	if got != want {
		t.Errorf("expressionToString(nested IfThenElse) = %q, want %q", got, want)
	}
}

// =============================================================================
// Issue #23: DataGrid2 column names derived from attribute or caption
// =============================================================================

// TestDeriveColumnName_FromAttribute verifies column name from attribute.
func TestDeriveColumnName_FromAttribute(t *testing.T) {
	col := rawDataGridColumn{Attribute: "MyModule.Order.OrderDate"}
	got := deriveColumnName(col, 0)
	if got != "OrderDate" {
		t.Errorf("deriveColumnName(attribute) = %q, want %q", got, "OrderDate")
	}
}

// TestDeriveColumnName_FromCaption verifies column name from caption.
func TestDeriveColumnName_FromCaption(t *testing.T) {
	col := rawDataGridColumn{Caption: "Order Date"}
	got := deriveColumnName(col, 0)
	if got != "Order_Date" {
		t.Errorf("deriveColumnName(caption) = %q, want %q", got, "Order_Date")
	}
}

// TestDeriveColumnName_Fallback verifies fallback to col%d.
func TestDeriveColumnName_Fallback(t *testing.T) {
	col := rawDataGridColumn{}
	got := deriveColumnName(col, 2)
	if got != "col3" {
		t.Errorf("deriveColumnName(empty) = %q, want %q", got, "col3")
	}
}

// TestDeriveColumnName_AttributePrecedence verifies attribute takes precedence over caption.
func TestDeriveColumnName_AttributePrecedence(t *testing.T) {
	col := rawDataGridColumn{
		Attribute: "MyModule.Order.Status",
		Caption:   "Order Status",
	}
	got := deriveColumnName(col, 0)
	if got != "Status" {
		t.Errorf("deriveColumnName(both) = %q, want %q", got, "Status")
	}
}

// TestDeriveColumnName_CaptionSpecialChars verifies caption sanitization.
func TestDeriveColumnName_CaptionSpecialChars(t *testing.T) {
	col := rawDataGridColumn{Caption: "Order #ID (main)"}
	got := deriveColumnName(col, 0)
	if got != "Order__ID__main" {
		t.Errorf("deriveColumnName(special chars) = %q, want %q", got, "Order__ID__main")
	}
}

// =============================================================================
// Issue #50: Association misidentified as Attribute (fallback without reader)
// =============================================================================

// TestResolveMemberChange_FallbackWithoutReader verifies that resolveMemberChange
// falls back to dot-contains heuristic when no reader is available.
// Regression: https://github.com/mendixlabs/mxcli/issues/50
func TestResolveMemberChange_FallbackWithoutReader(t *testing.T) {
	fb := &flowBuilder{
		// reader is nil — simulates no project context
	}

	// Without backend: a name without dot should default to attribute
	mc := &microflows.MemberChange{}
	fb.resolveMemberChange(mc, "Label", "Demo.Child")
	if mc.AttributeQualifiedName != "Demo.Child.Label" {
		t.Errorf("expected attribute 'Demo.Child.Label', got %q", mc.AttributeQualifiedName)
	}
	if mc.AssociationQualifiedName != "" {
		t.Errorf("expected empty association, got %q", mc.AssociationQualifiedName)
	}

	// With a dot in the name: should be treated as fully-qualified association (fallback)
	mc2 := &microflows.MemberChange{}
	fb.resolveMemberChange(mc2, "Demo.Child_Parent", "Demo.Child")
	if mc2.AssociationQualifiedName != "Demo.Child_Parent" {
		t.Errorf("expected association 'Demo.Child_Parent', got %q", mc2.AssociationQualifiedName)
	}
	if mc2.AttributeQualifiedName != "" {
		t.Errorf("expected empty attribute, got %q", mc2.AttributeQualifiedName)
	}
}
