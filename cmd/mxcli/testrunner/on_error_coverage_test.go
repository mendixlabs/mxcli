// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// errorRaisingCalls is one statement per call-shaped activity that takes an
// ON ERROR clause in a microflow — the grammar's onErrorClause on each rule in
// MDLMicroflow.g4. The test runner has to catch an error from any of them, or
// one raised by the body escapes the generated test microflow.
var errorRaisingCalls = []struct{ kind, stmt string }{
	{"call microflow", "$r = call microflow M.MF_Import(Json = '{}');"},
	{"call nanoflow", "$r = call nanoflow M.NF_Import(Json = '{}');"},
	{"call java action", "$created = call java action CsvServices.ImportCsvUrlData(Entity = 'Tests.AllTypes', CsvUrl = 'file:///etc/passwd', MaxRecords = -1, HasHeader = true, AlternativeHeader = empty, Delimiter = empty);"},
	{"call external action", "$r = call external action M.Svc.Act(P = 1);"},
	{"call web service", "$r = call web service M.WS operation M.WS.Op();"},
	{"call workflow", "$wf = call workflow M.WF($Ctx);"},
	{"call rest service", "$r = call rest service get 'https://example.com' returns String;"},
	{"send rest request", "$r = send rest request M.Client.Op;"},
	{"execute database query", "$r = execute database query M.Conn.Q;"},
}

// TestThrowsCatchesEveryCallKind pins that @throws attaches its handler to
// every call that can raise, not only CALL MICROFLOW / CALL NANOFLOW.
//
// A Java action that raised the expected error was reported as ERROR, not
// PASS, because the runner left the CALL JAVA ACTION without a handler and
// the error escaped the test microflow (ako/mxcli#1164).
func TestThrowsCatchesEveryCallKind(t *testing.T) {
	for _, c := range errorRaisingCalls {
		t.Run(c.kind, func(t *testing.T) {
			tc := parseOne(t, "/**\n * @test it raises\n * @throws 'Only http and https csv urls are supported'\n */\n"+c.stmt+"\n/\n")
			assertLeftToTheEndpoint(t, GenerateTestFlows(&TestSuite{Name: "s", Tests: []TestCase{tc}}), c.stmt)
			assertHandled(t, "legacy runner", GenerateTestRunner(&TestSuite{Name: "s", Tests: []TestCase{tc}}), legacyStmt(c.stmt))
		})
	}
}

// TestVerdictCatchesEveryCallKind is the same for a test without @throws: an
// error the body did not expect must become the usual "exception during
// execution: …" FAIL, not escape to the outer handler.
func TestVerdictCatchesEveryCallKind(t *testing.T) {
	for _, c := range errorRaisingCalls {
		t.Run(c.kind, func(t *testing.T) {
			tc := parseOne(t, "/**\n * @test it runs\n */\n"+c.stmt+"\n/\n")
			assertLeftToTheEndpoint(t, GenerateTestFlows(&TestSuite{Name: "s", Tests: []TestCase{tc}}), c.stmt)
			assertHandled(t, "legacy runner", GenerateTestRunner(&TestSuite{Name: "s", Tests: []TestCase{tc}}), legacyStmt(c.stmt))
		})
	}
}

// TestNonCallStatementsGetNoHandler is the other side: a statement that is not
// a call is left alone, including one that merely mentions a call in a string.
func TestNonCallStatementsGetNoHandler(t *testing.T) {
	tc := parseOne(t, "/**\n * @test it logs\n */\nlog info node 'T' 'about to call java action X';\n/\n")
	mdl := GenerateTestFlows(&TestSuite{Name: "s", Tests: []TestCase{tc}})
	if strings.Contains(mdl, "ON ERROR BEGIN") {
		t.Errorf("a LOG mentioning a call was given a handler:\n%s", mdl)
	}
}

// assertLeftToTheEndpoint checks the endpoint runner's side: the call is kept as
// written, with no handler, so an error of any kind leaves the test microflow and
// the endpoint reports it (toResult makes it the verdict). A handler reading the
// message is what mxbuild --serve rejects incrementally (CE0108,
// mendixlabs/mxcli#1874).
func assertLeftToTheEndpoint(t *testing.T, mdl, stmt string) {
	t.Helper()
	call := strings.TrimSuffix(stmt, ";")
	if !strings.Contains(mdl, stmt) || strings.Contains(mdl, call+" ON ERROR BEGIN") {
		t.Errorf("endpoint runner: %q is not left as written:\n%s", call, mdl)
	}
	if _, errs := visitor.Build(mdl); len(errs) > 0 {
		t.Errorf("endpoint runner: generated MDL does not parse: %v\n%s", errs[0], mdl)
	}
}

func assertHandled(t *testing.T, runner, mdl, stmt string) {
	t.Helper()
	call := strings.TrimSuffix(stmt, ";")
	if !strings.Contains(mdl, call+" ON ERROR BEGIN") {
		t.Errorf("%s: no ON ERROR handler on %q:\n%s", runner, call, mdl)
	}
	if _, errs := visitor.Build(mdl); len(errs) > 0 {
		t.Errorf("%s: generated MDL does not parse: %v\n%s", runner, errs[0], mdl)
	}
}

// legacyStmt is stmt as the legacy runner writes it for the suite's first test:
// that runner compiles every test into one flow, so it renames the variable a
// call defines with the test's suffix. These statements used to be matched
// unrenamed, which held only because a lower-case `call` was not renamed
// (TestLegacyRunnerRenamesLowerCaseAssignments) — nor, in any case, `send rest
// request` or `execute database query` (ako/mxcli#1775).
func legacyStmt(stmt string) string {
	return renameVariables(stmt, extractVariableNames(stmt, langver.V0), "_1")
}
