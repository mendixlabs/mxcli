// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// validateListViewEditableInputs (MDL-WIDGET31) reports a list view that will be
// written with Editable false while it holds input widgets that are meant to be
// editable. (ako/mxcli#631)
//
// Pages$ListView.Editable is what makes the inputs INSIDE a list view editable,
// and its read-only context wins over `editable: Always` on the input itself —
// including inside a nested data view. Without it every input renders as
// <div class="form-control-static">, with a valid document, a clean `mx check`
// and a successful build: the failure only shows in the running app.
//
// The writer is right to default it to false: that is Mendix's own default
// (mendixmodelsdk 4.115.0, Pages$ListView `editable` defaults to false and
// _initializeDefaultProperties does not set it). Studio Pro agrees, measured on
// ako/TestApp (Mendix 11.14.0): 30 of its 32 list views are stored Editable false
// and none of those holds an input; the one list view with inputs
// (Pages.EditableLIstView, five textboxes at Editability Always) was set to
// Editable true by its author. So the fix is a diagnostic for the combination,
// not a different default.
//
// Editability is read with GetBoolProp, the same call buildListViewV3 uses, so
// the rule reports what will be written: a quoted `editable: 'true'` is a string
// and is written false.
//
// A warning, not an error: an input shown read-only in a list view is odd but
// legal. An input the author already marked `editable: Never` is not counted.
//
// A pluggable input counts too — a combo box, slider or star rating bound to an
// attribute or association is rendered read-only by the same list view context.
// registry resolves those (pluggableEditableInput); nil limits the rule to the
// built-in inputs.
func validateListViewEditableInputs(w *ast.WidgetV3, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "listview") || w.GetBoolProp("Editable") {
		return nil
	}
	input := firstEditableInput(w.Children, registry)
	if input == nil {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET31",
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf(
			"%s: list view `%s` is written with Editable false (the Mendix default), so input `%s` (%s) "+
				"inside it renders read-only — the list view's context wins over the input's own `editable:`",
			locationPrefix, w.Name, input.Name, input.Type,
		),
		Suggestion: fmt.Sprintf("Add `editable: true` to list view `%s`, or mark the inputs `editable: Never` "+
			"if they are meant to be read-only", w.Name),
	}}
}

// firstEditableInput finds an input widget below a list view that the author has
// not already made read-only. It does not descend into a nested list view, whose
// own Editable governs its inputs and which is reported on its own visit.
func firstEditableInput(widgets []*ast.WidgetV3, registry *WidgetRegistry) *ast.WidgetV3 {
	for _, c := range widgets {
		if c == nil {
			continue
		}
		typ := strings.ToLower(c.Type)
		if typ == "listview" {
			continue
		}
		// `editable: false` is Never too, quoted or not (pages.StaticEditability).
		enum, _, hasStatic := staticEditability(c)
		never := hasStatic && enum == "Never"
		if editableWidgetTypes[typ] && typ != "dataview" && !never {
			return c
		}
		if !never && pluggableEditableInput(c, registry) {
			return c
		}
		if found := firstEditableInput(c.Children, registry); found != nil {
			return found
		}
	}
	return nil
}

// pluggableEditableInput reports whether c is a pluggable widget the author bound
// to an attribute or association it can edit: its definition maps an attribute or
// association property, the script sets it, and — when the widget's package can
// be read — the package declares the Editability system property. That last test
// is what separates a combo box or slider from a widget that only DISPLAYS an
// attribute, which has no Editability at all. Without a package to read (no
// project, or the hand-written definitions) the binding alone decides.
func pluggableEditableInput(c *ast.WidgetV3, registry *WidgetRegistry) bool {
	if registry == nil {
		return false
	}
	def := lookupWidgetDef(c, registry)
	if def == nil || !bindsEditableMember(c, def) {
		return false
	}
	hasEditability, known := widgetDeclaresEditability(registryProjectPath(registry), def.WidgetID)
	return !known || hasEditability
}

// bindsEditableMember reports whether c sets a property its definition maps as
// an attribute or association binding.
func bindsEditableMember(c *ast.WidgetV3, def *WidgetDefinition) bool {
	mappings := append([]PropertyMapping(nil), def.PropertyMappings...)
	for _, m := range def.Modes {
		mappings = append(mappings, m.PropertyMappings...)
	}
	for _, m := range mappings {
		if m.Operation != "attribute" && m.Operation != "association" {
			continue
		}
		for _, key := range append([]string{m.Source, m.PropertyKey}, m.MdlAliases...) {
			if key == "" {
				continue
			}
			if v, ok := lookupProperty(c.Properties, key); ok && stringifyAny(v) != "" {
				return true
			}
		}
	}
	return false
}

var (
	widgetEditabilityCache   = map[string]bool{}
	widgetEditabilityCacheMu sync.Mutex
)

// widgetDeclaresEditability reads the widget's installed package for the
// Editability system property. known is false when no package could be read.
func widgetDeclaresEditability(projectPath, widgetID string) (has, known bool) {
	if projectPath == "" || widgetID == "" {
		return false, false
	}
	cacheKey := projectPath + "\x00" + widgetID
	widgetEditabilityCacheMu.Lock()
	if v, ok := widgetEditabilityCache[cacheKey]; ok {
		widgetEditabilityCacheMu.Unlock()
		return v, true
	}
	widgetEditabilityCacheMu.Unlock()

	projectDir := projectPath
	if strings.EqualFold(filepath.Ext(projectDir), ".mpr") {
		projectDir = filepath.Dir(projectDir)
	}
	mpkPath, err := mpk.FindMPK(projectDir, widgetID)
	if err != nil || mpkPath == "" {
		return false, false
	}
	wd, err := mpk.ParseMPKForWidget(mpkPath, widgetID)
	if err != nil || wd == nil {
		return false, false
	}
	for _, sp := range wd.SystemProps {
		if strings.EqualFold(sp.Key, "Editability") {
			has = true
			break
		}
	}
	widgetEditabilityCacheMu.Lock()
	widgetEditabilityCache[cacheKey] = has
	widgetEditabilityCacheMu.Unlock()
	return has, true
}
