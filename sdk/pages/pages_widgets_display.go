// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Text and Display Widgets

// Text represents a static text widget.
type Text struct {
	BaseWidget
	Caption    *model.Text    `json:"caption,omitempty"`
	RenderMode TextRenderMode `json:"renderMode,omitempty"`
}

// TextRenderMode represents how text is rendered.
type TextRenderMode string

const (
	TextRenderModeText      TextRenderMode = "Text"
	TextRenderModeH1        TextRenderMode = "H1"
	TextRenderModeH2        TextRenderMode = "H2"
	TextRenderModeH3        TextRenderMode = "H3"
	TextRenderModeH4        TextRenderMode = "H4"
	TextRenderModeH5        TextRenderMode = "H5"
	TextRenderModeH6        TextRenderMode = "H6"
	TextRenderModeParagraph TextRenderMode = "Paragraph"
)

// ClientTemplate represents a text template with parameters.
// Used for dynamic text content in widgets like DynamicText and ActionButton captions.
type ClientTemplate struct {
	model.BaseElement
	Template   *model.Text                `json:"template,omitempty"`
	Parameters []*ClientTemplateParameter `json:"parameters,omitempty"`
	Fallback   *model.Text                `json:"fallback,omitempty"`
}

// AttributeRefStep is one hop of an association-navigated attribute reference
// (DomainModels$EntityRefStep). A ClientTemplateParameter that binds an
// attribute *over* an association (e.g. {1} = Order_Customer/Name) carries one
// step per association hop; the final attribute lives in AttributeRef.
type AttributeRefStep struct {
	Association       string `json:"association,omitempty"`       // qualified association name, e.g. "Sales.Order_Customer"
	DestinationEntity string `json:"destinationEntity,omitempty"` // qualified entity reached by this hop, e.g. "Sales.Customer"
}

// ClientTemplateParameter represents a parameter in a client template.
// Used to substitute values for placeholders like {1}, {2} in template text.
type ClientTemplateParameter struct {
	model.BaseElement
	AttributeRef       string             `json:"attributeRef,omitempty"`       // Qualified attribute path like "Module.Entity.Attribute"
	AttributeRefSteps  []AttributeRefStep `json:"attributeRefSteps,omitempty"`  // association hops when the attribute is navigated over associations (AttributeRef.EntityRef)
	Expression         string             `json:"expression,omitempty"`         // Literal expression like "'Hello'"
	SourceVariable     string             `json:"sourceVariable,omitempty"`     // Variable name (no $ prefix)
	SourceVariableKind string             `json:"sourceVariableKind,omitempty"` // "" (default = page parameter), "local" (page-level Variables entry), or "snippet"
	FormattingInfo     *FormattingInfo    `json:"formattingInfo,omitempty"`
}

// DynamicText represents dynamic text based on an attribute.
type DynamicText struct {
	BaseWidget
	AttributePath string          `json:"attributePath,omitempty"`
	Content       *ClientTemplate `json:"content,omitempty"`
	RenderMode    TextRenderMode  `json:"renderMode,omitempty"`
}

// Label represents a label widget.
type Label struct {
	BaseWidget
	Caption *model.Text `json:"caption,omitempty"`
	ForID   model.ID    `json:"forId,omitempty"`
}

// Title represents a page title widget.
type Title struct {
	BaseWidget
	Caption *model.Text `json:"caption,omitempty"`
}

// DynamicImage represents a dynamic image widget.
type DynamicImage struct {
	BaseWidget
	DefaultImage  model.ID     `json:"defaultImage,omitempty"`
	Width         int          `json:"width,omitempty"`
	WidthUnit     WidthUnit    `json:"widthUnit,omitempty"`
	Height        int          `json:"height,omitempty"`
	Responsive    bool         `json:"responsive"`
	OnClickAction ClientAction `json:"onClickAction,omitempty"`
}

// StaticImage represents a static image widget.
type StaticImage struct {
	BaseWidget
	ImageID       model.ID     `json:"imageId,omitempty"`
	Width         int          `json:"width,omitempty"`
	WidthUnit     WidthUnit    `json:"widthUnit,omitempty"`
	Height        int          `json:"height,omitempty"`
	Responsive    bool         `json:"responsive"`
	OnClickAction ClientAction `json:"onClickAction,omitempty"`
}
