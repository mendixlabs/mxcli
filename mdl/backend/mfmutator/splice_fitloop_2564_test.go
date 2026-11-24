// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2564: a node inside a loop body is moved in the loop box's
// coordinates, and the box is then fitted to the moved body, as `create`
// sizes it: the body's right and bottom edges plus the padding.
func TestMoveInLoopAndFitLoop2564(t *testing.T) {
	newLoop := func(size string, objs ...bson.D) bson.D {
		l := obj("loop", "Microflows$LoopedActivity", 600, 200)
		for i := range l {
			if l[i].Key == "Size" {
				l[i].Value = size
			}
		}
		arr := bson.A{int32(3)}
		for _, o := range objs {
			arr = append(arr, o)
		}
		return append(l, bson.E{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: bin("loopoc")},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: arr},
		}})
	}
	sizeOf := func(m *Mutator, id string) string { return m.graph().nodes[uid(id)].size.String() }
	posOf := func(m *Mutator, id string) string { return m.graph().nodes[uid(id)].pos.String() }
	min := model.Size{Width: 200, Height: 100}

	t.Run("shrinks to the moved body", func(t *testing.T) {
		m, _ := newMutator(t, unit([]bson.D{obj("start", "Microflows$StartEvent", 100, 200),
			newLoop("710;380", obj("inner", "Microflows$ActionActivity", 600, 300))}, nil))
		if err := m.Move(model.ID(uid("inner")), model.Point{X: 150, Y: 100}); err != nil {
			t.Fatal(err)
		}
		if err := m.FitLoop(model.ID(uid("loop")), nil, min); err != nil {
			t.Fatal(err)
		}
		if got := posOf(m, "inner"); got != "150;100" {
			t.Errorf("the body node is at %s", got)
		}
		// 150+60 = 210 right edge, 100+30 = 130 bottom edge, plus 50.
		if got := sizeOf(m, "loop"); got != "260;180" {
			t.Errorf("the box is %s, want 260;180", got)
		}
		if got := posOf(m, "loop"); got != "600;200" {
			t.Errorf("the box moved to %s; it keeps its centre", got)
		}
	})
	t.Run("a stated size", func(t *testing.T) {
		m, _ := newMutator(t, unit([]bson.D{newLoop("710;380", obj("inner", "Microflows$ActionActivity", 150, 100))}, nil))
		if err := m.FitLoop(model.ID(uid("loop")), &model.Size{Width: 400, Height: 300}, min); err != nil {
			t.Fatal(err)
		}
		if got := sizeOf(m, "loop"); got != "400;300" {
			t.Errorf("the box is %s, want the stated 400;300", got)
		}
	})
	t.Run("never smaller than the minimum", func(t *testing.T) {
		m, _ := newMutator(t, unit([]bson.D{newLoop("710;380", obj("inner", "Microflows$ActionActivity", 70, 40))}, nil))
		if err := m.FitLoop(model.ID(uid("loop")), nil, min); err != nil {
			t.Fatal(err)
		}
		if got := sizeOf(m, "loop"); got != "200;120" {
			t.Errorf("the box is %s, want 200;120", got)
		}
	})
	t.Run("a box drawn over a neighbour is refused", func(t *testing.T) {
		m, _ := newMutator(t, unit([]bson.D{newLoop("260;180", obj("inner", "Microflows$ActionActivity", 150, 100)),
			obj("next", "Microflows$ActionActivity", 900, 200)}, nil))
		before, _ := bson.Marshal(m.doc)
		err := m.FitLoop(model.ID(uid("loop")), &model.Size{Width: 800, Height: 200}, min)
		if err == nil || !strings.Contains(err.Error(), "drawn over") {
			t.Fatalf("want a refusal naming the neighbour, got %v", err)
		}
		if after, _ := bson.Marshal(m.doc); string(after) != string(before) {
			t.Error("the refused fit changed the document")
		}
	})
}
