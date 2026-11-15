// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"
	"testing"
)

// ako/mxcli#2204: a guard written with an empty `else` was laid out as an
// IF/ELSE. The unhappy branch took the main row (log at 530;200, its end at
// 690;200) and the happy path moved to 840;200, so the decision's false flow
// was drawn straight through the branch and its end event. Without the `else`
// the branch sits on its own row (530;300). The empty `else` adds nothing to
// the model, so the layout must be the same.
const guardFlowTemplate = `mdl 1;
create microflow MyFirstModule.ACT_Guard (Amount: Integer) returns Boolean
begin
  if $Amount < 0 then
    log error 'negative amount';
    return false;
  %s end if;
  log info 'processing';
  return true;
end;`

func layoutLabels(t *testing.T, src string) []string {
	t.Helper()
	var got []string
	for _, o := range buildLayout(t, src).Objects {
		got = append(got, canvasLabel(o))
	}
	sort.Strings(got)
	return got
}

func TestEmptyElseGuard_LaidOutAsTheGuard_Issue2204(t *testing.T) {
	noElse := layoutLabels(t, strings.Replace(guardFlowTemplate, "%s", "", 1))
	emptyElse := layoutLabels(t, strings.Replace(guardFlowTemplate, "%s", "else", 1))
	if strings.Join(noElse, "\n") != strings.Join(emptyElse, "\n") {
		t.Errorf("an empty else changed the layout:\n--- no else\n%s\n--- empty else\n%s",
			strings.Join(noElse, "\n"), strings.Join(emptyElse, "\n"))
	}
	// The measured guard shape: the unhappy branch on its own row, the main
	// line straight.
	for _, want := range []string{"ActionActivity 530;300", "EndEvent 690;300", "ActionActivity 530;200"} {
		found := false
		for _, g := range emptyElse {
			found = found || g == want
		}
		if !found {
			t.Errorf("empty else: missing %s in\n%s", want, strings.Join(emptyElse, "\n"))
		}
	}
	if overlaps := canvasOverlaps(buildLayout(t, strings.Replace(guardFlowTemplate, "%s", "else", 1)).Objects); len(overlaps) > 0 {
		t.Errorf("objects share a place:\n  %s", strings.Join(overlaps, "\n  "))
	}
}

// CONTROL: an empty `else` after a THEN that carries on still meets at a merge,
// and keeps the IF/ELSE layout it had.
func TestEmptyElseAfterContinuingThen_Unchanged_Issue2204(t *testing.T) {
	const src = `mdl 1;
create microflow MyFirstModule.ACT_Branch (Amount: Integer) returns Boolean
begin
  if $Amount < 0 then
    log error 'negative amount';
  else
  end if;
  log info 'processing';
  return true;
end;`
	oc := buildLayout(t, src)
	merges := 0
	for _, o := range oc.Objects {
		if typeShort(o) == "ExclusiveMerge" {
			merges++
		}
	}
	if merges != 1 {
		t.Errorf("got %d merges, want 1", merges)
	}
}
