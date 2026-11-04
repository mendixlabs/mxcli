// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#834: a hop whose association does not touch the entity it starts
// from was written when the author named its destination. `$Order/Mod.Account_Flag/Mod.Flag`
// — Account_Flag joins Account and Flag, not Order — passed every check the
// builder made (the association exists; Flag is one of its ends) and stored an
// EntityRefStep Mendix cannot follow from an Order.
//
// The control is the same hop from the entity it does start at.
func TestAssociationHop_RefusesAssociationOffTheContextEntity(t *testing.T) {
	for _, c := range []struct {
		name    string
		ds      *ast.DataSourceV3
		refused bool
	}{
		{"association source, first hop off the context", &ast.DataSourceV3{Type: "association", ContextVariable: "Order", Reference: "Mod.Account_Flag/Mod.Flag"}, true},
		{"association source, control", &ast.DataSourceV3{Type: "association", ContextVariable: "Order", Reference: "Mod.Order_Account/Mod.Account"}, false},
		{"association source, second hop off its entity", &ast.DataSourceV3{Type: "association", ContextVariable: "Order", Reference: "Mod.Order_Account/Mod.Account/Mod.Order_Account/Mod.Order"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := assocHopsPB().buildDataSourceV3(c.ds)
			assertHopRefusal(t, err, c.refused, c.ds.Reference)
		})
	}
	for _, c := range []struct {
		name    string
		path    string
		refused bool
	}{
		{"database over association, first hop off the context", "Mod.Account_Flag/Mod.Flag", true},
		{"database over association, control", "Mod.Order_Account/Mod.Account", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := assocHopsPB().buildDatabaseOverAssociationV3(&ast.DataSourceV3{
				Type: "database", ContextVariable: "Order", AssociationPath: c.path,
			})
			assertHopRefusal(t, err, c.refused, c.path)
		})
	}
}

func assertHopRefusal(t *testing.T, err error, refused bool, path string) {
	t.Helper()
	switch {
	case refused && err == nil:
		t.Fatalf("%s from an Order succeeded; Mod.Account_Flag does not touch Mod.Order", path)
	case refused && !strings.Contains(err.Error(), "Mod.Order"):
		t.Errorf("error %q does not name the entity the hop starts from", err)
	case !refused && err != nil:
		t.Fatalf("%s: %v", path, err)
	}
}

// The same refusal at check time (MDL-ASSOCDS03), so `mxcli check -p` reports
// what exec would refuse. viewAssocCtx's project: Item_Customer joins Item and
// Customer, Stats_Customer joins Stats and Customer.
func TestCheckAssocHopStart(t *testing.T) {
	ctx := viewAssocCtx(t)
	for _, tc := range []struct {
		name, script, param, ds string
		reported                bool
	}{
		{"association source off the context", "", "Item", "$currentObject/C1611.Stats_Customer/C1611.Stats", true},
		{"database over association off the context", "", "Item", "DATABASE FROM $currentObject/C1611.Stats_Customer/C1611.Stats", true},
		{"unqualified, off the context", "", "Item", "$currentObject/Stats_Customer/C1611.Stats", true},
		// controls
		{"from an end", "", "Item", "$currentObject/C1611.Item_Customer/C1611.Customer", false},
		{"from the other end", "", "Customer", "DATABASE FROM $currentObject/C1611.Item_Customer/C1611.Item", false},
		{"from a specialization the script declares", "create persistent entity C1611.Special extends C1611.Item (X: String(10));\n",
			"Special", "$currentObject/C1611.Item_Customer/C1611.Customer", false},
		{"unknown association is not judged here", "", "Item", "$currentObject/C1611.Nope/C1611.Customer", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits []string
			for _, e := range assocListErrs(t, ctx, tc.script+viewAssocPage(tc.param, "dataview", tc.ds)) {
				if strings.Contains(e, "MDL-ASSOCDS03") {
					hits = append(hits, e)
				}
			}
			if got := len(hits) > 0; got != tc.reported {
				t.Errorf("MDL-ASSOCDS03 reported = %v, want %v: %q", got, tc.reported, hits)
			}
		})
	}
}
