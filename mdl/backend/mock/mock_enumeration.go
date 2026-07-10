// SPDX-License-Identifier: Apache-2.0

package mock

import "github.com/JordtenBulte-OLC/mxcli/model"

// ---------------------------------------------------------------------------
// EnumerationBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListEnumerations() ([]*model.Enumeration, error) {
	if m.ListEnumerationsFunc != nil {
		return m.ListEnumerationsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetEnumeration(id model.ID) (*model.Enumeration, error) {
	if m.GetEnumerationFunc != nil {
		return m.GetEnumerationFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) CreateEnumeration(enum *model.Enumeration) error {
	if m.CreateEnumerationFunc != nil {
		return m.CreateEnumerationFunc(enum)
	}
	return nil
}

func (m *MockBackend) UpdateEnumeration(enum *model.Enumeration) error {
	if m.UpdateEnumerationFunc != nil {
		return m.UpdateEnumerationFunc(enum)
	}
	return nil
}

func (m *MockBackend) MoveEnumeration(enum *model.Enumeration) error {
	if m.MoveEnumerationFunc != nil {
		return m.MoveEnumerationFunc(enum)
	}
	return nil
}

func (m *MockBackend) DeleteEnumeration(id model.ID) error {
	if m.DeleteEnumerationFunc != nil {
		return m.DeleteEnumerationFunc(id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ConstantBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListConstants() ([]*model.Constant, error) {
	if m.ListConstantsFunc != nil {
		return m.ListConstantsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetConstant(id model.ID) (*model.Constant, error) {
	if m.GetConstantFunc != nil {
		return m.GetConstantFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) CreateConstant(constant *model.Constant) error {
	if m.CreateConstantFunc != nil {
		return m.CreateConstantFunc(constant)
	}
	return nil
}

func (m *MockBackend) UpdateConstant(constant *model.Constant) error {
	if m.UpdateConstantFunc != nil {
		return m.UpdateConstantFunc(constant)
	}
	return nil
}

func (m *MockBackend) MoveConstant(constant *model.Constant) error {
	if m.MoveConstantFunc != nil {
		return m.MoveConstantFunc(constant)
	}
	return nil
}

func (m *MockBackend) DeleteConstant(id model.ID) error {
	if m.DeleteConstantFunc != nil {
		return m.DeleteConstantFunc(id)
	}
	return nil
}
