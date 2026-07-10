// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestFlowObjectFromGen_InheritanceSplit guards the inheritance-split loop
// header: without reading SplitVariableName the header renders "split type $"
// (empty variable), and without Caption the @caption line is dropped.
func TestFlowObjectFromGen_InheritanceSplit(t *testing.T) {
	raw := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "is-1"},
		{Key: "$Type", Value: "Microflows$InheritanceSplit"},
		{Key: "RelativeMiddlePoint", Value: "100;200"},
		{Key: "SplitVariableName", Value: "account"},
		{Key: "Caption", Value: "has DB account ?"},
	})
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	split, ok := flowObjectFromGen(el).(*microflows.InheritanceSplit)
	if !ok {
		t.Fatalf("flowObjectFromGen → %T, want *microflows.InheritanceSplit", flowObjectFromGen(el))
	}
	if split.VariableName != "account" {
		t.Errorf("VariableName = %q, want account (header would render 'split type $')", split.VariableName)
	}
	if split.Caption != "has DB account ?" {
		t.Errorf("Caption = %q, want 'has DB account ?'", split.Caption)
	}
}
