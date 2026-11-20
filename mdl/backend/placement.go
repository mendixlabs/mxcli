// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// DocumentPlacementBackend moves top-level documents between containers, and
// finds them without being told their type.
//
// Placement is the one property every document type shares and none of them
// stores: a document's folder is its unit's container, not a field in its
// contents. So the typed Move* methods on the per-domain interfaces
// (MoveMicroflow, MoveEnumeration, …) all reduce to the same row update, and a
// new document type inherits a placement bug simply by not having one written
// for it — which is how import mappings, JSON structures, queues, scheduled
// events and a dozen others ended up unable to leave the module root (#932).
//
// These two methods are type-agnostic on purpose. FindDocumentUnit resolves a
// qualified name through the unit table rather than through a per-kind list, so
// it cannot inherit the blind spot that a hand-maintained list of document
// kinds keeps re-introducing (cf. #892, where the same kind of list made a
// non-empty folder render as empty and a destructive drop look safe). The typed
// Move* methods stay for the doctypes whose move does extra work — remapping
// document access roles, rewriting cross-module references.
type DocumentPlacementBackend interface {
	// MoveDocument reparents a top-level document unit to containerID, which is
	// either a module ID (the module root) or a folder ID.
	//
	// Implementations must be idempotent: moving a document to the container it
	// already occupies must not write. Placement changes nothing inside the
	// unit, so it is invisible to content-based no-op elision (ADR-0008) and has
	// to account for itself.
	MoveDocument(unitID, containerID model.ID) error

	// FindDocumentUnit returns the top-level document named name inside
	// moduleName, wherever it sits in that module's folder tree, or nil when
	// there is no such document.
	FindDocumentUnit(moduleName, name string) (*types.DocumentUnit, error)

	// FindDocumentUnits returns every top-level document named name inside
	// moduleName. A name is unique per document type, not per module: a
	// microflow and a scheduled event may share one (mendixlabs/mxcli#1792),
	// so a caller that knows the kind it wants picks it from these rather
	// than take whichever FindDocumentUnit meets first.
	FindDocumentUnits(moduleName, name string) ([]*types.DocumentUnit, error)

	// ListDocumentUnits returns every top-level document in the project with
	// its container, name and type.
	//
	// Type-agnostic for the same reason as FindDocumentUnit, and it matters
	// most here: a folder listing built from per-kind list calls can only show
	// the kinds someone remembered to add, and an under-count is what made
	// dropping a non-empty folder look safe in #892. This one cannot miss a
	// kind, because it never asks what kind anything is.
	ListDocumentUnits() ([]*types.DocumentUnit, error)
}
