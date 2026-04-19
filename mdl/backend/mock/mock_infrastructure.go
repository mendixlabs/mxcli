// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// ---------------------------------------------------------------------------
// RenameBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) UpdateQualifiedNameInAllUnits(oldName, newName string) (int, error) {
	if m.UpdateQualifiedNameInAllUnitsFunc != nil {
		return m.UpdateQualifiedNameInAllUnitsFunc(oldName, newName)
	}
	return 0, nil
}

func (m *MockBackend) RenameReferences(oldName, newName string, dryRun bool) ([]types.RenameHit, error) {
	if m.RenameReferencesFunc != nil {
		return m.RenameReferencesFunc(oldName, newName, dryRun)
	}
	return nil, nil
}

func (m *MockBackend) RenameDocumentByName(moduleName, oldName, newName string) error {
	if m.RenameDocumentByNameFunc != nil {
		return m.RenameDocumentByNameFunc(moduleName, oldName, newName)
	}
	return nil
}

// ---------------------------------------------------------------------------
// RawUnitBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) GetRawUnit(id model.ID) (map[string]any, error) {
	if m.GetRawUnitFunc != nil {
		return m.GetRawUnitFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) GetRawUnitBytes(id model.ID) ([]byte, error) {
	if m.GetRawUnitBytesFunc != nil {
		return m.GetRawUnitBytesFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) ListRawUnitsByType(typePrefix string) ([]*types.RawUnit, error) {
	if m.ListRawUnitsByTypeFunc != nil {
		return m.ListRawUnitsByTypeFunc(typePrefix)
	}
	return nil, nil
}

func (m *MockBackend) ListRawUnits(objectType string) ([]*types.RawUnitInfo, error) {
	if m.ListRawUnitsFunc != nil {
		return m.ListRawUnitsFunc(objectType)
	}
	return nil, nil
}

func (m *MockBackend) GetRawUnitByName(objectType, qualifiedName string) (*types.RawUnitInfo, error) {
	if m.GetRawUnitByNameFunc != nil {
		return m.GetRawUnitByNameFunc(objectType, qualifiedName)
	}
	return nil, nil
}

func (m *MockBackend) GetRawMicroflowByName(qualifiedName string) ([]byte, error) {
	if m.GetRawMicroflowByNameFunc != nil {
		return m.GetRawMicroflowByNameFunc(qualifiedName)
	}
	return nil, nil
}

func (m *MockBackend) UpdateRawUnit(unitID string, contents []byte) error {
	if m.UpdateRawUnitFunc != nil {
		return m.UpdateRawUnitFunc(unitID, contents)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MetadataBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListAllUnitIDs() ([]string, error) {
	if m.ListAllUnitIDsFunc != nil {
		return m.ListAllUnitIDsFunc()
	}
	return nil, nil
}

func (m *MockBackend) ListUnits() ([]*types.UnitInfo, error) {
	if m.ListUnitsFunc != nil {
		return m.ListUnitsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetUnitTypes() (map[string]int, error) {
	if m.GetUnitTypesFunc != nil {
		return m.GetUnitTypesFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetProjectRootID() (string, error) {
	if m.GetProjectRootIDFunc != nil {
		return m.GetProjectRootIDFunc()
	}
	return "", nil
}

func (m *MockBackend) ContentsDir() string {
	if m.ContentsDirFunc != nil {
		return m.ContentsDirFunc()
	}
	return ""
}

func (m *MockBackend) ExportJSON() ([]byte, error) {
	if m.ExportJSONFunc != nil {
		return m.ExportJSONFunc()
	}
	return nil, nil
}

func (m *MockBackend) InvalidateCache() {
	if m.InvalidateCacheFunc != nil {
		m.InvalidateCacheFunc()
	}
}

// ---------------------------------------------------------------------------
// WidgetBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) FindCustomWidgetType(widgetID string) (*types.RawCustomWidgetType, error) {
	if m.FindCustomWidgetTypeFunc != nil {
		return m.FindCustomWidgetTypeFunc(widgetID)
	}
	return nil, nil
}

func (m *MockBackend) FindAllCustomWidgetTypes(widgetID string) ([]*types.RawCustomWidgetType, error) {
	if m.FindAllCustomWidgetTypesFunc != nil {
		return m.FindAllCustomWidgetTypesFunc(widgetID)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// AgentEditorBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListAgentEditorModels() ([]*agenteditor.Model, error) {
	if m.ListAgentEditorModelsFunc != nil {
		return m.ListAgentEditorModelsFunc()
	}
	return nil, nil
}

func (m *MockBackend) ListAgentEditorKnowledgeBases() ([]*agenteditor.KnowledgeBase, error) {
	if m.ListAgentEditorKnowledgeBasesFunc != nil {
		return m.ListAgentEditorKnowledgeBasesFunc()
	}
	return nil, nil
}

func (m *MockBackend) ListAgentEditorConsumedMCPServices() ([]*agenteditor.ConsumedMCPService, error) {
	if m.ListAgentEditorConsumedMCPServicesFunc != nil {
		return m.ListAgentEditorConsumedMCPServicesFunc()
	}
	return nil, nil
}

func (m *MockBackend) ListAgentEditorAgents() ([]*agenteditor.Agent, error) {
	if m.ListAgentEditorAgentsFunc != nil {
		return m.ListAgentEditorAgentsFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateAgentEditorModel(model *agenteditor.Model) error {
	if m.CreateAgentEditorModelFunc != nil {
		return m.CreateAgentEditorModelFunc(model)
	}
	return nil
}

func (m *MockBackend) DeleteAgentEditorModel(id string) error {
	if m.DeleteAgentEditorModelFunc != nil {
		return m.DeleteAgentEditorModelFunc(id)
	}
	return nil
}

func (m *MockBackend) CreateAgentEditorKnowledgeBase(kb *agenteditor.KnowledgeBase) error {
	if m.CreateAgentEditorKnowledgeBaseFunc != nil {
		return m.CreateAgentEditorKnowledgeBaseFunc(kb)
	}
	return nil
}

func (m *MockBackend) DeleteAgentEditorKnowledgeBase(id string) error {
	if m.DeleteAgentEditorKnowledgeBaseFunc != nil {
		return m.DeleteAgentEditorKnowledgeBaseFunc(id)
	}
	return nil
}

func (m *MockBackend) CreateAgentEditorConsumedMCPService(svc *agenteditor.ConsumedMCPService) error {
	if m.CreateAgentEditorConsumedMCPServiceFunc != nil {
		return m.CreateAgentEditorConsumedMCPServiceFunc(svc)
	}
	return nil
}

func (m *MockBackend) DeleteAgentEditorConsumedMCPService(id string) error {
	if m.DeleteAgentEditorConsumedMCPServiceFunc != nil {
		return m.DeleteAgentEditorConsumedMCPServiceFunc(id)
	}
	return nil
}

func (m *MockBackend) CreateAgentEditorAgent(a *agenteditor.Agent) error {
	if m.CreateAgentEditorAgentFunc != nil {
		return m.CreateAgentEditorAgentFunc(a)
	}
	return nil
}

func (m *MockBackend) DeleteAgentEditorAgent(id string) error {
	if m.DeleteAgentEditorAgentFunc != nil {
		return m.DeleteAgentEditorAgentFunc(id)
	}
	return nil
}
