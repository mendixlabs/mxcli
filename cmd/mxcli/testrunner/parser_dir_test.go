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

// A folder starting with "_" below the root is skipped, like a dot-folder (the
// Go toolchain's convention), so a deliberate positive-control suite can sit
// beside the real ones without joining the default run. Skipping must not be
// silent: the walk reports what it left out. The root itself is always read.
const nestedControl = "/**\n * @test control\n */\n$c = 5;\n/\n"

func TestParseTestDirSkipsUnderscoreFoldersAndReportsThem(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tests")
	writeNested(t, dir, "unit/a.test.mdl", nestedTestA)
	writeNested(t, dir, "unit/_control/c.test.mdl", nestedControl)
	writeNested(t, dir, "unit/sub/_x/deep/x.test.mdl", nestedControl)
	writeNested(t, dir, "_top/t.test.mdl", nestedControl)
	writeNested(t, dir, ".hidden/h.test.mdl", nestedHidden)

	suite, err := ParseTestDir(dir)
	if err != nil {
		t.Fatalf("ParseTestDir: %v", err)
	}
	if suite.FilesRead != 1 || len(suite.Tests) != 1 || suite.Tests[0].Name != "in A" {
		t.Fatalf("FilesRead = %d, tests = %+v, want only \"in A\"", suite.FilesRead, suite.Tests)
	}
	var got []string
	for _, d := range suite.SkippedDirs {
		got = append(got, filepath.ToSlash(strings.TrimPrefix(d, dir)))
	}
	// Sorted; the dot-folder is noise (.git, .mxcli) and is not reported; a
	// skipped folder's own subfolders are not walked, so not listed either.
	want := "/_top,/unit/_control,/unit/sub/_x"
	if strings.Join(got, ",") != want {
		t.Errorf("SkippedDirs = %v, want %s", got, want)
	}
}

func TestParseTestDirReadsAnUnderscoreRoot(t *testing.T) {
	// `mxcli test tests/_control` runs the control suite: only folders BELOW the
	// root are skipped.
	dir := filepath.Join(t.TempDir(), "tests", "_control")
	writeNested(t, dir, "c.test.mdl", nestedControl)
	writeNested(t, dir, "deeper/d.test.mdl", nestedTestA)
	writeNested(t, dir, "_inner/i.test.mdl", nestedTestB)

	suite, err := ParseTestDir(dir)
	if err != nil {
		t.Fatalf("ParseTestDir: %v", err)
	}
	if suite.FilesRead != 2 || len(suite.Tests) != 2 {
		t.Errorf("FilesRead = %d, tests = %d, want 2 and 2 (root read, _inner skipped)", suite.FilesRead, len(suite.Tests))
	}
	if len(suite.SkippedDirs) != 1 || filepath.Base(suite.SkippedDirs[0]) != "_inner" {
		t.Errorf("SkippedDirs = %v, want just _inner", suite.SkippedDirs)
	}
}

func TestParseTestFilesDeduplicatesAndSortsSkippedDirs(t *testing.T) {
	// The same folder named twice on the command line is reported once.
	dir := t.TempDir()
	writeNested(t, dir, "b/_b/x.test.mdl", nestedControl)
	writeNested(t, dir, "a/_a/x.test.mdl", nestedControl)
	suite, err := parseTestFiles([]string{dir, dir})
	if err != nil {
		t.Fatalf("parseTestFiles: %v", err)
	}
	if len(suite.SkippedDirs) != 2 || filepath.Base(suite.SkippedDirs[0]) != "_a" || filepath.Base(suite.SkippedDirs[1]) != "_b" {
		t.Errorf("SkippedDirs = %v, want [_a _b]", suite.SkippedDirs)
	}
}

func TestSkippedDirsLine(t *testing.T) {
	if got := skippedDirsLine(nil); got != "" {
		t.Errorf("no skipped folders: line = %q, want empty", got)
	}
	got := skippedDirsLine([]string{"tests/unit/_control"})
	want := "  Skipped folder(s) starting with _: tests/unit/_control (name one explicitly to run it)"
	if got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	many := []string{"a/_1", "a/_2", "a/_3", "a/_4", "a/_5", "a/_6", "a/_7"}
	got = skippedDirsLine(many)
	if !strings.Contains(got, "a/_5") || strings.Contains(got, "a/_6") || !strings.Contains(got, "and 2 more") {
		t.Errorf("seven folders: want five named and \"and 2 more\":\n%s", got)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("line must be one line:\n%s", got)
	}
}

func TestEmptySuiteNamesAFolderThatHoldsTheTests(t *testing.T) {
	// The report that started this: the only suites are in a "_" folder, the
	// default run finds nothing, and the message must say where they went.
	dir := filepath.Join(t.TempDir(), "tests")
	writeNested(t, dir, "unit/_control/c.test.mdl", nestedControl)
	msg := emptySuiteError([]string{dir}).Error()
	if !strings.Contains(msg, "_control") || !strings.Contains(msg, "name one") {
		t.Errorf("message does not name the skipped folder and how to run it:\n%s", msg)
	}
	if strings.Contains(msg, "were not read") {
		t.Errorf("claimed misnamed files where there are none:\n%s", msg)
	}
	// Control: a tree with no "_" folder does not mention them.
	plain := emptySuiteError([]string{dirWith(t)}).Error()
	if strings.Contains(plain, "starting with _") {
		t.Errorf("empty directory mentions skipped folders:\n%s", plain)
	}
}

func TestListTestsWithZeroTestsExplainsOnTheHintWriterAndSucceeds(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tests")
	writeNested(t, dir, "unit/_control/c.test.mdl", nestedControl)

	var out, hints strings.Builder
	if err := ListTests([]string{dir}, &out, &hints); err != nil {
		t.Fatalf("ListTests returned %v, want nil: --list stays exit 0", err)
	}
	if !strings.Contains(out.String(), "Found 0 test(s):") {
		t.Errorf("stdout lost the count line:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Skipped folder(s) starting with _") {
		t.Errorf("stdout does not report the skipped folder:\n%s", out.String())
	}
	if !strings.Contains(hints.String(), "no tests found") || !strings.Contains(hints.String(), "_control") {
		t.Errorf("hint writer missing the explanation:\n%s", hints.String())
	}
	if strings.Contains(out.String(), "no tests found") {
		t.Errorf("explanation leaked to stdout, which scripts parse:\n%s", out.String())
	}
}

func TestListTestsWithTestsWritesNoHint(t *testing.T) {
	dir := t.TempDir()
	writeNested(t, dir, "a.test.mdl", nestedTestA)
	var out, hints strings.Builder
	if err := ListTests([]string{dir}, &out, &hints); err != nil {
		t.Fatalf("ListTests: %v", err)
	}
	if hints.String() != "" {
		t.Errorf("hint written for a non-empty listing:\n%s", hints.String())
	}
}

func TestListTestsEveryFileUnparseableSaysSo(t *testing.T) {
	// Mirrors Run: a directory whose every file is malformed is not an empty one.
	dir := t.TempDir()
	writeNested(t, dir, "bad.test.mdl", badTestB)
	var out, hints strings.Builder
	if err := ListTests([]string{dir}, &out, &hints); err == nil {
		t.Fatal("ListTests returned nil for an unparseable file, want the existing non-zero exit")
	}
	if !strings.Contains(hints.String(), "no tests could be parsed") || strings.Contains(hints.String(), "no tests found") {
		t.Errorf("hint = %q, want the \"no tests could be parsed\" variant", hints.String())
	}
}
