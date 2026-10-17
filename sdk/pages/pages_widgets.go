// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"strings"

	"github.com/mendixlabs/mxcli/model"
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
	// Editable is Mendix's Editability enum — "Always", "Never" or
	// "Conditional". Empty means the author did not say, which the writers
	// render as Mendix's own default, "Always".
	//
	// It lives on BaseWidget rather than on each input widget because the same
	// `editable:` property applies to every widget that has editability at all
	// (twelve MDL types — see editableWidgetTypes), and MDL-WIDGET20 already
	// refuses it on the ones that do not. Before this field existed the property
	// was parsed, validated, and then dropped between the AST and the model: the
	// writers hardcoded "Always" for TextBox, TextArea, CheckBox, DatePicker and
	// RadioButtons, so `editable: Never` produced an editable field with no
	// warning anywhere. Exactly the failure `Visible: false` had in
	// applyConditionalSettings, and the second half of issue #928.
	Editable string `json:"editable,omitempty"`
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
	Expression string `json:"expression,omitempty"`
	// ModuleRoles is Studio Pro's "Visible for module roles": qualified
	// Module.Role names (a BY_NAME reference list), stored as ModuleRoles [1,
	// "Mod.Role", …] — TestApp WorkflowCommons. Combinable with Expression or
	// Attribute; empty means the widget is not role-restricted.
	ModuleRoles []string `json:"moduleRoles,omitempty"`
	// SourceVariable names the object Attribute is read from when that is not
	// the enclosing data context: a page or snippet parameter, for a widget
	// outside every data container (TestApp WorkflowCommons). nil is the null
	// Studio Pro stores for a context-relative condition.
	SourceVariable *WidgetVariable `json:"sourceVariable,omitempty"`
	// Attribute is Studio Pro's "based on attribute value": the qualified
	// Module.Entity.Attr (a BY_NAME reference), with one Conditions entry per
	// value of it. Empty for the expression and module-role forms.
	Attribute  string           `json:"attribute,omitempty"`
	Conditions []ValueCondition `json:"conditions,omitempty"`
}

// ValueCondition is one Enumerations$Condition: whether the widget is
// visible (or editable) when the attribute holds Value. Studio Pro stores one
// per value — every enumeration value plus "(empty)", or "true"/"false".
type ValueCondition struct {
	Value   string `json:"value"`
	Visible bool   `json:"visible"`
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

// CanonicalEditability maps an author-written editability to the value Mendix
// stores, reporting whether it is one of the three the platform defines.
//
// The caller decides what to do with a false — a writer must not quietly pass an
// unknown string through, since Studio Pro resolves the enum on load and an
// invented member is exactly the kind of value that builds clean and will not
// open.
func CanonicalEditability(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "always":
		return "Always", true
	case "never":
		return "Never", true
	case "conditional":
		return "Conditional", true
	default:
		return "", false
	}
}

// StaticEditability resolves a static `Editable:` value, the counterpart of
// StaticVisibleExpression for the property that shares its grammar slot. A
// value is exactly one of:
//
//   - an enum word (Always | Never | Conditional, any case), returned canonical
//     as enum;
//   - a boolean, or the string "true"/"false": `true` is Always and `false` is
//     Never. Visible has no plain form and stores a constant condition for
//     `false`; editability has one, which is what Studio Pro writes for "Never";
//   - anything else non-empty, which is the client expression of a conditional
//     editability, verbatim (the caller roots it in $currentObject if it wants
//     that, as for Visible).
//
// ok is false for an empty value and for a type that is none of these. A caller
// must treat "not ok" as "nothing authored", never as "something unreadable":
// every non-empty string and boolean lands in one of the three.
func StaticEditability(v any) (enum, expr string, ok bool) {
	switch t := v.(type) {
	case bool:
		if t {
			return "Always", "", true
		}
		return "Never", "", true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return "", "", false
		}
		if canon, isEnum := CanonicalEditability(s); isEnum {
			return canon, "", true
		}
		switch strings.ToLower(s) {
		case "true":
			return "Always", "", true
		case "false":
			return "Never", "", true
		}
		return "", s, true
	default:
		return "", "", false
	}
}

// WidgetEditability is the value a writer should store for a widget: the
// conditional settings element wins, then an explicit `editable:`, then Mendix's
// own default.
func WidgetEditability(bw *BaseWidget) string {
	switch {
	case bw == nil:
		return "Always"
	case bw.ConditionalEditability != nil:
		return "Conditional"
	case bw.Editable != "":
		return bw.Editable
	default:
		return "Always"
	}
}
