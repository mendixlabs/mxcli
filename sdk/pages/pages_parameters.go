// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// LayoutCall represents a call to a layout with argument bindings.
type LayoutCall struct {
	model.BaseElement
	LayoutID   model.ID              `json:"layoutId"`
	LayoutName string                `json:"layoutName"` // Qualified name like "Atlas_Core.Atlas_Default"
	Arguments  []*LayoutCallArgument `json:"arguments,omitempty"`
}

// LayoutCallArgument represents an argument binding in a layout call.
type LayoutCallArgument struct {
	model.BaseElement
	ParameterID model.ID `json:"parameterId"`
	Widget      Widget   `json:"widget,omitempty"`
}

// PageParameter represents a parameter of a page.
type PageParameter struct {
	model.BaseElement
	ContainerID  model.ID `json:"containerId"`
	Name         string   `json:"name"`
	EntityID     model.ID `json:"entityId,omitempty"`
	EntityName   string   `json:"entityName,omitempty"`   // Qualified entity name (e.g., "Module.Entity")
	TypeName     string   `json:"typeName,omitempty"`     // BSON $Type for ParameterType (e.g., "DataTypes$StringType")
	DefaultValue string   `json:"defaultValue,omitempty"` // Default value expression
	IsRequired   bool     `json:"isRequired"`             // Whether the parameter is required
}

// GetName returns the parameter's name.
func (p *PageParameter) GetName() string {
	return p.Name
}

// GetContainerID returns the ID of the containing page.
func (p *PageParameter) GetContainerID() model.ID {
	return p.ContainerID
}

// SnippetParameter represents a parameter of a snippet.
type SnippetParameter struct {
	model.BaseElement
	ContainerID model.ID `json:"containerId"`
	Name        string   `json:"name"`
	EntityID    model.ID `json:"entityId,omitempty"`
	EntityName  string   `json:"entityName,omitempty"` // Qualified entity name (e.g., "Module.Entity")
	Type        string   `json:"type,omitempty"`
}

// GetName returns the parameter's name.
func (p *SnippetParameter) GetName() string {
	return p.Name
}

// GetContainerID returns the ID of the containing snippet.
func (p *SnippetParameter) GetContainerID() model.ID {
	return p.ContainerID
}

// LocalVariable represents a page-level variable (Forms$LocalVariable).
// Page variables are declared at the page/snippet level with a default value expression
// and can be referenced in expressions throughout the page (e.g., column visibility).
type LocalVariable struct {
	model.BaseElement
	ContainerID  model.ID `json:"containerId"`
	Name         string   `json:"name"`
	DefaultValue string   `json:"defaultValue"`           // Mendix expression string
	VariableType string   `json:"variableType,omitempty"` // BSON $Type, e.g., "DataTypes$BooleanType"
}

// GetName returns the variable's name.
func (v *LocalVariable) GetName() string {
	return v.Name
}

// GetContainerID returns the ID of the containing page/snippet.
func (v *LocalVariable) GetContainerID() model.ID {
	return v.ContainerID
}
