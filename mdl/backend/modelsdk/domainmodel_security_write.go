// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
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

	// Upsert: reuse the rule with the same module-role set AND the same XPath
	// constraint (keeps its ID stable so references stay valid), else add a fresh
	// rule.
	//
	// The constraint is part of the key because Mendix treats several rules
	// naming one module role as legitimate and combines their rights ("Rules are
	// additive", refguide/access-rules) — that is how row-level security is
	// normally written. Matching on the role set alone collapsed two such rules
	// into one and destroyed the first, silently (mendixlabs/mxcli#936). An empty
	// constraint is a value in the key, not a wildcard: an unconstrained rule and
	// a constrained one for the same role are distinct rules.
	var rule *genDm.AccessRule
	for _, el := range ent.AccessRulesItems() {
		r, ok := el.(*genDm.AccessRule)
		if ok && sameStringSet(r.ModuleRolesQualifiedNames(), p.RoleNames) &&
			r.XPathConstraint() == p.XPathConstraint {
			rule = r
			break
		}
	}

	// Everything the matched rule already grants, so the write below widens it
	// rather than replacing it. GRANT is additive by contract; narrowing is what
	// REVOKE is for. Rebuilding from the statement alone was #936: the executor
	// emits an entry for every member of the entity, unnamed ones at the rule's
	// default, so each GRANT used to reset every member it did not mention.
	prior := map[string]string{}
	var priorCreate, priorDelete bool
	priorDefault := ""
	if rule != nil {
		for _, mel := range rule.MemberAccessesItems() {
			ma, ok := mel.(*genDm.MemberAccess)
			if !ok {
				continue
			}
			ref := ma.AttributeQualifiedName()
			if ref == "" {
				ref = ma.AssociationQualifiedName()
			}
			if ref != "" {
				prior[ref] = ma.AccessRights()
			}
		}
		priorCreate, priorDelete = rule.AllowCreate(), rule.AllowDelete()
		priorDefault = rule.DefaultMemberAccessRights()
		for i := len(rule.MemberAccessesItems()) - 1; i >= 0; i-- {
			rule.RemoveMemberAccesses(i)
		}
	} else {
		rule = genDm.NewAccessRule()
		assignID(rule)
		ent.AddAccessRules(rule)
	}

	rule.SetModuleRolesQualifiedNames(p.RoleNames)
	rule.SetAllowCreate(p.AllowCreate || priorCreate)
	rule.SetAllowDelete(p.AllowDelete || priorDelete)
	if def := types.HigherAccessRights(p.DefaultMemberAccess, priorDefault); def != "" {
		rule.SetDefaultMemberAccessRights(def)
	}
	rule.SetXPathConstraint(p.XPathConstraint)
	for _, ma := range p.MemberAccesses {
		m := genDm.NewMemberAccess()
		assignID(m)
		ref := ma.AttributeRef
		if ref == "" {
			ref = ma.AssociationRef
		}
		m.SetAccessRights(types.HigherAccessRights(ma.AccessRights, prior[ref]))
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

	// The rules to narrow are those every role of which the statement names —
	// per role, as RemoveEntityAccessRule has always matched. Requiring the
	// rule's role set to EQUAL the statement's matched nothing when the rules
	// were granted one role each and revoked in one statement, and 0 modified
	// read as "nothing to revoke" while each rule kept the right
	// (mendixlabs/mxcli#1501). A rule shared with a role the statement does not
	// name cannot be narrowed without narrowing that role too, so it is refused
	// before anything is written — but only if the revocation would change it.
	// A shared rule that grants none of the revoked rights narrows nobody and is
	// skipped (ako/mxcli#2209). The dry run and the narrowing are one function,
	// so the check and the write cannot disagree.
	named := toStringSet(roleNames)
	readSet := toStringSet(rev.RevokeReadMembers)
	writeSet := toStringSet(rev.RevokeWriteMembers)
	var targets []*genDm.AccessRule
	for _, el := range ent.AccessRulesItems() {
		rule, ok := el.(*genDm.AccessRule)
		if !ok {
			continue
		}
		var in, out []string
		for _, r := range rule.ModuleRolesQualifiedNames() {
			if named[r] {
				in = append(in, r)
			} else {
				out = append(out, r)
			}
		}
		switch {
		case len(in) == 0:
			continue
		case len(out) > 0:
			if !narrowAccessRule(rule, rev, readSet, writeSet, false) {
				continue
			}
			return 0, fmt.Errorf("the access rule on %s for %s also grants %s, which this revoke does not name; "+
				"revoke from all of the rule's roles, or grant the roles in separate rules first",
				entityName, strings.Join(rule.ModuleRolesQualifiedNames(), ", "), strings.Join(out, ", "))
		}
		targets = append(targets, rule)
	}

	modified := 0
	for _, rule := range targets {
		if narrowAccessRule(rule, rev, readSet, writeSet, true) {
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

// narrowAccessRule reports whether rev would change rule, and applies the
// change when apply is set. One function for both, so the shared-rule check in
// RevokeEntityMemberAccess and the narrowing itself cannot disagree (#2209).
func narrowAccessRule(rule *genDm.AccessRule, rev types.EntityAccessRevocation, readSet, writeSet map[string]bool, apply bool) bool {
	changed := false
	if rev.RevokeCreate && rule.AllowCreate() {
		if apply {
			rule.SetAllowCreate(false)
		}
		changed = true
	}
	if rev.RevokeDelete && rule.AllowDelete() {
		if apply {
			rule.SetAllowDelete(false)
		}
		changed = true
	}
	switch cur := rule.DefaultMemberAccessRights(); {
	case rev.RevokeReadAll && cur != "None":
		if apply {
			rule.SetDefaultMemberAccessRights("None")
		}
		changed = true
	case rev.RevokeWriteAll && cur == "ReadWrite":
		if apply {
			rule.SetDefaultMemberAccessRights("ReadOnly")
		}
		changed = true
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
			if apply {
				ma.SetAccessRights(newRights)
			}
			changed = true
		}
	}
	return changed
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

// isAuditMemberRef reports whether a member reference names one of the two audit
// ASSOCIATIONS Mendix maintains from an entity's own flags.
//
// Exactly these two, not every System.* reference: an entity specialising a
// System entity legitimately inherits that module's real associations, and those
// are preserved by the foreign-module branch.
func isAuditMemberRef(ref string) bool {
	return ref == "System.owner" || ref == "System.changedBy"
}

// ReconcileMemberAccesses brings every populated access rule in a domain model
// into sync with its entity's current members: it adds a MemberAccess for each
// attribute, each FROM-side association (regular + cross), and each implicit
// system association (System.owner / System.changedBy); removes stale entries
// for members that no longer exist; and downgrades write rights on the members
// that may not carry them — calculated attributes AND autonumbers, both CE6592.
// It mirrors the legacy writer's reconcile and is invoked by the executor's
// finalize step after every program run, which is why the autonumber half
// mattered here as much as at the GRANT: a correct grant was re-broken by the
// next write touching the module (ako/mxcli#524).
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

	byName := entitiesByName(dm)

	// Cross-module associations declared ELSEWHERE whose TO end is an entity of
	// this module. Under `OWNER Both` such an association is a member of that
	// entity too, and nothing in this domain model says so (ako/mxcli#802).
	// The same scan records what every other module declares, which is what
	// lets a foreign reference be judged rather than kept on faith.
	elsewhere, err := b.scanProjectAssociations(moduleName)
	if err != nil {
		return 0, err
	}
	incoming := elsewhere.incoming

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

		// A specialization has every member of its generalization, associations
		// included, so its access rule needs an entry for each of them. Only the
		// part of the chain that lives in THIS module can be walked; an ancestor
		// in another module is handled by preserving its references below rather
		// than by resolving them.
		ancestors := sameModuleAncestors(ent, byName, moduleName)
		ownerIDs := map[string]bool{entityID: true}
		for _, anc := range ancestors {
			ownerIDs[string(anc.ID())] = true
		}

		// Attributes (in order) with calculated flags — the entity's OWN and
		// those it INHERITS from a generalization in this module.
		//
		// The ancestor walk above used to feed the ASSOCIATION pass only, so a
		// specialization's expected attribute set was its own attributes and
		// nothing else. Adding an attribute to a generalization therefore left
		// every specialization's rule short of a member, which Mendix reports as
		// CE0066 "Entity access is out of date" — and `UPDATE SECURITY`, the
		// command that exists to repair it, found nothing missing, reported 0
		// modified, and printed "All entity access rules are up to date" over a
		// project mx check rejects (mendixlabs/mxcli#1047, reported against
		// 0.21.0 and reproduced on both engines).
		//
		// Each reference is qualified against the entity that DECLARES the
		// member, which is what Mendix stores; qualifying an inherited one
		// against this entity is CE1613 "The selected attribute no longer
		// exists". A child attribute SHADOWS an ancestor's of the same name, as
		// the executor's own member walk (EntityMembersFor) already treats it —
		// emitting both would put two entries in the rule for one member.
		//
		// An ancestor in ANOTHER module is still not resolvable here, so its
		// members are neither added nor pruned: the existing entries are carried
		// through by the "preserve what cannot be checked" branch below. For the
		// same reason a stale inherited entry — one whose ancestor has since
		// dropped the attribute — is still preserved rather than removed; that is
		// the opposite direction from this defect and #1047's own control reports
		// "+1 added, -0 removed".
		// noWrite folds the two CE6592 causes — calculated and autonumber —
		// into the one question the downgrades below ask. Keeping them apart
		// here is what let the autonumber half go missing (ako/mxcli#524).
		type attrInfo struct {
			qn      string
			noWrite bool
		}
		var attrs []attrInfo
		attrSet := map[string]bool{}
		noWriteSet := map[string]bool{}
		claimed := map[string]bool{}
		collectAttrs := func(owner *genDm.Entity, ownerName string) {
			for _, ae := range owner.AttributesItems() {
				a, ok := ae.(*genDm.Attribute)
				if !ok {
					continue
				}
				if claimed[a.Name()] {
					continue // shadowed by a nearer entity in the chain
				}
				claimed[a.Name()] = true
				qn := moduleName + "." + ownerName + "." + a.Name()
				_, isCalc := a.Value().(*genDm.CalculatedValue)
				_, isAuto := a.Type().(*genDm.AutoNumberAttributeType)
				noWrite := types.WriteRightsForbidden(isCalc, isAuto, meta.SystemAttributeWriteForbidden(qn))
				attrs = append(attrs, attrInfo{qn, noWrite})
				attrSet[qn] = true
				if noWrite {
					noWriteSet[qn] = true
				}
			}
		}
		collectAttrs(ent, entityName)
		for _, anc := range ancestors { // nearest first, so the nearer shadows
			collectAttrs(anc, anc.Name())
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
			a, ok := ae.(*genDm.Association)
			if !ok {
				continue
			}
			// `OWNER Both` makes the association a member of BOTH ends, so the TO
			// entity's rule needs an entry too. Reconciling on the FROM side alone
			// stripped it back out on every run — including the one the executor
			// had just added — leaving the TO entity's rule incomplete, which
			// Mendix reports as CE0066 "Entity access is out of date"
			// (issuetracker #20). Verified on mxbuild 11.12.1: the same model with
			// `OWNER Default` checks clean, so the owner mode is the trigger.
			if ownerIDs[string(a.ParentRefID())] ||
				(a.Owner() == "Both" && ownerIDs[string(a.ChildRefID())]) {
				addAssoc(a.Name())
			}
		}
		for _, ce := range dm.CrossAssociationsItems() {
			if ca, ok := ce.(*genDm.CrossAssociation); ok && ownerIDs[string(ca.ParentRefID())] {
				addAssoc(ca.Name())
			}
		}
		// The TO side of another module's cross-module associations: an entry
		// under `OWNER Both`, and none otherwise. The reference is qualified by
		// the DECLARING module, so it is foreign here and the preserve branch
		// below would keep a stale one forever — which is why the non-Both ones
		// are collected too, as the entries to remove. An association declared
		// by a generalization in that same module may reach this entity through
		// inheritance instead, and cannot be judged from here, so it is left to
		// the preserve branch as before.
		foreignAnc := foreignAncestorModule(ent, ancestors, moduleName)
		staleForeign := map[string]bool{}
		for _, name := range append([]string{entityName}, entityNames(ancestors)...) {
			for _, in := range incoming[name] {
				switch {
				case in.both:
					if !assocSet[in.qn] {
						assocSet[in.qn] = true
						assocQNs = append(assocQNs, in.qn)
					}
				case !strings.EqualFold(in.module, foreignAnc):
					staleForeign[in.qn] = true
				}
			}
		}
		for qn := range assocSet {
			delete(staleForeign, qn)
		}

		// NO MemberAccess is written for an audit member, of either kind.
		//
		// The DATE members (createdDate/changedDate) were measured first: an entity
		// storing them checks clean with no entry, and mxbuild rejects a rule that
		// carries one with CE0066 (issuetracker #20, verified on 11.12.1).
		//
		// System.owner and System.changedBy were assumed to be the other case,
		// because they are associations rather than attributes — Mendix does add
		// them implicitly, so an entry for them looked required by symmetry. It is
		// not. Measured on mxbuild 11.14.0, one entity, one rule, one variable:
		//
		//	AutoOwner     + MemberAccess System.owner       CE0066
		//	AutoOwner     + no entry                        0 errors
		//	AutoChangedBy + MemberAccess System.changedBy   CE0066
		//	AutoChangedBy + no entry                        0 errors
		//
		// So all four audit members follow one rule: they are members Mendix
		// manages, and naming one in an access rule makes the rule out of date
		// (ako/mxcli#554). Nothing is added here, and a stored entry is removed
		// below rather than preserved — `update security` is the documented repair
		// for CE0066, so it has to be able to undo this.

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
					switch {
					case attrSet[attrRef]:
						covAttr[attrRef] = true
						if noWriteSet[attrRef] {
							if r := ma.AccessRights(); r == "ReadWrite" || r == "WriteOnly" {
								ma.SetAccessRights("ReadOnly")
								changed = true
							}
						}
					case !attrRefBelongsTo(attrRef, moduleName, entityName):
						// An attribute reference is qualified against the entity that
						// DECLARES it, so an inherited member carries an ancestor's name
						// rather than this entity's. attrSet holds only this entity's own
						// attributes, so every inherited entry looked stale and was
						// deleted — silently, on any write touching the module, and
						// immediately after the GRANT that had just written it correctly
						// (mendixlabs/mxcli#758). The ancestor may live in another module
						// or in System, neither loaded here, so an inherited reference
						// cannot be validated at this layer at all. Preserve what cannot
						// be checked instead of dropping it.
						covAttr[attrRef] = true
						// A System attribute IS known without loading anything, and two
						// refuse write rights (CE6592): a rule written before that was
						// known carries HasContents ReadWrite, and `update security`
						// is the repair, so it downgrades here.
						if meta.SystemAttributeWriteForbidden(attrRef) {
							if r := ma.AccessRights(); r == "ReadWrite" || r == "WriteOnly" {
								ma.SetAccessRights("ReadOnly")
								changed = true
							}
						}
					default:
						// Genuinely stale: the reference claims to be this entity's own
						// attribute and the entity no longer has it.
						rule.RemoveMemberAccesses(i)
						changed = true
					}
				case assocRef != "":
					switch {
					case isAuditMemberRef(assocRef):
						// An audit member Mendix manages itself. Removed rather than
						// preserved by the foreign-module branch below, which would
						// otherwise keep it forever on the grounds that System is not
						// loaded here (ako/mxcli#554).
						rule.RemoveMemberAccesses(i)
						changed = true
					case assocSet[assocRef]:
						covAssoc[assocRef] = true
					case staleForeign[assocRef]:
						// Another module's cross-module association to this entity
						// whose owner is no longer Both (ako/mxcli#802).
						rule.RemoveMemberAccesses(i)
						changed = true
					case !assocRefBelongsTo(assocRef, moduleName) &&
						elsewhere.staleForeignRef(assocRef, foreignAnc):
						// A foreign reference that is positively stale: its module no
						// longer declares it (CE1613), or this entity cannot be one of
						// its members (CE0066). See staleForeignRef. Kept before
						// ako/mxcli#1449 / #819, so `drop entity` on the FROM end of a
						// cross-module OWNER Both association, `drop association` on
						// one, and moving its TO end all left the entry for good — and
						// `update security` reported nothing to do.
						rule.RemoveMemberAccesses(i)
						changed = true
					case !assocRefBelongsTo(assocRef, moduleName):
						// An association is qualified by the module that DECLARES it, so
						// one inherited from a generalization in another module names
						// that module. Whether this entity inherits it cannot be decided
						// from here — preserve it, as the attribute branch above does,
						// rather than delete what cannot be checked. Without this, every
						// rule on a specialization of a marketplace entity lost its
						// inherited associations.
						covAssoc[assocRef] = true
					default:
						// Genuinely stale: an association of this module that no longer
						// has this entity (or an ancestor of it) on its FROM side.
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
				if ai.noWrite && (rights == "ReadWrite" || rights == "WriteOnly") {
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

// entitiesByName indexes a domain model's entities by name, for resolving a
// generalization's qualified name within the same module.
func entitiesByName(dm *genDm.DomainModel) map[string]*genDm.Entity {
	out := map[string]*genDm.Entity{}
	for _, el := range dm.EntitiesItems() {
		if e, ok := el.(*genDm.Entity); ok && e.Name() != "" {
			out[e.Name()] = e
		}
	}
	return out
}

// sameModuleAncestors walks an entity's generalization chain and returns the
// ancestors that live in this module, nearest first.
//
// The walk stops at the first ancestor it cannot resolve — one in another module
// (or in System). That is not a failure: an association declared by such an
// ancestor is qualified with the ancestor's module, so it is recognised by
// assocRefBelongsTo rather than by being found here.
func sameModuleAncestors(ent *genDm.Entity, byName map[string]*genDm.Entity, moduleName string) []*genDm.Entity {
	var out []*genDm.Entity
	seen := map[string]bool{string(ent.ID()): true}
	for cur := ent; ; {
		gen, ok := cur.Generalization().(*genDm.Generalization)
		if !ok {
			return out
		}
		qn := gen.GeneralizationQualifiedName()
		idx := strings.LastIndex(qn, ".")
		if idx < 0 || !strings.EqualFold(qn[:idx], moduleName) {
			return out
		}
		anc, found := byName[qn[idx+1:]]
		if !found || seen[string(anc.ID())] {
			return out // unresolvable, or a cycle a corrupt model could contain
		}
		seen[string(anc.ID())] = true
		out = append(out, anc)
		cur = anc
	}
}

// incomingCross is a cross-module association declared in another module whose
// TO end is an entity of the module being reconciled.
type incomingCross struct {
	qn     string // "DeclaringModule.Association", as a MemberAccess names it
	module string // the declaring module
	both   bool   // OWNER Both: a member of the TO entity as well
}

// projectAssociations is what a reconcile of one module needs to know about the
// associations every other module declares.
type projectAssociations struct {
	// incoming maps each entity name of the module being reconciled to the
	// cross-module associations other modules declare TO it.
	incoming map[string][]incomingCross
	// declared maps every module with a stored domain model (lower-cased) to the
	// lower-cased names of the associations, regular and cross-module, it
	// declares. System has no stored domain model and is absent.
	declared map[string]map[string]bool
}

// scanProjectAssociations reads every domain model once. A CrossAssociation
// names its TO entity by qualified name, so the incoming half is a scan of every
// other domain model — the only place the fact is stored.
func (b *Backend) scanProjectAssociations(moduleName string) (*projectAssociations, error) {
	units, err := listUnits[*genDm.DomainModel](b)
	if err != nil {
		return nil, fmt.Errorf("list domain models: %w", err)
	}
	mods, err := b.reader.ListModules()
	if err != nil {
		return nil, fmt.Errorf("list modules: %w", err)
	}
	moduleNameOf := make(map[model.ID]string, len(mods))
	for _, m := range mods {
		moduleNameOf[model.ID(m.ID)] = m.Name
	}
	prefix := moduleName + "."
	out := &projectAssociations{incoming: map[string][]incomingCross{}, declared: map[string]map[string]bool{}}
	for _, u := range units {
		declaring := moduleNameOf[u.ContainerID]
		if declaring == "" {
			continue // an orphaned domain model declares nothing reachable
		}
		names := out.declared[strings.ToLower(declaring)]
		if names == nil {
			names = map[string]bool{}
			out.declared[strings.ToLower(declaring)] = names
		}
		for _, ae := range u.Element.AssociationsItems() {
			if a, ok := ae.(*genDm.Association); ok && a.Name() != "" {
				names[strings.ToLower(a.Name())] = true
			}
		}
		for _, ce := range u.Element.CrossAssociationsItems() {
			ca, ok := ce.(*genDm.CrossAssociation)
			if !ok || ca.Name() == "" {
				continue
			}
			names[strings.ToLower(ca.Name())] = true
			if strings.EqualFold(declaring, moduleName) {
				continue
			}
			child := ca.ChildQualifiedName()
			if len(child) <= len(prefix) || !strings.EqualFold(child[:len(prefix)], prefix) {
				continue
			}
			name := child[len(prefix):]
			out.incoming[name] = append(out.incoming[name], incomingCross{
				qn: declaring + "." + ca.Name(), module: declaring, both: ca.Owner() == "Both",
			})
		}
	}
	return out, nil
}

// staleForeignRef reports whether a MemberAccess naming an association declared
// in ANOTHER module is positively stale on an entity of the module being
// reconciled. foreignAnc is foreignAncestorModule's answer for that entity: ""
// when its whole generalization chain lies in its own module.
//
// The reconcile used to keep every such reference, because the one it was
// written for — an association inherited from a generalization in another module
// — cannot be validated without walking that chain. Two cases can be decided
// without it, and both were reported (ako/mxcli#1449, ako/mxcli#819):
//
//   - The declaring module exists and no longer declares the association. It is a
//     member of nothing; mx check reports CE1613 "The selected association … no
//     longer exists". This is what `drop entity` on the FROM end of a cross-module
//     OWNER Both association, or `drop association` on one, leaves on the TO end.
//   - The association still exists, but the entity's generalization chain never
//     leaves its module. Then the only way a foreign association is a member of it
//     is as the TO end under OWNER Both, which `incoming` enumerates — so one the
//     reconcile did not collect is not a member: CE0066 "Entity access is out of
//     date". This is what moving the TO end of such an association leaves behind.
//
// A reference into a module the project does not store — System, whose domain
// model is virtual, or one that is simply absent — is still preserved, and so is
// an existing association on an entity that inherits from another module.
func (p *projectAssociations) staleForeignRef(ref, foreignAnc string) bool {
	idx := strings.LastIndex(ref, ".")
	if idx < 0 {
		return false
	}
	names, known := p.declared[strings.ToLower(ref[:idx])]
	if !known {
		return false
	}
	if !names[strings.ToLower(ref[idx+1:])] {
		return true
	}
	return foreignAnc == ""
}

// foreignAncestorModule returns the module of the first generalization in ent's
// chain that lies outside moduleName, or "" when the chain stays inside it.
// ancestors is sameModuleAncestors(ent, …), nearest first.
func foreignAncestorModule(ent *genDm.Entity, ancestors []*genDm.Entity, moduleName string) string {
	last := ent
	if len(ancestors) > 0 {
		last = ancestors[len(ancestors)-1]
	}
	gen, ok := last.Generalization().(*genDm.Generalization)
	if !ok {
		return ""
	}
	qn := gen.GeneralizationQualifiedName()
	idx := strings.LastIndex(qn, ".")
	if idx < 0 || strings.EqualFold(qn[:idx], moduleName) {
		return ""
	}
	return qn[:idx]
}

func entityNames(es []*genDm.Entity) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

// assocRefBelongsTo reports whether a MemberAccess association reference
// ("Module.Association") is declared in the given module, and so can be checked
// against the domain model at hand.
func assocRefBelongsTo(assocRef, moduleName string) bool {
	idx := strings.LastIndex(assocRef, ".")
	if idx < 0 {
		return false
	}
	return strings.EqualFold(assocRef[:idx], moduleName)
}

// attrRefBelongsTo reports whether a MemberAccess attribute reference
// ("Module.Entity.Attribute") names one of the given entity's OWN attributes,
// rather than one inherited from an ancestor.
//
// Only an own reference can be validated from a single domain model: an ancestor
// may live in another module or in System, neither of which is loaded here.
func attrRefBelongsTo(attrRef, moduleName, entityName string) bool {
	idx := strings.LastIndex(attrRef, ".")
	if idx < 0 {
		return false
	}
	return strings.EqualFold(attrRef[:idx], moduleName+"."+entityName)
}
