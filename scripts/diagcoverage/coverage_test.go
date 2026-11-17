// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogCodes(t *testing.T) {
	// Codes sit in the middle of unrelated bytes, repeat (one PO per locale) and one
	// near-miss is not a code.
	dll := []byte("MZ\x00\x01msgctxt \"CE0149\"\x00\xffCW0007 and CE0149 again\x00CI0001\x00CE12 CX0001 ce0001 CE00012")
	got := CatalogCodes(dll)
	want := []string{"CE0001", "CE0149", "CI0001", "CW0007"}
	// CE00012 holds CE0001 as a prefix: a code is four digits, and what follows is not
	// checked, which is why the catalog is read from PO text and not from arbitrary data.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CatalogCodes = %v, want %v", got, want)
	}
}

func TestStringLiteralCodes(t *testing.T) {
	src := "package p\n" +
		"// CE0001 in a line comment\n" +
		"/* CE0002 in a block comment */\n" +
		"var a = \"fails CE0003 at build\"\n" +
		"var b = `raw CE0004 and CW0005`\n" +
		"func f() { _ = \"x\" + \"CE0006\" } // trailing CE0007\n" +
		"const CE0008 = 1\n"
	got := stringLiteralCodes("p.go", []byte(src))
	want := []string{"CE0003", "CE0004", "CW0005", "CE0006"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stringLiteralCodes = %v, want %v (comments and identifiers are not diagnostics)", got, want)
	}
}

func TestSourceCodes(t *testing.T) {
	repo := t.TempDir()
	write(t, filepath.Join(repo, "mdl/executor/a.go"), "package executor\nvar m = \"CE0111 duplicate\"\n")
	write(t, filepath.Join(repo, "mdl/executor/a_test.go"), "package executor\nvar m = \"CE0222 only in a test\"\n")
	write(t, filepath.Join(repo, "mdl/executor/b.go"), "package executor\n// CE0333 is only a note\nvar x = 1\n")
	write(t, filepath.Join(repo, "cmd/mxcli/lint-rules/r.star"), "# rule\nMSG = 'CE0444 something'\n")
	write(t, filepath.Join(repo, "mdl/testdata/skip.go"), "package p\nvar m = \"CE0555\"\n")
	write(t, filepath.Join(repo, "docs/not-a-root.go"), "package p\nvar m = \"CE0666\"\n")
	for _, f := range []string{"c1", "c2", "c3", "c4", "c5"} { // more files than are remembered
		write(t, filepath.Join(repo, "sdk", f+".go"), "package sdk\nvar m = \"CE0777\"\n")
	}
	got, err := SourceCodes(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"CE0111", "CE0444", "CE0777"} {
		if len(got[c]) == 0 {
			t.Errorf("%s is named by the source and was not found", c)
		}
	}
	for _, c := range []string{"CE0222", "CE0333", "CE0555", "CE0666"} {
		if len(got[c]) != 0 {
			t.Errorf("%s must not count: test file, comment, testdata and non-root files are not read (%v)", c, got[c])
		}
	}
	if n := len(got["CE0777"]); n != maxFilesPerCode {
		t.Errorf("CE0777 remembers %d files, want the cap of %d", n, maxFilesPerCode)
	}
}

func TestFindingCounts(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a/one.json"), `{"ce": ["CE0117", "CE0117 again", "CE1613"], "symptom": "x"}`+"\n")
	write(t, filepath.Join(dir, "a/two.json"), `{"ce": ["CE0117"]}`+"\n")
	write(t, filepath.Join(dir, "b/old.jsonl"), `{"ce": ["CE0001"]}`+"\n"+`{"ce": ["CE0001", "CE0002"]}`+"\n")
	write(t, filepath.Join(dir, "b/none.json"), `{"ce": []}`+"\n")
	write(t, filepath.Join(dir, "b/bad.json"), `not json`+"\n")
	write(t, filepath.Join(dir, "README.md"), "CE9999 is not a record\n")
	counts, files, err := FindingCounts(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"CE0117": 2, "CE1613": 1, "CE0001": 2, "CE0002": 1}
	if !reflect.DeepEqual(counts, want) {
		t.Errorf("counts = %v, want %v (a record names a code once however often it repeats)", counts, want)
	}
	if files != 5 {
		t.Errorf("files read = %d, want 5 (.json and .jsonl only)", files)
	}
}

func TestLoadCoveredAs(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.tsv")
	write(t, good, "# comment\n\nCE0067\tMDL003\tmissing return\nCE0844\tE009\n")
	got, err := LoadCoveredAs(good)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"CE0067": "MDL003", "CE0844": "E009"}; !reflect.DeepEqual(got, want) {
		t.Errorf("LoadCoveredAs = %v, want %v", got, want)
	}
	bad := filepath.Join(dir, "bad.tsv")
	write(t, bad, "CE67\tMDL003\n")
	if _, err := LoadCoveredAs(bad); err == nil || !strings.Contains(err.Error(), "bad.tsv:1") {
		t.Errorf("a malformed line must be refused with its location, got %v", err)
	}
	missing, err := LoadCoveredAs(filepath.Join(dir, "nope.tsv"))
	if err != nil || len(missing) != 0 {
		t.Errorf("a missing file is an empty mapping, got %v, %v", missing, err)
	}
}

func TestBuild(t *testing.T) {
	catalog := []string{"CE0001", "CE0002", "CE0003", "CE0004", "CE0005", "CW0006", "CI0007"}
	named := map[string][]string{"CE0001": {"a.go"}, "CE9999": {"b.go"}} // CE9999: not in the catalog
	covered := map[string]string{"CE0002": "MDL003"}
	findings := map[string]int{
		"CE0001": 50,                          // named: not a gap however often it was hit
		"CE0002": 40,                          // covered-as: not a gap
		"CE0004": 5, "CE0003": 5, "CE0005": 9, // gaps: by count, ties by code
		"CE8888": 3, // not in the catalog
	}
	r := Build(catalog, named, covered, findings, 7, "11.16.1")
	if r.Catalog != 7 || r.Named != 1 || r.CoveredAs != 1 || r.NotNamed != 5 {
		t.Errorf("counts = catalog %d named %d covered %d not-named %d", r.Catalog, r.Named, r.CoveredAs, r.NotNamed)
	}
	if r.ByPrefix["CE"] != 5 || r.ByPrefix["CW"] != 1 || r.ByPrefix["CI"] != 1 {
		t.Errorf("ByPrefix = %v", r.ByPrefix)
	}
	want := []Gap{{"CE0005", 9}, {"CE0003", 5}, {"CE0004", 5}}
	if !reflect.DeepEqual(r.Gaps, want) {
		t.Errorf("Gaps = %v, want %v", r.Gaps, want)
	}
	if !reflect.DeepEqual(r.NamedNotIn, []string{"CE9999"}) || !reflect.DeepEqual(r.FindingsOff, []string{"CE8888"}) {
		t.Errorf("not-in-catalog = %v / %v", r.NamedNotIn, r.FindingsOff)
	}
	if r.Status["CE0001"] != StatusNamed || r.Status["CE0002"] != StatusCoveredAs || r.Status["CE0003"] != StatusNotNamed {
		t.Errorf("Status = %v", r.Status)
	}
}

func TestVersionLessIsNumeric(t *testing.T) {
	// Lexically "11.9.0" > "11.16.1"; as versions it is older.
	for _, c := range []struct {
		a, b string
		less bool
	}{{"11.9.0", "11.16.1", true}, {"11.16.1", "11.9.0", false}, {"11.14.0", "11.14.0", false}, {"10.24.0", "11.0.0", true}, {"11.1", "11.1.1", true}} {
		if got := versionLess(c.a, c.b); got != c.less {
			t.Errorf("versionLess(%q, %q) = %v, want %v", c.a, c.b, got, c.less)
		}
	}
}

func fakeHome(t *testing.T, versions ...string) string {
	t.Helper()
	home := t.TempDir()
	for _, v := range versions {
		write(t, filepath.Join(home, ".mxcli/mxbuild", v, "modeler", textsDLL), "MZ CE0001 CE0002 CW0003\n")
	}
	return home
}

func TestResolveDLL(t *testing.T) {
	home := fakeHome(t, "11.9.0", "11.16.1", "11.14.0")
	p, v, err := ResolveDLL("", "", home)
	if err != nil || v != "11.16.1" || !strings.Contains(p, "11.16.1") {
		t.Errorf("newest = %q %q %v, want 11.16.1", p, v, err)
	}
	if _, v, err := ResolveDLL("", "11.14.0", home); err != nil || v != "11.14.0" {
		t.Errorf("explicit version = %q %v", v, err)
	}
	if _, _, err := ResolveDLL("", "10.1.0", home); err == nil || !strings.Contains(err.Error(), "mxcli setup mxbuild") {
		t.Errorf("a missing version must say how to install it, got %v", err)
	}
	if _, _, err := ResolveDLL("", "", t.TempDir()); err == nil || !strings.Contains(err.Error(), "mxcli setup mxbuild") {
		t.Errorf("no MxBuild at all must say how to install one, got %v", err)
	}
	explicit := filepath.Join(home, ".mxcli/mxbuild/11.9.0/modeler", textsDLL)
	if p, v, err := ResolveDLL(explicit, "", home); err != nil || p != explicit || v != "11.9.0" {
		t.Errorf("explicit --dll = %q %q %v", p, v, err)
	}
}

// fakeRepo is a repository root with one named code, one finding-only gap and one
// covered-as code, against a DLL that holds those and an unrelated one.
func fakeRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	write(t, filepath.Join(repo, "go.mod"), "module x\n")
	write(t, filepath.Join(repo, "mdl/a.go"), "package mdl\nvar m = \"CE0001\"\n")
	write(t, filepath.Join(repo, ".claude/skills/fix-issue/findings/a/f.json"), `{"ce":["CE0002","CE0001"]}`+"\n")
	write(t, filepath.Join(repo, "scripts/diagcoverage/covered-as.tsv"), "CE0003\tMDL003\tnote\n")
	return repo
}

func TestRunReport(t *testing.T) {
	testutil.SetHome(t, fakeHome(t, "11.16.1"))
	repo := fakeRepo(t)
	var out, errOut bytes.Buffer
	if err := run([]string{"--repo", repo}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"MxBuild 11.16.1", "catalog", "3 codes", "CE0002", "named is a proxy"} {
		if !strings.Contains(text, want) {
			t.Errorf("report lacks %q:\n%s", want, text)
		}
	}
	// CE0001 is named and CE0003 covered-as, so neither is a gap; CE0002 is.
	gaps := text[strings.Index(text, "by how many records"):]
	if strings.Contains(gaps, "CE0001  ") || strings.Contains(gaps, "CE0003  ") || !strings.Contains(gaps, "CE0002") {
		t.Errorf("gap list wrong:\n%s", gaps)
	}
}

func TestRunAllAndJSON(t *testing.T) {
	testutil.SetHome(t, fakeHome(t, "11.16.1"))
	repo := fakeRepo(t)
	var out bytes.Buffer
	if err := run([]string{"--repo", repo, "--all"}, &out, os.Stderr); err != nil {
		t.Fatal(err)
	}
	want := "code\tstatus\tfindings\nCE0001\tnamed\t1\nCE0002\tnot-named\t1\nCW0003\tnot-named\t0\n"
	// CW0003 is in the fake DLL; CE0003 is not, so it is not listed.
	if out.String() != want {
		t.Errorf("--all =\n%q\nwant\n%q", out.String(), want)
	}
	out.Reset()
	if err := run([]string{"--repo", repo, "--json"}, &out, os.Stderr); err != nil {
		t.Fatal(err)
	}
	var r Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("--json is not JSON: %v", err)
	}
	if r.Catalog != 3 || r.Named != 1 || len(r.Gaps) != 1 || r.Gaps[0].Code != "CE0002" {
		t.Errorf("json report = %+v", r)
	}
}

func TestRunRefusesANonRepo(t *testing.T) {
	testutil.SetHome(t, fakeHome(t, "11.16.1"))
	err := run([]string{"--repo", t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("a directory that is not the repository must be refused, got %v", err)
	}
}

// The real repository, as a smoke test: the scan finds a sensible number of codes and
// does not fail on any file in it. The exact number is not asserted, because it
// changes with every check anyone adds.
func TestSourceCodesOnThisRepository(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skip("not run from inside the repository")
	}
	got, err := SourceCodes(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 100 {
		t.Errorf("only %d codes named by this repository's source; the scan is broken or the checks went", len(got))
	}
	for _, c := range []string{"CE0111", "CE0117"} { // long-standing checks
		if len(got[c]) == 0 {
			t.Errorf("%s is not found in this repository's source", c)
		}
	}
}
