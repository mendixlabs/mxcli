// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	genMf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestCaseValueFromGen_NewCaseValueSingular guards the DESCRIBE MICROFLOW
// then-skip bug: a SequenceFlow's branch case is stored under the BSON storage
// name "NewCaseValue" as a single child (the form Studio Pro writes for
// enumeration and boolean splits), not in the plural "CaseValues" list. The
// reader must surface it; otherwise the split reads as case-less and the
// renderer drops the entire then/when body, emitting "if cond then end if;".
func TestCaseValueFromGen_NewCaseValueSingular(t *testing.T) {
	raw := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "flow-1"},
		{Key: "$Type", Value: "Microflows$SequenceFlow"},
		{Key: "NewCaseValue", Value: bson.D{
			{Key: "$ID", Value: "case-1"},
			{Key: "$Type", Value: "Microflows$EnumerationCase"},
			{Key: "Value", Value: "TOTP"},
		}},
	})

	flow := decodeSequenceFlow(t, raw)
	cv := caseValueFromGen(flow)
	ec, ok := cv.(*microflows.EnumerationCase)
	if !ok {
		t.Fatalf("caseValueFromGen = %T, want *microflows.EnumerationCase (NewCaseValue not read → branch body dropped)", cv)
	}
	if ec.Value != "TOTP" {
		t.Errorf("EnumerationCase.Value = %q, want %q", ec.Value, "TOTP")
	}
}

// TestCaseValueFromGen_NoCaseIsNil confirms a normal (non-branch) flow, whose
// case is a NoCase, yields no model case value.
func TestCaseValueFromGen_NoCaseIsNil(t *testing.T) {
	raw := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "flow-2"},
		{Key: "$Type", Value: "Microflows$SequenceFlow"},
		{Key: "NewCaseValue", Value: bson.D{
			{Key: "$ID", Value: "case-2"},
			{Key: "$Type", Value: "Microflows$NoCase"},
		}},
	})
	if cv := caseValueFromGen(decodeSequenceFlow(t, raw)); cv != nil {
		t.Errorf("caseValueFromGen for NoCase = %T, want nil", cv)
	}
}

func mustMarshalFlow(d bson.D) bson.Raw {
	b, err := bson.Marshal(d)
	if err != nil {
		panic(err)
	}
	return bson.Raw(b)
}

func decodeSequenceFlow(t *testing.T, raw bson.Raw) *genMf.SequenceFlow {
	t.Helper()
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode SequenceFlow: %v", err)
	}
	flow, ok := el.(*genMf.SequenceFlow)
	if !ok {
		t.Fatalf("decoded %T, want *genMf.SequenceFlow", el)
	}
	return flow
}
