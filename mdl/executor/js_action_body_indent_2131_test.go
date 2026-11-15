// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// ako/mxcli#2131 (mendixlabs/mxcli#1812): a `$$ … $$` body indented one tab
// throughout was written with one extra tab on every line after the first, and
// describe printed the extra tab back. The parser trimmed only the first line's
// indentation, and the writer indents every line by a tab.
const indentedJSBody = "\tconst a = 3;\n\t\tif (a === 1) {\n\t\t\treturn true;\n\t\t}\n\t\treturn false;"

func parseJSActionBody(t *testing.T, body string) string {
	t.Helper()
	src := "mdl 1;\ncreate or modify javascript action BugRepro.JS_Fresh()\n  returns Boolean as $ReturnValueName\n  platform Web\nas $$\n" + body + "\n$$;"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("does not parse: %v", errs)
	}
	return prog.Statements[0].(*ast.CreateJavaScriptActionStmt).JavaScriptCode
}

// userCodeTabs returns the leading tab count of each user-code line of a .js file.
func userCodeTabs(source string) []int {
	uc, _ := sliceBetweenFold(source, "// BEGIN USER CODE", "// END USER CODE")
	var tabs []int
	for _, l := range strings.Split(strings.Trim(uc, "\n"), "\n") {
		if strings.TrimSpace(l) == "" || strings.Contains(l, "END USER CODE") {
			continue
		}
		tabs = append(tabs, len(l)-len(strings.TrimLeft(l, "\t")))
	}
	return tabs
}

func TestJavaScriptActionBody_KeepsItsIndentation_Issue2131(t *testing.T) {
	code := parseJSActionBody(t, indentedJSBody)
	source := javaactions.GenerateJavaScriptSource("JS_Fresh", code, nil, nil, nil, "")

	// The body as written: 1, 2, 3, 2, 2 tabs.
	if got, want := userCodeTabs(source), []int{1, 2, 3, 2, 2}; !equalInts(got, want) {
		t.Errorf("user code tabs = %v, want %v\n%s", got, want, source)
	}

	// describe reads back what exec parses again: a fixed point.
	dir := t.TempDir()
	jsDir := filepath.Join(dir, "javascriptsource", "BugRepro", "actions")
	if err := os.MkdirAll(jsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jsDir, "JS_Fresh.js"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	described, _ := readJavaScriptActionSource(filepath.Join(dir, "App.mpr"), "BugRepro", "JS_Fresh")
	if again := parseJSActionBody(t, described); again != code {
		t.Errorf("describe → exec changed the body:\nfirst  %q\nsecond %q", code, again)
	}
	if again := javaactions.GenerateJavaScriptSource("JS_Fresh", parseJSActionBody(t, described), nil, nil, nil, ""); again != source {
		t.Errorf("a second exec writes a different file:\n--- first\n%s\n--- second\n%s", source, again)
	}
}

// CONTROL: a body written flush left, the common case, is unchanged.
func TestJavaScriptActionBody_FlushLeftUnchanged_Issue2131(t *testing.T) {
	if got := parseJSActionBody(t, "return Promise.resolve(true);"); got != "return Promise.resolve(true);" {
		t.Errorf("code = %q", got)
	}
	if got := parseJSActionBody(t, "if (x) {\n\treturn 1;\n}"); got != "if (x) {\n\treturn 1;\n}" {
		t.Errorf("code = %q", got)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
