// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
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
