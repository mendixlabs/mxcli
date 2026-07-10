// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

// emptyRegistry creates a Registry with no handlers registered.
func emptyRegistry() *Registry {
	return &Registry{handlers: make(map[reflect.Type]StmtHandler)}
}

func TestNewRegistry_NoPanic(t *testing.T) {
	// Smoke test: constructing a registry with all stub registrations
	// must not panic.
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() returned nil")
	}
}

func TestRegistry_Dispatch_UnknownStatement(t *testing.T) {
	r := emptyRegistry()

	// ConnectStmt is not registered — Dispatch must return UnsupportedError.
	err := r.Dispatch(nil, &ast.ConnectStmt{Path: "/tmp/test.mpr"})
	if err == nil {
		t.Fatal("expected error for unregistered statement, got nil")
	}
	var unsupported *mdlerrors.UnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected UnsupportedError, got %T: %v", err, err)
	}
}

func TestRegistry_Register_Duplicate_Panics(t *testing.T) {
	r := emptyRegistry()
	handler := func(ctx *ExecContext, stmt ast.Statement) error { return nil }

	r.Register(&ast.ConnectStmt{}, handler)

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	r.Register(&ast.ConnectStmt{}, handler)
}

func TestRegistry_Dispatch_Success(t *testing.T) {
	r := emptyRegistry()
	called := false
	r.Register(&ast.ConnectStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		called = true
		if _, ok := stmt.(*ast.ConnectStmt); !ok {
			t.Fatalf("expected *ConnectStmt, got %T", stmt)
		}
		return nil
	})

	err := r.Dispatch(nil, &ast.ConnectStmt{Path: "/tmp/test.mpr"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestRegistry_Dispatch_HandlerError(t *testing.T) {
	r := emptyRegistry()
	sentinel := errors.New("test error")
	r.Register(&ast.ConnectStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return sentinel
	})

	err := r.Dispatch(nil, &ast.ConnectStmt{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
}

func TestRegistry_Validate_Empty(t *testing.T) {
	r := emptyRegistry()

	knownTypes := []ast.Statement{
		&ast.ConnectStmt{},
		&ast.DisconnectStmt{},
	}
	err := r.Validate(knownTypes)
	if err == nil {
		t.Fatal("expected validation error for empty registry")
	}
}

func TestRegistry_Validate_Complete(t *testing.T) {
	r := emptyRegistry()
	noop := func(ctx *ExecContext, stmt ast.Statement) error { return nil }
	r.Register(&ast.ConnectStmt{}, noop)
	r.Register(&ast.DisconnectStmt{}, noop)

	knownTypes := []ast.Statement{
		&ast.ConnectStmt{},
		&ast.DisconnectStmt{},
	}
	err := r.Validate(knownTypes)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRegistry_Validate_Partial(t *testing.T) {
	r := emptyRegistry()
	noop := func(ctx *ExecContext, stmt ast.Statement) error { return nil }
	r.Register(&ast.ConnectStmt{}, noop)

	knownTypes := []ast.Statement{
		&ast.ConnectStmt{},
		&ast.DisconnectStmt{},
		&ast.StatusStmt{},
	}
	err := r.Validate(knownTypes)
	if err == nil {
		t.Fatal("expected validation error for partial registry")
	}
	// Should mention 2 missing types.
	if got := err.Error(); !strings.Contains(got, "2 unregistered") {
		t.Fatalf("expected '2 unregistered' in error, got: %s", got)
	}
	// Should be a ValidationError.
	var ve *mdlerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
}

func TestRegistry_HandlerCount(t *testing.T) {
	r := emptyRegistry()
	if r.HandlerCount() != 0 {
		t.Fatalf("expected 0, got %d", r.HandlerCount())
	}
	noop := func(ctx *ExecContext, stmt ast.Statement) error { return nil }
	r.Register(&ast.ConnectStmt{}, noop)
	if r.HandlerCount() != 1 {
		t.Fatalf("expected 1, got %d", r.HandlerCount())
	}
}

// allKnownStatements returns one instance of every concrete AST statement type.
// Keep in sync with mdl/ast — if a new statement is added to the parser,
// add it here so the completeness test catches missing handler registrations.
func allKnownStatements() []ast.Statement {
	return []ast.Statement{
		&ast.AlterAgentStmt{},
		&ast.AlterAssociationStmt{},
		&ast.AlterConsumedMCPServiceStmt{},
		&ast.AlterEntityStmt{},
		&ast.AlterEnumerationStmt{},
		&ast.AlterKnowledgeBaseStmt{},
		&ast.AlterModelStmt{},
		&ast.AlterModuleJarDepStmt{},
		&ast.AlterNavigationStmt{},
		&ast.AlterODataClientStmt{},
		&ast.AlterODataServiceStmt{},
		&ast.AlterPageStmt{},
		&ast.AlterProjectSecurityStmt{},
		&ast.AlterPublishedRestServiceStmt{},
		&ast.AlterSettingsStmt{},
		&ast.AlterStylingStmt{},
		&ast.AlterUserRoleStmt{},
		&ast.AlterWorkflowStmt{},
		&ast.ConnectStmt{},
		&ast.CreateAgentStmt{},
		&ast.CreateAssociationStmt{},
		&ast.CreateBusinessEventServiceStmt{},
		&ast.CreateConfigurationStmt{},
		&ast.CreateConsumedMCPServiceStmt{},
		&ast.CreateConstantStmt{},
		&ast.CreateDatabaseConnectionStmt{},
		&ast.CreateDataTransformerStmt{},
		&ast.CreateDemoUserStmt{},
		&ast.CreateEntityStmt{},
		&ast.CreateEnumerationStmt{},
		&ast.CreateExportMappingStmt{},
		&ast.CreateExternalEntitiesStmt{},
		&ast.CreateExternalEntityStmt{},
		&ast.CreateImageCollectionStmt{},
		&ast.CreateImportMappingStmt{},
		&ast.CreateJavaActionStmt{},
		&ast.CreateJavaScriptActionStmt{},
		&ast.CreateJsonStructureStmt{},
		&ast.CreateKnowledgeBaseStmt{},
		&ast.CreateMicroflowStmt{},
		&ast.CreateNanoflowStmt{},
		&ast.CreateModelStmt{},
		&ast.CreateModuleRoleStmt{},
		&ast.CreateModuleStmt{},
		&ast.CreateODataClientStmt{},
		&ast.CreateODataServiceStmt{},
		&ast.CreatePageStmtV3{},
		&ast.CreatePublishedRestServiceStmt{},
		&ast.CreateRestClientStmt{},
		&ast.CreateSnippetStmtV3{},
		&ast.CreateUserRoleStmt{},
		&ast.CreateViewEntityStmt{},
		&ast.CreateWorkflowStmt{},
		&ast.DefineFragmentStmt{},
		&ast.DescribeCatalogTableStmt{},
		&ast.DescribeContractFromOpenAPIStmt{},
		&ast.DescribeFragmentFromStmt{},
		&ast.DescribeStmt{},
		&ast.DescribeStylingStmt{},
		&ast.DisconnectStmt{},
		&ast.DropAgentStmt{},
		&ast.DropAssociationStmt{},
		&ast.DropBusinessEventServiceStmt{},
		&ast.DropConfigurationStmt{},
		&ast.DropConsumedMCPServiceStmt{},
		&ast.DropConstantStmt{},
		&ast.DropDataTransformerStmt{},
		&ast.DropDemoUserStmt{},
		&ast.DropEntityStmt{},
		&ast.DropEnumerationStmt{},
		&ast.DropExportMappingStmt{},
		&ast.DropFolderStmt{},
		&ast.DropImageCollectionStmt{},
		&ast.DropImportMappingStmt{},
		&ast.DropJavaActionStmt{},
		&ast.DropJavaScriptActionStmt{},
		&ast.DropJsonStructureStmt{},
		&ast.DropKnowledgeBaseStmt{},
		&ast.DropMicroflowStmt{},
		&ast.DropNanoflowStmt{},
		&ast.DropModelStmt{},
		&ast.DropModuleRoleStmt{},
		&ast.DropModuleStmt{},
		&ast.DropODataClientStmt{},
		&ast.DropODataServiceStmt{},
		&ast.DropPageStmt{},
		&ast.DropPublishedRestServiceStmt{},
		&ast.DropRestClientStmt{},
		&ast.DropSnippetStmt{},
		&ast.DropUserRoleStmt{},
		&ast.DropWorkflowStmt{},
		&ast.ExecuteScriptStmt{},
		&ast.ExitStmt{},
		&ast.GrantEntityAccessStmt{},
		&ast.GrantMicroflowAccessStmt{},
		&ast.GrantNanoflowAccessStmt{},
		&ast.GrantODataServiceAccessStmt{},
		&ast.GrantPageAccessStmt{},
		&ast.GrantPublishedRestServiceAccessStmt{},
		&ast.GrantWorkflowAccessStmt{},
		&ast.HelpStmt{},
		&ast.ImportStmt{},
		&ast.LintStmt{},
		&ast.MoveFolderStmt{},
		&ast.MoveStmt{},
		&ast.RefreshCatalogStmt{},
		&ast.RefreshStmt{},
		&ast.RenameStmt{},
		&ast.RevokeEntityAccessStmt{},
		&ast.RevokeMicroflowAccessStmt{},
		&ast.RevokeNanoflowAccessStmt{},
		&ast.RevokeODataServiceAccessStmt{},
		&ast.RevokePageAccessStmt{},
		&ast.RevokePublishedRestServiceAccessStmt{},
		&ast.RevokeWorkflowAccessStmt{},
		&ast.SearchStmt{},
		&ast.SelectStmt{},
		&ast.SetStmt{},
		&ast.ShowDesignPropertiesStmt{},
		&ast.ShowFeaturesStmt{},
		&ast.ShowStmt{},
		&ast.ShowWidgetsStmt{},
		&ast.SQLConnectionsStmt{},
		&ast.SQLConnectStmt{},
		&ast.SQLDescribeTableStmt{},
		&ast.SQLDisconnectStmt{},
		&ast.SQLGenerateConnectorStmt{},
		&ast.SQLQueryStmt{},
		&ast.SQLShowFunctionsStmt{},
		&ast.SQLShowTablesStmt{},
		&ast.SQLShowViewsStmt{},
		&ast.StatusStmt{},
		&ast.UpdateSecurityStmt{},
		&ast.UpdateStmt{},
		&ast.UpdateWidgetsStmt{},
	}
}

// TestNewRegistry_Completeness verifies that NewRegistry() registers a handler
// for every known AST statement type. This test fails when a new statement is
// added to the parser without a corresponding handler registration.
func TestNewRegistry_Completeness(t *testing.T) {
	r := NewRegistry()
	err := r.Validate(allKnownStatements())
	if err != nil {
		t.Fatalf("registry is incomplete: %v", err)
	}
}

// TestNewRegistry_HandlerCountSnapshot verifies that the number of registered
// handlers matches allKnownStatements(). Keep allKnownStatements() in sync with
// known statement types and handler registrations.
func TestNewRegistry_HandlerCountSnapshot(t *testing.T) {
	r := NewRegistry()
	known := allKnownStatements()

	if got := r.HandlerCount(); got != len(known) {
		t.Errorf("handler count mismatch: registry has %d, allKnownStatements has %d — update allKnownStatements or register missing handlers", got, len(known))
	}
}
