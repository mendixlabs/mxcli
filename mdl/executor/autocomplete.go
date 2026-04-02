// SPDX-License-Identifier: Apache-2.0

// Package executor - Autocomplete support for LSP and REPL.
// Returns qualified names for modules, entities, microflows, pages, etc.
package executor

// GetModuleNames returns a list of all module names for autocomplete.
func (e *Executor) GetModuleNames() []string {
	if e.reader == nil {
		return nil
	}
	modules, err := e.reader.ListModules()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(modules))
	for _, m := range modules {
		names = append(names, m.Name)
	}
	return names
}

// GetMicroflowNames returns qualified microflow names, optionally filtered by module.
func (e *Executor) GetMicroflowNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	mfs, err := e.reader.ListMicroflows()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+mf.Name)
		}
	}
	return names
}

// GetEntityNames returns qualified entity names, optionally filtered by module.
func (e *Executor) GetEntityNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	dms, err := e.reader.ListDomainModels()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, dm := range dms {
		modID := h.FindModuleID(dm.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			for _, ent := range dm.Entities {
				names = append(names, modName+"."+ent.Name)
			}
		}
	}
	return names
}

// GetPageNames returns qualified page names, optionally filtered by module.
func (e *Executor) GetPageNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	pages, err := e.reader.ListPages()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, p := range pages {
		modID := h.FindModuleID(p.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+p.Name)
		}
	}
	return names
}

// GetSnippetNames returns qualified snippet names, optionally filtered by module.
func (e *Executor) GetSnippetNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	snippets, err := e.reader.ListSnippets()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, s := range snippets {
		modID := h.FindModuleID(s.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+s.Name)
		}
	}
	return names
}

// GetAssociationNames returns qualified association names, optionally filtered by module.
func (e *Executor) GetAssociationNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	dms, err := e.reader.ListDomainModels()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, dm := range dms {
		modID := h.FindModuleID(dm.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			for _, assoc := range dm.Associations {
				names = append(names, modName+"."+assoc.Name)
			}
		}
	}
	return names
}

// GetEnumerationNames returns qualified enumeration names, optionally filtered by module.
func (e *Executor) GetEnumerationNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	enums, err := e.reader.ListEnumerations()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, enum := range enums {
		modID := h.FindModuleID(enum.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+enum.Name)
		}
	}
	return names
}

// GetLayoutNames returns qualified layout names, optionally filtered by module.
func (e *Executor) GetLayoutNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	layouts, err := e.reader.ListLayouts()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, layout := range layouts {
		modID := h.FindModuleID(layout.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+layout.Name)
		}
	}
	return names
}

// GetJavaActionNames returns qualified Java action names, optionally filtered by module.
func (e *Executor) GetJavaActionNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	actions, err := e.reader.ListJavaActions()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, action := range actions {
		modID := h.FindModuleID(action.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+action.Name)
		}
	}
	return names
}

// GetODataClientNames returns qualified consumed OData service names, optionally filtered by module.
func (e *Executor) GetODataClientNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	services, err := e.reader.ListConsumedODataServices()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+svc.Name)
		}
	}
	return names
}

// GetODataServiceNames returns qualified published OData service names, optionally filtered by module.
func (e *Executor) GetODataServiceNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	services, err := e.reader.ListPublishedODataServices()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+svc.Name)
		}
	}
	return names
}

// GetRestClientNames returns qualified consumed REST service names, optionally filtered by module.
func (e *Executor) GetRestClientNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	services, err := e.reader.ListConsumedRestServices()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+svc.Name)
		}
	}
	return names
}

// GetDatabaseConnectionNames returns qualified database connection names, optionally filtered by module.
func (e *Executor) GetDatabaseConnectionNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	connections, err := e.reader.ListDatabaseConnections()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, conn := range connections {
		modID := h.FindModuleID(conn.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+conn.Name)
		}
	}
	return names
}

// GetBusinessEventServiceNames returns qualified business event service names, optionally filtered by module.
func (e *Executor) GetBusinessEventServiceNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	services, err := e.reader.ListBusinessEventServices()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+svc.Name)
		}
	}
	return names
}

// GetJsonStructureNames returns qualified JSON structure names, optionally filtered by module.
func (e *Executor) GetJsonStructureNames(moduleFilter string) []string {
	if e.reader == nil {
		return nil
	}
	h, err := e.getHierarchy()
	if err != nil {
		return nil
	}
	structures, err := e.reader.ListJsonStructures()
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	for _, js := range structures {
		modID := h.FindModuleID(js.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleFilter == "" || modName == moduleFilter {
			names = append(names, modName+"."+js.Name)
		}
	}
	return names
}
