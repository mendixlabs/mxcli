// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// execShowStructure handles SHOW STRUCTURE [DEPTH n] [IN module] [ALL].
func execShowStructure(ctx *ExecContext, s *ast.ShowStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	depth := min(max(s.Depth, 1), 3)

	// Ensure catalog is built (fast mode is sufficient)
	if err := ensureCatalog(ctx, false); err != nil {
		return mdlerrors.NewBackend("build catalog", err)
	}

	// Get modules from catalog
	modules, err := getStructureModules(ctx, s.InModule, s.All)
	if err != nil {
		return err
	}

	if len(modules) == 0 {
		if ctx.Format == FormatJSON {
			fmt.Fprintln(ctx.Output, "[]")
		} else {
			fmt.Fprintln(ctx.Output, "(no modules found)")
		}
		return nil
	}

	// JSON mode: emit structured table
	if ctx.Format == FormatJSON {
		return structureDepth1JSON(ctx, modules)
	}

	switch depth {
	case 1:
		return structureDepth1(ctx, modules)
	case 2:
		return structureDepth2(ctx, modules)
	case 3:
		return structureDepth3(ctx, modules)
	default:
		return structureDepth2(ctx, modules)
	}
}

// structureDepth1JSON emits structure as a JSON table with one row per module
// and columns for each element type count.
func structureDepth1JSON(ctx *ExecContext, modules []structureModule) error {
	entityCounts := queryCountByModule(ctx, "entities")
	mfCounts := queryCountByModule(ctx, "microflows WHERE MicroflowType = 'MICROFLOW'")
	nfCounts := queryCountByModule(ctx, "microflows WHERE MicroflowType = 'NANOFLOW'")
	pageCounts := queryCountByModule(ctx, "pages")
	enumCounts := queryCountByModule(ctx, "enumerations")
	snippetCounts := queryCountByModule(ctx, "snippets")
	jaCounts := queryCountByModule(ctx, "java_actions")
	wfCounts := queryCountByModule(ctx, "workflows")
	odataClientCounts := queryCountByModule(ctx, "odata_clients")
	odataServiceCounts := queryCountByModule(ctx, "odata_services")
	beServiceCounts := queryCountByModule(ctx, "business_event_services")
	constantCounts := countByModuleFromBackend(ctx, "constants")
	scheduledEventCounts := countByModuleFromBackend(ctx, "scheduled_events")

	tr := &TableResult{
		Columns: []string{
			"Module", "Entities", "Enumerations", "Microflows", "Nanoflows",
			"Workflows", "Pages", "Snippets", "JavaActions", "Constants",
			"ScheduledEvents", "ODataClients", "ODataServices", "BusinessEventServices",
		},
	}
	for _, m := range modules {
		tr.Rows = append(tr.Rows, []any{
			m.Name,
			entityCounts[m.Name],
			enumCounts[m.Name],
			mfCounts[m.Name],
			nfCounts[m.Name],
			wfCounts[m.Name],
			pageCounts[m.Name],
			snippetCounts[m.Name],
			jaCounts[m.Name],
			constantCounts[m.Name],
			scheduledEventCounts[m.Name],
			odataClientCounts[m.Name],
			odataServiceCounts[m.Name],
			beServiceCounts[m.Name],
		})
	}
	return writeResult(ctx, tr)
}

// structureModule holds module info for structure output.
type structureModule struct {
	Name string
	ID   model.ID
}

// getStructureModules returns filtered and sorted modules for structure output.
func getStructureModules(ctx *ExecContext, filterModule string, includeAll bool) ([]structureModule, error) {
	result, err := ctx.Catalog.Query("SELECT Id, Name, Source, AppStoreGuid FROM modules ORDER BY Name")
	if err != nil {
		return nil, mdlerrors.NewBackend("query modules", err)
	}

	var modules []structureModule
	for _, row := range result.Rows {
		id := asString(row[0])
		name := asString(row[1])
		source := asString(row[2])
		appStoreGuid := asString(row[3])

		// Filter by module name if specified
		if filterModule != "" && !strings.EqualFold(name, filterModule) {
			continue
		}

		// Skip system/marketplace modules unless --all
		if !includeAll && !isUserModule(name, source, appStoreGuid) {
			continue
		}

		modules = append(modules, structureModule{Name: name, ID: model.ID(id)})
	}

	sort.Slice(modules, func(i, j int) bool {
		return strings.ToLower(modules[i].Name) < strings.ToLower(modules[j].Name)
	})

	return modules, nil
}

// isUserModule returns true if the module is a user-created module (not system or marketplace).
func isUserModule(name, source, appStoreGuid string) bool {
	if source != "" {
		return false
	}
	if appStoreGuid != "" {
		return false
	}
	if strings.HasPrefix(name, "_") {
		return false
	}
	return true
}

// asString converts an interface{} value to string.
func asString(v any) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case int64:
		return fmt.Sprintf("%d", s)
	default:
		return fmt.Sprintf("%v", s)
	}
}

// ============================================================================
// Depth 1 — Module Summary
// ============================================================================

func structureDepth1(ctx *ExecContext, modules []structureModule) error {
	// Query counts per module from catalog
	entityCounts := queryCountByModule(ctx, "entities")
	mfCounts := queryCountByModule(ctx, "microflows WHERE MicroflowType = 'MICROFLOW'")
	nfCounts := queryCountByModule(ctx, "microflows WHERE MicroflowType = 'NANOFLOW'")
	pageCounts := queryCountByModule(ctx, "pages")
	enumCounts := queryCountByModule(ctx, "enumerations")
	snippetCounts := queryCountByModule(ctx, "snippets")
	jaCounts := queryCountByModule(ctx, "java_actions")
	wfCounts := queryCountByModule(ctx, "workflows")
	odataClientCounts := queryCountByModule(ctx, "odata_clients")
	odataServiceCounts := queryCountByModule(ctx, "odata_services")
	beServiceCounts := queryCountByModule(ctx, "business_event_services")

	// Get constants and scheduled events from backend (no catalog tables)
	constantCounts := countByModuleFromBackend(ctx, "constants")
	scheduledEventCounts := countByModuleFromBackend(ctx, "scheduled_events")

	// Calculate name column width for alignment
	nameWidth := 0
	for _, m := range modules {
		if len(m.Name) > nameWidth {
			nameWidth = len(m.Name)
		}
	}

	for _, m := range modules {
		var parts []string

		if c := entityCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "entity", "entities"))
		}
		if c := enumCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "enum", "enums"))
		}
		if c := mfCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "microflow", "microflows"))
		}
		if c := nfCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "nanoflow", "nanoflows"))
		}
		if c := wfCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "workflow", "workflows"))
		}
		if c := pageCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "page", "pages"))
		}
		if c := snippetCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "snippet", "snippets"))
		}
		if c := jaCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "java action", "java actions"))
		}
		if c := constantCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "constant", "constants"))
		}
		if c := scheduledEventCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "scheduled event", "scheduled events"))
		}
		if c := odataClientCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "odata client", "odata clients"))
		}
		if c := odataServiceCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "odata service", "odata services"))
		}
		if c := beServiceCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "business event service", "business event services"))
		}

		if len(parts) > 0 {
			fmt.Fprintf(ctx.Output, "%-*s  %s\n", nameWidth, m.Name, strings.Join(parts, ", "))
		}
	}
	return nil
}

// queryCountByModule queries a catalog table and returns a map of module name → count.
func queryCountByModule(ctx *ExecContext, tableAndWhere string) map[string]int {
	counts := make(map[string]int)
	sql := fmt.Sprintf("SELECT ModuleName, COUNT(*) FROM %s GROUP BY ModuleName", tableAndWhere)
	result, err := ctx.Catalog.Query(sql)
	if err != nil {
		return counts
	}
	for _, row := range result.Rows {
		name := asString(row[0])
		counts[name] = toInt(row[1])
	}
	return counts
}

// countByModuleFromBackend counts elements per module using the backend (for types without catalog tables).
func countByModuleFromBackend(ctx *ExecContext, kind string) map[string]int {
	counts := make(map[string]int)
	h, err := getHierarchy(ctx)
	if err != nil {
		return counts
	}

	switch kind {
	case "constants":
		if constants, err := ctx.Backend.ListConstants(); err == nil {
			for _, c := range constants {
				modID := h.FindModuleID(c.ContainerID)
				modName := h.GetModuleName(modID)
				counts[modName]++
			}
		}
	case "scheduled_events":
		if events, err := ctx.Backend.ListScheduledEvents(); err == nil {
			for _, ev := range events {
				modID := h.FindModuleID(ev.ContainerID)
				modName := h.GetModuleName(modID)
				counts[modName]++
			}
		}
	}
	return counts
}

// pluralize returns "N thing" or "N things" depending on count.
func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}

// ============================================================================
// Depth 2 — Elements with Signatures
// ============================================================================

func structureDepth2(ctx *ExecContext, modules []structureModule) error {
	// Pre-load data from the backend
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Load domain models for associations
	domainModels, _ := ctx.Backend.ListDomainModels()
	dmByModule := make(map[string]*domainmodel.DomainModel)
	for _, dm := range domainModels {
		modID := h.FindModuleID(dm.ContainerID)
		modName := h.GetModuleName(modID)
		dmByModule[modName] = dm
	}

	// Load enumerations for values
	allEnums, _ := ctx.Backend.ListEnumerations()
	enumsByModule := make(map[string][]*model.Enumeration)
	for _, enum := range allEnums {
		modID := h.FindModuleID(enum.ContainerID)
		modName := h.GetModuleName(modID)
		enumsByModule[modName] = append(enumsByModule[modName], enum)
	}

	// Load microflows for parameter types
	allMicroflows, _ := ctx.Backend.ListMicroflows()
	mfByModule := make(map[string][]*microflows.Microflow)
	for _, mf := range allMicroflows {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		mfByModule[modName] = append(mfByModule[modName], mf)
	}

	// Load nanoflows
	allNanoflows, _ := ctx.Backend.ListNanoflows()
	nfByModule := make(map[string][]*microflows.Nanoflow)
	for _, nf := range allNanoflows {
		modID := h.FindModuleID(nf.ContainerID)
		modName := h.GetModuleName(modID)
		nfByModule[modName] = append(nfByModule[modName], nf)
	}

	// Load constants
	allConstants, _ := ctx.Backend.ListConstants()
	constByModule := make(map[string][]*model.Constant)
	for _, c := range allConstants {
		modID := h.FindModuleID(c.ContainerID)
		modName := h.GetModuleName(modID)
		constByModule[modName] = append(constByModule[modName], c)
	}

	// Load scheduled events
	allEvents, _ := ctx.Backend.ListScheduledEvents()
	eventsByModule := make(map[string][]*model.ScheduledEvent)
	for _, ev := range allEvents {
		modID := h.FindModuleID(ev.ContainerID)
		modName := h.GetModuleName(modID)
		eventsByModule[modName] = append(eventsByModule[modName], ev)
	}

	// Load java actions for parameter types
	allJavaActions, _ := ctx.Backend.ListJavaActionsFull()
	jaByModule := make(map[string][]*javaactions.JavaAction)
	for _, ja := range allJavaActions {
		modID := h.FindModuleID(ja.ContainerID)
		modName := h.GetModuleName(modID)
		jaByModule[modName] = append(jaByModule[modName], ja)
	}

	// Load workflows
	allWorkflows, _ := ctx.Backend.ListWorkflows()
	wfByModule := make(map[string][]*workflows.Workflow)
	for _, wf := range allWorkflows {
		modID := h.FindModuleID(wf.ContainerID)
		modName := h.GetModuleName(modID)
		wfByModule[modName] = append(wfByModule[modName], wf)
	}

	for i, m := range modules {
		if i > 0 {
			fmt.Fprintln(ctx.Output)
		}
		fmt.Fprintln(ctx.Output, m.Name)

		// Entities
		structureEntities(ctx, m.Name, dmByModule[m.Name], false)

		// Enumerations
		if enums, ok := enumsByModule[m.Name]; ok {
			sortEnumerations(enums)
			for _, enum := range enums {
				values := make([]string, len(enum.Values))
				for i, v := range enum.Values {
					values[i] = v.Name
				}
				fmt.Fprintf(ctx.Output, "  Enumeration %s.%s [%s]\n", m.Name, enum.Name, strings.Join(values, ", "))
			}
		}

		// Microflows
		if mfs, ok := mfByModule[m.Name]; ok {
			sortMicroflows(mfs)
			for _, mf := range mfs {
				fmt.Fprintf(ctx.Output, "  Microflow %s.%s%s\n", m.Name, mf.Name, formatMicroflowSignature(mf.Parameters, mf.ReturnType, false))
			}
		}

		// Nanoflows
		if nfs, ok := nfByModule[m.Name]; ok {
			sortNanoflows(nfs)
			for _, nf := range nfs {
				fmt.Fprintf(ctx.Output, "  Nanoflow %s.%s%s\n", m.Name, nf.Name, formatMicroflowSignature(nf.Parameters, nf.ReturnType, false))
			}
		}

		// Workflows
		structureWorkflows(ctx, m.Name, wfByModule[m.Name], false)

		// Pages (from catalog)
		structurePages(ctx, m.Name)

		// Snippets (from catalog)
		structureSnippets(ctx, m.Name)

		// Java Actions
		outputJavaActions(ctx, m.Name, jaByModule[m.Name], false)

		// Constants
		if consts, ok := constByModule[m.Name]; ok {
			sortConstants(consts)
			for _, c := range consts {
				fmt.Fprintf(ctx.Output, "  Constant %s.%s: %s\n", m.Name, c.Name, formatConstantTypeBrief(c.Type))
			}
		}

		// Scheduled Events
		if events, ok := eventsByModule[m.Name]; ok {
			sortScheduledEvents(events)
			for _, ev := range events {
				fmt.Fprintf(ctx.Output, "  ScheduledEvent %s.%s\n", m.Name, ev.Name)
			}
		}

		// OData Clients
		structureODataClients(ctx, m.Name)

		// OData Services
		structureODataServices(ctx, m.Name)

		// Business Event Services
		structureBusinessEventServices(ctx, m.Name)
	}

	return nil
}

// ============================================================================
// Depth 3 — Include Types and Details
// ============================================================================

func structureDepth3(ctx *ExecContext, modules []structureModule) error {
	// Same data loading as depth 2
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	domainModels, _ := ctx.Backend.ListDomainModels()
	dmByModule := make(map[string]*domainmodel.DomainModel)
	for _, dm := range domainModels {
		modID := h.FindModuleID(dm.ContainerID)
		modName := h.GetModuleName(modID)
		dmByModule[modName] = dm
	}

	allEnums, _ := ctx.Backend.ListEnumerations()
	enumsByModule := make(map[string][]*model.Enumeration)
	for _, enum := range allEnums {
		modID := h.FindModuleID(enum.ContainerID)
		modName := h.GetModuleName(modID)
		enumsByModule[modName] = append(enumsByModule[modName], enum)
	}

	allMicroflows, _ := ctx.Backend.ListMicroflows()
	mfByModule := make(map[string][]*microflows.Microflow)
	for _, mf := range allMicroflows {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		mfByModule[modName] = append(mfByModule[modName], mf)
	}

	allNanoflows, _ := ctx.Backend.ListNanoflows()
	nfByModule := make(map[string][]*microflows.Nanoflow)
	for _, nf := range allNanoflows {
		modID := h.FindModuleID(nf.ContainerID)
		modName := h.GetModuleName(modID)
		nfByModule[modName] = append(nfByModule[modName], nf)
	}

	allConstants, _ := ctx.Backend.ListConstants()
	constByModule := make(map[string][]*model.Constant)
	for _, c := range allConstants {
		modID := h.FindModuleID(c.ContainerID)
		modName := h.GetModuleName(modID)
		constByModule[modName] = append(constByModule[modName], c)
	}

	allEvents, _ := ctx.Backend.ListScheduledEvents()
	eventsByModule := make(map[string][]*model.ScheduledEvent)
	for _, ev := range allEvents {
		modID := h.FindModuleID(ev.ContainerID)
		modName := h.GetModuleName(modID)
		eventsByModule[modName] = append(eventsByModule[modName], ev)
	}

	allJavaActions, _ := ctx.Backend.ListJavaActionsFull()
	jaByModule := make(map[string][]*javaactions.JavaAction)
	for _, ja := range allJavaActions {
		modID := h.FindModuleID(ja.ContainerID)
		modName := h.GetModuleName(modID)
		jaByModule[modName] = append(jaByModule[modName], ja)
	}

	// Load workflows
	allWorkflows, _ := ctx.Backend.ListWorkflows()
	wfByModule := make(map[string][]*workflows.Workflow)
	for _, wf := range allWorkflows {
		modID := h.FindModuleID(wf.ContainerID)
		modName := h.GetModuleName(modID)
		wfByModule[modName] = append(wfByModule[modName], wf)
	}

	for i, m := range modules {
		if i > 0 {
			fmt.Fprintln(ctx.Output)
		}
		fmt.Fprintln(ctx.Output, m.Name)

		// Entities (with types)
		structureEntities(ctx, m.Name, dmByModule[m.Name], true)

		// Enumerations
		if enums, ok := enumsByModule[m.Name]; ok {
			sortEnumerations(enums)
			for _, enum := range enums {
				values := make([]string, len(enum.Values))
				for i, v := range enum.Values {
					values[i] = v.Name
				}
				fmt.Fprintf(ctx.Output, "  Enumeration %s.%s [%s]\n", m.Name, enum.Name, strings.Join(values, ", "))
			}
		}

		// Microflows (with param names)
		if mfs, ok := mfByModule[m.Name]; ok {
			sortMicroflows(mfs)
			for _, mf := range mfs {
				fmt.Fprintf(ctx.Output, "  Microflow %s.%s%s\n", m.Name, mf.Name, formatMicroflowSignature(mf.Parameters, mf.ReturnType, true))
			}
		}

		// Nanoflows (with param names)
		if nfs, ok := nfByModule[m.Name]; ok {
			sortNanoflows(nfs)
			for _, nf := range nfs {
				fmt.Fprintf(ctx.Output, "  Nanoflow %s.%s%s\n", m.Name, nf.Name, formatMicroflowSignature(nf.Parameters, nf.ReturnType, true))
			}
		}

		// Workflows (with details)
		structureWorkflows(ctx, m.Name, wfByModule[m.Name], true)

		// Pages
		structurePages(ctx, m.Name)

		// Snippets
		structureSnippets(ctx, m.Name)

		// Java Actions (with param names)
		outputJavaActions(ctx, m.Name, jaByModule[m.Name], true)

		// Constants (with default value)
		if consts, ok := constByModule[m.Name]; ok {
			sortConstants(consts)
			for _, c := range consts {
				s := fmt.Sprintf("  Constant %s.%s: %s", m.Name, c.Name, formatConstantTypeBrief(c.Type))
				if c.DefaultValue != "" {
					s += " = " + c.DefaultValue
				}
				fmt.Fprintln(ctx.Output, s)
			}
		}

		// Scheduled Events
		if events, ok := eventsByModule[m.Name]; ok {
			sortScheduledEvents(events)
			for _, ev := range events {
				fmt.Fprintf(ctx.Output, "  ScheduledEvent %s.%s\n", m.Name, ev.Name)
			}
		}

		// OData
		structureODataClients(ctx, m.Name)
		structureODataServices(ctx, m.Name)

		// Business Event Services
		structureBusinessEventServices(ctx, m.Name)
	}

	return nil
}

// ============================================================================
// Shared Element Formatters
// ============================================================================

// structureEntities outputs entities for a module.
func structureEntities(ctx *ExecContext, moduleName string, dm *domainmodel.DomainModel, withTypes bool) {
	if dm == nil {
		return
	}

	// Build entity ID → name map for association resolution
	entityByID := make(map[model.ID]string)
	for _, ent := range dm.Entities {
		entityByID[ent.ID] = ent.Name
	}

	// Sort entities alphabetically
	entities := make([]*domainmodel.Entity, len(dm.Entities))
	copy(entities, dm.Entities)
	sort.Slice(entities, func(i, j int) bool {
		return strings.ToLower(entities[i].Name) < strings.ToLower(entities[j].Name)
	})

	// Build association lookup: parent entity ID → associations
	assocByParent := make(map[model.ID][]*domainmodel.Association)
	for _, assoc := range dm.Associations {
		assocByParent[assoc.ParentID] = append(assocByParent[assoc.ParentID], assoc)
	}

	for _, ent := range entities {
		// Format attributes
		var attrParts []string
		for _, attr := range ent.Attributes {
			if withTypes {
				attrParts = append(attrParts, formatAttributeWithType(attr))
			} else {
				attrParts = append(attrParts, attr.Name)
			}
		}
		qualName := moduleName + "." + ent.Name
		if len(attrParts) > 0 {
			fmt.Fprintf(ctx.Output, "  Entity %s [%s]\n", qualName, strings.Join(attrParts, ", "))
		} else {
			fmt.Fprintf(ctx.Output, "  Entity %s\n", qualName)
		}

		// Format associations (owned by this entity)
		if assocs, ok := assocByParent[ent.ID]; ok {
			var assocParts []string
			for _, assoc := range assocs {
				childName := entityByID[assoc.ChildID]
				if childName == "" {
					childName = "?"
				}
				cardinality := "(1)"
				if assoc.Type == domainmodel.AssociationTypeReferenceSet {
					cardinality = "(*)"
				}
				part := fmt.Sprintf("→ %s %s", childName, cardinality)
				if withTypes {
					// Add delete behavior if non-default (DeleteMeButKeepReferences is default)
					if assoc.ChildDeleteBehavior != nil && assoc.ChildDeleteBehavior.Type == domainmodel.DeleteBehaviorTypeDeleteMeAndReferences {
						part += " CASCADE"
					} else if assoc.ChildDeleteBehavior != nil && assoc.ChildDeleteBehavior.Type == domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences {
						part += " RESTRICT"
					}
				}
				assocParts = append(assocParts, part)
			}
			if len(assocParts) > 0 {
				fmt.Fprintf(ctx.Output, "    %s\n", strings.Join(assocParts, ", "))
			}
		}
	}
}

// structurePages outputs pages for a module from the catalog.
func structurePages(ctx *ExecContext, moduleName string) {
	// Query pages from catalog
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"SELECT Name FROM pages WHERE ModuleName = '%s' ORDER BY Name",
		escapeSQLString(moduleName)))
	if err != nil || len(result.Rows) == 0 {
		return
	}

	// Try to get top-level data widgets from widgets table
	widgetsByPage := make(map[string][]string)
	widgetResult, err := ctx.Catalog.Query(fmt.Sprintf(
		"SELECT ContainerQualifiedName, WidgetType, EntityRef FROM widgets WHERE ModuleName = '%s' AND ParentWidget = '' ORDER BY ContainerQualifiedName, WidgetType",
		escapeSQLString(moduleName)))
	if err == nil {
		for _, row := range widgetResult.Rows {
			pageName := asString(row[0])
			widgetType := asString(row[1])
			entityRef := asString(row[2])

			// Only include data-bound widgets
			if !isDataWidget(widgetType) {
				continue
			}

			// Extract short widget type name
			shortType := shortWidgetType(widgetType)
			if entityRef != "" {
				// Extract entity name from qualified name
				shortEntity := shortName(entityRef)
				widgetsByPage[pageName] = append(widgetsByPage[pageName], fmt.Sprintf("%s<%s>", shortType, shortEntity))
			} else {
				widgetsByPage[pageName] = append(widgetsByPage[pageName], shortType)
			}
		}
	}

	for _, row := range result.Rows {
		name := asString(row[0])
		qualName := moduleName + "." + name
		if widgets, ok := widgetsByPage[qualName]; ok && len(widgets) > 0 {
			fmt.Fprintf(ctx.Output, "  Page %s [%s]\n", qualName, strings.Join(widgets, ", "))
		} else {
			fmt.Fprintf(ctx.Output, "  Page %s\n", qualName)
		}
	}
}

// structureSnippets outputs snippets for a module from the catalog.
func structureSnippets(ctx *ExecContext, moduleName string) {
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"SELECT Name FROM snippets WHERE ModuleName = '%s' ORDER BY Name",
		escapeSQLString(moduleName)))
	if err != nil || len(result.Rows) == 0 {
		return
	}

	for _, row := range result.Rows {
		name := asString(row[0])
		fmt.Fprintf(ctx.Output, "  Snippet %s.%s\n", moduleName, name)
	}
}

// outputJavaActions outputs java actions for a module.
func outputJavaActions(ctx *ExecContext, moduleName string, actions []*javaactions.JavaAction, withNames bool) {
	if len(actions) == 0 {
		return
	}

	// Sort alphabetically
	sorted := make([]*javaactions.JavaAction, len(actions))
	copy(sorted, actions)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
	})

	for _, ja := range sorted {
		sig := formatJavaActionSignature(ja, withNames)
		fmt.Fprintf(ctx.Output, "  JavaAction %s.%s%s\n", moduleName, ja.Name, sig)
	}
}

// formatJavaActionSignature formats the parameter list and return type of a java action.
func formatJavaActionSignature(ja *javaactions.JavaAction, withNames bool) string {
	var paramParts []string
	for _, p := range ja.Parameters {
		typeName := ""
		if p.ParameterType != nil {
			typeName = formatJATypeDisplay(p.ParameterType.TypeString())
		}
		if withNames && p.Name != "" {
			paramParts = append(paramParts, fmt.Sprintf("%s: %s", p.Name, typeName))
		} else {
			paramParts = append(paramParts, typeName)
		}
	}

	sig := "(" + strings.Join(paramParts, ", ") + ")"

	// Add return type
	if ja.ReturnType != nil {
		retStr := ja.ReturnType.TypeString()
		if retStr != "" && retStr != "Void" && retStr != "Nothing" {
			sig += " → " + formatJATypeDisplay(retStr)
		}
	}

	return sig
}

// formatJATypeDisplay formats a java action type string for display.
func formatJATypeDisplay(typeStr string) string {
	// TypeString() returns things like "Module.Entity", "List of Module.Entity", "Boolean", etc.
	if after, ok := strings.CutPrefix(typeStr, "List of "); ok {
		entity := after
		return "List<" + shortName(entity) + ">"
	}
	// Check if it's a qualified name (contains a dot)
	if strings.Contains(typeStr, ".") {
		return shortName(typeStr)
	}
	return typeStr
}

// structureODataClients outputs OData clients for a module.
func structureODataClients(ctx *ExecContext, moduleName string) {
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"SELECT Name, ODataVersion FROM odata_clients WHERE ModuleName = '%s' ORDER BY Name",
		escapeSQLString(moduleName)))
	if err != nil || len(result.Rows) == 0 {
		return
	}

	for _, row := range result.Rows {
		name := asString(row[0])
		version := asString(row[1])
		qualName := moduleName + "." + name
		if version != "" {
			fmt.Fprintf(ctx.Output, "  ODataClient %s (%s)\n", qualName, version)
		} else {
			fmt.Fprintf(ctx.Output, "  ODataClient %s\n", qualName)
		}
	}
}

// structureODataServices outputs OData services for a module.
func structureODataServices(ctx *ExecContext, moduleName string) {
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"SELECT Name, Path, EntitySetCount FROM odata_services WHERE ModuleName = '%s' ORDER BY Name",
		escapeSQLString(moduleName)))
	if err != nil || len(result.Rows) == 0 {
		return
	}

	for _, row := range result.Rows {
		name := asString(row[0])
		path := asString(row[1])
		entitySetCount := toInt(row[2])
		qualName := moduleName + "." + name
		if path != "" {
			fmt.Fprintf(ctx.Output, "  ODataService %s %s (%s)\n", qualName, path, pluralize(entitySetCount, "entity", "entities"))
		} else {
			fmt.Fprintf(ctx.Output, "  ODataService %s\n", qualName)
		}
	}
}

// structureBusinessEventServices outputs business event services for a module.
func structureBusinessEventServices(ctx *ExecContext, moduleName string) {
	result, err := ctx.Catalog.Query(fmt.Sprintf(
		"SELECT Name, MessageCount, PublishCount, SubscribeCount FROM business_event_services WHERE ModuleName = '%s' ORDER BY Name",
		escapeSQLString(moduleName)))
	if err != nil || len(result.Rows) == 0 {
		return
	}

	for _, row := range result.Rows {
		name := asString(row[0])
		msgCount := toInt(row[1])
		publishCount := toInt(row[2])
		subscribeCount := toInt(row[3])
		qualName := moduleName + "." + name

		var parts []string
		if msgCount > 0 {
			parts = append(parts, pluralize(msgCount, "message", "messages"))
		}
		if publishCount > 0 {
			parts = append(parts, pluralize(publishCount, "publish", "publish"))
		}
		if subscribeCount > 0 {
			parts = append(parts, pluralize(subscribeCount, "subscribe", "subscribe"))
		}

		if len(parts) > 0 {
			fmt.Fprintf(ctx.Output, "  BusinessEventService %s (%s)\n", qualName, strings.Join(parts, ", "))
		} else {
			fmt.Fprintf(ctx.Output, "  BusinessEventService %s\n", qualName)
		}
	}
}

// structureWorkflows outputs workflows for a module.
func structureWorkflows(ctx *ExecContext, moduleName string, wfs []*workflows.Workflow, withDetails bool) {
	if len(wfs) == 0 {
		return
	}

	// Sort alphabetically
	sorted := make([]*workflows.Workflow, len(wfs))
	copy(sorted, wfs)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
	})

	for _, wf := range sorted {
		qualName := moduleName + "." + wf.Name
		var parts []string

		// Count activities
		total, userTasks, _, decisions := countStructureWorkflowActivities(wf)
		if total > 0 {
			parts = append(parts, pluralize(total, "activity", "activities"))
		}
		if userTasks > 0 {
			parts = append(parts, pluralize(userTasks, "user task", "user tasks"))
		}
		if decisions > 0 {
			parts = append(parts, pluralize(decisions, "decision", "decisions"))
		}

		if withDetails && wf.Parameter != nil && wf.Parameter.EntityRef != "" {
			entityPart := "param: " + shortName(wf.Parameter.EntityRef)
			parts = append(parts, entityPart)
		}

		if len(parts) > 0 {
			fmt.Fprintf(ctx.Output, "  Workflow %s (%s)\n", qualName, strings.Join(parts, ", "))
		} else {
			fmt.Fprintf(ctx.Output, "  Workflow %s\n", qualName)
		}
	}
}

// countStructureWorkflowActivities counts activity types in a workflow for structure output.
func countStructureWorkflowActivities(wf *workflows.Workflow) (total, userTasks, microflowCalls, decisions int) {
	if wf.Flow == nil {
		return
	}
	countStructureFlowActivities(wf.Flow, &total, &userTasks, &microflowCalls, &decisions)
	return
}

// countStructureFlowActivities recursively counts activity types in a flow.
func countStructureFlowActivities(flow *workflows.Flow, total, userTasks, microflowCalls, decisions *int) {
	if flow == nil {
		return
	}
	for _, act := range flow.Activities {
		*total++
		switch a := act.(type) {
		case *workflows.UserTask:
			*userTasks++
			for _, outcome := range a.Outcomes {
				countStructureFlowActivities(outcome.Flow, total, userTasks, microflowCalls, decisions)
			}
		case *workflows.CallMicroflowTask:
			*microflowCalls++
			for _, outcome := range a.Outcomes {
				if outcome != nil {
					countStructureFlowActivities(outcome.GetFlow(), total, userTasks, microflowCalls, decisions)
				}
			}
		case *workflows.SystemTask:
			*microflowCalls++
			for _, outcome := range a.Outcomes {
				if outcome != nil {
					countStructureFlowActivities(outcome.GetFlow(), total, userTasks, microflowCalls, decisions)
				}
			}
		case *workflows.ExclusiveSplitActivity:
			*decisions++
			for _, outcome := range a.Outcomes {
				if outcome != nil {
					countStructureFlowActivities(outcome.GetFlow(), total, userTasks, microflowCalls, decisions)
				}
			}
		case *workflows.ParallelSplitActivity:
			for _, outcome := range a.Outcomes {
				countStructureFlowActivities(outcome.Flow, total, userTasks, microflowCalls, decisions)
			}
		}
	}
}

// ============================================================================
// Formatting Helpers
// ============================================================================

// formatMicroflowSignature formats the parameter list and return type of a microflow.
func formatMicroflowSignature(params []*microflows.MicroflowParameter, returnType microflows.DataType, withNames bool) string {
	var paramParts []string
	for _, p := range params {
		typeName := formatDataTypeDisplay(p.Type)
		if withNames && p.Name != "" {
			paramParts = append(paramParts, fmt.Sprintf("%s: %s", p.Name, typeName))
		} else {
			paramParts = append(paramParts, typeName)
		}
	}

	sig := "(" + strings.Join(paramParts, ", ") + ")"

	// Add return type
	if returnType != nil {
		retName := formatDataTypeDisplay(returnType)
		if retName != "" && retName != "Void" && retName != "Nothing" {
			sig += " → " + retName
		}
	}

	return sig
}

// formatDataTypeDisplay formats a microflow DataType for display.
func formatDataTypeDisplay(dt microflows.DataType) string {
	if dt == nil {
		return ""
	}
	switch t := dt.(type) {
	case *microflows.BooleanType:
		return "Boolean"
	case *microflows.IntegerType:
		return "Integer"
	case *microflows.LongType:
		return "Long"
	case *microflows.DecimalType:
		return "Decimal"
	case *microflows.StringType:
		return "String"
	case *microflows.DateTimeType:
		return "DateTime"
	case *microflows.DateType:
		return "Date"
	case *microflows.ObjectType:
		return shortName(t.EntityQualifiedName)
	case *microflows.ListType:
		return "List<" + shortName(t.EntityQualifiedName) + ">"
	case *microflows.EnumerationType:
		return shortName(t.EnumerationQualifiedName)
	case *microflows.VoidType:
		return "Void"
	case *microflows.BinaryType:
		return "Binary"
	default:
		return dt.GetTypeName()
	}
}

// formatAttributeWithType formats an attribute with its type for depth 3.
func formatAttributeWithType(attr *domainmodel.Attribute) string {
	if attr.Type == nil {
		return attr.Name
	}
	switch t := attr.Type.(type) {
	case *domainmodel.StringAttributeType:
		if t.Length > 0 {
			return fmt.Sprintf("%s: String(%d)", attr.Name, t.Length)
		}
		return attr.Name + ": String(unlimited)"
	case *domainmodel.EnumerationAttributeType:
		return attr.Name + ": " + shortName(t.EnumerationRef)
	default:
		return attr.Name + ": " + attr.Type.GetTypeName()
	}
}

// formatConstantTypeBrief formats a constant type for display.
func formatConstantTypeBrief(dt model.ConstantDataType) string {
	switch dt.Kind {
	case "Enumeration":
		if dt.EnumRef != "" {
			return shortName(dt.EnumRef)
		}
		return "Enumeration"
	default:
		return dt.Kind
	}
}

// shortName extracts the name part from a qualified name (Module.Name → Name).
func shortName(qualifiedName string) string {
	if idx := strings.LastIndex(qualifiedName, "."); idx >= 0 {
		return qualifiedName[idx+1:]
	}
	return qualifiedName
}

// shortWidgetType extracts a readable widget type from the full type string.
func shortWidgetType(widgetType string) string {
	// Widget types may look like "DataGrid", "DataView", "ListView", etc.
	// Or pluggable widgets like "com.mendix.widget.web.datagrid2.DataGrid2"
	if idx := strings.LastIndex(widgetType, "."); idx >= 0 {
		return widgetType[idx+1:]
	}
	return widgetType
}

// isDataWidget returns true if the widget type is a data-bound widget worth showing in structure.
func isDataWidget(widgetType string) bool {
	lower := strings.ToLower(widgetType)
	return strings.Contains(lower, "dataview") ||
		strings.Contains(lower, "datagrid") ||
		strings.Contains(lower, "listview") ||
		strings.Contains(lower, "templategrid") ||
		strings.Contains(lower, "gallery")
}

// escapeSQLString escapes single quotes in a string for SQL.
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// ============================================================================
// Sort Helpers
// ============================================================================

func sortEnumerations(enums []*model.Enumeration) {
	sort.Slice(enums, func(i, j int) bool {
		return strings.ToLower(enums[i].Name) < strings.ToLower(enums[j].Name)
	})
}

func sortMicroflows(mfs []*microflows.Microflow) {
	sort.Slice(mfs, func(i, j int) bool {
		return strings.ToLower(mfs[i].Name) < strings.ToLower(mfs[j].Name)
	})
}

func sortNanoflows(nfs []*microflows.Nanoflow) {
	sort.Slice(nfs, func(i, j int) bool {
		return strings.ToLower(nfs[i].Name) < strings.ToLower(nfs[j].Name)
	})
}

func sortConstants(consts []*model.Constant) {
	sort.Slice(consts, func(i, j int) bool {
		return strings.ToLower(consts[i].Name) < strings.ToLower(consts[j].Name)
	})
}

func sortScheduledEvents(events []*model.ScheduledEvent) {
	sort.Slice(events, func(i, j int) bool {
		return strings.ToLower(events[i].Name) < strings.ToLower(events[j].Name)
	})
}
