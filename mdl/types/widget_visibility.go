// SPDX-License-Identifier: Apache-2.0

package types

import "strings"

// WidgetVisibilityRule declares that a pluggable widget property is hidden
// under certain configurations of the same widget. Pluggable widgets express
// this in their compiled editorConfig.js via Mendix's hidePropertyIn /
// hidePropertiesIn helpers; this struct is the structured form mxcli stores in
// a widget's .def.json and evaluates at BSON serialization time.
//
// When a TextTemplate-typed property is hidden, Studio Pro nulls its
// TextTemplate; emitting the template's populated default instead triggers
// CE0463 ("the definition of this widget has changed"). See issue #574.
type WidgetVisibilityRule struct {
	PropertyKey string `json:"propertyKey"`
	// ListPropertyKey names the object-list property whose ITEMS carry
	// PropertyKey (e.g. an Accordion group's `initialCollapsedState` lives under
	// list `groups`). Empty for a rule about a top-level widget property.
	//
	// Nested rules are evaluated per list item, so consumers that only understand
	// top-level properties MUST skip a rule with a non-empty ListPropertyKey —
	// its PropertyKey does not name a property of the widget itself.
	ListPropertyKey string                     `json:"listPropertyKey,omitempty"`
	HiddenWhen      *WidgetVisibilityCondition `json:"hiddenWhen,omitempty"`
	// And carries the REST of a conjunction whose first term is HiddenWhen. A
	// widget's editorConfig nests its branches — Combo box reaches
	// `source=="context" && optionsSourceType=="association" && showFooter==false`
	// three levels deep — and a rule that keeps only the innermost term claims
	// hidden in configurations the editor shows. Every term must hold.
	//
	// Consumers MUST evaluate these; Conditions() and Fires() exist so they do
	// not have to remember. A reader that looks only at HiddenWhen silently
	// over-fires, which is why the extractor previously refused to lift such a
	// rule at all rather than store a partial one.
	And []WidgetVisibilityCondition `json:"and,omitempty"`
}

// Conditions returns every condition that must hold for the rule to fire.
func (r WidgetVisibilityRule) Conditions() []WidgetVisibilityCondition {
	if r.HiddenWhen == nil {
		return nil
	}
	out := make([]WidgetVisibilityCondition, 0, 1+len(r.And))
	out = append(out, *r.HiddenWhen)
	return append(out, r.And...)
}

// Fires reports whether the rule hides its property, given a lookup of the
// current value of a condition's property in a given scope.
//
// determinable is false when ANY condition's property has no known value — a
// conjunction is only as decidable as its least decidable term, and guessing
// one would resurrect exactly the over-firing this type exists to prevent.
// Callers treat indeterminable as "not hidden" and keep asking for the binding.
func (r WidgetVisibilityRule) Fires(lookup func(c WidgetVisibilityCondition) (string, bool)) (fires, determinable bool) {
	conds := r.Conditions()
	if len(conds) == 0 {
		return false, false
	}
	all := true
	for _, c := range conds {
		switch c.Operator {
		case OperatorAlways:
			continue
		case OperatorNever:
			all = false
			continue
		}
		v, ok := lookup(c)
		if !ok {
			return false, false
		}
		if !c.Hidden(map[string]string{c.PropertyKey: v}) {
			all = false
		}
	}
	return all, true
}

// Nested reports whether the rule targets an item sub-property of an object list
// rather than a top-level widget property.
func (r WidgetVisibilityRule) Nested() bool { return r.ListPropertyKey != "" }

// WidgetVisibilityCondition is a single predicate evaluated against the
// widget's current property values. Operators cover the dominant patterns
// observed in marketplace editorConfig.js files:
//
//	eq       — the named property equals Value
//	ne       — the named property differs from Value
//	truthy   — the named property is set / non-empty / not "false"/"0"
//	falsy    — the named property is unset / empty / "false" / "0"
//	empty    — the named property is the empty string
//	notempty — the named property is anything but the empty string
//	in       — the named property is one of Value's comma-separated entries
//	notin    — the named property is none of them
//
// `empty` is NOT `falsy`: editorConfig writes `null===e.someDataSource` and
// `0===e.someList.length` for "the author has not picked one", which is a
// narrower claim than falsy — "false" and "0" are real values a boolean or a
// number property can hold, and treating them as unset hides a property the
// author is actively using. Keeping the two apart is what lets `empty` fire on
// a datasource without also firing on `false`.
//
// Conditions that don't fit (composite logic, runtime data lookups) are left
// unset, which evaluates to "not hidden" so serialization falls back to the
// template default.
type WidgetVisibilityCondition struct {
	PropertyKey string `json:"propertyKey"`
	Operator    string `json:"operator"`
	Value       string `json:"value,omitempty"`
	// Scope says which object PropertyKey belongs to. "" (the default) means the
	// widget itself; ConditionScopeItem means the sibling sub-property of the
	// same object-list item, which only makes sense on a nested rule. An
	// Accordion has both: its group properties are hidden when the WIDGET's
	// `collapsible` is off, and `initiallyCollapsed` is hidden unless the
	// GROUP's own `initialCollapsedState` is "dynamic".
	Scope string `json:"scope,omitempty"`
}

// OperatorAlways and OperatorNever are the conditions of a rule that was not
// read from the editorConfig's source but obtained by RUNNING its
// getProperties for one widget's values (ako/mxcli#1776): the answer for that
// configuration, so the rule hides its property, or does not, whatever a
// lookup would return. Such rules are built per widget instance and never
// stored in a .def.json.
const (
	OperatorAlways = "always"
	OperatorNever  = "never"
)

// Evaluated reports whether the rule is a per-instance answer from running the
// widget's getProperties rather than a rule lifted from its source.
func (r WidgetVisibilityRule) Evaluated() bool {
	return r.HiddenWhen != nil && (r.HiddenWhen.Operator == OperatorAlways || r.HiddenWhen.Operator == OperatorNever)
}

// ConditionScopeItem marks a condition evaluated against the object-list item
// that carries the rule's property, rather than against the widget.
const ConditionScopeItem = "item"

// Hidden reports whether the condition matches given the widget's current
// property values (keyed by property key, each value the property's primitive
// string form). An unset or unrecognized operator is treated as "not hidden".
func (c *WidgetVisibilityCondition) Hidden(values map[string]string) bool {
	if c == nil {
		return false
	}
	current := values[c.PropertyKey]
	switch c.Operator {
	case "eq":
		return current == c.Value
	case "ne":
		return current != c.Value
	case "truthy":
		return isTruthyPrimitive(current)
	case "falsy":
		return !isTruthyPrimitive(current)
	case "empty":
		return current == ""
	case "notempty":
		return current != ""
	case "in":
		return containsCSV(c.Value, current)
	case "notin":
		return !containsCSV(c.Value, current)
	case OperatorAlways:
		return true
	case OperatorNever:
		return false
	default:
		return false
	}
}

// containsCSV reports whether want is one of csv's comma-separated entries.
// The set comes from an editorConfig `["a","b"].includes(e.prop)` guard, whose
// members are property keys and enum values — neither of which can contain a
// comma, so no escaping is needed.
func containsCSV(csv, want string) bool {
	if csv == "" {
		return false
	}
	for _, part := range strings.Split(csv, ",") {
		if part == want {
			return true
		}
	}
	return false
}

// isTruthyPrimitive mirrors how Mendix treats a boolean/enum primitive in
// editorConfig.js: empty, "false", and "0" are falsy; everything else truthy.
func isTruthyPrimitive(v string) bool {
	switch v {
	case "", "false", "0":
		return false
	default:
		return true
	}
}
