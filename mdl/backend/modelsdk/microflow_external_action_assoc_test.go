// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"os"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
)

// ako/TestApp's Odata.IncludeAssociationsMF and its control _Off (Studio Pro
// 11.16.0): the same CreateBuffer call on the Bug1914 service, with
// "Associations to include" ticked for Quantity and the Unit under it, and with
// nothing ticked.
const (
	includeAssocFixture    = "testdata/TestApp.IncludeAssociationsMF.11_16.bson"
	includeAssocOffFixture = "testdata/TestApp.IncludeAssociationsMF_Off.11_16.bson"
)

// storedExternalAction returns the fixture's one CallExternalAction, as the
// decoded element and as its stored bytes.
func storedExternalAction(t *testing.T, path string) (element.Element, bson.Raw) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(data))
	if err != nil {
		t.Fatal(err)
	}
	var found element.Element
	element.Walk(root, func(e element.Element) bool {
		if _, ok := e.(*genMf.CallExternalAction); ok {
			found = e
		}
		return found == nil
	})
	if found == nil {
		t.Fatalf("%s: no CallExternalAction", path)
	}
	return found, found.Raw()
}

// includedAssociationsByParameter reads each parameter mapping's
// IncludedAssociations list as plain values, $IDs included. An empty list —
// Studio Pro writes the bare marker on every mapping — reads as absent: the
// writer leaves empty lists out (#1073's finding), and that is not this test's
// question.
func includedAssociationsByParameter(t *testing.T, call bson.Raw) map[string]any {
	t.Helper()
	out := map[string]any{}
	for _, md := range rawDocElements(call, "ParameterMappings") {
		var m bson.M
		if err := bson.Unmarshal(md, &m); err != nil {
			t.Fatal(err)
		}
		var v any
		if list, ok := m["IncludedAssociations"].(bson.A); ok && len(list) > 1 {
			v = list
		}
		out[rawStr(md, "ParameterName")] = v
	}
	return out
}

// #1914: "Associations to include" is a tree of Microflows$IncludedAssociation
// on each parameter mapping, and the reader and writer both ignored it, so any
// rewrite of the microflow — CREATE OR MODIFY, or an ALTER that re-encodes it —
// wrote the call back without it, and the OData request silently lost the
// associated objects at runtime.
func TestCallExternalAction_IncludedAssociationsRoundTrip(t *testing.T) {
	for _, fixture := range []string{includeAssocFixture, includeAssocOffFixture} {
		t.Run(fixture, func(t *testing.T) {
			el, stored := storedExternalAction(t, fixture)
			out, err := (&codec.Encoder{}).Encode(microflowActionToGen(actionFromGen(el)))
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			want := includedAssociationsByParameter(t, stored)
			got := includedAssociationsByParameter(t, bson.Raw(out))
			for param, w := range want {
				if !reflect.DeepEqual(got[param], w) {
					t.Errorf("parameter %s: IncludedAssociations written as\n  %v\nstored as\n  %v", param, got[param], w)
				}
			}
		})
	}
}
