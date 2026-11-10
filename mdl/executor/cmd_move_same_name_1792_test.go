// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// mendixlabs/mxcli#1792: a microflow M.Nightly and a scheduled event M.Nightly
// share a name. `move scheduled event M.Nightly to folder Jobs` was refused
// with "M.Nightly is a microflow, not a scheduled event": the lookup was by
// name only and returned whichever document it met first.
func TestMoveSameNamedDocumentPicksTheAskedKind(t *testing.T) {
	mod := mkModule("M")
	mf := &types.DocumentUnit{ID: nextID("unit"), ContainerID: mod.ID, Name: "Nightly",
		Type: "Microflows$Microflow", Kind: "microflow"}
	se := &types.DocumentUnit{ID: nextID("unit"), ContainerID: mod.ID, Name: "Nightly",
		Type: "ScheduledEvents$ScheduledEvent", Kind: "scheduled event"}

	var probe placementProbe
	mb, h := moveMockBackend(t, mod, mf, &probe) // the single lookup meets the microflow first
	mb.FindDocumentUnitsFunc = func(moduleName, name string) ([]*types.DocumentUnit, error) {
		if moduleName == "M" && name == "Nightly" {
			return []*types.DocumentUnit{mf, se}, nil
		}
		return nil, nil
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeScheduledEvent,
		Name:         ast.QualifiedName{Module: "M", Name: "Nightly"},
		Folder:       "Jobs",
	})
	assertNoError(t, err)
	if !probe.moved || probe.unitID != se.ID {
		t.Fatalf("moved %q (moved=%v), want the scheduled event %q", probe.unitID, probe.moved, se.ID)
	}
	assertContainsStr(t, buf.String(), "Moved scheduled event M.Nightly")
}

// Control: with only the microflow under that name, a mistyped doctype is
// still refused, naming what the document is.
func TestMoveSameNameControlStillRefusesAMistypedDoctype(t *testing.T) {
	mod := mkModule("M")
	mf := &types.DocumentUnit{ID: nextID("unit"), ContainerID: mod.ID, Name: "Nightly",
		Type: "Microflows$Microflow", Kind: "microflow"}
	var probe placementProbe
	mb, h := moveMockBackend(t, mod, mf, &probe)

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeScheduledEvent,
		Name:         ast.QualifiedName{Module: "M", Name: "Nightly"},
		Folder:       "Jobs",
	})
	if err == nil || probe.moved {
		t.Fatalf("err = %v, moved = %v; want a refusal and nothing moved", err, probe.moved)
	}
	assertContainsStr(t, err.Error(), "is a microflow, not a scheduled event")
}
