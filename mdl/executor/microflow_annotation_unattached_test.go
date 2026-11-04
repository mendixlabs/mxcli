// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1933 (mendixlabs/mxcli#1731): "annotations not linked to an
// activity are missing in MDL". The reporter's flow has two notes; Note2 is
// wired to two activities and is described, Note1 is not described at all.
//
// A note with no AnnotationFlow was already printed as a free note. What
// vanished was a note whose only links go where the describer never prints an
// @annotation: to the start event, or to another note. Such a note counted as
// "referenced", so it was not free, and its target emits nothing, so it was
// not attached either.
//
// Such a note is now described as a free note — which is also what makes the
// canonical describe keep the first statement's @position (useDerivedFlowLayout
// asks collectFreeAnnotations), so it comes back free — and the link MDL cannot
// state is flagged rather than dropped in silence.

func describeWithFreeNotes(t *testing.T, mf *microflows.Microflow) string {
	t.Helper()
	ctx := &ExecContext{}
	lines := formatMicroflowActivities(ctx, mf, nil, nil)
	return strings.Join(prependFreeAnnotationLines(ctx, mf.ObjectCollection, lines), "\n")
}

func reporterNotes() (*microflows.Annotation, *microflows.Annotation) {
	return note("note1", "Note1", model.Point{X: 100, Y: 300}, model.Size{Width: 200, Height: 50}),
		note("note2", "Note2", model.Point{X: 383, Y: 89}, model.Size{Width: 230, Height: 50})
}

func sharedNote2Flows() []*microflows.AnnotationFlow {
	return []*microflows.AnnotationFlow{
		{BaseElement: model.BaseElement{ID: "af1"}, OriginID: "note2", DestinationID: "a1"},
		{BaseElement: model.BaseElement{ID: "af2"}, OriginID: "note2", DestinationID: "a2"},
	}
}

func TestUnattachedNote_EveryNoteIsDescribed(t *testing.T) {
	for name, extra := range map[string]*microflows.AnnotationFlow{
		"linked to the start event": {OriginID: "note1", DestinationID: "start"},
		"linked to another note":    {OriginID: "note1", DestinationID: "note2"},
		"linked to nothing":         nil,
	} {
		t.Run(name, func(t *testing.T) {
			flows := sharedNote2Flows()
			if extra != nil {
				flows = append(flows, extra)
			}
			n1, n2 := reporterNotes()
			body := describeWithFreeNotes(t, mfWithNotes(flows, n1, n2))
			if !strings.Contains(body, "'Note1'") {
				t.Fatalf("Note1 is missing from the description:\n%s", body)
			}

			notes, rebuiltFlows, errs := rebuildFromBody(t, body)
			if len(errs) > 0 {
				t.Fatalf("rebuild reported errors: %v\n%s", errs, body)
			}
			if len(notes) != 2 {
				t.Fatalf("rebuilt %d notes, want 2:\n%s", len(notes), body)
			}
			// Note2 keeps its two links; Note1 comes back unattached.
			byNote := map[string]int{}
			captions := map[model.ID]string{}
			for _, n := range notes {
				captions[n.ID] = n.Caption
			}
			for _, f := range rebuiltFlows {
				byNote[captions[f.OriginID]]++
			}
			if byNote["Note2"] != 2 || byNote["Note1"] != 0 {
				t.Errorf("links per note = %v, want Note2: 2, Note1: 0\n%s", byNote, body)
			}
		})
	}
}

// A link MDL cannot state is not dropped in silence: the description says so.
func TestUnattachedNote_UnstatableLinkIsFlagged(t *testing.T) {
	n1, n2 := reporterNotes()
	flows := append(sharedNote2Flows(), &microflows.AnnotationFlow{OriginID: "note1", DestinationID: "start"})
	body := describeWithFreeNotes(t, mfWithNotes(flows, n1, n2))
	if !strings.Contains(body, "-- WARNING") || !strings.Contains(body, "start") {
		t.Errorf("the note's link to the start event is lost without a warning:\n%s", body)
	}

	// Control: a note linked to nothing has nothing to lose, so no warning.
	n1, n2 = reporterNotes()
	body = describeWithFreeNotes(t, mfWithNotes(sharedNote2Flows(), n1, n2))
	if strings.Contains(body, "-- WARNING") {
		t.Errorf("a free note was flagged:\n%s", body)
	}
}
