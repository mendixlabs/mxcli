// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// MDL-GRANT04: `Visible: roles (…)` naming a role of another module than the
// document's. Measured on MxBuild 11.16.1 over 17 documents, one `mx check -j`:
//
//	container, dynamic text, layout-grid row, navigation-list item, snippet
//	container, layout container, tab page, a widget inserted by `alter page`:
//	    a role of ANOTHER module (Administration.Administrator, or a role of a
//	    user-created module) is CE0149 "Visibility needs to be updated, reselect
//	    roles in properties"; a role of the document's own module is clean.
//	a list mixing an own role and a foreign one: CE0149.
//
// The same shape as MDL-GRANT01 (CE0148 for a document grant), and like it needs
// no project: both names are in the script.
func visibleRolesModuleViolations(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build("mdl 1;\n" + src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	var out []linter.Violation
	for _, v := range ValidateVisibleRolesModule(prog) {
		if v.RuleID == "MDL-GRANT04" {
			out = append(out, v)
		}
	}
	return out
}

func TestVisibleRolesModule_MDLGrant04(t *testing.T) {
	const page = "create page Wd.P (Title: 'T', Layout: Atlas_Core.Atlas_Default) { "
	cases := []struct {
		name string
		src  string
		want string // "" = clean; else a substring of the message
	}{
		{"container", page + "container c (Visible: roles (Administration.Administrator)) { dynamictext t (Content: 'a') } };", "Administration.Administrator"},
		{"dynamic text", page + "dynamictext t (Content: 'a', Visible: roles (Administration.Administrator)) };", "Administration.Administrator"},
		{"layout grid row", page + "layoutgrid g { row (Visible: roles (Administration.Administrator)) { column (desktopwidth: 12) { dynamictext t (Content: 'a') } } } };", "Administration.Administrator"},
		{"navigation list item", page + "navigationlist nl { item (Action: close page, Visible: roles (Administration.Administrator)) { dynamictext t (Content: 'a') } } };", "Administration.Administrator"},
		{"tab page", page + "tabcontainer tabs { tabpage tp (Caption: 'a', Visible: roles (Administration.Administrator)) { dynamictext t (Content: 'a') } } };", "Administration.Administrator"},
		{"role of a user-created module", page + "container c (Visible: roles (Wd2.Other)) { dynamictext t (Content: 'a') } };", "Wd2.Other"},
		{"own role beside a foreign one", page + "container c (Visible: roles (Wd.Viewer, Administration.Administrator)) { dynamictext t (Content: 'a') } };", "Administration.Administrator"},
		{"roles beside an expression", page + "container c (Visible: false, Visible: roles (Administration.Administrator)) { dynamictext t (Content: 'a') } };", "Administration.Administrator"},
		{"snippet", "create snippet Wd.S { container c (Visible: roles (Administration.Administrator)) { dynamictext t (Content: 'a') } };", "Administration.Administrator"},
		{"layout", "create layout Wd.L (layouttype: 'Default') { container c (Visible: roles (Administration.Administrator)) { dynamictext t (Content: 'a') } placeholder Main };", "Administration.Administrator"},
		{"alter page insert", "alter page Wd.P { insert after base { container ac (Visible: roles (Administration.Administrator)) { dynamictext t (Content: 'a') } } };", "Administration.Administrator"},

		{"own role", page + "container c (Visible: roles (Wd.Viewer)) { dynamictext t (Content: 'a') } };", ""},
		{"own roles, two", page + "container c (Visible: roles (Wd.Viewer, Wd.Other)) { dynamictext t (Content: 'a') } };", ""},
		{"own role, other letter case", page + "container c (Visible: roles (wd.Viewer)) { dynamictext t (Content: 'a') } };", ""},
		{"own role on a tab page", page + "tabcontainer tabs { tabpage tp (Caption: 'a', Visible: roles (Wd.Viewer)) { dynamictext t (Content: 'a') } } };", ""},
		{"own role in a snippet", "create snippet Wd.S { container c (Visible: roles (Wd.Viewer)) { dynamictext t (Content: 'a') } };", ""},
		{"own role in an alter page insert", "alter page Wd.P { insert after base { container ac (Visible: roles (Wd.Viewer)) { dynamictext t (Content: 'a') } } };", ""},
		{"no roles", page + "container c (Visible: false) { dynamictext t (Content: 'a') } };", ""},
		{"no visibility", page + "container c { dynamictext t (Content: 'a') } };", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := visibleRolesModuleViolations(t, c.src)
			switch {
			case c.want == "" && len(got) > 0:
				t.Fatalf("unexpected MDL-GRANT04: %s", got[0].Message)
			case c.want != "" && len(got) == 0:
				t.Fatalf("expected MDL-GRANT04 naming %s, got none", c.want)
			case c.want != "" && !strings.Contains(got[0].Message, c.want):
				t.Fatalf("message %q does not name %s", got[0].Message, c.want)
			}
		})
	}
}

// One violation per widget, however many foreign roles it names, and the message
// names each of them.
func TestVisibleRolesModule_OneViolationPerWidget(t *testing.T) {
	got := visibleRolesModuleViolations(t,
		"create page Wd.P (Title: 'T', Layout: Atlas_Core.Atlas_Default) { container c (Visible: roles (Administration.Administrator, Wd2.Other, Wd.Viewer)) { dynamictext t (Content: 'a') } };")
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	for _, want := range []string{"Administration.Administrator", "Wd2.Other"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("message %q does not name %s", got[0].Message, want)
		}
	}
	if strings.Contains(got[0].Message, "Wd.Viewer") {
		t.Errorf("message names the own-module role Wd.Viewer: %q", got[0].Message)
	}
}

// The rule must be part of the pre-flight pass that `check` and `exec` both run,
// not just a function nobody calls.
func TestVisibleRolesModule_RunsInValidateProgram(t *testing.T) {
	prog, errs := visitor.Build("mdl 1;\ncreate page Wd.P (Title: 'T', Layout: Atlas_Core.Atlas_Default) { container c (Visible: roles (Administration.Administrator)) { dynamictext t (Content: 'a') } };")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	found := false
	for _, v := range ValidateProgram(prog, "") {
		if v.RuleID == "MDL-GRANT04" {
			found = true
		}
	}
	if !found {
		t.Fatal("ValidateProgram did not report MDL-GRANT04")
	}
}
