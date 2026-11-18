// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcatalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// An enumeration split (`case $S/C when R … end case`) needs exactly one branch per
// enumeration value plus (empty), and every value written must exist. Measured on
// MxBuild 11.16.1, one flow per project (a shared project is wrong: CE1613 is an
// unresolved reference and hides every other error), with `check` and `exec`
// accepting every rejected case:
//
//	every value + (empty), or several values in one branch   clean
//	a value left out (attribute subject, parameter subject)  CE0079
//	the same on a nanoflow                                   CE0079
//	a value that is not in the enumeration                   CE1613
//	the right value in another case                          CE1613   (values are case-sensitive)
//	a value in two branches, also inside a multi-value one   CE0771   (one per branch)
//
// The missing (empty) branch was already MDL056. CE0773, which older records list
// beside CE0079, does not appear for any of these on 11.16.1: it is the build's
// follow-on error for a branch with no configured value, which an `else` produces
// and the grammar already refuses.
const (
	// enumSplitDuplicateRule needs no enumeration: the same value twice is wrong
	// whatever the enumeration holds, so it runs with the other flow rules.
	enumSplitDuplicateRule = "MDL105"
	// enumSplitValueRule needs the enumeration's members, so it runs where the
	// script's and the project's enumerations are known (`check -p`).
	enumSplitValueRule = "MDL106"
)

// splitValueName is a branch value without any module or enumeration prefix.
func splitValueName(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.LastIndex(v, "."); i >= 0 {
		v = v[i+1:]
	}
	return v
}

func isEmptyBranch(v string) bool { return strings.EqualFold(strings.TrimSpace(v), "(empty)") }

// caseValues is every value a split's branches name, in order, (empty) left out.
func caseValues(stmt *ast.EnumSplitStmt) []string {
	var out []string
	for _, c := range stmt.Cases {
		vals := c.Values
		if len(vals) == 0 && c.Value != "" {
			vals = []string{c.Value}
		}
		for _, v := range vals {
			if !isEmptyBranch(v) {
				out = append(out, splitValueName(v))
			}
		}
	}
	return out
}

// checkEnumSplitDuplicateValues (MDL105) reports a value that two branches of one
// enumeration split both name — CE0771, one error per branch that repeats it.
// Values are case-sensitive, so `r` and `R` are not the same value.
func (v *microflowValidator) checkEnumSplitDuplicateValues(body []ast.MicroflowStatement) {
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		stmt, ok := s.(*ast.EnumSplitStmt)
		if !ok {
			return
		}
		seen := map[string]int{}
		var dups []string
		for _, val := range caseValues(stmt) {
			seen[val]++
			if seen[val] == 2 {
				dups = append(dups, val)
			}
		}
		if len(dups) == 0 {
			return
		}
		v.addViolation(enumSplitDuplicateRule, linter.SeverityError,
			fmt.Sprintf("the case statement on '%s' names %s in more than one branch — mxbuild reports CE0771 "+
				"for each branch that repeats a value (values are case-sensitive)",
				enumSplitSubject(stmt), strings.Join(dups, ", ")),
			"Name each enumeration value in exactly one branch (a branch may list several: `when R, G then`).")
	})
}

// enumSplitValueViolations (MDL106) judges every enumeration split in the script's
// flows against the members of the enumeration it splits on: a value that is not
// a member is CE1613, and — once every value written exists — a member with no
// branch is CE0079. The build reports CE1613 before CE0079, so an unknown value is
// reported alone.
//
// Only a split whose subject resolves is judged: a parameter typed by an
// enumeration, or `$Obj/Attr` where $Obj is a parameter or a retrieved/created
// object of an entity whose attribute is an enumeration. A rule-call subject, an
// association path, an enumeration the script alters or drops (its members are no
// longer the reader's), or anything the reader does not know stays silent.
func enumSplitValueViolations(reader *exprcatalog.Reader, prog *ast.Program) []linter.Violation {
	changed := map[string]bool{} // lower-cased enumerations the script alters or drops
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.AlterEnumerationStmt:
			changed[strings.ToLower(s.Name.String())] = true
		case *ast.DropEnumerationStmt:
			changed[strings.ToLower(s.Name.String())] = true
		}
	}
	var out []linter.Violation
	judge := func(kind string, name ast.QualifiedName, params []ast.MicroflowParam, body []ast.MicroflowStatement) {
		vars := enumSplitScope(reader, params, body)
		forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
			stmt, ok := s.(*ast.EnumSplitStmt)
			if !ok || stmt.Condition != nil || stmt.Variable == "" {
				return
			}
			enumQN := resolveSplitEnumeration(reader, vars, stmt.Variable)
			if enumQN == "" || changed[strings.ToLower(enumQN)] {
				return
			}
			cases, ok := reader.EnumCases(enumQN)
			if !ok {
				return
			}
			if v, bad := judgeSplitValues(kind, name, stmt, enumQN, cases); bad {
				out = append(out, v)
			}
		})
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			judge("microflow", s.Name, s.Parameters, s.Body)
		case *ast.CreateNanoflowStmt:
			judge("nanoflow", s.Name, s.Parameters, s.Body)
		}
	}
	return out
}

// splitVar is what a flow variable is known to be: an enumeration value, or an
// object of an entity.
type splitVar struct{ enumQN, entityQN string }

// enumSplitScope types the flow's parameters and the objects its statements bind.
func enumSplitScope(reader *exprcatalog.Reader, params []ast.MicroflowParam, body []ast.MicroflowStatement) map[string]splitVar {
	vars := map[string]splitVar{}
	for _, p := range params {
		dt := p.Type
		switch {
		case dt.Kind == ast.TypeEnumeration && (dt.EnumRef != nil || dt.EntityRef != nil):
			qn := ""
			if dt.EnumRef != nil {
				qn = dt.EnumRef.String()
			} else {
				qn = dt.EntityRef.String()
			}
			vars[p.Name] = splitVar{enumQN: qn}
		case dt.Kind == ast.TypeEntity && dt.EntityRef != nil:
			// A bare `Module.Name` parses as an entity reference whatever it names;
			// one the reader knows as an enumeration, and not as an entity, IS the
			// enumeration (the same settlement scriptFlowParams makes).
			qn := dt.EntityRef.String()
			if _, isEnum := reader.EnumCases(qn); isEnum && !reader.KnowsEntity(qn) {
				vars[p.Name] = splitVar{enumQN: qn}
			} else {
				vars[p.Name] = splitVar{entityQN: qn}
			}
		}
	}
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		switch st := s.(type) {
		case *ast.DeclareStmt:
			if st.Type.Kind == ast.TypeEnumeration && st.Type.EnumRef != nil {
				vars[st.Variable] = splitVar{enumQN: st.Type.EnumRef.String()}
			}
		case *ast.RetrieveStmt:
			if st.First && st.Variable != "" && st.StartVariable == "" && st.Source.Name != "" {
				vars[st.Variable] = splitVar{entityQN: st.Source.String()}
			}
		case *ast.CreateObjectStmt:
			if st.Variable != "" && st.EntityType.Name != "" {
				vars[st.Variable] = splitVar{entityQN: st.EntityType.String()}
			}
		}
	})
	return vars
}

// resolveSplitEnumeration names the enumeration a split's subject holds, or "".
func resolveSplitEnumeration(reader *exprcatalog.Reader, vars map[string]splitVar, subject string) string {
	obj, attr, isPath := strings.Cut(subject, "/")
	v, ok := vars[obj]
	if !ok {
		return ""
	}
	if !isPath {
		return v.enumQN
	}
	if v.entityQN == "" || strings.Contains(attr, "/") {
		return ""
	}
	enumQN, _ := reader.AttributeEnumQN(v.entityQN, attr)
	return enumQN
}

func judgeSplitValues(kind string, name ast.QualifiedName, stmt *ast.EnumSplitStmt, enumQN string, members []string) (linter.Violation, bool) {
	known := map[string]bool{}
	for _, m := range members {
		known[m] = true
	}
	covered := map[string]bool{}
	var unknown []string
	for _, val := range caseValues(stmt) {
		if known[val] {
			covered[val] = true
		} else {
			unknown = append(unknown, val)
		}
	}
	loc := linter.Location{Module: name.Module, DocumentType: kind, DocumentName: name.Name}
	subject := enumSplitSubject(stmt)
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return linter.Violation{
			RuleID:   enumSplitValueRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf("the case statement on '%s' names %s, which %s does not have — mxbuild reports CE1613 "+
				"(enumeration values are case-sensitive)", subject, strings.Join(unknown, ", "), enumQN),
			Location:   loc,
			Suggestion: fmt.Sprintf("%s has: %s.", enumQN, strings.Join(members, ", ")),
		}, true
	}
	var missing []string
	for _, m := range members {
		if !covered[m] {
			missing = append(missing, m)
		}
	}
	if len(missing) == 0 {
		return linter.Violation{}, false
	}
	return linter.Violation{
		RuleID:   enumSplitValueRule,
		Severity: linter.SeverityError,
		Message: fmt.Sprintf("the case statement on '%s' has no branch for %s of %s — an enumeration split needs one "+
			"for every value, and mxbuild reports CE0079 \"The '%s' condition value should be configured in properties "+
			"for an outgoing flow\"", subject, strings.Join(missing, ", "), enumQN, missing[0]),
		Location:   loc,
		Suggestion: fmt.Sprintf("Add `when %s then …` for each missing value (a branch may list several: `when %s, … then`).", missing[0], missing[0]),
	}, true
}
