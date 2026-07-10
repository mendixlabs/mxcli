// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// buildXPathExpr builds an AST Expression from an xpathExpr grammar rule context.
// This is the entry point for parsing XPath expressions inside [...] constraints.
func buildXPathExpr(ctx parser.IXpathExprContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	exprCtx := ctx.(*parser.XpathExprContext)

	andExprs := exprCtx.AllXpathAndExpr()
	if len(andExprs) == 0 {
		return nil
	}
	if len(andExprs) == 1 {
		return buildXPathAndExpr(andExprs[0])
	}

	// Multiple AND expressions joined by OR
	result := buildXPathAndExpr(andExprs[0])
	for i := 1; i < len(andExprs); i++ {
		result = &ast.BinaryExpr{
			Left:     result,
			Operator: "or",
			Right:    buildXPathAndExpr(andExprs[i]),
		}
	}
	return result
}

// buildXPathAndExpr builds an AST Expression from xpathAndExpr.
func buildXPathAndExpr(ctx parser.IXpathAndExprContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	andCtx := ctx.(*parser.XpathAndExprContext)

	notExprs := andCtx.AllXpathNotExpr()
	if len(notExprs) == 0 {
		return nil
	}
	if len(notExprs) == 1 {
		return buildXPathNotExpr(notExprs[0])
	}

	result := buildXPathNotExpr(notExprs[0])
	for i := 1; i < len(notExprs); i++ {
		result = &ast.BinaryExpr{
			Left:     result,
			Operator: "and",
			Right:    buildXPathNotExpr(notExprs[i]),
		}
	}
	return result
}

// buildXPathNotExpr builds an AST Expression from xpathNotExpr.
func buildXPathNotExpr(ctx parser.IXpathNotExprContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	notCtx := ctx.(*parser.XpathNotExprContext)

	// Check for NOT prefix
	if notCtx.NOT() != nil {
		inner := buildXPathNotExpr(notCtx.XpathNotExpr())
		return &ast.UnaryExpr{
			Operator: "not",
			Operand:  inner,
		}
	}

	return buildXPathComparisonExpr(notCtx.XpathComparisonExpr())
}

// buildXPathComparisonExpr builds an AST Expression from xpathComparisonExpr.
func buildXPathComparisonExpr(ctx parser.IXpathComparisonExprContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	cmpCtx := ctx.(*parser.XpathComparisonExprContext)

	valueExprs := cmpCtx.AllXpathValueExpr()
	if len(valueExprs) == 0 {
		return nil
	}

	left := buildXPathValueExpr(valueExprs[0])

	// Check for comparison operator
	if cmpCtx.ComparisonOperator() != nil && len(valueExprs) >= 2 {
		right := buildXPathValueExpr(valueExprs[1])
		op := cmpCtx.ComparisonOperator().GetText()
		return &ast.BinaryExpr{
			Left:     left,
			Operator: op,
			Right:    right,
		}
	}

	return left
}

// buildXPathValueExpr builds an AST Expression from xpathValueExpr.
func buildXPathValueExpr(ctx parser.IXpathValueExprContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	valCtx := ctx.(*parser.XpathValueExprContext)

	// Function call: name(args...)
	if fc := valCtx.XpathFunctionCall(); fc != nil {
		return buildXPathFunctionCall(fc)
	}

	// Path: step/step/step
	if path := valCtx.XpathPath(); path != nil {
		return buildXPathPath(path)
	}

	// Parenthesized expression: (expr)
	if valCtx.LPAREN() != nil {
		inner := buildXPathExpr(valCtx.XpathExpr())
		return &ast.ParenExpr{Inner: inner}
	}

	return nil
}

// buildXPathPath builds an AST Expression from xpathPath.
// For single-step paths without predicates, returns the underlying expression type.
// For multi-step paths or paths with predicates, returns XPathPathExpr.
func buildXPathPath(ctx parser.IXpathPathContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	pathCtx := ctx.(*parser.XpathPathContext)

	steps := pathCtx.AllXpathStep()
	if len(steps) == 0 {
		return nil
	}

	// Build all steps
	var xpathSteps []ast.XPathStep
	for _, step := range steps {
		xpathSteps = append(xpathSteps, buildXPathStep(step))
	}

	// Optimization: single step without predicate → return the inner expression directly
	if len(xpathSteps) == 1 && xpathSteps[0].Predicate == nil {
		return xpathSteps[0].Expr
	}

	return &ast.XPathPathExpr{Steps: xpathSteps}
}

// buildXPathStep builds an XPathStep from xpathStep.
func buildXPathStep(ctx parser.IXpathStepContext) ast.XPathStep {
	stepCtx := ctx.(*parser.XpathStepContext)

	step := ast.XPathStep{
		Expr: buildXPathStepValue(stepCtx.XpathStepValue()),
	}

	// Check for nested predicate [expr]
	if stepCtx.LBRACKET() != nil {
		step.Predicate = buildXPathExpr(stepCtx.XpathExpr())
	}

	return step
}

// buildXPathStepValue builds an AST Expression from xpathStepValue.
func buildXPathStepValue(ctx parser.IXpathStepValueContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	svCtx := ctx.(*parser.XpathStepValueContext)

	if qn := svCtx.XpathQualifiedName(); qn != nil {
		return buildXPathQualifiedName(qn)
	}
	if v := svCtx.VARIABLE(); v != nil {
		return &ast.VariableExpr{Name: strings.TrimPrefix(v.GetText(), "$")}
	}
	if sl := svCtx.STRING_LITERAL(); sl != nil {
		return &ast.LiteralExpr{Value: unquoteString(sl.GetText()), Kind: ast.LiteralString}
	}
	if nl := svCtx.NUMBER_LITERAL(); nl != nil {
		text := nl.GetText()
		if strings.Contains(text, ".") {
			return &ast.LiteralExpr{Value: text, Kind: ast.LiteralDecimal}
		}
		return &ast.LiteralExpr{Value: text, Kind: ast.LiteralInteger}
	}
	if mt := svCtx.MENDIX_TOKEN(); mt != nil {
		text := mt.GetText()
		// Strip [% and %] delimiters
		token := strings.TrimPrefix(text, "[%")
		token = strings.TrimSuffix(token, "%]")
		return &ast.TokenExpr{Token: token}
	}

	return nil
}

// buildXPathQualifiedName builds an expression from an xpathQualifiedName context.
// Single-part names like "Active" become IdentifierExpr.
// Multi-part names like "Module.Entity" become QualifiedNameExpr.
// Special values "empty", "true", "false" become the appropriate LiteralExpr.
func buildXPathQualifiedName(ctx parser.IXpathQualifiedNameContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	qnCtx := ctx.(*parser.XpathQualifiedNameContext)
	words := qnCtx.AllXpathWord()

	if len(words) == 1 {
		raw := words[0].GetText()
		// Strip identifier quotes so a keyword-escaped attribute like "Status"
		// becomes the member name Status, not the string literal 'Status' (which
		// Mendix would compare as an always-false constant — silent zero rows).
		text := unquoteIdentifier(raw)
		// The special XPath values are only special when written UNquoted; a
		// quoted "empty"/"true"/"false" is an ordinary (keyword-escaped) name.
		if raw == text {
			switch strings.ToLower(text) {
			case "empty":
				return &ast.LiteralExpr{Value: nil, Kind: ast.LiteralEmpty}
			case "true":
				return &ast.LiteralExpr{Value: true, Kind: ast.LiteralBoolean}
			case "false":
				return &ast.LiteralExpr{Value: false, Kind: ast.LiteralBoolean}
			}
		}
		return &ast.IdentifierExpr{Name: text}
	}

	// Multi-part: first part is module, rest joined as name
	module := unquoteIdentifier(words[0].GetText())
	remaining := make([]string, len(words)-1)
	for i, w := range words[1:] {
		remaining[i] = unquoteIdentifier(w.GetText())
	}
	return &ast.QualifiedNameExpr{
		QualifiedName: ast.QualifiedName{
			Module: module,
			Name:   strings.Join(remaining, "."),
		},
	}
}

// buildXPathFunctionCall builds a FunctionCallExpr from xpathFunctionCall.
func buildXPathFunctionCall(ctx parser.IXpathFunctionCallContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	fcCtx := ctx.(*parser.XpathFunctionCallContext)

	name := ""
	if fn := fcCtx.XpathFunctionName(); fn != nil {
		name = fn.GetText()
	}

	var args []ast.Expression
	for _, argCtx := range fcCtx.AllXpathExpr() {
		args = append(args, buildXPathExpr(argCtx))
	}

	return &ast.FunctionCallExpr{
		Name:      name,
		Arguments: args,
	}
}
