// SPDX-License-Identifier: Apache-2.0

// Package executor - MDL script validation (reference checking without execution).
package executor

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// scriptContext holds objects defined within a script for reference validation.
type scriptContext struct {
	modules      map[string]bool // Modules created in the script
	entities     map[string]bool // Entities created (Module.Entity)
	enumerations map[string]bool // Enumerations created (Module.Enum)
	microflows   map[string]bool // Microflows created (Module.Microflow)
	nanoflows    map[string]bool // Nanoflows created (Module.Nanoflow)
	pages        map[string]bool // Pages created (Module.Page)
	snippets     map[string]bool // Snippets created (Module.Snippet)
}

// newScriptContext creates a new script context.
func newScriptContext() *scriptContext {
	return &scriptContext{
		modules:      make(map[string]bool),
		entities:     make(map[string]bool),
		enumerations: make(map[string]bool),
		microflows:   make(map[string]bool),
		nanoflows:    make(map[string]bool),
		pages:        make(map[string]bool),
		snippets:     make(map[string]bool),
	}
}

// collectDefinitions scans a program and collects all objects that will be created.
func (sc *scriptContext) collectDefinitions(prog *ast.Program) {
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateModuleStmt:
			sc.modules[s.Name] = true
		case *ast.CreateEntityStmt:
			if s.Name.Module != "" {
				sc.entities[s.Name.String()] = true
			}
		case *ast.CreateViewEntityStmt:
			if s.Name.Module != "" {
				sc.entities[s.Name.String()] = true
			}
		case *ast.CreateExternalEntityStmt:
			if s.Name.Module != "" {
				sc.entities[s.Name.String()] = true
			}
		case *ast.CreateEnumerationStmt:
			if s.Name.Module != "" {
				sc.enumerations[s.Name.String()] = true
			}
		case *ast.CreateMicroflowStmt:
			if s.Name.Module != "" {
				sc.microflows[s.Name.String()] = true
			}
		case *ast.CreateNanoflowStmt:
			if s.Name.Module != "" {
				sc.nanoflows[s.Name.String()] = true
			}
		case *ast.CreatePageStmtV3:
			if s.Name.Module != "" {
				sc.pages[s.Name.String()] = true
			}
		case *ast.CreateSnippetStmtV3:
			if s.Name.Module != "" {
				sc.snippets[s.Name.String()] = true
			}
		}
	}
}

// collectSingle records the object defined by a single statement.
func (sc *scriptContext) collectSingle(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.CreateModuleStmt:
		sc.modules[s.Name] = true
	case *ast.CreateEntityStmt:
		if s.Name.Module != "" {
			sc.entities[s.Name.String()] = true
		}
	case *ast.CreateViewEntityStmt:
		if s.Name.Module != "" {
			sc.entities[s.Name.String()] = true
		}
	case *ast.CreateExternalEntityStmt:
		if s.Name.Module != "" {
			sc.entities[s.Name.String()] = true
		}
	case *ast.CreateEnumerationStmt:
		if s.Name.Module != "" {
			sc.enumerations[s.Name.String()] = true
		}
	case *ast.CreateMicroflowStmt:
		if s.Name.Module != "" {
			sc.microflows[s.Name.String()] = true
		}
	case *ast.CreateNanoflowStmt:
		if s.Name.Module != "" {
			sc.nanoflows[s.Name.String()] = true
		}
	case *ast.CreatePageStmtV3:
		if s.Name.Module != "" {
			sc.pages[s.Name.String()] = true
		}
	case *ast.CreateSnippetStmtV3:
		if s.Name.Module != "" {
			sc.snippets[s.Name.String()] = true
		}
	}
}

// allNames returns all defined names across all categories.
func (sc *scriptContext) allNames() []string {
	var names []string
	for n := range sc.entities {
		names = append(names, n)
	}
	for n := range sc.enumerations {
		names = append(names, n)
	}
	for n := range sc.microflows {
		names = append(names, n)
	}
	for n := range sc.nanoflows {
		names = append(names, n)
	}
	for n := range sc.pages {
		names = append(names, n)
	}
	for n := range sc.snippets {
		names = append(names, n)
	}
	return names
}

// annotateForwardRef checks if a failed statement's error references an object
// that is defined later in the script. If so, it appends a hint to reorder.
func annotateForwardRef(err error, _ ast.Statement, created, allDefined *scriptContext) error {
	msg := err.Error()
	// Check each name that is defined in the script but not yet created.
	for _, name := range allDefined.allNames() {
		if created.has(name) {
			continue // already created before this statement
		}
		if strings.Contains(msg, name) {
			return fmt.Errorf("%w\n  hint: %s is defined later in this script — move its create statement before this one", err, name)
		}
	}
	return err
}

// has returns true if the name exists in any category.
func (sc *scriptContext) has(name string) bool {
	return sc.modules[name] || sc.entities[name] || sc.enumerations[name] ||
		sc.microflows[name] || sc.nanoflows[name] || sc.pages[name] || sc.snippets[name]
}

// validateProgram validates all statements in a program, skipping references
// to objects that are defined within the script itself.
func validateProgram(ctx *ExecContext, prog *ast.Program) []error {
	if !ctx.Connected() {
		return []error{mdlerrors.NewNotConnected()}
	}

	// Collect all objects defined in the script
	sc := newScriptContext()
	sc.collectDefinitions(prog)

	// Validate each statement
	var errors []error
	for i, stmt := range prog.Statements {
		if err := validateWithContext(ctx, stmt, sc); err != nil {
			errors = append(errors, fmt.Errorf("statement %d: %w", i+1, err))
		}
	}
	errors = append(errors, validateForwardPageRefs(ctx, prog)...)
	return errors
}

// validateForwardPageRefs catches widget `show_page` actions whose target page
// is defined LATER in the same script. The whole-script scriptContext used by
// validateWithContext tolerates these (the page exists *somewhere* in the
// script), but the executor resolves page references in statement order and
// fails on a forward reference. This ordered pass keeps `mxcli check` consistent
// with execution: a referenced page must already exist in the project or be
// created earlier in the script.
func validateForwardPageRefs(ctx *ExecContext, prog *ast.Program) []error {
	if !ctx.Connected() {
		return nil
	}

	known := buildPageQualifiedNames(ctx)
	definedEarlier := make(map[string]bool)

	var errors []error
	for i, stmt := range prog.Statements {
		var widgets []*ast.WidgetV3
		var label string
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			widgets, label = s.Widgets, "page "+s.Name.String()
		case *ast.CreateSnippetStmtV3:
			widgets, label = s.Widgets, "snippet "+s.Name.String()
		default:
			continue
		}

		refs := &widgetRefCollector{}
		refs.collectFromWidgets(widgets)
		refs.dedupe()
		for _, ref := range refs.pages {
			if known[ref] || definedEarlier[ref] {
				continue
			}
			// Unknown to both project and earlier-in-script. A truly-missing page
			// is already reported by validateWidgetReferences; only add the
			// forward-reference hint when the page IS defined later in the script.
			if pageDefinedAfter(prog, ref, i) {
				errors = append(errors, fmt.Errorf(
					"statement %d: %s references page %s before it is created — move the create statement for %s earlier in the script",
					i+1, label, ref, ref))
			}
		}

		if s, ok := stmt.(*ast.CreatePageStmtV3); ok && s.Name.Module != "" {
			definedEarlier[s.Name.String()] = true
		}
	}
	return errors
}

// pageDefinedAfter reports whether a page named ref is created by a
// CreatePageStmtV3 at a statement index greater than fromIdx.
func pageDefinedAfter(prog *ast.Program, ref string, fromIdx int) bool {
	for j := fromIdx + 1; j < len(prog.Statements); j++ {
		if s, ok := prog.Statements[j].(*ast.CreatePageStmtV3); ok && s.Name.Module != "" {
			if s.Name.String() == ref {
				return true
			}
		}
	}
	return false
}

// ValidateProgram validates all statements in a program, skipping references
// to objects that are defined within the script itself.
func (e *Executor) ValidateProgram(prog *ast.Program) []error {
	return validateProgram(e.newExecContext(context.Background()), prog)
}

// CheckProjectConflicts walks prog in statement order and returns errors for
// any plain CREATE (non-OR-MODIFY) that targets a document name that already
// exists in the connected project. Names created earlier in the same script are
// excluded — those will be caught by CheckScriptDuplicates.
func (e *Executor) CheckProjectConflicts(prog *ast.Program) []error {
	return CheckProjectConflicts(e.newExecContext(context.Background()), prog)
}

// validateWithContext validates a statement, considering objects defined in the script.
func validateWithContext(ctx *ExecContext, stmt ast.Statement, sc *scriptContext) error {
	switch s := stmt.(type) {
	// Statements that reference modules
	case *ast.CreateEntityStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// Validate enumeration references in attributes
		attrTypes := make(map[string]ast.DataType)
		for _, attr := range s.Attributes {
			attrTypes[attr.Name] = attr.Type
			if attr.Type.Kind == ast.TypeEnumeration && attr.Type.EnumRef != nil {
				enumRef := attr.Type.EnumRef
				// Check for missing module (common mistake - bare type name)
				if enumRef.Module == "" {
					return mdlerrors.NewValidationf("attribute '%s': enumeration reference '%s' is missing module prefix. "+
						"Did you mean to use a built-in type like DateTime instead of DateAndTime?",
						attr.Name, enumRef.Name)
				}
				// Check if enumeration exists (in project or script)
				enumQN := enumRef.String()
				if !sc.enumerations[enumQN] {
					if !enumerationExists(ctx, enumQN) {
						return mdlerrors.NewNotFoundMsg("enumeration", enumQN, fmt.Sprintf("attribute '%s': enumeration not found: %s", attr.Name, enumQN))
					}
				}
			}
		}
		// Validate index columns
		for _, idx := range s.Indexes {
			for _, col := range idx.Columns {
				dt, exists := attrTypes[col.Name]
				if !exists {
					return mdlerrors.NewValidationf("index on unknown attribute '%s'", col.Name)
				}
				if dt.Kind == ast.TypeString && dt.Length == 0 {
					return mdlerrors.NewValidationf("index on attribute '%s' is not allowed — String(unlimited) maps to text/CLOB which cannot be indexed. Use a fixed length, e.g. String(200)", col.Name)
				}
			}
		}
	case *ast.CreateAssociationStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// Check parent and child entity references
		if s.Parent.Module != "" && !sc.modules[s.Parent.Module] {
			if _, err := findModule(ctx, s.Parent.Module); err != nil {
				return mdlerrors.NewNotFoundMsg("module", s.Parent.Module, "parent entity module not found: "+s.Parent.Module)
			}
		}
		if s.Child.Module != "" && !sc.modules[s.Child.Module] {
			if _, err := findModule(ctx, s.Child.Module); err != nil {
				return mdlerrors.NewNotFoundMsg("module", s.Child.Module, "child entity module not found: "+s.Child.Module)
			}
		}
	case *ast.CreateImageCollectionStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
	case *ast.DropImageCollectionStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
	case *ast.CreateEnumerationStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
	case *ast.CreateMicroflowStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// Validate microflow body for semantic errors (e.g., undeclared variables)
		if validationErrors := ValidateMicroflowBody(s); len(validationErrors) > 0 {
			return mdlerrors.NewValidationf("microflow '%s' has validation errors:\n  - %s",
				s.Name.String(), strings.Join(validationErrors, "\n  - "))
		}
		// Validate references inside microflow body (pages, microflows, java actions, entities)
		if refErrors := validateMicroflowReferences(ctx, s, sc); len(refErrors) > 0 {
			return mdlerrors.NewValidationf("microflow '%s' has reference errors:\n  - %s",
				s.Name.String(), strings.Join(refErrors, "\n  - "))
		}
	case *ast.CreateNanoflowStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// Validate nanoflow body for semantic errors (e.g., undeclared variables)
		if validationErrors := ValidateNanoflowBody(s); len(validationErrors) > 0 {
			return mdlerrors.NewValidationf("nanoflow '%s' has validation errors:\n  - %s",
				s.Name.String(), strings.Join(validationErrors, "\n  - "))
		}
		// Validate references inside nanoflow body (skip excluded nanoflows)
		if !s.Excluded {
			if refErrors := validateFlowBodyReferences(ctx, s.Body, sc); len(refErrors) > 0 {
				return mdlerrors.NewValidationf("nanoflow '%s' has reference errors:\n  - %s",
					s.Name.String(), strings.Join(refErrors, "\n  - "))
			}
		}
	case *ast.CreatePageStmtV3:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// Validate widget references (DataSource, Action, Snippet)
		if refErrors := validateWidgetReferences(ctx, s.Widgets, sc); len(refErrors) > 0 {
			return mdlerrors.NewValidationf("page '%s' has reference errors:\n  - %s",
				s.Name.String(), strings.Join(refErrors, "\n  - "))
		}
		// Validate page context tree (parameter/selection/attribute bindings)
		if ctxErrors := validatePageContextTree(s.Parameters, s.Widgets); len(ctxErrors) > 0 {
			return mdlerrors.NewValidationf("page '%s' has context errors:\n  - %s",
				s.Name.String(), strings.Join(ctxErrors, "\n  - "))
		}
	case *ast.CreateSnippetStmtV3:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// Validate widget references (DataSource, Action, Snippet)
		if refErrors := validateWidgetReferences(ctx, s.Widgets, sc); len(refErrors) > 0 {
			return mdlerrors.NewValidationf("snippet '%s' has reference errors:\n  - %s",
				s.Name.String(), strings.Join(refErrors, "\n  - "))
		}
		// Validate snippet context tree (parameter/selection/attribute bindings)
		if ctxErrors := validatePageContextTree(s.Parameters, s.Widgets); len(ctxErrors) > 0 {
			return mdlerrors.NewValidationf("snippet '%s' has context errors:\n  - %s",
				s.Name.String(), strings.Join(ctxErrors, "\n  - "))
		}
	case *ast.CreateViewEntityStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// Validate OQL types match declared attribute types
		if typeErrors := validateViewEntityTypes(ctx, s); len(typeErrors) > 0 {
			return mdlerrors.NewValidationf("view entity '%s' has type mismatches:\n  - %s",
				s.Name.String(), strings.Join(typeErrors, "\n  - "))
		}
	case *ast.AlterEntityStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// Validate enumeration references in ADD ATTRIBUTE
		if s.Operation == ast.AlterEntityAddAttribute && s.Attribute != nil {
			attr := s.Attribute
			if attr.Type.Kind == ast.TypeEnumeration && attr.Type.EnumRef != nil {
				enumRef := attr.Type.EnumRef
				if enumRef.Module == "" {
					return mdlerrors.NewValidationf("attribute '%s': enumeration reference '%s' is missing module prefix",
						attr.Name, enumRef.Name)
				}
				enumQN := enumRef.String()
				if !sc.enumerations[enumQN] {
					if !enumerationExists(ctx, enumQN) {
						return mdlerrors.NewNotFoundMsg("enumeration", enumQN, fmt.Sprintf("attribute '%s': enumeration not found: %s", attr.Name, enumQN))
					}
				}
			}
		}
	case *ast.DropEntityStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
	case *ast.DropModuleStmt:
		// For DROP, check if module exists in project OR will be created in script
		if !sc.modules[s.Name] {
			if _, err := findModule(ctx, s.Name); err != nil {
				return mdlerrors.NewNotFound("module", s.Name)
			}
		}

	// Query statements - no validation needed for basic ones
	case *ast.ShowStmt, *ast.DescribeStmt, *ast.SelectStmt:
		// These are read-only and will fail gracefully at execution
		return nil

	// Connection/session statements - no validation needed
	case *ast.ConnectStmt, *ast.DisconnectStmt, *ast.StatusStmt,
		*ast.SetStmt, *ast.HelpStmt, *ast.ExitStmt, *ast.ExecuteScriptStmt,
		*ast.UpdateStmt, *ast.RefreshStmt, *ast.RefreshCatalogStmt,
		*ast.SearchStmt:
		return nil

	default:
		// For unhandled statement types, skip validation
		return nil
	}

	return nil
}

// validate checks if a statement's references are valid without executing it.
// This requires being connected to a project.
// Note: For validating entire programs with proper handling of script-defined objects,
// use validateProgram instead.
func validate(ctx *ExecContext, stmt ast.Statement) error {
	// Use validateWithContext with an empty script context for single statements
	return validateWithContext(ctx, stmt, newScriptContext())
}

// Validate checks if a statement's references are valid without executing it.
func (e *Executor) Validate(stmt ast.Statement) error {
	return validate(e.newExecContext(context.Background()), stmt)
}

// ----------------------------------------------------------------------------
// Microflow Body Reference Validation
// ----------------------------------------------------------------------------

// validateMicroflowReferences validates that all qualified name references in a
// microflow body (pages, microflows, java actions, entities) point to existing objects.
func validateMicroflowReferences(ctx *ExecContext, s *ast.CreateMicroflowStmt, sc *scriptContext) []string {
	if s.Excluded {
		// Studio Pro allows excluded documents to keep stale references. Reference
		// checks should not fail a roundtrip audit for microflows that are not part
		// of the runnable app.
		return nil
	}
	return validateFlowBodyReferences(ctx, s.Body, sc)
}

// validateFlowBodyReferences validates references in any flow body (microflow or nanoflow).
func validateFlowBodyReferences(ctx *ExecContext, body []ast.MicroflowStatement, sc *scriptContext) []string {
	if !ctx.Connected() || len(body) == 0 {
		return nil
	}

	refs := &flowRefCollector{}
	refs.collectFromStatements(body)

	if refs.empty() {
		return nil
	}

	var errors []string

	if len(refs.pages) > 0 {
		known := buildPageQualifiedNames(ctx)
		for _, ref := range refs.pages {
			if !known[ref] && !sc.pages[ref] {
				errors = append(errors, fmt.Sprintf("page not found: %s (referenced by show page)", ref))
			}
		}
	}

	if len(refs.microflows) > 0 {
		known := buildMicroflowQualifiedNames(ctx)
		for _, ref := range refs.microflows {
			if !known[ref] && !sc.microflows[ref] {
				errors = append(errors, fmt.Sprintf("microflow not found: %s (referenced by call microflow)", ref))
			}
		}
	}

	if len(refs.nanoflows) > 0 {
		known := buildNanoflowQualifiedNames(ctx)
		for _, ref := range refs.nanoflows {
			if !known[ref] && !sc.nanoflows[ref] {
				errors = append(errors, fmt.Sprintf("nanoflow not found: %s (referenced by call nanoflow)", ref))
			}
		}
	}

	if len(refs.javaActions) > 0 {
		known := buildJavaActionQualifiedNames(ctx)
		for _, ref := range refs.javaActions {
			// System.* Java actions (e.g. System.VerifyPassword,
			// System.GenerateRandomString) are runtime-provided and never
			// appear in the project's MPR. Skip them to avoid false
			// positives — Studio Pro's `mx check` resolves these against
			// the runtime, which `mxcli check` cannot reach.
			if isBuiltinModuleEntity(qualifiedNameModule(ref)) {
				continue
			}
			if !known[ref] {
				errors = append(errors, fmt.Sprintf("java action not found: %s (referenced by call java action)", ref))
			}
		}
	}

	if len(refs.javaScriptActions) > 0 {
		known := buildJavaScriptActionQualifiedNames(ctx)
		for _, ref := range refs.javaScriptActions {
			if isBuiltinModuleEntity(qualifiedNameModule(ref)) {
				continue
			}
			if !known[ref] {
				errors = append(errors, fmt.Sprintf("javascript action not found: %s (referenced by call javascript action)", ref))
			}
		}
	}

	if len(refs.entities) > 0 {
		known := buildEntityQualifiedNames(ctx)
		for _, ref := range refs.entities {
			if !known[ref.name] && !sc.entities[ref.name] {
				errors = append(errors, fmt.Sprintf("entity not found: %s (referenced by %s)", ref.name, ref.source))
			}
		}
	}

	if len(refs.retrieves) > 0 {
		errors = append(errors, validateRetrieveConstraints(ctx, refs.retrieves)...)
	}

	return errors
}

// systemMemberStore maps a System.* member usable in an XPath constraint to the
// entity flag that records whether the entity actually stores it. Referencing one
// of these in a constraint when the entity doesn't store it produces CE0161 in
// Studio Pro ("Error(s) in XPath constraint."). Issue #641.
var systemMemberStore = map[string]string{
	"owner":       "owner",
	"changedBy":   "changedBy",
	"changedDate": "changedDate",
	"createdDate": "createdDate",
}

// baseSystemMemberRe matches a `System.<member>` reference on the retrieve's own
// entity (not behind an association traversal, i.e. not preceded by `/`).
var baseSystemMemberRe = regexp.MustCompile(`(^|[^/\w.])System\.(owner|changedBy|changedDate|createdDate)\b`)

// validateRetrieveConstraints flags constraints that will fail Mendix's own
// `mx check` (CE0161) even though mxcli stored them faithfully: a System.owner /
// changedBy / changedDate / createdDate member referenced on an entity that
// doesn't store it. This is the legitimate "owned by current user" pattern that
// silently fails when owner isn't enabled (issue #641).
func validateRetrieveConstraints(ctx *ExecContext, retrieves []retrieveConstraintRef) []string {
	entities := buildEntityIndex(ctx)
	if entities == nil {
		return nil
	}
	var errors []string
	for _, r := range retrieves {
		ent := entities[r.entity]
		if ent == nil {
			continue // entity-not-found is reported separately
		}
		for _, m := range baseSystemMemberRe.FindAllStringSubmatch(r.constraint, -1) {
			member := m[2]
			if entityStoresSystemMember(ent, systemMemberStore[member]) {
				continue
			}
			errors = append(errors, fmt.Sprintf(
				"constraint references System.%s on %s, but the entity does not store %s — Studio Pro rejects this with CE0161. "+
					"Enable it first: alter entity %s add attribute %s: auto%s",
				member, r.entity, member, r.entity, member, strings.ToLower(member)))
		}
	}
	return errors
}

// entityStoresSystemMember reports whether the entity records the given system
// member (owner/changedBy/changedDate/createdDate).
func entityStoresSystemMember(e *domainmodel.Entity, member string) bool {
	switch member {
	case "owner":
		return e.HasOwner
	case "changedBy":
		return e.HasChangedBy
	case "changedDate":
		return e.HasChangedDate
	case "createdDate":
		return e.HasCreatedDate
	}
	return true // unknown member → don't flag
}

// buildEntityIndex maps every entity's qualified name to its definition for
// schema-aware validation. Returns nil if the domain model can't be read.
func buildEntityIndex(ctx *ExecContext) map[string]*domainmodel.Entity {
	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return nil
	}
	moduleNames := make(map[model.ID]string, len(modules))
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return nil
	}
	index := make(map[string]*domainmodel.Entity)
	for _, dm := range dms {
		modName := moduleNames[dm.ContainerID]
		if modName == "" {
			continue
		}
		for _, ent := range dm.Entities {
			index[modName+"."+ent.Name] = ent
		}
	}
	return index
}

// qualifiedNameModule returns the module portion of a "Module.Name" qualified
// name. It returns an empty string when the input has no dot.
func qualifiedNameModule(qn string) string {
	if i := strings.Index(qn, "."); i >= 0 {
		return qn[:i]
	}
	return ""
}

// flowRefCollector collects qualified name references from flow body statements.
type flowRefCollector struct {
	pages             []string
	microflows        []string
	nanoflows         []string
	javaActions       []string
	javaScriptActions []string
	entities          []entityRef
	retrieves         []retrieveConstraintRef
}

// entityRef tracks an entity reference along with the statement that referenced it.
type entityRef struct {
	name   string
	source string // e.g., "CREATE", "RETRIEVE", "CREATE LIST OF"
}

// retrieveConstraintRef pairs a database retrieve's entity with its XPath
// constraint so the constraint can be validated against the entity schema.
type retrieveConstraintRef struct {
	entity     string // entity qualified name (database retrieve only)
	constraint string // bracketed XPath constraint, e.g. "[System.owner = '[%CurrentUser%]']"
}

func (c *flowRefCollector) empty() bool {
	return len(c.pages) == 0 && len(c.microflows) == 0 && len(c.nanoflows) == 0 &&
		len(c.javaActions) == 0 && len(c.javaScriptActions) == 0 && len(c.entities) == 0 &&
		len(c.retrieves) == 0
}

func (c *flowRefCollector) collectFromStatements(stmts []ast.MicroflowStatement) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ShowPageStmt:
			if s.PageName.Module != "" {
				c.pages = append(c.pages, s.PageName.String())
			}
		case *ast.CallMicroflowStmt:
			if s.MicroflowName.Module != "" {
				c.microflows = append(c.microflows, s.MicroflowName.String())
			}
		case *ast.CallNanoflowStmt:
			if s.NanoflowName.Module != "" {
				c.nanoflows = append(c.nanoflows, s.NanoflowName.String())
			}
		case *ast.CallJavaActionStmt:
			if s.ActionName.Module != "" {
				c.javaActions = append(c.javaActions, s.ActionName.String())
			}
		case *ast.CallJavaScriptActionStmt:
			if s.ActionName.Module != "" {
				c.javaScriptActions = append(c.javaScriptActions, s.ActionName.String())
			}
		case *ast.CallWebServiceStmt:
			// Web service and mapping references can be raw IDs; reference validation
			// cannot safely resolve them without project metadata.
		case *ast.CreateObjectStmt:
			if s.EntityType.Module != "" {
				c.entities = append(c.entities, entityRef{name: s.EntityType.String(), source: "create"})
			}
		case *ast.RetrieveStmt:
			if s.StartVariable != "" {
				// Association retrieve — Source is an association name, not an entity; skip entity validation
			} else if s.Source.Module != "" {
				c.entities = append(c.entities, entityRef{name: s.Source.String(), source: "retrieve"})
				if s.Where != nil {
					c.retrieves = append(c.retrieves, retrieveConstraintRef{
						entity:     s.Source.String(),
						constraint: expressionToXPath(s.Where),
					})
				}
			}
		case *ast.CreateListStmt:
			if s.EntityType.Module != "" {
				c.entities = append(c.entities, entityRef{name: s.EntityType.String(), source: "create list of"})
			}
		case *ast.IfStmt:
			c.collectFromStatements(s.ThenBody)
			c.collectFromStatements(s.ElseBody)
		case *ast.EnumSplitStmt:
			for _, cse := range s.Cases {
				c.collectFromStatements(cse.Body)
			}
			c.collectFromStatements(s.ElseBody)
		case *ast.InheritanceSplitStmt:
			for _, cse := range s.Cases {
				c.collectFromStatements(cse.Body)
			}
			c.collectFromStatements(s.ElseBody)
		case *ast.LoopStmt:
			c.collectFromStatements(s.Body)
		}
		// Recurse into error handler bodies
		if eh := getErrorHandlerBody(stmt); eh != nil {
			c.collectFromStatements(eh)
		}
	}
}

// getErrorHandlerBody returns the custom error handler body if present, or nil.
func getErrorHandlerBody(stmt ast.MicroflowStatement) []ast.MicroflowStatement {
	switch s := stmt.(type) {
	case *ast.CreateObjectStmt:
		if s.ErrorHandling != nil && s.ErrorHandling.Body != nil {
			return s.ErrorHandling.Body
		}
	case *ast.RetrieveStmt:
		if s.ErrorHandling != nil && s.ErrorHandling.Body != nil {
			return s.ErrorHandling.Body
		}
	case *ast.CallMicroflowStmt:
		if s.ErrorHandling != nil && s.ErrorHandling.Body != nil {
			return s.ErrorHandling.Body
		}
	case *ast.CallNanoflowStmt:
		if s.ErrorHandling != nil && s.ErrorHandling.Body != nil {
			return s.ErrorHandling.Body
		}
	case *ast.CallJavaActionStmt:
		if s.ErrorHandling != nil && s.ErrorHandling.Body != nil {
			return s.ErrorHandling.Body
		}
	case *ast.DownloadFileStmt:
		if s.ErrorHandling != nil && s.ErrorHandling.Body != nil {
			return s.ErrorHandling.Body
		}
	case *ast.CallJavaScriptActionStmt:
		if s.ErrorHandling != nil && s.ErrorHandling.Body != nil {
			return s.ErrorHandling.Body
		}
	case *ast.CallWebServiceStmt:
		if s.ErrorHandling != nil && s.ErrorHandling.Body != nil {
			return s.ErrorHandling.Body
		}
	case *ast.ExecuteDatabaseQueryStmt:
		if s.ErrorHandling != nil && s.ErrorHandling.Body != nil {
			return s.ErrorHandling.Body
		}
	}
	return nil
}
