// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"strings"
	"testing"
)

func TestGenerateTestFlowsOneMicroflowPerTest(t *testing.T) {
	suite := &TestSuite{
		Name: "suite",
		Tests: []TestCase{
			{ID: "test_1", Name: "first", MDL: "$r = CALL MICROFLOW Mod.A();"},
			{ID: "test_2", Name: "second", MDL: "$r = CALL MICROFLOW Mod.B();"},
		},
	}
	mdl := GenerateTestFlows(suite)

	for _, want := range []string{
		"CREATE OR MODIFY MICROFLOW MxTest.Test_test_1 ()",
		"CREATE OR MODIFY MICROFLOW MxTest.Test_test_2 ()",
	} {
		if !strings.Contains(mdl, want) {
			t.Errorf("generated MDL is missing %q", want)
		}
	}
	// Plus the one errorMessageFlow every handler shares (ako/mxcli#2100).
	if n := strings.Count(mdl, "CREATE OR MODIFY MICROFLOW "+testFlowPrefix+"test_"); n != 2 {
		t.Errorf("got %d test microflows, want one per test (2)", n)
	}
}

// TestGenerateTestFlowsNoVariableRenaming pins the simplification that per-test
// microflows buy. The monolithic runner has to suffix every variable to keep
// test 1's $result apart from test 2's; separate microflows have separate
// scopes, so the same name in two tests must survive unmangled.
func TestGenerateTestFlowsNoVariableRenaming(t *testing.T) {
	suite := &TestSuite{
		Tests: []TestCase{
			{ID: "test_1", Name: "a", MDL: "$result = CALL MICROFLOW Mod.A();",
				Expects: []Expect{expectOf("$result = 'x'")}},
			{ID: "test_2", Name: "b", MDL: "$result = CALL MICROFLOW Mod.B();",
				Expects: []Expect{expectOf("$result = 'y'")}},
		},
	}
	mdl := GenerateTestFlows(suite)

	if strings.Contains(mdl, "$result_1") || strings.Contains(mdl, "$result_2") {
		t.Error("variables were suffix-renamed; per-test microflows have their own scope")
	}
	if n := strings.Count(mdl, "$result"); n < 4 {
		t.Errorf("expected $result to survive in both tests, found %d references", n)
	}
}

func TestGenerateTestFlowsExpectAssertion(t *testing.T) {
	suite := &TestSuite{Tests: []TestCase{{
		ID: "test_1", Name: "equality", MDL: "$r = CALL MICROFLOW Mod.A();",
		Expects: []Expect{expectOf("$r = 'John'")},
	}}}
	mdl := GenerateTestFlows(suite)

	if !strings.Contains(mdl, "IF $r = 'John' THEN") {
		t.Errorf("missing the equality check:\n%s", mdl)
	}
	if !strings.Contains(mdl, verdictFailPrefix+"expected $r = ''John''") {
		t.Errorf("missing the failure verdict with the expected value:\n%s", mdl)
	}
}

// TestGenerateTestFlowsNotEqualIsRewritten pins the inherited constraint and the
// way it is now met. `<>` still must never reach the model — Mendix's expression
// engine rejects that spelling — but the branch-swapping workaround is gone:
// ParseExpect rewrites the operator to `!=`, so the condition is emitted as
// written and every other operator can be too.
func TestGenerateTestFlowsNotEqualIsRewritten(t *testing.T) {
	suite := &TestSuite{Tests: []TestCase{{
		ID: "test_1", Name: "inequality", MDL: "$r = CALL MICROFLOW Mod.A();",
		Expects: []Expect{expectOf("$r <> 'John'")},
	}}}
	mdl := GenerateTestFlows(suite)

	if strings.Contains(mdl, "$r <> 'John'") {
		t.Error("the <> operator reached the generated Mendix expression")
	}
	if !strings.Contains(mdl, "IF $r != 'John' THEN") {
		t.Errorf("<> was not rewritten to !=:\n%s", mdl)
	}
}

// A call that throws is left to the endpoint, which catches the error and
// reports its message; toResult makes it the FAIL verdict
// (TestToResult_ThrownTestMicroflow). The flow used to give each call a handler
// that read the message into a variable, and mxbuild --serve's incremental build
// rejects any use of such a variable (CE0108, measured on 11.16.1), so every
// `test --attach` of a test calling a microflow fell back to a cold build it
// could not apply (mendixlabs/mxcli#1874).
func TestGenerateTestFlowsLeavesCallsToTheEndpoint(t *testing.T) {
	suite := &TestSuite{Tests: []TestCase{{
		ID: "test_1", Name: "throwing", MDL: "$r = CALL MICROFLOW Mod.A();",
	}}}
	mdl := GenerateTestFlows(suite)
	for _, unwanted := range []string{"ON ERROR BEGIN", "MxTestErrorMessage", errorMessageFlow} {
		if strings.Contains(mdl, unwanted) {
			t.Errorf("generated flows contain %q:\n%s", unwanted, mdl)
		}
	}
	if !strings.Contains(mdl, "$r = CALL MICROFLOW Mod.A();") {
		t.Errorf("the call is not kept as written:\n%s", mdl)
	}
	if _, errs := visitor.Build(mdl); len(errs) > 0 {
		t.Errorf("generated MDL does not parse: %v\n%s", errs[0], mdl)
	}
}

// TestGenerateTestFlowsThrowsTestStartsFailed pins that an @throws test whose
// body completes normally fails: the verdict is pre-set to a failure, and only
// an error (judged by toResult) can turn the test into a pass.
func TestGenerateTestFlowsThrowsTestStartsFailed(t *testing.T) {
	suite := &TestSuite{Tests: []TestCase{{
		ID: "test_1", Name: "expects a throw", MDL: "$r = CALL MICROFLOW Mod.A();",
		Throws: "boom",
	}}}
	mdl := GenerateTestFlows(suite)

	failIdx := strings.Index(mdl, verdictFailPrefix+"expected an exception")
	callIdx := strings.Index(mdl, "$r = CALL MICROFLOW Mod.A();")
	if failIdx < 0 || callIdx < 0 {
		t.Fatalf("want the pre-set failure verdict and the call as written:\n%s", mdl)
	}
	if failIdx > callIdx {
		t.Error("the failure verdict is set after the body; a non-throwing body would pass")
	}
	if strings.Contains(mdl, "ON ERROR BEGIN") {
		t.Errorf("a handler was generated:\n%s", mdl)
	}
}

func TestGenerateTestFlowsMultiLineCall(t *testing.T) {
	suite := &TestSuite{Tests: []TestCase{{
		ID: "test_1", Name: "multiline",
		MDL: "$r = CALL MICROFLOW Mod.A(\n  FirstName = 'John',\n  LastName = 'Doe'\n);",
	}}}
	mdl := GenerateTestFlows(suite)
	if strings.Contains(mdl, "ON ERROR BEGIN") {
		t.Errorf("a handler was generated:\n%s", mdl)
	}
	if _, errs := visitor.Build(mdl); len(errs) > 0 {
		t.Errorf("a statement spanning lines does not parse: %v\n%s", errs[0], mdl)
	}
}

// TestGenerateTestFlowsEscapesNameInComment pins that a test name cannot close
// the javadoc block it is written into and break the generated MDL.
func TestGenerateTestFlowsEscapesNameInComment(t *testing.T) {
	suite := &TestSuite{Tests: []TestCase{{
		ID: "test_1", Name: "ends the comment */ CREATE MODULE Evil;", MDL: "",
	}}}
	mdl := GenerateTestFlows(suite)

	head := mdl[:strings.Index(mdl, "CREATE OR MODIFY MICROFLOW")]
	if strings.Count(head, "*/") != 1 {
		t.Errorf("the test name closed the javadoc block early:\n%s", head)
	}
}

func TestEndpointCleanupCommands(t *testing.T) {
	suite := &TestSuite{Tests: []TestCase{{ID: "test_1"}, {ID: "test_2"}}}

	tests := []struct {
		name    string
		state   projectState
		present bool
		want    []string
	}{
		{
			name:    "drops the whole module when the runner created it",
			state:   projectState{afterStartup: "Mod.ASU", createdMxTest: true},
			present: true,
			want: []string{
				"ALTER SETTINGS RUNTIME (AfterStartupMicroflow: 'Mod.ASU')",
				"DROP MODULE MxTest",
			},
		},
		{
			name:    "drops only the generated documents from a user's module",
			state:   projectState{createdMxTest: false},
			present: true,
			want: []string{
				"ALTER SETTINGS RUNTIME (AfterStartupMicroflow: '')",
				"DROP MICROFLOW MxTest.Test_test_1",
				"DROP MICROFLOW MxTest.Test_test_2",
				"DROP MICROFLOW " + errorMessageFlow,
				"DROP MICROFLOW " + endpointStartupFlow,
				"DROP JAVA ACTION " + endpointRegisterAction,
			},
		},
		{
			name:    "drops nothing when the module never landed",
			state:   projectState{afterStartup: "Mod.ASU", createdMxTest: true},
			present: false,
			want:    []string{"ALTER SETTINGS RUNTIME (AfterStartupMicroflow: 'Mod.ASU')"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := endpointCleanupCommands(tc.state, suiteTestFlowNames(suite), tc.present)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d commands %q, want %d %q", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("command %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestEndpointCleanupRestoreIsAlwaysFirst pins the ordering: after-startup must
// stop pointing at the startup microflow before that microflow is dropped.
func TestEndpointCleanupRestoreIsAlwaysFirst(t *testing.T) {
	suite := &TestSuite{Tests: []TestCase{{ID: "test_1"}}}
	for _, st := range []projectState{
		{createdMxTest: true},
		{createdMxTest: false},
		{afterStartup: "Mod.ASU", createdMxTest: true},
	} {
		cmds := endpointCleanupCommands(st, suiteTestFlowNames(suite), true)
		if !strings.HasPrefix(cmds[0], "ALTER SETTINGS RUNTIME (AfterStartupMicroflow") {
			t.Errorf("state %+v: first command is %q, want the after-startup restore", st, cmds[0])
		}
	}
}

// The test-flow script runs after the endpoint script, which creates the MxTest
// module — and under --watch and --attach into a project that already holds it.
// A `CREATE MODULE` of its own is never needed, and exec answers it with
// "Module 'MxTest' already exists", which every `mxcli test --local` printed on
// a project that had no such module before the run (ako/mxcli#1856).
func TestGenerateTestFlowsDoesNotCreateTheModule(t *testing.T) {
	suite := &TestSuite{Tests: []TestCase{{ID: "test_1", Name: "a", MDL: "$r = CALL MICROFLOW Mod.A();"}}}
	if mdl := GenerateTestFlows(suite); strings.Contains(strings.ToUpper(mdl), "CREATE MODULE") {
		t.Errorf("test-flow script creates the module the endpoint script already created:\n%s", mdl)
	}
	if !strings.Contains(GenerateEndpointMDL(""), "CREATE MODULE "+mxTestModule+";") {
		t.Error("the endpoint script no longer creates the module; the test-flow script depends on it")
	}
}
