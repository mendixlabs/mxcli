// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

var _ backend.PageMutator = (*MockPageMutator)(nil)

// MockPageMutator implements backend.PageMutator. Every interface method is
// backed by a public function field. If the field is nil the method returns
// nil error (never panics). ContainerType defaults to ContainerPage when unset;
// all other methods return zero values.
type MockPageMutator struct {
	ContainerTypeFunc              func() backend.ContainerKind
	ResolveAlterTargetFunc         func(t backend.AlterTarget) (backend.AlterTargetMatch, error)
	SetWidgetPropertyFunc          func(widgetRef string, prop string, value any) error
	SetWidgetDataSourceFunc        func(widgetRef string, ds pages.DataSource) error
	SetWidgetActionFunc            func(widgetRef string, action pages.ClientAction) error
	SetWidgetNamedActionFunc       func(widgetRef string, propertyKey string, action pages.ClientAction) error
	SetColumnPropertyFunc          func(gridRef string, columnRef string, prop string, value any) error
	SetDesignPropertyFunc          func(widgetRef string, key string, valueType string, option string) error
	RemoveDesignPropertyFunc       func(widgetRef string, key string) error
	ClearDesignPropertiesFunc      func(widgetRef string) error
	InsertWidgetFunc               func(widgetRef string, columnRef string, position backend.InsertPosition, widgets []pages.Widget) error
	DropWidgetFunc                 func(refs []backend.WidgetRef) error
	ReplaceWidgetFunc              func(widgetRef string, columnRef string, widgets []pages.Widget) error
	InsertColumnsFunc              func(gridRef, afterColumnRef string, position backend.InsertPosition, columns []*backend.DataGridColumnSpec) error
	InsertListViewTemplatesFunc    func(listViewRef string, templates []*pages.ListViewTemplate) error
	InsertTabPagesFunc             func(targetRef string, position backend.InsertPosition, tabPages []*pages.TabPage) error
	InsertLayoutGridRowsFunc       func(gridRef string, rows []*pages.LayoutGridRow) error
	InsertNavigationListItemsFunc  func(targetRef string, position backend.InsertPosition, items []*pages.NavigationListItem) error
	ReplaceNavigationListItemFunc  func(targetRef string, items []*pages.NavigationListItem) error
	DropListViewTemplateFunc       func(listViewRef, specialization string) error
	ReplaceColumnFunc              func(gridRef, columnRef string, columns []*backend.DataGridColumnSpec) error
	FindWidgetFunc                 func(name string) bool
	AddVariableFunc                func(name, dataType, defaultValue string) error
	DropVariableFunc               func(name string) error
	AddParameterFunc               func(param backend.PageParameterSpec) error
	DropParameterFunc              func(name string) error
	SetLayoutFunc                  func(newLayout string, paramMappings map[string]string) error
	BoundPlaceholdersFunc          func() []string
	SetPluggablePropertyFunc       func(widgetRef string, propKey string, op backend.PluggablePropertyOp, ctx backend.PluggablePropertyContext) error
	EnclosingEntityFunc            func(widgetRef string) string
	EnclosingEntityForChildrenFunc func(widgetRef string) string
	EnclosingDataSourceFlowFunc    func(widgetRef string, forChildren bool) (string, string)
	WidgetScopeFunc                func() map[string]model.ID
	ParamScopeFunc                 func() (map[string]model.ID, map[string]string)
	SaveFunc                       func() error
}

func (m *MockPageMutator) ContainerType() backend.ContainerKind {
	if m.ContainerTypeFunc != nil {
		return m.ContainerTypeFunc()
	}
	return backend.ContainerPage
}

func (m *MockPageMutator) SetWidgetProperty(widgetRef string, prop string, value any) error {
	if m.SetWidgetPropertyFunc != nil {
		return m.SetWidgetPropertyFunc(widgetRef, prop, value)
	}
	return nil
}

func (m *MockPageMutator) SetWidgetDataSource(widgetRef string, ds pages.DataSource) error {
	if m.SetWidgetDataSourceFunc != nil {
		return m.SetWidgetDataSourceFunc(widgetRef, ds)
	}
	return nil
}

func (m *MockPageMutator) SetWidgetAction(widgetRef string, action pages.ClientAction) error {
	if m.SetWidgetActionFunc != nil {
		return m.SetWidgetActionFunc(widgetRef, action)
	}
	return nil
}

func (m *MockPageMutator) SetWidgetNamedAction(widgetRef string, propertyKey string, action pages.ClientAction) error {
	if m.SetWidgetNamedActionFunc != nil {
		return m.SetWidgetNamedActionFunc(widgetRef, propertyKey, action)
	}
	return nil
}

func (m *MockPageMutator) SetColumnProperty(gridRef string, columnRef string, prop string, value any) error {
	if m.SetColumnPropertyFunc != nil {
		return m.SetColumnPropertyFunc(gridRef, columnRef, prop, value)
	}
	return nil
}

func (m *MockPageMutator) SetDesignProperty(widgetRef string, key string, valueType string, option string) error {
	if m.SetDesignPropertyFunc != nil {
		return m.SetDesignPropertyFunc(widgetRef, key, valueType, option)
	}
	return nil
}

func (m *MockPageMutator) RemoveDesignProperty(widgetRef string, key string) error {
	if m.RemoveDesignPropertyFunc != nil {
		return m.RemoveDesignPropertyFunc(widgetRef, key)
	}
	return nil
}

func (m *MockPageMutator) ClearDesignProperties(widgetRef string) error {
	if m.ClearDesignPropertiesFunc != nil {
		return m.ClearDesignPropertiesFunc(widgetRef)
	}
	return nil
}

func (m *MockPageMutator) InsertWidget(widgetRef string, columnRef string, position backend.InsertPosition, widgets []pages.Widget) error {
	if m.InsertWidgetFunc != nil {
		return m.InsertWidgetFunc(widgetRef, columnRef, position, widgets)
	}
	return nil
}

func (m *MockPageMutator) DropWidget(refs []backend.WidgetRef) error {
	if m.DropWidgetFunc != nil {
		return m.DropWidgetFunc(refs)
	}
	return nil
}

func (m *MockPageMutator) ReplaceWidget(widgetRef string, columnRef string, widgets []pages.Widget) error {
	if m.ReplaceWidgetFunc != nil {
		return m.ReplaceWidgetFunc(widgetRef, columnRef, widgets)
	}
	return nil
}

func (m *MockPageMutator) InsertColumns(gridRef, afterColumnRef string, position backend.InsertPosition, columns []*backend.DataGridColumnSpec) error {
	if m.InsertColumnsFunc != nil {
		return m.InsertColumnsFunc(gridRef, afterColumnRef, position, columns)
	}
	return fmt.Errorf("MockBackend.InsertColumns not configured")
}

func (m *MockPageMutator) InsertListViewTemplates(listViewRef string, templates []*pages.ListViewTemplate) error {
	if m.InsertListViewTemplatesFunc != nil {
		return m.InsertListViewTemplatesFunc(listViewRef, templates)
	}
	return fmt.Errorf("MockBackend.InsertListViewTemplates not configured")
}

func (m *MockPageMutator) InsertLayoutGridRows(gridRef string, rows []*pages.LayoutGridRow) error {
	if m.InsertLayoutGridRowsFunc != nil {
		return m.InsertLayoutGridRowsFunc(gridRef, rows)
	}
	return fmt.Errorf("MockBackend.InsertLayoutGridRows not configured")
}

func (m *MockPageMutator) InsertTabPages(targetRef string, position backend.InsertPosition, tabPages []*pages.TabPage) error {
	if m.InsertTabPagesFunc != nil {
		return m.InsertTabPagesFunc(targetRef, position, tabPages)
	}
	return fmt.Errorf("MockBackend.InsertTabPages not configured")
}

func (m *MockPageMutator) InsertNavigationListItems(targetRef string, position backend.InsertPosition, items []*pages.NavigationListItem) error {
	if m.InsertNavigationListItemsFunc != nil {
		return m.InsertNavigationListItemsFunc(targetRef, position, items)
	}
	return fmt.Errorf("MockBackend.InsertNavigationListItems not configured")
}

func (m *MockPageMutator) ReplaceNavigationListItem(targetRef string, items []*pages.NavigationListItem) error {
	if m.ReplaceNavigationListItemFunc != nil {
		return m.ReplaceNavigationListItemFunc(targetRef, items)
	}
	return fmt.Errorf("MockBackend.ReplaceNavigationListItem not configured")
}

func (m *MockPageMutator) DropListViewTemplate(listViewRef, specialization string) error {
	if m.DropListViewTemplateFunc != nil {
		return m.DropListViewTemplateFunc(listViewRef, specialization)
	}
	return fmt.Errorf("MockBackend.DropListViewTemplate not configured")
}

func (m *MockPageMutator) ReplaceColumn(gridRef, columnRef string, columns []*backend.DataGridColumnSpec) error {
	if m.ReplaceColumnFunc != nil {
		return m.ReplaceColumnFunc(gridRef, columnRef, columns)
	}
	return fmt.Errorf("MockBackend.ReplaceColumn not configured")
}

func (m *MockPageMutator) FindWidget(name string) bool {
	if m.FindWidgetFunc != nil {
		return m.FindWidgetFunc(name)
	}
	return false
}

func (m *MockPageMutator) AddVariable(name, dataType, defaultValue string) error {
	if m.AddVariableFunc != nil {
		return m.AddVariableFunc(name, dataType, defaultValue)
	}
	return nil
}

func (m *MockPageMutator) DropVariable(name string) error {
	if m.DropVariableFunc != nil {
		return m.DropVariableFunc(name)
	}
	return nil
}

func (m *MockPageMutator) AddParameter(param backend.PageParameterSpec) error {
	if m.AddParameterFunc != nil {
		return m.AddParameterFunc(param)
	}
	return nil
}

func (m *MockPageMutator) DropParameter(name string) error {
	if m.DropParameterFunc != nil {
		return m.DropParameterFunc(name)
	}
	return nil
}

func (m *MockPageMutator) SetLayout(newLayout string, paramMappings map[string]string) error {
	if m.SetLayoutFunc != nil {
		return m.SetLayoutFunc(newLayout, paramMappings)
	}
	return nil
}

func (m *MockPageMutator) SetPluggableProperty(widgetRef string, propKey string, op backend.PluggablePropertyOp, ctx backend.PluggablePropertyContext) error {
	if m.SetPluggablePropertyFunc != nil {
		return m.SetPluggablePropertyFunc(widgetRef, propKey, op, ctx)
	}
	return nil
}

func (m *MockPageMutator) EnclosingEntity(widgetRef string) string {
	if m.EnclosingEntityFunc != nil {
		return m.EnclosingEntityFunc(widgetRef)
	}
	return ""
}

func (m *MockPageMutator) EnclosingEntityForChildren(widgetRef string) string {
	if m.EnclosingEntityForChildrenFunc != nil {
		return m.EnclosingEntityForChildrenFunc(widgetRef)
	}
	if m.EnclosingEntityFunc != nil {
		return m.EnclosingEntityFunc(widgetRef)
	}
	return ""
}

func (m *MockPageMutator) EnclosingDataSourceFlow(widgetRef string, forChildren bool) (string, string) {
	if m.EnclosingDataSourceFlowFunc != nil {
		return m.EnclosingDataSourceFlowFunc(widgetRef, forChildren)
	}
	return "", ""
}

func (m *MockPageMutator) WidgetScope() map[string]model.ID {
	if m.WidgetScopeFunc != nil {
		return m.WidgetScopeFunc()
	}
	return nil
}

func (m *MockPageMutator) ParamScope() (map[string]model.ID, map[string]string) {
	if m.ParamScopeFunc != nil {
		return m.ParamScopeFunc()
	}
	return nil, nil
}

func (m *MockPageMutator) Save() error {
	if m.SaveFunc != nil {
		return m.SaveFunc()
	}
	return nil
}

func (m *MockPageMutator) BoundPlaceholders() []string {
	if m.BoundPlaceholdersFunc != nil {
		return m.BoundPlaceholdersFunc()
	}
	return nil
}

// ResolveAlterTarget resolves every target unless ResolveAlterTargetFunc says
// otherwise, so a test that does not care about resolution exercises the
// operation behind it.
func (m *MockPageMutator) ResolveAlterTarget(t backend.AlterTarget) (backend.AlterTargetMatch, error) {
	if m.ResolveAlterTargetFunc != nil {
		return m.ResolveAlterTargetFunc(t)
	}
	return backend.AlterTargetMatch{Kind: "widget", Name: t.String()}, nil
}
