// SPDX-License-Identifier: Apache-2.0

// Package types — Java/JavaScript action type definitions.
// Canonical home for all CodeAction* types; sdk/javaactions re-exports these as type aliases.
package types

import "github.com/JordtenBulte-OLC/mxcli/model"

// CodeActionReturnType is the interface for Java/JavaScript action return types.
type CodeActionReturnType interface {
	isCodeActionReturnType()
	TypeString() string
}

// CodeActionParameterType is the interface for Java/JavaScript action parameter types.
type CodeActionParameterType interface {
	isCodeActionParameterType()
	TypeString() string
}

// JavaActionParameter represents a parameter of a Java/JavaScript action.
type JavaActionParameter struct {
	model.BaseElement
	Name          string                  `json:"name"`
	Description   string                  `json:"description,omitempty"`
	Category      string                  `json:"category,omitempty"`
	IsRequired    bool                    `json:"isRequired"`
	ParameterType CodeActionParameterType `json:"parameterType,omitempty"`
}

// TypeParameterDef represents a type parameter definition on a Java action (e.g., <pEntity>).
type TypeParameterDef struct {
	model.BaseElement
	Name string `json:"name"` // Type parameter name (e.g., "pEntity")
}

// MicroflowActionInfo exposes a Java action as a toolbox item in Studio Pro.
//
// The four *Data fields are the inline toolbox icon/image bitmaps. In Mendix
// BSON they are binary (byte[]) primitives that must ALWAYS be present and
// non-null — emitting BSON null (or omitting them) crashes Studio Pro's
// UnitWriter on re-serialize ("Null value found in primitive property
// 'ImageDataStorage'"); see issue #656. An empty bitmap is an empty binary, not
// null. The legacy `Icon` string field (a key removed from the current
// metamodel) was dropped.
type MicroflowActionInfo struct {
	model.BaseElement
	Caption       string `json:"caption,omitempty"`
	Category      string `json:"category,omitempty"`
	IconData      []byte `json:"iconData,omitempty"`
	IconDataDark  []byte `json:"iconDataDark,omitempty"`
	ImageData     []byte `json:"imageData,omitempty"`
	ImageDataDark []byte `json:"imageDataDark,omitempty"`
}

// TypeParameter represents a generic type parameter reference in a return type or parameter type.
// For ParameterizedEntityType parameters, TypeParameterID holds the BY_ID reference to a TypeParameterDef,
// and TypeParameter holds the resolved name (e.g., "pEntity").
type TypeParameter struct {
	model.BaseElement
	TypeParameter   string   `json:"typeParameter,omitempty"`   // e.g., "pEntity" (resolved name)
	TypeParameterID model.ID `json:"typeParameterId,omitempty"` // BY_ID reference to TypeParameterDef
}

func (TypeParameter) isCodeActionReturnType()    {}
func (TypeParameter) isCodeActionParameterType() {}
func (t TypeParameter) TypeString() string {
	if t.TypeParameter != "" {
		return t.TypeParameter
	}
	return "T"
}

// EntityTypeParameterType represents a parameter typed to a type parameter (generics).
// The TypeParameterID is a BY_ID reference to a TypeParameterDef.
type EntityTypeParameterType struct {
	model.BaseElement
	TypeParameterID   model.ID `json:"typeParameterId"`             // BY_ID reference to TypeParameterDef
	TypeParameterName string   `json:"typeParameterName,omitempty"` // Resolved name for display
}

func (EntityTypeParameterType) isCodeActionParameterType() {}
func (e EntityTypeParameterType) TypeString() string {
	if e.TypeParameterName != "" {
		return e.TypeParameterName
	}
	return "Object"
}

// VoidType represents no return value.
type VoidType struct {
	model.BaseElement
}

func (VoidType) isCodeActionReturnType() {}
func (VoidType) TypeString() string      { return "Void" }

// BooleanType represents a Boolean type.
type BooleanType struct {
	model.BaseElement
}

func (BooleanType) isCodeActionReturnType()    {}
func (BooleanType) isCodeActionParameterType() {}
func (BooleanType) TypeString() string         { return "Boolean" }

// IntegerType represents an Integer type.
type IntegerType struct {
	model.BaseElement
}

func (IntegerType) isCodeActionReturnType()    {}
func (IntegerType) isCodeActionParameterType() {}
func (IntegerType) TypeString() string         { return "Integer" }

// LongType represents a Long type.
type LongType struct {
	model.BaseElement
}

func (LongType) isCodeActionReturnType()    {}
func (LongType) isCodeActionParameterType() {}
func (LongType) TypeString() string         { return "Long" }

// DecimalType represents a Decimal type.
type DecimalType struct {
	model.BaseElement
}

func (DecimalType) isCodeActionReturnType()    {}
func (DecimalType) isCodeActionParameterType() {}
func (DecimalType) TypeString() string         { return "Decimal" }

// StringType represents a String type.
type StringType struct {
	model.BaseElement
}

func (StringType) isCodeActionReturnType()    {}
func (StringType) isCodeActionParameterType() {}
func (StringType) TypeString() string         { return "String" }

// DateTimeType represents a DateTime type.
type DateTimeType struct {
	model.BaseElement
}

func (DateTimeType) isCodeActionReturnType()    {}
func (DateTimeType) isCodeActionParameterType() {}
func (DateTimeType) TypeString() string         { return "DateTime" }

// EntityType represents an entity type (object parameter/return).
type EntityType struct {
	model.BaseElement
	Entity string `json:"entity,omitempty"` // Qualified entity name
}

func (EntityType) isCodeActionReturnType()    {}
func (EntityType) isCodeActionParameterType() {}
func (e EntityType) TypeString() string {
	if e.Entity != "" {
		return e.Entity
	}
	return "Object"
}

// ListType represents a list type.
type ListType struct {
	model.BaseElement
	Entity string `json:"entity,omitempty"` // Qualified entity name for list items
}

func (ListType) isCodeActionReturnType()    {}
func (ListType) isCodeActionParameterType() {}
func (l ListType) TypeString() string {
	if l.Entity != "" {
		return "List of " + l.Entity
	}
	return "List"
}

// StringTemplateParameterType represents a string template parameter type (for OQL, SQL, etc.).
type StringTemplateParameterType struct {
	model.BaseElement
	Grammar string `json:"grammar,omitempty"` // "Sql", "Oql", etc.
}

func (StringTemplateParameterType) isCodeActionParameterType() {}
func (s StringTemplateParameterType) TypeString() string {
	if s.Grammar != "" {
		return "StringTemplate(" + s.Grammar + ")"
	}
	return "StringTemplate"
}

// FileDocumentType represents a file document type.
type FileDocumentType struct {
	model.BaseElement
}

func (FileDocumentType) isCodeActionReturnType()    {}
func (FileDocumentType) isCodeActionParameterType() {}
func (FileDocumentType) TypeString() string         { return "FileDocument" }

// EnumerationType represents an enumeration type.
type EnumerationType struct {
	model.BaseElement
	Enumeration string `json:"enumeration,omitempty"` // Qualified enumeration name
}

func (EnumerationType) isCodeActionReturnType()    {}
func (EnumerationType) isCodeActionParameterType() {}
func (e EnumerationType) TypeString() string {
	if e.Enumeration != "" {
		return "Enum " + e.Enumeration
	}
	return "Enumeration"
}

// MicroflowType represents a microflow parameter type.
type MicroflowType struct {
	model.BaseElement
}

func (MicroflowType) isCodeActionParameterType() {}
func (MicroflowType) TypeString() string         { return "Microflow" }

// NanoflowType represents a nanoflow parameter type (JavaScript actions only).
type NanoflowType struct {
	model.BaseElement
}

func (NanoflowType) isCodeActionParameterType() {}
func (NanoflowType) TypeString() string         { return "Nanoflow" }
