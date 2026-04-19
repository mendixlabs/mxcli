// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
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
	return nil
}

func (m *MockBackend) ListNanoflows() ([]*microflows.Nanoflow, error) {
	if m.ListNanoflowsFunc != nil {
		return m.ListNanoflowsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetNanoflow(id model.ID) (*microflows.Nanoflow, error) {
	if m.GetNanoflowFunc != nil {
		return m.GetNanoflowFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) CreateNanoflow(nf *microflows.Nanoflow) error {
	if m.CreateNanoflowFunc != nil {
		return m.CreateNanoflowFunc(nf)
	}
	return nil
}

func (m *MockBackend) UpdateNanoflow(nf *microflows.Nanoflow) error {
	if m.UpdateNanoflowFunc != nil {
		return m.UpdateNanoflowFunc(nf)
	}
	return nil
}

func (m *MockBackend) DeleteNanoflow(id model.ID) error {
	if m.DeleteNanoflowFunc != nil {
		return m.DeleteNanoflowFunc(id)
	}
	return nil
}

func (m *MockBackend) MoveNanoflow(nf *microflows.Nanoflow) error {
	if m.MoveNanoflowFunc != nil {
		return m.MoveNanoflowFunc(nf)
	}
	return nil
}
