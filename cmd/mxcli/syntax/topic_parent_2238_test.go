// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"bytes"
	"strings"
	"testing"
)

// ako/mxcli#2238 (mendixlabs/mxcli#1852): `mxcli syntax workflow user-task`
// printed an index of workflow.user-task and its one subtopic instead of the
// user task syntax; the index's own hint suggested that very command. A nested
// topic that has subtopics now prints its own detail, then lists them. The
// control is a leaf topic, and a top-level area, which stays an index.
func TestMatchWriteText_ParentTopicShowsItsDetail(t *testing.T) {
	render := func(args ...string) string {
		var b bytes.Buffer
		Lookup(args).WriteText(&b)
		return b.String()
	}

	for _, args := range [][]string{{"workflow", "user-task"}, {"workflow.user-task"}} {
		out := render(args...)
		if !strings.HasPrefix(out, "workflow.user-task\n═") {
			t.Errorf("%v: want the user task detail first, got:\n%s", args, out)
		}
		if !strings.Contains(out, "Syntax:") || !strings.Contains(strings.ToUpper(out), "OUTCOMES") {
			t.Errorf("%v: the user task syntax is missing:\n%s", args, out)
		}
		if !strings.Contains(out, "workflow.user-task.targeting") {
			t.Errorf("%v: the subtopic is not listed:\n%s", args, out)
		}
	}

	// Control: a leaf topic is unchanged.
	if out := render("workflow", "user-task", "targeting"); !strings.HasPrefix(out, "workflow.user-task.targeting\n═") {
		t.Errorf("leaf topic: got:\n%s", out)
	}
	// Control: a top-level area stays an index of its topics.
	if out := render("workflow"); strings.Contains(out, "Syntax:") || !strings.Contains(out, "workflow.user-task ") {
		t.Errorf("top-level area: want an index, got:\n%s", out)
	}
}
