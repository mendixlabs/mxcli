// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ----------------------------------------------------------------------------
// Expression Building
// ----------------------------------------------------------------------------

// buildExpression converts an expression context to an Expression AST node.
func buildExpression(ctx parser.IExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	exprCtx := ctx.(*parser.ExpressionContext)

	// Expression is just orExpression at top level
	if or := exprCtx.OrExpression(); or != nil {
		return buildOrExpression(or)
	}

	return nil
}

// buildOrExpression handles OR expressions.
func buildOrExpression(ctx parser.IOrExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	orCtx := ctx.(*parser.OrExpressionContext)

	andExprs := orCtx.AllAndExpression()
	if len(andExprs) == 0 {
		return nil
	}

	// Build first operand
	result := buildAndExpression(andExprs[0])

	// Chain OR operations
	for i := 1; i < len(andExprs); i++ {
		result = &ast.BinaryExpr{
			Left:     result,
			Operator: "OR",
			Right:    buildAndExpression(andExprs[i]),
		}
	}

	return result
}

// buildAndExpression handles AND expressions.
func buildAndExpression(ctx parser.IAndExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	andCtx := ctx.(*parser.AndExpressionContext)

	notExprs := andCtx.AllNotExpression()
	if len(notExprs) == 0 {
		return nil
	}

	// Build first operand
	result := buildNotExpression(notExprs[0])

	// Chain AND operations
	for i := 1; i < len(notExprs); i++ {
		result = &ast.BinaryExpr{
			Left:     result,
			Operator: "AND",
			Right:    buildNotExpression(notExprs[i]),
		}
	}

	return result
}

// buildNotExpression handles NOT expressions.
func buildNotExpression(ctx parser.INotExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	notCtx := ctx.(*parser.NotExpressionContext)

	// not(expr) — parens are mandatory (Mendix CE0117 rejects bare "not expr")
	if notCtx.NOT() != nil {
		return &ast.UnaryExpr{
			Operator: "NOT",
			Operand:  buildExpression(notCtx.Expression()),
		}
	}

	return buildComparisonExpression(notCtx.ComparisonExpression())
}

// buildComparisonExpression handles comparison expressions.
func buildComparisonExpression(ctx parser.IComparisonExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	compCtx := ctx.(*parser.ComparisonExpressionContext)

	addExprs := compCtx.AllAdditiveExpression()
	if len(addExprs) == 0 {
		return nil
	}

	// Build first operand
	left := buildAdditiveExpression(addExprs[0])

	// Check for comparison operator
	if compOp := compCtx.ComparisonOperator(); compOp != nil {
		if len(addExprs) >= 2 {
			return &ast.BinaryExpr{
				Left:     left,
				Operator: compOp.GetText(),
				Right:    buildAdditiveExpression(addExprs[1]),
			}
		}
	}

	// Check for IS NULL / IS NOT NULL
	if compCtx.IS_NULL() != nil {
		return &ast.BinaryExpr{
			Left:     left,
			Operator: "IS NULL",
			Right:    nil,
		}
	}
	if compCtx.IS_NOT_NULL() != nil {
		return &ast.BinaryExpr{
			Left:     left,
			Operator: "IS NOT NULL",
			Right:    nil,
		}
	}

	return left
}

// buildAdditiveExpression handles + and - expressions.
func buildAdditiveExpression(ctx parser.IAdditiveExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	addCtx := ctx.(*parser.AdditiveExpressionContext)

	multExprs := addCtx.AllMultiplicativeExpression()
	if len(multExprs) == 0 {
		return nil
	}

	// Build first operand
	result := buildMultiplicativeExpression(multExprs[0])

	// Get operators (PLUS and MINUS tokens)
	plusTokens := addCtx.AllPLUS()
	minusTokens := addCtx.AllMINUS()

	// Reconstruct the sequence of operators
	// This is a simplified approach - for complex expressions we'd need to track token positions
	opIndex := 0
	for i := 1; i < len(multExprs); i++ {
		op := "+"
		if opIndex < len(plusTokens) {
			op = "+"
		} else if opIndex-len(plusTokens) < len(minusTokens) {
			op = "-"
		}
		opIndex++

		result = &ast.BinaryExpr{
			Left:     result,
			Operator: op,
			Right:    buildMultiplicativeExpression(multExprs[i]),
		}
	}

	return result
}

// buildMultiplicativeExpression handles *, /, div expressions.
// Importantly, it detects XPath-style attribute paths like $Var/Attr and converts
// them to AttributePathExpr instead of division.
func buildMultiplicativeExpression(ctx parser.IMultiplicativeExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	multCtx := ctx.(*parser.MultiplicativeExpressionContext)

	unaryExprs := multCtx.AllUnaryExpression()
	if len(unaryExprs) == 0 {
		return nil
	}

	// Build first operand
	result := buildUnaryExpression(unaryExprs[0])

	// If only one operand, return it directly
	if len(unaryExprs) == 1 {
		return result
	}

	// Get operators from children in order
	// Children alternate: unaryExpr, op, unaryExpr, op, ...
	var operators []string
	for _, child := range multCtx.GetChildren() {
		if term, ok := child.(antlr.TerminalNode); ok {
			symbol := term.GetSymbol()
			switch symbol.GetTokenType() {
			case parser.MDLParserSTAR:
				operators = append(operators, "*")
			case parser.MDLParserSLASH:
				operators = append(operators, "/")
			case parser.MDLParserCOLON:
				operators = append(operators, ":")
			case parser.MDLParserPERCENT:
				operators = append(operators, "%")
			case parser.MDLParserMOD:
				operators = append(operators, "mod")
			case parser.MDLParserDIV:
				operators = append(operators, "div")
			}
		}
	}

	// Process operators and operands
	for i := 1; i < len(unaryExprs); i++ {
		op := "*" // default
		if i-1 < len(operators) {
			op = operators[i-1]
		}

		right := buildUnaryExpression(unaryExprs[i])

		// Check if this is an XPath-style attribute path: $Var/Attr or $Var/Assoc/Attr
		if op == "/" {
			if pathExpr := tryBuildAttributePath(result, right); pathExpr != nil {
				result = pathExpr
				continue
			}
		}

		result = &ast.BinaryExpr{
			Left:     result,
			Operator: op,
			Right:    right,
		}
	}

	return result
}

// tryBuildAttributePath attempts to build an AttributePathExpr from a left expression
// and a right identifier. Returns nil if not an XPath-style path.
func tryBuildAttributePath(left ast.Expression, right ast.Expression) *ast.AttributePathExpr {
	// Right must be a path component: identifier, qualified name, or string literal
	var pathPart string
	switch r := right.(type) {
	case *ast.IdentifierExpr:
		pathPart = r.Name
	case *ast.QualifiedNameExpr:
		pathPart = r.QualifiedName.String()
	case *ast.LiteralExpr:
		if r.Kind == ast.LiteralString {
			pathPart, _ = r.Value.(string)
		}
	case *ast.VariableExpr:
		pathPart = r.Name
	}

	if pathPart == "" {
		return nil
	}

	// Left must be a VariableExpr or AttributePathExpr
	switch l := left.(type) {
	case *ast.VariableExpr:
		return &ast.AttributePathExpr{
			Variable: l.Name,
			Path:     []string{pathPart},
		}
	case *ast.AttributePathExpr:
		// Extend existing path
		return &ast.AttributePathExpr{
			Variable: l.Variable,
			Path:     append(l.Path, pathPart),
		}
	}

	return nil
}

// buildUnaryExpression handles unary +/- expressions.
func buildUnaryExpression(ctx parser.IUnaryExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	unaryCtx := ctx.(*parser.UnaryExpressionContext)

	// Build primary expression
	primary := buildPrimaryExpression(unaryCtx.PrimaryExpression())

	// Check for unary minus
	if unaryCtx.MINUS() != nil {
		return &ast.UnaryExpr{
			Operator: "-",
			Operand:  primary,
		}
	}

	return primary
}

// buildPrimaryExpression handles primary expressions (literals, variables, function calls, etc.).
func buildPrimaryExpression(ctx parser.IPrimaryExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	primCtx := ctx.(*parser.PrimaryExpressionContext)

	// Parenthesized expression
	if expr := primCtx.Expression(); expr != nil {
		inner := buildExpression(expr)
		return &ast.ParenExpr{Inner: inner}
	}

	// Inline if-then-else expression
	if ifExpr := primCtx.IfThenElseExpression(); ifExpr != nil {
		return buildIfThenElseExpression(ifExpr)
	}

	// Aggregate function (COUNT, SUM, AVG, MIN, MAX for SQL/OQL)
	// This matches before listAggregateOperation, so we convert it to FunctionCallExpr
	if aggrFunc := primCtx.AggregateFunction(); aggrFunc != nil {
		return buildAggregateFunctionAsCall(aggrFunc)
	}

	// List operation (HEAD, TAIL, FIND, etc.) - convert to FunctionCallExpr
	if listOp := primCtx.ListOperation(); listOp != nil {
		return buildListOperationAsFunction(listOp)
	}

	// List aggregate operation (COUNT, SUM, etc.) - convert to FunctionCallExpr
	if listAggr := primCtx.ListAggregateOperation(); listAggr != nil {
		return buildListAggregateAsFunction(listAggr)
	}

	// Function call
	if funcCall := primCtx.FunctionCall(); funcCall != nil {
		return buildFunctionCall(funcCall)
	}

	// Atomic expression (literals, variables, etc.)
	if atomic := primCtx.AtomicExpression(); atomic != nil {
		return buildAtomicExpression(atomic)
	}

	return nil
}

// buildAggregateFunctionAsCall converts an SQL aggregate function (COUNT, SUM, etc.) to FunctionCallExpr.
// This allows SET statements to recognize these as list operations in the microflow context.
func buildAggregateFunctionAsCall(ctx parser.IAggregateFunctionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	aggrCtx := ctx.(*parser.AggregateFunctionContext)

	funcExpr := &ast.FunctionCallExpr{}

	// Determine function name
	if aggrCtx.COUNT() != nil {
		funcExpr.Name = "COUNT"
	} else if aggrCtx.SUM() != nil {
		funcExpr.Name = "SUM"
	} else if aggrCtx.AVG() != nil {
		funcExpr.Name = "AVERAGE" // Map AVG to AVERAGE for Mendix
	} else if aggrCtx.MIN() != nil {
		funcExpr.Name = "MINIMUM" // Map MIN to MINIMUM for Mendix
	} else if aggrCtx.MAX() != nil {
		funcExpr.Name = "MAXIMUM" // Map MAX to MAXIMUM for Mendix
	}

	// Get the expression argument (or STAR for COUNT(*))
	if expr := aggrCtx.Expression(); expr != nil {
		funcExpr.Arguments = append(funcExpr.Arguments, buildExpression(expr))
	} else if aggrCtx.STAR() != nil {
		// COUNT(*) - represent as a literal
		funcExpr.Arguments = append(funcExpr.Arguments, &ast.LiteralExpr{Value: "*", Kind: ast.LiteralString})
	}

	return funcExpr
}

// buildListOperationAsFunction converts a list operation to a FunctionCallExpr.
// This allows SET statements to recognize list operations uniformly.
func buildListOperationAsFunction(ctx parser.IListOperationContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	listOpCtx := ctx.(*parser.ListOperationContext)

	funcExpr := &ast.FunctionCallExpr{}

	// Determine the operation name based on which token is present
	if listOpCtx.HEAD() != nil {
		funcExpr.Name = "HEAD"
	} else if listOpCtx.TAIL() != nil {
		funcExpr.Name = "TAIL"
	} else if listOpCtx.FIND() != nil {
		funcExpr.Name = "FIND"
	} else if listOpCtx.FILTER() != nil {
		funcExpr.Name = "FILTER"
	} else if listOpCtx.SORT() != nil {
		funcExpr.Name = "SORT"
	} else if listOpCtx.UNION() != nil {
		funcExpr.Name = "UNION"
	} else if listOpCtx.INTERSECT() != nil {
		funcExpr.Name = "INTERSECT"
	} else if listOpCtx.SUBTRACT() != nil {
		funcExpr.Name = "SUBTRACT"
	} else if listOpCtx.CONTAINS() != nil {
		funcExpr.Name = "CONTAINS"
	} else if listOpCtx.EQUALS_OP() != nil {
		funcExpr.Name = "EQUALS"
	}

	// Get all VARIABLE tokens as arguments
	for _, v := range listOpCtx.AllVARIABLE() {
		varText := v.GetText()
		varName := strings.TrimPrefix(varText, "$")
		funcExpr.Arguments = append(funcExpr.Arguments, &ast.VariableExpr{Name: varName})
	}

	// Get expression arguments (for FIND/FILTER/RANGE)
	for _, expr := range listOpCtx.AllExpression() {
		funcExpr.Arguments = append(funcExpr.Arguments, buildExpression(expr))
	}

	// Get sort spec list argument (for SORT)
	if sortSpecs := listOpCtx.SortSpecList(); sortSpecs != nil {
		// For sort specs, we add them as identifier expressions
		sortCtx := sortSpecs.(*parser.SortSpecListContext)
		for _, spec := range sortCtx.AllSortSpec() {
			specCtx := spec.(*parser.SortSpecContext)
			attrName := ""
			if id := specCtx.IDENTIFIER(); id != nil {
				attrName = id.GetText()
			} else if qid := specCtx.QUOTED_IDENTIFIER(); qid != nil {
				attrName = unquoteIdentifier(qid.GetText())
			}
			if attrName != "" {
				// Create a sort spec representation
				ascending := true
				if specCtx.DESC() != nil {
					ascending = false
				}
				// Store as identifier with direction suffix
				if ascending {
					funcExpr.Arguments = append(funcExpr.Arguments, &ast.IdentifierExpr{Name: attrName + " ASC"})
				} else {
					funcExpr.Arguments = append(funcExpr.Arguments, &ast.IdentifierExpr{Name: attrName + " DESC"})
				}
			}
		}
	}

	return funcExpr
}

// buildListAggregateAsFunction converts a list aggregate operation to a FunctionCallExpr.
func buildListAggregateAsFunction(ctx parser.IListAggregateOperationContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	aggrCtx := ctx.(*parser.ListAggregateOperationContext)

	funcExpr := &ast.FunctionCallExpr{}

	// Determine the operation name
	if aggrCtx.COUNT() != nil {
		funcExpr.Name = "COUNT"
	} else if aggrCtx.SUM() != nil {
		funcExpr.Name = "SUM"
	} else if aggrCtx.AVERAGE() != nil {
		funcExpr.Name = "AVERAGE"
	} else if aggrCtx.MINIMUM() != nil {
		funcExpr.Name = "MINIMUM"
	} else if aggrCtx.MAXIMUM() != nil {
		funcExpr.Name = "MAXIMUM"
	}

	// Get the variable argument for COUNT (which uses VARIABLE, not attributePath)
	if v := aggrCtx.VARIABLE(); v != nil {
		varText := v.GetText()
		varName := strings.TrimPrefix(varText, "$")
		funcExpr.Arguments = append(funcExpr.Arguments, &ast.VariableExpr{Name: varName})
	}

	// Get the attribute path argument for SUM/AVERAGE/MINIMUM/MAXIMUM (e.g., $List/Attr)
	if attrPath := aggrCtx.AttributePath(); attrPath != nil {
		attrPathCtx := attrPath.(*parser.AttributePathContext)
		pathText := attrPathCtx.GetText()
		// Parse the path like $Var/Attr
		if strings.Contains(pathText, "/") {
			parts := strings.Split(pathText, "/")
			varName := strings.TrimPrefix(parts[0], "$")
			funcExpr.Arguments = append(funcExpr.Arguments, &ast.AttributePathExpr{
				Variable: varName,
				Path:     parts[1:],
			})
		} else {
			// Just a variable
			varName := strings.TrimPrefix(pathText, "$")
			funcExpr.Arguments = append(funcExpr.Arguments, &ast.VariableExpr{Name: varName})
		}
	}

	return funcExpr
}

// buildIfThenElseExpression converts an inline if-then-else expression to an AST node.
func buildIfThenElseExpression(ctx parser.IIfThenElseExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	ifCtx := ctx.(*parser.IfThenElseExpressionContext)

	exprs := ifCtx.AllExpression()
	if len(exprs) < 3 {
		return nil
	}

	return &ast.IfThenElseExpr{
		Condition: buildExpression(exprs[0]),
		ThenExpr:  buildExpression(exprs[1]),
		ElseExpr:  buildExpression(exprs[2]),
	}
}

// buildFunctionCall handles function call expressions.
func buildFunctionCall(ctx parser.IFunctionCallContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	funcCtx := ctx.(*parser.FunctionCallContext)

	funcExpr := &ast.FunctionCallExpr{}

	// Get function name. `functionCall` accepts either a built-in `functionName`
	// (length, find, ...) or a `qualifiedName` (Module.Rule, Module.SubMicroflow)
	// so that describe → check roundtrips preserve rule-based split conditions
	// and sub-microflow calls used in expression position.
	if fn := funcCtx.FunctionName(); fn != nil {
		funcExpr.Name = fn.GetText()
	} else if qn := funcCtx.QualifiedName(); qn != nil {
		funcExpr.Name = buildQualifiedName(qn).String()
	}

	// Get arguments
	if argList := funcCtx.ArgumentList(); argList != nil {
		argCtx := argList.(*parser.ArgumentListContext)
		for _, expr := range argCtx.AllExpression() {
			funcExpr.Arguments = append(funcExpr.Arguments, buildExpression(expr))
		}
	}

	return funcExpr
}

// buildAtomicExpression handles atomic expressions (literals, variables, qualified names).
func buildAtomicExpression(ctx parser.IAtomicExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	atomCtx := ctx.(*parser.AtomicExpressionContext)

	// Literal
	if lit := atomCtx.Literal(); lit != nil {
		return buildLiteralExpression(lit)
	}

	// Variable: $Var or $Widget.Attr (data source attribute reference)
	if v := atomCtx.VARIABLE(); v != nil {
		varText := v.GetText()
		// Check for attribute path like $Var/Attr (xpath style)
		if strings.Contains(varText, "/") {
			parts := strings.Split(varText, "/")
			varName := strings.TrimPrefix(parts[0], "$")
			return &ast.AttributePathExpr{
				Variable: varName,
				Path:     parts[1:],
			}
		}

		// Check for $Widget.Attr syntax (data source attribute reference)
		// The grammar is: VARIABLE (DOT attributeName)*
		attrNames := atomCtx.AllAttributeName()
		if len(attrNames) > 0 {
			// Build the full path: Widget.Attr1.Attr2...
			varName := strings.TrimPrefix(varText, "$")
			path := make([]string, len(attrNames))
			for i, an := range attrNames {
				path[i] = attributeNameText(an)
			}
			return &ast.AttributePathExpr{
				Variable: varName,
				Path:     path,
			}
		}

		return &ast.VariableExpr{
			Name: strings.TrimPrefix(varText, "$"),
		}
	}

	// Constant reference: @Module.ConstantName
	if atomCtx.AT() != nil {
		if qn := atomCtx.QualifiedName(); qn != nil {
			return &ast.ConstantRefExpr{
				QualifiedName: buildQualifiedName(qn),
			}
		}
	}

	// Mendix token [%TokenName%]
	if token := atomCtx.MENDIX_TOKEN(); token != nil {
		tokenText := token.GetText()
		// Remove [% and %]
		tokenName := strings.TrimPrefix(tokenText, "[%")
		tokenName = strings.TrimSuffix(tokenName, "%]")
		return &ast.TokenExpr{Token: tokenName}
	}

	// Qualified name or identifier (entity reference, enum value, etc.)
	if qn := atomCtx.QualifiedName(); qn != nil {
		text := qn.GetText()
		// Could be an attribute path reference without $ prefix
		if strings.Contains(text, "/") {
			parts := strings.Split(text, "/")
			return &ast.AttributePathExpr{
				Variable: parts[0],
				Path:     parts[1:],
			}
		}
		name := buildQualifiedName(qn)
		// Simple identifier (no module) - use IdentifierExpr for XPath attribute names
		if name.Module == "" {
			return &ast.IdentifierExpr{
				Name: name.Name,
			}
		}
		// Qualified name with module (like association name) - use QualifiedNameExpr
		// This ensures they are not quoted when converted to expression strings
		return &ast.QualifiedNameExpr{
			QualifiedName: name,
		}
	}

	// Standalone IDENTIFIER (not part of $Widget.Attr)
	// Only match if no VARIABLE (otherwise attributeNames were handled above)
	if atomCtx.VARIABLE() == nil {
		if id := atomCtx.IDENTIFIER(); id != nil {
			// Simple identifier - use IdentifierExpr for unquoted output
			return &ast.IdentifierExpr{
				Name: id.GetText(),
			}
		}
	}

	return nil
}

// buildLiteralExpression converts a literal context to LiteralExpr.
func buildLiteralExpression(ctx parser.ILiteralContext) *ast.LiteralExpr {
	if ctx == nil {
		return nil
	}
	litCtx := ctx.(*parser.LiteralContext)

	// String literal
	if str := litCtx.STRING_LITERAL(); str != nil {
		return &ast.LiteralExpr{
			Value: unquoteString(str.GetText()),
			Kind:  ast.LiteralString,
		}
	}

	// Number literal
	if num := litCtx.NUMBER_LITERAL(); num != nil {
		text := num.GetText()
		// Try integer first
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return &ast.LiteralExpr{
				Value: i,
				Kind:  ast.LiteralInteger,
			}
		}
		// Try decimal
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return &ast.LiteralExpr{
				Value: f,
				Kind:  ast.LiteralDecimal,
			}
		}
	}

	// Boolean literal
	if boolLit := litCtx.BooleanLiteral(); boolLit != nil {
		boolCtx := boolLit.(*parser.BooleanLiteralContext)
		if boolCtx.TRUE() != nil {
			return &ast.LiteralExpr{
				Value: true,
				Kind:  ast.LiteralBoolean,
			}
		}
		if boolCtx.FALSE() != nil {
			return &ast.LiteralExpr{
				Value: false,
				Kind:  ast.LiteralBoolean,
			}
		}
	}

	// Null literal
	if litCtx.NULL() != nil {
		return &ast.LiteralExpr{
			Value: nil,
			Kind:  ast.LiteralNull,
		}
	}

	// Empty literal (Mendix keyword for empty/null)
	if litCtx.EMPTY() != nil {
		return &ast.LiteralExpr{
			Value: nil,
			Kind:  ast.LiteralNull,
		}
	}

	return nil
}
