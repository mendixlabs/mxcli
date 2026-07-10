// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestSplitConditionFromGen_RuleSplit guards DESCRIBE MICROFLOW rendering a
// rule-based split. The rule reference is stored under the "Microflow" key
// (rules share the microflow namespace), which gen decodes as the (empty)
// "Rule" property — so splitConditionFromGen must fall back to the raw storage
// key. Without it the condition reads as nil and the renderer emits
// "if true then …", losing the real rule call.
func TestSplitConditionFromGen_RuleSplit(t *testing.T) {
	raw := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "cond-1"},
		{Key: "$Type", Value: "Microflows$RuleSplitCondition"},
		{Key: "RuleCall", Value: bson.D{
			{Key: "$ID", Value: "rc-1"},
			{Key: "$Type", Value: "Microflows$RuleCall"},
			{Key: "Microflow", Value: "Authentication.Rule_ValidateAccount"},
			{Key: "ParameterMappings", Value: bson.A{
				int32(2), // Mendix list-count marker — skipped by DecodeChildren
				bson.D{
					{Key: "$ID", Value: "pm-1"},
					{Key: "$Type", Value: "Microflows$RuleCallParameterMapping"},
					{Key: "Parameter", Value: "Authentication.Rule_ValidateAccount.account"},
					{Key: "Argument", Value: "$account"},
				},
			}},
		}},
	})

	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode RuleSplitCondition: %v", err)
	}
	cond := splitConditionFromGen(el)
	rc, ok := cond.(*microflows.RuleSplitCondition)
	if !ok {
		t.Fatalf("splitConditionFromGen = %T, want *microflows.RuleSplitCondition (rule split → 'if true')", cond)
	}
	if rc.RuleQualifiedName != "Authentication.Rule_ValidateAccount" {
		t.Errorf("RuleQualifiedName = %q, want the value from the Microflow storage key", rc.RuleQualifiedName)
	}
	if len(rc.ParameterMappings) != 1 {
		t.Fatalf("ParameterMappings = %d, want 1", len(rc.ParameterMappings))
	}
	if pm := rc.ParameterMappings[0]; pm.ParameterName != "Authentication.Rule_ValidateAccount.account" || pm.Argument != "$account" {
		t.Errorf("param mapping = {%q, %q}, want {…account, $account}", pm.ParameterName, pm.Argument)
	}
}
