// SPDX-License-Identifier: Apache-2.0

// Package executor - MDL script validation (reference checking without execution).
package executor

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// scriptContext holds objects defined within a script for reference validation.
type scriptContext struct {
	modules  map[string]bool // Modules created in the script
	entities map[string]bool // Entities created (Module.Entity)
	// viewEntities is the subset of entities that are VIEW entities. Kept apart
	// because a view entity is refused where a persistent one is fine (CE6771),
	// and the endpoint check below skips anything the script creates — so
	// without this, creating the view entity and the association in one script
	// (the ordinary shape) walked straight past the rule.
	viewEntities map[string]bool
	enumerations map[string]bool // Enumerations created (Module.Enum)
	microflows   map[string]bool // Microflows created (Module.Microflow)
	nanoflows    map[string]bool // Nanoflows created (Module.Nanoflow)
	pages        map[string]bool // Pages created (Module.Page)
	snippets     map[string]bool // Snippets created (Module.Snippet)
	layouts      map[string]bool // Layouts created (Module.Layout)
	constants    map[string]bool // Constants created (Module.Constant)
	workflows    map[string]bool // Workflows created (Module.Workflow)

	// Java/JavaScript actions created in the script, mapped to their declared
	// parameter names. A bool would be enough to stop the false "not found",
	// but keeping the names means a call to a script-defined action still gets
	// its parameters checked, exactly as a call to a stored one does.
	javaActions       map[string][]string // Module.Action -> parameter names
	javaScriptActions map[string][]string // Module.Action -> parameter names

	// Microflows and nanoflows created in the script, mapped to their signature —
	// for the same reason as the code actions above. The overwhelmingly common
	// shape is one script that creates a data source microflow AND the page that
	// binds it, so without this the CE1571 check would only ever fire on a flow
	// that already existed, which is the minority case. The RETURN type is kept
	// as well as the parameters, because a data container built on a
	// script-defined flow is what puts an entity into context for the widgets
	// nested in it.
	flowParams map[string]*flowSignature // Module.Flow (lower-cased) -> signature

	// Pages created in the script, mapped to their parameters' entity names, and
	// entities created in the script, mapped to their generalization. Both for
	// the workflow task signature checks, for the same reason as flowParams: the
	// task page, the targeting microflow, the context entity and the workflow are
	// ordinarily one script.
	pageParams            map[string][]string // Module.Page (lower-cased) -> parameter entities
	entityGeneralizations map[string]string   // Module.Entity (lower-cased) -> generalization, "" for none

	// Associations and entity attributes declared in the script, for
	// MDL-XPATH01. Same reason as flowParams above: the overwhelmingly common
	// shape is ONE script that creates the entity, the association and the
	// microflow constraining on it, so a rule that could only see a stored
	// association would fire on the minority case only — and the majority case
	// is exactly the one that reaches a build half-written.
	associations  map[string]string          // Association (unqualified) -> Module.Association
	entityAttrs   map[string]map[string]bool // Module.Entity -> attribute names
	ambiguousAssc map[string]bool            // names defined in more than one module
}

// newScriptContext creates a new script context.
func newScriptContext() *scriptContext {
	return &scriptContext{
		modules:      make(map[string]bool),
		entities:     make(map[string]bool),
		viewEntities: make(map[string]bool),
		enumerations: make(map[string]bool),
		microflows:   make(map[string]bool),
		nanoflows:    make(map[string]bool),
		pages:        make(map[string]bool),
		workflows:    make(map[string]bool),
		snippets:     make(map[string]bool),
		layouts:      make(map[string]bool),
		constants:    make(map[string]bool),

		javaActions:       make(map[string][]string),
		javaScriptActions: make(map[string][]string),
		associations:      map[string]string{},
		entityAttrs:       map[string]map[string]bool{},
		ambiguousAssc:     map[string]bool{},
		flowParams:        make(map[string]*flowSignature),

		pageParams:            make(map[string][]string),
		entityGeneralizations: make(map[string]string),
	}
}

// recordEntityAttrs stores a script-declared entity's attribute names, for the
// rules that need to tell an attribute from an association (MDL-XPATH01).
func (sc *scriptContext) recordEntityAttrs(s *ast.CreateEntityStmt) {
	attrs := make(map[string]bool, len(s.Attributes))
	for _, a := range s.Attributes {
		attrs[a.Name] = true
	}
	sc.entityAttrs[s.Name.String()] = attrs
}

// recordAssociation stores a script-declared association under its UNQUALIFIED
// name, because that is the spelling an XPath constraint gets wrong. A name
// declared in two modules is recorded as ambiguous and then dropped by the
// rule: naming the right spelling is the whole value, and offering one of two
// would be wrong half the time.
//
// Both collectDefinitions and collectSingle call this. They are two parallel
// switches over the same statement types, so a case added to one and not the
// other is collected on one path only — which is how this rule first shipped
// firing against stored associations but not script-declared ones.
func (sc *scriptContext) recordAssociation(s *ast.CreateAssociationStmt) {
	if s.Name.Module == "" || s.Name.Name == "" {
		return
	}
	if prev, ok := sc.associations[s.Name.Name]; ok && prev != s.Name.String() {
		sc.ambiguousAssc[s.Name.Name] = true
		return
	}
	sc.associations[s.Name.Name] = s.Name.String()
}

// codeActionParamNames returns the declared parameter names of a CREATE JAVA
// ACTION / CREATE JAVASCRIPT ACTION statement, in declaration order.
func codeActionParamNames(params []ast.JavaActionParam) []string {
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	return names
}

// collectDefinitions scans a program and collects all objects that will be created.
// collectDefinitions records every object a program defines.
//
// It is a loop over collectSingle, and deliberately nothing more. The two used
// to be parallel switch statements over the same statement types, kept in step
// by hand — and they were not in step: collectSingle had no CreateConstantStmt
// case, and adding view-entity tracking to one of them left the other silent,
// so an association to a view entity created by the SAME script walked past the
// CE6771 rule. One list beats two agreeing lists.
func (sc *scriptContext) collectDefinitions(prog *ast.Program) {
	for _, stmt := range prog.Statements {
		sc.collectSingle(stmt)
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
			sc.recordEntityAttrs(s)
			sc.recordEntityGeneralization(s)
		}
	case *ast.CreateAssociationStmt:
		sc.recordAssociation(s)
	case *ast.CreateViewEntityStmt:
		if s.Name.Module != "" {
			sc.entities[s.Name.String()] = true
			sc.viewEntities[s.Name.String()] = true
		}
	case *ast.CreateExternalEntityStmt:
		if s.Name.Module != "" {
			sc.entities[s.Name.String()] = true
		}
	case *ast.CreateEnumerationStmt:
		if s.Name.Module != "" {
			sc.enumerations[s.Name.String()] = true
		}
	case *ast.CreateConstantStmt:
		if s.Name.Module != "" {
			sc.constants[s.Name.String()] = true
		}
	case *ast.CreateMicroflowStmt:
		if s.Name.Module != "" {
			sc.microflows[s.Name.String()] = true
			sc.recordFlowParams(s.Name.String(), s.Parameters, s.ReturnType)
		}
	case *ast.CreateNanoflowStmt:
		if s.Name.Module != "" {
			sc.nanoflows[s.Name.String()] = true
			sc.recordFlowParams(s.Name.String(), s.Parameters, s.ReturnType)
		}
	case *ast.CreatePageStmtV3:
		if s.Name.Module != "" {
			sc.pages[s.Name.String()] = true
			sc.recordPageParams(s.Name.String(), s.Parameters)
		}
	case *ast.CreateSnippetStmtV3:
		if s.Name.Module != "" {
			sc.snippets[s.Name.String()] = true
		}
	case *ast.CreateLayoutStmt:
		if s.Name.Module != "" {
			sc.layouts[s.Name.String()] = true
		}
	case *ast.CreateWorkflowStmt:
		if s.Name.Module != "" {
			sc.workflows[s.Name.String()] = true
		}
	case *ast.CreateJavaActionStmt:
		if s.Name.Module != "" {
			sc.javaActions[s.Name.String()] = codeActionParamNames(s.Parameters)
		}
	case *ast.CreateJavaScriptActionStmt:
		if s.Name.Module != "" {
			sc.javaScriptActions[s.Name.String()] = codeActionParamNames(s.Parameters)
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
	for n := range sc.workflows {
		names = append(names, n)
	}
	for n := range sc.javaActions {
		names = append(names, n)
	}
	for n := range sc.javaScriptActions {
		names = append(names, n)
	}
	return names
}

// annotateForwardRef checks if a failed statement's error references an object
// that is defined later in the script. If so, it appends a hint to reorder.
func annotateForwardRef(err error, stmt ast.Statement, created, allDefined *scriptContext) error {
	msg := err.Error()
	// A statement's OWN name is "defined in the script but not yet created" at
	// the moment it fails, so without this any validation error that names its
	// own subject picked up the reorder hint — telling the author to move a
	// statement before itself. Found via MDL054, whose message names the entity
	// being created (#832).
	self := newScriptContext()
	self.collectSingle(stmt)
	// Check each name that is defined in the script but not yet created.
	for _, name := range allDefined.allNames() {
		if created.has(name) || self.has(name) {
			continue // already created before this statement, or defined by it
		}
		if strings.Contains(msg, name) {
			return fmt.Errorf("%w\n  hint: %s is defined later in this script — move its create statement before this one", err, name)
		}
	}
	return err
}

// has returns true if the name exists in any category.
func (sc *scriptContext) has(name string) bool {
	if _, ok := sc.javaActions[name]; ok {
		return true
	}
	if _, ok := sc.javaScriptActions[name]; ok {
		return true
	}
	// Every kind allNames lists must be answerable here. allNames includes
	// workflows and this did not, so annotateForwardRef could exclude neither a
	// workflow's own name nor one already created, and told the author to move a
	// workflow statement before itself.
	return sc.modules[name] || sc.entities[name] || sc.enumerations[name] ||
		sc.microflows[name] || sc.nanoflows[name] || sc.pages[name] || sc.snippets[name] ||
		sc.workflows[name]
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
	// Resolve icon-collection references. Needs the project (the collections
	// are documents in it), so it belongs here rather than in the no-project
	// pass — MxBuild otherwise reports the typo as CE1613.
	errors = append(errors, validateIconRefs(ctx, prog)...)
	// Resolve a mapping's `with json structure` / `with xml schema` source, for
	// the same reason and at the same tier: MxBuild otherwise reports the typo
	// as CE1613, a whole build later (ako/mxcli#259).
	errors = append(errors, validateMappingSources(ctx, prog)...)
	// Resolve `HOME PAGE … FOR <user role>`. Also project-resolved, and a tier
	// worse than the two above: a module-qualified role here makes the project
	// unloadable rather than merely failing the build (mendixlabs/mxcli#1001).
	errors = append(errors, validateNavigationRoles(ctx, prog)...)
	// Resolve CALL EXTERNAL ACTION against the consumed service's cached
	// contract. MxBuild otherwise reports the drift as CE7252/CE7269 on the
	// microflow — errors whose wording sends people to the entity import, which
	// cannot fix either of them (mendixlabs/mxcli#1020).
	errors = append(errors, validateExternalActionCalls(ctx, prog)...)
	// Resolve MEMBER names inside a CREATE / CHANGE against the entity they are
	// assigned to. Reference checking used to stop at the document and entity
	// level, so a mistyped attribute passed check and exec and surfaced as
	// CE1613 at the far end of a build (mendixlabs/mxcli#1048).
	for _, msg := range validateMemberReferences(ctx, prog, sc) {
		errors = append(errors, mdlerrors.NewValidation(msg))
	}
	// Resolve the MEMBERS inside a widget's XPath constraint. The entity in
	// `database from Mod.Entity` was resolved and the `where […]` was not, so a
	// member that does not exist reached mxbuild as CE1613
	// (mendixlabs/mxcli#1049).
	errors = append(errors, validateXPathMembers(ctx, prog)...)
	// Dry-run every ALTER … SET against the stored document. The properties of a
	// widget the statement CARRIES are checked without a project; a SET names a
	// widget that is already stored, so its property can only be resolved
	// against the document — which is why it passed check and failed exec.
	errors = append(errors, validateAlterSetProperties(ctx, prog, sc)...)
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
	// Cross-module document-access grants (CE0148) are rejected at exec time by
	// checkDocumentAccessRolesSameModule. Run the same guard here so the failure
	// surfaces during --references instead of partway through a script (#836).
	// The check needs no project state, so it runs before the switch.
	if err := validateCrossModuleGrant(stmt); err != nil {
		return err
	}

	// An ALTER's target document must already exist. Until this, only the
	// MODULE was resolved, so a misspelled document passed --references and was
	// refused by exec — check was the weaker gate, which is backwards.
	if err := validateAlterTarget(ctx, stmt, sc); err != nil {
		return err
	}

	switch s := stmt.(type) {
	// Statements that reference modules
	case *ast.CreateEntityStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// Validate the EXTENDS target. Stored by name, so an unresolved one is
		// CE1613 at build time — or, unqualified, a project Mendix cannot open.
		if err := validateEntityGeneralization(ctx, s, sc); err != nil {
			return err
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
		// Mendix forbids associations to OR from a view entity (CE6771) — statically
		// impossible, so catch it here instead of letting it reach mxbuild. Both
		// endpoints are checked (either direction is rejected). Endpoints created in
		// the same script are skipped (a view entity created here is validated on its
		// own statement). (ledger finding #41)
		for _, ep := range []ast.QualifiedName{s.Parent, s.Child} {
			if ep.Module == "" {
				continue
			}
			if sc.viewEntities[ep.String()] {
				return viewEntityAssociationRefusal(s.Name.String(), ep.String())
			}
			if sc.entities[ep.String()] {
				continue
			}
			if ent, err := findEntity(ctx, ep.Module, ep.Name); err == nil && isViewEntity(ent) {
				return viewEntityAssociationRefusal(s.Name.String(), ep.String())
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
	case *ast.CreateRuleStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// The same validateRule the executor calls, so `check` and `exec` cannot
		// disagree about what a rule may contain.
		if errMsg := validateRule(s.Name.String(), s.Body, s.ReturnType); errMsg != "" {
			return mdlerrors.NewValidationf("%s", strings.TrimRight(errMsg, "\n"))
		}
		if validationErrors := ValidateRuleBody(s); len(validationErrors) > 0 {
			return mdlerrors.NewValidationf("rule '%s' has validation errors:\n  - %s",
				s.Name.String(), strings.Join(validationErrors, "\n  - "))
		}
		if !s.Excluded {
			if refErrors := validateFlowBodyReferences(ctx, s.Body, sc); len(refErrors) > 0 {
				return mdlerrors.NewValidationf("rule '%s' has reference errors:\n  - %s",
					s.Name.String(), strings.Join(refErrors, "\n  - "))
			}
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
		if ctxErrors := validatePageContextTree(ctx, s.Parameters, s.Widgets); len(ctxErrors) > 0 {
			return mdlerrors.NewValidationf("page '%s' has context errors:\n  - %s",
				s.Name.String(), strings.Join(ctxErrors, "\n  - "))
		}
		// CE1571: a microflow call must be given an argument per parameter —
		// as a data source and as an action alike (mendixlabs/mxcli#1082).
		if argErrors := validateFlowArguments(ctx, s.Parameters, s.Widgets, sc); len(argErrors) > 0 {
			return mdlerrors.NewValidationf("page '%s' has argument errors:\n  - %s",
				s.Name.String(), strings.Join(argErrors, "\n  - "))
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
		// A snippet takes the same data sources and actions a page does, and
		// CE1571 does not care which document the widget lives in.
		if argErrors := validateFlowArguments(ctx, s.Parameters, s.Widgets, sc); len(argErrors) > 0 {
			return mdlerrors.NewValidationf("snippet '%s' has argument errors:\n  - %s",
				s.Name.String(), strings.Join(argErrors, "\n  - "))
		}
		// Validate snippet context tree (parameter/selection/attribute bindings)
		if ctxErrors := validatePageContextTree(ctx, s.Parameters, s.Widgets); len(ctxErrors) > 0 {
			return mdlerrors.NewValidationf("snippet '%s' has context errors:\n  - %s",
				s.Name.String(), strings.Join(ctxErrors, "\n  - "))
		}
	case *ast.CreateWorkflowStmt:
		// Two reference passes. Missing targets first: a name that resolves to
		// nothing is the more basic error, and reporting "parameter not mapped"
		// for a microflow that does not exist would be actively misleading.
		// Syntax-only workflow checks (MDL-WF01/02/03) run separately in the
		// no-project phase.
		refErrors := validateWorkflowStatementRefs(ctx, s, sc)
		// Then, for targets that do resolve, that every parameter is mapped
		// (FINDINGS #40).
		refErrors = append(refErrors, validateWorkflowParameterMappings(ctx, s, sc)...)
		if len(refErrors) > 0 {
			return mdlerrors.NewValidationf("workflow '%s' has reference errors:\n  - %s",
				s.Name.String(), strings.Join(refErrors, "\n  - "))
		}
	case *ast.AlterWorkflowStmt:
		if refErrors := validateAlterWorkflowRefs(ctx, s, sc); len(refErrors) > 0 {
			return mdlerrors.NewValidationf("workflow '%s' has reference errors:\n  - %s",
				s.Name.String(), strings.Join(refErrors, "\n  - "))
		}
	case *ast.CreateViewEntityStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// An `<alias>.ID` column names an association, and the name has to be
		// free in the module — reported here because the fix is a rename, and a
		// rename is cheapest before the name spreads (FINDINGS §1).
		if nameErrors := validateViewAssociationNames(ctx, s.Name.Module, s.Name.Name, s.Query.RawQuery, sc.entities); len(nameErrors) > 0 {
			return mdlerrors.NewValidationf("view entity '%s':\n  - %s",
				s.Name.String(), strings.Join(nameErrors, "\n  - "))
		}
		// An attribute typed with an entity is refused before the type comparison,
		// which would line it up against the wrong column (FINDINGS §4).
		if objErrors := viewAttributeEntityTypeErrors(ctx, s.Query.RawQuery, s.Attributes, sc.entities); len(objErrors) > 0 {
			return mdlerrors.NewValidationf("view entity '%s':\n  - %s",
				s.Name.String(), strings.Join(objErrors, "\n  - "))
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

	// ALTER SETTINGS writes qualified names into the model — a startup microflow,
	// the workflow user entity, the constant an override names — and resolved none
	// of them, so a typo reached the build as CE1613 (#274).
	case *ast.AlterSettingsStmt:
		var errs []error
		if strings.EqualFold(s.Section, "constant") {
			if known, trusted := buildConstantQualifiedNames(ctx); trusted {
				errs = validateSettingsConstantRef(s, known, sc)
			}
		} else {
			var mfs, ents map[string]bool
			var rets map[string]string
			if strings.EqualFold(s.Section, "model") {
				mfs = buildMicroflowQualifiedNames(ctx)
				rets = buildMicroflowReturnTypes(ctx)
			}
			if strings.EqualFold(s.Section, "workflows") {
				ents = buildEntityQualifiedNames(ctx)
			}
			errs = validateSettingsReferences(s, mfs, ents, rets, sc)
		}
		if len(errs) > 0 {
			return errs[0]
		}
		return nil

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

	if len(refs.queues) > 0 {
		known := buildQueueQualifiedNames(ctx)
		for _, ref := range refs.queues {
			if !known[strings.ToLower(ref)] {
				errors = append(errors, fmt.Sprintf("task queue not found: %s (referenced by in queue)", ref))
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
			if isBuiltinModuleEntity(qualifiedNameModule(ref.name)) {
				continue
			}
			// An action created earlier in the same script is not in the
			// project yet. Entities, microflows, pages and nanoflows were
			// already exempt; java actions were not, so a script that created
			// one and called it failed reference checking against its own
			// output. mxcli-chat FINDINGS §37.
			if declared, inScript := sc.javaActions[ref.name]; inScript {
				errors = append(errors, validateCodeActionParams("java action", ref, declared)...)
				continue
			}
			if !known[ref.name] {
				errors = append(errors, fmt.Sprintf("java action not found: %s (referenced by call java action)", ref.name))
				continue
			}
			if ja, err := ctx.Backend.ReadJavaActionByName(ref.name); err == nil && ja != nil {
				var declared []string
				for _, p := range ja.Parameters {
					declared = append(declared, p.Name)
				}
				errors = append(errors, validateCodeActionParams("java action", ref, declared)...)
			}
		}
	}

	if len(refs.javaScriptActions) > 0 {
		known := buildJavaScriptActionQualifiedNames(ctx)
		for _, ref := range refs.javaScriptActions {
			if isBuiltinModuleEntity(qualifiedNameModule(ref.name)) {
				continue
			}
			if declared, inScript := sc.javaScriptActions[ref.name]; inScript {
				errors = append(errors, validateCodeActionParams("javascript action", ref, declared)...)
				continue
			}
			if !known[ref.name] {
				errors = append(errors, fmt.Sprintf("javascript action not found: %s (referenced by call javascript action)", ref.name))
				continue
			}
			if jsa, err := ctx.Backend.ReadJavaScriptActionByName(ref.name); err == nil && jsa != nil {
				var declared []string
				for _, p := range jsa.Parameters {
					declared = append(declared, p.Name)
				}
				errors = append(errors, validateCodeActionParams("javascript action", ref, declared)...)
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
		errors = append(errors, validateXPathAssociations(ctx, refs.retrieves, sc)...)
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
	javaActions       []codeActionCallRef
	javaScriptActions []codeActionCallRef
	entities          []entityRef
	retrieves         []retrieveConstraintRef
	queues            []string
}

// codeActionCallRef is a Java / JavaScript action call: the action's qualified
// name plus the parameter names the author wrote, so both the action's existence
// and its parameter names can be validated (a wrong/mis-cased name writes a
// dangling reference that only fails at build time with CE1613).
type codeActionCallRef struct {
	name     string
	argNames []string
}

// callArgNames extracts the written parameter names from a code-action call's
// argument list.
func callArgNames(args []ast.CallArgument) []string {
	if len(args) == 0 {
		return nil
	}
	names := make([]string, 0, len(args))
	for _, a := range args {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return names
}

// validateCodeActionParams checks that each parameter name written in a Java /
// JavaScript action call matches a declared parameter (case-sensitively — Mendix
// parameter names are case-sensitive, and a mismatch writes a dangling reference
// that fails the build with CE1613). When the mismatch is only a casing
// difference, the message suggests the correct spelling. `declared` empty means
// the backend could not report parameters — skip (degrade gracefully).
func validateCodeActionParams(kind string, ref codeActionCallRef, declared []string) []string {
	if len(declared) == 0 {
		return nil
	}
	declaredSet := make(map[string]bool, len(declared))
	for _, d := range declared {
		declaredSet[d] = true
	}
	var errs []string
	for _, written := range ref.argNames {
		if declaredSet[written] {
			continue
		}
		msg := fmt.Sprintf("%s %s has no parameter %q", kind, ref.name, written)
		// Case-only mismatch → point at the correct spelling.
		for _, d := range declared {
			if strings.EqualFold(d, written) {
				msg += fmt.Sprintf(" — did you mean %q?", d)
				break
			}
		}
		sorted := append([]string(nil), declared...)
		sort.Strings(sorted)
		msg += fmt.Sprintf(" (declared parameters: %s). Mendix build fails CE1613 \"The selected %s parameter … no longer exists\".",
			strings.Join(sorted, ", "), kind)
		errs = append(errs, msg)
	}
	return errs
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

// addQueue records an `IN QUEUE Module.Name` target. A queue that does not exist
// builds a dangling reference that only fails at build time, as CE1613 on the
// call activity rather than on the script — so it is worth catching in
// `check --references`.
func (c *flowRefCollector) addQueue(q *ast.QualifiedName) {
	if q != nil && q.Module != "" {
		c.queues = append(c.queues, q.Module+"."+q.Name)
	}
}

func (c *flowRefCollector) empty() bool {
	return len(c.pages) == 0 && len(c.microflows) == 0 && len(c.nanoflows) == 0 &&
		len(c.javaActions) == 0 && len(c.javaScriptActions) == 0 && len(c.entities) == 0 &&
		len(c.retrieves) == 0 && len(c.queues) == 0
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
			c.addQueue(s.Queue)
		case *ast.CallNanoflowStmt:
			if s.NanoflowName.Module != "" {
				c.nanoflows = append(c.nanoflows, s.NanoflowName.String())
			}
		case *ast.CallJavaActionStmt:
			if s.ActionName.Module != "" {
				c.javaActions = append(c.javaActions, codeActionCallRef{
					name: s.ActionName.String(), argNames: callArgNames(s.Arguments),
				})
			}
			c.addQueue(s.Queue)
		case *ast.CallJavaScriptActionStmt:
			if s.ActionName.Module != "" {
				c.javaScriptActions = append(c.javaScriptActions, codeActionCallRef{
					name: s.ActionName.String(), argNames: callArgNames(s.Arguments),
				})
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
	case *ast.SynchronizeStmt:
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

// execEnforcedMicroflowRules are the MDL rules `mxcli exec` refuses to write,
// not just report. Membership requires that the rule's claim has been verified
// against a real mxbuild — a rule that is merely plausible must not become a
// hard write barrier.
//
// All three are XPath-constraint rules whose constructs were built and confirmed
// to fail CE0161:
//
//	MDL047  [Mod.Assoc = empty]              — no `= empty` for an association
//	MDL048  [id = $StringVar]                — no id operator from an expression
//	MDL055  [Attr = $Var/Mod.Assoc/Attr]     — at most one hop off a variable
//
// The rest of the MDL0xx set stays check-only deliberately. Promoting all 17
// error-severity rules was tried and rejected: MDL009 ("enumeration splits
// require exactly one value per branch") is a FALSE POSITIVE — a multi-value
// branch covering every enum value builds at 0 errors on 11.6.6, and the
// shipped write-microflows skill documents that form — so promoting the set
// wholesale would have made exec refuse valid MDL. Verify a rule before adding
// it here.
var execEnforcedMicroflowRules = map[string]bool{
	"MDL047": true,
	"MDL048": true,
	"MDL055": true,
	// MDL057: `synchronize` in a microflow is CE0009 at build time, verified on
	// mxbuild 11.13.0 — the same class of "check caught it, exec did not" gap
	// that #833 was about.
	"MDL057": true,
	// MDL044: a call to a name that is not a Mendix expression function is
	// CE0117 "Error(s) in expression." at build time, verified on mxbuild
	// 11.13.0 with `currentDeviceType()` (issue #828). Promoting this rule means
	// exprcheck's funcTable is now a write barrier, so a name missing from it
	// blocks valid MDL rather than merely warning about it: three genuine
	// built-ins (isNew/isSynced/isSyncing) were found missing and added — each
	// built at 0 errors — before this line was added.
	"MDL044": true,
	// #884: an unknown annotation is silently dropped, so exec must refuse it too —
	// otherwise `check` catches the typo and the write that follows does not.
	"MDL059": true,
	"MDL060": true,
	// MDL-WF16: a notify workflow with no target is CE0166 at build time,
	// measured on the 11.6, 11.10 and 11.13 mxbuilds.
	"MDL-WF16": true,
}

// validateMicroflowRules runs the MDL0xx microflow rule set (ValidateMicroflow)
// on the exec path and turns the verified subset's ERROR-severity violations
// into a failure, so `mxcli exec` refuses to write what those rules reject.
//
// Before this, ValidateMicroflow was wired only into cmd_check.go and the LSP;
// the exec path ran ValidateMicroflowBody, a different validator with a
// different rule set, so a script that skipped `check` wrote microflows the
// build would reject (issue #833, reported via MDL048).
//
// Warnings are never promoted: they are advisory and `check` itself passes with
// them. The rule ID is included so an exec failure matches what `check` prints.
func validateMicroflowRules(stmt *ast.CreateMicroflowStmt) error {
	var msgs []string
	for _, v := range ValidateMicroflow(stmt) {
		if v.Severity != linter.SeverityError || !execEnforcedMicroflowRules[v.RuleID] {
			continue
		}
		msg := fmt.Sprintf("[%s] %s", v.RuleID, v.Message)
		if v.Suggestion != "" {
			msg += "\n    " + v.Suggestion
		}
		msgs = append(msgs, msg)
	}
	if len(msgs) == 0 {
		return nil
	}
	return mdlerrors.NewValidationf("microflow '%s' has validation errors:\n  - %s",
		stmt.Name.String(), strings.Join(msgs, "\n  - "))
}

// recordFlowParams remembers a script-defined flow's signature so a data source
// bound to it is checked exactly as one bound to a stored flow is.
func (sc *scriptContext) recordFlowParams(qualifiedName string, params []ast.MicroflowParam, ret *ast.MicroflowReturnType) {
	sc.flowParams[strings.ToLower(qualifiedName)] = astFlowSignature(params, ret)
}
