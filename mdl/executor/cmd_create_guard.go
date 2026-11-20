// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

// createGuardTarget is what `create … if not exists` tests before the handler
// runs: the element's kind and name as the author wrote them, and a probe for
// whether the project already has it.
type createGuardTarget struct {
	kind   string
	name   string
	exists func(ctx *ExecContext) (bool, error)
}

// skipExistingCreate honours `create … if not exists` (ako/mxcli#731, ADR-0010
// R1): when the named element already exists, the statement is skipped and the
// stored element is left exactly as it is; otherwise the handler creates it as
// a plain `create` would. It reports whether the statement was skipped.
//
// It runs once, in the dispatch, for every statement that embeds
// ast.CreateGuard — the same place `drop … if exists` is honoured — so a new
// document type gets the guard by embedding it rather than by each handler
// remembering to test it.
//
// A probe that cannot answer is an error, not a "no": creating on a guess is
// exactly what the guard promises not to do.
func skipExistingCreate(ctx *ExecContext, stmt ast.Statement) (bool, error) {
	g, ok := stmt.(ast.IfNotExistsCreate)
	if !ok || !g.CreateIfNotExists() || !ctx.Connected() {
		return false, nil
	}
	target, ok := createGuardTargetOf(stmt)
	if !ok {
		// Entities and associations test the guard in their own handlers, which
		// also know the cross-module association forms.
		return false, nil
	}
	exists, err := target.exists(ctx)
	if err != nil {
		return false, mdlerrors.NewBackend(fmt.Sprintf("check whether %s %s exists", target.kind, target.name), err)
	}
	if !exists {
		return false, nil
	}
	fmt.Fprintf(ctx.Output, "%s %s already exists, skipped (if not exists)\n", target.kind, target.name)
	return true, nil
}

// createGuardTargetOf names the element a guarded create would make. ok is
// false for a statement whose handler tests the guard itself.
func createGuardTargetOf(stmt ast.Statement) (createGuardTarget, bool) {
	doc := func(kind string, qn ast.QualifiedName, list func(*ExecContext) (any, error)) (createGuardTarget, bool) {
		return createGuardTarget{kind: kind, name: qn.String(), exists: func(ctx *ExecContext) (bool, error) {
			items, err := list(ctx)
			if err != nil {
				return false, err
			}
			return documentListed(ctx, items, qn)
		}}, true
	}
	switch s := stmt.(type) {
	case *ast.CreateEntityStmt, *ast.CreateAssociationStmt:
		return createGuardTarget{}, false
	case *ast.CreateViewEntityStmt:
		return entityGuardTarget("view entity", s.Name), true
	case *ast.CreateExternalEntityStmt:
		return entityGuardTarget("external entity", s.Name), true
	case *ast.CreateModuleStmt:
		return createGuardTarget{kind: "module", name: s.Name, exists: func(ctx *ExecContext) (bool, error) {
			mods, err := ctx.Backend.ListModules()
			if err != nil {
				return false, err
			}
			for _, m := range mods {
				if m != nil && strings.EqualFold(m.Name, s.Name) {
					return true, nil
				}
			}
			return false, nil
		}}, true
	case *ast.CreateModuleRoleStmt:
		return createGuardTarget{kind: "module role", name: s.Name.String(), exists: func(ctx *ExecContext) (bool, error) {
			mod, err := findModule(ctx, s.Name.Module)
			if err != nil {
				// No module, no role: let the handler report the module.
				return false, nil
			}
			ms, err := ctx.Backend.GetModuleSecurity(mod.ID)
			if err != nil || ms == nil {
				return false, err
			}
			for _, mr := range ms.ModuleRoles {
				// Role names are case-insensitive (CE0123), as in the handler. An
				// auto-provisioned role is not the author's: the handler adopts it.
				if mr != nil && strings.EqualFold(mr.Name, s.Name.Name) && mr.Description != autoDocumentRoleDescription {
					return true, nil
				}
			}
			return false, nil
		}}, true
	case *ast.CreateUserRoleStmt:
		return createGuardTarget{kind: "user role", name: s.Name, exists: func(ctx *ExecContext) (bool, error) {
			ps, err := ctx.Backend.GetProjectSecurity()
			if err != nil || ps == nil {
				return false, err
			}
			for _, ur := range ps.UserRoles {
				if ur != nil && ur.Name == s.Name {
					return true, nil
				}
			}
			return false, nil
		}}, true
	case *ast.CreateDemoUserStmt:
		return createGuardTarget{kind: "demo user", name: s.UserName, exists: func(ctx *ExecContext) (bool, error) {
			ps, err := ctx.Backend.GetProjectSecurity()
			if err != nil || ps == nil {
				return false, err
			}
			for _, du := range ps.DemoUsers {
				if du != nil && du.UserName == s.UserName {
					return true, nil
				}
			}
			return false, nil
		}}, true
	case *ast.CreateConfigurationStmt:
		return createGuardTarget{kind: "configuration", name: s.Name, exists: func(ctx *ExecContext) (bool, error) {
			ps, err := ctx.Backend.GetProjectSettings()
			if err != nil || ps == nil || ps.Configuration == nil {
				return false, err
			}
			for _, cfg := range ps.Configuration.Configurations {
				if cfg != nil && strings.EqualFold(cfg.Name, s.Name) {
					return true, nil
				}
			}
			return false, nil
		}}, true
	case *ast.CreateMicroflowStmt:
		return doc("microflow", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListMicroflows() })
	case *ast.CreateNanoflowStmt:
		return doc("nanoflow", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListNanoflows() })
	case *ast.CreateRuleStmt:
		return doc("rule", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListRules() })
	case *ast.CreateJavaActionStmt:
		return doc("java action", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListJavaActions() })
	case *ast.CreateJavaScriptActionStmt:
		return doc("javascript action", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListJavaScriptActions() })
	case *ast.CreatePageStmtV3:
		return doc("page", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListPages() })
	case *ast.CreateSnippetStmtV3:
		return doc("snippet", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListSnippets() })
	case *ast.CreateLayoutStmt:
		return doc("layout", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListLayouts() })
	case *ast.CreateEnumerationStmt:
		return doc("enumeration", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListEnumerations() })
	case *ast.CreateConstantStmt:
		return doc("constant", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListConstants() })
	case *ast.CreateDatabaseConnectionStmt:
		return doc("database connection", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListDatabaseConnections() })
	case *ast.CreateRestClientStmt:
		return doc("rest client", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListConsumedRestServices() })
	case *ast.CreatePublishedRestServiceStmt:
		return doc("published rest service", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListPublishedRestServices() })
	case *ast.CreateODataClientStmt:
		return doc("odata client", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListConsumedODataServices() })
	case *ast.CreateODataServiceStmt:
		return doc("odata service", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListPublishedODataServices() })
	case *ast.CreateBusinessEventServiceStmt:
		return doc("business event service", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListBusinessEventServices() })
	case *ast.CreateWorkflowStmt:
		return doc("workflow", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListWorkflows() })
	case *ast.CreateImageCollectionStmt:
		return doc("image collection", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListImageCollections() })
	case *ast.CreateQueueStmt:
		return doc("task queue", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListQueues() })
	case *ast.CreateScheduledEventStmt:
		return doc("scheduled event", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListScheduledEvents() })
	case *ast.CreateRegularExpressionStmt:
		return doc("regular expression", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListRegularExpressions() })
	case *ast.CreateJsonStructureStmt:
		return doc("json structure", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListJsonStructures() })
	case *ast.CreateMessageDefinitionCollectionStmt:
		return doc("message definition collection", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListMessageDefinitionCollections() })
	case *ast.CreateMessageDefinitionStmt:
		return doc("message definition", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListMessageDefinitionDocuments() })
	case *ast.CreateImportMappingStmt:
		return doc("import mapping", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListImportMappings() })
	case *ast.CreateExportMappingStmt:
		return doc("export mapping", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListExportMappings() })
	case *ast.CreateDataTransformerStmt:
		return doc("data transformer", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListDataTransformers() })
	case *ast.CreateModelStmt:
		return doc("model", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListAgentEditorModels() })
	case *ast.CreateConsumedMCPServiceStmt:
		return doc("consumed mcp service", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListAgentEditorConsumedMCPServices() })
	case *ast.CreatePublishedMCPServiceStmt:
		return doc("published mcp service", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListAgentEditorPublishedMCPServices() })
	case *ast.CreateKnowledgeBaseStmt:
		return doc("knowledge base", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListAgentEditorKnowledgeBases() })
	case *ast.CreateAgentStmt:
		return doc("agent", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListAgentEditorAgents() })
	case *ast.CreateMenuStmt:
		return doc("menu", s.Name, func(ctx *ExecContext) (any, error) { return ctx.Backend.ListMenuDocuments() })
	}
	// A statement that embeds the guard but is not listed here would have it
	// silently ignored; TestCreateGuardTargetCoversEveryGuardedStatement fails
	// first. At run time, refuse rather than create on a guess.
	return createGuardTarget{kind: fmt.Sprintf("%T", stmt), exists: func(*ExecContext) (bool, error) {
		return false, fmt.Errorf("`if not exists` has no existence check for %T", stmt)
	}}, true
}

// entityGuardTarget probes the domain models for an entity of any kind: a view
// or external entity shares its name space with every other entity.
func entityGuardTarget(kind string, qn ast.QualifiedName) createGuardTarget {
	return createGuardTarget{kind: kind, name: qn.String(), exists: func(ctx *ExecContext) (bool, error) {
		mod, err := findModule(ctx, qn.Module)
		if err != nil {
			return false, nil // the handler reports the missing module
		}
		dms, err := ctx.Backend.ListDomainModels()
		if err != nil {
			return false, err
		}
		for _, dm := range dms {
			if dm == nil || dm.ContainerID != mod.ID {
				continue
			}
			for _, e := range dm.Entities {
				if e != nil && strings.EqualFold(e.Name, qn.Name) {
					return true, nil
				}
			}
		}
		return false, nil
	}}
}

// documentListed reports whether items — a slice of pointers to document
// structs, each with a ContainerID and a Name, as every Backend.List* returns —
// holds the document qn. Names compare case-insensitively: Mendix refuses two
// documents in one module whose names differ only in case, so an element that
// matches that way is the one a create would collide with.
func documentListed(ctx *ExecContext, items any, qn ast.QualifiedName) (bool, error) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return false, err
	}
	if h == nil {
		return false, fmt.Errorf("no project hierarchy")
	}
	want := qn.String()
	v := reflect.ValueOf(items)
	if v.Kind() != reflect.Slice {
		return false, fmt.Errorf("document list is %T, not a slice", items)
	}
	for i := 0; i < v.Len(); i++ {
		e := v.Index(i)
		if e.Kind() == reflect.Pointer {
			if e.IsNil() {
				continue
			}
			e = e.Elem()
		}
		if e.Kind() != reflect.Struct {
			return false, fmt.Errorf("document list element is %s, not a struct", e.Type())
		}
		cid, name := e.FieldByName("ContainerID"), e.FieldByName("Name")
		if !cid.IsValid() || !name.IsValid() {
			return false, fmt.Errorf("document list element %s has no ContainerID or Name", e.Type())
		}
		id, ok := cid.Interface().(model.ID)
		if !ok {
			return false, fmt.Errorf("%s.ContainerID is %s, not model.ID", e.Type(), cid.Type())
		}
		if strings.EqualFold(h.GetQualifiedName(id, name.String()), want) {
			return true, nil
		}
	}
	return false, nil
}

// validateCreateGuardContradiction reports `create or modify … if not exists`
// (MDL085) on every guarded document kind. The two guards contradict each
// other: `or modify` makes the stored element match the statement, `if not
// exists` leaves it untouched, and which one wins is not readable from the
// statement. Entities and associations report it from their own flags.
func validateCreateGuardContradiction(stmt ast.Statement) []linter.Violation {
	g, ok := stmt.(ast.IfNotExistsCreate)
	if !ok || !g.CreateGuardContradicts() {
		return nil
	}
	target, ok := createGuardTargetOf(stmt)
	if !ok {
		return nil
	}
	return validateIdempotencyGuard(true, true, target.kind, target.name)
}
