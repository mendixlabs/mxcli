// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2106: Studio Pro stores a Forms$AssociationSource's SourceVariable
// key on every one, null when the source starts from $currentObject (40 of 40
// in FactoryManagement, 30 null). The writer left the key out, so describe ->
// exec of DigitalTwin.InvestigationTask_Done rewrote the page.
func TestWriteAssociationSource_NullSourceVariable(t *testing.T) {
	out, err := (&codec.Encoder{}).Encode(associationSourceToGen(&pages.AssociationSource{EntityPath: "M.Item_Order/M.Item"}))
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(out, &d); err != nil {
		t.Fatal(err)
	}
	for _, e := range d {
		if e.Key == "SourceVariable" {
			if e.Value != nil {
				t.Errorf("SourceVariable = %v, want null for a $currentObject source", e.Value)
			}
			return
		}
	}
	t.Error("SourceVariable key absent; Studio Pro stores it null")
}
