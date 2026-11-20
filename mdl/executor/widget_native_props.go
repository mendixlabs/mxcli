// SPDX-License-Identifier: Apache-2.0

// The properties each built-in page element takes (ako/mxcli#1698).
//
// `textbox tb (Attribute: Name, Caption: 'Item name')` passed check and exec and
// wrote a text box with no label: a text box takes `Label:`, and `Caption:` is
// a button's. MDL-WIDGET07 judged the key against a vocabulary that is the
// UNION over every built-in widget, so a key valid on some widget passed on all
// of them. That union is how #1307 (an action button's Tooltip), #1465 (a tab
// page's CaptionParams), #1582 (a container's RenderMode) and #1643 each
// reached a user before anyone saw them.
//
// This file replaces the union with one set per element kind: what that kind's
// builder reads off the element, plus what buildWidgetV3's common tail writes
// for it (appearance for every widget, visibility only where the stored $Type
// has ConditionalVisibilitySettings). nativeWidgetPropRefusals is the single
// verdict; check reports it (validateNativeWidgetProps) and exec refuses on it
// (refuseNativeWidgetProps), so the two cannot disagree.
//
// The sets are not a promise kept by hand. TestNativeWidgetProps_MatchBuilderReads
// derives, from the builders' source, every key each one reads off its element
// and fails unless that is exactly the kind's `reads`; TestNativeWidgetProps_TailMatchesGen
// ties `visible` to whether the gen type the writer encodes has a
// ConditionalVisibilitySettings setter; TestNativeWidgetProps_DescribeEmitsOnlyAccepted
// holds describe to the same sets, so describe → exec never refuses its own output.
//
// It is a new rejection, so per ADR-0011 it applies under `mdl 1;`; a script
// without the header keeps the old outcome — the key is dropped — and is told
// so with MDL-V1-WIDGETPROP. Same shape as MDL-V1-TABPAGEPROP (#1465).
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

var nativeWidgetPropDropped = langver.Change{
	Code:  "MDL-V1-WIDGETPROP",
	Since: langver.V1,
	Old: "A property a built-in page element does not take (`Caption:` on a textbox, `Label:` on a button, " +
		"a misspelt key) is accepted and dropped on write",
	New: "a refusal (MDL-WIDGET52), with nothing written",
}

// nativeWidgetKind is one built-in page element: a widget keyword, or a child
// element its parent's builder reads directly (a scroll container's region, a
// layout grid's row).
type nativeWidgetKind struct {
	// stored is the $Type the element is written as.
	stored string
	// reads are the keys its builder reads off the element — exactly, see
	// TestNativeWidgetProps_MatchBuilderReads.
	reads []string
	// tail: built by buildWidgetV3, whose common tail writes Class, Style,
	// DynamicClasses and DesignProperties (every widget has an Appearance).
	tail bool
	// visible: the tail also writes Visible — the stored type has
	// ConditionalVisibilitySettings. A scroll container, placeholder, menu
	// widget or snippet call does not, so `Visible:` on one was dropped.
	visible bool
}

var (
	appearanceKeys = []string{"Class", "Style", "DynamicClasses", "DesignProperties"}
	// The visitor lowers `Visible:` by shape: an expression to VisibleIf, a
	// plain value to Visible, `Attr in (…)` to VisibleWhen, `roles (…)` to
	// VisibleRoles. One property, four keys.
	visibilityKeys = []string{"Visible", "VisibleIf", "VisibleWhen", "VisibleRoles"}
)

// inputReads are what every attribute-bound input's builder reads. OnEnter /
// OnLeave (and their stored names) are applyInputFocusEventsV3's.
var inputReads = []string{"Attribute", "Label", "ShowLabel", "OnChange", "OnEnter", "OnEnterAction", "OnLeave", "OnLeaveAction"}

func withReads(base []string, more ...string) []string {
	return append(append([]string(nil), base...), more...)
}

// nativeWidgetKinds is keyed by kind: the lower-case keyword, or "<parent>
// <child>" for an element its parent builds (nativeWidgetKindOf).
var nativeWidgetKinds = map[string]nativeWidgetKind{
	// Data and list widgets. `Footer` is the legacy property form of a data
	// view's footer block, still read by buildDataViewV3.
	"dataview": {stored: "Forms$DataView", tail: true, visible: true,
		reads: []string{"DataSource", "FormOrientation", "LabelWidth", "ReadOnlyStyle", "ShowFooter", "Footer", "NoEntityMessage"}},
	"listview": {stored: "Forms$ListView", tail: true, visible: true,
		reads: []string{"DataSource", "Action", "Editable", "PageSize"}},

	// Inputs.
	"textbox": {stored: "Forms$TextBox", tail: true, visible: true,
		reads: withReads(inputReads, "Placeholder", "ReadOnlyStyle", "Password", "Validation", "ValidationMessage",
			"SubmitBehaviour", "SubmitOnInputDelay", "KeyboardType", "OnEnterKeyPress", "OnEnterKeyPressAction")},
	"textarea": {stored: "Forms$TextArea", tail: true, visible: true,
		reads: withReads(inputReads, "Placeholder", "ReadOnlyStyle", "NumberOfLines", "Autocomplete",
			"SubmitBehaviour", "SubmitOnInputDelay")},
	"datepicker": {stored: "Forms$DatePicker", tail: true, visible: true,
		reads: withReads(inputReads, "ReadOnlyStyle")},
	"dropdown":     {stored: "Forms$DropDown", tail: true, visible: true, reads: withReads(inputReads)},
	"checkbox":     {stored: "Forms$CheckBox", tail: true, visible: true, reads: withReads(inputReads, "ReadOnlyStyle", "LabelPosition")},
	"radiobuttons": {stored: "Forms$RadioButtonGroup", tail: true, visible: true, reads: withReads(inputReads, "ReadOnlyStyle", "RenderHorizontal")},
	"filemanager": {stored: "Forms$FileManager", tail: true, visible: true,
		reads: []string{"Label", "ShowLabel", "Type", "MaxFileSize", "AllowedExtensions", "ShowFileInBrowser"}},

	// Text.
	"dynamictext": {stored: "Forms$DynamicText", tail: true, visible: true,
		reads: []string{"Content", "ContentParams", "Attribute", "RenderMode", "NativeTextStyle"}},
	"title": {stored: "Forms$Title", tail: true, visible: true, reads: []string{"Content"}},
	"label": {stored: "Forms$Label", tail: true, visible: true, reads: []string{"Content"}},

	// Buttons. `ContentParams:` is what describe printed for a button's caption
	// parameters before #632, so it is still read.
	"actionbutton": {stored: "Forms$ActionButton", tail: true, visible: true, reads: buttonReads},
	"button":       {stored: "Forms$ActionButton", tail: true, visible: true, reads: buttonReads},
	"linkbutton":   {stored: "Forms$ActionButton", tail: true, visible: true, reads: buttonReads},
	"sidebartoggle": {stored: "Forms$SidebarToggleButton", tail: true, visible: true,
		reads: []string{"Caption", "CaptionParams", "ContentParams", "Tooltip", "ButtonStyle", "Icon", "RenderMode"}},

	// Structure.
	"container":       {stored: "Forms$DivContainer", tail: true, visible: true, reads: []string{"Action", "RenderMode"}},
	"customcontainer": {stored: "Forms$DivContainer", tail: true, visible: true, reads: []string{"Action", "RenderMode"}},
	"groupbox": {stored: "Forms$GroupBox", tail: true, visible: true,
		reads: []string{"Caption", "Collapsible", "HeaderMode"}},
	"tabcontainer": {stored: "Forms$TabControl", tail: true, visible: true},
	"layoutgrid":   {stored: "Forms$LayoutGrid", tail: true, visible: true, reads: []string{"Width"}},
	// A top-level `row` / `column` is a container holding a one-row layout
	// grid: the tail writes the container, the row's or column's own builder
	// reads the rest.
	"row": {stored: "Forms$DivContainer", tail: true, visible: true, reads: layoutGridRowReads},
	"column": {stored: "Forms$DivContainer", tail: true, visible: true,
		reads: withReads(appearanceKeys, "VerticalAlignment", "DesktopWidth", "TabletWidth", "PhoneWidth")},
	"navigationlist": {stored: "Forms$NavigationList", tail: true, visible: true},
	"snippetcall":    {stored: "Forms$SnippetCallWidget", tail: true, reads: []string{"Snippet", "Params"}},
	// Containers a list or data view's body is split into.
	"footer":     {stored: "Forms$DivContainer", tail: true, visible: true},
	"header":     {stored: "Forms$DivContainer", tail: true, visible: true},
	"controlbar": {stored: "Forms$DivContainer", tail: true, visible: true},
	"template":   {stored: "Forms$DivContainer", tail: true, visible: true},
	"filter":     {stored: "Forms$DivContainer", tail: true, visible: true},

	// Images.
	"staticimage": {stored: "Forms$StaticImageViewer", tail: true, visible: true,
		reads: []string{"Image", "Action", "Width", "Height", "WidthUnit", "HeightUnit", "Responsive"}},
	"dynamicimage": {stored: "Forms$ImageViewer", tail: true, visible: true,
		reads: []string{"DataSource", "DefaultImage", "DisplayAs", "OnClickType", "Action",
			"Width", "Height", "WidthUnit", "HeightUnit", "Responsive"}},

	// Layout elements.
	"scrollcontainer": {stored: "Forms$ScrollContainer", tail: true, reads: []string{"LayoutMode"}},
	"layoutheader":    {stored: "Forms$Header", tail: true},
	"placeholder":     {stored: "Forms$Placeholder", tail: true},
	"navigationtree":  {stored: "Forms$NavigationTree", tail: true, reads: []string{"Menu", "Profile"}},
	"menubar":         {stored: "Forms$MenuBar", tail: true, reads: []string{"Menu", "Profile"}},
	"simplemenubar":   {stored: "Forms$SimpleMenuBar", tail: true, reads: []string{"Menu", "Profile", "Orientation"}},

	// Elements a parent builds itself, so buildWidgetV3's tail never runs.
	"scrollcontainer region": {stored: "Forms$ScrollContainerRegion",
		reads: []string{"Class", "Size", "SizeMode", "ToggleMode"}},
	"layoutheader region": {stored: "Forms$Header"},
	"layoutgrid row":      {stored: "Forms$LayoutGridRow", reads: layoutGridRowReads},
	"layoutgrid column": {stored: "Forms$LayoutGridColumn",
		reads: withReads(appearanceKeys, "VerticalAlignment", "DesktopWidth", "TabletWidth", "PhoneWidth")},
	"navigationlist item": {stored: "Forms$NavigationListItem",
		reads: []string{"Caption", "Action", "Class", "Style", "DynamicClasses", "Visible", "VisibleIf", "VisibleWhen", "VisibleRoles"}},
	"dataview footer":   {stored: "Forms$DataView"},
	"listview template": {stored: "Forms$ListViewTemplate"},
}

var buttonReads = []string{"Caption", "CaptionParams", "ContentParams", "Tooltip", "ButtonStyle", "Icon", "Action"}

var layoutGridRowReads = withReads(appearanceKeys, visibilityKeys[0], visibilityKeys[1], visibilityKeys[2], visibilityKeys[3],
	"VerticalAlignment", "HorizontalAlignment", "SpacingBetweenColumns")

// nativeChildKinds are the elements a parent's builder reads itself, keyed by
// "<parent keyword> <child keyword>".
var nativeChildKinds = map[string]string{
	"scrollcontainer region": "scrollcontainer region",
	"layoutheader region":    "layoutheader region",
	"layoutgrid row":         "layoutgrid row",
	"row column":             "layoutgrid column",
	"navigationlist item":    "navigationlist item",
	"dataview footer":        "dataview footer",
}

// nativeWidgetKindOf is the kind w is built as when it sits under parent (nil
// at the document root), or "" when it is not a built-in element mxcli builds
// — a pluggable widget, a fragment sentinel, a keyword with no builder.
func nativeWidgetKindOf(w, parent *ast.WidgetV3) string {
	if w == nil {
		return ""
	}
	t := strings.ToLower(w.Type)
	if parent != nil {
		p := strings.ToLower(parent.Type)
		if k, ok := nativeChildKinds[p+" "+t]; ok {
			return k
		}
		// `template for Module.Entity { … }` is a list view's specialization.
		if p == "listview" && w.Specialization != "" {
			return "listview template"
		}
	}
	if _, ok := nativeWidgetKinds[t]; ok {
		return t
	}
	return ""
}

// nativeElementKeywords are the keywords that name a built-in element in some
// position: every kind's keyword and every child a native parent builds.
var nativeElementKeywords = func() map[string]bool {
	m := map[string]bool{"tabpage": true, "region": true, "item": true}
	for k := range nativeWidgetKinds {
		if !strings.Contains(k, " ") {
			m[k] = true
		}
	}
	return m
}()

// nativePositionalKeywords are keywords whose meaning depends on the parent: a
// `column` is a layout-grid column under a row, a data grid's column under a
// data grid, and a container shorthand elsewhere; a `template` is a list
// view's specialization, a gallery's item slot or a container.
var nativePositionalKeywords = map[string]bool{
	"column": true, "row": true, "item": true, "region": true,
	"template": true, "filter": true, "header": true, "footer": true, "controlbar": true,
}

// nativeWidgetKindInPosition is nativeWidgetKindOf for the check-time walk.
// A positional keyword is judged only where its parent is known and is a
// built-in element: under a pluggable widget or an object-list item it is that
// widget's column, item or slot, which the engine reads and this file does not
// judge; at the root of an ALTER … INSERT (enclosureKnown false) the parent is
// out of sight.
func nativeWidgetKindInPosition(w, parent *ast.WidgetV3, parentItem *ObjectListMapping, registry *WidgetRegistry, enclosureKnown bool) string {
	if w == nil {
		return ""
	}
	if nativePositionalKeywords[strings.ToLower(w.Type)] {
		if parent == nil && !enclosureKnown {
			return ""
		}
		if parent != nil && !isNativeParent(parent, parentItem, registry) {
			return ""
		}
	}
	return nativeWidgetKindOf(w, parent)
}

// isNativeParent reports whether parent is a built-in element, whose children
// buildWidgetV3 or the parent's own builder reads — not a pluggable widget, an
// object-list item, or a keyword with no builder.
func isNativeParent(parent *ast.WidgetV3, parentItem *ObjectListMapping, registry *WidgetRegistry) bool {
	if parentItem != nil || parent.TypeIsGeneric || explicitWidgetID(parent) != "" ||
		isUniversalObjectListKeyword(parent.Type) || lookupWidgetDef(parent, registry) != nil {
		return false
	}
	return nativeElementKeywords[strings.ToLower(parent.Type)]
}

// nativeWidgetAccepts reports whether a kind takes key, case-insensitively.
func nativeWidgetAccepts(kind, keyword, key string) bool {
	k, ok := nativeWidgetKinds[kind]
	if !ok {
		return true
	}
	for _, set := range nativeWidgetKeySets(k) {
		for _, a := range set {
			if strings.EqualFold(a, key) {
				return true
			}
		}
	}
	// The table-driven reads: a date picker's or text box's FormattingInfo,
	// a text box's or text area's submit settings. Their builders read the
	// keys from those tables, not by literal.
	if kind == keyword {
		if _, ok := inputFormatProp(keyword, key); ok {
			return true
		}
	}
	// A spelling of a key this kind takes, kept as written because its value
	// did not parse (nativeWidgetKeyAliases).
	if target, ok := nativeWidgetKeyAliases[strings.ToLower(key)]; ok {
		return nativeWidgetAccepts(kind, keyword, target)
	}
	return false
}

// nativeWidgetKeyAliases are spellings the visitor stores under another key —
// but only when the value is well formed. `onClick: close page` is stored as
// "Action" (#603, actionKeywordSource); `onClick: TOTALLY_MADE_UP` does not
// parse as an action, falls through to the generic `key: value` branch and
// keeps its key as written. That is not a property the widget lacks: it is
// the widget's click action holding something that is not an action, which
// MDL-WIDGET28 reports. Refusing it here as well named the wrong problem, and —
// this rule being gated on the header and that one not — made `mdl 1;` change
// what the script writes (ako/mxcli#1698, the upgrade property test on
// action-slot-not-an-action-1062.fail.mdl). Keyed lower-case: the alias, and
// the key it is stored under when it parses. An alias is not a builder read;
// TestNativeWidgetProps_AliasesFollowTheVisitor ties it to actionKeywordSource.
//
// The visitor renames no other key this way: every other dedicated branch
// stores under the key's own name (case aside, which nativeWidgetAccepts
// ignores), and `Visible:` / `Editable:` lower to keys judged as one group.
var nativeWidgetKeyAliases = map[string]string{"onclick": "Action"}

func nativeWidgetKeySets(k nativeWidgetKind) [][]string {
	sets := [][]string{k.reads}
	if k.tail {
		sets = append(sets, appearanceKeys)
		if k.visible {
			sets = append(sets, visibilityKeys)
		}
	}
	return sets
}

// nativeWidgetAcceptedNames lists a kind's keys for messages, sorted and
// without the internal spellings of Visible.
func nativeWidgetAcceptedNames(kind, keyword string) []string {
	k := nativeWidgetKinds[kind]
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if l := strings.ToLower(n); !seen[l] {
			seen[l] = true
			out = append(out, n)
		}
	}
	for _, set := range nativeWidgetKeySets(k) {
		for _, n := range set {
			switch n {
			case "VisibleIf", "VisibleWhen", "VisibleRoles", "Footer":
				continue // spellings of Visible, and a legacy form
			}
			if isInputEventStoredName(n) {
				continue // the stored name of an event action; its MDL name is listed
			}
			add(n)
		}
	}
	if kind == keyword {
		for _, n := range inputFormatProps[keyword] {
			add(n)
		}
		// Judged by MDL-WIDGET20 rather than by the set, but taken all the same.
		// (plainEditableWidgetTypes' `column` is a data grid's, not this kind.)
		if editableWidgetTypes[keyword] || keyword == "listview" {
			add("Editable")
		}
	}
	sort.Strings(out)
	return out
}

// nativeWidgetKeyOwnedElsewhere reports keys a narrower rule already judges on
// this kind, so they are not reported twice: editability (MDL-WIDGET20), a
// click action on a widget Mendix gives none (MDL-WIDGET23), a snippet call's
// appearance (#1511), a layout-grid column's Visible (#1461) and a dynamic
// text's per-parameter format keys (MDL-WIDGET18).
func nativeWidgetKeyOwnedElsewhere(kind, keyword, key string) bool {
	l := strings.ToLower(key)
	switch l {
	case "editable", "editableif", "editablewhen":
		return true
	case "action", "onclick":
		if clickDroppedNoSlot[keyword] || clickCapableInMendix[keyword] {
			return true
		}
	}
	switch kind {
	case "snippetcall":
		for _, a := range appearanceKeys {
			if strings.EqualFold(a, key) {
				return true
			}
		}
	case "layoutgrid column":
		for _, v := range visibilityKeys {
			if strings.EqualFold(v, key) {
				return true
			}
		}
	case "dynamictext":
		return paramFormatKeys[l]
	}
	return false
}

// nativeWidgetPropRefusal is one key a built-in element does not take.
type nativeWidgetPropRefusal struct {
	Key  string
	Hint string // a cross-widget mix-up or a "did you mean", "" when neither
}

// nativeWidgetPropRefusals is THE verdict: the keys on w, built as kind, that
// nothing writes. Check and exec both call it.
func nativeWidgetPropRefusals(kind string, w *ast.WidgetV3) []nativeWidgetPropRefusal {
	if w == nil || kind == "" {
		return nil
	}
	if _, ok := nativeWidgetKinds[kind]; !ok {
		return nil
	}
	keyword := strings.ToLower(w.Type)
	var out []nativeWidgetPropRefusal
	for _, key := range sortedPropertyKeys(w) {
		if nativeWidgetAccepts(kind, keyword, key) || nativeWidgetKeyOwnedElsewhere(kind, keyword, key) {
			continue
		}
		out = append(out, nativeWidgetPropRefusal{Key: key, Hint: nativeWidgetPropHint(kind, keyword, key)})
	}
	return out
}

// Kinds by what they are, for the cross-widget hints.
var (
	nativeInputKinds  = map[string]bool{"textbox": true, "textarea": true, "datepicker": true, "dropdown": true, "checkbox": true, "radiobuttons": true, "filemanager": true}
	nativeButtonKinds = map[string]bool{"actionbutton": true, "button": true, "linkbutton": true, "sidebartoggle": true}
	nativeTextKinds   = map[string]bool{"dynamictext": true, "title": true, "label": true}
)

// nativeWidgetPropHint names what the author most likely meant: the property
// this kind uses for the thing the key means on another widget, or the
// nearest key it takes.
func nativeWidgetPropHint(kind, keyword, key string) string {
	if h := inputEventHint(kind, key); h != "" {
		return h
	}
	switch strings.ToLower(key) {
	case "caption", "captionparams":
		switch {
		case nativeInputKinds[kind]:
			return "an input widget's label is `Label:` (`Caption:` is a button's and a column's)"
		case nativeTextKinds[kind]:
			return "its text is `Content:` (with `ContentParams:`)"
		}
	case "label":
		switch {
		case nativeButtonKinds[kind]:
			return "a button's text is `Caption:` (`Label:` is an input widget's)"
		case kind == "groupbox" || kind == "navigationlist item":
			return "its text is `Caption:`"
		case nativeTextKinds[kind]:
			return "its text is `Content:`"
		}
	case "content", "contentparams":
		switch {
		case nativeInputKinds[kind]:
			return "an input widget's label is `Label:`"
		case kind == "groupbox" || kind == "navigationlist item":
			return "its text is `Caption:`"
		}
	case "tooltip":
		return "only a button stores a tooltip"
	case "action", "onclick":
		return fmt.Sprintf("%s has no click action — put it on a `container` inside it, or use an `actionbutton`",
			nativeWidgetKinds[kind].stored)
	case "onchange":
		return "only an input widget has an on-change action"
	case "datasource":
		return "put the widget inside a `dataview` with that data source"
	case "visible", "visibleif", "visiblewhen", "visibleroles":
		k := nativeWidgetKinds[kind]
		return fmt.Sprintf("%s has no conditional visibility — wrap it in a `container` with Visible:", k.stored)
	case "class", "style", "dynamicclasses", "designproperties":
		if nativeWidgetAccepts(kind, keyword, "Class") {
			return "of its appearance only `Class:` is written"
		}
		return "none of its appearance is written — style a widget inside it"
	}
	if near := nearestKey(key, nativeWidgetAcceptedNames(kind, keyword)); near != "" {
		return fmt.Sprintf("did you mean `%s`?", near)
	}
	return ""
}

// nativeWidgetPropMessage is the shared wording of a refusal.
func nativeWidgetPropMessage(kind string, w *ast.WidgetV3, r nativeWidgetPropRefusal) string {
	what := strings.ToLower(w.Type)
	if strings.Contains(kind, " ") {
		what = kind
	}
	name := ""
	if w.Name != "" {
		name = " `" + w.Name + "`"
	}
	msg := fmt.Sprintf("%s%s: `%s` is not a property mxcli writes on a %s (%s), so it would be dropped",
		what, name, r.Key, what, nativeWidgetKinds[kind].stored)
	if r.Hint != "" {
		msg += " — " + r.Hint
	}
	return msg
}

func nativeWidgetPropSuggestion(kind string, w *ast.WidgetV3) string {
	names := nativeWidgetAcceptedNames(kind, strings.ToLower(w.Type))
	if len(names) == 0 {
		return "It takes no properties: remove them."
	}
	return "Properties it takes: " + strings.Join(names, ", ") + "."
}

// validateNativeWidgetProps is the check-time half (MDL-V1-WIDGETPROP, gated by
// GateWidgetViolations to MDL-WIDGET52 under mdl 1).
func validateNativeWidgetProps(kind string, w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, r := range nativeWidgetPropRefusals(kind, w) {
		out = append(out, linter.Violation{
			RuleID:     nativeWidgetPropDropped.Code,
			Severity:   linter.SeverityError,
			Message:    fmt.Sprintf("%s: %s", locationPrefix, nativeWidgetPropMessage(kind, w, r)),
			Suggestion: nativeWidgetPropSuggestion(kind, w),
		})
	}
	return out
}

// validateNativeWidgetPropsTree applies validateNativeWidgetProps to a widget
// tree that no other widget rule walks — a layout's. Pluggable widgets and
// their object-list items are skipped, as validateWidgetTreeIn skips them.
func validateNativeWidgetPropsTree(widgets []*ast.WidgetV3, parent *ast.WidgetV3, parentItem *ObjectListMapping,
	parentLists map[string]*ObjectListMapping, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		mapping := parentLists[strings.ToUpper(w.Type)]
		def := lookupWidgetDef(w, registry)
		if mapping == nil && def == nil && !w.TypeIsGeneric && explicitWidgetID(w) == "" && !isUniversalObjectListKeyword(w.Type) {
			kind := nativeWidgetKindInPosition(w, parent, parentItem, registry, true)
			out = append(out, validateNativeWidgetProps(kind, w, locationPrefix)...)
		}
		out = append(out, validateNativeWidgetPropsTree(w.Children, w, mapping, objectListMappingSet(def), registry, locationPrefix)...)
	}
	return out
}

// refuseNativeWidgetProps is the exec-time half, for `exec --no-check` and the
// paths no check pass sees: an error under mdl 1, a warning under mdl 0 (the
// keys are then dropped, as they always were).
func refuseNativeWidgetProps(ctx *ExecContext, kind string, w *ast.WidgetV3) error {
	refusals := nativeWidgetPropRefusals(kind, w)
	if len(refusals) == 0 {
		return nil
	}
	version := langver.V0
	if ctx != nil {
		version = ctx.LanguageVersion
	}
	if nativeWidgetPropDropped.Applies(version) {
		return mdlerrors.NewValidationf("%s, so nothing was written [MDL-WIDGET52]. %s",
			nativeWidgetPropMessage(kind, w, refusals[0]), nativeWidgetPropSuggestion(kind, w))
	}
	if ctx != nil {
		for _, r := range refusals {
			fmt.Fprintf(ctx.progress(), "Warning [%s]: %s; not written. %s\n",
				nativeWidgetPropDropped.Code, nativeWidgetPropMessage(kind, w, r), nativeWidgetPropDropped.Warning(version))
		}
	}
	return nil
}
