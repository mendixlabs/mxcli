// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A layout's `placeholder X { … }` is dropped from the widget tree — it is the
// page-side spelling. Recording the names is what lets `mxcli check` explain
// that, instead of the write failing later with a message that contradicts the
// script (mendixlabs/mxcli#1063).
func TestBuildLayoutV3_RecordsBracedPlaceholders(t *testing.T) {
	src := `create layout Mod.App_X (layouttype: 'Responsive') {
  scrollcontainer layoutContainer { region center { placeholder Main { } } }
}`
	l := buildOneLayout(t, src)
	if len(l.BracedPlaceholders) != 1 || l.BracedPlaceholders[0] != "Main" {
		t.Fatalf("BracedPlaceholders = %v, want [Main]", l.BracedPlaceholders)
	}
	// It must STILL be absent from the widget tree: recording it is a diagnostic,
	// not a decision to start honouring the braced form.
	if n := countPlaceholders(l.Widgets); n != 0 {
		t.Errorf("a braced placeholder became %d real placeholder widget(s)", n)
	}
}

// CONTROL: the bodiless form is the one that works, and must be untouched —
// no recording, and a real placeholder widget in the tree.
func TestBuildLayoutV3_BodilessFormIsUnaffected(t *testing.T) {
	src := `create layout Mod.App_X (layouttype: 'Responsive') {
  scrollcontainer layoutContainer { region center { placeholder Main } }
}`
	l := buildOneLayout(t, src)
	if len(l.BracedPlaceholders) != 0 {
		t.Errorf("a bodiless placeholder was recorded as braced: %v", l.BracedPlaceholders)
	}
	if n := countPlaceholders(l.Widgets); n != 1 {
		t.Errorf("got %d placeholder widgets, want 1", n)
	}
}

// CONTROL: a PAGE's braced placeholder fills a layout slot and is correct. The
// layout-only recording must not reach it — pages are built by the same body
// function, which is why this is worth pinning.
func TestBuildPageV3_BracedPlaceholderStillFillsASlot(t *testing.T) {
	src := `create page Mod.P (Title: 'T') {
  placeholder Content { textbox tb (Caption: 'x') }
}`
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	page, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("got %T, want *ast.CreatePageStmtV3", prog.Statements[0])
	}
	if len(page.Placeholders) != 1 || page.Placeholders[0].Name != "Content" {
		t.Fatalf("page placeholders = %+v, want one named Content", page.Placeholders)
	}
	if len(page.Placeholders[0].Widgets) != 1 {
		t.Errorf("the placeholder's widgets were dropped: %+v", page.Placeholders[0])
	}
}

func buildOneLayout(t *testing.T, src string) *ast.CreateLayoutStmt {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	l, ok := prog.Statements[0].(*ast.CreateLayoutStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.CreateLayoutStmt", prog.Statements[0])
	}
	return l
}

func countPlaceholders(ws []*ast.WidgetV3) int {
	n := 0
	for _, w := range ws {
		if w.Type == "placeholder" {
			n++
		}
		n += countPlaceholders(w.Children)
	}
	return n
}
