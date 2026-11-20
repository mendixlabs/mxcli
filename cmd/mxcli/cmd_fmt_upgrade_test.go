// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func runFmt(t *testing.T, args ...string) (string, error) {
	t.Helper()
	// cobra keeps flag values between runs in one process.
	for _, f := range []string{"write", "upgrade", "header", "force-header"} {
		_ = fmtCmd.Flags().Set(f, fmtCmd.Flags().Lookup(f).DefValue)
		fmtCmd.Flags().Lookup(f).Changed = false
	}
	_ = rootCmd.PersistentFlags().Set("project", "")
	rootCmd.PersistentFlags().Lookup("project").Changed = false
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(append([]string{"fmt"}, args...))
	err := rootCmd.ExecuteContext(context.Background())
	return out.String(), err
}

func TestFmtUpgrade_WritesOnlyTheRewrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.mdl")
	src := "-- keep me\nCREATE OR REPLACE entity M.User (Name: String);\n   show entities;\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runFmt(t, "--upgrade", "--header=false", "-w", path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	// Only the deprecated keywords change: no heuristic upper-casing (which
	// would turn M.User into M.USER), no re-indentation, and no header when it
	// is declined.
	want := "-- keep me\nCREATE OR MODIFY entity M.User (Name: String);\n   list entities;\n"
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	// mdl 1 is frozen (ako/mxcli#714): --upgrade adds the header by default.
	if _, err := runFmt(t, "--upgrade", "-w", path); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if !strings.HasPrefix(string(got), "mdl 1;\n-- keep me\n") {
		t.Fatalf("--upgrade did not add the header:\n%s", got)
	}

	// Idempotent: a second run leaves the file as it is.
	before := string(got)
	if _, err := runFmt(t, "--upgrade", "--header", "-w", path); err != nil {
		t.Fatal(err)
	}
	if got, _ = os.ReadFile(path); string(got) != before {
		t.Fatalf("second fmt --upgrade changed the file:\n%s", got)
	}
}

func TestFmtUpgrade_HeaderNeedsUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.mdl")
	if err := os.WriteFile(path, []byte("show entities;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runFmt(t, "--header", path); err == nil || !strings.Contains(err.Error(), "--upgrade") {
		t.Fatalf("--header without --upgrade: %v", err)
	}
}

// A construct with no mechanical rewrite is reported with its reason, the
// header is refused, and the file is left as it was.
func TestFmtUpgrade_ReportsWhatItCannotRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.mdl")
	src := "create microflow M.F ($L: List of M.E) begin\n  $n = count(filter($L, Name = 'x'));\nend\n/\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runFmt(t, "--upgrade", "--header", "-w", path)
	if err == nil || !strings.Contains(err.Error(), "MDL-V1-LIST") || !strings.Contains(err.Error(), "nested call") {
		t.Fatalf("want the nested list operation reported with its reason, got %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != src {
		t.Fatalf("the file was changed although the header was refused:\n%s", got)
	}
}

// Since the freeze --upgrade adds the header by default (ako/mxcli#714), so a
// construct that blocks it now fails a plain `fmt --upgrade`, which before the
// freeze upgraded the spellings and succeeded. The refusal names the way back
// to that, `--header=false`, and that way works: the spellings are upgraded,
// the blocked construct is left, and no header is added.
func TestFmtUpgrade_BlockedDefaultHeaderNamesTheOptOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.mdl")
	src := "create microflow M.F ($L: List of M.E) begin\n  $n = count(filter($L, Name = 'x'));\nend\n/\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runFmt(t, "--upgrade", "-w", path)
	if err == nil || !strings.Contains(err.Error(), "MDL-V1-LIST") {
		t.Fatalf("want the default header refused over the nested list operation, got %v", err)
	}
	if !strings.Contains(err.Error(), "--header=false") {
		t.Errorf("the refusal of the default header does not name --header=false:\n%v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != src {
		t.Fatalf("the file was changed although the header was refused:\n%s", got)
	}

	if _, err := runFmt(t, "--upgrade", "--header=false", "-w", path); err != nil {
		t.Fatalf("--header=false: %v", err)
	}
	if got, _ := os.ReadFile(path); strings.HasPrefix(string(got), "mdl ") {
		t.Fatalf("--header=false added a header:\n%s", got)
	}

	// Asked for explicitly, the header is the point: no hint to decline it.
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runFmt(t, "--upgrade", "--header", "-w", path); err == nil || strings.Contains(err.Error(), "--header=false") {
		t.Fatalf("an explicit --header: want the refusal without the opt-out hint, got %v", err)
	}
}

// A .test.mdl that check accepts is one fmt --upgrade can read (ako/mxcli#837):
// the bodies are upgraded and the @test / @expect doc comments are kept
// verbatim. Since the runner and check read a test file's header
// (ako/mxcli#847), the header is added by default, as for a script, with the
// header-gated `limit 1` rewritten to keep its meaning; --header=false
// declines it.
func TestFmtUpgrade_TestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "csv-import.test.mdl")
	src := "-- tests\n\n/**\n * @test Head of the rows\n * @expect $first/Merchant = 'Albert Heijn'\n */\n" +
		"retrieve $rows from Ledger.ImportRow where Batch = 't' limit 1;\n$first = head($rows);\n/\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runFmt(t, "--upgrade", "--header=false", "-w", path); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(src, "head($rows)", "head $rows", 1)
	if got, _ := os.ReadFile(path); string(got) != want {
		t.Fatalf("--header=false: got:\n%s\nwant:\n%s", got, want)
	}

	if _, err := runFmt(t, "--upgrade", "-w", path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(got), "mdl 1;\n-- tests\n") {
		t.Fatalf("the header was not added to the test file:\n%s", got)
	}
	if strings.Contains(string(got), "limit 1;") {
		t.Errorf("the header-gated `limit 1` was left in an mdl 1 file:\n%s", got)
	}
	if !strings.Contains(string(got), " * @test Head of the rows\n * @expect $first/Merchant = 'Albert Heijn'\n") {
		t.Errorf("the doc comment was not kept:\n%s", got)
	}

	// Plain fmt does not format a test file: it says what does.
	if _, err := runFmt(t, path); err == nil || !strings.Contains(err.Error(), "--upgrade") {
		t.Fatalf("plain fmt on a test file: %v", err)
	}
}

// `find(…)` over a flow call's result: without a project fmt cannot tell the
// string function from the List operation and refuses the header, saying to
// pass the project; with -p it reads what the called flow returns there and
// rewrites (ako/mxcli#860). PedApp's flows are Studio Pro-authored.
func TestFmtUpgrade_FindOnACallResultReadsTheProject(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "PedApp.mpr")

	path := filepath.Join(t.TempDir(), "s.mdl")
	const script = "create microflow MyFirstModule.F ($o: Administration.Account) begin\n" +
		"  declare $Pos Integer = 0;\n" +
		"  $Url = call microflow FeedbackModule.ConvertUUIDToURL(uuid = 'x');\n" +
		"  $Pos = find($Url, 'x');\n" +
		"  $Ctx = call nanoflow Atlas_Web_Content.DS_LoginContext();\n" +
		"  $Hit = find($Ctx, $currentObject = $o);\n" +
		"end;\n"
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runFmt(t, "--upgrade", "--header", "-w", path)
	if err == nil || !strings.Contains(err.Error(), "pass the project") {
		t.Fatalf("without -p: want the header refused, asking for the project; got %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(path); string(got) != script {
		t.Fatalf("a refused upgrade wrote the file:\n%s", got)
	}

	if out, err := runFmt(t, "--upgrade", "--header", "-w", "-p", mpr, path); err != nil {
		t.Fatalf("with -p: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(path)
	for _, want := range []string{
		"mdl 1;\n",
		"\n  set $Pos = find($Url, 'x');\n", // ConvertUUIDToURL returns a String
		"\n  $Hit = find $Ctx where $currentObject = $o;\n", // DS_LoginContext does not
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
}

// copyTree copies a fixture project, leaving out the .mxcli caches other
// packages' tests write into it in place (testutil.CopyFixture).
func copyTree(src, dst string) error {
	return testutil.CopyFixture(dst, src)
}

// ako/mxcli#873: without a project, a bare commit in a `create or modify`
// flow is left as written and reported — its meaning changed with #895, and
// only the stored flow says which one the script should keep.
func TestFmtUpgrade_BareCommitWithoutProjectIsANote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.mdl")
	src := "create or modify microflow M.F ($A: M.E)\nbegin\n  commit $A;\nend;\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runFmt(t, "--upgrade", "-w", path)
	if err != nil {
		t.Fatal(err)
	}
	// The commit is left as written; only the header (the default since the
	// freeze, ako/mxcli#714) is added.
	if got, _ := os.ReadFile(path); string(got) != "mdl 1;\n"+src {
		t.Fatalf("the commit changed without a project:\n%s", got)
	}
	for _, want := range []string{"s.mdl:3: note: M.F:", "MDL067", "-p app.mpr"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not mention %q:\n%s", want, out)
		}
	}
}

// ako/mxcli#876: `fmt --upgrade --header` migrated mxcli-rest's scripts with no
// warning, and exec then refused three of them under the header it had added.
// With -p, a `create or modify` of a stored flow whose change cannot be
// spliced in — refused under mdl 1, rebuilt under mdl 0, and no rewrite says
// "rebuild" — keeps the file off the header, and fmt says which statement and
// why; --force-header adds it anyway. Control: the stored flow restated as it
// is takes the header.
func TestFmtUpgrade_HeaderDeclinedWhereExecWouldRefuse(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "PedApp.mpr")
	const stub = "create or modify microflow MyFirstModule.Grow ($Items: List of System.User)\nreturns String as $Out\nbegin\n  declare $Out String = '';\n  loop $U in $Items\n  begin\n    set $Out = $Out + ',';\n  end loop;\n  return $Out;\nend;\n"
	const grown = "create or modify microflow MyFirstModule.Grow ($Items: List of System.User)\nreturns String as $Out\nbegin\n  declare $Out String = '';\n" +
		"  loop $U in $Items\n  begin\n    set $Out = $Out + ',';\n    if $U/Name != empty then\n      set $Out = $Out + $U/Name;\n    end if;\n  end loop;\n  return $Out;\nend;\n"
	exe := executor.New(io.Discard)
	exe.SetBackendFactory(newBackendFactory())
	prog, errs := visitor.Build(fmt.Sprintf("connect local '%s';\n%s", visitor.QuoteString(mpr), stub))
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if err := exe.ExecuteProgram(prog); err != nil {
		t.Fatalf("store the stub: %v", err)
	}
	_ = exe.Close()

	write := func(script string) string {
		path := filepath.Join(t.TempDir(), "s.mdl")
		if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// Control: the stored flow as it is takes the header.
	path := write(stub)
	if out, err := runFmt(t, "--upgrade", "--header", "-w", "-p", mpr, path); err != nil {
		t.Fatalf("the unchanged flow: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(path); !strings.HasPrefix(string(got), "mdl 1;\n") {
		t.Fatalf("the unchanged flow did not take the header:\n%s", got)
	}

	path = write(grown)
	out, err := runFmt(t, "--upgrade", "--header", "-w", "-p", mpr, path)
	if err != nil {
		t.Fatalf("the grown flow: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(path); string(got) != grown {
		t.Errorf("the header was added to a script exec would then refuse:\n%s", got)
	}
	for _, want := range []string{"no language header added", "MyFirstModule.Grow", "cannot be spliced", "--force-header"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in fmt's report:\n%s", want, out)
		}
	}

	if out, err := runFmt(t, "--upgrade", "--header", "--force-header", "-w", "-p", mpr, path); err != nil {
		t.Fatalf("--force-header: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(path); !strings.HasPrefix(string(got), "mdl 1;\n") {
		t.Errorf("--force-header did not add the header:\n%s", got)
	}
}
