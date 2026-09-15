// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// =============================================================================
// V3 Page AST Types
// =============================================================================
//
// V3 follows the pattern: WIDGET name (Prop: Value) { children }
//
// Key differences from V2:
// - Page header uses single () block with Params:, Title:, Layout:, Url:
// - DataSource: replaces -> for containers
// - Attribute: replaces -> for input widgets and columns
// - Action: replaces -> for buttons
// - All properties are explicit key-value pairs
//

// CreatePageStmtV3 represents a V3 page creation statement.
// V3 syntax: CREATE PAGE Module.Page (Title: '...', Layout: ...) { widgets }
type CreatePageStmtV3 struct {
	Name       QualifiedName
	Parameters []PageParameter // From Params: { } block
	Variables  []PageVariable  // From Variables: { } block
	Title      string
	Layout     string
	URL        string
	Folder     string
	// Class / Style set the page's Forms$Appearance CSS class and inline style
	// (issue #714). Empty means "not specified".
	Class   string
	Style   string
	Widgets []*WidgetV3
	// Placeholders holds widgets assigned to named layout placeholders via
	// `placeholder <Name> { … }` blocks (issue #532). `Widgets` above is the
	// bare-body content, which binds to the Main placeholder. A `placeholder
	// Main { … }` block merges into Main.
	Placeholders     []*PagePlaceholderV3
	Documentation    string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
	IsReplace        bool // CREATE OR REPLACE
	IsModify         bool // CREATE OR MODIFY
	Excluded         bool // @excluded — document excluded from project

	// Pop-up dimensions (issue #661). nil means "not specified" — the executor
	// applies the Mendix defaults (600 / 600 / false).
	PopupWidth     *int
	PopupHeight    *int
	PopupResizable *bool
}

func (s *CreatePageStmtV3) isStatement() {}

// PagePlaceholderV3 is a `placeholder <Name> { … }` block: widgets bound to the
// layout placeholder named Name (issue #532). Widgets may include fragment-use
// sentinels, expanded by the executor.
type PagePlaceholderV3 struct {
	Name    string
	Widgets []*WidgetV3
}

// CreateSnippetStmtV3 represents a V3 snippet creation statement.
type CreateSnippetStmtV3 struct {
	Name             QualifiedName
	Parameters       []PageParameter // From Params: { } block
	Variables        []PageVariable  // From Variables: { } block
	Folder           string
	Widgets          []*WidgetV3
	Documentation    string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
	IsReplace        bool
	IsModify         bool
}

func (s *CreateSnippetStmtV3) isStatement() {}

// CreateLayoutStmt is `create layout Module.Name ( … ) { … }`.
//
// A layout is a page's frame. Properties carry LayoutType — which Mendix stores
// on the content wrapper rather than on the layout element — and which
// placeholder a page's content goes into.
type CreateLayoutStmt struct {
	Name             QualifiedName
	Properties       map[string]any
	Widgets          []*WidgetV3
	Documentation    string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
	IsReplace        bool
	IsModify         bool

	// BracedPlaceholders names the `placeholder X { … }` blocks written inside
	// this layout. A layout DECLARES a placeholder with the bodiless form; the
	// braced form is the page-side spelling that FILLS one, and the visitor
	// drops it from the widget tree. Recording the names here is what lets the
	// checker say so, instead of the write failing later with "declares no
	// placeholder" — which contradicts what the author wrote
	// (mendixlabs/mxcli#1063).
	BracedPlaceholders []string
}

func (s *CreateLayoutStmt) isStatement() {}

// WidgetV3 represents a V3 widget with explicit properties.
// Pattern: WIDGET name (Props) { children }
type WidgetV3 struct {
	Type       string         // Widget type: TEXTBOX, DATAVIEW, etc.
	Name       string         // Required widget name
	Properties map[string]any // All properties as key-value pairs
	Children   []*WidgetV3    // Child widgets

	// Specialization is the entity a List View template renders, set only for
	// `template for Module.Entity { ... }`. It is a separate field rather than a
	// Property because it identifies the template — a Forms$ListViewTemplate has
	// no name, and carries nothing but this entity and its widgets. Empty for
	// every other widget, including a Gallery's named `template <name>` slot.
	Specialization string

	// TypeIsGeneric records that Type came from the grammar's generic
	// IDENTIFIER alternative rather than one of the enumerated widget-type
	// tokens (slice 2 of PROPOSAL_def_driven_widget_bodies.md).
	//
	// The distinction is invisible in Type — both arrive as a lowercase string —
	// but it is what tells a typo from a built-in. `htmlelemnt` can ONLY be a
	// misspelt widget definition, because a real built-in has its own token; a
	// generic type that resolves to no definition is therefore MDL-WIDGET25
	// rather than a static widget to be validated on the builtin property
	// vocabulary. Without it the typo passes `check` with a warning about the
	// wrong thing.
	//
	// Set by the visitor from the parse tree, never inferred from a list of
	// known widget names — inferring it would reintroduce the list this
	// proposal exists to remove.
	TypeIsGeneric bool
}

// WidgetIcon is the value of a widget's `Icon:` property.
//
// It is a struct rather than a string because Mendix stores three different
// icon ELEMENTS and they are not variants of one value: an icon-collection icon
// and an image icon each hold a qualified name — into an icon collection and an
// image collection, which are different documents — while a glyph icon holds a
// numeric character code and no name at all. A single string could express only
// one of the three, which is how a stored image icon was rewritten as a
// custom-icon reference and a glyph icon was deleted outright
// (mendixlabs/mxcli#1059).
//
// The vocabulary is types.MenuIconKind, shared with navigation rather than
// restated: the kinds are Mendix's, not navigation's, and a second copy of the
// mapping is the drift this fix exists to remove.
type WidgetIcon struct {
	// Kind says which of the three elements to write. Never MenuIconNone on a
	// value the visitor produced — an absent `Icon:` has no WidgetIcon at all.
	Kind types.MenuIconKind
	// Name is the qualified name, for the collection and image kinds.
	Name string
	// Code is the glyph's numeric character code, set only for MenuIconGlyph.
	Code int
}

// ObjectEntryListV3 is `[(k: v, …), …]` written as a widget property VALUE —
// a repeatable widget property (FileUploader `allowedFileFormats`, HTML Element
// `attributes`) spelled the way a JSON author would reach for.
//
// It is NOT how MDL writes an object list; the entries are container blocks in
// the widget body. This type exists so the mistake can be REPORTED
// (MDL-WIDGET27) rather than mis-handled, and it is deliberately never given a
// write path: a second spelling for one construct is the anti-pattern the syntax
// design guide names, and it is what a reader would then have to learn twice.
//
// Before it existed the two shapes failed differently and both badly
// (mendixlabs/mxcli#999): the single-key form parsed as a list of expressions,
// checked clean, exec'd successfully and was silently discarded, while the
// multi-key form died as `missing ')' at ','`.
type ObjectEntryListV3 struct {
	// Entries preserves what was written, so the diagnostic can name the first
	// key the author used rather than describing the shape abstractly.
	Entries []map[string]any
}

// DataSourceV3 represents a V3 datasource expression.
type DataSourceV3 struct {
	Type            string          // "parameter", "database", "microflow", "nanoflow", "association", "selection"
	Reference       string          // Entity name, flow name, widget name, or parameter name
	ContextVariable string          // Context variable name (for association source: $currentObject → "currentObject")
	Args            []FlowArgV3     // Arguments for microflow/nanoflow calls
	Where           string          // XPath constraint (for database source)
	OrderBy         []OrderByItemV3 // Sort order (for database source)
}

// FlowArgV3 represents an argument for microflow/nanoflow/page calls.
type FlowArgV3 struct {
	Name  string // Parameter name
	Value any    // Value (expression)
}

// OrderByItemV3 represents a sort column.
type OrderByItemV3 struct {
	Attribute string // Attribute path
	Direction string // "ASC" or "DESC"
}

// ActionV3 represents a V3 action expression.
type ActionV3 struct {
	Type         string      // "save", "cancel", "close", "delete", "create", "showPage", "microflow", "nanoflow", "openLink", "signOut", "completeTask"
	Target       string      // Entity, page, or flow qualified name (for create/showPage/microflow/nanoflow)
	Args         []FlowArgV3 // Arguments for showPage/microflow calls
	ThenAction   *ActionV3   // For CREATE_OBJECT ... THEN ...
	ClosePage    bool        // For SAVE_CHANGES CLOSE_PAGE
	LinkURL      string      // For OPEN_LINK
	OutcomeValue string      // For COMPLETE_TASK
}

// ColumnV3 represents a V3 datagrid column.
type ColumnV3 struct {
	Name       string      // Column name
	Attr       string      // Bound attribute
	Caption    string      // Header caption
	CanSort    bool        // Sortable
	CanFilter  bool        // Filterable
	Children   []*WidgetV3 // For action columns or custom content
	Properties map[string]any
}

// ParamAssignmentV3 represents a template parameter: {1} = value
type ParamAssignmentV3 struct {
	Index  int            // Parameter index (1, 2, 3, ...)
	Value  any            // Expression value
	Format *ParamFormatV3 // Optional per-parameter formatting (dynamic text), else nil
}

// ParamFormatV3 holds the optional per-parameter formatting of a dynamic-text
// parameter, mapping to the Mendix ClientTemplateParameter FormattingInfo. Props
// preserve the raw key/value pairs exactly as written, so the validator can flag
// unknown keys / bad values and DESCRIBE can round-trip them.
//
//	{1} = Amount format (decimalPrecision: 2, groupDigits: true)
type ParamFormatV3 struct {
	Props []ParamFormatProp
}

// ParamFormatProp is one `key: value` entry inside a parameter format block.
type ParamFormatProp struct {
	Key   string // lowercased key, e.g. "decimalprecision"
	Value string // raw value text, with surrounding quotes stripped for strings
}

// Get returns the value for a (case-insensitive) key and whether it was present.
func (f *ParamFormatV3) Get(key string) (string, bool) {
	if f == nil {
		return "", false
	}
	for _, p := range f.Props {
		if p.Key == key {
			return p.Value, true
		}
	}
	return "", false
}

// DesignPropertyEntryV3 represents a single design property entry. It is either
// flat (Value set) or compound (Nested set) — a compound property's value is
// itself a list of sub-properties, e.g. 'Spacing': ['margin-top': 'Large', …].
type DesignPropertyEntryV3 struct {
	Key    string                  // e.g., "Spacing top" or "Spacing"
	Value  string                  // flat value: "Large", "on", "off" (empty when Nested is set)
	Nested []DesignPropertyEntryV3 // compound sub-properties (empty for a flat property)
}

// Helper functions to extract typed properties from WidgetV3

// GetStringProp returns a string property or empty string if not found.
// MDL property names are case-insensitive, so a generic property stored under
// the user's original casing (e.g. `dynamicclasses`) still resolves for a
// canonical-case lookup (`DynamicClasses`). Exact match is tried first; the
// case-insensitive scan mirrors lookupProperty in the widget engine. Bug 10b.
func (w *WidgetV3) GetStringProp(key string) string {
	if v, ok := w.Properties[key].(string); ok {
		return v
	}
	lower := strings.ToLower(key)
	for k, v := range w.Properties {
		if strings.ToLower(k) == lower {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// GetIntProp returns an int property or 0 if not found. Case-insensitive on the
// same grounds as GetStringProp: the property is stored under whatever casing
// the script used, so an exact-match-only lookup silently reads 0 for `size:`
// when the caller asks for "Size".
func (w *WidgetV3) GetIntProp(key string) int {
	if n, ok := toInt(w.Properties[key]); ok {
		return n
	}
	lower := strings.ToLower(key)
	for k, v := range w.Properties {
		if strings.ToLower(k) == lower {
			if n, ok := toInt(v); ok {
				return n
			}
		}
	}
	return 0
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// GetBoolProp returns a bool property or false if not found.
func (w *WidgetV3) GetBoolProp(key string) bool {
	if v, ok := w.Properties[key].(bool); ok {
		return v
	}
	return false
}

// GetDataSource returns the DataSource property or nil if not found.
func (w *WidgetV3) GetDataSource() *DataSourceV3 {
	if v, ok := w.Properties["DataSource"].(*DataSourceV3); ok {
		return v
	}
	return nil
}

// GetAction returns the Action property or nil if not found.
func (w *WidgetV3) GetAction() *ActionV3 {
	if v, ok := w.Properties["Action"].(*ActionV3); ok {
		return v
	}
	return nil
}

// GetOnChange returns the OnChange action (input widgets) or nil.
func (w *WidgetV3) GetOnChange() *ActionV3 {
	if v, ok := w.Properties["OnChange"].(*ActionV3); ok {
		return v
	}
	return nil
}

// GetPlaceholder returns the Placeholder text (input widgets) or empty string.
func (w *WidgetV3) GetPlaceholder() string {
	return w.GetStringProp("Placeholder")
}

// GetAttribute returns the Attribute property (attribute path) or empty string.
func (w *WidgetV3) GetAttribute() string {
	return w.GetStringProp("Attribute")
}

// GetBinds returns the Binds property (attribute path) or empty string.
// Deprecated: use GetAttribute instead.
func (w *WidgetV3) GetBinds() string {
	if v, ok := w.Properties["Binds"].(string); ok {
		return v
	}
	return ""
}

// GetLabel returns the Label property or empty string.
func (w *WidgetV3) GetLabel() string {
	return w.GetStringProp("Label")
}

// GetCaption returns the Caption property or empty string.
func (w *WidgetV3) GetCaption() string {
	return w.GetStringProp("Caption")
}

// GetContent returns the Content property or empty string.
func (w *WidgetV3) GetContent() string {
	return w.GetStringProp("Content")
}

// GetRenderMode returns the RenderMode property or empty string.
func (w *WidgetV3) GetRenderMode() string {
	return w.GetStringProp("RenderMode")
}

// GetButtonStyle returns the ButtonStyle property or empty string.
func (w *WidgetV3) GetButtonStyle() string {
	return w.GetStringProp("ButtonStyle")
}

// GetClass returns the Class property or empty string.
func (w *WidgetV3) GetClass() string {
	return w.GetStringProp("Class")
}

// GetStyle returns the Style property or empty string.
func (w *WidgetV3) GetStyle() string {
	return w.GetStringProp("Style")
}

// GetDynamicClasses returns the DynamicClasses expression or empty string.
func (w *WidgetV3) GetDynamicClasses() string {
	return w.GetStringProp("DynamicClasses")
}

// GetDesktopWidth returns the DesktopWidth property.
func (w *WidgetV3) GetDesktopWidth() any {
	return w.Properties["DesktopWidth"]
}

// GetContentParams returns ContentParams or nil.
func (w *WidgetV3) GetContentParams() []ParamAssignmentV3 {
	if v, ok := w.Properties["ContentParams"].([]ParamAssignmentV3); ok {
		return v
	}
	return nil
}

// GetCaptionParams returns CaptionParams or nil.
func (w *WidgetV3) GetCaptionParams() []ParamAssignmentV3 {
	if v, ok := w.Properties["CaptionParams"].([]ParamAssignmentV3); ok {
		return v
	}
	return nil
}

// GetAttributes returns the Attributes property as a string slice (for filter widgets).
func (w *WidgetV3) GetAttributes() []string {
	if v, ok := w.Properties["Attributes"].([]string); ok {
		return v
	}
	return nil
}

// GetFilterType returns the FilterType property (for filter widgets).
func (w *WidgetV3) GetFilterType() string {
	return w.GetStringProp("FilterType")
}

// GetAttr returns the Attr property (for COLUMN widgets) or empty string.
func (w *WidgetV3) GetAttr() string {
	return w.GetStringProp("Attr")
}

// GetSnippet returns the Snippet property (qualified name) or empty string.
func (w *WidgetV3) GetSnippet() string {
	return w.GetStringProp("Snippet")
}

// SnippetCallParam represents one parameter mapping in a SNIPPETCALL Params: block.
type SnippetCallParam struct {
	ParamName string // Parameter name as written (may include leading $)
	Variable  string // Variable being passed, always includes leading $
}

// GetSnippetParams returns the Params mappings for a SNIPPETCALL widget, or nil.
func (w *WidgetV3) GetSnippetParams() []SnippetCallParam {
	if v, ok := w.Properties["Params"].([]SnippetCallParam); ok {
		return v
	}
	return nil
}

// GetSelection returns the Selection mode or empty string.
func (w *WidgetV3) GetSelection() string {
	return w.GetStringProp("Selection")
}

// GetDesignProperties returns the DesignProperties or nil.
func (w *WidgetV3) GetDesignProperties() []DesignPropertyEntryV3 {
	if v, ok := w.Properties["DesignProperties"].([]DesignPropertyEntryV3); ok {
		return v
	}
	return nil
}
