// SPDX-License-Identifier: Apache-2.0

// cmd_move_document.go — MOVE for document types that have no bespoke handler.
//
// Before #932 MOVE accepted nine doctypes. The other twenty-odd were not
// unimplemented, merely unlisted: a top-level document's move is one
// containment row whatever the document is, so the work per doctype was a
// grammar entry and a lookup, not new machinery. Rather than write that lookup
// twenty more times — each one a `List<Kind>()` scan that can only find the
// kinds someone remembered to add — this resolves the name through the unit
// table, which is type-agnostic by construction.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// moveDocumentUnit reparents a document located by name rather than by type,
// and reports it by the kind it turned out to be.
func moveDocumentUnit(ctx *ExecContext, docType ast.DocumentType, name ast.QualifiedName, targetContainerID model.ID) error {
	asked := docType.CanonicalSpelling()
	// The parser cannot produce a doctype outside the registry, so reaching
	// here with one means a DocumentType was added to the AST without being
	// added to the registry the grammar and this handler share. Refuse rather
	// than fall through: the lookup below is by name only, so an unregistered
	// doctype would happily move whatever that name resolved to.
	if !ast.IsMoveDocumentType(asked) {
		return mdlerrors.NewUnsupported("unsupported document type: " + string(docType))
	}
	doc, err := findDocumentOfKind(ctx, asked, name)
	if err != nil {
		return mdlerrors.NewBackend("find "+asked, err)
	}
	if doc == nil {
		return mdlerrors.NewNotFound(asked, name.String())
	}
	if err := checkMovedDocumentType(asked, doc.Kind, name); err != nil {
		return err
	}
	if err := ctx.Backend.MoveDocument(doc.ID, targetContainerID); err != nil {
		return mdlerrors.NewBackend("move "+asked, err)
	}
	invalidateHierarchy(ctx)
	// Report the kind read off the document, not the kind the statement named,
	// so the line cannot describe something other than what actually moved.
	fmt.Fprintf(ctx.Output, "Moved %s %s to new location\n", doc.Kind, name.String())
	return nil
}

// findDocumentOfKind resolves name to the document of the kind the statement
// asked for. A name is unique per document type, not per module — a microflow
// and a scheduled event may both be M.Nightly (mendixlabs/mxcli#1792) — so a
// name-only lookup returned whichever it met first and refused the move as a
// mistyped doctype. With no document of the asked kind, the first one is
// returned, so checkMovedDocumentType can say what the name really is.
func findDocumentOfKind(ctx *ExecContext, asked string, name ast.QualifiedName) (*types.DocumentUnit, error) {
	docs, err := ctx.Backend.FindDocumentUnits(name.Module, name.Name)
	if err != nil || len(docs) == 0 {
		return nil, err
	}
	for _, d := range docs {
		if strings.EqualFold(d.Kind, asked) {
			return d, nil
		}
	}
	return docs[0], nil
}

// checkMovedDocumentType refuses a document whose stored type is not the one
// the statement named, and says what it actually is.
//
// The comparison is against the kind DERIVED from the stored $Type, not against
// a hand-written table of storage names. That matters: the two such tables this
// repo already has disagree with each other and with a real project —
// mdl/types/unit_types.go says JavaScript actions live under "JavaActions$",
// while the writer inserts and Studio Pro stores "JavaScriptActions$" — so a
// third would be a third thing to get wrong, and getting it wrong here blocks
// legitimate moves.
//
// Hence the deliberate asymmetry: refuse only when the derived kind is itself a
// doctype MOVE knows how to spell. A kind mxcli has no MDL word for — a page
// template, a document type a future Mendix version adds — is a derivation this
// code cannot vouch for, so it defers to the name lookup. A name is unique per
// document type, and findDocumentOfKind has already preferred the asked kind,
// so reaching a mismatch here means nothing of that kind has the name; this
// check is here to catch a mistyped doctype, not to second-guess the model.
func checkMovedDocumentType(asked, found string, name ast.QualifiedName) error {
	if found == "" || strings.EqualFold(found, asked) || !ast.IsMoveDocumentType(found) {
		return nil
	}
	return mdlerrors.NewValidation(fmt.Sprintf(
		"%s is a %s, not a %s — use 'move %s %s to ...'",
		name.String(), found, asked, found, name.String()))
}
