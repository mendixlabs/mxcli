// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
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

// Icon represents a widget icon (e.g. on an action/link button).
//
// Mendix has three icon ELEMENTS and they are not variants of one value: an
// icon-collection icon and an image icon each hold a qualified name — into an
// icon collection and an image collection, which are different documents —
// while a glyph icon holds a numeric character code and no name at all. Kind
// says which, and the writers dispatch on it rather than inferring from the
// payload: the two named kinds are indistinguishable by their payload, so a
// writer that guesses produces a document mxbuild accepts and Studio Pro
// cannot open (mendixlabs/mxcli#1059).
//
// The vocabulary is types.MenuIconKind — Mendix's kinds, not navigation's —
// shared rather than restated. This type carried its own three-value enum until
// the page and navigation paths were found to disagree about the same three
// things, which is exactly the drift a second copy invites.
type Icon struct {
	model.BaseElement
	Kind types.MenuIconKind `json:"kind"`
	// Image is the qualified name of the referenced icon/image for the
	// collection and image kinds, e.g. "Atlas_Core.Atlas_Filled.pencil".
	Image string `json:"image,omitempty"`
	// Code is the glyph's numeric character code, set only for MenuIconGlyph.
	// It is the ONLY thing identifying a glyph icon.
	Code    int      `json:"code,omitempty"`
	Name    string   `json:"name,omitempty"`    // glyph name (legacy Glyph icons)
	ImageID model.ID `json:"imageId,omitempty"` // legacy image id
}

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
	// Class, Style and DynamicClasses are the item's Forms$Appearance — the
	// same three fields a widget's appearance carries.
	Class          string `json:"class,omitempty"`
	Style          string `json:"style,omitempty"`
	DynamicClasses string `json:"dynamicClasses,omitempty"`
	// ConditionalVisibility is the item's own Forms$ConditionalVisibilitySettings
	// — an expression, attribute condition and/or module roles, as a widget's.
	// nil is the null Studio Pro stores on an unrestricted item (#1460).
	ConditionalVisibility *ConditionalVisibilitySettings `json:"conditionalVisibility,omitempty"`
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

// ActionExecution is what every client action but NoAction stores about its
// own execution. Nil means "not written": the writer's default, true, which is
// what Studio Pro stores on all but a few actions (ako/mxcli#721 L2).
type ActionExecution struct {
	DisabledDuringExecution *bool `json:"disabledDuringExecution,omitempty"`
}

// Execution returns the action's execution settings.
func (e *ActionExecution) Execution() *ActionExecution { return e }

// ExecutionOf returns a client action's execution settings, or nil for an
// action that has none (NoClientAction).
func ExecutionOf(a ClientAction) *ActionExecution {
	if x, ok := a.(interface{ Execution() *ActionExecution }); ok {
		return x.Execution()
	}
	return nil
}

// FlowCallSettings are the settings Studio Pro shows on a microflow or nanoflow
// call: the progress bar, its message, and a confirmation asked before the call
// runs. The zero value is Mendix's default: no progress bar, no message, no
// confirmation. Asynchronous and FormValidations exist on a microflow call only
// (they live on its Forms$MicroflowSettings).
type FlowCallSettings struct {
	ProgressBar     string            `json:"progressBar,omitempty"` // "" (None) | None | NonBlocking | Blocking
	ProgressMessage *model.Text       `json:"progressMessage,omitempty"`
	Confirmation    *ConfirmationInfo `json:"confirmation,omitempty"`
	Asynchronous    bool              `json:"asynchronous,omitempty"`    // microflow only
	FormValidations string            `json:"formValidations,omitempty"` // microflow only: "" (All) | All | Widget | None
}

// FlowOutputMapping is Forms$OutputMapping, one row of Studio Pro's "Return
// value mappings": a value read from the flow's result into the page.
//
// The target is an attribute of the data container's object (TargetAttribute,
// qualified) or a page variable (TargetVariable, stored in the mapping's
// SourceVariable despite the name). The source is an attribute of the returned
// object (SourceAttribute, qualified) or an Expression; neither set means the
// return value itself.
type FlowOutputMapping struct {
	TargetAttribute string `json:"targetAttribute,omitempty"`
	TargetVariable  string `json:"targetVariable,omitempty"`
	SourceAttribute string `json:"sourceAttribute,omitempty"`
	Expression      string `json:"expression,omitempty"`
}

// ConfirmationInfo is Forms$ConfirmationInfo: the question asked before a flow
// call runs, and the captions of its two buttons.
type ConfirmationInfo struct {
	Question       *model.Text `json:"question,omitempty"`
	ProceedCaption *model.Text `json:"proceedCaption,omitempty"`
	CancelCaption  *model.Text `json:"cancelCaption,omitempty"`
}

// NoClientAction represents no action.
type NoClientAction struct {
	model.BaseElement
}

func (NoClientAction) isClientAction() {}

// PageClientAction opens a page.
type PageClientAction struct {
	model.BaseElement
	ActionExecution
	PageID            model.ID                      `json:"pageId"`
	PageName          string                        `json:"pageName,omitempty"` // Qualified name for by-name reference
	PageSettings      *PageSettings                 `json:"pageSettings,omitempty"`
	ParameterMappings []*PageClientParameterMapping `json:"parameterMappings,omitempty"`
	// NumberOfPagesToClose is Forms$FormAction.NumberOfPagesToClose2: how many
	// pages to close as the page opens, "" for none (ako/mxcli#2106).
	NumberOfPagesToClose string `json:"numberOfPagesToClose,omitempty"`
}

func (PageClientAction) isClientAction() {}

// PageClientParameterMapping maps a page parameter to a value in a PageClientAction.
// BSON storage type: Forms$PageParameterMapping. Uses "Argument" field (not "Expression").
type PageClientParameterMapping struct {
	model.BaseElement
	ParameterName string `json:"parameterName"`        // Page parameter name (without $)
	Variable      string `json:"variable,omitempty"`   // Variable reference (e.g., "$Customer")
	Expression    string `json:"expression,omitempty"` // Expression value (maps to BSON "Argument")
	// VariableKind is set when the argument is a page or snippet parameter
	// bound by name ("parameter" / "snippet"): the writer then stores an
	// explicit mapping whose Variable is a Forms$PageVariable, as Studio Pro
	// does. Empty means the argument is the context object, which Mendix infers
	// from an empty ParameterMappings array (#296). See mendixlabs/mxcli#1403.
	// "widget" is a control-bar button passing its grid's selection, Variable
	// then naming the grid (mendixlabs/mxcli#122).
	VariableKind string `json:"variableKind,omitempty"`
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
//
// An argument binds one of two ways, and Mendix does not treat them as
// interchangeable. A reference to a page parameter, snippet parameter or page
// variable is stored as a Forms$PageVariable under Variable; anything else — a
// literal, an expression — is stored as text under Expression. Writing a
// $-reference as an Expression leaves the parameter unbound: Studio Pro reports
// CE1571 and mxbuild builds it at 0 errors (mendixlabs/mxcli#1140).
//
// VariableKind is what says which of the two applies, and which slot of the
// PageVariable to fill. Empty means "not a page-variable reference" — Variable is
// then still written as the Expression, which is what $currentObject and every
// pre-#1140 caller relies on.
type MicroflowParameterMapping struct {
	model.BaseElement
	ParameterName string `json:"parameterName"`          // Parameter name (without $)
	Variable      string `json:"variable,omitempty"`     // Variable reference (e.g., "$Customer")
	VariableKind  string `json:"variableKind,omitempty"` // "" | "parameter" | "snippet" | "local"
	Expression    string `json:"expression,omitempty"`   // Expression value
}

// MicroflowClientAction calls a microflow.
type MicroflowClientAction struct {
	model.BaseElement
	ActionExecution
	FlowCallSettings
	MicroflowID       model.ID                     `json:"microflowId"`
	MicroflowName     string                       `json:"microflowName,omitempty"` // Qualified name for BSON serialization
	ParameterMappings []*MicroflowParameterMapping `json:"parameterMappings,omitempty"`
	// OutputMappings are the call's return value mappings (Mendix 11.5),
	// stored in its MicroflowSettings.
	OutputMappings []*FlowOutputMapping `json:"outputMappings,omitempty"`
}

func (MicroflowClientAction) isClientAction() {}

// NanoflowParameterMapping maps a nanoflow parameter to a value in a NanoflowClientAction.
// BSON storage type: Forms$NanoflowParameterMapping (not Pages$).
//
// Same two binding forms as MicroflowParameterMapping — see its comment for why
// VariableKind decides between them.
type NanoflowParameterMapping struct {
	model.BaseElement
	ParameterName string `json:"parameterName"`          // Parameter name (without $)
	Variable      string `json:"variable,omitempty"`     // Variable reference (e.g., "$Customer")
	VariableKind  string `json:"variableKind,omitempty"` // "" | "parameter" | "snippet" | "local"
	Expression    string `json:"expression,omitempty"`   // Expression value
}

// NanoflowClientAction calls a nanoflow.
type NanoflowClientAction struct {
	model.BaseElement
	ActionExecution
	FlowCallSettings
	NanoflowID        model.ID                    `json:"nanoflowId"`
	NanoflowName      string                      `json:"nanoflowName,omitempty"` // Qualified name for BSON serialization
	ParameterMappings []*NanoflowParameterMapping `json:"parameterMappings,omitempty"`
	// OutputMappings are the call's return value mappings (Mendix 11.5).
	OutputMappings []*FlowOutputMapping `json:"outputMappings,omitempty"`
}

func (NanoflowClientAction) isClientAction() {}

// ClosePageClientAction closes the current page.
type ClosePageClientAction struct {
	model.BaseElement
	ActionExecution
}

func (ClosePageClientAction) isClientAction() {}

// SaveChangesClientAction saves changes.
type SaveChangesClientAction struct {
	model.BaseElement
	ActionExecution
	ClosePage bool `json:"closePage"`
}

func (SaveChangesClientAction) isClientAction() {}

// CancelChangesClientAction cancels changes.
type CancelChangesClientAction struct {
	model.BaseElement
	ActionExecution
	ClosePage bool `json:"closePage"`
}

func (CancelChangesClientAction) isClientAction() {}

// CreateObjectClientAction creates an object.
type CreateObjectClientAction struct {
	model.BaseElement
	ActionExecution
	EntityID   model.ID `json:"entityId"`
	EntityName string   `json:"entityName,omitempty"` // Qualified name e.g. "Module.Entity"
	PageID     model.ID `json:"pageId,omitempty"`
	PageName   string   `json:"pageName,omitempty"` // Qualified name e.g. "Module.Page"
}

func (CreateObjectClientAction) isClientAction() {}

// DeleteClientAction deletes an object.
type DeleteClientAction struct {
	model.BaseElement
	ActionExecution
	ClosePage bool `json:"closePage"`
	// SourceVariable is the object deleted when it is not the context
	// object: a widget's selection ({Widget: "grid1"}, a button in a grid's
	// control bar) or a page/snippet parameter. nil is the context object,
	// stored as a null SourceVariable (ako/mxcli#1932).
	SourceVariable *WidgetVariable `json:"sourceVariable,omitempty"`
}

func (DeleteClientAction) isClientAction() {}

// SignOutClientAction signs out the user.
type SignOutClientAction struct {
	model.BaseElement
	ActionExecution
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
	ActionExecution
	LinkType LinkType `json:"linkType"`
	Address  string   `json:"address,omitempty"`
	// AddressAttribute, when set, makes the address dynamic: the runtime reads
	// it from this attribute (Module.Entity.Attr) and Address is ignored.
	AddressAttribute string `json:"addressAttribute,omitempty"`
}

func (LinkClientAction) isClientAction() {}

// SetTaskOutcomeClientAction completes a workflow user task with a named outcome.
type SetTaskOutcomeClientAction struct {
	model.BaseElement
	ActionExecution
	ClosePage    bool   `json:"closePage,omitempty"`
	Commit       bool   `json:"commit,omitempty"`
	OutcomeValue string `json:"outcomeValue,omitempty"`
}

func (SetTaskOutcomeClientAction) isClientAction() {}

// OpenUserTaskClientAction is Studio Pro's "Show user task page": it opens the
// page of the user task in the enclosing data container. AssignOnOpen is
// "Auto-assign"; OpenWhenAssigned is "Who can open", true for "Users with
// access" and false for "Assigned user". Nil leaves either unwritten, for a
// project older than Mendix 9.20, which introduced both.
type OpenUserTaskClientAction struct {
	model.BaseElement
	ActionExecution
	AssignOnOpen     *bool `json:"assignOnOpen,omitempty"`
	OpenWhenAssigned *bool `json:"openWhenAssigned,omitempty"`
}

func (OpenUserTaskClientAction) isClientAction() {}

// OpenWorkflowClientAction is Studio Pro's "Show workflow admin page": it opens
// the admin page of the workflow in the enclosing data container. DefaultPage
// is a page qualified name, "" for none.
type OpenWorkflowClientAction struct {
	model.BaseElement
	ActionExecution
	DefaultPage string `json:"defaultPage,omitempty"`
}

func (OpenWorkflowClientAction) isClientAction() {}

// CallWorkflowClientAction is Studio Pro's "Call workflow": it starts the
// workflow with the enclosing data container's object as its context.
type CallWorkflowClientAction struct {
	model.BaseElement
	ActionExecution
	WorkflowName string            `json:"workflowName,omitempty"` // Qualified name e.g. "Module.Workflow"
	ClosePage    bool              `json:"closePage"`
	Confirmation *ConfirmationInfo `json:"confirmation,omitempty"`
}

func (CallWorkflowClientAction) isClientAction() {}

// LinkType represents the type of link.
type LinkType string

const (
	LinkTypeWeb   LinkType = "Web"
	LinkTypeEmail LinkType = "Email"
	LinkTypePhone LinkType = "Phone"
)
