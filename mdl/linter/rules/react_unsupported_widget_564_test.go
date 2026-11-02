// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#564: MPR012 named only the two legacy image widgets, but CE0582 is
// a whole set. Measured with mxbuild on a project switched to the React client
// (Web UI → UseOptimizedClient = Yes):
//
//   - 11.16.0, FactoryManagement: CE0582 on "data grid", "reference selector"
//     and "reference selector (with select page)" — both are
//     Forms$ReferenceSelector — beside the static and dynamic image;
//   - 11.14.0: CE0582 on the classic drop-down (MDL-WIDGET40's measurement).
//
// The controls are measured too, and must stay silent: in the same 11.16 build
// the navigation tree, menu bar, simple menu bar, file manager and scroll
// container raised nothing; on 11.13.0 (JTSBootLogboek) neither did the image
// uploader; on a fresh 11.15.0 app neither did a text box, text area, check
// box, date picker, radio buttons, file manager, list view, group box or
// navigation list. The template grid and the reference set selectors were on
// no web page available to measure, so they are not claimed either way.
func TestReactUnsupportedWidgets_CoverTheMeasuredCE0582Set(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()

	unsupported := map[string]string{ // storage type -> mxbuild's term in CE0582
		"Forms$StaticImageViewer": "static image",
		"Forms$ImageViewer":       "dynamic image",
		"Forms$DropDown":          "drop-down",
		"Forms$DataGrid":          "data grid",
		"Forms$ReferenceSelector": "reference selector",
	}
	supported := []string{
		"Forms$NavigationTree", "Forms$MenuBar", "Forms$SimpleMenuBar", "Forms$FileManager",
		"Forms$ScrollContainer", "Forms$ImageUploader", "Forms$TextBox", "Forms$TextArea",
		"Forms$CheckBox", "Forms$DatePicker", "Forms$RadioButtonGroup", "Forms$ListView",
		"Forms$GroupBox", "Forms$NavigationList",
		"CustomWidgets$CustomWidget", // the replacements: image, combobox, datagrid
		// unmeasured, so not claimed
		"Forms$TemplateGrid", "Forms$ReferenceSetSelector", "Forms$InputReferenceSetSelector",
	}

	stmts := []string{
		`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source) VALUES
			('m1', 'MyFirstModule', 'MyFirstModule', 'MyFirstModule', '')`,
		`INSERT INTO pages_data (Id, Name, QualifiedName, ModuleName, LayoutRef) VALUES
			('p1', 'P_Legacy', 'MyFirstModule.P_Legacy', 'MyFirstModule', 'Atlas_Core.Atlas_Default')`,
	}
	n := 0
	add := func(widgetType string) {
		n++
		stmts = append(stmts, `INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType, ModuleName)
			VALUES ('w`+strings.Repeat("x", n)+`', '`+widgetType+`', '`+widgetType+`', 'p1', 'MyFirstModule.P_Legacy', 'PAGE', 'MyFirstModule')`)
	}
	for wt := range unsupported {
		add(wt)
	}
	for _, wt := range supported {
		add(wt)
	}
	for _, stmt := range stmts {
		if _, err := cat.CatalogDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	reader := settingsReader{ps: &model.ProjectSettings{WebUI: &model.WebUISettings{UseOptimizedClient: "Yes"}}}
	vs := NewReactUnsupportedWidgetRule().Check(linter.NewLintContext(cat, reader))

	var reported []string
	for _, v := range vs {
		// The widget is named after its storage type in this fixture, so the
		// message says which one it was.
		hit := ""
		for wt := range unsupported {
			if strings.Contains(v.Message, "'"+wt+"'") {
				hit = wt
			}
		}
		if hit == "" {
			t.Errorf("reported a widget outside the measured CE0582 set: %q", v.Message)
			continue
		}
		reported = append(reported, hit)
		// mxbuild's own words, so the lint finding and the build error match.
		if !strings.HasPrefix(v.Message, unsupported[hit]+" ") {
			t.Errorf("%s: message %q does not open with mxbuild's term %q", hit, v.Message, unsupported[hit])
		}
		if v.Suggestion == "" {
			t.Errorf("%s: no suggestion — the replacement is the useful half of the finding", hit)
		}
	}
	sort.Strings(reported)
	var want []string
	for wt := range unsupported {
		want = append(want, wt)
	}
	sort.Strings(want)
	if strings.Join(reported, ",") != strings.Join(want, ",") {
		t.Errorf("reported %v, want exactly %v", reported, want)
	}
}
