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
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
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
	// indirectEntities are entity names the script produces without a CREATE —
	// a RENAME or MOVE lands an entity there — and externalEntityModules the
	// modules a bulk `create external entities` imports into, whose names come
	// from the service contract rather than the statement. Both keep an
	// association endpoint check from refusing what exec will find (#555).
	indirectEntities      map[string]bool
	externalEntityModules map[string]bool
	enumerations          map[string]bool // Enumerations created (Module.Enum)
	microflows            map[string]bool // Microflows created (Module.Microflow)
	nanoflows             map[string]bool // Nanoflows created (Module.Nanoflow)
	pages                 map[string]bool // Pages created (Module.Page)
	snippets              map[string]bool // Snippets created (Module.Snippet)
	layouts               map[string]bool // Layouts created (Module.Layout)
	menus                 map[string]bool // Menu documents created (Module.Menu)
	constants             map[string]bool // Constants created (Module.Constant)
	workflows             map[string]bool // Workflows created (Module.Workflow)
	// Task queues created (Module.Queue, lower-cased — a queue is resolved
	// case-insensitively, like buildQueueQualifiedNames). Missing, a queue the
	// script creates was "not found" for every `in queue` call to it
	// (mendixlabs/mxcli#1211).
	queues map[string]bool

	// Image collections created (Module.Collection) and the images they declare
	// (Module.Collection.Image), both lower-cased like buildImageQualifiedNames.
	// Missing, an image widget naming a collection the same script creates was
	// "image collection not found" and exec refused the script
	// (mendixlabs/mxcli#1419).
	imageCollections map[string]bool
	images           map[string]bool

	// Microflow-typed parameters of the Java actions the script creates
	// (Module.Action -> parameter name). Such a parameter takes a microflow
	// NAME, never an expression (mendixlabs/mxcli#1210).
	javaActionFlowParams map[string]map[string]bool

	// Entities the script declares, and those it alters, for resolving the
	// bare members of a retrieve constraint on an entity the project does not
	// have yet (mendixlabs/mxcli#1213). An altered entity's member list is
	// not the declaration's, so it is not judged from it.
	entityDecls     map[string]*ast.CreateEntityStmt
	alteredEntities map[string]bool

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

	// associationEnds holds each script-declared association's {from, to}
	// entities, keyed by its lower-cased qualified name, so a widget's
	// association path can be followed to an entity the script also creates
	// (MDL-WIDGET39), and a retrieve constraint hopping over one resolves
	// against an entity the project already has.
	associationEnds map[string][2]string
	// associationShapes holds the same associations' multiplicity, so a list
	// widget over one can be judged before it is built (MDL-ASSOCDS01).
	associationShapes map[string]assocShape

	// warnings are findings that do not block: dangling references in an
	// EXCLUDED document, which Mendix itself does not validate. Reported so
	// that relaxing the check hides nothing.
	warnings []string
}

// newScriptContext creates a new script context.
func newScriptContext() *scriptContext {
	return &scriptContext{
		modules:      make(map[string]bool),
		entities:     make(map[string]bool),
		viewEntities: make(map[string]bool),
		enumerations: make(map[string]bool),

		indirectEntities:      make(map[string]bool),
		externalEntityModules: make(map[string]bool),
		microflows:            make(map[string]bool),
		nanoflows:             make(map[string]bool),
		pages:                 make(map[string]bool),
		workflows:             make(map[string]bool),
		snippets:              make(map[string]bool),
		layouts:               make(map[string]bool),
		menus:                 make(map[string]bool),
		constants:             make(map[string]bool),
		queues:                make(map[string]bool),
		imageCollections:      make(map[string]bool),
		images:                make(map[string]bool),

		javaActionFlowParams: make(map[string]map[string]bool),
		entityDecls:          make(map[string]*ast.CreateEntityStmt),
		alteredEntities:      make(map[string]bool),

		javaActions:       make(map[string][]string),
		javaScriptActions: make(map[string][]string),
		associations:      map[string]string{},
		entityAttrs:       map[string]map[string]bool{},
		ambiguousAssc:     map[string]bool{},
		associationEnds:   map[string][2]string{},
		associationShapes: map[string]assocShape{},
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
	if from, to := s.Parent.String(), s.Child.String(); s.Parent.Module != "" && s.Child.Module != "" {
		sc.associationEnds[strings.ToLower(s.Name.String())] = [2]string{from, to}
		sc.associationShapes[strings.ToLower(s.Name.String())] = assocShape{
			qn: s.Name.String(), from: from, to: to,
			refSet: s.Type == ast.AssocReferenceSet,
			owner:  astOwnerName(s.Owner),
		}
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
			sc.entityDecls[s.Name.String()] = s
		}
	case *ast.AlterEntityStmt:
		sc.alteredEntities[s.Name.String()] = true
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
	case *ast.CreateExternalEntitiesStmt:
		target := s.TargetModule
		if target == "" {
			target = s.ServiceRef.Module
		}
		if target != "" {
			sc.externalEntityModules[target] = true
		}
	case *ast.RenameStmt:
		if s.ObjectType == "entity" && s.Name.Module != "" && s.NewName != "" {
			sc.indirectEntities[s.Name.Module+"."+s.NewName] = true
		}
	case *ast.MoveStmt:
		if s.DocumentType == ast.DocumentTypeEntity && s.TargetModule != "" {
			sc.indirectEntities[s.TargetModule+"."+s.Name.Name] = true
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
	case *ast.CreateMenuStmt:
		if s.Name.Module != "" {
			sc.menus[s.Name.String()] = true
		}
	case *ast.CreateImageCollectionStmt:
		if s.Name.Module != "" {
			qn := strings.ToLower(s.Name.String())
			sc.imageCollections[qn] = true
			for _, img := range s.Images {
				sc.images[qn+"."+strings.ToLower(img.Name)] = true
			}
		}
	case *ast.CreateWorkflowStmt:
		if s.Name.Module != "" {
			sc.workflows[s.Name.String()] = true
		}
	case *ast.CreateJavaActionStmt:
		if s.Name.Module != "" {
			sc.javaActions[s.Name.String()] = codeActionParamNames(s.Parameters)
			sc.javaActionFlowParams[s.Name.String()] = microflowTypedParams(s.Parameters)
		}
	case *ast.CreateQueueStmt:
		if s.Name.Module != "" {
			sc.queues[strings.ToLower(s.Name.String())] = true
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
		if (allDefined.microflows[name] || allDefined.nanoflows[name]) &&
			(strings.Contains(msg, "CALL MICROFLOW '"+name+"'") || strings.Contains(msg, "CALL NANOFLOW '"+name+"'")) {
			// Reached only in a run that cannot roll back: one that can
			// resolves the call against the whole script (#1405).
			return fmt.Errorf("%w\n  hint: %s is defined later in this script; %s", err, name, forwardFlowCallRefusal)
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
	errs, _ := validateProgramWithWarnings(ctx, prog)
	return errs
}

// validateProgramWithWarnings is validateProgram that also returns the findings
// that do not block — the dangling references of excluded documents.
func validateProgramWithWarnings(ctx *ExecContext, prog *ast.Program) ([]error, []string) {
	if !ctx.Connected() {
		return []error{mdlerrors.NewNotConnected()}, nil
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
	// The same for the other references exec resolves in statement order, where
	// the later definition is a `create or modify` (mendixlabs/mxcli#1211, #1212).
	errors = append(errors, validateForwardDefRefs(ctx, prog)...)
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
	// Resolve the pages and flows a navigation profile or menu document names:
	// home, login and not-found pages and every menu item. Written by name and
	// unchecked, so a typo was CE1613 at build time, after the whole menu had
	// been rewritten.
	errors = append(errors, validateNavigationTargets(ctx, prog, sc)...)
	// Resolve what a GRANT / REVOKE, a user role and a demo user name: the
	// entity, document or member, and the module and user roles. exec refuses
	// the grants after the statements before them are written, and writes the
	// user-role and demo-user ones unresolved for MxBuild to report as CE1613.
	errors = append(errors, validateGrantReferences(ctx, prog)...)
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
	// MDL-RETRIEVE03 over an association: whether `retrieve $L from $h/M.A`
	// binds a list depends on the association and on which end $h is, so the
	// no-project rule cannot judge it. Using that list as an object built as
	// CE0117 / CE7247 (mendixlabs/mxcli#1516).
	errors = append(errors, validateAssociationRetrieveListUse(ctx, prog, sc)...)
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
	// The entity an ALTER's INSERT / REPLACE binds against is in the stored
	// document, not the statement. Where it has none (a missing flow source, or
	// no data container at all) a bare binding is written bare and the project
	// no longer loads — CREATE PAGE already refused this at check time.
	errors = append(errors, validateAlterUnscopedBindings(ctx, prog, sc)...)
	// An ALTER PAGE's INSERT / REPLACE / DROP targets and the references its
	// inserted widgets carry: only the document was resolved, so a typo in
	// either passed check and stopped exec (mendixlabs/mxcli#1472).
	errors = append(errors, validateAlterPageReferences(ctx, prog, sc)...)
	// `Module.Enum.Key` in any expression: the key must be one of the
	// enumeration's values (mendixlabs/mxcli#1472).
	errors = append(errors, validateEnumValueRefs(ctx, prog, sc)...)
	// A reference into a Protected (add-on) module's Hidden element, and a
	// Usable microflow there exposing a Hidden entity: mxbuild's CE1406 and
	// CE5400, after exec has written the model (mendixlabs/mxcli#1498).
	errors = append(errors, validateProtectedModuleRefs(ctx, prog)...)
	// A business event message attribute named like a member its entity
	// inherits (BusinessEvents.PublishedBusinessEvent's, read from the project):
	// mxbuild reports the duplicate after exec wrote it (mendixlabs/mxcli#1525).
	errors = append(errors, validateBusinessEventInheritedMembers(ctx, prog, sc)...)
	return errors, sc.warnings
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
		label, widgets, ok := documentWidgets(stmt)
		if !ok {
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
	defer e.enterLanguage(prog.LanguageVersion)()
	defer e.CacheUnitReads()()
	return validateProgram(e.newExecContext(context.Background()), prog)
}

// ValidateProgramWithWarnings is ValidateProgram plus the findings that do not
// block: unresolved references inside EXCLUDED documents, which Mendix does
// not validate. Callers print them so that nothing the check relaxed is hidden.
func (e *Executor) ValidateProgramWithWarnings(prog *ast.Program) ([]error, []string) {
	// The script's header decides the gated rejections check predicts
	// (viewAutoNumberRefused), as it does for exec.
	defer e.enterLanguage(prog.LanguageVersion)()
	defer e.CacheUnitReads()()
	return validateProgramWithWarnings(e.newExecContext(context.Background()), prog)
}

// CheckProjectConflicts walks prog in statement order and returns errors for
// any plain CREATE (non-OR-MODIFY) that targets a document name that already
// exists in the connected project. Names created earlier in the same script are
// excluded — those will be caught by CheckScriptDuplicates.
func (e *Executor) CheckProjectConflicts(prog *ast.Program) []error {
	defer e.CacheUnitReads()()
	return CheckProjectConflicts(e.newExecContext(context.Background()), prog)
}

// CheckProjectNameClashes returns an error for every create that would give an
// element a name another kind already has in its module (ako/mxcli#793).
func (e *Executor) CheckProjectNameClashes(prog *ast.Program) []error {
	defer e.CacheUnitReads()()
	return CheckProjectNameClashes(e.newExecContext(context.Background()), prog)
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

	// DROP … IF EXISTS names something that may legitimately be absent, module
	// included; exec skips it, so check must not refuse it (#531).
	if g, ok := stmt.(ast.IfExistsDrop); ok && g.DropIfExists() {
		return nil
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
		// An Auto* system member on a specialization: the same verdict exec
		// gives (checkSpecializationSystemMembers). A parent the script creates
		// is not in the project yet, so its chain is left to exec.
		if s.Generalization != nil {
			warnings, err := checkSpecializationSystemMembers(ctx, s.Name.String(), s.Generalization.String(), s.Attributes, true)
			sc.warnings = append(sc.warnings, warnings...)
			if err != nil {
				return err
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
		// The microflows its event handlers call (mendixlabs/mxcli#1472).
		if err := validateEntityEventHandlers(ctx, sc, s.Name.String(), s.EventHandlers); err != nil {
			return err
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
		// The endpoints themselves, not just their modules: exec resolves both and
		// stops on one it cannot find, after everything before it is written.
		if err := validateAssociationEndpoints(ctx, s, sc); err != nil {
			return err
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
	// exec resolves a module role's module first (findModule) and stops there,
	// after the statements above it are written (mendixlabs/mxcli#1354).
	case *ast.CreateModuleRoleStmt:
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
		// Reported together with the reference errors below rather than
		// instead of them: a body error used to hide a call's unknown
		// parameter, which mxbuild reports as well (CE1613, #953).
		validationErrors := validateFlowBody(s.Parameters, s.ReturnType, s.Body, true, checkAssociationShapes(ctx, sc))
		validationErrors = append(validationErrors, ruleCaseCoverageErrors(ctx, s.Body)...)
		// Validate references inside microflow body (pages, microflows, java actions, entities)
		refErrors := validateFlowBodyReferences(ctx, s.Body, sc)
		if ctx.Connected() {
			refErrors = append(refErrors, validateRetrieveOperandTypes(ctx, s.Parameters, s.Body, sc)...)
		}
		if len(refErrors) > 0 && s.Excluded {
			sc.warnExcluded("microflow", s.Name.String(), refErrors)
			refErrors = nil
		}
		// The signature's types, which exec resolves (and refuses) whether or
		// not the document is excluded — so these are never relaxed.
		refErrors = append(flowSignatureErrors(ctx, sc, "microflow", s.Name, s.Parameters, s.ReturnType), refErrors...)
		if len(validationErrors) > 0 || len(refErrors) > 0 {
			return flowValidationError("microflow", s.Name.String(), validationErrors, refErrors)
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
		if sigErrors := flowSignatureErrors(ctx, sc, "rule", s.Name, s.Parameters, s.ReturnType); len(sigErrors) > 0 {
			return mdlerrors.NewValidationf("rule '%s' has reference errors:\n  - %s",
				s.Name.String(), strings.Join(sigErrors, "\n  - "))
		}
		if refErrors := validateFlowBodyReferences(ctx, s.Body, sc); len(refErrors) > 0 {
			if s.Excluded {
				sc.warnExcluded("rule", s.Name.String(), refErrors)
			} else {
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
		// Reported together with the reference errors below rather than
		// instead of them: a body error used to hide a call's unknown
		// parameter, which mxbuild reports as well (CE1613, #953).
		validationErrors := validateFlowBody(s.Parameters, s.ReturnType, s.Body, true, checkAssociationShapes(ctx, sc))
		validationErrors = append(validationErrors, ruleCaseCoverageErrors(ctx, s.Body)...)
		// Validate references inside nanoflow body (an excluded nanoflow's are warnings)
		refErrors := validateFlowBodyReferences(ctx, s.Body, sc)
		if ctx.Connected() {
			refErrors = append(refErrors, validateRetrieveOperandTypes(ctx, s.Parameters, s.Body, sc)...)
		}
		if len(refErrors) > 0 && s.Excluded {
			sc.warnExcluded("nanoflow", s.Name.String(), refErrors)
			refErrors = nil
		}
		// The signature's types, which exec resolves (and refuses) whether or
		// not the document is excluded — so these are never relaxed.
		refErrors = append(flowSignatureErrors(ctx, sc, "nanoflow", s.Name, s.Parameters, s.ReturnType), refErrors...)
		if len(validationErrors) > 0 || len(refErrors) > 0 {
			return flowValidationError("nanoflow", s.Name.String(), validationErrors, refErrors)
		}
	case *ast.CreatePageStmtV3:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		// The parameters' entities: exec resolves each and stops on one it
		// cannot find, excluded page or not.
		if paramErrors := documentParameterErrors(ctx, sc, "page", s.Name, s.Parameters); len(paramErrors) > 0 {
			return mdlerrors.NewValidationf("page '%s' has reference errors:\n  - %s",
				s.Name.String(), strings.Join(paramErrors, "\n  - "))
		}
		// Every widget-bearing field, not just the bare body — see pageWidgets.
		pageWidgets := allPageWidgets(s)
		// Validate widget references (DataSource, Action, Snippet). An EXCLUDED
		// page may name documents that do not exist — Mendix does not validate
		// excluded documents, and a marketplace module can ship one as an
		// example (Feedback v4.0.2's ShareFeedback_Logo) — so for it they are
		// warnings. "Excluded" is what exec will WRITE: the statement's
		// @excluded, or the exclusion carried from the stored page (#914).
		if refErrors := validateWidgetReferences(ctx, pageWidgets, sc); len(refErrors) > 0 {
			if s.Excluded || carriedExclusion(ctx, "page", s.Name, s.IsReplace || s.IsModify) {
				if err := sc.relaxExcludedWidgetRefs("page", s.Name.String(), pageWidgets, refErrors); err != nil {
					return err
				}
			} else {
				return mdlerrors.NewValidationf("page '%s' has reference errors:\n  - %s",
					s.Name.String(), strings.Join(refErrors, "\n  - "))
			}
		}
		// Validate page context tree (parameter/selection/attribute bindings)
		if ctxErrors := validatePageContextTree(ctx, s.Parameters, pageWidgets); len(ctxErrors) > 0 {
			return mdlerrors.NewValidationf("page '%s' has context errors:\n  - %s",
				s.Name.String(), strings.Join(ctxErrors, "\n  - "))
		}
		// CE1571: a microflow call must be given an argument per parameter —
		// as a data source and as an action alike (mendixlabs/mxcli#1082).
		if argErrors := validateFlowArguments(ctx, s.Parameters, pageWidgets, sc); len(argErrors) > 0 {
			return mdlerrors.NewValidationf("page '%s' has argument errors:\n  - %s",
				s.Name.String(), strings.Join(argErrors, "\n  - "))
		}
		// A pluggable widget attribute of another entity than the one its
		// property binds to — the rule exec writes by (ako/mxcli#647).
		if scopeErrors := validatePluggableAttributeScopes(ctx, s.Layout, s.Parameters, pageWidgets, sc); len(scopeErrors) > 0 {
			return mdlerrors.NewValidationf("page '%s' has attribute binding errors:\n  - %s",
				s.Name.String(), strings.Join(scopeErrors, "\n  - "))
		}
		// A DataGrid 2 column or filter attribute the grid's entity lacks
		// (mendixlabs/mxcli#1472) — CE1613, unless Mendix skips the document.
		if colErrors := datagridColumnAttrErrors(pageWidgets, "", newColumnAttrJudge(ctx, sc)); len(colErrors) > 0 {
			if s.Excluded || carriedExclusion(ctx, "page", s.Name, s.IsReplace || s.IsModify) {
				sc.warnExcluded("page", s.Name.String(), colErrors)
			} else {
				return mdlerrors.NewValidationf("page '%s' has attribute binding errors:\n  - %s",
					s.Name.String(), strings.Join(colErrors, "\n  - "))
			}
		}
	case *ast.CreateLayoutStmt:
		// A layout's widgets name things too — a menu widget's menu document
		// above all, since layouts are where menu widgets live. Before
		// ako/mxcli#573 no layout was reference-checked at all, so
		// `simplemenubar b (Menu: M.Typo)` passed --references and mxbuild
		// answered CE1613 "The selected menu 'M.Typo' no longer exists."
		if refErrors := validateWidgetReferences(ctx, s.AllWidgets(), sc); len(refErrors) > 0 {
			return mdlerrors.NewValidationf("layout '%s' has reference errors:\n  - %s",
				s.Name.String(), strings.Join(refErrors, "\n  - "))
		}
		// The layout it is built on (mendixlabs/mxcli#1411), from this script
		// or the project.
		if outer := s.OuterLayout(); outer != "" && !sc.layouts[outer] && !buildLayoutQualifiedNames(ctx)[outer] {
			return mdlerrors.NewValidationf("layout '%s' is built on %s, which was not found",
				s.Name.String(), outer)
		}
	case *ast.CreateSnippetStmtV3:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		if paramErrors := documentParameterErrors(ctx, sc, "snippet", s.Name, s.Parameters); len(paramErrors) > 0 {
			return mdlerrors.NewValidationf("snippet '%s' has reference errors:\n  - %s",
				s.Name.String(), strings.Join(paramErrors, "\n  - "))
		}
		// Validate widget references (DataSource, Action, Snippet). A snippet
		// has no @excluded of its own; one exec keeps excluded (the carry) is
		// treated as an excluded page is.
		if refErrors := validateWidgetReferences(ctx, s.Widgets, sc); len(refErrors) > 0 {
			if carriedExclusion(ctx, "snippet", s.Name, s.IsReplace || s.IsModify) {
				if err := sc.relaxExcludedWidgetRefs("snippet", s.Name.String(), s.Widgets, refErrors); err != nil {
					return err
				}
			} else {
				return mdlerrors.NewValidationf("snippet '%s' has reference errors:\n  - %s",
					s.Name.String(), strings.Join(refErrors, "\n  - "))
			}
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
		if scopeErrors := validatePluggableAttributeScopes(ctx, "", s.Parameters, s.Widgets, sc); len(scopeErrors) > 0 {
			return mdlerrors.NewValidationf("snippet '%s' has attribute binding errors:\n  - %s",
				s.Name.String(), strings.Join(scopeErrors, "\n  - "))
		}
		if colErrors := datagridColumnAttrErrors(s.Widgets, "", newColumnAttrJudge(ctx, sc)); len(colErrors) > 0 {
			if carriedExclusion(ctx, "snippet", s.Name, s.IsReplace || s.IsModify) {
				sc.warnExcluded("snippet", s.Name.String(), colErrors)
			} else {
				return mdlerrors.NewValidationf("snippet '%s' has attribute binding errors:\n  - %s",
					s.Name.String(), strings.Join(colErrors, "\n  - "))
			}
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
	case *ast.CreatePublishedRestServiceStmt:
		return validatePublishedRestAuthMicroflow(ctx, s.Authentication, sc)
	case *ast.AlterPublishedRestServiceStmt:
		for _, a := range s.Actions {
			if set, ok := a.(*ast.PublishedRestSetAction); ok {
				if err := validatePublishedRestAuthMicroflow(ctx, set.Authentication, sc); err != nil {
					return err
				}
			}
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
		// The entities the query reads from (mendixlabs/mxcli#1472): first,
		// because every column of a missing source goes untyped below.
		if srcErrors := viewSourceEntityErrors(ctx, s.Query.RawQuery, sc); len(srcErrors) > 0 {
			return mdlerrors.NewValidationf("view entity '%s':\n  - %s",
				s.Name.String(), strings.Join(srcErrors, "\n  - "))
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
		typeErrors, typeWarnings := validateViewEntityTypes(ctx, s)
		sc.warnings = append(sc.warnings, typeWarnings...)
		if len(typeErrors) > 0 {
			return mdlerrors.NewValidationf("view entity '%s' has type mismatches:\n  - %s",
				s.Name.String(), strings.Join(typeErrors, "\n  - "))
		}
	case *ast.AlterEntityStmt:
		if s.Name.Module != "" && !sc.modules[s.Name.Module] {
			if _, err := findModule(ctx, s.Name.Module); err != nil {
				return mdlerrors.NewNotFound("module", s.Name.Module)
			}
		}
		if s.Operation == ast.AlterEntityAddEventHandler && s.EventHandler != nil {
			if err := validateEntityEventHandlers(ctx, sc, s.Name.String(), []ast.EventHandlerDef{*s.EventHandler}); err != nil {
				return err
			}
		}
		// ADD/DROP ATTRIBUTE on a view entity is refused by exec; say so here
		// too, or check passes a script that exec stops halfway (#1173).
		if s.Operation == ast.AlterEntityAddAttribute || s.Operation == ast.AlterEntityDropAttribute {
			if err := validateViewEntityAttributeSet(ctx, s, sc); err != nil {
				return err
			}
		}
		// An Auto* system member added to a project specialization: exec's
		// verdict (checkSpecializationSystemMembers), predicted here.
		if s.Operation == ast.AlterEntityAddAttribute && s.Attribute != nil &&
			systemMemberOfPseudoType(s.Attribute.Type.Kind) != "" && !sc.entities[s.Name.String()] {
			if b, ok := ctx.Backend.(entityLookupBackend); ok {
				if ent, found := findEntityByQN(b, s.Name.String()); found && ent.GeneralizationRef != "" {
					warnings, err := checkSpecializationSystemMembers(ctx, s.Name.String(), ent.GeneralizationRef,
						[]ast.Attribute{*s.Attribute}, true)
					sc.warnings = append(sc.warnings, warnings...)
					if err != nil {
						return err
					}
				}
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

// warnExcluded records the dangling references of an EXCLUDED document as
// warnings. Mendix does not validate excluded documents (an untouched project
// holding one passes `mx check` with 0 errors), so they must not block exec or
// `check --references` — but they are reported, so relaxing hides nothing.
func (sc *scriptContext) warnExcluded(kind, name string, refErrors []string) {
	sc.warnings = append(sc.warnings, fmt.Sprintf(
		"%s '%s' is excluded, so its unresolved references do not block (Mendix does not validate excluded documents):\n  - %s",
		kind, name, strings.Join(refErrors, "\n  - ")))
}

// relaxExcludedWidgetRefs handles the unresolved widget references of an
// EXCLUDED page or snippet: a dangling action target or snippet call becomes a
// warning (the writer stores it by name, and Mendix does not validate the
// document), while a dangling DATA SOURCE still blocks.
//
// The data source is the exception because it is not just a name: its flow's
// return type, or its entity, is what the widgets inside the container bind
// against. Without it the builder cannot qualify those bindings, and DESCRIBE
// has already printed them bare. Measured on Mendix 11.13.0 with Feedback
// v4.0.2's ShareFeedback_Logo: writing it anyway left an image URL parameter
// bound to a bare `ImageB64`, and `mx check` could no longer LOAD the project
// (ArgumentNullException setting 'Attribute') — excluded or not.
func (sc *scriptContext) relaxExcludedWidgetRefs(kind, name string, widgets []*ast.WidgetV3, refErrors []string) error {
	refs := &widgetRefCollector{}
	refs.collectFromWidgets(widgets)
	var blocking, warnings []string
	for _, e := range refErrors {
		ref := e[strings.LastIndex(e, ": ")+2:]
		switch {
		case refs.dataSources[ref]:
			// The flow is kept by name, but it is what puts an entity in scope:
			// only a container whose bindings are all qualified can be written.
			if bare := unscopedBindings(widgets, ref); len(bare) > 0 {
				blocking = append(blocking, e+" (data source) — and these bindings inside it are not qualified, "+
					"so nothing can resolve them: "+strings.Join(bare, ", "))
				continue
			}
			warnings = append(warnings, e+" (data source; the bindings inside it are qualified)")
		case strings.HasPrefix(e, "entity not found"):
			// The builder resolves an entity to write it (create_object and the
			// like), so exec would refuse it anyway; say so here instead of
			// letting check pass what exec then fails.
			blocking = append(blocking, e)
		default:
			warnings = append(warnings, e)
		}
	}
	if len(warnings) > 0 {
		sc.warnExcluded(kind, name, warnings)
	}
	if len(blocking) == 0 {
		return nil
	}
	return mdlerrors.NewValidationf("%s '%s' is excluded, but a data source or entity it names does not exist:\n  - %s\n"+
		"  An excluded document may keep a dangling flow, but the widgets inside a data container bind\n"+
		"  against the entity its flow returns — with the flow missing, a bare binding cannot be\n"+
		"  qualified, and one written bare leaves a project Mendix cannot load. Qualify those bindings\n"+
		"  (Module.Entity.Attribute, as `describe` writes them there), or create the missing document.",
		kind, name, strings.Join(blocking, "\n  - "))
}

// unscopedBindings names the attribute bindings that cannot be resolved inside
// the data container(s) whose data source is the missing flow ref: bare
// attributes, `$currentObject/…` paths and association hops, all of which
// resolve against the entity that flow would have returned. Descent stops at a
// nested container with a data source of its own, which scopes its children.
//
// Covered: `Attribute:`, `CaptionAttribute:`, `Visible: Attr in (…)` and
// template parameters (`…Params: ({1} = Attr)`). Anything else is caught by
// the page writer's refusal of a bare attribute reference.
func unscopedBindings(widgets []*ast.WidgetV3, ref string) []string {
	var out []string
	var find func(ws []*ast.WidgetV3)
	find = func(ws []*ast.WidgetV3) {
		for _, w := range ws {
			if w == nil {
				continue
			}
			if ds, ok := w.Properties["DataSource"].(*ast.DataSourceV3); ok && ds.Reference == ref &&
				(ds.Type == "microflow" || ds.Type == "nanoflow") {
				for _, b := range bareBindingsOf(w) { // the container's own bindings, e.g. its visibility
					out = append(out, fmt.Sprintf("%s `%s` (%s)", strings.ToLower(w.Type), w.Name, b))
				}
				out = append(out, bindingsWithoutScope(w.Children)...)
				continue
			}
			find(w.Children)
		}
	}
	find(widgets)
	return out
}

// bindingsWithoutScope names the bare bindings of widgets placed where no
// entity is in scope — the children of a container whose data source flow is
// missing, or widgets an ALTER inserts outside any container that resolves to
// an entity. Descent stops at a widget with a data source of its own, which
// scopes its children.
func bindingsWithoutScope(ws []*ast.WidgetV3) []string {
	var out []string
	for _, w := range ws {
		if w == nil {
			continue
		}
		if _, own := w.Properties["DataSource"].(*ast.DataSourceV3); own {
			continue // its own data source decides its children's scope
		}
		for _, b := range bareBindingsOf(w) {
			out = append(out, fmt.Sprintf("%s `%s` (%s)", strings.ToLower(w.Type), w.Name, b))
		}
		out = append(out, bindingsWithoutScope(w.Children)...)
	}
	return out
}

// bareBindingsOf lists a widget's attribute bindings that need an entity in
// scope to resolve.
func bareBindingsOf(w *ast.WidgetV3) []string {
	var out []string
	needsScope := func(v string) bool {
		switch {
		case v == "", strings.HasPrefix(v, "'"):
			return false // unset, or a literal
		case strings.HasPrefix(v, "$"):
			return strings.HasPrefix(strings.ToLower(v), "$currentobject/")
		}
		return strings.Contains(v, "/") || strings.Count(v, ".") < 2
	}
	for _, key := range []string{"Attribute", "CaptionAttribute"} {
		if v, ok := w.Properties[key].(string); ok && needsScope(v) {
			out = append(out, key+": "+v)
		}
	}
	if vw, ok := w.Properties["VisibleWhen"].(*ast.VisibleWhenV3); ok && needsScope(vw.Attribute) {
		out = append(out, "Visible: "+vw.Attribute+" in (…)")
	}
	for key, v := range w.Properties {
		params, ok := v.([]ast.ParamAssignmentV3)
		if !ok {
			continue
		}
		for _, p := range params {
			if s, ok := p.Value.(string); ok && needsScope(s) {
				out = append(out, fmt.Sprintf("%s {%d} = %s", key, p.Index, s))
			}
		}
	}
	sort.Strings(out)
	return out
}

// carriedExclusion reports whether exec will write the page or snippet named
// qn as EXCLUDED without the statement saying so: a CREATE OR REPLACE/MODIFY
// whose every stored namesake is excluded rewrites the first of them in place
// and keeps it excluded (#914, execCreatePageV3 / execCreateSnippetV3). A live
// namesake is the one rewritten, and it stays live.
func carriedExclusion(ctx *ExecContext, kind string, qn ast.QualifiedName, rewrite bool) bool {
	if !rewrite || !ctx.Connected() {
		return false
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return false
	}
	type doc struct {
		container model.ID
		name      string
		excluded  bool
	}
	var docs []doc
	switch kind {
	case "page":
		pgs, err := ctx.Backend.ListPages()
		if err != nil {
			return false
		}
		for _, p := range pgs {
			docs = append(docs, doc{p.ContainerID, p.Name, p.Excluded})
		}
	case "snippet":
		snips, err := ctx.Backend.ListSnippets()
		if err != nil {
			return false
		}
		for _, sn := range snips {
			docs = append(docs, doc{sn.ContainerID, sn.Name, sn.Excluded})
		}
	}
	excluded := false
	for _, d := range docs {
		if d.name != qn.Name || h.GetModuleName(h.FindModuleID(d.container)) != qn.Module {
			continue
		}
		if !d.excluded {
			return false
		}
		excluded = true
	}
	return excluded
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
			if !known[strings.ToLower(ref)] && !sc.queues[strings.ToLower(ref)] {
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

	// A queued call's target must return nothing (CE7033) — resolving the name
	// says nothing about that (mendixlabs/mxcli#1064).
	if len(refs.queues) > 0 && len(refs.microflows) > 0 {
		errors = append(errors, validateQueuedMicroflowTargets(body, buildMicroflowReturnTypes(ctx), sc)...)
	}

	if len(refs.nanoflows) > 0 {
		known := buildNanoflowQualifiedNames(ctx)
		for _, ref := range refs.nanoflows {
			if !known[ref] && !sc.nanoflows[ref] {
				errors = append(errors, fmt.Sprintf("nanoflow not found: %s (referenced by call nanoflow)", ref))
			}
		}
	}

	// An argument naming no parameter of the called flow is CE1613 "The
	// selected parameter 'M.F.X' no longer exists" (mxbuild 11.13.0, both
	// call microflow and call nanoflow), and exec writes it as given. Java and
	// JavaScript action calls were checked; flow calls were not (#953). A flow
	// that does not resolve is the not-found check's to report.
	if len(refs.flowCalls) > 0 {
		var stored map[string]*flowSignature
		for _, call := range refs.flowCalls {
			if len(call.argNames) == 0 {
				continue
			}
			key := strings.ToLower(call.name)
			sig, ok := sc.flowParams[key]
			if !ok {
				if stored == nil {
					stored = buildFlowSignatures(ctx)
				}
				sig, ok = stored[key]
			}
			if !ok || sig == nil {
				continue
			}
			errors = append(errors, validateCodeActionParams(call.kind,
				codeActionCallRef{name: call.name, argNames: call.argNames}, sig.paramNames())...)
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
				errors = append(errors, microflowParamArgErrors(ref, sc.javaActionFlowParams[ref.name])...)
				continue
			}
			if !known[ref.name] {
				errors = append(errors, fmt.Sprintf("java action not found: %s (referenced by call java action)", ref.name))
				continue
			}
			if ja, err := ctx.Backend.ReadJavaActionByName(ref.name); err == nil && ja != nil {
				declared := []string{} // read: an action with no parameters is known to have none
				flowParams := map[string]bool{}
				for _, p := range ja.Parameters {
					declared = append(declared, p.Name)
					if _, isFlow := p.ParameterType.(*javaactions.MicroflowType); isFlow {
						flowParams[p.Name] = true
					}
				}
				errors = append(errors, validateCodeActionParams("java action", ref, declared)...)
				errors = append(errors, microflowParamArgErrors(ref, flowParams)...)
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
				declared := []string{} // read: an action with no parameters is known to have none
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
		errors = append(errors, validateRetrieveMembers(ctx, refs.retrieves, sc)...)
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
			stores, known := systemMemberStoredOnChain(entities, r.entity, systemMemberStore[member])
			if stores || !known {
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

// systemMemberStoredOnChain reports whether the entity stores the given system
// member, reading the flags where Mendix keeps them: on the ROOT of the
// generalization chain (its NoGeneralization). A specialization's own flags are
// never set — an entity extending System.FileDocument stores owner, changedBy,
// createdDate and changedDate because FileDocument does — so reading
// entityStoresSystemMember off the entity itself refused `[System.owner = …]`
// on every such specialization while mxbuild built it clean.
//
// known is false when a link of the chain cannot be resolved: an ancestor the
// project does not have says nothing either way, and a check answers no only
// from a chain it walked to the end.
func systemMemberStoredOnChain(index map[string]*domainmodel.Entity, entityQN, member string) (stores, known bool) {
	stores, _, known = systemMemberRoot(index, entityQN, member)
	return stores, known
}

// systemMemberRoot is systemMemberStoredOnChain that also names the root entity
// whose flags answered — the entity a user has to change to store the member.
func systemMemberRoot(index map[string]*domainmodel.Entity, entityQN, member string) (stores bool, rootQN string, known bool) {
	seen := map[string]bool{}
	for qn := entityQN; qn != "" && !seen[qn]; {
		seen[qn] = true
		e := index[qn]
		if e == nil {
			if strings.HasPrefix(qn, "System.") {
				stores, known = meta.SystemEntityStoresMember(qn, member)
				return stores, systemChainRoot(qn), known
			}
			return false, "", false
		}
		if e.GeneralizationRef == "" {
			return entityStoresSystemMember(e, member), qn, true
		}
		qn = e.GeneralizationRef
	}
	return false, "", false
}

// systemChainRoot names the root of a System entity's generalization chain.
func systemChainRoot(qn string) string {
	seen := map[string]bool{}
	for !seen[qn] {
		seen[qn] = true
		var next string
		for _, e := range meta.SystemEntities {
			if "System."+e.Name == qn {
				next = e.Generalization
				break
			}
		}
		if next == "" {
			return qn
		}
		qn = next
	}
	return qn
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
	pages      []string
	microflows []string
	nanoflows  []string
	// flowCalls are the microflow and nanoflow calls with the parameter names
	// written on them, for the CE1613 parameter-name check (#953).
	flowCalls         []flowCallRef
	javaActions       []codeActionCallRef
	javaScriptActions []codeActionCallRef
	entities          []entityRef
	retrieves         []retrieveConstraintRef
	queues            []string
}

// flowCallRef is a `call microflow` / `call nanoflow` and the parameter names
// its arguments name.
type flowCallRef struct {
	kind     string // "microflow" or "nanoflow"
	name     string
	argNames []string
}

// codeActionCallRef is a Java / JavaScript action call: the action's qualified
// name plus the parameter names the author wrote, so both the action's existence
// and its parameter names can be validated (a wrong/mis-cased name writes a
// dangling reference that only fails at build time with CE1613).
type codeActionCallRef struct {
	name     string
	argNames []string
	args     []ast.CallArgument
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
//
// declared nil means the parameters are unknown, and nothing is reported; an
// empty, non-nil list is an action or flow known to take none, where any named
// argument is the CE1613.
func validateCodeActionParams(kind string, ref codeActionCallRef, declared []string) []string {
	if declared == nil {
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
		list := strings.Join(sorted, ", ")
		if list == "" {
			list = "none"
		}
		// mxbuild names the code action kind in the message, not the flow kind:
		// "The selected parameter 'M.F.X' no longer exists" for a flow call.
		selected := kind + " parameter"
		if kind == "microflow" || kind == "nanoflow" {
			selected = "parameter"
		}
		msg += fmt.Sprintf(" (declared parameters: %s). Mendix build fails CE1613 \"The selected %s … no longer exists\".",
			list, selected)
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
	stored     string // the constraint as the flow builder writes it (retrieveXPathConstraint)
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
				c.flowCalls = append(c.flowCalls, flowCallRef{
					kind: "microflow", name: s.MicroflowName.String(), argNames: callArgNames(s.Arguments),
				})
			}
			c.addQueue(s.Queue)
		case *ast.CallNanoflowStmt:
			if s.NanoflowName.Module != "" {
				c.nanoflows = append(c.nanoflows, s.NanoflowName.String())
				c.flowCalls = append(c.flowCalls, flowCallRef{
					kind: "nanoflow", name: s.NanoflowName.String(), argNames: callArgNames(s.Arguments),
				})
			}
		case *ast.CallJavaActionStmt:
			if s.ActionName.Module != "" {
				c.javaActions = append(c.javaActions, codeActionCallRef{
					name: s.ActionName.String(), argNames: callArgNames(s.Arguments), args: s.Arguments,
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
						stored:     retrieveXPathConstraint(s.Where, s.Source.String()),
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
//
// It reads the clause through stmtErrorHandling — the one table of which
// statements carry an ON ERROR clause — rather than keeping its own list. Its
// own list had drifted: it lacked the REST statements (and call workflow, the
// mapping statements, commit, delete, …), so every walk built on it skipped
// those handler bodies. The visible failure was MDL-FLOW02 reporting
// "merge X: nothing joins it" when the only join sat in a REST call's error
// handler — the retry loop Studio Pro-authored OIDC microflows use.
func getErrorHandlerBody(stmt ast.MicroflowStatement) []ast.MicroflowStatement {
	if eh := stmtErrorHandling(stmt); eh != nil && eh.Body != nil {
		return eh.Body
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
	// MDL091: `startsWith()` / `endsWith()` in a retrieve constraint are
	// CE0161, measured on mxbuild 11.13.0 against `starts-with()` as the
	// control (mendixlabs/mxcli#1213).
	"MDL091": true,
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
	// built at 0 errors — before this line was added. validateNanoflowRules
	// applies the same entry to nanoflow bodies (mendixlabs/mxcli#1033).
	"MDL044": true,
	// #884: an unknown annotation is silently dropped, so exec must refuse it too —
	// otherwise `check` catches the typo and the write that follows does not.
	"MDL059": true,
	"MDL060": true,
	// MDL092: an @anchor parameter the visitor cannot use is dropped, the edge
	// keeping its default sides — the same class as MDL060 (mendixlabs/mxcli#992).
	"MDL092": true,
	// MDL076: `on error continue` on an activity that rejects it is CE6035 at
	// build time — every row of continueUnsupportedOn was measured on 11.14.0.
	// check reported it; exec without the pre-check (-c, the REPL, --no-check)
	// wrote it (mendixlabs/mxcli#175).
	"MDL076": true,
	// MDL-WF16: a notify workflow with no target is CE0166 at build time,
	// measured on the 11.6, 11.10 and 11.13 mxbuilds.
	"MDL-WF16": true,
	// MDL-WF17: `lock workflow all` / `unlock workflow all` is CE1825, measured
	// on 11.13.0 and 11.14.0 (mendixlabs/mxcli#870).
	"MDL-WF17": true,
	// MDL-SET01: `set` on an object variable or on a parameter is CE7247,
	// measured on 11.14.0 (mendixlabs/mxcli#1323). The builder refuses an
	// object itself; a primitive parameter it would write.
	"MDL-SET01": true,
	// MDL-RETRIEVE02: a retrieve by association over System.owner /
	// System.changedBy is CE0136, measured on 11.12.2 (mendixlabs/mxcli#1358).
	"MDL-RETRIEVE02": true,
	// MDL-RETRIEVE03: `change` over a list retrieve is CE1613 on the stored
	// member, measured on 11.6.6 (mendixlabs/mxcli#1422). The builder refuses
	// it too, for the flows this AST walk cannot type.
	"MDL-RETRIEVE03": true,
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

// allPageWidgets returns every widget a CREATE PAGE statement carries: the bare
// body AND the content of each `placeholder <Name> { … }` block.
//
// A page's widgets live in two fields. `Widgets` is the bare body, which binds
// to Main; content addressed to a named layout placeholder is held apart in
// `Placeholders` (issue #532). The three page validators below were wired to
// `Widgets` alone, so a widget inside a placeholder block was validated by
// nothing — and `placeholder Main { … }` is the shape mxcli's own skills,
// examples and DESCRIBE output all use, so this was the common case rather than
// an edge one. Measured on a blank Mendix 11.14.0 project, the same button
// twice:
//
//	placeholder Main { actionbutton btn (Action: MICROFLOW Mod.NoSuchMicroflow) }
//	                                -> ✓ All references valid
//	actionbutton btn (Action: MICROFLOW Mod.NoSuchMicroflow)
//	                                -> microflow not found: Mod.NoSuchMicroflow
//
// This is the third time the same walk has been missed in this file's
// neighbourhood: validateIconRefs (mendixlabs/mxcli#1008) and forEachWidget
// both had to grow the placeholder arm separately. Collecting the roots once,
// here, is what stops the fourth.
//
// Every other page walker reaches it through documentWidgets below; the
// check-time validators that took `s.Widgets` directly were the fourth miss
// (ako/mxcli#841): MDL-WIDGET33 and MDL-WIDGET31 among them, so placeholder
// content stored a legacy DynamicClasses as its text, silently.
func allPageWidgets(s *ast.CreatePageStmtV3) []*ast.WidgetV3 {
	if len(s.Placeholders) == 0 {
		return s.Widgets
	}
	out := make([]*ast.WidgetV3, 0, len(s.Widgets))
	out = append(out, s.Widgets...)
	for _, ph := range s.Placeholders {
		if ph == nil {
			continue
		}
		out = append(out, ph.Widgets...)
	}
	return out
}

// documentWidgets returns the root widgets of a CREATE PAGE or CREATE SNIPPET
// statement — for a page, the bare body AND every `placeholder X { … }` block —
// and the label diagnostics about them are prefixed with. ok is false for any
// other statement. A page walker that needs "the page's widgets" takes them from
// here, never from `s.Widgets`, which is the bare body only (ako/mxcli#841).
func documentWidgets(stmt ast.Statement) (label string, widgets []*ast.WidgetV3, ok bool) {
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		return "page " + s.Name.String(), allPageWidgets(s), true
	case *ast.CreateSnippetStmtV3:
		return "snippet " + s.Name.String(), s.Widgets, true
	}
	return "", nil, false
}

// documentEntityParameters is the names of a page's or snippet's entity-typed
// parameters — the ones a `$Param.Attr` binding can read.
func documentEntityParameters(stmt ast.Statement) []string {
	var params []ast.PageParameter
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		params = s.Parameters
	case *ast.CreateSnippetStmtV3:
		params = s.Parameters
	}
	var out []string
	for _, p := range params {
		if p.EntityType.Name != "" {
			out = append(out, p.Name)
		}
	}
	return out
}

// documentVariables is a page's or snippet's `Variables:` declarations.
func documentVariables(stmt ast.Statement) []ast.PageVariable {
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		return s.Variables
	case *ast.CreateSnippetStmtV3:
		return s.Variables
	}
	return nil
}

// validateViewEntityAttributeSet reports an ADD/DROP ATTRIBUTE whose target is a
// view entity — one the script creates, or one already in the project. An entity
// the script (re)creates as anything else is judged by that statement instead.
func validateViewEntityAttributeSet(ctx *ExecContext, s *ast.AlterEntityStmt, sc *scriptContext) error {
	qn := s.Name.String()
	isView := sc.viewEntities[qn]
	if !isView && !sc.entities[qn] && s.Name.Module != "" && ctx.Connected() {
		if ent, err := findEntity(ctx, s.Name.Module, s.Name.Name); err == nil {
			isView = isViewEntity(ent)
		}
	}
	if !isView {
		return nil
	}
	if s.Operation == ast.AlterEntityAddAttribute {
		name := ""
		if s.Attribute != nil {
			name = s.Attribute.Name
		}
		return viewEntityAttributeSetRefusal(qn, "add", name)
	}
	return viewEntityAttributeSetRefusal(qn, "drop", s.AttributeName)
}

// flowValidationError joins a flow's body validation errors and its reference
// errors into one error, keeping each list's own heading.
func flowValidationError(kind, name string, validationErrors, refErrors []string) error {
	var parts []string
	if len(validationErrors) > 0 {
		parts = append(parts, fmt.Sprintf("%s '%s' has validation errors:\n  - %s",
			kind, name, strings.Join(validationErrors, "\n  - ")))
	}
	if len(refErrors) > 0 {
		parts = append(parts, fmt.Sprintf("%s '%s' has reference errors:\n  - %s",
			kind, name, strings.Join(refErrors, "\n  - ")))
	}
	return mdlerrors.NewValidationf("%s", strings.Join(parts, "\n  "))
}

// validatePublishedRestAuthMicroflow is exec's MDL-REST04 at check time: the
// custom-authentication microflow exists, and a project microflow has a
// signature Mendix accepts. A microflow the script creates is known by name
// only here; exec checks its signature once it exists.
func validatePublishedRestAuthMicroflow(ctx *ExecContext, auth *ast.PublishedRestAuthentication, sc *scriptContext) error {
	if auth == nil || auth.Microflow == "" {
		return nil
	}
	if problem := publishedRestAuthListProblem(auth.Parameters); problem != "" {
		return mdlerrors.NewValidationf("MDL-REST04: authentication microflow %s %s", auth.Microflow, problem)
	}
	if mf, ok := liveMicroflowsByQualifiedName(ctx)[auth.Microflow]; ok {
		if problem := publishedRestAuthMicroflowProblem(mf); problem != "" {
			return mdlerrors.NewValidationf("MDL-REST04: authentication microflow %s %s", auth.Microflow, problem)
		}
		// The parameter list names parameters the microflow has
		// (mendixlabs/mxcli#1417).
		if _, problem := resolvePublishedRestAuthParameters(auth.Microflow, mf, auth.Parameters); problem != "" {
			return mdlerrors.NewValidationf("MDL-REST04: authentication microflow %s %s", auth.Microflow, problem)
		}
		return nil
	}
	if sc.microflows[auth.Microflow] {
		return nil
	}
	return mdlerrors.NewValidationf("MDL-REST04: authentication microflow not found: %s", auth.Microflow)
}
