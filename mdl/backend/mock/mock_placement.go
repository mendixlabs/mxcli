// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// MoveDocument records nothing by default and refuses, so a test exercising a
// folder change has to say what it expects to happen rather than passing
// against a stub that silently succeeds.
func (m *MockBackend) MoveDocument(unitID, containerID model.ID) error {
	if m.MoveDocumentFunc != nil {
		return m.MoveDocumentFunc(unitID, containerID)
	}
	return fmt.Errorf("MockBackend.MoveDocument not configured")
}

// FindDocumentUnit returns "no such document" by default. Nil-with-nil-error is
// the honest default here — it is what the real backends return for a name that
// is not in the module — and it keeps an unconfigured mock from inventing a
// document for a handler to move.
func (m *MockBackend) FindDocumentUnit(moduleName, name string) (*types.DocumentUnit, error) {
	if m.FindDocumentUnitFunc != nil {
		return m.FindDocumentUnitFunc(moduleName, name)
	}
	return nil, nil
}

// FindDocumentUnits defaults to what FindDocumentUnit returns, so a test that
// configured only the single lookup sees the same document through both.
func (m *MockBackend) FindDocumentUnits(moduleName, name string) ([]*types.DocumentUnit, error) {
	if m.FindDocumentUnitsFunc != nil {
		return m.FindDocumentUnitsFunc(moduleName, name)
	}
	doc, err := m.FindDocumentUnit(moduleName, name)
	if err != nil || doc == nil {
		return nil, err
	}
	return []*types.DocumentUnit{doc}, nil
}

// ListDocumentUnits returns nothing by default. An empty project is the honest
// default for a mock: it makes a folder listing show only what the test set up
// through the typed list functions, rather than inventing documents.
func (m *MockBackend) ListDocumentUnits() ([]*types.DocumentUnit, error) {
	if m.ListDocumentUnitsFunc != nil {
		return m.ListDocumentUnitsFunc()
	}
	return nil, nil
}
