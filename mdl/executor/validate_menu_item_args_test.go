// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A menu item has no context object, so a flow or page it calls with a
// parameter no argument fills is CE1571 "No argument has been selected for
// parameter 'X' and no default is available" at Menu item. Measured on Mendix
// 11.14.0 (see validate_menu_item_args.go); `check -p` passed every one of them.
//
// The fixture is newNavFixture's project plus the parameterised targets of that
// measurement: Shop.ACT_WithObject($Thing: Shop.Thing),
// Shop.ACT_WithString($Code: String), Shop.NF_WithObject($Thing: Shop.Thing),
// and the pages Shop.Thing_Edit (required $Thing), Shop.Thing_Optional
// (optional $Thing) and Shop.Thing_Default ($Thing with a default).
func newMenuArgFixture(t *testing.T) *navFixture {
	t.Helper()
	f := newNavFixture(t)
	shop, _ := f.mb.GetModuleByName("Shop")
	thing := &microflows.ObjectType{EntityQualifiedName: "Shop.Thing"}
	withParams := func(mf *microflows.Microflow, params ...*microflows.MicroflowParameter) *microflows.Microflow {
		mf.Parameters = params
		return mf
	}
	param := func(name string, dt microflows.DataType) *microflows.MicroflowParameter {
		return &microflows.MicroflowParameter{Name: name, Type: dt}
	}
	mfs, _ := f.mb.ListMicroflows()
	mfs = append(mfs,
		withParams(mkMicroflow(shop.ID, "ACT_WithObject"), param("Thing", thing)),
		withParams(mkMicroflow(shop.ID, "ACT_WithString"), param("Code", &microflows.StringType{})),
	)
	f.mb.ListMicroflowsFunc = func() ([]*microflows.Microflow, error) { return mfs, nil }
	nfs, _ := f.mb.ListNanoflows()
	nf := mkNanoflow(shop.ID, "NF_WithObject")
	nf.Parameters = []*microflows.MicroflowParameter{param("Thing", thing)}
	nfs = append(nfs, nf)
	f.mb.ListNanoflowsFunc = func() ([]*microflows.Nanoflow, error) { return nfs, nil }

	pagesWithParam := func(name string, required bool, def string) *pages.Page {
		p := mkPage(shop.ID, name)
		p.Parameters = []*pages.PageParameter{{Name: "Thing", EntityName: "Shop.Thing", IsRequired: required, DefaultValue: def}}
		return p
	}
	pgs, _ := f.mb.ListPages()
	pgs = append(pgs,
		pagesWithParam("Thing_Edit", true, ""),
		pagesWithParam("Thing_Optional", false, ""),
		pagesWithParam("Thing_Default", true, "empty"),
	)
	f.mb.ListPagesFunc = func() ([]*pages.Page, error) { return pgs, nil }
	return f
}

func TestMenuItemArgs_CheckRefusesAParameterNoArgumentFills(t *testing.T) {
	cases := map[string]struct{ item, want string }{
		"microflow, object parameter": {
			"menu item 'A' ( OnClick: call microflow Shop.ACT_WithObject )",
			"menu item 'A': call microflow Shop.ACT_WithObject has no argument for parameter 'Thing' — Mendix rejects this with CE1571",
		},
		"microflow, String parameter": {
			"menu item 'A' ( OnClick: call microflow Shop.ACT_WithString )",
			"or pass a value: `call microflow Shop.ACT_WithString(Code = '…')`",
		},
		"nanoflow, object parameter": {
			"menu item 'A' ( OnClick: call nanoflow Shop.NF_WithObject )",
			"call nanoflow Shop.NF_WithObject has no argument for parameter 'Thing'",
		},
		"page, required parameter": {
			"menu item 'A' ( OnClick: show page Shop.Thing_Edit )",
			"show page Shop.Thing_Edit has no argument for parameter 'Thing'",
		},
		"nested in a group": {
			"menu 'G' { menu item 'A' ( OnClick: call microflow Shop.ACT_WithObject ) }",
			"menu item 'G' > 'A': call microflow",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newMenuArgFixture(t)
			got, _ := f.check(t, "mdl 1;\ncreate or modify navigation Responsive {\n  "+c.item+"\n};")
			if !strings.Contains(got, "navigation Responsive has argument errors") || !strings.Contains(got, c.want) {
				t.Errorf("check -p lacks %q in:\n%s", c.want, got)
			}
		})
	}
}

// The old clause spelling carries the same call, and the menu builder stores it
// the same way.
func TestMenuItemArgs_CheckRefusesTheDeprecatedSpelling(t *testing.T) {
	f := newMenuArgFixture(t)
	got, _ := f.check(t, `create or replace navigation Responsive
  menu (
    menu item 'A' microflow Shop.ACT_WithObject;
  );`)
	if !strings.Contains(got, "menu item 'A': call microflow Shop.ACT_WithObject has no argument") {
		t.Errorf("the deprecated spelling was not checked:\n%s", got)
	}
}

// Each of these built clean on 11.14.0 (or is not the item's own call), so
// nothing may be reported.
func TestMenuItemArgs_CheckAcceptsCallsMendixAccepts(t *testing.T) {
	cases := map[string]string{
		"parameterless microflow":  "menu item 'A' ( OnClick: call microflow Shop.ACT_Open )",
		"argument given":           "menu item 'A' ( OnClick: call microflow Shop.ACT_WithString(Code = 'abc') )",
		"optional page parameter":  "menu item 'A' ( OnClick: show page Shop.Thing_Optional )",
		"page parameter default":   "menu item 'A' ( OnClick: show page Shop.Thing_Default )",
		"page without parameters":  "menu item 'A' ( OnClick: show page Shop.Home )",
		"unresolved target":        "menu item 'A' ( OnClick: call microflow Shop.ACT_Gone )",
		"sign out":                 "menu item 'A' ( OnClick: sign out )",
		"open link":                "menu item 'A' ( OnClick: open link 'https://example.com' )",
		"parameterless nanoflow":   "menu item 'A' ( OnClick: call nanoflow Shop.NF_Go )",
		"argument, case differing": "menu item 'A' ( OnClick: call microflow Shop.ACT_WithString(code = 'abc') )",
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			f := newMenuArgFixture(t)
			got, _ := f.check(t, "mdl 1;\ncreate or modify navigation Responsive {\n  "+item+"\n};")
			if strings.Contains(got, "CE1571") {
				t.Errorf("check -p reported a call Mendix accepts:\n%s", got)
			}
		})
	}
}

// A flow or page the script creates counts, as for a page's widgets: the
// microflow and the menu item calling it are usually one script.
func TestMenuItemArgs_CheckUsesFlowsAndPagesTheScriptCreates(t *testing.T) {
	f := newMenuArgFixture(t)
	got, _ := f.check(t, `mdl 1;
create microflow Shop.ACT_New ($Code: string) begin end;
create microflow Shop.ACT_Open ($Code: string) begin end;
create page Shop.New_Optional ( Params: ( $Code: string optional ), Title: 'New', Layout: Atlas_Core.Atlas_Default ) { };
create or modify navigation Responsive {
  menu item 'New' ( OnClick: call microflow Shop.ACT_New )
  menu item 'Redefined' ( OnClick: call microflow Shop.ACT_Open )
  menu item 'Optional' ( OnClick: show page Shop.New_Optional )
};`)
	for _, want := range []string{
		"menu item 'New': call microflow Shop.ACT_New has no argument for parameter 'Code'",
		// The script's definition wins over the stored, parameterless one.
		"menu item 'Redefined': call microflow Shop.ACT_Open has no argument for parameter 'Code'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("check -p lacks %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "'Optional'") {
		t.Errorf("a script page's optional parameter was reported:\n%s", got)
	}
}

// An alter is judged on what it states: the items it inserts and the actions it
// sets, not a caption change or the stored items it leaves alone.
func TestMenuItemArgs_AlterJudgesOnlyWhatItStates(t *testing.T) {
	f := newMenuArgFixture(t)
	got, _ := f.check(t, `alter navigation Responsive {
  insert after 'Stored' { menu item 'New' ( OnClick: call microflow Shop.ACT_WithObject ) }
  set ( OnClick: show page Shop.Thing_Edit ) on 'Other'
  set ( Caption: 'Renamed' ) on 'Third'
};`)
	for _, want := range []string{
		"menu item 'New': call microflow Shop.ACT_WithObject",
		"menu item 'Other': show page Shop.Thing_Edit",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("check -p lacks %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "'Third'") {
		t.Errorf("a caption change was judged on its arguments:\n%s", got)
	}
	if clean, _ := f.check(t, `alter navigation Responsive { set ( Caption: 'Start' ) on 'Stored' };`); strings.Contains(clean, "CE1571") {
		t.Errorf("an alter that states no call was judged:\n%s", clean)
	}
}

func TestMenuItemArgs_CheckMenuDocuments(t *testing.T) {
	f := newMenuArgFixture(t)
	got, _ := f.check(t, `create or modify menu Shop.Main_Menu {
  menu item 'Ok' ( OnClick: call microflow Shop.ACT_Open )
  menu item 'Bad' ( OnClick: call microflow Shop.ACT_WithObject )
};`)
	if !strings.Contains(got, "menu Shop.Main_Menu has argument errors") || !strings.Contains(got, "menu item 'Bad'") {
		t.Errorf("check -p did not report the menu document's item:\n%s", got)
	}
	if strings.Contains(got, "'Ok'") {
		t.Errorf("a complete call was reported:\n%s", got)
	}
}
