// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestPedRuleInfoType(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"Required", "DomainModels$RequiredRuleInfo", false},
		{"Unique", "DomainModels$UniqueRuleInfo", false},
		{"Range", "", true}, // not supported by the slice
		{"", "", true},
	}
	for _, c := range cases {
		got, err := pedRuleInfoType(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("pedRuleInfoType(%q): want error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("pedRuleInfoType(%q): unexpected error %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("pedRuleInfoType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidationErrorText(t *testing.T) {
	if got := validationErrorText(nil); got != "" {
		t.Errorf("nil text: got %q, want empty", got)
	}
	enUS := &model.Text{Translations: map[string]string{"en_US": "required", "nl_NL": "verplicht"}}
	if got := validationErrorText(enUS); got != "required" {
		t.Errorf("en_US preferred: got %q, want %q", got, "required")
	}
	// No en_US: deterministic (lexicographically first) fallback.
	other := &model.Text{Translations: map[string]string{"nl_NL": "verplicht", "de_DE": "erforderlich"}}
	if got := validationErrorText(other); got != "erforderlich" {
		t.Errorf("fallback: got %q, want %q (de_DE sorts before nl_NL)", got, "erforderlich")
	}
}

// TestValidationRuleWireShape pins the JSON PED receives: $Type discriminators,
// the attribute as a qualified name, and ruleInfo as a nested typed element.
func TestValidationRuleWireShape(t *testing.T) {
	rule := &pedValidationRule{
		SType:        "DomainModels$ValidationRule",
		Attribute:    "McpDm.Customer.Email",
		ErrorMessage: &pedText{SType: "Texts$Text", Text: "Email is required"},
		RuleInfo:     &pedRuleInfo{SType: "DomainModels$RequiredRuleInfo"},
	}
	b, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["$Type"] != "DomainModels$ValidationRule" {
		t.Errorf("$Type = %v", got["$Type"])
	}
	if got["attribute"] != "McpDm.Customer.Email" {
		t.Errorf("attribute = %v (want qualified name, not an ID)", got["attribute"])
	}
	ri, ok := got["ruleInfo"].(map[string]any)
	if !ok || ri["$Type"] != "DomainModels$RequiredRuleInfo" {
		t.Errorf("ruleInfo = %v", got["ruleInfo"])
	}
	em, ok := got["errorMessage"].(map[string]any)
	if !ok || em["$Type"] != "Texts$Text" || em["text"] != "Email is required" {
		t.Errorf("errorMessage = %v", got["errorMessage"])
	}

	// errorMessage must be omitted entirely when there's no message.
	noMsg := &pedValidationRule{SType: "DomainModels$ValidationRule", Attribute: "M.E.A", RuleInfo: &pedRuleInfo{SType: "DomainModels$UniqueRuleInfo"}}
	b2, _ := json.Marshal(noMsg)
	var got2 map[string]any
	_ = json.Unmarshal(b2, &got2)
	if _, present := got2["errorMessage"]; present {
		t.Errorf("errorMessage should be omitted when empty, got %v", got2["errorMessage"])
	}
}
