// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// exitAlterPageStatement handles ALTER PAGE/SNIPPET Module.Name { operations }
func (b *Builder) exitAlterPageStatement(ctx *parser.AlterStatementContext) {
	stmt := &ast.AlterPageStmt{}

	// Container type
	if ctx.SNIPPET() != nil {
		stmt.ContainerType = "SNIPPET"
	} else {
		stmt.ContainerType = "PAGE"
	}

	// Page/snippet name
	if qn := ctx.QualifiedName(); qn != nil {
		stmt.PageName = buildQualifiedName(qn)
	}

	// Parse operations
	for _, opCtx := range ctx.AllAlterPageOperation() {
		op := opCtx.(*parser.AlterPageOperationContext)

		if setCtx := op.AlterPageSet(); setCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterPageSet(setCtx.(*parser.AlterPageSetContext)))
		} else if insertCtx := op.AlterPageInsert(); insertCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterPageInsert(insertCtx.(*parser.AlterPageInsertContext)))
		} else if dropCtx := op.AlterPageDrop(); dropCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterPageDrop(dropCtx.(*parser.AlterPageDropContext)))
		} else if replaceCtx := op.AlterPageReplace(); replaceCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterPageReplace(replaceCtx.(*parser.AlterPageReplaceContext)))
		} else if addVarCtx := op.AlterPageAddVariable(); addVarCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterPageAddVariable(addVarCtx.(*parser.AlterPageAddVariableContext)))
		} else if dropVarCtx := op.AlterPageDropVariable(); dropVarCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterPageDropVariable(dropVarCtx.(*parser.AlterPageDropVariableContext)))
		}
	}

	b.statements = append(b.statements, stmt)
}

// buildAlterPageSet builds a SetPropertyOp or SetLayoutOp from the parse tree.
func (b *Builder) buildAlterPageSet(ctx *parser.AlterPageSetContext) ast.AlterPageOperation {
	// SET Layout = Module.LayoutName [MAP (...)]
	if ctx.LAYOUT() != nil {
		return b.buildAlterPageSetLayout(ctx)
	}

	op := &ast.SetPropertyOp{
		Properties: make(map[string]interface{}),
	}

	// Widget ref (if ON widgetRef is present)
	if ctx.ON() != nil {
		if wr := ctx.WidgetRef(); wr != nil {
			op.Target = buildWidgetRef(wr)
		}
	}

	// Parse assignments
	for _, assignCtx := range ctx.AllAlterPageAssignment() {
		assign := assignCtx.(*parser.AlterPageAssignmentContext)
		name, value := b.buildAlterPageAssignment(assign)
		if name != "" {
			op.Properties[name] = value
		}
	}

	return op
}

// buildAlterPageSetLayout builds a SetLayoutOp from: SET Layout = QN [MAP (old -> new, ...)]
func (b *Builder) buildAlterPageSetLayout(ctx *parser.AlterPageSetContext) *ast.SetLayoutOp {
	op := &ast.SetLayoutOp{}

	// Layout qualified name
	if qn := ctx.QualifiedName(); qn != nil {
		op.NewLayout = buildQualifiedName(qn)
	}

	// Optional MAP (old -> new, ...)
	mappings := ctx.AllAlterLayoutMapping()
	if len(mappings) > 0 {
		op.Mappings = make(map[string]string, len(mappings))
		for _, m := range mappings {
			mc := m.(*parser.AlterLayoutMappingContext)
			ids := mc.AllIdentifierOrKeyword()
			if len(ids) == 2 {
				from := identifierOrKeywordText(ids[0])
				to := identifierOrKeywordText(ids[1])
				op.Mappings[from] = to
			}
		}
	}

	return op
}

// buildAlterPageAssignment extracts property name and value from an assignment context.
func (b *Builder) buildAlterPageAssignment(ctx *parser.AlterPageAssignmentContext) (string, interface{}) {
	// DataSource = dataSourceExprV3
	if dsCtx := ctx.DataSourceExprV3(); dsCtx != nil {
		return "DataSource", buildDataSourceV3(dsCtx)
	}

	// Visible = [expr] / Editable = [expr] — conditional visibility/editability.
	// Same context-rooting as CREATE PAGE (issue #627): bare attributes become
	// $currentObject/Attr. Routed to VisibleIf/EditableIf so the mutator builds a
	// ConditionalVisibilitySettings node instead of a static Visible string.
	if xc := ctx.XpathConstraint(); xc != nil {
		if ctx.VISIBLE() != nil {
			return "VisibleIf", buildConditionalExpression(xc)
		}
		if ctx.EDITABLE() != nil {
			return "EditableIf", buildConditionalExpression(xc)
		}
	}

	var name string

	if id := ctx.IdentifierOrKeyword(); id != nil {
		name = identifierOrKeywordText(id)
	} else if sl := ctx.STRING_LITERAL(); sl != nil {
		name = unquoteString(sl.GetText())
	}

	value := buildPropertyValueV3(ctx.PropertyValueV3())

	return name, value
}

// buildAlterPageInsert builds an InsertWidgetOp from the parse tree.
func (b *Builder) buildAlterPageInsert(ctx *parser.AlterPageInsertContext) *ast.InsertWidgetOp {
	op := &ast.InsertWidgetOp{}

	if ctx.AFTER() != nil {
		op.Position = "AFTER"
	} else if ctx.BEFORE() != nil {
		op.Position = "BEFORE"
	}

	if wr := ctx.WidgetRef(); wr != nil {
		op.Target = buildWidgetRef(wr)
	}

	if body := ctx.PageBodyV3(); body != nil {
		op.Widgets = buildPageBodyV3(body, b)
	}

	return op
}

// buildAlterPageDrop builds a DropWidgetOp from the parse tree.
func (b *Builder) buildAlterPageDrop(ctx *parser.AlterPageDropContext) *ast.DropWidgetOp {
	op := &ast.DropWidgetOp{}

	for _, wr := range ctx.AllWidgetRef() {
		op.Targets = append(op.Targets, buildWidgetRef(wr))
	}

	return op
}

// buildAlterPageReplace builds a ReplaceWidgetOp from the parse tree.
func (b *Builder) buildAlterPageReplace(ctx *parser.AlterPageReplaceContext) *ast.ReplaceWidgetOp {
	op := &ast.ReplaceWidgetOp{}

	if wr := ctx.WidgetRef(); wr != nil {
		op.Target = buildWidgetRef(wr)
	}

	if body := ctx.PageBodyV3(); body != nil {
		op.NewWidgets = buildPageBodyV3(body, b)
	}

	return op
}

// buildWidgetRef extracts a WidgetRef from a widgetRef grammar context.
// Supports both plain "btnSave" and dotted "dgProducts.Name" references.
func buildWidgetRef(ctx parser.IWidgetRefContext) ast.WidgetRef {
	wrCtx := ctx.(*parser.WidgetRefContext)
	ids := wrCtx.AllIdentifierOrKeyword()
	if len(ids) == 2 {
		return ast.WidgetRef{
			Widget: identifierOrKeywordText(ids[0]),
			Column: identifierOrKeywordText(ids[1]),
		}
	}
	if len(ids) == 1 {
		return ast.WidgetRef{Widget: identifierOrKeywordText(ids[0])}
	}
	return ast.WidgetRef{}
}

// buildAlterPageAddVariable builds an AddVariableOp from the parse tree.
func (b *Builder) buildAlterPageAddVariable(ctx *parser.AlterPageAddVariableContext) *ast.AddVariableOp {
	op := &ast.AddVariableOp{}
	if vd := ctx.VariableDeclaration(); vd != nil {
		op.Variable = buildSingleVariableDeclaration(vd.(*parser.VariableDeclarationContext))
	}
	return op
}

// buildAlterPageDropVariable builds a DropVariableOp from the parse tree.
func (b *Builder) buildAlterPageDropVariable(ctx *parser.AlterPageDropVariableContext) *ast.DropVariableOp {
	op := &ast.DropVariableOp{}
	if varTok := ctx.VARIABLE(); varTok != nil {
		op.VariableName = strings.TrimPrefix(varTok.GetText(), "$")
	}
	return op
}
