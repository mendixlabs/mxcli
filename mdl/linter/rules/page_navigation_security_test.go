// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

func TestCollectMenuPages_Flat(t *testing.T) {
	items := []*types.NavMenuItem{
		{Page: "MyModule.HomePage", Caption: "Home"},
		{Page: "MyModule.About", Caption: "About"},
	}

	navPages := make(map[string][]navUsage)
	collectMenuPages(items, "Responsive", navPages)

	if len(navPages) != 2 {
		t.Errorf("expected 2 pages, got %d", len(navPages))
	}
	if usages, ok := navPages["MyModule.HomePage"]; !ok || len(usages) != 1 {
		t.Errorf("expected 1 usage for HomePage")
	}
}

func TestCollectMenuPages_Nested(t *testing.T) {
	items := []*types.NavMenuItem{
		{
			Caption: "Parent",
			Items: []*types.NavMenuItem{
				{Page: "MyModule.ChildPage", Caption: "Child"},
			},
		},
	}

	navPages := make(map[string][]navUsage)
	collectMenuPages(items, "Responsive", navPages)

	if _, ok := navPages["MyModule.ChildPage"]; !ok {
		t.Error("expected nested page to be collected")
	}
}

func TestCollectMenuPages_EmptyPage(t *testing.T) {
	items := []*types.NavMenuItem{
		{Page: "", Caption: "Separator"},
	}

	navPages := make(map[string][]navUsage)
	collectMenuPages(items, "Responsive", navPages)

	if len(navPages) != 0 {
		t.Errorf("expected 0 pages for empty page ref, got %d", len(navPages))
	}
}

func TestModuleFromQualified(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"MyModule.Page", "MyModule"},
		{"Admin.Login", "Admin"},
		{"NoModule", "NoModule"},
		{"", ""},
	}
	for _, tt := range tests {
		got := moduleFromQualified(tt.input)
		if got != tt.want {
			t.Errorf("moduleFromQualified(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPageNavigationSecurityRule_NilReader(t *testing.T) {
	r := NewPageNavigationSecurityRule()
	ctx := linter.NewLintContextFromDB(nil)
	violations := r.Check(ctx)
	if violations != nil {
		t.Errorf("expected nil with nil reader, got %v", violations)
	}
}

func TestPageNavigationSecurityRule_Metadata(t *testing.T) {
	r := NewPageNavigationSecurityRule()
	if r.ID() != "MPR007" {
		t.Errorf("ID = %q, want MPR007", r.ID())
	}
	if r.Category() != "security" {
		t.Errorf("Category = %q, want security", r.Category())
	}
}
