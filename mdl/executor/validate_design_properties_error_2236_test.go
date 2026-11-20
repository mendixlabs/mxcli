// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ako/mxcli#2236 (mendixlabs/mxcli#1850): an unknown design property name or
// option value only warned, and exec wrote it into the page. Measured with mx
// check on 11.12.6, both fail the build:
//
//	DesignProperties: ('Hoover': ON)     CE6083 "Design property Hoover is not supported by your theme"
//	DesignProperties: ('Shadow': 'Tiny') CE6085 "Unknown option Tiny for design property Shadow"
//	DesignProperties: ('Shadow': 'Small') clean (control)
func TestDesignProperties_UnknownKeyAndValueAreErrors(t *testing.T) {
	reg := &ThemeRegistry{WidgetProperties: map[string][]ThemeProperty{
		"DivContainer": {{Name: "Shadow", Type: "Dropdown", Options: []ThemeOption{{Name: "Small"}, {Name: "Medium"}, {Name: "Large"}}}},
	}}
	run := func(key, value string) []linter.Violation {
		w := &ast.WidgetV3{Type: "container", Name: "c", Properties: map[string]any{
			"DesignProperties": []ast.DesignPropertyEntryV3{{Key: key, Value: value}},
		}}
		return validateWidgetDesignProps(w, reg, "page M.P")
	}
	for _, tc := range []struct{ key, value, rule string }{
		{"Hoover", "on", "MDL-WIDGET11"},
		{"Shadow", "Tiny", "MDL-WIDGET12"},
	} {
		got := run(tc.key, tc.value)
		if len(got) != 1 || got[0].RuleID != tc.rule || got[0].Severity != linter.SeverityError {
			t.Errorf("%s=%s: got %+v, want one %s error", tc.key, tc.value, got, tc.rule)
		}
	}
	if got := run("Shadow", "Small"); len(got) != 0 {
		t.Errorf("control: a declared option was reported: %+v", got)
	}
}
