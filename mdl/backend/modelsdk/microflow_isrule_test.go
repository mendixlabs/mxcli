// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genMf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/microflows"
)

// TestIsRule guards issue #723 §A (A4 / CE0117). The modelsdk backend never
// implemented IsRule, so it fell back to the embedded `unimplemented` stub,
// which returns an error. The flow-builder's rule detection
// (`isRule, err := backend.IsRule(...); if err != nil || !isRule { return nil }`)
// then treated every `if Module.SomeRule(...)` as a plain expression and emitted
// an invalid ExpressionSplitCondition → mx check CE0117 "Error in expression".
//
// IsRule must (a) NOT error on the modelsdk engine, (b) return true for a real
// rule's qualified name, and (c) return false for a non-rule.
func TestIsRule(t *testing.T) {
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName(MyFirstModule) = %v, %v", mod, err)
	}

	// Insert a Microflows$Rule document into the module (rules share the
	// microflow namespace but are stored under a distinct BSON $Type). MDL
	// cannot create rules, so build one directly.
	const ruleID = "11111111-1111-1111-1111-111111111111"
	r := genMf.NewRule()
	r.SetID(element.ID(ruleID))
	r.SetName("Rule_IsAdmin")
	r.SetExportLevel("Hidden")
	contents, err := (&codec.Encoder{}).Encode(r)
	if err != nil {
		t.Fatalf("encode rule: %v", err)
	}
	if err := b.writer.InsertUnit(ruleID, string(mod.ID), "Documents", "Microflows$Rule", contents); err != nil {
		t.Fatalf("insert rule unit: %v", err)
	}

	// (a)+(b): the rule's qualified name resolves as a rule, with no error.
	isRule, err := b.IsRule("MyFirstModule.Rule_IsAdmin")
	if err != nil {
		t.Fatalf("IsRule returned error (unimplemented on modelsdk → CE0117): %v", err)
	}
	if !isRule {
		t.Error("IsRule(MyFirstModule.Rule_IsAdmin) = false, want true")
	}

	// (c): a non-rule name is not a rule (and still no error).
	notRule, err := b.IsRule("MyFirstModule.NotARule")
	if err != nil {
		t.Fatalf("IsRule(non-rule) error: %v", err)
	}
	if notRule {
		t.Error("IsRule(MyFirstModule.NotARule) = true, want false")
	}

	// Empty name short-circuits to false, no error.
	if ok, err := b.IsRule(""); ok || err != nil {
		t.Errorf("IsRule(\"\") = %v, %v; want false, nil", ok, err)
	}
}
