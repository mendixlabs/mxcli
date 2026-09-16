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
	"github.com/mendixlabs/mxcli/mdl/types"
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
		registry.projectPath = projectPath
		// The validator and DESCRIBE WIDGET must agree about which properties a
		// widget has; they read different sources, so the definition is topped up
		// from the same .mpk DESCRIBE parses. See
		// widget_known_props_from_mpk.go for why this is not a list of nine.
		enrichKnownPropertiesFromMPK(registry, projectPath)
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
	return validateWidgetTreeIn(widgets, registry, locationPrefix, nil, nil, "", false)
}

// validateWidgetTreeIn is validateWidgetTree with the *parent* widget's
// object-list mappings keyed by container keyword (e.g. a chart's SERIES / LINE,
// a Maps DYNAMICMARKER). A child whose Type is one of those is an object-list
// item, not a built-in widget — its sub-properties (staticDataSource,
// staticXAttribute, …) are resolved by the object-list engine and written, so it
// must be exempt from the MDL-WIDGET07 "unrecognized property, silently dropped"
// warning. When the parent mapping is known, the child's enumeration
// sub-properties are validated against their member keys (MDL-WIDGET08). (9a)
func validateWidgetTreeIn(widgets []*ast.WidgetV3, registry *WidgetRegistry, locationPrefix string, parentObjectLists map[string]*ObjectListMapping, parent *ast.WidgetV3, contextVar string, contextKnown bool) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		mapping := parentObjectLists[strings.ToUpper(w.Type)]
		isObjectListItem := mapping != nil || isUniversalObjectListKeyword(w.Type)
		// Slice 0: is this a widget at all, and does the parent declare this
		// container? Both were previously left to `exec`.
		out = append(out, validateWidgetKind(w, registry, lookupWidgetDef(parent, registry), parentObjectLists, locationPrefix)...)
		// A keyword whose stored $Type Mendix no longer has. Unlike MDL-WIDGET25
		// this needs no project: the type is unknown to every Mendix version.
		out = append(out, validateRetiredWidgetKind(w, locationPrefix)...)
		out = append(out, validatePluggableWidgetProperties(w, registry, locationPrefix)...)
		// A repeatable property written as a property value — `attributes:
		// [(…)]` — which used to check clean, exec, and vanish (#999). Runs for
		// every widget kind and needs no definition: the SHAPE is wrong whatever
		// the widget declares.
		out = append(out, validateObjectEntryProperties(w, registry, locationPrefix)...)
		// #1062: an action slot holding something that is not an action, which
		// used to check clean, exec clean, build clean and render dead. Runs for
		// every widget kind and needs no definition, for the same reason as the
		// rule above: the SHAPE of the value is wrong whatever the widget is.
		out = append(out, validateWidgetActionSlot(w, locationPrefix)...)
		// #928: contentparams with no `{N}` placeholder to consume them.
		if lookupWidgetDef(w, registry) != nil {
			out = append(out, validatePluggableContentParams(w, locationPrefix)...)
		}
		out = append(out, validateWidgetVisibility(w, registry, locationPrefix)...)
		// An IMAGE with nothing to show — the default source needs an image
		// reference MDL cannot author. See validate_widget_image.go.
		out = append(out, validateImageSource(w, locationPrefix)...)
		out = append(out, validateStaticWidget(w, locationPrefix)...)
		out = append(out, validateDynamicTextFormatting(w, locationPrefix)...)
		out = append(out, validateDatasourceXPathAssociationEmpty(w, locationPrefix)...)
		out = append(out, validateComboBoxAssociation(w, locationPrefix)...)
		// A show_page argument naming anything but the context object is dropped.
		out = append(out, validateShowPageArguments(w, contextVar, contextKnown, locationPrefix)...)
		// Unknown-property warning applies only to built-in widgets; pluggable
		// widgets get the stricter def.json check (MDL-WIDGET01) above, and
		// object-list items are validated by the object-list engine.
		def := lookupWidgetDef(w, registry)
		// #2 from the view-entity-examples findings: a child the parent has
		// nowhere to put. MDL-WIDGET26 above covers a container KEYWORD in that
		// position; this covers a real widget, which resolves fine on its own and
		// so gets past every other rule. Needs the parent's definition, and stays
		// quiet without one for the same reason MDL-WIDGET26 does.
		out = append(out, validateUnroutedChildren(w, def, locationPrefix)...)
		// A generic widget type that resolved to nothing is already reported as
		// MDL-WIDGET25 (the kind is wrong). Validating its properties on top of
		// that says the kind is fine and the property is not, which points at
		// the wrong token — measured on `htmlelemnt frame (tagName: 'div')`,
		// which drew a `tagName` warning beside the real error. A built-in
		// (TypeIsGeneric false) keeps the check, since its properties are the
		// only thing that can be wrong about it.
		if def == nil && !isObjectListItem && !w.TypeIsGeneric {
			out = append(out, validateStaticWidgetUnknownProps(w, locationPrefix)...)
			// #928: `editable:` on a widget Mendix gives no editability — same
			// "silently dropped on write" family, but the flat property
			// allow-list cannot see it because it is type-agnostic.
			out = append(out, validateWidgetEditability(w, locationPrefix)...)
			// FINDINGS §21: the same blind spot for `onclick:`/`action:`, which
			// only three static widget kinds actually store.
			out = append(out, validateWidgetOnClick(w, locationPrefix)...)
		} else if def != nil {
			out = append(out, validatePluggableEditability(w, locationPrefix)...)
		}
		if mapping != nil {
			out = append(out, validateObjectListItemEnums(w, mapping, locationPrefix)...)
			// #931: a sub-property the widget's editorConfig hides must hold its
			// default; a non-default value there is CE0463.
			out = append(out, validateWidgetItemVisibility(parent, w, mapping, registry, locationPrefix)...)
		}
		// Reported once per grid, not once per column — see the rule's comment.
		out = append(out, validateDataGrid2ColumnNames(w, locationPrefix)...)
		if len(w.Children) > 0 {
			// A data-bound widget renames the context object for everything below it.
			childContextVar, childContextKnown := contextVar, contextKnown
			if ds := w.GetDataSource(); ds != nil {
				childContextVar, childContextKnown = contextVarFor(ds), true
			}
			out = append(out, validateWidgetTreeIn(w.Children, registry, locationPrefix, objectListMappingSet(def), w, childContextVar, childContextKnown)...)
		}
	}
	out = append(out, validateConsecutiveDynamicText(widgets, locationPrefix)...)
	return out
}

// validateDatasourceXPathAssociationEmpty flags `[Module.Association = empty]` in
// a widget's database datasource `where` clause — the page-datasource counterpart
// of the microflow-retrieve MDL047 check. Mendix XPath has no `= empty` for an
// association (CE0161); the nullability test is `not(Assoc/Target)`. (ledger #25,
// verification round: MDL047 originally covered only microflow retrieves.)
func validateDatasourceXPathAssociationEmpty(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	ds := w.GetDataSource()
	if ds == nil || ds.Where == "" {
		return nil
	}
	var out []linter.Violation
	for _, assoc := range xpathAssociationEmptyMatches(ds.Where) {
		out = append(out, linter.Violation{
			RuleID:   "MDL047",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` datasource constraint tests association `%s = empty`, which Mendix XPath does not support (CE0161 \"Error(s) in XPath constraint\") — `= empty` works on attributes, not associations",
				locationPrefix, w.Name, assoc),
			Suggestion: fmt.Sprintf("Test for the absence of the associated object with negation: `[not(%s/<Module.TargetEntity>)]`.", assoc),
		})
	}
	return out
}

// validateComboBoxAssociation flags an incomplete association-mode ComboBox.
// A ComboBox that binds an association (`Association:`) needs an options
// datasource (`DataSource:`, the entity whose objects populate the dropdown) and
// a caption attribute (`CaptionAttribute:`) — without a datasource the writer
// falls back to enumeration mode and drops the association, so the build fails
// CE0642 ("Property 'Attribute' is required"). Flag it at check time with the
// full, working syntax instead. (traceops #23)
func validateComboBoxAssociation(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "combobox") {
		return nil
	}
	if w.GetStringProp("Association") == "" {
		return nil
	}
	if w.GetDataSource() != nil {
		return nil // has an options datasource — association mode is complete enough
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET16",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"%s: combobox `%s` binds an association (`Association:`) but has no `datasource:` — Mendix drops the binding and fails the build with CE0642 (\"Property 'Attribute' is required\")",
			locationPrefix, w.Name),
		Suggestion: "Association mode needs the option list and a caption: `combobox " + w.Name + " (Association: Module.Ref, datasource: database Module.TargetEntity, CaptionAttribute: Name)`.",
	}}
}

// headingRenderModeRe matches the block-level dynamictext render modes H1–H6.
var headingRenderModeRe = regexp.MustCompile(`(?i)^h[1-6]$`)

// inlineDynamicText reports whether a widget is a dynamictext that renders
// INLINE — i.e. a `<span>`. Only the heading modes H1–H6 are genuinely
// block-level (`display: block`); `Text`/unset AND `Paragraph` both render as an
// inline `<span>` (verified on Mendix 11.12.1 + Atlas), so adjacent ones fuse.
// A heading followed by a subtitle therefore does NOT concatenate and must not
// be flagged. (ledger #27; #29 corrected the earlier mis-classification of
// Paragraph as block-level.)
func inlineDynamicText(w *ast.WidgetV3) bool {
	if w == nil || !strings.EqualFold(w.Type, "dynamictext") {
		return false
	}
	return !headingRenderModeRe.MatchString(w.GetRenderMode())
}

// validateConsecutiveDynamicText emits an advisory (MDL-WIDGET15) when two or
// more INLINE dynamictext widgets are direct siblings: Mendix renders a Text- or
// Paragraph-mode DynamicText inline (a `<span>`), so adjacent ones concatenate
// with no separator (`€ 310` + `7/24/2026` → `€ 3107/24/2026`). Only a heading
// render mode (H1–H6) is block-level and breaks the run. Info severity — it does
// not fail the build, it warns the author about a layout surprise. (ledger #27/#29)
func validateConsecutiveDynamicText(siblings []*ast.WidgetV3, locationPrefix string) []linter.Violation {
	run := 0
	for _, w := range siblings {
		if inlineDynamicText(w) {
			run++
		} else {
			run = 0
		}
		// Emit once, on the second inline dynamictext of a run, so a group of N
		// only warns once.
		if run == 2 {
			return []linter.Violation{{
				RuleID:   "MDL-WIDGET15",
				Severity: linter.SeverityInfo,
				Message: fmt.Sprintf(
					"%s: adjacent inline dynamictext widgets (RenderMode Text or Paragraph, both <span>) render with no separator, so their text concatenates. Merge them into one dynamictext with multiple content params, wrap each in its own container, or use a heading RenderMode (H1–H6, which is block-level). Note: Paragraph does NOT fix this — it also renders inline.",
					locationPrefix),
			}}
		}
	}
	return nil
}

// validateWidgetVisibility flags (MDL-WIDGET10) a property the user set on a
// pluggable widget that is hidden under that widget's current configuration — the
// widget's editorConfig.js suppresses it. A value equal to the property's default
// is merely ignored (warning); a non-default one is CE0463 and fails the build
// (error) — see hiddenPropertySeverity.
// Rules come from the widget's .def.json (propertyVisibility); for built-in
// widgets whose def carries none they are lifted on the fly from the installed
// .mpk's editorConfig.js (#574). Conservative by design: it only warns when the
// property is explicitly set AND the hiding condition's value is determinable
// from the MDL or a mapping default, so it never guesses.
func validateWidgetVisibility(w *ast.WidgetV3, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	def := lookupWidgetDef(w, registry)
	if def == nil {
		return nil
	}
	rules := visibilityRulesFor(def, registry)
	if len(rules) == 0 {
		return nil
	}
	values, explicit := widgetValueMap(w, def)
	defaults := widgetPropertyDefaults(registryProjectPath(registry), def.WidgetID)

	var out []linter.Violation
	for _, rule := range rules {
		// A nested rule is about a property of an object-list ITEM, not of the
		// widget — validateWidgetItemVisibility evaluates those, against the item.
		if rule.Nested() || rule.HiddenWhen == nil {
			continue
		}
		if !explicit[strings.ToLower(rule.PropertyKey)] {
			continue // user didn't set this property — nothing to warn about
		}
		// EVERY term of the rule's conjunction must hold; a rule read through
		// HiddenWhen alone over-fires (see WidgetVisibilityRule.And).
		fires, determinable := rule.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
			v, ok := values[strings.ToLower(c.PropertyKey)]
			return v, ok
		})
		if !determinable || !fires {
			continue // indeterminable, or the configuration does not hide it
		}
		out = append(out, hiddenPropertyViolation(locationPrefix, w.Name, def.MDLName, "", rule,
			values[strings.ToLower(rule.PropertyKey)],
			declaredDefault(defaults, def.PropertyMappings, nil, "", rule.PropertyKey),
			mappingOperationFor(def, rule.PropertyKey)))
	}
	return out
}

// widgetValueMap resolves a widget's current property values (keyed by
// lowercased widget property key), and reports which were set explicitly in the
// MDL (vs. filled from a mapping default). MDL keywords are resolved to widget
// keys via the def's property mappings (source/aliases), plus any direct
// key-named MDL property. Selection values are canonicalised (None/Single/Multi)
// to match how editorConfig conditions compare them.
func widgetValueMap(w *ast.WidgetV3, def *WidgetDefinition) (values map[string]string, explicit map[string]bool) {
	values = map[string]string{}
	explicit = map[string]bool{}

	mappings := append([]PropertyMapping(nil), def.PropertyMappings...)
	for _, m := range def.Modes {
		mappings = append(mappings, m.PropertyMappings...)
	}
	// An MDL source keyword shared by SEVERAL properties does not name any one of
	// them. File Uploader 2.5.0 routes both `associatedFiles` and
	// `associatedImages` from `DataSource:`, and `uploadMode` decides which one
	// Studio Pro shows — so a script writing `DataSource:` once has named the
	// keyword, not the hidden property, and must not be reported as having set
	// it (#956). The value is still recorded, because conditions read from it.
	sharedSource := map[string]int{}
	for _, m := range mappings {
		if m.Source != "" {
			sharedSource[m.Source]++
		}
	}
	for _, m := range mappings {
		key := strings.ToLower(m.PropertyKey)
		val, set := "", false
		viaSharedSource := false
		if m.Source != "" {
			if v, ok := lookupWidgetProp(w, m.Source); ok {
				val, set = v, true
				viaSharedSource = sharedSource[m.Source] > 1
			}
		}
		if !set {
			for _, a := range m.MdlAliases {
				if v, ok := lookupWidgetProp(w, a); ok {
					val, set = v, true
					break
				}
			}
		}
		if !set {
			if v, ok := lookupWidgetProp(w, m.PropertyKey); ok {
				val, set = v, true
			}
		}
		if set {
			if !viaSharedSource {
				explicit[key] = true
			}
		} else if m.Default != "" {
			val = m.Default
		} else if m.Operation == "primitive" && m.Value != "" {
			// A primitive mapping carries the widget XML's defaultValue in Value
			// (Default is only populated for selections), and the builder WRITES
			// that value when the script names nothing — so reading it here is
			// not a guess about the widget's configuration, it is the
			// configuration. Without it every rule keyed on an unnamed
			// enumeration is indeterminable and silently does not fire.
			//
			// Measured on File Uploader 2.5.0: `uploadMode` defaults to "files",
			// which hides `associatedImages`. With the condition unknown, the
			// `DataSource:` clause fanned out into BOTH datasource properties —
			// and a value in a pruned slot is CE0463 on every page carrying the
			// widget, with nothing having warned (#956). This is the same shape
			// as the selection case below, which was fixed first because
			// DataGrid2 was the widget then under test.
			val = m.Value
		} else if m.Operation == "selection" {
			// An omitted `Selection:` is written as None. That is the builder's
			// own behaviour rather than a guess — the stored itemSelection reads
			// `Selection None` on a DataGrid2 whose script never named it — and
			// the .mpk declares no defaultValue for a selection property, so
			// without this the condition is indeterminable and every rule keyed
			// on a selection silently does not fire. That includes
			// `onSelectionChange`, whose action then reaches the model as CE0463
			// with nothing having warned (#956).
			val = "None"
		}
		if val != "" {
			if m.Operation == "selection" {
				val = canonicalSelection(val)
			}
			values[key] = val
		}
	}
	// Direct MDL properties named after a widget key (not covered by a mapping).
	for k, raw := range w.Properties {
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

// lookupWidgetProp reads a widget property from the MDL by name (case-insensitive)
// as a string, reporting whether it was present.
func lookupWidgetProp(w *ast.WidgetV3, name string) (string, bool) {
	for k, v := range w.Properties {
		if strings.EqualFold(k, name) {
			return stringifyPropValue(v), true
		}
	}
	return "", false
}

// stringifyPropValue renders an MDL property value as a plain string for
// condition evaluation (strings, bools, ints).
func stringifyPropValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%g", x)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

// canonicalSelection normalises a selection value to the PascalCase form
// (None/Single/Multi) editorConfig conditions compare against.
func canonicalSelection(v string) string {
	switch strings.ToLower(v) {
	case "single":
		return "Single"
	case "multi", "multiple":
		return "Multi"
	case "none":
		return "None"
	}
	return v
}

// visibilityCondWord renders a visibility condition's operator+value as English
// for the MDL-WIDGET10 message.
func visibilityCondWord(c *types.WidgetVisibilityCondition) string {
	switch c.Operator {
	case "eq":
		return fmt.Sprintf("is %q", c.Value)
	case "ne":
		return fmt.Sprintf("is not %q", c.Value)
	case "truthy":
		return "is enabled"
	case "falsy":
		return "is disabled"
	default:
		return "matches"
	}
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
		"DataSource", "Attribute", "Binds", "Action", "OnClick", "OnChange", "Placeholder",
		"Caption", "Label",
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
		// button icon-collection reference (issue #602)
		"Icon",
		// fragment / building-block sentinel-internal keys (USE_FRAGMENT /
		// USE_BUILDING_BLOCK), consumed by the expander, never serialized
		"Args", "DataSourceOverride", "ActionOverride",
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
	for _, key := range sortedPropertyKeys(w) {
		if isKnownStaticWidgetProp(key) {
			continue
		}
		// Dynamic-text format keys placed at the widget level are reported by
		// MDL-WIDGET18 (with actionable move-into-format-block guidance); don't
		// also warn about them here.
		if paramFormatKeys[strings.ToLower(key)] {
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

// paramFormatKeys are the recognized keys inside a dynamic-text parameter format
// block, e.g. `{1} = Amount (decimalPrecision: 2, groupDigits: true)`. They map to
// the Mendix ClientTemplateParameter FormattingInfo fields.
var paramFormatKeys = map[string]bool{
	"decimalprecision": true, "groupdigits": true,
	"dateformat": true, "customdateformat": true, "enumformat": true,
}

var paramFormatKeyList = []string{"decimalPrecision", "groupDigits", "dateFormat", "customDateFormat", "enumFormat"}
var paramDateFormats = map[string]bool{"date": true, "datetime": true, "time": true, "custom": true}
var paramEnumFormats = map[string]bool{"text": true, "image": true}

// validateDynamicTextFormatting (MDL-WIDGET18) checks per-parameter formatting on
// dynamic text. It (1) turns a widget-level format property (e.g. a bare
// `decimalPrecision:` on the widget) into an actionable ERROR pointing at the
// ContentParams format block — instead of the old silent drop (ledger #75) — and
// (2) validates the keys/values inside each format block so typos and bad enum
// values fail at `check` time rather than building wrong.
func validateDynamicTextFormatting(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	var out []linter.Violation

	// (1) Format keys placed at the widget level are silently dropped on write —
	// formatting is per-parameter. Flag them with the correct location.
	if strings.EqualFold(w.Type, "dynamictext") {
		for _, key := range sortedPropertyKeys(w) {
			if paramFormatKeys[strings.ToLower(key)] {
				out = append(out, linter.Violation{
					RuleID:   "MDL-WIDGET18",
					Severity: linter.SeverityError,
					Message: fmt.Sprintf(
						"%s: widget `%s`: `%s` is a per-parameter format, not a widget property — put it in the ContentParams format block, e.g. `ContentParams: [{1} = Attr format (%s: <value>)]`. A widget-level `%s` is dropped on write.",
						locationPrefix, w.Name, key, strings.ToLower(key), key,
					),
				})
			}
		}
	}

	// (2) Validate the keys/values inside each parameter format block.
	for _, p := range w.GetContentParams() {
		if p.Format == nil {
			continue
		}
		for _, fp := range p.Format.Props {
			if !paramFormatKeys[fp.Key] {
				hint := ""
				if s := nearestKey(fp.Key, paramFormatKeyList); s != "" {
					hint = fmt.Sprintf(" — did you mean `%s`?", s)
				}
				out = append(out, violation18(locationPrefix, w,
					fmt.Sprintf("unknown format key `%s`%s", fp.Key, hint)))
				continue
			}
			switch fp.Key {
			case "decimalprecision":
				if n, err := strconv.Atoi(fp.Value); err != nil || n < 0 {
					out = append(out, violation18(locationPrefix, w,
						fmt.Sprintf("decimalPrecision must be a non-negative integer, got `%s`", fp.Value)))
				}
			case "groupdigits":
				if !strings.EqualFold(fp.Value, "true") && !strings.EqualFold(fp.Value, "false") {
					out = append(out, violation18(locationPrefix, w,
						fmt.Sprintf("groupDigits must be true or false, got `%s`", fp.Value)))
				}
			case "dateformat":
				if !paramDateFormats[strings.ToLower(fp.Value)] {
					out = append(out, violation18(locationPrefix, w,
						fmt.Sprintf("dateFormat must be one of Date, DateTime, Time, Custom, got `%s`", fp.Value)))
				}
			case "enumformat":
				if !paramEnumFormats[strings.ToLower(fp.Value)] {
					out = append(out, violation18(locationPrefix, w,
						fmt.Sprintf("enumFormat must be Text or Image, got `%s`", fp.Value)))
				}
			}
		}
		// customDateFormat is only meaningful with dateFormat: Custom.
		if _, hasCustom := p.Format.Get("customdateformat"); hasCustom {
			if df, _ := p.Format.Get("dateformat"); !strings.EqualFold(df, "Custom") {
				out = append(out, violation18(locationPrefix, w,
					"customDateFormat requires `dateFormat: Custom`"))
			}
		}
	}
	return out
}

func violation18(locationPrefix string, w *ast.WidgetV3, msg string) linter.Violation {
	return linter.Violation{
		RuleID:   "MDL-WIDGET18",
		Severity: linter.SeverityError,
		Message:  fmt.Sprintf("%s: widget `%s`: %s", locationPrefix, w.Name, msg),
	}
}

// validateStaticWidget checks value-level constraints on built-in (non-pluggable)
// widgets that the grammar can't express and that otherwise fail silently or at
// build time rather than at `mxcli check` time.
// validateConsumableConditional (MDL-WIDGET19) rejects a `Visible:` / `Editable:`
// value that no builder can consume, so an expression the visitor failed to turn
// into VisibleIf/EditableIf fails the command instead of vanishing from the page.
//
// The bracket form is routed to VisibleIf/EditableIf by the visitor; the plain
// slot then holds only a static form, which pages.StaticVisibleExpression reads
// from a bool or a string. Anything else is parse residue — the `[...]` matched
// the generic property-value alternative rather than an xpathConstraint — and the
// builder's `else if` simply doesn't fire. That is the silent-drop mechanism
// behind issue #852, where `trim(…)`/`length(…)` were unparseable as conditional
// expressions and the whole property disappeared; a missing Visible defaults to
// "always visible", so nothing downstream could notice.
//
// The grammar fix removes the known trigger. This rule is the general guard: the
// next function name promoted to a lexer token fails loudly here instead.
func validateConsumableConditional(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, p := range []struct{ plain, routed string }{
		{"Visible", "VisibleIf"},
		{"Editable", "EditableIf"},
	} {
		if _, routed := w.Properties[p.routed]; routed {
			continue
		}
		v, present := w.Properties[p.plain]
		if !present || v == nil {
			continue
		}
		switch v.(type) {
		case bool, string:
			continue // a static form StaticVisibleExpression consumes
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET19",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (%s) has a `%s` value that could not be parsed as a conditional expression "+
					"and would be dropped on write (leaving the widget unconditionally %s) — "+
					"check the expression inside `%s: [ ... ]`",
				locationPrefix, w.Name, w.Type, strings.ToLower(p.plain),
				map[string]string{"Visible": "visible", "Editable": "editable"}[p.plain],
				strings.ToLower(p.plain),
			),
		})
	}
	return out
}

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

	out = append(out, validateConsumableConditional(w, locationPrefix)...)

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

	// A widget EXPRESSION property (DynamicClasses / VisibleIf / EditableIf) that
	// walks an association fails the build with CE0117 — Mendix client-side
	// expressions cannot traverse associations (only data bindings such as
	// contentparams/attribute can). This is statically checkable and easy to trip
	// over, because a data binding on the SAME widget can traverse the same
	// association legitimately. (findings #4)
	out = append(out, validateWidgetExpressionAssociations(w, locationPrefix)...)

	// A contentparams/captionparams value is a DATA BINDING (an attribute path),
	// not a client-side expression: mxcli stores an unquoted value as an attribute
	// name, so a function call like `formatDateTime($obj/Date, 'd MMM')` is written
	// as a bogus attribute and Studio Pro rejects the page with CE1613 "attribute
	// no longer exists". Catch it at check time. (ledger finding #26)
	out = append(out, validateTemplateParamExpressions(w, locationPrefix)...)

	return out
}

// exprAssociationStepRe matches an association step inside an expression: a
// slash-delimited, module-qualified name (e.g. `/Feedline.Article_Source/`). A
// plain attribute access (`$obj/Slug`) is a single unqualified segment and does
// not match; a qualified enum literal (`Mod.Enum.Value`) has no leading slash and
// does not match either.
var exprAssociationStepRe = regexp.MustCompile(`/([A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*)/`)

// expressionWidgetProps are the widget property keys whose value is a client-side
// Mendix expression (never a data binding). An association step in any of these
// is always CE0117.
var expressionWidgetProps = []string{"DynamicClasses", "VisibleIf", "EditableIf"}

// validateWidgetExpressionAssociations flags an association traversal inside an
// expression-typed widget property (MDL-WIDGET13).
func validateWidgetExpressionAssociations(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, key := range expressionWidgetProps {
		expr, ok := lookupWidgetProp(w, key)
		if !ok || expr == "" {
			continue
		}
		if m := exprAssociationStepRe.FindStringSubmatch(expr); m != nil {
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET13",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: widget `%s` property `%s` expression traverses association `%s` — Mendix expressions cannot follow associations (CE0117). Bind the value via a data binding (contentparams/attribute) or precompute it onto the bound entity (e.g. a calculated attribute).",
					locationPrefix, w.Name, key, m[1],
				),
			})
		}
	}
	return out
}

// templateParamExprRe detects a client-side expression where an attribute-path
// data binding is expected. A binding is a path of identifier segments joined by
// `/` or `.` (optionally `$`-prefixed): `$obj/Date`, `Order_Customer/Name`. An
// expression carries a function call `foo(` or an arithmetic/comparison operator,
// none of which can appear in an attribute path.
var templateParamExprRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*\s*\(|[+\-*<>=!]`)

// validateTemplateParamExpressions flags a client expression supplied to a
// contentparams/captionparams slot (MDL-WIDGET14). mxcli stores an unquoted
// value as an attribute path, so an expression like
// `formatDateTime($obj/Date, 'd MMM')` would be written as a bogus attribute
// name and Studio Pro rejects the page. A quoted value is a legal string literal
// and is left alone.
//
// The refusal is right; the reason this rule used to give was not. It said a
// template parameter "is a data binding … not an expression", which is a claim
// about MENDIX and it is false: Studio Pro's Edit Template Parameter dialog
// offers "Parameter type: Value | Expression", and the Expression form has its
// own editor, variable list and wizard —
// `formatDecimal($currentObject/Score)` is a perfectly valid parameter there.
// The metamodel agrees: Pages$ClientTemplateParameter carries Expression beside
// AttributeRef and SourceVariable.
//
// So the honest message is that MXCLI cannot author that form yet, not that the
// platform forbids it. Telling a user to precompute a calculated attribute when
// Studio Pro offers the thing they asked for sends them to do unnecessary
// modelling. Authoring syntax for it is tracked separately.
func validateTemplateParamExpressions(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	check := func(slot string, params []ast.ParamAssignmentV3) {
		for _, p := range params {
			val, ok := p.Value.(string)
			if !ok || val == "" {
				continue
			}
			// A quoted string literal is a valid contentparams value.
			if strings.HasPrefix(val, "'") || strings.HasPrefix(val, "\"") {
				continue
			}
			if !templateParamExprRe.MatchString(val) {
				continue
			}
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET14",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: widget `%s` %s value `%s` looks like an expression, and MDL cannot author an expression-typed template parameter yet — an unquoted value is stored as an attribute path, so this would be written as a bogus attribute name. Mendix DOES support it: Studio Pro's Edit Template Parameter dialog has a `Value | Expression` choice. Set it there, or bind an attribute path (`$obj/Attr`) or a quoted string literal here.",
					locationPrefix, w.Name, slot, val,
				),
			})
		}
	}
	check("contentparams", w.GetContentParams())
	check("captionparams", w.GetCaptionParams())
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
	actionKeys := actionStorageKeys(def)
	knownUnmapped := knownUnmappedProperties(def, allowed)

	var out []linter.Violation
	for _, key := range sortedPropertyKeys(w) {
		// Builtin property names (Label, Class, Visible, DataSource, …) are
		// MDL-recognized keywords that the widget engine routes via a
		// dedicated path rather than via propertyMappings. Accept them
		// universally so the validator doesn't false-positive on legitimate
		// MDL idioms like `Label: 'X'` on widgets whose def.json omits it.
		// A builtin name the engine has no route for on *this* widget is worse
		// than an unknown one: it is accepted here, dropped on write, and shows
		// up as a required-property error from MxBuild with nothing pointing at
		// the cause.
		if right, wrong := misusedBuiltinProperty(def.WidgetID, key); wrong {
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET17",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: widget `%s` (%s) has no `%s` property — the value is dropped on write and "+
						"MxBuild then reports the property as missing. Use `%s:` instead",
					locationPrefix, w.Name, def.MDLName, key, right),
			})
			continue
		}
		if isBuiltinPropName(key) {
			continue
		}
		lower := strings.ToLower(key)

		// Named datasource properties are supported when the value parsed as a
		// real datasource expression (`people: microflow M.DS_People`). A scalar
		// that merely names an entity is still invalid: it cannot be persisted as
		// a datasource and caused the silent drop/CE0642 from issue #643.
		if dsKeys[lower] {
			if raw, ok := lookupProperty(w.Properties, key); ok {
				if _, ok := raw.(*ast.DataSourceV3); ok {
					continue
				}
			}
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET05",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: widget `%s` (%s) property `%s` is datasource-typed — use a datasource expression such as `%s: database Module.Entity` or the generic `datasource:` clause; the supplied scalar value cannot be persisted",
					locationPrefix, w.Name, def.MDLName, key, key,
				),
			})
			continue
		}

		// An action slot's storage key is not the MDL spelling — name the one
		// that is, rather than leaving the author to guess from a fuzzy match.
		if src, ok := actionKeys[lower]; ok {
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET01",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: widget `%s` (%s) property `%s` is the widget's internal storage name and is not written from MDL — use `%s:` instead",
					locationPrefix, w.Name, def.MDLName, key, src,
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
// addMappingNames records the MDL names a PropertyMapping is authorable under.
//
// For most operations that is both the widget's own storage key and the engine's
// source name. An `action` mapping is the exception: resolveMapping reads the
// fixed AST slot (`w.GetAction()` / `w.GetOnChange()`), so ONLY the source name
// (`Action`/`OnClick`/`OnChange`) reaches the writer. Allowing the storage key
// would accept `onChangeEvent: …` on a Combobox and drop it on write — the
// silent-drop class FINDINGS #14 was about. It is reported by
// actionStorageKeys() instead, with the spelling that works.
func addMappingNames(add func(string), m PropertyMapping) {
	if !readsFixedASTSlot(m) {
		add(m.PropertyKey)
	}
	add(m.Source)
	// The aliases are the names people are TOLD to write, so they have to be
	// accepted here — the builder already resolves them (widget_engine.go), and
	// the knownProperties set in widget_defs.go already walks them. Leaving them
	// out made the validator the odd one out of three readers of the same
	// def.json: `ValueAttribute: Total` on a PieChart persisted correctly and
	// was still reported as MDL-WIDGET01 "has no property", which — because exec
	// refuses a script with errors — blocked the page from being written at all.
	for _, a := range m.MdlAliases {
		add(a)
	}
}

// readsFixedASTSlot reports whether an operation's value is resolved from a
// dedicated AST accessor rather than from a property looked up by name.
//
// resolveMapping switches on the mapping's Source, and for these operations it
// reads a fixed slot — `w.GetOnChange()` for an action, the association binding
// for an association — so a script naming the widget's own storage key is
// accepted by check and written by nothing.
//
// Measured on a Combobox against Mendix 11.13, which is why this is a list of
// two rather than "every operation with a Source": `attributeAssociation:` does
// not persist while `Association:` does, and `onChangeEvent:` does not persist
// while `OnChange:` does — but `optionsSourceAssociationCaptionAttribute:` DOES
// persist, so excluding every storage key would reject working syntax. Add an
// operation here only after checking the written document, not from the shape of
// the mapping.
// An action mapping with NO Source is a NAMED slot: resolveMapping reads it from
// the AST property called by the mapping's own PropertyKey, so that key is the
// authorable name rather than an internal one (#956).
func readsFixedASTSlot(m PropertyMapping) bool {
	switch m.Operation {
	case "association":
		return true
	case "action":
		return m.Source != ""
	}
	return false
}

// actionStorageKeys maps each action mapping's storage key to the MDL name that
// actually writes it, so the validator can say "use OnChange" rather than only
// "unknown property".
func actionStorageKeys(def *WidgetDefinition) map[string]string {
	out := make(map[string]string)
	collect := func(ms []PropertyMapping) {
		for _, m := range ms {
			if readsFixedASTSlot(m) && m.PropertyKey != "" && m.Source != "" {
				out[strings.ToLower(m.PropertyKey)] = m.Source
			}
		}
	}
	collect(def.PropertyMappings)
	for _, mode := range def.Modes {
		collect(mode.PropertyMappings)
	}
	return out
}

// Datasource properties may be authored by name when their value is a real
// *ast.DataSourceV3. The key set lets validation reject scalar lookalikes.
//
// A datasource mapping's MdlAliases are authorable spellings too (resolved by
// namedDataSourceValue in widget_engine.go exactly like the PropertyKey
// itself), so they belong in this set with the same lowercased normalization
// as the PropertyKey — otherwise a scalar written under an alias (e.g.
// `ItemsSource: 'x'`) skips the MDL-WIDGET05 datasource-typed check entirely
// and falls through to the generic property handling below.
func datasourceTypedKeys(def *WidgetDefinition) map[string]bool {
	out := make(map[string]bool)
	collect := func(ms []PropertyMapping) {
		for _, m := range ms {
			if m.Operation != "datasource" {
				continue
			}
			if m.PropertyKey != "" {
				out[strings.ToLower(m.PropertyKey)] = true
			}
			for _, alias := range m.MdlAliases {
				out[strings.ToLower(alias)] = true
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
		addMappingNames(add, m)
	}
	for _, m := range def.ChildSlots {
		add(m.PropertyKey)
	}
	for _, m := range def.ObjectLists {
		add(m.PropertyKey)
	}
	for _, mode := range def.Modes {
		for _, m := range mode.PropertyMappings {
			addMappingNames(add, m)
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

// validateDataGrid2ColumnNames warns (MDL-WIDGET16) that the names written on a
// pluggable DataGrid 2's columns are discarded, and says what each column will
// actually be addressable as.
//
// Mendix stores no name on a DataGrid 2 column. Its schema has no name or
// identifier key at column level — the only human-facing label is `header`, the
// caption — so the name in `column colLabel (attribute: Label, …)` reaches
// DataGridColumnSpec, which has no field for it, and is dropped. Everything
// downstream then addresses the column by a *derived* name: the bound attribute
// for an attribute column, the sanitized caption otherwise, `colN` as a last
// resort.
//
// The consequence is not obvious from the MDL. An author who wrote `colLabel`
// reaches for `ALTER PAGE … ON dg1.colLabel` and gets "column not found" for a
// column they just named, while `describe page` shows a name they never wrote.
//
// **One violation per grid, listing its columns.** The first version emitted one
// per column, which a real project (mxcli-dbreplication, finding F6) reported as
// 44 infos saying the same thing. It is one fact about the grid; repeating it
// per column buries the rest of the report without adding information.
//
// It warns rather than rejects: the name is harmless, it reads as documentation
// in the source, and rejecting it would break every existing script — mxcli's
// own doctype tests name every column. What the author needs is to know which
// name addresses it.
func validateDataGrid2ColumnNames(grid *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if grid == nil || !strings.EqualFold(grid.Type, "DATAGRID") {
		return nil
	}
	var renamed []string
	for _, child := range grid.Children {
		if child == nil || !strings.EqualFold(child.Type, "COLUMN") || child.Name == "" {
			continue
		}
		addressable := derivedDataGrid2ColumnName(child)
		if addressable == "" || strings.EqualFold(addressable, child.Name) {
			continue
		}
		renamed = append(renamed, fmt.Sprintf("%s → %s", child.Name, addressable))
	}
	if len(renamed) == 0 {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET16",
		Severity: linter.SeverityInfo,
		Message: fmt.Sprintf(
			"%s: DataGrid 2 stores no column names, so the names on %s are dropped on write. "+
				"Address these columns by their derived name in ALTER PAGE (attribute columns "+
				"key on the bound attribute, others on the caption), and expect DESCRIBE to "+
				"show it: %s.",
			locationPrefix, grid.Name, strings.Join(renamed, ", ")),
	}}
}

// derivedDataGrid2ColumnName mirrors the name derivation the writer and the page
// mutator apply, so the warning names the same string ALTER will accept.
// Deliberately conservative: when it cannot tell (no attribute, no caption — the
// colN case, which depends on position) it returns "" and nothing is reported,
// because a wrong name in the message would be worse than none.
func derivedDataGrid2ColumnName(w *ast.WidgetV3) string {
	if attr := w.GetAttribute(); attr != "" {
		parts := strings.Split(attr, ".")
		return parts[len(parts)-1]
	}
	if caption := w.GetCaption(); caption != "" {
		sanitized := strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
				return r
			}
			return '_'
		}, caption)
		return strings.Trim(sanitized, "_")
	}
	return ""
}

// builtinPropertyMisuse names builtin MDL properties that are wrong on a
// specific widget, and the one that is right.
//
// isBuiltinPropName accepts Label/Caption/Class/… on every widget, deliberately:
// the engine routes them through dedicated paths rather than through a def's
// propertyMappings, so validating them against the def would false-positive on
// ordinary MDL. The cost is that a builtin the engine does *not* route for a
// given widget is accepted and silently dropped.
//
// That bit a real project: `combobox cb (Association: …, Caption: Name)` passes
// `check`, executes, loses the caption, and fails the build with
//
//	[error] [CE0642] "Property 'Caption' is required." at Combo box 'cb'
//
// The working spelling is `CaptionAttribute:`, which round-trips — so the author
// was one property name away and concluded the feature was unusable
// (mxcli-owid, finding #38).
//
// This is an explicit list rather than something inferred. Whether a builtin is
// routed for a widget lives in the engine's own dispatch, not in the .def.json —
// the combobox def declares optionsSourceAssociationCaption{Type,Expression} and
// nothing called `Caption` — so inferring it would mean reimplementing that
// dispatch here and getting it wrong in the other direction. Add a row when a
// case is measured, and cite the build error in the commit.
var builtinPropertyMisuse = map[string]map[string]string{
	"com.mendix.widget.web.combobox.Combobox": {
		"Caption": "CaptionAttribute",
	},
}

// misusedBuiltinProperty reports the right property name when key is a builtin
// that this widget does not route, matching case-insensitively the way the rest
// of the property lookup does.
func misusedBuiltinProperty(widgetID, key string) (string, bool) {
	byName, ok := builtinPropertyMisuse[widgetID]
	if !ok {
		return "", false
	}
	for wrong, right := range byName {
		if strings.EqualFold(wrong, key) {
			return right, true
		}
	}
	return "", false
}

// mappingOperationFor returns the def's operation for a property key, or "" when
// the widget declares no mapping for it. Used to tell an action slot apart from a
// valued property when judging how bad a hidden-property violation is.
func mappingOperationFor(def *WidgetDefinition, propertyKey string) string {
	scan := func(ms []PropertyMapping) string {
		for _, m := range ms {
			if strings.EqualFold(m.PropertyKey, propertyKey) {
				return m.Operation
			}
		}
		return ""
	}
	if op := scan(def.PropertyMappings); op != "" {
		return op
	}
	for _, mode := range def.Modes {
		if op := scan(mode.PropertyMappings); op != "" {
			return op
		}
	}
	for _, ol := range def.ObjectLists {
		for _, ip := range ol.ItemProperties {
			if strings.EqualFold(ip.PropertyKey, propertyKey) {
				return ip.Operation
			}
		}
	}
	return ""
}

// sortedPropertyKeys returns a widget's property keys in a stable order.
//
// A validator that appends one violation per property key was iterating the map
// directly, so `mxcli check` printed the same warnings in a different order from
// one run to the next. Measured before the fix: two runs of the same binary over
// mdl-examples/ disagreed on 11 of 515 scripts.
//
// Nothing was wrong with the diagnostics — but "the output is stable" is what
// makes a before/after diff of `check` usable as a measurement, and it was not.
// This surfaced while diffing check output across the corpus to size the
// grammar change for slices 2-3: the noise floor of the tool was larger than
// the signal being looked for.
//
// The three call sites are the ones that emit PER KEY (MDL-WIDGET07, WIDGET17,
// WIDGET18). Two other loops over w.Properties do a case-insensitive LOOKUP and
// break on the first hit; those are left alone, since they are only
// order-sensitive when a widget carries two keys differing solely in case, and
// picking either is equally correct.
func sortedPropertyKeys(w *ast.WidgetV3) []string {
	if w == nil {
		return nil
	}
	out := make([]string, 0, len(w.Properties))
	for k := range w.Properties {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
