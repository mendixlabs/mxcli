// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#1286 (mendixlabs/mxcli#1221): MDL could create a module and write
// its SCSS, but not mark it a UI resources module nor place it in the theme
// module order, so scripted app setup had to stop for Studio Pro. Measured on
// mx 11.12.6: the order decides where each listed module's themesource lands in
// the compiled theme (listed last wins), passes mx check, and an empty order is
// Mendix's default.
func TestUIResourcesAndThemeModuleOrder_Parse_Issue1286(t *testing.T) {
	prog, errs := visitor.Build(`mdl 1;
create module BrandTheme (UIResources: true);
alter module MyFirstModule set (UIResources: false);
alter settings webui (ThemeModuleOrder: ('Atlas_Core', 'BrandTheme'));`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	create := prog.Statements[0].(*ast.CreateModuleStmt)
	if create.Settings.UIResources == nil || !*create.Settings.UIResources {
		t.Errorf("create module: UIResources = %v, want true", create.Settings.UIResources)
	}
	alter := prog.Statements[1].(*ast.AlterModuleSetStmt)
	if alter.Settings.UIResources == nil || *alter.Settings.UIResources {
		t.Errorf("alter module: UIResources = %v, want false", alter.Settings.UIResources)
	}
	settings := prog.Statements[2].(*ast.AlterSettingsStmt)
	got, ok := settings.Properties["ThemeModuleOrder"].([]string)
	if !ok || strings.Join(got, ",") != "Atlas_Core,BrandTheme" {
		t.Errorf("ThemeModuleOrder = %#v, want [Atlas_Core BrandTheme]", settings.Properties["ThemeModuleOrder"])
	}
	if len(settingsStatementErrors(settings)) != 0 {
		t.Errorf("a valid order was refused: %v", settingsStatementErrors(settings))
	}
}

func TestThemeModuleOrder_Refusals_Issue1286(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"alter settings webui (ThemeModuleOrder: ('A', 'A'));", "names module A twice"},
		{"alter settings webui (ThemeModuleOrder: 'A');", "takes a list"},
		{"alter settings configuration 'Default' (DatabaseName: ('a', 'b'));", "takes one value, not a list"},
	} {
		prog, errs := visitor.Build("mdl 1;\n" + tc.src)
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", tc.src, errs)
		}
		got := settingsStatementErrors(prog.Statements[0].(*ast.AlterSettingsStmt))
		if len(got) == 0 || !strings.Contains(got[0].Error(), tc.want) {
			t.Errorf("%s: errors %v, want %q", tc.src, got, tc.want)
		}
	}
}

// describe module prints the flag only when it is set — an untouched module
// describes as before.
func TestModuleSettingsMDL_UIResources_Issue1286(t *testing.T) {
	ctx, _ := newMockCtx(t)
	if got := moduleSettingsMDL(ctx, appModuleSettings(), false); got != "" {
		t.Errorf("app module, no flag: %q, want nothing", got)
	}
	if got := moduleSettingsMDL(ctx, appModuleSettings(), true); !strings.Contains(got, "UIResources: true") {
		t.Errorf("UI resources module: %q", got)
	}
}
