// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	genPg "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// A ListView database source must serialize as Forms$ListViewXPathSource on the
// modelsdk engine (previously only microflow sources were supported, forcing
// --engine legacy).
func TestListViewSourceToGen_Database(t *testing.T) {
	el, err := listViewSourceToGen(&pages.DatabaseSource{
		EntityName:      "LvBug.Item",
		XPathConstraint: "[Rank > 5]",
		Sorting:         []*pages.GridSort{{AttributePath: "Name", Direction: "Ascending"}},
	})
	if err != nil {
		t.Fatalf("listViewSourceToGen(database): %v", err)
	}
	src, ok := el.(*genPg.ListViewXPathSource)
	if !ok {
		t.Fatalf("type = %T, want *genPg.ListViewXPathSource", el)
	}
	if src.XPathConstraint() != "[Rank > 5]" {
		t.Errorf("XPathConstraint = %q, want [Rank > 5]", src.XPathConstraint())
	}
	if src.EntityRef() == nil {
		t.Error("EntityRef must be set for a database source")
	}
	// Studio Pro requires the SortBar and Search sub-elements on a ListViewXPathSource.
	if src.SortBar() == nil {
		t.Error("SortBar must be set")
	}
	if src.Search() == nil {
		t.Error("Search must be set")
	}
}

// Database source with no explicit sorting still produces a valid source (empty SortBar).
func TestListViewSourceToGen_DatabaseNoSort(t *testing.T) {
	el, err := listViewSourceToGen(&pages.DatabaseSource{EntityName: "LvBug.Item"})
	if err != nil {
		t.Fatalf("listViewSourceToGen: %v", err)
	}
	if _, ok := el.(*genPg.ListViewXPathSource); !ok {
		t.Fatalf("type = %T, want *genPg.ListViewXPathSource", el)
	}
}

// Microflow source still works (regression guard).
func TestListViewSourceToGen_Microflow(t *testing.T) {
	el, err := listViewSourceToGen(&pages.MicroflowSource{Microflow: "LvBug.DS_Items"})
	if err != nil {
		t.Fatalf("listViewSourceToGen(microflow): %v", err)
	}
	if _, ok := el.(*genPg.MicroflowSource); !ok {
		t.Fatalf("type = %T, want *genPg.MicroflowSource", el)
	}
}
