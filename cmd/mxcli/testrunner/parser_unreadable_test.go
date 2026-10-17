// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// mendixlabs/mxcli#1368 review: with discovery recursive, one folder that cannot
// be read (a locked folder, a root-owned folder in a mounted volume) must not
// fail the whole run. Below the root the error belongs to that entry; only an
// error on the root itself is fatal.

func dirEntryOf(t *testing.T, path string) fs.DirEntry {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fs.FileInfoToDirEntry(info)
}

// fakeWalk replays what filepath.WalkDir does for a tree whose "locked" folder
// cannot be read: the callback is called for the folder first, and again with the
// ReadDir error. It makes the logic testable where chmod 000 does not block
// reading (Windows) or does not apply (root).
func fakeWalk(t *testing.T, permErr error) walkFunc {
	t.Helper()
	return func(root string, fn fs.WalkDirFunc) error {
		sales := filepath.Join(root, "Sales")
		locked := filepath.Join(root, "locked")
		steps := []struct {
			path string
			err  error
		}{
			{root, nil},
			{sales, nil},
			{filepath.Join(sales, "a.test.mdl"), nil},
			{locked, nil},
			{locked, permErr},
			{filepath.Join(root, "z.test.mdl"), nil},
		}
		for _, s := range steps {
			err := fn(s.path, dirEntryOf(t, s.path), s.err)
			if errors.Is(err, fs.SkipDir) {
				continue
			}
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func lockedFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "tests")
	writeNested(t, root, "Sales/a.test.mdl", nestedTestA)
	writeNested(t, root, "z.test.mdl", nestedTestTop)
	if err := os.MkdirAll(filepath.Join(root, "locked"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestWalkTestTreeErrorBelowTheRootIsThatEntrysAndTheWalkContinues(t *testing.T) {
	root := lockedFixture(t)
	permErr := &fs.PathError{Op: "open", Path: filepath.Join(root, "locked"), Err: fs.ErrPermission}

	var files []string
	var errPaths []string
	var errs []error
	_, err := walkTestTreeWith(fakeWalk(t, permErr), root,
		func(path string, d fs.DirEntry) { files = append(files, filepath.Base(path)) },
		func(path string, err error) { errPaths = append(errPaths, path); errs = append(errs, err) })
	if err != nil {
		t.Fatalf("walk returned %v, want nil: an error below the root is that entry's", err)
	}
	if got := strings.Join(files, ","); got != "a.test.mdl,z.test.mdl" {
		t.Errorf("files visited = %q, want both readable files", got)
	}
	if len(errPaths) != 1 || errPaths[0] != filepath.Join(root, "locked") {
		t.Fatalf("onErr paths = %v, want just the locked folder", errPaths)
	}
	if !errors.Is(errs[0], fs.ErrPermission) {
		t.Errorf("onErr error = %v, want the permission error", errs[0])
	}
}

func TestWalkTestTreeErrorBelowTheRootWithoutACallbackIsIgnored(t *testing.T) {
	// emptySuiteError walks only to name candidates; it passes no callback.
	root := lockedFixture(t)
	permErr := &fs.PathError{Op: "open", Path: filepath.Join(root, "locked"), Err: fs.ErrPermission}
	var files int
	if _, err := walkTestTreeWith(fakeWalk(t, permErr), root,
		func(string, fs.DirEntry) { files++ }, nil); err != nil {
		t.Fatalf("walk returned %v, want nil", err)
	}
	if files != 2 {
		t.Errorf("files visited = %d, want 2", files)
	}
}

func TestWalkTestTreeErrorOnTheRootIsFatal(t *testing.T) {
	root := lockedFixture(t)
	rootErr := &fs.PathError{Op: "open", Path: root, Err: fs.ErrPermission}
	walk := func(r string, fn fs.WalkDirFunc) error { return fn(r, nil, rootErr) }
	called := false
	_, err := walkTestTreeWith(walk, root, func(string, fs.DirEntry) {},
		func(string, error) { called = true })
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("walk returned %v, want the root's permission error", err)
	}
	if called {
		t.Error("the root's error was handed to onErr; it must fail the run instead")
	}
}

func TestParseTestDirUnreadableSubfolderKeepsTheOtherTests(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 does not stop a directory being read on Windows; the walk logic is covered by TestWalkTestTreeErrorBelowTheRootIsThatEntrysAndTheWalkContinues")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 000 does not stop root reading the folder")
	}
	root := lockedFixture(t)
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	suite, err := ParseTestDir(root)
	if err != nil {
		t.Fatalf("ParseTestDir returned a hard error: %v", err)
	}
	if len(suite.Tests) != 2 || suite.FilesRead != 2 {
		t.Errorf("tests = %d, FilesRead = %d, want the 2 readable files kept", len(suite.Tests), suite.FilesRead)
	}
	if len(suite.FileErrors) != 1 || !suite.FileErrors[0].Dir || suite.FileErrors[0].Path != locked {
		t.Fatalf("FileErrors = %+v, want exactly the locked folder, marked as a folder", suite.FileErrors)
	}
	if !errors.Is(suite.FileErrors[0].Err, fs.ErrPermission) {
		t.Errorf("error = %v, want a permission error", suite.FileErrors[0].Err)
	}
	// A folder that cannot be read is not an empty one, and the run stays red.
	if res := suiteFileErrorResults(suite.FileErrors); len(res) != 1 || res[0].Status != StatusError {
		t.Errorf("results = %+v, want one ERROR so the run is red", res)
	}
}

func TestParseTestDirSymlinkedRootReportsThePathTheUserTyped(t *testing.T) {
	target := filepath.Join(t.TempDir(), "realtests")
	writeNested(t, target, "A/a.test.mdl", nestedTestA)
	writeNested(t, target, "B/bad.test.mdl", badTestB)
	writeNested(t, target, "_control/c.test.mdl", nestedControl)
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	suite, err := ParseTestDir(link)
	if err != nil {
		t.Fatalf("ParseTestDir: %v", err)
	}
	if len(suite.Tests) != 1 || !strings.HasPrefix(suite.Tests[0].SourceFile, link) {
		t.Errorf("tests = %+v, want one test whose SourceFile is under %s", suite.Tests, link)
	}
	if len(suite.FileErrors) != 1 || !strings.HasPrefix(suite.FileErrors[0].Path, link) {
		t.Errorf("FileErrors = %+v, want the bad file under %s", suite.FileErrors, link)
	}
	if len(suite.SkippedDirs) != 1 || !strings.HasPrefix(suite.SkippedDirs[0], link) {
		t.Errorf("SkippedDirs = %v, want the _ folder under %s", suite.SkippedDirs, link)
	}
	for _, p := range append(suite.SkippedDirs, suite.FileErrors[0].Path, suite.Tests[0].SourceFile) {
		if strings.Contains(p, "realtests") {
			t.Errorf("path %q shows the resolved target, not the path typed", p)
		}
	}
}

func TestWalkTestTreeSymlinkedRootReportsErrorsUnderTheTypedName(t *testing.T) {
	target := filepath.Join(t.TempDir(), "realtests")
	writeNested(t, target, "Sales/a.test.mdl", nestedTestA)
	writeNested(t, target, "z.test.mdl", nestedTestTop)
	if err := os.MkdirAll(filepath.Join(target, "locked"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}
	permErr := &fs.PathError{Op: "open", Path: filepath.Join(resolved, "locked"), Err: fs.ErrPermission}

	var gotPath string
	var gotErr error
	_, err = walkTestTreeWith(fakeWalk(t, permErr), link, func(string, fs.DirEntry) {},
		func(path string, err error) { gotPath, gotErr = path, err })
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if want := filepath.Join(link, "locked"); gotPath != want {
		t.Errorf("onErr path = %q, want %q", gotPath, want)
	}
	if gotErr == nil || strings.Contains(gotErr.Error(), "realtests") || !strings.Contains(gotErr.Error(), "linked") {
		t.Errorf("error text = %v, want the path the user typed, not the resolved one", gotErr)
	}
	if !errors.Is(gotErr, fs.ErrPermission) {
		t.Errorf("rewriting the path lost the cause: %v", gotErr)
	}
}

// The messages say what happened. "file(s) failed to parse" for a folder that
// could not be read sends the reader to a parse error that does not exist.
func TestFailureMessagesDistinguishFoldersFromFiles(t *testing.T) {
	dirErr := FileError{Path: "tests/locked", Err: &fs.PathError{Op: "open", Path: "tests/locked", Err: fs.ErrPermission}, Dir: true}
	fileErr := FileError{Path: "tests/bad.test.mdl", Err: errors.New("bad block")}

	if got := noTestsParsedError([]FileError{dirErr}).Error(); !strings.Contains(got, "could not be read") || strings.Contains(got, "parse") {
		t.Errorf("folders only: %q, want \"could not be read\" and no mention of parsing", got)
	}
	if got := noTestsParsedError([]FileError{fileErr}).Error(); got != "no tests could be parsed: 1 file(s) failed to parse" {
		t.Errorf("files only: %q, want the unchanged message", got)
	}
	got := noTestsParsedError([]FileError{fileErr, dirErr}).Error()
	if !strings.Contains(got, "1 file(s) failed to parse") || !strings.Contains(got, "1 folder(s) could not be read") {
		t.Errorf("mixed: %q, want both counts", got)
	}

	var out strings.Builder
	err := reportFileErrors([]FileError{dirErr}, &out)
	if err == nil || strings.Contains(err.Error(), "parsed") || !strings.Contains(err.Error(), "folder(s) could not be read") {
		t.Errorf("reportFileErrors error = %v, want a folder wording", err)
	}
	if strings.Contains(out.String(), "could not be parsed") || !strings.Contains(out.String(), "tests/locked") {
		t.Errorf("report:\n%s", out.String())
	}

	res := suiteFileErrorResults([]FileError{dirErr, fileErr})
	if !strings.Contains(res[0].Name, "folder could not be read") || !strings.Contains(res[1].Name, "file could not be parsed") {
		t.Errorf("result names = %q, %q", res[0].Name, res[1].Name)
	}
}

// testFilesMTime shares discovery with the parser, so it cannot disagree with it
// about what is a test: hidden and "_" folders below the root are not entered,
// and the root is entered whatever it is called.
func TestTestFilesMTimeFollowsTheDiscoveryRules(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	now := time.Now()
	touch := func(path string, at time.Time) {
		t.Helper()
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}

	dir := filepath.Join(t.TempDir(), "tests")
	writeNested(t, dir, "A/a.test.mdl", nestedTestA)
	writeNested(t, dir, ".hidden/h.test.mdl", nestedHidden)
	writeNested(t, dir, "_control/c.test.mdl", nestedControl)
	for _, f := range []string{"A/a.test.mdl", ".hidden/h.test.mdl", "_control/c.test.mdl"} {
		touch(filepath.Join(dir, filepath.FromSlash(f)), old)
	}
	for _, d := range []string{"A", ".hidden", "_control", "."} {
		touch(filepath.Join(dir, d), old)
	}
	before := testFilesMTime([]string{dir})

	touch(filepath.Join(dir, ".hidden", "h.test.mdl"), now)
	touch(filepath.Join(dir, "_control", "c.test.mdl"), now)
	for _, d := range []string{"A", ".hidden", "_control", "."} {
		touch(filepath.Join(dir, d), old)
	}
	if after := testFilesMTime([]string{dir}); after.After(before) {
		t.Error("an edit in a hidden or _ folder moved the change signal, but discovery never reads those")
	}

	touch(filepath.Join(dir, "A", "a.test.mdl"), now)
	touch(filepath.Join(dir, "A"), old)
	if after := testFilesMTime([]string{dir}); !after.After(before) {
		t.Error("an edit in a subfolder produced no change signal")
	}

	// The root itself is read whatever it is called, as for the parser.
	hiddenRoot := filepath.Join(t.TempDir(), "_suite")
	writeNested(t, hiddenRoot, "x.test.mdl", nestedTestA)
	touch(filepath.Join(hiddenRoot, "x.test.mdl"), now)
	touch(hiddenRoot, old)
	if got := testFilesMTime([]string{hiddenRoot}); got.Before(now.Add(-time.Minute)) {
		t.Errorf("a root named with a leading _ was not read: mtime = %v", got)
	}
}

// The mapping from the resolved root back to the path the user typed is pure, so
// it is tested without a symlink (creating one needs a privilege on Windows).
func TestUnderTypedMapsAResolvedRootBackToWhatWasTyped(t *testing.T) {
	typed := filepath.Join("work", "tests")
	resolved := filepath.Join("mnt", "data", "suites")

	if got, want := underTypedPath(typed, resolved, filepath.Join(resolved, "A", "a.test.mdl")), filepath.Join(typed, "A", "a.test.mdl"); got != want {
		t.Errorf("underTypedPath = %q, want %q", got, want)
	}
	if got := underTypedPath(typed, typed, filepath.Join(typed, "A")); got != filepath.Join(typed, "A") {
		t.Errorf("no symlink: path changed to %q", got)
	}
	// A "./tests" root stays as typed when nothing was resolved.
	if got := underTypedPath("./tests", "./tests", "tests/A"); got != "tests/A" {
		t.Errorf("no symlink: path rewritten to %q", got)
	}

	cause := fs.ErrPermission
	err := underTyped(typed, resolved, &fs.PathError{Op: "open", Path: filepath.Join(resolved, "locked"), Err: cause})
	if strings.Contains(err.Error(), "suites") || !strings.Contains(err.Error(), filepath.Join(typed, "locked")) {
		t.Errorf("error text = %q, want the typed path", err)
	}
	if !errors.Is(err, cause) {
		t.Errorf("the cause was lost: %v", err)
	}
	plain := errors.New("not a path error")
	if got := underTyped(typed, resolved, plain); got != plain {
		t.Errorf("an error that names no path was changed to %v", got)
	}
}
