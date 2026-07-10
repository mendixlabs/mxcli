// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Input Widgets

// TextBox represents a text input widget.
type TextBox struct {
	BaseWidget
	Label          string          `json:"label,omitempty"`
	AttributePath  string          `json:"attributePath,omitempty"`
	FormattingInfo *FormattingInfo `json:"formattingInfo,omitempty"`
	Placeholder    *model.Text     `json:"placeholder,omitempty"`
	MaxLength      int             `json:"maxLength,omitempty"`
	IsPassword     bool            `json:"isPassword,omitempty"`
	ReadOnly       bool            `json:"readOnly,omitempty"`
	OnChangeAction ClientAction    `json:"onChangeAction,omitempty"`
	OnEnterAction  ClientAction    `json:"onEnterAction,omitempty"`
}

// TextArea represents a multi-line text input widget.
type TextArea struct {
	BaseWidget
	Label          string       `json:"label,omitempty"`
	AttributePath  string       `json:"attributePath,omitempty"`
	Placeholder    *model.Text  `json:"placeholder,omitempty"`
	MaxLength      int          `json:"maxLength,omitempty"`
	CounterMessage *model.Text  `json:"counterMessage,omitempty"`
	Rows           int          `json:"rows,omitempty"`
	ReadOnly       bool         `json:"readOnly,omitempty"`
	OnChangeAction ClientAction `json:"onChangeAction,omitempty"`
}

// FormattingInfo represents formatting configuration.
type FormattingInfo struct {
	model.BaseElement
	DateFormat       string `json:"dateFormat,omitempty"`
	TimeFormat       string `json:"timeFormat,omitempty"`
	DecimalPrecision int    `json:"decimalPrecision,omitempty"`
	GroupDigits      bool   `json:"groupDigits,omitempty"`
	EnumFormat       string `json:"enumFormat,omitempty"`
}

// DatePicker represents a date picker widget.
type DatePicker struct {
	BaseWidget
	Label          string       `json:"label,omitempty"`
	AttributePath  string       `json:"attributePath,omitempty"`
	Placeholder    *model.Text  `json:"placeholder,omitempty"`
	DateFormat     string       `json:"dateFormat,omitempty"`
	ReadOnly       bool         `json:"readOnly,omitempty"`
	OnChangeAction ClientAction `json:"onChangeAction,omitempty"`
}

// DropDown represents a drop-down selection widget.
type DropDown struct {
	BaseWidget
	Label          string       `json:"label,omitempty"`
	AttributePath  string       `json:"attributePath,omitempty"`
	EmptyOption    *model.Text  `json:"emptyOption,omitempty"`
	ReadOnly       bool         `json:"readOnly,omitempty"`
	OnChangeAction ClientAction `json:"onChangeAction,omitempty"`
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
	Label          string       `json:"label,omitempty"`
	AttributePath  string       `json:"attributePath,omitempty"`
	ReadOnly       bool         `json:"readOnly,omitempty"`
	OnChangeAction ClientAction `json:"onChangeAction,omitempty"`
}

// RadioButtons represents a radio button group widget.
type RadioButtons struct {
	BaseWidget
	Label           string          `json:"label,omitempty"`
	AttributePath   string          `json:"attributePath,omitempty"`
	RenderDirection RenderDirection `json:"renderDirection,omitempty"`
	ReadOnly        bool            `json:"readOnly,omitempty"`
	OnChangeAction  ClientAction    `json:"onChangeAction,omitempty"`
}

// RenderDirection represents the direction for rendering.
type RenderDirection string

const (
	RenderDirectionHorizontal RenderDirection = "Horizontal"
	RenderDirectionVertical   RenderDirection = "Vertical"
)

// FileManager represents a file manager widget.
type FileManager struct {
	BaseWidget
	Type              FileManagerType `json:"type"`
	AllowedExtensions string          `json:"allowedExtensions,omitempty"`
	MaxFileSize       int             `json:"maxFileSize,omitempty"`
	ShowButton        bool            `json:"showButton,omitempty"`
}

// FileManagerType represents the type of file manager.
type FileManagerType string

const (
	FileManagerTypeUpload   FileManagerType = "Upload"
	FileManagerTypeDownload FileManagerType = "Download"
	FileManagerTypeBoth     FileManagerType = "Both"
)

// ImageUploader represents an image uploader widget.
type ImageUploader struct {
	BaseWidget
	AllowedExtensions string `json:"allowedExtensions,omitempty"`
	MaxFileSize       int    `json:"maxFileSize,omitempty"`
	ThumbnailSize     int    `json:"thumbnailSize,omitempty"`
}
