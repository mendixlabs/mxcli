// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// TestListSnippets_PopulatesParameters guards CE1571: the page builder reads a
// snippet's declared Parameters (via ListSnippets) to wire SNIPPETCALL argument
// mappings. If ListSnippets drops them, every parameterised snippet call is
// written with an empty argument → "no argument selected" in Studio Pro.
func TestListSnippets_PopulatesParameters(t *testing.T) {
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
	sn := &pages.Snippet{
		ContainerID: mod.ID,
		Name:        "ZzSnippet",
		Parameters: []*pages.SnippetParameter{
			{Name: "Customer", EntityName: "MyFirstModule.Thing"},
		},
	}
	if err := b.CreateSnippet(sn); err != nil {
		t.Fatalf("CreateSnippet: %v", err)
	}

	// Reopen and confirm the parameter round-trips.
	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	snips, err := b2.ListSnippets()
	if err != nil {
		t.Fatalf("ListSnippets: %v", err)
	}
	for _, s := range snips {
		if s.Name != "ZzSnippet" {
			continue
		}
		if len(s.Parameters) != 1 || s.Parameters[0].Name != "Customer" {
			t.Fatalf("snippet params = %+v, want one named Customer (CE1571 regression)", s.Parameters)
		}
		return
	}
	t.Fatal("ZzSnippet not found after create")
}
