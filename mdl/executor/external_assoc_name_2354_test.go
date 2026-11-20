// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#2354: `create external entities` named Product's navigation
// property `Category` as the association `Category_2` — the nav name collides
// with the entity Category, and the fallback was a numeric suffix. A colliding
// name now takes the Mendix convention, `<Parent>_<NavProperty>`.
func TestUniqueAssocName_Issue2354(t *testing.T) {
	dm := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{{Name: "Product"}, {Name: "Category"}}}
	none := map[assocKey]bool{}

	if got := uniqueAssocName("Category", "Product", dm, none); got != "Product_Category" {
		t.Errorf("nav Category on Product = %q, want Product_Category", got)
	}
	// CONTROL: a nav name that collides with nothing is kept as it is.
	if got := uniqueAssocName("Supplier", "Product", dm, none); got != "Supplier" {
		t.Errorf("nav Supplier = %q, want Supplier", got)
	}
	// Two entities with the same nav name: the second is told apart by its
	// parent, not by a counter.
	taken := map[assocKey]bool{{parent: "Product", name: "season"}: true}
	if got := uniqueAssocName("season", "Race", dm, taken); got != "Race_season" {
		t.Errorf("second season = %q, want Race_season", got)
	}
	// Only when the parent-qualified name is taken too does a number appear.
	taken = map[assocKey]bool{{parent: "Product", name: "Product_Category"}: true}
	if got := uniqueAssocName("Category", "Product", dm, taken); got != "Product_Category_2" {
		t.Errorf("both taken = %q, want Product_Category_2", got)
	}
}
