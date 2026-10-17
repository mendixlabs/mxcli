// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// R4 (PROPOSAL_mdl_beta_syntax_freeze.md §3; ako/mxcli#751): every call site
// binds an argument as `Param = expression`, and every text template as
// `with ({1} = expression)`. The spellings below build exactly what the
// canonical form builds, so each is a registered alias (mdl/deprecation) whose
// use is recorded here, where its tokens are still at hand, together with the
// rewrite `fmt --upgrade` applies:
//
//   - MDL-DEPR006 `$Param = e` — the `$` of a variable on a parameter name;
//   - MDL-DEPR007 `Param: e`   — `:` sets a model property, `=` binds a value;
//   - MDL-DEPR008 a workflow call's `with (Param = '<expression>')`;
//   - MDL-DEPR009 `objects [a, b]` / `parameters [a, b]`.

// ExitCallArgument records `$Param = e` and `Param: e` in call
// microflow/nanoflow/java action/javascript action/external action/web
// service/execute database query, and refuses an argument by position.
func (b *Builder) ExitCallArgument(ctx *parser.CallArgumentContext) {
	if ctx.EQUALS() == nil && ctx.COLON() == nil {
		b.refusePositionalArgument(ctx.Expression(), "microflow.call")
		return
	}
	if ctx.COLON() != nil {
		b.recordColonArgument(ctx.ParameterName(), ctx.COLON(), ctx.Expression())
		return
	}
	if n := ctx.NOTHING(); n != nil && !isFlowCallArgument(ctx) {
		tok := n.GetSymbol()
		b.addError(fmt.Errorf("line %d:%d `= nothing` (a blank argument) is only valid in call microflow and "+
			"call nanoflow; bind the parameter to a value, or to `empty`",
			tok.GetLine(), tok.GetColumn()))
	}
	b.recordDollarArgument(ctx.VARIABLE())
}

// isFlowCallArgument reports whether a call argument belongs to a call
// microflow or call nanoflow — the two calls whose parameter mappings store a
// bare expression, where a blank one (`Argument: ""`) is a state Studio Pro
// writes when an argument field is left empty.
func isFlowCallArgument(ctx *parser.CallArgumentContext) bool {
	list := ctx.GetParent()
	if list == nil {
		return false
	}
	switch list.GetParent().(type) {
	case *parser.CallMicroflowStatementContext, *parser.CallNanoflowStatementContext:
		return true
	}
	return false
}

// ExitSendRestRequestParam records `$Param = e` in send rest request.
func (b *Builder) ExitSendRestRequestParam(ctx *parser.SendRestRequestParamContext) {
	b.recordDollarArgument(ctx.VARIABLE())
}

// ExitShowPageArg records the two deprecated argument spellings of show page.
func (b *Builder) ExitShowPageArg(ctx *parser.ShowPageArgContext) {
	if ctx.ParameterName() != nil {
		return
	}
	if ctx.EQUALS() == nil && ctx.COLON() == nil {
		b.refusePositionalArgument(ctx.Expression(), "microflow.show-page")
		return
	}
	if iok := ctx.IdentifierOrKeyword(); iok != nil {
		b.recordColonArgument(iok, ctx.COLON(), ctx.Expression())
		return
	}
	if vars := ctx.AllVARIABLE(); len(vars) > 0 {
		b.recordDollarArgument(vars[0])
	}
}

// ExitMicroflowArgV3 records the two deprecated argument spellings of a page
// action or a microflow/nanoflow data source.
func (b *Builder) ExitMicroflowArgV3(ctx *parser.MicroflowArgV3Context) {
	if ctx.ParameterName() != nil {
		return
	}
	if ctx.EQUALS() == nil && ctx.COLON() == nil {
		b.refusePositionalArgument(ctx.Expression(), "page.datasource (or page.action)")
		return
	}
	if iok := ctx.IdentifierOrKeyword(); iok != nil {
		b.recordColonArgument(iok, ctx.COLON(), ctx.Expression())
		return
	}
	b.recordDollarArgument(ctx.VARIABLE())
}

// recordDollarArgument records MDL-DEPR006 for the parameter-name token v, with
// the rewrite that drops its `$`.
func (b *Builder) recordDollarArgument(v antlr.TerminalNode) {
	if v == nil {
		return
	}
	tok := v.GetSymbol()
	name := strings.TrimPrefix(tok.GetText(), "$")
	b.recordDeprecation(deprecation.DollarArgumentName, tok, "argument")
	edit := ast.TextEdit{Start: tok.GetStart(), Stop: tok.GetStop() + 1, Text: ParameterNameSpelling(name)}
	b.fixLastDeprecation(deprecation.DollarArgumentName, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// refusePositionalArgument refuses an argument written by position, `M.F($O)`
// (ako/mxcli#533, #569). It is not an alias under any language version: which
// parameter it binds depends on the callee's declaration order, which a script
// cannot see. The grammar accepts it only so that the error lands on the
// argument itself and names the form that works; left to ANTLR, a page data
// source reported "no viable alternative" at the `DataSource` keyword.
func (b *Builder) refusePositionalArgument(expr parser.IExpressionContext, topic string) {
	if expr == nil || expr.GetStart() == nil {
		return
	}
	tok := expr.GetStart()
	text := expressionSourceText(expr)
	b.addError(fmt.Errorf("line %d:%d an argument is bound by name, not by position: write `Param = %s`, "+
		"where Param is the name of the parameter it is for (R4: `Param = expression` at every call site). "+
		"See: mxcli syntax %s", tok.GetLine(), tok.GetColumn(), text, topic))
}

// recordColonArgument records MDL-DEPR007 for `Param: e`, with the rewrite
// `Param = e`: the colon and the space around it become ` = `. name is the
// identifierOrKeyword or parameterName before the colon.
func (b *Builder) recordColonArgument(name antlr.ParserRuleContext, colon antlr.TerminalNode, expr parser.IExpressionContext) {
	if colon == nil || name == nil {
		return
	}
	b.recordDeprecation(deprecation.ColonArgument, colon.GetSymbol(), "argument")
	nameStop := name.GetStop()
	if nameStop == nil || expr == nil || expr.GetStart() == nil {
		b.fixLastDeprecation(deprecation.ColonArgument, nil, "the argument is incomplete")
		return
	}
	edit := replaceGap(nameStop.GetStop(), expr.GetStart().GetStart(), " = ")
	b.fixLastDeprecation(deprecation.ColonArgument, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// ParameterNameSpelling spells a parameter name as parameterName accepts it:
// bare when it parses as one (an identifier or a keyword), quoted otherwise.
func ParameterNameSpelling(name string) string {
	if parsesFully(name, func(p *parser.MDLParser) antlr.ParserRuleContext { return p.ParameterName() }) {
		return name
	}
	return `"` + name + `"`
}

// parsesFully reports whether text is exactly one rule, as rule parses it,
// with no syntax error and nothing left over.
func parsesFully(text string, rule func(*parser.MDLParser) antlr.ParserRuleContext) bool {
	_, ok := parseRule(text, rule)
	return ok
}

func parseRule(text string, rule func(*parser.MDLParser) antlr.ParserRuleContext) (antlr.ParserRuleContext, bool) {
	errs := newErrorListener()
	lexer := parser.NewMDLLexer(newScriptStream(text, langver.V0))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errs)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewMDLParser(stream)
	p.RemoveErrorListeners()
	p.AddErrorListener(errs)
	ctx := rule(p)
	if len(errs.errors) > 0 || ctx == nil || stream.LA(1) != antlr.TokenEOF {
		return nil, false
	}
	return ctx, true
}

// BareExpression reports whether a stored expression can be written as a bare
// MDL expression that the visitor reads back as exactly that text. describe
// asks it before writing a workflow argument without quotes; fmt --upgrade
// asks it before unquoting one.
func BareExpression(expr string) bool {
	if strings.TrimSpace(expr) != expr || expr == "" {
		return false
	}
	ctx, ok := parseRule(expr, func(p *parser.MDLParser) antlr.ParserRuleContext { return p.Expression() })
	if !ok {
		return false
	}
	return bareArgumentText(ctx.(parser.IExpressionContext)) == expr
}

// ParsesAsExpression reports whether text is one MDL expression, read with the
// same grammar rule as a bare `Visible: <expr>` / `Editable: <expr>`. check asks
// it of a quoted condition, whose text the parser otherwise never sees.
func ParsesAsExpression(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	_, ok := parseRule(text, func(p *parser.MDLParser) antlr.ParserRuleContext { return p.Expression() })
	return ok
}

// bareArgumentText is the expression a bare workflow argument stores: its
// source text, as the Mendix expression editor would hold it. Unlike a
// microflow expression stored as written (buildSourceExpression), a string in
// it is not respelled under mdl 0: workflow and widget expressions are stored
// as written in either language, and describe writes them back as stored.
func bareArgumentText(expr parser.IExpressionContext) string {
	return stripExpressionIdentifierQuotes(expressionSourceText(expr))
}

// buildWorkflowCallArguments builds the canonical `(Param = expression)` list
// of a workflow call.
func buildWorkflowCallArguments(ctx parser.IWorkflowCallArgumentsContext) []ast.WorkflowParameterMappingNode {
	if ctx == nil {
		return nil
	}
	var out []ast.WorkflowParameterMappingNode
	for _, a := range ctx.(*parser.WorkflowCallArgumentsContext).AllWorkflowCallArgument() {
		arg := a.(*parser.WorkflowCallArgumentContext)
		if arg.ParameterName() == nil || arg.Expression() == nil {
			continue
		}
		out = append(out, ast.WorkflowParameterMappingNode{
			Parameter:  parameterNameText(arg.ParameterName()),
			Expression: bareArgumentText(arg.Expression()),
		})
	}
	return out
}

// workflowCallCtx is what the two workflow call statements have in common.
type workflowCallCtx interface {
	antlr.ParserRuleContext
	QualifiedName() parser.IQualifiedNameContext
	WorkflowCallArguments() parser.IWorkflowCallArgumentsContext
	WITH() antlr.TerminalNode
	RPAREN() antlr.TerminalNode
	AllWorkflowParameterMapping() []parser.IWorkflowParameterMappingContext
}

// ExitWorkflowCallMicroflowStmt records MDL-DEPR008 on a workflow's call
// microflow activity.
func (b *Builder) ExitWorkflowCallMicroflowStmt(ctx *parser.WorkflowCallMicroflowStmtContext) {
	b.recordWorkflowStringArguments(ctx)
}

// ExitWorkflowCallWorkflowStmt records MDL-DEPR008 on a workflow's call
// workflow activity.
func (b *Builder) ExitWorkflowCallWorkflowStmt(ctx *parser.WorkflowCallWorkflowStmtContext) {
	b.recordWorkflowStringArguments(ctx)
}

func (b *Builder) recordWorkflowStringArguments(ctx workflowCallCtx) {
	with := ctx.WITH()
	if with == nil {
		return
	}
	if ctx.WorkflowCallArguments() != nil {
		b.addError(fmt.Errorf("line %d: a workflow call has two argument lists; write the arguments once, "+
			"as `(Param = expression)` after the callee", with.GetSymbol().GetLine()))
		return
	}
	b.recordDeprecation(deprecation.WorkflowStringArgument, with.GetSymbol(), "workflow call")
	fix, why := workflowStringArgumentsFix(ctx)
	b.fixLastDeprecation(deprecation.WorkflowStringArgument, fix, why)
}

// workflowStringArgumentsFix moves `with (P = 'e', …)` to `(P = e, …)` after
// the callee. A string whose content is not a bare expression that reads back
// as itself has no rewrite: unquoting it would change the stored expression.
func workflowStringArgumentsFix(ctx workflowCallCtx) (*ast.Fix, string) {
	qn, rparen := ctx.QualifiedName(), ctx.RPAREN()
	if qn == nil || qn.GetStop() == nil || rparen == nil {
		return nil, "the argument list is incomplete"
	}
	var args []string
	for _, m := range ctx.AllWorkflowParameterMapping() {
		pm := m.(*parser.WorkflowParameterMappingContext)
		if pm.QualifiedName() == nil || pm.STRING_LITERAL() == nil {
			return nil, "the argument list is incomplete"
		}
		expr := unquoteStringLit(pm.STRING_LITERAL())
		if !BareExpression(expr) {
			return nil, fmt.Sprintf("the string %s does not read back as the same bare expression; "+
				"write the argument as `(Param = expression)` by hand", pm.STRING_LITERAL().GetText())
		}
		name := bareWorkflowParameterName(pm.QualifiedName().GetText())
		args = append(args, ParameterNameSpelling(name)+" = "+expr)
	}
	// Delete ` with (…)` from the end of the token before WITH, and write the
	// list after the callee.
	withTok := ctx.WITH().GetSymbol()
	prevStop := -1
	for i := 0; i < ctx.GetChildCount(); i++ {
		if t, ok := ctx.GetChild(i).(antlr.TerminalNode); ok && t.GetSymbol() == withTok {
			break
		}
		if _, stop := nodeSpan(ctx.GetChild(i)); stop >= 0 {
			prevStop = stop
		}
	}
	if prevStop < 0 {
		return nil, "the argument list is incomplete"
	}
	return &ast.Fix{Edits: []ast.TextEdit{
		insertAt(qn.GetStop().GetStop()+1, "("+strings.Join(args, ", ")+")"),
		{Start: prevStop + 1, Stop: rparen.GetSymbol().GetStop() + 1, Text: ""},
	}}, ""
}

// ExitShowMessageStatement records MDL-DEPR009 for `objects [..]` and checks
// the numbering of `with ({n} = …)`.
func (b *Builder) ExitShowMessageStatement(ctx *parser.ShowMessageStatementContext) {
	b.recordObjectsList(ctx.OBJECTS(), ctx.LBRACKET(), ctx.ExpressionList(), ctx.RBRACKET())
	b.checkTemplateNumbering(ctx.TemplateParams())
	if msg := ctx.Expression(); msg != nil {
		after := msg.(antlr.ParserRuleContext).GetStop()
		if t := ctx.IdentifierOrKeyword(); t != nil {
			after = t.GetStop()
		}
		b.noteTemplateLineBreak(msg, ctx.TemplateParams() != nil || ctx.OBJECTS() != nil, after, ctx.SHOW().GetSymbol())
	}
}

// ExitValidationFeedbackStatement is ExitShowMessageStatement for validation
// feedback.
func (b *Builder) ExitValidationFeedbackStatement(ctx *parser.ValidationFeedbackStatementContext) {
	b.recordObjectsList(ctx.OBJECTS(), ctx.LBRACKET(), ctx.ExpressionList(), ctx.RBRACKET())
	b.checkTemplateNumbering(ctx.TemplateParams())
	if msg := ctx.Expression(); msg != nil {
		b.noteTemplateLineBreak(msg, ctx.TemplateParams() != nil || ctx.OBJECTS() != nil,
			msg.(antlr.ParserRuleContext).GetStop(), ctx.VALIDATION().GetSymbol())
	}
}

func (b *Builder) recordObjectsList(objects, lbracket antlr.TerminalNode, list parser.IExpressionListContext, rbracket antlr.TerminalNode) {
	if objects == nil {
		return
	}
	b.recordDeprecation(deprecation.PositionalTemplateArguments, objects.GetSymbol(), "text template")
	if lbracket == nil || rbracket == nil || list == nil {
		b.fixLastDeprecation(deprecation.PositionalTemplateArguments, nil, "the list is incomplete")
		return
	}
	var items []antlr.ParserRuleContext
	for _, e := range list.(*parser.ExpressionListContext).AllExpression() {
		items = append(items, e)
	}
	b.fixLastDeprecation(deprecation.PositionalTemplateArguments,
		numberedListFix(objects.GetSymbol(), lbracket.GetSymbol(), items, rbracket.GetSymbol()), "")
}

// numberedListFix rewrites `kw [a, b]` as `with ({1} = a, {2} = b)`: the
// keyword and bracket become `with (`, each item gets its number, and the
// closing bracket becomes `)`. The items themselves are not touched.
func numberedListFix(kw, lbracket antlr.Token, items []antlr.ParserRuleContext, rbracket antlr.Token) *ast.Fix {
	edits := []ast.TextEdit{replaceSpan(kw, lbracket, keywordLike(kw.GetText(), "with")+" (")}
	for i, it := range items {
		if it.GetStart() == nil {
			return nil
		}
		edits = append(edits, insertAt(it.GetStart().GetStart(), fmt.Sprintf("{%d} = ", i+1)))
	}
	edits = append(edits, replaceSpan(rbracket, rbracket, ")"))
	return &ast.Fix{Edits: edits}
}

// ExitTemplateParams records MDL-DEPR009 for `parameters [..]`.
func (b *Builder) ExitTemplateParams(ctx *parser.TemplateParamsContext) {
	kw := ctx.PARAMETERS()
	if kw == nil {
		return
	}
	b.recordDeprecation(deprecation.PositionalTemplateArguments, kw.GetSymbol(), "text template")
	arr, ok := ctx.ArrayLiteral().(*parser.ArrayLiteralContext)
	if !ok || arr == nil || arr.LBRACKET() == nil || arr.RBRACKET() == nil {
		b.fixLastDeprecation(deprecation.PositionalTemplateArguments, nil, "the list is incomplete")
		return
	}
	var items []antlr.ParserRuleContext
	for _, l := range arr.AllLiteral() {
		items = append(items, l)
	}
	if len(items) == 0 {
		// `parameters []` binds nothing: the clause goes, with the space before it.
		if prev := previousTokenStop(ctx); prev >= 0 {
			edit := ast.TextEdit{Start: prev + 1, Stop: arr.RBRACKET().GetSymbol().GetStop() + 1}
			b.fixLastDeprecation(deprecation.PositionalTemplateArguments, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
		}
		return
	}
	b.fixLastDeprecation(deprecation.PositionalTemplateArguments,
		numberedListFix(kw.GetSymbol(), arr.LBRACKET().GetSymbol(), items, arr.RBRACKET().GetSymbol()), "")
}

// previousTokenStop is the rune offset of the last rune before ctx's first
// token that is not hidden, or -1.
func previousTokenStop(ctx antlr.ParserRuleContext) int {
	start := ctx.GetStart()
	if start == nil || start.GetInputStream() == nil {
		return -1
	}
	return start.GetStart() - 1 - leadingSpace(start)
}

// leadingSpace counts the whitespace runes right before tok.
func leadingSpace(tok antlr.Token) int {
	in := tok.GetInputStream()
	n := 0
	for i := tok.GetStart() - 1; i >= 0; i-- {
		r := in.GetText(i, i)
		if r != " " && r != "\t" && r != "\n" && r != "\r" {
			break
		}
		n++
	}
	return n
}

// checkTemplateNumbering refuses a `with ({n} = …)` list on a show message or
// validation feedback whose numbers are not 1..N, each once: the model holds
// the arguments as a list, so `{3}` with no `{2}` has no place to go.
func (b *Builder) checkTemplateNumbering(ctx parser.ITemplateParamsContext) {
	if ctx == nil {
		return
	}
	tp := ctx.(*parser.TemplateParamsContext)
	if tp.WITH() == nil {
		return
	}
	var nums []int
	for _, p := range tp.AllTemplateParam() {
		if n := p.(*parser.TemplateParamContext).NUMBER_LITERAL(); n != nil {
			var v int
			fmt.Sscanf(n.GetText(), "%d", &v)
			nums = append(nums, v)
		}
	}
	sort.Ints(nums)
	for i, n := range nums {
		if n != i+1 {
			b.addError(fmt.Errorf("line %d: the placeholders of a text template are numbered {1} to {%d}, "+
				"each once; got %v", tp.GetStart().GetLine(), len(nums), nums))
			return
		}
	}
}

// templateArgsByNumber orders a `with ({n} = …)` list by its numbers, the
// positional list a show message or validation feedback stores.
func templateArgsByNumber(params []ast.TemplateParam) []ast.Expression {
	sorted := append([]ast.TemplateParam(nil), params...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Index < sorted[j].Index })
	out := make([]ast.Expression, 0, len(sorted))
	for _, p := range sorted {
		out = append(out, p.Value)
	}
	return out
}
