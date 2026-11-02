// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/upgrade"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// A test file may start with the `mdl <n>;` language header (ako/mxcli#847).
// These tests pin the four places that have to read it: the parser (which
// tests there are, and what version each is written in), CheckSource (what
// check and the LSP parse), the two generators (what the runner executes),
// and fmt --upgrade (which adds it).

const headeredTestFile = `mdl 1;

/**
 * @test first
 * @expect $n = 1
 */
DECLARE $n Integer = 1;
/

/**
 * @test second
 */
DECLARE $ok Boolean = true;
/
`

// The header used to be body text in the first chunk, so the first test's doc
// comment was no longer leading and the chunk was skipped with no message:
// `mxcli test --list` found 41 of 42 tests in mxcli-ledger's suite.
func TestHeaderDoesNotDropTheFirstTest(t *testing.T) {
	tests, err := parseMDLTests(headeredTestFile, "h.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 2 {
		t.Fatalf("found %d test(s), want 2 — the test behind the header was dropped", len(tests))
	}
	for _, tc := range tests {
		if tc.Version != langver.V1 {
			t.Errorf("test %q is mdl %d, want mdl 1 from the file header", tc.Name, tc.Version)
		}
	}
	if tests[0].Name != "first" || tests[0].BodyLine != 7 {
		t.Errorf("first test = %q at body line %d, want %q at 7", tests[0].Name, tests[0].BodyLine, "first")
	}
}

func TestHeaderlessTestFileIsMdl0(t *testing.T) {
	tests, err := parseMDLTests("/** @test a */\nDECLARE $x Integer = 1;\n/\n", "a.test.mdl")
	if err != nil || len(tests) != 1 {
		t.Fatalf("parseMDLTests = %d tests, %v", len(tests), err)
	}
	if tests[0].Version != langver.V0 || tests[0].HeaderLine != 0 {
		t.Errorf("headerless test is mdl %d (header line %d), want mdl 0 with none", tests[0].Version, tests[0].HeaderLine)
	}
}

// A version this mxcli does not know is refused by name. Leaving it in the
// text would drop the first test again, silently.
func TestUnknownHeaderVersionIsRefused(t *testing.T) {
	_, err := parseMDLTests(strings.Replace(headeredTestFile, "mdl 1;", "mdl 9;", 1), "h.test.mdl")
	if err == nil || !strings.Contains(err.Error(), "unknown MDL language version 9") {
		t.Fatalf("err = %v, want the unknown-version refusal", err)
	}
}

// A file-level @setup header comment still works behind a language header.
func TestHeaderBeforeFileSetupComment(t *testing.T) {
	src := "mdl 1;\n/** @setup M.Seed */\n/** @test a */\nDECLARE $x Integer = 1;\n/\n"
	tests, err := parseMDLTests(src, "a.test.mdl")
	if err != nil || len(tests) != 1 {
		t.Fatalf("parseMDLTests = %d tests, %v", len(tests), err)
	}
	if len(tests[0].Setups) != 1 || tests[0].Setups[0] != "M.Seed" {
		t.Errorf("setups = %v, want [M.Seed]", tests[0].Setups)
	}
}

// check ignored the header: CheckSource rendered only the bodies, so the file
// was checked as mdl 0 and a header-gated construct warned as if it had none.
func TestCheckSourceHonoursTheHeader(t *testing.T) {
	got, err := CheckSource(headeredTestFile, "h.test.mdl")
	if err != nil {
		t.Fatalf("CheckSource: %v", err)
	}
	prog, errs := visitor.Build(got.MDL)
	if len(errs) > 0 {
		t.Fatalf("rendering does not parse: %v\n--- rendered ---\n%s", errs, got.MDL)
	}
	if prog.LanguageVersion != langver.V1 {
		t.Errorf("rendering parses as mdl %d, want mdl 1\n--- rendered ---\n%s", prog.LanguageVersion, got.MDL)
	}
	if len(prog.Statements) != 2 {
		t.Errorf("rendering has %d statements, want 2", len(prog.Statements))
	}
	for _, n := range prog.LanguageNotes {
		t.Errorf("rendering carries %s at line %d: %s", n.Code, n.Line, n.Message)
	}
}

// The rendering puts no `/` on a separator line, under either version: under
// mdl 1 it is refused (MDL-V1-SLASH), and under mdl 0 it was warned about on a
// separator that is the test format's, not an MDL terminator the author wrote.
func TestCheckSourceWritesNoSlash(t *testing.T) {
	for _, src := range []string{headeredTestFile, strings.TrimPrefix(headeredTestFile, "mdl 1;\n")} {
		got, err := CheckSource(src, "h.test.mdl")
		if err != nil {
			t.Fatalf("CheckSource: %v", err)
		}
		for i, line := range strings.Split(got.MDL, "\n") {
			if strings.HasSuffix(strings.TrimSpace(line), "/") {
				t.Errorf("rendered line %d ends in '/': %q", i+1, line)
			}
		}
		prog, errs := visitor.Build(got.MDL)
		if len(errs) > 0 {
			t.Fatalf("rendering does not parse: %v\n%s", errs, got.MDL)
		}
		for _, n := range prog.LanguageNotes {
			if n.Code == "MDL-V1-SLASH" {
				t.Errorf("rendering warns %s at line %d on a separator the author did not write as a terminator", n.Code, n.Line)
			}
		}
	}
}

// Both generators wrote `/` after every microflow, which mdl 1 refuses. Under a
// headered suite they must emit the header and a script mdl 1 accepts.
func TestGeneratorsHonourTheHeader(t *testing.T) {
	tests, err := parseMDLTests(headeredTestFile, "h.test.mdl")
	if err != nil {
		t.Fatal(err)
	}
	suite := &TestSuite{Name: "h", Tests: tests}
	for name, script := range map[string]string{
		"GenerateTestRunner": GenerateTestRunner(suite),
		"GenerateTestFlows":  GenerateTestFlows(suite),
	} {
		if !strings.HasPrefix(script, "mdl 1;\n") {
			t.Errorf("%s does not start with the header:\n%s", name, script)
		}
		for i, line := range strings.Split(script, "\n") {
			if strings.TrimSpace(line) == "/" {
				t.Errorf("%s line %d is '/', which mdl 1 refuses", name, i+1)
			}
		}
		prog, errs := visitor.Build(script)
		if len(errs) > 0 {
			t.Errorf("%s output does not parse: %v\n%s", name, errs, script)
			continue
		}
		if prog.LanguageVersion != langver.V1 {
			t.Errorf("%s output parses as mdl %d", name, prog.LanguageVersion)
		}
		for _, d := range prog.Deprecations {
			t.Errorf("%s writes the deprecated spelling %s at line %d", name, d.Code, d.Line)
		}
	}
}

// A headerless suite's scripts get no header, and draw nothing about text the
// runner wrote itself. They used to end every microflow with a `/` line and
// open it with `create or replace`, so `mxcli test` printed an MDL-V1-SLASH and
// an MDL-DEPR001 per test — 104 warnings for a 52-test suite — about a script
// the author never saw (ako/mxcli#1855). The bodies here are valid under both
// versions, so any note is the generator's.
func TestGeneratorsHeaderlessSuiteDrawsNoNotes(t *testing.T) {
	tests, err := parseMDLTests(strings.TrimPrefix(headeredTestFile, "mdl 1;\n"), "h.test.mdl")
	if err != nil {
		t.Fatal(err)
	}
	suite := &TestSuite{Name: "h", Tests: tests}
	for name, script := range map[string]string{
		"GenerateTestRunner": GenerateTestRunner(suite),
		"GenerateTestFlows":  GenerateTestFlows(suite),
	} {
		if strings.HasPrefix(script, "mdl ") {
			t.Errorf("%s: headerless suite got a header:\n%s", name, script)
		}
		prog, errs := visitor.Build(script)
		if len(errs) > 0 {
			t.Errorf("%s output does not parse: %v\n%s", name, errs, script)
			continue
		}
		if prog.LanguageVersion != langver.V0 {
			t.Errorf("%s output parses as mdl %d, want mdl 0", name, prog.LanguageVersion)
		}
		for _, n := range prog.LanguageNotes {
			t.Errorf("%s draws %s at line %d: %s", name, n.Code, n.Line, n.Message)
		}
		for _, d := range prog.Deprecations {
			t.Errorf("%s writes the deprecated spelling %s at line %d", name, d.Code, d.Line)
		}
	}
}

// A suite is compiled into one script, and a script has one version. A
// directory that mixes them is refused, naming the files.
func TestMixedVersionSuiteIsRefused(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.test.mdl", headeredTestFile)
	write("b.test.mdl", "/** @test old */\nDECLARE $x Integer = 1;\n/\n")

	_, err := parseTestFiles([]string{dir})
	if err == nil {
		t.Fatal("a suite mixing mdl 0 and mdl 1 files was accepted")
	}
	for _, want := range []string{"a.test.mdl", "b.test.mdl", "mdl 0", "mdl 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}

	// The same files, one version, are a suite.
	write("b.test.mdl", "mdl 1;\n/** @test new */\nDECLARE $x Integer = 1;\n/\n")
	suite, err := parseTestFiles([]string{dir})
	if err != nil {
		t.Fatalf("a single-version suite was refused: %v", err)
	}
	if len(suite.Tests) != 3 {
		t.Errorf("suite has %d tests, want 3", len(suite.Tests))
	}
}

// A markdown test block may open with the header; the blocks of one file must
// agree, because check renders the file as one script.
func TestMarkdownBlockHeader(t *testing.T) {
	md := "# Spec\n\n```mdl-test\nmdl 1;\n/** @test a */\nDECLARE $x Integer = 1;\n```\n\n```mdl-test\nmdl 1;\n/** @test b */\nDECLARE $y Integer = 2;\n```\n"
	tests, err := parseMarkdownTests(md, "s.test.md")
	if err != nil || len(tests) != 2 {
		t.Fatalf("parseMarkdownTests = %d, %v", len(tests), err)
	}
	for _, tc := range tests {
		if tc.Version != langver.V1 || strings.HasPrefix(tc.Name, "test at line") {
			t.Errorf("test %q: mdl %d", tc.Name, tc.Version)
		}
	}
	got, err := CheckSource(md, "s.test.md")
	if err != nil {
		t.Fatalf("CheckSource: %v", err)
	}
	prog, errs := visitor.Build(got.MDL)
	if len(errs) > 0 || prog.LanguageVersion != langver.V1 || len(prog.Statements) != 2 {
		t.Fatalf("rendering: %v\n%s", errs, got.MDL)
	}

	mixed := strings.Replace(md, "```mdl-test\nmdl 1;\n/** @test b", "```mdl-test\n/** @test b", 1)
	if _, err := CheckSource(mixed, "s.test.md"); err == nil {
		t.Error("a markdown file mixing mdl 0 and mdl 1 blocks was accepted")
	}
}

// fmt --upgrade adds the header to a test file now that the runner reads it,
// and --header=false still declines it.
func TestUpgradeSourceAddsTheHeader(t *testing.T) {
	src := "-- suite\n/**\n * @test limit\n */\nRETRIEVE $o FROM M.E LIMIT 1;\n/\n"
	res, err := UpgradeSource(src, "a.test.mdl", upgrade.Options{AddHeader: true})
	if err != nil {
		t.Fatalf("UpgradeSource: %v", err)
	}
	if !res.HeaderAdded || !strings.HasPrefix(res.Source, "mdl 1;\n") {
		t.Fatalf("header not added:\n%s", res.Source)
	}
	tests, err := parseMDLTests(res.Source, "a.test.mdl")
	if err != nil || len(tests) != 1 || tests[0].Version != langver.V1 {
		t.Fatalf("upgraded file: %d tests, %v", len(tests), err)
	}
	// The gated rewrite keeps the old meaning under the new header: limit 1
	// was the object, which mdl 1 spells `first`.
	if !strings.Contains(strings.ToLower(tests[0].MDL), "first") {
		t.Errorf("the header-gated `limit 1` was not rewritten:\n%s", res.Source)
	}
	// Idempotent: a second run changes nothing.
	again, err := UpgradeSource(res.Source, "a.test.mdl", upgrade.Options{AddHeader: true})
	if err != nil || again.Source != res.Source {
		t.Errorf("second upgrade: %v\n%s", err, again.Source)
	}

	declined, err := UpgradeSource(src, "a.test.mdl", upgrade.Options{AddHeader: false})
	if err != nil || declined.HeaderAdded || strings.HasPrefix(declined.Source, "mdl") {
		t.Errorf("--header=false added a header: %v\n%s", err, declined.Source)
	}
}

func TestUpgradeSourceAddsTheHeaderToMarkdownBlocks(t *testing.T) {
	md := "# Spec\n\n```mdl-test\n/** @test a */\nDECLARE $x Integer = 1;\n```\n\nprose\n\n```mdl-test\n/** @test b */\nDECLARE $y Integer = 2;\n```\n"
	res, err := UpgradeSource(md, "s.test.md", upgrade.Options{AddHeader: true})
	if err != nil {
		t.Fatalf("UpgradeSource: %v", err)
	}
	if n := strings.Count(res.Source, "```mdl-test\nmdl 1;\n"); n != 2 {
		t.Fatalf("header added to %d of 2 blocks:\n%s", n, res.Source)
	}
	tests, err := parseMarkdownTests(res.Source, "s.test.md")
	if err != nil || len(tests) != 2 {
		t.Fatalf("upgraded markdown: %d tests, %v", len(tests), err)
	}
	for _, tc := range tests {
		if tc.Version != langver.V1 {
			t.Errorf("test %q is mdl %d after the upgrade", tc.Name, tc.Version)
		}
	}
}

// The header-gated rewrite of a backslash escape keeps the old meaning under
// mdl 1 by writing the character itself — for `\n`, a real line break — so
// the rewrite adds a line. Mapping the upgrade back line for line refused
// that whole file; the body line it replaces is the author's, so it maps.
func TestUpgradeSourceRewriteThatAddsALine(t *testing.T) {
	src := "/** @test esc\n * @expect $len = 3\n */\ndeclare $s String = 'a\\nb';\ndeclare $len Integer = length($s);\n/\n\n/** @test after */\ndeclare $x Integer = 1;\n/\n"
	res, err := UpgradeSource(src, "e.test.mdl", upgrade.Options{AddHeader: true})
	if err != nil {
		t.Fatalf("UpgradeSource: %v", err)
	}
	want := "mdl 1;\n/** @test esc\n * @expect $len = 3\n */\ndeclare $s String = 'a\nb';\ndeclare $len Integer = length($s);\n/\n\n/** @test after */\ndeclare $x Integer = 1;\n/\n"
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	tests, err := parseMDLTests(res.Source, "e.test.mdl")
	if err != nil || len(tests) != 2 || tests[1].Name != "after" {
		t.Fatalf("upgraded file: %d tests, %v", len(tests), err)
	}
}

// The diff-based mapping still refuses a rewrite that reaches a line that is
// not the author's statement: a wrapper fragment, or a doc comment's slot.
func TestMapUpgradeBackRefusesNonBodyLines(t *testing.T) {
	orig := []string{"/** @test a */", "declare $x Integer = 1;", "/", ""}
	rendered := []string{"CREATE OR MODIFY MICROFLOW MxTest.Check_a () BEGIN", "declare $x Integer = 1;", "END;", "", ""}

	ok := []string{rendered[0], "declare $x Integer =", "  1;", "END;", "", ""}
	if got, err := mapUpgradeBack(orig, rendered, ok); err != nil || len(got) != 5 || got[0] != orig[0] || got[3] != "/" {
		t.Errorf("a body rewrite adding a line: %v, %q", err, got)
	}
	for name, upgraded := range map[string][]string{
		"wrapper header": {"CREATE OR MODIFY MICROFLOW MxTest.Other () BEGIN", rendered[1], "END;", "", ""},
		"wrapper end":    {rendered[0], rendered[1], "END; -- x", "", ""},
		"spare line":     {rendered[0], rendered[1], "END;", "", "x"},
	} {
		if _, err := mapUpgradeBack(orig, rendered, upgraded); err == nil {
			t.Errorf("%s: a rewrite outside the statements was mapped back", name)
		}
	}
}

// The generators indented every body line, including the continuation of a
// string literal that spans lines — and under mdl 1 that is how a line break
// in a string is written (`fmt --upgrade` rewrites mdl 0's `\n` to it). The
// indent became part of the value: an upgraded test asserting length 3 saw 5
// at runtime. A line that starts inside a literal is written as it is.
func TestGeneratorsKeepMultiLineStringLiterals(t *testing.T) {
	src := "mdl 1;\n/** @test esc\n * @expect $len = 3\n */\ndeclare $s String = 'a\nb'; -- it's a comment\ndeclare $len Integer = length($s);\n/\n"
	tests, err := parseMDLTests(src, "e.test.mdl")
	if err != nil {
		t.Fatal(err)
	}
	suite := &TestSuite{Name: "e", Tests: tests}
	runner := GenerateTestRunner(suite)
	flows := GenerateTestFlows(suite)
	for name, script := range map[string]string{"GenerateTestRunner": runner, "GenerateTestFlows": flows} {
		if !strings.Contains(script, "'a\nb';") {
			t.Errorf("%s changed the string literal:\n%s", name, script)
		}
		// The line after the literal is indented again: the `'` in the
		// comment did not open a string.
		if !strings.Contains(script, "\n  declare $len") && !strings.Contains(script, "\n  declare $len_1") {
			t.Errorf("%s: the line after the literal lost its indent:\n%s", name, script)
		}
	}
	throws := strings.Replace(src, " * @expect $len = 3", " * @throws", 1)
	tests, err = parseMDLTests(throws, "e.test.mdl")
	if err != nil {
		t.Fatal(err)
	}
	suite = &TestSuite{Name: "e", Tests: tests}
	for name, script := range map[string]string{"GenerateTestRunner": GenerateTestRunner(suite), "GenerateTestFlows": GenerateTestFlows(suite)} {
		if !strings.Contains(script, "'a\nb';") {
			t.Errorf("%s (@throws) changed the string literal:\n%s", name, script)
		}
	}
}
