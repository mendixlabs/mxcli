// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// A navigation statement names pages and microflows in six places: the home
// page (also per role), a home flow, the login page, the not-found page, and
// every menu item at any depth. `check -p` resolved none of them, and `exec`
// wrote them by name, so a typo reached the model and was first reported by
// `mx check` as CE1613 "The selected page 'X' no longer exists." — after exec
// had rewritten the whole menu (CREATE OR REPLACE NAVIGATION replaces it).
//
// The project here has Shop (a user module), Administration (stands for a
// Marketplace module) and System.

type navFixture struct {
	mb       *mock.MockBackend
	ctx      *ExecContext
	updates  int // UpdateNavigationProfile calls
	added    int // AddNavigationProfile calls
	menuSets int // CreateMenuDocument + UpdateMenuDocument calls
}

func newNavFixture(t *testing.T) *navFixture {
	t.Helper()
	shop, admin, system := mkModule("Shop"), mkModule("Administration"), mkModule("System")
	h := mkHierarchy(shop, admin, system)

	f := &navFixture{}
	f.mb = &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc: func() ([]*pages.Page, error) {
			return []*pages.Page{
				mkPage(shop.ID, "Home"),
				mkPage(shop.ID, "Orders"),
				mkPage(admin.ID, "Account_Overview"),
				mkPage(system.ID, "Login"),
			}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{
				mkMicroflow(shop.ID, "ACT_Open"),
				mkMicroflow(admin.ID, "Rebuild"),
				mkMicroflow(system.ID, "Logout"),
			}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) {
			return []*microflows.Nanoflow{mkNanoflow(shop.ID, "NF_Go")}, nil
		},
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{Profiles: []*types.NavigationProfile{{
				Name:      "Responsive",
				Kind:      "Responsive",
				HomePage:  &types.NavHomePage{Page: "Shop.Home"},
				MenuItems: []*types.NavMenuItem{{Caption: "Stored", Page: "Shop.Home"}},
			}}}, nil
		},
		UpdateNavigationProfileFunc: func(model.ID, string, types.NavigationProfileSpec) error {
			f.updates++
			return nil
		},
		AddNavigationProfileFunc: func(model.ID, string) error { f.added++; return nil },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{UserRoles: []*security.UserRole{{Name: "Administrator"}}}, nil
		},
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{shop, admin, system}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			for _, m := range []*model.Module{shop, admin, system} {
				if m.Name == name {
					return m, nil
				}
			}
			return nil, nil
		},
		CreateMenuDocumentFunc: func(*types.MenuDocument) error { f.menuSets++; return nil },
		UpdateMenuDocumentFunc: func(*types.MenuDocument) error { f.menuSets++; return nil },
	}
	f.ctx, _ = newMockCtx(t, withBackend(f.mb), withHierarchy(h))
	return f
}

func buildProg(t *testing.T, script string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("does not parse: %v\n%s", errs[0], script)
	}
	return prog
}

// check is `mxcli check -p`: the program-level reference validation.
func (f *navFixture) check(t *testing.T, script string) (string, []string) {
	t.Helper()
	errs, warnings := validateProgramWithWarnings(f.ctx, buildProg(t, script))
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "\n"), warnings
}

// exec runs every statement through its handler, as `exec` does after its
// pre-flight, and returns the first refusal.
func (f *navFixture) exec(t *testing.T, script string) error {
	t.Helper()
	for _, stmt := range buildProg(t, script).Statements {
		var err error
		switch s := stmt.(type) {
		case *ast.AlterNavigationStmt:
			err = execAlterNavigation(f.ctx, s)
		case *ast.CreateMenuStmt:
			err = execCreateMenu(f.ctx, s)
		default:
			t.Fatalf("unexpected statement %T", stmt)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// The two spellings of the reported navigation, with the same four bad targets:
// the home page, a page and a microflow inside a group, and a top-level page.
const bogusNavDeprecated = `
create or replace navigation Responsive
  home page Shop.NoSuchHome
  menu (
    menu 'G' (
      menu item 'Bogus' page Shop.NoSuchPage;
      menu item 'Bogus2' microflow Shop.ACT_NoSuchFlow;
    );
    menu item 'Bogus3' page Shop.NoSuchPage3;
  );
`

const bogusNavV1 = `mdl 1;
create or modify navigation Responsive
  home page Shop.NoSuchHome
{
  menu item 'Bogus3' ( OnClick: show page Shop.NoSuchPage3 )
  menu 'G' {
    menu item 'Bogus' ( OnClick: show page Shop.NoSuchPage )
    menu item 'Bogus2' ( OnClick: call microflow Shop.ACT_NoSuchFlow )
  }
};
`

func TestNavigationTargets_CheckRefusesUnresolvedTargets(t *testing.T) {
	for name, script := range map[string]string{"deprecated": bogusNavDeprecated, "mdl 1": bogusNavV1} {
		t.Run(name, func(t *testing.T) {
			f := newNavFixture(t)
			got, _ := f.check(t, script)
			for _, want := range []string{
				"navigation Responsive",
				"home page: page not found: Shop.NoSuchHome",
				"menu item 'G' > 'Bogus': page not found: Shop.NoSuchPage",
				"menu item 'G' > 'Bogus2': microflow not found: Shop.ACT_NoSuchFlow",
				"menu item 'Bogus3': page not found: Shop.NoSuchPage3",
			} {
				if !strings.Contains(got, want) {
					t.Errorf("check -p lacks %q in:\n%s", want, got)
				}
			}
			// The group a nested item sits in is part of where it is.
			if !strings.Contains(got, "'G' > 'Bogus'") {
				t.Errorf("a nested item does not name its group:\n%s", got)
			}
		})
	}
}

func TestNavigationTargets_CheckAcceptsTargetsThatExist(t *testing.T) {
	// Existing pages and flows, a Marketplace-module page and microflow, System
	// elements, a nanoflow, a role-based home, the login page, and a sign-out
	// item (which has no target).
	script := `mdl 1;
create or modify navigation Responsive
  home page Shop.Home
  home microflow Shop.ACT_Open for Administrator
  login page System.Login
  not found page Shop.Orders
{
  menu item 'Home' ( OnClick: show page Shop.Home )
  menu 'Admin' {
    menu item 'Accounts' ( OnClick: show page Administration.Account_Overview )
    menu item 'Rebuild' ( OnClick: call microflow Administration.Rebuild )
    menu 'Deeper' {
      menu item 'Go' ( OnClick: call nanoflow Shop.NF_Go )
      menu item 'Out' ( OnClick: call microflow System.Logout )
    }
  }
  menu item 'Sign out' ( OnClick: sign out )
};
`
	f := newNavFixture(t)
	if got, _ := f.check(t, script); got != "" {
		t.Errorf("check -p refused a navigation whose targets all exist:\n%s", got)
	}
}

// A page or microflow the SAME script creates earlier resolves. This is the
// control for the scriptContext exemption: it is what makes a project that
// does not have the page yet checkable.
func TestNavigationTargets_CheckResolvesTargetsCreatedEarlierInTheScript(t *testing.T) {
	script := `mdl 1;
create page Shop.NewPage ( Title: 'New', Layout: Atlas_Core.Atlas_Default ) { };
create microflow Shop.ACT_New () begin end;
create or modify navigation Responsive
  home page Shop.NewPage
{
  menu item 'Flow' ( OnClick: call microflow Shop.ACT_New )
};
`
	f := newNavFixture(t)
	if got, _ := f.check(t, script); got != "" {
		t.Errorf("a target created earlier in the script was refused:\n%s", got)
	}
}

func TestNavigationTargets_CheckCoversEveryTargetKind(t *testing.T) {
	cases := map[string]struct{ clause, want string }{
		"home page":              {"home page Shop.Gone", "home page: page not found: Shop.Gone"},
		"home microflow":         {"home microflow Shop.Gone", "home microflow: microflow not found: Shop.Gone"},
		"role-based home page":   {"home page Shop.Gone for Administrator", "home page for Administrator: page not found: Shop.Gone"},
		"role-based home flow":   {"home microflow Shop.Gone for Administrator", "home microflow for Administrator: microflow not found: Shop.Gone"},
		"login page":             {"login page Shop.Gone", "login page: page not found: Shop.Gone"},
		"not found page":         {"not found page Shop.Gone", "not found page: page not found: Shop.Gone"},
		"menu item nanoflow":     {"{ menu item 'X' ( OnClick: call nanoflow Shop.Gone ) }", "menu item 'X': nanoflow not found: Shop.Gone"},
		"menu item create+show":  {"{ menu item 'X' ( OnClick: create object Shop.Order then show page Shop.Gone ) }", "menu item 'X': page not found: Shop.Gone"},
		"three deep sub-menus":   {"{ menu 'A' { menu 'B' { menu 'C' { menu item 'X' ( OnClick: show page Shop.Gone ) } } } }", "'A' > 'B' > 'C' > 'X': page not found: Shop.Gone"},
		"old clause, top level":  {"menu ( menu item 'X' page Shop.Gone; )", "menu item 'X': page not found: Shop.Gone"},
		"old clause, in a group": {"menu ( menu 'G' ( menu item 'X' microflow Shop.Gone; ); )", "'G' > 'X': microflow not found: Shop.Gone"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newNavFixture(t)
			// "create object" needs the entity to exist for the builder; the
			// reference check reports the page, which is what the case is about.
			got, _ := f.check(t, "create or modify navigation Responsive "+c.clause+";")
			if !strings.Contains(got, c.want) {
				t.Errorf("check -p lacks %q in:\n%s", c.want, got)
			}
		})
	}
}

func TestNavigationTargets_CheckMenuDocuments(t *testing.T) {
	f := newNavFixture(t)
	got, _ := f.check(t, `create or modify menu Shop.Main_Menu {
  menu item 'Ok' ( OnClick: show page Shop.Home )
  menu 'G' { menu item 'Bad' ( OnClick: show page Shop.Gone ) }
};`)
	for _, want := range []string{"menu Shop.Main_Menu", "'G' > 'Bad': page not found: Shop.Gone"} {
		if !strings.Contains(got, want) {
			t.Errorf("check -p lacks %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "'Ok'") {
		t.Errorf("a resolving item was reported:\n%s", got)
	}
}

// Mendix does not validate an excluded document, so a menu document stored as
// excluded keeps its dangling targets and says so as a warning, as an excluded
// page does.
func TestNavigationTargets_ExcludedMenuDocumentWarns(t *testing.T) {
	f := newNavFixture(t)
	shop, _ := f.mb.GetModuleByName("Shop")
	f.mb.ListMenuDocumentsFunc = func() ([]*types.MenuDocument, error) {
		return []*types.MenuDocument{{ID: "md1", ContainerID: shop.ID, Name: "Old_Menu", Excluded: true}}, nil
	}
	f.mb.GetMenuDocumentByQualifiedNameFunc = func(_, name string) (*types.MenuDocument, error) {
		return &types.MenuDocument{ID: "md1", ContainerID: shop.ID, Name: name, Excluded: true}, nil
	}
	got, warnings := f.check(t, `create or modify menu Shop.Old_Menu { menu item 'Bad' ( OnClick: show page Shop.Gone ) };`)
	if got != "" {
		t.Errorf("an excluded menu document blocked check:\n%s", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "Shop.Gone") {
		t.Errorf("warnings = %v, want one naming Shop.Gone", warnings)
	}
}

// exec refuses BEFORE it writes: CREATE OR REPLACE NAVIGATION rewrites the
// whole menu, so a partial write is the stored menu lost for a typo.
func TestNavigationTargets_ExecRefusesBeforeWriting(t *testing.T) {
	for name, script := range map[string]string{"deprecated": bogusNavDeprecated, "mdl 1": bogusNavV1} {
		t.Run(name, func(t *testing.T) {
			f := newNavFixture(t)
			before, _ := f.mb.GetNavigation()
			err := f.exec(t, script)
			if err == nil {
				t.Fatal("exec accepted a navigation whose targets do not exist")
			}
			for _, want := range []string{"Shop.NoSuchHome", "Shop.NoSuchPage", "Shop.ACT_NoSuchFlow", "'Bogus3'"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal lacks %q: %v", want, err)
				}
			}
			if f.updates != 0 || f.added != 0 {
				t.Errorf("the profile was written: %d updates, %d added", f.updates, f.added)
			}
			after, _ := f.mb.GetNavigation()
			if len(before.Profiles[0].MenuItems) != len(after.Profiles[0].MenuItems) ||
				before.Profiles[0].HomePage.Page != after.Profiles[0].HomePage.Page {
				t.Errorf("the stored profile changed: %+v -> %+v", before.Profiles[0], after.Profiles[0])
			}
		})
	}
}

// check and exec decide alike: what check -p passes, exec writes.
func TestNavigationTargets_ExecWritesWhatCheckPasses(t *testing.T) {
	script := `mdl 1;
create or modify navigation Responsive
  home page Shop.Home
{
  menu item 'Orders' ( OnClick: show page Shop.Orders )
  menu 'Admin' { menu item 'Accounts' ( OnClick: show page Administration.Account_Overview ) }
  menu item 'Flow' ( OnClick: call microflow Shop.ACT_Open )
};
`
	f := newNavFixture(t)
	if got, _ := f.check(t, script); got != "" {
		t.Fatalf("check refused a valid navigation:\n%s", got)
	}
	if err := f.exec(t, script); err != nil {
		t.Fatalf("exec refused what check passed: %v", err)
	}
	if f.updates != 1 {
		t.Errorf("updates = %d, want 1", f.updates)
	}
}

func TestNavigationTargets_ExecRefusesMenuDocumentBeforeWriting(t *testing.T) {
	f := newNavFixture(t)
	f.mb.GetMenuDocumentByQualifiedNameFunc = func(_, _ string) (*types.MenuDocument, error) { return nil, nil }
	err := f.exec(t, `create or modify menu Shop.Main_Menu { menu item 'Bad' ( OnClick: show page Shop.Gone ) };`)
	if err == nil || !strings.Contains(err.Error(), "Shop.Gone") {
		t.Fatalf("err = %v, want a refusal naming Shop.Gone", err)
	}
	if f.menuSets != 0 {
		t.Errorf("the menu document was written (%d writes)", f.menuSets)
	}
	// CONTROL: a resolving target is written.
	if err := f.exec(t, `create or modify menu Shop.Main_Menu { menu item 'Ok' ( OnClick: show page Shop.Home ) };`); err != nil {
		t.Fatalf("a resolving menu was refused: %v", err)
	}
	if f.menuSets != 1 {
		t.Errorf("menu writes = %d, want 1", f.menuSets)
	}
}

// A navigation first and its page or flow created further down: the target is
// stored by name, so a run that rolls back (plain `exec`, and `check`) accepts
// it, as it accepts a call to a flow created further down (mendixlabs/mxcli#1405).
// Measured on upstream/main, Mendix 11.12.5: such a script execs and `mx check`
// reports 0 errors, so refusing it would break a working script.
const forwardNavScript = `mdl 1;
create or modify navigation Responsive
  home page Shop.Later
{
  menu item 'Flow' ( OnClick: call microflow Shop.ACT_Later )
};
create page Shop.Later ( Title: 'Later', Layout: Atlas_Core.Atlas_Default ) { };
create microflow Shop.ACT_Later () begin end;
`

func TestNavigationTargets_ForwardReferenceIsAcceptedByARunThatRollsBack(t *testing.T) {
	// No project: the order rule has nothing to say about a stored-by-name target.
	for _, v := range ValidateScriptDefinitionOrder(buildProg(t, forwardNavScript)) {
		t.Errorf("project-free order check reported %s: %s", v.RuleID, v.Message)
	}
	f := newNavFixture(t)
	f.ctx.forwardFlowCalls = true
	if got, _ := f.check(t, forwardNavScript); got != "" {
		t.Errorf("check -p refused a navigation written before its targets:\n%s", got)
	}
}

// A run that cannot roll back would leave the target dangling if a later
// statement failed, so it refuses, with the reason.
func TestNavigationTargets_ForwardReferenceIsRefusedByARunThatCannotRollBack(t *testing.T) {
	f := newNavFixture(t)
	f.ctx.forwardFlowCalls = false
	got, _ := f.check(t, forwardNavScript)
	for _, want := range []string{
		"navigation Responsive: home page references page Shop.Later before it is created",
		"navigation Responsive: menu item 'Flow' references microflow Shop.ACT_Later before it is created",
		"cannot be rolled back",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("check -p lacks %q in:\n%s", want, got)
		}
	}
}

// The handler resolves a target the program declares further down only in a
// run that rolls back, and only the kinds that are stored by name.
func TestNavigationTargets_ExecResolvesForwardTargetsOnlyInARunThatRollsBack(t *testing.T) {
	script := "create or modify navigation Responsive home page Shop.Later;"
	declared := map[defKey]bool{{kind: defPage, name: "Shop.Later"}: true}

	f := newNavFixture(t)
	f.ctx.forwardFlowCalls, f.ctx.scriptTargets = true, declared
	if err := f.exec(t, script); err != nil {
		t.Fatalf("a target declared further down was refused in a run that rolls back: %v", err)
	}
	if f.updates != 1 {
		t.Errorf("updates = %d, want 1", f.updates)
	}

	for name, setup := range map[string]func(*ExecContext){
		"run that cannot roll back": func(c *ExecContext) { c.forwardFlowCalls, c.scriptTargets = false, declared },
		"target the script lacks":   func(c *ExecContext) { c.forwardFlowCalls, c.scriptTargets = true, map[defKey]bool{} },
		"declared as another kind": func(c *ExecContext) {
			c.forwardFlowCalls, c.scriptTargets = true, map[defKey]bool{{kind: defMicroflow, name: "Shop.Later"}: true}
		},
	} {
		f := newNavFixture(t)
		setup(f.ctx)
		if err := f.exec(t, script); err == nil || !strings.Contains(err.Error(), "page not found: Shop.Later") {
			t.Errorf("%s: err = %v, want a refusal naming Shop.Later", name, err)
		}
		if f.updates != 0 {
			t.Errorf("%s: the profile was written", name)
		}
	}
}

// The executor declares what the program creates before it runs the first
// statement, for the pages, microflows and nanoflows a navigation may name.
func TestNavigationTargets_EnterScriptFlowsDeclaresPagesAndFlows(t *testing.T) {
	e := &Executor{forwardFlowCalls: true}
	restore := e.enterScriptFlows(buildProg(t, forwardNavScript+"create nanoflow Shop.NF_Later () begin end;\n"))
	for _, k := range []defKey{{defPage, "Shop.Later"}, {defMicroflow, "Shop.ACT_Later"}, {defNanoflow, "Shop.NF_Later"}} {
		if !e.scriptTargets[k] {
			t.Errorf("scriptTargets lacks %v: %v", k, e.scriptTargets)
		}
	}
	restore()
	if len(e.scriptTargets) != 0 {
		t.Errorf("scriptTargets not restored: %v", e.scriptTargets)
	}
	// A run that cannot roll back declares nothing.
	e = &Executor{}
	e.enterScriptFlows(buildProg(t, forwardNavScript))
	if len(e.scriptTargets) != 0 {
		t.Errorf("a run that cannot roll back declared %v", e.scriptTargets)
	}
}

// Without a rollback, a target created further down by `create or modify` is
// judged against what the project holds: it does not, so exec would stop at the
// navigation.
func TestNavigationTargets_ForwardOrModifyIsRefusedWithoutRollback(t *testing.T) {
	f := newNavFixture(t)
	f.ctx.forwardFlowCalls = false
	got, _ := f.check(t, `mdl 1;
create or modify navigation Responsive home page Shop.Later;
create or modify page Shop.Later ( Title: 'Later', Layout: Atlas_Core.Atlas_Default ) { };
`)
	for _, want := range []string{"navigation Responsive: home page", "Shop.Later", "before it is created"} {
		if !strings.Contains(got, want) {
			t.Errorf("check -p lacks %q in:\n%s", want, got)
		}
	}
}

// A native profile's `home microflow` is stored as the nanoflow it is, so the
// name is looked up among the nanoflows.
func TestNavigationTargets_NativeHomeMicroflowIsLookedUpAsNanoflow(t *testing.T) {
	f := newNavFixture(t)
	f.mb.GetNavigationFunc = func() (*types.NavigationDocument, error) {
		return &types.NavigationDocument{Profiles: []*types.NavigationProfile{
			{Name: "NativePhone", IsNative: true},
		}}, nil
	}
	if got, _ := f.check(t, "create or modify navigation NativePhone home microflow Shop.NF_Go;"); got != "" {
		t.Errorf("a native home naming an existing nanoflow was refused:\n%s", got)
	}
	got, _ := f.check(t, "create or modify navigation NativePhone home nanoflow Shop.Gone;")
	if !strings.Contains(got, "home nanoflow: nanoflow not found: Shop.Gone") {
		t.Errorf("check -p lacks the native home nanoflow in:\n%s", got)
	}
}
