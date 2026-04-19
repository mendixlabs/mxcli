// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

func (m *MockBackend) ListNavigationDocuments() ([]*types.NavigationDocument, error) {
	if m.ListNavigationDocumentsFunc != nil {
		return m.ListNavigationDocumentsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetNavigation() (*types.NavigationDocument, error) {
	if m.GetNavigationFunc != nil {
		return m.GetNavigationFunc()
	}
	return nil, nil
}

func (m *MockBackend) UpdateNavigationProfile(navDocID model.ID, profileName string, spec types.NavigationProfileSpec) error {
	if m.UpdateNavigationProfileFunc != nil {
		return m.UpdateNavigationProfileFunc(navDocID, profileName, spec)
	}
	return nil
}
