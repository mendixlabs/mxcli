// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1735 (ako/mxcli#1939): "the drop-and-create rebuild, which
// the MDL-V1-REBUILD refusal recommends, deletes an existing send activity
// silently. check passes, exec prints no warning, and the activity is gone."
// Any stored activity MDL cannot express is read as an UnsupportedAction; every
// path that builds the flow from the statement must refuse rather than drop it.

func storedFlowWithUnsupported(t *testing.T) (*mock.MockBackend, *ExecContext) {
	t.Helper()
	mod := mkModule("Main")
	existing := &microflows.Microflow{Name: "ACT_Save"}
	existing.ID = "mf-existing"
	existing.ContainerID = mod.ID
	loop := &microflows.LoopedActivity{ObjectCollection: &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.ActionActivity{Action: &microflows.UnsupportedAction{StorageType: "Microflows$CounterMeterAction"}},
		},
	}}
	existing.ObjectCollection = &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
		&microflows.ActionActivity{Action: &microflows.UnsupportedAction{StorageType: "Microflows$PushToClientAction"}},
		loop, // nested: the walk must reach loop bodies
	}}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{existing}, nil
		},
		UpdateMicroflowFunc: func(mf *microflows.Microflow) error {
			t.Error("UpdateMicroflow must not run: the rewrite would drop the unexpressible activities")
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return mb, ctx
}

func TestUnsupportedActionTypes_WalksLoops(t *testing.T) {
	_, ctx := storedFlowWithUnsupported(t)
	got := storedFlowUnsupportedActions(ctx, "microflow", "Main.ACT_Save")
	want := "Microflows$CounterMeterAction,Microflows$PushToClientAction"
	if strings.Join(got, ",") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

func TestDropRecreateOfFlowWithUnsupportedActionIsRefused(t *testing.T) {
	_, ctx := storedFlowWithUnsupported(t)
	prog := parseMDL(t, `drop microflow Main.ACT_Save;
create microflow Main.ACT_Save ()
begin
  return;
end;`)
	errs := validateDropRecreateKeepsActivities(ctx, prog)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Microflows$PushToClientAction") {
		t.Fatalf("want one refusal naming the activity, got %v", errs)
	}

	// controls: a drop alone is a deliberate deletion, and a create without a
	// preceding drop is not this guard's case
	for _, src := range []string{
		"drop microflow Main.ACT_Save;",
		"create or modify microflow Main.ACT_Save ()\nbegin\n  return;\nend;",
	} {
		if errs := validateDropRecreateKeepsActivities(ctx, parseMDL(t, src)); len(errs) != 0 {
			t.Errorf("%q: unexpected refusal %v", src, errs)
		}
	}
}

// The whole-flow rebuild (what a create or modify the splice cannot make falls
// back to under mdl 0) is buildMicroflowFromStmt; the guard sits inside it, so
// this fails if it is removed.
func TestRebuildOfFlowWithUnsupportedActionIsRefused(t *testing.T) {
	_, ctx := storedFlowWithUnsupported(t)
	_, err := buildMicroflowFromStmt(ctx, &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "Main", Name: "ACT_Save"},
		CreateOrModify: true,
	}, buildFlowOpts{AllowCreate: true})
	if err == nil || !strings.Contains(err.Error(), "Microflows$CounterMeterAction") {
		t.Fatalf("expected the rebuild to be refused naming the activities, got %v", err)
	}
}
