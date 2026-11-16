// SPDX-License-Identifier: Apache-2.0

// Package menumutator is the splice behind ALTER NAVIGATION and ALTER MENU: it
// edits the Menus$MenuItemCollection of a navigation profile or of a menu
// document in the stored BSON, item by item.
//
// The two documents hold the same item tree (Menus$MenuItem, Items marker 3);
// only the key that holds the collection differs — `Menu` on a
// Navigation$NavigationProfile, `ItemCollection` on a Menus$MenuDocument — so
// one mutator serves both, and the profile's page clauses are a small addition
// on top.
//
// It is a splice, not a rebuild (ADR-0012 decision 3). An item no operation
// names stays the bson.D it was decoded into and re-encodes to the same bytes;
// a moved item is the same bson.D in another list, so it keeps its $ID, its
// sub-items, every caption translation and an action MDL cannot spell. Only new
// nodes get fresh $IDs, and no stored $ID is ever rewritten (CLAUDE.md rules 1
// and 3). The unit is saved as a patch (UpdateRawUnitPatch), so canon does not
// re-pair element $IDs the splice deliberately kept, nor carry back a
// translation it deliberately left out.
//
// Resolution and refusals are not here: the mutator takes positions the caller
// resolved (backend.ResolveMenuItemAddress over MenuTree), so check and exec
// run one function over it and refuse alike.
package menumutator

import (
	"bytes"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// Typed-array markers Studio Pro writes for the lists this package creates
// (censused in modelsdkbackend's navMarker* comment).
const (
	markerItems     = int32(3)
	markerHomeItems = int32(2)
	markerParams    = int32(2)
)

// Deps builds the new nodes, the way `create navigation` builds them, and
// persists the unit. The backend implements it.
type Deps interface {
	// BuildMenuItem is a Studio Pro-shaped Menus$MenuItem, sub-items included,
	// with fresh $IDs.
	BuildMenuItem(spec types.NavMenuItemSpec) (bson.D, error)
	// BuildMenuAction is the client action an item's OnClick stores.
	BuildMenuAction(spec types.NavMenuItemSpec) (bson.D, error)
	// BuildMenuIcon is the icon element, or nil for no icon.
	BuildMenuIcon(spec types.NavMenuItemSpec) any
	// SaveUnit writes the unit as a patch.
	SaveUnit(unitID string, contents []byte) error
}

// Mutator edits one document's menu. Create it with NewProfile or
// NewMenuDocument.
type Mutator struct {
	root    bson.D
	unitID  string
	deps    Deps
	profile bson.D // the profile, for a navigation document; nil for a menu document
	collKey string // the key holding the collection in its owner
}

var (
	_ backend.MenuMutator              = (*Mutator)(nil)
	_ backend.NavigationProfileMutator = (*Mutator)(nil)
)

// NewProfile opens the menu of the web profile named profileName in a
// Navigation$NavigationDocument. A native profile is refused: its bottom bar
// is a different element (BottomBarItems) that mxcli does not write.
func NewProfile(root bson.D, unitID, profileName string, deps Deps) (*Mutator, error) {
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(root, "Profiles")) {
		prof, ok := p.(bson.D)
		if !ok || !strings.EqualFold(bsonnav.DGetString(prof, "Name"), profileName) {
			continue
		}
		if t := bsonnav.DGetString(prof, "$Type"); t != "Navigation$NavigationProfile" {
			return nil, fmt.Errorf("navigation profile %s is a %s, not a web profile: its menu is a bottom bar mxcli does not write", profileName, t)
		}
		return &Mutator{root: root, unitID: unitID, deps: deps, profile: prof, collKey: "Menu"}, nil
	}
	return nil, fmt.Errorf("navigation profile not found: %s", profileName)
}

// NewMenuDocument opens a Menus$MenuDocument's items.
func NewMenuDocument(root bson.D, unitID string, deps Deps) (*Mutator, error) {
	if t := bsonnav.DGetString(root, "$Type"); t != "Menus$MenuDocument" {
		return nil, fmt.Errorf("unit %s is a %s, not a menu document", unitID, t)
	}
	return &Mutator{root: root, unitID: unitID, deps: deps, collKey: "ItemCollection"}, nil
}

// owner is the document holding the collection.
func (m *Mutator) owner() bson.D {
	if m.profile != nil {
		return m.profile
	}
	return m.root
}

// collection is the Menus$MenuItemCollection, created (as Studio Pro shapes
// it) when the owner stores none.
func (m *Mutator) collection() bson.D {
	if c := bsonnav.DGetDoc(m.owner(), m.collKey); c != nil {
		return c
	}
	c := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Menus$MenuItemCollection"},
		{Key: "Items", Value: bson.A{markerItems}},
	}
	m.setOwnerField(m.collKey, c)
	return c
}

// setOwnerField sets key on the owner document, appending it when absent. The
// owner is a slice inside its parent, so an append is written back there.
func (m *Mutator) setOwnerField(key string, value any) {
	if bsonnav.DSet(m.owner(), key, value) {
		return
	}
	grown := append(m.owner(), bson.E{Key: key, Value: value})
	if m.profile == nil {
		m.root = grown
		return
	}
	m.replaceProfile(grown)
}

// replaceProfile swaps the profile document for grown in the Profiles list.
func (m *Mutator) replaceProfile(grown bson.D) {
	for _, e := range m.root {
		if e.Key != "Profiles" {
			continue
		}
		arr, _ := e.Value.(bson.A)
		for i, p := range arr {
			if d, ok := p.(bson.D); ok && sameDoc(d, m.profile) {
				arr[i] = grown
			}
		}
	}
	m.profile = grown
}

// sameDoc reports whether a and b share their first element, i.e. are the same
// decoded document.
func sameDoc(a, b bson.D) bool {
	return len(a) > 0 && len(b) > 0 && &a[0] == &b[0]
}

// list is the Items list (marker included) of the item at parent, or of the
// collection for a nil parent, with the document that holds it.
func (m *Mutator) list(parent backend.MenuItemPath) (bson.A, bson.D, error) {
	holder := m.collection()
	for depth, i := range parent {
		items := itemElements(holder)
		if i < 0 || i >= len(items) {
			return nil, nil, fmt.Errorf("menu position %v does not exist (depth %d)", parent, depth)
		}
		holder = items[i]
	}
	arr, _ := bsonnav.DGet(holder, "Items").(bson.A)
	return arr, holder, nil
}

// itemElements are the Menus$MenuItem documents of a holder's Items, without
// the marker.
func itemElements(holder bson.D) []bson.D {
	var out []bson.D
	for _, v := range bsonnav.DGetArrayElements(bsonnav.DGet(holder, "Items")) {
		if d, ok := v.(bson.D); ok {
			out = append(out, d)
		}
	}
	return out
}

// setItems stores elements (without marker) as the holder's Items, keeping the
// stored marker, or Studio Pro's 3 for a list that had none.
func setItems(holder bson.D, elements []bson.D) error {
	marker := any(markerItems)
	if arr, ok := bsonnav.DGet(holder, "Items").(bson.A); ok && len(arr) > 0 && bsonnav.IsListMarker(arr[0]) {
		marker = arr[0]
	}
	out := bson.A{marker}
	for _, e := range elements {
		out = append(out, e)
	}
	if !bsonnav.DSet(holder, "Items", out) {
		return fmt.Errorf("menu item %s has no Items list", idString(holder))
	}
	return nil
}

// item is the Menus$MenuItem at path.
func (m *Mutator) item(path backend.MenuItemPath) (bson.D, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("empty menu position")
	}
	_, holder, err := m.list(path.Parent())
	if err != nil {
		return nil, err
	}
	items := itemElements(holder)
	if path.Index() < 0 || path.Index() >= len(items) {
		return nil, fmt.Errorf("menu position %v does not exist", path)
	}
	return items[path.Index()], nil
}

// MenuTree is the menu as a target sees it: each item's caption read with the
// page rule (model.SelectTranslation in the authoring language), whether its
// action is "Do nothing", and its sub-items.
func (m *Mutator) MenuTree() []*backend.MenuTreeNode {
	return treeOf(m.collection())
}

func treeOf(holder bson.D) []*backend.MenuTreeNode {
	var out []*backend.MenuTreeNode
	for _, it := range itemElements(holder) {
		out = append(out, &backend.MenuTreeNode{
			Caption:  captionOf(it),
			NoAction: isNoAction(bsonnav.DGetDoc(it, "Action")),
			Items:    treeOf(it),
		})
	}
	return out
}

// captionOf reads an item's caption as describe prints it.
func captionOf(item bson.D) string {
	return model.SelectTranslation(bsonnav.TextsTextTranslations(bsonnav.DGetDoc(item, "Caption")), model.AuthoringLanguage())
}

// isNoAction reports Studio Pro's "Do nothing" (or no action at all).
func isNoAction(action bson.D) bool {
	switch bsonnav.DGetString(action, "$Type") {
	case "", "Forms$NoAction", "Forms$NoClientAction", "Pages$NoClientAction":
		return true
	}
	return false
}

// InsertMenuItems builds items and places them in the list of parent (nil:
// the top level) at index; an index equal to the list's length appends.
func (m *Mutator) InsertMenuItems(parent backend.MenuItemPath, index int, specs []types.NavMenuItemSpec) error {
	built, err := m.build(specs)
	if err != nil {
		return err
	}
	_, holder, err := m.list(parent)
	if err != nil {
		return err
	}
	items := itemElements(holder)
	if index < 0 || index > len(items) {
		return fmt.Errorf("menu position %d is outside a list of %d items", index, len(items))
	}
	built, err = withoutItemsAlreadyThere(items, built)
	if err != nil {
		return err
	}
	if len(built) == 0 {
		return nil
	}
	out := append(append(append([]bson.D(nil), items[:index]...), built...), items[index:]...)
	return setItems(holder, out)
}

// withoutItemsAlreadyThere drops from built each item the list already holds
// as the statement builds it, so a script that inserts a menu item can run
// again without adding it twice (ako/mxcli#2312). An item of the same caption
// that is built differently — another action, other sub-items — is refused:
// inserting it would leave two items of one caption, and skipping it would
// drop the statement's change.
func withoutItemsAlreadyThere(items, built []bson.D) ([]bson.D, error) {
	var out []bson.D
	for _, b := range built {
		caption := captionOf(b)
		var same *bson.D
		for i := range items {
			if captionOf(items[i]) == caption {
				same = &items[i]
				break
			}
		}
		if same == nil {
			out = append(out, b)
			continue
		}
		if sameMenuItem(*same, b) {
			continue
		}
		return nil, fmt.Errorf("the list already has an item %q, with another action or other sub-items — "+
			"`replace '%s' with { … }` changes it; insert would leave two items of one caption", caption, caption)
	}
	return out, nil
}

// sameMenuItem reports whether stored is the item built would write: equal
// once element ids are ignored and stored's captions are kept to the languages
// built writes (a translation added in Studio Pro is not a difference).
func sameMenuItem(stored, built bson.D) bool {
	langs := map[string]bool{}
	collectLanguages(built, langs)
	a, err1 := bson.Marshal(comparableItem(stored, langs))
	b, err2 := bson.Marshal(comparableItem(built, langs))
	if err1 != nil || err2 != nil {
		return false
	}
	return bytes.Equal(a, b)
}

func collectLanguages(v any, into map[string]bool) {
	switch n := v.(type) {
	case bson.D:
		if bsonnav.DGetString(n, "$Type") == "Texts$Translation" {
			into[bsonnav.DGetString(n, "LanguageCode")] = true
		}
		for _, e := range n {
			collectLanguages(e.Value, into)
		}
	case bson.A:
		for _, e := range n {
			collectLanguages(e, into)
		}
	}
}

// comparableItem is a copy of v without $IDs, with its keys sorted, and with
// only the translations in langs.
func comparableItem(v any, langs map[string]bool) any {
	switch n := v.(type) {
	case bson.D:
		out := bson.D{}
		for _, e := range n {
			if e.Key == "$ID" {
				continue
			}
			out = append(out, bson.E{Key: e.Key, Value: comparableItem(e.Value, langs)})
		}
		sortKeys(out)
		return out
	case bson.A:
		out := bson.A{}
		for _, e := range n {
			if d, ok := e.(bson.D); ok && bsonnav.DGetString(d, "$Type") == "Texts$Translation" &&
				!langs[bsonnav.DGetString(d, "LanguageCode")] {
				continue
			}
			out = append(out, comparableItem(e, langs))
		}
		return out
	}
	return v
}

func sortKeys(d bson.D) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j-1].Key > d[j].Key; j-- {
			d[j-1], d[j] = d[j], d[j-1]
		}
	}
}

func (m *Mutator) build(specs []types.NavMenuItemSpec) ([]bson.D, error) {
	var out []bson.D
	for _, s := range specs {
		d, err := m.deps.BuildMenuItem(s)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// ReplaceMenuItem puts new items where the item at path was. The new items get
// fresh $IDs; one whose caption equals a replaced item's (paired by caption at
// each level) is given that caption's other-language translations, which the
// patch write would otherwise not carry.
func (m *Mutator) ReplaceMenuItem(path backend.MenuItemPath, specs []types.NavMenuItemSpec) error {
	old, err := m.item(path)
	if err != nil {
		return err
	}
	built, err := m.build(specs)
	if err != nil {
		return err
	}
	carryCaptionTranslations([]bson.D{old}, built)
	_, holder, err := m.list(path.Parent())
	if err != nil {
		return err
	}
	items := itemElements(holder)
	i := path.Index()
	out := append(append(append([]bson.D(nil), items[:i]...), built...), items[i+1:]...)
	return setItems(holder, out)
}

// carryCaptionTranslations pairs each new item with the first unclaimed old
// item of the same displayed caption and copies the old caption's translations
// in languages the new one lacks; then the same one level down.
func carryCaptionTranslations(old, built []bson.D) {
	used := make([]bool, len(old))
	for _, b := range built {
		caption := captionOf(b)
		for i, o := range old {
			if used[i] || captionOf(o) != caption {
				continue
			}
			used[i] = true
			copyMissingTranslations(bsonnav.DGetDoc(o, "Caption"), bsonnav.DGetDoc(b, "Caption"))
			carryCaptionTranslations(itemElements(o), itemElements(b))
			break
		}
	}
}

// copyMissingTranslations appends to dst a copy (fresh $ID) of each of src's
// translations in a language dst has no entry for.
func copyMissingTranslations(src, dst bson.D) {
	if src == nil || dst == nil {
		return
	}
	have := map[string]bool{}
	for _, e := range bsonnav.TextsTextTranslations(dst) {
		have[e.LanguageCode] = true
	}
	arr, ok := bsonnav.DGet(dst, "Items").(bson.A)
	if !ok {
		return
	}
	for _, e := range bsonnav.TextsTextTranslations(src) {
		if have[e.LanguageCode] {
			continue
		}
		have[e.LanguageCode] = true
		arr = append(arr, translationDoc(e.LanguageCode, e.Text))
	}
	bsonnav.DSet(dst, "Items", arr)
}

func translationDoc(lang, text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Texts$Translation"},
		{Key: "LanguageCode", Value: lang},
		{Key: "Text", Value: text},
	}
}

// MoveMenuItem relocates the stored item at from — the same bson.D, so its
// bytes, $ID, sub-items and translations go with it — before or after the
// item at target, or as target's last sub-item. Moving an item into its own
// subtree, or relative to itself, is refused.
func (m *Mutator) MoveMenuItem(from, target backend.MenuItemPath, position backend.MenuPosition) error {
	if target.HasPrefix(from) {
		if len(target) == len(from) {
			return fmt.Errorf("an item cannot be moved relative to itself")
		}
		return fmt.Errorf("an item cannot be moved into its own sub-items")
	}
	moved, err := m.item(from)
	if err != nil {
		return err
	}
	targetDoc, err := m.item(target)
	if err != nil {
		return err
	}
	targetID := idOf(targetDoc)

	// Take it out of its list first; the target is then found again by $ID,
	// since removing an earlier sibling shifts its position.
	_, srcHolder, err := m.list(from.Parent())
	if err != nil {
		return err
	}
	src := itemElements(srcHolder)
	rest := append(append([]bson.D(nil), src[:from.Index()]...), src[from.Index()+1:]...)
	if err := setItems(srcHolder, rest); err != nil {
		return err
	}

	tpath := m.findByID(targetID)
	if tpath == nil {
		return fmt.Errorf("move target lost after removing the moved item")
	}
	switch position {
	case backend.MenuInto:
		_, holder, err := m.list(tpath)
		if err != nil {
			return err
		}
		return setItems(holder, append(itemElements(holder), moved))
	case backend.MenuBefore, backend.MenuAfter:
		_, holder, err := m.list(tpath.Parent())
		if err != nil {
			return err
		}
		items := itemElements(holder)
		at := tpath.Index()
		if position == backend.MenuAfter {
			at++
		}
		out := append(append(append([]bson.D(nil), items[:at]...), moved), items[at:]...)
		return setItems(holder, out)
	}
	return fmt.Errorf("unknown move position %q", position)
}

// findByID is the path of the item whose $ID equals id.
func (m *Mutator) findByID(id []byte) backend.MenuItemPath {
	var walk func(holder bson.D, prefix backend.MenuItemPath) backend.MenuItemPath
	walk = func(holder bson.D, prefix backend.MenuItemPath) backend.MenuItemPath {
		for i, it := range itemElements(holder) {
			p := append(append(backend.MenuItemPath(nil), prefix...), i)
			if bytes.Equal(idOf(it), id) {
				return p
			}
			if found := walk(it, p); found != nil {
				return found
			}
		}
		return nil
	}
	return walk(m.collection(), nil)
}

func idOf(doc bson.D) []byte {
	switch v := bsonnav.DGet(doc, "$ID").(type) {
	case bson.Binary:
		return v.Data
	case string:
		return []byte(v)
	}
	return nil
}

func idString(doc bson.D) string {
	return fmt.Sprintf("%x", idOf(doc))
}

// DropMenuItem removes the item at path and its sub-items.
func (m *Mutator) DropMenuItem(path backend.MenuItemPath) error {
	if _, err := m.item(path); err != nil {
		return err
	}
	_, holder, err := m.list(path.Parent())
	if err != nil {
		return err
	}
	items := itemElements(holder)
	i := path.Index()
	return setItems(holder, append(append([]bson.D(nil), items[:i]...), items[i+1:]...))
}

// SetMenuItemCaption writes caption as the authoring language's translation of
// the item's caption — updated in place, or added — and keeps every other
// language's, as ALTER PAGE sets a text (bsonnav.SetTextsTextTranslation).
func (m *Mutator) SetMenuItemCaption(path backend.MenuItemPath, caption string) error {
	it, err := m.item(path)
	if err != nil {
		return err
	}
	if text := bsonnav.DGetDoc(it, "Caption"); text != nil && bsonnav.SetTextsTextTranslation(text, caption) {
		return nil
	}
	// No Texts$Text stored (a null caption): build the one create writes.
	text := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{markerItems, translationDoc(model.AuthoringLanguage(), caption)}},
	}
	return m.setItemField(path, "Caption", text)
}

// SetMenuItemAction replaces the item's client action with the one spec
// states. The stored action travels as spec.StoredAction, so a rebuilt action
// that equals it except for what MDL cannot spell keeps the stored bytes.
func (m *Mutator) SetMenuItemAction(path backend.MenuItemPath, spec types.NavMenuItemSpec) error {
	it, err := m.item(path)
	if err != nil {
		return err
	}
	if stored := bsonnav.DGetDoc(it, "Action"); stored != nil {
		if raw, err := bson.Marshal(stored); err == nil {
			spec.StoredAction = raw
		}
	}
	action, err := m.deps.BuildMenuAction(spec)
	if err != nil {
		return err
	}
	return m.setItemField(path, "Action", action)
}

// SetMenuItemIcon replaces the item's icon; a spec with no icon stores null.
func (m *Mutator) SetMenuItemIcon(path backend.MenuItemPath, spec types.NavMenuItemSpec) error {
	return m.setItemField(path, "Icon", m.deps.BuildMenuIcon(spec))
}

// setItemField sets key on the item at path. A key the stored item lacks is
// inserted where Studio Pro keeps it — properties in alphabetical order after
// $ID and $Type — and the grown item is written back into its list: an item
// `create menu` wrote has no Icon key when it has no icon, where every Studio
// Pro item carries `Icon: null`.
func (m *Mutator) setItemField(path backend.MenuItemPath, key string, value any) error {
	it, err := m.item(path)
	if err != nil {
		return err
	}
	if bsonnav.DSet(it, key, value) {
		return nil
	}
	at := len(it)
	for i, e := range it {
		if !strings.HasPrefix(e.Key, "$") && e.Key > key {
			at = i
			break
		}
	}
	grown := append(append(append(bson.D(nil), it[:at]...), bson.E{Key: key, Value: value}), it[at:]...)
	_, holder, err := m.list(path.Parent())
	if err != nil {
		return err
	}
	items := itemElements(holder)
	items[path.Index()] = grown
	return setItems(holder, items)
}

// --- the profile's page clauses ---

func (m *Mutator) requireProfile(what string) error {
	if m.profile == nil {
		return fmt.Errorf("%s: a menu document has no page clauses", what)
	}
	return nil
}

// SetHomePage sets the default home page (ForRole empty) or one role's. A
// stored element is updated in place, keeping its $ID; a role with none gets a
// new Navigation$RoleBasedHomePage, appended after the others.
func (m *Mutator) SetHomePage(home types.NavHomePageSpec) error {
	if err := m.requireProfile("set home"); err != nil {
		return err
	}
	page, mf := home.Target, ""
	if !home.IsPage {
		page, mf = "", home.Target
	}
	if home.ForRole == "" {
		if hp := bsonnav.DGetDoc(m.profile, "HomePage"); hp != nil && bsonnav.DGetString(hp, "$Type") == "Navigation$HomePage" {
			bsonnav.DSet(hp, "Page", page)
			bsonnav.DSet(hp, "Microflow", mf)
			return nil
		}
		m.setOwnerField("HomePage", bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Navigation$HomePage"},
			{Key: "Microflow", Value: mf},
			{Key: "Page", Value: page},
		})
		return nil
	}
	arr, _ := bsonnav.DGet(m.profile, "HomeItems").(bson.A)
	for _, v := range arr {
		d, ok := v.(bson.D)
		if ok && strings.EqualFold(bsonnav.DGetString(d, "UserRole"), home.ForRole) {
			bsonnav.DSet(d, "Page", page)
			bsonnav.DSet(d, "Microflow", mf)
			return nil
		}
	}
	if len(arr) == 0 || !bsonnav.IsListMarker(arr[0]) {
		arr = append(bson.A{markerHomeItems}, arr...)
	}
	arr = append(arr, bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Navigation$RoleBasedHomePage"},
		{Key: "Microflow", Value: mf},
		{Key: "Page", Value: page},
		{Key: "UserRole", Value: home.ForRole},
	})
	m.setOwnerField("HomeItems", arr)
	return nil
}

// DropRoleHomePage removes a role's home page, whatever its kind. A role with
// none is an error, not a silent no-op.
func (m *Mutator) DropRoleHomePage(role string) error {
	if err := m.requireProfile("drop home page"); err != nil {
		return err
	}
	arr, _ := bsonnav.DGet(m.profile, "HomeItems").(bson.A)
	out := bson.A{}
	found := false
	var roles []string
	for _, v := range arr {
		d, ok := v.(bson.D)
		if !ok {
			out = append(out, v) // the marker
			continue
		}
		r := bsonnav.DGetString(d, "UserRole")
		if !found && strings.EqualFold(r, role) {
			found = true
			continue
		}
		roles = append(roles, r)
		out = append(out, d)
	}
	if !found {
		if len(roles) == 0 {
			return fmt.Errorf("role %s has no home page: the profile has no role-based home pages", role)
		}
		return fmt.Errorf("role %s has no home page; the roles with one are %s", role, strings.Join(roles, ", "))
	}
	m.setOwnerField("HomeItems", out)
	return nil
}

// SetLoginPage sets the login page, or clears it for "" the way Studio Pro
// does: Form "" in the stored Forms$FormSettings, which stays.
func (m *Mutator) SetLoginPage(page string) error {
	if err := m.requireProfile("login page"); err != nil {
		return err
	}
	if ls := bsonnav.DGetDoc(m.profile, "LoginPageSettings"); ls != nil && bsonnav.HasKey(ls, "Form") {
		bsonnav.DSet(ls, "Form", page)
		return nil
	}
	if page == "" {
		return nil // nothing stored, nothing to clear
	}
	m.setOwnerField("LoginPageSettings", bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$FormSettings"},
		{Key: "Form", Value: page},
		{Key: "ParameterMappings", Value: bson.A{markerParams}},
		{Key: "TitleOverride", Value: nil},
	})
	return nil
}

// SetNotFoundPage sets the fallback page, or clears it for "": Studio Pro
// stores null there.
func (m *Mutator) SetNotFoundPage(page string) error {
	if err := m.requireProfile("not found page"); err != nil {
		return err
	}
	if page == "" {
		m.setOwnerField("NotFoundHomepage", nil)
		return nil
	}
	if nf := bsonnav.DGetDoc(m.profile, "NotFoundHomepage"); nf != nil && bsonnav.DGetString(nf, "$Type") == "Navigation$NotFoundHomePage" {
		bsonnav.DSet(nf, "Page", page)
		bsonnav.DSet(nf, "Microflow", "")
		return nil
	}
	m.setOwnerField("NotFoundHomepage", bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Navigation$NotFoundHomePage"},
		{Key: "Microflow", Value: ""},
		{Key: "Page", Value: page},
	})
	return nil
}

// Bytes is the unit as it would be saved.
func (m *Mutator) Bytes() ([]byte, error) {
	return bson.Marshal(m.root)
}

// Save writes the unit as a patch.
func (m *Mutator) Save() error {
	out, err := m.Bytes()
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return m.deps.SaveUnit(m.unitID, out)
}
