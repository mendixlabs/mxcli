// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
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

func (m *MockBackend) WriteJavaSourceFile(moduleName, actionName string, javaCode string, params []*javaactions.JavaActionParameter, returnType javaactions.CodeActionReturnType, extraImports []string, extraCode string) error {
	if m.WriteJavaSourceFileFunc != nil {
		return m.WriteJavaSourceFileFunc(moduleName, actionName, javaCode, params, returnType, extraImports, extraCode)
	}
	return nil
}

func (m *MockBackend) DeleteJavaSourceFile(moduleName, actionName string) error {
	if m.DeleteJavaSourceFileFunc != nil {
		return m.DeleteJavaSourceFileFunc(moduleName, actionName)
	}
	return nil
}

func (m *MockBackend) RenameJavaSourceFile(moduleName, oldName, newName string) error {
	if m.RenameJavaSourceFileFunc != nil {
		return m.RenameJavaSourceFileFunc(moduleName, oldName, newName)
	}
	return nil
}

func (m *MockBackend) ReadJavaSourceFile(moduleName, actionName string) (string, error) {
	if m.ReadJavaSourceFileFunc != nil {
		return m.ReadJavaSourceFileFunc(moduleName, actionName)
	}
	return "", nil
}

func (m *MockBackend) CreateJavaScriptAction(jsa *types.JavaScriptAction) error {
	if m.CreateJavaScriptActionFunc != nil {
		return m.CreateJavaScriptActionFunc(jsa)
	}
	return nil
}

func (m *MockBackend) UpdateJavaScriptAction(jsa *types.JavaScriptAction) error {
	if m.UpdateJavaScriptActionFunc != nil {
		return m.UpdateJavaScriptActionFunc(jsa)
	}
	return nil
}

func (m *MockBackend) DeleteJavaScriptAction(id model.ID) error {
	if m.DeleteJavaScriptActionFunc != nil {
		return m.DeleteJavaScriptActionFunc(id)
	}
	return nil
}

func (m *MockBackend) WriteJavaScriptSourceFile(moduleName, actionName string, jsCode string, params []*types.JavaActionParameter, returnType types.CodeActionReturnType) error {
	if m.WriteJavaScriptSourceFileFunc != nil {
		return m.WriteJavaScriptSourceFileFunc(moduleName, actionName, jsCode, params, returnType)
	}
	return nil
}

func (m *MockBackend) DeleteJavaScriptSourceFile(moduleName, actionName string) error {
	if m.DeleteJavaScriptSourceFileFunc != nil {
		return m.DeleteJavaScriptSourceFileFunc(moduleName, actionName)
	}
	return nil
}

func (m *MockBackend) RenameJavaScriptSourceFile(moduleName, oldName, newName string) error {
	if m.RenameJavaScriptSourceFileFunc != nil {
		return m.RenameJavaScriptSourceFileFunc(moduleName, oldName, newName)
	}
	return nil
}
