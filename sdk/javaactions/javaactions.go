// SPDX-License-Identifier: Apache-2.0

// Package javaactions provides types for Mendix Java actions.
//
// The CodeAction* type family (return/parameter types, parameters, type
// parameters) has its canonical home in mdl/types; this package re-exports
// those as type aliases so callers that mix the two packages see one identity.
// JavaAction itself stays here: it is the rich, fully-parsed action structure,
// distinct from the minimal mdl/types.JavaAction document reference.
package javaactions

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// JavaAction represents a Mendix Java action.
type JavaAction struct {
	model.BaseElement
	ContainerID             model.ID               `json:"containerId"`
	Name                    string                 `json:"name"`
	Documentation           string                 `json:"documentation,omitempty"`
	Excluded                bool                   `json:"excluded"`
	ExportLevel             string                 `json:"exportLevel,omitempty"`
	ActionDefaultReturnName string                 `json:"actionDefaultReturnName,omitempty"`
	ReturnType              CodeActionReturnType   `json:"returnType,omitempty"`
	Parameters              []*JavaActionParameter `json:"parameters,omitempty"`
	TypeParameters          []*TypeParameterDef    `json:"typeParameters,omitempty"`
	MicroflowActionInfo     *MicroflowActionInfo   `json:"microflowActionInfo,omitempty"`
}

// TypeParameterNames returns the type parameter names as a string slice (convenience).
func (ja *JavaAction) TypeParameterNames() []string {
	names := make([]string, len(ja.TypeParameters))
	for i, tp := range ja.TypeParameters {
		names[i] = tp.Name
	}
	return names
}

// FindTypeParameterName looks up a type parameter name by its ID.
func (ja *JavaAction) FindTypeParameterName(id model.ID) string {
	for _, tp := range ja.TypeParameters {
		if tp.ID == id {
			return tp.Name
		}
	}
	return ""
}

// GetName returns the Java action's name.
func (ja *JavaAction) GetName() string {
	return ja.Name
}

// GetContainerID returns the container ID.
func (ja *JavaAction) GetContainerID() model.ID {
	return ja.ContainerID
}

// CodeAction* type family — canonical definitions live in mdl/types.
// Re-exported here as aliases so javaactions.X and types.X are one identity.
type (
	CodeActionReturnType        = types.CodeActionReturnType
	CodeActionParameterType     = types.CodeActionParameterType
	JavaActionParameter         = types.JavaActionParameter
	TypeParameterDef            = types.TypeParameterDef
	MicroflowActionInfo         = types.MicroflowActionInfo
	TypeParameter               = types.TypeParameter
	EntityTypeParameterType     = types.EntityTypeParameterType
	VoidType                    = types.VoidType
	BooleanType                 = types.BooleanType
	IntegerType                 = types.IntegerType
	LongType                    = types.LongType
	DecimalType                 = types.DecimalType
	StringType                  = types.StringType
	DateTimeType                = types.DateTimeType
	EntityType                  = types.EntityType
	ListType                    = types.ListType
	StringTemplateParameterType = types.StringTemplateParameterType
	FileDocumentType            = types.FileDocumentType
	EnumerationType             = types.EnumerationType
	MicroflowType               = types.MicroflowType
	NanoflowType                = types.NanoflowType
)
