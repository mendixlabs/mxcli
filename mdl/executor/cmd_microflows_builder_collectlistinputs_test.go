// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// TestCollectListInputVariables_AddRemoveFromList pins issue #405:
// `add $X to $List` and `remove $Y from $List` consume the target list, so
// the list variable must be tracked as a list input. Without it, the output
// of an Owner=Both reverse retrieve fed straight into add/remove was
// misclassified as object-only and the AssociationRetrieveSource was
// suppressed, re-introducing the original #383 bug for this usage shape.
func TestCollectListInputVariables_AddRemoveFromList(t *testing.T) {
	stmts := []ast.MicroflowStatement{
		&ast.AddToListStmt{Item: "NewItem", List: "Items"},
		&ast.RemoveFromListStmt{Item: "OldItem", List: "Backlog"},
	}

	got := collectListInputVariables(stmts)

	if !got["Items"] {
		t.Errorf("AddToListStmt target `Items` must be marked as list input; got %v", got)
	}
	if !got["Backlog"] {
		t.Errorf("RemoveFromListStmt target `Backlog` must be marked as list input; got %v", got)
	}
}
