// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1914: the tree `include associations (Odata.Quantity (Odata.Unit))`
// builds — fresh $IDs, IsParent derived from the domain model — is written as
// the bytes Studio Pro 11.16 stored for the same ticks on ako/TestApp's
// Odata.IncludeAssociationsMF, $IDs aside. The stored tree is also the source
// of the builder test's expectation: both nodes IsParent, Unit under Quantity.
func TestCallExternalAction_AuthoredTreeMatchesStudioPro(t *testing.T) {
	el, stored := storedExternalAction(t, includeAssocFixture)
	action := actionFromGen(el).(*microflows.CallExternalAction)

	var cmd *microflows.ExternalActionParameterMapping
	for _, pm := range action.ParameterMappings {
		if pm.ParameterName == "command" {
			cmd = pm
		}
	}
	if cmd == nil || len(cmd.IncludedAssociations) != 1 {
		t.Fatalf("fixture: no command tree")
	}
	qty := cmd.IncludedAssociations[0]
	if qty.Association != "Odata.Quantity" || !qty.IsParent || len(qty.IncludedAssociations) != 1 ||
		qty.IncludedAssociations[0].Association != "Odata.Unit" || !qty.IncludedAssociations[0].IsParent {
		t.Fatalf("fixture tree is not Quantity(IsParent) > Unit(IsParent): %+v", qty)
	}

	// What the builder writes for the clause.
	node := func(name string, children ...*microflows.IncludedAssociation) *microflows.IncludedAssociation {
		return &microflows.IncludedAssociation{
			BaseElement: model.BaseElement{ID: model.ID("authored-" + name)}, Association: name, IsParent: true,
			IncludedAssociations: children,
		}
	}
	cmd.IncludedAssociations = []*microflows.IncludedAssociation{node("Odata.Quantity", node("Odata.Unit"))}

	out, err := (&codec.Encoder{}).Encode(microflowActionToGen(action))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := stripIDs(includedAssociationsByParameter(t, stored))
	got := stripIDs(includedAssociationsByParameter(t, bson.Raw(out)))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("authored tree written as\n  %v\nStudio Pro stored\n  %v", got, want)
	}
}

// stripIDs drops every $ID from a decoded BSON value.
func stripIDs(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if k != "$ID" {
				out[k] = stripIDs(e)
			}
		}
		return out
	case bson.M:
		return stripIDs(map[string]any(x))
	case bson.D:
		m := map[string]any{}
		for _, e := range x {
			m[e.Key] = e.Value
		}
		return stripIDs(m)
	case bson.A:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = stripIDs(e)
		}
		return out
	}
	return v
}
