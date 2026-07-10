// SPDX-License-Identifier: Apache-2.0

package api

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// ModulesAPI provides methods for working with modules.
type ModulesAPI struct {
	api *ModelAPI
}

// List returns all modules in the project.
func (m *ModulesAPI) List() ([]*model.Module, error) {
	return m.api.reader.ListModules()
}

// Get retrieves a module by name.
func (m *ModulesAPI) Get(name string) (*model.Module, error) {
	return m.api.reader.GetModuleByName(name)
}

// GetByID retrieves a module by ID.
func (m *ModulesAPI) GetByID(id model.ID) (*model.Module, error) {
	return m.api.reader.GetModule(id)
}
