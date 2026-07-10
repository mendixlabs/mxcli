// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Widget is the base interface for all page widgets.
type Widget interface {
	GetID() model.ID
	GetTypeName() string
	GetName() string
}

// DesignPropertyValue represents a design property (from Atlas UI theme).
// ValueType determines the BSON serialization type:
//   - "toggle"   → Forms$ToggleDesignPropertyValue (Toggle type, no value)
//   - "option"   → Forms$OptionDesignPropertyValue (Dropdown type, uses Option field)
//   - "custom"   → Forms$CustomDesignPropertyValue (ToggleButtonGroup/ColorPicker, uses Option as Value)
//   - "compound" → Forms$CompoundDesignPropertyValue (a property whose value is a set of
//     sub-properties, e.g. Atlas "Spacing" → margin-top/bottom/…; uses Compound)
type DesignPropertyValue struct {
	Key       string                // Design property key, e.g., "Shadow" or "Spacing"
	ValueType string                // "toggle", "option", "custom", or "compound"
	Option    string                // Selected value (for "option"/"custom" types)
	Compound  []DesignPropertyValue // Sub-properties (for "compound" type)
}

// BaseWidget provides common fields for all widgets.
type BaseWidget struct {
	model.BaseElement
	Name                   string                          `json:"name"`
	Class                  string                          `json:"class,omitempty"`
	Style                  string                          `json:"style,omitempty"`
	DynamicClasses         string                          `json:"dynamicClasses,omitempty"` // Set via DynamicClasses: expression
	TabIndex               int                             `json:"tabIndex,omitempty"`
	DesignProperties       []DesignPropertyValue           `json:"designProperties,omitempty"`
	ConditionalVisibility  *ConditionalVisibilitySettings  `json:"-"` // Set via VISIBLE IF
	ConditionalEditability *ConditionalEditabilitySettings `json:"-"` // Set via EDITABLE IF
}

// GetName returns the widget's name.
func (w *BaseWidget) GetName() string {
	return w.Name
}

// GetBaseWidget returns a pointer to the BaseWidget for accessing conditional settings.
func (w *BaseWidget) GetBaseWidget() *BaseWidget {
	return w
}

// SetAppearance sets the CSS class and inline style on the widget.
func (w *BaseWidget) SetAppearance(class, style string) {
	w.Class = class
	w.Style = style
}

// SetDynamicClasses sets the dynamic-classes expression on the widget's
// Forms$Appearance (a class-list computed at runtime from an expression).
func (w *BaseWidget) SetDynamicClasses(dynamicClasses string) {
	w.DynamicClasses = dynamicClasses
}

// SetDesignProperties sets the design properties on the widget.
func (w *BaseWidget) SetDesignProperties(props []DesignPropertyValue) {
	w.DesignProperties = props
}

// Placeholder Widgets

// LayoutPlaceholder represents a placeholder in a layout.
type LayoutPlaceholder struct {
	BaseWidget
}

// ConditionalVisibilitySettings represents visibility conditions.
type ConditionalVisibilitySettings struct {
	model.BaseElement
	Expression     string        `json:"expression,omitempty"`
	ModuleRoles    []model.ID    `json:"moduleRoles,omitempty"`
	SourceVariable *PageVariable `json:"sourceVariable,omitempty"`
	Attribute      model.ID      `json:"attribute,omitempty"`
}

// ConditionalEditabilitySettings represents editability conditions.
type ConditionalEditabilitySettings struct {
	model.BaseElement
	Expression string `json:"expression,omitempty"`
}

// StaticVisibleExpression maps a static/string `Visible` value to the client
// expression stored in a widget's ConditionalVisibilitySettings. Page widgets
// have no plain boolean "Visible" field — visibility is always modeled as a
// conditional-visibility expression — so `Visible: false` becomes the constant
// "false", and a string is treated as the expression verbatim (the caller is
// responsible for rooting it in $currentObject, unlike the `[ … ]` form which
// auto-roots). It returns hasSetting=false for the default-visible cases
// (`true` / "true" / empty), signalling "no settings node / clear it".
func StaticVisibleExpression(v any) (expr string, hasSetting bool) {
	switch t := v.(type) {
	case bool:
		if t {
			return "", false
		}
		return "false", true
	case string:
		s := strings.TrimSpace(t)
		switch strings.ToLower(s) {
		case "", "true":
			return "", false
		case "false":
			return "false", true
		default:
			return s, true
		}
	default:
		return "", false
	}
}

// PageVariable represents a page variable reference.
type PageVariable struct {
	model.BaseElement
	UseAllPages bool     `json:"useAllPages"`
	PageID      model.ID `json:"pageId,omitempty"`
	Widget      string   `json:"widget,omitempty"`
}
