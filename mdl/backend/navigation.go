// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// NavigationBackend provides navigation document operations.
type NavigationBackend interface {
	ListNavigationDocuments() ([]*types.NavigationDocument, error)
	GetNavigation() (*types.NavigationDocument, error)
	UpdateNavigationProfile(navDocID model.ID, profileName string, spec types.NavigationProfileSpec) error
	// AddNavigationProfile appends a new web (Responsive, Phone, Tablet, or their
	// *Offline twins) or native (NativePhone) navigation profile. Mendix's set is
	// fixed for both families, so this takes a kind/name rather than an
	// arbitrary one. Callers create then update: this leaves the profile empty
	// (or, for native, with its home page/menu still unset) and
	// UpdateNavigationProfile fills it.
	AddNavigationProfile(navDocID model.ID, name string) error

	// OpenNavigationProfileForMutation loads the navigation document and
	// returns a mutator over one web profile's menu and page clauses, for
	// ALTER NAVIGATION. A native profile is refused. Nothing is written until
	// Save; a mutator that is never saved (check -p) changes nothing.
	OpenNavigationProfileForMutation(profileName string) (NavigationProfileMutator, error)

	// Menu documents are standalone reusable menus (Menus$MenuDocument), not
	// the menu embedded in a navigation profile.
	ListMenuDocuments() ([]*types.MenuDocument, error)
	GetMenuDocumentByQualifiedName(moduleName, name string) (*types.MenuDocument, error)
	CreateMenuDocument(md *types.MenuDocument) error
	UpdateMenuDocument(md *types.MenuDocument) error
	DeleteMenuDocument(id model.ID) error
	// OpenMenuDocumentForMutation loads a menu document and returns a mutator
	// over its items, for ALTER MENU.
	OpenMenuDocumentForMutation(id model.ID) (MenuMutator, error)
}

// MenuPosition is where an inserted or moved item goes relative to a target.
type MenuPosition string

const (
	MenuBefore MenuPosition = "before"
	MenuAfter  MenuPosition = "after"
	MenuInto   MenuPosition = "into" // as the target's last sub-item
)

// MenuMutator splices one document's menu items in the stored BSON (ALTER
// NAVIGATION / ALTER MENU, ADR-0012 decision 3). Untouched items keep their
// stored bytes; new items get fresh $IDs; no stored $ID is rewritten.
//
// It takes positions, not addresses: the caller resolves an address against
// MenuTree with ResolveMenuItemAddress and applies its refusals there, so
// `check -p` and `exec` run one function and agree. MenuTree reflects every
// operation applied so far, which is what makes resolution sequential.
type MenuMutator interface {
	// MenuTree is the menu as a target sees it, now.
	MenuTree() []*MenuTreeNode
	// InsertMenuItems places new items in the list of parent (nil: the top
	// level) at index; index == the list's length appends.
	InsertMenuItems(parent MenuItemPath, index int, items []types.NavMenuItemSpec) error
	// ReplaceMenuItem puts new items where the item at path was, carrying the
	// other-language translations of a caption the replacement keeps.
	ReplaceMenuItem(path MenuItemPath, items []types.NavMenuItemSpec) error
	// MoveMenuItem relocates the stored item (its bytes, $ID, sub-items and
	// translations) before or after target, or into it as its last sub-item.
	MoveMenuItem(from, target MenuItemPath, position MenuPosition) error
	// DropMenuItem removes the item at path and its sub-items.
	DropMenuItem(path MenuItemPath) error
	// SetMenuItemCaption writes the authoring language's translation and keeps
	// the others, as ALTER PAGE sets a text.
	SetMenuItemCaption(path MenuItemPath, caption string) error
	// SetMenuItemAction replaces the item's action with the one item states.
	SetMenuItemAction(path MenuItemPath, item types.NavMenuItemSpec) error
	// SetMenuItemIcon replaces the item's icon; no icon in item stores null.
	SetMenuItemIcon(path MenuItemPath, item types.NavMenuItemSpec) error
	// Save writes the document as a patch (canon.Reconcile, elided when
	// nothing changed).
	Save() error
}

// NavigationProfileMutator is a MenuMutator over a web profile's menu that also
// sets and clears the profile's page clauses.
type NavigationProfileMutator interface {
	MenuMutator
	// SetHomePage sets the default home (ForRole empty) or one role's,
	// updating a stored element in place.
	SetHomePage(home types.NavHomePageSpec) error
	// DropRoleHomePage removes a role's home page; a role with none is an error.
	DropRoleHomePage(role string) error
	// SetLoginPage sets the login page; "" clears it (Form "", as Studio Pro).
	SetLoginPage(page string) error
	// SetNotFoundPage sets the fallback page; "" clears it (null).
	SetNotFoundPage(page string) error
}
