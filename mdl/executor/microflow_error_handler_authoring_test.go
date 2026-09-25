// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1078, second half. Fixing the describer alone made it emit an
// `on error { … }` block on eight statements whose grammar had no onErrorClause
// — MDL that does not parse. Trading a silent drop for a broken script is not a
// fix, so those eight statements now accept the clause.
//
// The reporter's activity is the first row: a create-variable with "custom with
// rollback", which is ErrorHandlingType "Custom" in the metamodel.

// buildFlowFromMDL parses a microflow and returns the objects the flow builder
// produced. Parsed rather than hand-built: the point is that the CLAUSE reaches
// the model from real source text, so a hand-made AST would test nothing.
func buildFlowFromMDL(t *testing.T, body string) *flowBuilder {
	t.Helper()
	prog, errs := visitor.Build("create microflow M.ACT_T()\nbegin\n" + body + "\nend;")
	if len(errs) > 0 {
		t.Fatalf("parsing:\n%s\nerrors: %v", body, errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
	}
	fb.buildFlowGraph(mf.Body, nil)
	return fb
}

// actionErrorHandlingTypes returns every action activity's ErrorHandlingType.
// All of them, not the first: a handler body contributes activities too, and the
// `set` case needs a declare in front of it.
func actionErrorHandlingTypes(fb *flowBuilder) []microflows.ErrorHandlingType {
	var out []microflows.ErrorHandlingType
	for _, o := range fb.objects {
		if a, ok := o.(*microflows.ActionActivity); ok {
			out = append(out, microflows.ActionErrorHandlingType(a.Action))
		}
	}
	return out
}

// countErrorHandling returns how many action activities carry the given type.
func countErrorHandling(fb *flowBuilder, want microflows.ErrorHandlingType) int {
	var n int
	for _, t := range actionErrorHandlingTypes(fb) {
		if t == want {
			n++
		}
	}
	return n
}

// Each of the eight statements #1078 opened up, with the reporter's own shape
// first. Before the grammar change every one of these was a parse error.
func TestAuthorOnError_EightStatementsThatCouldNotCarryTheClause(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"declare", "declare $name String = 'NameValue' on error { rollback $x; };"},
		{"set", "declare $name String = 'v';\n$name = 'w' on error { rollback $x; };"},
		{"change object", "change $Car (Brand = 'Opel') on error { rollback $x; };"},
		{"log", "log info node 'B' 'hi' on error { rollback $x; };"},
		{"show page", "show page M.Home on error { rollback $x; };"},
		{"close page", "close page on error { rollback $x; };"},
		{"show message", "show message 'hi' on error { rollback $x; };"},
		{"validation feedback", "validation feedback $Car/Brand message 'bad' on error { rollback $x; };"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fb := buildFlowFromMDL(t, tc.body)

			if len(actionErrorHandlingTypes(fb)) == 0 {
				t.Fatal("the builder produced no action activity")
			}
			// Custom is what Studio Pro's "custom with rollback" stores, and it is
			// what hasCustomErrorHandler needs to see for DESCRIBE to walk the
			// branch back out again. Exactly one activity carries it: the one the
			// clause was written on.
			if n := countErrorHandling(fb, microflows.ErrorHandlingTypeCustom); n != 1 {
				t.Errorf("%d activities carry ErrorHandlingType %q, want 1 — DESCRIBE "+
					"gates the whole branch on this value; got %v",
					n, microflows.ErrorHandlingTypeCustom, actionErrorHandlingTypes(fb))
			}

			// The handler's own activities and the error flow must exist, or the
			// clause parsed into nothing.
			var errFlows int
			for _, f := range fb.flows {
				if f.IsErrorHandler {
					errFlows++
				}
			}
			if errFlows != 1 {
				t.Errorf("got %d error-handler flows, want 1 — the handler body was not wired", errFlows)
			}
		})
	}
}

// Control for the table above. Without a clause the activity must keep the
// flow's own default and gain no error branch.
//
// The default is Rollback in a microflow (Abort in a nanoflow — see
// TestAuthorOnError_NanoflowKeepsAbortWithoutAClause), NOT the empty string.
// Empty was this test's original expectation and it was wrong in the direction
// that matters: it is what the writer turns into a literal "Rollback"
// regardless of flow flavour, which is CE6035 inside a nanoflow.
//
// Rollback is also exactly the value DESCRIBE must NOT render a suffix for
// (#840), so the two halves of the round trip agree: store the default, print
// nothing.
func TestAuthorOnError_AbsentClauseKeepsTheFlowDefault(t *testing.T) {
	for _, body := range []string{
		"declare $name String = 'NameValue';",
		"log info node 'B' 'hi';",
		"close page;",
	} {
		fb := buildFlowFromMDL(t, body)
		types := actionErrorHandlingTypes(fb)
		if len(types) == 0 {
			t.Fatalf("%s: no action activity", body)
		}
		for _, errType := range types {
			if errType != microflows.ErrorHandlingTypeRollback {
				t.Errorf("%s: ErrorHandlingType = %q, want %q (the microflow default)",
					body, errType, microflows.ErrorHandlingTypeRollback)
			}
		}
		// The part that actually gates DESCRIBE: no custom handler, no branch.
		if n := countErrorHandling(fb, microflows.ErrorHandlingTypeCustom); n != 0 {
			t.Errorf("%s: %d activities came back Custom with no clause written", body, n)
		}
		for _, f := range fb.flows {
			if f.IsErrorHandler {
				t.Errorf("%s: an error-handler flow was created with no clause", body)
			}
		}
	}
}

// `on error without rollback` must reach the model as its own value, not collapse
// into Custom — the two differ at runtime.
func TestAuthorOnError_WithoutRollbackIsDistinct(t *testing.T) {
	fb := buildFlowFromMDL(t,
		"declare $name String = 'v' on error without rollback { rollback $x; };")
	if n := countErrorHandling(fb, microflows.ErrorHandlingTypeCustomWithoutRollback); n != 1 {
		t.Errorf("%d activities carry %q, want 1; got %v", n,
			microflows.ErrorHandlingTypeCustomWithoutRollback, actionErrorHandlingTypes(fb))
	}
}

// MDL077: `set` is one statement form spanning activities that can and cannot
// hold the clause. Change variable can; list operation and aggregate have no
// ErrorHandlingType in the metamodel at all, so the clause is refused rather than
// accepted and discarded.
func TestMDL077_RefusesOnErrorWhereTheActivityHasNoField(t *testing.T) {
	refused := []struct{ name, body string }{
		{"list operation", "$h = head($list) on error { rollback $x; };"},
		{"aggregate", "$n = count($list) on error { rollback $x; };"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			out := checkMicroflowBodyForTest(t, tc.body)
			if !strings.Contains(out, "MDL077") {
				t.Errorf("expected MDL077 for %s, got:\n%s", tc.name, out)
			}
		})
	}

	// Control: the plain form of the SAME statement keyword is accepted, so the
	// rule is discriminating between activities and not just banning `set`.
	if out := checkMicroflowBodyForTest(t,
		"declare $a String = 'v';\n$a = 'w' on error { rollback $x; };"); strings.Contains(out, "MDL077") {
		t.Errorf("MDL077 fired on a plain set, which IS a Change variable activity:\n%s", out)
	}
}

// checkMicroflowBodyForTest runs the microflow validator over a body and returns
// the rule IDs it reported.
func checkMicroflowBodyForTest(t *testing.T, body string) string {
	t.Helper()
	prog, errs := visitor.Build("create microflow M.ACT_T()\nbegin\n" + body + "\nend;")
	if len(errs) > 0 {
		t.Fatalf("parsing:\n%s\nerrors: %v", body, errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	v := &microflowValidator{
		mfName:        "M.ACT_T",
		emptyListVars: map[string]bool{},
		varKinds:      map[string]exprcheck.TypeKind{},
	}
	v.walkBody(mf.Body)
	var b strings.Builder
	for _, viol := range v.violations {
		b.WriteString(viol.RuleID + ": " + viol.Message + "\n")
	}
	return b.String()
}

// The regression #1078 shipped and CI caught: a NANOFLOW activity with no
// clause must keep Abort, not fall through to the writer's "Rollback".
//
// Eight builders here previously set fb.ehType(nil) — context-dependent, and
// Abort inside a nanoflow. Switching them to explicitErrorHandling (which
// returns empty for "no clause") made the writer's literal "Rollback" apply
// instead, and mxbuild reports CE6035 "Error handling type is not supported" on
// every un-annotated change/log/close-page/validation-feedback activity in a
// nanoflow. Retrieve and Delete legitimately use explicitErrorHandling — their
// writers emitted a hardcoded "Rollback" those two actions accept everywhere —
// so the helper is right there and wrong here.
//
// go test ./... never saw it: the failure is in the mx-check integration suite
// (-tags integration), not the unit suite.
func TestAuthorOnError_NanoflowKeepsAbortWithoutAClause(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"change object", "change $Car (Brand = 'Opel');"},
		{"log", "log info node 'B' 'hi';"},
		{"close page", "close page;"},
		{"show message", "show message 'hi';"},
		{"validation feedback", "validation feedback $Car/Brand message 'bad';"},
		{"declare", "declare $name String = 'v';"},
		{"set", "declare $name String = 'v';\n$name = 'w';"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fb := buildNanoflowFromMDL(t, tc.body)
			for _, got := range actionErrorHandlingTypes(fb) {
				if got != microflows.ErrorHandlingTypeAbort {
					t.Errorf("nanoflow activity carries ErrorHandlingType %q, want %q — "+
						"an empty value falls through to the writer's \"Rollback\", which "+
						"mxbuild rejects as CE6035 in a nanoflow",
						got, microflows.ErrorHandlingTypeAbort)
				}
			}
		})
	}

	// Control: the same statements in a MICROFLOW must not become Abort.
	fb := buildFlowFromMDL(t, "log info node 'B' 'hi';")
	for _, got := range actionErrorHandlingTypes(fb) {
		if got == microflows.ErrorHandlingTypeAbort {
			t.Errorf("microflow activity got Abort, which is CE6035 outside a nanoflow")
		}
	}
}

// buildNanoflowFromMDL is buildFlowFromMDL with the nanoflow flag set, which is
// the only thing that changes the no-clause default.
func buildNanoflowFromMDL(t *testing.T, body string) *flowBuilder {
	t.Helper()
	prog, errs := visitor.Build("create microflow M.ACT_T()\nbegin\n" + body + "\nend;")
	if len(errs) > 0 {
		t.Fatalf("parsing:\n%s\nerrors: %v", body, errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing, isNanoflow: true,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
	}
	fb.buildFlowGraph(mf.Body, nil)
	return fb
}

// A nanoflow's error-handler BODY is walked for actions nanoflows cannot run.
// The eight statements #1078 opened up had to be added to getErrorHandling for
// that walk to reach them — otherwise a Java action nested in
// `declare … on error { … }` is accepted and fails only at build time.
func TestNanoflow_HandlerBodyOfNewStatementsIsValidated(t *testing.T) {
	// Control first: a statement that could ALREADY carry the clause. If this
	// stops reporting, the walk itself broke and the rows below prove nothing.
	if errs := nanoflowErrorsFor(t,
		"commit $Obj on error {\n  call java action M.SomeJava();\n};"); len(errs) == 0 {
		t.Fatal("control failed: a Java action inside a commit handler was accepted, " +
			"so this test cannot detect anything")
	}

	for _, tc := range []struct{ name, body string }{
		{"declare", "declare $n String = 'v' on error {\n  call java action M.SomeJava();\n};"},
		{"set", "declare $n String = 'v';\n$n = 'w' on error {\n  call java action M.SomeJava();\n};"},
		{"change object", "change $Car (Brand = 'x') on error {\n  call java action M.SomeJava();\n};"},
		{"log", "log info node 'B' 'hi' on error {\n  call java action M.SomeJava();\n};"},
		{"show page", "show page M.Home on error {\n  call java action M.SomeJava();\n};"},
		{"close page", "close page on error {\n  call java action M.SomeJava();\n};"},
		{"show message", "show message 'hi' on error {\n  call java action M.SomeJava();\n};"},
		{"validation feedback", "validation feedback $Car/Brand message 'b' on error {\n  call java action M.SomeJava();\n};"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if errs := nanoflowErrorsFor(t, tc.body); len(errs) == 0 {
				t.Errorf("a Java action inside this handler was accepted — the nanoflow "+
					"walk does not reach %s's error body", tc.name)
			}
		})
	}
}

// nanoflowErrorsFor parses a nanoflow body and returns the nanoflow-specific
// validation errors.
func nanoflowErrorsFor(t *testing.T, body string) []string {
	t.Helper()
	prog, errs := visitor.Build("create nanoflow M.NF_T()\nbegin\n" + body + "\nreturn;\nend;")
	if len(errs) > 0 {
		t.Fatalf("parsing:\n%s\nerrors: %v", body, errs)
	}
	// validateNanoflowBody, not the exported ValidateNanoflowBody: the latter
	// runs the variable/semantic checks, the former is the disallowed-action walk
	// this test is about.
	return validateNanoflowBody(prog.Statements[0].(*ast.CreateNanoflowStmt).Body)
}

// A nanoflow accepts error handling on almost none of the eight statements
// #1078 opened up: measured on 11.14.0, only the two VARIABLE activities take a
// clause, and the other six are CE6035 whichever form is written. Refused at
// exec rather than written into a nanoflow mxbuild rejects.
func TestNanoflow_RefusesErrorHandlingWhereMendixRejectsIt(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"change object", "change $Car (Brand = 'x') on error { show message 'e'; };"},
		{"log", "log info node 'B' 'hi' on error { show message 'e'; };"},
		{"show page", "show page M.Home on error { show message 'e'; };"},
		{"close page", "close page on error { show message 'e'; };"},
		{"show message", "show message 'hi' on error { close page; };"},
		{"validation feedback", "validation feedback $Car/Brand message 'b' on error { close page; };"},
		// Log is the activity measured in all three forms; all three are CE6035,
		// which is why the rule refuses the clause rather than one spelling.
		{"log continue", "log info node 'B' 'hi' on error continue;"},
		{"log rollback", "log info node 'B' 'hi' on error rollback;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := nanoflowErrorsFor(t, tc.body)
			if !containsSubstringAny(errs, "on error") {
				t.Errorf("accepted in a nanoflow, but mxbuild reports CE6035: %v", errs)
			}
		})
	}

	// Control 1: the permissive pair. Refusing these would reject nanoflows that
	// build today — measured, both accept a custom handler on 11.14.0.
	for _, body := range []string{
		"declare $n String = 'v' on error { show message 'e'; };",
		"declare $n String = 'v';\n$n = 'w' on error { show message 'e'; };",
	} {
		if errs := nanoflowErrorsFor(t, body); containsSubstringAny(errs, "on error") {
			t.Errorf("a variable activity was refused, but Mendix accepts it: %v", errs)
		}
	}

	// Control 2: no clause at all must never be refused.
	if errs := nanoflowErrorsFor(t, "log info node 'B' 'hi';"); containsSubstringAny(errs, "on error") {
		t.Errorf("an activity with no clause was refused: %v", errs)
	}
}

func containsSubstringAny(errs []string, want string) bool {
	for _, e := range errs {
		if strings.Contains(e, want) {
			return true
		}
	}
	return false
}
