// SPDX-License-Identifier: Apache-2.0

// Check-time validation for the `editable:` widget property.
//
// Mendix models editability on INPUT widgets only. Measured against
// generated/metamodel (the arbiter per CLAUDE.md), exactly eleven Pages types
// carry Editability / ConditionalEditabilitySettings — ten input widgets plus
// DataView — and not one of the fourteen button types does. A button has
// visibility, not editability.
//
// mxcli accepted `editable:` on any widget because the allow-lists behind
// MDL-WIDGET01 and MDL-WIDGET07 are widget-type-AGNOSTIC: isBuiltinPropName
// answers "is this a real MDL property name anywhere", and both validators read
// it as "is this valid on THIS widget". So `editable:` on a button passed check,
// passed exec, wrote nothing, and the button stayed enabled — a silent
// functional failure rather than a caught error. (issue #928)
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// editableWidgetTypes are the MDL widget types whose Mendix counterpart carries
// Editability / ConditionalEditabilitySettings.
//
// Kept in sync with generated/metamodel by
// TestEditableWidgetTypesMatchMetamodel, which fails if Mendix's set changes —
// the list cannot be derived at runtime because MDL type names are not the
// metamodel's type names.
var editableWidgetTypes = map[string]bool{
	"checkbox":                  true, // Pages$CheckBox
	"dataview":                  true, // Pages$DataView
	"datepicker":                true, // Pages$DatePicker
	"dropdown":                  true, // Pages$DropDown
	"filemanager":               true, // Pages$FileManager
	"imageuploader":             true, // Pages$ImageUploader
	"inputreferencesetselector": true, // Pages$InputReferenceSetSelector
	"radiobuttons":              true, // Pages$RadioButtonGroup
	"radiobuttongroup":          true, // Pages$RadioButtonGroup (alternate spelling)
	"referenceselector":         true, // Pages$ReferenceSelector
	"textarea":                  true, // Pages$TextArea
	"textbox":                   true, // Pages$TextBox
}

// plainEditableWidgetTypes are the MDL widget types whose Mendix counterpart has
// a plain `Editable bool` and NO Editability.
//
// That is a different property from the one above, with a different meaning, and
// conflating the two was ako/mxcli#510. Pages$ListView.Editable makes the input
// widgets INSIDE the list view editable; Pages$GridColumn.Editable does the same
// for a column's cell. mxcli writes both — buildListViewV3 sets it, `describe
// page` reads it back, and mxbuild accepts the result at 0 errors on 11.12.2 —
// so warning that they are "silently dropped" was false, and told authors to
// stop setting the one property that makes an authored list view's inputs
// editable at all. buildListViewV3's own comment records how that failure looks:
// every input rendered as `<div class="form-control-static">`, with entity
// access ReadWrite and `mx check` clean.
//
// Only the PLAIN form is written. Neither type has ConditionalEditabilitySettings,
// so `editable: [expr]` on one is genuinely dropped and still earns MDL-WIDGET20
// — which is why this is a separate set rather than more entries in the one
// above.
//
// Kept in sync with generated/metamodel by TestPlainEditableBoolTypesMatchMetamodel,
// the sibling of TestEditableWidgetTypesMatchMetamodel. The older test could not
// have caught #510: it enumerates types carrying Editability, and these carry
// none, so they were invisible to it.
var plainEditableWidgetTypes = map[string]bool{
	"listview": true, // Pages$ListView
	"column":   true, // Pages$GridColumn
}

// ValidateWidgetEditability reports (MDL-WIDGET20) an `editable:` property on a
// widget type that has no editability in the Mendix model.
//
// It runs in the no-project pass: the answer is the widget's own type, already
// in the statement. A warning rather than an error, matching MDL-WIDGET07 — the
// same "silently dropped on write" family, and the same reason (a hard reject on
// a vocabulary that cannot be proven complete risks false positives).
//
// Pluggable widgets are excluded. Whether one has editability is its package's
// answer, not its MDL type's: MDL-WIDGET41 (pluggable_system_props.go) checks
// the declared Editability system property instead.
func validateWidgetEditability(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	key, ok := widgetEditableKey(w)
	if !ok {
		return nil
	}
	typ := strings.ToLower(w.Type)
	if editableWidgetTypes[typ] {
		return nil
	}
	// A plain `Editable bool` type: the plain form is written, the bracket form
	// (lowered to EditableIf, and riding on ConditionalEditabilitySettings these
	// types do not have) is not. Only the second is worth reporting.
	if plainEditableWidgetTypes[typ] && !strings.EqualFold(key, "editableif") {
		// A quoted condition is the bracket form's twin: it is the part of
		// `Editable:` these types have no place for.
		if _, expr, ok := staticEditability(w); !ok || expr == "" {
			return nil
		}
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET20",
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf(
			"%s: %s has an `%s` property, but Mendix models editability on input "+
				"widgets only — %s has no Editability, so this is silently dropped on write and the "+
				"widget stays enabled",
			locationPrefix, widgetLabel(w.Name, w.Type), key, w.Type,
		),
		Suggestion: "Use `visible: <expression>` to hide it conditionally (buttons do support conditional " +
			"visibility), or move the condition into the microflow the button calls",
	}}
}

// widgetEditableKey finds an editability property however the author spelled it.
// Both MDL forms have to be caught: `editable: 'false'` stays as `Editable`,
// while the bracket form `editable: [expr]` is lowered by the visitor to
// `EditableIf`. Missing the second would leave the shape the docs
// actually recommend silently dropped, which is the bug. The key is returned as
// spelled so the message quotes the author's casing.
func widgetEditableKey(w *ast.WidgetV3) (string, bool) {
	for key := range w.Properties {
		l := strings.ToLower(key)
		if l == "editable" || l == "editableif" {
			return key, true
		}
	}
	return "", false
}
