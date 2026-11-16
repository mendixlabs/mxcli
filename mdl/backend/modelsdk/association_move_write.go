// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
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
	out.SetExportLevel(orDefault(ca.ExportLevel, "Hidden"))
	out.SetParentID(element.ID(string(ca.ParentID)))
	out.SetChildQualifiedName(ca.ChildRef)
	out.SetType(string(ca.Type))
	out.SetOwner(string(ca.Owner))
	sf := string(ca.StorageFormat)
	if sf == "" {
		sf = "Column"
	}
	out.SetStorageFormat(sf)
	db := deleteBehaviorToGen(behaviorType(ca.ParentDeleteBehavior), behaviorType(ca.ChildDeleteBehavior))
	// A restrict ("delete me if no references") child side needs its refusal
	// message, exactly as assocToGen writes it: a null one stops the runtime
	// starting (CapTrackV2 §1). Only the update path got it in #795; the create
	// path — CreateCrossAssociation — wrote null (ako/mxcli#803).
	patchCrossDeleteErrorMessage(db, ca.ChildDeleteBehavior)
	out.SetDeleteBehavior(db)
	if ca.Source == domainmodel.OqlViewAssociationSource {
		out.SetSource(oqlViewAssociationSourceToGen(ca.ViewSourceReference))
	}
	return out
}

// crossAssocFromGenAssoc converts a (regular) gen Association into the
// CrossAssociation a cross-module move turns it into. parentID is the local FROM
// entity; childRef is the remote TO entity's qualified name.
//
// It prefers a RAW transform of the stored document (crossAssocRawFromAssoc),
// because the conversion is a re-addressing of the same association and every
// property it does not touch should survive byte-for-byte — the GUID above all.
// The runtime keys the database on that GUID, and re-minting it here is what made
// the #1119 write guard refuse MOVE ENTITY for any entity in an association
// (ako/mxcli#503).
//
// The property-by-property build below is the fallback for an association with no
// stored bytes — one created earlier in the same session and moved before it was
// ever persisted. It is also the cautionary case: a hand-maintained copy list
// silently drops whatever nobody thought to add, which is how it lost the view
// source once (see the Source arm) and the GUID until #503.
func crossAssocFromGenAssoc(a *genDm.Association, parentID, childRef string) *genDm.CrossAssociation {
	if raw, ok := crossAssocRawFromAssoc(a, childRef); ok {
		out := genDm.NewCrossAssociation()
		// Clean element: SetRaw + InitFromRaw bind the properties without dirtying
		// any, so the encoder's existing-element path passes the whole document
		// through verbatim. The registered EmitGUID/NullFields defaults apply only
		// to a fresh (raw == nil) element, so nothing is appended on top.
		out.SetRaw(raw)
		out.InitFromRaw(raw)
		out.SetID(a.ID())
		return out
	}

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
	// Moving the target entity to another module converts the association to a
	// CrossAssociation. A view entity's Source has to survive that conversion, or
	// the move — which never mentioned the association — silently breaks the
	// build (CE6771).
	if src, ok := a.Source().(*genDm.OqlViewAssociationSource); ok && src != nil {
		out.SetSource(oqlViewAssociationSourceToGen(src.Reference()))
	}
	return out
}

// crossAssocRawFromAssoc rewrites a stored DomainModels$Association document as a
// DomainModels$CrossAssociation one. Three edits, and everything else passes
// through untouched:
//
//   - $Type becomes DomainModels$CrossAssociation.
//   - ChildPointer (a 16-byte element id, only resolvable inside one unit) becomes
//     Child, the target entity's qualified name.
//   - ChildConnection and ParentConnection are dropped. They are the association
//     line's on-canvas waypoints, and a cross-module association has no line to
//     draw to the other module.
//
// That difference is not guessed: it is the whole difference between the two types
// in `generated/metamodel` — the arbiter when the two generated sources disagree —
// whose DomainModelsCrossAssociation declares Child, GUID, DeleteBehavior,
// Documentation, ExportLevel, Name, Owner, ParentPointer, Source, StorageFormat
// and Type, and DomainModelsAssociation the same set with ChildPointer in place of
// Child plus the two connection points. Verified against the stored key set of a
// real association: exactly those thirteen keys plus $ID and $Type.
//
// ParentPointer is kept verbatim in both move directions. When the CHILD moves the
// cross-association stays in the source unit beside its unchanged parent; when the
// PARENT moves it travels to the target unit with it. Either way the id it holds
// still resolves in the unit the document ends up in.
//
// Key ORDER follows the stored association rather than any reference
// cross-association, because there is no Studio Pro-authored one to pin against
// here. That is sound rather than a gap: mxcli already writes cross-associations
// in gen-property order today and they load, so the order of properties is not
// something Mendix's reader depends on.
//
// Returns false when the association has no stored bytes, which is the one case
// the caller must build from properties instead.
func crossAssocRawFromAssoc(a *genDm.Association, childRef string) (bson.Raw, bool) {
	raw := a.Raw()
	if raw == nil {
		return nil, false
	}
	elems, err := bsoncore.Document(raw).Elements()
	if err != nil {
		return nil, false
	}

	out := make(bson.D, 0, len(elems))
	child := false
	for _, e := range elems {
		switch e.Key() {
		case "$Type":
			out = append(out, bson.E{Key: "$Type", Value: "DomainModels$CrossAssociation"})
		case "ChildPointer":
			out = append(out, bson.E{Key: "Child", Value: childRef})
			child = true
		case "ChildConnection", "ParentConnection":
			// No line to the other module; the type declares neither.
		default:
			v := e.Value()
			out = append(out, bson.E{
				Key:   e.Key(),
				Value: bson.RawValue{Type: bson.Type(v.Type), Value: v.Data},
			})
		}
	}

	// Child is mandatory on the target type (no omitempty in the metamodel), so a
	// source document without a ChildPointer to rename would produce a document
	// missing it. Hand that case to the property build rather than emit one.
	if !child {
		return nil, false
	}

	b, err := bson.Marshal(out)
	if err != nil {
		return nil, false
	}
	return bson.Raw(b), true
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
// rewriting the entity's view source, validation-rule attribute refs and
// access-rule member refs to the new module.
//
// Returns one entry per converted association, carrying the qualified name it had
// and the one it has afterwards, so the caller can sweep references from the names
// instead of deriving them — the two move directions differ (see MovedAssociation).
func (b *Backend) MoveEntity(entity *domainmodel.Entity, sourceDMID, targetDMID model.ID, sourceModuleName, targetModuleName string) ([]types.MovedAssociation, error) {
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

	// Remove the moved entity from the source DM, keeping the stored element so
	// its identities can be carried onto the rebuild that lands in the target.
	var orig *genDm.Entity
	for i, el := range sourceDM.EntitiesItems() {
		if string(el.ID()) != string(entity.ID) {
			continue
		}
		orig, _ = el.(*genDm.Entity)
		sourceDM.RemoveEntities(i)
		break
	}
	if orig == nil {
		return nil, fmt.Errorf("entity not found in source domain model: %s", entity.ID)
	}

	// Convert associations referencing the moved entity to cross-associations.
	//
	// Each conversion records the qualified name the association had and the one it
	// has afterwards, because Mendix stores an association in the module of its FROM
	// entity and the two directions therefore differ: the parent moving takes the
	// cross-association to the target module, the child moving leaves it in the
	// source. The caller sweeps references from these names, so the case that does
	// not move is a no-op rather than a branch it has to know about (#605).
	var converted []types.MovedAssociation
	var removeIdx []int
	for i, el := range sourceDM.AssociationsItems() {
		a, ok := el.(*genDm.Association)
		if !ok {
			continue
		}
		parentID, childID := string(a.ParentRefID()), string(a.ChildRefID())
		moved := types.MovedAssociation{
			Name:             a.Name(),
			OldQualifiedName: sourceModuleName + "." + a.Name(),
			NewQualifiedName: sourceModuleName + "." + a.Name(),
		}
		switch {
		case childID == string(entity.ID): // child moved → cross-assoc stays in source
			sourceDM.AddCrossAssociations(crossAssocFromGenAssoc(a, parentID, targetModuleName+"."+entity.Name))
			removeIdx = append(removeIdx, i)
			converted = append(converted, moved)
		case parentID == string(entity.ID): // parent moved → cross-assoc goes to target
			targetDM.AddCrossAssociations(crossAssocFromGenAssoc(a, parentID, sourceModuleName+"."+nameByID[childID]))
			removeIdx = append(removeIdx, i)
			moved.NewQualifiedName = targetModuleName + "." + a.Name()
			converted = append(converted, moved)
		}
	}
	for i := len(removeIdx) - 1; i >= 0; i-- {
		sourceDM.RemoveAssociations(removeIdx[i])
	}

	// The cross-associations that already exist (ako/mxcli#628): the ones the moved
	// entity is the FROM end of travel with it — or become plain associations again
	// when their TO entity is already in the target — and the ones in the target
	// that point at it become plain associations there. See association_move_cross.go.
	oldEntityQN := sourceModuleName + "." + entity.Name
	newEntityQN := targetModuleName + "." + entity.Name
	converted = append(converted, moveOwnCrossAssociations(sourceDM, targetDM, entity.ID, sourceModuleName, targetModuleName)...)
	converted = append(converted, convertIncomingCrossAssociations(targetDM, entity.ID, oldEntityQN, targetModuleName)...)

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

	// The same re-pointing for the entity's own ACCESS RULES, which name each member
	// by qualified name and were the missed sibling of the validation rules above
	// (#605). Leaving them stale is worse than a dangling string: entityToGen's
	// syncMemberAccesses matches existing entries by qualified name, so a stale
	// `Source.Entity.Attr` never equals the rebuilt `Target.Entity.Attr` and it
	// appends the new one while keeping the old — the moved entity ends up carrying
	// every member twice, half of the entries dangling. DESCRIBE renders members
	// bare, so the duplication is the only thing that shows.
	//
	// An ATTRIBUTE reference always follows the entity, so its module prefix is
	// rewritten unconditionally. An ASSOCIATION reference is rewritten only for the
	// associations this move actually sent to the target module — a converted one,
	// or a pre-existing cross-association the moved entity is the FROM end of
	// (#628) — never by a blanket prefix swap.
	assocRenames := make(map[string]string, len(converted))
	for _, m := range converted {
		if m.Moved() {
			assocRenames[m.OldQualifiedName] = m.NewQualifiedName
		}
	}
	for _, ar := range entity.AccessRules {
		for _, ma := range ar.MemberAccesses {
			if strings.HasPrefix(ma.AttributeName, oldPrefix) {
				ma.AttributeName = newPrefix + ma.AttributeName[len(oldPrefix):]
			}
			if to, ok := assocRenames[ma.AssociationName]; ok {
				ma.AssociationName = to
			}
		}
	}

	// Add the (rebuilt) entity to the target DM, carrying the identities the
	// rebuild has no business re-minting — the same two carries UpdateEntity does,
	// which MoveEntity never got (#657 for the entity, #1119 for its children).
	//
	// Nothing in the entity's unmodeled properties goes stale when the module
	// changes, which is what makes a blanket raw carry safe here as well as there.
	// `Image` is a qualified name pointing at an image document that does NOT move
	// with the entity, so keeping it is correct rather than stale — and it is
	// unmodeled, so without this carry a move silently drops an entity's
	// domain-model image as well as its GUID.
	// Everything that does need rewriting is modeled and therefore dirty:
	// `MaybeGeneralization`, `Location` and the member lists are all
	// re-encoded from the rebuild, and `Source`/`ValidationRules` were re-pointed on
	// `entity` just above.
	ge := entityToGen(entity, targetModuleName, b.majorVersion())
	ge.SetID(element.ID(entity.ID))
	assignEntityIDs(ge)
	// carryStoredEntity also keeps the export level entityToGen resets to Hidden,
	// and the access rules' captions (ako/mxcli#801).
	carryStoredEntity(ge, orig, entity)
	targetDM.AddEntities(ge)

	// The move writes two units, and a failure between them lost the entity:
	// removed from the source, refused by the target, in neither module
	// (ako/mxcli#2323). A script that stops rolls the source back with the rest
	// of the run, but `exec --continue-on-error` goes on to the next statement
	// with the source written. So both units are checked against the refusals
	// the write path makes on content before either is written, and a target
	// write that fails anyway puts the source back as it was.
	sourceContents, err := (&codec.Encoder{}).Encode(sourceDM)
	if err != nil {
		return nil, fmt.Errorf("MoveEntity: encode source: %w", err)
	}
	targetContents, err := (&codec.Encoder{}).Encode(targetDM)
	if err != nil {
		return nil, fmt.Errorf("MoveEntity: encode target: %w", err)
	}
	if err := refusedContents(sourceDMID, sourceContents); err != nil {
		return nil, fmt.Errorf("MoveEntity: source: %w; nothing was written", err)
	}
	if err := refusedContents(targetDMID, targetContents); err != nil {
		return nil, fmt.Errorf("MoveEntity: target: %w; nothing was written", err)
	}
	storedSource, err := b.reader.GetRawUnitBytes(string(sourceDMID))
	if err != nil {
		return nil, fmt.Errorf("MoveEntity: read source: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(sourceDMID), sourceContents); err != nil {
		return nil, fmt.Errorf("MoveEntity: persist source: persist domain model %s: %w", sourceDMID, err)
	}
	if err := b.writer.UpdateRawUnit(string(targetDMID), targetContents); err != nil {
		if rerr := b.writer.UpdateRawUnitPatch(string(sourceDMID), storedSource); rerr != nil {
			return nil, fmt.Errorf("MoveEntity: persist target: persist domain model %s: %w; "+
				"restoring the source domain model also failed, so the entity is in neither module: %v", targetDMID, err, rerr)
		}
		return nil, fmt.Errorf("MoveEntity: persist target: persist domain model %s: %w; the source domain model was restored", targetDMID, err)
	}
	// A cross-association in any OTHER module that names the moved entity is
	// re-pointed; nothing else in the move rewrites that module's unit (#628).
	if err := b.repointCrossAssociationsElsewhere(sourceDMID, targetDMID, oldEntityQN, newEntityQN); err != nil {
		return nil, fmt.Errorf("MoveEntity: re-point cross-associations: %w", err)
	}
	return converted, nil
}

// refusedContents is the refusal the write path makes on a unit's content
// alone, before anything is stored (Writer.updateUnit): two elements sharing an
// $ID, or a bare attribute reference.
func refusedContents(unitID model.ID, contents []byte) error {
	if err := canon.DuplicateElementIDError(string(unitID), contents); err != nil {
		return err
	}
	return canon.BareAttributeRefError(string(unitID), contents)
}
