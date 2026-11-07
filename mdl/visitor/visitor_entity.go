// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

func (b *Builder) ExitCreateEntityStatement(ctx *parser.CreateEntityStatementContext) {
	// Handle VIEW entities separately
	if ctx.VIEW() != nil {
		b.buildViewEntity(ctx)
		return
	}

	stmt := &ast.CreateEntityStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
		Kind: ast.EntityPersistent, // Default
	}

	// Entity type
	if ctx.NON_PERSISTENT() != nil {
		stmt.Kind = ast.EntityNonPersistent
	}
	stmt.KindStated = ctx.PERSISTENT() != nil || ctx.NON_PERSISTENT() != nil

	// Navigate to parent CreateStatement to get annotations and doc comment
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		// REPLACE is treated as MODIFY for entities — changing the UUID would drop the database table.
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}

		// Extract annotations (@Position, etc.)
		for _, annCtx := range createStmt.AllAnnotation() {
			ann := annCtx.(*parser.AnnotationContext)
			annName := ann.AnnotationName().GetText()
			if strings.EqualFold(annName, "Position") {
				// @Position(x, y)
				if params := ann.AnnotationParams(); params != nil {
					paramsCtx := params.(*parser.AnnotationParamsContext)
					allParams := paramsCtx.AllAnnotationParam()
					if len(allParams) >= 2 {
						x := parseAnnotationParamInt(allParams[0])
						y := parseAnnotationParamInt(allParams[1])
						stmt.Position = &ast.Position{X: x, Y: y}
					}
				}
			}
		}

	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)

	// Generalization clause (EXTENDS/GENERALIZATION before entity body)
	if genClause := ctx.GeneralizationClause(); genClause != nil {
		genCtx := genClause.(*parser.GeneralizationClauseContext)
		genName := buildQualifiedName(genCtx.QualifiedName())
		stmt.Generalization = &genName
	}

	// Entity body (attributes, options)
	if body := ctx.EntityBody(); body != nil {
		bodyCtx := body.(*parser.EntityBodyContext)

		// Attributes
		if attrList := bodyCtx.AttributeDefinitionList(); attrList != nil {
			stmt.Attributes = buildAttributes(attrList, b)
		}

		// Options (extends, indexes, system attributes, etc.)
		if opts := bodyCtx.EntityOptions(); opts != nil {
			optsCtx := opts.(*parser.EntityOptionsContext)
			for _, opt := range optsCtx.AllEntityOption() {
				optCtx := opt.(*parser.EntityOptionContext)
				// Handle INDEX option
				if optCtx.INDEX() != nil && optCtx.IndexDefinition() != nil {
					stmt.Indexes = append(stmt.Indexes, buildIndex(optCtx.IndexDefinition()))
				}
				// Handle event handler option (ON BEFORE/AFTER ... CALL ...)
				if ehCtx := optCtx.EventHandlerDefinition(); ehCtx != nil {
					if eh := buildEventHandler(ehCtx); eh != nil {
						stmt.EventHandlers = append(stmt.EventHandlers, *eh)
					}
				}
				if el := optCtx.MicroflowExportLevelClause(); el != nil {
					stmt.ExportLevel = exportLevelOf(el.(*parser.MicroflowExportLevelClauseContext))
				}
			}
		}
	}

	b.statements = append(b.statements, stmt)
}

// buildEventHandler converts an eventHandlerDefinition parse tree to an ast.EventHandlerDef.
func buildEventHandler(ctx parser.IEventHandlerDefinitionContext) *ast.EventHandlerDef {
	if ctx == nil {
		return nil
	}
	ehCtx := ctx.(*parser.EventHandlerDefinitionContext)
	eh := &ast.EventHandlerDef{
		PassEventObject: true, // default
	}
	if m := ehCtx.EventMoment(); m != nil {
		eh.Moment = strings.Title(strings.ToLower(m.GetText()))
	}
	if t := ehCtx.EventType(); t != nil {
		txt := strings.ToUpper(t.GetText())
		switch txt {
		case "CREATE":
			eh.Event = "Create"
		case "COMMIT":
			eh.Event = "Commit"
		case "DELETE":
			eh.Event = "Delete"
		case "ROLLBACK":
			eh.Event = "RollBack"
		}
	}
	if qn := ehCtx.QualifiedName(); qn != nil {
		eh.Microflow = buildQualifiedName(qn)
	}
	// Handle optional parentheses: () = no object, ($var) = pass object, no parens = pass object (default)
	if ehCtx.LPAREN() != nil {
		if ehCtx.VARIABLE() != nil {
			eh.PassEventObject = true
		} else {
			// Empty parens () = don't pass object
			eh.PassEventObject = false
		}
	}
	if ehCtx.RAISE() != nil {
		eh.RaiseErrorOnFalse = true
	}
	return eh
}

// buildViewEntity handles CREATE VIEW ENTITY statements.
func (b *Builder) buildViewEntity(ctx *parser.CreateEntityStatementContext) {
	stmt := &ast.CreateViewEntityStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// Navigate to parent CreateStatement to get annotations and doc comment
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		// Check for CREATE OR MODIFY / CREATE OR REPLACE
		// `or replace` means `or modify` from mdl 1 on; under mdl 0 it keeps
		// its delete-and-recreate meaning (viewEntityReplaceIsModify).
		if createStmt.OR() != nil {
			if createStmt.MODIFY() != nil || b.replaceMeansModify(createStmt) {
				stmt.CreateOrModify = true
			} else if createStmt.REPLACE() != nil {
				stmt.CreateOrReplace = true
			}
		}

		// Extract annotations (@Position, etc.)
		for _, annCtx := range createStmt.AllAnnotation() {
			ann := annCtx.(*parser.AnnotationContext)
			annName := ann.AnnotationName().GetText()
			if strings.EqualFold(annName, "Position") {
				if params := ann.AnnotationParams(); params != nil {
					paramsCtx := params.(*parser.AnnotationParamsContext)
					allParams := paramsCtx.AllAnnotationParam()
					if len(allParams) >= 2 {
						x := parseAnnotationParamInt(allParams[0])
						y := parseAnnotationParamInt(allParams[1])
						stmt.Position = &ast.Position{X: x, Y: y}
					}
				}
			}
		}

	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)

	// Entity body (view attributes)
	if body := ctx.EntityBody(); body != nil {
		bodyCtx := body.(*parser.EntityBodyContext)
		if attrList := bodyCtx.AttributeDefinitionList(); attrList != nil {
			stmt.Attributes = buildViewAttributes(attrList)
		}
		if opts := bodyCtx.EntityOptions(); opts != nil {
			for _, opt := range opts.(*parser.EntityOptionsContext).AllEntityOption() {
				if el := opt.(*parser.EntityOptionContext).MicroflowExportLevelClause(); el != nil {
					stmt.ExportLevel = exportLevelOf(el.(*parser.MicroflowExportLevelClauseContext))
				}
			}
		}
	}

	// OQL Query - use token stream to preserve whitespace, and walk parse tree for structured data
	if oqlCtx := ctx.OqlQuery(); oqlCtx != nil {
		raw := viewQueryText(ctx, oqlCtx)
		stmt.Query = ast.OQLQuery{
			RawQuery: raw,
			Parsed:   buildOQLParsed(oqlCtx.(*parser.OqlQueryContext), raw),
		}
	}

	b.statements = append(b.statements, stmt)
}

// viewQueryText is the text a view entity stores for its query: from the query's
// first token to its last, widened over any comment between `as (` and the
// first token and between the last token and `)`, then dedented (#653).
//
// The lexer skips comments, so the rule's own token span cannot see them: a
// comment heading or closing the query was dropped from every script, and from
// every describe → exec into another project, where there is no stored text to
// keep (ako/mxcli#1632). Only comments widen the span — the whitespace outside
// it is the script's layout, which is what the dedent already accounts for.
func viewQueryText(ctx *parser.CreateEntityStatementContext, oqlCtx parser.IOqlQueryContext) string {
	q := oqlCtx.(antlr.ParserRuleContext)
	start, stop := q.GetStart(), q.GetStop()
	if start == nil || stop == nil || start.GetInputStream() == nil ||
		start.GetStart() < 0 || stop.GetStop() < start.GetStart() {
		firstIndent, firstIsIndent := leadingLineWhitespace(q)
		return dedentOQL(extractOriginalText(q), firstIndent, firstIsIndent)
	}
	is := start.GetInputStream()
	from, to := start.GetStart(), stop.GetStop()

	// Before the query: back to the `(`, or to `as` when there is none.
	open := ctx.LPAREN()
	if open == nil {
		open = ctx.AS()
	}
	if open != nil {
		if p := open.GetSymbol().GetStop() + 1; p < from {
			if c := firstOuterComment([]rune(is.GetText(p, from-1))); c >= 0 {
				from = p + c
			}
		}
	}
	// After it: up to the `)`. Without one the statement's terminator follows
	// directly, so there is no region a comment could sit in.
	if closing := ctx.RPAREN(); closing != nil {
		if p := closing.GetSymbol().GetStart() - 1; p > to {
			if e := lastOuterCommentEnd([]rune(is.GetText(to+1, p))); e >= 0 {
				to = to + 1 + e
			}
		}
	}

	firstIndent, firstIsIndent := lineIndentBefore(is, from)
	return dedentOQL(is.GetText(from, to), firstIndent, firstIsIndent)
}

// firstOuterComment returns the index of the first comment in a region that
// holds only whitespace and comments, or -1 when it holds no comment or holds
// anything else (then the span is left as it was).
func firstOuterComment(region []rune) int {
	for i := 0; i < len(region); i++ {
		switch {
		case isOQLSpace(region[i]):
			continue
		case startsComment(region, i):
			return i
		default:
			return -1
		}
	}
	return -1
}

// lastOuterCommentEnd returns the index of the last character of the last
// comment in a region that holds only whitespace and comments, or -1.
func lastOuterCommentEnd(region []rune) int {
	last := -1
	for i := 0; i < len(region); {
		switch {
		case isOQLSpace(region[i]):
			i++
		case i+1 < len(region) && region[i] == '-' && region[i+1] == '-':
			j := i
			for j < len(region) && region[j] != '\n' && region[j] != '\r' {
				if !isOQLSpace(region[j]) {
					last = j
				}
				j++
			}
			i = j
		case i+1 < len(region) && region[i] == '/' && region[i+1] == '*':
			end := -1
			for j := i + 2; j+1 < len(region); j++ {
				if region[j] == '*' && region[j+1] == '/' {
					end = j + 1
					break
				}
			}
			if end < 0 {
				return -1
			}
			last, i = end, end+1
		default:
			return -1
		}
	}
	return last
}

func startsComment(r []rune, i int) bool {
	return i+1 < len(r) && ((r[i] == '-' && r[i+1] == '-') || (r[i] == '/' && r[i+1] == '*'))
}

func isOQLSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '\v' || r == '\f'
}

// lineIndentBefore returns the text between the start of pos's line and pos,
// and whether it is all whitespace — leadingLineWhitespace for a position that
// is not a token's.
func lineIndentBefore(is antlr.CharStream, pos int) (string, bool) {
	lineStart := pos
	for lineStart > 0 && is.GetText(lineStart-1, lineStart-1) != "\n" {
		lineStart--
	}
	if lineStart == pos {
		return "", true
	}
	prefix := is.GetText(lineStart, pos-1)
	return prefix, strings.TrimLeft(prefix, " \t") == ""
}

// leadingLineWhitespace returns the text between the start of the line holding
// ctx's first token and that token, and whether it is all whitespace. It is the
// indentation of the query's first line, which extractOriginalText drops.
func leadingLineWhitespace(ctx antlr.ParserRuleContext) (string, bool) {
	start := ctx.GetStart()
	if start == nil || start.GetInputStream() == nil || start.GetStart() < 0 {
		return "", false
	}
	col := start.GetColumn()
	if col <= 0 {
		return "", true
	}
	prefix := start.GetInputStream().GetText(start.GetStart()-col, start.GetStart()-1)
	return prefix, strings.TrimLeft(prefix, " \t") == ""
}

// dedentOQL removes the indentation a view entity's query carries only because
// of where it sits in the script, so what is stored is the query itself.
//
// DESCRIBE indents every stored line by two spaces inside `as (…)`. The source
// text starts at the first token, so line 1 arrives without its indentation and
// lines 2…n with all of it; storing that verbatim added two spaces to lines 2…n
// on every describe → exec cycle (ako/mxcli#653). The common leading-whitespace
// prefix of the non-blank lines — line 1 counted at its position in the script
// when only whitespace precedes it — is stripped from lines 2…n. Nothing else
// is touched: relative indentation, comments and line breaks are the author's.
func dedentOQL(raw string, firstIndent string, firstIsIndent bool) string {
	lines := strings.Split(raw, "\n")
	if len(lines) < 2 {
		return raw
	}
	common, have := "", false
	if firstIsIndent {
		common, have = firstIndent, true
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || trimmed == "\r" {
			continue
		}
		indent := line[:len(line)-len(trimmed)]
		if !have {
			common, have = indent, true
			continue
		}
		n := 0
		for n < len(common) && n < len(indent) && common[n] == indent[n] {
			n++
		}
		common = common[:n]
	}
	if common == "" {
		return raw
	}
	for i := 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], common) {
			lines[i] = lines[i][len(common):]
		} else if strings.HasPrefix(common, lines[i]) {
			lines[i] = "" // a blank line shorter than the indentation
		}
	}
	return strings.Join(lines, "\n")
}

// buildOQLParsed walks the ANTLR OQL parse tree and returns a structured OQLParsed.
func buildOQLParsed(ctx *parser.OqlQueryContext, rawQuery string) *ast.OQLParsed {
	if ctx == nil {
		return nil
	}

	parsed := &ast.OQLParsed{
		RawQuery: rawQuery,
	}

	// Take the first query term (UNION queries not supported for plan visualization)
	terms := ctx.AllOqlQueryTerm()
	if len(terms) == 0 {
		return parsed
	}
	term := terms[0].(*parser.OqlQueryTermContext)

	// SELECT clause
	if selCtx := term.SelectClause(); selCtx != nil {
		selClause := selCtx.(*parser.SelectClauseContext)
		if listCtx := selClause.SelectList(); listCtx != nil {
			selList := listCtx.(*parser.SelectListContext)
			if selList.STAR() != nil {
				parsed.Select = append(parsed.Select, ast.OQLSelectItem{Expression: "*"})
			} else {
				for _, itemCtx := range selList.AllSelectItem() {
					item := itemCtx.(*parser.SelectItemContext)
					parsed.Select = append(parsed.Select, buildOQLSelectItem(item))
				}
			}
		}
	}

	// FROM clause + JOINs
	if fromCtx := term.FromClause(); fromCtx != nil {
		fromClause := fromCtx.(*parser.FromClauseContext)

		// Primary table
		if tableRef := fromClause.TableReference(); tableRef != nil {
			parsed.Tables = append(parsed.Tables, buildOQLFromTable(tableRef.(*parser.TableReferenceContext)))
		}

		// JOIN clauses
		for _, joinCtx := range fromClause.AllJoinClause() {
			join := joinCtx.(*parser.JoinClauseContext)
			parsed.Tables = append(parsed.Tables, buildOQLJoinTable(join))
		}
	}

	// WHERE clause
	if whereCtx := term.WhereClause(); whereCtx != nil {
		wc := whereCtx.(*parser.WhereClauseContext)
		if expr := wc.Expression(); expr != nil {
			parsed.Where = extractOriginalText(expr.(antlr.ParserRuleContext))
		}
	}

	// GROUP BY clause
	if groupCtx := term.GroupByClause(); groupCtx != nil {
		gc := groupCtx.(*parser.GroupByClauseContext)
		if exprList := gc.ExpressionList(); exprList != nil {
			parsed.GroupBy = extractOriginalText(exprList.(antlr.ParserRuleContext))
		}
	}

	// HAVING clause
	if havingCtx := term.HavingClause(); havingCtx != nil {
		hc := havingCtx.(*parser.HavingClauseContext)
		if expr := hc.Expression(); expr != nil {
			parsed.Having = extractOriginalText(expr.(antlr.ParserRuleContext))
		}
	}

	// ORDER BY clause
	if orderCtx := term.OrderByClause(); orderCtx != nil {
		parsed.OrderBy = extractOriginalText(orderCtx.(antlr.ParserRuleContext))
	}

	return parsed
}

// buildOQLSelectItem converts a SelectItemContext to an OQLSelectItem.
func buildOQLSelectItem(ctx *parser.SelectItemContext) ast.OQLSelectItem {
	item := ast.OQLSelectItem{}

	if ctx.AggregateFunction() != nil {
		item.Expression = extractOriginalText(ctx.AggregateFunction().(antlr.ParserRuleContext))
		item.IsAggregate = true
	} else if ctx.Expression() != nil {
		item.Expression = extractOriginalText(ctx.Expression().(antlr.ParserRuleContext))
		// Check for scalar subquery inside the expression
		if oqlCtx := findOqlQueryInExpression(ctx.Expression().(antlr.ParserRuleContext)); oqlCtx != nil {
			subRaw := extractOriginalText(oqlCtx)
			item.Subquery = buildOQLParsed(oqlCtx, subRaw)
			item.SubqueryText = subRaw
		}
	}

	if ctx.SelectAlias() != nil {
		item.Alias = ctx.SelectAlias().GetText()
	}

	return item
}

// findOqlQueryInExpression recursively walks an expression parse tree to find
// a scalar subquery (oqlQuery). Returns the first OqlQueryContext found, or nil.
func findOqlQueryInExpression(ctx antlr.ParserRuleContext) *parser.OqlQueryContext {
	for i := 0; i < ctx.GetChildCount(); i++ {
		child := ctx.GetChild(i)
		if oqlCtx, ok := child.(*parser.OqlQueryContext); ok {
			return oqlCtx
		}
		if childRule, ok := child.(antlr.ParserRuleContext); ok {
			if found := findOqlQueryInExpression(childRule); found != nil {
				return found
			}
		}
	}
	return nil
}

// buildOQLFromTable converts a TableReferenceContext into an OQLTableRef for the FROM table.
func buildOQLFromTable(ctx *parser.TableReferenceContext) ast.OQLTableRef {
	ref := ast.OQLTableRef{
		JoinType: "from",
	}

	if qn := ctx.QualifiedName(); qn != nil {
		ref.Entity = getQualifiedNameText(qn)
	} else if oqlCtx := ctx.OqlQuery(); oqlCtx != nil {
		// Subquery: (FROM ... SELECT ...) AS alias
		subRaw := extractOriginalText(oqlCtx.(antlr.ParserRuleContext))
		sub := buildOQLParsed(oqlCtx.(*parser.OqlQueryContext), subRaw)
		ref.Subquery = sub
		ref.SubqueryText = subRaw
		// Use the subquery's first table entity as a fallback display entity
		if sub != nil && len(sub.Tables) > 0 {
			ref.Entity = sub.Tables[0].Entity
		}
	}

	ref.Alias = oqlSourceAliasText(ctx.OqlSourceAlias())

	return ref
}

// oqlSourceAliasText returns the alias an oqlSourceAlias names, without the AS.
func oqlSourceAliasText(ctx parser.IOqlSourceAliasContext) string {
	if ctx == nil {
		return ""
	}
	a := ctx.(*parser.OqlSourceAliasContext)
	if id := a.IDENTIFIER(); id != nil {
		return id.GetText()
	}
	if kw := a.Keyword(); kw != nil {
		return kw.GetText()
	}
	return ""
}

// buildOQLJoinTable converts a JoinClauseContext into an OQLTableRef.
func buildOQLJoinTable(ctx *parser.JoinClauseContext) ast.OQLTableRef {
	ref := ast.OQLTableRef{
		JoinType: buildOQLJoinType(ctx.JoinType()),
	}

	if tableRef := ctx.TableReference(); tableRef != nil {
		// Standard JOIN with table reference (entity or subquery)
		tr := tableRef.(*parser.TableReferenceContext)
		if qn := tr.QualifiedName(); qn != nil {
			ref.Entity = getQualifiedNameText(qn)
		} else if oqlCtx := tr.OqlQuery(); oqlCtx != nil {
			// Subquery in JOIN: JOIN (FROM ... SELECT ...) AS alias
			subRaw := extractOriginalText(oqlCtx.(antlr.ParserRuleContext))
			sub := buildOQLParsed(oqlCtx.(*parser.OqlQueryContext), subRaw)
			ref.Subquery = sub
			ref.SubqueryText = subRaw
			if sub != nil && len(sub.Tables) > 0 {
				ref.Entity = sub.Tables[0].Entity
			}
		}
		ref.Alias = oqlSourceAliasText(tr.OqlSourceAlias())
	} else if assocPath := ctx.AssociationPath(); assocPath != nil {
		// Association path JOIN
		ap := assocPath.(*parser.AssociationPathContext)
		ref.AssocPath = extractOriginalText(ap)

		// Extract entity from the last qualified name in the path
		qualNames := ap.AllQualifiedName()
		if len(qualNames) > 0 {
			lastQN := qualNames[len(qualNames)-1]
			ref.Entity = getQualifiedNameText(lastQN)
		}

		// Alias is on the JoinClause (not on AssociationPath)
		ref.Alias = oqlSourceAliasText(ctx.OqlSourceAlias())
	}

	// ON condition
	if ctx.ON() != nil && ctx.Expression() != nil {
		ref.OnExpr = extractOriginalText(ctx.Expression().(antlr.ParserRuleContext))
	}

	return ref
}

// buildOQLJoinType converts a JoinTypeContext to a canonical join type string.
func buildOQLJoinType(ctx parser.IJoinTypeContext) string {
	if ctx == nil {
		return "join"
	}
	jt := ctx.(*parser.JoinTypeContext)

	if jt.LEFT() != nil {
		return "left join"
	}
	if jt.RIGHT() != nil {
		return "right join"
	}
	if jt.FULL() != nil {
		return "full join"
	}
	if jt.CROSS() != nil {
		return "cross join"
	}
	if jt.INNER() != nil {
		return "join"
	}
	return "join"
}

// buildViewAttributes converts attribute definitions to ViewAttribute slice.
func buildViewAttributes(attrList parser.IAttributeDefinitionListContext) []ast.ViewAttribute {
	if attrList == nil {
		return nil
	}
	listCtx := attrList.(*parser.AttributeDefinitionListContext)
	var attrs []ast.ViewAttribute
	for _, attrDef := range listCtx.AllAttributeDefinition() {
		defCtx := attrDef.(*parser.AttributeDefinitionContext)
		// Nil check for AttributeName (can be nil on parse errors)
		if defCtx.AttributeName() == nil {
			continue
		}
		attr := ast.ViewAttribute{
			Name: attributeNameText(defCtx.AttributeName()),
		}
		if dt := defCtx.DataType(); dt != nil {
			attr.Type = buildDataType(dt)
		}
		for _, c := range defCtx.AllAttributeConstraint() {
			cc := c.(*parser.AttributeConstraintContext)
			applyLocalizeConstraint(&attr.Type, cc)
			if el := cc.MicroflowExportLevelClause(); el != nil {
				attr.ExportLevel = exportLevelOf(el.(*parser.MicroflowExportLevelClauseContext))
			}
		}
		attrs = append(attrs, attr)
	}
	return attrs
}

// findDocCommentText extracts the documentation comment text for a create statement.
// The docComment can appear either on the createStatement itself or on the parent statement rule.
func findDocCommentText(ctx antlr.RuleContext) string {
	text, _ := findDocComment(ctx)
	return text
}

// findDocComment returns the statement's doc comment and whether one was
// written at all. The two are different facts and the caller needs both:
// an ABSENT comment on a rewrite must preserve whatever is stored, while an
// explicitly EMPTY one (`/** */`) must clear it.
//
// Without the second return value they are indistinguishable — both yield "" —
// and a rewrite that says nothing about documentation silently deletes it
// (mendixlabs/mxcli#1018). There is no `ALTER MICROFLOW … SET DOCUMENTATION` to
// fall back on, so the empty comment is the only clearing spelling available.
func findDocComment(ctx antlr.RuleContext) (string, bool) {
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if docCtx := createStmt.DocComment(); docCtx != nil {
			return extractDocComment(docCtx.GetText()), true
		}
	}
	stmtCtx := findParentStatement(ctx)
	if stmtCtx != nil {
		if docCtx := stmtCtx.DocComment(); docCtx != nil {
			return extractDocComment(docCtx.GetText()), true
		}
	}
	return "", false
}

// findParentCreateStatement navigates up the parse tree to find the CreateStatement parent.
func findParentCreateStatement(ctx antlr.RuleContext) *parser.CreateStatementContext {
	parent := ctx.GetParent()
	for parent != nil {
		if createStmt, ok := parent.(*parser.CreateStatementContext); ok {
			return createStmt
		}
		parent = parent.GetParent()
	}
	return nil
}

// findParentStatement navigates up the parse tree to find the Statement parent.
func findParentStatement(ctx antlr.RuleContext) *parser.StatementContext {
	parent := ctx.GetParent()
	for parent != nil {
		if stmtCtx, ok := parent.(*parser.StatementContext); ok {
			return stmtCtx
		}
		parent = parent.GetParent()
	}
	return nil
}

// parseAnnotationParamInt parses an integer from an annotation parameter.
func parseAnnotationParamInt(ctx parser.IAnnotationParamContext) int {
	if ctx == nil {
		return 0
	}
	paramCtx := ctx.(*parser.AnnotationParamContext)
	if valueCtx := paramCtx.AnnotationValue(); valueCtx != nil {
		if val, err := strconv.Atoi(strings.TrimSpace(valueCtx.GetText())); err == nil {
			return val
		}
		annValue := valueCtx.(*parser.AnnotationValueContext)
		if lit := annValue.Literal(); lit != nil {
			litCtx := lit.(*parser.LiteralContext)
			if litCtx.NUMBER_LITERAL() != nil {
				text := litCtx.NUMBER_LITERAL().GetText()
				if val, err := strconv.Atoi(text); err == nil {
					return val
				}
			}
		}
	}
	return 0
}

// ExitAlterEntityAction handles ALTER ENTITY ... ADD/DROP/RENAME/MODIFY ATTRIBUTE ...
func (b *Builder) ExitAlterEntityAction(ctx *parser.AlterEntityActionContext) {
	// R3 respellings (visitor_r3_property_lists.go).
	b.recordAllowCreateChangeLocally(ctx)
	b.recordModifyAttributeColon(ctx)
	// Walk up to the parent AlterStatement to get the entity's qualified name
	parent := ctx.GetParent()
	for parent != nil {
		if alterStmt, ok := parent.(*parser.AlterStatementContext); ok {
			if alterStmt.ENTITY() == nil {
				return // Not an ALTER ENTITY statement
			}
			qn := alterStmt.QualifiedName()
			if qn == nil {
				return
			}
			name := buildQualifiedName(qn)
			attrNames := ctx.AllAttributeName()

			// ADD ATTRIBUTE / ADD COLUMN
			if ctx.ADD() != nil && ctx.AttributeKw() != nil {
				if attrDef := ctx.AttributeDefinition(); attrDef != nil {
					attr := buildSingleAttribute(attrDef.(*parser.AttributeDefinitionContext))
					// A `/** … */` doc comment written BETWEEN clauses (before the
					// ADD ATTRIBUTE keyword) documents the added attribute — same as a
					// doc comment on a CREATE ENTITY attribute. `--` line comments are
					// NOT an equivalent: they are discarded, whereas a doc comment is
					// persisted as the attribute's Mendix documentation. (traceops #27)
					if attr != nil && attr.Documentation == "" {
						if docCtx := ctx.DocComment(); docCtx != nil {
							attr.Documentation = extractDocComment(docCtx.GetText())
						}
					}
					if attr != nil {
						b.statements = append(b.statements, &ast.AlterEntityStmt{
							Name:        name,
							Operation:   ast.AlterEntityAddAttribute,
							Attribute:   attr,
							IfNotExists: ctx.IfNotExists() != nil,
						})
					}
				}
				return
			}

			// RENAME ATTRIBUTE / RENAME COLUMN
			if ctx.RENAME() != nil && ctx.AttributeKw() != nil && len(attrNames) >= 2 {
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:          name,
					Operation:     ast.AlterEntityRenameAttribute,
					AttributeName: attributeNameText(attrNames[0]),
					NewName:       attributeNameText(attrNames[1]),
				})
				return
			}

			// MODIFY ATTRIBUTE / MODIFY COLUMN
			if ctx.MODIFY() != nil && ctx.AttributeKw() != nil && len(attrNames) >= 1 {
				dt := buildDataType(ctx.DataType())
				stmt := &ast.AlterEntityStmt{
					Name:          name,
					Operation:     ast.AlterEntityModifyAttribute,
					AttributeName: attributeNameText(attrNames[0]),
					DataType:      dt,
				}
				// Capture constraints. MODIFY applies the constraints you
				// specify and preserves the ones you don't (Bug 12a). NULLABLE
				// explicitly clears NOT NULL; NOT NULL / REQUIRED set it.
				true_, false_ := true, false
				for _, constraintCtx := range ctx.AllAttributeConstraint() {
					c := constraintCtx.(*parser.AttributeConstraintContext)
					switch {
					case c.NULLABLE() != nil:
						stmt.ModifyNotNull = &false_
					case c.NOT_NULL() != nil || (c.NOT() != nil && c.NULL() != nil) || c.REQUIRED() != nil:
						stmt.ModifyNotNull = &true_
						if c.ConstraintErrorKeyword() != nil && c.STRING_LITERAL() != nil {
							stmt.ModifyNotNullError = unquoteStringLit(c.STRING_LITERAL())
						}
					case c.UNIQUE() != nil:
						stmt.ModifyUnique = &true_
						if c.ConstraintErrorKeyword() != nil && c.STRING_LITERAL() != nil {
							stmt.ModifyUniqueError = unquoteStringLit(c.STRING_LITERAL())
						}
					case c.DEFAULT() != nil:
						stmt.ModifyHasDefault = true
						if lit := c.Literal(); lit != nil {
							stmt.ModifyDefaultValue = extractLiteralValue(lit)
						} else if expr := c.Expression(); expr != nil {
							stmt.ModifyDefaultValue = unquoteQualifiedName(expr.GetText())
						}
					case c.CALCULATED() != nil:
						stmt.Calculated = true
						if qn := c.QualifiedName(); qn != nil {
							calcName := buildQualifiedName(qn)
							stmt.CalculatedMicroflow = &calcName
						}
					case c.LOCALIZED() != nil:
						applyLocalizeConstraint(&stmt.DataType, c)
					case c.MicroflowExportLevelClause() != nil:
						stmt.ModifyExportLevel = exportLevelOf(c.MicroflowExportLevelClause().(*parser.MicroflowExportLevelClauseContext))
					}
				}
				b.statements = append(b.statements, stmt)
				return
			}

			// DROP DEFAULT ON ATTRIBUTE — checked before DROP ATTRIBUTE, which
			// also matches DROP + ATTRIBUTE and would otherwise swallow it.
			if ctx.DROP() != nil && ctx.DEFAULT() != nil && ctx.ATTRIBUTE() != nil && len(attrNames) >= 1 {
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:          name,
					Operation:     ast.AlterEntityDropDefault,
					AttributeName: attributeNameText(attrNames[0]),
				})
				return
			}

			// DROP ATTRIBUTE / DROP COLUMN
			if ctx.DROP() != nil && ctx.AttributeKw() != nil && len(attrNames) >= 1 {
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:          name,
					Operation:     ast.AlterEntityDropAttribute,
					AttributeName: attributeNameText(attrNames[0]),
					IfExists:      ctx.IfExists() != nil,
				})
				return
			}

			// SET DOCUMENTATION
			if ctx.SET() != nil && ctx.DOCUMENTATION() != nil && ctx.STRING_LITERAL() != nil {
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:          name,
					Operation:     ast.AlterEntitySetDocumentation,
					Documentation: unquoteStringLit(ctx.STRING_LITERAL()),
				})
				return
			}

			// SET COMMENT: the deprecated spelling of SET DOCUMENTATION (R9,
			// MDL-DEPR135), and the same statement.
			if ctx.SET() != nil && ctx.COMMENT() != nil && ctx.STRING_LITERAL() != nil {
				b.recordDeprecation(deprecation.SetComment, ctx.COMMENT().GetSymbol(), "")
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:          name,
					Operation:     ast.AlterEntitySetDocumentation,
					Documentation: unquoteStringLit(ctx.STRING_LITERAL()),
				})
				return
			}

			// SET EXPORT LEVEL API | HIDDEN
			if el := ctx.MicroflowExportLevelClause(); ctx.SET() != nil && el != nil {
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:        name,
					Operation:   ast.AlterEntitySetExportLevel,
					ExportLevel: *exportLevelOf(el.(*parser.MicroflowExportLevelClauseContext)),
				})
				return
			}

			// SET POSITION (x, y)
			if ctx.SET() != nil && ctx.POSITION() != nil {
				nums := ctx.AllNUMBER_LITERAL()
				if len(nums) >= 2 {
					x, _ := strconv.Atoi(nums[0].GetText())
					y, _ := strconv.Atoi(nums[1].GetText())
					b.statements = append(b.statements, &ast.AlterEntityStmt{
						Name:      name,
						Operation: ast.AlterEntitySetPosition,
						Position:  &ast.Position{X: x, Y: y},
					})
				}
				return
			}

			// ADD INDEX
			if ctx.ADD() != nil && ctx.INDEX() != nil {
				if idxDef := ctx.IndexDefinition(); idxDef != nil {
					idx := buildIndex(idxDef)
					b.statements = append(b.statements, &ast.AlterEntityStmt{
						Name:        name,
						Operation:   ast.AlterEntityAddIndex,
						Index:       &idx,
						IfNotExists: ctx.IfNotExists() != nil,
					})
				}
				return
			}

			// DROP INDEX. The column-list form is preferred and is the only one
			// `describe entity` can round-trip — a Mendix index has no stored
			// name, so the ordinal form ("idx1") names a position that shifts
			// when an earlier index is dropped.
			if ctx.DROP() != nil && ctx.INDEX() != nil {
				stmt := &ast.AlterEntityStmt{
					Name:      name,
					Operation: ast.AlterEntityDropIndex,
					IfExists:  ctx.IfExists() != nil,
				}
				if idxDef := ctx.IndexDefinition(); idxDef != nil {
					idx := buildIndex(idxDef)
					stmt.Index = &idx
				} else if ctx.IDENTIFIER() != nil {
					stmt.IndexName = ctx.IDENTIFIER().GetText()
				} else {
					return
				}
				b.statements = append(b.statements, stmt)
				return
			}

			// ADD EVENT HANDLER
			if ctx.ADD() != nil && ctx.EVENT() != nil && ctx.HANDLER() != nil {
				if ehCtx := ctx.EventHandlerDefinition(); ehCtx != nil {
					if eh := buildEventHandler(ehCtx); eh != nil {
						b.statements = append(b.statements, &ast.AlterEntityStmt{
							Name:         name,
							Operation:    ast.AlterEntityAddEventHandler,
							EventHandler: eh,
							IfNotExists:  ctx.IfNotExists() != nil,
						})
					}
				}
				return
			}

			// DROP EVENT HANDLER ON Moment Type [CALL Mod.Microflow]
			if ctx.DROP() != nil && ctx.EVENT() != nil && ctx.HANDLER() != nil {
				eh := &ast.EventHandlerDef{}
				if m := ctx.EventMoment(); m != nil {
					eh.Moment = strings.Title(strings.ToLower(m.GetText()))
				}
				if t := ctx.EventType(); t != nil {
					txt := strings.ToUpper(t.GetText())
					switch txt {
					case "CREATE":
						eh.Event = "Create"
					case "COMMIT":
						eh.Event = "Commit"
					case "DELETE":
						eh.Event = "Delete"
					case "ROLLBACK":
						eh.Event = "RollBack"
					}
				}
				if qn := ctx.QualifiedName(); qn != nil {
					eh.Microflow = buildQualifiedName(qn)
				}
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:         name,
					Operation:    ast.AlterEntityDropEventHandler,
					EventHandler: eh,
					IfExists:     ctx.IfExists() != nil,
				})
				return
			}

			// SET ALLOW_CREATE_CHANGE_LOCALLY = true/false
			if ctx.SET() != nil && ctx.ALLOW_CREATE_CHANGE_LOCALLY() != nil {
				boolVal := false
				if ctx.TRUE() != nil {
					boolVal = true
				}
				b.statements = append(b.statements, &ast.AlterEntityStmt{
					Name:      name,
					Operation: ast.AlterEntitySetAllowCreateChangeLocally,
					BoolValue: boolVal,
				})
				return
			}

			break
		}
		parent = parent.GetParent()
	}
}

// ExitDropStatement handles DROP ENTITY/ASSOCIATION/ENUMERATION/MODULE/MICROFLOW/PAGE/SNIPPET
func (b *Builder) ExitDropStatement(ctx *parser.DropStatementContext) {
	// IF EXISTS is one grammar rule shared by every document-level alternative,
	// so it is applied here once, to whichever statement the chain below builds,
	// rather than in each branch where the next doctype would forget it (#531).
	built := len(b.statements)
	defer func() {
		if ctx.IfExists() == nil || len(b.statements) == built {
			return
		}
		if g, ok := b.statements[len(b.statements)-1].(ast.IfExistsDrop); ok {
			g.SetDropIfExists(true)
		}
	}()

	// DROP CONFIGURATION uses STRING_LITERAL, not qualifiedName — handle first
	if ctx.CONFIGURATION() != nil {
		if sl := ctx.STRING_LITERAL(); sl != nil {
			b.statements = append(b.statements, &ast.DropConfigurationStmt{
				Name: unquoteStringLit(sl),
			})
		}
		return
	}

	// DROP ANNOTATION addresses a note by caption or position, not by a
	// qualified name — an annotation has no name — so it must be handled before
	// the guard below, which returns silently when there is no qualifiedName.
	// Placed after it, the statement parsed, executed nothing, and exited 0.
	if ctx.ANNOTATION() != nil {
		stmt := &ast.DropAnnotationStmt{}
		if id := ctx.IdentifierOrKeyword(); id != nil {
			stmt.Module = unquoteIdentifier(id.GetText())
		}
		if nums := ctx.AllNUMBER_LITERAL(); ctx.AT_KW() != nil && len(nums) >= 2 {
			x, errX := strconv.Atoi(nums[0].GetText())
			y, errY := strconv.Atoi(nums[1].GetText())
			if errX == nil && errY == nil {
				stmt.Position = &ast.Position{X: x, Y: y}
			}
		} else if lit := ctx.STRING_LITERAL(); lit != nil {
			stmt.Title = unquoteStringLit(lit)
		}
		b.statements = append(b.statements, stmt)
		return
	}

	// Get the first qualified name (most DROP statements have at least one)
	names := ctx.AllQualifiedName()
	if len(names) == 0 {
		return
	}

	if ctx.ENTITY() != nil {
		b.statements = append(b.statements, &ast.DropEntityStmt{
			Name:     buildQualifiedName(names[0]),
			External: ctx.EXTERNAL() != nil,
		})
	} else if ctx.DATABASE() != nil && ctx.CONNECTION() != nil {
		b.statements = append(b.statements, &ast.DropDatabaseConnectionStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.VALIDATION() != nil {
		stmt := &ast.DropValidationRuleStmt{Attribute: buildQualifiedName(names[0])}
		switch {
		case ctx.REGEX() != nil:
			stmt.Kind = ast.ValidationRuleRegEx
		case ctx.RANGE() != nil:
			stmt.Kind = ast.ValidationRuleRange
		}
		b.statements = append(b.statements, stmt)
	} else if ctx.ASSOCIATION() != nil {
		b.statements = append(b.statements, &ast.DropAssociationStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.ENUMERATION() != nil {
		b.statements = append(b.statements, &ast.DropEnumerationStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.CONSTANT() != nil {
		b.statements = append(b.statements, &ast.DropConstantStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.MODULE() != nil {
		name := getQualifiedNameText(names[0])
		b.statements = append(b.statements, &ast.DropModuleStmt{
			Name: name,
		})
	} else if ctx.MICROFLOW() != nil {
		b.statements = append(b.statements, &ast.DropMicroflowStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.NANOFLOW() != nil {
		b.statements = append(b.statements, &ast.DropNanoflowStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.RULE() != nil {
		b.statements = append(b.statements, &ast.DropRuleStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.PAGE() != nil {
		b.statements = append(b.statements, &ast.DropPageStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.LAYOUT() != nil {
		b.statements = append(b.statements, &ast.DropLayoutStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.SNIPPET() != nil {
		b.statements = append(b.statements, &ast.DropSnippetStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.MENU_KW() != nil {
		b.statements = append(b.statements, &ast.DropMenuStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.JAVASCRIPT() != nil && ctx.ACTION() != nil {
		b.statements = append(b.statements, &ast.DropJavaScriptActionStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.JAVA() != nil && ctx.ACTION() != nil {
		b.statements = append(b.statements, &ast.DropJavaActionStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.ConsumedODataServiceKw() != nil {
		b.statements = append(b.statements, &ast.DropODataClientStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.PublishedODataServiceKw() != nil {
		b.statements = append(b.statements, &ast.DropODataServiceStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.BUSINESS() != nil && ctx.EVENT() != nil && ctx.SERVICE() != nil {
		b.statements = append(b.statements, &ast.DropBusinessEventServiceStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.WORKFLOW() != nil {
		b.statements = append(b.statements, &ast.DropWorkflowStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.IMAGE() != nil && ctx.COLLECTION() != nil {
		b.statements = append(b.statements, &ast.DropImageCollectionStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.TaskQueueKw() != nil {
		b.statements = append(b.statements, &ast.DropQueueStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.SCHEDULED() != nil && ctx.EVENT() != nil {
		b.statements = append(b.statements, &ast.DropScheduledEventStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.REGULAR() != nil && ctx.EXPRESSION() != nil {
		b.statements = append(b.statements, &ast.DropRegularExpressionStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.AiModelKw() != nil {
		b.statements = append(b.statements, &ast.DropModelStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.CONSUMED() != nil && ctx.MCP() != nil && ctx.SERVICE() != nil {
		b.statements = append(b.statements, &ast.DropConsumedMCPServiceStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.KNOWLEDGE() != nil && ctx.BASE() != nil {
		b.statements = append(b.statements, &ast.DropKnowledgeBaseStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.AGENT() != nil {
		b.statements = append(b.statements, &ast.DropAgentStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.PUBLISHED() != nil && ctx.REST() != nil && ctx.SERVICE() != nil {
		b.statements = append(b.statements, &ast.DropPublishedRestServiceStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.ConsumedRestServiceKw() != nil {
		b.statements = append(b.statements, &ast.DropRestClientStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.JSON() != nil && ctx.STRUCTURE() != nil {
		b.statements = append(b.statements, &ast.DropJsonStructureStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.MESSAGE() != nil && ctx.DEFINITION() != nil && ctx.COLLECTION() != nil {
		b.statements = append(b.statements, &ast.DropMessageDefinitionCollectionStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.MESSAGE() != nil && ctx.DEFINITION() != nil {
		b.statements = append(b.statements, &ast.DropMessageDefinitionStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.IMPORT() != nil && ctx.MAPPING() != nil {
		b.statements = append(b.statements, &ast.DropImportMappingStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.EXPORT() != nil && ctx.MAPPING() != nil {
		b.statements = append(b.statements, &ast.DropExportMappingStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.DATA() != nil && ctx.TRANSFORMER() != nil {
		b.statements = append(b.statements, &ast.DropDataTransformerStmt{
			Name: buildQualifiedName(names[0]),
		})
	} else if ctx.FOLDER() != nil {
		folderPath := unquoteStringLit(ctx.STRING_LITERAL())
		// Module can be a qualifiedName or IDENTIFIER
		var moduleName string
		if len(names) > 0 {
			moduleName = getQualifiedNameText(names[0])
		} else if ctx.IDENTIFIER() != nil {
			moduleName = ctx.IDENTIFIER().GetText()
		}
		b.statements = append(b.statements, &ast.DropFolderStmt{
			FolderPath: folderPath,
			Module:     moduleName,
		})
	}
}

// ExitRenameStatement handles RENAME ENTITY/MICROFLOW/NANOFLOW/PAGE/ENUMERATION/ASSOCIATION/CONSTANT/MODULE ... TO ...
func (b *Builder) ExitRenameStatement(ctx *parser.RenameStatementContext) {
	ioks := ctx.AllIdentifierOrKeyword()
	dryRun := ctx.DRY() != nil

	if ctx.MODULE() != nil {
		// RENAME MODULE oldName TO newName — both are identifierOrKeyword
		if len(ioks) >= 2 {
			oldName := identifierOrKeywordText(ioks[0])
			newName := identifierOrKeywordText(ioks[1])
			b.statements = append(b.statements, &ast.RenameStmt{
				ObjectType: "module",
				Name:       ast.QualifiedName{Module: oldName, Name: oldName},
				NewName:    newName,
				DryRun:     dryRun,
			})
		}
		return
	}

	// All other types use renameTarget qualifiedName TO identifierOrKeyword
	qn := buildQualifiedName(ctx.QualifiedName())
	if len(ioks) < 1 {
		return
	}
	newName := identifierOrKeywordText(ioks[0])

	objectType := ""
	if rt := ctx.RenameTarget(); rt != nil {
		objectType = strings.ToLower(rt.GetText())
	}
	if objectType == "" {
		return
	}

	b.statements = append(b.statements, &ast.RenameStmt{
		ObjectType: objectType,
		Name:       qn,
		NewName:    newName,
		DryRun:     dryRun,
	})
}

// ExitMoveStatement handles MOVE PAGE/MICROFLOW/SNIPPET/NANOFLOW/ENTITY/ENUMERATION to folder/module
func (b *Builder) ExitMoveStatement(ctx *parser.MoveStatementContext) {
	names := ctx.AllQualifiedName()
	if len(names) == 0 {
		return
	}

	// MOVE FOLDER is a different AST type, and is told apart by the ABSENCE of a
	// doctype. That is one check because moveDocumentType is a grammar rule: the
	// discriminator used to be a hand-written negation of every doctype keyword,
	// so every doctype added to MOVE had to be added here too or a folder move
	// silently started parsing as a document move (mxcli-formula1 #32).
	if ctx.MoveDocumentType() == nil && ctx.ENTITY() == nil {
		if len(ctx.AllFOLDER()) > 0 {
			b.exitMoveFolderStatement(ctx, names)
		}
		return
	}

	stmt := &ast.MoveStmt{
		Name: buildQualifiedName(names[0]),
	}

	if ctx.ENTITY() != nil {
		stmt.DocumentType = ast.DocumentTypeEntity
	} else {
		docType, ok := moveDocumentTypeFor(ctx.MoveDocumentType().GetText())
		if !ok {
			// A doctype in the grammar with no AST constant would otherwise move
			// whatever the name resolved to under an empty type. Refuse loudly:
			// the grammar and this table are meant to be added to together.
			b.addError(fmt.Errorf("MOVE does not support document type %q", ctx.MoveDocumentType().GetText()))
			return
		}
		stmt.DocumentType = docType
	}

	// Parse folder path if specified
	if len(ctx.AllFOLDER()) > 0 && ctx.STRING_LITERAL() != nil {
		stmt.Folder = unquoteStringLit(ctx.STRING_LITERAL())
	}

	// Parse target module if specified (IN Module or just Module)
	if ctx.IN() != nil && len(names) > 1 {
		// MOVE ... TO FOLDER 'path' IN Module
		stmt.TargetModule = getQualifiedNameText(names[1])
	} else if ctx.IDENTIFIER() != nil {
		// MOVE ... TO FOLDER 'path' IN ModuleName (identifier)
		stmt.TargetModule = ctx.IDENTIFIER().GetText()
	} else if len(ctx.AllFOLDER()) == 0 && len(names) > 1 {
		// MOVE ... TO Module (no folder, just target module)
		stmt.TargetModule = getQualifiedNameText(names[1])
	}

	b.statements = append(b.statements, stmt)
}

// exitMoveFolderStatement handles MOVE FOLDER Module.FolderName TO ...
func (b *Builder) exitMoveFolderStatement(ctx *parser.MoveStatementContext, names []parser.IQualifiedNameContext) {
	stmt := &ast.MoveFolderStmt{
		Name: buildQualifiedName(names[0]),
	}

	// Parse target: either FOLDER 'path' [IN Module] or just Module
	if ctx.STRING_LITERAL() != nil {
		// MOVE FOLDER ... TO FOLDER 'path' [IN Module]
		stmt.TargetFolder = unquoteStringLit(ctx.STRING_LITERAL())
		if len(names) > 1 {
			stmt.TargetModule = getQualifiedNameText(names[1])
		} else if ctx.IDENTIFIER() != nil {
			stmt.TargetModule = ctx.IDENTIFIER().GetText()
		}
	} else if len(names) > 1 {
		// MOVE FOLDER ... TO Module
		stmt.TargetModule = getQualifiedNameText(names[1])
	} else if ctx.IDENTIFIER() != nil {
		stmt.TargetModule = ctx.IDENTIFIER().GetText()
	}

	b.statements = append(b.statements, stmt)
}

// ----------------------------------------------------------------------------
// Association Statements
// ----------------------------------------------------------------------------

// ExitCreateAssociationStatement is called when exiting the createAssociationStatement production.

// moveDocumentTypeFor maps a moveDocumentType rule's text to its AST constant.
//
// The table lives in the ast package because the executor reads it too: it has
// to decide whether the document it found is the kind the statement named, and
// a second copy of the doctype list is exactly how the MOVE FOLDER
// discriminator drifted out of step with the grammar before.
func moveDocumentTypeFor(ruleText string) (ast.DocumentType, bool) {
	docType, ok := ast.MoveDocumentTypeByKeyword[strings.ToUpper(ruleText)]
	return docType, ok
}

// ExitCreateIndexStatement handles the standalone SQL spelling,
// `create index IdxRowCol on Mod.Cell (Row, Col DESC)`.
//
// The rule has been in the grammar since indexes were added, with no listener
// behind it — so the statement parsed, produced no AST node, and `exec` reported
// nothing and did nothing, even when the entity did not exist. That is worse
// than a parse error: the author reads "Syntax OK" and believes the index was
// created. Found while fixing sudoku finding #4, which is the same question
// asked from the other side ("where does an index go?").
//
// It carries exactly the meaning of `alter entity Mod.Cell add index (…)`, so it
// lowers to that statement rather than growing a second code path. The index
// name is accepted but not stored, as in every other spelling: a Mendix index
// is anonymous, identified by its column list. It is carried on the AST only
// so `check` can say so (MDL-IDX01).
func (b *Builder) ExitCreateIndexStatement(ctx *parser.CreateIndexStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}
	idx := &ast.Index{Columns: buildIndexColumns(ctx.IndexAttributeList())}
	if n := ctx.IDENTIFIER(); n != nil {
		idx.Name = n.GetText()
	}
	b.statements = append(b.statements, &ast.AlterEntityStmt{
		Name:      buildQualifiedName(qn),
		Operation: ast.AlterEntityAddIndex,
		Index:     idx,
	})
}

// ExitAlterEntitiesStatement handles the bulk form,
// ALTER ENTITIES [IN <module>] ADD ATTRIBUTE ... [WHERE PERSISTENT|NON-PERSISTENT].
//
// Each action is built into an ordinary AlterEntityStmt with an EMPTY Name: the
// executor resolves the target set and fills the name in per entity, so a bulk
// add and a single add run the identical code path and cannot diverge.
func (b *Builder) ExitAlterEntitiesStatement(ctx *parser.AlterEntitiesStatementContext) {
	stmt := &ast.AlterEntitiesStmt{}

	if mod := ctx.IdentifierOrKeyword(); mod != nil {
		stmt.Module = unquoteIdentifier(mod.GetText())
	}

	if f := ctx.EntityPersistenceFilter(); f != nil {
		fc := f.(*parser.EntityPersistenceFilterContext)
		switch {
		case fc.NON_PERSISTENT() != nil:
			stmt.Filter = ast.EntityFilterNonPersistent
		case fc.PERSISTENT() != nil:
			stmt.Filter = ast.EntityFilterPersistent
		}
	}

	for _, act := range ctx.AllAlterEntitiesAction() {
		ac := act.(*parser.AlterEntitiesActionContext)
		attrDef := ac.AttributeDefinition()
		if attrDef == nil {
			continue
		}
		attr := buildSingleAttribute(attrDef.(*parser.AttributeDefinitionContext))
		if attr == nil {
			continue
		}
		if attr.Documentation == "" {
			if docCtx := ac.DocComment(); docCtx != nil {
				attr.Documentation = extractDocComment(docCtx.GetText())
			}
		}
		stmt.Actions = append(stmt.Actions, &ast.AlterEntityStmt{
			Operation:   ast.AlterEntityAddAttribute,
			Attribute:   attr,
			IfNotExists: ac.IfNotExists() != nil,
		})
	}

	if len(stmt.Actions) > 0 {
		b.statements = append(b.statements, stmt)
	}
}
