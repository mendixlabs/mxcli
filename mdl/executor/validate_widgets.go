// SPDX-License-Identifier: Apache-2.0

// Widget property validation for `mxcli check`. Walks CREATE PAGE / CREATE
// SNIPPET / ALTER PAGE statements, finds pluggable widget AST nodes, and
// verifies that each property key the user wrote matches a known property
// in the widget's .def.json. Catches the most common authoring mistake:
// typos like `expanBehavior` instead of `expandBehavior`.
//
// Requires either built-in registry alone (no project context) or the
// project's .mxcli/widgets/*.def.json files (richer coverage) loaded via
// LoadUserDefinitions.
package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// extraUniversalWidgetProperties are AST keys produced by the visitor that
// aren't already covered by isBuiltinPropName (e.g. conditional-binding
// metadata). The main allow-list is derived from isBuiltinPropName so the
// validator stays in sync with whatever the widget engine actually accepts.
var extraUniversalWidgetProperties = map[string]bool{
	"conditionalvisibility":  true,
	"conditionaleditability": true,
}

// ValidateWidgetProperties walks pluggable widget AST nodes in the program
// and flags property keys that the widget definition doesn't recognize.
// projectPath, if non-empty, loads project-level .def.json files so
// project-installed widgets get validated too; otherwise only built-in
// definitions are consulted.
func ValidateWidgetProperties(prog *ast.Program, projectPath string) []linter.Violation {
	registry := LoadWidgetRegistry(projectPath)
	if registry == nil {
		return nil
	}
	var violations []linter.Violation
	for _, stmt := range prog.Statements {
		violations = append(violations, ValidateWidgetPropertiesForStatement(stmt, registry)...)
	}
	return violations
}

// LoadWidgetRegistry returns a widget registry loaded with both the built-in
// definitions and (when projectPath is non-empty) project-level .def.json
// files. Returns nil if registry initialization fails. The LSP uses this to
// load the registry once per session rather than per validation pass.
func LoadWidgetRegistry(projectPath string) *WidgetRegistry {
	registry, err := NewWidgetRegistry()
	if err != nil || registry == nil {
		return nil
	}
	if projectPath != "" {
		_ = registry.LoadUserDefinitions(projectPath)
	}
	return registry
}

// ValidateWidgetPropertiesForStatement runs widget property validation on a
// single statement using a pre-loaded registry. Returns no violations for
// statements that don't carry pluggable widgets (everything except
// CreatePageStmtV3, CreateSnippetStmtV3, AlterPageStmt).
func ValidateWidgetPropertiesForStatement(stmt ast.Statement, registry *WidgetRegistry) []linter.Violation {
	if registry == nil {
		return nil
	}
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		return validateWidgetTree(s.Widgets, registry, "page "+s.Name.String())
	case *ast.CreateSnippetStmtV3:
		return validateWidgetTree(s.Widgets, registry, "snippet "+s.Name.String())
	case *ast.AlterPageStmt:
		var out []linter.Violation
		for _, op := range s.Operations {
			switch o := op.(type) {
			case *ast.InsertWidgetOp:
				out = append(out, validateWidgetTree(o.Widgets, registry, "alter "+s.PageName.String())...)
			case *ast.ReplaceWidgetOp:
				out = append(out, validateWidgetTree(o.NewWidgets, registry, "alter "+s.PageName.String())...)
			}
		}
		return out
	}
	return nil
}

// validateWidgetTree recursively walks the AST widget tree and validates
// pluggable widgets it encounters.
func validateWidgetTree(widgets []*ast.WidgetV3, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	return validateWidgetTreeIn(widgets, registry, locationPrefix, nil)
}

// validateWidgetTreeIn is validateWidgetTree with the *parent* widget's
// object-list mappings keyed by container keyword (e.g. a chart's SERIES / LINE,
// a Maps DYNAMICMARKER). A child whose Type is one of those is an object-list
// item, not a built-in widget — its sub-properties (staticDataSource,
// staticXAttribute, …) are resolved by the object-list engine and written, so it
// must be exempt from the MDL-WIDGET07 "unrecognized property, silently dropped"
// warning. When the parent mapping is known, the child's enumeration
// sub-properties are validated against their member keys (MDL-WIDGET08). (9a)
func validateWidgetTreeIn(widgets []*ast.WidgetV3, registry *WidgetRegistry, locationPrefix string, parentObjectLists map[string]*ObjectListMapping) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		mapping := parentObjectLists[strings.ToUpper(w.Type)]
		isObjectListItem := mapping != nil || isUniversalObjectListKeyword(w.Type)
		out = append(out, validatePluggableWidgetProperties(w, registry, locationPrefix)...)
		out = append(out, validateStaticWidget(w, locationPrefix)...)
		// Unknown-property warning applies only to built-in widgets; pluggable
		// widgets get the stricter def.json check (MDL-WIDGET01) above, and
		// object-list items are validated by the object-list engine.
		def := lookupWidgetDef(w, registry)
		if def == nil && !isObjectListItem {
			out = append(out, validateStaticWidgetUnknownProps(w, locationPrefix)...)
		}
		if mapping != nil {
			out = append(out, validateObjectListItemEnums(w, mapping, locationPrefix)...)
		}
		if len(w.Children) > 0 {
			out = append(out, validateWidgetTreeIn(w.Children, registry, locationPrefix, objectListMappingSet(def))...)
		}
	}
	return out
}

// validateObjectListItemEnums flags an enumeration sub-property of an object-list
// item (e.g. a Maps marker `LocationType`, a chart series `Interpolation`) whose
// value isn't one of the widget's declared member keys. Studio Pro silently
// defaults an invalid enum value, so this class of typo otherwise fails quietly
// at build (e.g. CE "a dynamic marker requires an address"). MDL-WIDGET08.
func validateObjectListItemEnums(w *ast.WidgetV3, mapping *ObjectListMapping, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, ip := range mapping.ItemProperties {
		if len(ip.EnumValues) == 0 {
			continue
		}
		raw, ok := w.Properties[ip.PropertyKey]
		if !ok {
			// case-insensitive fallback (MDL keys keep the author's casing)
			for k, v := range w.Properties {
				if strings.EqualFold(k, ip.PropertyKey) {
					raw, ok = v, true
					break
				}
			}
		}
		if !ok {
			continue
		}
		val, isStr := raw.(string)
		if !isStr || val == "" {
			continue
		}
		if enumValuesContain(ip.EnumValues, val) {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET08",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (%s) property `%s` has invalid value `%s` — valid values are %s",
				locationPrefix, w.Name, w.Type, ip.PropertyKey, val, strings.Join(ip.EnumValues, ", "),
			),
		})
	}
	return out
}

// enumValuesContain reports whether val matches any member key case-insensitively.
func enumValuesContain(members []string, val string) bool {
	for _, m := range members {
		if strings.EqualFold(m, val) {
			return true
		}
	}
	return false
}

// isUniversalObjectListKeyword reports whether a widget Type is one of the
// grammar's object-list container keywords (the singular forms routed via a
// parent's def.json `objectLists`). These are never built-in widgets, so their
// sub-properties must be exempt from the MDL-WIDGET07 static-widget check even
// when no project def is loaded (syntax-only `check`). Mirrors the object-list
// keywords in MDLPage.g4 `widgetTypeV3`. Chart series (9a).
func isUniversalObjectListKeyword(widgetType string) bool {
	switch strings.ToUpper(widgetType) {
	case "SERIES", "LINE", "GROUP", "CUSTOMITEM", "ITEM", "MARKER", "DYNAMICMARKER":
		return true
	}
	return false
}

// objectListMappingSet returns a widget definition's object-list mappings keyed
// by uppercase container keyword (nil when def is nil or has none).
func objectListMappingSet(def *WidgetDefinition) map[string]*ObjectListMapping {
	if def == nil || len(def.ObjectLists) == 0 {
		return nil
	}
	set := make(map[string]*ObjectListMapping, len(def.ObjectLists))
	for i := range def.ObjectLists {
		set[strings.ToUpper(def.ObjectLists[i].MDLContainer)] = &def.ObjectLists[i]
	}
	return set
}

// staticWidgetKnownProps is the lowercase vocabulary of properties a built-in
// (non-pluggable) page widget can legitimately carry. It is the union of the
// grammar keyword properties, every key the executor builders consume, and every
// property `describe page` can emit (so the describe→create roundtrip never
// self-warns). A property outside this set on a core widget is not consumed by
// any builder — i.e. it is silently dropped — so it earns an MDL-WIDGET07
// warning. It is deliberately generous (a union across all widget types, not
// per-type) to avoid false positives; TestStaticWidgetKnownPropsCoverDescribe
// guards it against describe-vocabulary drift.
var staticWidgetKnownProps = func() map[string]bool {
	names := []string{
		// grammar keyword properties (widgetPropertyV3)
		"DataSource", "Attribute", "Binds", "Action", "OnClick", "Caption", "Label",
		"Attr", "Content", "RenderMode", "ContentParams", "CaptionParams", "ButtonStyle",
		"Class", "Style", "DesktopWidth", "TabletWidth", "PhoneWidth", "Selection",
		"Snippet", "Params", "Attributes", "FilterType", "DesignProperties", "Width",
		"Height", "Visible", "Editable", "Tooltip",
		// keys the builders/visitor consume and the conditional-binding metadata
		"CaptionAttribute", "Collapsible", "DatabaseHost", "DefaultLanguage", "Footer",
		"FormOrientation", "HeaderMode", "LabelWidth", "Prefix", "ShowContentAs", "Title",
		"Widget", "WidgetType", "ShowLabel", "VisibleIf", "EditableIf", "DynamicClasses",
		// vocabulary describe page emits (native widgets + datagrid columns)
		"Alignment", "AlternativeText", "ColumnClass", "ColumnWidth", "DesktopColumns",
		"DisplayAs", "Draggable", "DynamicCellClass", "HeightUnit", "Hidable", "ImageType",
		"ImageUrl", "LabelPosition", "PageSize", "Pagination", "PagingPosition",
		"PhoneColumns", "ReadOnlyStyle", "Resizable", "Responsive", "ShowPagingButtons",
		"Size", "Sortable", "TabletColumns", "WidthUnit", "WrapText", "Name",
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[strings.ToLower(n)] = true
	}
	return m
}()

// staticWidgetKnownPropList is the canonical-cased key list used for "did you
// mean" suggestions on MDL-WIDGET07.
var staticWidgetKnownPropList = func() []string {
	seen := map[string]bool{}
	var list []string
	add := func(names ...string) {
		for _, n := range names {
			if l := strings.ToLower(n); !seen[l] {
				seen[l] = true
				list = append(list, n)
			}
		}
	}
	add("DataSource", "Attribute", "Action", "OnClick", "Caption", "Label", "Content",
		"RenderMode", "ContentParams", "CaptionParams", "ButtonStyle", "Class", "Style",
		"DesktopWidth", "TabletWidth", "PhoneWidth", "Selection", "Snippet", "Params",
		"Attributes", "FilterType", "DesignProperties", "Width", "Height", "Visible",
		"Editable", "Tooltip", "DynamicClasses", "WidthUnit", "HeightUnit",
		"DesktopColumns", "TabletColumns", "PhoneColumns", "PageSize", "Pagination")
	return list
}()

// isKnownStaticWidgetProp reports whether key is a recognized property for a
// built-in widget (case-insensitively).
func isKnownStaticWidgetProp(key string) bool {
	if isBuiltinPropName(key) {
		return true
	}
	l := strings.ToLower(key)
	return extraUniversalWidgetProperties[l] || staticWidgetKnownProps[l]
}

// validateStaticWidgetUnknownProps warns (MDL-WIDGET07) about property keys on a
// built-in widget that no builder consumes — they pass `check`/`exec` but are
// silently dropped on write. It is a warning, not an error: the core-widget
// property vocabulary can't be proven complete per widget type, so a hard reject
// could false-positive on a valid property. Pluggable widgets are validated
// separately (MDL-WIDGET01) and must not reach here.
func validateStaticWidgetUnknownProps(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for key := range w.Properties {
		if isKnownStaticWidgetProp(key) {
			continue
		}
		hint := ""
		if suggestion := nearestKey(key, staticWidgetKnownPropList); suggestion != "" {
			hint = fmt.Sprintf(" — did you mean `%s`?", suggestion)
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET07",
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf(
				"%s: widget `%s` (%s) property `%s` is not recognized and will be silently dropped on write%s",
				locationPrefix, w.Name, w.Type, key, hint,
			),
		})
	}
	return out
}

// validateStaticWidget checks value-level constraints on built-in (non-pluggable)
// widgets that the grammar can't express and that otherwise fail silently or at
// build time rather than at `mxcli check` time.
func validateStaticWidget(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	var out []linter.Violation

	// An unrecognized (often mis-cased) button style is silently degraded to
	// btn-default by MxBuild. Reject it at check time. CanonicalButtonStyle is
	// case-insensitive, so legitimate lowercase values still pass.
	if bs := w.GetButtonStyle(); bs != "" {
		if _, ok := pages.CanonicalButtonStyle(bs); !ok {
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET02",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: widget `%s` has unknown button style `%s` — valid styles are %s",
					locationPrefix, w.Name, bs, strings.Join(pages.ValidButtonStyleList(), ", "),
				),
			})
		}
	}

	// An inline Style on a DynamicText crashes MxBuild with a
	// NullReferenceException — the widget must be wrapped in a container.
	if strings.EqualFold(w.Type, "dynamictext") && w.GetStyle() != "" {
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET03",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (dynamictext) cannot have an inline `style` — it crashes MxBuild; wrap it in a container and style the container instead",
				locationPrefix, w.Name,
			),
		})
	}

	// A dynamic text whose template references a {N} placeholder with no matching
	// parameter binding is an orphaned ClientTemplate — MxBuild fails with CE0720
	// and Studio Pro throws a NullReferenceException when the widget is opened
	// (issue #650). Catch it at check time.
	if v := validateDynamicTextPlaceholders(w, locationPrefix); v != nil {
		out = append(out, *v)
	}

	// A DataView cannot use a database data source — a data view shows one object,
	// so Mendix offers only Context / Microflow / Nanoflow / Listen sources.
	// mxcli used to accept it: the modelsdk engine then errors "not yet supported —
	// rerun with legacy", and the legacy engine silently writes a Forms$DataViewSource
	// that MxBuild rejects with CE7007 "Selected value is not valid for entity". Flag
	// it here so the user gets an actionable message.
	// (An *association* DataView source IS valid — "data from context over an
	// association" — so it is deliberately NOT flagged here.)
	if strings.EqualFold(w.Type, "dataview") {
		if ds := w.GetDataSource(); ds != nil && ds.Type == "database" {
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET09",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: dataview `%s` cannot use a database data source (`from %s`) — a data view shows one object; use a microflow/nanoflow source (or a page parameter), or a list widget (listview/datagrid/gallery) for a collection",
					locationPrefix, w.Name, ds.Reference,
				),
			})
		}
	}

	return out
}

var templatePlaceholderRe = regexp.MustCompile(`\{(\d+)\}`)

// validateDynamicTextPlaceholders flags a dynamictext whose Content template has
// a placeholder index higher than the number of bound parameters. Parameter
// sources mirror buildDynamicTextV3: explicit ContentParams, a single Attribute
// binding, or a whole-content reference (which carries no {N}, so is irrelevant
// here).
func validateDynamicTextPlaceholders(w *ast.WidgetV3, locationPrefix string) *linter.Violation {
	if !strings.EqualFold(w.Type, "dynamictext") {
		return nil
	}
	content := w.GetContent()
	maxIdx := 0
	for _, m := range templatePlaceholderRe.FindAllStringSubmatch(content, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > maxIdx {
			maxIdx = n
		}
	}
	if maxIdx == 0 {
		return nil // no placeholders → nothing to orphan
	}
	params := 0
	if cp := w.GetContentParams(); len(cp) > 0 {
		params = len(cp)
	} else if w.GetAttribute() != "" {
		params = 1
	}
	if maxIdx <= params {
		return nil
	}
	return &linter.Violation{
		RuleID:   "MDL-WIDGET04",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"%s: widget `%s` (dynamictext) references template placeholder {%d} but only %d parameter(s) are bound — bind it with `Attribute: <attr>` or `ContentParams: [{%d} = <attr>]`. An orphaned placeholder crashes Studio Pro.",
			locationPrefix, w.Name, maxIdx, params, maxIdx,
		),
	}
}

// validatePluggableWidgetProperties checks every AST property key on a
// pluggable widget against the widget's def.json. Non-pluggable widgets are
// skipped (those go through the static builder which already validates props).
func validatePluggableWidgetProperties(w *ast.WidgetV3, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	def := lookupWidgetDef(w, registry)
	if def == nil {
		return nil
	}
	allowed, knownKeys := allowedWidgetProperties(def)
	dsKeys := datasourceTypedKeys(def)
	knownUnmapped := knownUnmappedProperties(def, allowed)

	var out []linter.Violation
	for key := range w.Properties {
		// Builtin property names (Label, Class, Visible, DataSource, …) are
		// MDL-recognized keywords that the widget engine routes via a
		// dedicated path rather than via propertyMappings. Accept them
		// universally so the validator doesn't false-positive on legitimate
		// MDL idioms like `Label: 'X'` on widgets whose def.json omits it.
		if isBuiltinPropName(key) {
			continue
		}
		lower := strings.ToLower(key)

		// A datasource-typed property must be supplied via the widget's
		// `datasource:` clause (which the engine reads), NOT as a named value
		// like `optionsSourceAssociationDataSource: Module.Entity` — that lands
		// in a different slot and is silently dropped, so the widget builds
		// without an entity (CE0642). Flag it instead of passing it (issue #643).
		if dsKeys[lower] {
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET05",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: widget `%s` (%s) property `%s` is datasource-typed — provide it via the widget `datasource:` clause (e.g. `datasource: database Module.Entity`); a value written as `%s: …` is not persisted",
					locationPrefix, w.Name, def.MDLName, key, key,
				),
			})
			continue
		}

		if allowed[lower] {
			continue
		}

		// Recognized real property the .def.json doesn't map to a write path:
		// don't reject it as unknown, but be honest that a non-default value
		// won't persist through mxcli yet (issue #643).
		if knownUnmapped[lower] {
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET06",
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf(
					"%s: widget `%s` (%s) property `%s` is recognized but not yet persisted by mxcli — a non-default value will be dropped; set it in Studio Pro if needed",
					locationPrefix, w.Name, def.MDLName, key,
				),
			})
			continue
		}

		suggestion := nearestKey(key, knownKeys)
		hint := ""
		if suggestion != "" {
			hint = fmt.Sprintf(" — did you mean `%s`?", suggestion)
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET01",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (%s) has no property `%s`%s",
				locationPrefix, w.Name, def.MDLName, key, hint,
			),
		})
	}
	return out
}

// datasourceTypedKeys returns the lowercased propertyKeys whose def.json mapping
// has operation "datasource" (across the top-level mappings and every mode).
// These must be authored via the widget `datasource:` clause, not by name.
func datasourceTypedKeys(def *WidgetDefinition) map[string]bool {
	out := make(map[string]bool)
	collect := func(ms []PropertyMapping) {
		for _, m := range ms {
			if m.Operation == "datasource" && m.PropertyKey != "" {
				out[strings.ToLower(m.PropertyKey)] = true
			}
		}
	}
	collect(def.PropertyMappings)
	for _, mode := range def.Modes {
		collect(mode.PropertyMappings)
	}
	return out
}

// knownUnmappedProperties returns the lowercased def.KnownProperties that are
// not already in the mapped/allowed set (so they get the WIDGET04 warning, not
// silently accepted).
func knownUnmappedProperties(def *WidgetDefinition, allowed map[string]bool) map[string]bool {
	out := make(map[string]bool, len(def.KnownProperties))
	for _, k := range def.KnownProperties {
		l := strings.ToLower(k)
		if !allowed[l] {
			out[l] = true
		}
	}
	return out
}

// lookupWidgetDef finds the WidgetDefinition for an AST widget node.
// Tries `Properties["WidgetType"]` (set when MDL uses the explicit
// `pluggablewidget 'widget.id'` form) and then the registry's MDL-name
// lookup (when MDL uses a keyword like `accordion` or `combobox`).
// Returns nil for static built-in keywords (textbox, dataview, etc.)
// that don't go through the pluggable engine.
func lookupWidgetDef(w *ast.WidgetV3, registry *WidgetRegistry) *WidgetDefinition {
	if w == nil {
		return nil
	}
	if id, ok := w.Properties["WidgetType"].(string); ok && id != "" {
		if def, ok := registry.GetByWidgetID(id); ok {
			return def
		}
	}
	if w.Type == "" {
		return nil
	}
	if def, ok := registry.Get(strings.ToUpper(w.Type)); ok {
		return def
	}
	return nil
}

// allowedWidgetProperties returns the set of property keys (lowercased) that
// the widget definition recognizes — propertyMappings + childSlots +
// objectLists + mode-specific mappings + universal infrastructure keys.
// Also returns the same set as a slice in original case (for "did you mean"
// suggestions).
func allowedWidgetProperties(def *WidgetDefinition) (map[string]bool, []string) {
	allowed := make(map[string]bool, 32)
	var keys []string
	add := func(k string) {
		if k == "" {
			return
		}
		l := strings.ToLower(k)
		if !allowed[l] {
			allowed[l] = true
			keys = append(keys, k)
		}
	}

	for k := range extraUniversalWidgetProperties {
		allowed[k] = true
	}

	for _, m := range def.PropertyMappings {
		add(m.PropertyKey)
		add(m.Source)
	}
	for _, m := range def.ChildSlots {
		add(m.PropertyKey)
	}
	for _, m := range def.ObjectLists {
		add(m.PropertyKey)
	}
	for _, mode := range def.Modes {
		for _, m := range mode.PropertyMappings {
			add(m.PropertyKey)
			add(m.Source)
		}
		for _, m := range mode.ChildSlots {
			add(m.PropertyKey)
		}
	}

	sort.Strings(keys)
	return allowed, keys
}

// nearestKey returns the candidate key nearest to `input` by Levenshtein
// distance, when the distance is small enough (≤ 3 or ≤ ceil(len/3)).
// Returns "" when no candidate is close enough.
func nearestKey(input string, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	lower := strings.ToLower(input)
	best := ""
	bestDist := -1
	limit := 3
	if len(input)/3 > limit {
		limit = len(input) / 3
	}
	for _, c := range candidates {
		d := levenshtein(lower, strings.ToLower(c))
		if d > limit {
			continue
		}
		if bestDist == -1 || d < bestDist {
			best = c
			bestDist = d
		}
	}
	return best
}

// levenshtein returns the edit distance between two strings.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
