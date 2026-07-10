// SPDX-License-Identifier: Apache-2.0

// Regression test for the missing `begin` keyword in LOOP/WHILE describe
// output. The MDL grammar requires `loop $X in $Y begin ... end loop;` but
// the describer emitted the loop header and body without a `begin` between
// them. It used to accidentally parse because the first body statement's
// leading keyword was accepted after `loop`. Once @anchor annotations
// started landing before the first body statement, the parser saw a bare
// `@position(...)` immediately after `loop` and produced cascade errors.
package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestTraverseFlow_LoopEmitsBegin(t *testing.T) {
	e := newTestExecutor()

	// start -> loop -> end, with a single log activity in the loop body.
	innerActivity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("loop-log")},
		Action: &microflows.LogMessageAction{
			LogLevel:    "info",
			LogNodeName: "'App'",
			MessageTemplate: &model.Text{
				Translations: map[string]string{"en_US": "hi"},
			},
		},
	}
	loopStart := &microflows.StartEvent{BaseMicroflowObject: mkObj("loop-start")}
	loopEnd := &microflows.EndEvent{BaseMicroflowObject: mkObj("loop-end")}

	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: mkObj("loop"),
		LoopSource: &microflows.IterableList{
			BaseElement:      model.BaseElement{ID: mkID("src")},
			VariableName:     "item",
			ListVariableName: "items",
		},
		ObjectCollection: &microflows.MicroflowObjectCollection{
			BaseElement: model.BaseElement{ID: mkID("oc")},
			Objects:     []microflows.MicroflowObject{loopStart, innerActivity, loopEnd},
			Flows: []*microflows.SequenceFlow{
				mkFlow("loop-start", "loop-log"),
				mkFlow("loop-log", "loop-end"),
			},
		},
	}

	activityMap := map[model.ID]microflows.MicroflowObject{
		mkID("start"): &microflows.StartEvent{BaseMicroflowObject: mkObj("start")},
		mkID("loop"):  loop,
		mkID("end"):   &microflows.EndEvent{BaseMicroflowObject: mkObj("end")},
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("start"): {mkFlow("start", "loop")},
		mkID("loop"):  {mkFlow("loop", "end")},
	}

	var lines []string
	visited := make(map[model.ID]bool)
	e.traverseFlow(mkID("start"), activityMap, flowsByOrigin, nil, visited, nil, nil, &lines, 0, nil, 0, nil)

	// The output must include both a `loop $X in $Y` header and a `begin`
	// line before the body.
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "loop $item in $items") {
		t.Errorf("missing loop header\n%s", joined)
	}
	// The `begin` line must appear between the loop header and the body.
	loopIdx := strings.Index(joined, "loop $item in $items")
	beginIdx := strings.Index(joined, "begin")
	endIdx := strings.Index(joined, "end loop;")
	if loopIdx < 0 || beginIdx < 0 || endIdx < 0 {
		t.Fatalf("missing loop/begin/end loop. Output:\n%s", joined)
	}
	if !(loopIdx < beginIdx && beginIdx < endIdx) {
		t.Errorf("expected order: loop header < begin < end loop;. Got:\n%s", joined)
	}
}
