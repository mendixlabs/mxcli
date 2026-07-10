// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// TestCreateJsonStructure_RoundTrip creates a JSON structure with a nested element
// tree and confirms it round-trips through ListJsonStructures (name, snippet, and
// the recursive element children with their int32 numeric props).
func TestCreateJsonStructure_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	js := &types.JsonStructure{
		ContainerID: mod.ID,
		Name:        "ZzJson",
		JsonSnippet: `{"id":1,"items":["a"]}`,
		Elements: []*types.JsonElement{
			{ExposedName: "id", Path: "id", ElementType: "Value", PrimitiveType: "Integer", MaxOccurs: 1},
			{ExposedName: "items", Path: "items", ElementType: "Array", MaxOccurs: -1, Children: []*types.JsonElement{
				{ExposedName: "item", Path: "items[]", ElementType: "Value", PrimitiveType: "String", MaxLength: 200},
			}},
		},
	}
	if err := b.CreateJsonStructure(js); err != nil {
		t.Fatalf("CreateJsonStructure: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	all, err := b2.ListJsonStructures()
	if err != nil {
		t.Fatalf("ListJsonStructures: %v", err)
	}
	for _, g := range all {
		if g.Name != "ZzJson" {
			continue
		}
		if g.JsonSnippet != js.JsonSnippet {
			t.Errorf("JsonSnippet = %q, want %q", g.JsonSnippet, js.JsonSnippet)
		}
		if len(g.Elements) != 2 {
			t.Fatalf("got %d top-level elements, want 2", len(g.Elements))
		}
		arr := g.Elements[1]
		if arr.ExposedName != "items" || len(arr.Children) != 1 || arr.Children[0].MaxLength != 200 {
			t.Errorf("nested element not round-tripped: %+v", arr)
		}
		return
	}
	t.Fatal("ZzJson not found after create")
}
