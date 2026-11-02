// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// TestFormattingInfoFromParamFormat: a format block starts from the Mendix
// defaults and applies only the keys the user set (ledger #75).
func TestFormattingInfoFromParamFormat(t *testing.T) {
	if fi := formattingInfoFromParamFormat(nil); fi != nil {
		t.Errorf("nil format must yield nil FormattingInfo (keeps default write path), got %+v", fi)
	}
	if fi := formattingInfoFromParamFormat(&ast.ParamFormatV3{}); fi != nil {
		t.Errorf("empty format must yield nil, got %+v", fi)
	}

	f := &ast.ParamFormatV3{Props: []ast.ParamFormatProp{
		{Key: "decimalprecision", Value: "4"},
		{Key: "groupdigits", Value: "true"},
	}}
	fi := formattingInfoFromParamFormat(f)
	if fi == nil {
		t.Fatal("expected FormattingInfo")
	}
	if fi.DecimalPrecision != 4 || !fi.GroupDigits {
		t.Errorf("decimal/group = %d/%v, want 4/true", fi.DecimalPrecision, fi.GroupDigits)
	}
	// Untouched fields keep the defaults (so the writer stays schema-aligned).
	if fi.DateFormat != "Date" || fi.EnumFormat != "Text" {
		t.Errorf("defaults not preserved: DateFormat=%q EnumFormat=%q", fi.DateFormat, fi.EnumFormat)
	}

	dt := formattingInfoFromParamFormat(&ast.ParamFormatV3{Props: []ast.ParamFormatProp{
		{Key: "dateformat", Value: "Custom"}, {Key: "customdateformat", Value: "dd-MM-yyyy"},
	}})
	if dt.DateFormat != "Custom" || dt.CustomDateFormat != "dd-MM-yyyy" {
		t.Errorf("custom date: %q / %q", dt.DateFormat, dt.CustomDateFormat)
	}
}

// TestFormattingInfoFromParamFormat_EnumCase: check accepts the enum keys in
// any case (MDL-WIDGET18 lowercases before looking them up), so the converter
// must store Mendix's spelling. `dateFormat: custom` was stored as "custom",
// which Mendix does not read as Custom: the runtime showed the default format
// and every rerun diffed as modified (ako/mxcli#1864, mendixlabs/mxcli#1706).
func TestFormattingInfoFromParamFormat_EnumCase(t *testing.T) {
	tests := []struct {
		key, value, want string
		got              func(fi *pages.FormattingInfo) string
	}{
		{"dateformat", "custom", "Custom", func(fi *pages.FormattingInfo) string { return fi.DateFormat }},
		{"dateformat", "datetime", "DateTime", func(fi *pages.FormattingInfo) string { return fi.DateFormat }},
		{"dateformat", "TIME", "Time", func(fi *pages.FormattingInfo) string { return fi.DateFormat }},
		{"enumformat", "image", "Image", func(fi *pages.FormattingInfo) string { return fi.EnumFormat }},
		// Control: the canonical spelling passes through unchanged.
		{"dateformat", "Custom", "Custom", func(fi *pages.FormattingInfo) string { return fi.DateFormat }},
	}
	for _, tt := range tests {
		fi := formattingInfoFromParamFormat(&ast.ParamFormatV3{Props: []ast.ParamFormatProp{
			{Key: tt.key, Value: tt.value}, {Key: "customdateformat", Value: "d MMM yyyy, HH:mm"},
		}})
		if got := tt.got(fi); got != tt.want {
			t.Errorf("%s: %s stored as %q, want %q", tt.key, tt.value, got, tt.want)
		}
	}
}

func dtWidget(params []ast.ParamAssignmentV3, props map[string]any) *ast.WidgetV3 {
	all := map[string]any{"ContentParams": params}
	for k, v := range props {
		all[k] = v
	}
	return &ast.WidgetV3{Name: "txt", Type: "dynamictext", Properties: all}
}

func TestValidateDynamicTextFormatting(t *testing.T) {
	fmtBlock := func(props ...ast.ParamFormatProp) []ast.ParamAssignmentV3 {
		return []ast.ParamAssignmentV3{{Index: 1, Value: "Amount", Format: &ast.ParamFormatV3{Props: props}}}
	}
	tests := []struct {
		name    string
		w       *ast.WidgetV3
		wantSub string // "" = no violations expected
	}{
		{"valid decimal", dtWidget(fmtBlock(
			ast.ParamFormatProp{Key: "decimalprecision", Value: "2"},
			ast.ParamFormatProp{Key: "groupdigits", Value: "true"}), nil), ""},
		{"unknown key", dtWidget(fmtBlock(
			ast.ParamFormatProp{Key: "decimalprecison", Value: "2"}), nil), "unknown format key"},
		{"bad decimal", dtWidget(fmtBlock(
			ast.ParamFormatProp{Key: "decimalprecision", Value: "x"}), nil), "non-negative integer"},
		{"bad dateformat", dtWidget(fmtBlock(
			ast.ParamFormatProp{Key: "dateformat", Value: "Nope"}), nil), "dateFormat must be one of"},
		{"bad enumformat", dtWidget(fmtBlock(
			ast.ParamFormatProp{Key: "enumformat", Value: "Nope"}), nil), "enumFormat must be"},
		{"custom without Custom", dtWidget(fmtBlock(
			ast.ParamFormatProp{Key: "customdateformat", Value: "yyyy"}), nil), "requires `dateFormat: Custom`"},
		// Studio Pro keeps the pattern when the format is switched away from
		// Custom, and describe prints both: refusing it failed check on
		// describe's own output for TestApp's WorkflowCommons snippets (#968).
		{"pattern beside explicit DateTime", dtWidget(fmtBlock(
			ast.ParamFormatProp{Key: "dateformat", Value: "DateTime"},
			ast.ParamFormatProp{Key: "customdateformat", Value: "MM/dd/yyyy . hh:mma"}), nil), ""},
		{"widget-level format key", dtWidget(nil, map[string]any{"decimalPrecision": 2}), "per-parameter format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateDynamicTextFormatting(tt.w, "page P")
			if tt.wantSub == "" {
				if len(got) != 0 {
					t.Fatalf("expected no violations, got %d: %+v", len(got), got)
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("expected a violation containing %q, got none", tt.wantSub)
			}
			found := false
			for _, v := range got {
				if v.RuleID == "MDL-WIDGET18" && strings.Contains(v.Message, tt.wantSub) {
					found = true
				}
			}
			if !found {
				t.Errorf("no MDL-WIDGET18 with %q; got %+v", tt.wantSub, got)
			}
		})
	}
}

func TestFormatParamFormatSuffix(t *testing.T) {
	// Default FormattingInfo → empty suffix (unformatted params round-trip as before).
	def := map[string]any{"FormattingInfo": map[string]any{
		"DecimalPrecision": int64(2), "GroupDigits": false, "DateFormat": "Date", "EnumFormat": "Text", "CustomDateFormat": "",
	}}
	if s := formatParamFormatSuffix(nil, def); s != "" {
		t.Errorf("default FormattingInfo must yield empty suffix, got %q", s)
	}
	non := map[string]any{"FormattingInfo": map[string]any{
		"DecimalPrecision": int64(4), "GroupDigits": true, "DateFormat": "DateTime", "EnumFormat": "Text", "CustomDateFormat": "",
	}}
	s := formatParamFormatSuffix(nil, non)
	for _, want := range []string{" format (", "decimalPrecision: 4", "groupDigits: true", "dateFormat: DateTime"} {
		if !strings.Contains(s, want) {
			t.Errorf("suffix %q missing %q", s, want)
		}
	}
}
