// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#1862 (mendixlabs/mxcli#1704): a Data grid 2 `emptyplaceholder { … }`
// block was accepted by check and exec, and the page then failed mx check with
// CE0463 on the grid (measured on TestApp, 11.14). The slot is hidden while
// showEmptyPlaceholder is "none", its default, and a hidden slot holding widgets
// is a definition mismatch. Studio Pro's own grid with a placeholder
// (WorkflowCommons.ConflictResolution) stores showEmptyPlaceholder "custom", and
// a script that says so checks clean.
//
// MDL-WIDGET10 never saw it: a slot is filled by a child block, not by a
// `key: value` property, so it never counted as set. Gallery shares the slot
// and the rule; the rule is supplied here as the installed .mpk supplies it.

func galleryWithPlaceholderRule(t *testing.T) *WidgetRegistry {
	t.Helper()
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	real, ok := reg.Get("GALLERY")
	if !ok {
		t.Fatal("no GALLERY definition")
	}
	def := *real
	def.PropertyVisibility = []types.WidgetVisibilityRule{{
		PropertyKey: "emptyPlaceholder",
		HiddenWhen:  &types.WidgetVisibilityCondition{PropertyKey: "showEmptyPlaceholder", Operator: "eq", Value: "none"},
	}}
	return &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{"GALLERY": &def}}
}

func TestCheck1862_PlaceholderUnderNoneIsRefused(t *testing.T) {
	reg := galleryWithPlaceholderRule(t)
	w := parseSnippetWidget(t, `gallery g1 (DataSource: database from M.E) {
  emptyplaceholder { dynamictext t1 (Content: 'No items found.') }
}`)
	v := validateWidgetVisibility(w, reg, "page P")
	if len(v) != 1 || v[0].RuleID != "MDL-WIDGET10" {
		t.Fatalf("got %d violation(s) %+v, want 1 MDL-WIDGET10 for the hidden slot", len(v), v)
	}
	if v[0].Severity.String() != "error" {
		t.Errorf("severity = %v, want error: it is CE0463 at mx check", v[0].Severity)
	}
}

// Controls: showEmptyPlaceholder 'custom' shows the slot, and an empty grid
// with no placeholder block sets nothing.
func TestCheck1862_PlaceholderUnderCustomAndNoPlaceholderAreClean(t *testing.T) {
	reg := galleryWithPlaceholderRule(t)
	for name, src := range map[string]string{
		"custom": `gallery g1 (DataSource: database from M.E, showEmptyPlaceholder: 'custom') {
  emptyplaceholder { dynamictext t1 (Content: 'No items found.') }
}`,
		"no placeholder": `gallery g1 (DataSource: database from M.E)`,
	} {
		if v := validateWidgetVisibility(parseSnippetWidget(t, src), reg, "page P"); len(v) != 0 {
			t.Errorf("%s: got %+v, want no violation", name, v)
		}
	}
}
