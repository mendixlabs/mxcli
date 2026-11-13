// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// An access rule's WHERE whose members all resolve can still fail the build
// (ako/mxcli#2099, mendixlabs/mxcli#1803). Since #1472 the grant check resolved
// the members a constraint names (grantResolver.xpathProblem); it applied none
// of the FORM rules the retrieve and widget constraints already had, and nothing
// anywhere typed a literal against the attribute it is compared with. Measured
// on mxbuild 11.14.0 at security level Production, one entity and one access
// rule per constraint, so each error is attributed to exactly one form:
//
//	CE0161 "Error(s) in XPath constraint." at Access rule of entity …
//	  [Title = $x]  [M.E_User = $currentUser]  [M.E_User = $CurrentUser]
//	  [Title = $currentUser/Name]  [Title = $currentObject/Title]  [contains(Title, $x)]
//	  [M.E_Kid = empty]  [M.E_Kid != empty]
//	  '[%CurrentUser%]' on String, Integer, Long, Decimal, Boolean, DateTime, Enumeration
//	  '[%CurrentUser%]' on an association to M.Kid
//	  [Cnt = '5']  ['5' = Cnt]  [Cnt > '5']  [Cnt = 'abc']  [Cnt = '']  [Cnt = true()]
//	  [Big = '5']  [Big = 'abc']  [Amt = '5']  [Amt = '5.5']  [Amt = true()]
//	  [Title = 5]  [Title = 5.5]  [Title != 5]  [Title = true()]
//	  [Flag = 'yes']  [Flag = 1]   [D = 5]  [D = true()]
//	  [Status = 'red']  [Status != 'red']  [Status = 'Nope']  [Status = 'Green'] (the caption)
//	  [Status = '']  [Status = 5]  [Status = true()]
//	  [System.owner = '[%CurrentUser%]'] on an entity that stores no owner
//	  [E_Kid/M.Kid/Name = 'a']
//	0 errors (the legal twins)
//	  [M.E_User = '[%CurrentUser%]'], and on an association to a specialization
//	  of System.User; [System.owner = '[%CurrentUser%]'] on an entity storing owner
//	  [not(M.E_Kid/M.Kid)]  [Title = empty]  [Cnt = empty]  [Status = empty]
//	  [Cnt = 5]  [Cnt = 5.5]  [Cnt = -5]  [Big = 5.5]  [Amt = 5]  [Title = '5']  [Title = '']
//	  [Flag = 'true']  [Flag = 'True']  [Flag = 'TRUE']  [Flag = 'false']  [Flag = true]
//	  [Flag = false()]  [D = '2020-01-01']  [D = 'abc']  [D = '[%CurrentDateTime%]']
//	  [Status = 'Red']  [Status = 'Grn']  [M.E_Kid/M.Kid/Name = 'a']
//
// The rules run where the constraint's other checks do. The two that need no
// project — a variable, `empty` on an association — are part of the per-grant
// no-project pass (ValidateGrantEntityAccess); the three that need the entity
// are part of the reference pass (grantResolver.xpathFormProblems), which exec
// runs too. The association and System-member rules are the retrieve's own
// functions, not a third copy.
//
// Tokens other than [%CurrentUser%], measured the same way: a quoted token is
// a string, so any token on a number is CE0161, like '5'; a date token
// ([%CurrentDateTime%], [%BeginOf…%], [%EndOf…%]) or a user role token
// ([%UserRole_…%]) on a String is CE0161; a user role token on a DateTime is
// CE0161, a date token on one builds. And '[%CurrentUser%]' compared with an
// association whose far end is not System.User or a specialization of it is
// CE0161.
//
//	CE0161    [Title = '[%CurrentDateTime%]']  [Title = '[%BeginOfCurrentDay%]']
//	          [Title = '[%UserRole_User%]']  [D = '[%UserRole_User%]']
//	          [Cnt = '[%CurrentDateTime%]']  [Cnt = '[%DayLength%]']
//	          [M.E_Kid = '[%CurrentUser%]'] (M.Kid is not a user)
//	0 errors  [M.E_Acc = '[%CurrentUser%]'] (Administration.Account, a
//	          specialization of System.User)
//
// Could-not-establish is silence: an attribute reached over a path, an
// inherited one, one of a kind not measured (AutoNumber, HashedString, Binary,
// Date), an entity the script alters, a token on a Boolean or enumeration, any
// other token on a String or DateTime, an association the entity is not the
// FROM end of, a far end whose generalization chain cannot be read to the root,
// and an enumeration whose values neither the script nor the project states.

// accessRuleVariableRule reports a variable in an access rule. A flow, a page
// and a user task have variables to hand an XPath constraint; an access rule
// has none — not even $currentUser, which the rule spells '[%CurrentUser%]'.
const accessRuleVariableRule = "MDL-XPATH04"

// xpathVariableRe matches a variable reference, after blankXPathLiterals.
var xpathVariableRe = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)

// accessRuleXPathFormViolations are the form rules an access rule's WHERE can
// be judged on without a project: a variable (MDL-XPATH04) and `empty` compared
// with an association (MDL047, the retrieve's rule and its matcher).
func accessRuleXPathFormViolations(stmt *ast.GrantEntityAccessStmt, loc linter.Location) []linter.Violation {
	xpath := stmt.XPathConstraint
	if strings.TrimSpace(xpath) == "" {
		return nil
	}
	entity := stmt.Entity.String()
	var out []linter.Violation
	seen := map[string]bool{}
	for _, v := range xpathVariableRe.FindAllString(blankXPathLiterals(xpath), -1) {
		if seen[v] {
			continue
		}
		seen[v] = true
		fix := "Write the value into the constraint, or use a token: `'[%CurrentUser%]'` compared with an association to System.User (or System.owner)."
		if strings.EqualFold(v, "$currentUser") {
			fix = "An access rule spells the current user as a token: `[" + "<Module.Assoc> = '[%CurrentUser%]']`, the association leading to System.User."
		}
		out = append(out, linter.Violation{
			RuleID:   accessRuleVariableRule,
			Severity: linter.SeverityError,
			Location: loc,
			Message: fmt.Sprintf("access rule on %s: the constraint uses the variable %s — an access rule has no variables, "+
				"and mxbuild rejects it with CE0161 \"Error(s) in XPath constraint.\"", entity, v),
			Suggestion: fix,
		})
	}
	for _, h := range xpathAssociationEmptyMatches(xpath) {
		out = append(out, linter.Violation{
			RuleID:   "MDL047",
			Severity: linter.SeverityError,
			Location: loc,
			Message: fmt.Sprintf("access rule on %s: the constraint tests association `%s`, which Mendix XPath does not support "+
				"(CE0161 \"Error(s) in XPath constraint\") — `empty` compares attributes, not associations", entity, h.test()),
			Suggestion: h.fix(),
		})
	}
	return out
}

// xpathFormProblems are the project-tier form rules for an access rule's WHERE,
// judged on the constraint as exec stores it: an association named without its
// module (the retrieve's MDL-XPATH01 function), a System member the entity does
// not store (the retrieve's function), and a literal whose type the attribute
// it is compared with does not take. Run only once the members resolve — with
// a CE1613 standing, mxbuild reports no CE0161 at all.
func (g *grantResolver) xpathFormProblems(qn ast.QualifiedName, xpath string) []string {
	if strings.TrimSpace(xpath) == "" {
		return nil
	}
	name := qn.String()
	if g.whole.indirectEntities[name] {
		return nil
	}
	stored := accessRuleXPathConstraint(g.ctx, xpath, name)
	ref := []retrieveConstraintRef{{entity: name, constraint: stored, stored: stored}}
	var out []string
	for _, m := range validateXPathAssociations(g.ctx, ref, g.whole) {
		out = append(out, "access rule "+m)
	}
	for _, m := range validateRetrieveConstraints(g.ctx, ref, g.whole) {
		out = append(out, "access rule "+m)
	}
	if !g.whole.alteredEntities[name] {
		out = append(out, g.literalTypeProblems(name, stored)...)
	}
	return out
}

// xpathLiteral is a literal compared with an attribute.
type xpathLiteral struct {
	class string // "string", "number", "boolean"
	value string // the string's value; the text otherwise
}

// xpathLiteralOf classifies a comparison operand as a literal. `empty` and
// anything that is not a literal is not one.
func xpathLiteralOf(e ast.Expression) (xpathLiteral, bool) {
	switch x := e.(type) {
	case *ast.LiteralExpr:
		switch x.Kind {
		case ast.LiteralString:
			s, _ := x.Value.(string)
			return xpathLiteral{"string", s}, true
		case ast.LiteralInteger, ast.LiteralDecimal:
			return xpathLiteral{"number", fmt.Sprint(x.Value)}, true
		case ast.LiteralBoolean:
			return xpathLiteral{"boolean", fmt.Sprint(x.Value)}, true
		}
	case *ast.UnaryExpr:
		if x.Operator == "-" {
			if l, ok := xpathLiteralOf(x.Operand); ok && l.class == "number" {
				return xpathLiteral{"number", "-" + l.value}, true
			}
		}
	case *ast.FunctionCallExpr:
		if len(x.Arguments) == 0 && (strings.EqualFold(x.Name, "true") || strings.EqualFold(x.Name, "false")) {
			return xpathLiteral{"boolean", strings.ToLower(x.Name) + "()"}, true
		}
	}
	return xpathLiteral{}, false
}

func (l xpathLiteral) text() string {
	if l.class == "string" {
		return "'" + strings.ReplaceAll(l.value, "'", "''") + "'"
	}
	return l.value
}

// walkXPathLiteralComparisons calls fn for each comparison of a bare attribute
// with a literal, either way round, through and/or, not() and parentheses.
func walkXPathLiteralComparisons(e ast.Expression, fn func(attr, op string, lit xpathLiteral)) {
	walkXPathMemberComparisons(e, func(member ast.Expression, op string, lit xpathLiteral) {
		if id, ok := member.(*ast.IdentifierExpr); ok {
			fn(id.Name, op, lit)
		}
	})
}

// walkXPathMemberComparisons calls fn for each comparison of a member — a bare
// attribute or a qualified association — with a literal, either way round,
// through and/or, not() and parentheses.
func walkXPathMemberComparisons(e ast.Expression, fn func(member ast.Expression, op string, lit xpathLiteral)) {
	switch x := e.(type) {
	case *ast.ParenExpr:
		walkXPathMemberComparisons(x.Inner, fn)
	case *ast.UnaryExpr:
		walkXPathMemberComparisons(x.Operand, fn)
	case *ast.FunctionCallExpr:
		if strings.EqualFold(x.Name, "not") {
			for _, a := range x.Arguments {
				walkXPathMemberComparisons(a, fn)
			}
		}
	case *ast.BinaryExpr:
		op := strings.ToLower(x.Operator)
		switch op {
		case "and", "or":
			walkXPathMemberComparisons(x.Left, fn)
			walkXPathMemberComparisons(x.Right, fn)
		case "=", "!=", "<>", "<", ">", "<=", ">=":
			if isXPathMember(x.Left) {
				if lit, ok := xpathLiteralOf(x.Right); ok {
					fn(x.Left, op, lit)
				}
			} else if isXPathMember(x.Right) {
				if lit, ok := xpathLiteralOf(x.Left); ok {
					fn(x.Right, op, lit)
				}
			}
		}
	}
}

func isXPathMember(e ast.Expression) bool {
	switch e.(type) {
	case *ast.IdentifierExpr, *ast.QualifiedNameExpr:
		return true
	}
	return false
}

// literalTypeProblems reports each literal in the constraint whose type the
// attribute it is compared with does not take (the table in the file comment).
func (g *grantResolver) literalTypeProblems(entityQN, constraint string) []string {
	groups := visitor.SplitXPathPredicateGroups(constraint)
	if len(groups) == 0 {
		groups = []string{constraint}
	}
	var out []string
	seen := map[string]bool{}
	for _, grp := range groups {
		expr, ok := visitor.ParseXPathConstraint(grp)
		if !ok || expr == nil {
			continue
		}
		walkXPathMemberComparisons(expr, func(member ast.Expression, op string, lit xpathLiteral) {
			if qn, ok := member.(*ast.QualifiedNameExpr); ok {
				if p := g.currentUserAssociationProblem(entityQN, qn.QualifiedName.String(), op, lit); p != "" && !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
				return
			}
			attr := member.(*ast.IdentifierExpr).Name
			at, ok := retrieveAttributeType(g.ctx, g.whole, entityQN, attr)
			if !ok {
				return
			}
			msg := g.literalTypeProblem(at, op, lit)
			if msg == "" {
				return
			}
			p := fmt.Sprintf("the access rule's constraint compares %s (%s) with %s — %s; mxbuild rejects the constraint "+
				"with CE0161 \"Error(s) in XPath constraint.\"", attr, xpathOperandTypeName(at), lit.text(), msg)
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		})
	}
	return out
}

// literalTypeProblem is why an attribute of type at does not take lit, or ""
// when it does or that cannot be established.
func (g *grantResolver) literalTypeProblem(at ast.DataType, op string, lit xpathLiteral) string {
	class := xpathOperandClass(at.Kind)
	if class == "" {
		return ""
	}
	if lit.class == "string" && lit.value == "[%CurrentUser%]" {
		return "[%CurrentUser%] is the current user object, which compares with an association to System.User " +
			"(or System.owner), not with an attribute value"
	}
	if tok, ok := xpathToken(lit); ok && class != "number" {
		// On a number every quoted value is refused, a token included (the
		// number case below). Elsewhere only the measured pairs are judged.
		return tokenTypeProblem(class, tok)
	}
	switch class {
	case "string":
		if lit.class != "string" {
			return "a String attribute compares with a quoted string"
		}
	case "number":
		if lit.class != "number" {
			return "a number attribute compares with an unquoted number, such as 5 rather than '5'"
		}
	case "boolean":
		if lit.class == "number" || (lit.class == "string" && !strings.EqualFold(lit.value, "true") && !strings.EqualFold(lit.value, "false")) {
			return "a Boolean attribute compares with true() or false()"
		}
	case "datetime":
		if lit.class != "string" {
			return "a DateTime attribute compares with a date string or a date token such as '[%CurrentDateTime%]'"
		}
	case "enumeration":
		if op != "=" && op != "!=" {
			return ""
		}
		enumQN := qualifiedEnumRef(at)
		if lit.class != "string" {
			return "an enumeration attribute compares with the quoted key of one of " + enumQN + "'s values"
		}
		values, known := g.enumValues(enumQN)
		if !known || slices.Contains(values, lit.value) {
			return ""
		}
		return fmt.Sprintf("%s has no value named %q (its values: %s; the key is case-sensitive, and a caption is not a key)",
			enumQN, lit.value, strings.Join(values, ", "))
	}
	return ""
}

// enumValues is an enumeration's value names as the script leaves them, or the
// project's; false when neither states them.
func (g *grantResolver) enumValues(enumQN string) ([]string, bool) {
	if enumQN == "" {
		return nil, false
	}
	if g.scriptEnums == nil {
		g.scriptEnums, g.unjudgedEnums = scriptEnumerationValues(g.prog)
	}
	if g.unjudgedEnums[enumQN] {
		return nil, false
	}
	if v, ok := g.scriptEnums[enumQN]; ok {
		return v, true
	}
	mod, name, _ := strings.Cut(enumQN, ".")
	e := findEnumeration(g.ctx, mod, name)
	if e == nil || len(e.Values) == 0 {
		return nil, false
	}
	v := make([]string, 0, len(e.Values))
	for _, ev := range e.Values {
		v = append(v, ev.Name)
	}
	return v, true
}

// xpathToken is the name of the token a string literal is, such as
// "CurrentDateTime" for '[%CurrentDateTime%]'.
func xpathToken(lit xpathLiteral) (string, bool) {
	if lit.class != "string" || !strings.HasPrefix(lit.value, "[%") || !strings.HasSuffix(lit.value, "%]") {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(lit.value, "[%"), "%]"), true
}

// isDateToken reports the tokens that stand for a date and time.
func isDateToken(tok string) bool {
	return tok == "CurrentDateTime" || strings.HasPrefix(tok, "BeginOf") || strings.HasPrefix(tok, "EndOf")
}

// tokenTypeProblem is why an attribute of the class does not take the token
// (CurrentUser is judged before), or "" when it does or that was not measured.
func tokenTypeProblem(class, tok string) string {
	userRole := strings.HasPrefix(tok, "UserRole_")
	switch class {
	case "string":
		if isDateToken(tok) {
			return "[%" + tok + "%] is a date and time, not a string; it compares with a DateTime attribute"
		}
		if userRole {
			return "[%" + tok + "%] is a user role, not a string"
		}
	case "datetime":
		if userRole {
			return "[%" + tok + "%] is a user role, not a date; a DateTime attribute compares with a date string or a date token such as '[%CurrentDateTime%]'"
		}
	}
	return ""
}

// currentUserAssociationProblem reports '[%CurrentUser%]' compared with an
// association of the entity whose far end is not System.User or a
// specialization of it. Silent unless the association is known, the entity is
// its FROM end, and the far end's generalization chain was read to the root.
func (g *grantResolver) currentUserAssociationProblem(entityQN, assocQN, op string, lit xpathLiteral) string {
	if tok, ok := xpathToken(lit); !ok || tok != "CurrentUser" || (op != "=" && op != "!=") {
		return ""
	}
	from, to, ok := g.associationEndsFor(assocQN)
	if !ok || !strings.EqualFold(from, entityQN) {
		return ""
	}
	chain, complete := g.generalizationChain(to)
	if !complete {
		return ""
	}
	for _, e := range chain {
		if strings.EqualFold(e, "System.User") {
			return ""
		}
	}
	return fmt.Sprintf("the access rule's constraint compares association %s, which leads to %s, with '[%%CurrentUser%%]' — "+
		"the current user is a System.User and %s is not one; mxbuild rejects the constraint with CE0161 \"Error(s) in XPath constraint.\"",
		assocQN, to, to)
}

// associationEndsFor is an association's FROM and TO entity, as the script
// declares it or else as the project stores it.
func (g *grantResolver) associationEndsFor(assocQN string) (string, string, bool) {
	if ends, ok := g.whole.associationEnds[strings.ToLower(assocQN)]; ok {
		return ends[0], ends[1], true
	}
	if g.assocCatalog == nil {
		var cat associationCatalog
		if b, ok := g.ctx.Backend.(entityLookupBackend); ok {
			cat = associationCatalogFor(b)
		}
		g.assocCatalog = &cat
	}
	shape, ok := g.assocCatalog.shapes[strings.ToLower(assocQN)]
	if !ok {
		return "", "", false
	}
	return shape.from, shape.to, true
}

// generalizationChain is the entity followed by its generalizations, nearest
// first, and whether the walk reached the root. An entity the script declares
// answers with its declared generalization; one it alters, renames or moves is
// not established.
func (g *grantResolver) generalizationChain(entityQN string) ([]string, bool) {
	b, _ := g.ctx.Backend.(entityLookupBackend)
	var chain []string
	seen := map[string]bool{}
	for qn := entityQN; qn != ""; {
		key := strings.ToLower(qn)
		if seen[key] || g.whole.alteredEntities[qn] || g.whole.indirectEntities[qn] {
			return chain, false
		}
		seen[key] = true
		chain = append(chain, qn)
		if gen, ok := g.whole.entityGeneralizations[key]; ok {
			qn = gen
			continue
		}
		if strings.EqualFold(qn, "System.User") {
			return chain, true
		}
		e, ok := findEntityByQN(b, qn)
		if !ok {
			return chain, false
		}
		qn = e.GeneralizationRef
	}
	return chain, true
}
