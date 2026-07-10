// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"sort"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	genDm "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
)

// AddEntityAccessRule adds (or upserts by matching module-role set) an entity
// access rule on a domain-model entity: allow create/delete, default member
// access, XPath constraint, and per-member (attribute/association) access rights.
// Mirrors legacy writer.AddEntityAccessRule (rules live on the entity in the
// domain model unit; unitID is the domain model unit).
func (b *Backend) AddEntityAccessRule(p backend.EntityAccessRuleParams) error {
	if b.writer == nil {
		return fmt.Errorf("AddEntityAccessRule: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(p.UnitID)
	if err != nil {
		return err
	}
	var ent *genDm.Entity
	for _, el := range dm.EntitiesItems() {
		if e, ok := el.(*genDm.Entity); ok && e.Name() == p.EntityName {
			ent = e
			break
		}
	}
	if ent == nil {
		return fmt.Errorf("AddEntityAccessRule: entity not found: %s", p.EntityName)
	}

	// Upsert: reuse an existing rule whose module-role set matches (keeps its ID
	// stable so references stay valid), else add a fresh rule.
	var rule *genDm.AccessRule
	for _, el := range ent.AccessRulesItems() {
		if r, ok := el.(*genDm.AccessRule); ok && sameStringSet(r.ModuleRolesQualifiedNames(), p.RoleNames) {
			rule = r
			break
		}
	}
	if rule == nil {
		rule = genDm.NewAccessRule()
		assignID(rule)
		ent.AddAccessRules(rule)
	} else {
		for i := len(rule.MemberAccessesItems()) - 1; i >= 0; i-- {
			rule.RemoveMemberAccesses(i)
		}
	}

	rule.SetModuleRolesQualifiedNames(p.RoleNames)
	rule.SetAllowCreate(p.AllowCreate)
	rule.SetAllowDelete(p.AllowDelete)
	if p.DefaultMemberAccess != "" {
		rule.SetDefaultMemberAccessRights(p.DefaultMemberAccess)
	}
	rule.SetXPathConstraint(p.XPathConstraint)
	for _, ma := range p.MemberAccesses {
		m := genDm.NewMemberAccess()
		assignID(m)
		m.SetAccessRights(ma.AccessRights)
		if ma.AttributeRef != "" {
			m.SetAttributeQualifiedName(ma.AttributeRef)
		}
		if ma.AssociationRef != "" {
			m.SetAssociationQualifiedName(ma.AssociationRef)
		}
		rule.AddMemberAccesses(m)
	}
	return b.persistDM(p.UnitID, dm)
}

// RevokeEntityMemberAccess narrows access on rules matching roleNames for an entity:
// revoke create/delete, and downgrade member read/write rights (ReadWrite→ReadOnly
// for write revocation, →None for read revocation) for all members or named ones.
// Returns the number of rules modified. Mirrors legacy writer.RevokeEntityMemberAccess.
func (b *Backend) RevokeEntityMemberAccess(unitID model.ID, entityName string, roleNames []string, rev types.EntityAccessRevocation) (int, error) {
	if b.writer == nil {
		return 0, fmt.Errorf("RevokeEntityMemberAccess: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(unitID)
	if err != nil {
		return 0, err
	}
	var ent *genDm.Entity
	for _, el := range dm.EntitiesItems() {
		if e, ok := el.(*genDm.Entity); ok && e.Name() == entityName {
			ent = e
			break
		}
	}
	if ent == nil {
		return 0, fmt.Errorf("RevokeEntityMemberAccess: entity not found: %s", entityName)
	}

	readSet := toStringSet(rev.RevokeReadMembers)
	writeSet := toStringSet(rev.RevokeWriteMembers)
	modified := 0
	for _, el := range ent.AccessRulesItems() {
		rule, ok := el.(*genDm.AccessRule)
		if !ok || !sameStringSet(rule.ModuleRolesQualifiedNames(), roleNames) {
			continue
		}
		ruleMod := false
		if rev.RevokeCreate && rule.AllowCreate() {
			rule.SetAllowCreate(false)
			ruleMod = true
		}
		if rev.RevokeDelete && rule.AllowDelete() {
			rule.SetAllowDelete(false)
			ruleMod = true
		}
		switch cur := rule.DefaultMemberAccessRights(); {
		case rev.RevokeReadAll && cur != "None":
			rule.SetDefaultMemberAccessRights("None")
			ruleMod = true
		case rev.RevokeWriteAll && cur == "ReadWrite":
			rule.SetDefaultMemberAccessRights("ReadOnly")
			ruleMod = true
		}
		for _, mel := range rule.MemberAccessesItems() {
			ma, ok := mel.(*genDm.MemberAccess)
			if !ok {
				continue
			}
			ref := ma.AttributeQualifiedName()
			if ref == "" {
				ref = ma.AssociationQualifiedName()
			}
			if ref == "" {
				continue
			}
			rights := ma.AccessRights()
			newRights := rights
			switch {
			case rev.RevokeReadAll || readSet[ref]:
				newRights = "None"
			case (rev.RevokeWriteAll || writeSet[ref]) && rights == "ReadWrite":
				newRights = "ReadOnly"
			}
			if newRights != rights {
				ma.SetAccessRights(newRights)
				ruleMod = true
			}
		}
		if ruleMod {
			modified++
		}
	}
	if modified > 0 {
		if err := b.persistDM(unitID, dm); err != nil {
			return 0, err
		}
	}
	return modified, nil
}

// RemoveEntityAccessRule removes the named module roles from an entity's access
// rules: a rule left with no roles is dropped, one with remaining roles is kept
// (with the role removed). Returns the number of rules modified. Mirrors legacy.
func (b *Backend) RemoveEntityAccessRule(unitID model.ID, entityName string, roleNames []string) (int, error) {
	if b.writer == nil {
		return 0, fmt.Errorf("RemoveEntityAccessRule: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(unitID)
	if err != nil {
		return 0, err
	}
	var ent *genDm.Entity
	for _, el := range dm.EntitiesItems() {
		if e, ok := el.(*genDm.Entity); ok && e.Name() == entityName {
			ent = e
			break
		}
	}
	if ent == nil {
		return 0, fmt.Errorf("RemoveEntityAccessRule: entity not found: %s", entityName)
	}
	removeSet := toStringSet(roleNames)
	modified := 0
	// Back-to-front so RemoveAccessRules(i) indices stay valid.
	rules := ent.AccessRulesItems()
	for i := len(rules) - 1; i >= 0; i-- {
		rule, ok := rules[i].(*genDm.AccessRule)
		if !ok {
			continue
		}
		cur := rule.ModuleRolesQualifiedNames()
		kept := make([]string, 0, len(cur))
		for _, r := range cur {
			if !removeSet[r] {
				kept = append(kept, r)
			}
		}
		if len(kept) == len(cur) {
			continue // unchanged
		}
		modified++
		if len(kept) == 0 {
			ent.RemoveAccessRules(i)
		} else {
			rule.SetModuleRolesQualifiedNames(kept)
		}
	}
	if modified > 0 {
		if err := b.persistDM(unitID, dm); err != nil {
			return 0, err
		}
	}
	return modified, nil
}

// RemoveRoleFromAllEntities removes a single module role from every entity's access
// rules in the domain model (dropping rules left role-less). Returns the number of
// rules modified. Mirrors legacy.
func (b *Backend) RemoveRoleFromAllEntities(unitID model.ID, roleName string) (int, error) {
	if b.writer == nil {
		return 0, fmt.Errorf("RemoveRoleFromAllEntities: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(unitID)
	if err != nil {
		return 0, err
	}
	modified := 0
	for _, el := range dm.EntitiesItems() {
		ent, ok := el.(*genDm.Entity)
		if !ok {
			continue
		}
		rules := ent.AccessRulesItems()
		for i := len(rules) - 1; i >= 0; i-- {
			rule, ok := rules[i].(*genDm.AccessRule)
			if !ok {
				continue
			}
			cur := rule.ModuleRolesQualifiedNames()
			kept := make([]string, 0, len(cur))
			for _, r := range cur {
				if r != roleName {
					kept = append(kept, r)
				}
			}
			if len(kept) == len(cur) {
				continue
			}
			modified++
			if len(kept) == 0 {
				ent.RemoveAccessRules(i)
			} else {
				rule.SetModuleRolesQualifiedNames(kept)
			}
		}
	}
	if modified > 0 {
		if err := b.persistDM(unitID, dm); err != nil {
			return 0, err
		}
	}
	return modified, nil
}

func toStringSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// sameStringSet reports whether a and b contain the same elements (order-insensitive).
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

// ReconcileMemberAccesses brings every populated access rule in a domain model
// into sync with its entity's current members: it adds a MemberAccess for each
// attribute, each FROM-side association (regular + cross), and each implicit
// system association (System.owner / System.changedBy); removes stale entries
// for members that no longer exist; and downgrades write rights on calculated
// attributes (CE6592). It mirrors the legacy writer's reconcile and is invoked
// by the executor's finalize step after every program run.
//
// Rules with no MemberAccesses yet (a fresh, empty rule) are left untouched —
// matching legacy; those are populated at create time by the inline sync in
// entityToGen, not retro-actively here. Members are added in definition order
// so the serialized output is deterministic.
func (b *Backend) ReconcileMemberAccesses(unitID model.ID, moduleName string) (int, error) {
	if b.writer == nil {
		return 0, fmt.Errorf("ReconcileMemberAccesses: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(unitID)
	if err != nil {
		return 0, err
	}

	modified := 0
	for _, el := range dm.EntitiesItems() {
		ent, ok := el.(*genDm.Entity)
		if !ok {
			continue
		}
		entityID := string(ent.ID())
		entityName := ent.Name()
		if entityName == "" {
			continue
		}

		// Attributes (in order) with calculated flags.
		type attrInfo struct {
			qn   string
			calc bool
		}
		var attrs []attrInfo
		attrSet := map[string]bool{}
		calcSet := map[string]bool{}
		for _, ae := range ent.AttributesItems() {
			a, ok := ae.(*genDm.Attribute)
			if !ok {
				continue
			}
			qn := moduleName + "." + entityName + "." + a.Name()
			_, isCalc := a.Value().(*genDm.CalculatedValue)
			attrs = append(attrs, attrInfo{qn, isCalc})
			attrSet[qn] = true
			if isCalc {
				calcSet[qn] = true
			}
		}

		// FROM-side associations (ParentPointer == this entity), regular + cross.
		var assocQNs []string
		assocSet := map[string]bool{}
		addAssoc := func(name string) {
			if name == "" {
				return
			}
			qn := moduleName + "." + name
			if !assocSet[qn] {
				assocSet[qn] = true
				assocQNs = append(assocQNs, qn)
			}
		}
		for _, ae := range dm.AssociationsItems() {
			if a, ok := ae.(*genDm.Association); ok && string(a.ParentRefID()) == entityID {
				addAssoc(a.Name())
			}
		}
		for _, ce := range dm.CrossAssociationsItems() {
			if ca, ok := ce.(*genDm.CrossAssociation); ok && string(ca.ParentRefID()) == entityID {
				addAssoc(ca.Name())
			}
		}

		// Implicit system associations from NoGeneralization flags.
		var sysRefs []string
		sysSet := map[string]bool{}
		if ng, ok := ent.Generalization().(*genDm.NoGeneralization); ok {
			if ng.HasOwner() {
				sysRefs = append(sysRefs, "System.owner")
				sysSet["System.owner"] = true
			}
			if ng.HasChangedBy() {
				sysRefs = append(sysRefs, "System.changedBy")
				sysSet["System.changedBy"] = true
			}
		}

		for _, re := range ent.AccessRulesItems() {
			rule, ok := re.(*genDm.AccessRule)
			if !ok {
				continue
			}
			mas := rule.MemberAccessesItems()
			if len(mas) == 0 {
				continue // empty rule: populated at create time, not here
			}
			defRights := rule.DefaultMemberAccessRights()
			if defRights == "" {
				defRights = "ReadWrite"
			}

			covAttr := map[string]bool{}
			covAssoc := map[string]bool{}
			covSys := map[string]bool{}
			changed := false

			// Walk existing entries back-to-front: drop stale, downgrade calc.
			// Removing at index i leaves lower indices valid, so backward is safe.
			for i := len(mas) - 1; i >= 0; i-- {
				ma, ok := mas[i].(*genDm.MemberAccess)
				if !ok {
					continue
				}
				switch attrRef, assocRef := ma.AttributeQualifiedName(), ma.AssociationQualifiedName(); {
				case attrRef != "":
					if attrSet[attrRef] {
						covAttr[attrRef] = true
						if calcSet[attrRef] {
							if r := ma.AccessRights(); r == "ReadWrite" || r == "WriteOnly" {
								ma.SetAccessRights("ReadOnly")
								changed = true
							}
						}
					} else {
						rule.RemoveMemberAccesses(i)
						changed = true
					}
				case assocRef != "":
					switch {
					case sysSet[assocRef]:
						covSys[assocRef] = true
					case assocSet[assocRef]:
						covAssoc[assocRef] = true
					default:
						rule.RemoveMemberAccesses(i)
						changed = true
					}
				}
			}

			// Add missing members, in definition order.
			for _, ai := range attrs {
				if covAttr[ai.qn] {
					continue
				}
				rights := defRights
				if ai.calc && (rights == "ReadWrite" || rights == "WriteOnly") {
					rights = "ReadOnly"
				}
				rule.AddMemberAccesses(newMemberAccess(rights, ai.qn, true))
				changed = true
			}
			for _, qn := range assocQNs {
				if !covAssoc[qn] {
					rule.AddMemberAccesses(newMemberAccess(defRights, qn, false))
					changed = true
				}
			}
			for _, ref := range sysRefs {
				if !covSys[ref] {
					rule.AddMemberAccesses(newMemberAccess(defRights, ref, false))
					changed = true
				}
			}

			if changed {
				modified++
			}
		}
	}

	if modified > 0 {
		if err := b.persistDM(unitID, dm); err != nil {
			return 0, err
		}
	}
	return modified, nil
}

// newMemberAccess builds a fresh MemberAccess (with its own $ID) for either an
// attribute (isAttr=true) or an association reference.
func newMemberAccess(rights, qualifiedName string, isAttr bool) *genDm.MemberAccess {
	ma := genDm.NewMemberAccess()
	ma.SetAccessRights(rights)
	if isAttr {
		ma.SetAttributeQualifiedName(qualifiedName)
	} else {
		ma.SetAssociationQualifiedName(qualifiedName)
	}
	assignID(ma)
	return ma
}
