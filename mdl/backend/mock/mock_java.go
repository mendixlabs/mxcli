// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

func (m *MockBackend) ListJavaActions() ([]*types.JavaAction, error) {
	if m.ListJavaActionsFunc != nil {
		return m.ListJavaActionsFunc()
	}
	return nil, nil
}

func (m *MockBackend) ListJavaActionsFull() ([]*javaactions.JavaAction, error) {
	if m.ListJavaActionsFullFunc != nil {
		return m.ListJavaActionsFullFunc()
	}
	return nil, nil
}

func (m *MockBackend) ListJavaScriptActions() ([]*types.JavaScriptAction, error) {
	if m.ListJavaScriptActionsFunc != nil {
		return m.ListJavaScriptActionsFunc()
	}
	return nil, nil
}

func (m *MockBackend) ReadJavaActionByName(qualifiedName string) (*javaactions.JavaAction, error) {
	if m.ReadJavaActionByNameFunc != nil {
		return m.ReadJavaActionByNameFunc(qualifiedName)
	}
	return nil, nil
}

func (m *MockBackend) ReadJavaScriptActionByName(qualifiedName string) (*types.JavaScriptAction, error) {
	if m.ReadJavaScriptActionByNameFunc != nil {
		return m.ReadJavaScriptActionByNameFunc(qualifiedName)
	}
	return nil, nil
}

func (m *MockBackend) CreateJavaAction(ja *javaactions.JavaAction) error {
	if m.CreateJavaActionFunc != nil {
		return m.CreateJavaActionFunc(ja)
	}
	return nil
}

func (m *MockBackend) UpdateJavaAction(ja *javaactions.JavaAction) error {
	if m.UpdateJavaActionFunc != nil {
		return m.UpdateJavaActionFunc(ja)
	}
	return nil
}

func (m *MockBackend) DeleteJavaAction(id model.ID) error {
	if m.DeleteJavaActionFunc != nil {
		return m.DeleteJavaActionFunc(id)
	}
	return nil
}

func (m *MockBackend) WriteJavaSourceFile(moduleName, actionName string, javaCode string, params []*javaactions.JavaActionParameter, returnType javaactions.CodeActionReturnType) error {
	if m.WriteJavaSourceFileFunc != nil {
		return m.WriteJavaSourceFileFunc(moduleName, actionName, javaCode, params, returnType)
	}
	return nil
}

func (m *MockBackend) ReadJavaSourceFile(moduleName, actionName string) (string, error) {
	if m.ReadJavaSourceFileFunc != nil {
		return m.ReadJavaSourceFileFunc(moduleName, actionName)
	}
	return "", nil
}
