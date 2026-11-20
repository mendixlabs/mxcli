// SPDX-License-Identifier: Apache-2.0

// Package pages provides types for Mendix pages, layouts, and widgets.
package pages

import (
	"github.com/mendixlabs/mxcli/model"
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
	// PopupCloseAction names the widget whose action closes this page when shown
	// as a pop-up (Forms$Page.PopupCloseAction).
	PopupCloseAction string `json:"popupCloseAction,omitempty"`
	// Class / Style are the page's Forms$Appearance CSS class and inline style
	// (issue #714).
	Class string `json:"class,omitempty"`
	Style string `json:"style,omitempty"`
	// DesignProperties are the page's own Appearance.DesignProperties — Atlas
	// declares them under "Page" (ako/mxcli#1678).
	DesignProperties []DesignPropertyValue `json:"designProperties,omitempty"`
	MarkAsUsed       bool                  `json:"markAsUsed"`
	Excluded         bool                  `json:"excluded"`
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
//
// LayoutType is stored on the content wrapper, not on the layout element —
// Forms$Layout has no such key, and gen's Layout.LayoutType() binds one it does
// not have. It is modelled here because it is a property of the layout as a
// user thinks of it; the codec puts it where Mendix keeps it.
//
// Platform decides the wrapper and the vocabulary: web layouts use
// Forms$WebLayoutContent and one of Responsive/Phone/Tablet/ModalPopup, native
// ones use Forms$NativeLayoutContent and Default/Popup.
type Layout struct {
	model.BaseElement
	ContainerID   model.ID   `json:"containerId"`
	Name          string     `json:"name"`
	Documentation string     `json:"documentation,omitempty"`
	LayoutType    LayoutType `json:"layoutType"`
	// Excluded mirrors Studio Pro's "Exclude from project" (read only; the
	// layout writer still writes false).
	Excluded bool `json:"excluded,omitempty"`
	// Native selects Forms$NativeLayoutContent over Forms$WebLayoutContent.
	Native bool `json:"native,omitempty"`
	// Class and Style are the layout's own Forms$Appearance.
	//
	// The class is not decoration: Atlas scopes ~24 of its layout rules to
	// `.layout-atlas` and its variants, and every Atlas layout carries one
	// (measured: 'layout-atlas layout-atlas-responsive-topbar' and friends;
	// only PopupLayout is bare). A layout written without it renders with none
	// of Atlas's chrome styling — no topbar bar, no sidebar rail — which
	// mx check is entirely silent about.
	Class string `json:"class,omitempty"`
	Style string `json:"style,omitempty"`
	// Widgets is the content tree, normally a single ScrollContainer.
	Widgets []Widget `json:"widgets,omitempty"`
	// LayoutCall is set on a layout built on another layout (web only). The
	// outer layout's placeholders are filled by the call's arguments, the way a
	// page fills its layout's, and Widgets stays empty — measured on
	// ConversationalUI.Layout_MasterBase, which Studio Pro stores under
	// Content.LayoutCall with Content.Widgets [] (mendixlabs/mxcli#1411).
	LayoutCall *LayoutCall `json:"layoutCall,omitempty"`

	// The native-only parts of Forms$NativeLayoutContent, all stored on every
	// native Atlas layout (11.13.0): the placeholder on the header's right
	// (null on NativePhone_FullPage), whether the bottom bar shows, and the
	// sidebar with its widgets. A copy that left them out lost them silently.
	HeaderPlaceholder *LayoutPlaceholder `json:"headerPlaceholder,omitempty"`
	ShowBottomBar     bool               `json:"showBottomBar,omitempty"`
	Sidebar           bool               `json:"sidebar,omitempty"`
	SidebarWidgets    []Widget           `json:"sidebarWidgets,omitempty"`

	// MainPlaceholderID and Widget are the shallow forms older readers filled.
	MainPlaceholderID model.ID `json:"mainPlaceholderId,omitempty"`
	Widget            Widget   `json:"widget,omitempty"`
}

// WebLayoutTypes and NativeLayoutTypes are the values each platform actually
// uses, measured across all 22 layouts Atlas ships on 11.13.0. Neither
// modelsdk/gen (Default, Popup) nor generated/metamodel's enum lists more than
// two of the six, so this is the vocabulary real documents attest to.
//
// Legacy is accepted on read and deliberately absent here: it is declared in
// LayoutType below but appears in no Atlas layout, so mxcli does not offer to
// author a value it has never seen written.
var (
	WebLayoutTypes    = []LayoutType{LayoutTypeResponsive, LayoutTypePhone, LayoutTypeTablet, LayoutTypeModalPopup}
	NativeLayoutTypes = []LayoutType{LayoutTypeDefault, LayoutTypePopup}
)

// ValidLayoutType reports whether t is a value the given platform uses.
func ValidLayoutType(t LayoutType, native bool) bool {
	set := WebLayoutTypes
	if native {
		set = NativeLayoutTypes
	}
	for _, v := range set {
		if v == t {
			return true
		}
	}
	return false
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
	LayoutTypeDefault    LayoutType = "Default"
	LayoutTypeTablet     LayoutType = "Tablet"
	LayoutTypePhone      LayoutType = "Phone"
	LayoutTypeModalPopup LayoutType = "ModalPopup"
	LayoutTypePopup      LayoutType = "Popup"
	LayoutTypeLegacy     LayoutType = "Legacy"
)

// Snippet represents a reusable page snippet.
type Snippet struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
	// Excluded mirrors Studio Pro's "Exclude from project". Reads must supply
	// it so a rewrite can carry it forward instead of clearing it (#914).
	Excluded   bool                `json:"excluded,omitempty"`
	EntityID   model.ID            `json:"entityId,omitempty"`
	Parameters []*SnippetParameter `json:"parameters,omitempty"`
	Variables  []*LocalVariable    `json:"variables,omitempty"`
	Widgets    []Widget            `json:"widgets,omitempty"`
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
	ContainerID      model.ID `json:"containerId"`
	Name             string   `json:"name"`
	Documentation    string   `json:"documentation,omitempty"`
	DisplayName      string   `json:"displayName,omitempty"`
	Platform         string   `json:"platform,omitempty"`
	TemplateCategory string   `json:"templateCategory,omitempty"`
	Widget           Widget   `json:"widget,omitempty"`
	TemplateID       string   `json:"templateId,omitempty"`
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
