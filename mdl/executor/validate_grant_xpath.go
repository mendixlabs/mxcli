// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// An access rule's WHERE that names only existing members still fails the
// build when its FORM is one MxBuild refuses. xpathProblem resolves the
// members; xpathFormProblems judges the rest. Measured on a fresh 11.14.0
// project at security level Production, one entity and one grant per case,
// every one reported by mx check as
//
//	[CE0161] "Error(s) in XPath constraint." at Access rule of entity 'M.E'
//
// while `mxcli check -p` passed:
//
//	[Title = $x]                          a variable: an access rule has none
//	[M.E_Kid = empty] / [… != empty]       empty on an association (MDL047's form)
//	[System.owner = '[%CurrentUser%]']     the entity stores no owner
//	[E_Kid/M.Kid/Name = 'a']               an association written unqualified
//	[Title = '[%CurrentUser%]']            a token compared with a String
//	[Title = '[%CurrentDateTime%]'], [Title = '[%BeginOfCurrentDay%]'], [Title = '[%UserRole_User%]'],
//	[Title = 5], [Title = 2.5], [Title = true()]
//	[Cnt = 'abc'] / [Cnt = '5'] / [Cnt = '[%CurrentDateTime%]']  a string with an Integer
//	[Big = '5'] (Long), [Price = '1.5'] (Decimal)          a string with a number
//	[Created = '[%CurrentUser%]']          a user token with a DateTime (also [%UserRole_User%])
//	[Flag = 'yes']                         a Boolean with a string other than true/false
//	[Status = 'Bogus'] / [Status = 'red'] / [Status = 5]  not a value of the enumeration (case counts)
//	[M.E_Kid = '[%CurrentUser%]']          the user compared with an association to M.Kid
//
// and built clean: [Flag = 'true'], [Flag = true()], [Cnt = 5], [Cnt = 3.5],
// [Created = '2020-01-01'], [Created < '[%CurrentDateTime%]'],
// [Created = '[%BeginOfCurrentDay%]'], [Status = 'Red'], [Title = '$x'],
// [Title != empty], [Cnt = empty], [M.E_Owner = '[%CurrentUser%]'] (an
// association to System.User), [M.E_Acc = '[%CurrentUser%]'] (one to
// Administration.Account), [M.E_Kid/M.Kid], [Flag = 'false'],
// [not(Status = 'Hi')], [Status = 'Lo' or Status = 'Hi'].
//
// Left unjudged although refused: [id = '[%CurrentUser%]'] on an entity that is
// not a System.User — the implicit id is not in the member index.
//
// The rules are those cases and no more: a comparison is judged only when one
// side is an attribute of the constrained entity the project or script knows
// the type of and the other a literal, and anything else is silence.

// xpathFormProblems returns what MxBuild refuses in the form of an access
// rule's constraint, one message per finding.
func (g *grantResolver) xpathFormProblems(qn ast.QualifiedName, xpath string) []string {
	if strings.TrimSpace(xpath) == "" {
		return nil
	}
	name := qn.String()
	if g.whole.indirectEntities[name] {
		return nil
	}
	stored := accessRuleXPathConstraint(g.ctx, xpath, name)
	var out []string
	add := func(format string, args ...any) {
		out = append(out, fmt.Sprintf(format, args...)+" — MxBuild rejects this (CE0161 \"Error(s) in XPath constraint.\") at the access rule of "+name)
	}

	var exprs []ast.Expression
	for _, group := range visitor.SplitXPathPredicateGroups(stored) {
		if e, ok := visitor.ParseXPathConstraint(group); ok {
			exprs = append(exprs, e)
		}
	}

	for _, v := range xpathVariableNames(exprs) {
		add("the access rule's constraint uses the variable $%s, and an access rule has no variables: "+
			"compare with a token such as '[%%CurrentUser%%]' or a literal", v)
	}
	for _, h := range xpathAssociationEmptyMatches(stored) {
		add("the access rule's constraint tests association `%s`, and `empty` compares attributes, not associations (%s)",
			h.test(), strings.TrimSuffix(h.fix(), "."))
	}
	g.systemMemberProblems(name, stored, add)
	g.unqualifiedAssociationProblems(name, stored, add)

	cmp := &grantComparisonJudge{g: g, entity: name, ix: g.memberTypes()}
	for _, e := range exprs {
		cmp.walk(e)
	}
	for _, msg := range cmp.out {
		add("%s", msg)
	}
	return out
}

// memberTypes is the attribute and association index of the project and the
// whole script, built once per script.
func (g *grantResolver) memberTypes() memberTypeIndex {
	if g.types == nil {
		ix := checkMemberTypeIndex(g.ctx, g.whole)
		g.types = &ix
	}
	return *g.types
}

// systemMemberProblems reports a System.owner / changedBy / … on an entity
// that does not store it — validateRetrieveConstraints's rule for an access
// rule.
func (g *grantResolver) systemMemberProblems(entity, stored string, add func(string, ...any)) {
	matches := baseSystemMemberRe.FindAllStringSubmatch(blankXPathLiterals(stored), -1)
	if len(matches) == 0 {
		return
	}
	index := buildEntityIndex(g.ctx)
	if index == nil {
		return
	}
	seen := map[string]bool{}
	for _, m := range matches {
		member := m[2]
		if seen[member] {
			continue
		}
		seen[member] = true
		stores, root, known := scriptSystemMemberRoot(index, g.whole, entity, systemMemberStore[member])
		if stores || !known {
			continue
		}
		add("the access rule's constraint names System.%s, but %s does not store %s (%s)",
			member, entity, member, systemMemberStoreFix(root, member))
	}
}

// unqualifiedAssociationProblems reports an association the constraint names
// without its module — validateXPathAssociations's rule for an access rule.
func (g *grantResolver) unqualifiedAssociationProblems(entity, stored string, add func(string, ...any)) {
	attrs, ok := entityAttrNames(buildEntityIndex(g.ctx), g.whole, entity)
	if !ok {
		return
	}
	assocs := buildAssociationIndex(g.ctx)
	if assocs == nil {
		assocs = map[string]string{}
	}
	for n, q := range g.whole.associations {
		if !g.whole.ambiguousAssc[n] {
			assocs[n] = q
		}
	}
	for n := range g.whole.ambiguousAssc {
		delete(assocs, n)
	}
	for _, hit := range unqualifiedAssociationsInConstraint(stored, attrs, assocs) {
		add("the access rule's constraint names the association %s unqualified; write %s", hit.Name, hit.Qualified)
	}
}

// xpathVariableNames returns the $variables the parsed constraints use, sorted.
func xpathVariableNames(exprs []ast.Expression) []string {
	seen := map[string]bool{}
	var walk func(ast.Expression)
	walk = func(expr ast.Expression) {
		switch e := expr.(type) {
		case *ast.VariableExpr:
			seen[e.Name] = true
		case *ast.AttributePathExpr:
			seen[e.Variable] = true
		case *ast.BinaryExpr:
			walk(e.Left)
			walk(e.Right)
		case *ast.UnaryExpr:
			walk(e.Operand)
		case *ast.ParenExpr:
			walk(e.Inner)
		case *ast.FunctionCallExpr:
			for _, a := range e.Arguments {
				walk(a)
			}
		case *ast.XPathPathExpr:
			for _, st := range e.Steps {
				walk(st.Expr)
				for _, p := range st.Predicates {
					walk(p)
				}
			}
		}
	}
	for _, e := range exprs {
		walk(e)
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		if n != "" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// grantComparisonJudge judges the comparisons of an access rule's constraint
// at the top level of the constrained entity: an attribute or association of
// it on one side, a literal on the other.
type grantComparisonJudge struct {
	g      *grantResolver
	entity string
	ix     memberTypeIndex
	out    []string
}

func (j *grantComparisonJudge) walk(expr ast.Expression) {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		switch strings.ToLower(e.Operator) {
		case "and", "or":
			j.walk(e.Left)
			j.walk(e.Right)
		case "=", "!=", "<", ">", "<=", ">=":
			if !j.compare(e.Left, e.Right) {
				j.compare(e.Right, e.Left)
			}
		}
	case *ast.UnaryExpr:
		if strings.EqualFold(e.Operator, "not") {
			j.walk(e.Operand)
		}
	case *ast.ParenExpr:
		j.walk(e.Inner)
	case *ast.FunctionCallExpr:
		if e.Name == "not" {
			for _, a := range e.Arguments {
				j.walk(a)
			}
		}
	}
}

// grantComparand is a literal side of a comparison, classified.
type grantComparand struct {
	text   string // as written, for the message
	kind   string // "string", "number", "boolean"
	str    string // the string's value
	token  string // the token's name when the string is one: "CurrentUser", …
	isUser bool   // a token that is a user or user role
}

// classifyComparand returns the literal side of a comparison, or false for
// anything this check does not judge.
func classifyComparand(expr ast.Expression) (grantComparand, bool) {
	switch e := expr.(type) {
	case *ast.LiteralExpr:
		switch e.Kind {
		case ast.LiteralString:
			s, _ := e.Value.(string)
			c := grantComparand{text: "'" + s + "'", kind: "string", str: s}
			if strings.HasPrefix(s, "[%") && strings.HasSuffix(s, "%]") {
				c.token = strings.TrimSuffix(strings.TrimPrefix(s, "[%"), "%]")
				c.isUser = c.token == "CurrentUser" || strings.HasPrefix(c.token, "UserRole_")
			}
			return c, true
		case ast.LiteralInteger, ast.LiteralDecimal:
			return grantComparand{text: fmt.Sprint(e.Value), kind: "number"}, true
		}
	case *ast.FunctionCallExpr:
		if (e.Name == "true" || e.Name == "false") && len(e.Arguments) == 0 {
			return grantComparand{text: e.Name + "()", kind: "boolean"}, true
		}
	}
	return grantComparand{}, false
}

// compare judges `member op literal`; it reports whether member was one this
// check knows, so the caller does not judge the comparison the other way round.
func (j *grantComparisonJudge) compare(member, other ast.Expression) bool {
	c, ok := classifyComparand(other)
	if !ok {
		return false
	}
	switch m := member.(type) {
	case *ast.IdentifierExpr:
		if xpathImplicitMembers[m.Name] {
			return false
		}
		declaring, attr, found, known := j.ix.lookup(j.entity, m.Name)
		if !known || !found {
			return false
		}
		if msg := j.attributeMismatch(declaring, attr, c); msg != "" {
			j.out = append(j.out, msg)
		}
		return true
	case *ast.QualifiedNameExpr:
		if c.token != "CurrentUser" {
			return false
		}
		assoc := m.QualifiedName.String()
		_, target, ok := j.ix.follow(assoc, j.entity)
		if !ok || !j.chainKnownWithoutUser(target) {
			return false
		}
		j.out = append(j.out, fmt.Sprintf(
			"the access rule's constraint compares association %s, which leads to %s, with '[%%CurrentUser%%]' — "+
				"the current user is a System.User, and %s is not one", assoc, target, target))
		return true
	}
	return false
}

// chainKnownWithoutUser reports whether entity's whole generalization chain is
// known and System.User is not on it.
func (j *grantComparisonJudge) chainKnownWithoutUser(entity string) bool {
	chain := j.ix.chain(entity)
	for _, e := range chain {
		if e == "System.User" {
			return false
		}
		if _, ok := j.ix.attrs[e]; !ok {
			return false
		}
	}
	last := chain[len(chain)-1]
	return j.ix.parents[last] == ""
}

// attributeMismatch is the message for an attribute compared with a literal
// MxBuild refuses for its type, or "".
func (j *grantComparisonJudge) attributeMismatch(declaring string, attr indexedAttr, c grantComparand) string {
	refused := func(why string) string {
		return fmt.Sprintf("the access rule's constraint compares %s (%s) with %s, %s", attr.name, attr.kind, c.text, why)
	}
	switch attr.kind {
	case "String":
		switch {
		case c.kind == "number":
			return refused("a number; quote it")
		case c.kind == "boolean":
			return refused("a Boolean")
		case c.isUser || c.token == "CurrentDateTime" ||
			strings.HasPrefix(c.token, "BeginOf") || strings.HasPrefix(c.token, "EndOf"):
			return refused("a token that is not a string")
		}
	case "Integer", "Long", "Decimal":
		if c.kind == "string" {
			return refused("a string; write the number unquoted")
		}
	case "DateTime":
		if c.isUser {
			return refused("a user token, not a date")
		}
	case "Boolean":
		if c.kind == "string" && c.str != "true" && c.str != "false" {
			return refused("which is not a Boolean; use true() or false()")
		}
	case "Enumeration":
		if c.kind != "string" {
			return refused("which is not a key of the enumeration; compare with a quoted key")
		}
		enum, values, ok := j.g.enumerationValues(declaring, attr.name)
		if !ok {
			return ""
		}
		for _, v := range values {
			if v == c.str {
				return ""
			}
		}
		return refused(fmt.Sprintf("which is not a key of %s (keys: %s; case counts)", enum, strings.Join(values, ", ")))
	}
	return ""
}

// enumerationValues returns the enumeration of entity's attribute and its
// keys, from the script when it declares the entity or the enumeration and
// from the project otherwise. ok is false when either is not known for sure.
func (g *grantResolver) enumerationValues(entity, attr string) (string, []string, bool) {
	var enumQN string
	declared := false
	if decl := g.whole.entityDecls[entity]; decl != nil {
		for _, a := range decl.Attributes {
			if strings.EqualFold(a.Name, attr) {
				declared = true
				if a.Type.EnumRef != nil {
					enumQN = a.Type.EnumRef.String()
				}
			}
		}
	}
	if !declared {
		if e := buildEntityIndex(g.ctx)[entity]; e != nil {
			for _, a := range e.Attributes {
				if et, ok := a.Type.(*domainmodel.EnumerationAttributeType); ok && strings.EqualFold(a.Name, attr) {
					enumQN = et.EnumerationRef
				}
			}
		}
	}
	if enumQN == "" || g.unjudgedEnums[enumQN] {
		return "", nil, false
	}
	if v, ok := g.scriptEnums[enumQN]; ok {
		return enumQN, v, true
	}
	mod, name, _ := strings.Cut(enumQN, ".")
	e := findEnumeration(g.ctx, mod, name)
	if e == nil || len(e.Values) == 0 {
		return "", nil, false
	}
	values := make([]string, 0, len(e.Values))
	for _, ev := range e.Values {
		values = append(values, ev.Name)
	}
	return enumQN, values, true
}
