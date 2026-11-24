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
	mmpk "github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// ako/mxcli#2548: exec asked the widget's getProperties about the PRIMITIVE
// values only, so an attribute the statement binds reached it as unset, and a
// rule `!values.evaluation && hide(["textEngine"])` fired. The template the
// script set was stored null — the label rendered blank — while check, which
// passes the whole configuration, said nothing. (EvaluationBar, KnightShift.)

const evalWidgetXML = `<?xml version="1.0" encoding="utf-8" ?>
<widget id="test.eval.Eval" pluginWidget="true" needsEntityContext="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>Eval</name>
  <properties>
    <propertyGroup caption="General">
      <propertyGroup caption="General">
        <property key="evaluation" type="attribute" required="false"><caption>Evaluation</caption><description/>
          <attributeTypes><attributeType name="Decimal"/></attributeTypes></property>
        <property key="textEngine" type="textTemplate" required="false"><caption>Engine</caption><description/></property>
      </propertyGroup>
    </propertyGroup>
  </properties>
</widget>`

const evalEditorConfig = `"use strict";
function h(e, o) { e.forEach(function (e) {
  e.properties && (e.properties = e.properties.filter(function (e) { return o.indexOf(e.key) < 0 }));
  e.propertyGroups && h(e.propertyGroups, o) }) }
exports.getProperties = function (v, p) { v.evaluation || h(p, ["textEngine"]); return p; };`

func evalWidgetProject(t *testing.T) (string, *WidgetDefinition) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	mpkPath := filepath.Join(dir, "widgets", "Eval.mpk")
	f, err := os.Create(mpkPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for entry, body := range map[string]string{
		"package.xml":          `<package><clientModule name="Eval" version="1.0.0"><widgetFiles><widgetFile path="Eval.xml"/></widgetFiles></clientModule></package>`,
		"Eval.xml":             evalWidgetXML,
		"Eval.editorConfig.js": evalEditorConfig,
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
	return filepath.Join(dir, "App.mpr"), GenerateDefJSON(m, "evalwidget")
}

func TestEvaluatedVisibility_BuildSeesTheBoundAttribute(t *testing.T) {
	mpr, def := evalWidgetProject(t)
	e := pdsEngine(t, mpr)

	bound, err := e.Build(def, &ast.WidgetV3{Type: "evalwidget", Name: "e1", Properties: map[string]any{
		"evaluation": "Score", "textEngine": "EngineLabel",
	}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := renderedWidgetStrings(t, bound); !strings.Contains(got, "EngineLabel") {
		t.Errorf("the attribute is bound, so textEngine is shown, but its text was dropped:\n%s", got)
	}
	// Control: with the attribute unbound the package hides the template.
	unbound, err := e.Build(def, &ast.WidgetV3{Type: "evalwidget", Name: "e2", Properties: map[string]any{}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !evaluatedHiddenFor(t, mpr, def, nil)["textEngine"] {
		t.Fatal("control: with nothing bound, getProperties should hide textEngine")
	}
	_ = unbound
}

func evaluatedHiddenFor(t *testing.T, mpr string, def *WidgetDefinition, values map[string]string) map[string]bool {
	t.Helper()
	h, ok := evaluateHiddenProperties(mpr, def.WidgetID, values)
	if !ok {
		t.Fatal("the fixture's editorConfig did not run")
	}
	return h
}
