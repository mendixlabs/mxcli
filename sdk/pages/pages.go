// SPDX-License-Identifier: Apache-2.0

// Package pages provides types for Mendix pages, layouts, and widgets.
package pages

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Page represents a page in the Mendix model.
type Page struct {
	model.BaseElement
	ContainerID    model.ID         `json:"containerId"`
	Name           string           `json:"name"`
	Documentation  string           `json:"documentation,omitempty"`
	Title          *model.Text      `json:"title,omitempty"`
	URL            string           `json:"url,omitempty"`
	LayoutID       model.ID         `json:"layoutId,omitempty"`
	LayoutCall     *LayoutCall      `json:"layoutCall,omitempty"`
	AllowedRoles   []model.ID       `json:"allowedRoles,omitempty"`
	Parameters     []*PageParameter `json:"parameters,omitempty"`
	Variables      []*LocalVariable `json:"variables,omitempty"`
	PopupWidth     int              `json:"popupWidth,omitempty"`
	PopupHeight    int              `json:"popupHeight,omitempty"`
	PopupResizable bool             `json:"popupResizable,omitempty"`
	// Class / Style are the page's Forms$Appearance CSS class and inline style
	// (issue #714).
	Class      string `json:"class,omitempty"`
	Style      string `json:"style,omitempty"`
	MarkAsUsed bool   `json:"markAsUsed"`
	Excluded   bool   `json:"excluded"`
}

// GetName returns the page's name.
func (p *Page) GetName() string {
	return p.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (p *Page) GetContainerID() model.ID {
	return p.ContainerID
}

// Layout represents a layout in the Mendix model.
type Layout struct {
	model.BaseElement
	ContainerID       model.ID   `json:"containerId"`
	Name              string     `json:"name"`
	Documentation     string     `json:"documentation,omitempty"`
	LayoutType        LayoutType `json:"layoutType"`
	MainPlaceholderID model.ID   `json:"mainPlaceholderId,omitempty"`
	Widget            Widget     `json:"widget,omitempty"`
}

// GetName returns the layout's name.
func (l *Layout) GetName() string {
	return l.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (l *Layout) GetContainerID() model.ID {
	return l.ContainerID
}

// LayoutType represents the type of layout.
type LayoutType string

const (
	LayoutTypeResponsive LayoutType = "Responsive"
	LayoutTypeTablet     LayoutType = "Tablet"
	LayoutTypePhone      LayoutType = "Phone"
	LayoutTypeModalPopup LayoutType = "ModalPopup"
	LayoutTypePopup      LayoutType = "Popup"
	LayoutTypeLegacy     LayoutType = "Legacy"
)

// Snippet represents a reusable page snippet.
type Snippet struct {
	model.BaseElement
	ContainerID   model.ID            `json:"containerId"`
	Name          string              `json:"name"`
	Documentation string              `json:"documentation,omitempty"`
	EntityID      model.ID            `json:"entityId,omitempty"`
	Parameters    []*SnippetParameter `json:"parameters,omitempty"`
	Variables     []*LocalVariable    `json:"variables,omitempty"`
	Widgets       []Widget            `json:"widgets,omitempty"`
}

// GetName returns the snippet's name.
func (s *Snippet) GetName() string {
	return s.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (s *Snippet) GetContainerID() model.ID {
	return s.ContainerID
}

// BuildingBlock represents a building block.
type BuildingBlock struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
	Widget        Widget   `json:"widget,omitempty"`
	TemplateID    string   `json:"templateId,omitempty"`
}

// GetName returns the building block's name.
func (bb *BuildingBlock) GetName() string {
	return bb.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (bb *BuildingBlock) GetContainerID() model.ID {
	return bb.ContainerID
}

// PageTemplate represents a page template.
type PageTemplate struct {
	model.BaseElement
	ContainerID      model.ID         `json:"containerId"`
	Name             string           `json:"name"`
	Documentation    string           `json:"documentation,omitempty"`
	DisplayName      *model.Text      `json:"displayName,omitempty"`
	LayoutID         model.ID         `json:"layoutId,omitempty"`
	PageTemplateType PageTemplateType `json:"pageTemplateType"`
	Widget           Widget           `json:"widget,omitempty"`
}

// GetName returns the page template's name.
func (pt *PageTemplate) GetName() string {
	return pt.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (pt *PageTemplate) GetContainerID() model.ID {
	return pt.ContainerID
}

// PageTemplateType represents the type of page template.
type PageTemplateType string

const (
	PageTemplateTypeStandard PageTemplateType = "Standard"
	PageTemplateTypeEdit     PageTemplateType = "Edit"
	PageTemplateTypeSelect   PageTemplateType = "Select"
)
