// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ParseExpressionText parses text as one MDL expression and returns its AST.
// A widget's expression properties (Visible, Editable, DynamicClasses) are
// stored as source text, and check reads them back with this to judge the
// members they name (ako/mxcli#2258). ok is false for text that is not exactly
// one expression.
func ParseExpressionText(text string) (ast.Expression, bool) {
	if strings.TrimSpace(text) == "" {
		return nil, false
	}
	ctx, ok := parseRule(text, func(p *parser.MDLParser) antlr.ParserRuleContext { return p.Expression() })
	if !ok {
		return nil, false
	}
	e := buildExpression(ctx.(parser.IExpressionContext))
	return e, e != nil
}
