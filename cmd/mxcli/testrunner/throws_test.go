// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"
)

// parseOne parses a single test from a .test.mdl body and returns it.
func parseOne(t *testing.T, content string) TestCase {
	t.Helper()
	tests, err := parseMDLTests(content, "throws.test.mdl")
	if err != nil {
		t.Fatalf("ParseTestFile: %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("parsed %d tests, want 1", len(tests))
	}
	return tests[0]
}

const bareThrows = `/**
 * @test the import fails when the key does not match
 * @throws
 */
$result = call microflow M.MF_Import(Json = '{}');
/
`

// TestBareThrowsIsRecognised pins that @throws written without a message makes
// the test a throws test.
//
// It used to make no difference at all: both generators keyed on Throws != "",
// and a bare annotation leaves Throws empty — the same value an absent
// annotation leaves. The test was then generated as an ordinary one and failed
// on the very exception it was written to expect (ako/mxcli#301).
func TestBareThrowsIsRecognised(t *testing.T) {
	tc := parseOne(t, bareThrows)
	if !tc.ExpectsThrow {
		t.Fatal("ExpectsThrow = false, want true — a bare @throws was dropped")
	}
	if tc.Throws != "" {
		t.Errorf("Throws = %q, want empty", tc.Throws)
	}
	if len(tc.AssertionErrors) != 0 {
		t.Errorf("AssertionErrors = %v, want none", tc.AssertionErrors)
	}
	if got := tc.AssertionCount(); got != 1 {
		t.Errorf("AssertionCount = %d, want 1 — a bare @throws still asserts something", got)
	}
}

// TestBareThrowsGeneratesTheThrowsShape is the control's other half: the parse
// result must reach the generated microflow, whose verdict starts as a failure
// that only the error handler can clear.
func TestBareThrowsGeneratesTheThrowsShape(t *testing.T) {
	suite := &TestSuite{Name: "s", Tests: []TestCase{parseOne(t, bareThrows)}}
	mdl := GenerateTestFlows(suite)

	if !strings.Contains(mdl, "expected an exception but none was thrown") {
		t.Errorf("generated flow is not the throws shape:\n%s", mdl)
	}
	if strings.Contains(mdl, "FAIL:exception during execution") {
		t.Errorf("generated flow still treats the exception as a failure:\n%s", mdl)
	}
}

// TestThrowsMessageIsCompared pins that the annotation's message reaches the
// generated handler.
//
// It was decorative: the handler cleared the verdict for any exception at all,
// so @throws 'ZZZ-NOT-THE-REAL-MESSAGE' passed against a completely different
// error and a stale expectation never showed up (ako/mxcli#301).
func TestThrowsMessageIsCompared(t *testing.T) {
	tc := parseOne(t, `/**
 * @test the import fails when the key does not match
 * @throws 'the object was not found'
 */
$result = call microflow M.MF_Import(Json = '{}');
/
`)
	// The comparison is the runner's: the error leaves the test microflow and
	// the endpoint reports it (mendixlabs/mxcli#1874).
	if got := toResult(tc, &runResponse{OK: false, Error: "Import: the object was not found."}); got.Status != StatusPass {
		t.Errorf("a matching error gave %v %q, want PASS", got.Status, got.Message)
	}
	// The failing branch names what was actually raised — a verdict that says
	// only what was expected tells you nothing about what came back.
	got := toResult(tc, &runResponse{OK: false, Error: "a different error"})
	if got.Status != StatusFail || !strings.Contains(got.Message, "actual: a different error") {
		t.Errorf("a different error gave %v %q, want a FAIL naming it", got.Status, got.Message)
	}
}

// TestThrowsMessageIsComparedInLegacyRunner pins the same for the after-startup
// runner, which Docker still uses.
func TestThrowsMessageIsComparedInLegacyRunner(t *testing.T) {
	tc := parseOne(t, `/**
 * @test the import fails
 * @throws 'not found'
 */
$result = call microflow M.MF_Import(Json = '{}');
/
`)
	mdl := GenerateTestRunner(&TestSuite{Name: "s", Tests: []TestCase{tc}})

	if !strings.Contains(mdl, "contains($MxTestErrorMessage_1_1, 'not found')") {
		t.Errorf("legacy runner does not compare the message:\n%s", mdl)
	}
}

// TestThrowsMessageIsEscaped pins that an apostrophe in the expected message
// cannot break out of the MDL string literal it is written into.
func TestThrowsMessageIsEscaped(t *testing.T) {
	tc := parseOne(t, `/**
 * @test quoting
 * @throws 'it''s missing'
 */
$result = call microflow M.MF_Import(Json = '{}');
/
`)
	if got := toResult(tc, &runResponse{OK: false, Error: "it's missing here"}); got.Status != StatusPass {
		t.Errorf("an apostrophe in the expected message did not match: %v %q", got.Status, got.Message)
	}
	// The legacy runner still writes the message into MDL, doubled.
	if mdl := GenerateTestRunner(&TestSuite{Name: "s", Tests: []TestCase{tc}}); !strings.Contains(mdl, "'it''''s missing'") {
		t.Errorf("apostrophe not doubled for MDL in the legacy runner:\n%s", mdl)
	}
}

// TestThrowsMessageMustBeQuoted pins that an unquoted message is reported rather
// than dropped. The old pattern required the quotes to match and produced
// nothing when they did not, which removed the @throws entirely.
func TestThrowsMessageMustBeQuoted(t *testing.T) {
	for _, bad := range []string{"not found", "'not found", "not found'"} {
		tc := parseOne(t, `/**
 * @test unquoted
 * @throws `+bad+`
 */
$result = call microflow M.MF_Import(Json = '{}');
/
`)
		if len(tc.AssertionErrors) == 0 {
			t.Errorf("@throws %s: no AssertionErrors — the message was dropped silently", bad)
		}
	}
}
