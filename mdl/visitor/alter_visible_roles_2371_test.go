// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2371: the alter form of `visible: roles (…)` arrived at the
// executor as the expression text "roles (…)".
func TestAlterSetVisibleRoles_Issue2371(t *testing.T) {
	for src, want := range map[string]map[string]any{
		"alter page M.P { set ( visible: roles (M.Admin, M.User) ) on c };": {
			"VisibleRoles": []string{"M.Admin", "M.User"},
		},
		"alter page M.P { set ( visible: roles (M.Admin) and $currentObject/A = 1 ) on c };": {
			"VisibleRoles": []string{"M.Admin"},
			"VisibleIf":    "$currentObject/A = 1",
		},
		// CONTROL: a plain condition stays a condition.
		"alter page M.P { set ( visible: $currentObject/A = 1 ) on c };": {
			"VisibleIf": "$currentObject/A = 1",
		},
	} {
		prog, errs := Build(src)
		if len(errs) > 0 {
			t.Fatalf("%s: %v", src, errs)
		}
		op := prog.Statements[0].(*ast.AlterPageStmt).Operations[0].(*ast.SetPropertyOp)
		if !reflect.DeepEqual(op.Properties, want) {
			t.Errorf("%s:\n got %#v\nwant %#v", src, op.Properties, want)
		}
	}
}
