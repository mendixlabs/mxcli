// SPDX-License-Identifier: Apache-2.0

package pages

import "strings"

// coreNamesPluggableKeysShadow are MDL property names that mean a core
// widget property — a button's Caption and ButtonStyle, an input's Label, a
// button's Icon, a Tooltip — and that a CustomWidgets$CustomWidget never
// stores at its own level. A pluggable widget whose definition declares one
// of them as a property key of its own (PlatformDesignSystem's Button declares
// `caption`, `buttonStyle` and `icon`, its Form label `label`) can only mean
// that key: there is no core property on the widget for the name to reach.
//
// Resolving the name as core first is what ako/mxcli#2180
// (mendixlabs/mxcli#1833) reported: ALTER refused `caption` and wrote
// `buttonStyle` nowhere while saying "Altered page", `label` went to a core
// LabelTemplate beside the widget's own one, check judged the widget's own
// enumeration values against Mendix's button styles, and create page dropped
// `icon`. The names the CustomWidget DOES store itself — Name, Class, Style,
// visibility, editability — are not here: on those the core meaning is real.
//
// `Label` needs one more condition, which callers apply: a pluggable widget
// may declare the Label SYSTEM property (its PropertyType has ValueType
// System), and then `label:` is the widget-level label, as before.
var coreNamesPluggableKeysShadow = map[string]bool{
	"caption":     true,
	"label":       true,
	"buttonstyle": true,
	"icon":        true,
	"tooltip":     true,
}

// PluggableKeyShadowsCoreName reports whether name is a core property name that
// a pluggable widget's own (non-system) property key of the same spelling, in
// any case, takes over. Callers decide whether the widget declares such a key.
func PluggableKeyShadowsCoreName(name string) bool {
	return coreNamesPluggableKeysShadow[strings.ToLower(name)]
}
