// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"bytes"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// MoveWidget moves a widget's stored subtree to a new place on the same page:
// before or after another widget, or into a container. The subtree is moved as
// it is stored, never rebuilt, so a widget MDL cannot re-author — a legacy
// classic Data grid after a 9 → 11 upgrade — can still be moved, or wrapped in
// a new data view (ako/mxcli#2122, mendixlabs/mxcli#1807).
//
// Everything is checked before the tree changes: an unknown or ambiguous
// address, a source that is not a widget (a column, tab page, list item, table
// cell), a destination inside the widget itself, and a destination that cannot
// take it.
func (m *Mutator) MoveWidget(widgetRef, destRef string, position backend.InsertPosition) error {
	src := m.widgetFinder(m.rawData, widgetRef)
	if src == nil {
		return m.widgetNotFoundError(widgetRef)
	}
	if err := refuseMoveSource(src, widgetRef); err != nil {
		return err
	}
	if n := m.columnMatchCount(widgetRef); n > 1 {
		return columnAmbiguityError(widgetRef, n)
	}
	dest := m.widgetFinder(m.rawData, destRef)
	if dest == nil {
		return m.widgetNotFoundError(destRef)
	}
	if n := m.columnMatchCount(destRef); n > 1 {
		return columnAmbiguityError(destRef, n)
	}
	if subtreeHolds(src.widget, dest.widget) {
		return fmt.Errorf("cannot move %q %s %q: the destination is the widget itself or inside it",
			widgetRef, strings.ToLower(string(position)), destRef)
	}
	into := strings.EqualFold(string(position), "into")
	if into {
		if dest.item != nil || dest.cell != nil || !containerAcceptsWidgets(dest.widget) {
			return fmt.Errorf("cannot move %q into %q: it is not a container with a list of widgets — "+
				"move it before or after a widget inside it instead", widgetRef, destRef)
		}
	} else {
		if err := refuseMoveSource(dest, destRef); err != nil {
			return err
		}
	}

	moved := src.widget
	rest := make([]any, 0, len(src.parentArr)-1)
	rest = append(rest, src.parentArr[:src.index]...)
	rest = append(rest, src.parentArr[src.index+1:]...)
	bsonnav.DSetArray(src.parentDoc, src.parentKey, rest)

	// The removal may have shifted the destination's index: find it again.
	dest = m.widgetFinder(m.rawData, destRef)
	if dest == nil {
		return m.widgetNotFoundError(destRef)
	}
	if into {
		container, err := appendChildrenToContainer(dest.widget, destRef, []any{moved})
		if err != nil {
			return err
		}
		dest.parentArr[dest.index] = container
		bsonnav.DSetArray(dest.parentDoc, dest.parentKey, dest.parentArr)
		return nil
	}
	at := dest.index
	if strings.EqualFold(string(position), "after") {
		at++
	}
	arr := make([]any, 0, len(dest.parentArr)+1)
	arr = append(arr, dest.parentArr[:at]...)
	arr = append(arr, moved)
	arr = append(arr, dest.parentArr[at:]...)
	bsonnav.DSetArray(dest.parentDoc, dest.parentKey, arr)
	return nil
}

// refuseMoveSource refuses an address that is a position in a widget's
// structure rather than a widget: a column, a tab page, a navigation or
// object-list item, a table cell. Moving one would put it among widgets.
func refuseMoveSource(r *bsonWidgetResult, ref string) error {
	if r.item != nil || r.cell != nil {
		return fmt.Errorf("%q is not a widget: only a widget can be moved, or have one moved beside it", ref)
	}
	if err := refuseObjectListItemTarget(r, ref); err != nil {
		return err
	}
	if err := refuseTabPageSiblingEdit(r, ref, "move"); err != nil {
		return err
	}
	return refuseNavItemWidgetEdit(r, ref, "move")
}

// subtreeHolds reports whether target is root or an element inside it,
// compared by $ID, or by widget name for an element without one.
func subtreeHolds(root, target bson.D) bool {
	id, hasID := bsonnav.DGet(target, "$ID").(bson.Binary)
	name := bsonnav.DGetString(target, "Name")
	if !hasID && name == "" {
		return false
	}
	same := func(n bson.D) bool {
		if hasID {
			b, ok := bsonnav.DGet(n, "$ID").(bson.Binary)
			return ok && bytes.Equal(b.Data, id.Data)
		}
		return bsonnav.DGetString(n, "Name") == name
	}
	found := false
	var walk func(v any)
	walk = func(v any) {
		if found {
			return
		}
		switch n := v.(type) {
		case bson.D:
			if same(n) {
				found = true
				return
			}
			for _, e := range n {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range n {
				walk(e)
			}
		case []any:
			for _, e := range n {
				walk(e)
			}
		}
	}
	walk(root)
	return found
}
