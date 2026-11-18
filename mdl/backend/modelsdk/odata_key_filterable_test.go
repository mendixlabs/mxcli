// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A Rest$ODataKeyPart carries its own Filterable, which mxbuild compares with the
// service's: a key the contract says cannot be filtered on, stored true, is CE6630
// "'Id' is marked Filterable=False in the OData service, but True in the app".
// It used to be written true whatever the contract said.
func encodedKeyPartFilterable(t *testing.T, kp *domainmodel.RemoteKeyPart) bool {
	t.Helper()
	b, err := (&codec.Encoder{}).Encode(odataKeyToGen([]*domainmodel.RemoteKeyPart{kp}))
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	for _, e := range doc {
		if e.Key != "Parts" {
			continue
		}
		arr, _ := e.Value.(bson.A)
		part, _ := arr[len(arr)-1].(bson.D)
		for _, f := range part {
			if f.Key == "Filterable" {
				v, _ := f.Value.(bool)
				return v
			}
		}
	}
	t.Fatalf("no Filterable stored on the key part: %v", doc)
	return false
}

func TestODataKeyPart_FilterableIsWrittenFromTheModel(t *testing.T) {
	base := domainmodel.RemoteKeyPart{Name: "Id", RemoteName: "Id", RemoteType: "Edm.String", Type: &domainmodel.StringAttributeType{}}

	filterable := base
	if !encodedKeyPartFilterable(t, &filterable) {
		t.Error("a key part with nothing said stays filterable (the historic value)")
	}
	notFilterable := base
	notFilterable.NotFilterable = true
	if encodedKeyPartFilterable(t, &notFilterable) {
		t.Error("a key part the contract says is not filterable was written Filterable=true")
	}
}

// Reading must keep what the document says, or a rewrite of a Studio Pro entity
// whose key is not filterable would flip it back to true.
func TestODataKeyPart_FilterableSurvivesARoundTrip(t *testing.T) {
	for _, notFilterable := range []bool{false, true} {
		kp := &domainmodel.RemoteKeyPart{Name: "Id", RemoteName: "Id", RemoteType: "Edm.String",
			Type: &domainmodel.StringAttributeType{}, NotFilterable: notFilterable}
		back := odataKeyFromGen(odataKeyToGen([]*domainmodel.RemoteKeyPart{kp}))
		if len(back) != 1 || back[0].NotFilterable != notFilterable {
			t.Errorf("NotFilterable %v did not survive: %+v", notFilterable, back)
		}
	}
}
