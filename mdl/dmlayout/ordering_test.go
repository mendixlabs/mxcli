// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// catalog is ako/PizzaV1's Catalog module: its entities with their attribute
// counts, its associations, and the cross-module lines drawn at them (from
// Shop and Fulfilment). The model where the ordering search was measured.
func catalog() (*domainmodel.DomainModel, CrossLines) {
	attrs := func(name string, n int) *domainmodel.Entity {
		return ent(name, strings.Split(strings.Repeat("a,", n), ",")[:n]...)
	}
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			attrs("Allergen", 4), attrs("Category", 5), attrs("Ingredient", 6), attrs("Option", 16),
			attrs("Product", 46), attrs("ProductSize", 8), attrs("RecipeItem", 3),
		},
		Associations: []*domainmodel.Association{
			named("Ingredient_Allergen", "Ingredient", "Allergen"),
			named("Option_Ingredient", "Option", "Ingredient"),
			named("Product_Allergen", "Product", "Allergen"),
			named("Product_Category", "Product", "Category"),
			named("Product_DefaultCheese", "Product", "Option"),
			named("Product_DefaultCrust", "Product", "Option"),
			named("Product_DefaultSauce", "Product", "Option"),
			named("ProductSize_Product", "ProductSize", "Product"),
			named("RecipeItem_Ingredient", "RecipeItem", "Ingredient"),
			named("RecipeItem_Product", "RecipeItem", "Product"),
		},
	}
	id := func(n string) string { return string(ent(n).ID) }
	cross := CrossLines{}
	for n, lines := range map[string][]string{
		"Allergen":    {"CartLine_Allergen"},
		"Ingredient":  {"CartLine_Removed", "Choice_Ingredient"},
		"Option":      {"CartLine_Cheese", "CartLine_Crust", "CartLine_Extra", "CartLine_Sauce", "Choice_Option"},
		"Product":     {"CartLine_Product", "OrderLine_Product"},
		"ProductSize": {"CartLine_Size", "Choice_Size"},
	} {
		cross[model.ID(id(n))] = lines
	}
	return dm, cross
}

func score(dm *domainmodel.DomainModel, pos map[model.ID]model.Point, cross CrossLines) (int, int) {
	return Crossings(dm, pos, Anchors(dm, pos), cross)
}

// Rule 1: as few crossings as possible. On Catalog the order the layering
// makes leaves crossings that swapping two entities in a column removes; the
// search must find fewer, and never more.
func TestPlanWith_OrderingSearchLowersCrossings(t *testing.T) {
	dm, cross := catalog()
	g := newGraph(dm)
	base, iso := g.layers()
	bl, bb := score(dm, g.place(base, iso, cross), cross)
	pl, pb := score(dm, PlanWith(dm, cross), cross)
	if pl+2*pb >= bl+2*bb {
		t.Fatalf("search: %d crossings + %d through boxes, single sweep: %d + %d; want strictly fewer", pl, pb, bl, bb)
	}
	// Lowering crossings must not undo phase 3.
	if v := crossViolations(dm, PlanWith(dm, cross), cross); len(v) > 0 {
		t.Fatalf("cross-module lines blocked:\n%s", strings.Join(v, "\n"))
	}
}

// Never worse than the single sweep, on every fixture the package has.
func TestPlanWith_OrderingSearchNeverWorse(t *testing.T) {
	dm, cross := catalog()
	for name, c := range map[string]struct {
		dm    *domainmodel.DomainModel
		cross CrossLines
	}{"capTrack": {capTrack(), nil}, "catalog": {dm, cross}, "catalog without cross": {dm, nil}} {
		g := newGraph(c.dm)
		base, iso := g.layers()
		bl, bb := score(c.dm, g.place(base, iso, c.cross), c.cross)
		pl, pb := score(c.dm, PlanWith(c.dm, c.cross), c.cross)
		if pl+2*pb > bl+2*bb {
			t.Errorf("%s: search scored %d, single sweep %d", name, pl+2*pb, bl+2*bb)
		}
	}
}
