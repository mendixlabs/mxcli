// SPDX-License-Identifier: Apache-2.0

// Package microflows - Data types for microflows
package microflows

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Data Types

// DataType represents a data type in Mendix.
type DataType interface {
	isDataType()
	GetTypeName() string
}

// BooleanType represents a boolean type.
type BooleanType struct {
	model.BaseElement
}

func (BooleanType) isDataType()         {}
func (BooleanType) GetTypeName() string { return "Boolean" }

// IntegerType represents an integer type.
type IntegerType struct {
	model.BaseElement
}

func (IntegerType) isDataType()         {}
func (IntegerType) GetTypeName() string { return "Integer" }

// LongType represents a long type.
type LongType struct {
	model.BaseElement
}

func (LongType) isDataType()         {}
func (LongType) GetTypeName() string { return "Long" }

// DecimalType represents a decimal type.
type DecimalType struct {
	model.BaseElement
}

func (DecimalType) isDataType()         {}
func (DecimalType) GetTypeName() string { return "Decimal" }

// StringType represents a string type.
type StringType struct {
	model.BaseElement
}

func (StringType) isDataType()         {}
func (StringType) GetTypeName() string { return "String" }

// DateTimeType represents a date/time type.
type DateTimeType struct {
	model.BaseElement
}

func (DateTimeType) isDataType()         {}
func (DateTimeType) GetTypeName() string { return "DateTime" }

// DateType represents a date-only type (no time component).
// Stored as DataTypes$DateTimeType in BSON.
type DateType struct {
	model.BaseElement
}

func (DateType) isDataType()         {}
func (DateType) GetTypeName() string { return "Date" }

// ObjectType represents an object type.
type ObjectType struct {
	model.BaseElement
	EntityID            model.ID `json:"entityId"`
	EntityQualifiedName string   `json:"entityQualifiedName"` // Used for BY_NAME_REFERENCE serialization
}

func (ObjectType) isDataType()         {}
func (ObjectType) GetTypeName() string { return "Object" }

// ListType represents a list type.
type ListType struct {
	model.BaseElement
	EntityID            model.ID `json:"entityId"`
	EntityQualifiedName string   `json:"entityQualifiedName"` // Used for BY_NAME_REFERENCE serialization
}

func (ListType) isDataType()         {}
func (ListType) GetTypeName() string { return "List" }

// EnumerationType represents an enumeration type.
type EnumerationType struct {
	model.BaseElement
	EnumerationID            model.ID `json:"enumerationId"`
	EnumerationQualifiedName string   `json:"enumerationQualifiedName"` // Used for BY_NAME_REFERENCE serialization
}

func (EnumerationType) isDataType()         {}
func (EnumerationType) GetTypeName() string { return "Enumeration" }

// VoidType represents no return type.
type VoidType struct {
	model.BaseElement
}

func (VoidType) isDataType()         {}
func (VoidType) GetTypeName() string { return "Void" }

// BinaryType represents a binary type.
type BinaryType struct {
	model.BaseElement
}

func (BinaryType) isDataType()         {}
func (BinaryType) GetTypeName() string { return "Binary" }
