// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
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
// CREATE is idempotent. Returns empty strings when the statement is not a
// tracked CREATE.
//
// Idempotent means "exec will not fail on an element that already exists".
// There are three spellings, not two: OR MODIFY, OR REPLACE, and IF NOT EXISTS
// — the last skips the element rather than rewriting it, which is why
// re-runnable domain scripts use it. Missing it makes the check disagree with
// what exec does, and the check is wrong: a `create entity if not exists` was
// reported as a conflict for a statement exec cleanly skips.
// TestIfNotExistsCountsAsIdempotent guards the mapping. The guard is read once,
// here, for every document kind that carries it (ako/mxcli#731).
func stmtCreateInfo(stmt ast.Statement) (docType, name string, idempotent bool) {
	docType, name, idempotent = stmtCreateKind(stmt)
	if g, ok := stmt.(ast.IfNotExistsCreate); ok && g.CreateIfNotExists() {
		idempotent = true
	}
	return docType, name, idempotent
}

// stmtCreateKind is stmtCreateInfo without the `if not exists` guard: the
// doc-type key, the qualified name, and whether `or modify` / `or replace`
// makes the create idempotent.
func stmtCreateKind(stmt ast.Statement) (docType, name string, idempotent bool) {
	switch s := stmt.(type) {
	case *ast.CreateModuleStmt:
		// `or modify` applies the module's documentation and settings to a
		// module that exists (ako/mxcli#1921), so restating it is not a
		// redefinition.
		return "module", s.Name, s.CreateOrModify
	case *ast.CreateModuleRoleStmt:
		// Not a document, which is why it was missed when the document types
		// were swept (mendixlabs/mxcli#1067). exec refuses a plain CREATE of an
		// existing role, so check has to as well.
		return "module-role", s.Name.String(), s.CreateOrModify
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
	case *ast.CreateRuleStmt:
		return "rule", s.Name.String(), s.CreateOrModify
	case *ast.CreatePageStmtV3:
		return "page", s.Name.String(), s.IsModify || s.IsReplace
	case *ast.CreateSnippetStmtV3:
		return "snippet", s.Name.String(), s.IsModify || s.IsReplace
	case *ast.CreateLayoutStmt:
		return "layout", s.Name.String(), s.IsModify || s.IsReplace
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
	case *ast.CreateDemoUserStmt:
		// Not a document either, and like a module role it was missed by the
		// document sweep: exec refuses a plain CREATE of a user that exists
		// ("demo user already exists"), and check passed it (ako/mxcli#906).
		// The name is the user name, which is not module-qualified.
		return "demo-user", s.UserName, s.CreateOrModify
	case *ast.CreateUserRoleStmt:
		// Not a document, and missed by both switches at once, which the
		// switch-against-switch guard could not see (ako/mxcli#557): exec
		// refuses a plain CREATE of a role the project has — and a blank app
		// already ships `Administrator` — so check has to as well.
		return "user-role", s.Name, s.CreateOrModify
	case *ast.CreateConfigurationStmt:
		return "configuration", s.Name, s.CreateOrModify
	case *ast.CreateMenuStmt:
		return "menu", s.Name.String(), s.CreateOrModify
	case *ast.CreateQueueStmt:
		return "queue", s.Name.String(), s.CreateOrModify
	case *ast.CreateScheduledEventStmt:
		return "scheduled-event", s.Name.String(), s.CreateOrModify
	case *ast.CreateRegularExpressionStmt:
		return "regular-expression", s.Name.String(), s.CreateOrModify
	case *ast.CreateMessageDefinitionCollectionStmt:
		return "message-definition-collection", s.Name.String(), s.CreateOrModify
	case *ast.CreateMessageDefinitionStmt:
		return "message-definition", s.Name.String(), s.CreateOrModify
	case *ast.CreateDatabaseConnectionStmt:
		return "database-connection", s.Name.String(), s.CreateOrModify
	case *ast.CreateRestClientStmt:
		return "rest-client", s.Name.String(), s.CreateOrModify
	case *ast.CreateODataClientStmt:
		return "odata-client", s.Name.String(), s.CreateOrModify
	case *ast.CreateODataServiceStmt:
		return "odata-service", s.Name.String(), s.CreateOrModify
	}
	return "", "", false
}

// unqualifiedCreateKinds are the kinds stmtCreateKind names without a module:
// project-level elements, whose names are not Module.Name.
var unqualifiedCreateKinds = map[string]bool{
	"module":        true,
	"demo-user":     true,
	"user-role":     true,
	"configuration": true,
}

// stmtDropInfo returns the doc-type key and qualified name for DROP statements.
// Returns empty strings when the statement is not a tracked DROP.
func stmtDropInfo(stmt ast.Statement) (docType, name string) {
	switch s := stmt.(type) {
	case *ast.DropModuleStmt:
		return "module", s.Name
	case *ast.DropModuleRoleStmt:
		return "module-role", s.Name.String()
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
	case *ast.DropRuleStmt:
		return "rule", s.Name.String()
	case *ast.DropPageStmt:
		return "page", s.Name.String()
	case *ast.DropLayoutStmt:
		return "layout", s.Name.String()
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
	case *ast.DropDemoUserStmt:
		return "demo-user", s.UserName
	case *ast.DropUserRoleStmt:
		return "user-role", s.Name
	case *ast.DropConfigurationStmt:
		return "configuration", s.Name
	case *ast.DropMenuStmt:
		return "menu", s.Name.String()
	case *ast.DropQueueStmt:
		return "queue", s.Name.String()
	case *ast.DropScheduledEventStmt:
		return "scheduled-event", s.Name.String()
	case *ast.DropRegularExpressionStmt:
		return "regular-expression", s.Name.String()
	case *ast.DropMessageDefinitionCollectionStmt:
		return "message-definition-collection", s.Name.String()
	case *ast.DropMessageDefinitionStmt:
		return "message-definition", s.Name.String()
	case *ast.DropDatabaseConnectionStmt:
		return "database-connection", s.Name.String()
	case *ast.DropRestClientStmt:
		return "rest-client", s.Name.String()
	case *ast.DropODataClientStmt:
		return "odata-client", s.Name.String()
	case *ast.DropODataServiceStmt:
		return "odata-service", s.Name.String()
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
	case "javascriptaction":
		return "javascript action"
	case "module-role":
		return "module role"
	case "demo-user":
		return "demo user"
	case "user-role":
		return "user role"
	case "scheduled-event":
		return "scheduled event"
	case "regular-expression":
		return "regular expression"
	case "message-definition-collection":
		return "message definition collection"
	case "message-definition":
		return "message definition"
	case "database-connection":
		return "database connection"
	case "rest-client":
		return "REST client"
	case "odata-client":
		return "OData client"
	case "odata-service":
		return "OData service"
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
		// A name another kind in the same name space holds (MDL-DUPNAME,
		// ako/mxcli#793). Checked for every spelling: `or modify` of a kind
		// that does not have the name still adds an element.
		if v := checkScriptNameClash(reg, dt, name); v != nil {
			violations = append(violations, *v)
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

// projectNameSets answers which qualified names of a kind the project already
// holds. Each kind is listed on first use and kept: a listing is a scan of the
// stored model (on a 140 MB MPR v1 project ~0.1 s per document kind, and ~5 s
// for module roles, which read every module's security), and the conflict
// check asks about only the kinds its script plainly creates — usually none.
type projectNameSets struct {
	ctx    *ExecContext
	h      *ContainerHierarchy
	loaded map[string]map[string]bool
}

// setFor returns the existence set for the given doc-type key, listing that
// kind on first use, or nil if we don't check project conflicts for that type.
func (ps *projectNameSets) setFor(docType string) map[string]bool {
	if set, ok := ps.loaded[docType]; ok {
		return set
	}
	if ps.h == nil {
		// No hierarchy, no qualified names — callers treat nil as "no conflicts".
		return nil
	}
	ctx, h := ps.ctx, ps.h
	var set map[string]bool
	switch docType {
	case "entity":
		set = buildEntityQualifiedNames(ctx)
	case "enumeration":
		set = listedQualifiedNames(h, ctx.Backend.ListEnumerations)
	case "constant":
		set = listedQualifiedNames(h, ctx.Backend.ListConstants)
	case "microflow":
		set = buildMicroflowQualifiedNames(ctx)
	case "nanoflow":
		set = buildNanoflowQualifiedNames(ctx)
	case "page":
		set = buildPageQualifiedNames(ctx)
	case "snippet":
		set = buildSnippetQualifiedNames(ctx)
	case "layout":
		set = buildLayoutQualifiedNames(ctx)
	case "javaaction":
		set = buildJavaActionQualifiedNames(ctx)
	case "workflow":
		set = listedQualifiedNames(h, ctx.Backend.ListWorkflows)
	case "business-event-service":
		set = listedQualifiedNames(h, ctx.Backend.ListBusinessEventServices)
	case "published-rest-service":
		set = listedQualifiedNames(h, ctx.Backend.ListPublishedRestServices)
	case "json-structure":
		set = listedQualifiedNames(h, ctx.Backend.ListJsonStructures)
	case "import-mapping":
		set = listedQualifiedNames(h, ctx.Backend.ListImportMappings)
	case "export-mapping":
		set = listedQualifiedNames(h, ctx.Backend.ListExportMappings)
	case "data-transformer":
		set = listedQualifiedNames(h, ctx.Backend.ListDataTransformers)
	case "agent-model":
		set = listedQualifiedNames(h, ctx.Backend.ListAgentEditorModels)
	case "knowledge-base":
		set = listedQualifiedNames(h, ctx.Backend.ListAgentEditorKnowledgeBases)
	case "consumed-mcp-service":
		set = listedQualifiedNames(h, ctx.Backend.ListAgentEditorConsumedMCPServices)
	case "agent":
		set = listedQualifiedNames(h, ctx.Backend.ListAgentEditorAgents)
	case "image-collection":
		set = listedQualifiedNames(h, ctx.Backend.ListImageCollections)
	case "association":
		// Intra-module and cross-module.
		set = buildAssociationQualifiedNames(ctx)
	case "rule":
		set = buildRuleQualifiedNames(ctx)
	case "javascriptaction":
		set = buildJavaScriptActionQualifiedNames(ctx)
	case "module-role":
		set = buildModuleRoleQualifiedNames(ctx)
	case "demo-user", "user-role":
		// The same lookups execCreateDemoUser and execCreateUserRole refuse on.
		set = map[string]bool{}
		if sec, err := ctx.Backend.GetProjectSecurity(); err == nil && sec != nil {
			if docType == "demo-user" {
				for _, du := range sec.DemoUsers {
					if du != nil {
						set[du.UserName] = true
					}
				}
			} else {
				for _, ur := range sec.UserRoles {
					if ur != nil {
						set[ur.Name] = true
					}
				}
			}
		}
	case "configuration":
		// Project settings, not documents, so not module-qualified.
		set = map[string]bool{}
		if st, err := ctx.Backend.GetProjectSettings(); err == nil && st != nil && st.Configuration != nil {
			for _, cfg := range st.Configuration.Configurations {
				if cfg != nil {
					set[cfg.Name] = true
				}
			}
		}
	// The documents whose handlers refuse a plain CREATE of an existing one,
	// read through the same Backend lists the handlers search (ako/mxcli#557).
	case "menu":
		set = listedQualifiedNames(h, ctx.Backend.ListMenuDocuments)
	case "queue":
		set = listedQualifiedNames(h, ctx.Backend.ListQueues)
	case "scheduled-event":
		set = listedQualifiedNames(h, ctx.Backend.ListScheduledEvents)
	case "regular-expression":
		set = listedQualifiedNames(h, ctx.Backend.ListRegularExpressions)
	case "message-definition-collection":
		set = listedQualifiedNames(h, ctx.Backend.ListMessageDefinitionCollections)
	case "message-definition":
		set = listedQualifiedNames(h, ctx.Backend.ListMessageDefinitionDocuments)
	case "database-connection":
		set = listedQualifiedNames(h, ctx.Backend.ListDatabaseConnections)
	case "rest-client":
		set = listedQualifiedNames(h, ctx.Backend.ListConsumedRestServices)
	case "odata-client":
		set = listedQualifiedNames(h, ctx.Backend.ListConsumedODataServices)
	case "odata-service":
		set = listedQualifiedNames(h, ctx.Backend.ListPublishedODataServices)
	default:
		// "module" is deliberately absent: CREATE MODULE on an existing module
		// is a no-op that prints "already exists" and exits 0, so `create
		// module M;` is the standard script preamble. Flagging it would be a
		// false positive on essentially every script.
		// TestEveryCreateDocTypeIsProjectChecked carries this exemption
		// explicitly so it stays a decision rather than an omission.
		return nil
	}
	ps.loaded[docType] = set
	return set
}

// execFoldsExistingName lists the kinds whose exec handler finds the stored
// element case-insensitively, so a plain CREATE of `M.q` over `M.Q` is refused
// as "already exists". The project check has to match the same way or it
// passes what exec then refuses.
var execFoldsExistingName = map[string]bool{
	"constant":               true,
	"business-event-service": true,
	"configuration":          true,
	"queue":                  true,
	"scheduled-event":        true,
	"regular-expression":     true,
	"database-connection":    true,
	"rest-client":            true,
	"odata-client":           true,
	"odata-service":          true,
}

// projectSetHas reports whether set holds name the way exec's handler for dt
// looks it up.
func projectSetHas(dt string, set map[string]bool, name string) bool {
	if set == nil {
		return false
	}
	if set[name] {
		return true
	}
	if !execFoldsExistingName[dt] {
		return false
	}
	for qn := range set {
		if strings.EqualFold(qn, name) {
			return true
		}
	}
	return false
}

// listedQualifiedNames is the set of qualified names of the documents list
// returns — any Backend.List* whose elements carry a ContainerID and a Name.
// A list that cannot be read yields no names, as every build*QualifiedNames
// does: the handler reads the same list and reports the failure itself.
func listedQualifiedNames[T any](h *ContainerHierarchy, list func() ([]*T, error)) map[string]bool {
	out := map[string]bool{}
	items, err := list()
	if err != nil || h == nil {
		return out
	}
	for _, it := range items {
		if it == nil {
			continue
		}
		v := reflect.ValueOf(it).Elem()
		cid, name := v.FieldByName("ContainerID"), v.FieldByName("Name")
		if !cid.IsValid() || !name.IsValid() || name.Kind() != reflect.String {
			continue
		}
		id, ok := cid.Interface().(model.ID)
		if !ok {
			continue
		}
		out[h.GetQualifiedName(id, name.String())] = true
	}
	return out
}

// loadProjectNameSets prepares the project-side name lookup. Nothing is listed
// here: setFor lists a kind the first time it is asked about.
func loadProjectNameSets(ctx *ExecContext) *projectNameSets {
	ps := &projectNameSets{ctx: ctx, loaded: map[string]map[string]bool{}}
	if h, err := getHierarchy(ctx); err == nil {
		ps.h = h
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
	// Only a plain CREATE is ever flagged, so only the kinds plainly created
	// need their stored names — and a DROP matters only to a later plain
	// CREATE of its kind. A script of `create or modify` statements lists
	// nothing here.
	plainlyCreated := map[string]bool{}
	for _, stmt := range prog.Statements {
		if dt, _, idempotent := stmtCreateInfo(stmt); dt != "" && !idempotent {
			plainlyCreated[dt] = true
		}
	}
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
			if plainlyCreated[dt] && projectSetHas(dt, ps.setFor(dt), name) {
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
			if projectSetHas(dt, ps.setFor(dt), name) {
				hint := "use CREATE OR MODIFY to update it"
				if dt == "entity" {
					// For a persistent entity, CREATE OR MODIFY rebuilds the whole
					// definition and drops any attribute this statement omits — steer
					// toward the incremental, non-destructive path instead. (findings #13)
					hint = "to add or change a member use 'alter entity " + name +
						" add attribute ...' (leaves the rest intact); use CREATE OR MODIFY only to replace the whole definition"
				}
				errs = append(errs, fmt.Errorf(
					"statement %d: %s already exists in project: %s — %s",
					stmtNum, friendlyDocType(dt), name, hint,
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

	// A create over a name another kind already has (ako/mxcli#793).
	errs = append(errs, CheckProjectNameClashes(ctx, prog)...)
	// The refusals exec decides from project state other than a document
	// listing: jar dependencies, translations (ako/mxcli#906).
	errs = append(errs, CheckExecRefusals(ctx, prog)...)
	return errs
}
