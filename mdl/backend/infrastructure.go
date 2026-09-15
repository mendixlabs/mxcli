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
	// AddRawUnit inserts a new unit with the given contents.
	//
	// The raw counterpart to the typed Create* methods, for a caller copying
	// units it does not decode — the marketplace module transplant moves a
	// whole module's units between projects verbatim, which is the point: a
	// decode and re-encode would rewrite identities the runtime keys on (see
	// CLAUDE.md on GUIDs).
	//
	// Takes strings (not model.ID) to match the writer layer convention, as
	// UpdateRawUnit does.
	AddRawUnit(unitID, containerID, containmentName, unitType string, contents []byte) error

	// UpdateRawUnit replaces the contents of a unit by ID.
	// Takes string (not model.ID) to match the SDK writer layer convention.
	UpdateRawUnit(unitID string, contents []byte) error
	// UpdateRawUnitOwningTranslations is UpdateRawUnit for a write that started
	// from the stored bytes and is authoritative about the translations in them.
	//
	// The ordinary path carries the stored translations onto a write, because a
	// rebuild from MDL can express only one string per text and would otherwise
	// delete the rest. A targeted patch is the opposite case: a translation
	// missing from its output is missing on purpose, and carrying it back would
	// undo a deliberate deletion.
	UpdateRawUnitOwningTranslations(unitID string, contents []byte) error
}

// MetadataBackend provides project-level metadata and introspection.
type MetadataBackend interface {
	ListAllUnitIDs() ([]string, error)
	ListUnits() ([]*types.UnitInfo, error)
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
	UpdateAgentEditorModel(m *agenteditor.Model) error
	DeleteAgentEditorModel(id string) error
	CreateAgentEditorKnowledgeBase(k *agenteditor.KnowledgeBase) error
	UpdateAgentEditorKnowledgeBase(k *agenteditor.KnowledgeBase) error
	DeleteAgentEditorKnowledgeBase(id string) error
	CreateAgentEditorConsumedMCPService(c *agenteditor.ConsumedMCPService) error
	UpdateAgentEditorConsumedMCPService(c *agenteditor.ConsumedMCPService) error
	DeleteAgentEditorConsumedMCPService(id string) error
	CreateAgentEditorAgent(a *agenteditor.Agent) error
	UpdateAgentEditorAgent(a *agenteditor.Agent) error
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
	UpdateImageCollection(ic *types.ImageCollection) error
	DeleteImageCollection(id string) error
	// ListIconCollections reads the project's icon collections (CustomIcons$
	// CustomIconCollection) for SHOW / DESCRIBE ICON COLLECTION. Read-only.
	ListIconCollections() ([]*types.IconCollection, error)
}

// QueueBackend provides task queue (Queues$Queue) operations.
type QueueBackend interface {
	ListQueues() ([]*types.Queue, error)
	CreateQueue(q *types.Queue) error
	UpdateQueue(q *types.Queue) error
	DeleteQueue(id string) error
}

// RegularExpressionBackend provides regular expression document operations.
type RegularExpressionBackend interface {
	ListRegularExpressions() ([]*model.RegularExpression, error)
	CreateRegularExpression(re *model.RegularExpression) error
	UpdateRegularExpression(re *model.RegularExpression) error
	DeleteRegularExpression(id string) error
}

// ScheduledEventBackend provides scheduled event operations.
type ScheduledEventBackend interface {
	ListScheduledEvents() ([]*model.ScheduledEvent, error)
	GetScheduledEvent(id model.ID) (*model.ScheduledEvent, error)
	CreateScheduledEvent(ev *model.ScheduledEvent) error
	UpdateScheduledEvent(ev *model.ScheduledEvent) error
	DeleteScheduledEvent(id string) error
}
