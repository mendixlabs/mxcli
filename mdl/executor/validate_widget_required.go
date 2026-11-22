// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// widgetRequiredRule warns about a required pluggable-widget property left unset.
const widgetRequiredRule = "MDL-WIDGET56"

// validateWidgetRequired warns (MDL-WIDGET56) about a property the widget's
// package marks required that the script leaves unset: mxbuild fails the build
// with CE0642 ("Property 'X' is required"), or CE4899 for a text template.
// Measured with Native Mobile Resources 12.6.0 on mx check 11.13.0: 29 of the
// 39 native widgets placed bare passed check and exec, and failed there.
//
// Set means a value, a filled slot, or at least one list item. A property with
// a usable default needs nothing — an enumeration, a boolean or number, an
// expression or string with a defaultValue, a text template with shipped
// translations — and neither does one the widget's editorConfig hides under
// the configuration, which mx does not require.
//
// Needs the installed package (the project); a warning, because the hidden
// rules are lifted from editorConfig.js and an unread one could leave a
// hidden property looking required.
func validateWidgetRequired(w *ast.WidgetV3, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	def := lookupWidgetDef(w, registry)
	if def == nil {
		return nil
	}
	props := requiredTopLevelProps(registryProjectPath(registry), def.WidgetID)
	if len(props) == 0 {
		return nil
	}
	values, explicit := widgetValueMap(w, def)
	// What the statement wrote, through whichever keyword: `attribute:`
	// writes attributeEnumeration in a combo box's enumeration mode, and
	// that property is set although the script never names it
	// (ako/mxcli#2523).
	stated := map[string]bool{}
	for k, v := range values {
		if v != "" {
			stated[k] = true
		}
	}
	// The hide rules are judged on the configuration exec writes: what the
	// statement says, and the template's value for everything else. Judged
	// on the statement alone, a combo box's `source` and `optionsSourceType`
	// were unknown, no rule could decide, and every other mode's required
	// properties were reported — 12 on one combo box, which mxbuild builds
	// clean (ako/mxcli#2523).
	for k, v := range templateWrittenDefaults(def.WidgetID) {
		if lk := strings.ToLower(k); values[lk] == "" {
			values[lk] = v
		}
	}
	rules := visibilityRulesFor(def, registry)
	slots, lists := filledSlotsAndLists(w, def)

	var out []linter.Violation
	for _, p := range props {
		if propertyHasDefault(p) {
			continue
		}
		key := strings.ToLower(p.Key)
		switch {
		case p.Type == "widgets" && slots[key]:
			continue
		case p.Type == "object" && lists[key]:
			continue
		case explicit[key], stated[key]:
			continue
		case p.Type == "datasource" && p.IsLinked:
			// The containing widget fills it — a Data Grid 2 filter's
			// `linkedDs` is the grid's list (ako/mxcli#2523).
			continue
		}
		if hiddenByConfiguration(rules, p.Key, values) {
			continue
		}
		code := "CE0642"
		if p.Type == "textTemplate" {
			code = "CE4899"
		}
		out = append(out, linter.Violation{
			RuleID:   widgetRequiredRule,
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf("%s: %s: `%s` (%s) is required and not set — mxbuild fails the build with %s",
				locationPrefix, widgetLabel(w.Name, def.MDLName), p.Key, p.Caption, code),
			Suggestion: requiredSuggestion(p, def),
		})
	}
	return out
}

func requiredSuggestion(p mpk.PropertyDef, def *WidgetDefinition) string {
	switch p.Type {
	case "widgets":
		if kw := childSlotKeyword(def, strings.ToLower(p.Key)); kw != "" {
			return fmt.Sprintf("add a `%s { … }` block with at least one widget", kw)
		}
	case "object":
		for _, ol := range def.ObjectLists {
			if strings.EqualFold(ol.PropertyKey, p.Key) {
				return fmt.Sprintf("add at least one `%s` item", strings.ToLower(ol.MDLContainer))
			}
		}
	}
	return fmt.Sprintf("set `%s:`", p.Key)
}

// requiredTopLevelProps returns the widget's required, non-system top-level
// properties as its installed package declares them; nil without one.
func requiredTopLevelProps(projectPath, widgetID string) []mpk.PropertyDef {
	if projectPath == "" || widgetID == "" {
		return nil
	}
	mpkPath, err := mpk.FindMPK(filepath.Dir(projectPath), widgetID)
	if err != nil || mpkPath == "" {
		return nil
	}
	wd, err := mpk.ParseMPKForWidget(mpkPath, widgetID)
	if err != nil || wd == nil {
		return nil
	}
	var out []mpk.PropertyDef
	for _, p := range wd.Properties {
		if p.Required && !p.IsSystem {
			out = append(out, p)
		}
	}
	return out
}

// propertyHasDefault reports whether a required property is satisfied without
// the script naming it.
func propertyHasDefault(p mpk.PropertyDef) bool {
	switch p.Type {
	case "boolean", "integer", "decimal", "enumeration":
		return true
	case "expression", "string":
		return p.DefaultValue != ""
	case "textTemplate":
		return len(p.Translations) > 0
	case "attribute", "association", "datasource", "widgets", "object", "image", "file", "icon":
		return false
	}
	// action, selection and any other kind: not measured as required-when-unset.
	return true
}

// hiddenByConfiguration reports whether a top-level visibility rule determinably
// hides key under values.
func hiddenByConfiguration(rules []types.WidgetVisibilityRule, key string, values map[string]string) bool {
	for _, r := range rules {
		if r.Nested() || r.HiddenWhen == nil || !strings.EqualFold(r.PropertyKey, key) {
			continue
		}
		fires, determinable := r.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
			v, ok := values[strings.ToLower(c.PropertyKey)]
			return v, ok
		})
		if determinable && fires {
			return true
		}
	}
	return false
}

// filledSlotsAndLists reports which widgets slots hold a widget and which
// object lists hold an item, routed the way the engine routes children
// (applyChildSlots, applyObjectLists): a container keyword, `container
// <slot>`, or loose children into the default slot.
func filledSlotsAndLists(w *ast.WidgetV3, def *WidgetDefinition) (slots, lists map[string]bool) {
	slots, lists = map[string]bool{}, map[string]bool{}
	slotByKeyword := map[string]string{}
	all := append([]ChildSlotMapping(nil), def.ChildSlots...)
	for _, m := range def.Modes {
		all = append(all, m.ChildSlots...)
	}
	for _, s := range all {
		slotByKeyword[strings.ToLower(s.MDLContainer)] = strings.ToLower(s.PropertyKey)
	}
	listByKeyword := map[string]string{}
	for _, ol := range def.ObjectLists {
		listByKeyword[strings.ToLower(ol.MDLContainer)] = strings.ToLower(ol.PropertyKey)
	}
	loose := false
	for _, c := range w.Children {
		if c == nil {
			continue
		}
		t := strings.ToLower(c.Type)
		if key, ok := listByKeyword[t]; ok {
			lists[key] = true
			continue
		}
		key, ok := slotByKeyword[t]
		if !ok && t == "container" {
			key, ok = slotByKeyword[strings.ToLower(c.Name)]
		}
		if ok {
			if len(c.Children) > 0 {
				slots[key] = true
			}
			continue
		}
		loose = true
	}
	if key, ok := slotByKeyword[defaultSlotContainer]; ok && loose {
		slots[key] = true
	}
	return slots, lists
}
