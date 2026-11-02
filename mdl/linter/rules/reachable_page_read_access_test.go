// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// ako/mxcli#936. Each case is one mxbuild measurement (11.15.0 and 11.13.0,
// Production security, user role ProbeUser holding Probe.User and System.User,
// whose rule on Probe.Ticket reads Code and leaves Title at None): CE2729 on a
// grid column bound to Title, or not.

func (f *readAccessFixture) ref(source, kind, targetType, target string) {
	f.exec(`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?,?)`, "DOC", "s-"+source, source, targetType, target, kind, moduleFromQualified(source), "default", "s1")
}

func (f *readAccessFixture) pageDoc(qn string) {
	f.n++
	mod, name, _ := strings.Cut(qn, ".")
	f.exec(`INSERT INTO pages_data (Id, Name, QualifiedName, ModuleName, ProjectId, SnapshotId) VALUES (?,?,?,?,?,?)`,
		"p"+string(rune('a'+f.n)), name, qn, mod, "default", "s1")
}

func (f *readAccessFixture) snippetDoc(qn string) {
	f.n++
	mod, name, _ := strings.Cut(qn, ".")
	f.exec(`INSERT INTO snippets_data (Id, Name, QualifiedName, ModuleName, ProjectId, SnapshotId) VALUES (?,?,?,?,?,?)`,
		"s"+string(rune('a'+f.n)), name, qn, mod, "default", "s1")
}

func (f *readAccessFixture) menuItem(page string) {
	f.n++
	f.exec(`INSERT INTO navigation_menu_items (ProfileName, ItemPath, Depth, Caption, ActionType, TargetPage, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?)`, "Responsive", string(rune('a'+f.n)), 0, page, "PageAction", page, "default", "s1")
}

func (f *readAccessFixture) profileHome(page string) {
	f.exec(`INSERT INTO navigation_profiles_data (ProfileName, HomePage, HomePageType, ProjectId, SnapshotId) VALUES (?,?,?,?,?)`,
		"Responsive", page, "PAGE", "default", "s1")
}

func (f *readAccessFixture) execute(flow, moduleRole string) {
	f.exec(`INSERT INTO permissions (ModuleRoleName, ElementType, ElementName, AccessType, ModuleName) VALUES (?,?,?,?,?)`,
		moduleRole, "MICROFLOW", flow, "EXECUTE", moduleFromQualified(flow))
}

// grid adds a page whose data grid over entity shows attr.
func (f *readAccessFixture) grid(page, entity, attr string, viewers ...string) {
	f.pageDoc(page)
	f.page(page, viewers...)
	f.ref(page, "datasource", "ENTITY", entity)
	f.ref(page, "member", "ATTRIBUTE", attr)
}

func (f *readAccessFixture) checkReach() []linter.Violation {
	f.t.Helper()
	ctx := linter.NewLintContext(f.cat, navSecurityReader{sec: f.sec})
	return NewReachablePageReadAccessRule().Check(ctx)
}

func probeUser() *security.UserRole {
	return &security.UserRole{Name: "ProbeUser", ModuleRoles: []string{"System.User", "Probe.User"}}
}

func reachFixture(t *testing.T, level string) *readAccessFixture {
	f := newReadAccessFixture(t, level, probeUser())
	f.exec(`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES ('probe-id', 'Probe', 'default', 's1')`)
	f.entity("Probe.Ticket", "")
	f.read("Probe.Ticket", "Probe.User", "")
	f.memberRead("Probe.Ticket", "Probe.User", "", "Probe.Ticket.Code")
	return f
}

func reported(vs []linter.Violation) []string {
	var out []string
	for _, v := range vs {
		out = append(out, v.Location.Module+"."+v.Location.DocumentName)
	}
	sort.Strings(out)
	return out
}

func TestReachablePageRead_MenuPageIsReported(t *testing.T) {
	f := reachFixture(t, security.SecurityLevelProduction)
	f.grid("Probe.P_Menu", "Probe.Ticket", "Probe.Ticket.Title", "Probe.User")
	f.menuItem("Probe.P_Menu")

	v := oneViolation(t, f.checkReach())
	if v.RuleID != "SEC012" || v.Severity != linter.SeverityError {
		t.Errorf("rule/severity = %s %v", v.RuleID, v.Severity)
	}
	for _, want := range []string{"Probe.P_Menu", "'ProbeUser'", `CE2729 "No read access to attribute 'Title' in entity 'Probe.Ticket'"`} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message %q lacks %s", v.Message, want)
		}
	}
}

// The control: the member granted.
func TestReachablePageRead_GrantedMemberIsSilent(t *testing.T) {
	f := reachFixture(t, security.SecurityLevelProduction)
	f.memberRead("Probe.Ticket", "Probe.User", "", "Probe.Ticket.Title")
	f.grid("Probe.P_Menu", "Probe.Ticket", "Probe.Ticket.Title", "Probe.User")
	f.menuItem("Probe.P_Menu")
	if vs := f.checkReach(); len(vs) != 0 {
		t.Errorf("want nothing, got %+v", vs)
	}
}

// Measured: an unreferenced page, a menu page the role may not view, and a
// page reached only through a microflow the role may not execute are all
// clean; a page reached through a show-page button, a snippet, a microflow the
// role may execute, its sub-microflow, or a nanoflow, is reported.
func TestReachablePageRead_ReachabilityMatchesMxbuild(t *testing.T) {
	f := reachFixture(t, security.SecurityLevelProduction)
	for _, p := range []string{"P_Orphan", "P_ByButton", "P_ByMicroflow", "P_ByMicroflowNoAccess", "P_FromSnippet", "P_MfChain", "P_Nanoflow"} {
		f.grid("Probe."+p, "Probe.Ticket", "Probe.Ticket.Title", "Probe.User")
	}
	f.grid("Probe.P_NotViewable", "Probe.Ticket", "Probe.Ticket.Title", "Probe.Admin")
	f.menuItem("Probe.P_NotViewable")

	f.pageDoc("Probe.P_Menu")
	f.page("Probe.P_Menu", "Probe.User")
	f.menuItem("Probe.P_Menu")
	f.ref("Probe.P_Menu", "show_page", "PAGE", "Probe.P_ByButton")
	f.ref("Probe.P_Menu", "action", "MICROFLOW", "Probe.MF_Open")
	f.ref("Probe.P_Menu", "action", "MICROFLOW", "Probe.MF_OpenNoAccess")
	f.ref("Probe.P_Menu", "action", "MICROFLOW", "Probe.MF_Outer")
	f.ref("Probe.P_Menu", "action", "NANOFLOW", "Probe.NF_Open")
	f.ref("Probe.P_Menu", "show_page", "PAGE", "Probe.S_Buttons") // a snippet call
	f.snippetDoc("Probe.S_Buttons")
	f.ref("Probe.S_Buttons", "show_page", "PAGE", "Probe.P_FromSnippet")
	f.execute("Probe.MF_Open", "Probe.User")
	f.execute("Probe.MF_OpenNoAccess", "Probe.Admin")
	f.execute("Probe.MF_Outer", "Probe.User")
	f.exec(`INSERT INTO permissions (ModuleRoleName, ElementType, ElementName, AccessType, ModuleName) VALUES ('Probe.User', 'NANOFLOW', 'Probe.NF_Open', 'EXECUTE', 'Probe')`)
	f.ref("Probe.MF_Open", "show_page", "PAGE", "Probe.P_ByMicroflow")
	f.ref("Probe.MF_OpenNoAccess", "show_page", "PAGE", "Probe.P_ByMicroflowNoAccess")
	f.ref("Probe.MF_Outer", "call", "MICROFLOW", "Probe.MF_Inner") // MF_Inner has no allowed roles
	f.ref("Probe.MF_Inner", "show_page", "PAGE", "Probe.P_MfChain")
	f.ref("Probe.NF_Open", "show_page", "PAGE", "Probe.P_Nanoflow")

	got := strings.Join(reported(f.checkReach()), ",")
	want := "Probe.P_ByButton,Probe.P_ByMicroflow,Probe.P_FromSnippet,Probe.P_MfChain,Probe.P_Nanoflow"
	if got != want {
		t.Errorf("reported %s\nwant     %s", got, want)
	}
}

// Measured on JTSBootLogboek: the profile's home page is checked for a role
// that may not view it, and nothing it links to is.
func TestReachablePageRead_HomePageCheckedButNotWalkedWhenNotViewable(t *testing.T) {
	f := reachFixture(t, security.SecurityLevelProduction)
	f.grid("Probe.Home_Web", "Probe.Ticket", "Probe.Ticket.Title", "Probe.Admin")
	f.profileHome("Probe.Home_Web")
	f.grid("Probe.P_Linked", "Probe.Ticket", "Probe.Ticket.Title", "Probe.User")
	f.ref("Probe.Home_Web", "show_page", "PAGE", "Probe.P_Linked")

	if got := strings.Join(reported(f.checkReach()), ","); got != "Probe.Home_Web" {
		t.Errorf("reported %s, want only Probe.Home_Web", got)
	}
}

// A role with no read rule at all gets the entity error besides the attribute.
func TestReachablePageRead_NoRuleReportsEntityAndAttribute(t *testing.T) {
	f := reachFixture(t, security.SecurityLevelProduction)
	f.entity("Probe.Ticket2", "")
	f.grid("Probe.P_NoRule", "Probe.Ticket2", "Probe.Ticket2.Title", "Probe.User")
	f.menuItem("Probe.P_NoRule")

	vs := f.checkReach()
	if len(vs) != 2 || !strings.Contains(vs[0].Message, `"No access to entity 'Probe.Ticket2'"`) ||
		!strings.Contains(vs[1].Message, `"No read access to attribute 'Title' in entity 'Probe.Ticket2'"`) {
		t.Errorf("want the entity error then the attribute error, got %+v", vs)
	}
}

// Measured: an inherited attribute counts against the specialization's rules.
func TestReachablePageRead_InheritedAttribute(t *testing.T) {
	f := reachFixture(t, security.SecurityLevelProduction)
	f.entity("Probe.Base", "")
	f.entity("Probe.Ticket3", "Probe.Base")
	f.read("Probe.Ticket3", "Probe.User", "")
	f.memberRead("Probe.Ticket3", "Probe.User", "", "Probe.Ticket3.Own")
	f.grid("Probe.P_Inherited", "Probe.Ticket3", "Probe.Base.BaseAttr", "Probe.User")
	f.menuItem("Probe.P_Inherited")

	v := oneViolation(t, f.checkReach())
	if !strings.Contains(v.Message, "in entity 'Probe.Ticket3'") {
		t.Errorf("message = %q, want it named after the specialization", v.Message)
	}
	// Control: the inherited member granted on the specialization's rule.
	f.memberRead("Probe.Ticket3", "Probe.User", "", "Probe.Base.BaseAttr")
	if vs := f.checkReach(); len(vs) != 0 {
		t.Errorf("want nothing once granted, got %+v", vs)
	}
}

// FactoryManagement (mxbuild: no CE2729): a page lists Teamcenter.BOM and shows
// Teamcenter.Node's attributes through TcBOMLine, a specialization of Node
// reached over an association. Node has no rules of its own; TcBOMLine's
// grant them.
func TestReachablePageRead_AttributeShownThroughASpecializationIsSilent(t *testing.T) {
	f := reachFixture(t, security.SecurityLevelProduction)
	f.entity("Probe.Node", "")
	f.entity("Probe.Line", "Probe.Node")
	f.entity("Probe.Folder", "Probe.Node") // a second specialization, which reads nothing
	f.entity("Probe.Bom", "")
	f.read("Probe.Bom", "Probe.User", "")
	f.read("Probe.Line", "Probe.User", "")
	f.memberRead("Probe.Line", "Probe.User", "", "Probe.Node.HasFocus")
	f.grid("Probe.P_Bom", "Probe.Bom", "Probe.Node.HasFocus", "Probe.User")
	f.menuItem("Probe.P_Bom")
	if vs := f.checkReach(); len(vs) != 0 {
		t.Errorf("want nothing, got %+v", vs)
	}
}

func TestReachablePageRead_OnlyAtProduction(t *testing.T) {
	f := reachFixture(t, security.SecurityLevelPrototype)
	f.grid("Probe.P_Menu", "Probe.Ticket", "Probe.Ticket.Title", "Probe.User")
	f.menuItem("Probe.P_Menu")
	if vs := f.checkReach(); len(vs) != 0 {
		t.Errorf("want nothing below Production, got %+v", vs)
	}
}
