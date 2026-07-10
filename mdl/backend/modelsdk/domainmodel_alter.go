// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genDm "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/meta"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// persistDM re-encodes a (mutated) domain model and writes it back to its unit.
// The codec passes unchanged children through their original raw bytes, so only
// the elements actually mutated are rebuilt — the rest stay byte-faithful to
// what Studio Pro wrote.
func (b *Backend) persistDM(domainModelID model.ID, dm *genDm.DomainModel) error {
	enc := &codec.Encoder{}
	contents, err := enc.Encode(dm)
	if err != nil {
		return fmt.Errorf("encode domain model %s: %w", domainModelID, err)
	}
	if err := b.writer.UpdateRawUnit(string(domainModelID), contents); err != nil {
		return fmt.Errorf("persist domain model %s: %w", domainModelID, err)
	}
	return nil
}

// findGenEntity returns the gen entity with the given ID, or nil.
func findGenEntity(dm *genDm.DomainModel, entityID model.ID) *genDm.Entity {
	for _, el := range dm.EntitiesItems() {
		if string(el.ID()) == string(entityID) {
			if e, ok := el.(*genDm.Entity); ok {
				return e
			}
		}
	}
	return nil
}

// removeAssocsReferencing drops every regular association in dm whose FROM
// (ParentPointer) or TO (ChildPointer) endpoint is entityID. Returns whether
// anything was removed. Iterates back-to-front so removal indices stay valid.
func removeAssocsReferencing(dm *genDm.DomainModel, entityID model.ID) bool {
	changed := false
	items := dm.AssociationsItems()
	for i := len(items) - 1; i >= 0; i-- {
		a, ok := items[i].(*genDm.Association)
		if !ok {
			continue
		}
		if string(a.ParentRefID()) == string(entityID) || string(a.ChildRefID()) == string(entityID) {
			dm.RemoveAssociations(i)
			changed = true
		}
	}
	return changed
}

// DeleteAttribute removes an attribute from an entity. The remaining attributes
// pass through the codec unchanged; only the Attributes list is rebuilt. Mirrors
// legacy semantics (no cascade — dangling index/validation refs are left as-is,
// same as the legacy writer).
func (b *Backend) DeleteAttribute(domainModelID, entityID, attrID model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteAttribute: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	ent := findGenEntity(dm, entityID)
	if ent == nil {
		return fmt.Errorf("entity not found: %s", entityID)
	}
	idx := -1
	for i, el := range ent.AttributesItems() {
		if string(el.ID()) == string(attrID) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("attribute not found: %s", attrID)
	}
	ent.RemoveAttributes(idx)
	return b.persistDM(domainModelID, dm)
}

// UpdateEntity replaces an entity with the fully-modified domainmodel.Entity the
// executor passes (the executor routes every ALTER ENTITY op — rename, doc, add/
// modify/drop attribute, generalization, index — through here). The entity keeps
// its position: the entities list is rebuilt in order with the target swapped for
// a freshly-converted gen entity, while every other entity passes through its
// original raw bytes. Mirrors legacy UpdateEntity (full re-serialize of the
// replaced entity, siblings untouched).
func (b *Backend) UpdateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	if entity == nil {
		return fmt.Errorf("UpdateEntity: nil entity")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateEntity: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	order := dm.EntitiesItems()
	orig := findGenEntity(dm, entity.ID)
	if orig == nil {
		return fmt.Errorf("entity not found: %s", entity.ID)
	}

	ge := entityToGen(entity, b.moduleNameFor(domainModelID), b.majorVersion())
	ge.SetID(element.ID(entity.ID))
	assignEntityIDs(ge)

	// Carry the original raw bytes onto the rebuilt target so the codec treats it
	// as an EXISTING element: dirty (re-encoded) properties come from ge, while
	// unmodeled fields — notably the entity GUID — pass through verbatim from raw.
	// Without this, entityToGen produces a fresh (raw==nil) element and the codec
	// emits GUID = $ID (EmitGUID default), discarding the on-disk GUID. That GUID
	// is the entity's stable cross-reference identity: pages/grids and inheriting
	// entities in other modules resolve members through it, so changing it dangles
	// those references (CE1613 — GitHub issue #657). Siblings are untouched (raw
	// passthrough in the list rebuild below), so they already keep their GUID; this
	// closes the same gap for the ALTER target itself.
	if raw := orig.Raw(); raw != nil {
		ge.SetRaw(raw)
	}

	// Rebuild the list in place: drop all, re-add in original order swapping the
	// target. Re-added existing elements stay clean (only the list is dirtied),
	// so the codec re-emits them byte-faithfully; only ge is built fresh.
	for i := len(order) - 1; i >= 0; i-- {
		dm.RemoveEntities(i)
	}
	for _, el := range order {
		if string(el.ID()) == string(entity.ID) {
			dm.AddEntities(ge)
		} else {
			dm.AddEntities(el)
		}
	}
	return b.persistDM(domainModelID, dm)
}

// UpdateDomainModel persists a whole mutated domain model (the executor's
// read-modify-write path for ALTER ASSOCIATION, CREATE OR MODIFY ASSOCIATION,
// and RENAME). It rebuilds the Entities and Associations lists from the semantic
// model via the byte-faithful converters, preserving each element's identity.
// CrossAssociations and Annotations are NOT represented in domainmodel.DomainModel,
// so they are left as gen passthrough rather than dropped (ADR-0005: guard
// fidelity — the existing raw bytes carry forward unchanged).
func (b *Backend) UpdateDomainModel(dm *domainmodel.DomainModel) error {
	if dm == nil {
		return fmt.Errorf("UpdateDomainModel: nil domain model")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateDomainModel: not connected for writing")
	}
	gdm, err := b.loadDomainModelGen(dm.ID)
	if err != nil {
		return err
	}
	moduleName := b.moduleNameFor(dm.ID)
	major := b.majorVersion()

	for i := len(gdm.EntitiesItems()) - 1; i >= 0; i-- {
		gdm.RemoveEntities(i)
	}
	for _, e := range dm.Entities {
		ge := entityToGen(e, moduleName, major)
		ge.SetID(element.ID(e.ID))
		assignEntityIDs(ge)
		gdm.AddEntities(ge)
	}

	for i := len(gdm.AssociationsItems()) - 1; i >= 0; i-- {
		gdm.RemoveAssociations(i)
	}
	for _, a := range dm.Associations {
		ga := assocToGen(a)
		if a.ID != "" {
			ga.SetID(element.ID(a.ID))
		}
		assignAssociationIDs(ga)
		gdm.AddAssociations(ga)
	}

	return b.persistDM(dm.ID, gdm)
}

// DeleteAssociation removes an association from a domain model by ID. Used by
// DROP ASSOCIATION and by the executor's CREATE OR MODIFY ASSOCIATION (delete +
// recreate) path.
func (b *Backend) DeleteAssociation(domainModelID, assocID model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteAssociation: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	for i, el := range dm.AssociationsItems() {
		if string(el.ID()) == string(assocID) {
			dm.RemoveAssociations(i)
			return b.persistDM(domainModelID, dm)
		}
	}
	return fmt.Errorf("association not found: %s", assocID)
}

// DeleteCrossAssociation removes a cross-module association from a domain model by ID.
func (b *Backend) DeleteCrossAssociation(domainModelID, assocID model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteCrossAssociation: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	for i, el := range dm.CrossAssociationsItems() {
		if string(el.ID()) == string(assocID) {
			dm.RemoveCrossAssociations(i)
			return b.persistDM(domainModelID, dm)
		}
	}
	return fmt.Errorf("cross association not found: %s", assocID)
}

// DeleteEntity removes an entity and cascades association cleanup: associations
// in the same DM and in every other DM that reference the entity (by
// ParentPointer = FROM or ChildPointer = TO) are removed. Mirrors legacy
// DeleteEntity, including the cross-module cascade.
func (b *Backend) DeleteEntity(domainModelID, entityID model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteEntity: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	eidx := -1
	for i, el := range dm.EntitiesItems() {
		if string(el.ID()) == string(entityID) {
			eidx = i
			break
		}
	}
	if eidx < 0 {
		return fmt.Errorf("entity not found: %s", entityID)
	}
	dm.RemoveEntities(eidx)
	removeAssocsReferencing(dm, entityID)
	if err := b.persistDM(domainModelID, dm); err != nil {
		return err
	}

	// Cascade: remove associations referencing this entity from all other DMs.
	allDMs, err := b.ListDomainModels()
	if err != nil {
		return fmt.Errorf("DeleteEntity: cascade cleanup: list domain models: %w", err)
	}
	for _, other := range allDMs {
		if other.ID == domainModelID {
			continue
		}
		// The virtual System domain model has no on-disk unit and is immutable —
		// skip it (ListDomainModels injects it for entity resolution).
		if string(other.ID) == meta.SystemDomainModelID {
			continue
		}
		odm, err := b.loadDomainModelGen(other.ID)
		if err != nil {
			return fmt.Errorf("DeleteEntity: cascade cleanup: load %s: %w", other.ID, err)
		}
		if removeAssocsReferencing(odm, entityID) {
			if err := b.persistDM(other.ID, odm); err != nil {
				return fmt.Errorf("DeleteEntity: cascade cleanup: update %s: %w", other.ID, err)
			}
		}
	}
	return nil
}
