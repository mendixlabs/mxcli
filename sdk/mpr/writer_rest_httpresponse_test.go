// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSerializeRestResultHandlingHttpResponseUsesObjectType(t *testing.T) {
	handling := &microflows.ResultHandlingHttpResponse{
		BaseElement:  model.BaseElement{ID: "result-1"},
		VariableName: "HttpResponse",
	}

	doc := serializeRestResultHandling(handling, "HttpResponse")

	if got := getBSONField(doc, "ResultVariableName"); got != "HttpResponse" {
		t.Fatalf("ResultVariableName = %#v, want HttpResponse", got)
	}
	variableType, ok := getBSONField(doc, "VariableType").(bson.D)
	if !ok {
		t.Fatalf("VariableType is %T, want bson.D", getBSONField(doc, "VariableType"))
	}
	if got := getBSONField(variableType, "$Type"); got != "DataTypes$ObjectType" {
		t.Fatalf("VariableType.$Type = %#v, want DataTypes$ObjectType", got)
	}
	if got := getBSONField(variableType, "Entity"); got != "System.HttpResponse" {
		t.Fatalf("VariableType.Entity = %#v, want System.HttpResponse", got)
	}
}
