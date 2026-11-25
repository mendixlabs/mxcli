// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// The metrics activities (`increment counter`, `set gauge`). The metric name is
// an expression — Studio Pro stores `@Module.Constant` as readily as a quoted
// literal — so these rules can only judge a name written as a string literal.
//
// MDL-METRIC01 (error): a literal metric name must start with a letter and hold
// only letters, digits, dots and underscores, and must not start with `mx` in
// any case, which the runtime reserves for its own metrics. mxbuild 11.16.1
// enforces the same rules (measured): CE7040 "The metric name cannot be
// empty.", CE7048 "…cannot start with 'mx'." (also 'MX_upper') and CE7049
// "…only alpha-numeric characters, dots, or underscores." (also '9abc'). This
// reports them at check time instead of at build.
//
// MDL-METRIC02 (error): `increment counter … by` with a negative literal. A
// counter only goes up; the guide: "values cannot be negative". mxbuild 11.16.1
// ACCEPTS `by -1` (measured, 0 errors), so this is the only guard before the
// runtime refuses the increment.
const (
	meterNameRule     = "MDL-METRIC01"
	meterNegativeRule = "MDL-METRIC02"
)

var meterNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._]*$`)

// meterNameProblem is why a literal metric name is refused, or "".
func meterNameProblem(name string) string {
	switch {
	case name == "":
		return "is empty"
	case !meterNamePattern.MatchString(name):
		return "must start with a letter and contain only letters, digits, dots and underscores"
	case strings.HasPrefix(strings.ToLower(name), "mx"):
		return "must not start with 'mx', which Mendix reserves for its own metrics"
	}
	return ""
}

func (v *microflowValidator) checkMeterActivity(keyword string, name, value ast.Expression) {
	if lit, ok := literalOf(name); ok && lit.Kind == ast.LiteralString {
		s, _ := lit.Value.(string)
		if p := meterNameProblem(s); p != "" {
			v.addViolation(meterNameRule, linter.SeverityError,
				fmt.Sprintf("%s: metric name %q %s", keyword, s, p),
				"Use a name like 'acme.orders.placed': a prefix for your organisation and app, then dots or underscores.")
		}
	}
	if keyword == "increment counter" && isNegativeNumberLiteral(value) {
		v.addViolation(meterNegativeRule, linter.SeverityError,
			fmt.Sprintf("%s: a counter cannot be incremented by a negative value", keyword),
			"A counter only goes up. To report a value that goes down as well, use `set gauge … to …`.")
	}
}

// literalOf unwraps a SourceExpr / ParenExpr to a LiteralExpr.
func literalOf(e ast.Expression) (*ast.LiteralExpr, bool) {
	for {
		switch x := e.(type) {
		case *ast.SourceExpr:
			e = x.Expression
		case *ast.ParenExpr:
			e = x.Inner
		case *ast.LiteralExpr:
			return x, true
		default:
			return nil, false
		}
	}
}

// isNegativeNumberLiteral is true for `-5` / `-1.5` (a unary minus on a number)
// and for a number literal whose value is negative.
func isNegativeNumberLiteral(e ast.Expression) bool {
	for {
		switch x := e.(type) {
		case *ast.SourceExpr:
			e = x.Expression
			continue
		case *ast.ParenExpr:
			e = x.Inner
			continue
		case *ast.UnaryExpr:
			if x.Operator != "-" {
				return false
			}
			lit, ok := literalOf(x.Operand)
			return ok && (lit.Kind == ast.LiteralInteger || lit.Kind == ast.LiteralDecimal) && !isZeroLiteral(lit)
		case *ast.LiteralExpr:
			switch n := x.Value.(type) {
			case int64:
				return n < 0
			case int:
				return n < 0
			case float64:
				return n < 0
			}
		}
		return false
	}
}

func isZeroLiteral(l *ast.LiteralExpr) bool {
	switch n := l.Value.(type) {
	case int64:
		return n == 0
	case int:
		return n == 0
	case float64:
		return n == 0
	}
	return false
}
