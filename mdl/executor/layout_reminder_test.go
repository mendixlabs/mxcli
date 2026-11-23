// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// gridPlacedShop is what three `create entity` statements without @Position
// leave behind for linked entities: grid slots 0, 1 and 2.
func gridPlacedShop() *domainmodel.DomainModel {
	mk := func(id, name string, slot int) *domainmodel.Entity {
		return &domainmodel.Entity{
			BaseElement: model.BaseElement{ID: model.ID(id)},
			Name:        name,
			Location:    dmlayout.GridSlot(slot),
		}
	}
	return &domainmodel.DomainModel{
		Entities: []*domainmodel.Entity{mk("c", "Customer", 0), mk("o", "Order", 1), mk("l", "OrderLine", 2)},
		Associations: []*domainmodel.Association{
			{ParentID: "o", ChildID: "c"},
			{ParentID: "l", ChildID: "o"},
		},
	}
}

// finalizeWith runs the end-of-program step over one modified module, as a
// script that created entities in it leaves the executor.
func finalizeWith(t *testing.T, dm *domainmodel.DomainModel, mod *model.Module, quiet bool) string {
	t.Helper()
	var buf bytes.Buffer
	e := New(&buf)
	e.SetQuiet(quiet)
	e.backend = &mock.MockBackend{
		IsConnectedFunc:             func() bool { return true },
		GetDomainModelFunc:          func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		GetModuleFunc:               func(model.ID) (*model.Module, error) { return mod, nil },
		ReconcileMemberAccessesFunc: func(model.ID, string) (int, error) { return 0, nil },
	}
	e.cache = &executorCache{modifiedDomainModels: map[model.ID]string{mod.ID: mod.Name}}
	if err := e.finalizeProgramExecution(); err != nil {
		t.Fatalf("finalizeProgramExecution: %v", err)
	}
	return buf.String()
}

func shop() *model.Module {
	return &model.Module{Name: "Shop", BaseElement: model.BaseElement{ID: model.ID("m1")}}
}

// A script that leaves linked entities on the placement grid is told, once, at
// the moment it ran, which command lays the module out. Agents forget the step
// the skill describes; this is where they are looking.
func TestFinalize_RemindsToLayOutAGridPlacedModule(t *testing.T) {
	out := finalizeWith(t, gridPlacedShop(), shop(), false)
	for _, want := range []string{"mxcli layout", "--module Shop", "Order", "OrderLine"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q does not contain %q", out, want)
		}
	}
	if strings.Count(out, "mxcli layout") != 1 {
		t.Errorf("reminder printed %d times, want once: %q", strings.Count(out, "mxcli layout"), out)
	}
}

// The control: a module `mxcli layout` already arranged gets no reminder, so the
// reminder is about the grid and not about every domain model change.
func TestFinalize_NoReminderForALaidOutModule(t *testing.T) {
	dm := gridPlacedShop()
	plan := dmlayout.Plan(dm)
	for _, e := range dm.Entities {
		e.Location = plan[e.ID]
	}
	if out := finalizeWith(t, dm, shop(), false); strings.Contains(out, "mxcli layout") {
		t.Fatalf("laid-out module got a reminder: %q", out)
	}
}

func TestFinalize_NoReminderWhenQuiet(t *testing.T) {
	if out := finalizeWith(t, gridPlacedShop(), shop(), true); out != "" {
		t.Fatalf("quiet run printed %q", out)
	}
}

// `mxcli layout` skips Marketplace modules, so a reminder to run it there would
// name a command that does nothing.
func TestFinalize_NoReminderForAMarketplaceModule(t *testing.T) {
	mod := shop()
	mod.FromAppStore = true
	if out := finalizeWith(t, gridPlacedShop(), mod, false); strings.Contains(out, "mxcli layout") {
		t.Fatalf("Marketplace module got a reminder: %q", out)
	}
}
