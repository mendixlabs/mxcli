// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// driftAll moves every node of oc by its own offset, as another release's
// layout pass would have drawn it.
func driftAll(oc *microflows.MicroflowObjectCollection) {
	for i, obj := range oc.Objects {
		p := obj.GetPosition()
		obj.SetPosition(model.Point{X: p.X + 15*(i+1), Y: p.Y + 3})
	}
}

// ako/mxcli#2259: the same graph drawn by another layout pass is the same
// graph, where the script states no position — and the pairing by position
// alone says it is not.
func TestSameBuiltFlowUpToLayout_AnotherDrawingIsTheSame(t *testing.T) {
	built, stored := guardGraph("b-"), guardGraph("s-")
	driftAll(stored)
	stored.Flows[0].OriginConnectionIndex = 2
	if same, _ := sameBuiltFlow(built, stored); same {
		t.Fatal("control: paired by position, the drifted drawing must differ")
	}
	if same, why := sameBuiltFlowUpToLayout(built, stored, layoutStatements{}); !same {
		t.Fatalf("another drawing of the same graph differs: %s", why)
	}
}

// Controls: up to layout, every change a script can make to the graph is
// still a difference, and so is the geometry the script states.
func TestSameBuiltFlowUpToLayout_ChangesAreDifferences(t *testing.T) {
	for _, c := range []struct {
		name   string
		edit   func(oc *microflows.MicroflowObjectCollection)
		stated layoutStatements
		want   string
	}{
		{"a changed return value", func(oc *microflows.MicroflowObjectCollection) {
			oc.Objects[3].(*microflows.EndEvent).ReturnValue = "$N + 1"
		}, layoutStatements{}, "ReturnValue"},
		{"a changed condition", func(oc *microflows.MicroflowObjectCollection) {
			oc.Objects[1].(*microflows.ExclusiveSplit).SplitCondition.(*microflows.ExpressionSplitCondition).Expression = "$N <= 0"
		}, layoutStatements{}, "Expression"},
		{"swapped branches", func(oc *microflows.MicroflowObjectCollection) {
			oc.Flows[1].CaseValue, oc.Flows[2].CaseValue = oc.Flows[2].CaseValue, oc.Flows[1].CaseValue
		}, layoutStatements{}, "ReturnValue"},
		{"a redirected flow", func(oc *microflows.MicroflowObjectCollection) {
			oc.Flows[2].DestinationID = oc.Objects[2].GetID()
		}, layoutStatements{}, "nothing stored"},
		{"an extra node", func(oc *microflows.MicroflowObjectCollection) {
			oc.Objects = append(oc.Objects, &microflows.ExclusiveMerge{BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "extra"}, Position: model.Point{X: 700, Y: 200}}})
		}, layoutStatements{}, "objects"},
		{"a stated position the stored node is not at", func(oc *microflows.MicroflowObjectCollection) {},
			layoutStatements{points: map[model.Point]bool{{X: 530, Y: 300}: true}}, "Position"},
		{"a redrawn connector where the script states anchors", func(oc *microflows.MicroflowObjectCollection) {
			oc.Flows[0].OriginConnectionIndex = 3
		}, layoutStatements{anchors: true}, "OriginConnectionIndex"},
		{"a curve", func(oc *microflows.MicroflowObjectCollection) {
			oc.Flows[0].OriginControlVector = "40;-90"
		}, layoutStatements{}, "OriginControlVector"},
	} {
		t.Run(c.name, func(t *testing.T) {
			built, stored := guardGraph("b-"), guardGraph("s-")
			driftAll(stored)
			c.edit(stored)
			same, why := sameBuiltFlowUpToLayout(built, stored, c.stated)
			if same {
				t.Fatal("reported the same graph")
			}
			if !strings.Contains(why, c.want) {
				t.Errorf("why = %q, want it to name %q", why, c.want)
			}
		})
	}
}

// Two objects of one type that no flow reaches cannot be paired without
// their positions, so they are never a match.
func TestSameBuiltFlowUpToLayout_UnpairableIsADifference(t *testing.T) {
	built, stored := guardGraph("b-"), guardGraph("s-")
	for _, oc := range []*microflows.MicroflowObjectCollection{built, stored} {
		oc.Objects = append(oc.Objects, &microflows.StartEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: oc.ID + "-start2"}, Position: model.Point{X: 100, Y: 500}}})
	}
	if same, why := sameBuiltFlowUpToLayout(built, stored, layoutStatements{}); same || !strings.Contains(why, "unconnected") {
		t.Fatalf("same = %v, why = %q; want an unpairable difference", same, why)
	}
}
