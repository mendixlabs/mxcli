// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func mdlIDs(mf *ast.CreateMicroflowStmt) map[string]string {
	out := map[string]string{}
	for _, vi := range ValidateMicroflow(mf) {
		out[vi.RuleID] = vi.Message + " || " + vi.Suggestion
	}
	return out
}

func buildMF(t *testing.T, src string) *ast.CreateMicroflowStmt {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return prog.Statements[0].(*ast.CreateMicroflowStmt)
}

// TestValidateMicroflow_UnknownFunction covers MDL044 (findings #1): a call to a
// name that is not a Mendix expression function (e.g. randomInt) parses and
// passes a naive check but fails the build with CE0117.
func TestValidateMicroflow_UnknownFunction(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMDL bool
		wantSug string // substring expected in the suggestion (if wantMDL)
	}{
		{"randomInt is unknown", "declare $r Integer = randomInt(9);", true, "random()"},
		{"random is known", "declare $d Decimal = random();", false, ""},
		{"round(random()) is known", "declare $r Integer = round(random() * 8);", false, ""},
		{"nested known funcs", "declare $s String = toUpperCase(trim($x));", false, ""},
		{"unknown in if condition", "if isBlank($x) then\n    return;\n  end if;", true, ""},
		// count/sum/... are aggregate activities, not expression functions: MDL044
		// fires, but the hint must steer to "assign to a variable first", not a
		// did-you-mean against an unrelated math function (finding #7).
		{"count aggregate hint", "declare $ok Boolean = if count($x) > 0 then true else false;", true, "aggregate activity"},
		// FINDINGS #17: an aggregate inside a `create` attribute value must also be
		// flagged — previously only return/if/declare/set expressions were checked.
		{"aggregate in create attr", `$r = create "M"."E" (Total = formatDecimal(sum($x), '0.00'));`, true, "aggregate activity"},
		{"known func in create attr", `$r = create "M"."E" (Total = trim($x));`, false, ""},
		// #828: the object-state predicates were missing from funcTable, so MDL044
		// reported three real built-ins as hallucinated. Each builds at 0 errors on
		// mxbuild 11.13.0. This matters more since MDL044 became exec-enforced —
		// a false positive here refuses valid MDL rather than merely warning.
		{"isNew is a real built-in", "declare $b Boolean = isNew($x);", false, ""},
		{"isSynced is a real built-in", "declare $b Boolean = isSynced($x);", false, ""},
		{"isSyncing is a real built-in", "declare $b Boolean = isSyncing($x);", false, ""},
		// The control for the above: `trunc` looks like a sibling of round/floor/ceil
		// and was even listed in roundingFuncs, but Mendix has no such function —
		// `trunc($D)` is CE0117 on 11.13.0. Adding names to funcTable on resemblance
		// is how a write barrier stops catching anything.
		{"trunc is not a Mendix built-in", "declare $d Decimal = trunc($x);", true, ""},
		{"currentDeviceType is not a Mendix built-in", "declare $b Boolean = currentDeviceType() = 'Phone';", true, ""},
		// mendixlabs/mxcli#1033: a log message is an expression too, and the
		// reported repro put the unknown call there. Log statements were never
		// walked for MDL044, in a microflow or a nanoflow.
		{"unknown in log message", "log info 'device: ' + currentDeviceType();", true, ""},
		{"unknown in log template param", "log info 'device: {1}' with ({1} = currentDeviceType());", true, ""},
		{"known func in log message", "log info 'x: ' + toUpperCase($x);", false, ""},
		// currentDateTime() was listed in funcTable, so check and exec passed it and
		// the build failed: CE0117 on 11.14.0 in a microflow and a nanoflow alike.
		// The current time is the [%CurrentDateTime%] token, which builds clean —
		// the hint names it rather than a did-you-mean on spelling.
		{"currentDateTime() is not a Mendix built-in", "declare $d DateTime = currentDateTime();", true, "[%CurrentDateTime%]"},
		{"currentDateTime() nested in a call", "declare $d DateTime = addDays(currentDateTime(), 1);", true, "[%CurrentDateTime%]"},
		{"the [%CurrentDateTime%] token is accepted", "declare $d DateTime = addDays([%CurrentDateTime%], 1);", false, ""},
		// ako/mxcli#2199: `show message information (expr)` parses as a CALL to
		// information() — the type belongs after the message — and is stored as
		// '{1}' with that call as its parameter: CE0117 at "Show message activity
		// '{1}'" on 11.16.0. Show message and validation feedback were never walked.
		{"type word written before show message", "show message information (if $x = '' then 'a' else 'b');", true, "type Information"},
		{"unknown in show message template param", "show message '{1}' type Warning with ({1} = currentDeviceType());", true, ""},
		{"unknown in validation feedback", "validation feedback $x/Name message '{1}' with ({1} = currentDeviceType());", true, ""},
		{"show message expression is accepted", "show message (if $x = '' then 'a' else 'b') type Information;", false, ""},
		{"show message template is accepted", "show message '{1}' type Information with ({1} = toUpperCase($x));", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.F ($x: String)\nbegin\n  " + tc.body + "\nend;"
			ids := mdlIDs(buildMF(t, src))
			_, got := ids["MDL044"]
			if got != tc.wantMDL {
				t.Fatalf("MDL044 fired=%v, want %v (body %q)", got, tc.wantMDL, tc.body)
			}
			if tc.wantMDL && tc.wantSug != "" && !strings.Contains(ids["MDL044"], tc.wantSug) {
				t.Errorf("expected suggestion to contain %q, got %q", tc.wantSug, ids["MDL044"])
			}
		})
	}
}

// TestValidateMicroflow_DecimalFuncIntoInteger covers the #2 extension of MDL041:
// a Decimal-returning built-in (random, secondsBetween, …) assigned to an
// Integer/Long target — not just arithmetic div. Rounding funcs stay accepted.
func TestValidateMicroflow_DecimalFuncIntoInteger(t *testing.T) {
	cases := []struct {
		name    string
		params  string
		body    string
		wantMDL bool
	}{
		{"secondsBetween into Integer", "($a: DateTime, $b: DateTime)", "declare $n Integer = secondsBetween($a, $b);", true},
		{"random into Integer", "()", "declare $n Integer = random();", true},
		{"daysBetween into Long", "($a: DateTime, $b: DateTime)", "declare $n Long = daysBetween($a, $b);", true},
		{"round(random) into Integer ok", "()", "declare $n Integer = round(random() * 6);", false},
		{"secondsBetween into Decimal ok", "($a: DateTime, $b: DateTime)", "declare $n Decimal = secondsBetween($a, $b);", false},
		{"calendarMonthsBetween into Integer ok", "($a: DateTime, $b: DateTime)", "declare $n Integer = calendarMonthsBetween($a, $b);", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.F " + tc.params + "\nbegin\n  " + tc.body + "\nend;"
			_, got := mdlIDs(buildMF(t, src))["MDL041"]
			if got != tc.wantMDL {
				t.Errorf("MDL041 fired=%v, want %v (body %q)", got, tc.wantMDL, tc.body)
			}
		})
	}
}
