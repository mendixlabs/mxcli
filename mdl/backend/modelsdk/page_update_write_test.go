// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// TestUpdatePage_RoundTrip creates a page, rewrites it in place via UpdatePage
// (preserving the UUID) with a changed URL, and confirms the change persists.
func TestUpdatePage_RoundTrip(t *testing.T) {
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

	page := &pages.Page{
		ContainerID: mod.ID,
		Name:        "ZzPage",
		URL:         "zz-page",
	}
	if err := b.CreatePage(page); err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if page.ID == "" {
		t.Fatalf("CreatePage did not assign an ID")
	}

	// Rewrite in place with a new URL, preserving the UUID.
	page.URL = "zz-page-updated"
	if err := b.UpdatePage(page); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	got, err := b2.GetPage(page.ID)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.Name != "ZzPage" {
		t.Errorf("name = %q, want ZzPage", got.Name)
	}
	if got.URL != "zz-page-updated" {
		t.Errorf("URL not round-tripped: %q, want zz-page-updated", got.URL)
	}
	// The UUID must be preserved by the in-place update.
	if got.ID != page.ID {
		t.Errorf("ID changed on update: was %s, now %s", page.ID, got.ID)
	}
}
