// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// TestPageFromGen_ReadsAllowedRoles guards SHOW ACCESS ON PAGE and the Page
// section of SHOW SECURITY MATRIX: a page's allowed module roles are BY_NAME
// references (stored under the "AllowedModuleRoles" BSON key). The adapter must
// surface them as Page.AllowedRoles, or the security-audit commands report
// "no roles" for a restricted page on the modelsdk engine (issue #722).
func TestPageFromGen_ReadsAllowedRoles(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	pages, err := b.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	var roles []string
	for _, pg := range pages {
		if pg.Name == "Account_Overview" {
			for _, r := range pg.AllowedRoles {
				roles = append(roles, string(r))
			}
		}
	}
	if roles == nil {
		t.Fatal("Account_Overview not found or has no AllowedRoles (page access under-reported)")
	}
	found := false
	for _, r := range roles {
		if r == "Administration.Administrator" {
			found = true
		}
	}
	if !found {
		t.Errorf("AllowedRoles = %v, want to include Administration.Administrator", roles)
	}
}

// findPageIDByName returns the ID of the page with the given short name, or "".
func findPageIDByName(b *Backend, name string) model.ID {
	pages, err := b.ListPages()
	if err != nil {
		return ""
	}
	for _, pg := range pages {
		if pg.Name == name {
			return pg.ID
		}
	}
	return ""
}

// pageAllowedRoles returns the AllowedRoles of the named page as strings (nil if
// the page is missing). This mirrors the read path used by the MPR007 / CE0557
// lint rule (buildPageRoleCountMap counts len(pg.AllowedRoles) from ListPages).
func pageAllowedRoles(b *Backend, name string) []string {
	pages, err := b.ListPages()
	if err != nil {
		return nil
	}
	for _, pg := range pages {
		if pg.Name == name {
			out := make([]string, 0, len(pg.AllowedRoles))
			for _, r := range pg.AllowedRoles {
				out = append(out, string(r))
			}
			return out
		}
	}
	return nil
}

// TestUpdateAllowedRoles_PagePersistsForLintRead is the end-to-end guard for
// issue #696: GRANT VIEW ON PAGE reported success but the page's allowed roles
// appeared empty afterwards (CE0557 / MPR007 persisted). The write always
// persisted to disk — the regression was that the modelsdk read path
// (pageFromGen -> Page.AllowedRoles, which ListPages feeds to the lint rule)
// didn't surface the roles, so `mxcli lint` false-reported "no allowed roles".
//
// This test ties the two halves together on a FRESH reopen: write a role via
// UpdateAllowedRoles (the backend call GRANT VIEW ON PAGE makes), reconnect, and
// assert the lint read path sees it. It fails if either the write stops
// persisting or the read path stops surfacing page allowed roles.
func TestUpdateAllowedRoles_PagePersistsForLintRead(t *testing.T) {
	proj := copyFixture(t)

	// Start from a clean slate: clear Home_Web's allowed roles, then reopen and
	// confirm the lint read path sees zero (the CE0557 precondition).
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	pageID := findPageIDByName(b, "Home_Web")
	if pageID == "" {
		t.Skip("fixture has no Home_Web page")
	}
	if err := b.UpdateAllowedRoles(pageID, nil); err != nil {
		t.Fatalf("clear allowed roles: %v", err)
	}
	_ = b.Disconnect()

	bClear := New()
	if err := bClear.Connect(proj); err != nil {
		t.Fatalf("reconnect after clear: %v", err)
	}
	if got := pageAllowedRoles(bClear, "Home_Web"); len(got) != 0 {
		t.Fatalf("after clear, AllowedRoles = %v, want empty (CE0557 precondition)", got)
	}
	_ = bClear.Disconnect()

	// GRANT VIEW ON PAGE Home_Web TO MyFirstModule.User (what the executor calls).
	bGrant := New()
	if err := bGrant.Connect(proj); err != nil {
		t.Fatalf("reconnect for grant: %v", err)
	}
	if err := bGrant.UpdateAllowedRoles(pageID, []string{"MyFirstModule.User"}); err != nil {
		t.Fatalf("grant allowed role: %v", err)
	}
	_ = bGrant.Disconnect()

	// Fresh reopen: the lint read path must now see the granted role — this is the
	// exact assertion that failed for issue #696 (roles on disk, but read as empty).
	bVerify := New()
	if err := bVerify.Connect(proj); err != nil {
		t.Fatalf("reconnect for verify: %v", err)
	}
	t.Cleanup(func() { _ = bVerify.Disconnect() })
	got := pageAllowedRoles(bVerify, "Home_Web")
	found := false
	for _, r := range got {
		if r == "MyFirstModule.User" {
			found = true
		}
	}
	if !found {
		t.Errorf("after GRANT VIEW ON PAGE, AllowedRoles = %v, want to include MyFirstModule.User (issue #696: role persisted but read as empty -> false CE0557/MPR007)", got)
	}
}
