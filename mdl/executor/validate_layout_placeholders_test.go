// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A layout's placeholder structure is a RULE mxbuild validates, not the naming
// convention mxcli's docs claimed (mendixlabs/mxcli#1063). Measured on 11.12.1
// against a layout no page uses, so none of this depends on a page binding:
//
//	placeholder Content            → CE0848 "No placeholder with the name 'Main'
//	                                 found. There should be exactly one."
//	placeholder Main               → 0 errors
//	placeholder Main + Content     → 0 errors  (extra placeholders are fine)
//	placeholder Main + Main        → CE0849 + CE0495
//	placeholder Main + Side + Side → CE0495 "Duplicate name 'Side'."
//
// Before this rule, every one of those passed `mxcli check` AND `mxcli exec`,
// and the author found out a whole build later.
func TestValidateLayoutPlaceholders(t *testing.T) {
	cases := []struct {
		name       string
		names      []string
		wantRule   string
		wantPhrase string
	}{{
		name:  "exactly one Main is the shape that builds",
		names: []string{"Main"},
	}, {
		name:  "extra placeholders alongside Main are fine",
		names: []string{"Main", "Content", "Side"},
	}, {
		// The reported case.
		name:       "no Main at all",
		names:      []string{"Content"},
		wantRule:   "MDL081",
		wantPhrase: "CE0848",
	}, {
		name:       "two Mains",
		names:      []string{"Main", "Main"},
		wantRule:   "MDL081",
		wantPhrase: "CE0849",
	}, {
		// Uniqueness is general, not Main-specific — CE0495 fires on any repeat.
		name:       "duplicate non-Main name",
		names:      []string{"Main", "Side", "Side"},
		wantRule:   "MDL082",
		wantPhrase: "Side",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateLayoutPlaceholders(layoutWithPlaceholders(tc.names...))
			if tc.wantRule == "" {
				if len(got) != 0 {
					t.Fatalf("a layout that builds was flagged: %+v", got)
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("names %v produced no violation", tc.names)
			}
			if got[0].RuleID != tc.wantRule {
				t.Errorf("RuleID = %q, want %q", got[0].RuleID, tc.wantRule)
			}
			if !strings.Contains(got[0].Message, tc.wantPhrase) {
				t.Errorf("message %q does not mention %q", got[0].Message, tc.wantPhrase)
			}
			// The message has to name the layout, because a script that creates
			// several is the normal case and mxbuild's own error names it.
			if !strings.Contains(got[0].Message, "Mod.App_X") {
				t.Errorf("message %q does not name the layout", got[0].Message)
			}
		})
	}
}

// A layout with NO placeholder at all was already refused, but only at write
// time. It must now be reported by check, with the same rule as a missing Main —
// it IS a missing Main.
func TestValidateLayoutPlaceholders_NoneAtAll(t *testing.T) {
	got := validateLayoutPlaceholders(layoutWithPlaceholders())
	if len(got) == 0 || got[0].RuleID != "MDL081" {
		t.Fatalf("a layout with no placeholder was not flagged as MDL081: %+v", got)
	}
}

// CONTROL: the rule must not fire on a PAGE. A page's `placeholder X { … }`
// blocks FILL a layout's slots — they do not declare them, and a page naming one
// `Content` is correct and common.
func TestValidateLayoutPlaceholders_IgnoresPages(t *testing.T) {
	page := &ast.CreatePageStmtV3{
		Name:         ast.QualifiedName{Module: "Mod", Name: "P"},
		Placeholders: []*ast.PagePlaceholderV3{{Name: "Content"}},
	}
	if got := validateLayoutPlaceholders(page); len(got) != 0 {
		t.Errorf("a page was flagged by a layout rule: %+v", got)
	}
}

// layoutWithPlaceholders builds a layout whose placeholders sit one region deep,
// which is where a real layout puts them — the walk must not stop at the top
// level of the body.
func layoutWithPlaceholders(names ...string) *ast.CreateLayoutStmt {
	region := &ast.WidgetV3{Type: "region", Name: "center"}
	for _, n := range names {
		region.Children = append(region.Children, &ast.WidgetV3{Type: "placeholder", Name: n})
	}
	return &ast.CreateLayoutStmt{
		Name: ast.QualifiedName{Module: "Mod", Name: "App_X"},
		Widgets: []*ast.WidgetV3{{
			Type:     "scrollcontainer",
			Name:     "layoutContainer",
			Children: []*ast.WidgetV3{region},
		}},
	}
}

// A layout DECLARES a placeholder with a bare `placeholder Main`; the braced
// form `placeholder Main { … }` is the PAGE-side spelling, which fills a slot
// rather than declaring one. The visitor drops the braced form from a layout's
// widget tree, so the reported script (mendixlabs/mxcli#1063) passed check,
// reached exec, created the module, and only then failed with "declares no
// placeholder" — a message that contradicts what the author had written.
//
// Reaching for page syntax here is the obvious mistake: every other widget in a
// layout takes a body, and `alter page` uses the braced form for real.
func TestValidateLayoutPlaceholders_BracedFormIsRejected(t *testing.T) {
	stmt := layoutWithPlaceholders("Main")
	stmt.BracedPlaceholders = []string{"Main"}

	got := validateLayoutPlaceholders(stmt)
	if len(got) == 0 {
		t.Fatal("a braced placeholder in a layout was accepted")
	}
	if got[0].RuleID != "MDL083" {
		t.Errorf("RuleID = %q, want MDL083", got[0].RuleID)
	}
	if !strings.Contains(got[0].Message, "Main") {
		t.Errorf("message %q does not name the placeholder", got[0].Message)
	}
	// The remedy is the whole point: the author wrote something that looks
	// right, so the message has to show the form that works.
	if !strings.Contains(got[0].Suggestion, "placeholder Main") {
		t.Errorf("suggestion %q does not show the bodiless form", got[0].Suggestion)
	}
}
