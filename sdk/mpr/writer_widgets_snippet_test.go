// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// TestSnippetCall_ParameterMapping_CorrectBSONType verifies that
// Forms$SnippetParameterMapping (not Forms$PageParameterMapping) is written
// for snippet call parameter mappings (issue #291 / #295 follow-up).
// Studio Pro throws InvalidOperationException when it finds PageParameterMapping
// inside a SnippetCall container.
func TestSnippetCall_ParameterMapping_CorrectBSONType(t *testing.T) {
	sc := &pages.SnippetCallWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{ID: "sc-id"},
			Name:        "snippetCall1",
		},
		SnippetName: "Mod.MySnippet",
		ParameterMappings: []pages.SnippetParamMapping{
			{ParamName: "Asset", Argument: "$Asset"},
		},
	}

	doc := serializeSnippetCall(sc)
	if doc == nil {
		t.Fatal("serializeSnippetCall returned nil")
	}

	// Navigate to FormCall.ParameterMappings
	var formCall bson.D
	for _, e := range doc {
		if e.Key == "FormCall" {
			formCall, _ = e.Value.(bson.D)
		}
	}
	if formCall == nil {
		t.Fatal("FormCall is nil")
	}

	var paramMappings bson.A
	for _, e := range formCall {
		if e.Key == "ParameterMappings" {
			paramMappings, _ = e.Value.(bson.A)
		}
	}
	if len(paramMappings) < 2 {
		t.Fatalf("ParameterMappings: want count+1 elements, got %d", len(paramMappings))
	}

	// Element 0 is int32 count; element 1 is the first mapping
	mapping, ok := paramMappings[1].(bson.D)
	if !ok {
		t.Fatalf("ParameterMappings[1] is not bson.D, got %T", paramMappings[1])
	}

	var bsonType, argument, parameter string
	var variable any
	for _, e := range mapping {
		switch e.Key {
		case "$Type":
			bsonType, _ = e.Value.(string)
		case "Argument":
			argument, _ = e.Value.(string)
		case "Parameter":
			parameter, _ = e.Value.(string)
		case "Variable":
			variable = e.Value
		}
	}

	if bsonType != "Forms$SnippetParameterMapping" {
		t.Errorf("$Type = %q, want %q (PageParameterMapping is wrong for snippet context)", bsonType, "Forms$SnippetParameterMapping")
	}
	if argument != "" {
		t.Errorf("Argument = %q, want %q (variable belongs in Variable.PageParameter)", argument, "")
	}
	if parameter != "Mod.MySnippet.Asset" {
		t.Errorf("Parameter = %q, want %q", parameter, "Mod.MySnippet.Asset")
	}
	if variable == nil {
		t.Fatal("Variable is nil — Forms$SnippetParameterMapping requires non-null Forms$PageVariable")
	}

	varDoc, ok := variable.(bson.D)
	if !ok {
		t.Fatalf("Variable is not bson.D, got %T", variable)
	}

	var varType, pageParam string
	for _, e := range varDoc {
		switch e.Key {
		case "$Type":
			varType, _ = e.Value.(string)
		case "PageParameter":
			pageParam, _ = e.Value.(string)
		}
	}
	if varType != "Forms$PageVariable" {
		t.Errorf("Variable.$Type = %q, want %q", varType, "Forms$PageVariable")
	}
	if pageParam != "Asset" {
		t.Errorf("Variable.PageParameter = %q, want %q (stripped $)", pageParam, "Asset")
	}
}
