// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// ---------------------------------------------------------------------------
// PageMutationBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) OpenPageForMutation(unitID model.ID) (backend.PageMutator, error) {
	if m.OpenPageForMutationFunc != nil {
		return m.OpenPageForMutationFunc(unitID)
	}
	return nil, fmt.Errorf("MockBackend.OpenPageForMutation not configured")
}

// ---------------------------------------------------------------------------
// WorkflowMutationBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) OpenWorkflowForMutation(unitID model.ID) (backend.WorkflowMutator, error) {
	if m.OpenWorkflowForMutationFunc != nil {
		return m.OpenWorkflowForMutationFunc(unitID)
	}
	return nil, fmt.Errorf("MockBackend.OpenWorkflowForMutation not configured")
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

// ---------------------------------------------------------------------------
// WidgetBuilderBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) LoadWidgetTemplate(widgetID string, projectPath string) (backend.WidgetObjectBuilder, error) {
	if m.LoadWidgetTemplateFunc != nil {
		return m.LoadWidgetTemplateFunc(widgetID, projectPath)
	}
	return nil, nil
}

func (m *MockBackend) SerializeWidgetToOpaque(w pages.Widget) any {
	if m.SerializeWidgetToOpaqueFunc != nil {
		return m.SerializeWidgetToOpaqueFunc(w)
	}
	return nil
}

func (m *MockBackend) SerializeDataSourceToOpaque(ds pages.DataSource) any {
	if m.SerializeDataSourceToOpaqueFunc != nil {
		return m.SerializeDataSourceToOpaqueFunc(ds)
	}
	return nil
}

func (m *MockBackend) BuildCreateAttributeObject(attributePath string, objectTypeID, propertyTypeID, valueTypeID string) (any, error) {
	if m.BuildCreateAttributeObjectFunc != nil {
		return m.BuildCreateAttributeObjectFunc(attributePath, objectTypeID, propertyTypeID, valueTypeID)
	}
	return nil, nil
}

func (m *MockBackend) BuildFilterWidget(spec backend.FilterWidgetSpec, projectPath string) (pages.Widget, error) {
	if m.BuildFilterWidgetFunc != nil {
		return m.BuildFilterWidgetFunc(spec, projectPath)
	}
	return nil, fmt.Errorf("MockBackend.BuildFilterWidget not configured")
}
