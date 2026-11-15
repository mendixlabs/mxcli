// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A Mendix token, `[%Name%]`, is lexed whatever the name, and nothing read the
// name: a typo or a token Mendix does not have passed check and exec and the
// build refused it (ako/mxcli#2271). The vocabulary below is measured, not
// transcribed — mx check 11.14.0, a copy of testdata/testapp, one flow per
// token, each probed in a microflow expression, a nanoflow expression and an
// XPath (a retrieve; a page data source for [%CurrentObject%]):
//
//	CurrentDateTime, (Begin|End)OfCurrent(Minute|Hour|Day|Week|Month|Year)[UTC],
//	(Begin|End)Of(Tomorrow|Yesterday)[UTC],
//	CurrentUser                         clean in all three
//	(Second|Minute|Hour|Day|Week|Month|Year)Length
//	UserRole_<role>                     clean in an XPath, CE0117 in a flow
//	                                    (a length token even as a plain Long)
//	CurrentObject                       clean in a page data source, CE0161 in a
//	                                    microflow retrieve, CE0117 in a flow
//	CurrentDeviceType, CurrentSession, CurrentLocale, CurrentDate, Today, Now,
//	CurrentDateTimeUTC, BeginOfCurrentQuarter, BeginOfCurrentFortnight,
//	(Begin|End)Of(Next|Previous|Last)(Minute|Hour|Day|Week|Month|Year)…,
//	CurrentDateTim, currentdatetime, BeginOfCurrentDayUtc
//	                                    CE0117 in a flow, CE0161 in an XPath
//
// (130 names in all; the Tomorrow/Yesterday family was found by probing the
// plausible neighbours of the documented ones, and is the only one that is.)
//
// Names are case-sensitive. A role token's role is validateUserRoleTokens'
// business (ako/mxcli#1723); here only its form counts. [%CurrentObject%] is
// refused in a flow and a retrieve, which have no context object, and not
// judged in any other XPath (an access rule's was not measured). An
// expression outside a flow body (a page or workflow expression) was not
// measured, so there only a name valid nowhere is reported.

const tokenNameRuleID = "MDL-TOKEN01"

var (
	tokenDateRe   = regexp.MustCompile(`^(CurrentDateTime|(Begin|End)Of(Current(Minute|Hour|Day|Week|Month|Year)|Tomorrow|Yesterday)(UTC)?)$`)
	tokenLengthRe = regexp.MustCompile(`^(Second|Minute|Hour|Day|Week|Month|Year)Length$`)
	xpathTokenAny = regexp.MustCompile(`\[%([^%\]]*)%\]`)
)

// tokenContext is where a token is written.
type tokenContext int

const (
	tokenInFlow     tokenContext = iota // a microflow or nanoflow expression
	tokenInXPath                        // an XPath constraint
	tokenInRetrieve                     // a database retrieve's XPath, which has no context object
	tokenInOther                        // any other expression (not measured)
)

// tokenNameProblem is why a token name is refused in the context, or "".
func tokenNameProblem(name string, where tokenContext) string {
	flowToken := tokenDateRe.MatchString(name) || name == "CurrentUser"
	xpathOnly := tokenLengthRe.MatchString(name) || strings.HasPrefix(name, "UserRole_") && len(name) > len("UserRole_")
	switch {
	case flowToken:
		return ""
	case xpathOnly:
		if where == tokenInFlow {
			return fmt.Sprintf("[%%%s%%] is an XPath token, valid only in an XPath constraint, not in a flow's expression", name)
		}
		return ""
	case name == "CurrentObject":
		switch where {
		case tokenInFlow:
			return "[%CurrentObject%] is not a flow token; in a flow the object is a variable (`$currentObject` in a list operation's predicate, or the flow's parameter)"
		case tokenInRetrieve:
			return "[%CurrentObject%] has no object in a retrieve's XPath; it refers to a page's context object, so compare with the flow's variable or parameter instead"
		}
		return ""
	}
	msg := fmt.Sprintf("[%%%s%%] is not a Mendix token", name)
	if near := nearestTokenName(name); near != "" {
		msg += fmt.Sprintf(" — did you mean [%%%s%%]?", near)
		if strings.EqualFold(near, name) {
			msg += " Token names are case-sensitive."
		}
	}
	return msg
}

// tokenNames are the fixed names of the vocabulary, for a suggestion.
func tokenNames() []string {
	names := []string{"CurrentDateTime", "CurrentUser", "CurrentObject"}
	for _, be := range []string{"Begin", "End"} {
		for _, p := range []string{"CurrentMinute", "CurrentHour", "CurrentDay", "CurrentWeek", "CurrentMonth", "CurrentYear", "Tomorrow", "Yesterday"} {
			names = append(names, be+"Of"+p, be+"Of"+p+"UTC")
		}
	}
	for _, p := range []string{"Second", "Minute", "Hour", "Day", "Week", "Month", "Year"} {
		names = append(names, p+"Length")
	}
	return names
}

// nearestTokenName is the vocabulary name that differs from name only in case,
// or by at most two edits; "" when none does, or more than one is as near.
func nearestTokenName(name string) string {
	best, bestD, tie := "", 3, false
	for _, n := range tokenNames() {
		if strings.EqualFold(n, name) {
			return n
		}
		d := editDistance(strings.ToLower(n), strings.ToLower(name))
		switch {
		case d < bestD:
			best, bestD, tie = n, d, false
		case d == bestD:
			tie = true
		}
	}
	if tie {
		return ""
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// validateTokenNames reports each token in the statement whose name Mendix
// does not have where it is written. It needs no project.
func validateTokenNames(stmt ast.Statement) []linter.Violation {
	var out []linter.Violation
	seen := map[string]bool{}
	report := func(name string, where tokenContext) {
		msg := tokenNameProblem(name, where)
		if msg == "" || seen[msg] {
			return
		}
		seen[msg] = true
		ce := `CE0117 "Error(s) in expression."`
		if where == tokenInXPath || where == tokenInRetrieve {
			ce = `CE0161 "Error(s) in XPath constraint."`
		}
		out = append(out, linter.Violation{
			RuleID:   tokenNameRuleID,
			Severity: linter.SeverityError,
			Message:  msg + " (mxbuild: " + ce + ")",
		})
	}
	walkXPathSlots(stmt, func(slot, xpath string) {
		where := tokenInXPath
		if slot == "retrieve" {
			where = tokenInRetrieve
		}
		for _, m := range xpathTokenAny.FindAllStringSubmatch(xpath, -1) {
			report(m[1], where)
		}
	})
	walkExpressionTokens(stmt, tokenInOther, report)
	return out
}

// walkExpressionTokens calls fn for each token in an expression v holds,
// skipping the XPath constraints walkXPathSlots reads. A flow body's
// expressions are tokenInFlow; any other expression is tokenInOther.
func walkExpressionTokens(v any, where tokenContext, fn func(name string, where tokenContext)) {
	var walk func(rv reflect.Value, where tokenContext)
	walk = func(rv reflect.Value, where tokenContext) {
		switch rv.Kind() {
		case reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem(), where)
			}
		case reflect.Pointer:
			if rv.IsNil() {
				return
			}
			switch n := rv.Interface().(type) {
			case *ast.TokenExpr:
				fn(n.Token, where)
				return
			case *ast.CreateMicroflowStmt, *ast.CreateNanoflowStmt, *ast.AlterFlowStmt:
				where = tokenInFlow
			case *ast.RetrieveStmt:
				// A database retrieve's Where is an XPath, read by walkXPathSlots.
				if n.StartVariable == "" && n.Where != nil {
					el := rv.Elem()
					for i := 0; i < el.NumField(); i++ {
						if f := el.Type().Field(i); f.IsExported() && f.Name != "Where" {
							walk(el.Field(i), where)
						}
					}
					return
				}
			}
			walk(rv.Elem(), where)
		case reflect.Struct:
			for i := 0; i < rv.NumField(); i++ {
				if rv.Type().Field(i).IsExported() {
					walk(rv.Field(i), where)
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i), where)
			}
		case reflect.Map:
			iter := rv.MapRange()
			for iter.Next() {
				walk(iter.Value(), where)
			}
		}
	}
	walk(reflect.ValueOf(v), where)
}
