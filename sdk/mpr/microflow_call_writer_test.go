// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"reflect"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

func TestMicroflowCallAction_WritesStableFieldOrder(t *testing.T) {
	action := &microflows.MicroflowCallAction{
		BaseElement:       model.BaseElement{ID: "action-id"},
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
		MicroflowCall: &microflows.MicroflowCall{
			BaseElement: model.BaseElement{ID: "call-id"},
			Microflow:   "Demo.UpdateRecord",
			ParameterMappings: []*microflows.MicroflowCallParameterMapping{
				{
					BaseElement: model.BaseElement{ID: "mapping-id"},
					Argument:    "$Record/Name",
					Parameter:   "Demo.UpdateRecord.Name",
				},
			},
		},
		UseReturnVariable: true,
	}

	doc := serializeMicroflowAction(action)
	assertBSONKeys(t, doc, []string{
		"$ID",
		"$Type",
		"ErrorHandlingType",
		"MicroflowCall",
		"ResultVariableName",
		"UseReturnVariable",
	})

	callDoc, ok := bsonValue(doc, "MicroflowCall").(bson.D)
	if !ok {
		t.Fatalf("MicroflowCall type = %T, want bson.D", bsonValue(doc, "MicroflowCall"))
	}
	assertBSONKeys(t, callDoc, []string{
		"$ID",
		"$Type",
		"Microflow",
		"ParameterMappings",
		"QueueSettings",
	})

	mappings, ok := bsonValue(callDoc, "ParameterMappings").(bson.A)
	if !ok || len(mappings) != 2 {
		t.Fatalf("ParameterMappings = %#v, want marker plus one mapping", bsonValue(callDoc, "ParameterMappings"))
	}
	mappingDoc, ok := mappings[1].(bson.D)
	if !ok {
		t.Fatalf("mapping type = %T, want bson.D", mappings[1])
	}
	assertBSONKeys(t, mappingDoc, []string{
		"$ID",
		"$Type",
		"Argument",
		"Parameter",
	})
}

func assertBSONKeys(t *testing.T, doc bson.D, want []string) {
	t.Helper()

	var got []string
	for _, elem := range doc {
		got = append(got, elem.Key)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BSON keys = %#v, want %#v", got, want)
	}
}

func bsonValue(doc bson.D, key string) any {
	for _, elem := range doc {
		if elem.Key == key {
			return elem.Value
		}
	}
	return nil
}
