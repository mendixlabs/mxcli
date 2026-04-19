// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

func (m *MockBackend) ListImportMappings() ([]*model.ImportMapping, error) {
	if m.ListImportMappingsFunc != nil {
		return m.ListImportMappingsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetImportMappingByQualifiedName(moduleName, name string) (*model.ImportMapping, error) {
	if m.GetImportMappingByQualifiedNameFunc != nil {
		return m.GetImportMappingByQualifiedNameFunc(moduleName, name)
	}
	return nil, nil
}

func (m *MockBackend) CreateImportMapping(im *model.ImportMapping) error {
	if m.CreateImportMappingFunc != nil {
		return m.CreateImportMappingFunc(im)
	}
	return nil
}

func (m *MockBackend) UpdateImportMapping(im *model.ImportMapping) error {
	if m.UpdateImportMappingFunc != nil {
		return m.UpdateImportMappingFunc(im)
	}
	return nil
}

func (m *MockBackend) DeleteImportMapping(id model.ID) error {
	if m.DeleteImportMappingFunc != nil {
		return m.DeleteImportMappingFunc(id)
	}
	return nil
}

func (m *MockBackend) MoveImportMapping(im *model.ImportMapping) error {
	if m.MoveImportMappingFunc != nil {
		return m.MoveImportMappingFunc(im)
	}
	return nil
}

func (m *MockBackend) ListExportMappings() ([]*model.ExportMapping, error) {
	if m.ListExportMappingsFunc != nil {
		return m.ListExportMappingsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetExportMappingByQualifiedName(moduleName, name string) (*model.ExportMapping, error) {
	if m.GetExportMappingByQualifiedNameFunc != nil {
		return m.GetExportMappingByQualifiedNameFunc(moduleName, name)
	}
	return nil, nil
}

func (m *MockBackend) CreateExportMapping(em *model.ExportMapping) error {
	if m.CreateExportMappingFunc != nil {
		return m.CreateExportMappingFunc(em)
	}
	return nil
}

func (m *MockBackend) UpdateExportMapping(em *model.ExportMapping) error {
	if m.UpdateExportMappingFunc != nil {
		return m.UpdateExportMappingFunc(em)
	}
	return nil
}

func (m *MockBackend) DeleteExportMapping(id model.ID) error {
	if m.DeleteExportMappingFunc != nil {
		return m.DeleteExportMappingFunc(id)
	}
	return nil
}

func (m *MockBackend) MoveExportMapping(em *model.ExportMapping) error {
	if m.MoveExportMappingFunc != nil {
		return m.MoveExportMappingFunc(em)
	}
	return nil
}

func (m *MockBackend) ListJsonStructures() ([]*types.JsonStructure, error) {
	if m.ListJsonStructuresFunc != nil {
		return m.ListJsonStructuresFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetJsonStructureByQualifiedName(moduleName, name string) (*types.JsonStructure, error) {
	if m.GetJsonStructureByQualifiedNameFunc != nil {
		return m.GetJsonStructureByQualifiedNameFunc(moduleName, name)
	}
	return nil, nil
}

func (m *MockBackend) CreateJsonStructure(js *types.JsonStructure) error {
	if m.CreateJsonStructureFunc != nil {
		return m.CreateJsonStructureFunc(js)
	}
	return nil
}

func (m *MockBackend) DeleteJsonStructure(id string) error {
	if m.DeleteJsonStructureFunc != nil {
		return m.DeleteJsonStructureFunc(id)
	}
	return nil
}
