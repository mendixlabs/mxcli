// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ---------------------------------------------------------------------------
// PageMutationBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) OpenPageForMutation(unitID model.ID) (backend.PageMutator, error) {
	if m.OpenPageForMutationFunc != nil {
		return m.OpenPageForMutationFunc(unitID)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// WorkflowMutationBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) OpenWorkflowForMutation(unitID model.ID) (backend.WorkflowMutator, error) {
	if m.OpenWorkflowForMutationFunc != nil {
		return m.OpenWorkflowForMutationFunc(unitID)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// WidgetSerializationBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) SerializeWidget(w pages.Widget) (any, error) {
	if m.SerializeWidgetFunc != nil {
		return m.SerializeWidgetFunc(w)
	}
	return nil, nil
}

func (m *MockBackend) SerializeClientAction(a pages.ClientAction) (any, error) {
	if m.SerializeClientActionFunc != nil {
		return m.SerializeClientActionFunc(a)
	}
	return nil, nil
}

func (m *MockBackend) SerializeDataSource(ds pages.DataSource) (any, error) {
	if m.SerializeDataSourceFunc != nil {
		return m.SerializeDataSourceFunc(ds)
	}
	return nil, nil
}

func (m *MockBackend) SerializeWorkflowActivity(a workflows.WorkflowActivity) (any, error) {
	if m.SerializeWorkflowActivityFunc != nil {
		return m.SerializeWorkflowActivityFunc(a)
	}
	return nil, nil
}
