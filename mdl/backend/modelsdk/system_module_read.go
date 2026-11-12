// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// buildSystemDomainModel builds the virtual System-module domain model from the
// platform entity definitions in modelsdk/meta. The System module is not stored
// in the project (mprcontents), so without this its entities (WorkflowUserTask,
// User, FileDocument, …) can't be resolved — mirrors the legacy reader's
// BuildSystemModule. Entity IDs are synthetic (System entities are referenced by
// qualified name, never by ID, in serialized output).
//
// It builds the MODELER view: meta also carries the entities that exist only in
// the runtime's metamodel, and a model that names one of those is rejected by
// mxbuild with CE1613 "no longer exists" — upstream #972, where every
// specialization of System.Image inherited an access-rule member for
// System.Thumbnail_Image. See meta.SystemEntityDef.RuntimeOnly.
//
// atLeast is the project version's IsAtLeast, nil when it is unknown: the
// System module differs between versions (ako/mxcli#1810), and a check against
// the wrong one accepts a name the project's build then refuses.
func buildSystemDomainModel(atLeast func(major, minor int) bool) *domainmodel.DomainModel {
	dm := &domainmodel.DomainModel{ContainerID: model.ID(meta.SystemModuleID)}
	dm.ID = model.ID(meta.SystemDomainModelID)
	for _, e := range meta.ModelerSystemEntitiesIn(atLeast) {
		ent := &domainmodel.Entity{
			Name:              e.Name,
			Persistable:       e.Persistable,
			GeneralizationRef: e.Generalization,
			// The stored system members, as Mendix keeps them: on the root's
			// NoGeneralization only. A specialization reads its root's
			// (systemMemberStoredOnChain in the executor).
			HasOwner:       e.HasOwner,
			HasChangedBy:   e.HasChangedBy,
			HasCreatedDate: e.HasCreatedDate,
			HasChangedDate: e.HasChangedDate,
		}
		ent.ID = model.ID("System." + e.Name)
		for _, a := range e.Attributes {
			attr := &domainmodel.Attribute{
				Name: a.Name,
				Type: systemAttrType(a),
			}
			// Synthesize a stable, unique ID per attribute. The System module is
			// virtual (no BSON), so these IDs aren't stored — but the catalog keys
			// attributes_data on Id, so empty IDs collide (UNIQUE constraint on
			// attributes_data.Id). Mirror the entity ID scheme.
			attr.ID = model.ID("System." + e.Name + "." + a.Name)
			ent.Attributes = append(ent.Attributes, attr)
		}
		dm.Entities = append(dm.Entities, ent)
	}
	// Platform associations (UserRoles, Session_User, Workflow_*, …). Without
	// these, SHOW/LIST ASSOCIATIONS and DESCRIBE MODULE System omit every System
	// association on the modelsdk engine (legacy already builds them via
	// BuildSystemDomainModel). IDs are synthetic and match the entity ID scheme
	// so the Parent/Child columns resolve to qualified names.
	for _, a := range meta.ModelerSystemAssociationsIn(atLeast) {
		assoc := &domainmodel.Association{
			Name:     a.Name,
			ParentID: model.ID("System." + a.Parent),
			ChildID:  model.ID("System." + a.Child),
			Type:     domainmodel.AssociationType(a.Type),
			Owner:    domainmodel.AssociationOwner(a.Owner),
		}
		assoc.ID = model.ID("System." + a.Name)
		dm.Associations = append(dm.Associations, assoc)
	}
	return dm
}

// systemAttrType maps a System attribute definition to a domainmodel attribute type.
func systemAttrType(a meta.SystemAttrDef) domainmodel.AttributeType {
	switch a.Type {
	case "Integer":
		return &domainmodel.IntegerAttributeType{}
	case "Long":
		return &domainmodel.LongAttributeType{}
	case "Decimal":
		return &domainmodel.DecimalAttributeType{}
	case "Boolean":
		return &domainmodel.BooleanAttributeType{}
	case "DateTime":
		return &domainmodel.DateTimeAttributeType{LocalizeDate: true}
	case "Enumeration":
		return &domainmodel.EnumerationAttributeType{EnumerationRef: a.EnumQN}
	case "AutoNumber":
		return &domainmodel.AutoNumberAttributeType{}
	case "Binary":
		return &domainmodel.BinaryAttributeType{}
	case "HashedString":
		return &domainmodel.HashedStringAttributeType{}
	default: // String and anything unmapped
		return &domainmodel.StringAttributeType{Length: a.Length}
	}
}

// systemVersionAtLeast is the project version's IsAtLeast, or nil when the
// project version is not known.
func (b *Backend) systemVersionAtLeast() func(major, minor int) bool {
	if b == nil {
		return nil
	}
	pv := b.ProjectVersion()
	if pv == nil || pv.MajorVersion == 0 {
		return nil
	}
	return pv.IsAtLeast
}
