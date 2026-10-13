// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func mod(name string, marketplace bool) *model.Module {
	m := &model.Module{Name: name, FromAppStore: marketplace}
	m.ID = model.ID("id-" + name)
	if marketplace {
		m.AppStoreGuid = "guid-" + name
	}
	return m
}

func layoutFixture() []*model.Module {
	return []*model.Module{
		mod("System", false),
		mod("Administration", true),
		mod("Atlas_Core", true),
		mod("CapTrack", false),
		mod("MyFirstModule", false),
	}
}

// With no --module, only the modules the project owns are touched. Rearranging
// a Marketplace module is work the next update throws away, and System is not
// the user's to arrange.
func TestLayoutTargets_SkipsMarketplaceAndSystemByDefault(t *testing.T) {
	defer func(prev bool) { layoutIncludeMarketplace = prev }(layoutIncludeMarketplace)
	layoutIncludeMarketplace = false

	got, err := layoutTargets(layoutFixture(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var names []string
	for _, m := range got {
		names = append(names, m.Name)
	}
	want := "CapTrack,MyFirstModule"
	if strings.Join(names, ",") != want {
		t.Errorf("targets = %v, want %s", names, want)
	}
}

// Naming a Marketplace module explicitly is a mistake worth reporting, not
// something to silently drop — a silent skip reports success having done
// nothing, which is the failure mode this whole session kept finding.
func TestLayoutTargets_NamedMarketplaceModuleIsRefused(t *testing.T) {
	defer func(prev bool) { layoutIncludeMarketplace = prev }(layoutIncludeMarketplace)
	layoutIncludeMarketplace = false

	_, err := layoutTargets(layoutFixture(), map[string]string{"administration": "Administration"})
	if err == nil {
		t.Fatal("naming a Marketplace module was accepted")
	}
	if !strings.Contains(err.Error(), "--include-marketplace") {
		t.Errorf("the error should name the escape hatch: %v", err)
	}
}

// ...and the escape hatch works.
func TestLayoutTargets_IncludeMarketplaceOptsIn(t *testing.T) {
	defer func(prev bool) { layoutIncludeMarketplace = prev }(layoutIncludeMarketplace)
	layoutIncludeMarketplace = true

	got, err := layoutTargets(layoutFixture(), map[string]string{"administration": "Administration"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Administration" {
		t.Errorf("targets = %v, want [Administration]", got)
	}
}

// A typo must fail loudly, quoting what was typed rather than a normalised form.
func TestLayoutTargets_UnknownModuleIsAnError(t *testing.T) {
	_, err := layoutTargets(layoutFixture(), map[string]string{"nope": "Nope"})
	if err == nil {
		t.Fatal("an unknown module was accepted")
	}
	if !strings.Contains(err.Error(), "Nope") {
		t.Errorf("the error should quote the name as typed, got: %v", err)
	}
}

// Module names resolve case-insensitively in Mendix, so --module captrack has to
// find CapTrack.
func TestLayoutTargets_MatchesCaseInsensitively(t *testing.T) {
	got, err := layoutTargets(layoutFixture(), map[string]string{"captrack": "captrack"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "CapTrack" {
		t.Errorf("targets = %v, want [CapTrack]", got)
	}
}

// System is refused by name for the same reason the Marketplace one is: a silent
// skip would look like success.
func TestLayoutTargets_SystemIsRefusedByName(t *testing.T) {
	_, err := layoutTargets(layoutFixture(), map[string]string{"system": "System"})
	if err == nil || !strings.Contains(err.Error(), "System") {
		t.Errorf("naming System should be refused, got: %v", err)
	}
}

// The order the modules are processed in must not depend on the order the
// backend happened to list them — the output is a report someone reads.
func TestLayoutTargets_IsSorted(t *testing.T) {
	shuffled := []*model.Module{
		mod("MyFirstModule", false),
		mod("CapTrack", false),
		mod("Zeta", false),
		mod("Alpha", false),
	}
	got, err := layoutTargets(shuffled, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var names []string
	for _, m := range got {
		names = append(names, m.Name)
	}
	if strings.Join(names, ",") != "Alpha,CapTrack,MyFirstModule,Zeta" {
		t.Errorf("targets = %v, want them sorted", names)
	}
}

// overlapFixture is a domain model as CREATE ENTITY leaves it when one entity is
// longer than a grid row: Order (30 attributes) runs into Invoice below it.
// Pinned sits far from both, off any grid.
func overlapFixture() *domainmodel.DomainModel {
	attrs := func(n int) []*domainmodel.Attribute {
		out := make([]*domainmodel.Attribute, n)
		for i := range out {
			out[i] = &domainmodel.Attribute{Name: fmt.Sprintf("Attr%d", i)}
		}
		return out
	}
	mk := func(name string, n, x, y int) *domainmodel.Entity {
		e := &domainmodel.Entity{Name: name, Attributes: attrs(n), Location: model.Point{X: x, Y: y}}
		e.ID = model.ID("id-" + name)
		return e
	}
	return &domainmodel.DomainModel{Entities: []*domainmodel.Entity{
		mk("Order", 30, 100, 100),
		mk("Invoice", 3, 100, 380),
		mk("Pinned", 4, 4321, 1234),
	}}
}

func runLayoutModule(t *testing.T, dm *domainmodel.DomainModel, dryRun, only bool) (string, int, int) {
	t.Helper()
	var out bytes.Buffer
	writes := 0
	moved, err := layoutModule(&out, "Sales", dm, dryRun, only, func(*domainmodel.DomainModel) error {
		writes++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out.String(), moved, writes
}

// --dry-run lists the colliding pair and the move, and neither writes nor
// changes the model.
func TestLayoutOnlyOverlapping_DryRunListsPairsAndMoves(t *testing.T) {
	dm := overlapFixture()
	before := fmt.Sprint(dm.Entities[0].Location, dm.Entities[1].Location, dm.Entities[2].Location)
	out, moved, writes := runLayoutModule(t, dm, true, true)

	for _, want := range []string{
		"Sales.Invoice overlaps Sales.Order",
		"Sales.Invoice  (100, 380) -> (",
		"Sales: 1 of 3 entities would move",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Sales.Pinned  (") || strings.Contains(out, "Sales.Order  (") {
		t.Errorf("an entity that keeps its place was listed as a move:\n%s", out)
	}
	if moved != 1 || writes != 0 {
		t.Errorf("moved=%d writes=%d, want 1 and 0 on a dry run", moved, writes)
	}
	if after := fmt.Sprint(dm.Entities[0].Location, dm.Entities[1].Location, dm.Entities[2].Location); after != before {
		t.Errorf("a dry run changed the model: %s -> %s", before, after)
	}
}

// A real run moves only the colliding entity, writes once, and the next run has
// nothing to do.
func TestLayoutOnlyOverlapping_MovesOnlyTheCollidingEntityAndIsIdempotent(t *testing.T) {
	dm := overlapFixture()
	order, pinned := dm.Entities[0].Location, dm.Entities[2].Location
	invoice := dm.Entities[1].Location

	out, moved, writes := runLayoutModule(t, dm, false, true)
	if moved != 1 || writes != 1 || !strings.Contains(out, "Sales: moved 1 of 3 entities") {
		t.Fatalf("first run: moved=%d writes=%d\n%s", moved, writes, out)
	}
	if dm.Entities[0].Location != order || dm.Entities[2].Location != pinned {
		t.Error("an entity that collides with nothing was moved")
	}
	if dm.Entities[1].Location == invoice {
		t.Error("Invoice was not moved")
	}

	out, moved, writes = runLayoutModule(t, dm, false, true)
	if moved != 0 || writes != 0 || !strings.Contains(out, "Sales: no overlapping entities (3 entities)") {
		t.Errorf("second run: moved=%d writes=%d\n%s", moved, writes, out)
	}
}

// CONTROL: without the flag the command is the full layout, as before.
func TestLayoutWithoutOnlyOverlapping_StillArrangesEverything(t *testing.T) {
	dm := overlapFixture()
	_, moved, writes := runLayoutModule(t, dm, false, false)
	if moved != 3 || writes != 1 {
		t.Errorf("full layout moved %d of 3 (writes=%d), want all three", moved, writes)
	}
	out, moved, writes := runLayoutModule(t, dm, false, false)
	if moved != 0 || writes != 0 || !strings.Contains(out, "already laid out") {
		t.Errorf("second full layout: moved=%d writes=%d\n%s", moved, writes, out)
	}
}
