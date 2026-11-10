// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// A pluggable widget's editorConfig.js can hide a property under some
// configurations of the same widget. Studio Pro and mxbuild both evaluate that
// logic, and a HIDDEN property is required to hold its DEFAULT value: a
// non-default one is CE0463 "the definition of this widget has changed", which
// fails `mx check` and makes `mx create-module-package` refuse the module.
//
// Measured on mxbuild 11.13 with the stock Accordion (upstream #931). The widget
// hides its whole State group when `collapsible` is off, so:
//
//	collapsible: false + InitialCollapsedState: 'expanded'  → CE0463
//	collapsible: false + InitiallyCollapsed: 'false'        → CE0463
//	collapsible: false + expandBehavior: multipleExpanded   → CE0463
//	collapsible: false + animate: false                     → CE0463
//	collapsible: TRUE  + the same values                    → 0 errors
//	collapsible: false + every value left at its default    → 0 errors
//
// So the diagnostic splits on the value, not on the property: an explicitly set
// DEFAULT is merely redundant (warning — Studio Pro ignores it), a non-default
// one does not build (error).

// hiddenPropertySeverity returns the severity for setting value on a property the
// widget hides, plus the tail of the message explaining the consequence.
//
// def is the property's declared default; when it is unknown ("") the rule stays
// a warning — mxcli will not fail a script on a comparison it could not make.
func hiddenPropertySeverity(value, def, operation string) (linter.Severity, string) {
	if operation == "action" {
		// An action slot has no "default value" to compare a written action
		// against — its default is no action at all — so the `def == ""` branch
		// below would call every hidden action harmless. Measured on a DataGrid2
		// at 11.13.0: an action written into `onSelectionChange` while
		// `itemSelection` is None fails the build with CE0463, and the identical
		// page with the slot unset builds clean. (#956)
		return linter.SeverityError, "an action there fails the build with CE0463"
	}
	if value == slotFilledValue {
		// A widgets slot holding widgets, like an action, has no default to
		// compare against. Measured on a Data grid 2 at 11.14: widgets in
		// `emptyPlaceholder` while showEmptyPlaceholder is "none" fail mx check
		// with CE0463; with "custom", or with the slot empty, the page is clean
		// (ako/mxcli#1862).
		return linter.SeverityError, "widgets there fail the build with CE0463; set the condition's property so the slot shows"
	}
	if def == "" || strings.EqualFold(value, def) {
		return linter.SeverityWarning, "the value will be ignored"
	}
	return linter.SeverityError, fmt.Sprintf(
		"a non-default value there fails the build with CE0463 (the default is %q)", def)
}

// hiddenPropertyViolation builds the MDL-WIDGET10 diagnostic. itemLabel names the
// object-list item a nested rule fired on (e.g. "group `g1`"), empty for a
// top-level property.
func hiddenPropertyViolation(locationPrefix, widgetName, mdlName, itemLabel string,
	rule types.WidgetVisibilityRule, value, def, operation string) linter.Violation {
	severity, consequence := hiddenPropertySeverity(value, def, operation)
	where := ""
	if itemLabel != "" {
		where = " " + itemLabel
	}
	scope := ""
	if rule.HiddenWhen.Scope == types.ConditionScopeItem && itemLabel != "" {
		scope = "its own "
	}
	return linter.Violation{
		RuleID:   "MDL-WIDGET10",
		Severity: severity,
		Message: fmt.Sprintf(
			"%s: %s%s property `%s` is hidden when %s`%s` %s — %s",
			locationPrefix, widgetLabel(widgetName, mdlName), where, rule.PropertyKey,
			scope, rule.HiddenWhen.PropertyKey, visibilityCondWord(rule.HiddenWhen), consequence,
		),
	}
}

// validateWidgetItemVisibility applies the nested (object-list) visibility rules
// of parent's widget definition to one of its items — an Accordion GROUP, a
// DataGrid COLUMN, a PopupMenu ITEM.
//
// A nested rule's condition can name either object: `collapsible` belongs to the
// widget, `initialCollapsedState` to the group itself, and the Accordion uses
// both in the same callback. The condition's Scope says which, and each is looked
// up in its own value map — evaluating an item condition against the widget (or
// the reverse) silently reads an absent key and reports nothing.
func validateWidgetItemVisibility(parent *ast.WidgetV3, item *ast.WidgetV3,
	mapping *ObjectListMapping, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	if parent == nil || item == nil || mapping == nil {
		return nil
	}
	def := lookupWidgetDef(parent, registry)
	if def == nil {
		return nil
	}
	rules := visibilityRulesFor(def, registry)
	if len(rules) == 0 {
		return nil
	}
	widgetValues, _ := widgetValueMap(parent, def)
	itemValues, itemExplicit := itemValueMap(item, mapping)
	defaults := widgetPropertyDefaults(registryProjectPath(registry), def.WidgetID)

	var out []linter.Violation
	for _, rule := range rules {
		if !rule.Nested() || !strings.EqualFold(rule.ListPropertyKey, mapping.PropertyKey) {
			continue
		}
		if rule.HiddenWhen == nil {
			continue
		}
		if !itemExplicit[strings.ToLower(rule.PropertyKey)] {
			continue // the author did not set this sub-property
		}
		// Each term is resolved in its OWN scope: a nested rule mixes conditions
		// about the list item with conditions about the widget.
		fires, determinable := rule.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
			values := widgetValues
			if c.Scope == types.ConditionScopeItem {
				values = itemValues
			}
			v, ok := values[strings.ToLower(c.PropertyKey)]
			return v, ok
		})
		if !determinable || !fires {
			continue
		}
		value := itemValues[strings.ToLower(rule.PropertyKey)]
		declared := declaredDefault(defaults, nil, mapping.ItemProperties, mapping.PropertyKey, rule.PropertyKey)
		label := fmt.Sprintf("%s `%s`", strings.ToLower(item.Type), item.Name)
		out = append(out, hiddenPropertyViolation(locationPrefix, parent.Name, def.MDLName, label, rule, value, declared,
			mappingOperationFor(def, rule.PropertyKey)))
	}
	return out
}

// visibilityRulesFor returns a widget definition's visibility rules, falling back
// to lifting them from the installed .mpk when the def carries none. Mirrors what
// validateWidgetVisibility does for top-level rules.
func visibilityRulesFor(def *WidgetDefinition, registry *WidgetRegistry) []types.WidgetVisibilityRule {
	if len(def.PropertyVisibility) > 0 {
		return def.PropertyVisibility
	}
	if path := registryProjectPath(registry); path != "" {
		return resolveWidgetVisibilityRules(path, def.WidgetID)
	}
	return nil
}

func registryProjectPath(registry *WidgetRegistry) string {
	if registry == nil {
		return ""
	}
	return registry.projectPath
}

// sharedSourceOffMode reports whether an item property's shared Source lookup
// must be skipped because the item's dataSet mode selects its sibling.
//
// Scoped to a chart series' datasource pair, which is the one place two item
// properties claim the same Source. Anything wider would blind the
// hidden-property rule on every other object list, so the gate mirrors the
// builder's own condition rather than generalising it.
func sharedSourceOffMode(mapping *ObjectListMapping, m ItemPropertyMapping, dataSetMode string) bool {
	if m.Operation != "datasource" || !isChartSeriesContainer(mapping.MDLContainer) {
		return false
	}
	return !seriesDataSourceMatchesMode(m.PropertyKey, dataSetMode)
}

// itemValueMap resolves an object-list item's sub-property values (keyed by
// lowercased schema key) and reports which the MDL set explicitly. The item form
// of widgetValueMap.
func itemValueMap(item *ast.WidgetV3, mapping *ObjectListMapping) (values map[string]string, explicit map[string]bool) {
	values = map[string]string{}
	explicit = map[string]bool{}

	// A chart series' two datasource sub-properties SHARE the Source name
	// "DataSource" (measured on linechart.def.json: staticDataSource and
	// dynamicDataSource both declare it, neither declares an alias), so the
	// mode selects which one the friendly `DataSource:` lands in. The builder
	// routes on it — buildObjectListItem consults "DataSource" only for the
	// property seriesDataSourceMatchesMode picks — so the checker has to as
	// well, or it reports a property the script never wrote.
	itemDataSetMode := "static"
	if v, ok := lookupProperty(item.Properties, "dataSet"); ok {
		if s := stringifyAny(v); s != "" {
			itemDataSetMode = s
		}
	}

	for _, m := range mapping.ItemProperties {
		key := strings.ToLower(m.PropertyKey)
		val, set := "", false
		if m.Source != "" && !sharedSourceOffMode(mapping, m, itemDataSetMode) {
			if v, ok := lookupWidgetProp(item, m.Source); ok {
				val, set = v, true
			}
		}
		if !set {
			for _, a := range m.MdlAliases {
				if v, ok := lookupWidgetProp(item, a); ok {
					val, set = v, true
					break
				}
			}
		}
		if !set {
			if v, ok := lookupWidgetProp(item, m.PropertyKey); ok {
				val, set = v, true
			}
		}
		if set {
			explicit[key] = true
		} else if m.Default != "" {
			val = m.Default
		} else if m.Value != "" {
			val = m.Value
		}
		if val != "" {
			if m.Operation == "selection" {
				val = canonicalSelection(val)
			}
			values[key] = val
		}
	}
	// Sub-properties named after a schema key with no mapping entry.
	for k, raw := range item.Properties {
		lk := strings.ToLower(k)
		if _, ok := values[lk]; ok {
			continue
		}
		if s := stringifyPropValue(raw); s != "" {
			values[lk] = s
			explicit[lk] = true
		}
	}
	return values, explicit
}

// declaredDefault returns a property's default: the .mpk's declared value when
// the project is reachable, otherwise the definition's own mapping default.
//
// The mapping is the weaker source — it carries a default only for the
// primitive-typed properties (an expression property like the Accordion group's
// `initiallyCollapsed` has none) — but it is the one available when there is no
// project to read, which keeps the severity split working for `mxcli check`
// without `-p` and for in-memory tests. An unknown default keeps the diagnostic a
// warning; it never invents one.
func declaredDefault(mpkDefaults map[string]string, mappings []PropertyMapping,
	itemMappings []ItemPropertyMapping, listKey, propertyKey string) string {
	if v := mpkDefaults[defaultsKey(listKey, propertyKey)]; v != "" {
		return v
	}
	if listKey == "" {
		for _, m := range mappings {
			if strings.EqualFold(m.PropertyKey, propertyKey) {
				return firstNonEmpty(m.Default, m.Value)
			}
		}
		return ""
	}
	for _, m := range itemMappings {
		if strings.EqualFold(m.PropertyKey, propertyKey) {
			return firstNonEmpty(m.Default, m.Value)
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// defaultsKey builds the lookup key used by widgetPropertyDefaults: a bare
// lowercased property key for a top-level property, "list/key" for an item
// sub-property.
func defaultsKey(listKey, propertyKey string) string {
	if listKey == "" {
		return strings.ToLower(propertyKey)
	}
	return strings.ToLower(listKey) + "/" + strings.ToLower(propertyKey)
}

var (
	widgetDefaultsCache   = map[string]map[string]string{}
	widgetDefaultsCacheMu sync.Mutex
)

// widgetPropertyDefaults returns the DECLARED defaults of a widget's properties,
// read from the installed .mpk — top-level keys plus "list/key" entries for
// object-list item sub-properties.
//
// The .mpk is the only complete source: the executor's PropertyMapping carries a
// default only for the primitive-typed properties, so an expression property like
// the Accordion group's `initiallyCollapsed` (default "true") has none there —
// and that is one of the two properties #931 turns on. Returns an empty map when
// the widget or its package cannot be found, which downgrades every diagnostic to
// a warning rather than guessing a default.
func widgetPropertyDefaults(projectPath, widgetID string) map[string]string {
	if projectPath == "" || widgetID == "" {
		return nil
	}
	cacheKey := projectPath + "\x00" + widgetID
	widgetDefaultsCacheMu.Lock()
	if d, ok := widgetDefaultsCache[cacheKey]; ok {
		widgetDefaultsCacheMu.Unlock()
		return d
	}
	widgetDefaultsCacheMu.Unlock()

	defaults := map[string]string{}
	projectDir := projectPath
	if strings.EqualFold(filepath.Ext(projectDir), ".mpr") {
		projectDir = filepath.Dir(projectDir)
	}
	if mpkPath, err := mpk.FindMPK(projectDir, widgetID); err == nil && mpkPath != "" {
		if wd, err := mpk.ParseMPKForWidget(mpkPath, widgetID); err == nil && wd != nil {
			collectPropertyDefaults(defaults, "", wd.Properties)
		}
	}

	widgetDefaultsCacheMu.Lock()
	widgetDefaultsCache[cacheKey] = defaults
	widgetDefaultsCacheMu.Unlock()
	return defaults
}

// collectPropertyDefaults walks a widget's property tree, recording each declared
// default one level deep (object-list item sub-properties).
func collectPropertyDefaults(out map[string]string, listKey string, props []mpk.PropertyDef) {
	for _, p := range props {
		if p.DefaultValue != "" {
			out[defaultsKey(listKey, p.Key)] = p.DefaultValue
		}
		if len(p.Children) > 0 && listKey == "" {
			collectPropertyDefaults(out, p.Key, p.Children)
		}
	}
}
