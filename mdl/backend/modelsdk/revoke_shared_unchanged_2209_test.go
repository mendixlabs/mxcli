// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#2209: `revoke create, delete, write * on entity M.E from M.Admin`
// refused with "the access rule on E for Customer, SupportAgent, Admin also
// grants Customer, SupportAgent, which this revoke does not name" — on a shared
// rule that grants only `read *`, so the revoke would not change it at all.
//
// #1501's refusal exists so a shared rule is not narrowed for roles the
// statement does not name. A shared rule the revocation leaves as it is narrows
// nobody; it is skipped, not refused. A shared rule it WOULD change still
// refuses.
package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

const (
	roleAdmin    = "MyFirstModule.RoleA"
	roleCustomer = "MyFirstModule.RoleB"
	roleAgent    = "MyFirstModule.RoleC"
)

func revokeCreateDeleteWriteAll() types.EntityAccessRevocation {
	return types.EntityAccessRevocation{RevokeCreate: true, RevokeDelete: true, RevokeWriteAll: true}
}

// The reported shape: an Admin-only CRUD rule plus a read-only rule shared
// with Customer and SupportAgent.
func TestRevokeFromOneRole_SkipsASharedRuleItWouldNotChange(t *testing.T) {
	b, mod, dm := grantFixture(t)
	admin := grantParams(dm.ID, "ReadWrite", "", nil)
	admin.RoleNames = []string{roleAdmin}
	admin.AllowCreate, admin.AllowDelete = true, true
	if err := b.AddEntityAccessRule(admin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	shared := grantParams(dm.ID, "ReadOnly", "", nil)
	shared.RoleNames = []string{roleCustomer, roleAgent, roleAdmin}
	if err := b.AddEntityAccessRule(shared); err != nil {
		t.Fatalf("grant shared: %v", err)
	}

	n, err := b.RevokeEntityMemberAccess(dm.ID, "ZzCust", []string{roleAdmin}, revokeCreateDeleteWriteAll())
	if err != nil {
		t.Fatalf("revoke refused a shared rule it would not change: %v", err)
	}
	if n != 1 {
		t.Errorf("rules modified = %d, want 1 (the Admin-only rule)", n)
	}
	for _, rule := range rulesOf(t, b, mod.ID) {
		switch {
		case sameStringSet(rule.ModuleRoleNames, []string{roleAdmin}):
			if rule.AllowCreate || rule.AllowDelete {
				t.Errorf("Admin rule: create=%v delete=%v, want both revoked", rule.AllowCreate, rule.AllowDelete)
			}
			if string(rule.DefaultMemberAccessRights) != "ReadOnly" {
				t.Errorf("Admin rule: default = %q, want ReadOnly", rule.DefaultMemberAccessRights)
			}
			for m, got := range rightsOf(rule) {
				if got != "ReadOnly" {
					t.Errorf("Admin rule: %s = %q, want ReadOnly", m, got)
				}
			}
		case sameStringSet(rule.ModuleRoleNames, []string{roleCustomer, roleAgent, roleAdmin}):
			if rule.AllowCreate || rule.AllowDelete || string(rule.DefaultMemberAccessRights) != "ReadOnly" {
				t.Errorf("shared rule changed: create=%v delete=%v default=%q",
					rule.AllowCreate, rule.AllowDelete, rule.DefaultMemberAccessRights)
			}
			for m, got := range rightsOf(rule) {
				if got != "ReadOnly" {
					t.Errorf("shared rule: %s = %q, want ReadOnly", m, got)
				}
			}
		default:
			t.Errorf("unexpected rule for %v", rule.ModuleRoleNames)
		}
	}

	// Second run: nothing left to revoke, and still no refusal.
	if n, err := b.RevokeEntityMemberAccess(dm.ID, "ZzCust", []string{roleAdmin}, revokeCreateDeleteWriteAll()); err != nil || n != 0 {
		t.Errorf("rerun: modified = %d, err = %v; want 0, nil", n, err)
	}
}

// Control: #1501's guard stands when the shared rule WOULD change — here it
// grants create, which the revoke takes away. Refused, nothing written.
func TestRevokeFromOneRole_StillRefusesASharedRuleItWouldChange(t *testing.T) {
	b, mod, dm := grantFixture(t)
	admin := grantParams(dm.ID, "ReadWrite", "", nil)
	admin.RoleNames = []string{roleAdmin}
	admin.AllowCreate, admin.AllowDelete = true, true
	if err := b.AddEntityAccessRule(admin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	shared := grantParams(dm.ID, "ReadOnly", "", nil)
	shared.RoleNames = []string{roleCustomer, roleAdmin}
	shared.AllowCreate = true
	if err := b.AddEntityAccessRule(shared); err != nil {
		t.Fatalf("grant shared: %v", err)
	}

	n, err := b.RevokeEntityMemberAccess(dm.ID, "ZzCust", []string{roleAdmin}, revokeCreateDeleteWriteAll())
	if err == nil {
		t.Fatalf("revoke succeeded (%d rules); want a refusal naming the shared rule", n)
	}
	if !strings.Contains(err.Error(), roleCustomer) {
		t.Errorf("error does not name the unnamed role: %v", err)
	}
	for _, rule := range rulesOf(t, b, mod.ID) {
		if !rule.AllowCreate {
			t.Errorf("rule %v lost create before the refusal", rule.ModuleRoleNames)
		}
	}
}
