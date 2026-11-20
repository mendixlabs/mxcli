// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2450, found measuring it: `create javascript action … (Callback:
// Nanoflow)` stored the parameter as a String (a CodeActions$BasicParameterType),
// so no call could pass it a nanoflow. Studio Pro stores
// JavaScriptActions$NanoflowJavaScriptActionParameterType, which the reader
// already maps to NanoflowType.
func TestEncodeJavaScriptAction_NanoflowParameterType_Issue2450(t *testing.T) {
	raw, err := encodeJavaScriptAction(&types.JavaScriptAction{
		BaseElement: model.BaseElement{ID: "a"},
		Name:        "Subscribe",
		Parameters: []*types.JavaActionParameter{
			{Name: "Callback", ParameterType: &types.NanoflowType{}},
			{Name: "Name", ParameterType: &types.StringType{}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	get := func(d bson.D, k string) any {
		for _, e := range d {
			if e.Key == k {
				return e.Value
			}
		}
		return nil
	}
	var got []string
	for _, p := range get(doc, "Parameters").(bson.A)[1:] {
		got = append(got, get(get(p.(bson.D), "ParameterType").(bson.D), "$Type").(string))
	}
	if got[0] != "JavaScriptActions$NanoflowJavaScriptActionParameterType" {
		t.Errorf("Callback type = %s, want JavaScriptActions$NanoflowJavaScriptActionParameterType", got[0])
	}
	// CONTROL: a String stays a basic parameter type.
	if !strings.HasSuffix(got[1], "BasicParameterType") {
		t.Errorf("Name type = %s", got[1])
	}
}
