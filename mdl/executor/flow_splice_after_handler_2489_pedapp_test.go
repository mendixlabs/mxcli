// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2489: a statement appended after an activity with a custom error
// handler (`on error without rollback begin … end error;`) was spliced onto the
// activity's normal outgoing flow — between the activity and the merge the
// handler rejoins. On the error path the flow jumped past it, and the next exec
// of the unchanged script was refused: `the stored statement "Join" has no
// activity to address … (MDL-V1-REBUILD)`. The new statement belongs after the
// merge, where the source puts it.

const handler2489Setup = `mdl 1;
create microflow MyFirstModule.SUB_Ensure2489 () returns Boolean as $R
begin
  return true;
end;
create microflow MyFirstModule.SUB_Startup2489 ()
begin
  log info node 'Probe' 'start';
end;`

const handler2489Before = `mdl 1;
create or modify microflow MyFirstModule.ASU2489 () returns Boolean as $Ok
begin
  $Ready = call microflow MyFirstModule.SUB_Ensure2489() on error without rollback begin
    log warning node 'Probe' 'ensure failed';
  end error;
  return true;
end;`

var handler2489After = strings.Replace(handler2489Before, "  end error;\n  return true;",
	"  end error;\n  call microflow MyFirstModule.SUB_Startup2489();\n  return true;", 1)

func TestSpliceAppendsAfterTheHandlersRejoin_2489(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	for _, src := range []string{handler2489Setup, handler2489Before} {
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("setup: %v\n%s", err, out.String())
		}
	}
	out.Reset()
	if err := agreeExec(t, exec, handler2489After); err != nil {
		t.Fatalf("append: %v\n%s", err, out.String())
	}
	out.Reset()
	if err := afRun(t, exec, "describe microflow MyFirstModule.ASU2489;"); err != nil {
		t.Fatal(err)
	}
	desc := out.String()
	call := strings.Index(desc, "call microflow MyFirstModule.SUB_Startup2489")
	merge := strings.Index(desc, "merge ")
	if call < 0 {
		t.Fatalf("appended call missing:\n%s", desc)
	}
	// Either no explicit merge (the rejoin is implicit, at the call) or the
	// call after it: never between the handler's join and its merge.
	if merge >= 0 && call < merge {
		t.Errorf("the appended call sits before the rejoin merge, so the error path skips it:\n%s", desc)
	}

	// The unchanged script re-runs: nothing refused.
	out.Reset()
	if err := agreeExec(t, exec, handler2489After); err != nil {
		t.Fatalf("re-run of the unchanged script refused: %v\n%s", err, out.String())
	}
}
