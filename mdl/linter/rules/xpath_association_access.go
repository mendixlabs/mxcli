// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// XPathAssociationAccessRule (SEC011) reports a page data source whose XPath
// crosses an association that a user role able to open the page may not read
// (ako/mxcli#945, #1337).
//
// At Production security the runtime checks member access on every association
// step of a data-source XPath, on the entity the step starts from, and refuses
// the whole retrieve when the role cannot read it — the page opens to an error
// dialog and the log says
//
//	SecurityRuntimeException: No access rights for Shop$Account/Shop.Order_Account
//
// mxbuild reports 0 errors (it checks bound paths, CE2729, not paths inside an
// XPath filter), and so did check, exec and lint.
//
// # What was measured (11.15.0, Production, a user role holding one module role)
//
//   - An association is a member of its FROM entity's access rules, and of its
//     TO entity's too when its owner is Both. Traversed from an entity it is a
//     member of, with that member at None: refused — from the TO side of an
//     owner-Both association and from the FROM side of a default one alike.
//   - Traversed from an entity it is NOT a member of (the TO side of a default
//     association): the page opens. Nothing to check, so nothing is reported.
//   - The same XPath inside an entity ACCESS RULE whose own rule leaves the
//     member at None: the page opens. Access-rule XPaths are left out.
//   - Control: with the members granted ReadOnly, every page opens.
//
// Left out because unmeasured: a step whose membership comes from a
// generalization of the entity it starts on; snippets (which roles open them
// depends on the pages that call them); microflow retrieves, which apply entity
// access only when asked to and are not page data sources.
//
// A role that may not read the step's starting entity at all is not reported
// here: for the widget's own entity that is SEC010's finding, and the step
// itself is not where the runtime stops.
type XPathAssociationAccessRule struct{}

// NewXPathAssociationAccessRule creates the SEC011 rule.
func NewXPathAssociationAccessRule() *XPathAssociationAccessRule {
	return &XPathAssociationAccessRule{}
}

func (r *XPathAssociationAccessRule) ID() string                       { return "SEC011" }
func (r *XPathAssociationAccessRule) Name() string                     { return "XPathAssociationNotReadable" }
func (r *XPathAssociationAccessRule) Category() string                 { return "security" }
func (r *XPathAssociationAccessRule) DefaultSeverity() linter.Severity { return linter.SeverityError }

func (r *XPathAssociationAccessRule) Description() string {
	return "Checks that a page data source's XPath crosses only associations the roles opening the page may read (at Production the page fails to open: \"No access rights for …\")"
}

// RequiredCatalogMode: xpath_expressions, widgets and permissions are written
// by REFRESH CATALOG FULL.
func (r *XPathAssociationAccessRule) RequiredCatalogMode() linter.CatalogMode {
	return linter.CatalogFull
}

// assocEnds is one association's ends and owner, as the catalog records them.
type assocEnds struct {
	from, to, owner string
}

// memberOf reports whether the association is a member of entity's access
// rules: of its FROM entity always, of its TO entity when the owner is Both.
func (a assocEnds) memberOf(entity string) bool {
	return entity == a.from || (strings.EqualFold(a.owner, "Both") && entity == a.to)
}

// other returns the entity a step from entity lands on.
func (a assocEnds) other(entity string) string {
	if entity == a.from {
		return a.to
	}
	if entity == a.to {
		return a.from
	}
	return ""
}

// hop is one association step and the entity it starts from.
type hop struct {
	entity, association string
}

// Check reports each (page data source, association step, user role) where the
// role opens the page, reads the step's starting entity, and not the member.
func (r *XPathAssociationAccessRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}
	ps, err := reader.GetProjectSecurity()
	if err != nil || ps == nil || ps.SecurityLevel != security.SecurityLevelProduction || len(ps.UserRoles) == 0 {
		return nil
	}

	assocs := map[string]assocEnds{}
	if rows, err := ctx.Query(`SELECT QualifiedName, COALESCE(FromEntity, ''), COALESCE(ToEntity, ''), COALESCE(Owner, '') FROM associations`); err == nil {
		for rows.Next() {
			var qn string
			var a assocEnds
			if rows.Scan(&qn, &a.from, &a.to, &a.owner) == nil {
				assocs[qn] = a
			}
		}
		rows.Close()
	}
	if len(assocs) == 0 {
		return nil
	}

	pageRoles := map[string][]string{}
	readsEntity := map[string]map[string]bool{} // entity -> module role -> any READ rule
	readsMember := map[string]map[string]bool{} // entity + "\x00" + member -> module role
	for p := range ctx.Permissions() {
		switch {
		case p.ElementType == "PAGE" && p.AccessType == "VIEW":
			pageRoles[p.ElementName] = append(pageRoles[p.ElementName], p.ModuleRoleName)
		case p.ElementType == "ENTITY" && p.AccessType == "READ" && p.MemberName == "":
			addRole(readsEntity, p.ElementName, p.ModuleRoleName)
		case p.ElementType == "ENTITY" && p.AccessType == "MEMBER_READ" && p.MemberName != "":
			addRole(readsMember, p.ElementName+"\x00"+p.MemberName, p.ModuleRoleName)
		}
	}

	widgetsOn := map[string]map[string][]string{} // page -> entity -> data widget names
	for w := range ctx.Widgets() {
		if w.EntityRef == "" || !strings.EqualFold(w.ContainerType, "page") {
			continue
		}
		if _, ok := dataWidgetLabel(w.WidgetType); !ok {
			continue
		}
		if widgetsOn[w.ContainerQualifiedName] == nil {
			widgetsOn[w.ContainerQualifiedName] = map[string][]string{}
		}
		widgetsOn[w.ContainerQualifiedName][w.EntityRef] = append(widgetsOn[w.ContainerQualifiedName][w.EntityRef], w.Name)
	}

	var violations []linter.Violation
	for x := range ctx.XPathExpressions() {
		if x.DocumentType != "PAGE" || x.ComponentType != "WIDGET" || x.TargetEntity == "" {
			continue
		}
		roles := pageRoles[x.DocumentQualifiedName]
		if len(roles) == 0 {
			continue // no role may open it: MPR007 territory
		}
		for _, h := range associationHops(x.XPathExpression, x.TargetEntity, assocs) {
			if !assocs[h.association].memberOf(h.entity) {
				continue // not a member there: measured, the runtime does not refuse it
			}
			var blind []string
			for _, ur := range ps.UserRoles {
				if !sharesRole(ur.ModuleRoles, roles) || !anyRole(readsEntity[h.entity], ur.ModuleRoles) {
					continue
				}
				if anyRole(readsMember[h.entity+"\x00"+h.association], ur.ModuleRoles) {
					continue
				}
				blind = append(blind, ur.Name)
			}
			if len(blind) > 0 {
				violations = append(violations, r.violation(x, h, blind, widgetsOn[x.DocumentQualifiedName][x.TargetEntity]))
			}
		}
	}
	return violations
}

func (r *XPathAssociationAccessRule) violation(x linter.XPathExpressionEntry, h hop, blind, widgets []string) linter.Violation {
	sort.Strings(blind)
	source := "A data source"
	if len(widgets) == 1 {
		// Named only when the page has exactly one data widget over the entity;
		// the catalog does not tie an XPath to its widget.
		source = fmt.Sprintf("The data source of '%s'", widgets[0])
	}
	entityName := h.entity[strings.LastIndex(h.entity, ".")+1:]
	module := moduleFromQualified(h.entity)
	return linter.Violation{
		RuleID:   r.ID(),
		Severity: r.DefaultSeverity(),
		Message: fmt.Sprintf("%s on page %s filters through %s from %s, which user role%s %s can open the page but may not read on %s — "+
			"at Production the page fails to open: \"No access rights for %s$%s/%s\"",
			source, x.DocumentQualifiedName, h.association, h.entity, plural(blind), quoteList(blind), h.entity,
			module, entityName, h.association),
		Location: linter.Location{
			Module:       moduleFromQualified(x.DocumentQualifiedName),
			DocumentType: "page",
			DocumentName: docNameFromQualified(x.DocumentQualifiedName),
			DocumentID:   x.DocumentID,
		},
		Suggestion: fmt.Sprintf("Grant read on the %s member in %s's access rule for a module role those user roles hold "+
			"(e.g. GRANT READ (…, %s) ON ENTITY %s TO <role>, repeating that rule's XPath so the grant merges into it), or filter without crossing it",
			h.association, h.entity, h.association[strings.LastIndex(h.association, ".")+1:], h.entity),
	}
}

func addRole(m map[string]map[string]bool, key, role string) {
	if m[key] == nil {
		m[key] = map[string]bool{}
	}
	m[key][role] = true
}

func anyRole(granted map[string]bool, roles []string) bool {
	for _, r := range roles {
		if granted[r] {
			return true
		}
	}
	return false
}

// associationHops returns every association step in constraint, each with the
// entity it starts from, walking from target. A group mxcli cannot parse, and
// any step whose entity the walk loses (a variable root, an unknown name), is
// skipped: the rule reports only steps it can place.
func associationHops(constraint, target string, assocs map[string]assocEnds) []hop {
	var hops []hop
	for _, group := range visitor.SplitXPathPredicateGroups(constraint) {
		expr, ok := visitor.ParseXPathConstraint(group)
		if !ok || expr == nil {
			continue
		}
		walkHops(expr, target, assocs, &hops)
	}
	return hops
}

func walkHops(expr ast.Expression, cur string, assocs map[string]assocEnds, hops *[]hop) {
	switch e := expr.(type) {
	case *ast.XPathPathExpr:
		for _, st := range e.Steps {
			next := ""
			if q, ok := st.Expr.(*ast.QualifiedNameExpr); ok && cur != "" {
				qn := q.QualifiedName.String()
				if a, ok := assocs[qn]; ok {
					if next = a.other(cur); next != "" {
						*hops = append(*hops, hop{entity: cur, association: qn})
					}
				} else {
					next = qn // an entity step names where the path is
				}
			}
			for _, pred := range st.Predicates {
				walkHops(pred, next, assocs, hops)
			}
			cur = next
		}
	case *ast.BinaryExpr:
		walkHops(e.Left, cur, assocs, hops)
		walkHops(e.Right, cur, assocs, hops)
	case *ast.UnaryExpr:
		walkHops(e.Operand, cur, assocs, hops)
	case *ast.ParenExpr:
		walkHops(e.Inner, cur, assocs, hops)
	case *ast.FunctionCallExpr:
		for _, a := range e.Arguments {
			walkHops(a, cur, assocs, hops)
		}
	}
}
