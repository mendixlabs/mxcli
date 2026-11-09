// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// readAccessFixture is a catalog plus project security for SEC010. Pages,
// widgets, entities and rules are added per test.
type readAccessFixture struct {
	t   *testing.T
	cat *catalog.Catalog
	sec *security.ProjectSecurity
	n   int
}

func newReadAccessFixture(t *testing.T, level string, userRoles ...*security.UserRole) *readAccessFixture {
	t.Helper()
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	f := &readAccessFixture{t: t, cat: cat, sec: &security.ProjectSecurity{SecurityLevel: level, UserRoles: userRoles}}
	for _, mod := range []string{"App", "Administration"} {
		f.exec(`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`, mod+"-id", mod, "default", "s1")
	}
	return f
}

func (f *readAccessFixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.cat.CatalogDB().Exec(q, args...); err != nil {
		f.t.Fatalf("%s: %v", q, err)
	}
}

func (f *readAccessFixture) entity(qn, generalization string) {
	f.n++
	mod, name, _ := strings.Cut(qn, ".")
	f.exec(`INSERT INTO entities_data (Id, Name, QualifiedName, ModuleName, Generalization, ProjectId, SnapshotId) VALUES (?,?,?,?,?,?,?)`,
		"e"+string(rune('a'+f.n)), name, qn, mod, generalization, "default", "s1")
}

func (f *readAccessFixture) read(entity, moduleRole, xpath string) {
	f.exec(`INSERT INTO permissions (ModuleRoleName, ElementType, ElementName, AccessType, XPathConstraint, ModuleName) VALUES (?,?,?,?,?,?)`,
		moduleRole, "ENTITY", entity, "READ", xpath, moduleFromQualified(entity))
}

// memberRead records a MEMBER_READ row as the catalog writes it: the member
// qualified by the entity that DECLARES it (System.User.Name on an
// Administration.Account rule).
func (f *readAccessFixture) memberRead(entity, moduleRole, xpath, member string) {
	f.exec(`INSERT INTO permissions (ModuleRoleName, ElementType, ElementName, MemberName, AccessType, XPathConstraint, ModuleName) VALUES (?,?,?,?,?,?,?)`,
		moduleRole, "ENTITY", entity, member, "MEMBER_READ", xpath, moduleFromQualified(entity))
}

func (f *readAccessFixture) page(page string, moduleRoles ...string) {
	for _, r := range moduleRoles {
		f.exec(`INSERT INTO permissions (ModuleRoleName, ElementType, ElementName, AccessType, ModuleName) VALUES (?,?,?,?,?)`,
			r, "PAGE", page, "VIEW", moduleFromQualified(page))
	}
}

func (f *readAccessFixture) widget(page, name, widgetType, entity string) {
	f.n++
	f.exec(`INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType, ModuleName, EntityRef, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"w"+string(rune('a'+f.n)), name, widgetType, "c-"+page, page, "PAGE", moduleFromQualified(page), entity, "default", "s1")
}

func (f *readAccessFixture) ctx() *linter.LintContext {
	return linter.NewLintContext(f.cat, navSecurityReader{sec: f.sec})
}

func (f *readAccessFixture) check() []linter.Violation {
	f.t.Helper()
	return NewPageEntityReadAccessRule().Check(f.ctx())
}

const (
	dataGrid = "com.mendix.widget.web.datagrid.Datagrid"
	comboBox = "com.mendix.widget.web.combobox.Combobox"
)

func plainUser() *security.UserRole {
	return &security.UserRole{Name: "Member", ModuleRoles: []string{"System.User", "App.User", "Administration.User"}}
}

func administrator() *security.UserRole {
	return &security.UserRole{Name: "Administrator", ModuleRoles: []string{"System.Administrator", "App.Admin", "Administration.Administrator"}}
}

func oneViolation(t *testing.T, vs []linter.Violation) linter.Violation {
	t.Helper()
	if len(vs) != 1 {
		t.Fatalf("want 1 violation, got %d: %+v", len(vs), vs)
	}
	return vs[0]
}

// A page open to App.User with a grid over App.Order, which App.User has no
// read rule on: at Production the grid renders empty for Member.
func TestPageEntityReadAccess_NoReadRuleWarns(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser())
	f.entity("App.Order", "")
	f.read("App.Order", "App.Admin", "")
	f.page("App.Orders", "App.User")
	f.widget("App.Orders", "dg1", dataGrid, "App.Order")

	v := oneViolation(t, f.check())
	if v.Severity != linter.SeverityWarning {
		t.Errorf("severity = %v, want warning", v.Severity)
	}
	want := "Data grid 'dg1' on page App.Orders lists App.Order, which none of the page's roles (App.User) may read — it renders empty for them"
	if !strings.HasPrefix(v.Message, want) {
		t.Errorf("message = %q\nwant prefix %q", v.Message, want)
	}
	if v.RuleID != "SEC010" || v.Location.DocumentName != "Orders" || v.Location.Module != "App" {
		t.Errorf("rule/location = %s %+v", v.RuleID, v.Location)
	}
}

// The control: the same page once App.User may read App.Order.
func TestPageEntityReadAccess_ReadRuleIsSilent(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser())
	f.entity("App.Order", "")
	f.read("App.Order", "App.User", "")
	f.page("App.Orders", "App.User")
	f.widget("App.Orders", "dg1", dataGrid, "App.Order")

	if vs := f.check(); len(vs) != 0 {
		t.Errorf("want nothing, got %+v", vs)
	}
}

// Read granted on a generalization is inherited.
func TestPageEntityReadAccess_GeneralizationRuleCounts(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser())
	f.entity("App.Party", "")
	f.entity("App.Customer", "App.Party")
	f.read("App.Party", "App.User", "")
	f.page("App.Customers", "App.User")
	f.widget("App.Customers", "dg1", dataGrid, "App.Customer")

	if vs := f.check(); len(vs) != 0 {
		t.Errorf("want nothing, got %+v", vs)
	}
}

// A row-scoped rule is often the intent ("my own requests"): info, not warning.
func TestPageEntityReadAccess_RowScopedOnlyIsInfo(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser())
	f.entity("App.Order", "")
	f.read("App.Order", "App.User", "[System.owner = '[%CurrentUser%]']")
	f.page("App.Orders", "App.User")
	f.widget("App.Orders", "dg1", dataGrid, "App.Order")

	v := oneViolation(t, f.check())
	if v.Severity != linter.SeverityInfo {
		t.Errorf("severity = %v, want info", v.Severity)
	}
	if !strings.Contains(v.Message, "XPath-constrained") {
		t.Errorf("message = %q", v.Message)
	}
}

// ChipCoV9: a combo box over System.User for a non-admin role. System.User's
// own rule is [id = '[%CurrentUser%]'] (measured, meta.SystemReadAccess), and
// the role cannot read a specialization in full either.
func TestPageEntityReadAccess_SystemUserForPlainRole(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser(), administrator())
	f.entity("Administration.Account", "System.User")
	f.read("Administration.Account", "Administration.Administrator", "")
	f.read("Administration.Account", "Administration.User", "[id = '[%CurrentUser%]']")
	f.page("App.Assign", "App.User")
	f.widget("App.Assign", "cbEngineer", comboBox, "System.User")

	v := oneViolation(t, f.check())
	if v.Severity != linter.SeverityWarning {
		t.Errorf("severity = %v, want warning", v.Severity)
	}
	for _, want := range []string{
		"Combo box 'cbEngineer' on page App.Assign lists System.User",
		"lists only the current user for user role 'Member'",
		"denormalise the name onto an entity of your own (e.g. Administration.Account) or use a microflow data source",
	} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message %q\nlacks %q", v.Message, want)
		}
	}
}

// Administrator reads Administration.Account, a specialization of System.User,
// unconstrained — so its user list fills. Nothing to report.
func TestPageEntityReadAccess_SystemUserForAdministrator(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser(), administrator())
	f.entity("Administration.Account", "System.User")
	f.read("Administration.Account", "Administration.Administrator", "")
	f.page("App.Assign", "App.Admin")
	f.widget("App.Assign", "cbEngineer", comboBox, "System.User")

	if vs := f.check(); len(vs) != 0 {
		t.Errorf("want nothing, got %+v", vs)
	}
}

// ChipCoV9, re-checked: the real Mendix 11.15 Administration module grants
// Administration.User `read (FullName, Email)` on Account UNCONSTRAINED. The
// rows of a list over System.User come through, but System.User's own members
// (Name) are not in that rule, so every caption is blank except the current
// user's (System.User's own [id = '[%CurrentUser%]'] rule). Measured in the
// browser: ["", "", "demo_manager", ""]. The unconstrained entity-level read
// on the specialization made SEC010 call System.User readable and stay silent.
func TestPageEntityReadAccess_SystemUserMembersNotGrantedBySpecialization(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser(), administrator())
	f.entity("Administration.Account", "System.User")
	f.read("Administration.Account", "Administration.User", "")
	f.memberRead("Administration.Account", "Administration.User", "", "Administration.Account.FullName")
	f.memberRead("Administration.Account", "Administration.User", "", "Administration.Account.Email")
	f.page("App.Assign", "App.User")
	f.widget("App.Assign", "cbEngineer", comboBox, "System.User")

	v := oneViolation(t, f.check())
	if v.Severity != linter.SeverityWarning {
		t.Errorf("severity = %v, want warning", v.Severity)
	}
	for _, want := range []string{
		"Combo box 'cbEngineer' on page App.Assign lists System.User",
		"user role 'Member' may read through Administration.Account",
		"Administration.Account",
		"blank",
	} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message %q\nlacks %q", v.Message, want)
		}
	}
}

// An Administrator's Account rule grants no System.User member either, yet
// every name shows: System.User's own rule also lets a user role read the users
// whose roles it may manage. Measured on 11.15 (ChipCoV9 scratch): with Manage
// All Roles every name shows; a ServiceManager given ManageableRoles
// (FieldEngineer) saw demo_engineer's name and no other but their own.
func TestPageEntityReadAccess_SystemUserManageAllRolesReadsMembers(t *testing.T) {
	admin := administrator()
	admin.ManageAllRoles = true
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser(), admin)
	f.entity("Administration.Account", "System.User")
	f.read("Administration.Account", "Administration.Administrator", "")
	f.memberRead("Administration.Account", "Administration.Administrator", "", "Administration.Account.FullName")
	f.page("App.Assign", "App.Admin")
	f.widget("App.Assign", "cbEngineer", comboBox, "System.User")

	if vs := f.check(); len(vs) != 0 {
		t.Errorf("want nothing, got %+v", vs)
	}
}

// The control: the same rule, now also granting System.User.Name.
func TestPageEntityReadAccess_SpecializationGrantingInheritedMemberIsSilent(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser(), administrator())
	f.entity("Administration.Account", "System.User")
	f.read("Administration.Account", "Administration.User", "")
	f.memberRead("Administration.Account", "Administration.User", "", "Administration.Account.FullName")
	f.memberRead("Administration.Account", "Administration.User", "", "System.User.Name")
	f.page("App.Assign", "App.User")
	f.widget("App.Assign", "cbEngineer", comboBox, "System.User")

	if vs := f.check(); len(vs) != 0 {
		t.Errorf("want nothing, got %+v", vs)
	}
}

// System entities have no rules in the .mpr; without the measured table every
// one of them would be flagged. System.Language is readable by every role.
func TestPageEntityReadAccess_SystemLanguageIsReadable(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser())
	f.page("App.Profile", "App.User")
	f.widget("App.Profile", "cbLanguage", comboBox, "System.Language")

	if vs := f.check(); len(vs) != 0 {
		t.Errorf("want nothing, got %+v", vs)
	}
}

// An admin-only System entity on a page a plain role may open.
func TestPageEntityReadAccess_AdminOnlySystemEntity(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser(), administrator())
	f.page("App.Sessions", "App.User", "App.Admin")
	f.widget("App.Sessions", "dgSessions", dataGrid, "System.Session")

	v := oneViolation(t, f.check())
	if !strings.Contains(v.Message, "which user role 'Member' can open the page but may not read") {
		t.Errorf("message = %q", v.Message)
	}
}

// Prototype security is stored as CheckFormsAndMicroflows: entity access is not
// enforced, the data is not withheld, so there is nothing to report. Off too.
func TestPageEntityReadAccess_OnlyAtProduction(t *testing.T) {
	for _, level := range []string{security.SecurityLevelPrototype, security.SecurityLevelOff} {
		f := newReadAccessFixture(t, level, plainUser())
		f.entity("App.Order", "")
		f.page("App.Orders", "App.User")
		f.widget("App.Orders", "dg1", dataGrid, "App.Order")
		if vs := f.check(); len(vs) != 0 {
			t.Errorf("%s: want nothing, got %+v", level, vs)
		}
	}
}

// Only list data sources: a data view's object usually arrives as a page
// parameter, and an action button's entity is not a list.
func TestPageEntityReadAccess_IgnoresNonListWidgets(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelProduction, plainUser())
	f.entity("App.Order", "")
	f.page("App.Orders", "App.User")
	f.widget("App.Orders", "dv1", "Forms$DataView", "App.Order")
	f.widget("App.Orders", "btn1", "Forms$ActionButton", "App.Order")

	if vs := f.check(); len(vs) != 0 {
		t.Errorf("want nothing, got %+v", vs)
	}
}

func TestPageEntityReadAccess_Identity(t *testing.T) {
	r := NewPageEntityReadAccessRule()
	if r.ID() != "SEC010" || r.Category() != "security" || r.Name() == "" || r.Description() == "" {
		t.Errorf("identity: %s %s %s", r.ID(), r.Category(), r.Name())
	}
	if r.RequiredCatalogMode() != linter.CatalogFull {
		t.Error("widgets and permissions need the full catalog")
	}
}
