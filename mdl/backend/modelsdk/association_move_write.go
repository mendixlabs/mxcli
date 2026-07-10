// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genDm "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/meta"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

func init() {
	// A cross-module association carries a GUID (= its own $ID) and an always-null
	// Source (verified against the legacy writer). Note: unlike a regular
	// Association it has NO Parent/Child connection points.
	codec.RegisterTypeDefaults("DomainModels$CrossAssociation", codec.TypeDefaults{
		EmitGUID:   true,
		NullFields: []string{"Source"},
	})
}

// CreateCrossAssociation adds a cross-module association (FROM entity local by-id
// ParentPointer, remote TO entity by-name Child) to a domain model.
func (b *Backend) CreateCrossAssociation(domainModelID model.ID, ca *domainmodel.CrossModuleAssociation) error {
	if ca == nil {
		return fmt.Errorf("CreateCrossAssociation: nil cross association")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateCrossAssociation: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	gca := crossAssocToGen(ca)
	assignCrossAssocIDs(gca)
	dm.AddCrossAssociations(gca)
	return b.persistDM(domainModelID, dm)
}

// crossAssocToGen converts a domainmodel.CrossModuleAssociation to a gen element.
func crossAssocToGen(ca *domainmodel.CrossModuleAssociation) *genDm.CrossAssociation {
	out := genDm.NewCrossAssociation()
	if ca.ID != "" {
		out.SetID(element.ID(ca.ID))
	}
	out.SetName(ca.Name)
	out.SetDocumentation(ca.Documentation)
	out.SetExportLevel("Hidden")
	out.SetParentID(element.ID(string(ca.ParentID)))
	out.SetChildQualifiedName(ca.ChildRef)
	out.SetType(string(ca.Type))
	out.SetOwner(string(ca.Owner))
	sf := string(ca.StorageFormat)
	if sf == "" {
		sf = "Column"
	}
	out.SetStorageFormat(sf)
	out.SetDeleteBehavior(deleteBehaviorToGen(behaviorType(ca.ParentDeleteBehavior), behaviorType(ca.ChildDeleteBehavior)))
	return out
}

// crossAssocFromGenAssoc builds a gen CrossAssociation from a (regular) gen
// Association being converted during a cross-module move. parentID is the local
// FROM entity; childRef is the remote TO entity's qualified name.
func crossAssocFromGenAssoc(a *genDm.Association, parentID, childRef string) *genDm.CrossAssociation {
	out := genDm.NewCrossAssociation()
	out.SetID(a.ID()) // preserve the original association's ID
	out.SetName(a.Name())
	out.SetDocumentation(a.Documentation())
	out.SetExportLevel("Hidden")
	out.SetParentID(element.ID(parentID))
	out.SetChildQualifiedName(childRef)
	out.SetType(a.Type())
	out.SetOwner(a.Owner())
	sf := a.StorageFormat()
	if sf == "" {
		sf = "Column"
	}
	out.SetStorageFormat(sf)
	pdb, cdb := "DeleteMeButKeepReferences", "DeleteMeButKeepReferences"
	if odb, ok := a.DeleteBehavior().(*genDm.AssociationDeleteBehavior); ok {
		if v := odb.ParentDeleteBehavior(); v != "" {
			pdb = v
		}
		if v := odb.ChildDeleteBehavior(); v != "" {
			cdb = v
		}
	}
	out.SetDeleteBehavior(deleteBehaviorToGen(pdb, cdb))
	return out
}

// deleteBehaviorToGen builds an AssociationDeleteBehavior with the given parent/
// child behaviors (its null error-message slots come from the registered default).
func deleteBehaviorToGen(parent, child string) *genDm.AssociationDeleteBehavior {
	db := genDm.NewAssociationDeleteBehavior()
	db.SetParentDeleteBehavior(parent)
	db.SetChildDeleteBehavior(child)
	return db
}

func behaviorType(b *domainmodel.DeleteBehavior) string {
	if b != nil && b.Type != "" {
		return string(b.Type)
	}
	return "DeleteMeButKeepReferences"
}

func assignCrossAssocIDs(ca *genDm.CrossAssociation) {
	assignID(ca)
	assignID(ca.DeleteBehavior())
}

// UpdateEnumerationRefsInAllDomainModels rewrites every enumeration-typed
// attribute that references oldQualifiedName to newQualifiedName, across all
// domain models. Used after a MOVE ENUMERATION so dependent attributes don't
// dangle (CE1613). Mutating the nested type marks the owning entity dirty; the
// codec re-encodes only the touched entities, passing the rest through verbatim.
func (b *Backend) UpdateEnumerationRefsInAllDomainModels(oldQualifiedName, newQualifiedName string) error {
	if b.writer == nil {
		return fmt.Errorf("UpdateEnumerationRefsInAllDomainModels: not connected for writing")
	}
	dms, err := b.ListDomainModels()
	if err != nil {
		return fmt.Errorf("UpdateEnumerationRefsInAllDomainModels: list domain models: %w", err)
	}
	for _, info := range dms {
		// The System module's domain model is virtual (not stored in mprcontents),
		// so it can't be re-loaded from disk — and it never references a user
		// enumeration. Skip it; otherwise loadDomainModelGen fails with a spurious
		// "no such file or directory" warning.
		if string(info.ID) == meta.SystemDomainModelID {
			continue
		}
		gdm, err := b.loadDomainModelGen(info.ID)
		if err != nil {
			return err
		}
		changed := false
		for _, el := range gdm.EntitiesItems() {
			ent, ok := el.(*genDm.Entity)
			if !ok {
				continue
			}
			for _, ael := range ent.AttributesItems() {
				attr, ok := ael.(*genDm.Attribute)
				if !ok {
					continue
				}
				if et, ok := attr.Type().(*genDm.EnumerationAttributeType); ok && et.EnumerationQualifiedName() == oldQualifiedName {
					et.SetEnumerationQualifiedName(newQualifiedName)
					changed = true
				}
			}
		}
		if changed {
			if err := b.persistDM(info.ID, gdm); err != nil {
				return err
			}
		}
	}
	return nil
}

// MoveEntity moves an entity from a source domain model to a target one,
// converting any same-DM associations that reference it into cross-module
// associations (FROM-child stays in source, FROM-parent goes to target), and
// rewriting the entity's view source / validation-rule attribute refs to the new
// module. Mirrors the legacy MoveEntity. Returns the names of converted
// associations as warnings.
func (b *Backend) MoveEntity(entity *domainmodel.Entity, sourceDMID, targetDMID model.ID, sourceModuleName, targetModuleName string) ([]string, error) {
	if entity == nil {
		return nil, fmt.Errorf("MoveEntity: nil entity")
	}
	if b.writer == nil {
		return nil, fmt.Errorf("MoveEntity: not connected for writing")
	}
	sourceDM, err := b.loadDomainModelGen(sourceDMID)
	if err != nil {
		return nil, err
	}
	targetDM, err := b.loadDomainModelGen(targetDMID)
	if err != nil {
		return nil, err
	}

	// Entity-name lookup (before removing the moved entity) for child-side refs.
	nameByID := make(map[string]string)
	for _, el := range sourceDM.EntitiesItems() {
		if e, ok := el.(*genDm.Entity); ok {
			nameByID[string(e.ID())] = e.Name()
		}
	}

	// Remove the moved entity from the source DM.
	removed := false
	for i, el := range sourceDM.EntitiesItems() {
		if string(el.ID()) == string(entity.ID) {
			sourceDM.RemoveEntities(i)
			removed = true
			break
		}
	}
	if !removed {
		return nil, fmt.Errorf("entity not found in source domain model: %s", entity.ID)
	}

	// Convert associations referencing the moved entity to cross-associations.
	var converted []string
	var removeIdx []int
	for i, el := range sourceDM.AssociationsItems() {
		a, ok := el.(*genDm.Association)
		if !ok {
			continue
		}
		parentID, childID := string(a.ParentRefID()), string(a.ChildRefID())
		switch {
		case childID == string(entity.ID): // child moved → cross-assoc stays in source
			sourceDM.AddCrossAssociations(crossAssocFromGenAssoc(a, parentID, targetModuleName+"."+entity.Name))
			removeIdx = append(removeIdx, i)
			converted = append(converted, a.Name())
		case parentID == string(entity.ID): // parent moved → cross-assoc goes to target
			targetDM.AddCrossAssociations(crossAssocFromGenAssoc(a, parentID, sourceModuleName+"."+nameByID[childID]))
			removeIdx = append(removeIdx, i)
			converted = append(converted, a.Name())
		}
	}
	for i := len(removeIdx) - 1; i >= 0; i-- {
		sourceDM.RemoveAssociations(removeIdx[i])
	}

	// Rewrite the moved entity's module-qualified refs (view source + validations).
	oldPrefix, newPrefix := sourceModuleName+".", targetModuleName+"."
	if entity.Source == "DomainModels$OqlViewEntitySource" && strings.HasPrefix(entity.SourceDocumentRef, oldPrefix) {
		entity.SourceDocumentRef = newPrefix + entity.SourceDocumentRef[len(oldPrefix):]
	}
	for _, vr := range entity.ValidationRules {
		if strings.HasPrefix(string(vr.AttributeID), oldPrefix) {
			vr.AttributeID = model.ID(newPrefix + string(vr.AttributeID)[len(oldPrefix):])
		}
	}

	if err := b.persistDM(sourceDMID, sourceDM); err != nil {
		return nil, fmt.Errorf("MoveEntity: persist source: %w", err)
	}

	// Add the (rebuilt) entity to the target DM.
	ge := entityToGen(entity, targetModuleName, b.majorVersion())
	ge.SetID(element.ID(entity.ID))
	assignEntityIDs(ge)
	targetDM.AddEntities(ge)
	if err := b.persistDM(targetDMID, targetDM); err != nil {
		return nil, fmt.Errorf("MoveEntity: persist target: %w", err)
	}
	return converted, nil
}
