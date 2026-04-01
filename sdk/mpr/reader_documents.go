// SPDX-License-Identifier: Apache-2.0

// Package mpr - Document listing and retrieval methods for Reader.
package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/security"
	"github.com/mendixlabs/mxcli/sdk/workflows"

	"go.mongodb.org/mongo-driver/bson"
)

// ListModules returns all modules in the project.
func (r *Reader) ListModules() ([]*model.Module, error) {
	// Use Projects$ModuleImpl (not Projects$Module which also matches ModuleSettings)
	units, err := r.listUnitsByType("Projects$ModuleImpl")
	if err != nil {
		return nil, err
	}

	var modules []*model.Module
	for _, u := range units {
		module, err := r.parseModule(u.ID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse module %s: %w", u.ID, err)
		}
		modules = append(modules, module)
	}

	return modules, nil
}

// GetModule retrieves a module by ID.
func (r *Reader) GetModule(id model.ID) (*model.Module, error) {
	modules, err := r.ListModules()
	if err != nil {
		return nil, err
	}

	for _, m := range modules {
		if m.ID == id {
			return m, nil
		}
	}

	return nil, fmt.Errorf("module not found: %s", id)
}

// GetModuleByName retrieves a module by name.
func (r *Reader) GetModuleByName(name string) (*model.Module, error) {
	modules, err := r.ListModules()
	if err != nil {
		return nil, err
	}

	for _, m := range modules {
		if m.Name == name {
			return m, nil
		}
	}

	return nil, fmt.Errorf("module not found: %s", name)
}

// ListDomainModels returns all domain models in the project.
func (r *Reader) ListDomainModels() ([]*domainmodel.DomainModel, error) {
	units, err := r.listUnitsByType("DomainModels$DomainModel")
	if err != nil {
		return nil, err
	}

	var domainModels []*domainmodel.DomainModel
	for _, u := range units {
		dm, err := r.parseDomainModel(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse domain model %s: %w", u.ID, err)
		}
		domainModels = append(domainModels, dm)
	}

	// Load OQL queries for view entities
	oqlMap, err := r.loadViewEntityOqlQueries()
	if err != nil {
		// Non-fatal error, just skip OQL population
		return domainModels, nil
	}

	// Populate OQL queries for view entities
	for _, dm := range domainModels {
		for _, entity := range dm.Entities {
			if entity.SourceDocumentRef != "" {
				if oql, ok := oqlMap[entity.SourceDocumentRef]; ok {
					entity.OqlQuery = oql
				}
			}
		}
	}

	return domainModels, nil
}

// loadViewEntityOqlQueries loads all ViewEntitySourceDocuments and returns a map of qualified name -> OQL query.
func (r *Reader) loadViewEntityOqlQueries() (map[string]string, error) {
	units, err := r.listUnitsByType("DomainModels$ViewEntitySourceDocument")
	if err != nil {
		return nil, err
	}

	// Build module ID -> name map once (for efficiency)
	modules, err := r.ListModules()
	if err != nil {
		return nil, err
	}
	moduleNames := make(map[string]string)
	for _, m := range modules {
		moduleNames[string(m.ID)] = m.Name
	}

	result := make(map[string]string)
	for _, u := range units {
		var raw map[string]any
		if err := bson.Unmarshal(u.Contents, &raw); err != nil {
			continue
		}

		name, _ := raw["Name"].(string)
		oql, _ := raw["Oql"].(string)

		if name != "" {
			// Build qualified name from module + name
			moduleName := moduleNames[u.ContainerID]
			qualifiedName := moduleName + "." + name
			result[qualifiedName] = oql
		}
	}

	return result, nil
}

// GetDomainModel retrieves a domain model by module ID.
func (r *Reader) GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	domainModels, err := r.ListDomainModels()
	if err != nil {
		return nil, err
	}

	for _, dm := range domainModels {
		if dm.ContainerID == moduleID {
			return dm, nil
		}
	}

	return nil, fmt.Errorf("domain model not found for module: %s", moduleID)
}

// GetDomainModelByID retrieves a domain model by its own ID.
func (r *Reader) GetDomainModelByID(id model.ID) (*domainmodel.DomainModel, error) {
	domainModels, err := r.ListDomainModels()
	if err != nil {
		return nil, err
	}

	for _, dm := range domainModels {
		if dm.ID == id {
			return dm, nil
		}
	}

	return nil, fmt.Errorf("domain model not found: %s", id)
}

// ListMicroflows returns all microflows in the project.
func (r *Reader) ListMicroflows() ([]*microflows.Microflow, error) {
	units, err := r.listUnitsByType("Microflows$Microflow")
	if err != nil {
		return nil, err
	}

	var result []*microflows.Microflow
	for _, u := range units {
		mf, err := r.parseMicroflow(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse microflow %s: %w", u.ID, err)
		}
		result = append(result, mf)
	}

	return result, nil
}

// GetMicroflow retrieves a microflow by ID.
func (r *Reader) GetMicroflow(id model.ID) (*microflows.Microflow, error) {
	microflowsList, err := r.ListMicroflows()
	if err != nil {
		return nil, err
	}

	for _, mf := range microflowsList {
		if mf.ID == id {
			return mf, nil
		}
	}

	return nil, fmt.Errorf("microflow not found: %s", id)
}

// ListNanoflows returns all nanoflows in the project.
func (r *Reader) ListNanoflows() ([]*microflows.Nanoflow, error) {
	units, err := r.listUnitsByType("Microflows$Nanoflow")
	if err != nil {
		return nil, err
	}

	var result []*microflows.Nanoflow
	for _, u := range units {
		nf, err := r.parseNanoflow(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse nanoflow %s: %w", u.ID, err)
		}
		result = append(result, nf)
	}

	return result, nil
}

// GetNanoflow retrieves a nanoflow by ID.
func (r *Reader) GetNanoflow(id model.ID) (*microflows.Nanoflow, error) {
	nanoflows, err := r.ListNanoflows()
	if err != nil {
		return nil, err
	}

	for _, nf := range nanoflows {
		if nf.ID == id {
			return nf, nil
		}
	}

	return nil, fmt.Errorf("nanoflow not found: %s", id)
}

// ListPages returns all pages in the project.
func (r *Reader) ListPages() ([]*pages.Page, error) {
	// Try Forms$Page first (Mendix 10+), then Pages$Page (older versions)
	units, err := r.listUnitsByType("Forms$Page")
	if err != nil {
		return nil, err
	}
	if len(units) == 0 {
		units, err = r.listUnitsByType("Pages$Page")
		if err != nil {
			return nil, err
		}
	}

	var result []*pages.Page
	for _, u := range units {
		page, err := r.parsePage(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse page %s: %w", u.ID, err)
		}
		result = append(result, page)
	}

	return result, nil
}

// GetPage retrieves a page by ID.
func (r *Reader) GetPage(id model.ID) (*pages.Page, error) {
	pagesList, err := r.ListPages()
	if err != nil {
		return nil, err
	}

	for _, p := range pagesList {
		if p.ID == id {
			return p, nil
		}
	}

	return nil, fmt.Errorf("page not found: %s", id)
}

// ListLayouts returns all layouts in the project.
func (r *Reader) ListLayouts() ([]*pages.Layout, error) {
	// Try Forms$Layout first (Mendix 10+), then Pages$Layout (older versions)
	units, err := r.listUnitsByType("Forms$Layout")
	if err != nil {
		return nil, err
	}
	if len(units) == 0 {
		units, err = r.listUnitsByType("Pages$Layout")
		if err != nil {
			return nil, err
		}
	}

	var result []*pages.Layout
	for _, u := range units {
		layout, err := r.parseLayout(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse layout %s: %w", u.ID, err)
		}
		result = append(result, layout)
	}

	return result, nil
}

// GetLayout retrieves a layout by ID.
func (r *Reader) GetLayout(id model.ID) (*pages.Layout, error) {
	layouts, err := r.ListLayouts()
	if err != nil {
		return nil, err
	}

	for _, l := range layouts {
		if l.ID == id {
			return l, nil
		}
	}

	return nil, fmt.Errorf("layout not found: %s", id)
}

// ListEnumerations returns all enumerations in the project.
func (r *Reader) ListEnumerations() ([]*model.Enumeration, error) {
	units, err := r.listUnitsByType("Enumerations$Enumeration")
	if err != nil {
		return nil, err
	}

	var result []*model.Enumeration
	for _, u := range units {
		enum, err := r.parseEnumeration(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse enumeration %s: %w", u.ID, err)
		}
		result = append(result, enum)
	}

	return result, nil
}

// GetEnumeration retrieves an enumeration by ID.
func (r *Reader) GetEnumeration(id model.ID) (*model.Enumeration, error) {
	enums, err := r.ListEnumerations()
	if err != nil {
		return nil, err
	}

	for _, e := range enums {
		if e.ID == id {
			return e, nil
		}
	}

	return nil, fmt.Errorf("enumeration not found: %s", id)
}

// ListConstants returns all constants in the project.
func (r *Reader) ListConstants() ([]*model.Constant, error) {
	units, err := r.listUnitsByType("Constants$Constant")
	if err != nil {
		return nil, err
	}

	var result []*model.Constant
	for _, u := range units {
		constant, err := r.parseConstant(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse constant %s: %w", u.ID, err)
		}
		result = append(result, constant)
	}

	return result, nil
}

// GetConstant retrieves a constant by ID.
func (r *Reader) GetConstant(id model.ID) (*model.Constant, error) {
	constants, err := r.ListConstants()
	if err != nil {
		return nil, err
	}

	for _, c := range constants {
		if c.ID == id {
			return c, nil
		}
	}

	return nil, fmt.Errorf("constant not found: %s", id)
}

// GetRawUnit retrieves raw BSON data for a unit by ID as a map.
func (r *Reader) GetRawUnit(id model.ID) (map[string]any, error) {
	// Try to get raw contents for the unit
	var contents []byte
	var err error

	if r.version == MPRVersionV2 {
		// V2: Read from mprcontents folder
		contents, err = r.readMprContents(string(id))
		if err != nil {
			return nil, fmt.Errorf("failed to read unit contents: %w", err)
		}
	} else {
		// V1: Read from database
		row := r.db.QueryRow("SELECT Contents FROM Unit WHERE UnitID = ?", string(id))
		err = row.Scan(&contents)
		if err != nil {
			return nil, fmt.Errorf("failed to read unit from database: %w", err)
		}
	}

	contents, err = r.resolveContents(string(id), contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	return raw, nil
}

// ListScheduledEvents returns all scheduled events in the project.
func (r *Reader) ListScheduledEvents() ([]*model.ScheduledEvent, error) {
	units, err := r.listUnitsByType("ScheduledEvents$ScheduledEvent")
	if err != nil {
		return nil, err
	}

	var result []*model.ScheduledEvent
	for _, u := range units {
		event, err := r.parseScheduledEvent(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse scheduled event %s: %w", u.ID, err)
		}
		result = append(result, event)
	}

	return result, nil
}

// GetScheduledEvent retrieves a scheduled event by ID.
func (r *Reader) GetScheduledEvent(id model.ID) (*model.ScheduledEvent, error) {
	events, err := r.ListScheduledEvents()
	if err != nil {
		return nil, err
	}

	for _, e := range events {
		if e.ID == id {
			return e, nil
		}
	}

	return nil, fmt.Errorf("scheduled event not found: %s", id)
}

// ListSnippets returns all snippets in the project.
func (r *Reader) ListSnippets() ([]*pages.Snippet, error) {
	// Try Forms$Snippet first (Mendix 10+), then Pages$Snippet (older versions)
	units, err := r.listUnitsByType("Forms$Snippet")
	if err != nil {
		return nil, err
	}
	if len(units) == 0 {
		units, err = r.listUnitsByType("Pages$Snippet")
		if err != nil {
			return nil, err
		}
	}

	var result []*pages.Snippet
	for _, u := range units {
		snippet, err := r.parseSnippet(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse snippet %s: %w", u.ID, err)
		}
		result = append(result, snippet)
	}

	return result, nil
}

// GetProjectSecurity returns the project security configuration.
func (r *Reader) GetProjectSecurity() (*security.ProjectSecurity, error) {
	units, err := r.listUnitsByType("Security$ProjectSecurity")
	if err != nil {
		return nil, err
	}

	if len(units) == 0 {
		return nil, fmt.Errorf("project security not found")
	}

	return r.parseProjectSecurity(units[0].ID, units[0].ContainerID, units[0].Contents)
}

// ListModuleSecurity returns all module security configurations.
func (r *Reader) ListModuleSecurity() ([]*security.ModuleSecurity, error) {
	units, err := r.listUnitsByType("Security$ModuleSecurity")
	if err != nil {
		return nil, err
	}

	var result []*security.ModuleSecurity
	for _, u := range units {
		ms, err := r.parseModuleSecurity(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse module security %s: %w", u.ID, err)
		}
		result = append(result, ms)
	}

	return result, nil
}

// ListConsumedODataServices returns all consumed OData services in the project.
func (r *Reader) ListConsumedODataServices() ([]*model.ConsumedODataService, error) {
	units, err := r.listUnitsByType("Rest$ConsumedODataService")
	if err != nil {
		return nil, err
	}

	var result []*model.ConsumedODataService
	for _, u := range units {
		svc, err := r.parseConsumedODataService(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse consumed OData service %s: %w", u.ID, err)
		}
		result = append(result, svc)
	}

	return result, nil
}

// ListPublishedODataServices returns all published OData services in the project.
func (r *Reader) ListPublishedODataServices() ([]*model.PublishedODataService, error) {
	units, err := r.listUnitsByType("ODataPublish$PublishedODataService2")
	if err != nil {
		return nil, err
	}

	var result []*model.PublishedODataService
	for _, u := range units {
		svc, err := r.parsePublishedODataService(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse published OData service %s: %w", u.ID, err)
		}
		result = append(result, svc)
	}

	return result, nil
}

// ListPublishedRestServices returns all published REST services in the project.
func (r *Reader) ListPublishedRestServices() ([]*model.PublishedRestService, error) {
	units, err := r.listUnitsByType("Rest$PublishedRestService")
	if err != nil {
		return nil, err
	}

	var result []*model.PublishedRestService
	for _, u := range units {
		svc, err := r.parsePublishedRestService(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse published REST service %s: %w", u.ID, err)
		}
		result = append(result, svc)
	}

	return result, nil
}

// ListConsumedRestServices returns all consumed REST services in the project.
func (r *Reader) ListConsumedRestServices() ([]*model.ConsumedRestService, error) {
	units, err := r.listUnitsByType("Rest$ConsumedRestService")
	if err != nil {
		return nil, err
	}

	var result []*model.ConsumedRestService
	for _, u := range units {
		svc, err := r.parseConsumedRestService(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse consumed REST service %s: %w", u.ID, err)
		}
		result = append(result, svc)
	}

	return result, nil
}

// ListWorkflows returns all workflows in the project.
func (r *Reader) ListWorkflows() ([]*workflows.Workflow, error) {
	units, err := r.listUnitsByType("Workflows$Workflow")
	if err != nil {
		return nil, err
	}

	var result []*workflows.Workflow
	for _, u := range units {
		wf, err := r.parseWorkflow(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse workflow %s: %w", u.ID, err)
		}
		result = append(result, wf)
	}

	return result, nil
}

// GetWorkflow retrieves a workflow by ID.
func (r *Reader) GetWorkflow(id model.ID) (*workflows.Workflow, error) {
	wfs, err := r.ListWorkflows()
	if err != nil {
		return nil, err
	}

	for _, wf := range wfs {
		if wf.ID == id {
			return wf, nil
		}
	}

	return nil, fmt.Errorf("workflow not found: %s", id)
}

// ListBusinessEventServices returns all business event services in the project.
func (r *Reader) ListBusinessEventServices() ([]*model.BusinessEventService, error) {
	units, err := r.listUnitsByType("BusinessEvents$BusinessEventService")
	if err != nil {
		return nil, err
	}

	var result []*model.BusinessEventService
	for _, u := range units {
		svc, err := r.parseBusinessEventService(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse business event service %s: %w", u.ID, err)
		}
		result = append(result, svc)
	}

	return result, nil
}

// ListDatabaseConnections returns all database connections in the project.
func (r *Reader) ListDatabaseConnections() ([]*model.DatabaseConnection, error) {
	units, err := r.listUnitsByType("DatabaseConnector$DatabaseConnection")
	if err != nil {
		return nil, err
	}

	var result []*model.DatabaseConnection
	for _, u := range units {
		conn, err := r.parseDBConnection(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse database connection %s: %w", u.ID, err)
		}
		result = append(result, conn)
	}

	return result, nil
}

// GetProjectSettings returns the project settings.
func (r *Reader) GetProjectSettings() (*model.ProjectSettings, error) {
	units, err := r.listUnitsByType("Settings$ProjectSettings")
	if err != nil {
		return nil, err
	}

	if len(units) == 0 {
		return nil, fmt.Errorf("project settings not found")
	}

	return r.parseProjectSettings(units[0].ID, units[0].ContainerID, units[0].Contents)
}

// GetModuleSecurity returns the module security for a given module ID.
func (r *Reader) GetModuleSecurity(moduleID model.ID) (*security.ModuleSecurity, error) {
	allMS, err := r.ListModuleSecurity()
	if err != nil {
		return nil, err
	}

	for _, ms := range allMS {
		if ms.ContainerID == moduleID {
			return ms, nil
		}
	}

	return nil, fmt.Errorf("module security not found for module: %s", moduleID)
}

// ListJsonStructures returns all JSON structure documents in the project.
func (r *Reader) ListJsonStructures() ([]*model.JsonStructure, error) {
	units, err := r.listUnitsByType("JsonStructures$JsonStructure")
	if err != nil {
		return nil, err
	}

	var result []*model.JsonStructure
	for _, u := range units {
		js, err := r.parseJsonStructure(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JSON structure %s: %w", u.ID, err)
		}
		result = append(result, js)
	}
	return result, nil
}

// GetJsonStructureByQualifiedName retrieves a JSON structure by its qualified name (Module.Name).
func (r *Reader) GetJsonStructureByQualifiedName(moduleName, name string) (*model.JsonStructure, error) {
	all, err := r.ListJsonStructures()
	if err != nil {
		return nil, err
	}

	modules, err := r.ListModules()
	if err != nil {
		return nil, err
	}

	moduleID := ""
	for _, m := range modules {
		if m.Name == moduleName {
			moduleID = string(m.ID)
			break
		}
	}

	for _, js := range all {
		if js.Name == name && (moduleID == "" || string(js.ContainerID) == moduleID) {
			return js, nil
		}
	}
	return nil, fmt.Errorf("JSON structure %s.%s not found", moduleName, name)
}

// ListImportMappings returns all import mapping documents in the project.
func (r *Reader) ListImportMappings() ([]*model.ImportMapping, error) {
	units, err := r.listUnitsByType("ImportMappings$ImportMapping")
	if err != nil {
		return nil, err
	}

	var result []*model.ImportMapping
	for _, u := range units {
		im, err := r.parseImportMapping(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse import mapping %s: %w", u.ID, err)
		}
		result = append(result, im)
	}
	return result, nil
}

// GetImportMappingByQualifiedName retrieves an import mapping by its qualified name (Module.Name).
func (r *Reader) GetImportMappingByQualifiedName(moduleName, name string) (*model.ImportMapping, error) {
	all, err := r.ListImportMappings()
	if err != nil {
		return nil, err
	}

	modules, err := r.ListModules()
	if err != nil {
		return nil, err
	}

	moduleID := ""
	for _, m := range modules {
		if m.Name == moduleName {
			moduleID = string(m.ID)
			break
		}
	}

	for _, im := range all {
		if im.Name == name && (moduleID == "" || string(im.ContainerID) == moduleID) {
			return im, nil
		}
	}
	return nil, fmt.Errorf("import mapping %s.%s not found", moduleName, name)
}
