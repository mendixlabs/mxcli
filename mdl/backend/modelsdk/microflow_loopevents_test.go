// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestFlowObjectFromGen_BreakAndContinue guards loop break/continue rendering.
// Without explicit cases these objects fall through to the ActionActivity
// default and render "-- Empty action" instead of "break;"/"continue;".
func TestFlowObjectFromGen_BreakAndContinue(t *testing.T) {
	cases := []struct {
		typeName string
		want     any
	}{
		{"Microflows$BreakEvent", &microflows.BreakEvent{}},
		{"Microflows$ContinueEvent", &microflows.ContinueEvent{}},
	}
	for _, tc := range cases {
		raw := mustMarshalFlow(bson.D{
			{Key: "$ID", Value: "ev-1"},
			{Key: "$Type", Value: tc.typeName},
			{Key: "RelativeMiddlePoint", Value: "100;200"},
		})
		el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
		if err != nil {
			t.Fatalf("decode %s: %v", tc.typeName, err)
		}
		got := flowObjectFromGen(el)
		switch tc.want.(type) {
		case *microflows.BreakEvent:
			if _, ok := got.(*microflows.BreakEvent); !ok {
				t.Errorf("%s → %T, want *microflows.BreakEvent", tc.typeName, got)
			}
		case *microflows.ContinueEvent:
			if _, ok := got.(*microflows.ContinueEvent); !ok {
				t.Errorf("%s → %T, want *microflows.ContinueEvent", tc.typeName, got)
			}
		}
	}
}
