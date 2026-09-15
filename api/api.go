// SPDX-License-Identifier: Apache-2.0

// Package api provides a high-level fluent API for modifying Mendix projects.
//
// This API is inspired by the Mendix Web Extensibility Model API and provides
// a simplified interface on top of the low-level SDK types.
//
// # Opening a project
//
// For the common case, let the package open the project itself:
//
//	a, err := api.Open("app.mpr")
//	defer a.Close()
//
// For anything else — a project already open, a different engine, or a live
// Studio Pro session — hand it a backend:
//
//	a := api.New(someBackend)
//
// # Why a backend rather than a writer
//
// This package used to take a concrete `*mpr.Writer`, which tied it to one
// storage implementation and meant it did not benefit from anything the backend
// abstraction ([ADR-0002]) provides. Taking a `backend.FullBackend` instead is
// what lets the same fluent builders run against the codec engine, against a
// live Studio Pro over MCP, or against a test double — none of which were
// reachable before.
//
// Example usage:
//
//	a, _ := api.Open("app.mpr")
//	defer a.Close()
//
//	module, _ := a.GetModule("MyModule")
//	a.SetModule(module)
//
//	entity, _ := a.DomainModels.CreateEntity("Customer").
//	    Persistent().
//	    WithStringAttribute("Name", 100).
//	    Build()
//
// [ADR-0002]: ../docs/13-decisions/0002-backend-abstraction.md
package api

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/model"
)

// ModelAPI is the main entry point for the high-level API.
// It provides access to domain-specific APIs for pages, domain models,
// microflows, and enumerations.
type ModelAPI struct {
	backend backend.FullBackend

	// owned is true when Open created the backend, so Close should disconnect
	// it. A backend handed to New belongs to the caller and is left alone.
	owned bool

	// Current module context (optional)
	currentModule *model.Module

	// Domain-specific APIs
	Pages        *PagesAPI
	DomainModels *DomainModelsAPI
	Microflows   *MicroflowsAPI
	Enumerations *EnumerationsAPI
	Modules      *ModulesAPI
}

// New creates a ModelAPI over an already-connected backend.
//
// The backend's lifetime belongs to the caller: Close does not disconnect it.
// Use Open instead if you want the project opened and closed for you.
func New(b backend.FullBackend) *ModelAPI {
	api := &ModelAPI{backend: b}
	api.Pages = &PagesAPI{api: api}
	api.DomainModels = &DomainModelsAPI{api: api}
	api.Microflows = &MicroflowsAPI{api: api}
	api.Enumerations = &EnumerationsAPI{api: api}
	api.Modules = &ModulesAPI{api: api}
	return api
}

// Open opens a Mendix project for reading and writing through the default
// engine, and returns a ModelAPI over it.
//
// The returned ModelAPI owns the connection; call Close when done.
func Open(path string) (*ModelAPI, error) {
	b := modelsdkbackend.New()
	if err := b.Connect(path); err != nil {
		return nil, err
	}
	api := New(b)
	api.owned = true
	return api, nil
}

// Close releases the project if this ModelAPI opened it, and is a no-op for a
// backend handed to New.
func (api *ModelAPI) Close() error {
	if !api.owned || api.backend == nil {
		return nil
	}
	return api.backend.Disconnect()
}

// Backend returns the underlying backend, for callers that need an operation
// the fluent API does not cover.
func (api *ModelAPI) Backend() backend.FullBackend { return api.backend }

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
	return api.backend.GetModuleByName(name)
}

// ListModules returns all modules in the project.
func (api *ModelAPI) ListModules() ([]*model.Module, error) {
	return api.backend.ListModules()
}
