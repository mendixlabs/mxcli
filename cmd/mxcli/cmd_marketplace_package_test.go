// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// packageFake replaces the three seams `marketplace package` calls out through —
// the project reader, mx resolution and the mx process — so the command runs
// without Studio Pro. The mx stub records its arguments and, when exit is 0 and
// entries is non-nil, writes a .mpk holding entries into the directory it was
// given with -d.
type packageFake struct {
	version string
	modules []string
	exit    int
	output  string
	entries []string
	calls   [][]string
	mx      string

	mxVersion        string // version of the mx that resolution finds; defaults to version
	mxVersionUnknown bool   // the version of that mx cannot be told
	gotMxbuildPath   string // the --mxbuild-path resolution was given
	inspectCopy      func(workDir string)
}

func installPackageFake(t *testing.T, f *packageFake) {
	t.Helper()
	origInfo, origResolve, origMx, origVer := packageProjectInfo, resolveMxForPackage, mxCreateModulePackageCmd, mxBinaryVersion
	t.Cleanup(func() {
		packageProjectInfo, resolveMxForPackage, mxCreateModulePackageCmd, mxBinaryVersion = origInfo, origResolve, origMx, origVer
	})
	if f.version == "" {
		f.version = "11.12.5"
	}
	if f.mxVersion == "" {
		f.mxVersion = f.version
	}
	packageProjectInfo = func(string) (string, []string, error) { return f.version, f.modules, nil }
	resolveMxForPackage = func(mxbuildPath, version string) (string, error) {
		f.gotMxbuildPath = mxbuildPath
		f.mx = "mx-stub@" + version
		return f.mx, nil
	}
	mxBinaryVersion = func(string) string {
		if f.mxVersionUnknown {
			return ""
		}
		return f.mxVersion
	}
	mxCreateModulePackageCmd = func(_ context.Context, _ string, args []string, out io.Writer) (int, error) {
		f.calls = append(f.calls, args)
		if f.inspectCopy != nil {
			f.inspectCopy(filepath.Dir(args[len(args)-2])) // args end with <work.mpr> <module>
		}
		io.WriteString(out, f.output)
		if f.exit != 0 {
			return f.exit, nil
		}
		if f.entries != nil {
			dir := pkgArgAfter(args, "-d")
			module := args[len(args)-1]
			pkgWriteZip(t, filepath.Join(dir, module+".mpk"), f.entries)
		}
		return 0, nil
	}
}

func pkgArgAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func pkgWriteZip(t *testing.T, path string, names []string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(n, "/") {
			w.Write([]byte("x"))
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

// pkgPlaceholderProject writes an .mpr placeholder: the command only copies and
// names it, the reader is faked.
func pkgPlaceholderProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(p, []byte("not a real mpr"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func runPackageForTest(t *testing.T, opts packageOptions) (stdout, stderr string, res *packageResult, err error) {
	t.Helper()
	var so, se bytes.Buffer
	res, err = runPackage(context.Background(), opts, &so, &se)
	return so.String(), se.String(), res, err
}

var pkgOwnEntries = []string{"project.mpr", "package.xml", "manifest.json", "themesource/pbxexport/web/main.scss"}

func pkgContainsArg(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestPackage_DefaultArguments(t *testing.T) {
	f := &packageFake{modules: []string{"System", "PbxExport"}, entries: pkgOwnEntries}
	installPackageFake(t, f)
	mpr := pkgPlaceholderProject(t)
	out := filepath.Join(t.TempDir(), "new", "dir") // does not exist yet

	_, _, res, err := runPackageForTest(t, packageOptions{ProjectPath: mpr, Module: "pbxexport", OutputDir: out})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("mx called %d times, want 1", len(f.calls))
	}
	args := f.calls[0]
	if args[0] != "create-module-package" {
		t.Errorf("args[0] = %q, want create-module-package", args[0])
	}
	if !pkgContainsArg(args, "-l") {
		t.Errorf("-l (filter required libs) must be the default: %v", args)
	}
	if pkgContainsArg(args, "-e") || pkgContainsArg(args, "-m") {
		t.Errorf("-e/-m must be absent by default: %v", args)
	}
	if got := args[len(args)-1]; got != "PbxExport" {
		t.Errorf("module argument = %q, want the project's spelling PbxExport", got)
	}
	target := args[len(args)-2]
	if target == mpr || filepath.Base(target) != "App.mpr" {
		t.Errorf("mx must run on a temporary copy of the project, got target %q (original %q)", target, mpr)
	}
	if f.mx != "mx-stub@11.12.5" {
		t.Errorf("mx resolved as %q, want the project's version", f.mx)
	}
	want := filepath.Join(out, "PbxExport.mpk")
	if res.Path != want {
		t.Errorf("Path = %q, want %q", res.Path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("package not at the output dir: %v", err)
	}
	if res.Files != len(pkgOwnEntries) {
		t.Errorf("Files = %d, want %d", res.Files, len(pkgOwnEntries))
	}
	if left, _ := filepath.Glob(filepath.Join(out, ".mxcli-package-*")); len(left) != 0 {
		t.Errorf("staging directory left behind: %v", left)
	}
}

func TestPackage_FlagsMapToMxOptions(t *testing.T) {
	f := &packageFake{modules: []string{"PbxExport"}, entries: pkgOwnEntries}
	installPackageFake(t, f)
	_, _, _, err := runPackageForTest(t, packageOptions{
		ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir(),
		AllLibs: true, Exclude: "^resources.EventLog.|^themesource.foo.", ExcludeManaged: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	args := f.calls[0]
	if pkgContainsArg(args, "-l") {
		t.Errorf("--all-libs must drop -l: %v", args)
	}
	if got := pkgArgAfter(args, "-e"); got != "^resources.EventLog.|^themesource.foo." {
		t.Errorf("-e = %q", got)
	}
	if !pkgContainsArg(args, "-m") {
		t.Errorf("--exclude-managed-dependencies must pass -m: %v", args)
	}
}

func TestPackage_MapsMxExitCodes(t *testing.T) {
	cases := []struct {
		exit int
		want string
	}{
		{1, "project file"},
		{2, "output folder"},
		{3, "module name"},
		{4, "unknown export error"},
		{9, "exit status 9"},
	}
	for _, tc := range cases {
		f := &packageFake{modules: []string{"PbxExport"}, exit: tc.exit, output: "MX SAYS NO"}
		installPackageFake(t, f)
		_, _, _, err := runPackageForTest(t, packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir()})
		if err == nil {
			t.Fatalf("exit %d: no error", tc.exit)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("exit %d: error %q does not mention %q", tc.exit, err, tc.want)
		}
		if !strings.Contains(err.Error(), "MX SAYS NO") {
			t.Errorf("exit %d: error must carry mx's output: %q", tc.exit, err)
		}
	}
}

func TestPackage_UnknownModuleListsModulesAndSkipsMx(t *testing.T) {
	f := &packageFake{modules: []string{"System", "Zeta", "Alpha"}}
	installPackageFake(t, f)
	_, _, _, err := runPackageForTest(t, packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "Typo", OutputDir: t.TempDir()})
	if err == nil {
		t.Fatal("want an error for an unknown module")
	}
	for _, want := range []string{`"Typo"`, "Alpha, System, Zeta"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("mx must not run for an unknown module, ran %d time(s)", len(f.calls))
	}
}

func TestPackage_ExitZeroWithoutPackageIsAnError(t *testing.T) {
	// A defensive check: the mx measured (11.12.5) exits 1 for a pattern it cannot
	// parse, but a build that exited 0 without a package must not pass for success.
	f := &packageFake{modules: []string{"PbxExport"}, output: "Usage: mx create-module-package"}
	installPackageFake(t, f)
	_, _, _, err := runPackageForTest(t, packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "wrote no .mpk") || !strings.Contains(err.Error(), "Usage: mx create-module-package") {
		t.Fatalf("want a 'wrote no .mpk' error carrying mx's output, got %v", err)
	}
}

func TestPackage_WarnsAboutForeignEntries(t *testing.T) {
	f := &packageFake{modules: []string{"PbxExport"}, entries: append([]string{
		"resources/EventLog/x.txt", "resources/EventLog/y.txt", "javasource/other/Foo.java",
	}, pkgOwnEntries...)}
	installPackageFake(t, f)
	stdout, stderr, res, err := runPackageForTest(t, packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("foreign entries are a warning, not an error: %v", err)
	}
	if len(res.ForeignEntries) != 3 {
		t.Errorf("ForeignEntries = %v", res.ForeignEntries)
	}
	for _, want := range []string{"resources/EventLog/x.txt", "javasource/other/Foo.java", `--exclude '^javasource.other.|^resources.EventLog.'`} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(stdout, "warning") {
		t.Errorf("warnings belong on stderr, stdout was:\n%s", stdout)
	}
}

func TestPackage_JSONShape(t *testing.T) {
	f := &packageFake{modules: []string{"PbxExport"}, entries: append([]string{"resources/EventLog/x.txt"}, pkgOwnEntries...)}
	installPackageFake(t, f)
	var so bytes.Buffer
	res, err := runPackage(context.Background(), packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir(), JSON: true}, &so, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(so.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, so.String())
	}
	if len(got) != 4 {
		t.Errorf("keys = %v, want path, sizeBytes, files, foreignEntries", got)
	}
	if got["path"] != res.Path || got["files"] != float64(5) || got["sizeBytes"].(float64) <= 0 {
		t.Errorf("unexpected JSON: %v", got)
	}
	if fe, _ := got["foreignEntries"].([]any); len(fe) != 1 || fe[0] != "resources/EventLog/x.txt" {
		t.Errorf("foreignEntries = %v", got["foreignEntries"])
	}
}

func TestPackage_JSONForeignEntriesIsEmptyArrayNotNull(t *testing.T) {
	f := &packageFake{modules: []string{"PbxExport"}, entries: pkgOwnEntries}
	installPackageFake(t, f)
	var so bytes.Buffer
	if _, err := runPackage(context.Background(), packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir(), JSON: true}, &so, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(so.String(), `"foreignEntries": []`) {
		t.Errorf("want an empty array, got:\n%s", so.String())
	}
}

func TestPackage_ReplacesAnExistingPackage(t *testing.T) {
	f := &packageFake{modules: []string{"PbxExport"}, entries: pkgOwnEntries}
	installPackageFake(t, f)
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "PbxExport.mpk"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := runPackage(context.Background(), packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: out}, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != len(pkgOwnEntries) {
		t.Errorf("reported a stale package: %+v", res)
	}
}

func TestPackage_StaleFileIsNotReportedWhenMxWritesNothing(t *testing.T) {
	f := &packageFake{modules: []string{"PbxExport"}} // exit 0, writes nothing
	installPackageFake(t, f)
	out := t.TempDir()
	pkgWriteZip(t, filepath.Join(out, "PbxExport.mpk"), pkgOwnEntries)
	_, _, _, err := runPackageForTest(t, packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: out})
	if err == nil {
		t.Fatal("an old package in the output dir must not pass for a fresh export")
	}
}

func TestClassifyPackageEntries_ForeignEntries(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m.mpk")
	pkgWriteZip(t, p, []string{
		"project.mpr", "package.xml",
		"resources/", "resources/loose.txt", // a directory entry; a file directly under resources/ belongs to no module
		"resources/PbxExport/own.txt", "resources/pbxexport/own2.txt", // own, in any case
		"resources/EventLog/a.txt", "resources\\Win\\b.txt", // backslash separators are normalised
		"javasource/pbxexport/Own.java", "javasource/other/Foo.java",
		"themesource/pbxexport/web/main.scss", "themesource/atlas_core/web/x.scss",
		"userlib/foo.jar", // not attributable to a module: never flagged
	})
	names, err := readPackageEntries(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 12 { // 13 entries minus the directory entry
		t.Errorf("files = %d, want 12", len(names))
	}
	foreign, _ := classifyPackageEntries(names, "PbxExport")
	want := []string{"javasource/other/Foo.java", "resources/EventLog/a.txt", "resources/Win/b.txt", "themesource/atlas_core/web/x.scss"}
	if !reflect.DeepEqual(foreign, want) {
		t.Errorf("foreign = %v\nwant     %v", foreign, want)
	}
}

func TestExcludePattern_PlainDotForm(t *testing.T) {
	pat, ambiguous := excludePatternFor(
		[]string{"resources/EventLog/a.txt", "resources/EventLog/b.txt", "themesource/atlas_core/web/x.scss", "resources/My Folder/c.txt"},
		pkgOwnEntries)
	if pat != "^resources.EventLog.|^resources.My.Folder.|^themesource.atlas_core." || len(ambiguous) != 0 {
		t.Errorf("pattern = %q ambiguous = %v", pat, ambiguous)
	}
	if strings.ContainsAny(pat, `[]\`) {
		t.Errorf("mx rejects character classes and backslashes in -e: %q", pat)
	}
}

func TestExcludePattern_NeverDropsTheModulesOwnFiles(t *testing.T) {
	// Exporting EventLog, with a foreign "Event" folder: '^resources.Event.' would
	// also match resources/EventLog/..., because '.' stands for any character.
	pat, ambiguous := excludePatternFor(
		[]string{"resources/Event/a.txt"},
		[]string{"resources/EventLog/own.txt"})
	if pat != "" || !reflect.DeepEqual(ambiguous, []string{"resources/Event/a.txt"}) {
		t.Errorf("pattern = %q ambiguous = %v, want the clash reported and no pattern", pat, ambiguous)
	}
}

func TestPackage_RefusesAnMxOfAnotherVersion(t *testing.T) {
	// docker.ResolveMxForVersion prefers an mx on PATH and falls back to any cached
	// one, so the mx it returns may belong to another Mendix version than the app.
	f := &packageFake{modules: []string{"PbxExport"}, version: "11.12.5", mxVersion: "11.12.1", entries: pkgOwnEntries}
	installPackageFake(t, f)
	out := t.TempDir()
	_, _, _, err := runPackageForTest(t, packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: out})
	if err == nil {
		t.Fatal("an mx of Mendix 11.12.1 made a package for a 11.12.5 app without complaint")
	}
	for _, want := range []string{"11.12.1", "11.12.5", "mxcli setup mxbuild --version 11.12.5", "--mxbuild-path"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should contain %q:\n%v", want, err)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("mx ran %d time(s) although it is the wrong version", len(f.calls))
	}
	if left, _ := filepath.Glob(filepath.Join(out, "*")); len(left) != 0 {
		t.Errorf("a refused export left %v behind", left)
	}
}

func TestPackage_AcceptsAnMxOfTheProjectsVersion(t *testing.T) {
	cases := []struct{ project, mx string }{
		{"11.12.5", "11.12.5"},
		{"10.24.9", "10.24.9.81004"},    // Studio Pro folders carry the build number
		{"10.24.9.81004", "10.24.9"},    // and so may the project's stamp
		{"11.12.5.1234", "11.12.5.999"}, // same release, other build
	}
	for _, tc := range cases {
		f := &packageFake{modules: []string{"PbxExport"}, version: tc.project, mxVersion: tc.mx, entries: pkgOwnEntries}
		installPackageFake(t, f)
		_, stderr, _, err := runPackageForTest(t, packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir()})
		if err != nil {
			t.Errorf("project %s, mx %s: %v", tc.project, tc.mx, err)
		}
		if strings.Contains(stderr, "warning") {
			t.Errorf("project %s, mx %s: unexpected warning:\n%s", tc.project, tc.mx, stderr)
		}
	}
}

func TestPackage_RefusesAnMxWhoseVersionIsUnknown(t *testing.T) {
	f := &packageFake{modules: []string{"PbxExport"}, mxVersionUnknown: true, entries: pkgOwnEntries}
	installPackageFake(t, f)
	_, _, _, err := runPackageForTest(t, packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "--mxbuild-path") || !strings.Contains(err.Error(), "11.12.5") {
		t.Fatalf("want a refusal naming the project's version and the override, got %v", err)
	}
	if len(f.calls) != 0 {
		t.Error("mx ran although its version could not be told")
	}
}

func TestPackage_ExplicitMxbuildPathOverridesButWarns(t *testing.T) {
	mxbuildDir := t.TempDir()
	for _, tc := range []struct {
		name    string
		unknown bool
		mx      string
		want    []string
	}{
		{"mismatch", false, "11.12.1", []string{"warning", "11.12.1", "11.12.5"}},
		{"unknown", true, "", []string{"warning", "cannot tell", "11.12.5"}},
	} {
		f := &packageFake{modules: []string{"PbxExport"}, mxVersion: tc.mx, mxVersionUnknown: tc.unknown, entries: pkgOwnEntries}
		installPackageFake(t, f)
		stdout, stderr, res, err := runPackageForTest(t, packageOptions{
			ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir(), MxBuildPath: mxbuildDir, JSON: true})
		if err != nil || res == nil {
			t.Errorf("%s: an explicit --mxbuild-path is the override, got %v", tc.name, err)
			continue
		}
		if f.gotMxbuildPath != mxbuildDir {
			t.Errorf("%s: resolution got --mxbuild-path %q, want %q", tc.name, f.gotMxbuildPath, mxbuildDir)
		}
		for _, w := range tc.want {
			if !strings.Contains(stderr, w) {
				t.Errorf("%s: stderr should contain %q:\n%s", tc.name, w, stderr)
			}
		}
		if strings.Contains(stdout, "warning") || !json.Valid([]byte(stdout)) {
			t.Errorf("%s: the warning must stay off stdout, which carries the JSON:\n%s", tc.name, stdout)
		}
		if len(f.calls) != 1 {
			t.Errorf("%s: mx should have run once, ran %d times", tc.name, len(f.calls))
		}
	}
}

func TestPackage_ExplicitMxbuildPathThatDoesNotExistIsAnError(t *testing.T) {
	// ResolveMxForVersion silently falls back to PATH when the explicit path is
	// bad; the override must not turn a typo into "any mx on PATH, warned about".
	f := &packageFake{modules: []string{"PbxExport"}, entries: pkgOwnEntries}
	installPackageFake(t, f)
	_, _, _, err := runPackageForTest(t, packageOptions{
		ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir(),
		MxBuildPath: filepath.Join(t.TempDir(), "no-such-mendix")})
	if err == nil || !strings.Contains(err.Error(), "--mxbuild-path") {
		t.Fatalf("want an error naming --mxbuild-path, got %v", err)
	}
	if len(f.calls) != 0 {
		t.Error("mx ran")
	}
}

func TestPackage_MxOutputOnSuccessGoesToStderr(t *testing.T) {
	f := &packageFake{modules: []string{"PbxExport"}, output: "MX WARNING: something to read\n", entries: pkgOwnEntries}
	installPackageFake(t, f)
	stdout, stderr, _, err := runPackageForTest(t, packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir(), JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "MX WARNING: something to read") {
		t.Errorf("mx's output never reached the user:\nstderr: %q", stderr)
	}
	if strings.Contains(stdout, "MX WARNING") || !json.Valid([]byte(stdout)) {
		t.Errorf("mx's output must not go to stdout, which carries the JSON:\n%s", stdout)
	}
}

func TestPackage_NoMxOutputMeansNothingOnStderr(t *testing.T) {
	f := &packageFake{modules: []string{"PbxExport"}, entries: pkgOwnEntries}
	installPackageFake(t, f)
	_, stderr, _, err := runPackageForTest(t, packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir()})
	if err != nil || stderr != "" {
		t.Errorf("err = %v, stderr = %q", err, stderr)
	}
}

// TestMxCreateModulePackageCmd_StopsWhenContextIsCancelled runs the real process
// runner (not the seam) on this test binary acting as a child that sleeps.
func TestMxCreateModulePackageCmd_StopsWhenContextIsCancelled(t *testing.T) {
	if os.Getenv("MXCLI_TEST_SLEEP_CHILD") == "1" {
		time.Sleep(time.Minute)
		return
	}
	t.Setenv("MXCLI_TEST_SLEEP_CHILD", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	var out bytes.Buffer
	code, err := mxCreateModulePackageCmd(ctx, os.Args[0], []string{"-test.run=^TestMxCreateModulePackageCmd_StopsWhenContextIsCancelled$"}, &out)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("code = %d, err = %v, want the context's error", code, err)
	}
	if time.Since(start) > 20*time.Second {
		t.Errorf("mx was not stopped by the context (took %s)", time.Since(start))
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		`^resources.EventLog.`: `'^resources.EventLog.'`,
		``:                     `''`,
		`it's`:                 `'it'\''s'`,
		`'a'|'b'`:              `''\''a'\''|'\''b'\'''`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestWarnForeignEntries_QuotesAUsersQuoteSafelyForPasting(t *testing.T) {
	var w bytes.Buffer
	warnForeignEntries(&w, "PbxExport", []string{"resources/EventLog/a.txt"}, pkgOwnEntries, `^resources.it's`)
	want := `--exclude '^resources.it'\''s|^resources.EventLog.'`
	if !strings.Contains(w.String(), want) {
		t.Errorf("want %s in:\n%s", want, w.String())
	}
}

func TestPackage_OutputFolderInsideTheProjectIsNotCopied(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	os.WriteFile(mpr, []byte("not a real mpr"), 0o644)
	os.MkdirAll(filepath.Join(dir, "dist"), 0o755)
	os.WriteFile(filepath.Join(dir, "dist", "Old.mpk"), []byte("earlier package"), 0o644)
	os.MkdirAll(filepath.Join(dir, "widgets"), 0o755)
	os.WriteFile(filepath.Join(dir, "widgets", "w.mpk"), []byte("a widget, mx needs it"), 0o644)

	var distCopied, widgetCopied bool
	f := &packageFake{modules: []string{"PbxExport"}, entries: pkgOwnEntries, inspectCopy: func(work string) {
		_, e1 := os.Stat(filepath.Join(work, "dist", "Old.mpk"))
		_, e2 := os.Stat(filepath.Join(work, "widgets", "w.mpk"))
		distCopied, widgetCopied = e1 == nil, e2 == nil
	}}
	installPackageFake(t, f)
	res, err := runPackage(context.Background(), packageOptions{ProjectPath: mpr, Module: "PbxExport", OutputDir: filepath.Join(dir, "dist")}, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if distCopied {
		t.Error("the output folder's earlier packages were copied along with the project")
	}
	if !widgetCopied {
		t.Error("the rest of the project must still be copied")
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Errorf("package missing: %v", err)
	}
}

func TestPackage_OutputFolderIsTheProjectFolderStillCopiesTheProject(t *testing.T) {
	// "-o ." run from inside the project: skipping the output folder must never
	// skip the project itself.
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	os.WriteFile(mpr, []byte("not a real mpr"), 0o644)
	var mprCopied bool
	f := &packageFake{modules: []string{"PbxExport"}, entries: pkgOwnEntries, inspectCopy: func(work string) {
		_, err := os.Stat(filepath.Join(work, "App.mpr"))
		mprCopied = err == nil
	}}
	installPackageFake(t, f)
	if _, err := runPackage(context.Background(), packageOptions{ProjectPath: mpr, Module: "PbxExport", OutputDir: dir}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !mprCopied {
		t.Error("the project was not copied when the output folder is the project folder")
	}
}
