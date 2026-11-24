// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// row is three entities on one row: A, M in the middle, B.
func row() *domainmodel.DomainModel {
	mk := func(id, name string, x int) *domainmodel.Entity {
		return &domainmodel.Entity{BaseElement: model.BaseElement{ID: model.ID(id)}, Name: name, Location: model.Point{X: x, Y: 100}}
	}
	return &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm1"},
		ContainerID: model.ID("m1"),
		Entities:    []*domainmodel.Entity{mk("a", "Customer", 100), mk("m", "Invoice", 400), mk("b", "Order", 700)},
	}
}

func TestAssociationCrossesEntityRule_Metadata(t *testing.T) {
	r := NewAssociationCrossesEntityRule()
	if r.ID() != "MPR021" || r.Name() != "AssociationCrossesEntity" || r.Category() != "design" {
		t.Errorf("metadata: %s %s %s", r.ID(), r.Name(), r.Category())
	}
	// A warning: box sizes are estimates, and a crossing makes a diagram hard
	// to read, not wrong.
	if r.DefaultSeverity() != linter.SeverityWarning {
		t.Errorf("severity %v, want warning", r.DefaultSeverity())
	}
}

// Order -> Customer drawn straight through Invoice, between them on the row.
func TestAssociationCrossesEntityRule_ReportsALineThroughABox(t *testing.T) {
	dm := row()
	dm.Associations = []*domainmodel.Association{{
		BaseElement: model.BaseElement{ID: "x"}, Name: "Order_Customer", ParentID: "b", ChildID: "a",
		ParentConnection: &model.Point{X: 0, Y: 50}, ChildConnection: &model.Point{X: 100, Y: 50},
	}}
	got := NewAssociationCrossesEntityRule().Check(newDMContext(t, dmReader{dms: []*domainmodel.DomainModel{dm}, modules: shopModule()}))
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	for _, want := range []string{"Order_Customer", "Invoice"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("message %q does not name %q", got[0].Message, want)
		}
	}
	if !strings.Contains(got[0].Suggestion, "mxcli layout --module Shop") {
		t.Errorf("suggestion %q does not name the command", got[0].Suggestion)
	}
}

// The control: the same line routed over the top, clear of Invoice.
func TestAssociationCrossesEntityRule_ClearLineIsNotReported(t *testing.T) {
	dm := row()
	dm.Entities[1].Location.Y = 400 // Invoice moved off the row
	dm.Associations = []*domainmodel.Association{{
		BaseElement: model.BaseElement{ID: "x"}, Name: "Order_Customer", ParentID: "b", ChildID: "a",
		ParentConnection: &model.Point{X: 0, Y: 50}, ChildConnection: &model.Point{X: 100, Y: 50},
	}}
	if got := NewAssociationCrossesEntityRule().Check(newDMContext(t, dmReader{dms: []*domainmodel.DomainModel{dm}, modules: shopModule()})); len(got) != 0 {
		t.Fatalf("got %d violations for a clear line, want 0: %+v", len(got), got)
	}
}

// A cross-module line is fixed by Studio Pro (left side to the canvas edge);
// one from another module's association into Order runs through Invoice and
// Customer, to its left.
func TestAssociationCrossesEntityRule_ReportsACrossModuleLine(t *testing.T) {
	dm := row()
	crm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm2"}, ContainerID: model.ID("m2"),
		CrossAssociations: []*domainmodel.CrossModuleAssociation{{Name: "Lead_Order", ParentID: "lead", ChildRef: "Shop.Order"}},
	}
	mods := append(shopModule(), &model.Module{Name: "Crm", BaseElement: model.BaseElement{ID: model.ID("m2")}})
	got := NewAssociationCrossesEntityRule().Check(newDMContext(t, dmReader{dms: []*domainmodel.DomainModel{dm, crm}, modules: mods}))
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	for _, want := range []string{"Lead_Order", "Invoice", "Customer"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("message %q does not name %q", got[0].Message, want)
		}
	}
}

// Box sizes are estimates, so a line that only grazes a box edge — within a
// few pixels of where the estimate puts it — is not reported.
func TestAssociationCrossesEntityRule_GrazingIsNotReported(t *testing.T) {
	dm := row()
	// The line runs at y = 100 + 50% of 84 = 142; put Invoice's estimated
	// bottom edge 4 px under it.
	dm.Entities[1].Location.Y = 142 + 4 - 84
	dm.Associations = []*domainmodel.Association{{
		BaseElement: model.BaseElement{ID: "x"}, Name: "Order_Customer", ParentID: "b", ChildID: "a",
		ParentConnection: &model.Point{X: 0, Y: 50}, ChildConnection: &model.Point{X: 100, Y: 50},
	}}
	if got := NewAssociationCrossesEntityRule().Check(newDMContext(t, dmReader{dms: []*domainmodel.DomainModel{dm}, modules: shopModule()})); len(got) != 0 {
		t.Fatalf("got %d violations for a line grazing a box edge, want 0", len(got))
	}
}
