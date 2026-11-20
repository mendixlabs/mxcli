// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// An if/else inside a `while true` whose branches join before one more step
// and the jump back (mendixlabs/mxcli#1829, ako/mxcli#2168). The loop header is
// on a cycle, so it "reaches" the inner if's merge — round the loop, through
// the if itself. DESCRIBE took that as the inner merge lying past the header,
// paired the if with the header instead of its own merge, and printed
// `set $i = $i + 1` as the last statement of the THEN branch. Rebuilt from that
// text, the else branch jumps straight back and the loop never ends.
const whileJoinThenStepMDL = `create microflow M.WhileJoinThenStep ()
begin
  declare $i Integer = 0;
  declare $created Integer = 0;
  while true
  begin
    if $i > 5 then
      return;
    end if;
    if $i = 2 then
      set $created = $created + 1;
    else
      log info node 'T' 'exists';
    end if;
    set $i = $i + 1;
  end while;
end;`

func TestDescribeWhileTrue_StepAfterIfJoinStaysAfterEndIf(t *testing.T) {
	oc := buildMicroflowFromMDL(t, whileJoinThenStepMDL)
	mf := &microflows.Microflow{ObjectCollection: oc}
	e := newTestExecutor()
	out := strings.Join(formatMicroflowActivities(e.newExecContext(t.Context()), mf, nil, nil), "\n")

	endIf := strings.LastIndex(out, "end if;")
	step := strings.Index(out, "set $i = $i + 1;")
	if endIf < 0 || step < 0 || step < endIf {
		t.Fatalf("the step after the join must follow the last `end if;`:\n%s", out)
	}
	assertDescriptionRebuildsGraph(t, out, oc)
}

// The other shape the same walk has to get right: the loop is left from the
// ELSE arm, and the THEN arm goes round again (WorkflowCommons'
// SUB_UserTaskView_Migrate on TestApp). Going round, the THEN arm comes back
// through this same `if` into the ELSE arm, which must not read as the two arms
// joining there — that pulled the exit out of the `if` and made it run on every
// iteration.
const whileExitInElseMDL = `create microflow M.WhileExitInElse () returns Integer
begin
  declare $total Integer = 0;
  while true
  begin
    if $total < 5 then
      set $total = $total + 1;
      log info node 'T' 'again';
    else
      log info node 'T' 'done';
      return $total;
    end if;
  end while;
end;`

func TestDescribeWhileTrue_ExitInElseStaysInElse(t *testing.T) {
	oc := buildMicroflowFromMDL(t, whileExitInElseMDL)
	mf := &microflows.Microflow{ObjectCollection: oc}
	e := newTestExecutor()
	out := strings.Join(formatMicroflowActivities(e.newExecContext(t.Context()), mf, nil, nil), "\n")

	if !strings.Contains(out, "else") || strings.Index(out, "return $total;") > strings.Index(out, "end if;") {
		t.Fatalf("the exit must stay in the ELSE arm:\n%s", out)
	}
	assertDescriptionRebuildsGraph(t, out, oc)
}
