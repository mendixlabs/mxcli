// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseScript(t *testing.T, mdl string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return prog
}

func dupViolationMessages(t *testing.T, mdl string) []string {
	t.Helper()
	prog := parseScript(t, mdl)
	vs := CheckScriptDuplicates(prog)
	msgs := make([]string, len(vs))
	for i, v := range vs {
		msgs[i] = v.Message
	}
	return msgs
}

func assertNoDupViolations(t *testing.T, mdl string) {
	t.Helper()
	msgs := dupViolationMessages(t, mdl)
	if len(msgs) != 0 {
		t.Errorf("expected no duplicate violations but got: %v", msgs)
	}
}

func assertHasDupViolation(t *testing.T, mdl, containsName string) {
	t.Helper()
	msgs := dupViolationMessages(t, mdl)
	for _, m := range msgs {
		if strings.Contains(m, containsName) {
			return
		}
	}
	t.Errorf("expected a duplicate violation mentioning %q, got: %v", containsName, msgs)
}

// ---------------------------------------------------------------------------
// Phase 1: CheckScriptDuplicates — basic CREATE duplication
// ---------------------------------------------------------------------------

func TestCheckScriptDuplicates_CreateCreate_Entity(t *testing.T) {
	assertHasDupViolation(t, `
create module Dup;
create persistent entity Dup.Customer (Name: string);
create persistent entity Dup.Customer (Name: string);
`, "Dup.Customer")
}

func TestCheckScriptDuplicates_CreateCreate_Microflow(t *testing.T) {
	assertHasDupViolation(t, `
create module Dup;
create microflow Dup.MF_Test () begin return; end;
create microflow Dup.MF_Test () begin return; end;
`, "Dup.MF_Test")
}

func TestCheckScriptDuplicates_CreateCreate_JavaAction(t *testing.T) {
	assertHasDupViolation(t, `
create module Dup;
create java action Dup.Helper() returns boolean as $$ return false; $$;
create java action Dup.Helper() returns boolean as $$ return false; $$;
`, "Dup.Helper")
}

func TestCheckScriptDuplicates_CreateCreate_Enumeration(t *testing.T) {
	assertHasDupViolation(t, `
create module Dup;
create enumeration Dup.Status (Active 'Active', Inactive 'Inactive');
create enumeration Dup.Status (Active 'Active', Inactive 'Inactive');
`, "Dup.Status")
}

func TestCheckScriptDuplicates_CreateCreate_Workflow(t *testing.T) {
	assertHasDupViolation(t, `
create module Dup;
create workflow Dup.WF_Process begin end workflow;
create workflow Dup.WF_Process begin end workflow;
`, "Dup.WF_Process")
}

// ---------------------------------------------------------------------------
// Phase 1: DROP clears the name — CREATE, DROP, CREATE should be clean
// ---------------------------------------------------------------------------

func TestCheckScriptDuplicates_CreateDropCreate_Entity(t *testing.T) {
	assertNoDupViolations(t, `
create module Dup;
create persistent entity Dup.Customer (Name: string);
drop entity Dup.Customer;
create persistent entity Dup.Customer (Name: string);
`)
}

func TestCheckScriptDuplicates_CreateDropCreate_Microflow(t *testing.T) {
	assertNoDupViolations(t, `
create module Dup;
create microflow Dup.MF_Test () begin return; end;
drop microflow Dup.MF_Test;
create microflow Dup.MF_Test () begin return; end;
`)
}

func TestCheckScriptDuplicates_CreateDropCreate_Workflow(t *testing.T) {
	assertNoDupViolations(t, `
create module Dup;
create workflow Dup.WF_Process begin end workflow;
drop workflow Dup.WF_Process;
create workflow Dup.WF_Process begin end workflow;
`)
}

// ---------------------------------------------------------------------------
// Phase 1: CREATE OR MODIFY is never flagged
// ---------------------------------------------------------------------------

func TestCheckScriptDuplicates_OrModify_NoError(t *testing.T) {
	assertNoDupViolations(t, `
create module Dup;
create persistent entity Dup.Customer (Name: string);
create or modify persistent entity Dup.Customer (Name: string);
`)
}

func TestCheckScriptDuplicates_OrModifyOnly_NoError(t *testing.T) {
	// OR MODIFY as first occurrence is also fine
	assertNoDupViolations(t, `
create module Dup;
create or modify persistent entity Dup.Customer (Name: string);
create or modify persistent entity Dup.Customer (Name: string);
`)
}

func TestCheckScriptDuplicates_OrModify_Microflow(t *testing.T) {
	assertNoDupViolations(t, `
create module Dup;
create microflow Dup.MF_Test () begin return; end;
create or modify microflow Dup.MF_Test () begin return; end;
`)
}

// ---------------------------------------------------------------------------
// Phase 1: Different types with the same qualified name are independent
// ---------------------------------------------------------------------------

func TestCheckScriptDuplicates_DifferentTypes_SameQN_NoError(t *testing.T) {
	// A microflow and a page can share the same qualified name without conflict.
	assertNoDupViolations(t, `
create module Dup;
create microflow Dup.Foo () begin return; end;
create page Dup.Foo ( title: 'Foo' ) {};
`)
}

// ---------------------------------------------------------------------------
// Phase 1: RENAME removes old name and adds new name
// ---------------------------------------------------------------------------

func TestCheckScriptDuplicates_RenameAllowsCreateOldName(t *testing.T) {
	// After renaming Entity.Customer to Client, creating Customer again is fine.
	assertNoDupViolations(t, `
create module Dup;
create persistent entity Dup.Customer (Name: string);
rename entity Dup.Customer to Client;
create persistent entity Dup.Customer (Name: string);
`)
}

func TestCheckScriptDuplicates_RenameBlocksCreateNewName(t *testing.T) {
	// After renaming Customer to Client, creating Client is a duplicate.
	assertHasDupViolation(t, `
create module Dup;
create persistent entity Dup.Customer (Name: string);
rename entity Dup.Customer to Client;
create persistent entity Dup.Client (Email: string);
`, "Dup.Client")
}

// ---------------------------------------------------------------------------
// Phase 1: RENAME MODULE cascades across all names
// ---------------------------------------------------------------------------

func TestCheckScriptDuplicates_RenameModuleCascade_AllowsOldPrefix(t *testing.T) {
	// After rename module OldMod to NewMod, creating OldMod.Foo is fine.
	assertNoDupViolations(t, `
create module OldMod;
create persistent entity OldMod.Foo (X: string);
rename module OldMod to NewMod;
create module OldMod;
create persistent entity OldMod.Foo (X: string);
`)
}

func TestCheckScriptDuplicates_RenameModuleCascade_BlocksNewPrefix(t *testing.T) {
	// After rename module OldMod to NewMod, creating NewMod.Foo is a duplicate.
	assertHasDupViolation(t, `
create module OldMod;
create persistent entity OldMod.Foo (X: string);
rename module OldMod to NewMod;
create persistent entity NewMod.Foo (X: string);
`, "NewMod.Foo")
}

// ---------------------------------------------------------------------------
// Phase 1: RENAME DRY RUN does not affect the registry
// ---------------------------------------------------------------------------

func TestCheckScriptDuplicates_RenameDryRun_NoRegistryChange(t *testing.T) {
	// DRY RUN rename should not update the registry.
	assertHasDupViolation(t, `
create module Dup;
create persistent entity Dup.Customer (Name: string);
rename entity Dup.Customer to Client dry run;
create persistent entity Dup.Customer (Name: string);
`, "Dup.Customer")
}

// ---------------------------------------------------------------------------
// Phase 2: CheckProjectConflicts — project-side existence checks
// ---------------------------------------------------------------------------

// setupProjectConflictCtx creates a mock context with a workflow "M.ExistingWF"
// and a microflow "M.ExistingMF" already present in the project.
func setupProjectConflictCtx(t *testing.T) (*ExecContext, *model.Module) {
	t.Helper()
	mod := mkModule("M")
	h := mkHierarchy(mod)

	wf := mkWorkflow(mod.ID, "ExistingWF")
	mf := mkMicroflow(mod.ID, "ExistingMF")

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) {
			return []*workflows.Workflow{wf}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{mf}, nil
		},
		// Other list functions return empty (no conflicts for those types)
		ListEnumerationsFunc:              func() ([]*model.Enumeration, error) { return nil, nil },
		ListConstantsFunc:                 func() ([]*model.Constant, error) { return nil, nil },
		ListBusinessEventServicesFunc:     func() ([]*model.BusinessEventService, error) { return nil, nil },
		ListPublishedRestServicesFunc:     func() ([]*model.PublishedRestService, error) { return nil, nil },
		ListJsonStructuresFunc:            func() ([]*types.JsonStructure, error) { return nil, nil },
		ListImportMappingsFunc:            func() ([]*model.ImportMapping, error) { return nil, nil },
		ListExportMappingsFunc:            func() ([]*model.ExportMapping, error) { return nil, nil },
		ListDataTransformersFunc:          func() ([]*model.DataTransformer, error) { return nil, nil },
		ListAgentEditorModelsFunc:         func() ([]*agenteditor.Model, error) { return nil, nil },
		ListAgentEditorKnowledgeBasesFunc: func() ([]*agenteditor.KnowledgeBase, error) { return nil, nil },
		ListAgentEditorConsumedMCPServicesFunc: func() ([]*agenteditor.ConsumedMCPService, error) {
			return nil, nil
		},
		ListAgentEditorAgentsFunc: func() ([]*agenteditor.Agent, error) { return nil, nil },
		ListImageCollectionsFunc:  func() ([]*types.ImageCollection, error) { return nil, nil },
		ListDomainModelsFunc:      func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListNanoflowsFunc:         func() ([]*microflows.Nanoflow, error) { return nil, nil },
		ListPagesFunc:             func() ([]*pages.Page, error) { return nil, nil },
		ListSnippetsFunc:          func() ([]*pages.Snippet, error) { return nil, nil },
		ListJavaActionsFunc:       func() ([]*types.JavaAction, error) { return nil, nil },
		ListJavaScriptActionsFunc: func() ([]*types.JavaScriptAction, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, mod
}

func conflictErrorMessages(ctx *ExecContext, mdl string, t *testing.T) []string {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	errsOut := CheckProjectConflicts(ctx, prog)
	msgs := make([]string, len(errsOut))
	for i, e := range errsOut {
		msgs[i] = e.Error()
	}
	return msgs
}

func assertNoConflicts(t *testing.T, ctx *ExecContext, mdl string) {
	t.Helper()
	msgs := conflictErrorMessages(ctx, mdl, t)
	if len(msgs) != 0 {
		t.Errorf("expected no project conflicts but got: %v", msgs)
	}
}

func assertHasConflict(t *testing.T, ctx *ExecContext, mdl, containsName string) {
	t.Helper()
	msgs := conflictErrorMessages(ctx, mdl, t)
	for _, m := range msgs {
		if strings.Contains(m, containsName) {
			return
		}
	}
	t.Errorf("expected conflict mentioning %q, got: %v", containsName, msgs)
}

func TestCheckProjectConflicts_CreateNew_NoError(t *testing.T) {
	ctx, _ := setupProjectConflictCtx(t)
	// BrandNewWF does not exist in project
	assertNoConflicts(t, ctx, `
create module M;
create workflow M.BrandNewWF begin end workflow;
`)
}

func TestCheckProjectConflicts_CreateExisting_Error(t *testing.T) {
	ctx, _ := setupProjectConflictCtx(t)
	assertHasConflict(t, ctx, `
create module M;
create workflow M.ExistingWF begin end workflow;
`, "M.ExistingWF")
}

func TestCheckProjectConflicts_CreateOrModify_NoError(t *testing.T) {
	ctx, _ := setupProjectConflictCtx(t)
	// OR MODIFY is idempotent — never a conflict
	assertNoConflicts(t, ctx, `
create module M;
create or modify workflow M.ExistingWF begin end workflow;
`)
}

func TestCheckProjectConflicts_CreateExistingMicroflow_Error(t *testing.T) {
	ctx, _ := setupProjectConflictCtx(t)
	assertHasConflict(t, ctx, `
create module M;
create microflow M.ExistingMF () begin return; end;
`, "M.ExistingMF")
}

func TestCheckProjectConflicts_ScriptCreatedFirst_NoProjectCheck(t *testing.T) {
	ctx, _ := setupProjectConflictCtx(t)
	// The script creates M.BrandNew first — even if it existed in the project,
	// the second CREATE (within the script) would be caught by Phase 1 instead.
	// Phase 2 skips the project check for names already alive in the registry.
	assertNoConflicts(t, ctx, `
create module M;
create workflow M.BrandNew begin end workflow;
create or modify workflow M.BrandNew begin end workflow;
`)
}

func TestCheckProjectConflicts_DropThenCreate_ChecksProjectAgain(t *testing.T) {
	ctx, _ := setupProjectConflictCtx(t)
	// The first CREATE conflicts because ExistingWF is in the project.
	// The DROP records ExistingWF as dropped-from-project, so the second CREATE
	// is not flagged — the script already removed it.
	msgs := conflictErrorMessages(ctx, `
create module M;
create workflow M.ExistingWF begin end workflow;
drop workflow M.ExistingWF;
create workflow M.ExistingWF begin end workflow;
`, t)
	// Only the first CREATE should be flagged; the re-create after DROP is clean.
	count := 0
	for _, m := range msgs {
		if strings.Contains(m, "M.ExistingWF") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 conflict for M.ExistingWF (first CREATE only), got %d: %v", count, msgs)
	}
}

func TestCheckProjectConflicts_DropThenCreate_NoConflict(t *testing.T) {
	ctx, _ := setupProjectConflictCtx(t)
	// A script that drops an existing project entity and immediately recreates it
	// with the same name should produce zero conflicts.
	assertNoConflicts(t, ctx, `
create module M;
drop workflow M.ExistingWF;
create workflow M.ExistingWF begin end workflow;
`)
}

func TestCheckProjectConflicts_NotConnected_NoErrors(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	prog := parseScript(t, `create module M;`)
	errs := CheckProjectConflicts(ctx, prog)
	if len(errs) != 0 {
		t.Errorf("expected no errors when not connected, got: %v", errs)
	}
}
