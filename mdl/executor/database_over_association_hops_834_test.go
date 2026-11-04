// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#834: `database from $ctx/Assoc/Entity …` (the List View / pluggable
// database source over associations, ako/mxcli#721 L5) read its path two
// segments at a time, association then destination. The association source had
// been taught every spelling of a multi-hop path (mendixlabs/mxcli#1467), this
// builder had not: the short form `A1/A2` read A2 as an entity and was refused
// ("entity … does not exist"), and `A1/A2/E2` stored A2 as the destination of
// A1 — so the two builders disagreed about the same path.
//
// Both now resolve hops with one function, so every spelling the association
// source accepts builds the same steps here.
func TestDatabaseOverAssociation_MultiHopSpellings(t *testing.T) {
	want := []pages.AttributeRefStep{
		{Association: "Mod.Order_Account", DestinationEntity: "Mod.Account"},
		{Association: "Mod.Account_Flag", DestinationEntity: "Mod.Flag"},
	}
	for _, path := range []string{
		"Mod.Order_Account/Mod.Account/Mod.Account_Flag/Mod.Flag", // full form (describe's output) — the control
		"Mod.Order_Account/Mod.Account_Flag",                      // short form
		"Mod.Order_Account/Mod.Account_Flag/Mod.Flag",             // first destination derived
		"Mod.Order_Account/Mod.Account/Mod.Account_Flag",          // last destination derived
	} {
		t.Run(path, func(t *testing.T) {
			ds, entity, err := assocHopsPB().buildDatabaseOverAssociationV3(&ast.DataSourceV3{
				Type: "database", ContextVariable: "Order", AssociationPath: path,
			})
			if err != nil {
				t.Fatalf("database from $Order/%s: %v", path, err)
			}
			src := ds.(*pages.DatabaseSource)
			if len(src.EntitySteps) != len(want) {
				t.Fatalf("steps = %+v, want %+v", src.EntitySteps, want)
			}
			for i := range want {
				if src.EntitySteps[i].Association != want[i].Association || src.EntitySteps[i].DestinationEntity != want[i].DestinationEntity {
					t.Errorf("step %d = %+v, want %+v", i, src.EntitySteps[i], want[i])
				}
			}
			if entity != "Mod.Flag" {
				t.Errorf("entity = %q, want Mod.Flag", entity)
			}
		})
	}
}

// A destination that exists but is not an end of its association was written
// as stated — a step naming an entity the hop does not arrive at.
func TestDatabaseOverAssociation_RefusesDestinationOffTheAssociation(t *testing.T) {
	const path = "Mod.Order_Account/Mod.Flag"
	_, _, err := assocHopsPB().buildDatabaseOverAssociationV3(&ast.DataSourceV3{
		Type: "database", ContextVariable: "Order", AssociationPath: path,
	})
	if err == nil {
		t.Fatalf("database from $Order/%s succeeded; Mod.Flag is not an end of Mod.Order_Account", path)
	}
	if !strings.Contains(err.Error(), "Mod.Flag") {
		t.Errorf("error %q does not name the destination", err)
	}
}
