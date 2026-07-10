// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// Issue #641 — a System.owner/changedBy/... reference on the retrieve's own entity
// must be matched (it needs the member stored), but a traversed reference
// (Assoc/Entity/System.owner, targeting a related entity) must NOT be flagged here.
func TestBaseSystemMemberRe(t *testing.T) {
	cases := []struct {
		constraint string
		wantMember string // "" = no base-entity System member match
	}{
		{"[System.owner = '[%CurrentUser%]']", "owner"},
		{"[System.changedBy = '[%CurrentUser%]']", "changedBy"},
		{"[Title = 'abc' and System.owner = '[%CurrentUser%]']", "owner"},
		{"[Mod.Assoc/Mod.Entity/System.owner = '[%CurrentUser%]']", ""}, // traversed → not base
		{"[Title = 'abc']", ""},
	}
	for _, c := range cases {
		got := ""
		if m := baseSystemMemberRe.FindStringSubmatch(c.constraint); m != nil {
			got = m[2]
		}
		if got != c.wantMember {
			t.Errorf("constraint %q: matched member %q, want %q", c.constraint, got, c.wantMember)
		}
	}
}

func TestEntityStoresSystemMember(t *testing.T) {
	e := &domainmodel.Entity{HasOwner: true, HasChangedDate: true}
	if !entityStoresSystemMember(e, "owner") {
		t.Error("owner should be stored")
	}
	if entityStoresSystemMember(e, "changedBy") {
		t.Error("changedBy should NOT be stored")
	}
	if !entityStoresSystemMember(e, "changedDate") {
		t.Error("changedDate should be stored")
	}
	if entityStoresSystemMember(e, "createdDate") {
		t.Error("createdDate should NOT be stored")
	}
}
