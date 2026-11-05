// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// exitAlterDocumentStatement handles the generic
// ALTER <type> Module.Name { set / insert / replace / drop } (ADR-0012
// decision 2). The page family — page, snippet, layout — is the first set of
// document types on it, and builds the AlterPageStmt every page validator and
// the executor already speak.
func (b *Builder) exitAlterDocumentStatement(ctx *parser.AlterStatementContext) {
	stmt := &ast.AlterPageStmt{}

	docType := ctx.AlterDocumentType().(*parser.AlterDocumentTypeContext)
	switch {
	case docType.SNIPPET() != nil:
		stmt.ContainerType = "SNIPPET"
	case docType.LAYOUT() != nil:
		stmt.ContainerType = "LAYOUT"
	default:
		stmt.ContainerType = "PAGE"
	}

	if qn := ctx.QualifiedName(); qn != nil {
		stmt.PageName = buildQualifiedName(qn)
	}

	for _, opCtx := range ctx.AllAlterOperation() {
		op := opCtx.(*parser.AlterOperationContext)

		if setCtx := op.AlterSet(); setCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterSet(setCtx.(*parser.AlterSetContext)))
		} else if insertCtx := op.AlterInsert(); insertCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterInsert(insertCtx.(*parser.AlterInsertContext)))
		} else if dropCtx := op.AlterDrop(); dropCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterDrop(dropCtx.(*parser.AlterDropContext)))
		} else if dropTplCtx := op.AlterPageDropTemplate(); dropTplCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterPageDropTemplate(dropTplCtx.(*parser.AlterPageDropTemplateContext)))
		} else if replaceCtx := op.AlterReplace(); replaceCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterReplace(replaceCtx.(*parser.AlterReplaceContext)))
		} else if addVarCtx := op.AlterPageAddVariable(); addVarCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterPageAddVariable(addVarCtx.(*parser.AlterPageAddVariableContext)))
		} else if dropVarCtx := op.AlterPageDropVariable(); dropVarCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterPageDropVariable(dropVarCtx.(*parser.AlterPageDropVariableContext)))
		} else if addParamCtx := op.AlterPageAddParameter(); addParamCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterPageAddParameter(addParamCtx.(*parser.AlterPageAddParameterContext)))
		} else if dropParamCtx := op.AlterPageDropParameter(); dropParamCtx != nil {
			stmt.Operations = append(stmt.Operations, b.buildAlterPageDropParameter(dropParamCtx.(*parser.AlterPageDropParameterContext)))
		}
	}

	b.statements = append(b.statements, stmt)
}

// buildAlterPageDropTemplate builds a DropListViewTemplateOp:
// DROP TEMPLATE FOR Module.Specialization IN listViewName
func (b *Builder) buildAlterPageDropTemplate(ctx *parser.AlterPageDropTemplateContext) ast.AlterPageOperation {
	op := &ast.DropListViewTemplateOp{}
	if qn := ctx.QualifiedName(); qn != nil {
		op.Specialization = qn.GetText()
	}
	if tr := ctx.AlterTarget(); tr != nil {
		op.ListView = b.buildAlterTarget(tr).Widget
	}
	return op
}

// buildAlterSet builds a SetPropertyOp or SetLayoutOp from the parse tree.
func (b *Builder) buildAlterSet(ctx *parser.AlterSetContext) ast.AlterPageOperation {
	// SET Layout = Module.LayoutName [MAP (...)]
	if ctx.LAYOUT() != nil {
		return b.buildAlterSetLayout(ctx)
	}

	op := &ast.SetPropertyOp{
		Properties: make(map[string]interface{}),
	}

	if ctx.ON() != nil {
		if tr := ctx.AlterTarget(); tr != nil {
			op.Target = b.buildAlterTarget(tr)
		}
	}

	for _, assignCtx := range ctx.AllAlterPageAssignment() {
		assign := assignCtx.(*parser.AlterPageAssignmentContext)
		name, value := b.buildAlterPageAssignment(assign)
		if name != "" {
			op.Properties[name] = value
		}
	}
	b.recordAlterPageSet(ctx)

	return op
}

// buildAlterSetLayout builds a SetLayoutOp from: SET Layout = QN [MAP (old -> new, ...)]
func (b *Builder) buildAlterSetLayout(ctx *parser.AlterSetContext) *ast.SetLayoutOp {
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
	// CaptionParams / ContentParams: a text template's parameters, the same
	// list CREATE PAGE reads (ako/mxcli#1966).
	if pl := ctx.ParamListV3(); pl != nil {
		name := "ContentParams"
		if ctx.CAPTIONPARAMS() != nil {
			name = "CaptionParams"
		}
		return name, buildParamListV3(pl)
	}
	// An expression-typed property takes the expression as written, from
	// whichever value alternative matched (visitor_widget_expression.go).
	if id := ctx.IdentifierOrKeyword(); id != nil && ctx.STRING_LITERAL() == nil {
		name := identifierOrKeywordText(id)
		if isWidgetExpressionProp(name) {
			if v := lastRuleChild(ctx); v != nil {
				return name, b.widgetExpressionPropValue(name, v)
			}
		}
		if expr := ctx.Expression(); expr != nil {
			// `$Param.Attr` binds an attribute property to a page or snippet
			// parameter (ako/mxcli#1622); the executor, which knows the
			// property's kind, resolves or refuses it.
			if text := stripExpressionIdentifierQuotes(ruleSourceText(expr.(antlr.ParserRuleContext))); ast.IsParameterAttributeText(text) {
				return name, &ast.WidgetExpressionV3{Text: text}
			}
			b.addError(widgetExpressionNotAllowed(name, expr))
			return "", nil
		}
	}
	// DataSource = dataSourceExprV3
	if dsCtx := ctx.DataSourceExprV3(); dsCtx != nil {
		return "DataSource", buildDataSourceV3(dsCtx)
	}

	// Action = actionExprV3 — the same action grammar CREATE PAGE uses, so every
	// form is available here (MICROFLOW/NANOFLOW with arguments, SHOW_PAGE,
	// SAVE_CHANGES CLOSE_PAGE, CREATE_OBJECT … THEN …). Retargeting a button was
	// previously only possible by REPLACEing the whole widget, which silently
	// drops any property the author did not restate.
	if acCtx := ctx.ActionExprV3(); acCtx != nil {
		// A named pluggable action slot — `set 'createFileAction' = microflow
		// M.F` — keeps the author's key; the executor routes any action value
		// whose key is not `Action` to the slot of that name (#995).
		if id := ctx.IdentifierOrKeyword(); id != nil {
			return identifierOrKeywordText(id), buildActionV3(acCtx)
		}
		if sl := ctx.STRING_LITERAL(); sl != nil {
			return unquoteStringLit(sl), buildActionV3(acCtx)
		}
		return "Action", buildActionV3(acCtx)
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
	// Visible: <expression> / Editable: <expression> — the canonical form, the
	// expression stored as written (R5); a plain value keeps the key it had
	// when it reached the generic alternative below.
	if kw := visibleOrEditable(ctx.VISIBLE(), ctx.EDITABLE()); kw != nil {
		if e := ctx.Expression(); e != nil {
			if ctx.VISIBLE() != nil {
				return "VisibleIf", bareArgumentText(e)
			}
			return "EditableIf", bareArgumentText(e)
		}
		return kw.GetText(), buildPropertyValueV3(ctx.PropertyValueV3())
	}

	var name string

	if id := ctx.IdentifierOrKeyword(); id != nil {
		name = identifierOrKeywordText(id)
	} else if sl := ctx.STRING_LITERAL(); sl != nil {
		name = unquoteStringLit(sl)
	}

	value := buildPropertyValueV3(ctx.PropertyValueV3())

	return name, value
}

// buildAlterInsert builds an InsertWidgetOp from the parse tree.
func (b *Builder) buildAlterInsert(ctx *parser.AlterInsertContext) *ast.InsertWidgetOp {
	op := &ast.InsertWidgetOp{}

	if ctx.AFTER() != nil {
		op.Position = "AFTER"
	} else if ctx.BEFORE() != nil {
		op.Position = "BEFORE"
	} else if ctx.INTO() != nil {
		op.Position = "INTO"
	}

	if tr := ctx.AlterTarget(); tr != nil {
		op.Target = b.buildAlterTarget(tr)
	}

	op.Widgets = b.buildAlterFragment(ctx.AlterFragment())
	return op
}

// buildAlterDrop builds a DropWidgetOp from the parse tree.
func (b *Builder) buildAlterDrop(ctx *parser.AlterDropContext) *ast.DropWidgetOp {
	op := &ast.DropWidgetOp{}
	b.recordAlterPageDropWidget(ctx)
	for _, tr := range ctx.AllAlterTarget() {
		op.Targets = append(op.Targets, b.buildAlterTarget(tr))
	}
	return op
}

// buildAlterReplace builds a ReplaceWidgetOp from the parse tree.
func (b *Builder) buildAlterReplace(ctx *parser.AlterReplaceContext) *ast.ReplaceWidgetOp {
	op := &ast.ReplaceWidgetOp{}

	if tr := ctx.AlterTarget(); tr != nil {
		op.Target = b.buildAlterTarget(tr)
	}

	op.NewWidgets = b.buildAlterFragment(ctx.AlterFragment())
	return op
}

// buildAlterFragment builds the widgets of an INSERT / REPLACE fragment, which
// is written exactly as CREATE PAGE writes its body.
func (b *Builder) buildAlterFragment(ctx parser.IAlterFragmentContext) []*ast.WidgetV3 {
	if ctx == nil {
		return nil
	}
	if body := ctx.(*parser.AlterFragmentContext).PageBodyV3(); body != nil {
		return buildPageBodyV3(body, b)
	}
	return nil
}

// buildAlterTarget extracts the generic ALTER address — a name, a dotted
// name.member, or a quoted caption, each with an optional @n. What an address
// means is the document type's call (backend.AlterTargetResolver); this only
// records what was written.
func (b *Builder) buildAlterTarget(ctx parser.IAlterTargetContext) ast.WidgetRef {
	tc := ctx.(*parser.AlterTargetContext)
	var ref ast.WidgetRef
	// A cell address's numbers are its row and column, read below; every other
	// form's one number is @n.
	if n := tc.NUMBER_LITERAL(0); n != nil && tc.CELL() == nil {
		// @n counts matches from 1, as the ambiguity error lists them.
		if v, err := strconv.Atoi(n.GetText()); err == nil && v >= 1 {
			ref.Ordinal = v
		} else {
			b.addError(fmt.Errorf("alter target %s: @%s is not a match number — matches are counted from @1",
				tc.GetText(), n.GetText()))
		}
	}
	if tc.ITEM() != nil {
		// `nav item(t1)` / `nav item('Home')` / `nav item@2` (#1388). The @n is
		// the item's, so it moves off the target, where it would mean a choice
		// among widgets.
		ids := tc.AllIdentifierOrKeyword()
		// `acc1 item@1.headercontent` names a slot of the item. It is carried
		// as Column, the way `grid.controlbar` carries a widget's slot, and is
		// taken off the identifiers first: `item@1.slot` has two of them, the
		// shape `item(t1)` is otherwise recognised by.
		if slot := tc.GetItemSlot(); slot != nil {
			ref.Column = identifierOrKeywordText(slot)
			ids = ids[:len(ids)-1]
		}
		item := &ast.NavItemRef{List: identifierOrKeywordText(ids[0]), Ordinal: ref.Ordinal}
		if sl := tc.STRING_LITERAL(); sl != nil {
			item.Text = unquoteStringLit(sl)
		} else if len(ids) == 2 {
			item.Widget = identifierOrKeywordText(ids[1])
		}
		ref.Ordinal = 0
		ref.Item = item
		ref.Widget = item.Spell()
		return ref
	}
	if tc.CELL() != nil {
		// `table1 cell(row, column)`: a legacy table cell, by its position.
		cell := &ast.TableCellRef{Table: identifierOrKeywordText(tc.IdentifierOrKeyword(0))}
		for _, n := range []struct {
			text string
			dst  *int
		}{{tc.GetCellRow().GetText(), &cell.Row}, {tc.GetCellColumn().GetText(), &cell.Column}} {
			v, err := strconv.Atoi(n.text)
			if err != nil || v < 1 {
				b.addError(fmt.Errorf("alter target %s: table rows and columns are counted from 1, as describe prints them",
					tc.GetText()))
				v = 1
			}
			*n.dst = v
		}
		ref.Ordinal = 0
		ref.Cell = cell
		ref.Widget = cell.Spell()
		return ref
	}
	if tc.COLUMN() != nil {
		// `grid column(Attr)` / `grid column('Caption')` (#749).
		ref.Widget = identifierOrKeywordText(tc.IdentifierOrKeyword(0))
		if sl := tc.STRING_LITERAL(); sl != nil {
			ref.ColumnCaption = unquoteStringLit(sl)
		} else if ap := tc.AttributePathV3(); ap != nil {
			ref.ColumnAttribute = buildAttributePathV3(ap)
		}
		return ref
	}
	if sl := tc.STRING_LITERAL(); sl != nil {
		ref.Caption = unquoteStringLit(sl)
		return ref
	}
	ids := tc.AllIdentifierOrKeyword()
	if len(ids) >= 1 {
		ref.Widget = identifierOrKeywordText(ids[0])
	}
	if len(ids) == 2 {
		ref.Column = identifierOrKeywordText(ids[1])
	}
	return ref
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

// buildAlterPageAddParameter builds an AddParameterOp from the parse tree. The
// declaration goes through buildPageParameter, CREATE's own conversion, so a
// parameter means the same thing whichever statement declares it.
func (b *Builder) buildAlterPageAddParameter(ctx *parser.AlterPageAddParameterContext) *ast.AddParameterOp {
	op := &ast.AddParameterOp{}
	if p := ctx.PageParameter(); p != nil {
		op.Parameter = buildPageParameter(p.(*parser.PageParameterContext))
	}
	return op
}

// buildAlterPageDropParameter builds a DropParameterOp from the parse tree.
func (b *Builder) buildAlterPageDropParameter(ctx *parser.AlterPageDropParameterContext) *ast.DropParameterOp {
	op := &ast.DropParameterOp{}
	if varTok := ctx.VARIABLE(); varTok != nil {
		op.ParameterName = strings.TrimPrefix(varTok.GetText(), "$")
	}
	return op
}

// exitAlterPagesLayoutStatement builds the bulk repoint:
// ALTER PAGES [IN <module>] SET LAYOUT = QN [MAP (…)] [WHERE LAYOUT = QN]
func (b *Builder) exitAlterPagesLayoutStatement(ctx *parser.AlterPagesLayoutStatementContext) {
	stmt := &ast.AlterPagesLayoutStmt{}

	if id := ctx.IdentifierOrKeyword(); id != nil {
		stmt.Module = identifierOrKeywordText(id)
	}

	// Two qualified names in the rule, in source order: the new layout, then
	// the WHERE filter. Positional rather than named because ANTLR gives back
	// one list — and getting the order backwards would repoint every page onto
	// the layout being migrated away from.
	qns := ctx.AllQualifiedName()
	if len(qns) > 0 {
		stmt.NewLayout = buildQualifiedName(qns[0])
	}
	if len(qns) > 1 {
		where := buildQualifiedName(qns[1])
		stmt.WhereLayout = &where
	}

	if mappings := ctx.AllAlterLayoutMapping(); len(mappings) > 0 {
		stmt.Mappings = make(map[string]string, len(mappings))
		for _, m := range mappings {
			ids := m.(*parser.AlterLayoutMappingContext).AllIdentifierOrKeyword()
			if len(ids) == 2 {
				stmt.Mappings[identifierOrKeywordText(ids[0])] = identifierOrKeywordText(ids[1])
			}
		}
	}

	b.statements = append(b.statements, stmt)
}

// exitAlterPagesStylingStatement builds the bulk design-property sweep:
// ALTER PAGES [IN <module>] SET 'key' = value, … WHERE WIDGETTYPE = <kw> [DRY RUN]
func (b *Builder) exitAlterPagesStylingStatement(ctx *parser.AlterPagesStylingStatementContext) {
	stmt := &ast.AlterPagesStylingStmt{DryRun: ctx.DRY() != nil}

	// Two identifierOrKeyword positions in the rule — the optional module and
	// the WIDGETTYPE value — and ANTLR returns one list, so which is which
	// depends on whether IN was given. Getting it backwards would scope a
	// project-wide sweep to a module named after a widget type, or vice versa.
	ids := ctx.AllIdentifierOrKeyword()
	if ctx.IN() != nil && len(ids) > 0 {
		stmt.Module = identifierOrKeywordText(ids[0])
		ids = ids[1:]
	}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		// The WHERE value as a quoted string — a full widget id.
		stmt.WidgetType = unquoteStringLit(lit)
	} else if len(ids) > 0 {
		stmt.WidgetType = identifierOrKeywordText(ids[0])
	}

	for _, a := range ctx.AllAlterPagesStylingAssignment() {
		ac := a.(*parser.AlterPagesStylingAssignmentContext)
		lits := ac.AllSTRING_LITERAL()
		if len(lits) == 0 {
			continue
		}
		assignment := ast.StylingAssignment{Property: unquoteStringLit(lits[0])}
		switch {
		case ac.ON() != nil:
			assignment.IsToggle, assignment.ToggleOn = true, true
		case ac.OFF() != nil:
			assignment.IsToggle, assignment.ToggleOn = true, false
		case len(lits) > 1:
			assignment.Value = unquoteStringLit(lits[1])
		}
		stmt.Assignments = append(stmt.Assignments, assignment)
	}

	b.statements = append(b.statements, stmt)
}
