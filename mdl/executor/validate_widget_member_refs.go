// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/mdl/xpathrefs"
)

// Two member positions inside a widget that reference checking walked past.
// Both passed `check --references`, passed `exec`, and failed the build.
//
//   - An XPATH CONSTRAINT naming a member the constrained entity does not have
//     (mendixlabs/mxcli#1049). The check resolved the entity in
//     `database from Bench.Order` and never looked inside the `where […]`:
//
//     where [Bench.Order_Status = 'Open']
//     mx check -> [CE1613] "The selected association 'Bench.Order_Status'
//     no longer exists."
//
//   - A CONTENTPARAMS value written as a `$variable/` PATH where the position
//     takes an attribute reachable from the widget's context object
//     (mendixlabs/mxcli#1046). The writer put the whole string in the attribute
//     name, so the model came out naming an attribute that could never exist:
//
//     ContentParams: [{1} = $Customer/Name]
//     mx check -> [CE1613] "The selected attribute
//     'Bench.Customer.$Customer/Name' no longer exists."
//
// They sit in one file because they are the same defect in two spellings, but
// they run in different TIERS, and that difference is worth keeping: the XPath
// one is a question about the model and needs -p, while the ContentParams one is
// answerable from the statement alone. Putting the second under --references
// would mean `mxcli check page.mdl` stayed silent on a mistake it can see.

// ---------------------------------------------------------------------------
// ContentParams: a path where a bare attribute belongs (no project needed)
// ---------------------------------------------------------------------------

// ValidateWidgetParamPaths flags a template parameter whose value carries a
// `$variable/` prefix.
//
// An association PATH is legal here — `{1} = Customer/Name` is an attribute
// reached over an association, which the writer stores as AttributeRef plus
// steps. What is not legal is rooting that path in a variable: the parameter is
// evaluated against the widget's own context object, so there is nowhere for a
// `$Customer/` to be resolved, and the writer keeps it as part of the name.
func ValidateWidgetParamPaths(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		forEachWidget(stmt, func(w *ast.WidgetV3, where string) {
			for _, prop := range []string{"ContentParams", "CaptionParams"} {
				params, ok := w.Properties[prop].([]ast.ParamAssignmentV3)
				if !ok {
					continue
				}
				for _, p := range params {
					text := paramValueText(p.Value)
					rest, bad := templateParamDefect(text)
					if !bad {
						continue
					}
					out = append(out, linter.Violation{
						RuleID:   "MDL-WIDGET24",
						Severity: linter.SeverityError,
						Message: fmt.Sprintf(
							"%s: %s parameter {%d} is written as %q — a template parameter is "+
								"evaluated against the widget's own context object, so it takes the "+
								"attribute name (or an association path to one), not a path rooted in "+
								"a variable. Write %q. Left as-is the prefix becomes part of the "+
								"attribute name and mxbuild reports CE1613 \"The selected attribute "+
								"'….%s' no longer exists\"",
							where, prop, p.Index, text, rest, text),
					})
				}
			}
		})
	}
	return out
}

// templateParamDefect decides whether a parameter value is one the writer
// cannot resolve, and what to write instead.
//
// The rule is narrower than "a variable root is wrong", because the writer
// already strips one specific prefix. Measured on 11.13.0, five shapes, each
// executed and built:
//
//	OrderNo                                       clean
//	Bench.Order_Customer/Name                     clean  (association hop)
//	$currentObject/Bench.Order_Customer/Name      clean  (prefix stripped)
//	$currentObject/OrderNo                        CE1613
//	$Customer/Name                                CE1613
//
// So `$currentObject/` is only stripped on the association branch
// (resolveAssociationAttributePath), and falls through with the prefix intact
// when the remainder is a bare attribute. Flagging every `$` root would report
// the third line, which builds — a false error on a working page.
//
// The fifth shape in that measurement, `Assoc/Entity/Attr` (the XPath spelling),
// also fails — but deciding it needs the model to say which segments are
// associations, so it belongs in the --references tier and is not claimed here.
func templateParamDefect(text string) (fix string, bad bool) {
	if !strings.HasPrefix(text, "$") {
		return "", false
	}
	slash := strings.Index(text, "/")
	if slash < 0 || slash == len(text)-1 {
		return "", false
	}
	variable, rest := text[1:slash], text[slash+1:]
	if variable == "currentObject" {
		// Stripped by the writer when what follows is an association path;
		// broken when it is a bare attribute.
		if strings.Contains(rest, "/") {
			return "", false
		}
		return rest, true
	}
	// Any other variable root is kept verbatim, whatever follows it.
	return rest, true
}

// paramValueText renders a parameter value back to the text the author wrote,
// as far as the AST preserves it. A value this cannot render is reported as
// empty and skipped, never guessed at.
func paramValueText(v any) string {
	switch e := v.(type) {
	case nil:
		return ""
	case string:
		return e
	case *ast.IdentifierExpr:
		return e.Name
	case *ast.VariableExpr:
		return "$" + e.Name
	case *ast.AttributePathExpr:
		// The only shape that can carry the defect: a variable root plus a
		// path. Rendered here rather than through expressionToString so the
		// separator the author wrote is preserved.
		if e.Variable == "" || len(e.Path) == 0 {
			return ""
		}
		return "$" + e.Variable + "/" + strings.Join(e.Path, "/")
	case ast.Expression:
		return expressionToString(e)
	}
	return ""
}

// ---------------------------------------------------------------------------
// XPath constraints: members of the entity being constrained (needs -p)
// ---------------------------------------------------------------------------

// validateXPathMembers resolves every step of a widget's XPath constraint
// against the entity the constraint filters.
//
// Only a DATABASE source is checked, because only there is the entity named in
// the statement. An association or microflow source carries its entity
// elsewhere, and an entity this cannot establish leaves the constraint
// unchecked rather than wrongly checked — the same rule as the member resolver
// in validate_member_refs.go, and for the same reason.
func validateXPathMembers(ctx *ExecContext, prog *ast.Program) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	m := &execXPathModel{ctx: ctx}
	var errs []error
	for _, stmt := range prog.Statements {
		forEachWidget(stmt, func(w *ast.WidgetV3, where string) {
			ds := w.GetDataSource()
			if ds == nil || ds.Where == "" || !strings.EqualFold(ds.Type, "database") {
				return
			}
			entityQN := ds.Reference
			if entityQN == "" || !strings.Contains(entityQN, ".") {
				return
			}
			for _, bad := range unresolvableXPathSteps(ctx, m, ds.Where, entityQN) {
				errs = append(errs, mdlerrors.NewValidation(fmt.Sprintf(
					"%s: the constraint on %s names %q, which is neither an attribute nor an "+
						"association of it — mxbuild reports this as CE1613 \"The selected %s "+
						"'%s' no longer exists\"",
					where, entityQN, bad.name, bad.kind, bad.name)))
			}
		})
	}
	return errs
}

// badStep is one step of a constraint that resolved to nothing.
type badStep struct {
	name string
	kind string // "association" or "attribute" — which mxbuild will call it
}

// unresolvableXPathSteps walks the constraint's predicate groups and returns the
// steps that name nothing on the entity.
//
// The parse is deliberately lenient (ANTLR with the error listeners removed), so
// a group it cannot read is SKIPPED rather than reported: a tree that quietly
// omits part of its input would otherwise produce an error about text nobody
// wrote. That is the same trap xpathrefs documents for the rename path, where
// the consequence is a corrupted constraint; here it is only a false error, but
// a false error still blocks a script that builds.
func unresolvableXPathSteps(ctx *ExecContext, m xpathrefs.Model, constraint, entityQN string) []badStep {
	groups := visitor.SplitXPathPredicateGroups(constraint)
	if len(groups) == 0 {
		groups = []string{constraint}
	}
	var out []badStep
	for _, g := range groups {
		expr, ok := visitor.ParseXPathConstraint(g)
		if !ok || expr == nil {
			continue
		}
		v := &xpathMemberVisitor{ctx: ctx, model: m}
		v.walk(expr, entityQN)
		out = append(out, v.bad...)
	}
	return out
}

// xpathMemberVisitor walks a parsed constraint carrying the entity each step is
// evaluated against, the same traversal xpathrefs' walker performs for renames.
type xpathMemberVisitor struct {
	ctx   *ExecContext
	model xpathrefs.Model
	bad   []badStep
	seen  map[string]bool
}

func (v *xpathMemberVisitor) add(s badStep) {
	if v.seen == nil {
		v.seen = map[string]bool{}
	}
	if v.seen[s.name] {
		return
	}
	v.seen[s.name] = true
	v.bad = append(v.bad, s)
}

func (v *xpathMemberVisitor) walk(expr ast.Expression, cur string) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.XPathPathExpr:
		v.walkPath(e.Steps, cur)
	case *ast.BinaryExpr:
		v.walk(e.Left, cur)
		v.walk(e.Right, cur)
	case *ast.UnaryExpr:
		v.walk(e.Operand, cur)
	case *ast.ParenExpr:
		v.walk(e.Inner, cur)
	case *ast.FunctionCallExpr:
		for _, a := range e.Arguments {
			v.walk(a, cur)
		}
	case *ast.IfThenElseExpr:
		v.walk(e.Condition, cur)
		v.walk(e.ThenExpr, cur)
		v.walk(e.ElseExpr, cur)
	case *ast.SourceExpr:
		v.walk(e.Expression, cur)
	case *ast.IdentifierExpr:
		v.noteBare(e.Name, cur)
	case *ast.QualifiedNameExpr:
		// A qualified name standing alone in a predicate is the shape #1049
		// reported: `[Bench.Order_Status = 'Open']`, where the name looks like
		// an association and is not one.
		v.noteQualified(e.QualifiedName.String(), cur)
	}
}

// noteBare checks a bare step, which Mendix evaluates as an attribute of cur.
func (v *xpathMemberVisitor) noteBare(name, cur string) {
	if cur == "" || name == "" {
		return
	}
	if resolveMemberOnEntity(v.ctx, cur, name) == memberMissing {
		v.add(badStep{name: name, kind: "attribute"})
	}
}

// noteQualified checks a `Module.Name` step, which is an association hop or an
// entity cast. Anything else names nothing.
func (v *xpathMemberVisitor) noteQualified(qn, cur string) {
	if qn == "" {
		return
	}
	if _, ok := v.model.AssociationTarget(qn, cur); ok {
		return
	}
	if v.model.IsEntity(qn) {
		return
	}
	// Only report when the entity it would have hung off is itself KNOWN.
	//
	// Without this the check fires on a constraint whose base entity the project
	// does not have — a page written against a module the script creates, or run
	// against the wrong app — and reports a perfectly good association as
	// missing. Caught by a control: `[Bench.Order_Customer/…]` on a project with
	// no Bench module was reported, and the association was real.
	//
	// This is the same three-valued discipline the bare-member path gets from
	// resolveMemberOnEntity: could-not-establish is silence, not a finding.
	if cur == "" || !v.model.IsEntity(cur) {
		return
	}
	// And the same discipline on the OTHER axis, which this guard does not cover:
	// the base entity being known says nothing about whether the ASSOCIATION was
	// ruled out. AssociationTarget above answers yes-or-not-established, so a hop
	// mxcli merely failed to resolve would be reported as a defect. Only a lookup
	// that actually ruled the name out becomes a finding.
	switch _, res := resolveAssociationFrom(v.ctx, qn, cur); res {
	case assocMissing, assocNotAnEnd:
		v.add(badStep{name: qn, kind: "association"})
	}
}

func (v *xpathMemberVisitor) walkPath(steps []ast.XPathStep, cur string) {
	for i, st := range steps {
		next := ""
		switch e := st.Expr.(type) {
		case *ast.IdentifierExpr:
			if i == len(steps)-1 {
				v.noteBare(e.Name, cur)
			}
		case *ast.QualifiedNameExpr:
			qn := e.QualifiedName.String()
			if t, ok := v.model.AssociationTarget(qn, cur); ok {
				next = t
			} else if v.model.IsEntity(qn) {
				next = qn
			} else {
				v.noteQualified(qn, cur)
			}
		}
		if st.Predicate != nil {
			v.walk(st.Predicate, next)
		}
		cur = next
	}
}

// execXPathModel answers xpathrefs.Model from the connected project.
type execXPathModel struct{ ctx *ExecContext }

func (m *execXPathModel) IsEntity(qn string) bool {
	b, ok := m.ctx.Backend.(entityLookupBackend)
	if !ok {
		return false
	}
	_, found := findEntityByQN(b, qn)
	return found
}

func (m *execXPathModel) AssociationTarget(qn, from string) (string, bool) {
	return associationTargetFrom(m.ctx, qn, from)
}

// ---------------------------------------------------------------------------
// Shared widget walk
// ---------------------------------------------------------------------------

// forEachWidget visits every widget a statement carries, with a label naming
// where it is. Every widget-bearing field has to be walked: a widget the walk
// misses is a widget nothing checks, and the escape is silent both ways.
func forEachWidget(stmt ast.Statement, fn func(w *ast.WidgetV3, where string)) {
	var doc string
	var roots []*ast.WidgetV3
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		doc = "page " + s.Name.String()
		roots = append(roots, s.Widgets...)
		for _, ph := range s.Placeholders {
			if ph != nil {
				roots = append(roots, ph.Widgets...)
			}
		}
	case *ast.CreateSnippetStmtV3:
		doc = "snippet " + s.Name.String()
		roots = append(roots, s.Widgets...)
	case *ast.CreateLayoutStmt:
		doc = "layout " + s.Name.String()
		roots = append(roots, s.Widgets...)
	case *ast.AlterPageStmt:
		doc = strings.ToLower(s.ContainerType) + " " + s.PageName.String()
		for _, op := range s.Operations {
			switch o := op.(type) {
			case *ast.InsertWidgetOp:
				roots = append(roots, o.Widgets...)
			case *ast.ReplaceWidgetOp:
				roots = append(roots, o.NewWidgets...)
			}
		}
	default:
		return
	}
	var walk func(w *ast.WidgetV3)
	walk = func(w *ast.WidgetV3) {
		if w == nil {
			return
		}
		fn(w, fmt.Sprintf("%s: %s %q", doc, strings.ToLower(w.Type), w.Name))
		for _, c := range w.Children {
			walk(c)
		}
	}
	for _, w := range roots {
		walk(w)
	}
}
