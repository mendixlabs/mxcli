// SPDX-License-Identifier: Apache-2.0

// Regression tests for issue #281: DESCRIBE MICROFLOW silently truncated the
// output at an ExclusiveMerge used as a pure junction (e.g., the loop-back
// target of a manual retry-loop pattern, visible in the screenshot attached
// to the issue). The describer used to early-return on every merge, but
// merges that aren't the matching end point of an IF/ELSE block must be
// traversed as pass-through or their successors are dropped.
package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestTraverseFlow_UnpairedMergeIsPassThrough(t *testing.T) {
	// Flow graph: start → act1 → merge → act2 → end.
	// The merge has no matching split (splitMergeMap is empty), so it's a
	// pure junction, not the end of an IF/ELSE.
	e := newTestExecutor()
	activityMap := map[model.ID]microflows.MicroflowObject{
		mkID("start"): &microflows.StartEvent{BaseMicroflowObject: mkObj("start")},
		mkID("act1"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("act1")},
			Action: &microflows.LogMessageAction{
				LogLevel:        "Info",
				LogNodeName:     "'App'",
				MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "before merge"}},
			},
		},
		mkID("merge"): &microflows.ExclusiveMerge{BaseMicroflowObject: mkObj("merge")},
		mkID("act2"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("act2")},
			Action: &microflows.LogMessageAction{
				LogLevel:        "Info",
				LogNodeName:     "'App'",
				MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "after merge"}},
			},
		},
		mkID("end"): &microflows.EndEvent{BaseMicroflowObject: mkObj("end")},
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("start"): {mkFlow("start", "act1")},
		mkID("act1"):  {mkFlow("act1", "merge")},
		mkID("merge"): {mkFlow("merge", "act2")},
		mkID("act2"):  {mkFlow("act2", "end")},
	}

	var lines []string
	visited := make(map[model.ID]bool)
	// Empty splitMergeMap — the merge isn't paired with any split.
	e.traverseFlow(mkID("start"), activityMap, flowsByOrigin, nil, visited, nil, nil, &lines, 0, nil, 0, nil)

	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "before merge") {
		t.Errorf("missing `before merge` activity (should always be emitted):\n%s", out)
	}
	if !strings.Contains(out, "after merge") {
		t.Errorf("issue #281: `after merge` activity was dropped — describer stopped at unpaired merge:\n%s", out)
	}
	// The merge itself must not produce an `end if;` here — it's a
	// junction, not the closing bracket of an IF/ELSE.
	if strings.Contains(out, "end if;") {
		t.Errorf("unpaired merge must not emit `end if;`:\n%s", out)
	}
}

func TestTraverseFlow_PairedMergeStillClosesIfElse(t *testing.T) {
	// Regression guard for the partnered case: a merge that IS paired with
	// a split must still be handled by the split's branch logic (not by the
	// new pass-through path). Builds a standard IF/ELSE:
	//
	//   start → split(true→thenAct, false→elseAct) → merge → end
	e := newTestExecutor()
	activityMap := map[model.ID]microflows.MicroflowObject{
		mkID("start"): &microflows.StartEvent{BaseMicroflowObject: mkObj("start")},
		mkID("split"): &microflows.ExclusiveSplit{
			BaseMicroflowObject: mkObj("split"),
			SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$x > 0"},
		},
		mkID("thenAct"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("thenAct")},
			Action: &microflows.LogMessageAction{
				LogLevel:        "Info",
				LogNodeName:     "'App'",
				MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "yes"}},
			},
		},
		mkID("elseAct"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("elseAct")},
			Action: &microflows.LogMessageAction{
				LogLevel:        "Info",
				LogNodeName:     "'App'",
				MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "no"}},
			},
		},
		mkID("merge"): &microflows.ExclusiveMerge{BaseMicroflowObject: mkObj("merge")},
		mkID("end"):   &microflows.EndEvent{BaseMicroflowObject: mkObj("end")},
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("start"):   {mkFlow("start", "split")},
		mkID("split"):   {mkBranchFlow("split", "thenAct", microflows.EnumerationCase{Value: "true"}), mkBranchFlow("split", "elseAct", microflows.EnumerationCase{Value: "false"})},
		mkID("thenAct"): {mkFlow("thenAct", "merge")},
		mkID("elseAct"): {mkFlow("elseAct", "merge")},
		mkID("merge"):   {mkFlow("merge", "end")},
	}

	splitMergeMap := map[model.ID]model.ID{
		mkID("split"): mkID("merge"),
	}

	var lines []string
	visited := make(map[model.ID]bool)
	e.traverseFlow(mkID("start"), activityMap, flowsByOrigin, splitMergeMap, visited, nil, nil, &lines, 0, nil, 0, nil)

	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "if $x > 0 then") {
		t.Errorf("missing IF header:\n%s", out)
	}
	if !strings.Contains(out, "else") {
		t.Errorf("missing else branch:\n%s", out)
	}
	if !strings.Contains(out, "end if;") {
		t.Errorf("missing `end if;` — paired merge must still close the IF:\n%s", out)
	}
	// The merge should produce exactly one `end if;`, not two.
	if count := strings.Count(out, "end if;"); count != 1 {
		t.Errorf("expected exactly one `end if;`, got %d:\n%s", count, out)
	}
}

func TestTraverseFlow_ManualRetryLoopPatternEmitsEverything(t *testing.T) {
	// Closer reproduction of the user's microflow from issue #281. The retry
	// pattern wires a back-edge from a "change retry count + delay" branch
	// into a merge that sits between the setup activities and the REST call,
	// so control re-enters the REST call every retry iteration.
	//
	//        start → setup → merge → call → decide
	//                         ↑                ↓ (retry)
	//                        delay ← change ←──┘
	//
	// Before the fix the describer emitted setup and stopped at the merge,
	// dropping call, decide, change and delay — exactly what the issue
	// reported.
	e := newTestExecutor()
	activityMap := map[model.ID]microflows.MicroflowObject{
		mkID("start"): &microflows.StartEvent{BaseMicroflowObject: mkObj("start")},
		mkID("setup"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("setup")},
			Action: &microflows.LogMessageAction{
				LogLevel:        "Info",
				LogNodeName:     "'App'",
				MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "setup"}},
			},
		},
		mkID("merge"): &microflows.ExclusiveMerge{BaseMicroflowObject: mkObj("merge")},
		mkID("call"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("call")},
			Action: &microflows.LogMessageAction{
				LogLevel:        "Info",
				LogNodeName:     "'App'",
				MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "call-rest"}},
			},
		},
		mkID("decide"): &microflows.ExclusiveSplit{
			BaseMicroflowObject: mkObj("decide"),
			SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$retry"},
		},
		mkID("change"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("change")},
			Action: &microflows.LogMessageAction{
				LogLevel:        "Info",
				LogNodeName:     "'App'",
				MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "change-retry-count"}},
			},
		},
		mkID("delay"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("delay")},
			Action: &microflows.LogMessageAction{
				LogLevel:        "Info",
				LogNodeName:     "'App'",
				MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "delay"}},
			},
		},
		mkID("end"): &microflows.EndEvent{BaseMicroflowObject: mkObj("end")},
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("start"):  {mkFlow("start", "setup")},
		mkID("setup"):  {mkFlow("setup", "merge")},
		mkID("merge"):  {mkFlow("merge", "call")},
		mkID("call"):   {mkFlow("call", "decide")},
		mkID("decide"): {mkBranchFlow("decide", "change", microflows.EnumerationCase{Value: "true"}), mkBranchFlow("decide", "end", microflows.EnumerationCase{Value: "false"})},
		mkID("change"): {mkFlow("change", "delay")},
		mkID("delay"):  {mkFlow("delay", "merge")},
	}

	var lines []string
	visited := make(map[model.ID]bool)
	// The `decide` split has no merge target (both branches leave the loop),
	// so splitMergeMap is empty.
	e.traverseFlow(mkID("start"), activityMap, flowsByOrigin, nil, visited, nil, nil, &lines, 0, nil, 0, nil)

	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "while true") || !strings.Contains(out, "end while;") {
		t.Errorf("manual retry loop must be emitted as a while-true block so exec rebuilds the back-edge:\n%s", out)
	}
	for _, want := range []string{"setup", "call-rest", "if $retry then", "change-retry-count", "delay"} {
		if !strings.Contains(out, want) {
			t.Errorf("issue #281: missing %q from describe output:\n%s", want, out)
		}
	}
}
