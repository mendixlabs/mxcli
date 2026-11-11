// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1819: CATALOG.pages has an Excluded column, CATALOG.snippets
// and CATALOG.layouts did not — `select Name, Excluded from CATALOG.snippets`
// failed with "no such column: Excluded", so a snippet or layout set to
// "Exclude from project" could not be told apart from an active one.
func TestSnippetAndLayoutTables_RecordExcluded(t *testing.T) {
	reader := &mock.MockBackend{
		ListSnippetsFunc: func() ([]*pages.Snippet, error) {
			return []*pages.Snippet{
				{BaseElement: model.BaseElement{ID: "sn-excluded"}, ContainerID: twinModuleID, Name: "SN_Old", Excluded: true},
				{BaseElement: model.BaseElement{ID: "sn-live"}, ContainerID: twinModuleID, Name: "SN_New",
					Parameters: []*pages.SnippetParameter{{Name: "Item"}, {Name: "Other"}}},
			}, nil
		},
		ListLayoutsFunc: func() ([]*pages.Layout, error) {
			return []*pages.Layout{
				{BaseElement: model.BaseElement{ID: "ly-excluded"}, ContainerID: twinModuleID, Name: "Ly_Old", Excluded: true},
				{BaseElement: model.BaseElement{ID: "ly-live"}, ContainerID: twinModuleID, Name: "Ly_New"},
			}, nil
		},
	}
	b, cat := twinBuilder(t, reader, nil)
	if err := b.buildSnippets(); err != nil {
		t.Fatalf("buildSnippets: %v", err)
	}
	if err := b.buildLayouts(); err != nil {
		t.Fatalf("buildLayouts: %v", err)
	}
	if err := b.tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	for _, tc := range []struct{ table, liveID, excludedID string }{
		{"snippets", "sn-live", "sn-excluded"},
		{"layouts", "ly-live", "ly-excluded"},
	} {
		rows, err := cat.db.Query("SELECT Id, Excluded FROM " + tc.table)
		if err != nil {
			t.Fatalf("%s: %v", tc.table, err)
		}
		got := map[string]bool{}
		for rows.Next() {
			var id string
			var excluded bool
			if err := rows.Scan(&id, &excluded); err != nil {
				t.Fatalf("%s scan: %v", tc.table, err)
			}
			got[id] = excluded
		}
		rows.Close()
		if ex, ok := got[tc.excludedID]; !ok || !ex {
			t.Errorf("%s: the excluded document must have Excluded=1; got %v", tc.table, got)
		}
		if ex, ok := got[tc.liveID]; !ok || ex {
			t.Errorf("%s: the active document must have Excluded=0; got %v", tc.table, got)
		}
	}

	// The snippet's ParameterCount was a literal 0 beside the same row: the
	// same "absent" answer for a fact the reader already had.
	var n int
	if err := cat.db.QueryRow("SELECT ParameterCount FROM snippets WHERE Id = 'sn-live'").Scan(&n); err != nil {
		t.Fatalf("ParameterCount: %v", err)
	}
	if n != 2 {
		t.Errorf("snippets.ParameterCount = %d, want 2", n)
	}
}
