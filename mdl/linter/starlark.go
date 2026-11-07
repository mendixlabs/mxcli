// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"

	"github.com/mendixlabs/mxcli/mdl/exprcheck"
)

// StarlarkRule is a lint rule implemented in Starlark.
type StarlarkRule struct {
	id           string
	name         string
	description  string
	severity     Severity
	category     string
	path         string
	ctx          *LintContext
	checkFn      starlark.Callable
	globals      starlark.StringDict
	options      map[string]any
	requiredMode CatalogMode
	// optionWarnings collects lint-config.yaml values this run could not use;
	// Check reports each as a warning after the rule has run on its default.
	optionWarnings []string
}

// RequiredCatalogMode reports the catalog depth this rule needs (CatalogRequirer).
func (r *StarlarkRule) RequiredCatalogMode() CatalogMode { return r.requiredMode }

// Configure stores options from the lint config file so Starlark rules can read them via get_option().
func (r *StarlarkRule) Configure(options map[string]any) {
	r.options = options
}

// ID returns the rule ID.
func (r *StarlarkRule) ID() string { return r.id }

// Name returns the rule name.
func (r *StarlarkRule) Name() string { return r.name }

// Description returns the rule description.
func (r *StarlarkRule) Description() string { return r.description }

// DefaultSeverity returns the rule severity.
func (r *StarlarkRule) DefaultSeverity() Severity { return r.severity }

// Category returns the rule category.
func (r *StarlarkRule) Category() string { return r.category }

// Check executes the Starlark check function and returns violations.
func (r *StarlarkRule) Check(ctx *LintContext) []Violation {
	r.ctx = ctx
	r.optionWarnings = nil

	// Create a new thread for execution
	thread := &starlark.Thread{
		Name: r.id,
		Print: func(_ *starlark.Thread, msg string) {
			fmt.Println(msg)
		},
	}

	// Call the check function
	result, err := starlark.Call(thread, r.checkFn, nil, nil)
	if err != nil {
		return append(r.optionWarningViolations(), ruleFailureViolation(r.id, err))
	}

	// Convert result to violations
	return append(r.convertViolations(result), r.optionWarningViolations()...)
}

// optionWarningViolations reports the configured option values the rule could
// not use. They are RuleFailure findings: a misconfigured lint-config.yaml says
// nothing about the project, so it must not move the score.
func (r *StarlarkRule) optionWarningViolations() []Violation {
	var out []Violation
	for _, msg := range r.optionWarnings {
		out = append(out, Violation{
			RuleID:      r.id,
			Severity:    SeverityWarning,
			Message:     msg,
			Suggestion:  "Fix the value in lint-config.yaml; until then the rule runs with its built-in default.",
			RuleFailure: true,
		})
	}
	return out
}

// missingStructAttrRe matches the evaluator's message for reading a field a
// struct does not have — "entity struct has no .document_noun attribute",
// optionally followed by a "(did you mean .x?)" hint. The evaluator flattens
// starlark.NoSuchAttrError into a plain error, so the message is all there is.
var missingStructAttrRe = regexp.MustCompile(`(?:\S+ )?struct has no \.[A-Za-z_][A-Za-z0-9_]* attribute.*`)

// ruleFailureViolation reports a rule that failed to run.
//
// A rule that reads a struct field this binary does not expose is almost
// always a rule written for a newer mxcli: the shipped rules gain fields with
// the catalog (document_noun arrived after v0.24.0, and QUAL004, CONV010 and
// CUSTOM002 all crashed on it). That is reported as what it is — an info line
// naming the cause — rather than as a project error. Any other failure is a
// broken rule and stays an error. Both are RuleFailure: neither says anything
// about the project, so neither may move its score (ako/mxcli#952).
func ruleFailureViolation(ruleID string, err error) Violation {
	msg := err.Error()
	var evalErr *starlark.EvalError
	if errors.As(err, &evalErr) {
		msg = evalErr.Msg // without the backtrace
	}
	if detail := missingStructAttrRe.FindString(msg); detail != "" {
		return Violation{
			RuleID:      ruleID,
			Severity:    SeverityInfo,
			Message:     fmt.Sprintf("rule %s needs a newer mxcli (%s)", ruleID, detail),
			Suggestion:  "Update mxcli to the version that wrote this project's lint rules; if the rule is your own, check the field name against the write-lint-rules skill.",
			RuleFailure: true,
		}
	}
	return Violation{
		RuleID:      ruleID,
		Severity:    SeverityError,
		Message:     fmt.Sprintf("Starlark rule error: %v", err),
		RuleFailure: true,
	}
}

// convertViolations converts a Starlark list to Go violations.
func (r *StarlarkRule) convertViolations(result starlark.Value) []Violation {
	var violations []Violation

	list, ok := result.(*starlark.List)
	if !ok {
		return violations
	}

	iter := list.Iterate()
	defer iter.Done()

	var v starlark.Value
	for iter.Next(&v) {
		if viol := r.convertViolation(v); viol != nil {
			violations = append(violations, *viol)
		}
	}

	return violations
}

// convertViolation converts a Starlark struct to a Go Violation.
func (r *StarlarkRule) convertViolation(v starlark.Value) *Violation {
	s, ok := v.(*starlarkstruct.Struct)
	if !ok {
		return nil
	}

	viol := &Violation{
		RuleID:   r.id,
		Severity: r.severity,
	}

	if msg, err := s.Attr("message"); err == nil {
		if str, ok := msg.(starlark.String); ok {
			viol.Message = string(str)
		}
	}

	if loc, err := s.Attr("location"); err == nil {
		if locStruct, ok := loc.(*starlarkstruct.Struct); ok {
			if module, err := locStruct.Attr("module"); err == nil {
				if str, ok := module.(starlark.String); ok {
					viol.Location.Module = string(str)
				}
			}
			if docType, err := locStruct.Attr("document_type"); err == nil {
				if str, ok := docType.(starlark.String); ok {
					viol.Location.DocumentType = string(str)
				}
			}
			if docName, err := locStruct.Attr("document_name"); err == nil {
				if str, ok := docName.(starlark.String); ok {
					viol.Location.DocumentName = string(str)
				}
			}
			if docID, err := locStruct.Attr("document_id"); err == nil {
				if str, ok := docID.(starlark.String); ok {
					viol.Location.DocumentID = string(str)
				}
			}
		}
	}

	if sug, err := s.Attr("suggestion"); err == nil {
		if str, ok := sug.(starlark.String); ok {
			viol.Suggestion = string(str)
		}
	}

	return viol
}

// LoadStarlarkRule loads a Starlark rule from a file.
// communityBuiltins need REFRESH CATALOG COMMUNITIES (the graph_* tables).
var communityBuiltins = []string{
	"cycles", "module_cycles", "module_dependencies", "community_of", "layer_of",
	"centrality", "god_nodes", "integration_surface",
}

// fullBuiltins read tables only REFRESH CATALOG FULL writes: refs, widgets,
// xpath_expressions, activities, permissions and strings. Under the default fast build
// they answer [] with no warning, so a rule calling one checked nothing.
// starlark_catalog_mode_guard_test.go builds a fixture both ways and fails on
// any builtin whose answer differs and is not listed here.
var fullBuiltins = []string{
	"refs_to", "refs_from",
	"widgets", "xpath_expressions", "activities_for",
	"permissions", "permissions_for",
	"strings",
}

// fullFields are struct fields a fast build leaves at their zero value
// (page.widget_count and snippet.widget_count count widgets only a full build
// extracts; a page's and a layout's own class_name, style and dynamic_classes
// are read from the raw unit a full build loads). They are detected as an
// attribute access, `.name`; a match on another struct's field of the same
// name only builds a deeper catalog than needed.
var fullFields = []string{"widget_count", "class_name", "style", "dynamic_classes"}

// detectRequiredCatalogMode infers the catalog depth a Starlark rule needs by
// scanning its source for calls to the graph / full-only builtins and reads of
// full-only fields.
func detectRequiredCatalogMode(src string) CatalogMode {
	for _, b := range communityBuiltins {
		if strings.Contains(src, b+"(") {
			return CatalogCommunities
		}
	}
	for _, b := range fullBuiltins {
		if strings.Contains(src, b+"(") {
			return CatalogFull
		}
	}
	for _, f := range fullFields {
		if strings.Contains(src, "."+f) {
			return CatalogFull
		}
	}
	return CatalogFast
}

// requiresFromGlobal reads an explicit `REQUIRES` declaration — a string or list
// of strings, each "refs"/"full" (→ full) or "communities"/"graph" (→ communities).
// It lets a rule author raise the auto-detected mode (e.g. when a helper hides the
// builtin call from the source scan).
func requiresFromGlobal(globals starlark.StringDict) (CatalogMode, bool) {
	v, ok := globals["REQUIRES"]
	if !ok {
		return CatalogFast, false
	}
	mode := CatalogFast
	apply := func(s string) {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "communities", "graph":
			if CatalogCommunities > mode {
				mode = CatalogCommunities
			}
		case "full", "refs":
			if CatalogFull > mode {
				mode = CatalogFull
			}
		}
	}
	switch val := v.(type) {
	case starlark.String:
		apply(string(val))
	case *starlark.List:
		it := val.Iterate()
		defer it.Done()
		var x starlark.Value
		for it.Next(&x) {
			if s, ok := x.(starlark.String); ok {
				apply(string(s))
			}
		}
	default:
		return CatalogFast, false
	}
	return mode, true
}

func LoadStarlarkRule(path string) (*StarlarkRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read rule file: %w", err)
	}

	rule := &StarlarkRule{
		path:     path,
		severity: SeverityWarning,
		category: "custom",
	}

	// Build predeclared environment
	predeclared := rule.buildPredeclared()

	// Parse and execute the file
	thread := &starlark.Thread{
		Name: filepath.Base(path),
	}

	// Recursion is on: an expression tree from parse_xpath is walked
	// recursively — the write-lint-rules skill's own example does — and with
	// Starlark's default that failed as "function called recursively"
	// (ako/mxcli#1439). The interpreter still stops a runaway rule at 100,000
	// frames with "Starlark stack overflow", which is a rule failure, not a crash.
	opts := syntax.LegacyFileOptions()
	opts.Recursion = true
	globals, err := starlark.ExecFileOptions(opts, thread, path, data, predeclared)
	if err != nil {
		return nil, fmt.Errorf("failed to execute Starlark file: %w", err)
	}

	rule.globals = globals

	// Determine the catalog depth this rule needs. Auto-detect from the builtins
	// the source calls (so existing rules Just Work), and let an explicit
	// REQUIRES global override/raise it. Without this, a rule using refs_to /
	// cycles / … under `mxcli lint` silently returns empty (issue #721).
	rule.requiredMode = detectRequiredCatalogMode(string(data))
	if m, ok := requiresFromGlobal(globals); ok && m > rule.requiredMode {
		rule.requiredMode = m
	}

	// Extract metadata
	if id, ok := globals["RULE_ID"]; ok {
		if str, ok := id.(starlark.String); ok {
			rule.id = string(str)
		}
	}
	if rule.id == "" {
		// Use filename as fallback
		rule.id = strings.TrimSuffix(filepath.Base(path), ".star")
	}

	if name, ok := globals["RULE_NAME"]; ok {
		if str, ok := name.(starlark.String); ok {
			rule.name = string(str)
		}
	}
	if rule.name == "" {
		rule.name = rule.id
	}

	if desc, ok := globals["DESCRIPTION"]; ok {
		if str, ok := desc.(starlark.String); ok {
			rule.description = string(str)
		}
	}

	if sev, ok := globals["SEVERITY"]; ok {
		if str, ok := sev.(starlark.String); ok {
			rule.severity = ParseSeverity(string(str))
		}
	}

	if cat, ok := globals["CATEGORY"]; ok {
		if str, ok := cat.(starlark.String); ok {
			rule.category = string(str)
		}
	}

	// Get the check function
	checkVal, ok := globals["check"]
	if !ok {
		return nil, fmt.Errorf("rule must define a check() function")
	}

	checkFn, ok := checkVal.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("check must be a callable function")
	}

	rule.checkFn = checkFn

	return rule, nil
}

// buildPredeclared creates the predeclared environment for Starlark rules.
func (r *StarlarkRule) buildPredeclared() starlark.StringDict {
	return starlark.StringDict{
		// Query functions
		"entities":              starlark.NewBuiltin("entities", r.builtinEntities),
		"microflows":            starlark.NewBuiltin("microflows", r.builtinMicroflows),
		"java_actions":          starlark.NewBuiltin("java_actions", r.builtinJavaActions),
		"documentable_elements": starlark.NewBuiltin("documentable_elements", r.builtinDocumentableElements),
		"documents":             starlark.NewBuiltin("documents", r.builtinDocuments),
		"navigation_targets":    starlark.NewBuiltin("navigation_targets", r.builtinNavigationTargets),
		"pages":                 starlark.NewBuiltin("pages", r.builtinPages),
		"enumerations":          starlark.NewBuiltin("enumerations", r.builtinEnumerations),
		"constants":             starlark.NewBuiltin("constants", r.builtinConstants),
		"constant_values":       starlark.NewBuiltin("constant_values", r.builtinConstantValues),
		"widgets":               starlark.NewBuiltin("widgets", r.builtinWidgets),
		"refs_to":               starlark.NewBuiltin("refs_to", r.builtinRefsTo),
		"refs_from":             starlark.NewBuiltin("refs_from", r.builtinRefsFrom),
		"attributes_for":        starlark.NewBuiltin("attributes_for", r.builtinAttributesFor),
		"scheduled_events":      starlark.NewBuiltin("scheduled_events", r.builtinScheduledEvents),
		"queues":                starlark.NewBuiltin("queues", r.builtinQueues),

		// Catalog tables with a typed projection (mendixlabs/mxcli#1265).
		"associations":              starlark.NewBuiltin("associations", r.builtinAssociations),
		"entity_event_handlers":     starlark.NewBuiltin("entity_event_handlers", r.builtinEntityEventHandlers),
		"navigation_menu_items":     starlark.NewBuiltin("navigation_menu_items", r.builtinNavigationMenuItems),
		"jar_dependencies":          starlark.NewBuiltin("jar_dependencies", r.builtinJarDependencies),
		"strings":                   starlark.NewBuiltin("strings", r.builtinStrings),
		"layouts":                   starlark.NewBuiltin("layouts", r.builtinLayouts),
		"published_rest_operations": starlark.NewBuiltin("published_rest_operations", r.builtinPublishedRestOperations),
		"modules":                   starlark.NewBuiltin("modules", r.builtinModules),

		// Graph-analysis facts (populated by `refresh catalog communities`).
		"community_of":         starlark.NewBuiltin("community_of", r.builtinCommunityOf),
		"layer_of":             starlark.NewBuiltin("layer_of", r.builtinLayerOf),
		"cycles":               starlark.NewBuiltin("cycles", r.builtinCycles),
		"module_cycles":        starlark.NewBuiltin("module_cycles", r.builtinModuleCycles),
		"module_dependencies":  starlark.NewBuiltin("module_dependencies", r.builtinModuleDependencies),
		"centrality":           starlark.NewBuiltin("centrality", r.builtinCentrality),
		"god_nodes":            starlark.NewBuiltin("god_nodes", r.builtinGodNodes),
		"integration_surface":  starlark.NewBuiltin("integration_surface", r.builtinIntegrationSurface),
		"permissions":          starlark.NewBuiltin("permissions", r.builtinPermissions),
		"permissions_for":      starlark.NewBuiltin("permissions_for", r.builtinPermissionsFor),
		"snippets":             starlark.NewBuiltin("snippets", r.builtinSnippets),
		"database_connections": starlark.NewBuiltin("database_connections", r.builtinDatabaseConnections),
		"rest_clients":         starlark.NewBuiltin("rest_clients", r.builtinRestClients),
		"rest_operations":      starlark.NewBuiltin("rest_operations", r.builtinRestOperations),
		"activities_for":       starlark.NewBuiltin("activities_for", r.builtinActivitiesFor),

		// Project-level queries
		"user_roles":       starlark.NewBuiltin("user_roles", r.builtinUserRoles),
		"module_roles":     starlark.NewBuiltin("module_roles", r.builtinModuleRoles),
		"role_mappings":    starlark.NewBuiltin("role_mappings", r.builtinRoleMappings),
		"project_security": starlark.NewBuiltin("project_security", r.builtinProjectSecurity),
		"languages":        starlark.NewBuiltin("languages", r.builtinLanguages),
		"project_settings": starlark.NewBuiltin("project_settings", r.builtinProjectSettings),

		// XPath / expression analysis
		"xpath_expressions": starlark.NewBuiltin("xpath_expressions", r.builtinXPathExpressions),
		"parse_xpath":       starlark.NewBuiltin("parse_xpath", r.builtinParseXPath),

		// Violation helpers
		"violation": starlark.NewBuiltin("violation", builtinViolation),
		"location":  starlark.NewBuiltin("location", builtinLocation),

		// String utilities
		"is_pascal_case": starlark.NewBuiltin("is_pascal_case", builtinIsPascalCase),
		"is_camel_case":  starlark.NewBuiltin("is_camel_case", builtinIsCamelCase),
		"matches":        starlark.NewBuiltin("matches", builtinMatches),
		"find_all":       starlark.NewBuiltin("find_all", builtinFindAll),

		// Struct constructor (from starlarkstruct)
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),

		// Options access: get_option("key") or get_option("key", default)
		"get_option": starlark.NewBuiltin("get_option", r.builtinGetOption),
		// get_list_option("key", default): a list of strings, given in YAML
		// as a list or as comma-separated text
		"get_list_option": starlark.NewBuiltin("get_list_option", r.builtinGetListOption),
	}
}

// builtinEntities returns an iterator over entities.
func (r *StarlarkRule) builtinEntities(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var entities []starlark.Value
	for entity := range r.ctx.Entities() {
		entities = append(entities, entityToStarlark(entity))
	}

	return starlark.NewList(entities), nil
}

// builtinMicroflows returns an iterator over microflows.
func (r *StarlarkRule) builtinMicroflows(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var microflows []starlark.Value
	for mf := range r.ctx.Microflows() {
		microflows = append(microflows, microflowToStarlark(mf))
	}

	return starlark.NewList(microflows), nil
}

// builtinJavaActions returns an iterator over Java actions. Each carries its
// parameters, so a rule reporting an undocumented parameter can name its action
// without a second lookup.
func (r *StarlarkRule) builtinJavaActions(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var actions []starlark.Value
	for ja := range r.ctx.JavaActions() {
		actions = append(actions, javaActionToStarlark(ja))
	}

	return starlark.NewList(actions), nil
}

// builtinDocumentableElements returns every element that can carry
// documentation, across all document types, as a uniform (kind, name,
// qualified_name, module_name, description) projection.
//
// One builtin rather than nineteen: a rule sweeping for missing documentation
// wants "every document", and a new Mendix document type should be covered by
// adding a row to documentableSources, not by writing another builtin and
// remembering to call it.
func (r *StarlarkRule) builtinDocumentableElements(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var out []starlark.Value
	for d := range r.ctx.DocumentableElements() {
		out = append(out, starlarkstruct.FromStringDict(starlark.String("documentable"), starlark.StringDict{
			"kind":           starlark.String(d.Kind),
			"name":           starlark.String(d.Name),
			"qualified_name": starlark.String(d.QualifiedName),
			"module_name":    starlark.String(d.ModuleName),
			"description":    starlark.String(d.Description),
		}))
	}

	return starlark.NewList(out), nil
}

// builtinDocuments returns every element of the App Explorer tree as a uniform
// (kind, name, qualified_name, module_name, folder) projection.
//
// The companion to documentable_elements for rules about where a document
// LIVES rather than what it says: it covers microflows and Java actions, which
// that projection deliberately omits, and it carries `folder`, which no
// per-kind builtin exposes uniformly.
func (r *StarlarkRule) builtinDocuments(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var out []starlark.Value
	for d := range r.ctx.Documents() {
		out = append(out, starlarkstruct.FromStringDict(starlark.String("document"), starlark.StringDict{
			"kind":           starlark.String(d.Kind),
			"name":           starlark.String(d.Name),
			"qualified_name": starlark.String(d.QualifiedName),
			"module_name":    starlark.String(d.ModuleName),
			"folder":         starlark.String(d.Folder),
		}))
	}

	return starlark.NewList(out), nil
}

// builtinNavigationTargets returns every page a navigation profile routes to —
// the profile home page, role-specific home pages, and menu item targets.
//
// Navigation was reachable only from the Go rules (through the reader's
// GetNavigation), so no Starlark rule could ask which pages a user can actually
// reach. Login and not-found pages are excluded: the platform routes to those.
func (r *StarlarkRule) builtinNavigationTargets(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var out []starlark.Value
	for t := range r.ctx.NavigationTargets() {
		out = append(out, starlarkstruct.FromStringDict(starlark.String("navigation_target"), starlark.StringDict{
			"profile": starlark.String(t.Profile),
			"kind":    starlark.String(t.Kind),
			"role":    starlark.String(t.Role),
			"caption": starlark.String(t.Caption),
			"page":    starlark.String(t.Page),
		}))
	}

	return starlark.NewList(out), nil
}

// builtinPages returns an iterator over pages.
func (r *StarlarkRule) builtinPages(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var pages []starlark.Value
	for page := range r.ctx.Pages() {
		pages = append(pages, pageToStarlark(page))
	}

	return starlark.NewList(pages), nil
}

// builtinEnumerations returns an iterator over enumerations.
func (r *StarlarkRule) builtinEnumerations(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var enums []starlark.Value
	for enum := range r.ctx.Enumerations() {
		enums = append(enums, enumerationToStarlark(enum))
	}

	return starlark.NewList(enums), nil
}

// builtinConstants returns an iterator over constants.
func (r *StarlarkRule) builtinConstants(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var constants []starlark.Value
	for c := range r.ctx.Constants() {
		constants = append(constants, constantToStarlark(c))
	}

	return starlark.NewList(constants), nil
}

// builtinConstantValues returns the constant values the run configurations set
// (ako/mxcli#1453). A configuration that does not override a constant has no
// entry: it runs on the constant's default_value.
func (r *StarlarkRule) builtinConstantValues(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(b.Name(), args, kwargs); err != nil {
		return nil, err
	}
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}
	var out []starlark.Value
	for c := range r.ctx.ConstantValues() {
		out = append(out, starlarkstruct.FromStringDict(starlark.String("constant_value"), starlark.StringDict{
			"constant":      starlark.String(c.Constant),
			"configuration": starlark.String(c.Configuration),
			"value":         starlark.String(c.Value),
		}))
	}
	return starlark.NewList(out), nil
}

// builtinProjectSettings returns the project-wide settings a guideline rule
// checks (ako/mxcli#1453), or None when no project is open. Like languages(),
// it reads the settings from the project rather than a catalog table, so it
// needs no particular catalog depth; a failed read fails the rule rather than
// answering defaults a rule would report as the project's.
func (r *StarlarkRule) builtinProjectSettings(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(b.Name(), args, kwargs); err != nil {
		return nil, err
	}
	if r.ctx == nil || r.ctx.Reader() == nil {
		return starlark.None, nil
	}
	ps, err := r.ctx.Reader().GetProjectSettings()
	if err != nil {
		return nil, fmt.Errorf("%s: reading project settings: %w", b.Name(), err)
	}
	if ps == nil {
		return starlark.None, nil
	}
	d := starlark.StringDict{
		"default_language":        starlark.String(""),
		"use_oql_version_2":       starlark.Bool(false),
		"java_version":            starlark.String(""),
		"hash_algorithm":          starlark.String(""),
		"default_time_zone":       starlark.String(""),
		"rounding_mode":           starlark.String(""),
		"decimal_scale":           starlark.MakeInt(0),
		"optimistic_locking":      starlark.Bool(false),
		"foreign_key_constraints": starlark.Bool(false),
		"web_client":              starlark.String(""),
	}
	if m := ps.Model; m != nil {
		d["use_oql_version_2"] = starlark.Bool(m.UseOQLVersion2)
		d["java_version"] = starlark.String(m.JavaVersion)
		d["hash_algorithm"] = starlark.String(m.HashAlgorithm)
		d["default_time_zone"] = starlark.String(m.DefaultTimeZoneCode)
		d["rounding_mode"] = starlark.String(m.RoundingMode)
		d["decimal_scale"] = starlark.MakeInt(m.DecimalScale)
		d["optimistic_locking"] = starlark.Bool(m.EnableDataStorageOptimisticLocking)
		d["foreign_key_constraints"] = starlark.Bool(m.UseDatabaseForeignKeyConstraints)
	}
	if ps.Language != nil {
		d["default_language"] = starlark.String(ps.Language.DefaultLanguageCode)
	}
	if ps.WebUI != nil {
		// Web UI → UseOptimizedClient: "Yes" is the React client, "No" the
		// classic one, "MigrationMode" Mendix's mixed mode.
		d["web_client"] = starlark.String(ps.WebUI.UseOptimizedClient)
	}
	return starlarkstruct.FromStringDict(starlark.String("project_settings"), d), nil
}

// builtinWidgets returns an iterator over widgets.
func (r *StarlarkRule) builtinWidgets(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var widgets []starlark.Value
	for widget := range r.ctx.Widgets() {
		widgets = append(widgets, widgetToStarlark(widget))
	}

	return starlark.NewList(widgets), nil
}

// builtinAttributesFor returns the attributes for a given entity.
func (r *StarlarkRule) builtinAttributesFor(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var entityQualifiedName starlark.String
	if err := starlark.UnpackArgs("attributes_for", args, kwargs,
		"entity_qualified_name", &entityQualifiedName,
	); err != nil {
		return nil, err
	}

	var attrs []starlark.Value
	for attr := range r.ctx.AttributesFor(string(entityQualifiedName)) {
		attrs = append(attrs, attributeToStarlark(attr))
	}

	return starlark.NewList(attrs), nil
}

// builtinRefsTo returns references to a given target name.
func (r *StarlarkRule) builtinRefsTo(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var targetName starlark.String

	if err := starlark.UnpackArgs("refs_to", args, kwargs, "target_name", &targetName); err != nil {
		return nil, err
	}

	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	refs := r.ctx.FindReferences(string(targetName))
	var result []starlark.Value
	for _, ref := range refs {
		result = append(result, referenceToStarlark(ref))
	}

	return starlark.NewList(result), nil
}

// builtinPermissions returns all permissions across all element types.
func (r *StarlarkRule) builtinPermissions(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var result []starlark.Value
	for p := range r.ctx.Permissions() {
		result = append(result, allPermissionToStarlark(p))
	}

	return starlark.NewList(result), nil
}

// builtinPermissionsFor returns the permissions for a given entity.
func (r *StarlarkRule) builtinPermissionsFor(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var entityQualifiedName starlark.String
	if err := starlark.UnpackArgs("permissions_for", args, kwargs,
		"entity_qualified_name", &entityQualifiedName,
	); err != nil {
		return nil, err
	}

	var perms []starlark.Value
	for perm := range r.ctx.PermissionsFor(string(entityQualifiedName)) {
		perms = append(perms, permissionToStarlark(perm))
	}

	return starlark.NewList(perms), nil
}

// builtinSnippets returns an iterator over snippets.
func (r *StarlarkRule) builtinSnippets(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var snippets []starlark.Value
	for s := range r.ctx.Snippets() {
		snippets = append(snippets, snippetToStarlark(s))
	}

	return starlark.NewList(snippets), nil
}

// builtinDatabaseConnections returns an iterator over database connections.
func (r *StarlarkRule) builtinDatabaseConnections(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var connections []starlark.Value
	for dc := range r.ctx.DatabaseConnections() {
		connections = append(connections, databaseConnectionToStarlark(dc))
	}

	return starlark.NewList(connections), nil
}

// builtinRestClients returns all consumed REST service documents.
func (r *StarlarkRule) builtinRestClients(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var clients []starlark.Value
	for rc := range r.ctx.RestClients() {
		clients = append(clients, restClientToStarlark(rc))
	}

	return starlark.NewList(clients), nil
}

// builtinRestOperations returns all consumed REST operations.
func (r *StarlarkRule) builtinRestOperations(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var operations []starlark.Value
	for ro := range r.ctx.RestOperations() {
		operations = append(operations, restOperationToStarlark(ro))
	}

	return starlark.NewList(operations), nil
}

// builtinScheduledEvents returns all scheduled events.
func (r *StarlarkRule) builtinScheduledEvents(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var result []starlark.Value
	for se := range r.ctx.ScheduledEvents() {
		result = append(result, scheduledEventToStarlark(se))
	}

	return starlark.NewList(result), nil
}

// builtinGetOption returns a rule option value from the lint config file.
// Signature: get_option(key, default=None)
//
// A value that cannot be represented in Starlark is reported as a warning and
// the default is returned — never a silent None (mendixlabs/mxcli#1509).
func (r *StarlarkRule) builtinGetOption(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key starlark.String
	var defaultVal starlark.Value = starlark.None
	if err := starlark.UnpackArgs("get_option", args, kwargs,
		"key", &key,
		"default?", &defaultVal,
	); err != nil {
		return nil, err
	}
	v, ok := r.options[string(key)]
	if !ok || v == nil {
		return defaultVal, nil
	}
	sv, err := goValueToStarlark(v)
	if err != nil {
		r.warnOption(string(key), v, err.Error())
		return defaultVal, nil
	}
	return sv, nil
}

// builtinGetListOption returns a rule option as a list of strings. The option
// may be a YAML list of strings or one comma-separated string; parts are
// trimmed and empty parts dropped. An unset option returns the default; any
// other value is reported as a warning and also returns the default.
// Signature: get_list_option(key, default=None)
func (r *StarlarkRule) builtinGetListOption(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key starlark.String
	var defaultVal starlark.Value = starlark.None
	if err := starlark.UnpackArgs("get_list_option", args, kwargs,
		"key", &key,
		"default?", &defaultVal,
	); err != nil {
		return nil, err
	}
	v, ok := r.options[string(key)]
	if !ok || v == nil {
		return defaultVal, nil
	}
	var items []starlark.Value
	switch val := v.(type) {
	case string:
		for _, part := range strings.Split(val, ",") {
			if part = strings.TrimSpace(part); part != "" {
				items = append(items, starlark.String(part))
			}
		}
	case []any:
		for _, e := range val {
			s, isStr := e.(string)
			if !isStr {
				r.warnOption(string(key), v, "must be a list of strings or a comma-separated string")
				return defaultVal, nil
			}
			if s = strings.TrimSpace(s); s != "" {
				items = append(items, starlark.String(s))
			}
		}
	default:
		r.warnOption(string(key), v, "must be a list of strings or a comma-separated string")
		return defaultVal, nil
	}
	return starlark.NewList(items), nil
}

// warnOption records a configured option value the rule cannot use.
func (r *StarlarkRule) warnOption(key string, v any, problem string) {
	r.optionWarnings = append(r.optionWarnings, fmt.Sprintf(
		"lint-config.yaml: rules.%s.options.%s %s (got %T %v); the rule ran with its default",
		r.id, key, problem, v, v))
}

// goValueToStarlark converts a Go value (from YAML unmarshaling) to a Starlark
// value. Lists and string-keyed maps convert recursively; anything else is an
// error, so the caller can say which option was dropped instead of handing
// the rule a None it cannot tell from "unset".
func goValueToStarlark(v any) (starlark.Value, error) {
	switch val := v.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(val), nil
	case int:
		return starlark.MakeInt(val), nil
	case int64:
		return starlark.MakeInt64(val), nil
	case uint64:
		return starlark.MakeUint64(val), nil
	case float64:
		return starlark.Float(val), nil
	case string:
		return starlark.String(val), nil
	case []any:
		items := make([]starlark.Value, 0, len(val))
		for _, e := range val {
			sv, err := goValueToStarlark(e)
			if err != nil {
				return nil, err
			}
			items = append(items, sv)
		}
		return starlark.NewList(items), nil
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		d := starlark.NewDict(len(val))
		for _, k := range keys {
			sv, err := goValueToStarlark(val[k])
			if err != nil {
				return nil, err
			}
			if err := d.SetKey(starlark.String(k), sv); err != nil {
				return nil, err
			}
		}
		return d, nil
	default:
		return nil, fmt.Errorf("has a value of a type rules cannot read")
	}
}

// builtinActivitiesFor returns the activities for a given microflow.
func (r *StarlarkRule) builtinActivitiesFor(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	// nested=False (the default) returns the top level only, as before the
	// catalog had loop bodies, so existing rules keep their counts;
	// nested=True adds the objects inside loops (mendixlabs/mxcli#1266).
	var microflowQualifiedName starlark.String
	var nested bool
	if err := starlark.UnpackArgs("activities_for", args, kwargs,
		"microflow_qualified_name", &microflowQualifiedName,
		"nested?", &nested,
	); err != nil {
		return nil, err
	}

	var activities []starlark.Value
	for a := range r.ctx.ActivitiesIn(string(microflowQualifiedName), nested) {
		activities = append(activities, activityToStarlark(a))
	}

	return starlark.NewList(activities), nil
}

// builtinUserRoles returns all user roles from project security.
func (r *StarlarkRule) builtinUserRoles(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	roles := r.ctx.UserRoles()
	var result []starlark.Value
	for _, ur := range roles {
		result = append(result, userRoleToStarlark(ur))
	}

	return starlark.NewList(result), nil
}

// builtinModuleRoles returns all module roles from the catalog.
func (r *StarlarkRule) builtinModuleRoles(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var result []starlark.Value
	for mr := range r.ctx.ModuleRoles() {
		result = append(result, moduleRoleToStarlark(mr))
	}

	return starlark.NewList(result), nil
}

// builtinRoleMappings returns all user role to module role mappings.
func (r *StarlarkRule) builtinRoleMappings(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var result []starlark.Value
	for rm := range r.ctx.RoleMappings() {
		result = append(result, roleMappingToStarlark(rm))
	}

	return starlark.NewList(result), nil
}

// builtinProjectSecurity returns project security settings as a Starlark struct.
func (r *StarlarkRule) builtinProjectSecurity(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.None, nil
	}

	reader := r.ctx.Reader()
	if reader == nil {
		return starlark.None, nil
	}

	ps, err := reader.GetProjectSecurity()
	if err != nil || ps == nil {
		return starlark.None, nil
	}

	// Build password_policy sub-struct
	ppDict := starlark.StringDict{
		"min_length":         starlark.MakeInt(0),
		"require_digit":      starlark.Bool(false),
		"require_mixed_case": starlark.Bool(false),
		"require_symbol":     starlark.Bool(false),
	}
	if ps.PasswordPolicy != nil {
		ppDict["min_length"] = starlark.MakeInt(ps.PasswordPolicy.MinimumLength)
		ppDict["require_digit"] = starlark.Bool(ps.PasswordPolicy.RequireDigit)
		ppDict["require_mixed_case"] = starlark.Bool(ps.PasswordPolicy.RequireMixedCase)
		ppDict["require_symbol"] = starlark.Bool(ps.PasswordPolicy.RequireSymbol)
	}

	return starlarkstruct.FromStringDict(starlark.String("project_security"), starlark.StringDict{
		"security_level":      starlark.String(ps.SecurityLevel),
		"enable_demo_users":   starlark.Bool(ps.EnableDemoUsers),
		"enable_guest_access": starlark.Bool(ps.EnableGuestAccess),
		"check_security":      starlark.Bool(ps.CheckSecurity),
		"strict_mode":         starlark.Bool(ps.StrictMode),
		"anonymous_user_role": starlark.String(ps.GuestUserRole),
		// The administrator account's name and user role. Its password is
		// deliberately not exposed: a rule has no business reading it, and a
		// violation message or debug print would put it in a report.
		"admin_user_name": starlark.String(ps.AdminUserName),
		"admin_user_role": starlark.String(ps.AdminUserRole),
		"password_policy": starlarkstruct.FromStringDict(starlark.String("password_policy"), ppDict),
	}), nil
}

// builtinLanguages returns the languages enabled in the project settings
// (mendixlabs/mxcli#1306). strings() cannot answer this: it has a row for every
// stored translation, and a project carries texts in languages it never
// enabled. A failed settings read fails the rule rather than answering [],
// which a per-language rule would report as clean.
func (r *StarlarkRule) builtinLanguages(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(b.Name(), args, kwargs); err != nil {
		return nil, err
	}
	if r.ctx == nil || r.ctx.Reader() == nil {
		return starlark.NewList(nil), nil
	}
	ps, err := r.ctx.Reader().GetProjectSettings()
	if err != nil {
		return nil, fmt.Errorf("%s: reading project settings: %w", b.Name(), err)
	}
	if ps == nil || ps.Language == nil {
		return starlark.NewList(nil), nil
	}
	var out []starlark.Value
	for _, l := range ps.Language.Languages {
		if l.Code == "" {
			continue
		}
		out = append(out, starlarkstruct.FromStringDict(starlark.String("language"), starlark.StringDict{
			"code":               starlark.String(l.Code),
			"is_default":         starlark.Bool(l.Code == ps.Language.DefaultLanguageCode),
			"check_completeness": starlark.Bool(l.CheckCompleteness),
		}))
	}
	return starlark.NewList(out), nil
}

// entityToStarlark converts an Entity to a Starlark struct.
func entityToStarlark(e Entity) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("entity"), starlark.StringDict{
		"id":                    starlark.String(e.ID),
		"name":                  starlark.String(e.Name),
		"qualified_name":        starlark.String(e.QualifiedName),
		"module_name":           starlark.String(e.ModuleName),
		"folder":                starlark.String(e.Folder),
		"entity_type":           starlark.String(e.EntityType),
		"description":           starlark.String(e.Description),
		"generalization":        starlark.String(e.Generalization),
		"attribute_count":       starlark.MakeInt(e.AttributeCount),
		"access_rule_count":     starlark.MakeInt(e.AccessRuleCount),
		"validation_rule_count": starlark.MakeInt(e.ValidationRuleCount),
		"has_event_handlers":    starlark.Bool(e.HasEventHandlers),
		"is_external":           starlark.Bool(e.IsExternal),
		"has_created_date":      starlark.Bool(e.HasCreatedDate),
		"has_changed_date":      starlark.Bool(e.HasChangedDate),
		"has_owner":             starlark.Bool(e.HasOwner),
		"has_changed_by":        starlark.Bool(e.HasChangedBy),
	})
}

// microflowToStarlark converts a Microflow to a Starlark struct.
func microflowToStarlark(mf Microflow) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("microflow"), starlark.StringDict{
		"id":              starlark.String(mf.ID),
		"name":            starlark.String(mf.Name),
		"qualified_name":  starlark.String(mf.QualifiedName),
		"module_name":     starlark.String(mf.ModuleName),
		"folder":          starlark.String(mf.Folder),
		"microflow_type":  starlark.String(mf.MicroflowType),
		"description":     starlark.String(mf.Description),
		"return_type":     starlark.String(mf.ReturnType),
		"parameter_count": starlark.MakeInt(mf.ParameterCount),
		"activity_count":  starlark.MakeInt(mf.ActivityCount),
		// Loop bodies included, at any depth; activity_count counts a loop as one.
		"total_activity_count": starlark.MakeInt(mf.TotalActivityCount),
		"complexity":           starlark.MakeInt(mf.Complexity),
		// microflows() yields all three flow flavours, so a rule naming the
		// document in a message or a location must not hardcode "Microflow".
		// Title case matches the document_type spelling Starlark rules use.
		"document_noun":       starlark.String(mf.DocumentNoun()),
		"document_noun_title": starlark.String(mf.DocumentNounTitle()),
		// "Exclude from project": a guideline rule usually skips such a flow,
		// and could not tell it apart (ako/mxcli#1453).
		"excluded":            starlark.Bool(mf.Excluded),
		"apply_entity_access": starlark.Bool(mf.ApplyEntityAccess),
	})
}

// javaActionToStarlark converts a JavaAction to a Starlark struct.
//
// The doc field is exposed as BOTH `documentation` (the Mendix term, and what
// the metamodel calls it) and `description` (what every other document struct
// here calls it), so a rule that loops over mixed document kinds can read one
// field name throughout.
func javaActionToStarlark(ja JavaAction) starlark.Value {
	params := make([]starlark.Value, 0, len(ja.Parameters))
	for _, p := range ja.Parameters {
		params = append(params, starlarkstruct.FromStringDict(starlark.String("java_action_parameter"), starlark.StringDict{
			"name":           starlark.String(p.Name),
			"description":    starlark.String(p.Description),
			"parameter_type": starlark.String(p.ParameterType),
			"is_required":    starlark.Bool(p.IsRequired),
		}))
	}
	return starlarkstruct.FromStringDict(starlark.String("java_action"), starlark.StringDict{
		"id":              starlark.String(ja.ID),
		"name":            starlark.String(ja.Name),
		"qualified_name":  starlark.String(ja.QualifiedName),
		"module_name":     starlark.String(ja.ModuleName),
		"folder":          starlark.String(ja.Folder),
		"documentation":   starlark.String(ja.Documentation),
		"description":     starlark.String(ja.Documentation),
		"export_level":    starlark.String(ja.ExportLevel),
		"return_type":     starlark.String(ja.ReturnType),
		"parameter_count": starlark.MakeInt(len(ja.Parameters)),
		"parameters":      starlark.NewList(params),
	})
}

// pageToStarlark converts a Page to a Starlark struct.
func pageToStarlark(p Page) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("page"), starlark.StringDict{
		"id":             starlark.String(p.ID),
		"name":           starlark.String(p.Name),
		"qualified_name": starlark.String(p.QualifiedName),
		"module_name":    starlark.String(p.ModuleName),
		"folder":         starlark.String(p.Folder),
		"title":          starlark.String(p.Title),
		"url":            starlark.String(p.URL),
		"description":    starlark.String(p.Description),
		"widget_count":   starlark.MakeInt(p.WidgetCount),
		// Named class_name because class is a Starlark keyword, as on a widget.
		"class_name":      starlark.String(p.Class),
		"style":           starlark.String(p.Style),
		"dynamic_classes": starlark.String(p.DynamicClasses),
	})
}

// enumerationToStarlark converts an Enumeration to a Starlark struct.
func enumerationToStarlark(e Enumeration) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("enumeration"), starlark.StringDict{
		"id":             starlark.String(e.ID),
		"name":           starlark.String(e.Name),
		"qualified_name": starlark.String(e.QualifiedName),
		"module_name":    starlark.String(e.ModuleName),
		"folder":         starlark.String(e.Folder),
		"description":    starlark.String(e.Description),
		"value_count":    starlark.MakeInt(e.ValueCount),
		"values":         enumerationValuesToStarlark(e.Values),
	})
}

// enumerationValuesToStarlark lists an enumeration's values in ordinal order,
// each with the qualified name refs_from() uses for it, so a rule can resolve
// getCaption(Module.Enum.Value) (mendixlabs/mxcli#1310).
func enumerationValuesToStarlark(values []EnumerationValue) *starlark.List {
	out := make([]starlark.Value, 0, len(values))
	for _, v := range values {
		out = append(out, starlarkstruct.FromStringDict(starlark.String("enumeration_value"), starlark.StringDict{
			"name":           starlark.String(v.Name),
			"qualified_name": starlark.String(v.QualifiedName),
			"caption":        starlark.String(v.Caption),
			"ordinal":        starlark.MakeInt(v.Ordinal),
		}))
	}
	return starlark.NewList(out)
}

// constantToStarlark converts a LintConstant to a Starlark struct.
func constantToStarlark(c LintConstant) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("constant"), starlark.StringDict{
		"id":                starlark.String(c.ID),
		"name":              starlark.String(c.Name),
		"qualified_name":    starlark.String(c.QualifiedName),
		"module_name":       starlark.String(c.ModuleName),
		"folder":            starlark.String(c.Folder),
		"description":       starlark.String(c.Description),
		"default_value":     starlark.String(c.DefaultValue),
		"exposed_to_client": starlark.Bool(c.ExposedToClient),
	})
}

// widgetToStarlark converts a Widget to a Starlark struct.
func widgetToStarlark(w Widget) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("widget"), starlark.StringDict{
		"id":                       starlark.String(w.ID),
		"name":                     starlark.String(w.Name),
		"widget_type":              starlark.String(w.WidgetType),
		"container_id":             starlark.String(w.ContainerID),
		"container_qualified_name": starlark.String(w.ContainerQualifiedName),
		"container_type":           starlark.String(w.ContainerType),
		"module_name":              starlark.String(w.ModuleName),
		"entity_ref":               starlark.String(w.EntityRef),
		"attribute_ref":            starlark.String(w.AttributeRef),
		// microflow_ref / nanoflow_ref expose a widget's action or datasource
		// flow so custom rules can detect e.g. a microflow-datasource ListView
		// (no database pushdown). CATALOG.WIDGETS already records these; before
		// they were dropped from the Starlark projection (findings #35).
		"microflow_ref": starlark.String(w.MicroflowRef),
		"nanoflow_ref":  starlark.String(w.NanoflowRef),
		"page_ref":      starlark.String(w.PageRef),
		// Tree position, appearance and primary action (mendixlabs/mxcli#1268).
		// `class` is a Starlark keyword, hence class_name.
		"parent_widget_id": starlark.String(w.ParentWidgetID),
		"depth":            starlark.MakeInt(w.Depth),
		"class_name":       starlark.String(w.Class),
		"style":            starlark.String(w.Style),
		"dynamic_classes":  starlark.String(w.DynamicClasses),
		"action_type":      starlark.String(w.ActionType),
		"has_confirmation": starlark.Bool(w.HasConfirmation),
	})
}

// attributeToStarlark converts an Attribute to a Starlark struct.
func attributeToStarlark(a Attribute) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("attribute"), starlark.StringDict{
		"id":                    starlark.String(a.ID),
		"name":                  starlark.String(a.Name),
		"entity_id":             starlark.String(a.EntityID),
		"entity_qualified_name": starlark.String(a.EntityQualifiedName),
		"module_name":           starlark.String(a.ModuleName),
		"data_type":             starlark.String(a.DataType),
		"length":                starlark.MakeInt(a.Length),
		"is_unique":             starlark.Bool(a.IsUnique),
		"is_required":           starlark.Bool(a.IsRequired),
		"default_value":         starlark.String(a.DefaultValue),
		"is_calculated":         starlark.Bool(a.IsCalculated),
		"description":           starlark.String(a.Description),
	})
}

// referenceToStarlark converts a Reference to a Starlark struct.
func referenceToStarlark(r Reference) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("reference"), starlark.StringDict{
		"source_type": starlark.String(r.SourceType),
		"source_id":   starlark.String(r.SourceID),
		"source_name": starlark.String(r.SourceName),
		"target_type": starlark.String(r.TargetType),
		"target_id":   starlark.String(r.TargetID),
		"target_name": starlark.String(r.TargetName),
		"ref_kind":    starlark.String(r.RefKind),
		"module_name": starlark.String(r.ModuleName),
	})
}

// allPermissionToStarlark converts an AllPermission to a Starlark struct.
func allPermissionToStarlark(p AllPermission) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("permission"), starlark.StringDict{
		"module_role_name":             starlark.String(p.ModuleRoleName),
		"element_type":                 starlark.String(p.ElementType),
		"element_name":                 starlark.String(p.ElementName),
		"member_name":                  starlark.String(p.MemberName),
		"access_type":                  starlark.String(p.AccessType),
		"xpath_constraint":             starlark.String(p.XPathConstraint),
		"is_constrained":               starlark.Bool(p.IsConstrained),
		"module_name":                  starlark.String(p.ModuleName),
		"default_member_access_rights": starlark.String(p.DefaultMemberAccessRights),
		"access_rule_id":               starlark.String(p.AccessRuleID),
		"access_rule_index":            starlark.MakeInt(p.AccessRuleIndex),
	})
}

// permissionToStarlark converts a Permission to a Starlark struct.
func permissionToStarlark(p Permission) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("entity_permission"), starlark.StringDict{
		"module_role_name":             starlark.String(p.ModuleRoleName),
		"module_name":                  starlark.String(p.ModuleName),
		"entity_name":                  starlark.String(p.EntityName),
		"access_type":                  starlark.String(p.AccessType),
		"member_name":                  starlark.String(p.MemberName),
		"xpath_constraint":             starlark.String(p.XPathConstraint),
		"is_constrained":               starlark.Bool(p.IsConstrained),
		"default_member_access_rights": starlark.String(p.DefaultMemberAccessRights),
		// Rows sharing an access_rule_id are one access rule (ako/mxcli#1437).
		"access_rule_id":    starlark.String(p.AccessRuleID),
		"access_rule_index": starlark.MakeInt(p.AccessRuleIndex),
	})
}

// userRoleToStarlark converts a UserRoleInfo to a Starlark struct.
func userRoleToStarlark(ur UserRoleInfo) starlark.Value {
	var moduleRoles []starlark.Value
	for _, mr := range ur.ModuleRoles {
		moduleRoles = append(moduleRoles, starlark.String(mr))
	}
	var manageableRoles []starlark.Value
	for _, mr := range ur.ManageableRoles {
		manageableRoles = append(manageableRoles, starlark.String(mr))
	}
	return starlarkstruct.FromStringDict(starlark.String("user_role"), starlark.StringDict{
		"name":                       starlark.String(ur.Name),
		"description":                starlark.String(ur.Description),
		"is_anonymous":               starlark.Bool(ur.IsAnonymous),
		"module_roles":               starlark.NewList(moduleRoles),
		"check_security":             starlark.Bool(ur.CheckSecurity),
		"manage_all_roles":           starlark.Bool(ur.ManageAllRoles),
		"manage_users_without_roles": starlark.Bool(ur.ManageUsersWithoutRoles),
		"manageable_roles":           starlark.NewList(manageableRoles),
	})
}

// moduleRoleToStarlark converts a ModuleRoleInfo to a Starlark struct.
func moduleRoleToStarlark(mr ModuleRoleInfo) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("module_role"), starlark.StringDict{
		"name":        starlark.String(mr.Name),
		"module_name": starlark.String(mr.ModuleName),
		"description": starlark.String(mr.Description),
	})
}

// roleMappingToStarlark converts a RoleMappingInfo to a Starlark struct.
func roleMappingToStarlark(rm RoleMappingInfo) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("role_mapping"), starlark.StringDict{
		"user_role_name":   starlark.String(rm.UserRoleName),
		"module_role_name": starlark.String(rm.ModuleRoleName),
		"module_name":      starlark.String(rm.ModuleName),
	})
}

// snippetToStarlark converts a Snippet to a Starlark struct.
func snippetToStarlark(s Snippet) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("snippet"), starlark.StringDict{
		"id":             starlark.String(s.ID),
		"name":           starlark.String(s.Name),
		"qualified_name": starlark.String(s.QualifiedName),
		"module_name":    starlark.String(s.ModuleName),
		"folder":         starlark.String(s.Folder),
		"widget_count":   starlark.MakeInt(s.WidgetCount),
	})
}

// activityToStarlark converts an Activity to a Starlark struct.
func activityToStarlark(a Activity) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("activity"), starlark.StringDict{
		"id":                       starlark.String(a.ID),
		"name":                     starlark.String(a.Name),
		"caption":                  starlark.String(a.Caption),
		"activity_type":            starlark.String(a.ActivityType),
		"action_type":              starlark.String(a.ActionType),
		"microflow_id":             starlark.String(a.MicroflowID),
		"microflow_qualified_name": starlark.String(a.MicroflowQualifiedName),
		"module_name":              starlark.String(a.ModuleName),
		"entity_ref":               starlark.String(a.EntityRef),
		"service_ref":              starlark.String(a.ServiceRef),
		"action_ref":               starlark.String(a.ActionRef),
		"use_request_timeout":      starlark.Bool(a.UseRequestTimeout),
		"timeout_expression":       starlark.String(a.TimeoutExpression),
		"auto_generate_caption":    starlark.Bool(a.AutoGenerateCaption),
		"description":              starlark.String(a.Description),
		"parent_loop_id":           starlark.String(a.ParentLoopID),
		"loop_depth":               starlark.MakeInt(a.LoopDepth),
		"condition_expression":     starlark.String(a.ConditionExpression),
		"condition_rule":           starlark.String(a.ConditionRule),
		"error_handling_type":      starlark.String(a.ErrorHandlingType),
		"log_level":                starlark.String(a.LogLevel),
		"log_node_expression":      starlark.String(a.LogNodeExpression),
		"log_message":              starlark.String(a.LogMessage),
		"commit_type":              starlark.String(a.CommitType),
		"with_events":              starlark.Bool(a.WithEvents),
		"retrieve_source":          starlark.String(a.RetrieveSource),
		"queue_ref":                starlark.String(a.QueueRef),
	})
}

// restClientToStarlark converts a RestClient to a Starlark struct.
func restClientToStarlark(rc RestClient) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("rest_client"), starlark.StringDict{
		"id":              starlark.String(rc.ID),
		"name":            starlark.String(rc.Name),
		"qualified_name":  starlark.String(rc.QualifiedName),
		"module_name":     starlark.String(rc.ModuleName),
		"folder":          starlark.String(rc.Folder),
		"base_url":        starlark.String(rc.BaseUrl),
		"auth_scheme":     starlark.String(rc.AuthScheme),
		"operation_count": starlark.MakeInt(rc.OperationCount),
		"documentation":   starlark.String(rc.Documentation),
	})
}

// restOperationToStarlark converts a RestOperation to a Starlark struct.
func restOperationToStarlark(ro RestOperation) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("rest_operation"), starlark.StringDict{
		"id":                     starlark.String(ro.ID),
		"service_id":             starlark.String(ro.ServiceID),
		"service_qualified_name": starlark.String(ro.ServiceQualifiedName),
		"name":                   starlark.String(ro.Name),
		"http_method":            starlark.String(ro.HttpMethod),
		"path":                   starlark.String(ro.Path),
		"parameter_count":        starlark.MakeInt(ro.ParameterCount),
		"has_body":               starlark.Bool(ro.HasBody),
		"response_type":          starlark.String(ro.ResponseType),
		"timeout":                starlark.MakeInt(ro.Timeout),
		"module_name":            starlark.String(ro.ModuleName),
	})
}

// databaseConnectionToStarlark converts a DatabaseConnection to a Starlark struct.
func databaseConnectionToStarlark(dc DatabaseConnection) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("database_connection"), starlark.StringDict{
		"id":             starlark.String(dc.ID),
		"name":           starlark.String(dc.Name),
		"qualified_name": starlark.String(dc.QualifiedName),
		"module_name":    starlark.String(dc.ModuleName),
		"folder":         starlark.String(dc.Folder),
		"database_type":  starlark.String(dc.DatabaseType),
		"query_count":    starlark.MakeInt(dc.QueryCount),
	})
}

func scheduledEventToStarlark(se ScheduledEvent) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("scheduled_event"), starlark.StringDict{
		"name":           starlark.String(se.Name),
		"qualified_name": starlark.String(se.QualifiedName),
		"module_name":    starlark.String(se.ModuleName),
		"microflow_name": starlark.String(se.MicroflowName),
		// Derived from the Schedule child, not the legacy Interval/IntervalType
		// pair — see ScheduledEvent.IntervalSeconds.
		"interval_seconds": starlark.MakeInt(se.IntervalSeconds),
		"repeat":           starlark.String(se.Repeat),
		"on_overlap":       starlark.String(se.OnOverlap),
		"time_zone":        starlark.String(se.TimeZone),
		"enabled":          starlark.Bool(se.Enabled),
	})
}

// builtinQueues returns all task queues.
func (r *StarlarkRule) builtinQueues(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}
	var result []starlark.Value
	for q := range r.ctx.Queues() {
		result = append(result, queueToStarlark(q))
	}
	return starlark.NewList(result), nil
}

func queueToStarlark(q Queue) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("queue"), starlark.StringDict{
		"name":           starlark.String(q.Name),
		"qualified_name": starlark.String(q.QualifiedName),
		"module_name":    starlark.String(q.ModuleName),
		// An EXPRESSION string, not a number.
		"parallelism":  starlark.String(q.Parallelism),
		"cluster_wide": starlark.Bool(q.ClusterWide),
	})
}

// builtinXPathExpressions returns all XPath expression entries from the catalog.
func (r *StarlarkRule) builtinXPathExpressions(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var result []starlark.Value
	for e := range r.ctx.XPathExpressions() {
		result = append(result, xpathExpressionEntryToStarlark(e))
	}

	return starlark.NewList(result), nil
}

// builtinParseXPath parses a raw XPath/expression string and returns its AST as a Starlark struct tree.
// Outer [ ] brackets are stripped automatically. Parse failures produce a "recovered" root node.
func (r *StarlarkRule) builtinParseXPath(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var s starlark.String
	if err := starlark.UnpackArgs("parse_xpath", args, kwargs, "s", &s); err != nil {
		return nil, err
	}

	inner := stripXPathBrackets(string(s))
	parser := exprcheck.NewParser()
	ctx := exprcheck.NewSyntaxContext("", "")
	ctx.XPath = true
	ast, _ := parser.Parse(inner, ctx)
	return robustExprToStarlark(ast), nil
}

// builtinViolation creates a violation struct.
func builtinViolation(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var message starlark.String
	var location starlark.Value = starlark.None
	var suggestion starlark.String

	if err := starlark.UnpackArgs("violation", args, kwargs,
		"message", &message,
		"location?", &location,
		"suggestion?", &suggestion,
	); err != nil {
		return nil, err
	}

	return starlarkstruct.FromStringDict(starlark.String("violation"), starlark.StringDict{
		"message":    message,
		"location":   location,
		"suggestion": suggestion,
	}), nil
}

// builtinLocation creates a location struct.
func builtinLocation(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var module, documentType, documentName, documentID starlark.String

	if err := starlark.UnpackArgs("location", args, kwargs,
		"module", &module,
		"document_type", &documentType,
		"document_name", &documentName,
		"document_id?", &documentID,
	); err != nil {
		return nil, err
	}

	return starlarkstruct.FromStringDict(starlark.String("location"), starlark.StringDict{
		"module":        module,
		"document_type": documentType,
		"document_name": documentName,
		"document_id":   documentID,
	}), nil
}

// builtinIsPascalCase checks if a string is PascalCase.
func builtinIsPascalCase(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var s starlark.String
	if err := starlark.UnpackArgs("is_pascal_case", args, kwargs, "s", &s); err != nil {
		return nil, err
	}

	str := string(s)
	if str == "" {
		return starlark.False, nil
	}

	runes := []rune(str)
	if !unicode.IsUpper(runes[0]) {
		return starlark.False, nil
	}

	for _, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return starlark.False, nil
		}
	}

	return starlark.True, nil
}

// builtinIsCamelCase checks if a string is camelCase.
func builtinIsCamelCase(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var s starlark.String
	if err := starlark.UnpackArgs("is_camel_case", args, kwargs, "s", &s); err != nil {
		return nil, err
	}

	str := string(s)
	if str == "" {
		return starlark.False, nil
	}

	runes := []rune(str)
	if !unicode.IsLower(runes[0]) {
		return starlark.False, nil
	}

	for _, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return starlark.False, nil
		}
	}

	return starlark.True, nil
}

// builtinMatches checks if a string matches a regex pattern.
func builtinMatches(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var s, pattern starlark.String
	if err := starlark.UnpackArgs("matches", args, kwargs, "s", &s, "pattern", &pattern); err != nil {
		return nil, err
	}

	re, err := regexp.Compile(string(pattern))
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	if re.MatchString(string(s)) {
		return starlark.True, nil
	}
	return starlark.False, nil
}

// builtinFindAll returns every match of pattern in s: per match, a tuple of
// its capture groups, or the matched text when the pattern has none — the
// shape of Python's re.findall. matches() answers only yes or no, so a rule
// could not read the part of a name it had matched (ako/mxcli#1439).
func builtinFindAll(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var s, pattern starlark.String
	if err := starlark.UnpackArgs("find_all", args, kwargs, "s", &s, "pattern", &pattern); err != nil {
		return nil, err
	}

	re, err := regexp.Compile(string(pattern))
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	var out []starlark.Value
	for _, m := range re.FindAllStringSubmatch(string(s), -1) {
		if len(m) == 1 {
			out = append(out, starlark.String(m[0]))
			continue
		}
		groups := make(starlark.Tuple, len(m)-1)
		for i, g := range m[1:] {
			groups[i] = starlark.String(g)
		}
		out = append(out, groups)
	}
	return starlark.NewList(out), nil
}

// RuleLoadFailure is a .star file that was found but produced no rule.
type RuleLoadFailure struct {
	Path   string
	Reason string
}

// LoadStarlarkRulesFromDir loads all Starlark rules from a directory, returning
// the rules that loaded and every file that did not.
//
// Failures are returned rather than printed. The previous version wrote them to
// **stdout** with fmt.Printf, which put diagnostics into the same stream as
// `--format json`/`sarif` payloads and gave the caller nothing to act on. The
// caller now decides where a warning goes and whether it is fatal (#904).
//
// A missing directory is not an error: most projects have no custom rules.
func LoadStarlarkRulesFromDir(dir string) ([]*StarlarkRule, []RuleLoadFailure, error) {
	var rules []*StarlarkRule
	var failures []RuleLoadFailure

	if dir == "" {
		return nil, nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return rules, nil, nil
		}
		return nil, nil, err
	}

	// os.ReadDir sorts by filename, so failures come out in a stable order.
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".star") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		rule, err := LoadStarlarkRule(path)
		if err != nil {
			reason := err.Error()
			// A name the resolver does not know is, in a rule that used to
			// load, a builtin from a newer mxcli — say so, as the run-time
			// counterpart in ruleFailureViolation does (ako/mxcli#952).
			if strings.Contains(reason, ": undefined: ") {
				reason += " (a builtin this mxcli does not have — the rule may need a newer mxcli)"
			}
			failures = append(failures, RuleLoadFailure{Path: path, Reason: reason})
			continue
		}

		rules = append(rules, rule)
	}

	return rules, failures, nil
}
