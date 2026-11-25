// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A branch's last activity enters the closing merge on the side it comes from
// (ako/mxcli#2593). The edge was always drawn into the merge's bottom, which is
// right for a branch in the lane below and wrong for one placed on the merge's
// line: the true edge of an if without else, its log at (320;200) and the merge
// at (480;200), looped under the merge. A merge's entry side is not written by
// DESCRIBE — the merge has no statement — so the builder is the only place it
// can come from.
func TestBranchTailEntersMergeFromItsSide_2593(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		{"true branch on the merge's line, false to bottom", `create microflow Mod.ACT_merge_to_bottom ($Flag: Boolean)
begin
  @start(0, 200)
  @position(160, 200)
  @anchor(true: (from: right), false: (from: bottom, to: bottom))
  @merge(480, 200)
  if $Flag then
    @position(320, 200)
    log info node 'Repro' 'true';
  end if;
  @position(640, 200)
  return;
end;`, AnchorLeft},
		{"true branch on the merge's line, false default", `create microflow Mod.ACT_merge_to_bottom2 ($Flag: Boolean)
begin
  @start(0, 200)
  @position(160, 200)
  @anchor(true: (from: right), false: (from: bottom))
  @merge(480, 200)
  if $Flag then
    @position(320, 200)
    log info node 'Repro' 'true';
  end if;
  @position(640, 200)
  return;
end;`, AnchorLeft},
		// The control: the default layout puts the true branch in the lane
		// below, and it still comes up into the merge's bottom.
		{"true branch in the lane below", `create microflow Mod.ACT_merge_below ($Flag: Boolean)
begin
  if $Flag then
    log info node 'Repro' 'true';
  end if;
  return;
end;`, AnchorBottom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oc := buildMicroflowFromMDL(t, tc.src)
			var merge *microflows.ExclusiveMerge
			var log microflows.MicroflowObject
			for _, o := range oc.Objects {
				switch v := o.(type) {
				case *microflows.ExclusiveMerge:
					merge = v
				case *microflows.ActionActivity:
					if _, ok := v.Action.(*microflows.LogMessageAction); ok {
						log = v
					}
				}
			}
			if merge == nil || log == nil {
				t.Fatalf("merge %v, log %v", merge, log)
			}
			for _, f := range oc.Flows {
				if f.OriginID == log.GetID() && f.DestinationID == merge.ID {
					if f.OriginConnectionIndex != AnchorRight || f.DestinationConnectionIndex != tc.want {
						t.Fatalf("log -> merge: from %s to %s, want right -> %s", anchorSideKeyword(f.OriginConnectionIndex),
							anchorSideKeyword(f.DestinationConnectionIndex), anchorSideKeyword(tc.want))
					}
					return
				}
			}
			t.Fatal("no log -> merge flow")
		})
	}
}
