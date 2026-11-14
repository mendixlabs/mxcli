// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2253: an audit field declared with a pseudo-type
// (`ChangedDate: AutoChangedDate`) is the system member, not an attribute of
// that name. In a widget expression the declared name passed check and exec,
// and mx check reported CE0117. Measured with mx check on 11.12.6, one
// container per spelling:
//
//	$currentObject/ChangedDate  CE0117    $currentObject/changedDate       clean
//	$currentObject/CreatedDate  CE0117    $currentObject/createdDate       clean
//	$currentObject/Owner        CE0117    $currentObject/System.owner      clean
//	$currentObject/ChangedBy    CE0117    $currentObject/System.changedBy  clean
func TestExprSystemMemberSpelling(t *testing.T) {
	decl := &ast.CreateEntityStmt{Attributes: []ast.Attribute{
		{Name: "Sku", Type: ast.DataType{Kind: ast.TypeString}},
		{Name: "LastChanged", Type: ast.DataType{Kind: ast.TypeAutoChangedDate}},
		{Name: "Owner", Type: ast.DataType{Kind: ast.TypeAutoOwner}},
	}}
	for _, tc := range []struct {
		expr, want string
	}{
		{"$currentObject/LastChanged != empty", "$currentObject/changedDate"},
		{"$currentObject/Owner != empty", "$currentObject/System.owner"},
		// controls: the system spellings and an ordinary attribute
		{"$currentObject/changedDate != empty", ""},
		{"$currentObject/System.owner != empty", ""},
		{"$currentObject/Sku != empty", ""},
	} {
		got := exprSystemMemberErrors("dynamictext t1", map[string]any{"Visible": tc.expr}, "M.Product", decl, nil)
		switch {
		case tc.want == "" && len(got) > 0:
			t.Errorf("%s: reported %v", tc.expr, got)
		case tc.want != "" && (len(got) != 1 || !strings.Contains(got[0], tc.want) || !strings.Contains(got[0], "CE0117")):
			t.Errorf("%s: got %v, want one CE0117 error naming %s", tc.expr, got, tc.want)
		}
	}

	// A stored entity keeps no name for the field; describe prints the four
	// conventional ones, which are refused when no attribute has that name.
	stored := map[string]bool{"sku": true}
	if got := exprSystemMemberErrors("c1", map[string]any{"Visible": "$currentObject/ChangedDate != empty"}, "M.Product", nil, stored); len(got) != 1 || !strings.Contains(got[0], "changedDate") {
		t.Errorf("stored entity: got %v", got)
	}
	// Control: a real attribute of that name (a non-persistent entity may have one).
	stored["changeddate"] = true
	if got := exprSystemMemberErrors("c1", map[string]any{"Visible": "$currentObject/ChangedDate != empty"}, "M.Product", nil, stored); len(got) != 0 {
		t.Errorf("a real ChangedDate attribute was refused: %v", got)
	}
}
