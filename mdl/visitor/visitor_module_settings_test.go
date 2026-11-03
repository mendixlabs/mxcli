// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1921: the General tab of Studio Pro's Module settings dialog as a
// property list on `create module` and `alter module … set`.
func TestModuleSettings_Parse(t *testing.T) {
	str := func(s string) *string { return &s }
	for _, tc := range []struct {
		src  string
		want ast.Statement
	}{{
		src: "create or modify module Inventory (Version: '2.3.1', ModuleType: addon,);",
		want: &ast.CreateModuleStmt{Name: "Inventory", CreateOrModify: true,
			Settings: ast.ModuleSettingsClause{Version: str("2.3.1"), ModuleType: str("addon")}},
	}, {
		// Keys and the type are case-insensitive; the type is stored lower-case.
		src:  "alter module Inventory set (moduletype: App);",
		want: &ast.AlterModuleSetStmt{ModuleName: "Inventory", Settings: ast.ModuleSettingsClause{ModuleType: str("app")}},
	}, {
		src:  "alter module Inventory set (Version: '1.0.0-rc.1+build.5');",
		want: &ast.AlterModuleSetStmt{ModuleName: "Inventory", Settings: ast.ModuleSettingsClause{Version: str("1.0.0-rc.1+build.5")}},
	}, {
		// Control: no list states no setting, so a rerun keeps what is stored.
		src:  "create or modify module Inventory;",
		want: &ast.CreateModuleStmt{Name: "Inventory", CreateOrModify: true},
	}} {
		t.Run(tc.src, func(t *testing.T) {
			prog := mustBuild(t, tc.src)
			if len(prog.Statements) != 1 || !reflect.DeepEqual(prog.Statements[0], tc.want) {
				t.Fatalf("got %#v, want %#v", prog.Statements, tc.want)
			}
		})
	}
}

// What the dialog would refuse is refused here, so check and exec agree.
func TestModuleSettings_Refused(t *testing.T) {
	for src, want := range map[string]string{
		"alter module M set (Versoin: '1.0.0');":                   "did you mean 'Version'",
		"alter module M set (Version: '1.0.0', Version: '1.0.1');": "sets Version twice",
		"alter module M set (Version: '2.3');":                     "not a SemVer version",
		"alter module M set (Version: 2);":                         "Version takes a string",
		"alter module M set (ModuleType: solution);":               "ModuleType takes app or addon",
		"alter module M set (ModuleType: 'addon');":                "ModuleType takes app or addon",
		"alter module M set ();":                                   "names no setting",
		"create module M (Location: marketplace);":                 "has no property 'Location'",
	} {
		t.Run(src, func(t *testing.T) {
			_, errs := Build(src)
			if len(errs) == 0 || !strings.Contains(errs[0].Error(), want) {
				t.Errorf("errors = %v, want one containing %q", errs, want)
			}
		})
	}
}
