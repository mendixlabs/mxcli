// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/antlr4-go/antlr/v4"
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ParseXPathConstraint parses a raw XPath constraint string — including the outer
// [ ] brackets stored by Mendix in the XPathConstraint BSON field — and returns the
// AST expression. Returns (nil, false) if the input cannot be parsed (e.g. empty,
// malformed, or not starting with '[').
func ParseXPathConstraint(input string) (ast.Expression, bool) {
	if input == "" {
		return nil, false
	}

	is := antlr.NewInputStream(input)
	lexer := parser.NewMDLLexer(is)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewMDLParser(stream)
	p.RemoveErrorListeners()

	ctx := p.XpathConstraint()
	xcCtx, ok := ctx.(*parser.XpathConstraintContext)
	if !ok {
		return nil, false
	}
	xpathExpr := xcCtx.XpathExpr()
	if xpathExpr == nil {
		return nil, false
	}
	return buildXPathExpr(xpathExpr), true
}
