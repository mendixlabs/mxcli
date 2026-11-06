// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"
)

// expectOf compiles an @expect body for use in a table literal. It panics on a
// bad expression, which in a test is the same as failing it.
func expectOf(raw string) Expect {
	exp, err := ParseExpect(raw)
	if err != nil {
		panic(err)
	}
	return exp
}

// TestExpectCanariesAreEvaluated is the regression test for the silent-pass
// defect: mxcli test evaluated only `$var = <literal>` and let every other
// assertion shape through unconditionally, with no warning and nothing in the
// output distinguishing it from a real assertion. Each row is an assertion from
// the field report that must fail; each must now compile into a condition that
// can produce a failure, and none may be dropped.
func TestExpectCanariesAreEvaluated(t *testing.T) {
	canaries := []struct {
		raw  string
		cond string
	}{
		{"1 = 2", "1 = 2"},
		{"length($result) = 999", "length($result) = 999"},
		{"find($result, 'Z') >= 0", "find($result, 'Z') >= 0"},
		{"find($result, '5') < 0", "find($result, '5') < 0"},
		{"substring($result, 0, 1) = 'Z'", "substring($result, 0, 1) = 'Z'"},
		{"substring($result, 0, 1) = substring($result, 1, 1)",
			"substring($result, 0, 1) = substring($result, 1, 1)"},
		{"$result != 'WRONG'", "$result != 'WRONG'"},
		{"find($result, '0') >= 0 and find($result, '0') < 0",
			"find($result, '0') >= 0 and find($result, '0') < 0"},
	}
	for _, c := range canaries {
		exp, err := ParseExpect(c.raw)
		if err != nil {
			t.Errorf("ParseExpect(%q): %v", c.raw, err)
			continue
		}
		if exp.Condition != c.cond {
			t.Errorf("ParseExpect(%q).Condition = %q, want %q", c.raw, exp.Condition, c.cond)
		}
	}
}

// TestExpectCanariesReachTheGeneratedMicroflow closes the loop on the canaries:
// compiling them is not enough, the condition has to end up in the microflow the
// runner actually invokes.
func TestExpectCanariesReachTheGeneratedMicroflow(t *testing.T) {
	suite := &TestSuite{Name: "canary", Tests: []TestCase{{
		ID:      "test_1",
		Name:    "a self-evident falsehood",
		MDL:     "$result = CALL MICROFLOW MyModule.Anything();",
		Expects: []Expect{expectOf("1 = 2")},
	}}}
	mdl := GenerateTestFlows(suite)
	if !strings.Contains(mdl, "IF 1 = 2 THEN") {
		t.Errorf("the assertion never reached the microflow:\n%s", mdl)
	}
}

// TestParseExpectRejectsWhatItCannotEvaluate is the fail-closed half. Each of
// these must be an error — never an assertion, and never silence.
func TestParseExpectRejectsWhatItCannotEvaluate(t *testing.T) {
	bad := []struct {
		raw  string
		want string
	}{
		{"", "needs an expression"},
		{"'abc'", "not a condition"},                         // a value, not an assertion
		{"$a + $b", "not a condition"},                       // arithmetic, not an assertion
		{"length($result)", "not a condition"},               // Integer-valued, not an assertion
		{"$result =", "expected a value"},                    // truncated
		{"$result = 'x' extra", "unexpected trailing input"}, // garbage after
		{"randomInt($result) = 1", "not a Mendix expression function"},
		{"length($result, 2) = 1", "takes 1 argument(s), got 2"},
		{"substring($result) = 'x'", "takes 2 to 3 arguments, got 1"},
		{"$result = #", "unexpected character"},
		{"length($result = 1", "expected a closing parenthesis"},
		{"$result/ = 'x'", "expected a member name"},
	}
	for _, c := range bad {
		exp, err := ParseExpect(c.raw)
		if err == nil {
			t.Errorf("ParseExpect(%q) accepted it as %q; want an error", c.raw, exp.Condition)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("ParseExpect(%q) error = %q, want it to mention %q", c.raw, err, c.want)
		}
	}
}

// TestParseExpectAcceptsRealAssertions guards against over-tightening: the
// shapes below are ordinary Mendix expressions and must keep working.
func TestParseExpectAcceptsRealAssertions(t *testing.T) {
	good := []string{
		"$result = 'John Doe'",
		"$product/Name = 'Widget'",
		"$order/Customer/Name != empty",
		"$count = 3",
		"$done = true",
		"not($done)",
		"contains($result, 'abc')",
		"trim($result) = 'x'",
		"length($result) = 81",
		"toUpperCase($name) = 'ABC'",
		"$total = $price * 2 + 1",
		"$status = MyModule.Status.Open",
		"($a = 1 or $b = 2) and $c = 3",
		"$user = [%CurrentUser%]",
		"$done", // a Boolean variable is a condition on its own
		"if $flag then $a else $b = 1",
	}
	for _, raw := range good {
		if _, err := ParseExpect(raw); err != nil {
			t.Errorf("ParseExpect(%q): %v", raw, err)
		}
	}
}

// TestParseExpectRewritesNotEquals pins the one rewrite the parser performs.
// Mendix's expression engine accepts `!=` and rejects `<>`; the annotation
// accepts both because MDL's own lexer does.
func TestParseExpectRewritesNotEquals(t *testing.T) {
	for _, raw := range []string{"$r <> 'John'", "$r != 'John'"} {
		exp, err := ParseExpect(raw)
		if err != nil {
			t.Fatalf("ParseExpect(%q): %v", raw, err)
		}
		if exp.Condition != "$r != 'John'" {
			t.Errorf("ParseExpect(%q).Condition = %q, want %q", raw, exp.Condition, "$r != 'John'")
		}
	}
}

// TestExpectActualValueIsTypeSafe pins when the observed value is reported.
//
// Mendix's expression engine is typed: `+` only concatenates Strings and
// toString() rejects a String. The rule is that the operand is used directly only
// when it is known to be a String, wrapped in toString() only when it is known
// not to be, and omitted when neither is established — reporting nothing beats
// emitting an expression the build rejects.
func TestExpectActualValueIsTypeSafe(t *testing.T) {
	cases := []struct {
		raw    string
		actual string
	}{
		// Comparing against a string literal proves the other side is a String.
		{"$result = 'John'", "$result"},
		{"'John' = $result", "$result"},
		{"$product/Name != 'Widget'", "$product/Name"},
		// A String-returning built-in needs no wrapping.
		{"substring($result, 0, 1) = 'Z'", "substring($result, 0, 1)"},
		// An Integer-returning built-in does.
		{"length($result) = 999", "toString(length($result))"},
		{"find($result, 'Z') >= 0", "toString(find($result, 'Z'))"},
		// The literal pins the unknown side's type, so toString() is safe.
		{"$count = 3", "toString($count)"},
		{"$done = true", "toString($done)"},
		// An enumeration value pins the other side, in either order.
		{"$Paid/Status = Procurement.InvoiceStatus.PAID", "toString($Paid/Status)"},
		{"Procurement.InvoiceStatus.PAID = $Paid/Status", "toString($Paid/Status)"},
		{"$Paid/Status != Procurement.InvoiceStatus.PAID", "toString($Paid/Status)"},
		// Neither side pins anything, or the assertion is not a comparison.
		{"Mod.Enum.A = Mod.Enum.B", ""}, // two literals, nothing was observed
		{"$a/X = $b/Y", ""},
		{"1 = 2", ""}, // nothing was observed
		{"$a = $b", ""},
		{"$result != empty", ""},
		{"find($result, '0') >= 0 and find($result, '1') >= 0", ""},
		{"not($done)", ""},
	}
	for _, c := range cases {
		exp, err := ParseExpect(c.raw)
		if err != nil {
			t.Fatalf("ParseExpect(%q): %v", c.raw, err)
		}
		if exp.Actual != c.actual {
			t.Errorf("ParseExpect(%q).Actual = %q, want %q", c.raw, exp.Actual, c.actual)
		}
	}
}

// TestParseAnnotationsRecordsAssertionErrors pins that a bad @expect survives as an
// error on the test rather than vanishing.
func TestParseAnnotationsRecordsAssertionErrors(t *testing.T) {
	doc := `/**
 * @test broken
 * @expect randomInt($result) = 1
 * @expect $result = 'ok'
 */`
	a := parseAnnotations(doc)
	if len(a.Expects) != 1 {
		t.Errorf("Expects: got %d, want 1", len(a.Expects))
	}
	if len(a.AssertionErrors) != 1 {
		t.Fatalf("AssertionErrors: got %d, want 1", len(a.AssertionErrors))
	}
	if !strings.Contains(a.AssertionErrors[0], "randomInt") {
		t.Errorf("AssertionErrors[0] = %q, want it to name the function", a.AssertionErrors[0])
	}
}

// TestUncompilableExpectIsAnErrorNotAPass is the end-to-end statement of the
// rule: such a test is never generated, and the suite reports ERROR — which
// FailCount counts, so the run's exit code is non-zero.
func TestUncompilableExpectIsAnErrorNotAPass(t *testing.T) {
	tc := TestCase{
		ID:              "test_1",
		Name:            "broken",
		MDL:             "$result = CALL MICROFLOW M.Anything();",
		AssertionErrors: []string{"@expect randomInt($result) = 1: randomInt() is not a Mendix expression function"},
	}
	suite := &TestSuite{Name: "s", Tests: []TestCase{tc}}

	if mdl := GenerateTestFlows(suite); strings.Contains(mdl, testFlowName(tc)) {
		t.Errorf("a test with an uncompilable @expect was generated:\n%s", mdl)
	}
	if mdl := GenerateTestRunner(suite); strings.Contains(mdl, "MXTEST:RUN:test_1") {
		t.Errorf("a test with an uncompilable @expect was generated into the runner:\n%s", mdl)
	}

	res, bad := assertionErrorResult(tc)
	if !bad {
		t.Fatal("assertionErrorResult did not flag the test")
	}
	if res.Status != StatusError {
		t.Errorf("status = %v, want ERROR", res.Status)
	}
	sr := &SuiteResult{Tests: []TestResult{res}}
	if sr.PassCount() != 0 || sr.FailCount() != 1 || sr.AllPassed() {
		t.Errorf("an uncompilable @expect reported as passing: pass=%d fail=%d allPassed=%v",
			sr.PassCount(), sr.FailCount(), sr.AllPassed())
	}
}

// TestFailureMessageReportsTheActualValue pins the second half of the fix: a
// failing test that only echoes the expectation tells you nothing about what
// came back.
func TestFailureMessageReportsTheActualValue(t *testing.T) {
	var b strings.Builder
	writeExpectCheck(&b, expectOf("$result = 'John'"))
	got := b.String()
	if !strings.Contains(got, "', actual: ' + $result") {
		t.Errorf("the failure message does not carry the actual value:\n%s", got)
	}
	if !strings.Contains(got, "expected $result = ''John''") {
		t.Errorf("the failure message does not echo the assertion:\n%s", got)
	}
}

// TestFailureMessageReportsTheActualEnumValue pins that an enumeration compare
// reports what the attribute held, via toString() (the enumeration key).
func TestFailureMessageReportsTheActualEnumValue(t *testing.T) {
	var b strings.Builder
	writeExpectCheck(&b, expectOf("$Paid/Status = Procurement.InvoiceStatus.APPROVED"))
	got := b.String()
	if !strings.Contains(got, ", actual: ' + toString($Paid/Status)") {
		t.Errorf("the failure message does not carry the actual enum value:\n%s", got)
	}
}

// TestSummaryDistinguishesErrorsFromFailures pins that the summary line does not
// fold a never-evaluated assertion into the failure count. The output not
// distinguishing the two is what let 16 vacuous tests sit inside a green 22/22.
func TestSummaryDistinguishesErrorsFromFailures(t *testing.T) {
	sr := &SuiteResult{Name: "s", Tests: []TestResult{
		{ID: "1", Name: "ok", Status: StatusPass},
		{ID: "2", Name: "wrong", Status: StatusFail},
		{ID: "3", Name: "unevaluated", Status: StatusError},
	}}
	if sr.ErrorCount() != 1 {
		t.Errorf("ErrorCount = %d, want 1", sr.ErrorCount())
	}
	var b strings.Builder
	PrintResults(&b, sr, false)
	if !strings.Contains(b.String(), "Total: 3  Passed: 1  Failed: 1  Errors: 1  Skipped: 0") {
		t.Errorf("summary does not separate errors from failures:\n%s", b.String())
	}
}
