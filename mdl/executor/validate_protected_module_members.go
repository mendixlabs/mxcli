// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// checkFlowMembers is the member half of CE1406 (mendixlabs/mxcli#1498): a flow
// in an ordinary module may not use a Hidden attribute or association of a
// Protected (add-on) module, even of a Usable entity. Measured on 11.14.0, one
// microflow per shape, each refused while the member is Hidden and accepted
// once it is Usable, and accepted in the module itself and in an ordinary one:
//
//	change $r (Payload = …)            Change object activity … attribute
//	create AddOn.Request (Payload = …) Create object activity … attribute
//	change / create (AddOn.Assoc = …)  … association
//	retrieve $x from $i/AddOn.Assoc    Retrieve object(s) activity … association
//	retrieve … where Payload = …       Retrieve … attribute (and an association step)
//	return $r/Payload, $i/AddOn.Assoc  End event … attribute / association
//
// The entity of a create and of a database retrieve is checked here too: a
// Hidden entity is refused by both (Create object activity / Retrieve object(s)
// activity … is not allowed to use entity), even with no member named
// (mendixlabs/mxcli#1652).
//
// A member is reported only when it is established: its variable's entity is
// known, the member resolves on it (an association hop through
// associationTargetFrom), and its level is known — a level the script states,
// else the stored one, else Hidden for a member the script creates. Anything
// short of that is silent, because a false refusal blocks exec.
func (r *protectedRefResolver) checkFlowMembers(name ast.QualifiedName, params []ast.MicroflowParam,
	body []ast.MicroflowStatement) []string {

	if r.protected[name.Module] {
		return nil // a module uses its own Hidden members freely
	}
	var out []string
	report := func(who string, m protectedMember, ok bool) {
		if ok {
			out = appendNonEmpty(out, r.memberRef(who, m))
		}
	}
	exprs := func(who string, vars map[string]string, es ...ast.Expression) {
		for _, e := range es {
			for _, p := range exprAttributePaths(e) {
				for _, m := range r.pathMembers(vars[p.Variable], memberPathSteps(p)) {
					report(who, m, true)
				}
			}
		}
	}

	walkTypedFlow(r.ctx, params, body, func(st ast.MicroflowStatement, vars map[string]string) {
		switch s := st.(type) {
		case *ast.CreateObjectStmt:
			who := "create " + s.EntityType.String()
			out = appendNonEmpty(out, r.entityRef(who, s.EntityType, "app modules"))
			for _, ch := range s.Changes {
				m, ok := r.changeMember(s.EntityType.String(), ch.Attribute)
				report(who, m, ok)
				exprs(who, vars, ch.Value)
			}
		case *ast.ChangeObjectStmt:
			who := "change $" + s.Variable
			for _, ch := range s.Changes {
				m, ok := r.changeMember(vars[s.Variable], ch.Attribute)
				report(who, m, ok)
				exprs(who, vars, ch.Value)
			}
		case *ast.RetrieveStmt:
			who := "retrieve $" + s.Variable
			if s.StartVariable != "" {
				report(who, protectedMember{kind: "association", qn: s.Source.String()}, s.Source.Module != "")
				break
			}
			// A database retrieve names the entity itself, so a Hidden one is
			// refused with or without a where clause.
			out = appendNonEmpty(out, r.entityRef(who, s.Source, "app modules"))
			if s.Where != nil {
				for _, steps := range xpathMemberPaths(s.Where) {
					for _, m := range r.pathMembers(s.Source.String(), steps) {
						report(who, m, true)
					}
				}
			}
		case *ast.ReturnStmt:
			exprs("the end event", vars, s.Value)
		case *ast.DeclareStmt:
			exprs("declare $"+s.Variable, vars, s.InitialValue)
		case *ast.MfSetStmt:
			exprs("set "+s.Target, vars, s.Value)
		case *ast.IfStmt:
			exprs("the decision", vars, s.Condition)
		case *ast.WhileStmt:
			exprs("the while loop", vars, s.Condition)
		case *ast.LogStmt:
			exprs("the log message", vars, s.Node, s.Message)
			for _, tp := range s.Template {
				exprs("the log message", vars, tp.Value)
			}
		case *ast.CallMicroflowStmt:
			for _, a := range s.Arguments {
				exprs("call microflow "+s.MicroflowName.String(), vars, a.Value)
			}
		case *ast.CallNanoflowStmt:
			for _, a := range s.Arguments {
				exprs("call nanoflow "+s.NanoflowName.String(), vars, a.Value)
			}
		}
	})
	return out
}

// protectedMember is an attribute ("Module.Entity.Attr", with the entity that
// declares it) or an association ("Module.Assoc") a flow uses.
type protectedMember struct {
	kind   string // "attribute" or "association"
	qn     string
	entity string // the declaring entity, for an attribute
	attr   string
}

// changeMember is the member a CREATE / CHANGE item assigns on entityQN: an
// association when the name is `Module.Assoc` over a module, else an attribute
// of the entity or one it inherits.
func (r *protectedRefResolver) changeMember(entityQN, name string) (protectedMember, bool) {
	name = strings.Trim(name, `"`)
	if entityQN == "" || name == "" {
		return protectedMember{}, false
	}
	if strings.Count(name, ".") == 1 {
		return protectedMember{kind: "association", qn: name}, true
	}
	if strings.Contains(name, ".") {
		return protectedMember{}, false // Module.Entity.Attribute: not modelled
	}
	return r.attributeMember(entityQN, name)
}

// attributeMember resolves an attribute name on entityQN, walking its
// generalizations, to the entity that declares it.
func (r *protectedRefResolver) attributeMember(entityQN, attr string) (protectedMember, bool) {
	chain := []string{entityQN}
	if c, _ := generalizationChain(r.ctx, entityQN); len(c) > 0 {
		chain = c
	}
	for _, qn := range chain {
		if _, declared := r.scriptAttr[qn+"."+attr]; declared {
			return protectedMember{kind: "attribute", qn: qn + "." + attr, entity: qn, attr: attr}, true
		}
		if _, ok := r.storedAttribute(qn, attr); ok {
			return protectedMember{kind: "attribute", qn: qn + "." + attr, entity: qn, attr: attr}, true
		}
	}
	return protectedMember{}, false
}

// pathMembers resolves a member path from entityQN — the steps of `$v/A/B` or
// of an XPath path — to the members it uses: each association hop and the
// attribute it ends in. It stops at the first step it cannot resolve, so a path
// it does not understand contributes what it established up to there.
func (r *protectedRefResolver) pathMembers(entityQN string, steps []string) []protectedMember {
	var out []protectedMember
	cur := entityQN
	for i := 0; i < len(steps) && cur != ""; i++ {
		step := steps[i]
		if !strings.Contains(step, ".") {
			if m, ok := r.attributeMember(cur, step); ok {
				out = append(out, m)
			}
			return out
		}
		out = append(out, protectedMember{kind: "association", qn: step})
		to, ok := associationTargetFrom(r.ctx, step, cur)
		if !ok {
			return out
		}
		cur = to
		// XPath names the entity after a hop (`Assoc/Module.Entity/Attr`); an
		// expression path does not. Either way the step is the target entity.
		if i+1 < len(steps) && steps[i+1] == to {
			i++
		}
	}
	return out
}

// memberRef is the CE1406 report for m, or "" when m is not a Hidden member of
// a Protected module.
func (r *protectedRefResolver) memberRef(who string, m protectedMember) string {
	module, _, _ := strings.Cut(m.qn, ".")
	if !r.protected[module] {
		return ""
	}
	var fix string
	switch m.kind {
	case "attribute":
		if !r.attributeHidden(m.entity, m.attr) {
			return ""
		}
		fix = fmt.Sprintf("alter entity %s modify attribute %s: %s export level api",
			m.entity, m.attr, r.attributeTypeText(m.entity, m.attr))
	case "association":
		if !r.associationHidden(m.qn) {
			return ""
		}
		fix = fmt.Sprintf("alter association %s set export level api", m.qn)
	default:
		return ""
	}
	return fmt.Sprintf("CE1406: %s is not allowed to use %s '%s' because app modules %s — %s is a protected module "+
		"and the %s's export level is Hidden; make it usable with `%s`",
		who, m.kind, m.qn, ce1406Tail, module, m.kind, fix)
}

func (r *protectedRefResolver) attributeHidden(entityQN, attr string) bool {
	stated, defined := r.scriptAttr[entityQN+"."+attr]
	if stated != nil {
		return *stated != types.ExportLevelAPI
	}
	if a, ok := r.storedAttribute(entityQN, attr); ok {
		return a.ExportLevel != types.ExportLevelAPI
	}
	return defined
}

func (r *protectedRefResolver) associationHidden(qn string) bool {
	stated, defined := r.scriptAssoc[qn]
	if stated != nil {
		return *stated != types.ExportLevelAPI
	}
	if lvl, ok := r.storedAssociationLevel(qn); ok {
		return lvl != types.ExportLevelAPI
	}
	return defined
}

// attributeTypeText is the attribute's type as MDL spells it, for the hint —
// MODIFY ATTRIBUTE restates it.
func (r *protectedRefResolver) attributeTypeText(entityQN, attr string) string {
	if a, ok := r.storedAttribute(entityQN, attr); ok && a.Type != nil {
		return formatAttributeType(a.Type)
	}
	return "<its type>"
}

func (r *protectedRefResolver) storedAttribute(entityQN, attr string) (*domainmodel.Attribute, bool) {
	b, ok := r.ctx.Backend.(entityLookupBackend)
	if !ok {
		return nil, false
	}
	e, ok := findEntityByQN(b, entityQN)
	if !ok {
		return nil, false
	}
	for _, a := range e.Attributes {
		if a != nil && a.Name == attr {
			return a, true
		}
	}
	return nil, false
}

func (r *protectedRefResolver) storedAssociationLevel(qn string) (string, bool) {
	b, ok := r.ctx.Backend.(entityLookupBackend)
	if !ok {
		return "", false
	}
	modName, name, found := strings.Cut(qn, ".")
	if !found {
		return "", false
	}
	mod, err := b.GetModuleByName(modName)
	if err != nil || mod == nil {
		return "", false
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return "", false
	}
	for _, a := range dm.Associations {
		if a != nil && a.Name == name {
			return a.ExportLevel, true
		}
	}
	for _, a := range dm.CrossAssociations {
		if a != nil && a.Name == name {
			return a.ExportLevel, true
		}
	}
	return "", false
}

// exprAttributePaths collects every `$var/…` path in an expression.
func exprAttributePaths(e ast.Expression) []*ast.AttributePathExpr {
	var out []*ast.AttributePathExpr
	var walk func(ast.Expression)
	walk = func(e ast.Expression) {
		switch x := e.(type) {
		case *ast.AttributePathExpr:
			out = append(out, x)
		case *ast.BinaryExpr:
			walk(x.Left)
			walk(x.Right)
		case *ast.UnaryExpr:
			walk(x.Operand)
		case *ast.FunctionCallExpr:
			for _, a := range x.Arguments {
				walk(a)
			}
		case *ast.ParenExpr:
			walk(x.Inner)
		case *ast.IfThenElseExpr:
			walk(x.Condition)
			walk(x.ThenExpr)
			walk(x.ElseExpr)
		case *ast.SourceExpr:
			walk(x.Expression)
		}
	}
	walk(e)
	return out
}

func memberPathSteps(p *ast.AttributePathExpr) []string {
	if len(p.Path) > 0 {
		return p.Path
	}
	steps := make([]string, 0, len(p.Segments))
	for _, sg := range p.Segments {
		steps = append(steps, sg.Name)
	}
	return steps
}

// xpathMemberPaths collects the member paths of an XPath constraint relative to
// the retrieved entity: a bare attribute, an association, and `/` chains of them
// (`Mod.Assoc/Mod.Entity/Attr`), through and/or, not() and parentheses. A path
// that starts at a variable (`$v/…`) is not relative to the entity and is left
// out.
func xpathMemberPaths(e ast.Expression) [][]string {
	var out [][]string
	var walk func(ast.Expression)
	walk = func(e ast.Expression) {
		switch x := e.(type) {
		case *ast.SourceExpr:
			walk(x.Expression)
		case *ast.ParenExpr:
			walk(x.Inner)
		case *ast.UnaryExpr:
			walk(x.Operand)
		case *ast.FunctionCallExpr:
			for _, a := range x.Arguments {
				walk(a)
			}
		case *ast.BinaryExpr:
			if x.Operator == "/" {
				if steps, ok := xpathSteps(x); ok {
					out = append(out, steps)
				}
				return
			}
			walk(x.Left)
			walk(x.Right)
		case *ast.IdentifierExpr:
			out = append(out, []string{x.Name})
		case *ast.QualifiedNameExpr:
			out = append(out, []string{x.QualifiedName.String()})
		}
	}
	walk(e)
	return out
}

// xpathSteps flattens a left-nested `/` chain of names into its steps.
func xpathSteps(e ast.Expression) ([]string, bool) {
	switch x := e.(type) {
	case *ast.BinaryExpr:
		if x.Operator != "/" {
			return nil, false
		}
		left, ok := xpathSteps(x.Left)
		if !ok {
			return nil, false
		}
		right, ok := xpathSteps(x.Right)
		if !ok {
			return nil, false
		}
		return append(left, right...), true
	case *ast.IdentifierExpr:
		return []string{x.Name}, true
	case *ast.QualifiedNameExpr:
		return []string{x.QualifiedName.String()}, true
	}
	return nil, false
}
