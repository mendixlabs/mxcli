// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2421 (mendixlabs/mxcli#1898): the custom error handler of a
// `change` rejoined at the merge of a LATER activity's handler, so at runtime
// an error in the change skipped every statement up to and including that
// activity. Nothing reported it. A change (and a `set`) modifies a variable
// that already exists, so nothing after it needs skipping: its handler rejoins
// right after it, as a commit's does.

// rejoinTarget follows the handler's tail: the activity whose value contains
// tailValue, then any merges, to the first non-merge object it reaches.
func rejoinTarget(t *testing.T, fb *flowBuilder, tailValue string) microflows.MicroflowObject {
	t.Helper()
	var tail model.ID
	for _, o := range fb.objects {
		if a, ok := o.(*microflows.ActionActivity); ok {
			if cv, ok := a.Action.(*microflows.ChangeVariableAction); ok && strings.Contains(cv.Value, tailValue) {
				tail = a.ID
				break
			}
		}
	}
	if tail == "" {
		t.Fatalf("no handler tail activity with %q", tailValue)
	}
	cur := tail
	for range 5 {
		next := model.ID("")
		for _, f := range fb.flows {
			if f.OriginID == cur && !f.IsErrorHandler {
				next = f.DestinationID
			}
		}
		o := fb.objectByID(next)
		if o == nil {
			t.Fatalf("handler tail %s leads nowhere", cur)
		}
		if _, isMerge := o.(*microflows.ExclusiveMerge); !isMerge {
			return o
		}
		cur = next
	}
	t.Fatal("no activity after the merges")
	return nil
}

func changeValue(o microflows.MicroflowObject) string {
	if a, ok := o.(*microflows.ActionActivity); ok {
		if cv, ok := a.Action.(*microflows.ChangeVariableAction); ok {
			return cv.Value
		}
	}
	return ""
}

func TestChangeErrorHandlerRejoinsRightAfterTheChange_Issue2421(t *testing.T) {
	fb := buildFlowFromMDL(t, `declare $N Integer = 0;
  change $Order (Name = 'a') on error without rollback begin
    set $N = $N + 1;
  end error;
  set $N = $N + 10;
  commit $Order on error without rollback begin
    set $N = $N + 2;
  end error;
  return $N;`)
	if got := changeValue(rejoinTarget(t, fb, "$N + 1")); !strings.Contains(got, "+ 10") {
		t.Errorf("the change's handler rejoins before %q, want `set $N = $N + 10` — the statements between were skipped", got)
	}
}

// CONTROL: a `create` keeps rejoining after the statements that use the object
// it failed to create, which does not exist on the error path.
func TestCreateErrorHandlerStillSkipsUsesOfTheObject_Issue2421(t *testing.T) {
	fb := buildFlowFromMDL(t, `declare $N Integer = 0;
  $Order = create M.Order (Name = 'a') on error without rollback begin
    set $N = $N + 1;
  end error;
  commit $Order;
  set $N = $N + 10;
  return $N;`)
	if got := changeValue(rejoinTarget(t, fb, "$N + 1")); !strings.Contains(got, "+ 10") {
		t.Errorf("the create's handler rejoins before %q, want after `commit $Order`", got)
	}
}
