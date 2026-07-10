// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// Issue #661 — CREATE PAGE header carries pop-up dimensions.
func TestCreatePage_PopupHeaderProperties(t *testing.T) {
	input := `CREATE PAGE M.P (
		Title: 'Popup',
		Layout: Atlas_Core.PopupLayout,
		PopupWidth: 800,
		PopupHeight: 480,
		PopupResizable: true
	) {
		CONTAINER c { DYNAMICTEXT t (Content: 'x') }
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("expected CreatePageStmtV3, got %T", prog.Statements[0])
	}
	if stmt.PopupWidth == nil || *stmt.PopupWidth != 800 {
		t.Errorf("PopupWidth = %v, want 800", stmt.PopupWidth)
	}
	if stmt.PopupHeight == nil || *stmt.PopupHeight != 480 {
		t.Errorf("PopupHeight = %v, want 480", stmt.PopupHeight)
	}
	if stmt.PopupResizable == nil || *stmt.PopupResizable != true {
		t.Errorf("PopupResizable = %v, want true", stmt.PopupResizable)
	}
}

// PopupWidth: 0 is valid — 0 is Studio Pro's default (auto-size), so it must parse
// and carry through as an explicit 0, not be rejected (issue #713).
func TestCreatePage_PopupWidthZero(t *testing.T) {
	prog, errs := Build(`CREATE PAGE M.P (PopupWidth: 0, PopupHeight: 0) { CONTAINER c { DYNAMICTEXT t (Content: 'x') } };`)
	if len(errs) > 0 {
		t.Fatalf("PopupWidth/Height: 0 should parse cleanly, got: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	if stmt.PopupWidth == nil || *stmt.PopupWidth != 0 {
		t.Errorf("PopupWidth = %v, want explicit 0", stmt.PopupWidth)
	}
	if stmt.PopupHeight == nil || *stmt.PopupHeight != 0 {
		t.Errorf("PopupHeight = %v, want explicit 0", stmt.PopupHeight)
	}
}

// Absent pop-up properties stay nil so the executor applies the Mendix defaults.
func TestCreatePage_PopupHeaderOmitted(t *testing.T) {
	prog, errs := Build(`CREATE PAGE M.P (Title: 'x') { CONTAINER c { DYNAMICTEXT t (Content: 'x') } };`)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	if stmt.PopupWidth != nil || stmt.PopupHeight != nil || stmt.PopupResizable != nil {
		t.Errorf("expected nil pop-up fields when omitted, got %v/%v/%v",
			stmt.PopupWidth, stmt.PopupHeight, stmt.PopupResizable)
	}
}

func TestCreatePage_PopupHeaderErrors(t *testing.T) {
	cases := []struct {
		name, input, wantSubstr string
	}{
		{"unknown property", `CREATE PAGE M.P (Bogus: 5) { CONTAINER c { DYNAMICTEXT t (Content: 'x') } };`, "unknown page property"},
		{"non-bool resizable", `CREATE PAGE M.P (PopupResizable: 5) { CONTAINER c { DYNAMICTEXT t (Content: 'x') } };`, "PopupResizable must be true or false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, errs := Build(c.input)
			if len(errs) == 0 {
				t.Fatalf("expected an error, got none")
			}
			joined := ""
			for _, e := range errs {
				joined += e.Error() + "\n"
			}
			if !strings.Contains(joined, c.wantSubstr) {
				t.Errorf("error %q does not contain %q", joined, c.wantSubstr)
			}
		})
	}
}
