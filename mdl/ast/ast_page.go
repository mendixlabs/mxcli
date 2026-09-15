// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// Page Statements
// ============================================================================

// PageParameter represents a page parameter: $Name: Type
// Type can be an entity (Module.Entity) or a primitive (String, Integer, etc.).
type PageParameter struct {
	Name       string        // Parameter name (without $ prefix)
	EntityType QualifiedName // Entity type (for backward compatibility; empty for primitives)
	Type       DataType      // Full data type (primitives + entities)
}

// PageVariable represents a page variable: $Name: DataType = 'defaultExpression'
type PageVariable struct {
	Name         string // Variable name (without $ prefix)
	DataType     string // MDL type name: Boolean, String, Integer, Decimal, DateTime, or entity type
	DefaultValue string // Mendix expression string
}

// SortColumnDef represents a sort column: attribute ASC/DESC
type SortColumnDef struct {
	Attribute string // Qualified name or simple identifier
	Order     string // "ASC" or "DESC"
}

// DataGridColumnDef represents a DataGrid2 column definition.
type DataGridColumnDef struct {
	Attribute  string         // Attribute name (empty for action columns)
	Caption    string         // Column header caption
	ChildrenV3 []*WidgetV3    // Child widgets (V3 syntax)
	Properties map[string]any // Column properties (Alignment, WrapText, etc.)
}

// DropPageStmt represents: DROP PAGE Module.Name
type DropPageStmt struct {
	Name QualifiedName
}

func (s *DropPageStmt) isStatement() {}

// DropLayoutStmt represents: DROP LAYOUT Module.Name
//
// Layouts were the only document mxcli could create and alter but not delete, so
// a layout written by mistake — the CE0848 shape in mendixlabs/mxcli#1063 was
// exactly that — had no headless remedy at all.
type DropLayoutStmt struct {
	Name QualifiedName
}

func (s *DropLayoutStmt) isStatement() {}

// DropSnippetStmt represents: DROP SNIPPET Module.Name
type DropSnippetStmt struct {
	Name QualifiedName
}

func (s *DropSnippetStmt) isStatement() {}
