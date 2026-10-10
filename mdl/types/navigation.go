// SPDX-License-Identifier: Apache-2.0

package types

import (
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// NavigationDocument represents a parsed navigation document.
type NavigationDocument struct {
	model.BaseElement
	ContainerID model.ID             `json:"containerId"`
	Name        string               `json:"name"`
	Profiles    []*NavigationProfile `json:"profiles,omitempty"`
}

// GetName returns the navigation document's name.
func (nd *NavigationDocument) GetName() string { return nd.Name }

// GetContainerID returns the container ID.
func (nd *NavigationDocument) GetContainerID() model.ID { return nd.ContainerID }

// NavigationProfile represents a single navigation profile.
type NavigationProfile struct {
	Name               string              `json:"name"`
	Kind               string              `json:"kind"`
	IsNative           bool                `json:"isNative"`
	HomePage           *NavHomePage        `json:"homePage,omitempty"`
	RoleBasedHomePages []*NavRoleBasedHome `json:"roleBasedHomePages,omitempty"`
	LoginPage          string              `json:"loginPage,omitempty"`
	NotFoundPage       string              `json:"notFoundPage,omitempty"`
	MenuItems          []*NavMenuItem      `json:"menuItems,omitempty"`
	OfflineEntities    []*NavOfflineEntity `json:"offlineEntities,omitempty"`
	// ThrowPartialSyncError is stored on every WEB profile, online ones
	// included, and is declared by NEITHER modelsdk/gen NOR
	// generated/metamodel — measured on ako/TestApp, zero occurrences in each.
	// It is therefore read and written as raw BSON rather than through the
	// codec's typed accessors.
	ThrowPartialSyncError bool `json:"throwPartialSyncError,omitempty"`
	// ProgressiveWebApp is Navigation$ProgressiveWebAppSettings: nil when the
	// profile stores null, which is what a profile created in Studio Pro or by
	// mxcli carries until someone ticks "Progressive web app". An offline
	// profile with nil settings gets no service worker (mendixlabs/mxcli#1377).
	ProgressiveWebApp *NavPWASettings `json:"progressiveWebApp,omitempty"`
}

// NavPWASettings is a profile's Progressive web app settings, as stored.
type NavPWASettings struct {
	Precaching    bool `json:"precaching"`
	InstallPrompt bool `json:"installPrompt"`
}

// The platform defaults of Navigation$ProgressiveWebAppSettings, from Studio
// Pro's own schema: precaching false, installPrompt true. A bare
// `progressive web app` clause writes these, and describe omits a key that
// holds its default (R12).
const (
	NavPWADefaultPrecaching    = false
	NavPWADefaultInstallPrompt = true
)

// NavPWASpec is the PROGRESSIVE WEB APP clause. Off stores null; otherwise the
// stored settings (or the defaults, when there are none) are kept and the keys
// the clause names are overlaid -- nil keys are left as they are.
type NavPWASpec struct {
	Off           bool
	Precaching    *bool
	InstallPrompt *bool
}

// NavHomePage holds a profile's default home page.
type NavHomePage struct {
	Page      string `json:"page,omitempty"`
	Microflow string `json:"microflow,omitempty"`
}

// NavRoleBasedHome maps a user role to a home page.
type NavRoleBasedHome struct {
	UserRole  string `json:"userRole"`
	Page      string `json:"page,omitempty"`
	Microflow string `json:"microflow,omitempty"`
}

// NavMenuItem is a recursive navigation menu entry.
type NavMenuItem struct {
	Caption    string `json:"caption"`
	Page       string `json:"page,omitempty"`
	Microflow  string `json:"microflow,omitempty"`
	ActionType string `json:"actionType,omitempty"`
	// Icon is the qualified name an icon-collection or image icon points at.
	// Empty for no icon and for a glyph icon, which carries a numeric Code
	// instead. IconType keeps the storage $Type so a reader can tell the three
	// apart — DESCRIBE only round-trips Forms$IconCollectionIcon.
	Icon     string `json:"icon,omitempty"`
	IconType string `json:"iconType,omitempty"`
	// IconCode is Forms$GlyphIcon's numeric Code — the ONLY thing that
	// identifies a glyph icon, since it carries no qualified name. Without it a
	// reader knows a glyph was there but not which one, so it can neither be
	// re-emitted by DESCRIBE nor carried through a rewrite.
	IconCode int `json:"iconCode,omitempty"`
	// StoredAction is the item's client action exactly as stored (its raw
	// BSON document), so an action MDL cannot spell is carried through a
	// rewrite instead of being replaced by Forms$NoAction (ako/mxcli#980).
	StoredAction []byte `json:"-"`
	// ActionDoc is StoredAction decoded into the map shape the page describer's
	// client-action renderer reads, so a menu item's action is printed by the
	// same code as a button's.
	ActionDoc map[string]any `json:"-"`
	// Action is the action to write, built from MDL — a pages.ClientAction,
	// typed any because sdk/pages imports this package. Set on items the menu
	// document writer builds from a script; nil on items read from storage.
	Action any            `json:"-"`
	Items  []*NavMenuItem `json:"items,omitempty"`
}

// HasIcon reports whether the item carries an icon of ANY of the three kinds.
//
// Not `Icon != ""`: a glyph icon has a numeric code and no name, so the obvious
// test calls an item with a perfectly good icon iconless. MDL077 asks this
// question, and asking it the obvious way would have made the rule fire on every
// Studio Pro-authored menu.
func (m *NavMenuItem) HasIcon() bool {
	if m == nil {
		return false
	}
	switch MenuIconKindOf(m.IconType) {
	case MenuIconNone:
		return false
	case MenuIconGlyph:
		return true
	default:
		// A collection or image icon without a name is a malformed element, not
		// an icon anyone can see.
		return m.Icon != ""
	}
}

// MenuIconKind names which of Mendix's three icon elements a menu item carries.
//
// They are not variations on one shape: an icon-collection icon and an image
// icon each hold a qualified name (into an icon collection and an image
// collection respectively — different documents), while a glyph icon holds a
// numeric character code and no name at all. Treating them as one "icon string"
// is what made a rewrite silently convert a glyph into nothing.
type MenuIconKind string

const (
	MenuIconNone       MenuIconKind = ""
	MenuIconCollection MenuIconKind = "collection"
	MenuIconGlyph      MenuIconKind = "glyph"
	MenuIconImage      MenuIconKind = "image"
	// MenuIconUnknown is a stored $Type this build does not know. It is
	// deliberately NOT MenuIconNone: reporting an unrecognised element as "no
	// icon" is how a future fourth variant would get silently dropped by a
	// rewrite, which is the bug this vocabulary exists to prevent.
	MenuIconUnknown MenuIconKind = "unknown"
)

// MenuIconKindOf maps a stored $Type onto the vocabulary.
//
// Matched on the suffix because the same element has two spellings: the
// metamodel calls it Pages$IconCollectionIcon and storage calls it
// Forms$IconCollectionIcon ("Form" was the original term for "Page"). A reader
// handing over either name must land on the same kind.
func MenuIconKindOf(iconType string) MenuIconKind {
	switch {
	case iconType == "":
		return MenuIconNone
	case strings.HasSuffix(iconType, "IconCollectionIcon"):
		return MenuIconCollection
	case strings.HasSuffix(iconType, "GlyphIcon"):
		return MenuIconGlyph
	case strings.HasSuffix(iconType, "ImageIcon"):
		return MenuIconImage
	}
	return MenuIconUnknown
}

// MenuIconStorageType is the inverse: the $Type a writer must emit for a kind.
// Empty for MenuIconNone (no Icon element at all) and for MenuIconUnknown,
// which a writer must never invent a name for.
func MenuIconStorageType(kind MenuIconKind) string {
	switch kind {
	case MenuIconCollection:
		return "Forms$IconCollectionIcon"
	case MenuIconGlyph:
		return "Forms$GlyphIcon"
	case MenuIconImage:
		return "Forms$ImageIcon"
	}
	return ""
}

// MenuDocument is a standalone `Menus$MenuDocument` — a reusable menu that menu
// widgets point at, stored as its own document rather than inside a navigation
// profile. Atlas_Core ships two of them (Phone_Menu, Tablet_Menu).
//
// Its entries are the same `Menus$MenuItem` elements a navigation profile holds,
// so they are modelled as NavMenuItem rather than a parallel type — one item
// shape, one parser, one renderer.
type MenuDocument struct {
	ID            model.ID       `json:"id"`
	ContainerID   model.ID       `json:"containerId"`
	Name          string         `json:"name"`
	Documentation string         `json:"documentation,omitempty"`
	ExportLevel   string         `json:"exportLevel,omitempty"`
	Excluded      bool           `json:"excluded,omitempty"`
	Items         []*NavMenuItem `json:"items,omitempty"`
}

// GetName returns the menu document's name.
func (m *MenuDocument) GetName() string { return m.Name }

// NavOfflineEntity declares offline sync rules for an entity.
//
// These are the four properties Studio Pro writes on a web profile, measured
// against ako/TestApp's TabletOffline profile (seven configs, all six sync
// modes). modelsdk/gen declares two more — DownloadMode and ShouldDownload —
// which occur ZERO times in that document; they are presumably native-only, and
// a writer must not start emitting them. A property absent from every real
// document is one Studio Pro fills in on load, so writing it is how a document
// mxbuild accepts becomes one Studio Pro cannot open.
//
// CompatibilityMode is carried but not authorable. It exists so a future write
// path can put it back unchanged instead of dropping it — the mistake that had
// `create or modify entity` deleting access rules.
type NavOfflineEntity struct {
	Entity     string `json:"entity"`
	SyncMode   string `json:"syncMode"`
	Constraint string `json:"constraint,omitempty"`
	// CompatibilityMode is read and preserved, never authored. Every reference
	// config carries false; the true case has not been observed.
	CompatibilityMode bool `json:"compatibilityMode,omitempty"`
}

// NavigationProfileSpec specifies changes to a navigation profile.
type NavigationProfileSpec struct {
	HomePages    []NavHomePageSpec
	LoginPage    string
	NotFoundPage string
	MenuItems    []NavMenuItemSpec
	HasMenu      bool
	// OfflineEntities is the SYNC block. HasSync distinguishes "no block was
	// written, leave the stored list alone" from "an empty block was written,
	// clear it" — the same distinction HasMenu draws, and the reason a spec
	// field alone is not enough.
	OfflineEntities []NavOfflineEntitySpec
	HasSync         bool
	// ThrowSyncError is Studio Pro's "Throw error when server rejects objects
	// during synchronization". A POINTER, so nil means the statement said
	// nothing and the stored value is left alone — the property is a bare bool
	// with no unset value of its own, so a non-pointer would silently reset it
	// on every rewrite.
	ThrowSyncError *bool
	// ProgressiveWebApp is the PROGRESSIVE WEB APP clause; nil leaves the
	// stored settings exactly as they are.
	ProgressiveWebApp *NavPWASpec
}

// NavOfflineEntitySpec is one entity's offline sync rule, as MDL can express
// it. CompatibilityMode is deliberately absent: it is stored, carried on read
// and preserved on write, but there is no syntax for it — so a spec that could
// express it would invite a writer to set it from a value nobody supplied.
type NavOfflineEntitySpec struct {
	Entity     string
	SyncMode   string
	Constraint string
}

// NavHomePageSpec specifies a home page assignment.
type NavHomePageSpec struct {
	IsPage  bool
	Target  string
	ForRole string
}

// NavMenuItemSpec specifies a menu item (recursive).
type NavMenuItemSpec struct {
	Caption   string
	Page      string
	Microflow string
	// SignOut is the third action a menu item can carry. Studio Pro stores it
	// as the same Forms$SignOutClientAction a button uses, so it needs no
	// target — which is why it is a flag rather than another name field.
	SignOut bool
	// Icon is the qualified name for a collection or image icon
	// (Atlas_Core.Atlas.home). Empty with IconKind None means no icon, which
	// serializes as a null Icon.
	Icon string
	// IconKind selects which of the three icon elements to write. The spec used
	// to carry a name and nothing else, so every icon became a
	// Forms$IconCollectionIcon and a glyph turned into nothing on rewrite.
	IconKind MenuIconKind
	// IconCode is the glyph's numeric Code, meaningful only for MenuIconGlyph.
	IconCode int
	// KeepAction is a stored client action (raw BSON) the writer puts back
	// verbatim instead of building one from Page/Microflow/SignOut. The executor
	// sets it when the script's item states no action and the stored one is an
	// action MDL cannot express — writing Forms$NoAction there would silently
	// delete it (ako/mxcli#980).
	KeepAction []byte
	// Action is the item's action built from the script's OnClick — a
	// pages.ClientAction, typed any because sdk/pages imports this package. The
	// writer serializes it with the widget client-action serializer, so a menu
	// item's action is stored in the shape a button's is (ako/mxcli#980).
	Action any
	// StoredAction is the stored action of the item this one replaces (paired by
	// caption path). When Action builds the same action except for properties
	// MDL cannot spell, the writer keeps StoredAction, so those properties — a
	// page title override, a link type — survive a rewrite.
	StoredAction []byte
	Items        []NavMenuItemSpec
}
