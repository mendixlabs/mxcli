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
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	mmpk "github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// ako/mxcli#1776, #2507: a widget that hides properties through its OWN helper,
// the shape ChessClock uses, has no rule the static reader can lift — so check
// passed a hidden value and exec wrote a hidden text template with its default
// text, both CE0463. Running getProperties answers it.

const clockWidgetXML = `<?xml version="1.0" encoding="utf-8" ?>
<widget id="test.clock.Clock" pluginWidget="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>Clock</name>
  <properties>
    <propertyGroup caption="General">
      <propertyGroup caption="Mode">
        <property key="isCorrespondence" type="boolean" defaultValue="false"><caption>Correspondence</caption><description/></property>
        <property key="lowTimeMs" type="integer" defaultValue="10000"><caption>Low time</caption><description/></property>
        <property key="textDays" type="textTemplate" required="false"><caption>Days</caption><description/>
          <translations><translation lang="en_US">DaysLeft</translation></translations></property>
      </propertyGroup>
    </propertyGroup>
  </properties>
</widget>`

// Hides lowTimeMs in correspondence games and textDays otherwise.
const clockEditorConfig = `"use strict";
function t(e, o) { e.forEach(function (e) {
  e.properties && (e.properties = e.properties.filter(function (e) { return o.indexOf(e.key) < 0 }));
  e.propertyGroups && t(e.propertyGroups, o) }) }
exports.getProperties = function (e, r) {
  e.isCorrespondence ? t(r, ["lowTimeMs"]) : t(r, ["textDays"]);
  return r;
};`

func clockWidgetRegistry(t *testing.T) (*WidgetRegistry, *WidgetDefinition) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	mpkPath := filepath.Join(dir, "widgets", "Clock.mpk")
	f, err := os.Create(mpkPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for entry, body := range map[string]string{
		"package.xml":           `<package><clientModule name="Clock" version="1.0.0"><widgetFiles><widgetFile path="Clock.xml"/></widgetFiles></clientModule></package>`,
		"Clock.xml":             clockWidgetXML,
		"Clock.editorConfig.js": clockEditorConfig,
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
	def := GenerateDefJSON(m, "clockwidget")
	reg := &WidgetRegistry{
		byMDLName:   map[string]*WidgetDefinition{"CLOCKWIDGET": def},
		byWidgetID:  map[string]*WidgetDefinition{def.WidgetID: def},
		projectPath: filepath.Join(dir, "App.mpr"),
	}
	return reg, def
}

func hiddenViolations(t *testing.T, reg *WidgetRegistry, widget string) []string {
	t.Helper()
	prog, errs := visitor.Build("create page M.P (Title: 'P', Layout: M.L) {\n" + widget + "\n};")
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], widget)
	}
	var got []string
	for _, v := range validateWidgetTree(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets, reg, "page M.P") {
		if v.RuleID == "MDL-WIDGET10" {
			got = append(got, v.Message)
		}
	}
	return got
}

func TestEvaluatedVisibility_CheckReportsAPropertyNoRuleWasLiftedFor(t *testing.T) {
	reg, def := clockWidgetRegistry(t)
	if rules := visibilityRulesFor(def, reg); len(rules) != 0 {
		t.Fatalf("control: the static reader lifted %d rules from a widget's own helper, want 0", len(rules))
	}
	got := hiddenViolations(t, reg, "clockwidget c1 (isCorrespondence: true, lowTimeMs: 5000)")
	if len(got) != 1 || !strings.Contains(got[0], "`lowTimeMs` is hidden by the widget's editor configuration") ||
		!strings.Contains(got[0], "CE0463") {
		t.Fatalf("MDL-WIDGET10 = %q, want one error for lowTimeMs", got)
	}
	// The same value with the property shown is fine.
	if got := hiddenViolations(t, reg, "clockwidget c1 (isCorrespondence: false, lowTimeMs: 5000)"); len(got) != 0 {
		t.Errorf("shown property reported: %q", got)
	}
}

func TestEvaluatedVisibility_HiddenSetFollowsTheValues(t *testing.T) {
	reg, def := clockWidgetRegistry(t)
	hidden, ok := evaluateHiddenProperties(reg.projectPath, def.WidgetID, nil)
	if !ok || !hidden["textDays"] || hidden["lowTimeMs"] {
		t.Fatalf("defaults: hidden %v ok %v, want textDays only", hidden, ok)
	}
	hidden, ok = evaluateHiddenProperties(reg.projectPath, def.WidgetID, map[string]string{"iscorrespondence": "true"})
	if !ok || hidden["textDays"] || !hidden["lowTimeMs"] {
		t.Fatalf("correspondence: hidden %v ok %v, want lowTimeMs only", hidden, ok)
	}
}

// No package to run, or a bundle that throws: the lifted rules stand as they
// are, so a widget mxcli cannot evaluate behaves exactly as before.
func TestEvaluatedVisibility_FallsBackToLiftedRules(t *testing.T) {
	lifted := []types.WidgetVisibilityRule{{PropertyKey: "a", HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "b", Operator: "eq", Value: "x"}}}
	got := evaluatedVisibilityRules(t.TempDir(), "no.such.Widget", lifted, nil)
	if len(got) != 1 || got[0].Evaluated() {
		t.Errorf("rules %+v, want the lifted rule unchanged", got)
	}
}

// The merge: a property the package hides keeps its lifted rule (it names the
// condition) and gains one that always fires; a property it shows has its
// lifted rule replaced by one that never fires — a lifted rule from a guard on
// Studio's `target` said otherwise (TreeNode). Nested rules are kept.
func TestMergeEvaluatedRules(t *testing.T) {
	cond := &types.WidgetVisibilityCondition{PropertyKey: "advancedMode", Operator: "falsy"}
	lifted := []types.WidgetVisibilityRule{
		{PropertyKey: "animate", HiddenWhen: cond},
		{PropertyKey: "headerCaption", HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "headerType", Operator: "eq", Value: "custom"}},
		{PropertyKey: "sub", ListPropertyKey: "items", HiddenWhen: cond},
	}
	got := mergeEvaluatedRules(lifted, map[string]bool{"headerCaption": true, "advancedMode": true})
	byKey := map[string][]types.WidgetVisibilityRule{}
	for _, r := range got {
		byKey[r.PropertyKey] = append(byKey[r.PropertyKey], r)
	}
	if rs := byKey["animate"]; len(rs) != 1 || rs[0].HiddenWhen.Operator != types.OperatorNever {
		t.Errorf("animate (shown in Studio Pro) = %+v, want one never-rule", rs)
	}
	if rs := byKey["headerCaption"]; len(rs) != 2 || rs[0].Evaluated() || rs[1].HiddenWhen.Operator != types.OperatorAlways {
		t.Errorf("headerCaption (hidden) = %+v, want its lifted rule then an always-rule", rs)
	}
	if rs := byKey["advancedMode"]; len(rs) != 1 || rs[0].HiddenWhen.Operator != types.OperatorAlways {
		t.Errorf("advancedMode (hidden, no lifted rule) = %+v, want an always-rule", rs)
	}
	if rs := byKey["sub"]; len(rs) != 1 || !rs[0].Nested() {
		t.Errorf("nested rule = %+v, want kept", rs)
	}
	for _, r := range got {
		fires, determinable := r.Fires(func(types.WidgetVisibilityCondition) (string, bool) { return "", false })
		if r.Evaluated() && (!determinable || fires != (r.HiddenWhen.Operator == types.OperatorAlways)) {
			t.Errorf("%s: Fires = %v,%v without any lookup, want decided by its operator", r.PropertyKey, fires, determinable)
		}
	}
}

// exec: a hidden text template the script never names is stored null, as Studio
// Pro stores it — not with the package's default text (ako/mxcli#2507).
func TestEvaluatedVisibility_BuildNullsAHiddenTextTemplate(t *testing.T) {
	reg, def := clockWidgetRegistry(t)
	e := pdsEngine(t, reg.projectPath)

	hiddenCase, err := e.Build(def, &ast.WidgetV3{Type: "clockwidget", Name: "c1", Properties: map[string]any{}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := renderedWidgetStrings(t, hiddenCase); strings.Contains(got, "DaysLeft") {
		t.Errorf("textDays is hidden by default but was written with its default text:\n%s", got)
	}
	// Control: shown, the template keeps the package's text.
	shownCase, err := e.Build(def, &ast.WidgetV3{Type: "clockwidget", Name: "c2", Properties: map[string]any{"isCorrespondence": true}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := renderedWidgetStrings(t, shownCase); !strings.Contains(got, "DaysLeft") {
		t.Errorf("control: textDays is shown in a correspondence game but its default text is missing:\n%s", got)
	}
}

// describe: the evaluated answer decides what the example leaves out, over a
// lifted rule that says otherwise (TreeNode's Studio-only guard).
func TestEvaluatedVisibility_DecidesTheExample(t *testing.T) {
	d := WidgetDescription{
		Properties: []DescribedProperty{{Key: "advancedMode", Type: "boolean", Default: "false"}, {Key: "animate", Type: "boolean"}},
		Rules:      []DescribedRule{{Property: "animate", Cond: &types.WidgetVisibilityCondition{PropertyKey: "advancedMode", Operator: "falsy"}}},
	}
	if !hiddenUnder(d, "animate") {
		t.Fatal("control: the lifted rule hides animate while advancedMode is unset")
	}
	d.evaluated, d.evaluatedHidden = true, map[string]bool{"advancedMode": true}
	if hiddenUnder(d, "animate") || !hiddenUnder(d, "ADVANCEDMODE") {
		t.Error("with the package's answer, animate is shown and advancedMode hidden")
	}
}

func TestEvaluatedCoverageText(t *testing.T) {
	if got := evaluatedCoverageText(ruleCoverageText(true, 0, 0)); !strings.HasPrefix(got, "the widget's own hide logic") ||
		strings.Contains(got, "unknown here") {
		t.Errorf("own-helper widget: %q", got)
	}
	if got := evaluatedCoverageText(ruleCoverageText(true, 12, 16)); !strings.HasPrefix(got, "12 of 16 editor hide-rules recognized as rules; check and exec run") {
		t.Errorf("partly read widget: %q", got)
	}
}
