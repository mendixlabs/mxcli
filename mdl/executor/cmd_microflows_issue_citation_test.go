// SPDX-License-Identifier: Apache-2.0

package executor

// ako/mxcli#1005: the describe warnings cited "mxcli #923", which in the fork
// these builds ship from is an unrelated MCP-backend issue. The issue they mean
// is mendixlabs/mxcli#923 ("roundtrip for @merge does not work"), so every
// user-facing citation has to carry the owner.

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func assertCitesUpstream923(t *testing.T, what, text string) {
	t.Helper()
	if !strings.Contains(text, "mendixlabs/mxcli#923") {
		t.Errorf("%s does not cite mendixlabs/mxcli#923: %q", what, text)
	}
	if strings.Contains(text, "mxcli #923") {
		t.Errorf("%s still cites the ambiguous \"mxcli #923\": %q", what, text)
	}
}

func TestIssueCitation_IrreducibleWarningNamesUpstream(t *testing.T) {
	got := irreducibleGraphWarnings(reporterCollection(), nil)
	if len(got) != 1 {
		t.Fatalf("want 1 warning, got %d: %v", len(got), got)
	}
	assertCitesUpstream923(t, "the non-nesting decision warning", got[0])
}

func TestIssueCitation_DroppedMergeWarningNamesUpstream(t *testing.T) {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("act", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(100)}})
	f.add("merge", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(200)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(300)})
	f.edge("start", "act", false)
	f.edge("act", "merge", false)
	f.edge("merge", "end", false)

	got := droppedMergeWarnings(nil, f.col, labelRejoinMerges(f.col))
	if len(got) != 1 {
		t.Fatalf("want 1 warning, got %d: %v", len(got), got)
	}
	assertCitesUpstream923(t, "the unrepresented-merge warning", got[0])
}

func TestIssueCitation_NormalizedNoteNamesUpstream(t *testing.T) {
	_, notes := normalizeCollection(reporterGraph("$B").col)
	if len(notes) != 1 {
		t.Fatalf("want 1 note, got %v", notes)
	}
	assertCitesUpstream923(t, "the NORMALIZED note", notes[0])
}

// The bug-test script's flow (mdl-examples/bug-tests/1005-…) is the reporter's
// graph written with join points; NORMALIZED folds it and the note carries
// the citation, as the script's header says.
func TestIssueCitation_BugTestScriptNormalizes(t *testing.T) {
	oc := buildMicroflowFromMDL(t, `create microflow BugTest1005.Crossing ($A: Boolean, $B: Boolean)
begin
  if $A then
    if $B then
      join shared_log;
    else
      join after_log;
    end if;
  else
    join shared_log;
  end if;
  merge shared_log;
  log info node 'BugTest1005' 'shared';
  join after_log;
  merge after_log;
end;`)
	_, notes := normalizeCollection(oc)
	if len(notes) != 1 {
		t.Fatalf("want 1 NORMALIZED note, got %v", notes)
	}
	assertCitesUpstream923(t, "the NORMALIZED note on the bug-test flow", notes[0])
}
