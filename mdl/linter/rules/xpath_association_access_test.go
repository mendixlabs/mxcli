// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// ako/mxcli#945, #1337. Measured on 11.15.0 at Production security, signed in
// as a user role holding only Shop.RoleA, each case below a Data grid 2 over a
// database source whose XPath crosses one association:
//
//   - owner Both, traversed from its TO entity (Account), whose rule leaves the
//     association at None: the page fails, "No access rights for
//     Shop$Account/Shop.Order_Account".
//   - owner Default, traversed from its FROM entity, member at None: fails,
//     "No access rights for Shop$Order3/Shop.Order3_Account".
//   - owner Default, traversed from its TO entity: the association is not a
//     member there, and the page opens.
//   - the same XPath inside an entity ACCESS RULE whose own member is at None:
//     the page opens — the runtime does not check member access there.
//
// Control: with both members granted ReadOnly, all pages open.

func (f *readAccessFixture) association(qn, from, to, owner string) {
	f.n++
	mod, name, _ := strings.Cut(qn, ".")
	f.exec(`INSERT INTO associations_data (Id, Name, QualifiedName, ModuleName, FromEntity, ToEntity, Owner, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?,?)`, "a"+string(rune('a'+f.n)), name, qn, mod, from, to, owner, "default", "s1")
}

func (f *readAccessFixture) xpath(documentType, document, componentType, xpath, target string) {
	f.n++
	f.exec(`INSERT INTO xpath_expressions_data (Id, DocumentType, DocumentId, DocumentQualifiedName, ComponentType, ComponentId,
		XPathExpression, TargetEntity, UsageType, ModuleName, ProjectId, SnapshotId) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		"x"+string(rune('a'+f.n)), documentType, "d-"+document, document, componentType, "c"+string(rune('a'+f.n)),
		xpath, target, "DATASOURCE", moduleFromQualified(document), "default", "s1")
}

func (f *readAccessFixture) checkXPathAccess() []linter.Violation {
	f.t.Helper()
	ctx := linter.NewLintContext(f.cat, navSecurityReader{sec: f.sec})
	return NewXPathAssociationAccessRule().Check(ctx)
}

func shopUser() *security.UserRole {
	return &security.UserRole{Name: "RoleAUser", ModuleRoles: []string{"System.User", "Shop.RoleA"}}
}

// shopFixture is the measured project: Account and Order, Order_Account owner
// Both, RoleA reading Account.Code only and all of Order.
func shopFixture(t *testing.T, level string) *readAccessFixture {
	f := newReadAccessFixture(t, level, shopUser())
	f.entity("Shop.Account", "")
	f.entity("Shop.Order", "")
	f.association("Shop.Order_Account", "Shop.Order", "Shop.Account", "Both")
	f.read("Shop.Account", "Shop.RoleA", "")
	f.memberRead("Shop.Account", "Shop.RoleA", "", "Shop.Account.Code")
	f.read("Shop.Order", "Shop.RoleA", "")
	f.memberRead("Shop.Order", "Shop.RoleA", "", "Shop.Order.IsActive")
	f.memberRead("Shop.Order", "Shop.RoleA", "", "Shop.Order_Account")
	f.page("Shop.Portal", "Shop.RoleA")
	f.widget("Shop.Portal", "dgOrders", dataGrid, "Shop.Account")
	f.xpath("PAGE", "Shop.Portal", "WIDGET", "[Shop.Order_Account/Shop.Order/IsActive = true()]", "Shop.Account")
	return f
}

func TestXPathAssociationAccess_OwnerBothFromTheToSideFails(t *testing.T) {
	v := oneViolation(t, shopFixture(t, security.SecurityLevelProduction).checkXPathAccess())
	if v.RuleID != "SEC011" || v.Severity != linter.SeverityError {
		t.Errorf("rule/severity = %s %v, want SEC011 error — the page fails to open", v.RuleID, v.Severity)
	}
	for _, want := range []string{"'dgOrders'", "Shop.Portal", "Shop.Order_Account", "Shop.Account", "'RoleAUser'", "No access rights for Shop$Account/Shop.Order_Account"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message %q does not mention %s", v.Message, want)
		}
	}
	if v.Location.DocumentType != "page" || v.Location.DocumentName != "Portal" || v.Location.Module != "Shop" {
		t.Errorf("location = %+v", v.Location)
	}
	if !strings.Contains(v.Suggestion, "Order_Account") || !strings.Contains(v.Suggestion, "Shop.Account") {
		t.Errorf("suggestion %q does not say which member to grant on which entity", v.Suggestion)
	}
}

// The control: the member granted on the entity the step starts from.
func TestXPathAssociationAccess_GrantedMemberIsSilent(t *testing.T) {
	f := shopFixture(t, security.SecurityLevelProduction)
	f.memberRead("Shop.Account", "Shop.RoleA", "", "Shop.Order_Account")
	if vs := f.checkXPathAccess(); len(vs) != 0 {
		t.Errorf("want nothing once RoleA reads Order_Account on Account, got %+v", vs)
	}
}

// Granted by another of the user role's module roles counts too: what a person
// logs in as is the user role.
func TestXPathAssociationAccess_AnyModuleRoleOfTheUserRoleCounts(t *testing.T) {
	f := shopFixture(t, security.SecurityLevelProduction)
	f.sec.UserRoles[0].ModuleRoles = append(f.sec.UserRoles[0].ModuleRoles, "Shop.Auditor")
	f.memberRead("Shop.Account", "Shop.Auditor", "[Code != empty]", "Shop.Order_Account")
	if vs := f.checkXPathAccess(); len(vs) != 0 {
		t.Errorf("want nothing, Shop.Auditor reads the member: %+v", vs)
	}
}

func TestXPathAssociationAccess_FromTheFromSideFails(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, shopUser())
	f.entity("Shop.Account", "")
	f.entity("Shop.Order3", "")
	f.association("Shop.Order3_Account", "Shop.Order3", "Shop.Account", "Default")
	f.read("Shop.Order3", "Shop.RoleA", "")
	f.memberRead("Shop.Order3", "Shop.RoleA", "", "Shop.Order3.IsActive")
	f.read("Shop.Account", "Shop.RoleA", "")
	f.page("Shop.P_Forward", "Shop.RoleA")
	f.widget("Shop.P_Forward", "dgForward", dataGrid, "Shop.Order3")
	f.xpath("PAGE", "Shop.P_Forward", "WIDGET", "[Shop.Order3_Account/Shop.Account/Code = 'x']", "Shop.Order3")

	v := oneViolation(t, f.checkXPathAccess())
	if !strings.Contains(v.Message, "No access rights for Shop$Order3/Shop.Order3_Account") {
		t.Errorf("message = %q", v.Message)
	}
}

// Measured: a Default-owner association is not a member of its TO entity, and
// traversing it from there opened the page. Must stay silent.
func TestXPathAssociationAccess_DefaultOwnerFromTheToSideIsSilent(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, shopUser())
	f.entity("Shop.Account", "")
	f.entity("Shop.Order2", "")
	f.association("Shop.Order2_Account", "Shop.Order2", "Shop.Account", "Default")
	f.read("Shop.Account", "Shop.RoleA", "")
	f.memberRead("Shop.Account", "Shop.RoleA", "", "Shop.Account.Code")
	f.read("Shop.Order2", "Shop.RoleA", "")
	f.page("Shop.P_Default", "Shop.RoleA")
	f.widget("Shop.P_Default", "dgDefault", dataGrid, "Shop.Account")
	f.xpath("PAGE", "Shop.P_Default", "WIDGET", "[Shop.Order2_Account/Shop.Order2/IsActive = true()]", "Shop.Account")

	if vs := f.checkXPathAccess(); len(vs) != 0 {
		t.Errorf("want nothing, got %+v", vs)
	}
}

// Measured: the runtime does not check member access inside an access rule's
// own XPath. This pins the measured outcome; an access rule's XPath has no page
// roles, so it is also silent before the rule's ComponentType filter is reached.
func TestXPathAssociationAccess_AccessRuleXPathIsSilent(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, shopUser())
	f.entity("Shop.Account4", "")
	f.entity("Shop.Order4", "")
	f.association("Shop.Order4_Account4", "Shop.Order4", "Shop.Account4", "Both")
	f.read("Shop.Account4", "Shop.RoleA", "[Shop.Order4_Account4/Shop.Order4/IsActive = true()]")
	f.xpath("DOMAIN_MODEL", "Shop.DomainModel", "ACCESS_RULE", "[Shop.Order4_Account4/Shop.Order4/IsActive = true()]", "Shop.Account4")

	if vs := f.checkXPathAccess(); len(vs) != 0 {
		t.Errorf("want nothing, got %+v", vs)
	}
}

// Below Production the runtime does not apply entity access, so nothing is
// refused.
func TestXPathAssociationAccess_PrototypeIsSilent(t *testing.T) {
	if vs := shopFixture(t, security.SecurityLevelPrototype).checkXPathAccess(); len(vs) != 0 {
		t.Errorf("want nothing below Production, got %+v", vs)
	}
}

// A role that cannot open the page is not asked; one that cannot read the
// starting entity at all is SEC010's finding, not this rule's.
func TestXPathAssociationAccess_OnlyRolesThatOpenThePageAndReadTheEntity(t *testing.T) {
	f := shopFixture(t, security.SecurityLevelProduction)
	f.sec.UserRoles = append(f.sec.UserRoles, &security.UserRole{Name: "Outsider", ModuleRoles: []string{"System.User", "Shop.Other"}})
	if vs := f.checkXPathAccess(); len(vs) != 1 || strings.Contains(vs[0].Message, "Outsider") {
		t.Errorf("want only RoleAUser reported, got %+v", vs)
	}

	g := shopFixture(t, security.SecurityLevelProduction)
	g.exec(`DELETE FROM permissions WHERE ElementName = 'Shop.Account'`)
	if vs := g.checkXPathAccess(); len(vs) != 0 {
		t.Errorf("RoleA cannot read Shop.Account at all (SEC010's finding); want nothing here, got %+v", vs)
	}
}

// Every hop is checked from the entity it starts on: the second step here
// starts on Order, whose rule leaves Order_Customer at None.
func TestXPathAssociationAccess_SecondHopStartsOnTheIntermediateEntity(t *testing.T) {
	f := shopFixture(t, security.SecurityLevelProduction)
	f.memberRead("Shop.Account", "Shop.RoleA", "", "Shop.Order_Account")
	f.entity("Shop.Customer", "")
	f.association("Shop.Order_Customer", "Shop.Order", "Shop.Customer", "Default")
	f.read("Shop.Customer", "Shop.RoleA", "")
	f.xpath("PAGE", "Shop.Portal", "WIDGET", "[Shop.Order_Account/Shop.Order[Shop.Order_Customer/Shop.Customer/Name = 'x']]", "Shop.Account")

	v := oneViolation(t, f.checkXPathAccess())
	if !strings.Contains(v.Message, "Shop$Order/Shop.Order_Customer") {
		t.Errorf("message = %q, want the hop from Shop.Order", v.Message)
	}
}
