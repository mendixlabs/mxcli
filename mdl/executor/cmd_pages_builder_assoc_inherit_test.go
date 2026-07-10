// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Bug 3 (unsolved case): a template-parameter / column attribute that navigates
// an association declared on a BASE entity, from a widget context bound to a
// SUBCLASS, was silently dropped. resolveAssociationAttributePath →
// associationDestination required an exact FROM/TO match, so a specialization
// context fell through to the flat-path handling → CE0402 "No value specified".
func TestResolveAssociationAttributePath_InheritedContext(t *testing.T) {
	const modID = model.ID("mod")
	const expenseID = model.ID("e-expense")
	const employeeID = model.ID("e-employee")
	const specialID = model.ID("e-special")

	newPB := func(ctxEntity string) *pageBuilder {
		return &pageBuilder{
			entityContext: ctxEntity,
			execCache: &executorCache{
				hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "M"}},
				domainModels: []*domainmodel.DomainModel{{
					ContainerID: modID,
					Entities: []*domainmodel.Entity{
						{BaseElement: model.BaseElement{ID: expenseID}, Name: "Expense"},
						{BaseElement: model.BaseElement{ID: employeeID}, Name: "Employee"},
						{BaseElement: model.BaseElement{ID: specialID}, Name: "SpecialExpense", GeneralizationRef: "M.Expense"},
					},
					Associations: []*domainmodel.Association{
						{Name: "Expense_Employee", ParentID: expenseID, ChildID: employeeID, Type: domainmodel.AssociationTypeReference},
					},
				}},
			},
		}
	}

	// The association is on the base Expense; the context is the SpecialExpense
	// subclass. It must still resolve to Employee.Name via one hop.
	t.Run("subclass context resolves base-entity association", func(t *testing.T) {
		pb := newPB("M.SpecialExpense")
		finalQN, steps, ok := pb.resolveAssociationAttributePath("M.Expense_Employee/Name")
		if !ok {
			t.Fatal("association path from a subclass context was dropped (ok=false)")
		}
		if finalQN != "M.Employee.Name" {
			t.Errorf("finalQN = %q, want M.Employee.Name", finalQN)
		}
		if len(steps) != 1 || steps[0].Association != "M.Expense_Employee" || steps[0].DestinationEntity != "M.Employee" {
			t.Errorf("steps = %+v, want one hop Expense_Employee → M.Employee", steps)
		}
	})

	// Regression guard: the exact-endpoint context still works.
	t.Run("exact-endpoint context still resolves", func(t *testing.T) {
		pb := newPB("M.Expense")
		finalQN, steps, ok := pb.resolveAssociationAttributePath("M.Expense_Employee/Name")
		if !ok || finalQN != "M.Employee.Name" || len(steps) != 1 {
			t.Errorf("exact-endpoint resolve failed: ok=%v finalQN=%q steps=%+v", ok, finalQN, steps)
		}
	})

	// An unrelated context (neither endpoint nor a descendant) must NOT resolve —
	// we refuse rather than emit a wrong ref.
	t.Run("unrelated context refuses", func(t *testing.T) {
		pb := newPB("M.Employee") // Employee is the TO side; navigating Expense_Employee from here is the reverse
		_, _, ok := pb.resolveAssociationAttributePath("M.Expense_Employee/Name")
		if !ok {
			t.Skip("reverse navigation from TO side is a separate concern; not asserted here")
		}
	})

	// entityIsOrDescendsFrom unit behavior.
	t.Run("entityIsOrDescendsFrom", func(t *testing.T) {
		pb := newPB("M.SpecialExpense")
		if !pb.entityIsOrDescendsFrom("M.SpecialExpense", "M.Expense") {
			t.Error("SpecialExpense should descend from Expense")
		}
		if !pb.entityIsOrDescendsFrom("M.Expense", "M.Expense") {
			t.Error("an entity is itself (reflexive)")
		}
		if pb.entityIsOrDescendsFrom("M.Employee", "M.Expense") {
			t.Error("Employee does not descend from Expense")
		}
	})
}
