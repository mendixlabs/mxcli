// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1932 (mendixlabs/mxcli#1730): a Delete button in a Data grid 2
// control bar deletes the grid's selection. Studio Pro stores it as
// Forms$DeleteClientAction.SourceVariable = Forms$PageVariable{Widget: "grid1"},
// the other name slots empty — the same PageVariable shape as a flow argument
// passing a grid's selection (#1560). mxcli wrote SourceVariable null, which mx
// check reports as CE7005 "No value selection has been made".
func TestClientActionToGen_DeleteWritesSelectionSource(t *testing.T) {
	el, err := clientActionToGen(&pages.DeleteClientAction{
		BaseElement:    model.BaseElement{ID: "d"},
		SourceVariable: &pages.WidgetVariable{Widget: "grid1"},
	})
	if err != nil {
		t.Fatalf("clientActionToGen: %v", err)
	}
	d := encodeToD(t, el)
	sv := lookupD(d, "SourceVariable")
	pv, ok := sv.(bson.D)
	if !ok {
		t.Fatalf("SourceVariable = %#v, want a Forms$PageVariable document", sv)
	}
	want := map[string]any{
		"$Type": "Forms$PageVariable", "Widget": "grid1", "PageParameter": "",
		"SnippetParameter": "", "LocalVariable": "", "SubKey": "", "UseAllPages": false,
	}
	for k, v := range want {
		if got := lookupD(pv, k); got != v {
			t.Errorf("SourceVariable.%s = %#v, want %#v", k, got, v)
		}
	}
}

// A page parameter fills the PageParameter slot alone.
func TestClientActionToGen_DeleteWritesParameterSource(t *testing.T) {
	el, err := clientActionToGen(&pages.DeleteClientAction{
		BaseElement:    model.BaseElement{ID: "d"},
		SourceVariable: &pages.WidgetVariable{Variable: "Order", Kind: "parameter"},
	})
	if err != nil {
		t.Fatalf("clientActionToGen: %v", err)
	}
	pv, _ := lookupD(encodeToD(t, el), "SourceVariable").(bson.D)
	if lookupD(pv, "PageParameter") != "Order" || lookupD(pv, "Widget") != "" {
		t.Errorf("SourceVariable = %v, want PageParameter Order, Widget \"\"", pv)
	}
}

// Control: a delete in a data view deletes its context object and stores
// SourceVariable null — the key present, as on all 13 Studio Pro-authored Delete
// actions in ako/TestApp (mxcli omitted it before).
func TestClientActionToGen_DeleteWithoutSourceStaysNull(t *testing.T) {
	el, err := clientActionToGen(&pages.DeleteClientAction{BaseElement: model.BaseElement{ID: "d"}})
	if err != nil {
		t.Fatalf("clientActionToGen: %v", err)
	}
	d := encodeToD(t, el)
	if !hasKey(d, "SourceVariable") {
		t.Fatalf("SourceVariable key absent, want it present and null")
	}
	if v := lookupD(d, "SourceVariable"); v != nil {
		t.Errorf("SourceVariable = %#v, want null", v)
	}
}

func lookupD(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}
