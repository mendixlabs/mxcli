// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func decodeAction(t *testing.T, d bson.D) microflows.MicroflowAction {
	t.Helper()
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(mustMarshalFlow(d))
	if err != nil {
		t.Fatalf("decode action: %v", err)
	}
	return actionFromGen(el)
}

// TestActionFromGen_ExportXml guards EXPORT TO MAPPING rendering. Without the
// ExportXmlAction case the action renders "-- Empty action". The mapping +
// argument live in ResultHandling and the output var in OutputMethod.
func TestActionFromGen_ExportXml(t *testing.T) {
	act := decodeAction(t, bson.D{
		{Key: "$ID", Value: "a-1"},
		{Key: "$Type", Value: "Microflows$ExportXmlAction"},
		{Key: "ErrorHandlingType", Value: "Rollback"},
		{Key: "OutputMethod", Value: bson.D{
			{Key: "$ID", Value: "om-1"},
			{Key: "$Type", Value: "ExportXmlAction$StringExport"},
			{Key: "OutputVariableName", Value: "data"},
		}},
		{Key: "ResultHandling", Value: bson.D{
			{Key: "$ID", Value: "rh-1"},
			{Key: "$Type", Value: "Microflows$MappingRequestHandling"},
			{Key: "ContentType", Value: "Json"},
			{Key: "MappingId", Value: "BE.EM_ConsumedEventData"},
			{Key: "MappingVariableName", Value: "src"},
		}},
	})
	ex, ok := act.(*microflows.ExportXmlAction)
	if !ok {
		t.Fatalf("actionFromGen → %T, want *microflows.ExportXmlAction", act)
	}
	if ex.OutputVariable != "data" {
		t.Errorf("OutputVariable = %q, want data", ex.OutputVariable)
	}
	if ex.RequestHandling == nil || string(ex.RequestHandling.MappingID) != "BE.EM_ConsumedEventData" || ex.RequestHandling.ParameterVariable != "src" {
		t.Errorf("RequestHandling = %+v", ex.RequestHandling)
	}
}

// TestActionFromGen_ImportXml guards IMPORT FROM MAPPING rendering, including the
// single-object cardinality: ForceSingleOccurrence must fold into SingleObject
// so the describer prints "as Entity" not "as list of Entity".
func TestActionFromGen_ImportXml(t *testing.T) {
	act := decodeAction(t, bson.D{
		{Key: "$ID", Value: "a-2"},
		{Key: "$Type", Value: "Microflows$ImportXmlAction"},
		{Key: "ErrorHandlingType", Value: "CustomWithoutRollBack"},
		{Key: "XmlDocumentVariableName", Value: "resp"},
		{Key: "ResultHandling", Value: bson.D{
			{Key: "$ID", Value: "rh-2"},
			{Key: "$Type", Value: "Microflows$ResultHandling"},
			{Key: "ResultVariableName", Value: "out"},
			{Key: "ImportMappingCall", Value: bson.D{
				{Key: "$ID", Value: "imc-2"},
				{Key: "$Type", Value: "Microflows$ImportMappingCall"},
				{Key: "ReturnValueMapping", Value: "KS.IM_ErrorResponse"},
				{Key: "ForceSingleOccurrence", Value: true},
			}},
		}},
	})
	im, ok := act.(*microflows.ImportXmlAction)
	if !ok {
		t.Fatalf("actionFromGen → %T, want *microflows.ImportXmlAction", act)
	}
	if im.XmlDocumentVariable != "resp" {
		t.Errorf("XmlDocumentVariable = %q, want resp", im.XmlDocumentVariable)
	}
	if im.ResultHandling == nil {
		t.Fatal("ResultHandling nil")
	}
	if string(im.ResultHandling.MappingID) != "KS.IM_ErrorResponse" || im.ResultHandling.ResultVariable != "out" {
		t.Errorf("ResultHandling = %+v", im.ResultHandling)
	}
	if !im.ResultHandling.SingleObject {
		t.Error("SingleObject = false, want true (ForceSingleOccurrence must fold in → 'as Entity')")
	}
}

// TestRestResultHandling_ObjectTypeIsSingle guards the REST mapping cardinality:
// an object (non-list) result VariableType means a single object, so the
// describer prints "as Entity" not "as list of Entity".
func TestRestResultHandling_ObjectTypeIsSingle(t *testing.T) {
	doc := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "rh-3"},
		{Key: "ResultVariableName", Value: "profile"},
		{Key: "ImportMappingCall", Value: bson.D{
			{Key: "$Type", Value: "Microflows$ImportMappingCall"},
			{Key: "ReturnValueMapping", Value: "Sprintr.IM_ProfileResponse"},
			{Key: "ForceSingleOccurrence", Value: false},
		}},
		{Key: "VariableType", Value: bson.D{
			{Key: "$Type", Value: "DataTypes$ObjectType"},
			{Key: "Entity", Value: "Sprintr.UserProfileResponse"},
		}},
	})
	h, ok := restResultHandlingFromRaw(doc).(*microflows.ResultHandlingMapping)
	if !ok {
		t.Fatalf("restResultHandlingFromRaw → not a mapping handling")
	}
	if !h.SingleObject {
		t.Error("SingleObject = false, want true (object var type → 'as Entity', not 'as list of')")
	}
}
