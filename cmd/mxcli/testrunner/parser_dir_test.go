// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1363: `mxcli test tests --list` reported "Found 0 test(s)"
// when the suites live in tests/<Module>/*.test.mdl, while `mxcli test
// tests/Sales --list` found them. ParseTestDir skipped every subdirectory.

const (
	nestedTestA   = "/**\n * @test in A\n */\n$x = 1;\n/\n"
	nestedTestB   = "# Spec\n\n```mdl-test\n/** @test in B sub */\n$y = 2;\n```\n"
	nestedTestTop = "/**\n * @test at top\n */\n$z = 3;\n/\n"
	nestedHidden  = "/**\n * @test hidden\n */\n$h = 4;\n/\n"
)

func writeNested(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestParseTestDirFindsTestsInSubfolders(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tests")
	writeNested(t, dir, "A/a.test.mdl", nestedTestA)
	writeNested(t, dir, "B/sub/b.test.md", nestedTestB)
	writeNested(t, dir, "top.test.mdl", nestedTestTop)
	writeNested(t, dir, ".hidden/x.test.mdl", nestedHidden)

	suite, err := ParseTestDir(dir)
	if err != nil {
		t.Fatalf("ParseTestDir: %v", err)
	}
	if suite.Name != "tests" {
		t.Errorf("suite name = %q, want %q", suite.Name, "tests")
	}
	if suite.FilesRead != 3 {
		t.Errorf("FilesRead = %d, want 3 (the hidden folder is not read)", suite.FilesRead)
	}
	var got []string
	for _, tc := range suite.Tests {
		got = append(got, tc.Name)
	}
	// Lexical walk order: A, B/sub, then top.test.mdl.
	want := "in A,in B sub,at top"
	if strings.Join(got, ",") != want {
		t.Errorf("tests = %v, want %s", got, want)
	}
	if len(suite.FileErrors) != 0 {
		t.Errorf("unexpected file errors: %v", suite.FileErrors)
	}
}

func TestParseTestDirBadFileInSubfolderDoesNotStopTheWalk(t *testing.T) {
	dir := t.TempDir()
	writeNested(t, dir, "A/a.test.mdl", nestedTestA)
	writeNested(t, dir, "B/bad.test.mdl", badTestB)
	writeNested(t, dir, "C/c.test.mdl", goodTestC)

	suite, err := ParseTestDir(dir)
	if err != nil {
		t.Fatalf("ParseTestDir returned a hard error: %v", err)
	}
	if suite.FilesRead != 3 {
		t.Errorf("FilesRead = %d, want 3", suite.FilesRead)
	}
	if len(suite.Tests) != 2 || suite.Tests[0].Name != "in A" || suite.Tests[1].Name != "also good" {
		t.Errorf("tests = %+v, want [in A, also good]", suite.Tests)
	}
	if len(suite.FileErrors) != 1 || !strings.Contains(filepath.ToSlash(suite.FileErrors[0].Path), "B/bad.test.mdl") {
		t.Fatalf("FileErrors = %+v, want exactly B/bad.test.mdl", suite.FileErrors)
	}
}

func TestParseTestDirHiddenRootIsStillRead(t *testing.T) {
	// Only hidden folders BELOW the root are skipped: `mxcli test .` has a root
	// named ".", and a project kept under a dot-folder is the user's choice.
	dir := filepath.Join(t.TempDir(), ".suite")
	writeNested(t, dir, "a.test.mdl", nestedTestA)
	suite, err := ParseTestDir(dir)
	if err != nil {
		t.Fatalf("ParseTestDir: %v", err)
	}
	if suite.FilesRead != 1 || len(suite.Tests) != 1 {
		t.Errorf("FilesRead = %d, tests = %d, want 1 and 1", suite.FilesRead, len(suite.Tests))
	}
}

func TestEmptySuiteNamesSkippedCandidatesInSubfolders(t *testing.T) {
	dir := t.TempDir()
	writeNested(t, dir, "Sales/workflow.mdl", "show entities;\n")
	writeNested(t, dir, ".git/ignored.mdl", "show entities;\n")
	msg := emptySuiteError([]string{dir}).Error()
	if !strings.Contains(msg, "workflow.mdl") {
		t.Errorf("nested skipped candidate not named:\n%s", msg)
	}
	if strings.Contains(msg, "ignored.mdl") {
		t.Errorf("file in a hidden folder listed:\n%s", msg)
	}
}
