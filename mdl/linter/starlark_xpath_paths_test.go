// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// render walks a parse_xpath tree RECURSIVELY, the way the write-lint-rules
// skill's count_not example does — which failed with "function called
// recursively" before recursion was enabled (ako/mxcli#1439).
const renderXPath = `
def render(n):
    if n.kind == "bin":
        return "(" + render(n.left) + " " + n.op + " " + render(n.right) + ")"
    if n.kind == "attr_path":
        segs = []
        for i, seg in enumerate(n.path):
            p = n.predicates[i]
            segs.append(seg if p.kind == "null" else seg + "[" + render(p) + "]")
        return ("$" + n.variable + "/" if n.variable else "") + "/".join(segs)
    if n.kind == "string":
        return "'" + n.value + "'"
    if n.kind == "variable":
        return n.name
    if n.kind == "qname":
        return n.module + "." + n.name
    if n.kind == "unary":
        return n.op + " " + render(n.operand)
    if n.kind == "paren":
        return "(" + render(n.inner) + ")"
    if n.kind == "call":
        return n.name + "(" + ", ".join([render(a) for a in n.args]) + ")"
    return "<" + n.kind + ">"
`

func lintCtx(t *testing.T) *linter.LintContext {
	t.Helper()
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	return linter.NewLintContext(cat, nil)
}

// "parse_xpath("[Mod.A/Mod.E/Name = 'x' and Mod.A/Mod.E/Code = 'y']") returns
// only qname Mod.A; the comparison and the rest are lost. Plain attributes
// parse ([Name = 'x'] gives bin), association paths do not." (ako/mxcli#1439)
func TestParseXPathAssociationPaths(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		// Control: a plain attribute parsed before the fix too.
		{`[Name = 'x']`, `(Name = 'x')`},
		{`[Mod.A/Mod.E/Name = 'x' and Mod.A/Mod.E/Code = 'y']`,
			`((Mod.A/Mod.E/Name = 'x') AND (Mod.A/Mod.E/Code = 'y'))`},
		{`[Mod.A/Mod.E[Code = 'y']/Name = 'x']`, `(Mod.A/Mod.E[(Code = 'y')]/Name = 'x')`},
		{`[Mod.Order_Customer = $Customer]`, `(Mod.Order_Customer = Customer)`},
		{`[not(Mod.A/Mod.E)]`, `NOT (Mod.A/Mod.E)`},
		{`[$currentObject/Mod.A/Name = 'x']`, `($currentObject/Mod.A/Name = 'x')`},
	} {
		vs, _ := runSrc(t, lintCtx(t), renderXPath+`
def check():
    return [violation(message = render(parse_xpath(`+"\""+escapeStarlark(tc.in)+"\""+`)))]
`)
		if len(vs) != 1 || vs[0].Message != tc.want {
			var got []string
			for _, v := range vs {
				got = append(got, v.Message)
			}
			t.Errorf("parse_xpath(%s) renders %v, want %q", tc.in, got, tc.want)
		}
	}
}

func escapeStarlark(s string) string {
	out := ""
	for _, r := range s {
		if r == '"' || r == '\\' {
			out += "\\"
		}
		out += string(r)
	}
	return out
}

// "matches() returns only true/false, without captures" (ako/mxcli#1439).
// find_all returns every match, each as its capture groups (or the whole match
// when the pattern has none).
func TestFindAllReturnsCaptures(t *testing.T) {
	vs, _ := runSrc(t, lintCtx(t), `
def check():
    a = find_all("ACT_Order_Save, ACT_Line_Delete", "ACT_([A-Za-z]+)_([A-Za-z]+)")
    b = find_all("x1 y22", "[0-9]+")
    c = find_all("none", "[0-9]+")
    return [violation(message = repr(a) + " " + repr(b) + " " + repr(c))]
`)
	want := `[("Order", "Save"), ("Line", "Delete")] ["1", "22"] []`
	if len(vs) != 1 || vs[0].Message != want {
		t.Errorf("find_all = %+v, want %q", vs, want)
	}
}

// TestSkillCountNotExampleRuns runs the write-lint-rules skill's walking
// example as written. It could not run at all (recursion was disabled), and
// it matched `not` as a call although parse_xpath returns a unary NOT — so
// even with recursion it counted nothing (ako/mxcli#1439).
func TestSkillCountNotExampleRuns(t *testing.T) {
	doc, err := os.ReadFile("../../.claude/skills/mendix/write-lint-rules/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	const marker = "Example — count `not(…)` calls in an XPath"
	i := strings.Index(string(doc), marker)
	if i < 0 {
		t.Fatalf("skill has no %q section", marker)
	}
	rest := string(doc)[i:]
	start := strings.Index(rest, "```python\n")
	end := strings.Index(rest[start+10:], "```")
	if start < 0 || end < 0 {
		t.Fatal("no python block after the example heading")
	}
	example := rest[start+10 : start+10+end]

	vs, _ := runSrc(t, lintCtx(t), example+`
def check():
    n = count_not(parse_xpath("[not(Mod.A/Mod.E) and Mod.A/Mod.E[not(Code = 'x')]/Name = 'y' and Active]"))
    return [violation(message = str(n))]
`)
	if len(vs) != 1 || vs[0].Message != "2" {
		t.Errorf("count_not = %+v, want 2", vs)
	}
}

// "parse_xpath silently drops everything after an XPath true() or false()"
// (mendixlabs/mxcli#1894): both branches of the or come back, in either order.
func TestParseXPathBooleanFunctions(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`[IsConfidential = false() or Mod.Report_Owner = '[%CurrentUser%]']`,
			`((IsConfidential = <bool>) OR (Mod.Report_Owner = '[%CurrentUser%]'))`},
		{`[Mod.Report_Owner = '[%CurrentUser%]' or IsConfidential = false()]`,
			`((Mod.Report_Owner = '[%CurrentUser%]') OR (IsConfidential = <bool>))`},
		{`[Active = true() and (Mod.A/Mod.E/Name = 'x')]`,
			`((Active = <bool>) AND ((Mod.A/Mod.E/Name = 'x')))`},
	} {
		vs, _ := runSrc(t, lintCtx(t), renderXPath+`
def check():
    return [violation(message = render(parse_xpath(`+"\""+escapeStarlark(tc.in)+"\""+`)))]
`)
		if len(vs) != 1 || vs[0].Message != tc.want {
			var got []string
			for _, v := range vs {
				got = append(got, v.Message)
			}
			t.Errorf("parse_xpath(%s) renders %v, want %q", tc.in, got, tc.want)
		}
	}
}

// A constraint the parser cannot read to the end comes back as one recovered
// node holding the whole source, as the write-lint-rules skill says, not as a
// partial tree a rule would take for the whole XPath (mendixlabs/mxcli#1894).
func TestParseXPathTrailingTokensAreRecovered(t *testing.T) {
	vs, _ := runSrc(t, lintCtx(t), `
def check():
    n = parse_xpath("[Name = 'x' 'y']")
    return [violation(message = n.kind + "|" + n.source)]
`)
	if len(vs) != 1 || vs[0].Message != "recovered|Name = 'x' 'y'" {
		var got []string
		for _, v := range vs {
			got = append(got, v.Message)
		}
		t.Errorf("parse_xpath with trailing tokens gives %v, want a recovered root with the whole source", got)
	}
}
