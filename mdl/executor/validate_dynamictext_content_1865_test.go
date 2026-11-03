// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"testing"
)

// ako/mxcli#1865 (mendixlabs/mxcli#1707): `mxcli check -p` on 0.28.0 warned
//
//	MDL-WIDGET10 … property `content` is hidden when … — the value will be ignored
//
// on `dynamictext x (Content: '…')`, also inside a Data grid 2 column, although
// exec writes a Forms$DynamicText whose content renders and passes mx check.
// Without a project there was no warning — the visibility rule came from the
// project.
//
// `dynamictext` is a keyword the page builder builds with its own switch, so no
// pluggable definition may answer for it. On 0.28.0 one could: a
// `.mxcli/widgets/dynamictext.def.json` that `widget init` generated for a
// package widget whose id ends in `dynamictext` took over the keyword in the
// registry, and check validated the built-in against that widget's editorConfig
// rules. mendixlabs/mxcli#1463 closed both doors (the loader refuses a
// builder-owned name; lookupWidgetDef never resolves one), so current main does
// not reproduce. This pins it for `dynamictext` specifically, top-level and in a
// column, since the #1463 test only covers `image` and `radiobuttons`.
const shadowingDynamicTextDef = `{"widgetId":"com.example.widget.web.dynamictext.DynamicText","mdlName":"DYNAMICTEXT","widgetKind":"pluggable","templateFile":"dynamictext.json","defaultEditable":"Always",
 "propertyMappings":[{"propertyKey":"mode","value":"auto","operation":"primitive"},{"propertyKey":"content","source":"Content","operation":"primitive"}],
 "propertyVisibility":[{"propertyKey":"content","hiddenWhen":{"propertyKey":"mode","operator":"eq","value":"auto"}}]}`

func TestCheck1865_DynamicTextContentIsNotHidden(t *testing.T) {
	dir := t.TempDir()
	widgets := filepath.Join(dir, ".mxcli", "widgets")
	if err := os.MkdirAll(widgets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(widgets, "dynamictext.def.json"), []byte(shadowingDynamicTextDef), 0o644); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(dir, "App.mpr")

	src := `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  dynamictext top (Content: 'Hello')
  datagrid dg (DataSource: database from M.E) {
    column (Attribute: Name, Caption: 'Name') { dynamictext inCol (Content: 'plain') }
  }
};`
	for _, v := range ValidateWidgetProperties(parseMDL(t, src), projectPath) {
		if v.RuleID == "MDL-WIDGET10" {
			t.Errorf("false hidden-property warning on a built-in dynamictext: %s", v.Message)
		}
	}
}
