// SPDX-License-Identifier: Apache-2.0

// MDL-WORKFLOW10: SET TASK OUTCOME on a user task the microflow never claimed.
//
// Completing a user task that is not assigned to the current user fails at
// RUNTIME, with nothing in either checker to warn you:
//
//	ERROR - Client: You can't complete this user task, it is not assigned to you.
//
// and the button simply does nothing. `mxcli check` passed, `mx check` passed,
// the build was clean, and the fault cost about eight minutes of runtime
// archaeology (ako/mxcli-maintenance-2).
//
// The trap is that TARGETING XPATH / TARGETING MICROFLOW decides who may SEE a
// task; it does not assign it. There is no ASSIGN TASK statement — claiming is a
// plain write to the Assignees association, and it has to come first:
//
//	change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
//	commit $Task;
//	set task outcome $Task 'Plan';
//
// A WARNING, not an error, and deliberately so: a task can legitimately be
// claimed somewhere this rule cannot see — in a nanoflow on the button, or by an
// earlier step in the process. A microflow this one calls IS read, when the
// script creates it or (with a project) the project holds it; one that cannot
// be found counts as a possible claim (ako/mxcli#943). Reporting those
// as errors would block correct apps. What the rule can say for certain is that
// THIS microflow completes a task it never assigned, which is the shape that
// fails.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// assigneesAssociation is the association a claim writes. Matched on the member
// name alone so every spelling of the qualified form is caught — quoted,
// unquoted, and with or without the System prefix, all of which parse.
const assigneesAssociation = "workflowusertask_assignees"

// assigneeAssociation is the same association's name before Mendix 10.0: a
// Reference (one user) named WorkflowUserTask_Assignee, renamed to the
// ReferenceSet Assignees in 10.0 with its GUID kept (ako/mxcli#1810). A 9.x
// claim writes this one, so it claims just the same.
const assigneeAssociation = "workflowusertask_assignee"

// isClaimAssociation reports whether a member name (unqualified) is the
// association a claim writes, under either version's name.
func isClaimAssociation(name string) bool {
	return strings.EqualFold(name, assigneesAssociation) || strings.EqualFold(name, assigneeAssociation)
}

// taskClaimSuggestion is MDL-WORKFLOW10's fix for the project's version.
// atLeast is the project version's IsAtLeast, nil when it is not known; then
// both spellings are given, each with its versions, rather than one that does
// not build on the other (CE1613 on 9.x for Assignees, ako/mxcli#1810).
func taskClaimSuggestion(task string, atLeast func(major, minor int) bool) string {
	var claim string
	switch {
	case atLeast == nil:
		claim = fmt.Sprintf("    change $%s (System.WorkflowUserTask_Assignees = [%%CurrentUser%%]);   -- Mendix 10.0+\n"+
			"    change $%s (System.WorkflowUserTask_Assignee = [%%CurrentUser%%]);    -- Mendix 9.x\n", task, task)
	case atLeast(10, 0):
		claim = fmt.Sprintf("    change $%s (System.WorkflowUserTask_Assignees = [%%CurrentUser%%]);\n", task)
	default:
		claim = fmt.Sprintf("    change $%s (System.WorkflowUserTask_Assignee = [%%CurrentUser%%]);\n"+
			"  (Mendix 9 names it WorkflowUserTask_Assignee, one user; Assignees is 10.0+ and CE1613 here.)\n", task)
	}
	return "Claim the task before setting the outcome:\n" + claim +
		fmt.Sprintf("    commit $%s;\n", task) +
		"  TARGETING XPATH / TARGETING MICROFLOW decides who may SEE a task; it does not " +
		"assign it. A microflow this one calls first, passing the task, counts when it claims " +
		"it. Ignore this if the task is claimed elsewhere — in a nanoflow on the button, or " +
		"earlier in the process."
}

// ValidateTaskClaimsIn reports SET TASK OUTCOME statements whose task was not
// assigned earlier in the same microflow — or in a microflow it calls first,
// passing the task (ako/mxcli#943). A callee the script itself creates is read;
// one it does not create cannot be, so a call that passes the task to it counts
// as a possible claim and the outcome is not reported. With a project,
// StoredTaskClaimViolations reads those callees too.
//
// version returns the project version's IsAtLeast (nil when unknown), asked
// only when a warning is written, so the suggested claim is the one that
// builds on the project's version (ako/mxcli#1810).
func ValidateTaskClaimsIn(prog *ast.Program, version func() func(major, minor int) bool) []linter.Violation {
	return validateTaskClaims(prog, nil, version)
}

// storedClaimSource reads a microflow the script does not create: whether it
// claims its parameter param directly, and which calls it passes param on to
// (callee, callee parameter). found is false when the project has no such
// microflow, which counts as a possible claim.
type storedClaimSource interface {
	TaskParameterClaims(flow, param string) (claims bool, calls [][2]string, found bool)
}

// maxClaimCallDepth bounds the walk through nested calls. Deeper than this the
// call is treated as a possible claim — the quiet direction.
const maxClaimCallDepth = 8

func validateTaskClaims(prog *ast.Program, stored storedClaimSource, version func() func(major, minor int) bool) []linter.Violation {
	if prog == nil {
		return nil
	}
	r := &taskClaimResolver{script: map[string]*ast.CreateMicroflowStmt{}, stored: stored, version: version}
	for _, stmt := range prog.Statements {
		if mf, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			r.script[strings.ToLower(mf.Name.String())] = mf
		}
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		mf, ok := stmt.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		out = append(out, r.taskClaimViolations(mf.Name, mf.Body)...)
	}
	return out
}

// StoredTaskClaimViolations is the MDL-WORKFLOW10 warnings ValidateTaskClaimsIn
// stays quiet about because the claiming callee is not in the script: with the
// project's microflows readable, a call to a stored microflow that does not
// claim the task it is passed no longer hides the outcome. Only the warnings
// ValidateTaskClaimsIn did not already report are returned, so the two can be
// printed together.
func StoredTaskClaimViolations(prog *ast.Program, b backend.FullBackend) []linter.Violation {
	if prog == nil || b == nil {
		return nil
	}
	base := map[string]bool{}
	for _, v := range validateTaskClaims(prog, nil, nil) {
		base[v.Location.DocumentName+"\x00"+v.Message] = true
	}
	var out []linter.Violation
	for _, v := range validateTaskClaims(prog, &storedTaskClaims{b: b}, func() func(major, minor int) bool {
		return projectVersionAtLeast(b)
	}) {
		if !base[v.Location.DocumentName+"\x00"+v.Message] {
			out = append(out, v)
		}
	}
	return out
}

// taskClaimResolver knows the script's microflows and, optionally, the
// project's, and answers whether a call claims the task it passes.
type taskClaimResolver struct {
	script map[string]*ast.CreateMicroflowStmt
	stored storedClaimSource
	// version returns the project version's IsAtLeast, or nil when unknown.
	// Asked only when a warning is written, so no project is opened for a
	// script that has nothing to report.
	version func() func(major, minor int) bool
}

func (r *taskClaimResolver) versionAtLeast() func(major, minor int) bool {
	if r.version == nil {
		return nil
	}
	return r.version()
}

// callClaims reports whether calling flow with the task bound to param may
// claim it. An unresolvable callee may, so it returns true.
func (r *taskClaimResolver) callClaims(flow, param string, depth int, visited map[string]bool) bool {
	key := strings.ToLower(flow) + "\x00" + strings.ToLower(param)
	if depth > maxClaimCallDepth {
		return true
	}
	if visited[key] {
		// Recursion: the cycle itself claims nothing; another path may.
		return false
	}
	visited[key] = true
	if mf, ok := r.script[strings.ToLower(flow)]; ok {
		return r.bodyClaims(mf.Body, param, depth+1, visited)
	}
	if r.stored != nil {
		if claims, calls, found := r.stored.TaskParameterClaims(flow, param); found {
			if claims {
				return true
			}
			for _, c := range calls {
				if r.callClaims(c[0], c[1], depth+1, visited) {
					return true
				}
			}
			return false
		}
	}
	return true
}

// bodyClaims reports whether a flow body claims variable v anywhere — order
// does not matter inside a callee, the call as a whole precedes the outcome.
func (r *taskClaimResolver) bodyClaims(body []ast.MicroflowStatement, v string, depth int, visited map[string]bool) bool {
	for _, st := range body {
		switch s := st.(type) {
		case *ast.ChangeObjectStmt:
			if strings.EqualFold(s.Variable, v) && changeClaimsTask(s) {
				return true
			}
		case *ast.CallMicroflowStmt:
			for _, param := range callParamsBoundTo(s, v) {
				if r.callClaims(s.MicroflowName.String(), param, depth, visited) {
					return true
				}
			}
		}
		if r.bodyClaims(nestedStatements(st), v, depth, visited) {
			return true
		}
	}
	return false
}

// callParamsBoundTo returns the callee parameters a call binds to $v.
func callParamsBoundTo(s *ast.CallMicroflowStmt, v string) []string {
	var params []string
	for _, a := range s.Arguments {
		if name, ok := plainVariable(a.Value); ok && strings.EqualFold(name, v) {
			params = append(params, a.Name)
		}
	}
	return params
}

// plainVariable returns the variable an expression is, when it is nothing else.
func plainVariable(e ast.Expression) (string, bool) {
	for {
		switch x := e.(type) {
		case *ast.SourceExpr:
			e = x.Expression
		case *ast.ParenExpr:
			e = x.Inner
		case *ast.VariableExpr:
			return strings.TrimPrefix(x.Name, "$"), true
		default:
			return "", false
		}
	}
}

// taskClaimViolations walks one flow body in order, remembering which task
// variables have been claimed by the time each outcome is set.
//
// Order matters and is the point: claiming AFTER the outcome does not help, so
// the walk records claims as it passes them rather than collecting them all
// first.
func (r *taskClaimResolver) taskClaimViolations(name ast.QualifiedName, body []ast.MicroflowStatement) []linter.Violation {
	flowName := name.String()
	claimed := map[string]bool{}
	var out []linter.Violation

	var walk func(stmts []ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, st := range stmts {
			switch s := st.(type) {
			case *ast.ChangeObjectStmt:
				if changeClaimsTask(s) {
					claimed[strings.ToLower(s.Variable)] = true
				}
			case *ast.CallMicroflowStmt:
				// A claim made by a called microflow counts (ako/mxcli#943).
				for _, a := range s.Arguments {
					v, ok := plainVariable(a.Value)
					if !ok || claimed[strings.ToLower(v)] {
						continue
					}
					if r.callClaims(s.MicroflowName.String(), a.Name, 0, map[string]bool{}) {
						claimed[strings.ToLower(v)] = true
					}
				}
			case *ast.SetTaskOutcomeStmt:
				v := strings.ToLower(s.WorkflowTaskVariable)
				if claimed[v] {
					continue
				}
				out = append(out, linter.Violation{
					RuleID:   "MDL-WORKFLOW10",
					Severity: linter.SeverityWarning,
					Location: linter.Location{
						Module:       name.Module,
						DocumentType: "microflow",
						DocumentName: name.Name,
					},
					Message: fmt.Sprintf(
						"microflow %s: completes user task $%s without assigning it first — a user task "+
							"that is not assigned to the current user cannot be completed, and this fails at "+
							"RUNTIME (\"You can't complete this user task, it is not assigned to you\") with "+
							"the button appearing to do nothing. Neither mxcli check nor mx check catches it.",
						flowName, s.WorkflowTaskVariable),
					Suggestion: taskClaimSuggestion(s.WorkflowTaskVariable, r.versionAtLeast()),
				})
			}
			// Nested bodies (loops, decisions, error handlers) can hold either half,
			// so a claim inside a branch still counts — being wrong in the quiet
			// direction is what keeps a warning worth reading.
			walk(nestedStatements(st))
		}
	}
	walk(body)
	return out
}

// changeClaimsTask reports whether a CHANGE writes the Assignees association.
func changeClaimsTask(s *ast.ChangeObjectStmt) bool {
	for _, c := range s.Changes {
		name := c.Attribute
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		if isClaimAssociation(strings.Trim(name, `"`)) {
			return true
		}
	}
	return false
}

// nestedStatements returns the sub-bodies a control-flow statement holds.
//
// Only the containers a claim or an outcome can realistically sit in. A
// statement type missing here makes the rule quieter, never noisier, which is
// the right way for it to fail.
func nestedStatements(st ast.MicroflowStatement) []ast.MicroflowStatement {
	switch s := st.(type) {
	case *ast.IfStmt:
		return append(append([]ast.MicroflowStatement{}, s.ThenBody...), s.ElseBody...)
	case *ast.LoopStmt:
		return s.Body
	case *ast.WhileStmt:
		return s.Body
	}
	return nil
}

// projectVersionAtLeast is b's project version as IsAtLeast, nil when unknown.
func projectVersionAtLeast(b backend.FullBackend) func(major, minor int) bool {
	if b == nil {
		return nil
	}
	pv := b.ProjectVersion()
	if pv == nil || pv.MajorVersion == 0 {
		return nil
	}
	return pv.IsAtLeast
}
