// SPDX-License-Identifier: Apache-2.0

package ast

import "github.com/mendixlabs/mxcli/mdl/types"

// AlterNavigationStmt represents: CREATE [OR REPLACE] NAVIGATION <profile> [clauses...]
// This is a full-replacement command: omitted clauses clear that section.
type AlterNavigationStmt struct {
	ProfileName  string           // e.g. "Responsive"
	HomePages    []NavHomePageDef // HOME PAGE/MICROFLOW ... [FOR role]
	LoginPage    *QualifiedName   // LOGIN PAGE ...
	NotFoundPage *QualifiedName   // NOT FOUND PAGE ...
	MenuItems    []NavMenuItemDef // MENU (...) block
	HasMenuBlock bool             // true if MENU (...) was present (even if empty → clears menu)
	SyncEntries  []NavSyncDef     // SYNC (...) block — offline synchronization
	HasSyncBlock bool             // true if SYNC (...) was present (even if empty → clears the list)
	// ThrowSyncError is ON SYNC ERROR THROW|CONTINUE, and is a POINTER so an
	// omitted clause leaves the stored value alone. A plain bool would make
	// every rewrite that never mentions it reset the flag to false — the
	// guard-don't-drop failure, in the one property on this statement that is
	// a bare boolean and so has no "unset" value of its own.
	ThrowSyncError *bool
	CreateOrModify bool // true if CREATE OR REPLACE/MODIFY was used
}

func (s *AlterNavigationStmt) isStatement() {}

// NavSyncDef represents one `SYNC <entity> <mode>` line inside a SYNC block.
//
// Mode carries the STORED enum member, not the word the user typed: the visitor
// maps ONLINE/ALL/NEVER/NONE/NONE PRESERVE DATA/WHERE onto Online/All/Never/
// None/NoneAndPreserveData/Constrained, so nothing downstream has to know the
// spelling. Studio Pro's captions ("All Objects", "By XPath") are not members
// of the enumeration and never appear here.
type NavSyncDef struct {
	Entity QualifiedName
	Mode   string
	// Constraint is the XPath from a WHERE clause, and is set only when Mode is
	// Constrained — the two are derived from the same alternative precisely so
	// they cannot disagree.
	Constraint string
}

// NavHomePageDef represents a HOME PAGE or HOME MICROFLOW clause.
type NavHomePageDef struct {
	IsPage  bool           // true = PAGE, false = MICROFLOW
	Target  QualifiedName  // the page or microflow qualified name
	ForRole *QualifiedName // nil for default home, set for role-based
}

// NavMenuItemDef represents a MENU ITEM or MENU sub-menu definition.
type NavMenuItemDef struct {
	Caption   string         // from STRING_LITERAL
	Page      *QualifiedName // PAGE target
	Microflow *QualifiedName // MICROFLOW target
	SignOut   bool           // SIGN_OUT — the third action a menu item can carry
	Icon      string         // the qualified name, for the collection and image kinds
	// IconKind says WHICH of Mendix's three icon elements was written. They are
	// not variants of one value — a glyph carries a numeric code and no name —
	// so a single Icon string could express only one of the three, and the other
	// two were destroyed on rewrite.
	IconKind types.MenuIconKind
	// IconCode is the glyph's numeric character code, set only for MenuIconGlyph.
	IconCode int
	Items    []NavMenuItemDef // Sub-items (for MENU 'caption' (...))
}

// CreateMenuStmt is `create [or modify] menu Module.Name ( <items> )` — a
// standalone Menus$MenuDocument, not the menu inside a navigation profile.
// Items reuse NavMenuItemDef so both constructs share one item syntax.
//
// Like CREATE NAVIGATION, this is a full replacement: the item list given is the
// document's complete contents, so an omitted item is a removed item.
type CreateMenuStmt struct {
	Folder           string // Folder path within module (empty = leave placement alone)
	Name             QualifiedName
	Items            []NavMenuItemDef
	CreateOrModify   bool // CREATE OR MODIFY / OR REPLACE
	Documentation    string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
}

func (s *CreateMenuStmt) isStatement() {}

// DropMenuStmt is `drop menu Module.Name`.
type DropMenuStmt struct {
	Name QualifiedName
	DropIfExists
}

func (s *DropMenuStmt) isStatement() {}
