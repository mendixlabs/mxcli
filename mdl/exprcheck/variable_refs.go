// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "strings"

// VarRef is one `$name` reference found in an expression.
type VarRef struct {
	Name   string // without the `$`
	Line   int
	Column int
}

// VariableReferences parses a Mendix expression and returns every `$name` it
// reads, in source order and with repeats: the variable of a `$name` and of a
// `$name/Attr/Path`. A bare identifier (`Status`, as a filter predicate writes an
// attribute of the item under test) is not a `$` reference and is left out, and
// so is an XPath path that starts at an entity rather than at a variable.
func VariableReferences(src string) []VarRef {
	if strings.TrimSpace(src) == "" {
		return nil
	}
	root, _ := (&parserImpl{}).Parse(src, Context{})
	if root == nil {
		return nil
	}
	var out []VarRef
	walkVars(root, &out)
	return out
}

func walkVars(e RobustExpr, out *[]VarRef) {
	switch n := e.(type) {
	case *VariableExpr:
		if !n.Bare && n.Name != "" {
			*out = append(*out, VarRef{Name: n.Name, Line: n.Pos().Line, Column: n.Pos().Column})
		}
	case *AttributePathExpr:
		if n.Variable != "" {
			*out = append(*out, VarRef{Name: n.Variable, Line: n.Pos().Line, Column: n.Pos().Column})
		}
	case *CallExpr:
		for _, a := range n.Args {
			walkVars(a, out)
		}
	case *BinExpr:
		walkVars(n.L, out)
		walkVars(n.R, out)
	case *UnaryExpr:
		walkVars(n.Operand, out)
	case *ParenExpr:
		walkVars(n.Inner, out)
	case *IfThenElseExpr:
		walkVars(n.Cond, out)
		walkVars(n.Then, out)
		walkVars(n.Else, out)
	}
}
