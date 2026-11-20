// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// ako/mxcli#1003: describe wrapped long text in `$$ … $$`, and the lexer ends a
// DOLLAR_STRING at the first `$$` it meets — it has no escape. A text holding
// `$$` (a Teamcenter UID in a JSON sample, a Java string literal) ended the
// literal early and `mxcli check` on the output reported
// `token recognition error at: '$$`. Every describer that emits `$$` now goes
// through mdlDollarQuote / mdlSourceBlock, which fall back to a string literal.
// Each case has a control without `$$`, which must keep the dollar form.

func TestDescribeJavaAction_SourceContainingDollarDollar(t *testing.T) {
	for _, c := range []struct {
		name, body, extra string
		dollarForm        bool // the body is written as `as $$ … $$`
	}{
		{"control", `return "a" + N;`, "private static final String S = \"x\";", true},
		{"body holds $$", `return "SR::N::10.7$$Otk" + N;`, "private static final String S = \"x\";", false},
		{"extra code holds $$", `return "a" + N;`, "private static final String S = \"C:\\\\x$$y\";", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			ja := &javaactions.JavaAction{
				Name:       "JA_Uid",
				ReturnType: &javaactions.StringType{},
				Parameters: []*javaactions.JavaActionParameter{
					{Name: "N", IsRequired: true, ParameterType: &javaactions.StringType{}},
				},
			}
			ja.ID = nextID("ja")
			dir := t.TempDir()
			mprPath := filepath.Join(dir, "App.mpr")
			source := javaactions.GenerateSource("ModJ", "JA_Uid", c.body, ja.Parameters, ja.ReturnType,
				[]string{
					"import com.mendix.systemwideinterfaces.core.IContext;",
					"import com.mendix.systemwideinterfaces.core.UserAction;",
				}, c.extra, "")
			javaDir := filepath.Join(dir, "javasource", "modj", "actions")
			if err := os.MkdirAll(javaDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(javaDir, "JA_Uid.java"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			mb := &mock.MockBackend{
				IsConnectedFunc:          func() bool { return true },
				ReadJavaActionByNameFunc: func(string) (*javaactions.JavaAction, error) { return ja, nil },
			}
			ctx, buf := newMockCtx(t, withBackend(mb), withMprPath(mprPath))
			assertNoError(t, describeJavaAction(ctx, ast.QualifiedName{Module: "ModJ", Name: "JA_Uid"}))
			out := buf.String()
			if got := strings.Contains(out, "\nas $$\n"); got != c.dollarForm {
				t.Errorf("dollar-quoted body = %v, want %v:\n%s", got, c.dollarForm, out)
			}
			s := findStmt[*ast.CreateJavaActionStmt](t, reparse(t, describedSource(ctx, out)), out)
			if s.JavaCode != c.body {
				t.Errorf("body read back as %q, want %q\n%s", s.JavaCode, c.body, out)
			}
			if s.ExtraCode != c.extra {
				t.Errorf("extra code read back as %q, want %q\n%s", s.ExtraCode, c.extra, out)
			}
		})
	}
}

// The other positions describe writes `$$` at: an agent's prompt, a data
// transformer's JSLT step, a JavaScript action's body. Each is checked by
// parsing the helper's output where describe puts it.
func TestDollarQuoteHelpers_ReparseAtEveryPosition(t *testing.T) {
	ctx, _ := newMockCtx(t)
	for _, text := range []string{
		"line one\nline two",       // control
		"uid 10.7$$Otklm9\nit's",   // holds $$
		"costs 5$",                 // ends in $
		"C:\\temp$$x\nsecond line", // backslash: the string fallback must keep it
	} {
		header := describedSource(ctx, "")

		prog, errs := visitor.Build(header + "create agent M.A (UsageType: Task, SystemPrompt: " + mdlDollarQuote(ctx, text) + ");")
		if len(errs) > 0 {
			t.Fatalf("agent prompt %q does not parse: %v", text, errs)
		}
		if a := prog.Statements[len(prog.Statements)-1].(*ast.CreateAgentStmt); a.SystemPrompt != text {
			t.Errorf("agent prompt read back as %q, want %q", a.SystemPrompt, text)
		}

		js := header + "create javascript action M.J() returns Boolean as " + mdlSourceBlock(ctx, text) + ";"
		prog, errs = visitor.Build(js)
		if len(errs) > 0 {
			t.Fatalf("javascript body %q does not parse: %v\n%s", text, errs, js)
		}
		if j := prog.Statements[len(prog.Statements)-1].(*ast.CreateJavaScriptActionStmt); j.JavaScriptCode != text {
			t.Errorf("javascript body read back as %q, want %q", j.JavaScriptCode, text)
		}
	}
}
