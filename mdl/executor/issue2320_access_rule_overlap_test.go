// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// ako/mxcli#2320: a GRANT with a WHERE meant to narrow a role's access wrote a
// constrained rule beside the role's unconstrained one. Access rules add up, so
// the role still read every row — and exec, check and lint said nothing.

func overlapFixture(t *testing.T, rules []*domainmodel.AccessRule) (*ExecContext, func() string) {
	t.Helper()
	mod := mkModule("Shop")
	h := mkHierarchy(mod)
	cust := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: model.ID("e-cust")},
		Name:        "Customer",
		Attributes:  []*domainmodel.Attribute{{Name: "Name", Type: &domainmodel.StringAttributeType{Length: 200}}},
		AccessRules: rules,
	}
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: "dm-shop"}, ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{cust}}
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc:  func(string) (*model.Module, error) { return mod, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ModuleRoles: []*security.ModuleRole{{Name: "Clerk"}, {Name: "Manager"}}}, nil
		},
		AddEntityAccessRuleFunc:     func(backend.EntityAccessRuleParams) error { return nil },
		ReconcileMemberAccessesFunc: func(model.ID, string) (int, error) { return 0, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, buf.String
}

func openClerkRule() []*domainmodel.AccessRule {
	return []*domainmodel.AccessRule{{ModuleRoleNames: []string{"Shop.Clerk"}, AllowCreate: true,
		DefaultMemberAccessRights: "ReadWrite"}}
}

func constrainedGrant(role string) *ast.GrantEntityAccessStmt {
	return &ast.GrantEntityAccessStmt{
		Roles:           []ast.QualifiedName{{Module: "Shop", Name: role}},
		Entity:          ast.QualifiedName{Module: "Shop", Name: "Customer"},
		Rights:          []ast.EntityAccessRight{{Type: ast.EntityAccessReadAll}},
		XPathConstraint: "[Name != '']",
	}
}

func TestGrantWhere_WarnsBesideAnUnconstrainedRule_Issue2320(t *testing.T) {
	ctx, out := overlapFixture(t, openClerkRule())
	if err := execGrantEntityAccess(ctx, constrainedGrant("Clerk")); err != nil {
		t.Fatal(err)
	}
	got := out()
	if !strings.Contains(got, "MDL-GRANT03") || !strings.Contains(got, "narrows nothing") {
		t.Fatalf("no overlap warning in:\n%s", got)
	}
	if !strings.Contains(got, "revoke all on entity Shop.Customer from Shop.Clerk;") {
		t.Errorf("warning does not name the revoke that narrows it:\n%s", got)
	}
}

// CONTROL: a constrained grant to a role with no unconstrained rule is the
// ordinary row-level rule and warns nothing.
func TestGrantWhere_NoWarningWithoutAnOpenRule_Issue2320(t *testing.T) {
	ctx, out := overlapFixture(t, openClerkRule())
	if err := execGrantEntityAccess(ctx, constrainedGrant("Manager")); err != nil {
		t.Fatal(err)
	}
	if got := out(); strings.Contains(got, "MDL-GRANT03") {
		t.Errorf("unexpected warning for a role with no other rule:\n%s", got)
	}
}

func TestGrantOpen_WarnsBesideAConstrainedRule_Issue2320(t *testing.T) {
	ctx, out := overlapFixture(t, []*domainmodel.AccessRule{{ModuleRoleNames: []string{"Shop.Clerk"},
		XPathConstraint: "[Name != '']", DefaultMemberAccessRights: "ReadOnly"}})
	g := constrainedGrant("Clerk")
	g.XPathConstraint = ""
	if err := execGrantEntityAccess(ctx, g); err != nil {
		t.Fatal(err)
	}
	if got := out(); !strings.Contains(got, "MDL-GRANT03") || !strings.Contains(got, "restricts nothing") {
		t.Errorf("no overlap warning in:\n%s", got)
	}
}

func TestCheck_ScriptOverlap_Issue2320(t *testing.T) {
	both := parseMDL(t, `mdl 1;
grant read * on entity Shop.Customer to Shop.Clerk;
grant read * on entity Shop.Customer to Shop.Clerk where [Name != ''];`)
	if v := ValidateAccessRuleOverlap(both); len(v) != 1 || v[0].RuleID != "MDL-GRANT03" {
		t.Fatalf("violations = %v, want one MDL-GRANT03", v)
	}
	// CONTROL: revoking the rule first is the way to narrow, and is clean.
	revoked := parseMDL(t, `mdl 1;
grant read * on entity Shop.Customer to Shop.Clerk;
revoke all on entity Shop.Customer from Shop.Clerk;
grant read * on entity Shop.Customer to Shop.Clerk where [Name != ''];`)
	if v := ValidateAccessRuleOverlap(revoked); len(v) != 0 {
		t.Errorf("violations = %v, want none after the revoke", v)
	}
}

// With a project, the stored unconstrained rule is what the grant overlaps —
// and the no-project pass does not report it, so it is said once.
func TestCheck_StoredOverlap_Issue2320(t *testing.T) {
	prog := parseMDL(t, `mdl 1;
grant read * on entity Shop.Customer to Shop.Clerk where [Name != ''];`)
	stored := func(entity string) (map[string]bool, map[string]bool, bool) {
		if entity != "Shop.Customer" {
			return nil, nil, false
		}
		return map[string]bool{"Shop.Clerk": true}, map[string]bool{}, true
	}
	if v := accessRuleOverlaps(prog, stored); len(v) != 1 {
		t.Fatalf("violations = %v, want one", v)
	}
	if v := ValidateAccessRuleOverlap(prog); len(v) != 0 {
		t.Errorf("no-project pass reported a stored overlap: %v", v)
	}
	revoked := parseMDL(t, `mdl 1;
revoke all on entity Shop.Customer from Shop.Clerk;
grant read * on entity Shop.Customer to Shop.Clerk where [Name != ''];`)
	if v := accessRuleOverlaps(revoked, stored); len(v) != 0 {
		t.Errorf("violations = %v, want none after the revoke", v)
	}
}
