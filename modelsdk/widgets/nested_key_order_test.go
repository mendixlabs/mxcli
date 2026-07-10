// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"testing"
)

// Bug 6 — DataGrid2 columns are an object-list; Studio Pro requires each column
// WidgetObject's Properties to mirror the WidgetType's PropertyTypes order or it
// raises CE0463. The modelsdk registry previously dropped that order, so the
// object-list builder fell back to alphabetical → CE0463 on every column. The
// loader must now populate NestedKeyOrder for the `columns` object-list property.
func TestGetTemplateFullBSON_ColumnNestedKeyOrder(t *testing.T) {
	counter := 0
	idGen := func() string { counter++; return idForCounter(counter) }

	_, _, propIDs, _, _, err := GetTemplateFullBSON("com.mendix.widget.web.datagrid.Datagrid", idGen, "")
	if err != nil {
		t.Skipf("datagrid template not found: %v", err)
	}
	if propIDs == nil {
		t.Skip("no datagrid template")
	}

	cols, ok := propIDs["columns"]
	if !ok {
		t.Fatal("datagrid template has no `columns` property")
	}
	if len(cols.NestedPropertyIDs) == 0 {
		t.Fatal("columns has no NestedPropertyIDs (object-list schema missing)")
	}
	if len(cols.NestedKeyOrder) != len(cols.NestedPropertyIDs) {
		t.Fatalf("NestedKeyOrder has %d keys, want %d (one per nested property) — order not captured",
			len(cols.NestedKeyOrder), len(cols.NestedPropertyIDs))
	}

	// The first nested properties must be in template PropertyTypes order, not
	// alphabetical (alphabetical would start "alignment", "allowEventPropagation").
	wantPrefix := []string{"showContentAs", "attribute", "content", "dynamicText"}
	for i, want := range wantPrefix {
		if cols.NestedKeyOrder[i] != want {
			t.Errorf("NestedKeyOrder[%d] = %q, want %q (full: %v)", i, cols.NestedKeyOrder[i], want, cols.NestedKeyOrder)
		}
	}

	// Every ordered key must resolve to a real nested property.
	for _, k := range cols.NestedKeyOrder {
		if _, ok := cols.NestedPropertyIDs[k]; !ok {
			t.Errorf("NestedKeyOrder key %q is not in NestedPropertyIDs", k)
		}
	}
}
