// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2166 (mendixlabs/mxcli#1827): the warning for a note linked to
// another note quoted the note's text verbatim. A text with a line break ended
// the `--` comment mid-quote, the rest of the line parsed as MDL, and both
// `check` on the description and the splice's comparison against the stored
// flow failed. The control is the same flow with single-line texts.
func TestUnattachedNote_MultiLineTextKeepsTheWarningOneLine(t *testing.T) {
	for _, tc := range []struct{ name, n1, n2 string }{
		{"single-line control", "Note1", "Note2"},
		{"multi-line", "Note1\nwith multiple lines", "Note2\r\nwith multiple lines"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n1 := note("note1", tc.n1, model.Point{X: 78, Y: -42}, model.Size{Width: 230, Height: 50})
			n2 := note("note2", tc.n2, model.Point{X: 383, Y: 89}, model.Size{Width: 230, Height: 50})
			flows := append(sharedNote2Flows(), &microflows.AnnotationFlow{OriginID: "note1", DestinationID: "note2"})
			body := describeWithFreeNotes(t, mfWithNotes(flows, n1, n2))

			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "-- WARNING") && strings.Count(line, "'")%2 != 0 {
					t.Errorf("the warning comment is cut mid-quote: %q", line)
				}
			}
			if !strings.Contains(body, "-- WARNING") {
				t.Fatalf("the note-to-note link is not flagged:\n%s", body)
			}
			notes, _, errs := rebuildFromBody(t, body)
			if len(errs) > 0 {
				t.Fatalf("the description does not parse: %v\n%s", errs, body)
			}
			got := map[string]bool{}
			for _, n := range notes {
				got[n.Caption] = true
			}
			if !got[tc.n1] {
				t.Errorf("Note1's text did not survive the round trip: %v", got)
			}
		})
	}
}
