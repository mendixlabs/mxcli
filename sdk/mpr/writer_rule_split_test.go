// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

// TestSerializeExclusiveSplit_RuleSplitCondition_Roundtrip verifies that an
// ExclusiveSplit whose SplitCondition is a RuleSplitCondition survives
// serialize → BSON → parse without losing the rule reference or its parameter
// mappings. This is the BSON-level regression guard for the CE0117 Studio Pro
// error that appears when a rule-based decision is stored as an expression.
func TestSerializeExclusiveSplit_RuleSplitCondition_Roundtrip(t *testing.T) {
	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: "11111111-1111-1111-1111-111111111111"},
			Position:    model.Point{X: 100, Y: 200},
			Size:        model.Size{Width: 50, Height: 50},
		},
		Caption:           "Module.IsEligible($Customer)",
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
		SplitCondition: &microflows.RuleSplitCondition{
			BaseElement:       model.BaseElement{ID: "22222222-2222-2222-2222-222222222222"},
			RuleQualifiedName: "Module.IsEligible",
			ParameterMappings: []*microflows.RuleCallParameterMapping{
				{
					BaseElement:   model.BaseElement{ID: "33333333-3333-3333-3333-333333333333"},
					ParameterName: "Module.IsEligible.Customer",
					Argument:      "$Customer",
				},
			},
		},
	}

	doc := serializeMicroflowObject(split)
	if doc == nil {
		t.Fatal("serializeMicroflowObject returned nil")
	}

	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("bson.Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal failed: %v", err)
	}

	parsed := parseMicroflowObject(raw)
	roundtripSplit, ok := parsed.(*microflows.ExclusiveSplit)
	if !ok {
		t.Fatalf("parsed object: got %T, want *microflows.ExclusiveSplit", parsed)
	}

	ruleCond, ok := roundtripSplit.SplitCondition.(*microflows.RuleSplitCondition)
	if !ok {
		t.Fatalf("split condition after roundtrip: got %T, want *microflows.RuleSplitCondition", roundtripSplit.SplitCondition)
	}
	if ruleCond.RuleQualifiedName != "Module.IsEligible" {
		t.Errorf("rule qualified name: got %q, want %q", ruleCond.RuleQualifiedName, "Module.IsEligible")
	}
	if len(ruleCond.ParameterMappings) != 1 {
		t.Fatalf("parameter mappings: got %d, want 1", len(ruleCond.ParameterMappings))
	}
	pm := ruleCond.ParameterMappings[0]
	if pm.ParameterName != "Module.IsEligible.Customer" {
		t.Errorf("parameter name: got %q, want %q", pm.ParameterName, "Module.IsEligible.Customer")
	}
	if pm.Argument != "$Customer" {
		t.Errorf("argument: got %q, want %q", pm.Argument, "$Customer")
	}
}

// TestSerializeExclusiveSplit_ExpressionSplitCondition_Roundtrip is the
// complementary baseline that ensures the existing expression path still
// roundtrips correctly after the Rule branch was added to the writer switch.
func TestSerializeExclusiveSplit_ExpressionSplitCondition_Roundtrip(t *testing.T) {
	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: "44444444-4444-4444-4444-444444444444"},
			Position:    model.Point{X: 100, Y: 200},
			Size:        model.Size{Width: 50, Height: 50},
		},
		Caption:           "$Var = 'x'",
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
		SplitCondition: &microflows.ExpressionSplitCondition{
			BaseElement: model.BaseElement{ID: "55555555-5555-5555-5555-555555555555"},
			Expression:  "$Var = 'x'",
		},
	}

	doc := serializeMicroflowObject(split)
	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("bson.Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal failed: %v", err)
	}

	parsed := parseMicroflowObject(raw)
	roundtripSplit := parsed.(*microflows.ExclusiveSplit)

	exprCond, ok := roundtripSplit.SplitCondition.(*microflows.ExpressionSplitCondition)
	if !ok {
		t.Fatalf("split condition after roundtrip: got %T, want *microflows.ExpressionSplitCondition", roundtripSplit.SplitCondition)
	}
	if exprCond.Expression != "$Var = 'x'" {
		t.Errorf("expression: got %q, want %q", exprCond.Expression, "$Var = 'x'")
	}
}
