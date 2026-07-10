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
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
)

// MoveEnumeration reparents an enumeration unit to its (already-updated) target
// container module. Enumerations are top-level units, so this is a unit reparent.
func (b *Backend) MoveEnumeration(enum *model.Enumeration) error {
	if enum == nil {
		return fmt.Errorf("MoveEnumeration: nil enumeration")
	}
	if b.writer == nil {
		return fmt.Errorf("MoveEnumeration: not connected for writing")
	}
	return b.writer.MoveUnit(string(enum.ID), string(enum.ContainerID))
}

// MoveConstant reparents a constant unit to its target container module.
func (b *Backend) MoveConstant(c *model.Constant) error {
	if c == nil {
		return fmt.Errorf("MoveConstant: nil constant")
	}
	if b.writer == nil {
		return fmt.Errorf("MoveConstant: not connected for writing")
	}
	return b.writer.MoveUnit(string(c.ID), string(c.ContainerID))
}

// UpdateOqlQueriesForMovedEntity rewrites every ViewEntitySourceDocument whose
// OQL text references oldQualifiedName to newQualifiedName. Used after a MOVE
// ENTITY so view sources joining through the moved entity still resolve (CE0174).
// Mirrors the legacy string-replace; returns the number of docs updated.
func (b *Backend) UpdateOqlQueriesForMovedEntity(oldQualifiedName, newQualifiedName string) (int, error) {
	if b.writer == nil {
		return 0, fmt.Errorf("UpdateOqlQueriesForMovedEntity: not connected for writing")
	}
	units, err := mprread.ListUnitsWithContainer[*genDm.ViewEntitySourceDocument](b.reader)
	if err != nil {
		return 0, fmt.Errorf("UpdateOqlQueriesForMovedEntity: list source docs: %w", err)
	}
	updated := 0
	for _, u := range units {
		oql := u.Element.Oql()
		if oql == "" || !strings.Contains(oql, oldQualifiedName) {
			continue
		}
		u.Element.SetOql(strings.ReplaceAll(oql, oldQualifiedName, newQualifiedName))
		contents, err := (&codec.Encoder{}).Encode(u.Element)
		if err != nil {
			return updated, fmt.Errorf("UpdateOqlQueriesForMovedEntity: encode %s: %w", u.Element.ID(), err)
		}
		if err := b.writer.UpdateRawUnit(string(u.Element.ID()), contents); err != nil {
			return updated, fmt.Errorf("UpdateOqlQueriesForMovedEntity: update %s: %w", u.Element.ID(), err)
		}
		updated++
	}

	// Mendix < 11 also stores the OQL inline on the entity's OqlViewEntitySource
	// (see entityToGen), a second copy the ViewEntitySourceDocument sweep above
	// does not touch. Without this the moved-entity reference stays stale there and
	// `mx check` on 10.x reports CE0174 (11.x keeps the OQL only on the source
	// document, so it isn't affected). Rewrite the inline copy in every domain model.
	dms, err := b.ListDomainModels()
	if err != nil {
		return updated, fmt.Errorf("UpdateOqlQueriesForMovedEntity: list domain models: %w", err)
	}
	for _, info := range dms {
		// The System module's domain model is virtual (not stored in mprcontents),
		// so it can't be re-loaded from disk — and it holds no view entities with
		// inline OQL. Skip it; otherwise loadDomainModelGen fails with a spurious
		// "no such file or directory" warning.
		if string(info.ID) == meta.SystemDomainModelID {
			continue
		}
		gdm, err := b.loadDomainModelGen(info.ID)
		if err != nil {
			return updated, err
		}
		changed := false
		for _, el := range gdm.EntitiesItems() {
			ent, ok := el.(*genDm.Entity)
			if !ok {
				continue
			}
			src, ok := ent.Source().(*genDm.OqlViewEntitySource)
			if !ok {
				continue
			}
			oql := src.Oql()
			if oql == "" || !strings.Contains(oql, oldQualifiedName) {
				continue
			}
			src.SetOql(strings.ReplaceAll(oql, oldQualifiedName, newQualifiedName))
			changed = true
			updated++
		}
		if changed {
			if err := b.persistDM(info.ID, gdm); err != nil {
				return updated, err
			}
		}
	}
	return updated, nil
}

// CreateViewEntitySourceDocument creates the OQL source document (a top-level
// unit) that backs a view entity. The entity's OqlViewEntitySource references it
// by qualified name (wired in entityToGen).
func (b *Backend) CreateViewEntitySourceDocument(moduleID model.ID, moduleName, docName, oqlQuery, documentation string) (model.ID, error) {
	if b.writer == nil {
		return "", fmt.Errorf("CreateViewEntitySourceDocument: not connected for writing")
	}
	docID := model.ID(mmpr.GenerateID())
	d := genDm.NewViewEntitySourceDocument()
	d.SetID(element.ID(docID))
	d.SetName(docName)
	d.SetDocumentation(documentation)
	d.SetExcluded(false)
	d.SetExportLevel("Hidden")
	d.SetOql(oqlQuery)
	contents, err := (&codec.Encoder{}).Encode(d)
	if err != nil {
		return "", fmt.Errorf("CreateViewEntitySourceDocument: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(docID), string(moduleID), "Documents", "DomainModels$ViewEntitySourceDocument", contents); err != nil {
		return "", fmt.Errorf("CreateViewEntitySourceDocument: insert: %w", err)
	}
	return docID, nil
}

// MoveViewEntitySourceDocument reparents the OQL source document backing a moved
// view entity to the target module. Without it the doc is orphaned in the source
// module (CE6786). A view entity and its source doc share a name.
func (b *Backend) MoveViewEntitySourceDocument(sourceModuleName string, targetModuleID model.ID, docName string) error {
	if b.writer == nil {
		return fmt.Errorf("MoveViewEntitySourceDocument: not connected for writing")
	}
	docID, err := b.FindViewEntitySourceDocumentID(sourceModuleName, docName)
	if err != nil || docID == "" {
		return err // nil docID → nothing to move
	}
	return b.writer.MoveUnit(string(docID), string(targetModuleID))
}

// FindAllViewEntitySourceDocumentIDs returns every ViewEntitySourceDocument unit
// named docName in the given module.
func (b *Backend) FindAllViewEntitySourceDocumentIDs(moduleName, docName string) ([]model.ID, error) {
	mod, err := b.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return nil, nil
	}
	units, err := mprread.ListUnitsWithContainer[*genDm.ViewEntitySourceDocument](b.reader)
	if err != nil {
		return nil, err
	}
	var ids []model.ID
	for _, u := range units {
		if string(u.ContainerID) == string(mod.ID) && u.Element.Name() == docName {
			ids = append(ids, model.ID(u.Element.ID()))
		}
	}
	return ids, nil
}

// FindViewEntitySourceDocumentID returns the first matching source-doc ID, or "".
func (b *Backend) FindViewEntitySourceDocumentID(moduleName, docName string) (model.ID, error) {
	ids, err := b.FindAllViewEntitySourceDocumentIDs(moduleName, docName)
	if err != nil || len(ids) == 0 {
		return "", err
	}
	return ids[0], nil
}

// DeleteViewEntitySourceDocument removes a source-doc unit by ID.
func (b *Backend) DeleteViewEntitySourceDocument(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteViewEntitySourceDocument: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// DeleteViewEntitySourceDocumentByName removes every source-doc named docName in
// the module (no-op when none exist — the executor calls this before re-creating).
func (b *Backend) DeleteViewEntitySourceDocumentByName(moduleName, docName string) error {
	ids, err := b.FindAllViewEntitySourceDocumentIDs(moduleName, docName)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := b.DeleteViewEntitySourceDocument(id); err != nil {
			return err
		}
	}
	return nil
}
