// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
	"go.mongodb.org/mongo-driver/bson"
)

// TestSerializeJavaScriptAction_Shape asserts the serialized JS action uses the
// JavaScriptActions$ document and parameter $Type names, carries a Platform
// field, and reuses the CodeActions$ inner parameter/return types.
func TestSerializeJavaScriptAction_Shape(t *testing.T) {
	w := &Writer{}
	jsa := &JavaScriptAction{
		BaseElement: model.BaseElement{ID: "11111111-1111-1111-1111-111111111111"},
		Name:        "JSA",
		Platform:    "All",
		Parameters: []*javaactions.JavaActionParameter{
			{
				BaseElement:   model.BaseElement{ID: "22222222-2222-2222-2222-222222222222", TypeName: "JavaScriptActions$JavaScriptActionParameter"},
				Name:          "Input",
				IsRequired:    true,
				ParameterType: &javaactions.StringType{BaseElement: model.BaseElement{ID: "33333333-3333-3333-3333-333333333333", TypeName: "CodeActions$StringType"}},
			},
		},
		ReturnType: &javaactions.BooleanType{BaseElement: model.BaseElement{ID: "44444444-4444-4444-4444-444444444444", TypeName: "CodeActions$BooleanType"}},
	}

	raw, err := w.serializeJavaScriptAction(jsa)
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	m := map[string]any{}
	for _, e := range doc {
		m[e.Key] = e.Value
	}

	if m["$Type"] != "JavaScriptActions$JavaScriptAction" {
		t.Errorf("$Type = %v", m["$Type"])
	}
	if m["Platform"] != "All" {
		t.Errorf("Platform = %v, want All", m["Platform"])
	}
	if m["ActionDefaultReturnName"] != "ReturnValueName" {
		t.Errorf("ActionDefaultReturnName = %v", m["ActionDefaultReturnName"])
	}

	params, ok := m["Parameters"].(bson.A)
	if !ok || len(params) < 2 {
		t.Fatalf("Parameters = %v", m["Parameters"])
	}
	if marker, _ := params[0].(int32); marker != 2 {
		t.Errorf("param array marker = %v, want 2", params[0])
	}
	p0 := params[1].(bson.D)
	var pType string
	for _, e := range p0 {
		if e.Key == "$Type" {
			pType, _ = e.Value.(string)
		}
	}
	if pType != "JavaScriptActions$JavaScriptActionParameter" {
		t.Errorf("param $Type = %q", pType)
	}
}

// TestSerializeJavaScriptAction_DefaultPlatform asserts an unset platform
// defaults to Web.
func TestSerializeJavaScriptAction_DefaultPlatform(t *testing.T) {
	w := &Writer{}
	raw, err := w.serializeJavaScriptAction(&JavaScriptAction{
		BaseElement: model.BaseElement{ID: "11111111-1111-1111-1111-111111111111"},
		Name:        "JSA",
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	_ = bson.Unmarshal(raw, &doc)
	for _, e := range doc {
		if e.Key == "Platform" && e.Value != "Web" {
			t.Errorf("default Platform = %v, want Web", e.Value)
		}
	}
}
