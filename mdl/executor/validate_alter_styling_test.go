// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// atlasShapedThemeRegistry mirrors the shape Atlas ships on 11.12.2: a `Widget` group
// every widget inherits, plus type-specific groups.
func atlasShapedThemeRegistry() *ThemeRegistry {
	return &ThemeRegistry{WidgetProperties: map[string][]ThemeProperty{
		"Widget": {
			{Name: "Align self", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Left"}, {Name: "Right"}}},
			{Name: "Hide on", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Phone"}, {Name: "Tablet"}}},
		},
		"ListView": {
			{Name: "Style", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Lined"}, {Name: "Striped"}}},
			{Name: "Hover style", Type: "Toggle", Class: "listview-hover"},
			{Name: "Row size", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Small"}, {Name: "Large"}}},
		},
		"Button": {
			{Name: "Size", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Small"}, {Name: "Large"}}},
		},
	}}
}

func stylingViolations(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog := parseMDL(t, src)
	return validateAlterStylingDesignProps(prog, atlasShapedThemeRegistry())
}

// ako/mxcli#509. `ALTER STYLING` is the one statement whose entire job is
// writing design properties, and it was the one statement MDL-WIDGET11 never
// looked at — ValidateDesignPropertiesForStatement switches on CreatePageStmtV3,
// CreateSnippetStmtV3 and AlterPageStmt, and `*ast.AlterStylingStmt` is not
// among them.
//
// Measured on a blank Mendix 11.12.2 project:
//
//	alter styling on page … widget lvThings set 'Remove empty text' = on;
//	  mxcli check --references -> Check passed!
//	  mxcli exec               -> Updated styling on widget "lvThings"
//	  mxcli docker check       -> [CE6083] "Design property Remove empty text is
//	                              not supported by your theme." 1 error
//
// The control, a key the theme does declare, is 0 errors.
func TestAlterStyling_UndeclaredKeyIsReported(t *testing.T) {
	got := stylingViolations(t, `alter styling on page M.P widget lvThings
	  set 'Remove empty text' = on;`)
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %#v", len(got), got)
	}
	if got[0].RuleID != "MDL-WIDGET11" {
		t.Errorf("RuleID = %q, want MDL-WIDGET11", got[0].RuleID)
	}
	// An error (ako/mxcli#2236): the registry is the project's own
	// themesource, the theme mxbuild builds against, and CE6083 above is what
	// a warning let exec write.
	if got[0].Severity != linter.SeverityError {
		t.Errorf("severity = %v, want Error — mxbuild refuses it with CE6083", got[0].Severity)
	}
	if !strings.Contains(got[0].Message, "Remove empty text") {
		t.Errorf("message does not name the key: %q", got[0].Message)
	}
}

// The controls. Each must stay silent, and each covers a different way the
// check could be wrong.
func TestAlterStyling_DeclaredKeysAreSilent(t *testing.T) {
	cases := []struct{ name, stmt string }{
		// Type-specific: declared under ListView.
		{"type-specific key", `set 'Row size' = 'Small'`},
		// Inherited: declared under `Widget`, which applies to every widget. A
		// check built from a single type's properties would report this one, and
		// that is the mistake this whole area keeps producing.
		{"inherited key", `set 'Align self' = 'Right'`},
		// Declared for a DIFFERENT widget type. This check cannot tell which
		// widget `lvThings` is — the statement names a stored widget and its
		// $Type lives only in the document — so a key the theme declares
		// SOMEWHERE is accepted. Under-reporting, never over-reporting.
		{"key of another widget type", `set 'Size' = 'Small'`},
		// CSS assignments are not design properties at all.
		{"Class", `set Class = 'my-list'`},
		{"Style", `set Style = 'color: red;'`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "alter styling on page M.P widget lvThings\n  " + c.stmt + ";"
			if got := stylingViolations(t, src); len(got) != 0 {
				t.Errorf("reported a declared key: %#v", got)
			}
		})
	}
}

// CLEAR DESIGN PROPERTIES names no keys, so there is nothing to resolve.
func TestAlterStyling_ClearIsSilent(t *testing.T) {
	src := `alter styling on page M.P widget lvThings clear design properties;`
	if got := stylingViolations(t, src); len(got) != 0 {
		t.Errorf("CLEAR DESIGN PROPERTIES reported something: %#v", got)
	}
}

// Without a theme there is nothing to be undeclared relative to. A project whose
// themesource defines no design properties must report nothing at all, or every
// key in every script becomes a warning.
func TestAlterStyling_NoRegistryMeansNoClaims(t *testing.T) {
	prog := parseMDL(t, `alter styling on page M.P widget lvThings set 'Whatever' = on;`)
	if got := validateAlterStylingDesignProps(prog, nil); len(got) != 0 {
		t.Errorf("claimed a key is undeclared with no theme to judge against: %#v", got)
	}
	empty := &ThemeRegistry{WidgetProperties: map[string][]ThemeProperty{}}
	if got := validateAlterStylingDesignProps(prog, empty); len(got) != 0 {
		t.Errorf("claimed a key is undeclared against an empty registry: %#v", got)
	}
}
