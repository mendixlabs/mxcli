// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ConnectionBackend manages the lifecycle of a backend connection.
type ConnectionBackend interface {
	// Connect opens a connection to the project at path.
	Connect(path string) error
	// Disconnect closes the connection, finalizing any pending work.
	Disconnect() error
	// Commit flushes any pending writes. Implementations that auto-commit
	// (e.g. MprBackend) may treat this as a no-op.
	Commit() error
	// IsConnected reports whether the backend has an active connection.
	IsConnected() bool
	// Path returns the path of the connected project, or "" if not connected.
	Path() string
	// Version returns the MPR format version.
	Version() types.MPRVersion
	// ProjectVersion returns the Mendix project version.
	ProjectVersion() *types.ProjectVersion
	// GetMendixVersion returns the Mendix version string.
	// NOTE: uses Get prefix unlike Version()/ProjectVersion() for historical SDK compatibility.
	GetMendixVersion() (string, error)
}

// ModuleBackend provides module-level operations.
type ModuleBackend interface {
	// ListModules returns all modules in the project.
	ListModules() ([]*model.Module, error)
	// GetModule returns a module by ID.
	GetModule(id model.ID) (*model.Module, error)
	// GetModuleByName returns a module by name.
	GetModuleByName(name string) (*model.Module, error)
	// CreateModule adds a new module to the project.
	CreateModule(module *model.Module) error
	// UpdateModule persists changes to an existing module.
	UpdateModule(module *model.Module) error
	// DeleteModule removes a module by ID.
	DeleteModule(id model.ID) error
	// DeleteModuleWithCleanup removes a module and cleans up associated documents.
	DeleteModuleWithCleanup(id model.ID, moduleName string) error
}

// FolderBackend provides folder operations.
type FolderBackend interface {
	// ListFolders returns all folders in the project.
	ListFolders() ([]*types.FolderInfo, error)
	// CreateFolder adds a new folder.
	CreateFolder(folder *model.Folder) error
	// DeleteFolder removes a folder by ID.
	DeleteFolder(id model.ID) error
	// MoveFolder moves a folder to a new container.
	MoveFolder(id model.ID, newContainerID model.ID) error
}
