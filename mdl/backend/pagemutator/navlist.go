// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// navListItemType is a navigation list's entry. It lives in the list's Items,
// not in a widget list, and Studio Pro stores no Name on it.
const navListItemType = "Forms$NavigationListItem"

// isNavigationList reports whether a stored widget is a navigation list, whose
// entries — and the widgets inside them — sit under Items rather than Widgets.
func isNavigationList(widget bson.D) bool {
	switch widgetTypeName(widget) {
	case "Forms$NavigationList", "Pages$NavigationList":
		return true
	}
	return false
}

// InsertNavigationListItems adds items to an existing navigation list
// (mendixlabs/mxcli#1388).
//
// An item lives in the list's Items and is not a widget — the same reason tab
// pages and list view templates have their own paths. INSERT INTO <list>
// appends; INSERT BEFORE/AFTER <item> places them next to that item. An item has
// no Name (Studio Pro writes none, and mxcli stopped writing one in #1373), so
// an item is addressed instead by what it holds or where it is — `nav item(t1)`,
// `nav item('Home')`, `nav item@2` — which the widget finder resolves. A widget
// inside an item is that item's content, not its sibling, and is refused with
// the item address that would work, instead of guessing that "after t1" meant
// "after the item holding t1".
//
// The items are serialized by wrapping them in a throwaway navigation list and
// taking its Items back out, so they get exactly the shape CREATE writes.
func (m *Mutator) InsertNavigationListItems(targetRef string, position backend.InsertPosition, items []*pages.NavigationListItem) error {
	result := m.widgetFinder(m.rawData, targetRef)
	if result == nil {
		return m.widgetNotFoundError(targetRef)
	}
	into := strings.EqualFold(string(position), "into")
	isItem := widgetTypeName(result.widget) == navListItemType
	switch {
	case into && !isNavigationList(result.widget):
		return fmt.Errorf("cannot insert a navigation list item into %q (%s): an item can only be added to a navigation list — "+
			"use `insert into <navigationlist> { item … }`", targetRef, widgetTypeName(result.widget))
	case !into && !isItem:
		hint := "address an item as `<list> item(<widget>)`, `<list> item('<text>')` or `<list> item@n`"
		if list := m.enclosingNavigationList(targetRef); list != "" {
			hint = fmt.Sprintf("to place it next to the item holding %s, write `insert %s %s { item … }`",
				targetRef, strings.ToLower(string(position)), backend.NavItemSelector{List: list, Widget: targetRef})
		}
		return fmt.Errorf("cannot insert a navigation list item %s %q (%s): an item's siblings are items — %s",
			strings.ToLower(string(position)), targetRef, widgetTypeName(result.widget), hint)
	}

	newDocs, err := m.serializeNavigationListItems(items)
	if err != nil {
		return err
	}

	if !into {
		insertIdx := result.index
		if strings.EqualFold(string(position), "after") {
			insertIdx++
		}
		out := make([]any, 0, len(result.parentArr)+len(newDocs))
		out = append(out, result.parentArr[:insertIdx]...)
		out = append(out, newDocs...)
		out = append(out, result.parentArr[insertIdx:]...)
		bsonnav.DSetArray(result.parentDoc, result.parentKey, out)
		return nil
	}

	list := result.widget
	stored := bsonnav.ToBsonA(bsonnav.DGet(list, "Items"))
	var out bson.A
	if len(stored) == 0 {
		out = append(out, int32(2)) // Items' list marker, as the codec registers it
	} else {
		out = append(out, stored...)
	}
	out = append(out, newDocs...)
	if !bsonnav.DSet(list, "Items", out) {
		list = append(list, bson.E{Key: "Items", Value: out})
	}
	if result.parentArr == nil || result.index < 0 || result.index >= len(result.parentArr) {
		return fmt.Errorf("cannot add items to %q: its parent slot could not be located", targetRef)
	}
	result.parentArr[result.index] = list
	bsonnav.DSetArray(result.parentDoc, result.parentKey, result.parentArr)
	return nil
}

// enclosingNavigationList names the navigation list a widget sits in, or "".
func (m *Mutator) enclosingNavigationList(widgetRef string) string {
	var found string
	var walk func(node any)
	walk = func(node any) {
		switch v := node.(type) {
		case bson.D:
			if found != "" {
				return
			}
			if isNavigationList(v) {
				for _, it := range bsonnav.DGetArrayElements(bsonnav.DGet(v, "Items")) {
					if itDoc, ok := it.(bson.D); ok && findInWidgetArray(itDoc, "Widgets", widgetRef) != nil {
						found = bsonnav.DGetString(v, "Name")
						return
					}
				}
			}
			for _, e := range v {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range v {
				walk(e)
			}
		}
	}
	walk(m.rawData)
	return found
}

// serializeNavigationListItems returns the stored form of each item, without a
// list marker, by serializing them inside a wrapper navigation list.
func (m *Mutator) serializeNavigationListItems(items []*pages.NavigationListItem) ([]any, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no navigation list items to insert")
	}
	wrapper := &pages.NavigationList{Items: items}
	wrapper.TypeName = "Forms$NavigationList"
	doc := m.deps.SerializeWidget(wrapper)
	var out []any
	for _, el := range bsonnav.DGetArrayElements(bsonnav.DGet(doc, "Items")) {
		if d, ok := el.(bson.D); ok {
			out = append(out, d)
		}
	}
	if len(out) != len(items) {
		return nil, fmt.Errorf("serialize navigation list items: got %d stored items for %d inserted", len(out), len(items))
	}
	return out, nil
}

// withNavItemAddresses extends a widget finder with navigation list item
// addresses (`nav item(t1)`, `nav item('Home')`, `nav item@2`). The result for
// an item is the item itself, in the list's Items, so every operation that
// finds a widget — SET, INSERT INTO, DROP, and placing an item BEFORE/AFTER
// another — reaches an item through the path it already has. A miss or an
// ambiguous address finds nothing; widgetNotFoundError then says which.
func withNavItemAddresses(inner widgetFinder) widgetFinder {
	return func(rawData bson.D, ref string) *bsonWidgetResult {
		if sel, ok := backend.ParseNavItemSelector(ref); ok {
			r, _ := resolveNavItem(rawData, inner, sel)
			return r
		}
		return inner(rawData, ref)
	}
}

// resolveNavItem finds the one item a selector addresses.
func resolveNavItem(rawData bson.D, find widgetFinder, sel backend.NavItemSelector) (*bsonWidgetResult, error) {
	list := find(rawData, sel.List)
	if list == nil {
		// The owner may be a navigation list or a pluggable widget with an
		// object list (an Accordion); calling every miss a "navigation list"
		// sent an Accordion's author looking in the wrong place (#2173).
		return nil, fmt.Errorf("%s: widget %q not found (an item address names a navigation list or a widget with an item list, such as an Accordion)", sel, sel.List)
	}
	if !isNavigationList(list.widget) {
		// A pluggable widget's object-list item (an Accordion group) has no
		// name either, and is addressed the same way.
		if bsonnav.DGetString(list.widget, "$Type") == "CustomWidgets$CustomWidget" {
			return resolveObjectListItem(list, sel)
		}
		return nil, fmt.Errorf("%s: %q is a %s, not a navigation list — `item(…)` and `item@n` address a navigation list's "+
			"items or a pluggable widget's object-list items", sel, sel.List, widgetTypeName(list.widget))
	}
	items := bsonnav.DGetArrayElements(bsonnav.DGet(list.widget, "Items"))
	var matches []int
	for i, el := range items {
		itemDoc, ok := el.(bson.D)
		if !ok {
			continue
		}
		switch {
		case sel.Widget != "":
			if findInWidgetArray(itemDoc, "Widgets", sel.Widget) == nil {
				continue
			}
		case sel.Text != "":
			if !containsString(navItemTexts(itemDoc), sel.Text) {
				continue
			}
		}
		matches = append(matches, i)
	}
	pick := -1
	switch {
	case sel.Ordinal > 0 && sel.Ordinal <= len(matches):
		pick = matches[sel.Ordinal-1]
	case sel.Ordinal > 0:
		return nil, fmt.Errorf("%s: %s, so there is no @%d. %s",
			sel, countMatches(len(matches), sel), sel.Ordinal, navItemList(sel.List, items))
	case len(matches) == 1:
		pick = matches[0]
	case len(matches) == 0:
		return nil, fmt.Errorf("%s: no item matches. %s", sel, navItemList(sel.List, items))
	default:
		return nil, fmt.Errorf("%s is ambiguous: %s — add @n to choose one (@1 … @%d). %s",
			sel, countMatches(len(matches), sel), len(matches), navItemList(sel.List, items))
	}
	return &bsonWidgetResult{
		widget:    items[pick].(bson.D),
		parentArr: items,
		parentKey: "Items",
		parentDoc: list.widget,
		index:     pick,
	}, nil
}

func countMatches(n int, sel backend.NavItemSelector) string {
	noun := "items"
	if n == 1 {
		noun = "item"
	}
	if sel.Widget != "" || sel.Text != "" {
		return fmt.Sprintf("%d %s match", n, noun)
	}
	return fmt.Sprintf("the list has %d %s", n, noun)
}

// navItemList lists each item's addresses, so a miss shows what would hit.
func navItemList(list string, items []any) string {
	if len(items) == 0 {
		return fmt.Sprintf("navigation list %q has no items", list)
	}
	var parts []string
	for i, el := range items {
		itemDoc, _ := el.(bson.D)
		addrs := []string{fmt.Sprintf("item@%d", i+1)}
		if names := navItemWidgetNames(itemDoc); len(names) > 0 {
			addrs = append(addrs, "item("+names[0]+")")
		}
		if texts := navItemTexts(itemDoc); len(texts) > 0 {
			addrs = append(addrs, "item('"+strings.ReplaceAll(texts[0], "'", "''")+"')")
		}
		parts = append(parts, strings.Join(addrs, " = "))
	}
	return fmt.Sprintf("Items of %s: %s", list, strings.Join(parts, "; "))
}

// navItemTexts returns the texts an item shows: every translation of every
// text under its widgets, in document order.
func navItemTexts(itemDoc bson.D) []string {
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch d := v.(type) {
		case bson.D:
			if bsonnav.DGetString(d, "$Type") == "Texts$Translation" {
				if t := bsonnav.DGetString(d, "Text"); t != "" && !containsString(out, t) {
					out = append(out, t)
				}
				return
			}
			for _, e := range d {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range d {
				walk(e)
			}
		}
	}
	walk(bsonnav.DGet(itemDoc, "Widgets"))
	return out
}

// navItemWidgetNames returns the names of the widgets under an item.
func navItemWidgetNames(itemDoc bson.D) []string {
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch d := v.(type) {
		case bson.D:
			if n := bsonnav.DGetString(d, "Name"); n != "" && strings.Contains(bsonnav.DGetString(d, "$Type"), "$") {
				out = append(out, n)
			}
			for _, e := range d {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range d {
				walk(e)
			}
		}
	}
	walk(bsonnav.DGet(itemDoc, "Widgets"))
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// navItemAddressError is the precise miss for an item address, or nil when the
// ref is not one.
func (m *Mutator) navItemAddressError(ref string) error {
	sel, ok := backend.ParseNavItemSelector(ref)
	if !ok {
		return nil
	}
	if _, err := resolveNavItem(m.rawData, m.widgetFinder, sel); err != nil {
		return err
	}
	return nil
}

// refuseNavItemWidgetEdit refuses putting ordinary widgets where only items
// belong: INSERT BEFORE/AFTER an item and REPLACE of an item with widgets
// would put them into the list's Items. What an item does support — SET, DROP,
// INSERT INTO it, placing an item next to it — goes through the other paths.
func refuseNavItemWidgetEdit(result *bsonWidgetResult, name, op string) error {
	if result == nil || widgetTypeName(result.widget) != navListItemType {
		return nil
	}
	return fmt.Errorf("%s is a navigation list item: %s would put widgets into the list's items, "+
		"where only items belong. Use `insert into %s { … }` to add widgets to the item, or write "+
		"`item (…) { … }` blocks to add or replace items", name, op, name)
}

// ReplaceNavigationListItem replaces one navigation list item with the given
// items, in its place. The target must resolve to an item (`nav item@2`);
// replacing the whole list stays an ordinary widget REPLACE.
func (m *Mutator) ReplaceNavigationListItem(targetRef string, items []*pages.NavigationListItem) error {
	result := m.widgetFinder(m.rawData, targetRef)
	if result == nil {
		return m.widgetNotFoundError(targetRef)
	}
	if widgetTypeName(result.widget) != navListItemType {
		return fmt.Errorf("cannot replace %q (%s) with navigation list items: only an item can be — "+
			"address one as `<list> item(<widget>)`, `<list> item('<text>')` or `<list> item@n`",
			targetRef, widgetTypeName(result.widget))
	}
	newDocs, err := m.serializeNavigationListItems(items)
	if err != nil {
		return err
	}
	out := make([]any, 0, len(result.parentArr)-1+len(newDocs))
	out = append(out, result.parentArr[:result.index]...)
	out = append(out, newDocs...)
	out = append(out, result.parentArr[result.index+1:]...)
	bsonnav.DSetArray(result.parentDoc, result.parentKey, out)
	return nil
}
