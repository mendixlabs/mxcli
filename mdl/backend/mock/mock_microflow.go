// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func (m *MockBackend) ListMicroflows() ([]*microflows.Microflow, error) {
	if m.ListMicroflowsFunc != nil {
		return m.ListMicroflowsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetMicroflow(id model.ID) (*microflows.Microflow, error) {
	if m.GetMicroflowFunc != nil {
		return m.GetMicroflowFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) CreateMicroflow(mf *microflows.Microflow) error {
	if m.CreateMicroflowFunc != nil {
		return m.CreateMicroflowFunc(mf)
	}
	return nil
}

func (m *MockBackend) UpdateMicroflow(mf *microflows.Microflow) error {
	if m.UpdateMicroflowFunc != nil {
		return m.UpdateMicroflowFunc(mf)
	}
	return nil
}

func (m *MockBackend) DeleteMicroflow(id model.ID) error {
	if m.DeleteMicroflowFunc != nil {
		return m.DeleteMicroflowFunc(id)
	}
	return nil
}

func (m *MockBackend) MoveMicroflow(mf *microflows.Microflow) error {
	if m.MoveMicroflowFunc != nil {
		return m.MoveMicroflowFunc(mf)
	}
	return nil
}

func (m *MockBackend) ParseMicroflowFromRaw(raw map[string]any, unitID, containerID model.ID) *microflows.Microflow {
	if m.ParseMicroflowFromRawFunc != nil {
		return m.ParseMicroflowFromRawFunc(raw, unitID, containerID)
	}
	panic("mock ParseMicroflowFromRaw called but ParseMicroflowFromRawFunc is not set")
}

func (m *MockBackend) ParseMicroflowBSON(contents []byte, unitID, containerID model.ID) (*microflows.Microflow, error) {
	if m.ParseMicroflowBSONFunc != nil {
		return m.ParseMicroflowBSONFunc(contents, unitID, containerID)
	}
	return nil, fmt.Errorf("MockBackend.ParseMicroflowBSON not configured")
}

func (m *MockBackend) ListNanoflows() ([]*microflows.Nanoflow, error) {
	if m.ListNanoflowsFunc != nil {
		return m.ListNanoflowsFunc()
	}
	return nil, fmt.Errorf("MockBackend.ListNanoflows not configured")
}

func (m *MockBackend) GetNanoflow(id model.ID) (*microflows.Nanoflow, error) {
	if m.GetNanoflowFunc != nil {
		return m.GetNanoflowFunc(id)
	}
	return nil, fmt.Errorf("MockBackend.GetNanoflow not configured")
}

func (m *MockBackend) CreateNanoflow(nf *microflows.Nanoflow) error {
	if m.CreateNanoflowFunc != nil {
		return m.CreateNanoflowFunc(nf)
	}
	return fmt.Errorf("MockBackend.CreateNanoflow not configured")
}

func (m *MockBackend) UpdateNanoflow(nf *microflows.Nanoflow) error {
	if m.UpdateNanoflowFunc != nil {
		return m.UpdateNanoflowFunc(nf)
	}
	return fmt.Errorf("MockBackend.UpdateNanoflow not configured")
}

func (m *MockBackend) DeleteNanoflow(id model.ID) error {
	if m.DeleteNanoflowFunc != nil {
		return m.DeleteNanoflowFunc(id)
	}
	return fmt.Errorf("MockBackend.DeleteNanoflow not configured")
}

func (m *MockBackend) MoveNanoflow(nf *microflows.Nanoflow) error {
	if m.MoveNanoflowFunc != nil {
		return m.MoveNanoflowFunc(nf)
	}
	return fmt.Errorf("MockBackend.MoveNanoflow not configured")
}

func (m *MockBackend) IsRule(qualifiedName string) (bool, error) {
	if m.IsRuleFunc != nil {
		return m.IsRuleFunc(qualifiedName)
	}
	return false, nil
}
