// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// Attributes
// ============================================================================

// Attribute represents an entity attribute definition.
type Attribute struct {
	Name                string
	Type                DataType
	NotNull             bool
	NotNullError        string // Custom error message for NOT NULL
	Unique              bool
	UniqueError         string // Custom error message for UNIQUE
	HasDefault          bool
	DefaultValue        any            // string, int64, float64, bool, or QualifiedName for enums
	Calculated          bool           // attribute is calculated (not stored)
	CalculatedMicroflow *QualifiedName // microflow that computes the calculated value
	Comment             string
	Documentation       string
	RenamedFrom         string // @RenamedFrom annotation value
}

// ============================================================================
// Index Definitions
// ============================================================================

// IndexColumn represents a column in an index definition.
type IndexColumn struct {
	Name       string
	Descending bool // true for DESC, false for ASC (default)
}

// Index represents an index definition on an entity.
type Index struct {
	Columns []IndexColumn
}
