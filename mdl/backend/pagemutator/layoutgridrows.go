// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// isLayoutGrid reports whether a stored widget is a layout grid.
func isLayoutGrid(widget bson.D) bool {
	switch widgetTypeName(widget) {
	case "Forms$LayoutGrid", "Pages$LayoutGrid":
		return true
	}
	return false
}

// InsertLayoutGridRows appends rows to an existing layout grid
// (mendixlabs/mxcli#1791).
//
// A row lives in the grid's Rows list, not in a Widgets list, and is not a
// widget — the reason INSERT INTO refused a layout grid as "not a simple
// container", as it did a tab container before #1215. A row has no stored Name,
// so it cannot be the target of BEFORE/AFTER; INTO the grid, which appends, is
// the only position.
//
// The rows are serialized by wrapping them in a throwaway layout grid and
// taking its Rows back out, so they get exactly the shape CREATE PAGE writes.
func (m *Mutator) InsertLayoutGridRows(gridRef string, rows []*pages.LayoutGridRow) error {
	result := m.widgetFinder(m.rawData, gridRef)
	if result == nil {
		return m.widgetNotFoundError(gridRef)
	}
	if !isLayoutGrid(result.widget) {
		return fmt.Errorf("cannot insert a row into %q (%s): a row can only be added to a layout grid",
			gridRef, widgetTypeName(result.widget))
	}
	if len(rows) == 0 {
		return fmt.Errorf("no rows to insert")
	}
	wrapper := &pages.LayoutGrid{Rows: rows}
	wrapper.TypeName = widgetTypeName(result.widget)
	serialized := bsonnav.ToBsonA(bsonnav.DGet(m.deps.SerializeWidget(wrapper), "Rows"))
	var newDocs []any
	marker := any(nil)
	for _, el := range serialized {
		if isListMarker(el) {
			marker = el
			continue
		}
		if d, ok := el.(bson.D); ok {
			newDocs = append(newDocs, d)
		}
	}
	if len(newDocs) != len(rows) {
		return fmt.Errorf("serialize layout grid rows: got %d stored rows for %d inserted", len(newDocs), len(rows))
	}

	grid := result.widget
	stored := bsonnav.ToBsonA(bsonnav.DGet(grid, "Rows"))
	var out bson.A
	switch {
	case len(stored) > 0:
		out = append(out, stored...) // marker + existing rows, kept as stored
	case marker != nil:
		out = append(out, marker) // an empty grid takes the marker CREATE PAGE writes
	}
	out = append(out, newDocs...)
	if !bsonnav.DSet(grid, "Rows", out) {
		grid = append(grid, bson.E{Key: "Rows", Value: out})
	}
	if result.parentArr == nil || result.index < 0 || result.index >= len(result.parentArr) {
		return fmt.Errorf("cannot add rows to %q: its parent slot could not be located", gridRef)
	}
	result.parentArr[result.index] = grid
	bsonnav.DSetArray(result.parentDoc, result.parentKey, result.parentArr)
	return nil
}
