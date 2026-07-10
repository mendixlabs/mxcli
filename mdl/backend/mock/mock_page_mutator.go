// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

var _ backend.PageMutator = (*MockPageMutator)(nil)

// MockPageMutator implements backend.PageMutator. Every interface method is
// backed by a public function field. If the field is nil the method returns
// nil error (never panics). ContainerType defaults to ContainerPage when unset;
// all other methods return zero values.
type MockPageMutator struct {
	ContainerTypeFunc              func() backend.ContainerKind
	SetWidgetPropertyFunc          func(widgetRef string, prop string, value any) error
	SetWidgetDataSourceFunc        func(widgetRef string, ds pages.DataSource) error
	SetColumnPropertyFunc          func(gridRef string, columnRef string, prop string, value any) error
	SetDesignPropertyFunc          func(widgetRef string, key string, valueType string, option string) error
	RemoveDesignPropertyFunc       func(widgetRef string, key string) error
	ClearDesignPropertiesFunc      func(widgetRef string) error
	InsertWidgetFunc               func(widgetRef string, columnRef string, position backend.InsertPosition, widgets []pages.Widget) error
	DropWidgetFunc                 func(refs []backend.WidgetRef) error
	ReplaceWidgetFunc              func(widgetRef string, columnRef string, widgets []pages.Widget) error
	InsertColumnsFunc              func(gridRef, afterColumnRef string, position backend.InsertPosition, columns []*backend.DataGridColumnSpec) error
	ReplaceColumnFunc              func(gridRef, columnRef string, columns []*backend.DataGridColumnSpec) error
	FindWidgetFunc                 func(name string) bool
	AddVariableFunc                func(name, dataType, defaultValue string) error
	DropVariableFunc               func(name string) error
	SetLayoutFunc                  func(newLayout string, paramMappings map[string]string) error
	SetPluggablePropertyFunc       func(widgetRef string, propKey string, op backend.PluggablePropertyOp, ctx backend.PluggablePropertyContext) error
	EnclosingEntityFunc            func(widgetRef string) string
	EnclosingEntityForChildrenFunc func(widgetRef string) string
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
