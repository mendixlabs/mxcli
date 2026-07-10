// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/JordtenBulte-OLC/mxcli/mdl/exprcheck"
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
		return []Violation{{
			RuleID:   r.id,
			Severity: SeverityError,
			Message:  fmt.Sprintf("Starlark rule error: %v", err),
		}}
	}

	// Convert result to violations
	return r.convertViolations(result)
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
	"cycles", "module_dependencies", "community_of", "layer_of",
	"centrality", "god_nodes", "integration_surface",
}

// fullBuiltins need REFRESH CATALOG FULL (the refs cross-reference table).
var fullBuiltins = []string{"refs_to", "refs_from"}

// detectRequiredCatalogMode infers the catalog depth a Starlark rule needs by
// scanning its source for calls to the graph / refs builtins.
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

	globals, err := starlark.ExecFile(thread, path, data, predeclared)
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
		"entities":         starlark.NewBuiltin("entities", r.builtinEntities),
		"microflows":       starlark.NewBuiltin("microflows", r.builtinMicroflows),
		"pages":            starlark.NewBuiltin("pages", r.builtinPages),
		"enumerations":     starlark.NewBuiltin("enumerations", r.builtinEnumerations),
		"constants":        starlark.NewBuiltin("constants", r.builtinConstants),
		"widgets":          starlark.NewBuiltin("widgets", r.builtinWidgets),
		"refs_to":          starlark.NewBuiltin("refs_to", r.builtinRefsTo),
		"refs_from":        starlark.NewBuiltin("refs_from", r.builtinRefsFrom),
		"attributes_for":   starlark.NewBuiltin("attributes_for", r.builtinAttributesFor),
		"scheduled_events": starlark.NewBuiltin("scheduled_events", r.builtinScheduledEvents),

		// Graph-analysis facts (populated by `refresh catalog communities`).
		"community_of":         starlark.NewBuiltin("community_of", r.builtinCommunityOf),
		"layer_of":             starlark.NewBuiltin("layer_of", r.builtinLayerOf),
		"cycles":               starlark.NewBuiltin("cycles", r.builtinCycles),
		"module_dependencies":  starlark.NewBuiltin("module_dependencies", r.builtinModuleDependencies),
		"centrality":           starlark.NewBuiltin("centrality", r.builtinCentrality),
		"god_nodes":            starlark.NewBuiltin("god_nodes", r.builtinGodNodes),
		"integration_surface":  starlark.NewBuiltin("integration_surface", r.builtinIntegrationSurface),
		"permissions":          starlark.NewBuiltin("permissions", r.builtinPermissions),
		"permissions_for":      starlark.NewBuiltin("permissions_for", r.builtinPermissionsFor),
		"snippets":             starlark.NewBuiltin("snippets", r.builtinSnippets),
		"database_connections": starlark.NewBuiltin("database_connections", r.builtinDatabaseConnections),
		"activities_for":       starlark.NewBuiltin("activities_for", r.builtinActivitiesFor),

		// Project-level queries
		"user_roles":       starlark.NewBuiltin("user_roles", r.builtinUserRoles),
		"module_roles":     starlark.NewBuiltin("module_roles", r.builtinModuleRoles),
		"role_mappings":    starlark.NewBuiltin("role_mappings", r.builtinRoleMappings),
		"project_security": starlark.NewBuiltin("project_security", r.builtinProjectSecurity),

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

		// Struct constructor (from starlarkstruct)
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),

		// Options access: get_option("key") or get_option("key", default)
		"get_option": starlark.NewBuiltin("get_option", r.builtinGetOption),
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
func (r *StarlarkRule) builtinGetOption(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key starlark.String
	var defaultVal starlark.Value = starlark.None
	if err := starlark.UnpackArgs("get_option", args, kwargs,
		"key", &key,
		"default?", &defaultVal,
	); err != nil {
		return nil, err
	}
	if r.options != nil {
		if v, ok := r.options[string(key)]; ok {
			return goValueToStarlark(v), nil
		}
	}
	return defaultVal, nil
}

// goValueToStarlark converts a Go value (from YAML unmarshaling) to a Starlark value.
func goValueToStarlark(v any) starlark.Value {
	switch val := v.(type) {
	case bool:
		return starlark.Bool(val)
	case int:
		return starlark.MakeInt(val)
	case float64:
		return starlark.Float(val)
	case string:
		return starlark.String(val)
	default:
		return starlark.None
	}
}

// builtinActivitiesFor returns the activities for a given microflow.
func (r *StarlarkRule) builtinActivitiesFor(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}

	var microflowQualifiedName starlark.String
	if err := starlark.UnpackArgs("activities_for", args, kwargs,
		"microflow_qualified_name", &microflowQualifiedName,
	); err != nil {
		return nil, err
	}

	var activities []starlark.Value
	for a := range r.ctx.ActivitiesFor(string(microflowQualifiedName)) {
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
		"password_policy":     starlarkstruct.FromStringDict(starlark.String("password_policy"), ppDict),
	}), nil
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
		"complexity":      starlark.MakeInt(mf.Complexity),
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
	})
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
		"module_role_name": starlark.String(p.ModuleRoleName),
		"element_type":     starlark.String(p.ElementType),
		"element_name":     starlark.String(p.ElementName),
		"member_name":      starlark.String(p.MemberName),
		"access_type":      starlark.String(p.AccessType),
		"xpath_constraint": starlark.String(p.XPathConstraint),
		"is_constrained":   starlark.Bool(p.IsConstrained),
		"module_name":      starlark.String(p.ModuleName),
	})
}

// permissionToStarlark converts a Permission to a Starlark struct.
func permissionToStarlark(p Permission) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("entity_permission"), starlark.StringDict{
		"module_role_name": starlark.String(p.ModuleRoleName),
		"module_name":      starlark.String(p.ModuleName),
		"entity_name":      starlark.String(p.EntityName),
		"access_type":      starlark.String(p.AccessType),
		"member_name":      starlark.String(p.MemberName),
		"xpath_constraint": starlark.String(p.XPathConstraint),
		"is_constrained":   starlark.Bool(p.IsConstrained),
	})
}

// userRoleToStarlark converts a UserRoleInfo to a Starlark struct.
func userRoleToStarlark(ur UserRoleInfo) starlark.Value {
	var moduleRoles []starlark.Value
	for _, mr := range ur.ModuleRoles {
		moduleRoles = append(moduleRoles, starlark.String(mr))
	}
	return starlarkstruct.FromStringDict(starlark.String("user_role"), starlark.StringDict{
		"name":         starlark.String(ur.Name),
		"is_anonymous": starlark.Bool(ur.IsAnonymous),
		"module_roles": starlark.NewList(moduleRoles),
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
		"name":             starlark.String(se.Name),
		"qualified_name":   starlark.String(se.QualifiedName),
		"module_name":      starlark.String(se.ModuleName),
		"microflow_name":   starlark.String(se.MicroflowName),
		"interval_seconds": starlark.MakeInt(se.IntervalSeconds),
		"enabled":          starlark.Bool(se.Enabled),
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
	ast, _ := parser.Parse(inner, exprcheck.NewSyntaxContext("", ""))
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

// LoadStarlarkRulesFromDir loads all Starlark rules from a directory.
func LoadStarlarkRulesFromDir(dir string) ([]*StarlarkRule, error) {
	var rules []*StarlarkRule

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return rules, nil
		}
		return nil, err
	}

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
			// Log warning but continue loading other rules
			fmt.Printf("Warning: failed to load rule %s: %v\n", path, err)
			continue
		}

		rules = append(rules, rule)
	}

	return rules, nil
}
