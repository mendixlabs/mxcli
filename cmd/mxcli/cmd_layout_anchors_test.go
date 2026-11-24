// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func layoutTestModel() *domainmodel.DomainModel {
	ent := func(id, name string, attrs int) *domainmodel.Entity {
		e := &domainmodel.Entity{BaseElement: model.BaseElement{ID: model.ID(id)}, Name: name}
		for i := 0; i < attrs; i++ {
			e.Attributes = append(e.Attributes, &domainmodel.Attribute{Name: "A"})
		}
		return e
	}
	return &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{ent("c", "Customer", 1), ent("o", "Order", 9), ent("l", "OrderLine", 3)},
		Associations: []*domainmodel.Association{
			{BaseElement: model.BaseElement{ID: "a1"}, Name: "Order_Customer", ParentID: "o", ChildID: "c"},
			{BaseElement: model.BaseElement{ID: "a2"}, Name: "OrderLine_Order", ParentID: "l", ChildID: "o"},
		},
	}
}

// A layout writes the computed connection points into the associations, and a
// second run over the result changes nothing: the unit is not rewritten again
// (ADR-0008). The first half is the control for the second: an "unchanged"
// that held because nothing was ever written would pass on its own.
func TestApplyAnchors_WritesThenSettles(t *testing.T) {
	dm := layoutTestModel()
	plan := dmlayout.Plan(dm)

	if n := applyAnchors(dm, plan, true); n != len(dm.Associations) {
		t.Fatalf("first run re-anchored %d of %d associations, want all (none had anchors)", n, len(dm.Associations))
	}
	want := dmlayout.Anchors(dm, plan)
	for _, a := range dm.Associations {
		if a.ParentConnection == nil || *a.ParentConnection != want[a.ID].Parent ||
			a.ChildConnection == nil || *a.ChildConnection != want[a.ID].Child {
			t.Errorf("%s: stored %v / %v, want %v", a.Name, a.ParentConnection, a.ChildConnection, want[a.ID])
		}
	}
	if n := applyAnchors(dm, plan, true); n != 0 {
		t.Fatalf("second run re-anchored %d associations, want 0", n)
	}
}

// --dry-run counts without touching the model.
func TestApplyAnchors_DryRunWritesNothing(t *testing.T) {
	dm := layoutTestModel()
	if n := applyAnchors(dm, dmlayout.Plan(dm), false); n == 0 {
		t.Fatal("dry run counted no changes on a model with no anchors")
	}
	for _, a := range dm.Associations {
		if a.ParentConnection != nil || a.ChildConnection != nil {
			t.Fatalf("%s: dry run wrote anchors", a.Name)
		}
	}
}
