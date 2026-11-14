// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2259: a project whose scripts were in sync under one nightly —
// every re-run wrote nothing — had three of them refused with MDL-V1-REBUILD
// by the next, with the scripts and the model unchanged. The scripts state no
// @position, so every node of the three flows was placed by the layout pass of
// the release that wrote them; the next release's layout pass draws some of
// them elsewhere, the built graph no longer paired with the stored one by
// position, and the body went on to the statement diff, which does not match
// these three shapes as describe prints them back:
//
//   - an `if` with no else inside a call's error handler (described with an
//     empty `else`);
//   - a call's handler that falls through, inside an `if` that ends in nested
//     guards (described as a chain of joins and merges);
//   - a nested `while` whose body holds an `if` with no else.
//
// Each case writes the flow, moves its stored nodes as another release's
// layout would have drawn them, and runs the unchanged script again: nothing
// is written and the statement reports Unchanged. The controls: the re-run
// with the nodes where this release draws them (unchanged before the fix
// too), a genuine change of the flow's shape (still refused), and a position
// the script states (still a change).

const layoutDriftSub = `mdl 1;
create or modify non-persistent entity MyFirstModule.LayoutDriftSlot ("Start": Integer);
create or modify microflow MyFirstModule.LayoutDrift_Sub ($Q: String) returns String
begin
  return $Q;
end;`

var layoutDriftCases = []struct {
	name, flow, script string
	// refusal is what the unfixed splice says of the re-run once the nodes
	// moved.
	refusal string
	// reshaped is the script with the flow's shape changed, which must still
	// be refused.
	reshaped func(string) string
}{
	{
		name: "if without else in an error handler",
		flow: "LayoutDrift_Handler",
		script: `mdl 1;
create or modify microflow MyFirstModule.LayoutDrift_Handler ($Query: String, $Flag: Boolean) returns String
begin
  $Full = call microflow MyFirstModule.LayoutDrift_Sub (Q = $Query) on error without rollback begin
    log warning node 'M' 'not available: {1}' with ({1} = $latestError/Message);
    $Fallback = call microflow MyFirstModule.LayoutDrift_Sub (Q = $Query);
    if $Flag then
      log info node 'M' 'word match';
    end if;
    return $Fallback;
  end error;
  if length($Full) > 0 then
    if $Flag then
      log info node 'M' 'full text';
    end if;
    return $Full;
  end if;
  $Words = call microflow MyFirstModule.LayoutDrift_Sub (Q = $Query);
  return $Words;
end;`,
		refusal: "changes where its paths end or meet",
		reshaped: func(s string) string {
			return strings.Replace(s, "      log info node 'M' 'word match';\n", "      log info node 'M' 'word match';\n      return 'early';\n", 1)
		},
	},
	{
		name: "a falling-through handler inside nested guards",
		flow: "LayoutDrift_Draft",
		script: `mdl 1;
create or modify microflow MyFirstModule.LayoutDrift_Draft ($N: Integer) returns Boolean
begin
  declare $Accepted Boolean = $N > 0;
  $Agent = call microflow MyFirstModule.LayoutDrift_Sub (Q = 'agent');
  if $Agent != empty then
    $Response = call microflow MyFirstModule.LayoutDrift_Sub (Q = $Agent) on error without rollback begin
      log error node 'M' 'failed: {1}' with ({1} = $latestError/Message);
    end error;
    if $Response != empty and $Accepted then
      $Reply = call microflow MyFirstModule.LayoutDrift_Sub (Q = $Response);
      if $Reply != '' then
        log info node 'M' 'drafted';
        return true;
      end if;
    end if;
  end if;
  log info node 'M' 'fallback';
  return false;
end;`,
		refusal: "has no activity to address",
		reshaped: func(s string) string {
			return strings.Replace(s, "      log error node 'M' 'failed: {1}' with ({1} = $latestError/Message);\n",
				"      log error node 'M' 'failed: {1}' with ({1} = $latestError/Message);\n      return false;\n", 1)
		},
	},
	{
		name: "a nested while whose body holds an if without else",
		flow: "LayoutDrift_Slots",
		script: `mdl 1;
create or modify microflow MyFirstModule.LayoutDrift_Slots () returns Integer
begin
  declare $Count Integer = 0;
  declare $Day Integer = 1;
  while $Day <= 14 begin
    declare $Weekday Integer = $Day mod 7;
    if $Weekday != 6 and $Weekday != 0 then
      declare $Slot Integer = 0;
      while $Slot < 4 begin
        declare $Taken Boolean = $Slot = 2;
        if $Taken = false then
          create MyFirstModule.LayoutDriftSlot ("Start" = $Slot);
        end if;
        set $Slot = $Slot + 1;
      end while;
    end if;
    set $Day = $Day + 1;
  end while;
  return $Count;
end;`,
		refusal: "changes inside its body",
		reshaped: func(s string) string {
			return strings.Replace(s, "          create MyFirstModule.LayoutDriftSlot (\"Start\" = $Slot);\n        end if;",
				"          create MyFirstModule.LayoutDriftSlot (\"Start\" = $Slot);\n        else\n          set $Count = $Count + 1;\n        end if;", 1)
		},
	},
}

// driftStoredLayout moves every top-level node of the stored flow as another
// layout pass would have drawn it — each by its own offset, so no two moves
// are alike — and stores it.
func driftStoredLayout(t *testing.T, exec *Executor, name string) {
	t.Helper()
	mf := loopEditFlowOf(t, exec, name)
	for i, obj := range mf.ObjectCollection.Objects {
		p := obj.GetPosition()
		obj.SetPosition(model.Point{X: p.X + 5*(i+1), Y: p.Y})
	}
	ctx := exec.newExecContext(context.Background())
	if err := ctx.Backend.UpdateMicroflow(mf); err != nil {
		t.Fatalf("moving the stored nodes: %v", err)
	}
}

func TestCreateOrModify_LayoutDriftIsUnchanged(t *testing.T) {
	for _, c := range layoutDriftCases {
		t.Run(c.name, func(t *testing.T) {
			exec, out, dir := openPedAppCopy(t)
			if err := agreeExec(t, exec, layoutDriftSub+"\n"+c.script); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			unchanged := func(t *testing.T, what string) {
				t.Helper()
				before := projectFiles(t, dir)
				out.Reset()
				if err := agreeExec(t, exec, c.script); err != nil {
					t.Fatalf("%s: the unchanged script is refused: %v", what, err)
				}
				if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) != 0 {
					t.Errorf("%s: the unchanged script wrote %v", what, changed)
				}
				if !strings.Contains(out.String(), "Unchanged ") {
					t.Errorf("%s: want Unchanged, got:\n%s", what, out.String())
				}
			}
			// Control: drawn where this release draws it, the flow was in
			// sync before the fix as well.
			unchanged(t, "as drawn by this release")

			driftStoredLayout(t, exec, c.flow)
			unchanged(t, "as drawn by another release")

			// Control: a genuine change of the flow's shape is still refused.
			reshaped := c.reshaped(c.script)
			if reshaped == c.script {
				t.Fatal("the reshaping edit does not apply")
			}
			before := projectFiles(t, dir)
			err := agreeExec(t, exec, reshaped)
			if err == nil || !strings.Contains(err.Error(), "MDL-V1-REBUILD") && !strings.Contains(err.Error(), "cannot be spliced") {
				t.Errorf("a changed shape must still be refused, got %v\n%s", err, out.String())
			}
			if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) != 0 {
				t.Errorf("the refused change wrote %v", changed)
			}
		})
	}
}

// Control: a position the script states is compared however the rest is
// drawn — a stated node at another place than stored is moved, not reported
// Unchanged.
func TestCreateOrModify_LayoutDriftKeepsStatedPositions(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	c := layoutDriftCases[0]
	if err := agreeExec(t, exec, layoutDriftSub+"\n"+c.script); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	driftStoredLayout(t, exec, c.flow)
	stated := strings.Replace(c.script, "  $Words = call", "  @position(2400, 600)\n  $Words = call", 1)
	out.Reset()
	if err := agreeExec(t, exec, stated); err != nil {
		t.Fatalf("the stated position is refused: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "Unchanged ") {
		t.Fatalf("a stated position the stored node is not at was reported Unchanged:\n%s", out.String())
	}
	mf := loopEditFlowOf(t, exec, c.flow)
	for _, obj := range mf.ObjectCollection.Objects {
		if obj.GetPosition() == (model.Point{X: 2400, Y: 600}) {
			if _, ok := obj.(*microflows.ActionActivity); ok {
				return
			}
		}
	}
	t.Errorf("no activity is at the stated (2400, 600):\n%s", out.String())
}
