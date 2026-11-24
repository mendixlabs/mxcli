// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The runner attaches its own `ON ERROR BEGIN … END ERROR` to every call in a
// test body, so an exception becomes a verdict. A call the author already gave
// an error-handling clause used to get a SECOND one appended, and the generated
// MDL did not parse: the run stopped at injection with "mismatched input 'ON'
// expecting ';'" on a line of a script the author never saw
// (ako/mxcli#1448, mendixlabs/mxcli#1550).
//
// The runner always writes its handler in upper case and these bodies write
// theirs in lower case, so counting "ON ERROR BEGIN" counts the runner's
// handlers only.
const runnerHandler = "ON ERROR BEGIN"

// generatedBy renders one test body through each of the four generators that
// rewrite a body: the endpoint runner and the legacy after-startup runner, each
// for a test with @expect and one with @throws.
func generatedBy(t *testing.T, body string) map[string]string {
	t.Helper()
	expect := parseOne(t, "/**\n * @test it runs\n * @expect $r != empty\n */\n"+body+"\n/\n")
	throws := parseOne(t, "/**\n * @test it raises\n * @throws 'boom'\n */\n"+body+"\n/\n")
	one := func(tc TestCase) *TestSuite { return &TestSuite{Name: "s", Tests: []TestCase{tc}} }
	return map[string]string{
		"endpoint @expect": GenerateTestFlows(one(expect)),
		"endpoint @throws": GenerateTestFlows(one(throws)),
		"legacy @expect":   GenerateTestRunner(one(expect)),
		"legacy @throws":   GenerateTestRunner(one(throws)),
	}
}

// runnerHandlers is how many handlers the runner should add where the legacy
// runner adds legacy: the endpoint runner adds none, since a thrown error leaves
// the test microflow and the endpoint reports it (mendixlabs/mxcli#1874).
func runnerHandlers(runner string, legacy int) int {
	if strings.HasPrefix(runner, "endpoint") {
		return 0
	}
	return legacy
}

func mustParse(t *testing.T, runner, mdl string) {
	t.Helper()
	if _, errs := visitor.Build(mdl); len(errs) > 0 {
		t.Errorf("%s: generated MDL does not parse: %v\n%s", runner, errs[0], mdl)
	}
}

// TestCallWithItsOwnHandlerKeepsIt pins the semantics: a call that continues
// on error, or has a custom handler, keeps exactly what the author wrote — the
// test exercises what the production statement does — and gets no second
// handler from the runner.
func TestCallWithItsOwnHandlerKeepsIt(t *testing.T) {
	cases := []struct{ name, body, clause string }{
		{"on error continue",
			"$r = call java action Shop.JA_Upper(Text = 'abc') on error continue;",
			"on error continue"},
		{"custom handler",
			"$r = call java action Shop.JA_Upper(Text = 'abc') on error begin\n  log error node 'T' 'handled';\nend error;",
			"on error begin"},
		{"custom handler without rollback",
			"$r = call java action Shop.JA_Upper(Text = 'abc') on error without rollback begin\n  log error node 'T' 'handled';\nend error;",
			"on error without rollback begin"},
		{"custom handler that re-raises",
			"$r = call java action Shop.JA_Upper(Text = 'abc') on error begin\n  log error node 'T' 'handled';\n  raise error;\nend error;",
			"raise error;"},
		{"deprecated brace form",
			"$r = call java action Shop.JA_Upper(Text = 'abc') on error { log error node 'T' 'handled'; };",
			"on error {"},
		{"clause on the line after the call",
			"$r = call microflow Shop.Order_Label(Text = '1')\n  on error continue;",
			"on error continue"},
		{"clause after a multi-line argument list",
			"$r = call microflow Shop.Order_Label(\n  Text = '1'\n) on error continue;",
			"on error continue"},
		{"nanoflow call", "$r = call nanoflow Shop.NF(Text = '1') on error continue;", "on error continue"},
		{"rest call",
			"$r = call rest service get 'https://example.com' returns String on error continue;",
			"on error continue"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for runner, mdl := range generatedBy(t, c.body) {
				mustParse(t, runner, mdl)
				if n := strings.Count(mdl, runnerHandler); n != 0 {
					t.Errorf("%s: the runner added %d handler(s) to a call that has its own:\n%s", runner, n, mdl)
				}
				if !strings.Contains(mdl, c.clause) {
					t.Errorf("%s: the author's clause %q is gone:\n%s", runner, c.clause, mdl)
				}
			}
		})
	}
}

// TestOnErrorRollbackIsTheDefaultSpelledOut pins the one clause that is NOT
// kept. `on error rollback` is Mendix's default, the same ErrorHandlingType an
// un-annotated call stores, so `call X()` and `call X() on error rollback` are
// one model and must get one verdict. Keeping the clause would let the error
// escape the test microflow: an ERROR instead of a FAIL, a failed @throws that
// should pass, and under the legacy runner an exception that aborts the whole
// suite's after-startup flow.
func TestOnErrorRollbackIsTheDefaultSpelledOut(t *testing.T) {
	// The statement from the report, verbatim, and its microflow twin.
	for _, body := range []string{
		"$r = call java action Shop.JA_Upper(Text = 'abc') on error rollback;",
		"$r = call microflow Shop.Order_Label(Text = '1') on error rollback;",
		"$r = call microflow Shop.Order_Label(Text = '1')\n  ON ERROR ROLLBACK;",
	} {
		for runner, mdl := range generatedBy(t, body) {
			mustParse(t, runner, mdl)
			want := runnerHandlers(runner, 1)
			if n := strings.Count(mdl, runnerHandler); n != want {
				t.Errorf("%s: want the runner's handler %d time(s) in place of `on error rollback`, got %d:\n%s", runner, want, n, mdl)
			}
			if want > 0 && strings.Contains(strings.ToLower(mdl), "on error rollback") {
				t.Errorf("%s: `on error rollback` was kept beside the runner's handler:\n%s", runner, mdl)
			}
		}
	}
}

// TestUnhandledCallStillFails is the control for both tests above: a call with
// no clause still gets the handler that turns its exception into a FAIL, and an
// @throws test still gets the one that clears its pre-set failure.
func TestUnhandledCallStillFails(t *testing.T) {
	got := generatedBy(t, "$r = call java action Shop.JA_Upper(Text = 'abc');")
	for runner, mdl := range got {
		mustParse(t, runner, mdl)
		if n, want := strings.Count(mdl, runnerHandler), runnerHandlers(runner, 1); n != want {
			t.Errorf("%s: want %d runner handler(s) on an unhandled call, got %d:\n%s", runner, want, n, mdl)
		}
	}
	// The endpoint test lets the error leave the microflow; toResult makes it
	// the FAIL (TestToResult_ThrownTestMicroflow).
	if strings.Contains(got["endpoint @expect"], "MxTestErrorMessage") {
		t.Errorf("the endpoint flow still reads the message in a handler (CE0108 under mxbuild --serve):\n%s", got["endpoint @expect"])
	}
	if !strings.Contains(got["legacy @expect"], "MXTEST:FAIL:") {
		t.Errorf("the legacy handler does not FAIL the test:\n%s", got["legacy @expect"])
	}
}

// TestOnErrorTextThatIsNotAClause pins that the clause is found by parsing, not
// by searching text: `on error` in a string literal or a comment is not a clause,
// so the call is unhandled and gets the runner's handler.
func TestOnErrorTextThatIsNotAClause(t *testing.T) {
	for name, body := range map[string]string{
		"in a string literal": "$r = call microflow Shop.Order_Label(Text = 'on error continue');",
		"in a line comment":   "$r = call microflow Shop.Order_Label(Text = '1'); -- on error continue",
		"in a block comment":  "$r = call microflow Shop.Order_Label(Text = '1') /* on error continue */;",
		"in a log after the call": "$r = call microflow Shop.Order_Label(Text = '1');\n" +
			"log info node 'T' 'call microflow X() on error rollback';",
	} {
		t.Run(name, func(t *testing.T) {
			for runner, mdl := range generatedBy(t, body) {
				mustParse(t, runner, mdl)
				if n, want := strings.Count(mdl, runnerHandler), runnerHandlers(runner, 1); n != want {
					t.Errorf("%s: want %d runner handler(s), got %d:\n%s", runner, want, n, mdl)
				}
			}
		})
	}
}

// TestHandlerDecisionIsPerCall pins that the decision is made for each call on
// its own: a handled call beside an unhandled one, on one line or nested in a
// block, leaves the unhandled one wrapped. A call inside the author's own
// handler body has no clause of its own, so it is wrapped like any other.
func TestHandlerDecisionIsPerCall(t *testing.T) {
	for name, body := range map[string]string{
		"two calls on one line": "$a = call microflow Shop.Order_Label(Text = '1'); " +
			"$r = call microflow Shop.Order_Label(Text = '2') on error continue;",
		"handled call inside an if": "if true then\n" +
			"  $r = call microflow Shop.Order_Label(Text = '1') on error continue;\n" +
			"  $a = call microflow Shop.Order_Label(Text = '2');\n" +
			"end if;",
		"call inside the author's handler": "$r = call microflow Shop.Order_Label(Text = '1') on error begin\n" +
			"  $a = call microflow Shop.Order_Label(Text = '2');\n" +
			"end error;",
	} {
		t.Run(name, func(t *testing.T) {
			for runner, mdl := range generatedBy(t, body) {
				mustParse(t, runner, mdl)
				if n, want := strings.Count(mdl, runnerHandler), runnerHandlers(runner, 1); n != want {
					t.Errorf("%s: want the runner's handler %d time(s), got %d:\n%s", runner, want, n, mdl)
				}
			}
		})
	}
}

// TestErrorRaisingCallRulesAreGrammarRules pins errorRaisingCallRules to the
// grammar. The set is matched against rule names from the parse, so a rule
// renamed in MDLMicroflow.g4 would silently stop being wrapped — every error it
// raised escaping the test microflow again (ako/mxcli#1164).
func TestErrorRaisingCallRulesAreGrammarRules(t *testing.T) {
	parser.MDLParserInit() // the rule names are filled in lazily
	known := map[string]bool{}
	for _, name := range parser.MDLParserParserStaticData.RuleNames {
		known[name] = true
	}
	if !known["callMicroflowStatement"] || len(errorRaisingCallRules) == 0 {
		t.Fatalf("no rule names to check against; the guard would pass vacuously")
	}
	for rule := range errorRaisingCallRules {
		if !known[rule] {
			t.Errorf("errorRaisingCallRules names %q, which is not a rule of the MDL grammar", rule)
		}
	}
}

// TestEveryAssertionIsCounted pins the guard against the false green that
// keeping the author's handler opens up. A custom handler that ends normally
// rejoins the flow past every statement reading the failed call's output — the
// assertions — so without a count the test returned PASS having evaluated
// nothing (measured on 11.13.0, ako/mxcli#1448). Each assertion counts itself in
// both branches of its decision, and a guard after the last one fails the test
// when the count is short.
func TestEveryAssertionIsCounted(t *testing.T) {
	src := "/**\n * @test handled\n * @expect $r = 'X'\n * @expect $r != empty\n */\n" +
		"$r = call java action Shop.JA_Fail(Text = 'a') on error begin\n  log error node 'T' 'handled';\nend error;\n/\n"
	tc := parseOne(t, src)
	suite := &TestSuite{Name: "s", Tests: []TestCase{tc}}

	for runner, c := range map[string]struct {
		mdl, counter, guard string
	}{
		"endpoint": {GenerateTestFlows(suite), assertionsEvaluatedVar,
			"IF $Verdict = '" + verdictPass + "' and " + assertionsEvaluatedVar + " < 2 THEN"},
		"legacy": {GenerateTestRunner(suite), assertionsEvaluatedVar + "_1",
			"IF " + assertionsEvaluatedVar + "_1 < 2 THEN"},
	} {
		mustParse(t, runner, c.mdl)
		inc := "SET " + c.counter + " = " + c.counter + " + 1;"
		if n := strings.Count(c.mdl, inc); n != 4 {
			t.Errorf("%s: want the counter raised in both branches of both assertions (4), got %d:\n%s", runner, n, c.mdl)
		}
		g := strings.Index(c.mdl, c.guard)
		if g < 0 {
			t.Fatalf("%s: no guard %q:\n%s", runner, c.guard, c.mdl)
		}
		if g < strings.LastIndex(c.mdl, inc) {
			t.Errorf("%s: the guard comes before the last assertion:\n%s", runner, c.mdl)
		}
		if !strings.Contains(c.mdl[g:], escapeMDLString(assertionsSkippedReason)) {
			t.Errorf("%s: the guard does not fail the test with the reason:\n%s", runner, c.mdl[g:])
		}
	}

	// Control: a test that asserts nothing has nothing to count.
	bare := parseOne(t, "/**\n * @test bare\n */\n$r = call java action Shop.JA_Upper(Text = 'a');\n/\n")
	for runner, mdl := range map[string]string{
		"endpoint": GenerateTestFlows(&TestSuite{Name: "s", Tests: []TestCase{bare}}),
		"legacy":   GenerateTestRunner(&TestSuite{Name: "s", Tests: []TestCase{bare}}),
	} {
		if strings.Contains(mdl, assertionsEvaluatedVar) {
			t.Errorf("%s: a test without @expect got an assertion counter:\n%s", runner, mdl)
		}
	}
}

// TestLegacyRunWithoutVerdictIsNotAPass pins the log runner's other false green.
// A test whose error leaves the monolithic runner — a call whose own handler
// re-raises, kept as written since ako/mxcli#1448 — aborts the flow after its
// RUN line and before any verdict. The parser created the result on RUN with
// the zero status, which is PASS: measured on 11.13.0 under --legacy-runner, the
// re-raising test reported PASS and the next one "not executed".
func TestLegacyRunWithoutVerdictIsNotAPass(t *testing.T) {
	suite := &TestSuite{Name: "s", Tests: []TestCase{
		{ID: "test_1", Name: "passes", Expects: []Expect{expectOf("$a = 1")}},
		{ID: "test_2", Name: "re-raises", Expects: []Expect{expectOf("$b = 1")}},
		{ID: "test_3", Name: "never reached", Expects: []Expect{expectOf("$c = 1")}},
	}}
	const log = "MXTEST: MXTEST:START:\n" +
		"MXTEST: MXTEST:RUN:test_1:passes\n" +
		"MXTEST: MXTEST:PASS:test_1\n" +
		"MXTEST: MXTEST:RUN:test_2:re-raises\n"
	got := ParseLogResults(strings.NewReader(log), suite, false)
	if s := got.Tests[0].Status; s != StatusPass {
		t.Errorf("control: test_1 logged PASS, status = %v", s)
	}
	if s := got.Tests[1].Status; s == StatusPass {
		t.Errorf("test_2 logged RUN and no verdict, and reads as PASS")
	} else if !strings.Contains(got.Tests[1].Message, "no verdict") {
		t.Errorf("test_2: message %q does not say it reported no verdict", got.Tests[1].Message)
	}
	if s := got.Tests[2].Status; s != StatusError {
		t.Errorf("control: test_3 never ran, status = %v", s)
	}
	if got.AllPassed() {
		t.Error("a suite whose runner aborted mid-test passed")
	}
}
