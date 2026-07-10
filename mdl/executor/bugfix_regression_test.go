// SPDX-License-Identifier: Apache-2.0

// Regression tests for GitHub Issues #18, #19, #23, #25, #26, #27, #28.
package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
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
	if got != "end while" {
		t.Errorf("loopEndKeyword(WhileLoop) = %q, want %q", got, "end while")
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
	if got != "end loop" {
		t.Errorf("loopEndKeyword(ForEachLoop) = %q, want %q", got, "end loop")
	}
}

// TestLoopEndKeyword_NilSource verifies loopEndKeyword returns "END LOOP" when LoopSource is nil.
func TestLoopEndKeyword_NilSource(t *testing.T) {
	loop := &microflows.LoopedActivity{}
	got := loopEndKeyword(loop)
	if got != "end loop" {
		t.Errorf("loopEndKeyword(nil) = %q, want %q", got, "end loop")
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
	if got != "while $Counter <= $N" {
		t.Errorf("got %q, want %q", got, "while $Counter <= $N")
	}
}

func TestAddLoopStatement_PreservesAnnotatedPosition(t *testing.T) {
	fb := &flowBuilder{
		posX:         350,
		posY:         200,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"Items": "List of Test.Item"},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{varTypes: map[string]string{"Items": "List of Test.Item"}},
	}

	stmt := &ast.LoopStmt{
		LoopVariable: "Item",
		ListVariable: "Items",
		Annotations: &ast.ActivityAnnotations{
			Position: &ast.Position{X: 350, Y: 200},
		},
	}

	id := fb.addLoopStatement(stmt)
	if id == "" {
		t.Fatal("expected loop activity ID")
	}

	loop, ok := fb.objects[len(fb.objects)-1].(*microflows.LoopedActivity)
	if !ok {
		t.Fatalf("expected LoopedActivity, got %T", fb.objects[len(fb.objects)-1])
	}
	if loop.Position.X != 350 || loop.Position.Y != 200 {
		t.Fatalf("got loop position (%d, %d), want (350, 200)", loop.Position.X, loop.Position.Y)
	}
	wantNextX := 350 + loop.Size.Width/2 + HorizontalSpacing
	if fb.posX != wantNextX {
		t.Fatalf("got next posX %d, want %d", fb.posX, wantNextX)
	}
}

func TestAddWhileStatement_PreservesAnnotatedPosition(t *testing.T) {
	fb := &flowBuilder{
		posX:         420,
		posY:         180,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{varTypes: map[string]string{}},
	}

	stmt := &ast.WhileStmt{
		Condition: &ast.BinaryExpr{
			Left:     &ast.VariableExpr{Name: "Count"},
			Operator: "<",
			Right:    &ast.LiteralExpr{Kind: ast.LiteralInteger, Value: int64(10)},
		},
		Annotations: &ast.ActivityAnnotations{
			Position: &ast.Position{X: 420, Y: 180},
		},
	}

	id := fb.addWhileStatement(stmt)
	if id == "" {
		t.Fatal("expected while activity ID")
	}

	loop, ok := fb.objects[len(fb.objects)-1].(*microflows.LoopedActivity)
	if !ok {
		t.Fatalf("expected LoopedActivity, got %T", fb.objects[len(fb.objects)-1])
	}
	if loop.Position.X != 420 || loop.Position.Y != 180 {
		t.Fatalf("got while position (%d, %d), want (420, 180)", loop.Position.X, loop.Position.Y)
	}
	wantNextX := 420 + loop.Size.Width/2 + HorizontalSpacing
	if fb.posX != wantNextX {
		t.Fatalf("got next posX %d, want %d", fb.posX, wantNextX)
	}
}

func TestAddLogMessageAction_PreservesNodeExpression(t *testing.T) {
	fb := &flowBuilder{
		posX:     100,
		posY:     200,
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
	}

	stmt := &ast.LogStmt{
		Level: ast.LogInfo,
		Node: &ast.ConstantRefExpr{
			QualifiedName: ast.QualifiedName{Module: "TestModule", Name: "SecurityLogNode"},
		},
		Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "User added"},
	}

	id := fb.addLogMessageAction(stmt)
	if id == "" {
		t.Fatal("expected log activity ID")
	}

	activity, ok := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("expected ActionActivity, got %T", fb.objects[len(fb.objects)-1])
	}

	action, ok := activity.Action.(*microflows.LogMessageAction)
	if !ok {
		t.Fatalf("expected LogMessageAction, got %T", activity.Action)
	}

	if action.LogNodeName != "@TestModule.SecurityLogNode" {
		t.Fatalf("got log node %q, want %q", action.LogNodeName, "@TestModule.SecurityLogNode")
	}
}

func TestAddLogMessageAction_TemplateLiteralDoesNotKeepQuotes(t *testing.T) {
	fb := &flowBuilder{
		posX:     100,
		posY:     200,
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
	}

	stmt := &ast.LogStmt{
		Level: ast.LogInfo,
		Node:  &ast.LiteralExpr{Kind: ast.LiteralString, Value: "App"},
		Message: &ast.LiteralExpr{
			Kind:  ast.LiteralString,
			Value: "Order {1}",
		},
		Template: []ast.TemplateParam{
			{Index: 1, Value: &ast.VariableExpr{Name: "OrderNumber"}},
		},
	}

	id := fb.addLogMessageAction(stmt)
	if id == "" {
		t.Fatal("expected log activity ID")
	}

	activity, ok := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("expected ActionActivity, got %T", fb.objects[len(fb.objects)-1])
	}

	action, ok := activity.Action.(*microflows.LogMessageAction)
	if !ok {
		t.Fatalf("expected LogMessageAction, got %T", activity.Action)
	}

	if got := action.MessageTemplate.Translations["en_US"]; got != "Order {1}" {
		t.Fatalf("got message template %q, want %q", got, "Order {1}")
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
	if !strings.Contains(gotStr, "comment 'Maximum retry attempts'") {
		t.Errorf("expected comment clause in output, got:\n%s", gotStr)
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
	if strings.Contains(gotStr, "comment") {
		t.Errorf("expected no comment clause, got:\n%s", gotStr)
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
	if !strings.Contains(gotStr, "comment 'It''s a test'") {
		t.Errorf("expected escaped quote in comment, got:\n%s", gotStr)
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
	want := "return Module.Status.Active;"
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
	want := "return $Result;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// =============================================================================
// Issue #28: Inline if-then-else expression parsing and serialization
// =============================================================================

// TestParseInlineIfThenElse verifies the parser handles inline if-then-else in SET.
func TestParseInlineIfThenElse(t *testing.T) {
	input := `create microflow Test.InlineIf ()
begin
  declare $X Integer = 0;
  set $X = if $X > 10 then 1 else 0;
end;`

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
		t.Error("Expected set with IfThenElseExpr, not found in parsed body")
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
// falls back to the member-name shape heuristic when no metadata is available.
// Regression: https://github.com/JordtenBulte-OLC/mxcli/issues/50
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

func TestResolveMemberChange_FallbackWithoutReader_QualifiedAttributeStaysAttribute(t *testing.T) {
	fb := &flowBuilder{}

	mc := &microflows.MemberChange{}
	fb.resolveMemberChange(mc, "Demo.Child.Label", "Demo.Child")

	if mc.AttributeQualifiedName != "Demo.Child.Label" {
		t.Errorf("expected qualified attribute preserved, got %q", mc.AttributeQualifiedName)
	}
	if mc.AssociationQualifiedName != "" {
		t.Errorf("expected empty association, got %q", mc.AssociationQualifiedName)
	}
}

func TestCallMicroflowResultType_ResolvesSubsequentChangeMember(t *testing.T) {
	moduleID := model.ID("module-1")
	backend := &mock.MockBackend{
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name != "MfTest" {
				return nil, nil
			}
			return &model.Module{
				BaseElement: model.BaseElement{ID: moduleID},
				Name:        "MfTest",
			}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{
				{
					ContainerID: moduleID,
					Name:        "M012_CreateEntity",
					ReturnType: &microflows.ObjectType{
						EntityQualifiedName: "MfTest.Product",
					},
				},
			}, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id != moduleID {
				return nil, nil
			}
			return &domainmodel.DomainModel{
				ContainerID: moduleID,
				Entities: []*domainmodel.Entity{
					{
						Name: "Product",
						Attributes: []*domainmodel.Attribute{
							{
								Name: "Price",
								Type: &domainmodel.DecimalAttributeType{},
							},
						},
					},
				},
			}, nil
		},
	}

	fb := &flowBuilder{
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		backend:      backend,
	}

	fb.addCallMicroflowAction(&ast.CallMicroflowStmt{
		OutputVariable: "Product",
		MicroflowName:  ast.QualifiedName{Module: "MfTest", Name: "M012_CreateEntity"},
	})
	fb.addChangeObjectAction(&ast.ChangeObjectStmt{
		Variable: "Product",
		Changes: []ast.ChangeItem{
			{
				Attribute: "Price",
				Value:     &ast.LiteralExpr{Value: 10.0, Kind: ast.LiteralDecimal},
			},
		},
	})

	if got := fb.varTypes["Product"]; got != "MfTest.Product" {
		t.Fatalf("expected call result type MfTest.Product, got %q", got)
	}

	activity, ok := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("expected last object to be ActionActivity, got %T", fb.objects[len(fb.objects)-1])
	}
	changeAction, ok := activity.Action.(*microflows.ChangeObjectAction)
	if !ok {
		t.Fatalf("expected last action to be ChangeObjectAction, got %T", activity.Action)
	}
	if len(changeAction.Changes) != 1 {
		t.Fatalf("expected one member change, got %d", len(changeAction.Changes))
	}
	if got := changeAction.Changes[0].AttributeQualifiedName; got != "MfTest.Product.Price" {
		t.Fatalf("expected qualified attribute MfTest.Product.Price, got %q", got)
	}
}

func TestEmptyChangeObjectRefreshesInClient(t *testing.T) {
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}

	id := fb.addChangeObjectAction(&ast.ChangeObjectStmt{Variable: "Object"})
	if id == "" || len(fb.objects) != 1 {
		t.Fatalf("expected one change object activity, got id=%q objects=%d", id, len(fb.objects))
	}

	activity, ok := fb.objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("object type = %T, want *microflows.ActionActivity", fb.objects[0])
	}
	action, ok := activity.Action.(*microflows.ChangeObjectAction)
	if !ok {
		t.Fatalf("action type = %T, want *microflows.ChangeObjectAction", activity.Action)
	}
	if !action.RefreshInClient {
		t.Fatal("empty change object must refresh in client to remain valid without member changes or commit")
	}

	id = fb.addChangeObjectAction(&ast.ChangeObjectStmt{
		Variable: "Object",
		Changes: []ast.ChangeItem{{
			Attribute: "Name",
			Value:     &ast.LiteralExpr{Kind: ast.LiteralString, Value: "changed"},
		}},
	})
	if id == "" || len(fb.objects) != 2 {
		t.Fatalf("expected second change object activity, got id=%q objects=%d", id, len(fb.objects))
	}
	activity, ok = fb.objects[1].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("object type = %T, want *microflows.ActionActivity", fb.objects[1])
	}
	action, ok = activity.Action.(*microflows.ChangeObjectAction)
	if !ok {
		t.Fatalf("action type = %T, want *microflows.ChangeObjectAction", activity.Action)
	}
	if action.RefreshInClient {
		t.Fatal("non-empty change object must not infer refresh in client")
	}
}

func TestListFindAttributeEqualsExpressionUsesAttributeOperation(t *testing.T) {
	fb := &flowBuilder{
		posX:    100,
		posY:    100,
		spacing: HorizontalSpacing,
		varTypes: map[string]string{
			"Items": "List of Demo.Item",
		},
	}

	id := fb.addListOperationAction(&ast.ListOperationStmt{
		OutputVariable: "ExistingItem",
		Operation:      ast.ListOpFind,
		InputVariable:  "Items",
		Condition: &ast.BinaryExpr{
			Left:     &ast.IdentifierExpr{Name: "Code"},
			Operator: "=",
			Right: &ast.AttributePathExpr{
				Variable: "IteratorItem",
				Path:     []string{"ExternalCode"},
			},
		},
	})
	if id == "" || len(fb.objects) != 1 {
		t.Fatalf("expected one list operation activity, got id=%q objects=%d", id, len(fb.objects))
	}

	activity, ok := fb.objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("object type = %T, want *microflows.ActionActivity", fb.objects[0])
	}
	action, ok := activity.Action.(*microflows.ListOperationAction)
	if !ok {
		t.Fatalf("action type = %T, want *microflows.ListOperationAction", activity.Action)
	}
	op, ok := action.Operation.(*microflows.FindByAttributeOperation)
	if !ok {
		t.Fatalf("operation type = %T, want *microflows.FindByAttributeOperation", action.Operation)
	}
	if op.Attribute != "Demo.Item.Code" {
		t.Fatalf("Attribute = %q, want Demo.Item.Code", op.Attribute)
	}
	if op.Expression != "$IteratorItem/ExternalCode" {
		t.Fatalf("Expression = %q, want $IteratorItem/ExternalCode", op.Expression)
	}
}

func TestListFindUnknownListTypeFallsBackToExpressionOperation(t *testing.T) {
	fb := &flowBuilder{
		posX:    100,
		posY:    100,
		spacing: HorizontalSpacing,
	}

	id := fb.addListOperationAction(&ast.ListOperationStmt{
		OutputVariable: "ExistingItem",
		Operation:      ast.ListOpFind,
		InputVariable:  "Items",
		Condition: &ast.BinaryExpr{
			Left:     &ast.IdentifierExpr{Name: "Code"},
			Operator: "=",
			Right:    &ast.LiteralExpr{Kind: ast.LiteralString, Value: "A"},
		},
	})
	if id == "" || len(fb.objects) != 1 {
		t.Fatalf("expected one list operation activity, got id=%q objects=%d", id, len(fb.objects))
	}

	activity, ok := fb.objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("object type = %T, want *microflows.ActionActivity", fb.objects[0])
	}
	action, ok := activity.Action.(*microflows.ListOperationAction)
	if !ok {
		t.Fatalf("action type = %T, want *microflows.ListOperationAction", activity.Action)
	}
	if _, ok := action.Operation.(*microflows.FindByAttributeOperation); ok {
		t.Fatal("expected expression fallback, got *microflows.FindByAttributeOperation")
	}
	op, ok := action.Operation.(*microflows.FindOperation)
	if !ok {
		t.Fatalf("operation type = %T, want *microflows.FindOperation", action.Operation)
	}
	if op.Expression == "" {
		t.Fatal("expected expression fallback to preserve the condition")
	}
}

func TestCallMicroflowUnknownResultTypeStillDeclaresVariable(t *testing.T) {
	fb := &flowBuilder{
		varTypes:     map[string]string{"Result": "Old.ModuleEntity"},
		declaredVars: map[string]string{},
	}

	fb.addCallMicroflowAction(&ast.CallMicroflowStmt{
		OutputVariable: "Result",
		MicroflowName:  ast.QualifiedName{Module: "Missing", Name: "Unknown"},
	})

	if _, ok := fb.varTypes["Result"]; ok {
		t.Fatalf("expected stale entity typing to be cleared, got %q", fb.varTypes["Result"])
	}
	if got := fb.declaredVars["Result"]; got != "Unknown" {
		t.Fatalf("expected Result to remain declared as Unknown, got %q", got)
	}
	if !fb.isVariableDeclared("Result") {
		t.Fatal("expected Result to remain declared after unresolved call return type")
	}
}

func TestValidateMicroflowReferencesSkipsExcludedMicroflow(t *testing.T) {
	moduleID := model.ID("module-1")
	backend := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{
				BaseElement: model.BaseElement{ID: moduleID},
				Name:        "SyntheticAudit",
			}}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(backend))

	stmt := &ast.CreateMicroflowStmt{
		Excluded: true,
		Name:     ast.QualifiedName{Module: "SyntheticAudit", Name: "ExcludedLegacyFlow"},
		Body: []ast.MicroflowStatement{
			&ast.CallMicroflowStmt{
				MicroflowName: ast.QualifiedName{Module: "SyntheticAudit", Name: "DeletedScaffoldFlow"},
			},
		},
	}

	if err := validate(ctx, stmt); err != nil {
		t.Fatalf("excluded microflow reference validation returned error: %v", err)
	}
}

func TestValidateMicroflowReferencesReportsIncludedMissingMicroflow(t *testing.T) {
	moduleID := model.ID("module-1")
	backend := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{
				BaseElement: model.BaseElement{ID: moduleID},
				Name:        "SyntheticAudit",
			}}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(backend))

	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "SyntheticAudit", Name: "IncludedFlow"},
		Body: []ast.MicroflowStatement{
			&ast.CallMicroflowStmt{
				MicroflowName: ast.QualifiedName{Module: "SyntheticAudit", Name: "DeletedScaffoldFlow"},
			},
		},
	}

	err := validate(ctx, stmt)
	if err == nil {
		t.Fatal("expected missing microflow reference error")
	}
	if !strings.Contains(err.Error(), "microflow not found: SyntheticAudit.DeletedScaffoldFlow") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

// =============================================================================
// Issue #476: Business event MESSAGE attribute names reject reserved keywords
// =============================================================================

// TestBusinessEvent_KeywordAttributeNames verifies that reserved keywords (Date,
// Amount, etc.) are accepted as MESSAGE attribute names and message names.
func TestBusinessEvent_KeywordAttributeNames(t *testing.T) {
	input := `CREATE BUSINESS EVENT SERVICE MyModule.Events
(ServiceName: 'Svc', EventNamePrefix: 'com.test')
{
  MESSAGE OrderCreated (Date: DateTime, Amount: Decimal) PUBLISH ENTITY MyModule.Order;
};`

	_, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("issue #476: keyword attribute names rejected: %v", errs[0])
	}
}
