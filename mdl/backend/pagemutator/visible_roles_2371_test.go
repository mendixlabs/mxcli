// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// ako/mxcli#2371: `alter page … set (visible: roles (A, B)) on c` stored the
// roles as the visibility expression; mx check reported CE0117. The roles are
// "Visible for module roles": ConditionalVisibilitySettings.ModuleRoles, a
// by-name list Studio Pro writes with marker 1.
func TestSetWidgetProperty_VisibleRoles_Issue2371(t *testing.T) {
	for name, stored := range map[string]any{
		"no settings yet": nil,
		"condition kept": bson.D{
			{Key: "$ID", Value: "x"},
			{Key: "$Type", Value: "Forms$ConditionalVisibilitySettings"},
			{Key: "Attribute", Value: ""},
			{Key: "Conditions", Value: bson.A{int32(2)}},
			{Key: "Expression", Value: "$currentObject/Active"},
			{Key: "IgnoreSecurity", Value: false},
			{Key: "ModuleRoles", Value: bson.A{int32(1), "M.Old"}},
			{Key: "SourceVariable", Value: nil},
		},
	} {
		w := bson.D{
			{Key: "$Type", Value: "Forms$DivContainer"},
			{Key: "Name", Value: "ctn"},
			{Key: "ConditionalVisibilitySettings", Value: stored},
		}
		rawData := makeRawPage(w)
		m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("ctn", "VisibleRoles", []string{"M.Admin", "M.User"}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		cvs, ok := bsonnav.DGet(findBsonWidget(rawData, "ctn").widget, "ConditionalVisibilitySettings").(bson.D)
		if !ok {
			t.Fatalf("%s: no ConditionalVisibilitySettings written", name)
		}
		roles, _ := bsonnav.DGet(cvs, "ModuleRoles").(bson.A)
		if len(roles) != 3 || roles[0] != int32(1) || roles[1] != "M.Admin" || roles[2] != "M.User" {
			t.Errorf("%s: ModuleRoles = %v, want [1 M.Admin M.User]", name, roles)
		}
		wantExpr := ""
		if stored != nil {
			wantExpr = "$currentObject/Active"
		}
		if got := bsonnav.DGetString(cvs, "Expression"); got != wantExpr {
			t.Errorf("%s: Expression = %q, want %q (the roles are not an expression)", name, got, wantExpr)
		}
	}
}
