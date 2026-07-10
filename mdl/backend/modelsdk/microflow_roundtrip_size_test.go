// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestMicroflowRoundTrip_NodeSize guards issue #723 §A: the read path carried a
// flow object's Position but not its Size, so every activity/decision was
// rewritten with size "0;0". Studio Pro then rendered each node as a 1-px sliver
// (the caption wrapping one letter per line). mx check has no error for this —
// it ignores box size — so the corruption was silent.
func TestMicroflowRoundTrip_NodeSize(t *testing.T) {
	act := &microflows.ActionActivity{}
	act.ID = model.ID("act-1")
	act.Position = model.Point{X: 200, Y: 100}
	act.Size = model.Size{Width: 120, Height: 60}

	mf := &microflows.Microflow{
		Name: "ACT_Test",
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{act},
		},
	}
	mf.ID = model.ID("mf-1")

	got := roundTripMicroflow(t, mf)
	if got.ObjectCollection == nil || len(got.ObjectCollection.Objects) != 1 {
		t.Fatalf("round-trip lost the activity: %+v", got.ObjectCollection)
	}
	ra, ok := got.ObjectCollection.Objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("object[0] = %T, want *microflows.ActionActivity", got.ObjectCollection.Objects[0])
	}
	if ra.Size.Width != 120 || ra.Size.Height != 60 {
		t.Errorf("Size = %dx%d, want 120x60 (was reset to 0;0 → 1-px sliver in Studio Pro)", ra.Size.Width, ra.Size.Height)
	}
}
