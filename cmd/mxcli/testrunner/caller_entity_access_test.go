// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The generated test callers apply entity access, and read a raised error's
// message through a helper that does not (ako/mxcli#2100, mendixlabs/mxcli#1805).
//
// Measured with mx check on 11.12.1, at security Off and Production alike:
//
//   - a microflow that does not apply entity access calling one that does is
//     CE0114, so a test could not call a production microflow that applies it;
//   - a microflow that applies entity access reading $latestError/Message is
//     CE0150 ("attributes of System.Error … cannot be accessed in microflows
//     that apply entity access. Pass the object to a microflow that does not
//     apply entity access if you want to inspect it.");
//   - an applying flow passing $latestError to a non-applying helper that
//     returns $Error/Message is 0 errors — Mendix's own prescription.
//
// The tests run under a system context, which entity access does not restrict,
// so the setting changes what compiles and nothing about what a test sees.

const callerSuiteSrc = `mdl 1;
/**
 * @test calls an applying microflow
 * @expect $ok
 */
$ok = call microflow MyModule.ACT_Applying();
$ok2 = call microflow MyModule.ACT_Applying();
/

/**
 * @test throws with a message
 * @throws 'must be positive'
 */
$r = call microflow MyModule.Apply(Amount = -5.0);
/
`

func callerSuite(t *testing.T, src string) *TestSuite {
	t.Helper()
	tests, err := parseMDLTests(src, "t.test.mdl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return &TestSuite{Name: "t", Tests: tests}
}

// microflowsIn parses a generated script and returns its microflows by name.
func microflowsIn(t *testing.T, mdl string) map[string]*ast.CreateMicroflowStmt {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("generated MDL does not parse: %v\n%s", errs[0], mdl)
	}
	flows := map[string]*ast.CreateMicroflowStmt{}
	for _, s := range prog.Statements {
		if mf, ok := s.(*ast.CreateMicroflowStmt); ok {
			flows[mf.Name.String()] = mf
		}
	}
	return flows
}

func applies(mf *ast.CreateMicroflowStmt) bool {
	return mf.ApplyEntityAccess != nil && *mf.ApplyEntityAccess
}

// outsideHelper is the script with the helper's own definition removed: the
// helper is the one place that may read an error's attributes.
func outsideHelper(mdl string) string {
	start := strings.Index(mdl, createFlow+" "+errorMessageFlow)
	if start < 0 {
		return mdl
	}
	end := strings.Index(mdl[start:], "END;")
	return mdl[:start] + mdl[start+end:]
}

var errorAttributeRead = regexp.MustCompile(`\$latest(Error|SoapFault)/`)

func TestEndpointTestFlowsApplyEntityAccess(t *testing.T) {
	suite := callerSuite(t, callerSuiteSrc)
	mdl := GenerateTestFlows(suite)
	flows := microflowsIn(t, mdl)
	for _, tc := range suite.Tests {
		mf := flows[testFlowName(tc)]
		if mf == nil {
			t.Fatalf("no %s in:\n%s", testFlowName(tc), mdl)
		}
		if !applies(mf) {
			t.Errorf("%s does not apply entity access, so it cannot call a microflow that does (CE0114)", testFlowName(tc))
		}
	}
	helper := flows[errorMessageFlow]
	if helper == nil {
		t.Fatalf("no %s in:\n%s", errorMessageFlow, mdl)
	}
	if applies(helper) {
		t.Errorf("%s applies entity access, so it cannot read the error's message either (CE0150)", errorMessageFlow)
	}
	if n := strings.Count(mdl, createFlow+" "+errorMessageFlow); n != 1 {
		t.Errorf("%s is defined %d times, want once", errorMessageFlow, n)
	}
	if m := errorAttributeRead.FindString(outsideHelper(mdl)); m != "" {
		t.Errorf("an applying test flow reads %q directly (CE0150):\n%s", m, mdl)
	}
	// The verdict and the @throws comparison still carry the message.
	for _, want := range []string{
		"CALL MICROFLOW " + errorMessageFlow + "(RaisedError = $latestError)",
		"'" + verdictFailPrefix + "exception during execution: ' + $MxTestErrorMessage_1",
		"contains($MxTestErrorMessage_1, 'must be positive')",
	} {
		if !strings.Contains(mdl, want) {
			t.Errorf("generated flows do not contain %q:\n%s", want, mdl)
		}
	}
}

// Every handler reads the message into its own variable: a body with two
// calls gets two handlers in one microflow.
func TestErrorMessageVariablePerHandler(t *testing.T) {
	mdl := GenerateTestFlows(callerSuite(t, callerSuiteSrc))
	for _, v := range []string{"$MxTestErrorMessage_1 = CALL", "$MxTestErrorMessage_2 = CALL"} {
		if !strings.Contains(mdl, v) {
			t.Errorf("want %q in:\n%s", v, mdl)
		}
	}
}

func TestLegacyTestRunnerAppliesEntityAccess(t *testing.T) {
	mdl := GenerateTestRunner(callerSuite(t, callerSuiteSrc))
	flows := microflowsIn(t, mdl)
	runner := flows[mxTestRunner]
	if runner == nil {
		t.Fatalf("no %s in:\n%s", mxTestRunner, mdl)
	}
	if !applies(runner) {
		t.Errorf("%s does not apply entity access (CE0114)", mxTestRunner)
	}
	if helper := flows[errorMessageFlow]; helper == nil || applies(helper) {
		t.Errorf("want a non-applying %s, got %+v", errorMessageFlow, helper)
	}
	if m := errorAttributeRead.FindString(outsideHelper(mdl)); m != "" {
		t.Errorf("the applying runner reads %q directly (CE0150):\n%s", m, mdl)
	}
	// Two tests share the runner, so their message variables must differ.
	if !strings.Contains(mdl, "contains($MxTestErrorMessage_2_1, 'must be positive')") {
		t.Errorf("the @throws comparison does not read the helper's result:\n%s", mdl)
	}
}

func TestCheckWrapperAppliesEntityAccess(t *testing.T) {
	cs, err := CheckSource(callerSuiteSrc, "t.test.mdl")
	if err != nil {
		t.Fatal(err)
	}
	flows := microflowsIn(t, cs.MDL)
	n := 0
	for name, mf := range flows {
		if !strings.HasPrefix(name, mxTestModule+".Check_") {
			continue
		}
		n++
		if !applies(mf) {
			t.Errorf("%s does not apply entity access, so check reports MDL-SEC22 for a call the run accepts", name)
		}
	}
	if n != 2 {
		t.Errorf("found %d check wrappers, want 2:\n%s", n, cs.MDL)
	}
}

// A test whose own handler reads the error's attributes keeps a caller that
// does not apply entity access: applying it would fail that read (CE0150) in a
// test that builds today. The control is the same text inside a string, which
// reads nothing.
func TestBodyReadingErrorAttributesKeepsCallerNonApplying(t *testing.T) {
	const reads = `mdl 1;
/**
 * @test inspects the error itself
 * @expect $msg != empty
 */
declare $msg String = empty;
$r = call microflow MyModule.Fail() on error without rollback begin
  set $msg = $latestError/Message;
end error;
/
`
	control := strings.Replace(reads, "set $msg = $latestError/Message;", "set $msg = '$latestError/Message';", 1)

	for _, c := range []struct {
		name, src string
		want      bool
	}{{"reads the error", reads, false}, {"control: only a string", control, true}} {
		t.Run(c.name, func(t *testing.T) {
			suite := callerSuite(t, c.src)
			if got := applies(microflowsIn(t, GenerateTestFlows(suite))[testFlowName(suite.Tests[0])]); got != c.want {
				t.Errorf("endpoint test flow applies entity access = %v, want %v", got, c.want)
			}
			if got := applies(microflowsIn(t, GenerateTestRunner(suite))[mxTestRunner]); got != c.want {
				t.Errorf("legacy runner applies entity access = %v, want %v", got, c.want)
			}
			cs, err := CheckSource(c.src, "t.test.mdl")
			if err != nil {
				t.Fatal(err)
			}
			for name, mf := range microflowsIn(t, cs.MDL) {
				if got := applies(mf); got != c.want {
					t.Errorf("check wrapper %s applies entity access = %v, want %v", name, got, c.want)
				}
			}
		})
	}
}

// The helper is generated for the run, so cleanup removes it with the test
// flows, and a build error in it is mxcli's.
func TestErrorMessageFlowIsCleanedUpAndAttributed(t *testing.T) {
	got := generatedTestFlowNames([]string{"Test_test_1", strings.TrimPrefix(errorMessageFlow, mxTestModule+"."), "MyOwnFlow"})
	if strings.Join(got, ",") != testFlowPrefix+"test_1,"+errorMessageFlow {
		t.Errorf("discovered for cleanup: %v", got)
	}
	if names := suiteTestFlowNames(suiteOf("test_1")); !containsString(names, errorMessageFlow) {
		t.Errorf("the fallback drop list misses %s: %v", errorMessageFlow, names)
	}
	if cmds := cleanupCommands(projectState{}, true); !containsString(cmds, "DROP MICROFLOW "+errorMessageFlow) {
		t.Errorf("legacy cleanup in a pre-existing MxTest module does not drop %s: %v", errorMessageFlow, cmds)
	}

	ce := problem("CE0109", "Undefined variable 'x'.", "MxTest", "Microflow '"+strings.TrimPrefix(errorMessageFlow, mxTestModule+".")+"'", "End event")
	_, err := resultsForBuildFailure(buildFailure(failedBuild(ce)), suiteOf("test_1"))
	if err == nil || !strings.Contains(err.Error(), "defect in mxcli") || strings.Contains(err.Error(), "left behind") {
		t.Errorf("an error in %s is not reported as mxcli's: %v", errorMessageFlow, err)
	}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
