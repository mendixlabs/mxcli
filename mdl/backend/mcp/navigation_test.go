// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"os"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

func TestNavMenuAction(t *testing.T) {
	page := navMenuAction(types.NavMenuItemSpec{Page: "MyMod.Overview"})
	if page["$Type"] != "Pages$PageClientAction" {
		t.Fatalf("page action $Type = %v", page["$Type"])
	}
	ps, ok := page["pageSettings"].(map[string]any)
	if !ok || ps["page"] != "MyMod.Overview" || ps["$Type"] != "Pages$PageSettings" {
		t.Fatalf("page action pageSettings = %#v", page["pageSettings"])
	}

	mf := navMenuAction(types.NavMenuItemSpec{Microflow: "MyMod.ACT_Do"})
	if mf["$Type"] != "Pages$MicroflowClientAction" {
		t.Fatalf("microflow action $Type = %v", mf["$Type"])
	}
	ms, ok := mf["microflowSettings"].(map[string]any)
	if !ok || ms["microflow"] != "MyMod.ACT_Do" {
		t.Fatalf("microflow action microflowSettings = %#v", mf["microflowSettings"])
	}

	none := navMenuAction(types.NavMenuItemSpec{Caption: "Admin"})
	if none["$Type"] != "Pages$NoClientAction" {
		t.Fatalf("container action $Type = %v", none["$Type"])
	}
}

func TestNavMenuItemValue(t *testing.T) {
	// A container with one page child (mirrors the ExpenseApproval "Admin" menu).
	spec := types.NavMenuItemSpec{
		Caption: "Admin",
		Items: []types.NavMenuItemSpec{
			{Caption: "Employees", Page: "ExpenseApproval.Employee_Overview"},
		},
	}
	got := navMenuItemValue(spec)
	if got["$Type"] != "Menus$MenuItem" || got["caption"] != "Admin" {
		t.Fatalf("menu item header = %#v", got)
	}
	if _, hasID := got["$ID"]; hasID {
		t.Fatal("menu item must never carry $ID")
	}
	// Container item has no explicit target -> NoClientAction.
	if act := got["action"].(map[string]any); act["$Type"] != "Pages$NoClientAction" {
		t.Fatalf("container action = %#v", act)
	}
	subs, ok := got["items"].([]any)
	if !ok || len(subs) != 1 {
		t.Fatalf("expected 1 sub-item, got %#v", got["items"])
	}
	sub := subs[0].(map[string]any)
	if sub["caption"] != "Employees" {
		t.Fatalf("sub caption = %v", sub["caption"])
	}
	if sub["action"].(map[string]any)["$Type"] != "Pages$PageClientAction" {
		t.Fatalf("sub action = %#v", sub["action"])
	}
}

// TestLive_Navigation exercises the real PED navigation document path against a
// running Studio Pro MCP server. Skipped unless MXCLI_MCP_URL is set. It is
// non-destructive: it locates the profile, appends a uniquely-named container
// menu item, reads it back, then removes it — leaving the menu as it found it.
//
//	MXCLI_MCP_URL=http://localhost/mcp \
//	MXCLI_MCP_DIAL=host.docker.internal:7784 \
//	MXCLI_MCP_NAV_PROFILE=Responsive \
//	go test ./mdl/backend/mcp/ -run TestLive_Navigation -v
func TestLive_Navigation(t *testing.T) {
	url := os.Getenv("MXCLI_MCP_URL")
	if url == "" {
		t.Skip("set MXCLI_MCP_URL to run the live MCP integration test")
	}
	profile := os.Getenv("MXCLI_MCP_NAV_PROFILE")
	if profile == "" {
		profile = "Responsive"
	}

	c, err := NewClient(ClientOptions{URL: url, Dial: os.Getenv("MXCLI_MCP_DIAL")})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	b := &Backend{client: c}

	st, err := b.navProfileState(profile)
	if err != nil {
		t.Fatalf("navProfileState: %v", err)
	}
	if st.isNative {
		t.Skipf("profile %q is native; this test targets a web profile", profile)
	}
	t.Logf("profile %q at index %d, %d menu items", profile, st.index, st.menuItemCount)

	itemsPath := navItemsPath(st.index)
	const probeCaption = "__mxcli_nav_probe__"

	// Append a probe item (container/NoClientAction — no page ref needed).
	value := navMenuItemValue(types.NavMenuItemSpec{Caption: probeCaption})
	if err := b.pedUpdateNav(pedOpEntry{Path: itemsPath, Operation: pedOperation{Type: "add", Value: value}}); err != nil {
		t.Fatalf("add probe menu item: %v", err)
	}

	// Confirm the count grew by one, then restore by removing the last item.
	after, err := b.navProfileState(profile)
	if err != nil {
		t.Fatalf("navProfileState (after add): %v", err)
	}
	removeIdx := after.menuItemCount - 1
	if err := b.pedUpdateNav(pedOpEntry{Path: itemsPath, Operation: pedOperation{Type: "remove", Index: &removeIdx}}); err != nil {
		t.Fatalf("remove probe menu item (MENU LEFT DIRTY at index %d): %v", removeIdx, err)
	}
	if after.menuItemCount != st.menuItemCount+1 {
		t.Fatalf("expected menu to grow by 1 (%d -> %d), got %d", st.menuItemCount, st.menuItemCount+1, after.menuItemCount)
	}

	restored, err := b.navProfileState(profile)
	if err != nil {
		t.Fatalf("navProfileState (after restore): %v", err)
	}
	if restored.menuItemCount != st.menuItemCount {
		t.Fatalf("menu not restored: started %d, ended %d", st.menuItemCount, restored.menuItemCount)
	}
}
