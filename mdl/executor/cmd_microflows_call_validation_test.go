// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// moduleID used across sub-tests.
const callValidationModuleID = model.ID("test-module-id")

// backendWithMicroflow returns a MockBackend that reports exactly one microflow
// (MyModule.ExistingMF) as present. GetRawUnitByName uses the fast path; the
// slow path is exercised when the fast path returns nil.
func backendWithMicroflow() *mock.MockBackend {
	return &mock.MockBackend{
		GetRawUnitByNameFunc: func(objectType, qualifiedName string) (*types.RawUnitInfo, error) {
			if objectType == "microflow" && qualifiedName == "MyModule.ExistingMF" {
				return &types.RawUnitInfo{ID: "existing-mf-id"}, nil
			}
			return nil, nil
		},
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "MyModule" {
				return &model.Module{BaseElement: model.BaseElement{ID: callValidationModuleID}, Name: name}, nil
			}
			return nil, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{
				{
					BaseElement: model.BaseElement{ID: "existing-mf-id"},
					ContainerID: callValidationModuleID,
					Name:        "ExistingMF",
				},
			}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) {
			return nil, nil
		},
	}
}

// backendWithNanoflow returns a MockBackend that reports exactly one nanoflow.
func backendWithNanoflow() *mock.MockBackend {
	return &mock.MockBackend{
		GetRawUnitByNameFunc: func(objectType, qualifiedName string) (*types.RawUnitInfo, error) {
			if objectType == "nanoflow" && qualifiedName == "MyModule.ExistingNF" {
				return &types.RawUnitInfo{ID: "existing-nf-id"}, nil
			}
			return nil, nil
		},
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "MyModule" {
				return &model.Module{BaseElement: model.BaseElement{ID: callValidationModuleID}, Name: name}, nil
			}
			return nil, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) {
			return []*microflows.Nanoflow{
				{
					BaseElement: model.BaseElement{ID: "existing-nf-id"},
					ContainerID: callValidationModuleID,
					Name:        "ExistingNF",
				},
			}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return nil, nil
		},
	}
}

// TestCallMicroflow_NonExistent_ReportsError checks that calling a microflow
// that doesn't exist in the project produces a flowBuilder validation error.
func TestCallMicroflow_NonExistent_ReportsError(t *testing.T) {
	fb := &flowBuilder{backend: backendWithMicroflow()}

	fb.addCallMicroflowAction(&ast.CallMicroflowStmt{
		MicroflowName: ast.QualifiedName{Module: "MyModule", Name: "DoesNotExist"},
	})

	errs := fb.GetErrors()
	if len(errs) == 0 {
		t.Fatal("expected a validation error for non-existent microflow, got none")
	}
	if !strings.Contains(errs[0], "DoesNotExist") {
		t.Fatalf("error should mention the missing microflow name; got: %s", errs[0])
	}
}

// TestCallMicroflow_Existing_NoError checks that calling an existing microflow
// does not add any validation errors.
func TestCallMicroflow_Existing_NoError(t *testing.T) {
	fb := &flowBuilder{backend: backendWithMicroflow()}

	fb.addCallMicroflowAction(&ast.CallMicroflowStmt{
		MicroflowName: ast.QualifiedName{Module: "MyModule", Name: "ExistingMF"},
	})

	if errs := fb.GetErrors(); len(errs) != 0 {
		t.Fatalf("expected no errors for existing microflow; got: %v", errs)
	}
}

// TestCallNanoflow_NonExistent_ReportsError checks that calling a nanoflow that
// doesn't exist in the project produces a flowBuilder validation error.
func TestCallNanoflow_NonExistent_ReportsError(t *testing.T) {
	fb := &flowBuilder{backend: backendWithNanoflow()}

	fb.addCallNanoflowAction(&ast.CallNanoflowStmt{
		NanoflowName: ast.QualifiedName{Module: "MyModule", Name: "DoesNotExist"},
	})

	errs := fb.GetErrors()
	if len(errs) == 0 {
		t.Fatal("expected a validation error for non-existent nanoflow, got none")
	}
	if !strings.Contains(errs[0], "DoesNotExist") {
		t.Fatalf("error should mention the missing nanoflow name; got: %s", errs[0])
	}
}

// TestCallNanoflow_Existing_NoError checks that calling an existing nanoflow
// does not add any validation errors.
func TestCallNanoflow_Existing_NoError(t *testing.T) {
	fb := &flowBuilder{backend: backendWithNanoflow()}

	fb.addCallNanoflowAction(&ast.CallNanoflowStmt{
		NanoflowName: ast.QualifiedName{Module: "MyModule", Name: "ExistingNF"},
	})

	if errs := fb.GetErrors(); len(errs) != 0 {
		t.Fatalf("expected no errors for existing nanoflow; got: %v", errs)
	}
}

// TestCallMicroflow_NoBackend_NoError verifies that the check is skipped when
// there is no backend (syntax-check / offline mode).
func TestCallMicroflow_NoBackend_NoError(t *testing.T) {
	fb := &flowBuilder{} // no backend

	fb.addCallMicroflowAction(&ast.CallMicroflowStmt{
		MicroflowName: ast.QualifiedName{Module: "MyModule", Name: "Anything"},
	})

	if errs := fb.GetErrors(); len(errs) != 0 {
		t.Fatalf("expected no errors without a backend; got: %v", errs)
	}
}
