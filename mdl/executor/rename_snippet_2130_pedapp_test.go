// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#2130 (mendixlabs/mxcli#1811): there was no way to rename a snippet —
// `rename snippet` was a syntax error, and `mxcli rename page <snippet>` said
// page not found — while lint CONV005 asks for SNIPPET_ prefixes. RENAME
// SNIPPET renames it and every snippet call follows: in a page, in a layout and
// in another snippet.
func TestRenameSnippet_UpdatesEverySnippetCall_Issue2130(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	dir := filepath.Dir(exec.newExecContext(context.Background()).Backend.Path())
	const setup = `mdl 1;
create snippet MyFirstModule.Combo2130 {
  dynamictext t1 (Content: 'combo')
};
create snippet MyFirstModule.Outer2130 {
  snippetcall inner (Snippet: MyFirstModule.Combo2130)
};
create page MyFirstModule.Page2130 ( Title: 'P', Layout: Atlas_Core.Atlas_Default ) {
  snippetcall sc1 (Snippet: MyFirstModule.Combo2130)
};
create page MyFirstModule.Other2130 ( Title: 'O', Layout: Atlas_Core.Atlas_Default ) {
  dynamictext t2 (Content: 'no call')
};`
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	// A dry run names the documents that would change and changes nothing.
	out.Reset()
	if err := agreeExec(t, exec, "mdl 1;\nrename snippet MyFirstModule.Combo2130 to SNIPPET_Combo2130 dry run;"); err != nil {
		t.Fatalf("dry run: %v\n%s", err, out.String())
	}
	if d := describeOut(t, exec, "describe page MyFirstModule.Page2130;"); !strings.Contains(d, "MyFirstModule.Combo2130") {
		t.Errorf("a dry run changed the page:\n%s", d)
	}

	assertAgree(t, exec, dir, "mdl 1;\nrename snippet MyFirstModule.Combo2130 to SNIPPET_Combo2130;", "")

	for _, stmt := range []string{"describe page MyFirstModule.Page2130;", "describe snippet MyFirstModule.Outer2130;"} {
		d := describeOut(t, exec, stmt)
		if !strings.Contains(d, "MyFirstModule.SNIPPET_Combo2130") || strings.Contains(d, "MyFirstModule.Combo2130") {
			t.Errorf("%s does not call the renamed snippet:\n%s", stmt, d)
		}
	}
	if d := describeOut(t, exec, "describe snippet MyFirstModule.SNIPPET_Combo2130;"); !strings.Contains(d, "'combo'") {
		t.Errorf("the renamed snippet lost its content:\n%s", d)
	}

	// Refusals, in check and exec alike: a snippet that does not exist, and a
	// name already taken in the module's page/snippet/layout name space.
	assertAgree(t, exec, dir, "mdl 1;\nrename snippet MyFirstModule.NoSuch2130 to X2130;", "NoSuch2130")
	assertAgree(t, exec, dir, "mdl 1;\nrename snippet MyFirstModule.Outer2130 to SNIPPET_Combo2130;", "SNIPPET_Combo2130")
	// Pages, snippets and layouts share one name space per module.
	assertAgree(t, exec, dir, "mdl 1;\nrename snippet MyFirstModule.Outer2130 to Other2130;", "Other2130")
}

func describeOut(t *testing.T, exec *Executor, stmt string) string {
	t.Helper()
	var buf bytes.Buffer
	saved := exec.output
	exec.output = &buf
	err := afRun(t, exec, stmt)
	exec.output = saved
	if err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
	return buf.String()
}
