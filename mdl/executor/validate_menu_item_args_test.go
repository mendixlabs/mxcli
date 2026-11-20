// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2098 (mendixlabs/mxcli#1799): a menu item that calls a flow with a
// parameter, or shows a page with a required one, passed `check -p` and exec,
// and mx check 11.14.0 then reported
//
//	[error] [CE1571] "No argument has been selected for parameter 'Thing' and
//	no default is available. Please select an argument manually." at Menu item
//
// A menu has no context object, so nothing fills the parameter in.

// newMenuArgFixture is the navigation fixture with parameterised documents:
// Shop.ACT_Thing($Thing: Shop.Thing), Shop.NF_Text($Text: String),
// Shop.Thing_View($Thing required), Shop.Maybe_View($Thing optional), and the
// parameterless Shop.ACT_Open / Shop.Home.
func newMenuArgFixture(t *testing.T) *navFixture {
	t.Helper()
	f := newNavFixture(t)
	shopID := model.ID("")
	if mods, _ := f.mb.ListModulesFunc(); len(mods) > 0 {
		shopID = mods[0].ID
	}
	thing := &microflows.MicroflowParameter{Name: "Thing", Type: &microflows.ObjectType{EntityQualifiedName: "Shop.Thing"}}
	text := &microflows.MicroflowParameter{Name: "Text", Type: &microflows.StringType{}}
	f.mb.ListMicroflowsFunc = func() ([]*microflows.Microflow, error) {
		withThing := mkMicroflow(shopID, "ACT_Thing")
		withThing.Parameters = []*microflows.MicroflowParameter{thing}
		withText := mkMicroflow(shopID, "ACT_Text")
		withText.Parameters = []*microflows.MicroflowParameter{text}
		return []*microflows.Microflow{mkMicroflow(shopID, "ACT_Open"), withThing, withText}, nil
	}
	f.mb.ListNanoflowsFunc = func() ([]*microflows.Nanoflow, error) {
		nf := mkNanoflow(shopID, "NF_Text")
		nf.Parameters = []*microflows.MicroflowParameter{text}
		return []*microflows.Nanoflow{mkNanoflow(shopID, "NF_Go"), nf}, nil
	}
	f.mb.ListPagesFunc = func() ([]*pages.Page, error) {
		required := mkPage(shopID, "Thing_View")
		required.Parameters = []*pages.PageParameter{{Name: "Thing", EntityName: "Shop.Thing", IsRequired: true}}
		optional := mkPage(shopID, "Maybe_View")
		optional.Parameters = []*pages.PageParameter{{Name: "Thing", EntityName: "Shop.Thing", IsRequired: false}}
		return []*pages.Page{mkPage(shopID, "Home"), required, optional}, nil
	}
	return f
}

func TestMenuItemArgs_ReportsParameterNothingFills(t *testing.T) {
	f := newMenuArgFixture(t)
	got, _ := f.check(t, `
create or modify navigation Responsive
  home page Shop.Home
{
  menu item 'MfObject' ( OnClick: call microflow Shop.ACT_Thing )
  menu 'Group' {
    menu item 'NfText' ( OnClick: call nanoflow Shop.NF_Text )
  }
  menu item 'PageObject' ( OnClick: show page Shop.Thing_View )
};
create menu Shop.Side_Menu {
  menu item 'DocMf' ( OnClick: call microflow Shop.ACT_Thing )
};
alter navigation Responsive {
  insert { menu item 'AlterMf' ( OnClick: call microflow Shop.ACT_Text ) }
};
`)
	for _, want := range []string{
		"menu item 'MfObject': microflow Shop.ACT_Thing has no argument for parameter 'Thing'",
		"menu item 'Group' > 'NfText': nanoflow Shop.NF_Text has no argument for parameter 'Text'",
		"menu item 'PageObject': page Shop.Thing_View has no argument for parameter 'Thing'",
		"menu item 'DocMf': microflow Shop.ACT_Thing has no argument for parameter 'Thing'",
		"menu item 'AlterMf': microflow Shop.ACT_Text has no argument for parameter 'Text'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("check -p does not report %q\ngot:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "CE1571"); n != 5 {
		t.Errorf("want 5 CE1571 reports, one per measured mx check error, got %d:\n%s", n, got)
	}
}

// The controls mx check 11.14.0 builds at 0 errors: a parameterless flow and
// page, an optional page parameter, and a flow given a literal argument.
func TestMenuItemArgs_LegalItemsStaySilent(t *testing.T) {
	f := newMenuArgFixture(t)
	got, _ := f.check(t, `
create or modify navigation Responsive
  home page Shop.Home
{
  menu item 'Plain' ( OnClick: call microflow Shop.ACT_Open )
  menu item 'PlainNf' ( OnClick: call nanoflow Shop.NF_Go )
  menu item 'PlainPage' ( OnClick: show page Shop.Home )
  menu item 'Maybe' ( OnClick: show page Shop.Maybe_View )
  menu item 'Literal' ( OnClick: call microflow Shop.ACT_Text(Text = 'hello') )
};
create menu Shop.Side_Menu {
  menu item 'DocPlain' ( OnClick: call microflow Shop.ACT_Open )
};
`)
	if got != "" {
		t.Errorf("legal menu items reported:\n%s", got)
	}
}

// A target that does not resolve is the not-found check's to report; this rule
// stays silent rather than guess at a signature.
func TestMenuItemArgs_UnresolvedTargetSilent(t *testing.T) {
	f := newMenuArgFixture(t)
	got, _ := f.check(t, `
create or modify navigation Responsive
  home page Shop.Home
{
  menu item 'Ghost' ( OnClick: call microflow Shop.NoSuchFlow )
  menu item 'GhostPage' ( OnClick: show page Shop.NoSuchPage )
};
`)
	if strings.Contains(got, "CE1571") {
		t.Errorf("an unresolved target was judged for arguments:\n%s", got)
	}
}

// Flows and pages the script itself creates are judged by their declaration.
func TestMenuItemArgs_ScriptCreatedTargets(t *testing.T) {
	f := newMenuArgFixture(t)
	got, _ := f.check(t, `
create microflow Shop.ACT_New ($Count: Integer)
begin
end;
/
create page Shop.New_View (Params: ( $Count: Integer ), Title: 'New', Layout: Atlas_Core.Atlas_Default) {
  dynamictext txt (Content: 'New')
};
create page Shop.Opt_View (Params: ( $Count: Integer optional ), Title: 'Opt', Layout: Atlas_Core.Atlas_Default) {
  dynamictext txt (Content: 'Opt')
};
create or modify navigation Responsive
  home page Shop.Home
{
  menu item 'NewMf' ( OnClick: call microflow Shop.ACT_New )
  menu item 'NewPage' ( OnClick: show page Shop.New_View )
  menu item 'NewOptional' ( OnClick: show page Shop.Opt_View )
};
`)
	for _, want := range []string{
		"menu item 'NewMf': microflow Shop.ACT_New has no argument for parameter 'Count'",
		"menu item 'NewPage': page Shop.New_View has no argument for parameter 'Count'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("check -p does not report %q\ngot:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Opt_View") {
		t.Errorf("an optional page parameter was reported:\n%s", got)
	}
}

// `create object E then show page P` hands the new object to the page: the
// page's parameter is filled, so the item is not judged.
func TestMenuItemArgs_CreateObjectThenShowPageSilent(t *testing.T) {
	f := newMenuArgFixture(t)
	got, _ := f.check(t, `
create or modify navigation Responsive
  home page Shop.Home
{
  menu item 'New' ( OnClick: create object Shop.Thing then show page Shop.Thing_View )
};
`)
	if strings.Contains(got, "CE1571") {
		t.Errorf("create object then show page was judged:\n%s", got)
	}
}
