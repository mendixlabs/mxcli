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
// Could-not-establish is silence: an attribute reached over a path, an
// inherited one, one of a kind not measured (AutoNumber, HashedString, Binary,
// Date), an entity the script alters, a token pairing xpathTokenTypeProblem has
// not measured (the date, length and role tokens it judges are ako/mxcli#2189's
// measurements), and an enumeration whose values neither the script nor the
// project states.

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
	switch x := e.(type) {
	case *ast.ParenExpr:
		walkXPathLiteralComparisons(x.Inner, fn)
	case *ast.UnaryExpr:
		walkXPathLiteralComparisons(x.Operand, fn)
	case *ast.FunctionCallExpr:
		if strings.EqualFold(x.Name, "not") {
			for _, a := range x.Arguments {
				walkXPathLiteralComparisons(a, fn)
			}
		}
	case *ast.BinaryExpr:
		op := strings.ToLower(x.Operator)
		switch op {
		case "and", "or":
			walkXPathLiteralComparisons(x.Left, fn)
			walkXPathLiteralComparisons(x.Right, fn)
		case "=", "!=", "<>", "<", ">", "<=", ">=":
			if name, ok := xpathMemberName(x.Left); ok {
				if lit, ok := xpathLiteralOf(x.Right); ok {
					fn(name, op, lit)
				}
			} else if name, ok := xpathMemberName(x.Right); ok {
				if lit, ok := xpathLiteralOf(x.Left); ok {
					fn(name, op, lit)
				}
			}
		}
	}
}

// xpathMemberName is the member a comparison operand names: a bare attribute,
// or a qualified association (`M.E_Kid`, which the parser reads as a qualified
// name rather than an identifier).
func xpathMemberName(e ast.Expression) (string, bool) {
	switch x := e.(type) {
	case *ast.IdentifierExpr:
		return x.Name, true
	case *ast.QualifiedNameExpr:
		return x.QualifiedName.String(), true
	}
	return "", false
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
		walkXPathLiteralComparisons(expr, func(attr, op string, lit xpathLiteral) {
			at, ok := retrieveAttributeType(g.ctx, g.whole, entityQN, attr)
			if !ok {
				// Not an attribute: `[M.Assoc = '[%CurrentUser%]']` compares an
				// association with the current user (ako/mxcli#2189).
				if p := g.currentUserAssociationProblem(entityQN, attr, lit); p != "" && !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
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
	if lit.class == "string" && strings.Contains(lit.value, "[%") {
		return xpathTokenTypeProblem(class, lit.value)
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

// xpathTokenRE is a whole XPath token, `[%Name%]`; one inside a longer value
// (date arithmetic) is not judged.
var xpathTokenRE = regexp.MustCompile(`^\[%([A-Za-z_][A-Za-z0-9_]*)%\]$`)

var (
	// The date tokens are the vocabulary's (validate_token_names.go), so the two
	// cannot drift: a date token is a date wherever it is compared.
	xpathDateTokenRE   = tokenDateRe
	xpathLengthTokenRE = regexp.MustCompile(`^(Second|Minute|Hour|Day|Week|Month|Year)Length$`)
)

// xpathTokenTypeProblem is why a quoted token does not compare with an
// attribute of operand class class, or "" when it does or that has not been
// measured (ako/mxcli#2189). Measured with mx check on 11.14.0, security level
// Production — each CE0161 "Error(s) in XPath constraint.":
//
//	date token   ('[%CurrentDateTime%]', '[%BeginOfCurrentDay%]')  on a String, on an Integer
//	length token ('[%DayLength%]')                                  on an Integer
//	role token   ('[%UserRole_User%]')                              on a String, on a DateTime
//
// and clean: a date token on a DateTime (=, >). Every other pairing is left to
// mxbuild.
func xpathTokenTypeProblem(class, value string) string {
	m := xpathTokenRE.FindStringSubmatch(value)
	if m == nil {
		return ""
	}
	name := m[1]
	switch {
	case xpathDateTokenRE.MatchString(name):
		switch class {
		case "string":
			return value + " is a date and time, and a String attribute compares with a quoted string"
		case "number":
			return value + " is a date and time; a number attribute compares with an unquoted number"
		}
	case xpathLengthTokenRE.MatchString(name):
		if class == "number" {
			return value + " is quoted, so it is a string; a number attribute compares with an unquoted number"
		}
	case strings.HasPrefix(name, "UserRole_"):
		switch class {
		case "string", "datetime":
			return value + " is a user role, which compares with System.UserRoles, not with an attribute value"
		}
	}
	return ""
}

// currentUserAssociationProblem is why `[<assoc> = '[%CurrentUser%]']` fails,
// or "": the current user is a System.User, so the association must lead to
// System.User or a specialization of it. Measured on 11.14.0: an association to
// another entity is CE0161; one to System.User, or to Administration.Account
// (which extends it), is clean. An association or a destination that cannot be
// established is not judged.
func (g *grantResolver) currentUserAssociationProblem(entityQN, attr string, lit xpathLiteral) string {
	if lit.class != "string" || lit.value != "[%CurrentUser%]" || strings.Count(attr, ".") != 1 {
		return ""
	}
	dest := ""
	if g.whole != nil {
		if ends, ok := g.whole.associationEnds[strings.ToLower(attr)]; ok {
			switch entityQN {
			case ends[0]:
				dest = ends[1]
			case ends[1]:
				dest = ends[0]
			}
		}
	}
	if dest == "" {
		to, res := resolveAssociationFrom(g.ctx, attr, entityQN)
		if res != assocResolved {
			return ""
		}
		dest = to
	}
	chain, complete := scriptGeneralizationChain(g.ctx, g.whole, dest)
	if slices.Contains(chain, "System.User") || !complete {
		return ""
	}
	return fmt.Sprintf("the access rule's constraint compares %s with '[%%CurrentUser%%]' — %s leads to %s, and the "+
		"current user is a System.User: compare an association to System.User (or to an entity that extends it); "+
		"mxbuild rejects the constraint with CE0161 \"Error(s) in XPath constraint.\"", attr, attr, dest)
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
