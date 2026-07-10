// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// Issue #714 — CREATE PAGE header carries a page-level CSS Class and Style.
func TestCreatePage_ClassAndStyle(t *testing.T) {
	prog, errs := Build(`CREATE PAGE M.P (
		Title: 'x',
		Layout: Atlas_Core.Atlas_Default,
		Class: 'my-page bg-primary',
		Style: 'padding: 10px'
	) { CONTAINER c { DYNAMICTEXT t (Content: 'x') } };`)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	if stmt.Class != "my-page bg-primary" {
		t.Errorf("Class = %q, want 'my-page bg-primary'", stmt.Class)
	}
	if stmt.Style != "padding: 10px" {
		t.Errorf("Style = %q, want 'padding: 10px'", stmt.Style)
	}
}

// Class/Style are optional — omitting them leaves the fields empty.
func TestCreatePage_ClassOmitted(t *testing.T) {
	prog, errs := Build(`CREATE PAGE M.P (Title: 'x') { CONTAINER c { DYNAMICTEXT t (Content: 'x') } };`)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	if stmt.Class != "" || stmt.Style != "" {
		t.Errorf("expected empty Class/Style, got %q/%q", stmt.Class, stmt.Style)
	}
}
