// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"
)

// A suite whose one test did not build ended in "Passed: 0, Errors: 1,
// Skipped: 108", the CE0161 that caused it one row among 108 identical "not
// run" rows — so the run read as if every test had failed for no reason
// (ako/mxcli#1834). The build error is now repeated in a block of its own
// directly above the summary line, where the reader's eye lands.
func TestPrintResultsLeadsTheSummaryWithTheBuildError(t *testing.T) {
	ids := []string{"test_1", "test_2", "test_3", "test_4"}
	suite := suiteOf(ids...)
	suite.Tests[1].SourceFile = "tests/orders.test.mdl"
	err := buildFailure(failedBuild(problem("CE0161", "Error(s) in XPath constraint.",
		"MxTest", "Microflow 'Test_test_2'", "Retrieve object(s) activity 'Retrieve list of Order from database'")))
	result, err := resultsForBuildFailure(err, suite)
	if err != nil || result == nil {
		t.Fatalf("resultsForBuildFailure: %v", err)
	}

	var out strings.Builder
	PrintResults(&out, result, false)
	s := out.String()

	block := strings.Index(s, "Build failed")
	summary := strings.Index(s, "Total:")
	lastSkip := strings.LastIndex(s, "SKIP")
	if block < 0 {
		t.Fatalf("no build-failure block in the output:\n%s", s)
	}
	if !(lastSkip < block && block < summary) {
		t.Errorf("build-failure block at %d, want after the last row (%d) and before the summary (%d):\n%s",
			block, lastSkip, summary, s)
	}
	tail := s[block:summary]
	for _, want := range []string{"name of test_2", "tests/orders.test.mdl", "CE0161", "Error(s) in XPath constraint."} {
		if !strings.Contains(tail, want) {
			t.Errorf("build-failure block is missing %q:\n%s", want, tail)
		}
	}
}

// A run that built prints no such block — the control for the test above.
func TestPrintResultsNoBuildBlockWhenTheBuildSucceeded(t *testing.T) {
	result := &SuiteResult{Name: "s", Tests: []TestResult{
		{Name: "a", Status: StatusPass},
		{Name: "b", Status: StatusError, Message: "calling the test endpoint: boom"},
	}}
	var out strings.Builder
	PrintResults(&out, result, false)
	if strings.Contains(out.String(), "Build failed") {
		t.Errorf("a run that built printed a build-failure block:\n%s", out.String())
	}
}
