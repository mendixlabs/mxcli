// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1206: before Mendix 10.1 a page's or snippet's own parameter
// supplies an argument the way an enclosing data container does. Measured with
// one script (a snippet and a page each with an $Order parameter; a nanoflow
// data source and a nanoflow button action, both without an argument), one
// project per version:
//
//	9.24.30, 10.0.0                            → 0 errors
//	10.1.0, 10.2.0, 10.3.1, 10.6.25, 10.12.19,
//	10.24.24, 11.12.6                          → CE1571 on all three
//
// check applied the 10.1+ rule to every project, so a 9.x / 10.0 page that
// builds was refused.
func TestFlowArgs_ParameterSuppliesTheArgumentBefore10_1(t *testing.T) {
	widgets := []*ast.WidgetV3{dsWidget("dgParamScope", "DS.DS_Rows")}
	params := []ast.PageParameter{
		{Name: "Ctx", EntityType: ast.QualifiedName{Module: "DS147", Name: "ReportContext"}},
	}
	if errs := validateFlowArgumentsScoped(params, widgets, f147Sigs(), sameName, true); len(errs) != 0 {
		t.Errorf("before 10.1 a parameter of the right type supplies the argument (0 errors measured); got %v", errs)
	}
	// Control: from 10.1 the same page is CE1571, as before.
	if errs := validateFlowArgumentsScoped(params, widgets, f147Sigs(), sameName, false); len(errs) != 1 {
		t.Errorf("from 10.1 the parameter alone does not supply it (CE1571 measured); got %v", errs)
	}
	// A parameter of ANOTHER type supplies nothing in either version.
	other := []ast.PageParameter{{Name: "Row", EntityType: ast.QualifiedName{Module: "DS147", Name: "Row"}}}
	if errs := validateFlowArgumentsScoped(other, widgets, f147Sigs(), sameName, true); len(errs) != 1 ||
		!strings.Contains(errs[0], "CE1571") {
		t.Errorf("a parameter of a different entity must not supply it; got %v", errs)
	}
}

func TestParamsSupplyFlowArguments(t *testing.T) {
	at := func(maj, min int) func(int, int) bool {
		return func(a, b int) bool { return maj > a || (maj == a && min >= b) }
	}
	for _, c := range []struct {
		name    string
		atLeast func(int, int) bool
		want    bool
	}{
		{"9.24", at(9, 24), true}, {"10.0", at(10, 0), true}, {"10.1", at(10, 1), false},
		{"11.12", at(11, 12), false}, {"unknown", nil, false},
	} {
		if got := paramsSupplyFlowArguments(c.atLeast); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
