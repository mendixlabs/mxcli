// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2014 (mendixlabs/mxcli#1774): when `check --references` found a
// reference error it stopped before the flow verdicts — the "exec would refuse
// this statement" test — so a script with a reference error in statement A and
// a flow change exec refuses in statement B reported A only. Fixing A and
// checking again was the first sign of B; on a 1,257-statement script the
// reporter met it as an exec that ran far and rolled everything back. The
// verdicts run whatever the reference pass found, and the run reports both.

const loopFlow2014 = `mdl 1;
create microflow MyFirstModule.LoopFlow2014 ($Items: List of FeedbackModule.Feedback)
begin
  loop $It in $Items
  begin
    commit $It;
  end loop;
end;`

// loopFlowProject is a PedApp copy holding LoopFlow2014.
func loopFlowProject(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "PedApp.mpr")
	exec := executor.New(io.Discard)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: mpr}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	prog, errs := visitor.Build(loopFlow2014)
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if err := exec.ExecuteProgram(prog); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_ = exec.Execute(&ast.DisconnectStmt{})
	return mpr
}

// refusedLoopChange is B: a decision added inside the loop body, a change of
// the body's shape the splice refuses under mdl 1.
func refusedLoopChange() string {
	changed := strings.Replace(strings.Replace(loopFlow2014, "create microflow", "create or modify microflow", 1),
		"commit $It;", "commit $It;\n    if $It/Subject = empty then log info node 'X' 'committed'; end if;", 1)
	return strings.TrimPrefix(changed, "mdl 1;\n")
}

// referenceError is A: a call to a microflow that does not exist.
const referenceError2014 = `create or modify microflow MyFirstModule.CallsNothing2014 ()
begin
  call microflow MyFirstModule.NoSuchFlow2014();
end;
`

func runCheck2014(t *testing.T, mpr, script string) (int, string) {
	t.Helper()
	_ = checkCmd.InheritedFlags()
	_ = rootCmd.PersistentFlags().Set("project", mpr)
	t.Cleanup(func() {
		_ = rootCmd.PersistentFlags().Set("project", "")
		rootCmd.PersistentFlags().Lookup("project").Changed = false
	})
	if f := checkCmd.Flags().Lookup("format"); f != nil {
		_ = f.Value.Set(f.DefValue)
	}
	path := writeScript(t, t.TempDir(), "s.mdl", script)
	var code int
	out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{path}) })
	return code, out
}

func TestCheck_ReferenceErrorDoesNotHideAFlowExecWouldRefuse(t *testing.T) {
	mpr := loopFlowProject(t)
	const refusal = "replace body of loop $It in $Items"

	// Control: B alone is reported as a refusal.
	code, out := runCheck2014(t, mpr, "mdl 1;\n"+refusedLoopChange()+"\n")
	if code == 0 || !strings.Contains(out, refusal) {
		t.Fatalf("control: B alone must be reported as a flow change exec refuses (exit %d):\n%s", code, out)
	}

	// A and B: both are reported, and the summary counts both.
	code, out = runCheck2014(t, mpr, "mdl 1;\n"+referenceError2014+refusedLoopChange()+"\n")
	if code == 0 {
		t.Fatalf("a script with a reference error passed check:\n%s", out)
	}
	if !strings.Contains(out, "microflow not found: MyFirstModule.NoSuchFlow2014") {
		t.Errorf("the reference error in A is not reported:\n%s", out)
	}
	if !strings.Contains(out, refusal) {
		t.Errorf("the reference error hid the refusal of B:\n%s", out)
	}
	if !strings.Contains(out, "2 errors") {
		t.Errorf("the summary does not count both errors:\n%s", out)
	}
}
