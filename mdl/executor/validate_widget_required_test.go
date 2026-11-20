// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	mmpk "github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// A pluggable widget property the package marks required, left unset, fails
// the build with CE0642 ("Property 'X' is required") or, for a text template,
// CE4899. check said nothing: measured with Native Mobile Resources 12.6.0 on
// mx check 11.13.0, 29 of the 39 native widgets placed bare passed check and
// exec and failed mx check — a gallery with no data source, a slider with no
// attribute, a repeater with no content.
//
// What counts as set: a value, a filled slot, at least one list item. What
// needs nothing: a value with a usable default (an enumeration, a boolean, an
// expression with a defaultValue, a text template with shipped translations),
// and a property the widget's editorConfig hides under the configuration —
// mx does not require what the editor does not show.
const requiredWidgetXML = `<?xml version="1.0" encoding="utf-8" ?>
<widget id="test.req.Req" pluginWidget="true" supportedPlatform="Native" xmlns="http://www.mendix.com/widget/1.0/">
  <name>Req</name>
  <properties>
    <propertyGroup caption="General">
      <propertyGroup caption="General">
        <property key="ds" type="datasource" isList="true"><caption>Data source</caption><description/></property>
        <property key="valueAttribute" type="attribute"><caption>Value attribute</caption><description/>
          <attributeTypes><attributeType name="Decimal"/></attributeTypes></property>
        <property key="content" type="widgets"><caption>Content</caption><description/></property>
        <property key="items" type="object" isList="true"><caption>Items</caption><description/>
          <properties><propertyGroup caption="Item"><property key="itemLabel" type="string" required="false"><caption>Label</caption><description/></property></propertyGroup></properties>
        </property>
        <property key="title" type="textTemplate"><caption>Title</caption><description/></property>
        <property key="hint" type="textTemplate"><caption>Hint</caption><description/>
          <translations><translation lang="en_US">Hint</translation></translations></property>
        <property key="minimum" type="expression" defaultValue="0"><caption>Minimum</caption><description/>
          <returnType type="Decimal"/></property>
        <property key="mode" type="enumeration" defaultValue="plain"><caption>Mode</caption><description/>
          <enumerationValues><enumerationValue key="plain">Plain</enumerationValue><enumerationValue key="url">URL</enumerationValue></enumerationValues></property>
        <property key="urlAttribute" type="attribute"><caption>URL attribute</caption><description/>
          <attributeTypes><attributeType name="String"/></attributeTypes></property>
        <property key="optionalAttribute" type="attribute" required="false"><caption>Optional</caption><description/>
          <attributeTypes><attributeType name="String"/></attributeTypes></property>
      </propertyGroup>
    </propertyGroup>
  </properties>
</widget>`

func requiredWidgetRegistry(t *testing.T) (*WidgetRegistry, *WidgetDefinition) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	mpkPath := filepath.Join(dir, "widgets", "Req.mpk")
	f, err := os.Create(mpkPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for entry, body := range map[string]string{
		"package.xml": `<package><clientModule name="Req" version="1.0.0"><widgetFiles><widgetFile path="Req.xml"/></widgetFiles></clientModule></package>`,
		"Req.xml":     requiredWidgetXML,
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
	mpk.ClearCache()
	mmpk.ClearCache()
	t.Cleanup(func() { mpk.ClearCache(); mmpk.ClearCache() })

	m, err := mpk.ParseMPK(mpkPath)
	if err != nil {
		t.Fatal(err)
	}
	def := GenerateDefJSON(m, "reqwidget")
	// The editor hides urlAttribute unless mode is "url".
	def.PropertyVisibility = []types.WidgetVisibilityRule{{
		PropertyKey: "urlAttribute",
		HiddenWhen:  &types.WidgetVisibilityCondition{PropertyKey: "mode", Operator: "ne", Value: "url"},
	}}
	reg := &WidgetRegistry{
		byMDLName:   map[string]*WidgetDefinition{"REQWIDGET": def},
		byWidgetID:  map[string]*WidgetDefinition{def.WidgetID: def},
		projectPath: filepath.Join(dir, "App.mpr"),
	}
	return reg, def
}

func requiredReported(t *testing.T, reg *WidgetRegistry, widget string) []string {
	t.Helper()
	prog, errs := visitor.Build("create page M.P (Title: 'P', Layout: M.L) {\n" + widget + "\n};")
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], widget)
	}
	var got []string
	for _, v := range validateWidgetTree(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets, reg, "page M.P") {
		if v.RuleID != widgetRequiredRule {
			continue
		}
		// "page M.P: widget `r1` (reqwidget): `key` (Caption) is required …"
		parts := strings.Split(v.Message, "`")
		if len(parts) < 4 {
			t.Fatalf("message shape: %s", v.Message)
		}
		got = append(got, parts[3])
	}
	sort.Strings(got)
	return got
}

func TestWidgetRequired_UnsetRequiredPropertiesReported(t *testing.T) {
	reg, _ := requiredWidgetRegistry(t)
	got := strings.Join(requiredReported(t, reg, "reqwidget r1"), ",")
	if want := "content,ds,items,title,valueAttribute"; got != want {
		t.Errorf("reported %s, want %s — a default, a translation, an enumeration, a hidden and an optional property need nothing", got, want)
	}
}

func TestWidgetRequired_SetPropertiesNotReported(t *testing.T) {
	reg, def := requiredWidgetRegistry(t)
	if len(def.ObjectLists) != 1 || len(def.ChildSlots) != 1 {
		t.Fatalf("definition: %d lists, %d slots — the test widget's shape changed", len(def.ObjectLists), len(def.ChildSlots))
	}
	widget := fmt.Sprintf(`reqwidget r1 (ds: database from M.E, valueAttribute: Score, title: 'T') {
  %s { dynamictext t1 (Content: 'x') }
  %s i1 (itemLabel: 'a')
}`, def.ChildSlots[0].MDLContainer, def.ObjectLists[0].MDLContainer)
	if got := requiredReported(t, reg, widget); len(got) != 0 {
		t.Errorf("reported %v for a widget that sets every required property:\n%s", got, widget)
	}
}

// A property the configuration shows again is required again.
func TestWidgetRequired_ShownByConfigurationIsRequired(t *testing.T) {
	reg, _ := requiredWidgetRegistry(t)
	got := requiredReported(t, reg, "reqwidget r1 (mode: url)")
	if !strings.Contains(strings.Join(got, ","), "urlAttribute") {
		t.Errorf("mode: url shows urlAttribute, so it is required; reported %v", got)
	}
}
