// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// RenameBackend provides cross-cutting rename and reference-update operations.
type RenameBackend interface {
	UpdateQualifiedNameInAllUnits(oldName, newName string) (int, error)
	RenameReferences(oldName, newName string, dryRun bool) ([]types.RenameHit, error)
	RenameDocumentByName(moduleName, oldName, newName string) error
}

// RawUnitBackend provides low-level unit access for operations that
// manipulate raw unit contents (e.g. widget patching, alter page/workflow).
type RawUnitBackend interface {
	GetRawUnit(id model.ID) (map[string]any, error)
	GetRawUnitBytes(id model.ID) ([]byte, error)
	ListRawUnitsByType(typePrefix string) ([]*types.RawUnit, error)
	ListRawUnits(objectType string) ([]*types.RawUnitInfo, error)
	GetRawUnitByName(objectType, qualifiedName string) (*types.RawUnitInfo, error)
	GetRawMicroflowByName(qualifiedName string) ([]byte, error)
	// UpdateRawUnit replaces the contents of a unit by ID.
	// Takes string (not model.ID) to match the SDK writer layer convention.
	UpdateRawUnit(unitID string, contents []byte) error
}

// MetadataBackend provides project-level metadata and introspection.
type MetadataBackend interface {
	ListAllUnitIDs() ([]string, error)
	ListUnits() ([]*types.UnitInfo, error)
	GetUnitTypes() (map[string]int, error)
	GetProjectRootID() (string, error)
	ContentsDir() string
	ExportJSON() ([]byte, error)
	InvalidateCache()
}

// WidgetBackend provides widget introspection operations.
type WidgetBackend interface {
	FindCustomWidgetType(widgetID string) (*types.RawCustomWidgetType, error)
	FindAllCustomWidgetTypes(widgetID string) ([]*types.RawCustomWidgetType, error)
}

// AgentEditorBackend provides agent editor document operations.
// Delete methods take string IDs to match the SDK writer layer convention.
type AgentEditorBackend interface {
	ListAgentEditorModels() ([]*agenteditor.Model, error)
	ListAgentEditorKnowledgeBases() ([]*agenteditor.KnowledgeBase, error)
	ListAgentEditorConsumedMCPServices() ([]*agenteditor.ConsumedMCPService, error)
	ListAgentEditorAgents() ([]*agenteditor.Agent, error)
	CreateAgentEditorModel(m *agenteditor.Model) error
	DeleteAgentEditorModel(id string) error
	CreateAgentEditorKnowledgeBase(k *agenteditor.KnowledgeBase) error
	DeleteAgentEditorKnowledgeBase(id string) error
	CreateAgentEditorConsumedMCPService(c *agenteditor.ConsumedMCPService) error
	DeleteAgentEditorConsumedMCPService(id string) error
	CreateAgentEditorAgent(a *agenteditor.Agent) error
	DeleteAgentEditorAgent(id string) error
}

// SettingsBackend provides project settings operations.
type SettingsBackend interface {
	GetProjectSettings() (*model.ProjectSettings, error)
	UpdateProjectSettings(ps *model.ProjectSettings) error
}

// ImageBackend provides image collection operations.
type ImageBackend interface {
	ListImageCollections() ([]*types.ImageCollection, error)
	CreateImageCollection(ic *types.ImageCollection) error
	DeleteImageCollection(id string) error
}

// ScheduledEventBackend provides scheduled event operations.
type ScheduledEventBackend interface {
	ListScheduledEvents() ([]*model.ScheduledEvent, error)
	GetScheduledEvent(id model.ID) (*model.ScheduledEvent, error)
}
