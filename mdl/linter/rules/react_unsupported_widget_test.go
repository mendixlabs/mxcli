// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"database/sql"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// The two legacy image widgets are not supported by the React client, which
// Mendix added in 10.7 and which is the only client on 11. mxbuild reports
// CE0582 on each whenever it is enabled, so on a Mendix 11 app these are build
// ERRORS, not style points — measured on 11.12.1:
//
//	[error] [CE0582] "Widget static image is not supported in React client.
//	                  Right-click this error to convert it to an alternative
//	                  widget type."   at Static image 'imgLogo'
//
// mxcli can author both (it must: a project being converted up already contains
// them), and nothing told the author they were reaching for a widget their app
// cannot build. This rule is that telling.
func TestReactUnsupportedWidget_NamesTheImageTypesAndTheReplacement(t *testing.T) {
	cases := []struct {
		widgetType string
		wantTerm   string
		wantMDL    string
	}{
		{"Forms$StaticImageViewer", "static image", "staticimage"},
		{"Forms$ImageViewer", "dynamic image", "dynamicimage"},
	}
	for _, c := range cases {
		t.Run(c.widgetType, func(t *testing.T) {
			got, ok := ReactUnsupportedWidget(c.widgetType)
			if !ok {
				t.Fatalf("%s not recognised as a legacy image widget", c.widgetType)
			}
			// Mendix's own term, so the lint message and the CE0582 the author
			// will see from mxbuild use the same words.
			if got.MendixTerm != c.wantTerm {
				t.Errorf("MendixTerm = %q, want %q", got.MendixTerm, c.wantTerm)
			}
			if got.MDLKeyword != c.wantMDL {
				t.Errorf("MDLKeyword = %q, want %q", got.MDLKeyword, c.wantMDL)
			}
		})
	}
}

// The CONTROL, and the reason this is a deny-list rather than anything
// broader: every other widget must be silent. A rule that fires on the
// PLUGGABLE image — the widget it tells people to move TO — would be worse than
// no rule at all.
func TestReactUnsupportedWidget_IsSilentOnTheReplacements(t *testing.T) {
	for _, widgetType := range []string{
		"CustomWidgets$CustomWidget", // the pluggable image lives here
		"Forms$DynamicText",
		"Forms$DataView",
		"DocumentTemplates$StaticImageViewer", // a document template, not a page
		"",
	} {
		if got, ok := ReactUnsupportedWidget(widgetType); ok {
			t.Errorf("%q was reported as React-unsupported (%+v)", widgetType, got)
		}
	}
}

// The rule's identity is part of its contract: an ID that collides with another
// rule's silently shadows it in the config, and the category decides what
// `--category` filters it into.
func TestReactUnsupportedWidgetRule_Identity(t *testing.T) {
	r := NewReactUnsupportedWidgetRule()
	if r.ID() != "MPR012" {
		t.Errorf("ID = %q, want MPR012", r.ID())
	}
	if r.Category() != "correctness" {
		t.Errorf("Category = %q — CE0582 is a build error, not a style preference", r.Category())
	}
	if r.Name() == "" || r.Description() == "" {
		t.Error("a rule with no name or description cannot be configured or explained")
	}
}

// ako/mxcli#953 item 3: CE0582 is the REACT client's error, and a native page
// is not rendered by it. Measured on mxbuild 11.13.0 (JTSBootLogboek): a
// staticimage on a page with layout Atlas_Core.NativePhone_Default builds
// clean, the same widget on an Atlas_Default page is CE0582. The layout's
// platform is what tells them apart — note that the native layout lives in a
// Marketplace module (Atlas_Core), so the lookup must not apply the
// platform-module filter the iterators use. The web page is the control.
func TestReactUnsupportedWidgetRule_SkipsNativePages(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE modules (Id TEXT, Name TEXT PRIMARY KEY, Source TEXT)`,
		`INSERT INTO modules VALUES ('m1', 'MyFirstModule', ''), ('m2', 'Atlas_Core', 'Atlas_Core.mpk')`,
		`CREATE TABLE layouts (Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			LayoutType TEXT, Platform TEXT, Description TEXT)`,
		`INSERT INTO layouts VALUES
			('l1', 'Atlas_Default', 'Atlas_Core.Atlas_Default', 'Atlas_Core', '', 'Responsive', 'Web', ''),
			('l2', 'NativePhone_Default', 'Atlas_Core.NativePhone_Default', 'Atlas_Core', '', 'Default', 'Native', '')`,
		`CREATE TABLE pages (Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			Title TEXT, URL TEXT, LayoutRef TEXT, Description TEXT, WidgetCount INTEGER)`,
		`INSERT INTO pages VALUES
			('p1', 'Logboek_Images', 'MyFirstModule.Logboek_Images', 'MyFirstModule', '', '', '', 'Atlas_Core.Atlas_Default', '', 1),
			('p2', 'Login_Native', 'MyFirstModule.Login_Native', 'MyFirstModule', '', '', '', 'Atlas_Core.NativePhone_Default', '', 1)`,
		`CREATE TABLE widgets (Id TEXT, Name TEXT, WidgetType TEXT, ContainerId TEXT, ContainerQualifiedName TEXT,
			ContainerType TEXT, ModuleName TEXT, EntityRef TEXT, AttributeRef TEXT, MicroflowRef TEXT, NanoflowRef TEXT,
			PageRef TEXT, ParentWidgetId TEXT, Depth INTEGER, Class TEXT, Style TEXT, DynamicClasses TEXT,
			ActionType TEXT, HasConfirmation INTEGER)`,
		`INSERT INTO widgets (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType,
			ModuleName, EntityRef, AttributeRef, MicroflowRef, NanoflowRef) VALUES
			('w1', 'imgWith', 'Forms$StaticImageViewer', 'p1', 'MyFirstModule.Logboek_Images', 'PAGE', 'MyFirstModule', '', '', '', ''),
			('w2', 'imgNative', 'Forms$StaticImageViewer', 'p2', 'MyFirstModule.Login_Native', 'PAGE', 'MyFirstModule', '', '', '', '')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	violations := NewReactUnsupportedWidgetRule().Check(linter.NewLintContextFromDB(catalog.WrapSqlDB(db)))
	if len(violations) != 1 || violations[0].Location.DocumentName != "Logboek_Images" {
		var got []string
		for _, v := range violations {
			got = append(got, v.Location.DocumentName)
		}
		t.Fatalf("want only the web page Logboek_Images reported, got %v", got)
	}
}
