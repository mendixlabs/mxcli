// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1308 (ako/mxcli#1299): a Starlark rule could not see the
// sequence flows of a microflow — which object leads to which, which flow is
// an error handler, which branch of a split is `true`, which activity an
// annotation is attached to — nor where an object sits on the canvas. The
// reader decodes all of it (describe prints `on error`, `@annotation` and
// `@position` from it); the catalog wrote none of it.

func placed[T microflows.MicroflowObject](id string, x, y, w, h int, o T) T {
	o = loopObj(id, o)
	o.SetPosition(model.Point{X: x, Y: y})
	if s, ok := any(o).(interface{ SetSize(model.Size) }); ok {
		s.SetSize(model.Size{Width: w, Height: h})
	}
	return o
}

func seqFlow(id, from, to string, cv microflows.CaseValue, errHandler bool, oi, di int) *microflows.SequenceFlow {
	f := &microflows.SequenceFlow{
		OriginID: model.ID(from), DestinationID: model.ID(to),
		OriginConnectionIndex: oi, DestinationConnectionIndex: di,
		CaseValue: cv, IsErrorHandler: errHandler,
	}
	f.ID = model.ID(id)
	return f
}

// flowsFixture is the issue's microflow, plus an enumeration split, an
// inheritance split and a loop whose body flows sit in the loop's own
// collection (older models; 11.12 keeps them on the microflow):
//
//	start -> @annotation change -> commit (on error -> log warning)
//	      -> if (true -> commit2, false -> merge) -> case (Open, (empty))
//	      -> loop { change2 -> commit3 } -> end
func flowsFixture() *microflows.MicroflowObjectCollection {
	loop := placed("loop", 1500, 200, 300, 150, &microflows.LoopedActivity{
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{
				placed("change2", 1450, 200, 120, 60, &microflows.ActionActivity{Action: &microflows.ChangeObjectAction{}}),
				placed("commit3", 1550, 200, 120, 60, &microflows.ActionActivity{Action: &microflows.CommitObjectsAction{}}),
			},
			Flows: []*microflows.SequenceFlow{seqFlow("f-loop", "change2", "commit3", nil, false, 1, 3)},
		},
	})
	note := &microflows.AnnotationFlow{OriginID: "note", DestinationID: "change", Sides: &microflows.ConnectionSides{Origin: 2, Destination: 0}}
	note.ID = "af-note"
	return &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			placed("start", 200, 200, 20, 20, &microflows.StartEvent{}),
			placed("note", 360, 100, 200, 50, &microflows.Annotation{Caption: "Amount is reset before storing"}),
			placed("change", 360, 200, 120, 60, &microflows.ActionActivity{Action: &microflows.ChangeObjectAction{}}),
			placed("commit", 500, 200, 120, 60, &microflows.ActionActivity{Action: &microflows.CommitObjectsAction{}}),
			placed("log", 500, 330, 120, 60, &microflows.ActionActivity{Action: &microflows.LogMessageAction{LogLevel: "Warning"}}),
			placed("split", 700, 200, 120, 60, &microflows.ExclusiveSplit{}),
			placed("commit2", 700, 330, 120, 60, &microflows.ActionActivity{Action: &microflows.CommitObjectsAction{}}),
			placed("merge", 900, 200, 30, 30, &microflows.ExclusiveMerge{}),
			placed("enum", 1100, 200, 120, 60, &microflows.ExclusiveSplit{}),
			placed("isplit", 1300, 200, 120, 60, &microflows.InheritanceSplit{}),
			loop,
			placed("end", 1800, 200, 20, 20, &microflows.EndEvent{}),
		},
		Flows: []*microflows.SequenceFlow{
			seqFlow("f1", "start", "change", &microflows.NoCase{}, false, 1, 3),
			seqFlow("f2", "change", "commit", nil, false, 1, 3),
			seqFlow("f3", "commit", "log", nil, true, 2, 0),
			seqFlow("f4", "commit", "split", nil, false, 1, 3),
			seqFlow("f5", "split", "commit2", &microflows.EnumerationCase{Value: "true"}, false, 2, 0),
			seqFlow("f6", "split", "merge", &microflows.EnumerationCase{Value: "false"}, false, 1, 3),
			seqFlow("f7", "merge", "enum", nil, false, 1, 3),
			seqFlow("f8", "enum", "isplit", &microflows.EnumerationCase{Value: "Open"}, false, 1, 3),
			seqFlow("f9", "enum", "isplit", &microflows.EnumerationCase{Value: "(empty)"}, false, 2, 0),
			seqFlow("f10", "isplit", "loop", &microflows.InheritanceCase{EntityQualifiedName: "Loops.Special"}, false, 1, 3),
			seqFlow("f11", "loop", "end", &microflows.BooleanCase{Value: false}, false, 1, 3),
		},
		AnnotationFlows: []*microflows.AnnotationFlow{note},
	}
}

func flowRowsOf(t *testing.T, cat *Catalog, flow string) []string {
	t.Helper()
	res, err := cat.Query(`SELECT Id, FlowType, OriginId, DestinationId, IsErrorHandler, CaseValue,
			OriginConnectionIndex, DestinationConnectionIndex, MicroflowId, ModuleName
		FROM flows WHERE MicroflowQualifiedName = '` + flow + `' ORDER BY Sequence`)
	if err != nil {
		t.Fatalf("query flows: %v", err)
	}
	var out []string
	for _, r := range res.Rows {
		out = append(out, fmt.Sprintf("%v|%v|%v|%v|%v|%v|%v|%v|%v|%v", r...))
	}
	return out
}

func assertFlowRows(t *testing.T, cat *Catalog, flow, flowID string) {
	t.Helper()
	sf := func(id, from, to string, eh int, cv string, oi, di int) string {
		return fmt.Sprintf("%s|SequenceFlow|%s|%s|%d|%s|%d|%d|%s|Loops", id, from, to, eh, cv, oi, di, flowID)
	}
	want := []string{
		sf("f1", "start", "change", 0, "", 1, 3),
		sf("f2", "change", "commit", 0, "", 1, 3),
		sf("f3", "commit", "log", 1, "", 2, 0),
		sf("f4", "commit", "split", 0, "", 1, 3),
		sf("f5", "split", "commit2", 0, "true", 2, 0),
		sf("f6", "split", "merge", 0, "false", 1, 3),
		sf("f7", "merge", "enum", 0, "", 1, 3),
		sf("f8", "enum", "isplit", 0, "Open", 1, 3),
		sf("f9", "enum", "isplit", 0, "(empty)", 2, 0),
		sf("f10", "isplit", "loop", 0, "Loops.Special", 1, 3),
		sf("f11", "loop", "end", 0, "false", 1, 3),
		"af-note|AnnotationFlow|note|change|0||2|0|" + flowID + "|Loops",
		sf("f-loop", "change2", "commit3", 0, "", 1, 3),
	}
	got := flowRowsOf(t, cat, flow)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s flows:\n%s\nwant:\n%s", flow, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestFlowsTable_Issue1299(t *testing.T) {
	mf := &microflows.Microflow{ContainerID: loopTestModule, Name: "Act_Store", ObjectCollection: flowsFixture()}
	mf.ID = "mf-store"
	nf := &microflows.Nanoflow{ContainerID: loopTestModule, Name: "NF_Store", ObjectCollection: flowsFixture()}
	nf.ID = "nf-store"
	rule := &microflows.Rule{ContainerID: loopTestModule, Name: "R_Store", ObjectCollection: flowsFixture()}
	rule.ID = "rule-store"
	// The three share the fixture's ids, so each is built on its own catalog.
	assertFlowRows(t, buildFlowsForTest(t, []*microflows.Microflow{mf}, nil, nil), "Loops.Act_Store", "mf-store")
	assertFlowRows(t, buildFlowsForTest(t, nil, []*microflows.Nanoflow{nf}, nil), "Loops.NF_Store", "nf-store")
	assertFlowRows(t, buildFlowsForTest(t, nil, nil, []*microflows.Rule{rule}), "Loops.R_Store", "rule-store")
}

// Each activity's canvas box: X / Y are the stored middle point (what
// describe prints as @position), Width / Height its size.
func TestActivityPosition_Issue1299(t *testing.T) {
	mf := &microflows.Microflow{ContainerID: loopTestModule, Name: "Act_Store", ObjectCollection: flowsFixture()}
	mf.ID = "mf-store"
	cat := buildFlowsForTest(t, []*microflows.Microflow{mf}, nil, nil)
	res, err := cat.Query(`SELECT Id, X, Y, Width, Height FROM activities
		WHERE Id IN ('start', 'change', 'note', 'merge', 'commit3') ORDER BY Sequence`)
	if err != nil {
		t.Fatalf("query activities: %v", err)
	}
	var got []string
	for _, r := range res.Rows {
		got = append(got, fmt.Sprintf("%v|%v|%v|%v|%v", r...))
	}
	want := []string{
		"start|200|200|20|20",
		"note|360|100|200|50",
		"change|360|200|120|60",
		"merge|900|200|30|30",
		"commit3|1550|200|120|60",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("activity boxes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
