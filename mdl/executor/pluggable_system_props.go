// SPDX-License-Identifier: Apache-2.0

// Visibility and editability on pluggable widgets.
//
// Editability is opt-in per package: its widget XML declares
// <systemProperty key="Editability"/> (Combobox.xml and Switch.xml do,
// Datagrid.xml does not). Visibility is available on every pluggable widget —
// a declared <systemProperty key="Visibility"/> only places it in the widget's
// own tabs; Studio Pro stores conditional visibility on a Datagrid whose Type
// declares none (TestApp WorkflowCommons.UserTask_Assign).
// Studio Pro then stores the settings on the CustomWidgets$CustomWidget itself
// — ConditionalVisibilitySettings, Editable, ConditionalEditabilitySettings,
// the same Forms$ elements a text box carries — and NOT as a WidgetProperty in
// the widget's Object: a declared system property has a PropertyType in the
// widget's Type (ValueType.Type "System") and no value beside it. Measured on
// Studio Pro-authored combo boxes in a stock 11.14 app.
//
// The declaration is read from the widget's Type: at build time the one being
// written (cw.RawType), at check time the same template the build loads —
// GetTemplateBSON, which augments it from the project's .mpk exactly as
// LoadWidgetTemplate does — so check and exec cannot disagree about a widget.
package executor

import (
	"fmt"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// pluggableSystemPropRule refuses `visible:`/`editable:` on a pluggable widget
// whose package does not declare the matching system property.
const pluggableSystemPropRule = "MDL-WIDGET41"

const (
	sysPropVisibility  = "Visibility"
	sysPropEditability = "Editability"
)

// widgetAsksVisibility reports whether the statement sets a visibility that has
// to be stored. `visible: true` is Mendix's default and stores nothing.
func widgetAsksVisibility(w *ast.WidgetV3) bool {
	if w.GetStringProp("VisibleIf") != "" {
		return true
	}
	if vw, ok := w.Properties["VisibleWhen"].(*ast.VisibleWhenV3); ok && vw != nil {
		return true
	}
	_, ok := pages.StaticVisibleExpression(w.Properties["Visible"])
	return ok
}

// widgetAsksEditability reports whether the statement sets an editability that
// has to be stored. `editable: Always` is Mendix's default and stores nothing.
func widgetAsksEditability(w *ast.WidgetV3) bool {
	if w.GetStringProp("EditableIf") != "" {
		return true
	}
	enum, expr, ok := staticEditability(w)
	return ok && (expr != "" || enum != "Always")
}

// systemPropsOfType returns the system properties a CustomWidgetType declares,
// and false when the Type carries no property types at all (nothing to judge).
func systemPropsOfType(rawType any) (map[string]bool, bool) {
	pts := storedPropertyTypes(rawType)
	if len(pts) == 0 {
		return nil, false
	}
	out := map[string]bool{}
	for key, vt := range pts {
		if t, ok := bsonLookup(vt, "Type"); ok && t == "System" {
			out[key] = true
		}
	}
	return out, true
}

// undeclaredSystemProps lists the system properties the statement needs that
// the widget does not declare.
func undeclaredSystemProps(w *ast.WidgetV3, declared map[string]bool) []string {
	var missing []string
	// Visibility is not gated: Studio Pro offers conditional visibility on
	// every pluggable widget, declared or not — TestApp's Studio Pro-authored
	// WorkflowCommons.UserTask_Assign stores ConditionalVisibilitySettings on
	// a Datagrid whose Type declares no Visibility system property. Refusing
	// it made describe → exec of that page fail. A declared Visibility only
	// places the setting in the widget's own property tabs.
	if widgetAsksEditability(w) && !declared[sysPropEditability] {
		missing = append(missing, sysPropEditability)
	}
	return missing
}

func undeclaredSystemPropMessage(name, kind string, missing []string) string {
	return fmt.Sprintf("%s cannot have its %s set: its widget package declares no %s system propert%s, "+
		"so Studio Pro offers no such setting and the widget would never evaluate one",
		widgetLabel(name, kind), strings.ToLower(strings.Join(missing, " or ")),
		strings.Join(missing, " / "), pluralY(len(missing)))
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// applyPluggableSystemSettings stores a pluggable widget's visibility and
// editability, refusing either when the widget's package does not declare it.
// The attribute-based `visible: Attr in (…)` form is applied by the caller,
// which has the entity context it resolves against.
func applyPluggableSystemSettings(cw *pages.CustomWidget, w *ast.WidgetV3) error {
	if cw == nil || w == nil {
		return nil
	}
	if declared, known := systemPropsOfType(cw.RawType); known {
		if missing := undeclaredSystemProps(w, declared); len(missing) > 0 {
			return mdlerrors.NewValidation(undeclaredSystemPropMessage(w.Name, w.Type, missing))
		}
	}
	applyConditionalSettings(cw, w)
	// The writer stores CustomWidget.Editable (the engine fills it with the
	// definition's default); the shared helper above set BaseWidget's.
	if cw.ConditionalEditability != nil || cw.BaseWidget.Editable != "" {
		cw.Editable = pages.WidgetEditability(&cw.BaseWidget)
	}
	return nil
}

// templateSystemPropsCache: projectPath + "\x00" + widgetID → declared set
// (nil when the widget has no template to judge by).
var templateSystemPropsCache sync.Map

// templateSystemProps returns the system properties of the Type the build would
// write for widgetID, and false when there is no template to judge by.
func templateSystemProps(projectPath, widgetID string) (map[string]bool, bool) {
	key := projectPath + "\x00" + widgetID
	if v, ok := templateSystemPropsCache.Load(key); ok {
		m := v.(map[string]bool)
		return m, m != nil
	}
	var out map[string]bool
	n := 0
	idGen := func() string { n++; return fmt.Sprintf("%032d", n) }
	if typ, _, err := mwidgets.GetTemplateBSON(widgetID, idGen, projectPath); err == nil && typ != nil {
		// The registry speaks bson v2, the stored-Type readers v1.
		var v1 bson.D
		if raw, err := bsonv2.Marshal(typ); err == nil && bson.Unmarshal(raw, &v1) == nil {
			if declared, ok := systemPropsOfType(v1); ok {
				out = declared
			}
		}
	}
	templateSystemPropsCache.Store(key, out)
	return out, out != nil
}

// validatePluggableSystemProps (MDL-WIDGET41) refuses `visible:`/`editable:` on
// a pluggable widget whose package does not declare the system property.
//
// It replaces the editability half of MDL-WIDGET21, which warned that
// `editable:` on ANY pluggable widget "reaches no stored property". That was
// measured against a builder that never wrote it; CustomWidgets$CustomWidget
// has Editable and both Conditional* settings, and the builder now stores them
// whenever the package declares them.
func validatePluggableSystemProps(w *ast.WidgetV3, def *WidgetDefinition, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	if w == nil || def == nil || def.WidgetID == "" {
		return nil
	}
	if !widgetAsksVisibility(w) && !widgetAsksEditability(w) {
		return nil
	}
	declared, known := templateSystemProps(registryProjectPath(registry), def.WidgetID)
	if !known {
		return nil
	}
	missing := undeclaredSystemProps(w, declared)
	if len(missing) == 0 {
		return nil
	}
	return []linter.Violation{{
		RuleID:   pluggableSystemPropRule,
		Severity: linter.SeverityError,
		Message:  fmt.Sprintf("%s: %s", locationPrefix, undeclaredSystemPropMessage(w.Name, def.MDLName, missing)),
		Suggestion: "Remove the property. To hide the widget conditionally, wrap it in a `container` " +
			"and set `visible:` there; `mxcli widget describe " + def.WidgetID + "` lists what the widget declares.",
	}}
}
