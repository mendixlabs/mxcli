// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Button Widgets

// ActionButton represents a button that triggers an action.
type ActionButton struct {
	BaseWidget
	Caption         *model.Text      `json:"caption,omitempty"`         // Simple caption (backward compat)
	CaptionTemplate *ClientTemplate  `json:"captionTemplate,omitempty"` // Caption with parameters
	Tooltip         *model.Text      `json:"tooltip,omitempty"`
	Icon            *Icon            `json:"icon,omitempty"`
	ButtonStyle     ButtonStyle      `json:"buttonStyle,omitempty"`
	RenderMode      ButtonRenderMode `json:"renderMode,omitempty"`
	Action          ClientAction     `json:"action,omitempty"`
}

// ButtonStyle represents the style of a button.
type ButtonStyle string

const (
	ButtonStyleDefault   ButtonStyle = "Default"
	ButtonStylePrimary   ButtonStyle = "Primary"
	ButtonStyleSecondary ButtonStyle = "Secondary"
	ButtonStyleSuccess   ButtonStyle = "Success"
	ButtonStyleWarning   ButtonStyle = "Warning"
	ButtonStyleDanger    ButtonStyle = "Danger"
	ButtonStyleInfo      ButtonStyle = "Info"
	ButtonStyleInverse   ButtonStyle = "Inverse"
	ButtonStyleLink      ButtonStyle = "Link"
)

// canonicalButtonStyles maps a lowercased button-style token to its canonical
// Mendix enum value. The set mirrors the generated metamodel's PagesButtonStyle
// — note that Mendix recognizes neither "Secondary" nor "Link" as a button
// style, so they are intentionally absent.
var canonicalButtonStyles = map[string]ButtonStyle{
	"default": ButtonStyleDefault,
	"primary": ButtonStylePrimary,
	"success": ButtonStyleSuccess,
	"warning": ButtonStyleWarning,
	"danger":  ButtonStyleDanger,
	"info":    ButtonStyleInfo,
	"inverse": ButtonStyleInverse,
}

// CanonicalButtonStyle normalizes a button-style token (in any case) to its
// canonical Mendix enum value. ok is false when s does not name a recognized
// style — callers should surface an error rather than silently writing an
// unknown value, which Mendix degrades to btn-default at build time.
func CanonicalButtonStyle(s string) (ButtonStyle, bool) {
	bs, ok := canonicalButtonStyles[strings.ToLower(strings.TrimSpace(s))]
	return bs, ok
}

// ValidButtonStyleList returns the recognized button styles in canonical case,
// for use in error messages and suggestions.
func ValidButtonStyleList() []string {
	return []string{"Default", "Primary", "Success", "Warning", "Danger", "Info", "Inverse"}
}

// ButtonRenderMode represents how a button is rendered.
type ButtonRenderMode string

const (
	ButtonRenderModeButton ButtonRenderMode = "Button"
	ButtonRenderModeLink   ButtonRenderMode = "Link"
)

// Icon represents an icon.
type Icon struct {
	model.BaseElement
	Type    IconType `json:"type"`
	Name    string   `json:"name,omitempty"`
	ImageID model.ID `json:"imageId,omitempty"`
}

// IconType represents the type of icon.
type IconType string

const (
	IconTypeGlyph IconType = "Glyph"
	IconTypeImage IconType = "Image"
)

// DropDownButton represents a dropdown button.
type DropDownButton struct {
	BaseWidget
	Caption     *model.Text           `json:"caption,omitempty"`
	Tooltip     *model.Text           `json:"tooltip,omitempty"`
	Icon        *Icon                 `json:"icon,omitempty"`
	ButtonStyle ButtonStyle           `json:"buttonStyle,omitempty"`
	Items       []*DropDownButtonItem `json:"items,omitempty"`
}

// DropDownButtonItem represents an item in a dropdown button.
type DropDownButtonItem struct {
	model.BaseElement
	Caption *model.Text  `json:"caption,omitempty"`
	Action  ClientAction `json:"action,omitempty"`
}

// NavigationList represents a navigation list widget.
type NavigationList struct {
	BaseWidget
	Items []*NavigationListItem `json:"items,omitempty"`
}

// NavigationListItem represents an item in a navigation list.
type NavigationListItem struct {
	model.BaseElement
	Name    string       `json:"name,omitempty"`
	Caption *model.Text  `json:"caption,omitempty"`
	Action  ClientAction `json:"action,omitempty"`
	Widgets []Widget     `json:"widgets,omitempty"` // Nested widgets beyond the caption
}

// LinkButton represents a link-style button.
type LinkButton struct {
	BaseWidget
	Caption *model.Text  `json:"caption,omitempty"`
	Tooltip *model.Text  `json:"tooltip,omitempty"`
	Action  ClientAction `json:"action,omitempty"`
}

// ClientActions

// ClientAction represents an action triggered by client interaction.
type ClientAction interface {
	isClientAction()
}

// NoClientAction represents no action.
type NoClientAction struct {
	model.BaseElement
}

func (NoClientAction) isClientAction() {}

// PageClientAction opens a page.
type PageClientAction struct {
	model.BaseElement
	PageID            model.ID                      `json:"pageId"`
	PageName          string                        `json:"pageName,omitempty"` // Qualified name for by-name reference
	PageSettings      *PageSettings                 `json:"pageSettings,omitempty"`
	ParameterMappings []*PageClientParameterMapping `json:"parameterMappings,omitempty"`
}

func (PageClientAction) isClientAction() {}

// PageClientParameterMapping maps a page parameter to a value in a PageClientAction.
// BSON storage type: Forms$PageParameterMapping. Uses "Argument" field (not "Expression").
type PageClientParameterMapping struct {
	model.BaseElement
	ParameterName string `json:"parameterName"`        // Page parameter name (without $)
	Variable      string `json:"variable,omitempty"`   // Variable reference (e.g., "$Customer")
	Expression    string `json:"expression,omitempty"` // Expression value (maps to BSON "Argument")
}

// PageSettings represents page display settings.
type PageSettings struct {
	model.BaseElement
	FormLocation FormLocation `json:"formLocation"`
}

// FormLocation represents where a form is displayed.
type FormLocation string

const (
	FormLocationContent FormLocation = "Content"
	FormLocationPopup   FormLocation = "Popup"
	FormLocationModal   FormLocation = "Modal"
)

// MicroflowParameterMapping maps a microflow parameter to a value in a MicroflowClientAction.
// BSON storage type: Forms$MicroflowParameterMapping (not Pages$ or Microflows$).
type MicroflowParameterMapping struct {
	model.BaseElement
	ParameterName string `json:"parameterName"`        // Parameter name (without $)
	Variable      string `json:"variable,omitempty"`   // Variable reference (e.g., "$Customer")
	Expression    string `json:"expression,omitempty"` // Expression value
}

// MicroflowClientAction calls a microflow.
type MicroflowClientAction struct {
	model.BaseElement
	MicroflowID       model.ID                     `json:"microflowId"`
	MicroflowName     string                       `json:"microflowName,omitempty"` // Qualified name for BSON serialization
	ParameterMappings []*MicroflowParameterMapping `json:"parameterMappings,omitempty"`
}

func (MicroflowClientAction) isClientAction() {}

// NanoflowParameterMapping maps a nanoflow parameter to a value in a NanoflowClientAction.
// BSON storage type: Forms$NanoflowParameterMapping (not Pages$).
type NanoflowParameterMapping struct {
	model.BaseElement
	ParameterName string `json:"parameterName"`        // Parameter name (without $)
	Variable      string `json:"variable,omitempty"`   // Variable reference (e.g., "$Customer")
	Expression    string `json:"expression,omitempty"` // Expression value
}

// NanoflowClientAction calls a nanoflow.
type NanoflowClientAction struct {
	model.BaseElement
	NanoflowID        model.ID                    `json:"nanoflowId"`
	NanoflowName      string                      `json:"nanoflowName,omitempty"` // Qualified name for BSON serialization
	ParameterMappings []*NanoflowParameterMapping `json:"parameterMappings,omitempty"`
}

func (NanoflowClientAction) isClientAction() {}

// ClosePageClientAction closes the current page.
type ClosePageClientAction struct {
	model.BaseElement
}

func (ClosePageClientAction) isClientAction() {}

// SaveChangesClientAction saves changes.
type SaveChangesClientAction struct {
	model.BaseElement
	ClosePage bool `json:"closePage"`
}

func (SaveChangesClientAction) isClientAction() {}

// CancelChangesClientAction cancels changes.
type CancelChangesClientAction struct {
	model.BaseElement
	ClosePage bool `json:"closePage"`
}

func (CancelChangesClientAction) isClientAction() {}

// CreateObjectClientAction creates an object.
type CreateObjectClientAction struct {
	model.BaseElement
	EntityID   model.ID `json:"entityId"`
	EntityName string   `json:"entityName,omitempty"` // Qualified name e.g. "Module.Entity"
	PageID     model.ID `json:"pageId,omitempty"`
	PageName   string   `json:"pageName,omitempty"` // Qualified name e.g. "Module.Page"
}

func (CreateObjectClientAction) isClientAction() {}

// DeleteClientAction deletes an object.
type DeleteClientAction struct {
	model.BaseElement
	ClosePage bool `json:"closePage"`
}

func (DeleteClientAction) isClientAction() {}

// SignOutClientAction signs out the user.
type SignOutClientAction struct {
	model.BaseElement
}

func (SignOutClientAction) isClientAction() {}

// ShowHomePageClientAction shows the home page.
type ShowHomePageClientAction struct {
	model.BaseElement
}

func (ShowHomePageClientAction) isClientAction() {}

// LinkClientAction opens a link.
type LinkClientAction struct {
	model.BaseElement
	LinkType LinkType `json:"linkType"`
	Address  string   `json:"address,omitempty"`
}

func (LinkClientAction) isClientAction() {}

// SetTaskOutcomeClientAction completes a workflow user task with a named outcome.
type SetTaskOutcomeClientAction struct {
	model.BaseElement
	ClosePage    bool   `json:"closePage,omitempty"`
	Commit       bool   `json:"commit,omitempty"`
	OutcomeValue string `json:"outcomeValue,omitempty"`
}

func (SetTaskOutcomeClientAction) isClientAction() {}

// LinkType represents the type of link.
type LinkType string

const (
	LinkTypeWeb   LinkType = "Web"
	LinkTypeEmail LinkType = "Email"
	LinkTypePhone LinkType = "Phone"
)
