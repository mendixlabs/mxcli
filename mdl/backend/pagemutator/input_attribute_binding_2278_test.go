// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2278: a built-in input's Attribute SET wrote a null AttributeRef
// for anything that was not already `Mod.Entity.Attr`, and never wrote the
// association steps. The executor now hands it a resolved AttributeBinding.
func TestSetWidgetAttributeRef_Binding(t *testing.T) {
	for _, typ := range []string{"Forms$TextBox", "Forms$TextArea", "Forms$DatePicker",
		"Forms$DropDown", "Forms$CheckBox", "Forms$RadioButtonGroup"} {
		t.Run(typ, func(t *testing.T) {
			w := bson.D{{Key: "$Type", Value: typ}, {Key: "AttributeRef", Value: nil}}
			b := backend.AttributeBinding{Path: "M.Cust.Name",
				Steps: []pages.AttributeRefStep{{Association: "M.Ord_Cust", DestinationEntity: "M.Cust"}}}
			if err := setRawWidgetPropertyMut(w, "Attribute", b); err != nil {
				t.Fatal(err)
			}
			ref := bsonnav.DGetDoc(w, "AttributeRef")
			if got := bsonnav.DGetString(ref, "Attribute"); got != "M.Cust.Name" {
				t.Errorf("Attribute = %q", got)
			}
			er := bsonnav.DGetDoc(ref, "EntityRef")
			if bsonnav.DGetString(er, "$Type") != "DomainModels$IndirectEntityRef" {
				t.Fatalf("EntityRef = %#v, want the association steps", er)
			}
		})
	}
}

// Control: a qualified string is written as before, with no EntityRef; an
// unqualified one is refused instead of nulling the binding.
func TestSetWidgetAttributeRef_StringControl(t *testing.T) {
	w := bson.D{{Key: "$Type", Value: "Forms$TextBox"}, {Key: "AttributeRef", Value: nil}}
	if err := setRawWidgetPropertyMut(w, "Attribute", "M.Ord.Number"); err != nil {
		t.Fatal(err)
	}
	ref := bsonnav.DGetDoc(w, "AttributeRef")
	if bsonnav.DGetString(ref, "Attribute") != "M.Ord.Number" || bsonnav.DGet(ref, "EntityRef") != nil {
		t.Errorf("qualified string: AttributeRef = %#v", ref)
	}
	for _, v := range []string{"Number", "Ord_Cust/Name", "M.Ord_Cust/M.Cust/Name"} {
		err := setRawWidgetPropertyMut(w, "Attribute", v)
		if err == nil || !strings.Contains(err.Error(), "not a qualified attribute") {
			t.Errorf("%q: want a refusal, got %v", v, err)
		}
	}
}
