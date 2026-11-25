// SPDX-License-Identifier: Apache-2.0

// Package executor - Association commands (SHOW/DESCRIBE/CREATE/DROP ASSOCIATION)
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// execCreateAssociation handles CREATE ASSOCIATION statements.
func execCreateAssociation(ctx *ExecContext, s *ast.CreateAssociationStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}

	// A FROM entity in another module writes a project that cannot be LOADED, so
	// this must fail before the write — `check` reporting it is not enough for a
	// script run with --no-check (the #833 lesson, same failure class).
	if vs := ValidateAssociationModules(s); len(vs) > 0 {
		return mdlerrors.NewValidationf("%s\n    %s", vs[0].Message, vs[0].Suggestion)
	}

	// The association's name becomes a member of its FROM entity (and of its
	// TO entity under owner Both), which must be unique along that entity's
	// generalization chain — CE0069 (ako/mxcli#1645). Before the module is
	// auto-created, so a refusal leaves nothing behind.
	if err := refuseMemberNameClash(ctx, s); err != nil {
		return err
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Find parent and child entities (supports cross-module associations)
	parentModule := s.Parent.Module
	if parentModule == "" {
		parentModule = s.Name.Module
	}
	parentEntity, err := findEntity(ctx, parentModule, s.Parent.Name)
	if err != nil {
		return mdlerrors.NewNotFound("parent entity", s.Parent.String())
	}
	parentID := parentEntity.ID

	childModule := s.Child.Module
	if childModule == "" {
		childModule = s.Name.Module
	}
	childEntity, err := findEntity(ctx, childModule, s.Child.Name)
	if err != nil {
		return mdlerrors.NewNotFound("child entity", s.Child.String())
	}
	childID := childEntity.ID

	// A view entity at either end is CE6771. `check` reports it too, but a script
	// run with --no-check must not be able to write it — that is how the reported
	// case got a model mxbuild refuses (FINDINGS §1), and it is the same #833
	// lesson as the module guard above.
	for _, ep := range []struct {
		entity *domainmodel.Entity
		name   string
	}{{parentEntity, s.Parent.String()}, {childEntity, s.Child.String()}} {
		if isViewEntity(ep.entity) {
			return viewEntityAssociationRefusal(s.Name.String(), ep.name)
		}
	}

	// A persistable and a non-persistable entity may only be associated FROM the
	// non-persistable one under owner Default — anything else is CE0001
	// (ako/mxcli#819). The same function judges it in `check`.
	if v := associationPersistenceViolation(s.Name,
		ast.QualifiedName{Module: parentModule, Name: s.Parent.Name},
		ast.QualifiedName{Module: childModule, Name: s.Child.Name},
		s.Owner == ast.OwnerBoth, projectEntityPersistence(ctx)); v != nil {
		return mdlerrors.NewValidationf("%s\n    %s", v.Message, v.Suggestion)
	}

	// Convert types
	assocType := domainmodel.AssociationTypeReference
	if s.Type == ast.AssocReferenceSet {
		assocType = domainmodel.AssociationTypeReferenceSet
	}

	owner := domainmodel.AssociationOwnerDefault
	switch s.Owner {
	case ast.OwnerBoth:
		owner = domainmodel.AssociationOwnerBoth
	}

	deleteBehavior := storageDeleteBehavior(s.DeleteBehavior)
	deleteMessage := s.DeleteErrorMessage
	// The message is a Texts$Text, keyed by the project's default language like
	// every other text mxcli authors (mendixlabs/mxcli#1344, #970).
	deleteLang := authoringLanguage(ctx)

	// Convert storage type. A new association defaults to Column (foreign key on
	// the FROM entity's table); on OR MODIFY an unstated storage keeps what is
	// stored — see statedStorageFormat.
	storageFormat, storageStated := statedStorageFormat(s.Storage)
	if !storageStated {
		storageFormat = domainmodel.StorageFormatColumn
	}

	// Create association
	// ParentID = FROM entity (the one with the FK)
	// ChildID = TO entity (the one being referenced)
	// Cross-module associations use BY_NAME for the child entity
	isCrossModule := parentModule != childModule

	// OR MODIFY: update existing association properties in place, preserving its UUID.
	if s.CreateOrModify {
		if !isCrossModule {
			for _, assoc := range dm.Associations {
				if assoc.Name == s.Name.Name {
					assoc.Type = assocType
					assoc.Owner = owner
					// Unstated storage keeps the stored one: describe omits the
					// clause for table storage, so the column default would flip
					// a table association on re-executing unchanged describe
					// output — a database schema change nobody asked for (#704).
					if s.Storage != ast.StorageDefault {
						assoc.StorageFormat = storageFormat
					}
					assoc.ChildDeleteBehavior = &domainmodel.DeleteBehavior{Type: deleteBehavior, ErrorMessage: deleteMessage, ErrorMessageLanguage: deleteLang}
					assoc.Documentation = carriedDocumentation(
						associationDocumentationStated(s), associationDocumentation(s), assoc.Documentation)
					if s.ExportLevel != nil { // absent keeps the stored level (#1498)
						assoc.ExportLevel = *s.ExportLevel
					}
					// Anchors are applied only when the statement names them —
					// silence preserves what is stored, so a `create or modify`
					// that is not about layout does not flatten a hand-tuned line.
					applyAnchors(assoc, s.FromAnchor, s.ToAnchor)
					if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
						return mdlerrors.NewBackend("update association", err)
					}
					invalidateHierarchy(ctx)
					invalidateDomainModelsCache(ctx)
					ctx.trackModifiedDomainModel(module)
					ctx.ReportMutation("Modified", "association: %s", s.Name)
					return nil
				}
			}
		} else {
			childRef := childModule + "." + s.Child.Name
			for _, ca := range dm.CrossAssociations {
				if ca.Name == s.Name.Name {
					// Read before it is overwritten: the TO end may be moving.
					oldToModule := qualifiedNameModule(ca.ChildRef)
					ca.Type = assocType
					ca.Owner = owner
					if s.Storage != ast.StorageDefault { // as above (#704)
						ca.StorageFormat = storageFormat
					}
					ca.ChildDeleteBehavior = &domainmodel.DeleteBehavior{Type: deleteBehavior, ErrorMessage: deleteMessage, ErrorMessageLanguage: deleteLang}
					ca.ChildRef = childRef
					ca.Documentation = carriedDocumentation(
						associationDocumentationStated(s), associationDocumentation(s), ca.Documentation)
					if s.ExportLevel != nil { // as above (#1498)
						ca.ExportLevel = *s.ExportLevel
					}
					if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
						return mdlerrors.NewBackend("update cross-module association", err)
					}
					invalidateHierarchy(ctx)
					invalidateDomainModelsCache(ctx)
					ctx.trackModifiedDomainModel(module)
					ctx.ReportMutation("Modified", "association: %s", s.Name)
					// Under OWNER Both the TO entity holds a member entry, in its own
					// module. Only the declaring module was reconciled (at the end of
					// the program), so moving the TO end left the old TO entity's
					// entry behind and never gave the new one its entry — CE0066 at
					// both modules; changing the owner alone missed the TO module the
					// same way (ako/mxcli#819).
					return reconcileModulesAccess(ctx, "after modifying the association",
						module.Name, oldToModule, childModule)
				}
			}
		}
		// Association not found — fall through to create it.
	}

	// IF NOT EXISTS: an association of this name anywhere in the domain model
	// means there is nothing to do. Checked once here rather than in each of the
	// four lookups below, which exist to phrase the same-module and cross-module
	// errors. (sudoku findings #10)
	if s.IfNotExists && associationExists(dm, s.Name.Name) {
		fmt.Fprintf(ctx.Output, "Association %s already exists — skipped\n", s.Name)
		return nil
	}

	if isCrossModule {
		if !s.CreateOrModify {
			for _, ca := range dm.CrossAssociations {
				if ca.Name == s.Name.Name {
					return mdlerrors.NewAlreadyExistsMsg("association", s.Name.String(),
						fmt.Sprintf("association '%s' already exists — use 'create or modify association ...' to update it in place, or 'drop association %s' first", s.Name.String(), s.Name.String()))
				}
			}
			for _, assoc := range dm.Associations {
				if assoc.Name == s.Name.Name {
					return mdlerrors.NewAlreadyExistsMsg("association", s.Name.String(),
						fmt.Sprintf("association '%s' already exists — use 'create or modify association ...' to update it in place, or 'drop association %s' first", s.Name.String(), s.Name.String()))
				}
			}
		}
		childRef := childModule + "." + s.Child.Name
		ca := &domainmodel.CrossModuleAssociation{
			Name:          s.Name.Name,
			Documentation: associationDocumentation(s),
			Type:          assocType,
			Owner:         owner,
			StorageFormat: storageFormat,
			ParentID:      parentID,
			ChildRef:      childRef,
			ExportLevel:   authoredExportLevel(s.ExportLevel),
			ChildDeleteBehavior: &domainmodel.DeleteBehavior{
				Type:                 deleteBehavior,
				ErrorMessage:         deleteMessage,
				ErrorMessageLanguage: deleteLang,
			},
		}
		if err := ctx.Backend.CreateCrossAssociation(dm.ID, ca); err != nil {
			return mdlerrors.NewBackend("create cross-module association", err)
		}
	} else {
		// Check for existing association when not using OR MODIFY.
		if !s.CreateOrModify {
			for _, assoc := range dm.Associations {
				if assoc.Name == s.Name.Name {
					return mdlerrors.NewAlreadyExistsMsg("association", s.Name.String(),
						fmt.Sprintf("association '%s' already exists — use 'create or modify association ...' to update it in place, or 'drop association %s' first", s.Name.String(), s.Name.String()))
				}
			}
			for _, ca := range dm.CrossAssociations {
				if ca.Name == s.Name.Name {
					return mdlerrors.NewAlreadyExistsMsg("association", s.Name.String(),
						fmt.Sprintf("association '%s' already exists — use 'create or modify association ...' to update it in place, or 'drop association %s' first", s.Name.String(), s.Name.String()))
				}
			}
		}
		assoc := &domainmodel.Association{
			Name:          s.Name.Name,
			Documentation: associationDocumentation(s),
			Type:          assocType,
			Owner:         owner,
			StorageFormat: storageFormat,
			ParentID:      parentID,
			ChildID:       childID,
			ExportLevel:   authoredExportLevel(s.ExportLevel),
			ChildDeleteBehavior: &domainmodel.DeleteBehavior{
				Type:                 deleteBehavior,
				ErrorMessage:         deleteMessage,
				ErrorMessageLanguage: deleteLang,
			},
		}
		applyAnchors(assoc, s.FromAnchor, s.ToAnchor)
		if err := ctx.Backend.CreateAssociation(dm.ID, assoc); err != nil {
			return mdlerrors.NewBackend("create association", err)
		}
	}

	// Invalidate hierarchy cache so the new association's container is visible
	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	// Reconcile MemberAccesses immediately — existing access rules on entities
	// in this DM need MemberAccess entries for the new association (CE0066).
	// Under OWNER Both a cross-module association is a member of the TO entity
	// too, whose rules live in the other module (ako/mxcli#802).
	if freshDM, err := ctx.Backend.GetDomainModel(module.ID); err == nil {
		if count, err := ctx.Backend.ReconcileMemberAccesses(freshDM.ID, module.Name); err == nil && count > 0 {
			fmt.Fprintf(ctx.Output, "Reconciled %d access rule(s) for new association\n", count)
		}
	}
	if childModule != module.Name {
		if err := reconcileModuleAccess(ctx, childModule, "for new association"); err != nil {
			return err
		}
	}

	ctx.trackModifiedDomainModel(module)
	fmt.Fprintf(ctx.Output, "Created association: %s\n", s.Name)
	return nil
}

// execAlterAssociation handles ALTER ASSOCIATION statements.
func execAlterAssociation(ctx *ExecContext, s *ast.AlterAssociationStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// `set owner both` across the persistence boundary is CE0001, as on create
	// (ako/mxcli#819).
	if s.Operation == ast.AlterAssociationSetOwner && s.Owner == ast.OwnerBoth {
		if from, to, ok := storedAssociationEnds(ctx, s.Name); ok {
			if v := associationPersistenceViolation(s.Name, from, to, true, projectEntityPersistence(ctx)); v != nil {
				return mdlerrors.NewValidationf("%s\n    %s", v.Message, v.Suggestion)
			}
		}
		// Owner Both makes it a member of the TO entity too (CE0069,
		// ako/mxcli#1645).
		if err := refuseMemberNameClash(ctx, s); err != nil {
			return err
		}
	}

	// Try intra-module associations first
	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			switch s.Operation {
			case ast.AlterAssociationSetDeleteBehavior:
				assoc.ChildDeleteBehavior = &domainmodel.DeleteBehavior{
					Type:                 storageDeleteBehavior(s.DeleteBehavior),
					ErrorMessage:         s.DeleteErrorMessage,
					ErrorMessageLanguage: authoringLanguage(ctx),
				}
			case ast.AlterAssociationSetOwner:
				assoc.Owner = domainmodel.AssociationOwner(s.Owner.String())
			case ast.AlterAssociationSetStorage:
				assoc.StorageFormat = domainmodel.AssociationStorageFormat(s.Storage.String())
			case ast.AlterAssociationSetComment:
				assoc.Documentation = s.Comment
			case ast.AlterAssociationSetAnchor:
				applyAnchors(assoc, s.FromAnchor, s.ToAnchor)
			case ast.AlterAssociationSetExportLevel:
				assoc.ExportLevel = s.ExportLevel
			}
			want := alteredAssociationValue(s.Operation, assocAlterView{
				del: assoc.ChildDeleteBehavior, owner: string(assoc.Owner), storage: string(assoc.StorageFormat),
				doc: assoc.Documentation, lang: authoringLanguage(ctx), anchors: associationAnchors(assoc),
				exportLevel: assoc.ExportLevel,
			})
			if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
				return mdlerrors.NewBackend("update association", err)
			}
			if err := verifyAssociationAltered(ctx, module.ID, s, want); err != nil {
				return err
			}
			if err := reconcileAfterAssociationAlter(ctx, s, module.Name); err != nil {
				return err
			}
			fmt.Fprintf(ctx.Output, "Altered association: %s\n", s.Name)
			return nil
		}
	}

	// Try cross-module associations
	for _, ca := range dm.CrossAssociations {
		if ca.Name == s.Name.Name {
			switch s.Operation {
			case ast.AlterAssociationSetDeleteBehavior:
				ca.ChildDeleteBehavior = &domainmodel.DeleteBehavior{
					Type:                 storageDeleteBehavior(s.DeleteBehavior),
					ErrorMessage:         s.DeleteErrorMessage,
					ErrorMessageLanguage: authoringLanguage(ctx),
				}
			case ast.AlterAssociationSetOwner:
				ca.Owner = domainmodel.AssociationOwner(s.Owner.String())
			case ast.AlterAssociationSetStorage:
				ca.StorageFormat = domainmodel.AssociationStorageFormat(s.Storage.String())
			case ast.AlterAssociationSetComment:
				ca.Documentation = s.Comment
			case ast.AlterAssociationSetExportLevel:
				ca.ExportLevel = s.ExportLevel
			case ast.AlterAssociationSetAnchor:
				// DomainModels$CrossAssociation has no connection properties at
				// all, and writing them there crashes Studio Pro (#50) — so this
				// is refused rather than silently ignored.
				return mdlerrors.NewValidationf(
					"association %s is cross-module, and Mendix stores no line anchors for those — "+
						"the connector is routed automatically", s.Name.String())
			}
			want := alteredAssociationValue(s.Operation, assocAlterView{
				del: ca.ChildDeleteBehavior, owner: string(ca.Owner), storage: string(ca.StorageFormat),
				doc: ca.Documentation, lang: authoringLanguage(ctx), exportLevel: ca.ExportLevel,
			})
			if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
				return mdlerrors.NewBackend("update cross-module association", err)
			}
			if err := verifyAssociationAltered(ctx, module.ID, s, want); err != nil {
				return err
			}
			if err := reconcileAfterAssociationAlter(ctx, s, module.Name, qualifiedNameModule(ca.ChildRef)); err != nil {
				return err
			}
			fmt.Fprintf(ctx.Output, "Altered association: %s\n", s.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("association", s.Name.String())
}

// reconcileAfterAssociationAlter brings the access rules of every module an end
// of the association lives in back in line with its members, when the alter
// changed who the members are.
//
// ako/mxcli#802: `OWNER Both` makes the association a member of the TO entity as
// well, so `set owner` adds (or, back to Default, removes) an entry on that
// entity's rules. The alter wrote the owner and reconciled nothing, and mx check
// reported CE0066 "Entity access is out of date" — at the TO entity's module,
// which for a cross-module association is not the module altered. The other
// operations (delete behaviour, storage, comment, anchors) do not change
// membership and leave the rules alone.
func reconcileAfterAssociationAlter(ctx *ExecContext, s *ast.AlterAssociationStmt, modules ...string) error {
	if s.Operation != ast.AlterAssociationSetOwner {
		return nil
	}
	return reconcileModulesAccess(ctx, "after changing the owner", modules...)
}

// reconcileModulesAccess reconciles the entity access rules of each named module
// once, after a write that changed which entities an association is a member of
// — or removed associations outright. It is the one reconcile behind create,
// alter and drop association and drop entity, so that every statement that can
// leave a member entry behind reconciles the same set of modules the same way.
//
// The modules are named by the caller because the rule that needs fixing is not
// always in the module written: an association is a member of its FROM entity,
// and under OWNER Both of its TO entity, which for a cross-module association
// lives in another module.
func reconcileModulesAccess(ctx *ExecContext, why string, modules ...string) error {
	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)
	seen := map[string]bool{}
	for _, name := range modules {
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		if err := reconcileModuleAccess(ctx, name, why); err != nil {
			return err
		}
	}
	return nil
}

// reconcileModuleAccess reconciles one module's entity access rules and tracks
// its domain model as modified. A module that cannot be found (the TO end of a
// cross-module association named by a stale reference) has nothing to fix, and
// neither does System: its domain model is virtual, not a stored unit, so the
// reconcile failed with "no such file" after an association to a System entity
// had already been written — and its rules cannot be changed by a project.
func reconcileModuleAccess(ctx *ExecContext, moduleName, why string) error {
	if isBuiltinModuleEntity(moduleName) {
		return nil
	}
	mod, err := findModule(ctx, moduleName)
	if err != nil || mod == nil {
		return nil
	}
	dm, err := ctx.Backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return nil
	}
	count, err := ctx.Backend.ReconcileMemberAccesses(dm.ID, mod.Name)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("reconcile access rules of module %s", mod.Name), err)
	}
	if count > 0 {
		fmt.Fprintf(ctx.Output, "Reconciled %d access rule(s) in module %s %s\n", count, mod.Name, why)
	}
	ctx.trackModifiedDomainModel(mod)
	return nil
}

// assocAlterView is the part of an association ALTER ASSOCIATION can change,
// common to the same-module and the cross-module kind.
type assocAlterView struct {
	del                                       *domainmodel.DeleteBehavior
	owner, storage, doc, anchors, exportLevel string
	// lang is the language the delete message is compared in — the one the
	// alter wrote. Reading it back in en_US would compare against a stale
	// translation the write carried over (mendixlabs/mxcli#1344).
	lang string
}

// alteredAssociationValue renders the one property op changes, so the value the
// statement asked for can be compared with the value read back from storage.
func alteredAssociationValue(op ast.AlterAssociationOperation, v assocAlterView) string {
	switch op {
	case ast.AlterAssociationSetDeleteBehavior:
		if v.del == nil {
			return ""
		}
		// The message is stored only on the restrict side (assocToGen), so it is
		// only part of the comparison there.
		if v.del.Type == domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences {
			return string(v.del.Type) + "|" + deleteMessageIn(v.del, v.lang)
		}
		return string(v.del.Type)
	case ast.AlterAssociationSetOwner:
		return v.owner
	case ast.AlterAssociationSetStorage:
		return v.storage
	case ast.AlterAssociationSetComment:
		return v.doc
	case ast.AlterAssociationSetAnchor:
		return v.anchors
	case ast.AlterAssociationSetExportLevel:
		return v.exportLevel
	}
	return ""
}

func associationAnchors(a *domainmodel.Association) string {
	return domainmodel.FormatConnectionPoint(a.ParentConnection, domainmodel.DefaultParentConnection) + " " +
		domainmodel.FormatConnectionPoint(a.ChildConnection, domainmodel.DefaultChildConnection)
}

// verifyAssociationAltered reads the association back after an ALTER and refuses
// to report success unless storage holds what the statement asked for.
//
// ako/mxcli#792: on a cross-module association the backend dropped every edit
// and "Altered association" was printed anyway. The backend is fixed; this is the
// backstop for the class — a statement that reports success must have written.
func verifyAssociationAltered(ctx *ExecContext, moduleID model.ID, s *ast.AlterAssociationStmt, want string) error {
	dm, err := ctx.Backend.GetDomainModel(moduleID)
	if err != nil {
		return mdlerrors.NewBackend("re-read domain model after alter association", err)
	}
	got, found := "", false
	if dm != nil {
		for _, a := range dm.Associations {
			if a.Name == s.Name.Name {
				got, found = alteredAssociationValue(s.Operation, assocAlterView{
					del: a.ChildDeleteBehavior, owner: string(a.Owner), storage: string(a.StorageFormat),
					doc: a.Documentation, lang: authoringLanguage(ctx), anchors: associationAnchors(a),
					exportLevel: a.ExportLevel,
				}), true
				break
			}
		}
		if !found {
			for _, ca := range dm.CrossAssociations {
				if ca.Name == s.Name.Name {
					got, found = alteredAssociationValue(s.Operation, assocAlterView{
						del: ca.ChildDeleteBehavior, owner: string(ca.Owner), storage: string(ca.StorageFormat),
						doc: ca.Documentation, lang: authoringLanguage(ctx), exportLevel: ca.ExportLevel,
					}), true
					break
				}
			}
		}
	}
	if !found {
		return mdlerrors.NewValidationf("alter association %s: change not persisted — the association "+
			"is no longer found in module %s after the write", s.Name.String(), s.Name.Module)
	}
	if got != want {
		return mdlerrors.NewValidationf("alter association %s: change not persisted — storage holds %q "+
			"where the statement set %q", s.Name.String(), got, want)
	}
	return nil
}

// execDropAssociation handles DROP ASSOCIATION statements.
func execDropAssociation(ctx *ExecContext, s *ast.DropAssociationStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find module
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Dropping an association leaves a MemberAccess entry behind on every access
	// rule that named it, which Mendix rejects with CE1613 "The selected
	// association … no longer exists". Creating one already reconciles; dropping
	// one has to as well, or the drop produces a model that will not build.
	// toModule is a cross-module association's TO end: under OWNER Both its
	// entity holds an entry too, in a module this one is not (ako/mxcli#1449).
	afterDrop := func(toModule string) error {
		return reconcileModulesAccess(ctx, "after dropping the association", module.Name, toModule)
	}

	// A message definition holds the association by qualified name, and nothing
	// keeps the two in step — the drop leaves the definition pointing at nothing
	// and mxbuild rejects the project with CE1613. Unlike the access rules above
	// there is nothing to reconcile: removing the member would change the
	// published contract, which is the author's decision and not mxcli's. So the
	// drop is refused, naming what to fix — the same posture `drop message
	// definition collection` already takes from the other side.
	if remedies := messageDefinitionsUsingAssociation(ctx, s.Name.String()); len(remedies) > 0 {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"association %s is still exposed by a message definition — dropping it would leave "+
				"the definition bound to nothing (CE1613). Remove the member first:\n  %s",
			s.Name.String(), strings.Join(remedies, ";\n  ")+";"))
	}

	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			if err := ctx.Backend.DeleteAssociation(dm.ID, assoc.ID); err != nil {
				return mdlerrors.NewBackend("delete association", err)
			}
			if err := afterDrop(""); err != nil {
				return err
			}
			fmt.Fprintf(ctx.Output, "Dropped association: %s\n", s.Name)
			return nil
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Name == s.Name.Name {
			if err := ctx.Backend.DeleteCrossAssociation(dm.ID, ca.ID); err != nil {
				return mdlerrors.NewBackend("delete cross-module association", err)
			}
			if err := afterDrop(qualifiedNameModule(ca.ChildRef)); err != nil {
				return err
			}
			fmt.Fprintf(ctx.Output, "Dropped cross-module association: %s\n", s.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("association", s.Name.String())
}

// listAssociations handles SHOW ASSOCIATIONS command.
func listAssociations(ctx *ExecContext, moduleName string) error {
	// Build module ID -> name map (single query)
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("list modules", err)
	}
	moduleNames := make(map[model.ID]string)
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}

	// Get all domain models in a single query (avoids O(n²) behavior)
	domainModels, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return mdlerrors.NewBackend("list domain models", err)
	}

	// Build entity ID -> qualified name map
	entityNames := make(map[model.ID]string)
	for _, dm := range domainModels {
		modName := moduleNames[dm.ContainerID]
		for _, entity := range dm.Entities {
			entityNames[entity.ID] = modName + "." + entity.Name
		}
	}

	// Collect rows
	type row struct {
		qualifiedName string
		module        string
		name          string
		parent        string
		child         string
		assocType     string
		owner         string
		storage       string
	}
	var rows []row

	for _, dm := range domainModels {
		modName := moduleNames[dm.ContainerID]
		// Filter by module name if specified
		if moduleName != "" && modName != moduleName {
			continue
		}
		// Intra-module associations
		for _, assoc := range dm.Associations {
			qualifiedName := modName + "." + assoc.Name
			parent := entityNames[assoc.ParentID]
			child := entityNames[assoc.ChildID]
			if parent == "" {
				parent = string(assoc.ParentID)
			}
			if child == "" {
				child = string(assoc.ChildID)
			}
			rows = append(rows, row{qualifiedName, modName, assoc.Name, parent, child, string(assoc.Type), string(assoc.Owner), string(assoc.StorageFormat)})
		}
		// Cross-module associations
		for _, ca := range dm.CrossAssociations {
			qualifiedName := modName + "." + ca.Name
			parent := entityNames[ca.ParentID]
			if parent == "" {
				parent = string(ca.ParentID)
			}
			rows = append(rows, row{qualifiedName, modName, ca.Name, parent, ca.ChildRef, string(ca.Type), string(ca.Owner), string(ca.StorageFormat)})
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	// Build TableResult
	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Parent", "Child", "Type", "Owner", "Storage"},
		Summary: fmt.Sprintf("(%d associations)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.parent, r.child, r.assocType, r.owner, r.storage})
	}
	return writeResult(ctx, result)
}

// listAssociation handles SHOW ASSOCIATION command.
func listAssociation(ctx *ExecContext, name *ast.QualifiedName) error {
	if name == nil {
		return mdlerrors.NewValidation("association name required")
	}

	module, err := findModule(ctx, name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	for _, assoc := range dm.Associations {
		if assoc.Name == name.Name {
			fmt.Fprintf(ctx.Output, "Association: %s.%s\n", module.Name, assoc.Name)
			fmt.Fprintf(ctx.Output, "  Type: %s\n", assoc.Type)
			fmt.Fprintf(ctx.Output, "  Owner: %s\n", assoc.Owner)
			fmt.Fprintf(ctx.Output, "  Storage: %s\n", assoc.StorageFormat)
			return nil
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Name == name.Name {
			fmt.Fprintf(ctx.Output, "Association: %s.%s (cross-module)\n", module.Name, ca.Name)
			fmt.Fprintf(ctx.Output, "  Type: %s\n", ca.Type)
			fmt.Fprintf(ctx.Output, "  Owner: %s\n", ca.Owner)
			fmt.Fprintf(ctx.Output, "  Storage: %s\n", ca.StorageFormat)
			fmt.Fprintf(ctx.Output, "  Child: %s\n", ca.ChildRef)
			return nil
		}
	}

	return mdlerrors.NewNotFound("association", name.String())
}

// describeAssociation handles DESCRIBE ASSOCIATION command.
func describeAssociation(ctx *ExecContext, name ast.QualifiedName) error {
	module, err := findModule(ctx, name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Build entity ID -> qualified name map across all modules
	entityNames := make(map[model.ID]string)
	allDomainModels, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return mdlerrors.NewBackend("list domain models", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}
	for _, otherDM := range allDomainModels {
		modName := h.GetModuleName(otherDM.ContainerID)
		for _, entity := range otherDM.Entities {
			entityNames[entity.ID] = modName + "." + entity.Name
		}
	}

	// formatAssocDetails prints the clauses after `from … to …` and the
	// terminator. A clause is printed only when it differs from what the
	// statement defaults to (R12, #748): type Reference, owner Default, storage
	// column and on delete set null are what an unstated clause means, so
	// printing them is noise that says nothing a reader or a re-execution needs.
	//
	// Storage is the asymmetric one. Column is the create default, so it is
	// omitted; Table is not, so it is always printed — omitting it made a table
	// association come back as column wherever the description was replayed
	// (#704).
	formatAssocDetails := func(assocType domainmodel.AssociationType, assocOwner domainmodel.AssociationOwner, storageFormat domainmodel.AssociationStorageFormat, childDeleteBehavior *domainmodel.DeleteBehavior, exportLevel string) {
		var clauses []string
		if assocType == domainmodel.AssociationTypeReferenceSet {
			clauses = append(clauses, "type ReferenceSet")
		}
		if assocOwner == domainmodel.AssociationOwnerBoth {
			clauses = append(clauses, "owner Both")
		}
		if storageFormat == domainmodel.StorageFormatTable {
			clauses = append(clauses, "storage table")
		}
		// DELETE_AND_REFERENCES, not DELETE_CASCADE: DESCRIBE has to emit MDL the
		// parser accepts, and DELETE_CASCADE is not a token (upstream #901). The
		// round-trip test in cmd_associations_delete_behavior_test.go feeds this
		// back through the parser.
		if del := describeDeleteClause(ctx, childDeleteBehavior); del != defaultDeleteClause {
			clauses = append(clauses, del)
		}
		if c := exportLevelClause(exportLevel); c != "" { // mendixlabs/mxcli#1498
			clauses = append(clauses, c)
		}
		for _, c := range clauses {
			fmt.Fprintf(ctx.Output, "\n%s", c)
		}
		fmt.Fprint(ctx.Output, ";\n")
	}

	for _, assoc := range dm.Associations {
		if assoc.Name == name.Name {
			fromEntity := entityNames[assoc.ParentID]
			toEntity := entityNames[assoc.ChildID]

			if isViewAssociationSource(assoc.Source) {
				describeViewAssociation(ctx, module.Name+"."+assoc.Name, fromEntity, toEntity, assoc.ViewSourceReference)
				return nil
			}

			if assoc.Documentation != "" {
				outputJavadocIndented(ctx.Output, assoc.Documentation, "")
			}

			describeConnectionPoints(ctx, assoc)
			fmt.Fprintf(ctx.Output, "create or modify association %s.%s\n", module.Name, assoc.Name)
			fmt.Fprintf(ctx.Output, "from %s to %s", fromEntity, toEntity)
			formatAssocDetails(assoc.Type, assoc.Owner, assoc.StorageFormat, assoc.ChildDeleteBehavior, assoc.ExportLevel)
			return nil
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Name == name.Name {
			fromEntity := entityNames[ca.ParentID]
			if fromEntity == "" {
				fromEntity = string(ca.ParentID)
			}

			if isViewAssociationSource(ca.Source) {
				describeViewAssociation(ctx, module.Name+"."+ca.Name, fromEntity, ca.ChildRef, ca.ViewSourceReference)
				return nil
			}

			if ca.Documentation != "" {
				outputJavadocIndented(ctx.Output, ca.Documentation, "")
			}

			fmt.Fprintf(ctx.Output, "create or modify association %s.%s\n", module.Name, ca.Name)
			fmt.Fprintf(ctx.Output, "from %s to %s", fromEntity, ca.ChildRef)
			formatAssocDetails(ca.Type, ca.Owner, ca.StorageFormat, ca.ChildDeleteBehavior, ca.ExportLevel)
			return nil
		}
	}

	return mdlerrors.NewNotFound("association", name.String())
}

// describeViewAssociation describes an association a view entity's OQL
// declares — a `<alias>.ID as <name>` select column — as a pointer to that
// view entity, never as a statement.
//
// There is no statement that can say it: a plain `create association` with a
// view entity at either end is CE6771, and check and exec both refuse one. The
// column IS the declaration, and the view entity's own description carries it,
// so re-executing that rebuilds the association (syncViewEntityAssociations).
// Printing `create or modify association` here made `describe module` output
// that its own check refused (mendixlabs/mxcli#1627).
func describeViewAssociation(ctx *ExecContext, qualifiedName, fromEntity, toEntity, column string) {
	if column == "" {
		column = shortAssociationName(qualifiedName)
	}
	fmt.Fprintf(ctx.Output, "-- association %s (%s -> %s) is declared by view entity %s's OQL column `as %s`;\n",
		qualifiedName, fromEntity, toEntity, fromEntity, column)
	fmt.Fprintf(ctx.Output, "-- it has no statement of its own: describe entity %s to reproduce it\n", fromEntity)
}

// isViewAssociationSource reports whether an association's Source makes it a
// view entity's OQL-declared association, which describes with the view entity
// rather than on its own.
func isViewAssociationSource(source string) bool {
	return source == domainmodel.OqlViewAssociationSource
}

// statedStorageFormat maps an authored storage clause onto the stored value, and
// reports whether the statement stated one. An unstated storage is not a request
// for the default: on `create or modify` it means "leave it", because the
// storage format decides the database schema and flipping it migrates data.
func statedStorageFormat(s ast.StorageType) (domainmodel.AssociationStorageFormat, bool) {
	switch s {
	case ast.StorageColumn:
		return domainmodel.StorageFormatColumn, true
	case ast.StorageTable:
		return domainmodel.StorageFormatTable, true
	}
	return "", false
}

// storageClause renders a stored storage format as the MDL clause that
// reproduces it. Empty for an empty or unrecognised value, which an unstated
// storage then carries unchanged on replay.
func storageClause(f domainmodel.AssociationStorageFormat) string {
	switch f {
	case domainmodel.StorageFormatColumn:
		return "storage column"
	case domainmodel.StorageFormatTable:
		return "storage table"
	}
	return ""
}

// storageDeleteBehavior maps an authored delete behaviour onto the value Mendix
// stores. Mendix's DeletingBehavior admits exactly three (generated/metamodel
// enums.go); ast.DeleteBehavior has six, three of which name nothing Mendix has.
//
// This is the single conversion for every path that writes one. The ALTER path
// used to build it as DeleteBehaviorType(s.DeleteBehavior.String()) instead, and
// String() spells the prevent case "DeleteIfNoReferences" where Mendix writes
// "DeleteMeIfNoReferences" — so `ALTER ASSOCIATION ... SET DELETE_BEHAVIOR
// PREVENT` put an out-of-domain enum on disk (upstream #901). Nothing downstream
// rejects one: mxbuild is lenient about property values and Studio Pro is not,
// so the failure surfaces only when someone opens the project.
//
// String() is a display helper. Do not reintroduce it as a storage encoding.
func storageDeleteBehavior(b ast.DeleteBehavior) domainmodel.DeleteBehaviorType {
	switch b {
	case ast.DeleteCascade:
		return domainmodel.DeleteBehaviorTypeDeleteMeAndReferences
	case ast.DeleteIfNoReferences:
		return domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences
	default:
		return domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences
	}
}

// applyAnchors copies authored `@anchor(from: …, to: …)` / `SET ANCHOR` values
// onto the association.
//
// An unnamed end is left alone rather than defaulted. That is what keeps a
// hand-tuned line safe through a `create or modify association` whose subject is
// the delete behaviour, and it is why the AST carries pointers: "not mentioned"
// and "mentioned as (0, 0)" are different instructions, and (0, 0) is a real
// anchor (the box's top-left). (issue #872)
func applyAnchors(assoc *domainmodel.Association, from, to *ast.Position) {
	if assoc == nil {
		return
	}
	if from != nil {
		assoc.ParentConnection = &model.Point{X: from.X, Y: from.Y}
	}
	if to != nil {
		assoc.ChildConnection = &model.Point{X: to.X, Y: to.Y}
	}
}

// describeConnectionPoints emits the association's line anchors — where the
// connector attaches to the FROM and TO entity boxes in the domain model editor
// — as the `@anchor(from: (x, y), to: (x, y))` annotation that authors them, so
// a describe → edit → exec cycle round-trips the layout.
//
// The units are PERCENTAGES of the entity box, 0..100 — measured across 88
// coordinate pairs in four Studio-Pro-authored sources (a blank 11.13 app plus
// the Advanced Audit Trail Core, Email Connector and Workflow Commons modules):
// nothing falls outside 0..100, and 85 of the 88 pin one coordinate to exactly 0
// or 100 while the other varies, i.e. "which edge, and how far along it". Pixels
// is ruled out by the model itself: `DomainModels$EntityImpl` stores only
// `Location` and NO size, so the box's dimensions are computed from the name and
// attribute list — a pixel anchor would have nothing to measure against and
// would drift every time an attribute is added.
//
// The pair is CONTINUOUS, which is why the syntax takes numbers rather than the
// eight named anchors the issue proposed: the observed x values are 0, 9, 11,
// 17, 18, 47, 49, 50, 65, 77, 78, 84, 87, 100, and mxcli's own default 0;50
// differs from Studio Pro's 0;54 by four points. (issue #872)
//
// Only non-default anchors print, so the common case — an association mxcli
// created itself — describes exactly as before.
func describeConnectionPoints(ctx *ExecContext, assoc *domainmodel.Association) {
	parent := domainmodel.FormatConnectionPoint(assoc.ParentConnection, domainmodel.DefaultParentConnection)
	child := domainmodel.FormatConnectionPoint(assoc.ChildConnection, domainmodel.DefaultChildConnection)
	if parent == domainmodel.DefaultParentConnection && child == domainmodel.DefaultChildConnection {
		return
	}
	from := domainmodel.ParseConnectionPoint(parent)
	to := domainmodel.ParseConnectionPoint(child)
	if from == nil || to == nil {
		return
	}
	fmt.Fprintf(ctx.Output, "@anchor(from: (%d, %d), to: (%d, %d))\n", from.X, from.Y, to.X, to.Y)
}

// --- Executor method wrappers for callers not yet migrated ---

// associationExists reports whether the domain model already holds an
// association of this name, same-module or cross-module.
func associationExists(dm *domainmodel.DomainModel, name string) bool {
	for _, assoc := range dm.Associations {
		if assoc.Name == name {
			return true
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Name == name {
			return true
		}
	}
	return false
}

// associationDocumentationStated reports whether the statement said anything
// about documentation — a doc comment, even an empty one. The OR MODIFY path
// used `if doc != ""`, which preserved the stored value but also made it
// unclearable; #1018's rule is that an explicitly empty comment clears while
// an absent one preserves. The `comment '…'` clause is folded into the doc
// comment by the visitor (R9, where it is a deprecated alias).
func associationDocumentationStated(s *ast.CreateAssociationStmt) bool {
	return s.DocumentationSet
}

func associationDocumentation(s *ast.CreateAssociationStmt) string {
	return s.Documentation
}

// defaultDeleteClause is the delete behaviour an association statement with no
// delete clause writes (storageDeleteBehavior's default arm).
const defaultDeleteClause = "on delete set null"

// describeDeleteClause renders a child delete behaviour as MDL.
//
// It emits the SQL spelling, because that is the one that says what the
// behaviour DOES: `ON DELETE RESTRICT` on a FROM/TO pair reads the way a foreign
// key does, while `DELETE_IF_NO_REFERENCES` leaves a reader to work out which
// side is governed. The old spelling still parses, so nothing that already
// exists breaks — this only changes what DESCRIBE chooses to write.
//
// The message is emitted whenever there is one. Dropping it would make a
// describe -> exec round trip produce an association whose runtime does not
// start, which is the failure this whole clause exists to prevent (CapTrackV2
// §1) — and the round trip is exactly how these scripts get regenerated.
func describeDeleteClause(ctx *ExecContext, db *domainmodel.DeleteBehavior) string {
	action := defaultDeleteClause
	if db != nil {
		switch db.Type {
		case domainmodel.DeleteBehaviorTypeDeleteMeAndReferences:
			action = "on delete cascade"
		case domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences:
			action = "on delete restrict"
		}
	}
	if msg := deleteMessageIn(db, describeDefaultLanguage(ctx)); msg != "" {
		return action + " error message " + mdlQuote(ctx, msg)
	}
	return action
}

// deleteMessageIn is a delete behaviour's error message in lang: the language
// CREATE writes it under, so DESCRIBE reads back what was written and describe
// -> exec round-trips on a project whose default is not en_US
// (mendixlabs/mxcli#1344). Without the stored translations — a behaviour built
// in memory rather than read — it is the one string there is.
func deleteMessageIn(db *domainmodel.DeleteBehavior, lang string) string {
	if db == nil {
		return ""
	}
	if len(db.ErrorMessageTranslations) == 0 {
		return db.ErrorMessage
	}
	return pickTextTranslation(&model.Text{Translations: db.ErrorMessageTranslations}, lang)
}
