// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// These tests pin the pg_patch_page (LightPage) payload shapes to what Studio Pro actually
// produces, captured live in testdata/pg-page-contact-newedit*.json. They guard
// three fixes for MCP page authoring: typed parameters, edit-button client
// actions, and design properties.

func TestPageParameters_TypedParam(t *testing.T) {
	out := pageParameters([]*pages.PageParameter{
		{Name: "Contact", EntityName: "MyFirstModule.Contact", IsRequired: true},
	})
	if len(out) != 1 {
		t.Fatalf("want 1 param, got %d", len(out))
	}
	p := out[0].(map[string]any)
	pt, ok := p["parameterType"].(map[string]any)
	if !ok {
		t.Fatalf("missing parameterType element: %v", p)
	}
	if pt["$Type"] != "DataTypes$ObjectType" || pt["entity"] != "MyFirstModule.Contact" {
		t.Errorf("parameterType wrong: %v", pt)
	}
	if p["isRequired"] != true {
		t.Errorf("isRequired = %v, want true", p["isRequired"])
	}
	// The old flat `entity` field (which pg_patch_page ignores → UnknownType) must be gone.
	if _, bad := p["entity"]; bad {
		t.Errorf("flat `entity` field must not be emitted: %v", p)
	}
}

func TestMapClientAction_EditButtons(t *testing.T) {
	save, err := mapClientAction(&pages.SaveChangesClientAction{ClosePage: true})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if save["$Type"] != "Pages$SaveChangesClientAction" || save["closePage"] != true || save["syncAutomatically"] != false {
		t.Errorf("save action wrong: %v", save)
	}

	cancel, err := mapClientAction(&pages.CancelChangesClientAction{ClosePage: false})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancel["$Type"] != "Pages$CancelChangesClientAction" || cancel["closePage"] != false {
		t.Errorf("cancel action wrong: %v", cancel)
	}

	closeAct, err := mapClientAction(&pages.ClosePageClientAction{})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if closeAct["$Type"] != "Pages$ClosePageClientAction" {
		t.Errorf("close action wrong: %v", closeAct)
	}
}

func TestDesignPropertiesMap(t *testing.T) {
	out := designPropertiesMap([]pages.DesignPropertyValue{
		{Key: "Column gap", ValueType: "option", Option: "Medium"},
		{Key: "Background color", ValueType: "option", Option: "Background Secondary"},
		{Key: "Cards style", ValueType: "toggle"},
		{Key: "Spacing", ValueType: "compound", Compound: []pages.DesignPropertyValue{
			{Key: "margin-top", ValueType: "option", Option: "L"},
		}},
	})
	if out["option:Column gap"] != "Medium" {
		t.Errorf("option mapping wrong: %v", out)
	}
	if out["option:Background color"] != "Background Secondary" {
		t.Errorf("option mapping wrong: %v", out)
	}
	if out["toggle:Cards style"] != true {
		t.Errorf("toggle mapping wrong: %v", out)
	}
	// Compound nests an object of the same shape, keyed "compound:<Name>".
	nested, ok := out["compound:Spacing"].(map[string]any)
	if !ok {
		t.Fatalf("compound mapping missing/wrong type: %v", out["compound:Spacing"])
	}
	if nested["option:margin-top"] != "L" {
		t.Errorf("nested compound mapping wrong: %v", nested)
	}
}
