// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestHasExplicitFalseBranchAnchor pins ako's review follow-up for PR #364:
// the heuristic that decides whether a false-branch flow carries an explicit
// anchor annotation should fire only on the AnchorTop→AnchorBottom pair and
// must stay dormant for nil flows, builder defaults, and any other side
// combination.
func TestHasExplicitFalseBranchAnchor(t *testing.T) {
	tests := []struct {
		name string
		flow *microflows.SequenceFlow
		want bool
	}{
		{name: "nil flow", flow: nil, want: false},
		{
			name: "builder default right→left",
			flow: &microflows.SequenceFlow{OriginConnectionIndex: AnchorRight, DestinationConnectionIndex: AnchorLeft},
			want: false,
		},
		{
			name: "split default bottom→top",
			flow: &microflows.SequenceFlow{OriginConnectionIndex: AnchorBottom, DestinationConnectionIndex: AnchorTop},
			want: false,
		},
		{
			name: "authored top→bottom",
			flow: &microflows.SequenceFlow{OriginConnectionIndex: AnchorTop, DestinationConnectionIndex: AnchorBottom},
			want: true,
		},
		{
			name: "only origin customised",
			flow: &microflows.SequenceFlow{OriginConnectionIndex: AnchorTop, DestinationConnectionIndex: AnchorTop},
			want: false,
		},
		{
			name: "only destination customised",
			flow: &microflows.SequenceFlow{OriginConnectionIndex: AnchorBottom, DestinationConnectionIndex: AnchorBottom},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasExplicitFalseBranchAnchor(tc.flow)
			if got != tc.want {
				t.Fatalf("hasExplicitFalseBranchAnchor(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
