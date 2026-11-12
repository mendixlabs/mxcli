// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// Input Widgets

// ReadOnlyStyles are the members of Mendix's "Read-only style", stored as
// ReadOnlyStyle on a check box, text box, text area, date picker, radio button
// group and data view.
var ReadOnlyStyles = []string{"Inherit", "Control", "Text"}

// CanonicalReadOnlyStyle matches s case-insensitively against ReadOnlyStyles
// and returns the member as Mendix spells it.
func CanonicalReadOnlyStyle(s string) (string, bool) {
	for _, v := range ReadOnlyStyles {
		if strings.EqualFold(strings.TrimSpace(s), v) {
			return v, true
		}
	}
	return "", false
}

// KeyboardTypes are the members of a text box's "Keyboard type", the keyboard
// a native app opens for it (Forms$TextBox.KeyboardType, mendixmodelsdk; Studio
// Pro's default is Default).
var KeyboardTypes = []string{"Default", "NumberPad", "DecimalPad", "EmailAddress", "PhonePad", "URL"}

// CanonicalKeyboardType matches s case-insensitively against KeyboardTypes and
// returns the member as Mendix spells it.
func CanonicalKeyboardType(s string) (string, bool) {
	for _, v := range KeyboardTypes {
		if strings.EqualFold(strings.TrimSpace(s), v) {
			return v, true
		}
	}
	return "", false
}

// WidgetVariable is an input widget's SourceVariable: the Forms$PageVariable
// that names which object the widget's attribute is read from when that is not
// simply the enclosing data context. Studio Pro stores a widget-scoped one —
// Widget names a data view, and the data view's own variable fills the slot
// Kind selects ("" page parameter, "snippet", "local") — on
// Administration.Account_Edit/Account_New and WorkflowCommons'
// WorkflowComment_Edit_Admin (ako/mxcli#826).
type WidgetVariable struct {
	Widget   string `json:"widget,omitempty"`
	Variable string `json:"variable,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

// TextBox represents a text input widget.
type TextBox struct {
	BaseWidget
	Label             string             `json:"label,omitempty"`
	AttributePath     string             `json:"attributePath,omitempty"`
	AttributeRefSteps []AttributeRefStep `json:"attributeRefSteps,omitempty"` // association hops when the attribute is reached over associations (AttributeRef.EntityRef)
	SourceVariable    *WidgetVariable    `json:"sourceVariable,omitempty"`    // widget-scoped variable the attribute is read from (#826)
	FormattingInfo    *FormattingInfo    `json:"formattingInfo,omitempty"`
	Placeholder       *model.Text        `json:"placeholder,omitempty"`
	MaxLength         int                `json:"maxLength,omitempty"`
	IsPassword        bool               `json:"isPassword,omitempty"`
	// KeyboardType is one of KeyboardTypes; empty writes Default. It was
	// hard-coded, so a rewrite reset a native text box's keyboard
	// (ako/mxcli#1209).
	KeyboardType string `json:"keyboardType,omitempty"`
	// ReadOnlyStyle: Inherit, Control or Text, as on CheckBox. Empty writes
	// Studio Pro's default, Inherit (ako/mxcli#977).
	ReadOnlyStyle string `json:"readOnlyStyle,omitempty"`
	// ValidationExpression / ValidationMessage are the two fields of the widget's
	// Forms$WidgetValidation. Without them a rewrite wrote the empty default over
	// whatever Studio Pro had stored (ako/mxcli#550).
	ValidationExpression string       `json:"validationExpression,omitempty"`
	ValidationMessage    string       `json:"validationMessage,omitempty"`
	ReadOnly             bool         `json:"readOnly,omitempty"`
	OnChangeAction       ClientAction `json:"onChangeAction,omitempty"`
	OnEnterAction        ClientAction `json:"onEnterAction,omitempty"`
	// OnEnterAction / OnLeaveAction are Studio Pro's Events → "On enter" and
	// "On leave" (focus in, focus out), stored on every input widget. nil
	// writes the empty NoAction (ako/mxcli#1743).
	OnLeaveAction ClientAction `json:"onLeaveAction,omitempty"`
	// OnEnterKeyPressAction is Studio Pro's Events → "On enter key press",
	// Forms$TextBox.OnEnterKeyPressAction. A text box's only: Forms$TextArea
	// declares no such slot. nil writes the empty NoAction (ako/mxcli#1743).
	OnEnterKeyPressAction ClientAction `json:"onEnterKeyPressAction,omitempty"`
	// SubmitBehaviour is Forms$TextBox/TextArea.SubmitBehaviour: "OnEndEditing"
	// (Studio Pro's default) or "WhileEditing", which submits as the user types.
	// SubmitOnInputDelay is the debounce in milliseconds that goes with it; nil
	// keeps the default 300. Empty / nil write the defaults. Both were hard-coded
	// in the writer, so describe → exec rewrote a stored WhileEditing to the
	// default (ako/mxcli-ledger FINDINGS #170).
	SubmitBehaviour    string `json:"submitBehaviour,omitempty"`
	SubmitOnInputDelay *int   `json:"submitOnInputDelay,omitempty"`
}

// TextArea represents a multi-line text input widget.
type TextArea struct {
	BaseWidget
	Label             string             `json:"label,omitempty"`
	AttributePath     string             `json:"attributePath,omitempty"`
	AttributeRefSteps []AttributeRefStep `json:"attributeRefSteps,omitempty"` // association hops when the attribute is reached over associations (AttributeRef.EntityRef)
	SourceVariable    *WidgetVariable    `json:"sourceVariable,omitempty"`    // widget-scoped variable the attribute is read from (#826)
	Placeholder       *model.Text        `json:"placeholder,omitempty"`
	MaxLength         int                `json:"maxLength,omitempty"`
	CounterMessage    *model.Text        `json:"counterMessage,omitempty"`
	// NumberOfLines is Forms$TextArea.NumberOfLines; nil writes Studio Pro's
	// default, 5 (52 of 58 text areas in PedApp and TestApp). Studio Pro also
	// stores 0, which an int defaulting on zero could not express (ako/mxcli#977).
	NumberOfLines *int32 `json:"numberOfLines,omitempty"`
	// Autocomplete is Forms$TextArea.Autocomplete; nil writes true, the
	// default (ako/mxcli#977).
	Autocomplete *bool `json:"autocomplete,omitempty"`
	// ReadOnlyStyle: Inherit, Control or Text, as on CheckBox. Empty writes
	// Studio Pro's default, Inherit (ako/mxcli#977).
	ReadOnlyStyle  string       `json:"readOnlyStyle,omitempty"`
	ReadOnly       bool         `json:"readOnly,omitempty"`
	OnChangeAction ClientAction `json:"onChangeAction,omitempty"`
	OnEnterAction  ClientAction `json:"onEnterAction,omitempty"` // as on TextBox
	OnLeaveAction  ClientAction `json:"onLeaveAction,omitempty"` // as on TextBox
	// SubmitBehaviour / SubmitOnInputDelay: as on TextBox.
	SubmitBehaviour    string `json:"submitBehaviour,omitempty"`
	SubmitOnInputDelay *int   `json:"submitOnInputDelay,omitempty"`
}

// FormattingInfo represents formatting configuration.
type FormattingInfo struct {
	model.BaseElement
	DateFormat       string `json:"dateFormat,omitempty"`
	CustomDateFormat string `json:"customDateFormat,omitempty"` // pattern paired with DateFormat="Custom"
	TimeFormat       string `json:"timeFormat,omitempty"`       // not a Forms$FormattingInfo schema field — never serialized (CE0463)
	DecimalPrecision int    `json:"decimalPrecision,omitempty"`
	GroupDigits      bool   `json:"groupDigits,omitempty"`
	EnumFormat       string `json:"enumFormat,omitempty"`
}

// DatePicker represents a date picker widget.
type DatePicker struct {
	BaseWidget
	Label             string             `json:"label,omitempty"`
	AttributePath     string             `json:"attributePath,omitempty"`
	AttributeRefSteps []AttributeRefStep `json:"attributeRefSteps,omitempty"` // association hops when the attribute is reached over associations (AttributeRef.EntityRef)
	SourceVariable    *WidgetVariable    `json:"sourceVariable,omitempty"`    // widget-scoped variable the attribute is read from (#826)
	Placeholder       *model.Text        `json:"placeholder,omitempty"`
	// FormattingInfo is the picker's Forms$FormattingInfo: DateFormat (Date,
	// Time, DateTime, Custom) and the CustomDateFormat pattern. nil writes the
	// defaults (Date). It replaced a DateFormat string nothing read, so every
	// picker was written as date-only (ako/mxcli#968).
	FormattingInfo *FormattingInfo `json:"formattingInfo,omitempty"`
	// ReadOnlyStyle: Inherit, Control or Text, as on CheckBox. Empty writes
	// Studio Pro's default, Inherit (ako/mxcli#977).
	ReadOnlyStyle  string       `json:"readOnlyStyle,omitempty"`
	ReadOnly       bool         `json:"readOnly,omitempty"`
	OnChangeAction ClientAction `json:"onChangeAction,omitempty"`
	OnEnterAction  ClientAction `json:"onEnterAction,omitempty"` // as on TextBox
	OnLeaveAction  ClientAction `json:"onLeaveAction,omitempty"` // as on TextBox
}

// DropDown represents a drop-down selection widget.
type DropDown struct {
	BaseWidget
	Label             string             `json:"label,omitempty"`
	AttributePath     string             `json:"attributePath,omitempty"`
	AttributeRefSteps []AttributeRefStep `json:"attributeRefSteps,omitempty"` // association hops when the attribute is reached over associations (AttributeRef.EntityRef)
	SourceVariable    *WidgetVariable    `json:"sourceVariable,omitempty"`    // widget-scoped variable the attribute is read from (#826)
	EmptyOption       *model.Text        `json:"emptyOption,omitempty"`
	ReadOnly          bool               `json:"readOnly,omitempty"`
	OnChangeAction    ClientAction       `json:"onChangeAction,omitempty"`
	OnEnterAction     ClientAction       `json:"onEnterAction,omitempty"` // as on TextBox
	OnLeaveAction     ClientAction       `json:"onLeaveAction,omitempty"` // as on TextBox
}

// ReferenceSelector represents a reference selector widget.
type ReferenceSelector struct {
	BaseWidget
	AttributePath  string         `json:"attributePath,omitempty"`
	EmptyOption    *model.Text    `json:"emptyOption,omitempty"`
	SelectorSource SelectorSource `json:"selectorSource,omitempty"`
	ReadOnly       bool           `json:"readOnly,omitempty"`
	OnChangeAction ClientAction   `json:"onChangeAction,omitempty"`
}

// SelectorSource represents the source for a reference selector.
type SelectorSource interface {
	isSelectorSource()
}

// DatabaseSelectorSource uses a database query.
type DatabaseSelectorSource struct {
	model.BaseElement
	XPathConstraint string `json:"xPathConstraint,omitempty"`
}

func (DatabaseSelectorSource) isSelectorSource() {}

// MicroflowSelectorSource uses a microflow.
type MicroflowSelectorSource struct {
	model.BaseElement
	MicroflowID model.ID `json:"microflowId"`
}

func (MicroflowSelectorSource) isSelectorSource() {}

// ReferenceSetSelector represents a reference set selector widget.
type ReferenceSetSelector struct {
	BaseWidget
	AttributePath  string         `json:"attributePath,omitempty"`
	SelectorSource SelectorSource `json:"selectorSource,omitempty"`
	ReadOnly       bool           `json:"readOnly,omitempty"`
	OnChangeAction ClientAction   `json:"onChangeAction,omitempty"`
	ShowSelectPage bool           `json:"showSelectPage,omitempty"`
	SelectPageID   model.ID       `json:"selectPageId,omitempty"`
}

// CheckBox represents a checkbox widget.
type CheckBox struct {
	BaseWidget
	Label             string             `json:"label,omitempty"`
	AttributePath     string             `json:"attributePath,omitempty"`
	AttributeRefSteps []AttributeRefStep `json:"attributeRefSteps,omitempty"` // association hops when the attribute is reached over associations (AttributeRef.EntityRef)
	SourceVariable    *WidgetVariable    `json:"sourceVariable,omitempty"`    // widget-scoped variable the attribute is read from (#826)
	ReadOnly          bool               `json:"readOnly,omitempty"`
	// ReadOnlyStyle is Mendix's "Read-only style": Inherit, Control or Text.
	// Empty means unset — the writer keeps the stored default (Inherit), so a
	// script that never mentions it produces the document it always did.
	//
	// It decides how a READ-ONLY check box renders, and the difference is not
	// cosmetic: Text renders the words "Yes"/"No", Control renders the (disabled)
	// checkbox glyph. That is the whole of "show a Boolean as a checkbox" in a
	// DataGrid2 cell (ako/mxcli#490).
	ReadOnlyStyle  string       `json:"readOnlyStyle,omitempty"`
	OnChangeAction ClientAction `json:"onChangeAction,omitempty"`
	OnEnterAction  ClientAction `json:"onEnterAction,omitempty"` // as on TextBox
	OnLeaveAction  ClientAction `json:"onLeaveAction,omitempty"` // as on TextBox
	// LabelPosition is Forms$CheckBox.LabelPosition: Default, BeforeControl or
	// AfterControl. Empty keeps the stored default (ako/mxcli#2106).
	LabelPosition string `json:"labelPosition,omitempty"`
}

// RadioButtons represents a radio button group widget.
type RadioButtons struct {
	BaseWidget
	Label             string             `json:"label,omitempty"`
	AttributePath     string             `json:"attributePath,omitempty"`
	AttributeRefSteps []AttributeRefStep `json:"attributeRefSteps,omitempty"` // association hops when the attribute is reached over associations (AttributeRef.EntityRef)
	SourceVariable    *WidgetVariable    `json:"sourceVariable,omitempty"`    // widget-scoped variable the attribute is read from (#826)
	RenderDirection   RenderDirection    `json:"renderDirection,omitempty"`
	// ReadOnlyStyle: Inherit, Control or Text, as on CheckBox. Empty writes
	// Studio Pro's default, Inherit (ako/mxcli#977).
	ReadOnlyStyle  string       `json:"readOnlyStyle,omitempty"`
	ReadOnly       bool         `json:"readOnly,omitempty"`
	OnChangeAction ClientAction `json:"onChangeAction,omitempty"`
	OnEnterAction  ClientAction `json:"onEnterAction,omitempty"` // as on TextBox
	OnLeaveAction  ClientAction `json:"onLeaveAction,omitempty"` // as on TextBox
}

// RenderDirection represents the direction for rendering.
type RenderDirection string

const (
	RenderDirectionHorizontal RenderDirection = "Horizontal"
	RenderDirectionVertical   RenderDirection = "Vertical"
)

// FileManager represents the classic file manager (Forms$FileManager). It has
// no attribute: it uploads into / downloads from the enclosing data view's
// object, which must be a System.FileDocument (mendixlabs/mxcli#151).
type FileManager struct {
	BaseWidget
	Label             string          `json:"label,omitempty"`
	Type              FileManagerType `json:"type"`
	AllowedExtensions string          `json:"allowedExtensions,omitempty"`
	MaxFileSize       int             `json:"maxFileSize,omitempty"` // megabytes
	ShowFileInBrowser bool            `json:"showFileInBrowser,omitempty"`
}

// FileManagerType represents the type of file manager.
type FileManagerType string

const (
	FileManagerTypeUpload   FileManagerType = "Upload"
	FileManagerTypeDownload FileManagerType = "Download"
	FileManagerTypeBoth     FileManagerType = "Both"
)

// CanonicalFileManagerType resolves an authored file manager Type, matched
// case-insensitively, to Mendix's member casing. An unknown member is a
// document Studio Pro cannot load, so it is reported rather than stored.
func CanonicalFileManagerType(s string) (FileManagerType, bool) {
	for _, t := range []FileManagerType{FileManagerTypeUpload, FileManagerTypeDownload, FileManagerTypeBoth} {
		if strings.EqualFold(s, string(t)) {
			return t, true
		}
	}
	return "", false
}

// IsFileManager reports whether a stored $Type is the classic file manager.
func IsFileManager(typeName string) bool {
	return typeName == "Forms$FileManager" || typeName == "Pages$FileManager"
}

// ImageUploader represents an image uploader widget.
type ImageUploader struct {
	BaseWidget
	AllowedExtensions string `json:"allowedExtensions,omitempty"`
	MaxFileSize       int    `json:"maxFileSize,omitempty"`
	ThumbnailSize     int    `json:"thumbnailSize,omitempty"`
}
