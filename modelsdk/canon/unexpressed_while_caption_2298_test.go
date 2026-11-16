// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2298 (mendixlabs/mxcli#1869): a while loop's condition carries a
// caption (WhileLoopCondition.Caption, shown on the loop in Studio Pro) that MDL
// cannot state. A `drop` + `create` of the flow wrote it empty, though #2178
// already kept the loop's other unstatable properties.
func whileFlowUnit(caption, expr string) []byte {
	cond := bson.D{{Key: "$ID", Value: id16(0x31)}, {Key: "$Type", Value: "Microflows$WhileLoopCondition"},
		{Key: "Caption", Value: caption}, {Key: "WhileExpression", Value: expr}}
	loop := bson.D{{Key: "$ID", Value: id16(0x30)}, {Key: "$Type", Value: "Microflows$LoopedActivity"},
		{Key: "Documentation", Value: ""}, {Key: "ErrorHandlingType", Value: "Rollback"}, {Key: "LoopSource", Value: cond}}
	b, _ := bson.Marshal(bson.D{
		{Key: "$ID", Value: id16(0xAA)}, {Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "ObjectCollection", Value: bson.D{{Key: "$ID", Value: id16(0xAB)}, {Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{int32(3), loop}}}},
	})
	return b
}

func whileCaption(t *testing.T, unit []byte) string {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(unit, &d); err != nil {
		t.Fatal(err)
	}
	caption := ""
	walkElements(d, func(e bson.D) {
		if docLookup(e, "$Type") == "Microflows$WhileLoopCondition" {
			caption, _ = docLookup(e, "Caption").(string)
		}
	})
	return caption
}

func TestUnexpressed_WhileConditionCaptionSurvivesARebuild_Issue2298(t *testing.T) {
	stored := whileFlowUnit("true", "true")
	written := whileFlowUnit("", "true")
	if got := whileCaption(t, CarryUnexpressed(written, stored)); got != "true" {
		t.Errorf("caption after the rebuild = %q, want the stored %q", got, "true")
	}
}

// CONTROL: a caption the write states (non-empty) is not replaced, and a
// changed condition expression still lands.
func TestUnexpressed_WhileConditionCaptionLeavesAStatedValue_Issue2298(t *testing.T) {
	stored := whileFlowUnit("true", "true")
	written := whileFlowUnit("other", "$N < 10")
	out := CarryUnexpressed(written, stored)
	if got := whileCaption(t, out); got != "other" {
		t.Errorf("caption = %q, want the written %q", got, "other")
	}
	var d bson.D
	_ = bson.Unmarshal(out, &d)
	expr := ""
	walkElements(d, func(e bson.D) {
		if docLookup(e, "$Type") == "Microflows$WhileLoopCondition" {
			expr, _ = docLookup(e, "WhileExpression").(string)
		}
	})
	if expr != "$N < 10" {
		t.Errorf("the changed condition did not land: %q", expr)
	}
}
