// SPDX-License-Identifier: Apache-2.0

package adapters

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/exprcheck"
	exprhints "github.com/JordtenBulte-OLC/mxcli/mdl/exprcheck/hints"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

type CheckAdapter struct {
	parser  exprcheck.Parser
	slots   exprcheck.SlotResolver
	catalog exprcheck.CatalogReader
}

func NewCheckAdapter(cat exprcheck.CatalogReader) *CheckAdapter {
	return &CheckAdapter{
		parser:  exprcheck.NewParser(),
		slots:   exprcheck.DefaultSlotResolver(),
		catalog: cat,
	}
}

type Result struct {
	Hints []exprcheck.Hint
}

func (c *CheckAdapter) CheckMicroflow(stmt *ast.CreateMicroflowStmt) *Result {
	r := &Result{}
	if stmt == nil {
		return r
	}
	c.walkBody(stmt.Body, stmt.Name.String(), r)
	return r
}

func (c *CheckAdapter) walkBody(body []ast.MicroflowStatement, mf string, r *Result) {
	scope := buildVarEntityScope(body)
	c.walkBodyWithScope(body, mf, scope, r)
}

func (c *CheckAdapter) walkBodyWithScope(body []ast.MicroflowStatement, mf string, scope map[string]string, r *Result) {
	for _, s := range body {
		switch n := s.(type) {
		case *ast.IfStmt:
			c.checkExpr(n.Condition, "IfStmt.Condition", mf, r)
			c.walkBodyWithScope(n.ThenBody, mf, scope, r)
			c.walkBodyWithScope(n.ElseBody, mf, scope, r)
		case *ast.WhileStmt:
			c.checkExpr(n.Condition, "WhileStmt.Condition", mf, r)
			c.walkBodyWithScope(n.Body, mf, scope, r)
		case *ast.LoopStmt:
			c.walkBodyWithScope(n.Body, mf, scope, r)
		case *ast.ReturnStmt:
			c.checkExpr(n.Value, "ReturnStmt.Value", mf, r)
		case *ast.DeclareStmt:
			c.checkExpr(n.InitialValue, "DeclareStmt.InitialValue", mf, r)
		case *ast.MfSetStmt:
			c.checkExpr(n.Value, "MfSetStmt.Value", mf, r)
		case *ast.LogStmt:
			c.checkExpr(n.Message, "LogStmt.Message", mf, r)
		case *ast.CreateObjectStmt:
			entityQN := n.EntityType.String()
			for _, ci := range n.Changes {
				slot := "CreateItem.Value:" + entityQN + "." + ci.Attribute
				c.checkExpr(ci.Value, slot, mf, r)
			}
		case *ast.ChangeObjectStmt:
			entityQN := scope[n.Variable]
			for _, ci := range n.Changes {
				slot := "ChangeItem.Value"
				if entityQN != "" {
					slot = "ChangeItem.Value:" + entityQN + "." + ci.Attribute
				}
				c.checkExpr(ci.Value, slot, mf, r)
			}
		case *ast.CallMicroflowStmt:
			for _, a := range n.Arguments {
				c.checkExpr(a.Value, "CallArgument.Value", mf, r)
			}
		case *ast.CallNanoflowStmt:
			for _, a := range n.Arguments {
				c.checkExpr(a.Value, "CallArgument.Value", mf, r)
			}
		}
	}
}

func (c *CheckAdapter) checkExpr(expr ast.Expression, slot, mf string, r *Result) {
	src := exprSource(expr)
	if src == "" {
		return
	}
	_, hints := c.parser.Parse(src, exprcheck.Context{
		SlotPath:  slot,
		Microflow: mf,
		Slots:     c.slots,
		Catalog:   c.catalog,
	})
	r.Hints = append(r.Hints, hints...)
}

func exprSource(expr ast.Expression) string {
	if se, ok := expr.(*ast.SourceExpr); ok {
		return se.Source
	}
	return ""
}

func (r *Result) AsViolations() []linter.Violation {
	out := make([]linter.Violation, 0, len(r.Hints))
	for _, h := range r.Hints {
		out = append(out, linter.Violation{
			RuleID:     h.Code,
			Severity:   mapSeverity(h.Severity),
			Message:    h.Problem,
			Suggestion: h.Fix,
			Location: linter.Location{
				DocumentType: "microflow",
				DocumentName: h.Where.Microflow,
			},
		})
	}
	return out
}

func mapSeverity(s exprhints.Severity) linter.Severity {
	switch s {
	case exprhints.SeverityError:
		return linter.SeverityError
	case exprhints.SeverityWarning:
		return linter.SeverityWarning
	case exprhints.SeverityInfo:
		return linter.SeverityInfo
	}
	return linter.SeverityHint
}
