// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

func TestBuildSequenceFlowCase_InheritanceCase(t *testing.T) {
	doc := buildSequenceFlowCase(&microflows.InheritanceCase{
		BaseElement:         model.BaseElement{ID: "case-1"},
		EntityQualifiedName: "Sample.SpecializedInput",
	})

	if got := bsonGetKey(doc, "$Type"); got != "Microflows$InheritanceCase" {
		t.Fatalf("$Type = %v, want Microflows$InheritanceCase", got)
	}
	if got := bsonGetKey(doc, "Value"); got != "Sample.SpecializedInput" {
		t.Fatalf("Value = %v, want Sample.SpecializedInput", got)
	}
}

func TestSerializeMicroflowObject_InheritanceSplit(t *testing.T) {
	doc := serializeMicroflowObject(&microflows.InheritanceSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: "split-1"},
			Position:    model.Point{X: 100, Y: 200},
			Size:        model.Size{Width: 120, Height: 60},
		},
		VariableName:      "Input",
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
	})

	if got := bsonGetKey(doc, "$Type"); got != "Microflows$InheritanceSplit" {
		t.Fatalf("$Type = %v, want Microflows$InheritanceSplit", got)
	}
	if got := bsonGetKey(doc, "SplitVariableName"); got != "Input" {
		t.Fatalf("SplitVariableName = %v, want Input", got)
	}
}

func TestCastAction_RoundtripVariableName(t *testing.T) {
	action := &microflows.CastAction{
		BaseElement:    model.BaseElement{ID: "cast-1"},
		OutputVariable: "SpecificInput",
	}
	doc := serializeMicroflowAction(action)
	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal cast action: %v", err)
	}
	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal cast action: %v", err)
	}

	parsed := parseCastAction(raw)
	if parsed.OutputVariable != "SpecificInput" {
		t.Fatalf("OutputVariable = %q, want SpecificInput", parsed.OutputVariable)
	}
}

// TestSerializeCastAction_UsesVariableNameFieldKey pins the BSON field key
// Studio Pro emits for Microflows$CastAction. Empirical evidence (BSON
// dump of the Control Centre app on Mendix 9.24): Studio Pro stores the
// output variable under "VariableName", not "OutputVariableName". The
// parser falls back to "VariableName" when "OutputVariableName" is
// absent so projects authored by Studio Pro still parse cleanly; the
// writer must match Studio Pro's authored shape so projects we produce
// open without surprises.
func TestSerializeCastAction_UsesVariableNameFieldKey(t *testing.T) {
	action := &microflows.CastAction{
		BaseElement:    model.BaseElement{ID: "cast-1"},
		OutputVariable: "SpecificInput",
	}
	doc := serializeMicroflowAction(action)
	if got := bsonGetKey(doc, "VariableName"); got != "SpecificInput" {
		t.Fatalf("VariableName = %v, want SpecificInput", got)
	}
	if got := bsonGetKey(doc, "OutputVariableName"); got != nil {
		t.Fatalf("OutputVariableName = %v, want absent (Studio Pro uses VariableName)", got)
	}
}

// TestBuildSequenceFlowCase_InheritanceCase_UsesValueFieldKey pins the
// BSON field key for Microflows$InheritanceCase. Empirical evidence
// (BSON dump of the Control Centre app, Mendix 9.24): the entity
// reference is stored under "Value" as a qualified-name string
// (e.g. "Administration.Account"), not "Entity". The parser falls back
// to "Entity" for forward compatibility; the writer must emit "Value"
// so output matches Studio Pro's authored shape.
func TestBuildSequenceFlowCase_InheritanceCase_UsesValueFieldKey(t *testing.T) {
	doc := buildSequenceFlowCase(&microflows.InheritanceCase{
		BaseElement:         model.BaseElement{ID: "case-1"},
		EntityQualifiedName: "Sample.SpecializedInput",
	})
	if got := bsonGetKey(doc, "Value"); got != "Sample.SpecializedInput" {
		t.Fatalf("Value = %v, want Sample.SpecializedInput", got)
	}
	if got := bsonGetKey(doc, "Entity"); got != nil {
		t.Fatalf("Entity = %v, want absent (Studio Pro uses Value)", got)
	}
}
