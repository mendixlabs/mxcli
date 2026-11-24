// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A lookup sits in the column just left of its nearest referrer, not always in
// column 0: Catalog's Category, referenced only by Product three columns in,
// had a line across two columns to reach it.
func TestPlan_LookupSitsLeftOfItsNearestReferrer(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			ent("Allergen"), ent("Ingredient"), ent("Option"), ent("Product"), ent("Category"),
		},
		Associations: []*domainmodel.Association{
			assoc("Ingredient_Allergen", "Ingredient", "Allergen"),
			assoc("Option_Ingredient", "Option", "Ingredient"),
			assoc("Product_Option", "Product", "Option"),
			assoc("Product_Category", "Product", "Category"),
		},
	}
	p := planByName(t, dm)
	if p["Category"].X != p["Option"].X {
		t.Errorf("Category x = %d, want Option's column %d, the one left of Product (%d)",
			p["Category"].X, p["Option"].X, p["Product"].X)
	}
	// The control: a lookup whose nearest referrer is in column 1 stays in 0.
	if p["Allergen"].X != OriginX {
		t.Errorf("Allergen x = %d, want column 0 (%d): Ingredient, its referrer, is column 1", p["Allergen"].X, OriginX)
	}
}

// A lookup moves to the row of what references it. Here R's row is set by T,
// which it references twice; L, referenced once by R, stacked at the top of
// column 0 by name and was left a long diagonal away from R.
func TestPlan_LookupFollowsItsReferrersRow(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			ent("ALookup"),
			ent("Filler1", "a", "b", "c", "d", "e", "f"),
			ent("Filler2", "a", "b", "c", "d", "e", "f"),
			ent("Target"),
			ent("Referrer"),
			ent("F1User"), ent("F2User"),
		},
		Associations: []*domainmodel.Association{
			assoc("R_T1", "Referrer", "Target"),
			assoc("R_T2", "Referrer", "Target"),
			assoc("R_L", "Referrer", "ALookup"),
			assoc("U1", "F1User", "Filler1"),
			assoc("U2", "F2User", "Filler2"),
		},
	}
	p := planByName(t, dm)
	dist := func(a, b string) int {
		d := p[a].Y - p[b].Y
		if d < 0 {
			d = -d
		}
		return d
	}
	_, hT := boxSize(dm.Entities[3])
	// Target holds Referrer's row in column 0, so the nearest the lookup can get
	// is right under it.
	if d := dist("ALookup", "Referrer"); d > hT+rowGutter {
		t.Errorf("ALookup is %d px from Referrer's row, want at most %d (just under Target)", d, hT+rowGutter)
	}
	assertNoOverlap(t, dm)
}

// Moving a lookup must not undo phase 3: an entity's cross-module lines run
// left over the lookups' column, and a lookup that follows its referrer into
// that strip would put a box back in the line's way.
func TestPlanWith_LookupDoesNotBlockACrossModuleLine(t *testing.T) {
	dm := capTrack()
	byName := map[string]*domainmodel.Entity{}
	for _, e := range dm.Entities {
		byName[e.Name] = e
	}
	cross := CrossLines{
		byName["Employee"].ID:  {"Employee_Account", "Employee_Manager", "Employee_Photo"},
		byName["PlanScope"].ID: {"PlanScope_Owner"},
		byName["Team"].ID:      {"Team_Lead"},
		// Lookups with lines of their own: moving one must keep its own clear too.
		byName["Department"].ID:     {"Department_Head"},
		byName["Region"].ID:         {"Region_Manager"},
		byName["EmploymentType"].ID: {"EmploymentType_Contract"},
	}
	pos := PlanWith(dm, cross)
	if v := crossViolations(dm, pos, cross); len(v) > 0 {
		t.Fatalf("cross-module lines blocked:\n%s", strings.Join(v, "\n"))
	}
}

func assertNoOverlap(t *testing.T, dm *domainmodel.DomainModel) {
	t.Helper()
	pos := Plan(dm)
	for i, a := range dm.Entities {
		for _, b := range dm.Entities[i+1:] {
			if EntityBoxAt(a, pos[a.ID]).Intersects(EntityBoxAt(b, pos[b.ID])) {
				t.Errorf("%s and %s overlap", a.Name, b.Name)
			}
		}
	}
}

// A lookup with a cross-module line of its own keeps it clear when it follows
// its referrer. Here L (column 1) would follow R up to the rows beside the tall
// A in column 0, putting A in L's own line.
func TestPlanWith_LookupKeepsItsOwnLineClear(t *testing.T) {
	dm := &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{
			ent("A", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t"),
			ent("X"), ent("L"), ent("R"),
		},
		Associations: []*domainmodel.Association{
			assoc("X_A", "X", "A"),
			assoc("R_X1", "R", "X"),
			assoc("R_X2", "R", "X"),
			assoc("R_L", "R", "L"),
		},
	}
	l := dm.Entities[2]
	cross := CrossLines{l.ID: {"L_Account"}}
	pos := PlanWith(dm, cross)
	if pos[l.ID].X == OriginX {
		t.Fatalf("precondition: L should sit in column 1, left of R")
	}
	if v := crossViolations(dm, pos, cross); len(v) > 0 {
		t.Fatalf("lookup's own line blocked:\n%s", strings.Join(v, "\n"))
	}
}

// A lookup can share its nearest referrer's column instead of having a column
// of its own left of it: Catalog's Allergen with Ingredient, a short vertical
// line, one column fewer, crossings 10 -> 5. Whether it goes above or below
// is the crossing search's call (above: 1 crossing, below: 3).
func TestPlanWith_LookupSharesItsReferrersColumnWhenThatIsBetter(t *testing.T) {
	dm, cross := catalog()
	pos := PlanWith(dm, cross)
	byName := map[string]*domainmodel.Entity{}
	for _, e := range dm.Entities {
		byName[e.Name] = e
	}
	al, in := pos[byName["Allergen"].ID], pos[byName["Ingredient"].ID]
	if al.X != in.X {
		t.Fatalf("Allergen at %v, Ingredient at %v: want them in one column", al, in)
	}
	_, hAl := boxSize(byName["Allergen"])
	_, hIn := boxSize(byName["Ingredient"])
	if !(al.Y+hAl+rowGutter == in.Y || in.Y+hIn+rowGutter == al.Y) {
		t.Errorf("Allergen (y %d) and Ingredient (y %d) are not directly above one another", al.Y, in.Y)
	}
	if v := crossViolations(dm, pos, cross); len(v) > 0 {
		t.Fatalf("cross-module lines blocked:\n%s", strings.Join(v, "\n"))
	}
}

// The lookup move never adds a line through a box. On Shop (ako/PizzaV1's
// real module: entities with their attribute counts, associations and the
// cross-module lines drawn at them), Cart, alone in its column, moved into its
// referrers' column scored fewer crossings overall but sent MenuView_Cart
// straight through HomeView.
func TestPlanWith_LookupMoveNeverAddsALineThroughABox(t *testing.T) {
	dm, cross := shop()
	pos := PlanWith(dm, cross)
	if _, boxes := Crossings(dm, pos, Anchors(dm, pos), cross); boxes != 0 {
		t.Fatalf("%d line(s) through a box", boxes)
	}
}

func shop() (*domainmodel.DomainModel, CrossLines) {
	sizes := map[string]int{
		"Cart": 28, "CartLine": 21, "CheckoutForm": 51, "Choice": 11, "ErrorView": 1,
		"HomeView": 17, "LookupAttempt": 1, "LookupForm": 3, "MenuView": 10, "TrackView": 69,
	}
	dm := &domainmodel.DomainModel{}
	for _, n := range []string{"Cart", "CartLine", "CheckoutForm", "Choice", "ErrorView", "HomeView", "LookupAttempt", "LookupForm", "MenuView", "TrackView"} {
		dm.Entities = append(dm.Entities, withN(n, sizes[n]))
	}
	dm.Associations = []*domainmodel.Association{
		named("CartLine_Cart", "CartLine", "Cart"),
		named("CartLine_EditOf", "CartLine", "CartLine"),
		named("Choice_Line", "Choice", "CartLine"),
		named("HomeView_Cart", "HomeView", "Cart"),
		named("MenuView_Cart", "MenuView", "Cart"),
	}
	cross := CrossLines{}
	for n, lines := range map[string][]string{
		"Cart":          {"Cart_ActiveOrder", "Cart_Coupon", "Cart_Owner"},
		"CartLine":      {"CartLine_Allergen", "CartLine_Cheese", "CartLine_Crust", "CartLine_Extra", "CartLine_Product", "CartLine_Removed", "CartLine_Sauce", "CartLine_Size"},
		"CheckoutForm":  {"CheckoutForm_Owner", "CheckoutForm_Zone"},
		"Choice":        {"Choice_Ingredient", "Choice_Option", "Choice_Size"},
		"LookupAttempt": {"LookupAttempt_User"},
		"TrackView":     {"TrackView_Order"},
	} {
		cross[ent(n).ID] = lines
	}
	return dm, cross
}
