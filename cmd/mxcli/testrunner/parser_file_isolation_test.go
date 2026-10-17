// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"errors"
	"strings"
	"testing"
)

// Issue #903, second half: "one bad file poisons the whole tests directory".
//
// The parser used to abort the entire run on the first file it could not read,
// so an unrelated malformed file stopped every other test in the directory from
// being listed or executed. That is stricter than the fail-closed rule the rest
// of this package follows: an @expect that cannot be compiled does not abort the
// suite either — it becomes an ERROR result, which is counted with the failures
// and makes the exit code non-zero.
//
// A bad file gets the same treatment. The run stays red and says exactly which
// file and why; the tests that parse still run.

const (
	goodTestA = "/**\n * @test good one\n * @expect $x = 1\n */\n$x = 1;\n/\n"
	goodTestC = "/**\n * @test also good\n * @expect $z = 3\n */\n$z = 3;\n/\n"
	// Two @test comments with no '/' between them: a refusal parseMDLTests
	// already raises, used here as a stand-in for any unparseable file.
	badTestB = "/**\n * @test first\n */\n$x = 1;\n/**\n * @test second\n */\n$y = 2;\n/\n"
)

func TestParseTestDirKeepsGoodFilesWhenOneIsBad(t *testing.T) {
	dir := t.TempDir()
	writeBytes(t, dir, "a_good.test.mdl", goodTestA)
	writeBytes(t, dir, "b_bad.test.mdl", badTestB)
	writeBytes(t, dir, "c_good.test.mdl", goodTestC)

	suite, err := ParseTestDir(dir)
	if err != nil {
		t.Fatalf("ParseTestDir returned a hard error, want the bad file isolated: %v", err)
	}
	if len(suite.Tests) != 2 {
		t.Fatalf("got %d tests, want 2 (the two files that parse)", len(suite.Tests))
	}
	names := []string{suite.Tests[0].Name, suite.Tests[1].Name}
	if names[0] != "good one" || names[1] != "also good" {
		t.Errorf("tests = %v, want [good one, also good]", names)
	}

	if len(suite.FileErrors) != 1 {
		t.Fatalf("got %d file errors, want 1 — the bad file must still be reported", len(suite.FileErrors))
	}
	fe := suite.FileErrors[0]
	if !strings.Contains(fe.Path, "b_bad.test.mdl") {
		t.Errorf("FileErrors[0].Path = %q, want it to name b_bad.test.mdl", fe.Path)
	}
	if fe.Err == nil || !strings.Contains(fe.Err.Error(), "separator") {
		t.Errorf("FileErrors[0].Err = %v, want the parser's own explanation", fe.Err)
	}
}

// A directory in which every file is bad must not look like an empty directory:
// zero tests and zero errors is what "no tests here" means, and a run that
// silently found nothing is the failure mode this package exists to prevent.
func TestParseTestDirAllFilesBad(t *testing.T) {
	dir := t.TempDir()
	writeBytes(t, dir, "b_bad.test.mdl", badTestB)

	suite, err := ParseTestDir(dir)
	if err != nil {
		t.Fatalf("ParseTestDir: %v", err)
	}
	if len(suite.Tests) != 0 {
		t.Errorf("got %d tests, want 0", len(suite.Tests))
	}
	if len(suite.FileErrors) != 1 {
		t.Fatalf("got %d file errors, want 1", len(suite.FileErrors))
	}
}

// The control: with no bad file there are no file errors, so a green run cannot
// be confused with one that skipped something.
func TestParseTestDirCleanDirectoryReportsNoFileErrors(t *testing.T) {
	dir := t.TempDir()
	writeBytes(t, dir, "a_good.test.mdl", goodTestA)
	writeBytes(t, dir, "c_good.test.mdl", goodTestC)

	suite, err := ParseTestDir(dir)
	if err != nil {
		t.Fatalf("ParseTestDir: %v", err)
	}
	if len(suite.Tests) != 2 {
		t.Fatalf("got %d tests, want 2", len(suite.Tests))
	}
	if len(suite.FileErrors) != 0 {
		t.Errorf("FileErrors = %v, want none", suite.FileErrors)
	}
}

// Files named individually on the command line get the same isolation: naming
// three files and having one of them refuse to parse should not cost the other
// two, any more than pointing at their directory would.
func TestParseTestFilesIsolatesNamedBadFile(t *testing.T) {
	dir := t.TempDir()
	a := writeBytes(t, dir, "a_good.test.mdl", goodTestA)
	b := writeBytes(t, dir, "b_bad.test.mdl", badTestB)
	c := writeBytes(t, dir, "c_good.test.mdl", goodTestC)

	suite, err := parseTestFiles([]string{a, b, c})
	if err != nil {
		t.Fatalf("parseTestFiles: %v", err)
	}
	if len(suite.Tests) != 2 {
		t.Fatalf("got %d tests, want 2", len(suite.Tests))
	}
	if len(suite.FileErrors) != 1 {
		t.Fatalf("got %d file errors, want 1", len(suite.FileErrors))
	}
}

// A path that does not exist is still a hard error: it is a mistake in the
// invocation, not a malformed test, and continuing would run a different suite
// than the one that was asked for.
func TestParseTestFilesStillFailsOnMissingPath(t *testing.T) {
	if _, err := parseTestFiles([]string{"/nonexistent/does-not-exist.test.mdl"}); err == nil {
		t.Fatal("parseTestFiles succeeded on a missing path, want an error")
	}
}

// The results a bad file contributes must be counted as errors, so a run that
// skipped a file cannot report a clean pass. This is what makes the isolation
// safe: the tests that parse run, and the suite is still red.
func TestFileErrorResultsCountAsErrors(t *testing.T) {
	res := &SuiteResult{
		Name:  "mxtest",
		Tests: []TestResult{{Name: "a passing test", Status: StatusPass, Assertions: 1}},
	}
	res.Tests = append(res.Tests, suiteFileErrorResults([]FileError{
		{Path: "/tests/b_bad.test.mdl", Err: errors.New("two @test comments, no separator")},
	})...)

	if got := res.ErrorCount(); got != 1 {
		t.Errorf("ErrorCount = %d, want 1", got)
	}
	if got := res.FailCount(); got != 1 {
		t.Errorf("FailCount = %d, want 1 — an ERROR counts with the failures", got)
	}
	if res.AllPassed() {
		t.Error("AllPassed = true, want false — a skipped file must not read as green")
	}
	// The name has to say which file, because a bad file has no test name of
	// its own to identify it by.
	if name := res.Tests[1].Name; !strings.Contains(name, "b_bad.test.mdl") {
		t.Errorf("result name = %q, want it to name the file", name)
	}
	if msg := res.Tests[1].Message; !strings.Contains(msg, "no separator") {
		t.Errorf("result message = %q, want the parser's explanation", msg)
	}
}

// ListTests must print the tests it found AND exit non-zero, so `--list` in CI
// cannot report a clean listing over a directory it only partly read.
func TestListTestsReportsFileErrorsAndFails(t *testing.T) {
	dir := t.TempDir()
	writeBytes(t, dir, "a_good.test.mdl", goodTestA)
	writeBytes(t, dir, "b_bad.test.mdl", badTestB)

	var out strings.Builder
	err := ListTests([]string{dir}, &out, nil)
	if err == nil {
		t.Fatal("ListTests returned nil, want an error so the command exits non-zero")
	}
	listing := out.String()
	if !strings.Contains(listing, "good one") {
		t.Errorf("listing did not include the test that parses:\n%s", listing)
	}
	if !strings.Contains(listing, "b_bad.test.mdl") {
		t.Errorf("listing did not name the file that failed:\n%s", listing)
	}
}
