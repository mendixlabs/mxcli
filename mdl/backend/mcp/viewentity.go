// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// CreateViewEntitySourceDocument is gated on the capability model
// (capabilities.yaml / view_entities) and rejects. A view entity is only valid
// when its attributes are synced to the OQL's SELECT columns (each attribute's
// value becomes an OqlViewValue). PED has no deterministic way to do that — its
// Attribute schema exposes no `value`, a `set` on the nested value is refused,
// and the sync (syncViewEntity) is reachable only through the LLM-backed
// oql_generate tool, which regenerates the query rather than writing the user's
// OQL verbatim. Writing the OQL with PED's plain tools leaves the entity out of
// sync with its query (CE6770). So mxcli refuses up front instead of leaving a
// broken entity in the model.
//
// This is the executor's first view-entity backend call (before the companion
// entity is created), so rejecting here leaves nothing half-created.
func (b *Backend) CreateViewEntitySourceDocument(moduleID model.ID, moduleName, docName, oqlQuery, documentation string) (model.ID, error) {
	qn := moduleName + "." + docName
	if !b.canAuthor(capViewEntityCreate) {
		return "", b.notAuthorable("view entity", qn, capViewEntityCreate)
	}
	return "", errCreatePathUnbuilt("view entity", qn)
}

// DeleteViewEntitySourceDocumentByName is a no-op: the PED server exposes no
// delete-document tool. The executor calls this before every
// CreateViewEntitySourceDocument; for a new view entity there is nothing to
// delete, so a no-op lets CREATE proceed. Re-creating an existing view entity
// fails at ped_create_document (duplicate document) with a clear error.
func (b *Backend) DeleteViewEntitySourceDocumentByName(_ string, _ string) error {
	return nil
}
