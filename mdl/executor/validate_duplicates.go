// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

// nameRegistry tracks which document names are currently "alive" (created but
// not yet dropped) as we walk a script in statement order. Keyed by doc-type
// string → qualified name → statement index (1-based) where the name was first
// created.
type nameRegistry struct {
	alive map[string]map[string]int
}

func newNameRegistry() *nameRegistry {
	return &nameRegistry{alive: make(map[string]map[string]int)}
}

func (r *nameRegistry) has(docType, name string) (int, bool) {
	m := r.alive[docType]
	if m == nil {
		return 0, false
	}
	idx, ok := m[name]
	return idx, ok
}

func (r *nameRegistry) isAlive(docType, name string) bool {
	_, ok := r.has(docType, name)
	return ok
}

func (r *nameRegistry) add(docType, name string, stmtIdx int) {
	if r.alive[docType] == nil {
		r.alive[docType] = make(map[string]int)
	}
	r.alive[docType][name] = stmtIdx
}

func (r *nameRegistry) remove(docType, name string) {
	if m := r.alive[docType]; m != nil {
		delete(m, name)
	}
}

// renameModule cascades a module rename: every alive name with the old module
// prefix is re-keyed with the new module prefix, across all doc-type maps.
func (r *nameRegistry) renameModule(oldMod, newMod string) {
	prefix := oldMod + "."
	for _, m := range r.alive {
		var toRename []string
		for k := range m {
			if strings.HasPrefix(k, prefix) {
				toRename = append(toRename, k)
			}
		}
		for _, k := range toRename {
			idx := m[k]
			delete(m, k)
			m[newMod+"."+k[len(prefix):]] = idx
		}
	}
	// Also rename the module entry itself.
	if m := r.alive["module"]; m != nil {
		if idx, ok := m[oldMod]; ok {
			delete(m, oldMod)
			m[newMod] = idx
		}
	}
}

// ----------------------------------------------------------------------------
// Statement classification helpers
// ----------------------------------------------------------------------------

// stmtCreateInfo returns the doc-type key, qualified name, and whether the
// CREATE is idempotent (OR MODIFY / OR REPLACE). Returns empty strings when
// the statement is not a tracked CREATE.
func stmtCreateInfo(stmt ast.Statement) (docType, name string, idempotent bool) {
	switch s := stmt.(type) {
	case *ast.CreateModuleStmt:
		return "module", s.Name, false
	case *ast.CreateEntityStmt:
		return "entity", s.Name.String(), s.CreateOrModify
	case *ast.CreateViewEntityStmt:
		return "entity", s.Name.String(), s.CreateOrModify || s.CreateOrReplace
	case *ast.CreateExternalEntityStmt:
		return "entity", s.Name.String(), s.CreateOrModify
	case *ast.CreateEnumerationStmt:
		return "enumeration", s.Name.String(), s.CreateOrModify
	case *ast.CreateAssociationStmt:
		return "association", s.Name.String(), s.CreateOrModify
	case *ast.CreateConstantStmt:
		return "constant", s.Name.String(), s.CreateOrModify
	case *ast.CreateMicroflowStmt:
		return "microflow", s.Name.String(), s.CreateOrModify
	case *ast.CreateNanoflowStmt:
		return "nanoflow", s.Name.String(), s.CreateOrModify
	case *ast.CreatePageStmtV3:
		return "page", s.Name.String(), s.IsModify || s.IsReplace
	case *ast.CreateSnippetStmtV3:
		return "snippet", s.Name.String(), s.IsModify || s.IsReplace
	case *ast.CreateJavaActionStmt:
		return "javaaction", s.Name.String(), s.CreateOrModify
	case *ast.CreateJavaScriptActionStmt:
		return "javascriptaction", s.Name.String(), s.CreateOrModify
	case *ast.CreateWorkflowStmt:
		return "workflow", s.Name.String(), s.CreateOrModify
	case *ast.CreateBusinessEventServiceStmt:
		return "business-event-service", s.Name.String(), s.CreateOrModify
	case *ast.CreatePublishedRestServiceStmt:
		return "published-rest-service", s.Name.String(), s.CreateOrModify
	case *ast.CreateJsonStructureStmt:
		return "json-structure", s.Name.String(), s.CreateOrModify
	case *ast.CreateImportMappingStmt:
		return "import-mapping", s.Name.String(), s.CreateOrModify
	case *ast.CreateExportMappingStmt:
		return "export-mapping", s.Name.String(), s.CreateOrModify
	case *ast.CreateDataTransformerStmt:
		return "data-transformer", s.Name.String(), s.CreateOrModify
	case *ast.CreateModelStmt:
		return "agent-model", s.Name.String(), s.CreateOrModify
	case *ast.CreateKnowledgeBaseStmt:
		return "knowledge-base", s.Name.String(), s.CreateOrModify
	case *ast.CreateConsumedMCPServiceStmt:
		return "consumed-mcp-service", s.Name.String(), s.CreateOrModify
	case *ast.CreateAgentStmt:
		return "agent", s.Name.String(), s.CreateOrModify
	case *ast.CreateImageCollectionStmt:
		return "image-collection", s.Name.String(), s.CreateOrModify
	}
	return "", "", false
}

// stmtDropInfo returns the doc-type key and qualified name for DROP statements.
// Returns empty strings when the statement is not a tracked DROP.
func stmtDropInfo(stmt ast.Statement) (docType, name string) {
	switch s := stmt.(type) {
	case *ast.DropModuleStmt:
		return "module", s.Name
	case *ast.DropEntityStmt:
		return "entity", s.Name.String()
	case *ast.DropEnumerationStmt:
		return "enumeration", s.Name.String()
	case *ast.DropAssociationStmt:
		return "association", s.Name.String()
	case *ast.DropConstantStmt:
		return "constant", s.Name.String()
	case *ast.DropMicroflowStmt:
		return "microflow", s.Name.String()
	case *ast.DropNanoflowStmt:
		return "nanoflow", s.Name.String()
	case *ast.DropPageStmt:
		return "page", s.Name.String()
	case *ast.DropSnippetStmt:
		return "snippet", s.Name.String()
	case *ast.DropJavaActionStmt:
		return "javaaction", s.Name.String()
	case *ast.DropJavaScriptActionStmt:
		return "javascriptaction", s.Name.String()
	case *ast.DropWorkflowStmt:
		return "workflow", s.Name.String()
	case *ast.DropBusinessEventServiceStmt:
		return "business-event-service", s.Name.String()
	case *ast.DropPublishedRestServiceStmt:
		return "published-rest-service", s.Name.String()
	case *ast.DropJsonStructureStmt:
		return "json-structure", s.Name.String()
	case *ast.DropImportMappingStmt:
		return "import-mapping", s.Name.String()
	case *ast.DropExportMappingStmt:
		return "export-mapping", s.Name.String()
	case *ast.DropDataTransformerStmt:
		return "data-transformer", s.Name.String()
	case *ast.DropModelStmt:
		return "agent-model", s.Name.String()
	case *ast.DropKnowledgeBaseStmt:
		return "knowledge-base", s.Name.String()
	case *ast.DropConsumedMCPServiceStmt:
		return "consumed-mcp-service", s.Name.String()
	case *ast.DropAgentStmt:
		return "agent", s.Name.String()
	case *ast.DropImageCollectionStmt:
		return "image-collection", s.Name.String()
	}
	return "", ""
}

// renameDocType maps a RenameStmt.ObjectType string to the registry doc-type
// key. Returns "" for object types we don't track (or that use renameModule).
func renameDocType(objectType string) string {
	switch objectType {
	case "entity":
		return "entity"
	case "enumeration":
		return "enumeration"
	case "association":
		return "association"
	case "constant":
		return "constant"
	case "microflow":
		return "microflow"
	case "nanoflow":
		return "nanoflow"
	case "page":
		return "page"
	case "workflow":
		return "workflow"
	case "javaaction":
		return "javaaction"
	}
	return ""
}

// friendlyDocType returns a human-readable label for a doc-type key.
func friendlyDocType(docType string) string {
	switch docType {
	case "agent-model":
		return "model"
	case "business-event-service":
		return "business event service"
	case "consumed-mcp-service":
		return "consumed MCP service"
	case "data-transformer":
		return "data transformer"
	case "export-mapping":
		return "export mapping"
	case "image-collection":
		return "image collection"
	case "import-mapping":
		return "import mapping"
	case "javaaction":
		return "java action"
	case "json-structure":
		return "JSON structure"
	case "knowledge-base":
		return "knowledge base"
	case "published-rest-service":
		return "published REST service"
	default:
		return docType
	}
}

// ----------------------------------------------------------------------------
// Phase 1: CheckScriptDuplicates
// ----------------------------------------------------------------------------

// CheckScriptDuplicates walks prog in statement order and reports any CREATE
// that targets a name that is already alive in the script (created earlier and
// not yet dropped). The check is type-aware: a microflow and a page may share
// the same qualified name without triggering a violation.
//
// CREATE OR MODIFY / OR REPLACE are never flagged. DROP removes a name from
// the live set. RENAME removes the old name and adds the new one.
func CheckScriptDuplicates(prog *ast.Program) []linter.Violation {
	var violations []linter.Violation
	reg := newNameRegistry()

	for i, stmt := range prog.Statements {
		stmtNum := i + 1

		// RENAME — remove old name, add new name (module renames cascade)
		if s, ok := stmt.(*ast.RenameStmt); ok && !s.DryRun {
			if s.ObjectType == "module" {
				reg.renameModule(s.Name.Name, s.NewName)
			} else if dt := renameDocType(s.ObjectType); dt != "" {
				oldQN := s.Name.String()
				newQN := s.Name.Module + "." + s.NewName
				reg.remove(dt, oldQN)
				reg.add(dt, newQN, stmtNum)
			}
			continue
		}

		// DROP — remove from live set
		if dt, name := stmtDropInfo(stmt); dt != "" {
			reg.remove(dt, name)
			continue
		}

		// CREATE — check for duplicate, then add to live set
		dt, name, idempotent := stmtCreateInfo(stmt)
		if dt == "" {
			continue
		}
		if idempotent {
			// OR MODIFY / OR REPLACE: add if absent, no error if present
			if !reg.isAlive(dt, name) {
				reg.add(dt, name, stmtNum)
			}
			continue
		}
		if firstIdx, exists := reg.has(dt, name); exists {
			violations = append(violations, linter.Violation{
				RuleID:   "MDL-DUPDEF",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s already defined in this script: %s (first defined at statement %d)",
					friendlyDocType(dt), name, firstIdx,
				),
				Suggestion: "use CREATE OR MODIFY to update an existing document, or DROP it before re-creating",
			})
		} else {
			reg.add(dt, name, stmtNum)
		}
	}

	return violations
}

// ----------------------------------------------------------------------------
// Phase 2: CheckProjectConflicts
// ----------------------------------------------------------------------------

// projectNameSets holds sets of qualified names already present in the project,
// loaded once before the walk.
type projectNameSets struct {
	entities         map[string]bool
	enumerations     map[string]bool
	constants        map[string]bool
	microflows       map[string]bool
	nanoflows        map[string]bool
	pages            map[string]bool
	snippets         map[string]bool
	javaActions      map[string]bool
	workflows        map[string]bool
	businessEvents   map[string]bool
	publishedRest    map[string]bool
	jsonStructures   map[string]bool
	importMappings   map[string]bool
	exportMappings   map[string]bool
	dataTransformers map[string]bool
	agentModels      map[string]bool
	knowledgeBases   map[string]bool
	consumedMcp      map[string]bool
	agents           map[string]bool
	imageCollections map[string]bool
}

// projectSetFor returns the existence set for the given doc-type key, or nil
// if we don't check project conflicts for that type.
func (ps *projectNameSets) setFor(docType string) map[string]bool {
	switch docType {
	case "entity":
		return ps.entities
	case "enumeration":
		return ps.enumerations
	case "constant":
		return ps.constants
	case "microflow":
		return ps.microflows
	case "nanoflow":
		return ps.nanoflows
	case "page":
		return ps.pages
	case "snippet":
		return ps.snippets
	case "javaaction":
		return ps.javaActions
	case "workflow":
		return ps.workflows
	case "business-event-service":
		return ps.businessEvents
	case "published-rest-service":
		return ps.publishedRest
	case "json-structure":
		return ps.jsonStructures
	case "import-mapping":
		return ps.importMappings
	case "export-mapping":
		return ps.exportMappings
	case "data-transformer":
		return ps.dataTransformers
	case "agent-model":
		return ps.agentModels
	case "knowledge-base":
		return ps.knowledgeBases
	case "consumed-mcp-service":
		return ps.consumedMcp
	case "agent":
		return ps.agents
	case "image-collection":
		return ps.imageCollections
	}
	return nil
}

// loadProjectNameSets queries the project for all existing document names.
func loadProjectNameSets(ctx *ExecContext) *projectNameSets {
	ps := &projectNameSets{}
	h, err := getHierarchy(ctx)
	if err != nil {
		// Return empty sets — callers treat empty as "no conflicts".
		return ps
	}

	ps.entities = buildEntityQualifiedNames(ctx)
	ps.microflows = buildMicroflowQualifiedNames(ctx)
	ps.nanoflows = buildNanoflowQualifiedNames(ctx)
	ps.pages = buildPageQualifiedNames(ctx)
	ps.snippets = buildSnippetQualifiedNames(ctx)
	ps.javaActions = buildJavaActionQualifiedNames(ctx)

	// Enumerations
	ps.enumerations = make(map[string]bool)
	if enums, err := ctx.Backend.ListEnumerations(); err == nil {
		for _, e := range enums {
			ps.enumerations[h.GetQualifiedName(e.ContainerID, e.Name)] = true
		}
	}

	// Constants
	ps.constants = make(map[string]bool)
	if consts, err := ctx.Backend.ListConstants(); err == nil {
		for _, c := range consts {
			ps.constants[h.GetQualifiedName(c.ContainerID, c.Name)] = true
		}
	}

	// Workflows
	ps.workflows = make(map[string]bool)
	if wfs, err := ctx.Backend.ListWorkflows(); err == nil {
		for _, w := range wfs {
			ps.workflows[h.GetQualifiedName(w.ContainerID, w.Name)] = true
		}
	}

	// Business event services
	ps.businessEvents = make(map[string]bool)
	if bes, err := ctx.Backend.ListBusinessEventServices(); err == nil {
		for _, b := range bes {
			ps.businessEvents[h.GetQualifiedName(b.ContainerID, b.Name)] = true
		}
	}

	// Published REST services
	ps.publishedRest = make(map[string]bool)
	if prs, err := ctx.Backend.ListPublishedRestServices(); err == nil {
		for _, p := range prs {
			ps.publishedRest[h.GetQualifiedName(p.ContainerID, p.Name)] = true
		}
	}

	// JSON structures
	ps.jsonStructures = make(map[string]bool)
	if jss, err := ctx.Backend.ListJsonStructures(); err == nil {
		for _, j := range jss {
			ps.jsonStructures[h.GetQualifiedName(j.ContainerID, j.Name)] = true
		}
	}

	// Import mappings
	ps.importMappings = make(map[string]bool)
	if ims, err := ctx.Backend.ListImportMappings(); err == nil {
		for _, m := range ims {
			ps.importMappings[h.GetQualifiedName(m.ContainerID, m.Name)] = true
		}
	}

	// Export mappings
	ps.exportMappings = make(map[string]bool)
	if ems, err := ctx.Backend.ListExportMappings(); err == nil {
		for _, m := range ems {
			ps.exportMappings[h.GetQualifiedName(m.ContainerID, m.Name)] = true
		}
	}

	// Data transformers
	ps.dataTransformers = make(map[string]bool)
	if dts, err := ctx.Backend.ListDataTransformers(); err == nil {
		for _, d := range dts {
			ps.dataTransformers[h.GetQualifiedName(d.ContainerID, d.Name)] = true
		}
	}

	// Agent editor: models
	ps.agentModels = make(map[string]bool)
	if ms, err := ctx.Backend.ListAgentEditorModels(); err == nil {
		for _, m := range ms {
			ps.agentModels[h.GetQualifiedName(m.ContainerID, m.Name)] = true
		}
	}

	// Agent editor: knowledge bases
	ps.knowledgeBases = make(map[string]bool)
	if kbs, err := ctx.Backend.ListAgentEditorKnowledgeBases(); err == nil {
		for _, k := range kbs {
			ps.knowledgeBases[h.GetQualifiedName(k.ContainerID, k.Name)] = true
		}
	}

	// Agent editor: consumed MCP services
	ps.consumedMcp = make(map[string]bool)
	if svcs, err := ctx.Backend.ListAgentEditorConsumedMCPServices(); err == nil {
		for _, s := range svcs {
			ps.consumedMcp[h.GetQualifiedName(s.ContainerID, s.Name)] = true
		}
	}

	// Agent editor: agents
	ps.agents = make(map[string]bool)
	if ags, err := ctx.Backend.ListAgentEditorAgents(); err == nil {
		for _, a := range ags {
			ps.agents[h.GetQualifiedName(a.ContainerID, a.Name)] = true
		}
	}

	// Image collections
	ps.imageCollections = make(map[string]bool)
	if ics, err := ctx.Backend.ListImageCollections(); err == nil {
		for _, ic := range ics {
			ps.imageCollections[h.GetQualifiedName(ic.ContainerID, ic.Name)] = true
		}
	}

	return ps
}

// CheckProjectConflicts walks prog in statement order and reports any plain
// CREATE (non-OR-MODIFY/OR-REPLACE) that targets a document name that already
// exists in the project. Names created earlier in the same script (and not yet
// dropped) are excluded from the project check — those conflicts will be caught
// by CheckScriptDuplicates instead.
//
// A DROP that targets a name present in the project records that name in the
// droppedFromProject registry. A subsequent CREATE for the same name is not
// flagged as a conflict because the script already removed it.
func CheckProjectConflicts(ctx *ExecContext, prog *ast.Program) []error {
	if !ctx.Connected() {
		return nil
	}

	ps := loadProjectNameSets(ctx)
	reg := newNameRegistry()
	droppedFromProject := newNameRegistry()
	var errs []error

	for i, stmt := range prog.Statements {
		stmtNum := i + 1

		// RENAME — update live registry
		if s, ok := stmt.(*ast.RenameStmt); ok && !s.DryRun {
			if s.ObjectType == "module" {
				reg.renameModule(s.Name.Name, s.NewName)
			} else if dt := renameDocType(s.ObjectType); dt != "" {
				reg.remove(dt, s.Name.String())
				reg.add(dt, s.Name.Module+"."+s.NewName, stmtNum)
			}
			continue
		}

		// DROP — remove from live registry; if the name existed in the project,
		// record it so a subsequent CREATE is not flagged as a conflict.
		if dt, name := stmtDropInfo(stmt); dt != "" {
			reg.remove(dt, name)
			projectSet := ps.setFor(dt)
			if projectSet != nil && projectSet[name] {
				droppedFromProject.add(dt, name, stmtNum)
			}
			continue
		}

		// CREATE — check for project conflict if not idempotent, not alive in
		// script, and not already dropped from the project earlier in this script.
		dt, name, idempotent := stmtCreateInfo(stmt)
		if dt == "" {
			continue
		}

		if !idempotent && !reg.isAlive(dt, name) && !droppedFromProject.isAlive(dt, name) {
			projectSet := ps.setFor(dt)
			if projectSet != nil && projectSet[name] {
				errs = append(errs, fmt.Errorf(
					"statement %d: %s already exists in project: %s — use CREATE OR MODIFY to update it",
					stmtNum, friendlyDocType(dt), name,
				))
			}
		}

		// Update live registry (idempotent adds only if absent)
		if idempotent {
			if !reg.isAlive(dt, name) {
				reg.add(dt, name, stmtNum)
			}
		} else {
			reg.add(dt, name, stmtNum)
		}
	}

	return errs
}
