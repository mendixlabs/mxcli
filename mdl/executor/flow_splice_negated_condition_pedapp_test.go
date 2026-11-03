// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1930 (mendixlabs/mxcli#1728): a decision whose true path goes
// straight on and whose false path holds the activities is printed by describe
// folded — `if not(X) then <false path> end if;` — with its paths swapped. Fed
// back through `create or modify` with only the condition changed, the in-place
// condition edit stored the script's `not(Y)` as the decision's expression
// while the true and false flows stayed where they were: the decision was
// inverted, and check, exec ("spliced: 1 edited in place") and diff all looked
// harmless. The stored expression must be the one that keeps the meaning the
// script states: the negation of the printed condition.

const negatedSetup = `mdl 1;
create persistent entity MyFirstModule.Sale1930 (Code: String(200), Reference: String(200));
create or modify microflow MyFirstModule.VAL_Sale1930 ($Sale: MyFirstModule.Sale1930)
returns Boolean
begin
  declare $Valid Boolean = true;
  @caption 'Code filled?'
  if %s then
  else
    set $Valid = false;
  end if;
  return $Valid;
end;`

// storedDecision returns the decision of VAL_Sale1930 and its expression.
func storedDecision(t *testing.T, exec *Executor) (*microflows.ExclusiveSplit, string) {
	t.Helper()
	mf := loopEditFlowOf(t, exec, "VAL_Sale1930")
	for _, obj := range mf.ObjectCollection.Objects {
		if s, ok := obj.(*microflows.ExclusiveSplit); ok {
			cond, ok := s.SplitCondition.(*microflows.ExpressionSplitCondition)
			if !ok {
				t.Fatalf("the decision's condition is %T, want an expression", s.SplitCondition)
			}
			return s, cond.Expression
		}
	}
	t.Fatal("VAL_Sale1930 has no decision")
	return nil, ""
}

func TestCreateOrModify_FoldedDecisionKeepsItsMeaning(t *testing.T) {
	for _, tc := range []struct {
		name, stored, printed, edited, want string
	}{
		{
			// The reporter's flow.
			name:    "printed as not(X)",
			stored:  "trim($Sale/Code) != ''",
			printed: "if not(trim($Sale/Code) != '') then",
			edited:  "if not(trim($Sale/Reference) != '') then",
			want:    "trim($Sale/Reference) != ''",
		},
		{
			// describe takes a stored not(Z) out of the fold rather than
			// printing not(not(Z)); the edit must put it back.
			name:    "stored not(Z), printed as Z",
			stored:  "not($Sale/Code = empty)",
			printed: "if $Sale/Code = empty then",
			edited:  "if $Sale/Reference = empty then",
			want:    "not($Sale/Reference = empty)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec, out, _ := openPedAppCopy(t)
			if err := agreeExec(t, exec, strings.Replace(negatedSetup, "%s", tc.stored, 1)); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			before, _ := storedDecision(t, exec)

			described := describeFlow(t, exec, "VAL_Sale1930")
			// Control: the flow is printed folded, which is the case this is about.
			if !strings.Contains(described, tc.printed) {
				t.Fatalf("control: describe does not print %q:\n%s", tc.printed, described)
			}
			src := strings.Replace(described, tc.printed, tc.edited, 1)
			if !strings.HasPrefix(strings.TrimSpace(src), "mdl ") {
				src = "mdl 1;\n" + src
			}
			out.Reset()
			if err := agreeExec(t, exec, src); err != nil {
				t.Fatalf("exec of the edited description: %v\n%s", err, out.String())
			}

			after, expr := storedDecision(t, exec)
			if after.ID != before.ID {
				t.Fatalf("the decision was replaced (%s → %s), not edited in place:\n%s", before.ID, after.ID, out.String())
			}
			if expr != tc.want {
				t.Fatalf("stored expression = %q, want %q: the decision's flows did not move, so storing the printed condition inverts it", expr, tc.want)
			}
			if again := describeFlow(t, exec, "VAL_Sale1930"); !strings.Contains(again, tc.edited) {
				t.Errorf("describe no longer prints the edited condition %q:\n%s", tc.edited, again)
			}
		})
	}
}

// The same fold inside a loop body, which describe prints from the loop's own
// collection. The splice does not change a decision in a loop body at all — it
// refuses, writing nothing — so the inversion cannot happen there; this pins
// that the folded condition is not stored by some other route.
func TestCreateOrModify_FoldedDecisionInALoopIsNotInverted(t *testing.T) {
	const setup = `mdl 1;
create persistent entity MyFirstModule.Sale1930 (Code: String(200), Reference: String(200));
create or modify microflow MyFirstModule.VAL_Loop1930 ($Sales: list of MyFirstModule.Sale1930)
returns Boolean
begin
  declare $Valid Boolean = true;
  loop $Sale in $Sales
  begin
    if trim($Sale/Code) != '' then
    else
      set $Valid = false;
    end if;
  end loop;
  return $Valid;
end;`
	const printed = "if not(trim($Sale/Code) != '') then"
	const edited = "if not(trim($Sale/Reference) != '') then"

	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	described := describeFlow(t, exec, "VAL_Loop1930")
	if !strings.Contains(described, printed) {
		t.Fatalf("control: describe does not print %q:\n%s", printed, described)
	}
	src := strings.Replace(described, printed, edited, 1)
	if !strings.HasPrefix(strings.TrimSpace(src), "mdl ") {
		src = "mdl 1;\n" + src
	}
	out.Reset()
	if err := agreeExec(t, exec, src); err == nil || !strings.Contains(err.Error(), "cannot be spliced") {
		t.Fatalf("exec of the edited description = %v, want the loop-body refusal\n%s", err, out.String())
	}

	var expr string
	var walk func(oc *microflows.MicroflowObjectCollection)
	walk = func(oc *microflows.MicroflowObjectCollection) {
		for _, obj := range oc.Objects {
			switch o := obj.(type) {
			case *microflows.ExclusiveSplit:
				if c, ok := o.SplitCondition.(*microflows.ExpressionSplitCondition); ok {
					expr = c.Expression
				}
			case *microflows.LoopedActivity:
				walk(o.ObjectCollection)
			}
		}
	}
	walk(loopEditFlowOf(t, exec, "VAL_Loop1930").ObjectCollection)
	if want := "trim($Sale/Code) != ''"; expr != want {
		t.Fatalf("stored expression in the loop = %q, want it unchanged, %q", expr, want)
	}
}
