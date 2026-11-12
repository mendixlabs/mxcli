// SPDX-License-Identifier: Apache-2.0

package adapters

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	exprhints "github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

type CheckAdapter struct {
	parser  exprcheck.Parser
	slots   exprcheck.SlotResolver
	catalog exprcheck.CatalogReader
	source  func(ast.Expression) string
	// assoc is the catalog when it can also answer association questions; the
	// interface does not require it, so this is nil for a reader that cannot.
	assoc associationResolver
	// entities and kinds are set for the duration of one flow's walk.
	entities exprcheck.EntityScope
	kinds    exprcheck.Scope
}

// Option configures a CheckAdapter.
type Option func(*CheckAdapter)

// WithSourceFunc supplies the function that recovers an expression's source
// text.
//
// The default reads only ast.SourceExpr, which the visitor produces for some
// slots and not others: measured on a create-and-change microflow, the CREATE's
// value arrived as a SourceExpr and the CHANGE's as a bare LiteralExpr, so half
// the enum-literal mistakes in one flow were invisible. Callers that can render
// an expression back to text — mdl/executor has expressionToString — should pass
// it in so coverage does not depend on which slot the visitor happened to wrap.
func WithSourceFunc(f func(ast.Expression) string) Option {
	return func(c *CheckAdapter) {
		if f != nil {
			c.source = f
		}
	}
}

func NewCheckAdapter(cat exprcheck.CatalogReader, opts ...Option) *CheckAdapter {
	c := &CheckAdapter{
		parser:  exprcheck.NewParser(),
		slots:   exprcheck.DefaultSlotResolver(),
		catalog: cat,
		source:  exprSource,
	}
	// Association traversal is optional: CatalogReader does not require it, so a
	// reader that cannot answer it simply leaves multi-hop paths unresolved.
	if ar, ok := cat.(associationResolver); ok {
		c.assoc = ar
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type Result struct {
	Hints []exprcheck.Hint
}

func (c *CheckAdapter) CheckMicroflow(stmt *ast.CreateMicroflowStmt) *Result {
	r := &Result{}
	if stmt == nil {
		return r
	}
	c.walkFlow(stmt.Body, stmt.Parameters, stmt.Name.String(), r)
	return r
}

// CheckNanoflow checks a nanoflow's expressions.
//
// A nanoflow's body is the same []ast.MicroflowStatement, and every rule here
// is about expressions rather than about which activities are legal, so the two
// share one walk. Giving nanoflows their own entry point rather than leaving
// callers to reach for CheckMicroflow keeps the asymmetry from looking
// deliberate.
func (c *CheckAdapter) CheckNanoflow(stmt *ast.CreateNanoflowStmt) *Result {
	r := &Result{}
	if stmt == nil {
		return r
	}
	c.walkFlow(stmt.Body, stmt.Parameters, stmt.Name.String(), r)
	return r
}

// walkFlow checks one flow's expressions with its variables typed.
//
// The variable→entity map used to be built here and used only to label a
// CHANGE's slot path; it was never handed to the checker, so `$obj/Attr` had
// nothing to resolve against and every rule that depends on an attribute path
// stayed quiet. It is now the EntityScope for the whole walk, and the primitive
// half is the Scope beside it — a locally declared String was Unknown until
// then, and a rule that needs both operands typed (E004) stayed quiet on the
// most ordinary shape in the language (mendixlabs/mxcli#1100).
func (c *CheckAdapter) walkFlow(body []ast.MicroflowStatement, params []ast.MicroflowParam, mf string, r *Result) {
	scope := buildFlowScope(body, params, c.assoc)
	c.entities = entityScope{vars: scope.entities, assoc: c.assoc}
	c.kinds = kindScope(scope.kinds)
	defer func() { c.entities, c.kinds = nil, nil }()
	c.walkBodyWithScope(body, mf, scope.entities, r)
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
		case *ast.ListOperationStmt:
			c.checkListOperationCondition(n, mf, scope, r)
		case *ast.ReturnStmt:
			c.checkExpr(n.Value, "ReturnStmt.Value", mf, r)
		case *ast.DeclareStmt:
			c.checkExpr(n.InitialValue, "DeclareStmt.InitialValue", mf, r)
		case *ast.MfSetStmt:
			c.checkExpr(n.Value, "MfSetStmt.Value", mf, r)
		case *ast.LogStmt:
			c.checkExpr(n.Message, "LogStmt.Message", mf, r)
			// The template parameters were not walked at all, so a non-String
			// one reached mxbuild as CE0117 (mendixlabs/mxcli#1043).
			for _, tp := range n.Template {
				c.checkExpr(tp.Value, "LogStmt.TemplateParam", mf, r)
			}
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
			// The slot names the parameter the argument is passed to, so the
			// checker can resolve its type ("TargetParameter") the way a
			// CHANGE member names its attribute (ako/mxcli#2096).
			for _, a := range n.Arguments {
				c.checkExpr(a.Value, exprcheck.CallArgumentSlot(n.MicroflowName.String(), a.Name), mf, r)
			}
		case *ast.CallNanoflowStmt:
			for _, a := range n.Arguments {
				c.checkExpr(a.Value, exprcheck.CallArgumentSlot(n.NanoflowName.String(), a.Name), mf, r)
			}
		case *ast.SendEmailStmt:
			c.checkSendEmail(n, mf, r)
		}
		// A custom ON ERROR body is a block of ordinary statements and its
		// expressions are as checkable as any other. It was not walked at all,
		// so moving a statement into a handler exempted it from every rule. It
		// is walked after the statement that carries it, which is when it runs.
		if eb := errorHandlerBody(s); eb != nil {
			c.walkBodyWithScope(eb, mf, scope, r)
		}
	}
}

// checkListOperationCondition checks a FIND/FILTER predicate with
// $currentObject bound to the element type of the list under test.
//
// The predicate is block-scoped in the same way a loop body is: $currentObject
// exists only inside it, and it is named per statement rather than once per
// flow — two FILTERs over different lists in one microflow mean two different
// entities — so the binding is made for the duration of this one expression
// rather than folded into the flow scope.
//
// A BARE attribute name in the predicate (`FILTER($L, Status = 'Open')`, the
// spelling the skills recommend) still resolves to nothing: the parser reads it
// as a variable, not as an attribute of the item. That gap is deliberate here —
// binding bare names to the element entity would change what a bare identifier
// means everywhere in an expression, which is a larger decision than this one.
//
// A KNOWN element entity is the condition for checking at all, not just for
// binding $currentObject. `set $At = find($Hay, $Needle)` is Mendix's STRING
// find, and the visitor still builds it as a ListOperationStmt — the ambiguity
// is resolved later, in the flow builder, by looking at whether the input is a
// declared String (mdl-examples/bug-tests/ledger-63-string-find.mdl). Checking
// its second argument as a predicate reported the needle as a non-Boolean, on a
// script that builds at 0 errors. Requiring the entity applies the same
// disambiguation the builder already makes.
func (c *CheckAdapter) checkListOperationCondition(n *ast.ListOperationStmt, mf string, scope map[string]string, r *Result) {
	if n.Condition == nil {
		return
	}
	if n.Operation != ast.ListOpFind && n.Operation != ast.ListOpFilter {
		return
	}
	elem := scope[strings.TrimPrefix(n.InputVariable, "$")]
	if elem == "" {
		return
	}
	vars := make(map[string]string, len(scope)+1)
	for k, v := range scope {
		vars[k] = v
	}
	vars["currentObject"] = elem
	saved := c.entities
	c.entities = entityScope{vars: vars, assoc: c.assoc}
	c.checkExpr(n.Condition, "ListOperation.Condition", mf, r)
	c.entities = saved
}

func (c *CheckAdapter) checkExpr(expr ast.Expression, slot, mf string, r *Result) {
	// The captured source can carry the trailing layout of the statement it was
	// lifted from ("'Open'\n  "), which the lexer would then have to recover
	// from. Trim before parsing rather than teaching every rule about it.
	src := strings.TrimSpace(c.source(expr))
	if src == "" {
		return
	}
	_, hints := c.parser.Parse(src, exprcheck.Context{
		SlotPath:  slot,
		Microflow: mf,
		Slots:     c.slots,
		Catalog:   c.catalog,
		Entities:  c.entities,
		Scope:     c.kinds,
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

// checkSendEmail checks each SEND EMAIL expression against the type mxbuild
// requires for that field. A non-literal subject or body is stored as the one
// parameter of a `{1}` template, so it is checked as a template parameter.
func (c *CheckAdapter) checkSendEmail(n *ast.SendEmailStmt, mf string, r *Result) {
	for _, e := range []ast.Expression{n.From, n.To, n.Cc, n.Bcc} {
		if e != nil {
			c.checkExpr(e, "SendEmailStmt.Address", mf, r)
		}
	}
	for _, t := range []*ast.EmailTemplateClause{&n.Subject, n.BodyText, n.BodyHTML} {
		if t == nil {
			continue
		}
		if lit, ok := t.Text.(*ast.LiteralExpr); t.Text != nil && (!ok || lit.Kind != ast.LiteralString) && len(t.Params) == 0 {
			c.checkExpr(t.Text, "SendEmailStmt.TemplateParam", mf, r)
		}
		for _, p := range t.Params {
			c.checkExpr(p.Value, "SendEmailStmt.TemplateParam", mf, r)
		}
	}
	if n.Host != nil {
		c.checkExpr(n.Host, "SendEmailStmt.Host", mf, r)
	}
	if n.Port != nil {
		c.checkExpr(n.Port, "SendEmailStmt.Port", mf, r)
	}
	if n.Auth != nil {
		c.checkExpr(n.Auth.Username, "SendEmailStmt.Credential", mf, r)
		c.checkExpr(n.Auth.Password, "SendEmailStmt.Credential", mf, r)
	}
}
