// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strconv"
	"strings"
)

// ============================================================================
// ALTER PAGE / ALTER SNIPPET — in-place widget tree modification
// ============================================================================

// AlterPageStmt represents: ALTER PAGE/SNIPPET Module.Name { operations }
type AlterPageStmt struct {
	ContainerType string        // "PAGE" or "SNIPPET"
	PageName      QualifiedName // page or snippet qualified name
	Operations    []AlterPageOperation
}

func (s *AlterPageStmt) isStatement() {}

// AlterPageOperation is the interface for individual ALTER PAGE operations.
type AlterPageOperation interface {
	isAlterPageOperation()
}

// WidgetRef is the <target> of a generic ALTER operation as written: the
// element `set … on`, `insert before|after|into`, `replace … with` and `drop`
// address (ADR-0012 decision 2).
//
// One address syntax serves every document type, and the document type's
// resolver (backend.AlterTargetResolver) decides which forms it accepts:
//
//	btnSave            Widget="btnSave"
//	dgProducts.Name    Widget="dgProducts", Column="Name" (a grid column by derived name, a scroll-container region)
//	dg column(Name)    Widget="dg", ColumnAttribute="Name" (a DataGrid 2 column by its attribute, #749)
//	dg column('Total') Widget="dg", ColumnCaption="Total" (… by its caption)
//	'Approve order'    Caption="Approve order" (content addressing, for elements with no name)
//	nav item('Home')   Widget="nav item('Home')", Item={List: "nav", Text: "Home"} (a navigation list item, #1388)
//	hdr@2 / 'x'@2      Ordinal=2 — picks one of several matches; never a guess
//
// The name stays WidgetRef because the page family is the first document type
// on the generic path and every page operation already speaks it.
type WidgetRef struct {
	Widget  string // name (empty when the target is addressed by caption)
	Column  string // sub-element name within Widget (empty for a plain name)
	Caption string // quoted content address; empty when addressed by name
	Ordinal int    // @n, 1-based; 0 when absent
	// ColumnAttribute and ColumnCaption are the explicit column address
	// `Widget column(…)`: the column's Attribute as describe writes it, or its
	// Caption. At most one is set, and never together with Column. Ordinal
	// then chooses among the grid's columns that match.
	ColumnAttribute string
	ColumnCaption   string
	// Item is set for the navigation list item address `nav item(…)` /
	// `nav item@n`. Widget then holds the address as Item.Spell() writes it —
	// the spelling the page mutator's widget lookup resolves — so every
	// operation that takes a widget reaches the item unchanged. The item's @n
	// lives in Item, never in Ordinal.
	Item *NavItemRef
	// Cell is set for a legacy table cell address `table1 cell(row, column)`.
	// Widget then holds Cell.Spell(), the spelling the page mutator resolves.
	Cell *TableCellRef
}

// TableCellRef is a legacy table cell address. A cell stores no name; it is the
// cell whose top-left position is Row, Column, both counted from 1.
type TableCellRef struct {
	Table  string
	Row    int
	Column int
}

// Spell writes the address as the page mutator reads it
// (backend.ParseTableCellSelector).
func (r TableCellRef) Spell() string {
	return r.Table + " cell(" + strconv.Itoa(r.Row) + "," + strconv.Itoa(r.Column) + ")"
}

// NavItemRef is a navigation list item address (mendixlabs/mxcli#1388). An item
// has no name, so it is addressed by a widget it holds, a text it shows, or its
// position: at most one of Widget and Text is set, and with neither, Ordinal
// counts every item of the list.
type NavItemRef struct {
	List    string
	Widget  string
	Text    string
	Ordinal int
}

// Spell writes the address as MDL does. backend.ParseNavItemSelector reads this
// spelling; a test in the executor pins the two together.
func (r NavItemRef) Spell() string {
	s := r.List + " item"
	switch {
	case r.Text != "":
		s += "('" + strings.ReplaceAll(r.Text, "'", "''") + "')"
	case r.Widget != "":
		s += "(" + r.Widget + ")"
	}
	if r.Ordinal > 0 {
		s += "@" + strconv.Itoa(r.Ordinal)
	}
	return s
}

// IsColumnAddress reports whether the target is the explicit `grid column(…)`
// form.
func (r WidgetRef) IsColumnAddress() bool {
	return r.ColumnAttribute != "" || r.ColumnCaption != ""
}

// Name returns the full reference string for error messages, as it was written.
func (r WidgetRef) Name() string {
	var s string
	switch {
	case r.Caption != "":
		s = "'" + r.Caption + "'"
	case r.ColumnCaption != "":
		s = r.Widget + " column('" + strings.ReplaceAll(r.ColumnCaption, "'", "''") + "')"
	case r.ColumnAttribute != "":
		s = r.Widget + " column(" + r.ColumnAttribute + ")"
	case r.Column != "":
		s = r.Widget + "." + r.Column
	default:
		s = r.Widget
	}
	if r.Ordinal > 0 {
		s += "@" + strconv.Itoa(r.Ordinal)
	}
	return s
}

// IsColumn returns true if this addresses a member of a widget: a grid column
// (`dg.Name` or `dg column(Name)`) or a scroll-container region.
func (r WidgetRef) IsColumn() bool {
	return r.Column != "" || r.IsColumnAddress()
}

// SetPropertyOp represents: SET prop = value ON widgetRef
// or SET prop = value (page-level, Target.Widget empty)
type SetPropertyOp struct {
	Target     WidgetRef              // empty Widget for page-level SET
	Properties map[string]interface{} // property name -> value
}

func (s *SetPropertyOp) isAlterPageOperation() {}

// InsertWidgetOp represents: INSERT AFTER/BEFORE/INTO widgetRef { widgets }
type InsertWidgetOp struct {
	Position string    // "AFTER", "BEFORE", or "INTO" (append as children of the container)
	Target   WidgetRef // widget to insert relative to, or container to insert into
	Widgets  []*WidgetV3
}

func (s *InsertWidgetOp) isAlterPageOperation() {}

// DropWidgetOp represents: DROP WIDGET ref1, ref2, ...
type DropWidgetOp struct {
	Targets []WidgetRef
}

func (s *DropWidgetOp) isAlterPageOperation() {}

// MoveWidgetOp represents: MOVE widgetRef AFTER|BEFORE|INTO destinationRef.
// The widget's stored subtree is moved as it is (ako/mxcli#2122).
type MoveWidgetOp struct {
	Target      WidgetRef
	Position    string // "AFTER", "BEFORE" or "INTO"
	Destination WidgetRef
}

func (s *MoveWidgetOp) isAlterPageOperation() {}

// WrapWidgetOp represents: WRAP widgetRef IN { container }. The container is
// inserted where the widget is, and the widget moved into it (ako/mxcli#2122).
type WrapWidgetOp struct {
	Target    WidgetRef
	Container []*WidgetV3
}

func (s *WrapWidgetOp) isAlterPageOperation() {}

// DropListViewTemplateOp represents:
// DROP TEMPLATE FOR Module.Specialization IN listViewName
//
// A List View specialization template has no name, so it is addressed by the
// entity it renders plus the list view holding it — a page can carry two list
// views with a template for the same entity.
type DropListViewTemplateOp struct {
	Specialization string
	ListView       string
}

func (s *DropListViewTemplateOp) isAlterPageOperation() {}

// ReplaceWidgetOp represents: REPLACE widgetRef WITH { widgets }
type ReplaceWidgetOp struct {
	Target     WidgetRef
	NewWidgets []*WidgetV3
}

func (s *ReplaceWidgetOp) isAlterPageOperation() {}

// AddVariableOp represents: ADD Variables $name: Type = <default expression>
type AddVariableOp struct {
	Variable PageVariable
}

func (s *AddVariableOp) isAlterPageOperation() {}

// DropVariableOp represents: DROP Variables $name
type DropVariableOp struct {
	VariableName string // without $ prefix
}

func (s *DropVariableOp) isAlterPageOperation() {}

// AddParameterOp represents: ADD Parameters $Name: Type
type AddParameterOp struct {
	Parameter PageParameter
}

func (s *AddParameterOp) isAlterPageOperation() {}

// DropParameterOp represents: DROP Parameters $Name
type DropParameterOp struct {
	ParameterName string // without $ prefix
}

func (s *DropParameterOp) isAlterPageOperation() {}

// SetLayoutOp represents: SET Layout = Module.LayoutName [MAP (Old -> New, ...)]
type SetLayoutOp struct {
	NewLayout QualifiedName     // New layout qualified name
	Mappings  map[string]string // Old placeholder -> New placeholder (nil = auto-map)
}

func (s *SetLayoutOp) isAlterPageOperation() {}

// LayoutMapping represents a single placeholder mapping: Old -> New
type LayoutMapping struct {
	From string
	To   string
}

// AlterPagesLayoutStmt represents the bulk repoint:
//
//	ALTER PAGES [IN <module>] SET LAYOUT = Module.Layout
//	  [MAP (Old AS New, …)] [WHERE LAYOUT = Module.Old]
//
// It is a statement of its own rather than an ALTER PAGE operation because it
// names no page: the set is computed from Module and WhereLayout.
type AlterPagesLayoutStmt struct {
	Module      string            // "" = every module the project owns
	NewLayout   QualifiedName     // the layout to point pages at
	Mappings    map[string]string // Old placeholder -> New placeholder
	WhereLayout *QualifiedName    // nil = every page in scope, whatever its layout
}

func (s *AlterPagesLayoutStmt) isStatement() {}

// AlterPagesStylingStmt is the bulk form of ALTER PAGE's design-property SET:
// set a design property on every widget of one TYPE, across a module or the
// whole project.
//
// The predicate is a widget type and never a name, because a widget name is
// unique only within its page — measured across a blank 11.12.2 project,
// `actionButton1` appears in 30 units, so a name predicate would sweep
// unrelated widgets together (ako/mxcli#515).
type AlterPagesStylingStmt struct {
	Module      string              // "" = every module the project owns
	Assignments []StylingAssignment // reuses ALTER STYLING's assignment shape
	WidgetType  string              // MDL keyword (`datagrid`) or a full widget id
	DryRun      bool                // report the matches and write nothing
}

func (s *AlterPagesStylingStmt) isStatement() {}
