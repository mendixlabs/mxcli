// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// scriptPlacedModel is what `create entity` leaves behind for three linked
// entities when nobody runs `mxcli layout`: slots 0, 1 and 2 of the grid.
func scriptPlacedModel() *domainmodel.DomainModel {
	mk := func(id, name string, slot int) *domainmodel.Entity {
		return &domainmodel.Entity{
			BaseElement: model.BaseElement{ID: model.ID(id)},
			Name:        name,
			Location:    dmlayout.GridSlot(slot),
		}
	}
	return &domainmodel.DomainModel{
		ContainerID: model.ID("m1"),
		Entities:    []*domainmodel.Entity{mk("c", "Customer", 0), mk("o", "Order", 1), mk("l", "OrderLine", 2)},
		Associations: []*domainmodel.Association{
			{ParentID: "o", ChildID: "c"},
			{ParentID: "l", ChildID: "o"},
		},
	}
}

func shopModule() []*model.Module {
	return []*model.Module{{Name: "Shop", BaseElement: model.BaseElement{ID: model.ID("m1")}}}
}

func TestDomainModelNotLaidOutRule_Metadata(t *testing.T) {
	r := NewDomainModelNotLaidOutRule()
	if r.ID() != "MPR020" {
		t.Errorf("ID = %q, want MPR020", r.ID())
	}
	if r.Name() != "DomainModelNotLaidOut" {
		t.Errorf("Name = %q", r.Name())
	}
	if r.Category() != "design" {
		t.Errorf("Category = %q, want design", r.Category())
	}
	// Error, not warning: `mxcli lint` and `mxcli verify` fail only on errors by
	// default, and the point of this rule is that an agent cannot pass the gate
	// with a diagram it never laid out. A warning is what MPR016 is, and agents
	// ship overlapping boxes past it.
	if r.DefaultSeverity() != linter.SeverityError {
		t.Errorf("Severity = %v, want error", r.DefaultSeverity())
	}
}

// One finding per module, naming the entities and the command that repairs it.
func TestDomainModelNotLaidOutRule_ReportsAScriptPlacedModule(t *testing.T) {
	ctx := newDMContext(t, dmReader{dms: []*domainmodel.DomainModel{scriptPlacedModel()}, modules: shopModule()})

	got := NewDomainModelNotLaidOutRule().Check(ctx)
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	v := got[0]
	if v.RuleID != "MPR020" || v.Location.Module != "Shop" || v.Location.DocumentName != "Order" {
		t.Errorf("violation = %+v", v)
	}
	for _, want := range []string{"Order", "OrderLine"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message %q does not name %q", v.Message, want)
		}
	}
	if !strings.Contains(v.Suggestion, "mxcli layout") || !strings.Contains(v.Suggestion, "--module Shop") {
		t.Errorf("suggestion %q does not name the repair command for the module", v.Suggestion)
	}
}

// The control: the same module after `mxcli layout` has run is clean, so the
// finding's own repair clears it.
func TestDomainModelNotLaidOutRule_LaidOutModuleIsClean(t *testing.T) {
	dm := scriptPlacedModel()
	for _, e := range dm.Entities {
		e.Location = dmlayout.Plan(dm)[e.ID]
	}
	ctx := newDMContext(t, dmReader{dms: []*domainmodel.DomainModel{dm}, modules: shopModule()})
	if got := NewDomainModelNotLaidOutRule().Check(ctx); len(got) != 0 {
		t.Fatalf("got %d violations after layout, want 0: %+v", len(got), got)
	}
}

func TestDomainModelNotLaidOutRule_HonoursModuleExclusion(t *testing.T) {
	ctx := newDMContext(t, dmReader{dms: []*domainmodel.DomainModel{scriptPlacedModel()}, modules: shopModule()})
	ctx.SetExcludedModules([]string{"Shop"})
	if got := NewDomainModelNotLaidOutRule().Check(ctx); len(got) != 0 {
		t.Fatalf("got %d violations for an excluded module, want 0", len(got))
	}
}

// A Marketplace module is not the user's to arrange (and `mxcli layout` skips
// it), so reporting it would be a finding with no repair.
func TestDomainModelNotLaidOutRule_SkipsMarketplaceModules(t *testing.T) {
	mods := shopModule()
	mods[0].FromAppStore = true
	ctx := newDMContext(t, dmReader{dms: []*domainmodel.DomainModel{scriptPlacedModel()}, modules: mods})
	if got := NewDomainModelNotLaidOutRule().Check(ctx); len(got) != 0 {
		t.Fatalf("got %d violations for a Marketplace module, want 0", len(got))
	}
}

func TestDomainModelNotLaidOutRule_ReaderWithoutDomainModels(t *testing.T) {
	if got := NewDomainModelNotLaidOutRule().Check(newDMContext(t, rawUnitReader{})); len(got) != 0 {
		t.Fatalf("got %d violations, want 0", len(got))
	}
	if got := NewDomainModelNotLaidOutRule().Check(linter.NewLintContextFromDB(nil)); len(got) != 0 {
		t.Fatalf("got %d violations with a nil reader, want 0", len(got))
	}
}
