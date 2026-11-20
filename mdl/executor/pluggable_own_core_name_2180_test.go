// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// ako/mxcli#2180 (mendixlabs/mxcli#1833): PlatformDesignSystem 44.4.0's Button
// declares property keys of its own named `caption`, `buttonStyle` and `icon`,
// its Form label one named `label`. mxcli read those names as the core widget
// properties first:
//
//	check:  pdsbutton (buttonStyle: 'fillBlue') -> MDL-WIDGET02 unknown button style
//	check:  pdsbutton (icon: 'bookmark')        -> icon is not a qualified reference
//	create: pdsbutton (icon: 'bookmark')        -> icon silently dropped
//	create: pdsformlabel (label: 'Name')        -> a core LabelTemplate beside the own `label`
//
// PDS is not downloadable here, so the widget below is shaped like it: its
// package declares exactly those keys. `caption` is a plain string, the shape
// whose value the explicit-property pass writes (a text template has a mapping).
const pdsButtonXML = `<?xml version="1.0" encoding="utf-8" ?>
<widget id="test.pds.PdsButton" pluginWidget="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>PDS button</name>
  <properties>
    <propertyGroup caption="General">
      <propertyGroup caption="General">
        <property key="caption" type="string" required="false"><caption>Caption</caption><description/></property>
        <property key="label" type="textTemplate" required="false"><caption>Label</caption><description/></property>
        <property key="buttonStyle" type="enumeration" defaultValue="fill">
          <caption>Button style</caption><description/>
          <enumerationValues>
            <enumerationValue key="fill">Fill</enumerationValue>
            <enumerationValue key="fillBlue">Fill blue</enumerationValue>
            <enumerationValue key="outline">Outline</enumerationValue>
          </enumerationValues>
        </property>
        <property key="icon" type="string" required="false"><caption>Icon</caption><description/></property>
      </propertyGroup>
      <propertyGroup caption="Common"><systemProperty key="Name"/></propertyGroup>
    </propertyGroup>
  </properties>
</widget>`

// plainButtonXML is the control: a pluggable widget declaring none of the
// names, so `label:` keeps meaning the widget-level label.
const plainButtonXML = `<?xml version="1.0" encoding="utf-8" ?>
<widget id="test.pds.PlainButton" pluginWidget="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>Plain button</name>
  <properties>
    <propertyGroup caption="General">
      <propertyGroup caption="General">
        <property key="value" type="textTemplate" required="false"><caption>Value</caption><description/></property>
      </propertyGroup>
    </propertyGroup>
  </properties>
</widget>`

// writePDSProject writes a project directory holding both widgets' packages
// and returns the .mpr path.
func writePDSProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, xml := range map[string]string{"PdsButton": pdsButtonXML, "PlainButton": plainButtonXML} {
		f, err := os.Create(filepath.Join(dir, "widgets", name+".mpk"))
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(f)
		for entry, body := range map[string]string{
			"package.xml": `<package><clientModule name="` + name + `" version="1.0.0"><widgetFiles><widgetFile path="` +
				name + `.xml"/></widgetFiles></clientModule></package>`,
			name + ".xml": xml,
		} {
			w, err := zw.Create(entry)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(w, body); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	mpk.ClearCache()
	t.Cleanup(mpk.ClearCache)
	return filepath.Join(dir, "App.mpr")
}

// pdsButtonDef is the definition mxcli generates for the widget, with the
// enumeration keys its installed package declares.
func pdsButtonDef(t *testing.T, mprPath string) *WidgetDefinition {
	t.Helper()
	m, err := mpk.ParseMPK(filepath.Join(filepath.Dir(mprPath), "widgets", "PdsButton.mpk"))
	if err != nil {
		t.Fatalf("parse mpk: %v", err)
	}
	def := GenerateDefJSON(m, "pdsbutton")
	def.enumKeys = map[string][]string{"buttonstyle": {"fill", "fillBlue", "outline"}}
	return def
}

func pdsEngine(t *testing.T, mprPath string) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	b := &mock.MockBackend{
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		PathFunc:               func() string { return mprPath },
	}
	pb := &pageBuilder{backend: b, paramEntityNames: map[string]string{}, widgetScope: map[string]model.ID{}}
	e := NewPluggableWidgetEngine(b, pb)
	pb.pluggableEngine = e
	return e
}

// pdsButtonAsParsed is the widget as the visitor hands it over: `caption:`,
// `label:` and `buttonStyle:` are MDL keywords lowered to their core spelling,
// and `icon:` arrives as a typed collection icon.
func pdsButtonAsParsed() *ast.WidgetV3 {
	return &ast.WidgetV3{Type: "pdsbutton", Name: "btn", Properties: map[string]any{
		"Caption":     "Save",
		"Label":       "Name",
		"ButtonStyle": "fillBlue",
		"Icon":        &ast.WidgetIcon{Kind: types.MenuIconCollection, Name: "bookmark"},
	}}
}

func TestCreatePluggableOwnCoreNames_WritesTheWidgetsOwnKeys(t *testing.T) {
	mpr := writePDSProject(t)
	w, err := pdsEngine(t, mpr).Build(pdsButtonDef(t, mpr), pdsButtonAsParsed())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := renderedWidgetStrings(t, w)
	for _, want := range []string{"Save", "Name", "fillBlue", "bookmark"} {
		if !strings.Contains(got, want) {
			t.Errorf("own property value %q not written:\n%s", want, got)
		}
	}
	// `label:` is the widget's own key; no core label is invented beside it.
	if w.Label != "" {
		t.Errorf("core Label = %q, want none: the widget declares its own `label`", w.Label)
	}
}

// Control: a pluggable widget that declares none of the names keeps the old
// reading — `label:` is the widget-level label, `Icon:` is not its property.
func TestCreatePluggableOwnCoreNames_NoOwnKeyKeepsCoreLabel(t *testing.T) {
	mpr := writePDSProject(t)
	m, err := mpk.ParseMPK(filepath.Join(filepath.Dir(mpr), "widgets", "PlainButton.mpk"))
	if err != nil {
		t.Fatal(err)
	}
	def := GenerateDefJSON(m, "plainbutton")
	w, err := pdsEngine(t, mpr).Build(def, &ast.WidgetV3{Type: "plainbutton", Name: "b",
		Properties: map[string]any{"Label": "Core"}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if w.Label != "Core" {
		t.Errorf("core Label = %q, want Core", w.Label)
	}
}

func TestCheckPluggableOwnButtonStyle_JudgedByTheWidgetsOwnEnumeration(t *testing.T) {
	mpr := writePDSProject(t)
	def := pdsButtonDef(t, mpr)
	reg := &WidgetRegistry{
		byMDLName:  map[string]*WidgetDefinition{"PDSBUTTON": def},
		byWidgetID: map[string]*WidgetDefinition{def.WidgetID: def},
	}
	rules := func(w *ast.WidgetV3) []string {
		var ids []string
		for _, v := range validateWidgetTree([]*ast.WidgetV3{w}, reg, "page P") {
			ids = append(ids, v.RuleID+": "+v.Message)
		}
		return ids
	}
	has := func(ids []string, rule string) bool {
		for _, id := range ids {
			if strings.HasPrefix(id, rule) {
				return true
			}
		}
		return false
	}

	for _, style := range []string{"fill", "fillBlue", "outline"} {
		got := rules(&ast.WidgetV3{Type: "pdsbutton", Name: "btn", Properties: map[string]any{"ButtonStyle": style}})
		if has(got, "MDL-WIDGET02") {
			t.Errorf("buttonStyle %q is the widget's own enumeration key, but check said %v", style, got)
		}
	}
	// A key the widget does not declare is still refused — by its own list.
	got := rules(&ast.WidgetV3{Type: "pdsbutton", Name: "btn", Properties: map[string]any{"ButtonStyle": "Primary"}})
	if has(got, "MDL-WIDGET02") || !has(got, widgetEnumRefused.Code) {
		t.Errorf("buttonStyle Primary on a pdsbutton: want %s naming its own keys, got %v", widgetEnumRefused.Code, got)
	}
	// Control: a core button keeps MDL-WIDGET02.
	got = rules(&ast.WidgetV3{Type: "actionbutton", Name: "ab", Properties: map[string]any{"ButtonStyle": "fillBlue"}})
	if !has(got, "MDL-WIDGET02") {
		t.Errorf("a core actionbutton with ButtonStyle fillBlue must keep MDL-WIDGET02, got %v", got)
	}
}

func TestCheckPluggableOwnIcon_IsNotAnIconReference(t *testing.T) {
	mpr := writePDSProject(t)
	def := pdsButtonDef(t, mpr)
	reg := &WidgetRegistry{
		byMDLName:  map[string]*WidgetDefinition{"PDSBUTTON": def},
		byWidgetID: map[string]*WidgetDefinition{def.WidgetID: def},
	}
	stmt := &ast.CreatePageStmtV3{Widgets: []*ast.WidgetV3{
		pdsButtonAsParsed(),
		{Type: "actionbutton", Name: "ab", Properties: map[string]any{
			"Icon": &ast.WidgetIcon{Kind: types.MenuIconCollection, Name: "pencil"},
		}},
	}}
	var resolved []string
	for _, ref := range iconRefsToResolve(stmt, reg) {
		resolved = append(resolved, ref.value)
	}
	if strings.Join(resolved, ",") != "pencil" {
		t.Errorf("icon references to resolve = %v, want only the core button's [pencil]", resolved)
	}
	// Control: with no definitions, nothing is known to own `icon`.
	if n := len(iconRefsToResolve(stmt, nil)); n != 2 {
		t.Errorf("with no registry, %d references, want both", n)
	}
}
