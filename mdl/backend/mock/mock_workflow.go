// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ---------------------------------------------------------------------------
// WorkflowBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListWorkflows() ([]*workflows.Workflow, error) {
	if m.ListWorkflowsFunc != nil {
		return m.ListWorkflowsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetWorkflow(id model.ID) (*workflows.Workflow, error) {
	if m.GetWorkflowFunc != nil {
		return m.GetWorkflowFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) CreateWorkflow(wf *workflows.Workflow) error {
	if m.CreateWorkflowFunc != nil {
		return m.CreateWorkflowFunc(wf)
	}
	return nil
}

func (m *MockBackend) DeleteWorkflow(id model.ID) error {
	if m.DeleteWorkflowFunc != nil {
		return m.DeleteWorkflowFunc(id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// SettingsBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) GetProjectSettings() (*model.ProjectSettings, error) {
	if m.GetProjectSettingsFunc != nil {
		return m.GetProjectSettingsFunc()
	}
	return nil, nil
}

func (m *MockBackend) UpdateProjectSettings(ps *model.ProjectSettings) error {
	if m.UpdateProjectSettingsFunc != nil {
		return m.UpdateProjectSettingsFunc(ps)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ImageBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListImageCollections() ([]*types.ImageCollection, error) {
	if m.ListImageCollectionsFunc != nil {
		return m.ListImageCollectionsFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateImageCollection(ic *types.ImageCollection) error {
	if m.CreateImageCollectionFunc != nil {
		return m.CreateImageCollectionFunc(ic)
	}
	return nil
}

func (m *MockBackend) DeleteImageCollection(id string) error {
	if m.DeleteImageCollectionFunc != nil {
		return m.DeleteImageCollectionFunc(id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ScheduledEventBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListScheduledEvents() ([]*model.ScheduledEvent, error) {
	if m.ListScheduledEventsFunc != nil {
		return m.ListScheduledEventsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetScheduledEvent(id model.ID) (*model.ScheduledEvent, error) {
	if m.GetScheduledEventFunc != nil {
		return m.GetScheduledEventFunc(id)
	}
	return nil, nil
}
