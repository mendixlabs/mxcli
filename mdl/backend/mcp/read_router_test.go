// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestLastSegment(t *testing.T) {
	cases := map[string]string{
		"MyFirstModule.Order.Total": "Total",
		"Mod.Entity":                "Entity",
		"Bare":                      "Bare",
	}
	for in, want := range cases {
		if got := lastSegment(in); got != want {
			t.Errorf("lastSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveEntityRef(t *testing.T) {
	order := []string{"id-A", "id-B", "id-C"}
	if got := resolveEntityRef("$id(/entities/1)", order); got != model.ID("id-B") {
		t.Errorf("got %q, want id-B", got)
	}
	if got := resolveEntityRef("$id(/entities/9)", order); got != "" {
		t.Errorf("out-of-range ref should be empty, got %q", got)
	}
	if got := resolveEntityRef("garbage", order); got != "" {
		t.Errorf("unparseable ref should be empty, got %q", got)
	}
}

func TestReconstructEntities(t *testing.T) {
	b := &Backend{synthetic: map[model.ID]string{}}
	raw := json.RawMessage(`[
		{"$Type":"DomainModels$Entity","$QualifiedName":"M.Order","name":"Order",
		 "attributes":[
			{"$QualifiedName":"M.Order.Total","$Type":"DomainModels$Attribute"},
			{"$QualifiedName":"M.Order.Title","$Type":"DomainModels$Attribute"}]},
		{"$Type":"DomainModels$Entity","$QualifiedName":"M.Customer","name":"Customer","attributes":[]}
	]`)

	ents, order, err := b.reconstructEntities("M", model.ID("dm-1"), raw)
	if err != nil {
		t.Fatalf("reconstructEntities: %v", err)
	}
	if len(ents) != 2 || ents[0].Name != "Order" || ents[1].Name != "Customer" {
		t.Fatalf("unexpected entities: %+v", ents)
	}
	if got := []string{ents[0].Attributes[0].Name, ents[0].Attributes[1].Name}; got[0] != "Total" || got[1] != "Title" {
		t.Fatalf("attribute names parsed wrong: %v", got)
	}
	// synthetic IDs registered + resolvable back to names
	if name, ok := b.syntheticName(ents[0].ID); !ok || name != "Order" {
		t.Fatalf("synthetic ID for Order not registered: %v / %q", ok, name)
	}
	if order[0] != string(ents[0].ID) {
		t.Fatalf("order[0] %q != entity ID %q", order[0], ents[0].ID)
	}
}

func TestReconstructAssociations_ResolvesParentChild(t *testing.T) {
	b := &Backend{synthetic: map[model.ID]string{}}
	// entity order: index 0 -> Location, index 1 -> SalesData
	order := []string{"id-Location", "id-SalesData"}
	raw := json.RawMessage(`[
		{"$Type":"DomainModels$Association","name":"SalesData_Location",
		 "type":"Reference","owner":"Default",
		 "parent":"$id(/entities/1)","child":"$id(/entities/0)"}
	]`)

	assocs, err := b.reconstructAssociations("ObjListV10", raw, order)
	if err != nil {
		t.Fatalf("reconstructAssociations: %v", err)
	}
	if len(assocs) != 1 {
		t.Fatalf("want 1 association, got %d", len(assocs))
	}
	a := assocs[0]
	if a.ParentID != model.ID("id-SalesData") || a.ChildID != model.ID("id-Location") {
		t.Fatalf("parent/child resolved wrong: parent=%q child=%q", a.ParentID, a.ChildID)
	}
	if name, ok := b.syntheticName(a.ID); !ok || name != "SalesData_Location" {
		t.Fatalf("assoc synthetic ID not registered")
	}
}

func TestPedUpdate_MarksModuleDirty(t *testing.T) {
	f := newFakePED(t, func(string, map[string]any) (string, bool) { return "SUCCESS", false })
	b := &Backend{client: f.connectClient(t), dirty: map[string]bool{}}

	if b.dirty["MyFirstModule"] {
		t.Fatal("module should start clean")
	}
	if err := b.pedUpdate("MyFirstModule", pedOpEntry{Path: "/entities", Operation: pedOperation{Type: "add", Value: map[string]any{"name": "X"}}}); err != nil {
		t.Fatalf("pedUpdate: %v", err)
	}
	if !b.dirty["MyFirstModule"] {
		t.Fatal("module should be marked dirty after a successful write")
	}
}

func TestLiveAttributeNames(t *testing.T) {
	f := newFakePED(t, func(string, map[string]any) (string, bool) {
		return `{"results":[{"path":"/entities/0/attributes","result":[
			{"$QualifiedName":"M.E.Sku"},{"$QualifiedName":"M.E.Price"}]}]}`, false
	})
	b := &Backend{client: f.connectClient(t)}

	names, err := b.liveAttributeNames("M", 0)
	if err != nil {
		t.Fatalf("liveAttributeNames: %v", err)
	}
	if len(names) != 2 || names[0] != "Sku" || names[1] != "Price" {
		t.Fatalf("got %v, want [Sku Price]", names)
	}
}
