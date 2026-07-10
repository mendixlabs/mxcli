// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// ---------------------------------------------------------------------------
// ModuleBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListModules() ([]*model.Module, error) {
	if m.ListModulesFunc != nil {
		return m.ListModulesFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetModule(id model.ID) (*model.Module, error) {
	if m.GetModuleFunc != nil {
		return m.GetModuleFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) GetModuleByName(name string) (*model.Module, error) {
	if m.GetModuleByNameFunc != nil {
		return m.GetModuleByNameFunc(name)
	}
	return nil, nil
}

func (m *MockBackend) CreateModule(module *model.Module) error {
	if m.CreateModuleFunc != nil {
		return m.CreateModuleFunc(module)
	}
	return nil
}

func (m *MockBackend) UpdateModule(module *model.Module) error {
	if m.UpdateModuleFunc != nil {
		return m.UpdateModuleFunc(module)
	}
	return nil
}

func (m *MockBackend) DeleteModule(id model.ID) error {
	if m.DeleteModuleFunc != nil {
		return m.DeleteModuleFunc(id)
	}
	return nil
}

func (m *MockBackend) DeleteModuleWithCleanup(id model.ID, moduleName string) error {
	if m.DeleteModuleWithCleanupFunc != nil {
		return m.DeleteModuleWithCleanupFunc(id, moduleName)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ModuleSettingsBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListModuleSettings() ([]*types.ModuleSettings, error) {
	if m.ListModuleSettingsFunc != nil {
		return m.ListModuleSettingsFunc()
	}
	return nil, fmt.Errorf("MockBackend.ListModuleSettings not configured")
}

func (m *MockBackend) GetModuleSettings(moduleID model.ID) (*types.ModuleSettings, error) {
	if m.GetModuleSettingsFunc != nil {
		return m.GetModuleSettingsFunc(moduleID)
	}
	return nil, fmt.Errorf("MockBackend.GetModuleSettings not configured")
}

func (m *MockBackend) UpdateModuleSettings(ms *types.ModuleSettings) error {
	if m.UpdateModuleSettingsFunc != nil {
		return m.UpdateModuleSettingsFunc(ms)
	}
	return fmt.Errorf("MockBackend.UpdateModuleSettings not configured")
}

// ---------------------------------------------------------------------------
// FolderBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListFolders() ([]*types.FolderInfo, error) {
	if m.ListFoldersFunc != nil {
		return m.ListFoldersFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateFolder(folder *model.Folder) error {
	if m.CreateFolderFunc != nil {
		return m.CreateFolderFunc(folder)
	}
	return nil
}

func (m *MockBackend) DeleteFolder(id model.ID) error {
	if m.DeleteFolderFunc != nil {
		return m.DeleteFolderFunc(id)
	}
	return nil
}

func (m *MockBackend) MoveFolder(id model.ID, newContainerID model.ID) error {
	if m.MoveFolderFunc != nil {
		return m.MoveFolderFunc(id, newContainerID)
	}
	return nil
}
