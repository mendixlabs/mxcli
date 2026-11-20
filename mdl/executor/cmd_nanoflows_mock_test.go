// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// --- SHOW NANOFLOWS ---

func TestShowNanoflows_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Sales")
	mod2 := mkModule("HR")
	nf1 := mkNanoflow(mod1.ID, "NF_Sell")
	nf2 := mkNanoflow(mod2.ID, "NF_Hire")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, nf1.ContainerID, mod1.ID)
	withContainer(h, nf2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf1, nf2}, nil },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod1, mod2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listNanoflows(ctx, "HR"))

	out := buf.String()
	assertNotContainsStr(t, out, "Sales.NF_Sell")
	assertContainsStr(t, out, "HR.NF_Hire")
}

func TestShowNanoflows_Mock_Empty(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listNanoflows(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "(0 nanoflows)")
}

// --- DESCRIBE NANOFLOW ---

func TestDescribeNanoflow_Mock_Minimal(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_Validate")

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeNanoflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "NF_Validate"}))

	out := buf.String()
	assertContainsStr(t, out, "create or modify nanoflow MyModule.NF_Validate")
}

func TestDescribeNanoflow_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := describeNanoflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "Missing"})
	assertError(t, err)
}

func TestDescribeNanoflow_Mock_WithReturnType(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_GetName")
	nf.ReturnType = &microflows.StringType{}

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeNanoflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "NF_GetName"}))

	out := buf.String()
	assertContainsStr(t, out, "nanoflow MyModule.NF_GetName")
}

func TestDescribeNanoflow_ReturningFlowSkipsEmptyEndEvent(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_Value")
	nf.ReturnType = &microflows.StringType{}
	nf.ObjectCollection = &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.StartEvent{BaseMicroflowObject: mkObj("start")},
			&microflows.EndEvent{BaseMicroflowObject: mkObj("end")},
		},
		Flows: []*microflows.SequenceFlow{mkFlow("start", "end")},
	}

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return nil, nil },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeNanoflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "NF_Value"}))

	out := buf.String()
	assertContainsStr(t, out, "returns String")
	assertNotContainsStr(t, out, "return;")
}

// --- DROP NANOFLOW ---

func TestDropNanoflow_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_ToDelete")

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	var deletedID model.ID
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListNanoflowsFunc:  func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		DeleteNanoflowFunc: func(id model.ID) error { deletedID = id; return nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execDropNanoflow(ctx, &ast.DropNanoflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "NF_ToDelete"},
	}))

	if deletedID != nf.ID {
		t.Errorf("Expected DeleteNanoflow called with ID %s, got %s", nf.ID, deletedID)
	}
}

func TestDropNanoflow_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropNanoflow(ctx, &ast.DropNanoflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "Missing"},
	})
	assertError(t, err)
}

// --- MOVE NANOFLOW ---

func TestMoveNanoflow_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_Move")
	folderID := nextID("folder")
	folders := []*types.FolderInfo{
		{ID: folderID, ContainerID: mod.ID, Name: "SubFolder"},
	}

	var movedNF *microflows.Nanoflow
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },

		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:   func() ([]*types.FolderInfo, error) { return folders, nil },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		MoveNanoflowFunc:  func(n *microflows.Nanoflow) error { movedNF = n; return nil },
	}

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, moveNanoflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "NF_Move"}, folderID, nil, false))

	if movedNF == nil {
		t.Fatal("Expected MoveNanoflow to be called")
	}
	if movedNF.ContainerID != folderID {
		t.Errorf("Expected ContainerID %s, got %s", folderID, movedNF.ContainerID)
	}
}

// --- GRANT / REVOKE ---

func TestGrantNanoflowAccess_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_Secure")

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	roleID := nextID("role")
	modSec := &security.ModuleSecurity{
		ContainerID: mod.ID,
		ModuleRoles: []*security.ModuleRole{
			{BaseElement: model.BaseElement{ID: model.ID(roleID)}, Name: "User"},
		},
	}

	var grantedRoles []string
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListNanoflowsFunc:      func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleSecurityFunc:  func(moduleID model.ID) (*security.ModuleSecurity, error) { return modSec, nil },
		UpdateAllowedRolesFunc: func(unitID model.ID, roles []string) error { grantedRoles = roles; return nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execGrantNanoflowAccess(ctx, &ast.GrantNanoflowAccessStmt{
		Nanoflow: ast.QualifiedName{Module: "MyModule", Name: "NF_Secure"},
		Roles:    []ast.QualifiedName{{Module: "MyModule", Name: "User"}},
	}))

	if len(grantedRoles) == 0 {
		t.Error("Expected at least one allowed module role after grant")
	}
}

func TestRevokeNanoflowAccess_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	roleID := nextID("role")
	nf := mkNanoflow(mod.ID, "NF_Revoke")
	nf.AllowedModuleRoles = []model.ID{"MyModule.User"}

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	modSec := &security.ModuleSecurity{
		ContainerID: mod.ID,
		ModuleRoles: []*security.ModuleRole{
			{BaseElement: model.BaseElement{ID: model.ID(roleID)}, Name: "User"},
		},
	}

	var revokedRoles []string
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListNanoflowsFunc:      func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleSecurityFunc:  func(moduleID model.ID) (*security.ModuleSecurity, error) { return modSec, nil },
		UpdateAllowedRolesFunc: func(unitID model.ID, roles []string) error { revokedRoles = roles; return nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execRevokeNanoflowAccess(ctx, &ast.RevokeNanoflowAccessStmt{
		Nanoflow: ast.QualifiedName{Module: "MyModule", Name: "NF_Revoke"},
		Roles:    []ast.QualifiedName{{Module: "MyModule", Name: "User"}},
	}))

	if len(revokedRoles) != 0 {
		t.Errorf("Expected empty roles after revoke, got %v", revokedRoles)
	}
}

// --- SHOW ACCESS ON NANOFLOW ---

func TestShowAccessOnNanoflow_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_Access")
	nf.AllowedModuleRoles = []model.ID{"MyModule.Admin", "MyModule.User"}

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listAccessOnNanoflow(ctx, &ast.QualifiedName{Module: "MyModule", Name: "NF_Access"}))

	out := buf.String()
	assertContainsStr(t, out, "Admin")
	assertContainsStr(t, out, "User")
}

func TestShowAccessOnNanoflow_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := listAccessOnNanoflow(ctx, &ast.QualifiedName{Module: "MyModule", Name: "Missing"})
	assertError(t, err)
}

// --- NANOFLOW VALIDATION ---

func TestValidateNanoflowBody_DisallowedActions(t *testing.T) {
	// EXHAUSTIVE: every type in checkDisallowedNanoflowAction must appear here.
	// If a new server-side action is added to the AST but not to the denylist,
	// add it here so the test fails visibly.
	tests := []struct {
		name string
		stmt ast.MicroflowStatement
		want string
	}{
		{"RaiseError", &ast.RaiseErrorStmt{}, "ErrorEvent"},
		{"JavaAction", &ast.CallJavaActionStmt{}, "Java"},
		{"DatabaseQuery", &ast.ExecuteDatabaseQueryStmt{}, "database"},
		{"CallExternalAction", &ast.CallExternalActionStmt{}, "external action"},
		{"ShowHomePage", &ast.ShowHomePageStmt{}, "SHOW HOME PAGE"},
		{"RestCall", &ast.RestCallStmt{}, "REST"},
		{"SendRestRequest", &ast.SendRestRequestStmt{}, "REST"},
		{"SendEmail", &ast.SendEmailStmt{}, "SEND EMAIL"},
		{"ImportFromMapping", &ast.ImportFromMappingStmt{}, "import mapping"},
		{"ExportToMapping", &ast.ExportToMappingStmt{}, "export mapping"},
		{"TransformJson", &ast.TransformJsonStmt{}, "JSON transformation"},
		{"CallWorkflow", &ast.CallWorkflowStmt{}, "workflow"},
		{"GetWorkflowData", &ast.GetWorkflowDataStmt{}, "workflow"},
		{"GetWorkflows", &ast.GetWorkflowsStmt{}, "workflow"},
		{"GetWorkflowActivityRecords", &ast.GetWorkflowActivityRecordsStmt{}, "workflow"},
		{"WorkflowOperation", &ast.WorkflowOperationStmt{}, "workflow"},
		{"SetTaskOutcome", &ast.SetTaskOutcomeStmt{}, "workflow"},
		{"OpenUserTask", &ast.OpenUserTaskStmt{}, "workflow"},
		{"NotifyWorkflow", &ast.NotifyWorkflowStmt{}, "workflow"},
		{"OpenWorkflow", &ast.OpenWorkflowStmt{}, "workflow"},
		{"LockWorkflow", &ast.LockWorkflowStmt{}, "workflow"},
		{"UnlockWorkflow", &ast.UnlockWorkflowStmt{}, "workflow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateNanoflowBody([]ast.MicroflowStatement{tt.stmt})
			if len(errors) == 0 {
				t.Fatalf("Expected validation error for %s", tt.name)
			}
			assertContainsStr(t, errors[0], tt.want)
		})
	}
}

func TestValidateNanoflowBody_AllowedActions(t *testing.T) {
	tests := []struct {
		name string
		stmt ast.MicroflowStatement
	}{
		{"CreateObject", &ast.CreateObjectStmt{}},
		{"ChangeObject", &ast.ChangeObjectStmt{}},
		{"Retrieve", &ast.RetrieveStmt{}},
		{"ShowPage", &ast.ShowPageStmt{}},
		{"CallMicroflow", &ast.CallMicroflowStmt{}},
		{"CallNanoflow", &ast.CallNanoflowStmt{}},
		{"CallJavaScriptAction", &ast.CallJavaScriptActionStmt{}},
		{"CreateVariable", &ast.DeclareStmt{}},
		{"ChangeVariable", &ast.MfSetStmt{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateNanoflowBody([]ast.MicroflowStatement{tt.stmt})
			if len(errors) != 0 {
				t.Errorf("Expected no validation errors for %s, got: %v", tt.name, errors)
			}
		})
	}
}

func TestValidateNanoflowReturnType_Binary(t *testing.T) {
	msg := validateNanoflowReturnType(&ast.MicroflowReturnType{
		Type: ast.DataType{Kind: ast.TypeBinary},
	})
	if msg == "" {
		t.Error("Expected validation error for Binary return type")
	}
	assertContainsStr(t, msg, "Binary")
}

func TestValidateNanoflowReturnType_AllowedTypes(t *testing.T) {
	allowedKinds := []ast.DataTypeKind{
		ast.TypeString,
		ast.TypeInteger,
		ast.TypeBoolean,
		ast.TypeDateTime,
		ast.TypeDecimal,
		ast.TypeVoid,
	}
	for _, kind := range allowedKinds {
		msg := validateNanoflowReturnType(&ast.MicroflowReturnType{
			Type: ast.DataType{Kind: kind},
		})
		if msg != "" {
			t.Errorf("Expected no error for return type %v, got: %s", kind, msg)
		}
	}
}

func TestValidateNanoflowBody_NestedDisallowed(t *testing.T) {
	// Disallowed action nested inside IF body
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			ThenBody: []ast.MicroflowStatement{
				&ast.CallJavaActionStmt{},
			},
		},
	}
	errors := validateNanoflowBody(body)
	if len(errors) == 0 {
		t.Error("Expected validation error for Java action nested in IF body")
	}
}

func TestValidateNanoflow_Combined(t *testing.T) {
	// Both body and return type errors
	body := []ast.MicroflowStatement{&ast.RaiseErrorStmt{}}
	retType := &ast.MicroflowReturnType{
		Type: ast.DataType{Kind: ast.TypeBinary},
	}

	msg := validateNanoflow("TestNF", body, retType)
	assertContainsStr(t, msg, "ErrorEvent")
	assertContainsStr(t, msg, "Binary")
	assertContainsStr(t, msg, "validation errors")
}

// --- NOT-CONNECTED GUARDS ---

func TestCreateNanoflow_Mock_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execCreateNanoflow(ctx, &ast.CreateNanoflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "NF_Fail"},
	})
	assertError(t, err)
}

func TestDropNanoflow_Mock_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execDropNanoflow(ctx, &ast.DropNanoflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "NF_Fail"},
	})
	assertError(t, err)
}

func TestGrantNanoflowAccess_Mock_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execGrantNanoflowAccess(ctx, &ast.GrantNanoflowAccessStmt{
		Nanoflow: ast.QualifiedName{Module: "MyModule", Name: "NF_Fail"},
		Roles:    []ast.QualifiedName{{Module: "MyModule", Name: "User"}},
	})
	assertError(t, err)
}

func TestRevokeNanoflowAccess_Mock_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execRevokeNanoflowAccess(ctx, &ast.RevokeNanoflowAccessStmt{
		Nanoflow: ast.QualifiedName{Module: "MyModule", Name: "NF_Fail"},
		Roles:    []ast.QualifiedName{{Module: "MyModule", Name: "User"}},
	})
	assertError(t, err)
}

// --- GRANT non-existent nanoflow ---

func TestGrantNanoflowAccess_Mock_NanoflowNotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execGrantNanoflowAccess(ctx, &ast.GrantNanoflowAccessStmt{
		Nanoflow: ast.QualifiedName{Module: "MyModule", Name: "Missing"},
		Roles:    []ast.QualifiedName{{Module: "MyModule", Name: "User"}},
	})
	assertError(t, err)
}

// --- REVOKE non-existent nanoflow ---

func TestRevokeNanoflowAccess_Mock_NanoflowNotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execRevokeNanoflowAccess(ctx, &ast.RevokeNanoflowAccessStmt{
		Nanoflow: ast.QualifiedName{Module: "MyModule", Name: "Missing"},
		Roles:    []ast.QualifiedName{{Module: "MyModule", Name: "User"}},
	})
	assertError(t, err)
}

// --- SHOW ACCESS nil name ---

func TestShowAccessOnNanoflow_Mock_NilName(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	err := listAccessOnNanoflow(ctx, nil)
	assertError(t, err)
}

// --- SHOW ACCESS empty roles ---

func TestShowAccessOnNanoflow_Mock_EmptyRoles(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_NoRoles")
	nf.AllowedModuleRoles = nil

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listAccessOnNanoflow(ctx, &ast.QualifiedName{Module: "MyModule", Name: "NF_NoRoles"}))

	// Should output something (empty table or "no roles") but not error
	_ = buf.String()
}

// --- MOVE non-existent ---

func TestMoveNanoflow_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := moveNanoflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "Missing"}, "some-folder-id", nil, false)
	assertError(t, err)
}

// --- GRANT idempotent ---

func TestGrantNanoflowAccess_Mock_Idempotent(t *testing.T) {
	mod := mkModule("MyModule")
	roleID := nextID("role")
	nf := mkNanoflow(mod.ID, "NF_GrantIdem")
	nf.AllowedModuleRoles = []model.ID{"MyModule.User"}

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	modSec := &security.ModuleSecurity{
		ContainerID: mod.ID,
		ModuleRoles: []*security.ModuleRole{
			{BaseElement: model.BaseElement{ID: model.ID(roleID)}, Name: "User"},
		},
	}

	var grantedRoles []string
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListNanoflowsFunc:      func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleSecurityFunc:  func(moduleID model.ID) (*security.ModuleSecurity, error) { return modSec, nil },
		UpdateAllowedRolesFunc: func(unitID model.ID, roles []string) error { grantedRoles = roles; return nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execGrantNanoflowAccess(ctx, &ast.GrantNanoflowAccessStmt{
		Nanoflow: ast.QualifiedName{Module: "MyModule", Name: "NF_GrantIdem"},
		Roles:    []ast.QualifiedName{{Module: "MyModule", Name: "User"}},
	}))

	// Should mention "already have access" or similar
	out := buf.String()
	_ = out
	// Role should still be present (not duplicated)
	if len(grantedRoles) > 1 {
		t.Errorf("Expected at most 1 role after idempotent grant, got %d: %v", len(grantedRoles), grantedRoles)
	}
}

// --- REVOKE role not present ---

func TestRevokeNanoflowAccess_Mock_RoleNotPresent(t *testing.T) {
	mod := mkModule("MyModule")
	roleID := nextID("role")
	nf := mkNanoflow(mod.ID, "NF_RevNoRole")
	nf.AllowedModuleRoles = nil // no roles

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	modSec := &security.ModuleSecurity{
		ContainerID: mod.ID,
		ModuleRoles: []*security.ModuleRole{
			{BaseElement: model.BaseElement{ID: model.ID(roleID)}, Name: "User"},
		},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListNanoflowsFunc:      func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleSecurityFunc:  func(moduleID model.ID) (*security.ModuleSecurity, error) { return modSec, nil },
		UpdateAllowedRolesFunc: func(unitID model.ID, roles []string) error { return nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execRevokeNanoflowAccess(ctx, &ast.RevokeNanoflowAccessStmt{
		Nanoflow: ast.QualifiedName{Module: "MyModule", Name: "NF_RevNoRole"},
		Roles:    []ast.QualifiedName{{Module: "MyModule", Name: "User"}},
	}))

	// Should mention "none of the specified roles" or similar
	_ = buf.String()
}

// --- DESCRIBE with activities ---

func TestDescribeNanoflow_Mock_WithActivities(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_WithActs")
	nf.ReturnType = &microflows.StringType{}
	// Add a parameter to make the describe output richer
	nf.Parameters = []*microflows.MicroflowParameter{
		{
			BaseElement: model.BaseElement{ID: nextID("param")},
			Name:        "Input",
			Type:        &microflows.StringType{},
		},
	}

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeNanoflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "NF_WithActs"}))

	out := buf.String()
	assertContainsStr(t, out, "nanoflow MyModule.NF_WithActs")
	assertContainsStr(t, out, "$Input")
}

// --- VALIDATION: all disallowed workflow actions ---

func TestValidateNanoflowBody_AllWorkflowActions(t *testing.T) {
	workflowStmts := []ast.MicroflowStatement{
		&ast.CallWorkflowStmt{},
		&ast.GetWorkflowDataStmt{},
		&ast.GetWorkflowsStmt{},
		&ast.GetWorkflowActivityRecordsStmt{},
		&ast.WorkflowOperationStmt{},
		&ast.SetTaskOutcomeStmt{},
		&ast.OpenUserTaskStmt{},
		&ast.NotifyWorkflowStmt{},
		&ast.OpenWorkflowStmt{},
		&ast.LockWorkflowStmt{},
		&ast.UnlockWorkflowStmt{},
	}

	for _, stmt := range workflowStmts {
		errors := validateNanoflowBody([]ast.MicroflowStatement{stmt})
		if len(errors) == 0 {
			t.Errorf("Expected validation error for %T", stmt)
		}
		assertContainsStr(t, errors[0], "workflow")
	}
}

// --- VALIDATION: nested in else body ---

func TestValidateNanoflowBody_NestedInElseBody(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			ElseBody: []ast.MicroflowStatement{
				&ast.RaiseErrorStmt{},
			},
		},
	}
	errors := validateNanoflowBody(body)
	if len(errors) == 0 {
		t.Error("Expected validation error for disallowed action in ELSE body")
	}
}

// --- VALIDATION: nested in while body ---

func TestValidateNanoflowBody_NestedInWhileBody(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.WhileStmt{
			Body: []ast.MicroflowStatement{
				&ast.ExecuteDatabaseQueryStmt{},
			},
		},
	}
	errors := validateNanoflowBody(body)
	if len(errors) == 0 {
		t.Error("Expected validation error for disallowed action in WHILE body")
	}
}

// --- VALIDATION: nested in loop body ---

func TestValidateNanoflowBody_NestedInLoopBody(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.LoopStmt{
			Body: []ast.MicroflowStatement{
				&ast.ShowHomePageStmt{},
			},
		},
	}
	errors := validateNanoflowBody(body)
	if len(errors) == 0 {
		t.Error("Expected validation error for disallowed action in LOOP body")
	}
}

// --- VALIDATION: nested in error handling body ---

func TestValidateNanoflowBody_NestedInErrorHandling(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.CallMicroflowStmt{
			ErrorHandling: &ast.ErrorHandlingClause{
				Body: []ast.MicroflowStatement{
					&ast.CallJavaActionStmt{},
				},
			},
		},
	}
	errors := validateNanoflowBody(body)
	if len(errors) == 0 {
		t.Error("Expected validation error for disallowed action in error handling body")
	}
}

// --- VALIDATION: multiple errors ---

func TestValidateNanoflowBody_MultipleErrors(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.RaiseErrorStmt{},
		&ast.CallJavaActionStmt{},
		&ast.RestCallStmt{},
	}
	errors := validateNanoflowBody(body)
	if len(errors) != 3 {
		t.Errorf("Expected 3 validation errors, got %d: %v", len(errors), errors)
	}
}

// --- VALIDATION: nil return type ---

func TestValidateNanoflowReturnType_Nil(t *testing.T) {
	msg := validateNanoflowReturnType(nil)
	if msg != "" {
		t.Errorf("Expected no error for nil return type, got: %s", msg)
	}
}

// --- VALIDATION: entity return type ---

func TestValidateNanoflowReturnType_Entity(t *testing.T) {
	msg := validateNanoflowReturnType(&ast.MicroflowReturnType{
		Type: ast.DataType{Kind: ast.TypeEntity},
	})
	if msg != "" {
		t.Errorf("Expected no error for entity return type, got: %s", msg)
	}
}

// --- SHOW NANOFLOWS all (no module filter) ---

func TestShowNanoflows_Mock_All(t *testing.T) {
	mod1 := mkModule("Sales")
	mod2 := mkModule("HR")
	nf1 := mkNanoflow(mod1.ID, "NF_Sell")
	nf2 := mkNanoflow(mod2.ID, "NF_Hire")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, nf1.ContainerID, mod1.ID)
	withContainer(h, nf2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf1, nf2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listNanoflows(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "NF_Sell")
	assertContainsStr(t, out, "NF_Hire")
	assertContainsStr(t, out, "(2 nanoflows)")
}

// --- OBS-2: Module not found error for SHOW NANOFLOWS ---

func TestShowNanoflows_Mock_ModuleNotFound(t *testing.T) {
	mod := mkModule("Sales")

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	err := listNanoflows(ctx, "NonExistent")
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

// --- OBS-8: Empty nanoflow name validation ---

func TestCreateNanoflow_Mock_EmptyName(t *testing.T) {
	mod := mkModule("MyModule")

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	stmt := &ast.CreateNanoflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: ""},
	}
	err := execCreateNanoflow(ctx, stmt)
	assertError(t, err)
	assertContainsStr(t, err.Error(), "must not be empty")
}

func TestDescribeNanoflow_Mock_Excluded(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_Excluded")
	nf.Excluded = true

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeNanoflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "NF_Excluded"}))

	out := buf.String()
	assertContainsStr(t, out, "@excluded")
}

// ako/mxcli#705: DESCRIBE must print a nanoflow's return variable (`as $Var`)
// the way it does a microflow's, or re-executing the output drops it. Both
// describe paths are checked — describe and the source-mapped variant behind
// diff-local — since they render the header separately.
func TestDescribeNanoflow_PrintsReturnVariable(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_Get")
	nf.ReturnType = &microflows.StringType{}
	nf.ReturnVariableName = "Feedback"

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	name := ast.QualifiedName{Module: "MyModule", Name: "NF_Get"}
	assertNoError(t, describeNanoflow(ctx, name))
	assertContainsStr(t, buf.String(), "returns String as $Feedback")

	mdl, _, err := describeNanoflowToString(ctx, name)
	assertNoError(t, err)
	assertContainsStr(t, mdl, "returns String as $Feedback")
}

// The rebuild takes the return variable from the statement, and carries
// MarkAsUsed — which it hardcoded to false, clearing Studio Pro's "Mark as used"
// on every rewrite.
func TestBuildNanoflow_ReturnVariableAndMarkAsUsed(t *testing.T) {
	mod := mkModule("MyModule")
	stored := mkNanoflow(mod.ID, "NF_Get")
	stored.MarkAsUsed = true

	h := mkHierarchy(mod)
	withContainer(h, stored.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{stored}, nil },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	stmt := firstStatement[*ast.CreateNanoflowStmt](t, `create or modify nanoflow MyModule.NF_Get ()
returns String as $Result
begin
  return 'x';
end;`)
	built, err := buildNanoflowFromStmt(ctx, stmt, buildFlowOpts{})
	assertNoError(t, err)
	if got := built.Nanoflow.ReturnVariableName; got != "Result" {
		t.Errorf("ReturnVariableName = %q, want the authored Result", got)
	}
	if !built.Nanoflow.MarkAsUsed {
		t.Error("MarkAsUsed = false — the stored true was cleared by the rewrite")
	}
}

// mendixlabs/mxcli#1345: describe nanoflow printed no grant line, so a
// describe -> exec round trip left the nanoflow with no allowed roles.
// Parse the output rather than grep it: the round trip is what was lost.
func TestDescribeNanoflow_Mock_EmitsGrantForAllowedRoles(t *testing.T) {
	mod := mkModule("Shop")
	nf := mkNanoflow(mod.ID, "NF_Checkout")
	nf.AllowedModuleRoles = []model.ID{"Shop.User", "Shop.Admin"}

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeNanoflow(ctx, ast.QualifiedName{Module: "Shop", Name: "NF_Checkout"}))

	out := buf.String()
	prog, errs := visitor.Build(out)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, out)
	}
	var grant *ast.GrantNanoflowAccessStmt
	for _, stmt := range prog.Statements {
		if g, ok := stmt.(*ast.GrantNanoflowAccessStmt); ok {
			grant = g
		}
	}
	if grant == nil {
		t.Fatalf("describe nanoflow emitted no grant execute statement:\n%s", out)
	}
	if grant.Nanoflow.String() != "Shop.NF_Checkout" {
		t.Errorf("grant targets %s, want Shop.NF_Checkout", grant.Nanoflow.String())
	}
	var roles []string
	for _, r := range grant.Roles {
		roles = append(roles, r.String())
	}
	if got := strings.Join(roles, ", "); got != "Shop.User, Shop.Admin" {
		t.Errorf("grant roles = %q, want %q", got, "Shop.User, Shop.Admin")
	}
}

// mendixlabs/mxcli#1498 (follow-up): `export level api` on a nanoflow parsed
// and passed check, and exec stored Hidden — the clause never left the parser.
func TestCreateNanoflow_ExportLevelReachesTheWriter(t *testing.T) {
	var written *microflows.Nanoflow
	modID := model.ID("mod-shop")
	mod := &model.Module{BaseElement: model.BaseElement{ID: modID}, Name: "Shop"}
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(string) (*model.Module, error) { return mod, nil },
		ListMicroflowsFunc:  func() ([]*microflows.Microflow, error) { return nil, nil },
		ListNanoflowsFunc:   func() ([]*microflows.Nanoflow, error) { return nil, nil },
		CreateNanoflowFunc:  func(nf *microflows.Nanoflow) error { written = nf; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	for _, tc := range []struct{ clause, want string }{
		{"export level api", types.ExportLevelAPI},
		{"export level hidden", types.ExportLevelHidden},
		// Control: no clause authors nothing, so the stored level is carried.
		{"", ""},
	} {
		written = nil
		stmt := firstStatement[*ast.CreateNanoflowStmt](t, "create nanoflow Shop.NF_IsOpen ()\n"+tc.clause+"\nbegin\nend;")
		if err := execCreateNanoflow(ctx, stmt); err != nil {
			t.Fatalf("%q: %v", tc.clause, err)
		}
		if written == nil || written.ExportLevel != tc.want {
			t.Errorf("%q: written ExportLevel = %+v, want %q", tc.clause, written, tc.want)
		}
	}
}

// DESCRIBE prints the clause for an API nanoflow, and only that clause: the
// microflow-only concurrency and URL lines have no nanoflow property. Both
// renderers are checked — `describe nanoflow` and the one `create or modify`
// compares against — because they are separate code and only one was fixed at
// first.
func TestDescribeNanoflow_EmitsExportLevel(t *testing.T) {
	for _, tc := range []struct{ level, want string }{
		{"API", "export level api"},
		// Control: Hidden is the default and is not printed.
		{"Hidden", ""},
	} {
		mod := mkModule("Shop")
		nf := mkNanoflow(mod.ID, "NF_IsOpen")
		nf.ExportLevel = tc.level
		h := mkHierarchy(mod)
		withContainer(h, nf.ContainerID, mod.ID)
		mb := &mock.MockBackend{
			IsConnectedFunc:      func() bool { return true },
			ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
			ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
			ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		}
		ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
		qn := ast.QualifiedName{Module: "Shop", Name: "NF_IsOpen"}
		assertNoError(t, describeNanoflow(ctx, qn))
		rendered := renderMicroflowMDL(ctx, "nanoflow", nanoflowAsMicroflow(nf), qn, nil, nil, nil)
		for label, out := range map[string]string{"describe nanoflow": buf.String(), "modify comparison": rendered} {
			if got := strings.Contains(out, "export level"); got != (tc.want != "") || (tc.want != "" && !strings.Contains(out, tc.want)) {
				t.Errorf("%s, stored %s: want %q in\n%s", label, tc.level, tc.want, out)
			}
			if strings.Contains(out, "concurrent") {
				t.Errorf("%s prints a concurrency clause a nanoflow cannot hold:\n%s", label, out)
			}
		}
	}
}
