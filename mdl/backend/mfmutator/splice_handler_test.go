// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1489 (ako/mxcli#1345): an activity with a custom error
// handler could not be replaced at all — "cannot replace the ActionActivity at
// (360, 200): it has an error handler, which would be left with no activity;
// rewrite the flow with create or modify instead" — even by the same kind of
// activity handling errors the same way. Replaced by one activity that has a
// custom handler of the same kind, it is swapped in place: the stored handler,
// its error flow and the merge it rejoins at all stay, and point at the new
// activity.

// withEH gives a stored activity document the error handling its action has.
func withEH(d bson.D, eh microflows.ErrorHandlingType) bson.D {
	return append(d, bson.E{Key: "Action", Value: bson.D{
		{Key: "$Type", Value: "Microflows$MicroflowCallAction"},
		{Key: "ErrorHandlingType", Value: string(eh)},
	}})
}

// oneActivityEH is oneActivity whose action handles errors with eh.
func oneActivityEH(eh microflows.ErrorHandlingType) *backend.MicroflowFragment {
	frag := oneActivity()
	frag.Objects[0].(*microflows.ActionActivity).Action = &microflows.MicroflowCallAction{ErrorHandlingType: eh}
	return frag
}

// handlerFlow is start -> a -> merge -> end, with a's error handler h
// rejoining at the merge.
func handlerFlow() ([]bson.D, []bson.D) {
	a := withEH(obj("a", "Microflows$ActionActivity", 250, 200), microflows.ErrorHandlingTypeCustomWithoutRollback)
	dSet(a, "Size", "160;60") // resized in Studio Pro: kept
	return []bson.D{
		obj("start", "Microflows$StartEvent", 100, 200),
		a,
		obj("h", "Microflows$ActionActivity", 250, 330),
		obj("m", "Microflows$ExclusiveMerge", 400, 200),
		obj("end", "Microflows$EndEvent", 550, 200),
	}, []bson.D{
		flow("f1", "start", "a", 1, 3, false),
		flow("f2", "a", "m", 1, 3, false),
		flow("err", "a", "h", 2, 0, true),
		flow("fh", "h", "m", 1, 2, false),
		flow("f3", "m", "end", 1, 3, false),
	}
}

func TestSplice_ReplaceKeepsTheErrorHandler(t *testing.T) {
	// The same kind of activity (a call for a call) is an edit of the stored
	// one, which keeps its $ID (ako/mxcli#1761): the replacement equal to it,
	// the unit is the stored one, byte for byte.
	t.Run("the same kind of activity", func(t *testing.T) {
		objs, flows := handlerFlow()
		m, deps := newMutator(t, unit(objs, flows))
		if err := m.Replace(model.ID(uid("a")), oneActivityEH(microflows.ErrorHandlingTypeCustomWithoutRollback)); err != nil {
			t.Fatal(err)
		}
		if err := m.Save(); err != nil {
			t.Fatalf("save: %v", err)
		}
		raw, _ := bson.Marshal(unit(handlerFlow()))
		if !bytes.Equal(deps.saved, raw) {
			var got bson.D
			_ = bson.Unmarshal(deps.saved, &got)
			t.Errorf("the replace by the same activity changed the unit:\n got %v\nwant %v", got, unit(handlerFlow()))
		}
	})
	// Another kind is a new activity: what the replace should leave is the
	// stored unit with a's document swapped for the new one, drawn where a
	// was at a's size, and the three flows through a — in, out, and the
	// error flow — pointing at it.
	t.Run("another kind of activity", func(t *testing.T) {
		objs, flows := handlerFlow()
		raw, _ := bson.Marshal(unit(objs, flows))
		var d bson.D
		_ = bson.Unmarshal(raw, &d)
		java := bson.D{
			{Key: "$Type", Value: "Microflows$JavaActionCallAction"},
			{Key: "ErrorHandlingType", Value: string(microflows.ErrorHandlingTypeCustomWithoutRollback)},
		}
		deps := &editDeps{action: java}
		m, err := New(d, "unit", deps)
		if err != nil {
			t.Fatal(err)
		}
		frag := oneActivityEH(microflows.ErrorHandlingTypeCustomWithoutRollback)
		if err := m.Replace(model.ID(uid("a")), frag); err != nil {
			t.Fatal(err)
		}
		if err := m.Save(); err != nil {
			t.Fatalf("save: %v", err)
		}
		newID := bson.Binary{Data: types.UUIDToBlob(string(frag.Entry))}
		wantObjs, wantFlows := handlerFlow()
		wantObjs[1] = bson.D{
			{Key: "$ID", Value: newID},
			{Key: "$Type", Value: "Microflows$ActionActivity"},
			{Key: "RelativeMiddlePoint", Value: "250;200"},
			{Key: "Size", Value: "160;60"},
			{Key: "Action", Value: java},
		}
		dSet(wantFlows[0], "DestinationPointer", newID)
		dSet(wantFlows[1], "OriginPointer", newID)
		dSet(wantFlows[2], "OriginPointer", newID)
		want, _ := bson.Marshal(unit(wantObjs, wantFlows))
		if !bytes.Equal(deps.saved, want) {
			var got bson.D
			_ = bson.Unmarshal(deps.saved, &got)
			t.Errorf("the replace changed more than the activity and the flows through it:\n got %v\nwant %v", got, unit(wantObjs, wantFlows))
		}
	})
}

func TestSplice_ReplaceOfAHandledActivityRefusals(t *testing.T) {
	for _, tc := range []struct {
		name string
		frag func() *backend.MicroflowFragment
		want string
	}{
		// Without a custom handler the stored one would hang off an
		// activity that does not raise to it.
		{"no handler", oneActivity, "states none"},
		{"another kind of handler", func() *backend.MicroflowFragment {
			return oneActivityEH(microflows.ErrorHandlingTypeCustom)
		}, "the replacement's is `on error begin … end error`"},
		{"two activities for one", func() *backend.MicroflowFragment {
			frag := oneActivityEH(microflows.ErrorHandlingTypeCustomWithoutRollback)
			second := oneActivity()
			frag.Objects = append(frag.Objects, second.Objects...)
			frag.Exit = second.Exit
			return frag
		}, "by one activity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objs, flows := handlerFlow()
			m, _ := newMutator(t, unit(objs, flows))
			before, _ := bson.Marshal(m.doc)
			err := m.Replace(model.ID(uid("a")), tc.frag())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
			// The advice must not send the reader to create or modify,
			// which refuses the same change (the circular advice of #1489).
			if strings.Contains(err.Error(), "create or modify") {
				t.Errorf("the refusal advises create or modify: %v", err)
			}
			after, _ := bson.Marshal(m.doc)
			if !bytes.Equal(before, after) {
				t.Error("a refused replace changed the document")
			}
		})
	}
}
