// SPDX-License-Identifier: Apache-2.0

// Package api provides a high-level fluent API for modifying Mendix projects.
//
// This API is inspired by the Mendix Web Extensibility Model API and provides
// a simplified interface on top of the low-level SDK types.
//
// Example usage:
//
//	writer, _ := mpr.NewWriter("app.mpr")
//	defer writer.Close()
//
//	api := api.New(writer)
//
//	// Create an entity
//	entity, _ := api.DomainModels.CreateEntity("Customer").
//	    InModule(module).
//	    Persistent().
//	    WithStringAttribute("Name", 100).
//	    Build()
//
//	// Create a page
//	page, _ := api.Pages.CreatePage("Customer_Edit").
//	    InModule(module).
//	    WithTitle("Edit Customer").
//	    Build()
package api

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
)

// ModelAPI is the main entry point for the high-level API.
// It provides access to domain-specific APIs for pages, domain models,
// microflows, and enumerations.
type ModelAPI struct {
	writer *mpr.Writer
	reader *mpr.Reader

	// Current module context (optional)
	currentModule *model.Module

	// Domain-specific APIs
	Pages        *PagesAPI
	DomainModels *DomainModelsAPI
	Microflows   *MicroflowsAPI
	Enumerations *EnumerationsAPI
	Modules      *ModulesAPI
}

// New creates a new ModelAPI instance for the given writer.
func New(writer *mpr.Writer) *ModelAPI {
	api := &ModelAPI{
		writer: writer,
		reader: writer.Reader(),
	}

	// Initialize domain-specific APIs
	api.Pages = &PagesAPI{api: api}
	api.DomainModels = &DomainModelsAPI{api: api}
	api.Microflows = &MicroflowsAPI{api: api}
	api.Enumerations = &EnumerationsAPI{api: api}
	api.Modules = &ModulesAPI{api: api}

	return api
}

// Writer returns the underlying writer.
func (api *ModelAPI) Writer() *mpr.Writer {
	return api.writer
}

// Reader returns the underlying reader.
func (api *ModelAPI) Reader() *mpr.Reader {
	return api.reader
}

// SetModule sets the current module context.
// When set, builders will use this module by default.
func (api *ModelAPI) SetModule(module *model.Module) *ModelAPI {
	api.currentModule = module
	return api
}

// CurrentModule returns the current module context, or nil if not set.
func (api *ModelAPI) CurrentModule() *model.Module {
	return api.currentModule
}

// GetModule retrieves a module by name.
func (api *ModelAPI) GetModule(name string) (*model.Module, error) {
	return api.reader.GetModuleByName(name)
}

// ListModules returns all modules in the project.
func (api *ModelAPI) ListModules() ([]*model.Module, error) {
	return api.reader.ListModules()
}
