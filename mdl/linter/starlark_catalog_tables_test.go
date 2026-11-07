// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// catalogTablesFixture is a catalog holding one row per builtin added for
// mendixlabs/mxcli#1265 in a user module (Sales), plus the same kind of row in
// a Marketplace module (Mkt) and in System -- the controls for the module
// filter every typed builtin applies. A second user module (Hr) is the control
// for --exclude-modules.
func catalogTablesFixture(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec %s: %v", q, err)
		}
	}

	exec(`INSERT INTO modules_data (Id, Name, Source, DomainModelDocumentation, ProjectId, SnapshotId) VALUES (?,?,?,?,?,?)`,
		"mod-sales", "Sales", "", "Orders and customers.", "p", "s")
	exec(`INSERT INTO modules_data (Id, Name, Source, DomainModelDocumentation, ProjectId, SnapshotId) VALUES (?,?,?,?,?,?)`,
		"mod-hr", "Hr", "", "", "p", "s")
	exec(`INSERT INTO modules_data (Id, Name, Source, ProjectId, SnapshotId) VALUES (?,?,?,?,?)`,
		"mod-mkt", "Mkt", "Marketplace v1.0.0", "p", "s")
	exec(`INSERT INTO modules_data (Id, Name, Source, ProjectId, SnapshotId) VALUES (?,?,?,?,?)`,
		"00000000-0000-0000-0000-000000000001", "System", "", "p", "s")

	for _, mod := range []string{"Sales", "Hr", "Mkt", "System"} {
		exec(`INSERT INTO associations_data (Id, Name, QualifiedName, ModuleName, FromEntity, ToEntity,
				AssociationType, Owner, StorageFormat, Description,
				ToDeleteBehavior, FromDeleteBehavior, ToDeleteErrorMessage, FromDeleteErrorMessage,
				ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			"a-"+mod, "Line_Order", mod+".Line_Order", mod, mod+".Line", mod+".Order",
			"Reference", "Default", "Column", "Lines of an order",
			"DeleteMeIfNoReferences", "DeleteMeAndReferences", "Order has lines", "",
			"p", "s")
		exec(`INSERT INTO entity_event_handlers_data (Id, EntityId, EntityQualifiedName, ModuleName,
				Moment, Event, Microflow, RaiseErrorOnFalse, PassEventObject, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			"h-"+mod, "e-"+mod, mod+".Order", mod, "Before", "Commit", mod+".BCo_Order", 1, 0, "p", "s")
		exec(`INSERT INTO jar_dependencies_data (Id, ModuleName, GroupId, ArtifactId, Coordinate, Version,
				IsIncluded, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			"j-"+mod, mod, "org.example", "lib", "org.example:lib", "1.2.3", 0, "p", "s")
		exec(`INSERT INTO layouts_data (Id, Name, QualifiedName, ModuleName, Folder, LayoutType, Platform,
				Description, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			"l-"+mod, "Atlas_Popup", mod+".Atlas_Popup", mod, "Layouts", "ModalPopup", "Web", "A popup", "p", "s")
		exec(`INSERT INTO published_rest_operations_data (Id, ServiceId, ServiceQualifiedName, ResourceName,
				HttpMethod, Path, Summary, Microflow, Deprecated, ModuleName, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			"o-"+mod, "svc-"+mod, mod+".OrderApi", "orders", "Get", "/{id}", "Get one order",
			mod+".PRS_GetOrder", 1, mod, "p", "s")
		for _, lang := range []string{"en_US", "nl_NL"} {
			exec(`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName)
				VALUES (?,?,?,?,?,?,?)`,
				mod+".Order_Overview", "PAGE", "Orders "+lang, "Forms$Page.Title", lang, "w-"+mod, mod)
		}
	}
	// Navigation belongs to the project: no module, no module filter.
	exec(`INSERT INTO navigation_menu_items (ProfileName, ItemPath, Depth, Caption, ActionType,
			TargetPage, TargetMicroflow, SubItemCount, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"Responsive", "0", 0, "Admin", "NoAction", "", "", 1, "p", "s")
	exec(`INSERT INTO navigation_menu_items (ProfileName, ItemPath, Depth, Caption, ActionType,
			TargetPage, TargetMicroflow, SubItemCount, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"Responsive", "0.0", 1, "Recalculate", "MicroflowAction", "", "Sales.ACT_Recalculate", 0, "p", "s")
	return cat
}

// runSrc loads src as a Starlark rule and runs it.
func runSrc(t *testing.T, ctx *linter.LintContext, src string) ([]linter.Violation, *linter.StarlarkRule) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rule.star")
	if err := os.WriteFile(path, []byte(`RULE_ID = "T1265"
RULE_NAME = "T1265"
DESCRIPTION = "test"
CATEGORY = "quality"
SEVERITY = "info"
`+src), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := linter.LoadStarlarkRule(path)
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v", err)
	}
	vs := r.Check(ctx)
	for _, v := range vs {
		if strings.HasPrefix(v.Message, "Starlark rule error") {
			t.Fatalf("%s", v.Message)
		}
	}
	if errs := ctx.QueryErrors(); len(errs) > 0 {
		t.Fatalf("query errors: %v", errs)
	}
	return vs, r
}

// dumpRule renders every field of every struct a builtin returns, sorted by
// field name, one violation per struct.
func dumpRule(call string) string {
	return `
def check():
    out = []
    for x in ` + call + `:
        out.append(violation(message="|".join([k + "=" + str(getattr(x, k)) for k in sorted(dir(x))])))
    return out
`
}

func TestCatalogTableBuiltins(t *testing.T) {
	for _, tc := range []struct {
		call string
		want []string
	}{
		{"associations()", []string{
			"description=Lines of an order|from_delete_behavior=DeleteMeAndReferences|from_delete_error_message=|" +
				"from_entity=Sales.Line|module_name=Sales|name=Line_Order|owner=Default|qualified_name=Sales.Line_Order|" +
				"storage_format=Column|to_delete_behavior=DeleteMeIfNoReferences|to_delete_error_message=Order has lines|" +
				"to_entity=Sales.Order|type=Reference",
		}},
		{"entity_event_handlers()", []string{
			"entity=Sales.Order|event=Commit|microflow=Sales.BCo_Order|module_name=Sales|moment=Before|" +
				"pass_event_object=False|raise_error_on_false=True",
		}},
		{"navigation_menu_items()", []string{
			"action_type=NoAction|caption=Admin|depth=0|item_path=0|profile=Responsive|target_microflow=|target_page=",
			"action_type=MicroflowAction|caption=Recalculate|depth=1|item_path=0.0|profile=Responsive|" +
				"target_microflow=Sales.ACT_Recalculate|target_page=",
		}},
		{"jar_dependencies()", []string{
			"artifact_id=lib|coordinate=org.example:lib|group_id=org.example|is_included=False|" +
				"module_name=Sales|version=1.2.3",
		}},
		{"strings()", []string{
			"context=Forms$Page.Title|element_id=w-Sales|language=en_US|module_name=Sales|object_type=PAGE|" +
				"qualified_name=Sales.Order_Overview|value=Orders en_US",
			"context=Forms$Page.Title|element_id=w-Sales|language=nl_NL|module_name=Sales|object_type=PAGE|" +
				"qualified_name=Sales.Order_Overview|value=Orders nl_NL",
		}},
		{`strings(language = "nl_NL")`, []string{
			"context=Forms$Page.Title|element_id=w-Sales|language=nl_NL|module_name=Sales|object_type=PAGE|" +
				"qualified_name=Sales.Order_Overview|value=Orders nl_NL",
		}},
		// An untranslated language has no row, rather than an empty one.
		{`strings("de_DE")`, nil},
		{"layouts()", []string{
			"class_name=|description=A popup|dynamic_classes=|folder=Layouts|layout_type=ModalPopup|module_name=Sales|" +
				"name=Atlas_Popup|platform=Web|qualified_name=Sales.Atlas_Popup|style=",
		}},
		{"published_rest_operations()", []string{
			"deprecated=True|http_method=Get|microflow=Sales.PRS_GetOrder|module_name=Sales|path=/{id}|" +
				"resource=orders|service=Sales.OrderApi|summary=Get one order",
		}},
		{"modules()", []string{
			"domain_model_documentation=Orders and customers.|id=mod-sales|name=Sales",
		}},
	} {
		t.Run(tc.call, func(t *testing.T) {
			ctx := linter.NewLintContext(catalogTablesFixture(t), &minimalReader{})
			// Hr is a user module like Sales; excluding it is the IsExcluded
			// half of the filter. Mkt and System go by notPlatformModule.
			ctx.SetExcludedModules([]string{"Hr"})
			vs, _ := runSrc(t, ctx, dumpRule(tc.call))
			var got []string
			for _, v := range vs {
				got = append(got, v.Message)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("%s:\n got  %q\n want %q", tc.call, got, tc.want)
			}
		})
	}
}

// Control for the module filter: without the exclusion, Hr's rows appear, so
// the single-row results above are the filter working, not a fixture with one row.
func TestCatalogTableBuiltinsIncludeEveryUserModule(t *testing.T) {
	ctx := linter.NewLintContext(catalogTablesFixture(t), &minimalReader{})
	vs, _ := runSrc(t, ctx, `
def check():
    return [violation(message=a.module_name) for a in associations()]
`)
	var got []string
	for _, v := range vs {
		got = append(got, v.Message)
	}
	if want := []string{"Hr", "Sales"}; !slices.Equal(got, want) {
		t.Errorf("associations() modules = %v, want %v (Mkt and System are platform modules)", got, want)
	}
}

// --document narrows the document-scoped builtins to the named document.
func TestCatalogTableBuiltinsHonourDocumentFilter(t *testing.T) {
	ctx := linter.NewLintContext(catalogTablesFixture(t), &minimalReader{})
	ctx.SetIncludedDocuments([]string{"Sales.Atlas_Popup"})
	vs, _ := runSrc(t, ctx, `
def check():
    out = [violation(message="layout " + l.qualified_name) for l in layouts()]
    out += [violation(message="op " + o.service) for o in published_rest_operations()]
    out += [violation(message="string " + s.qualified_name) for s in strings()]
    return out
`)
	if got := messages(vs); got != "layout Sales.Atlas_Popup" {
		t.Errorf("with --document Sales.Atlas_Popup got:\n%s", got)
	}
}

// strings is filled only by a FULL catalog, so using it must raise the rule's
// catalog mode -- otherwise it reads an empty table and reports a clean pass.
func TestStringsRequiresFullCatalog(t *testing.T) {
	ctx := linter.NewLintContext(catalogTablesFixture(t), &minimalReader{})
	_, r := runSrc(t, ctx, "def check():\n    return [violation(message=s.value) for s in strings()]\n")
	if got := r.RequiredCatalogMode(); got != linter.CatalogFull {
		t.Errorf("a rule calling strings() requires %v, want CatalogFull", got)
	}
	// Control: a fast-catalog builtin does not.
	_, r = runSrc(t, ctx, "def check():\n    return [violation(message=a.name) for a in associations()]\n")
	if got := r.RequiredCatalogMode(); got != linter.CatalogFast {
		t.Errorf("a rule calling associations() requires %v, want CatalogFast", got)
	}
}

type securityReader struct {
	minimalReader
	ps *security.ProjectSecurity
}

func (r *securityReader) GetProjectSecurity() (*security.ProjectSecurity, error) { return r.ps, nil }

// mendixlabs/mxcli#1269: the admin account's name and role were read and not
// exposed. The password must stay unexposed.
func TestProjectSecurityExposesAdminUserButNotPassword(t *testing.T) {
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	reader := &securityReader{ps: &security.ProjectSecurity{
		SecurityLevel: "CheckEverything",
		AdminUserName: "MxAdmin",
		AdminPassword: "s3cret-do-not-leak",
		AdminUserRole: "Administrator",
	}}
	vs, _ := runSrc(t, linter.NewLintContext(cat, reader), `
def check():
    ps = project_security()
    return [violation(message=ps.admin_user_name + "|" + ps.admin_user_role + "|" + ",".join(dir(ps)) + "|" + str(ps))]
`)
	if len(vs) != 1 {
		t.Fatalf("got %d violations", len(vs))
	}
	msg := vs[0].Message
	if !strings.HasPrefix(msg, "MxAdmin|Administrator|") {
		t.Errorf("admin fields = %q, want MxAdmin|Administrator", msg)
	}
	if strings.Contains(msg, "s3cret") || strings.Contains(strings.ToLower(msg), "password=") ||
		strings.Contains(msg, "admin_password") {
		t.Errorf("project_security() exposes the admin password:\n%s", msg)
	}
}

// ako/mxcli#1442: "A user role has a Documentation/Description in Project
// Security, but ... user_roles() in Starlark has no description (module roles
// do)." The undocumented role is the control: it reads back as "".
func TestUserRolesExposeDescription(t *testing.T) {
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	reader := &securityReader{ps: &security.ProjectSecurity{UserRoles: []*security.UserRole{
		{Name: "Planner", Description: "Plans the routes; maps to the dispatcher desk"},
		{Name: "User"},
	}}}
	vs, _ := runSrc(t, linter.NewLintContext(cat, reader), `
def check():
    return [violation(message=r.name + "=" + r.description) for r in user_roles()]
`)
	var got []string
	for _, v := range vs {
		got = append(got, v.Message)
	}
	want := []string{"Planner=Plans the routes; maps to the dispatcher desk", "User="}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("user_roles() descriptions:\n%v\nwant:\n%v", got, want)
	}
}

// mendixlabs/mxcli#1238: user_roles() exposes the role's "Check security" flag
// and its user-management grants, so a rule such as "security is checked for
// every role" can be written. The second role sets none of them and is the
// control: every flag reads back false and its list empty.
func TestUserRolesExposeSecurityFlags(t *testing.T) {
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	reader := &securityReader{ps: &security.ProjectSecurity{UserRoles: []*security.UserRole{
		{Name: "Administrator", CheckSecurity: true, ManageAllRoles: true, ManageUsersWithoutRoles: true,
			ManageableRoles: []string{"User", "Planner"}},
		{Name: "User"},
	}}}
	vs, _ := runSrc(t, linter.NewLintContext(cat, reader), `
def check():
    return [violation(message="%s check=%s all=%s noroles=%s manage=%s" % (
        r.name, r.check_security, r.manage_all_roles, r.manage_users_without_roles,
        ",".join(r.manageable_roles))) for r in user_roles()]
`)
	var got []string
	for _, v := range vs {
		got = append(got, v.Message)
	}
	want := []string{
		"Administrator check=True all=True noroles=True manage=User,Planner",
		"User check=False all=False noroles=False manage=",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("user_roles() security flags:\n%v\nwant:\n%v", got, want)
	}
}
