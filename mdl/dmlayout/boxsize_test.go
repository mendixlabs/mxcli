// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func withAttributes(name string, n int) *domainmodel.Entity {
	e := &domainmodel.Entity{Name: name}
	for i := 0; i < n; i++ {
		e.Attributes = append(e.Attributes, &domainmodel.Attribute{Name: "A"})
	}
	return e
}

// Box sizes measured in Studio Pro (PROPOSAL_domain_model_readable_layout.md,
// "Measurements"), each shot calibrated against the positions stored in its
// project: classic viewer on TestApp's Mappings module and a grid-placed
// Fulfilment module, beta viewer (11.16.1) on ako/PizzaV1's Fulfilment and
// Catalog modules.
//
// The estimate must cover both viewers, because a project is opened in either:
// a box drawn taller than estimated is the overlap MPR016 exists to catch and
// Place exists to avoid.
var measuredHeights = []struct {
	attrs          int
	classic, beta  int // drawn height in model px; 0 = not measured
	measuredEntity string
}{
	{1, 84, 72, "OrderAccess"},
	{2, 84, 0, "Item"},
	{3, 84, 77, "Order (TestApp) / SimulationEvent"},
	{4, 101, 93, "Customer, OrderLine / Allergen"},
	{5, 0, 109, "Category"},
	{6, 132, 0, "Oven, DeliveryZone"},
	{7, 0, 141, "OrderStatusChange"},
	{8, 0, 157, "ProductSize"},
	{16, 0, 284, "Option"},
	{51, 855, 0, "Order (Fulfilment), ±3 px"},
}

func TestBoxSize_CoversEveryMeasuredHeight(t *testing.T) {
	for _, m := range measuredHeights {
		_, h := boxSize(withAttributes("E", m.attrs))
		drawn := max(m.classic, m.beta)
		if h < drawn {
			t.Errorf("%d attributes (%s): estimated height %d, drawn %d: the box below can touch it",
				m.attrs, m.measuredEntity, h, drawn)
		}
		// Generous is fine, but not by a row and more: that is wasted space in
		// every column Plan stacks.
		if h > drawn+16 {
			t.Errorf("%d attributes (%s): estimated height %d, drawn %d: more than a row too tall",
				m.attrs, m.measuredEntity, h, drawn)
		}
	}
}

// Studio Pro draws every entity the same width (150 classic, 158 beta) and cuts
// long names off with "…". The width does not grow with the name.
func TestBoxSize_WidthIsFixed(t *testing.T) {
	short, _ := boxSize(withAttributes("Oven", 3))
	long := withAttributes("OrderLineModifierWithAVeryLongName", 3)
	long.Attributes[0].Name = strings.Repeat("TotalOrderAmount", 3)
	wide, _ := boxSize(long)
	if short != wide {
		t.Fatalf("width depends on names: %d for a short one, %d for a long one", short, wide)
	}
	if short < 158 || short > 170 {
		t.Fatalf("width = %d, want at least the beta viewer's 158 and not much more", short)
	}
}

func TestBoxSize_NilEntity(t *testing.T) {
	w, h := boxSize(nil)
	if w2, h2 := boxSize(withAttributes("E", 0)); w != w2 || h != h2 {
		t.Fatalf("boxSize(nil) = %dx%d, want an empty entity's %dx%d", w, h, w2, h2)
	}
}
