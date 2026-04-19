// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// hierarchySource is the minimal interface needed to build a ContainerHierarchy.
// Both *mpr.Reader and backend.FullBackend satisfy this.
type hierarchySource interface {
	ListModules() ([]*model.Module, error)
	ListUnits() ([]*types.UnitInfo, error)
	ListFolders() ([]*types.FolderInfo, error)
}

// ContainerHierarchy provides efficient module and folder resolution for documents.
// It caches the container hierarchy to avoid repeated lookups.
type ContainerHierarchy struct {
	moduleIDs       map[model.ID]bool
	moduleNames     map[model.ID]string
	containerParent map[model.ID]model.ID
	folderNames     map[model.ID]string
}

// NewContainerHierarchy creates a new hierarchy from any source that provides
// modules, units, and folders (e.g. *mpr.Reader or backend.FullBackend).
func NewContainerHierarchy(src hierarchySource) (*ContainerHierarchy, error) {
	return newContainerHierarchyImpl(src)
}

// NewContainerHierarchyFromBackend creates a new hierarchy from a Backend interface.
func NewContainerHierarchyFromBackend(b backend.FullBackend) (*ContainerHierarchy, error) {
	return newContainerHierarchyImpl(b)
}

func newContainerHierarchyImpl(src hierarchySource) (*ContainerHierarchy, error) {
	h := &ContainerHierarchy{
		moduleIDs:       make(map[model.ID]bool),
		moduleNames:     make(map[model.ID]string),
		containerParent: make(map[model.ID]model.ID),
		folderNames:     make(map[model.ID]string),
	}

	modules, err := src.ListModules()
	if err != nil {
		return nil, err
	}
	for _, m := range modules {
		h.moduleIDs[m.ID] = true
		h.moduleNames[m.ID] = m.Name
	}

	units, _ := src.ListUnits()
	for _, u := range units {
		h.containerParent[u.ID] = u.ContainerID
	}

	folders, _ := src.ListFolders()
	for _, f := range folders {
		h.folderNames[f.ID] = f.Name
		h.containerParent[f.ID] = f.ContainerID
	}

	return h, nil
}

// FindModuleID finds the module ID for any container by traversing the hierarchy.
func (h *ContainerHierarchy) FindModuleID(containerID model.ID) model.ID {
	current := containerID
	for range 100 {
		if h.moduleIDs[current] {
			return current
		}
		parent, ok := h.containerParent[current]
		if !ok || parent == current {
			return containerID
		}
		current = parent
	}
	return containerID
}

// GetModuleName returns the module name for a module ID.
func (h *ContainerHierarchy) GetModuleName(moduleID model.ID) string {
	return h.moduleNames[moduleID]
}

// IsModule returns true if the ID is a module ID.
func (h *ContainerHierarchy) IsModule(id model.ID) bool {
	return h.moduleIDs[id]
}

// BuildFolderPath builds a folder path string from container to module.
func (h *ContainerHierarchy) BuildFolderPath(containerID model.ID) string {
	var parts []string
	current := containerID
	for range 100 {
		if h.moduleIDs[current] {
			break
		}
		if name := h.folderNames[current]; name != "" {
			parts = append([]string{name}, parts...)
		}
		parent, ok := h.containerParent[current]
		if !ok || parent == current {
			break
		}
		current = parent
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "/")
}

// GetQualifiedName returns the fully qualified name for a document.
func (h *ContainerHierarchy) GetQualifiedName(containerID model.ID, name string) string {
	modID := h.FindModuleID(containerID)
	modName := h.GetModuleName(modID)
	return modName + "." + name
}

// getHierarchy returns a cached ContainerHierarchy or creates a new one.
func getHierarchy(ctx *ExecContext) (*ContainerHierarchy, error) {
	if !ctx.Connected() {
		return nil, nil
	}
	if ctx.Cache == nil {
		ctx.Cache = &executorCache{}
		if ctx.executor != nil {
			ctx.executor.cache = ctx.Cache
		}
	}
	if ctx.Cache.hierarchy != nil {
		return ctx.Cache.hierarchy, nil
	}
	h, err := NewContainerHierarchyFromBackend(ctx.Backend)
	if err != nil {
		return nil, err
	}
	ctx.Cache.hierarchy = h
	return h, nil
}

// invalidateHierarchy clears the cached hierarchy so it will be rebuilt on next access.
// This should be called after any write operation that creates or deletes units.
func invalidateHierarchy(ctx *ExecContext) {
	if ctx.Cache != nil {
		ctx.Cache.hierarchy = nil
	}
}

// invalidateDomainModelsCache clears the cached domain models so they will be reloaded.
// This should be called after any write operation that creates or modifies entities.
func invalidateDomainModelsCache(ctx *ExecContext) {
	if ctx.Cache != nil {
		ctx.Cache.domainModels = nil
	}
}

// ----------------------------------------------------------------------------
// Executor method wrappers (for callers in unmigrated files)
// ----------------------------------------------------------------------------

func (e *Executor) getHierarchy() (*ContainerHierarchy, error) {
	return getHierarchy(e.newExecContext(context.Background()))
}
