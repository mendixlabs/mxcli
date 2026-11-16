// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/wfnames"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
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
	var qerr error
	entityCounts := queryCountByModule(ctx, "entities", &qerr)
	mfCounts := queryCountByModule(ctx, flowTypeFilter(catalog.MicroflowTypeMicroflow), &qerr)
	nfCounts := queryCountByModule(ctx, flowTypeFilter(catalog.MicroflowTypeNanoflow), &qerr)
	pageCounts := queryCountByModule(ctx, "pages", &qerr)
	enumCounts := queryCountByModule(ctx, "enumerations", &qerr)
	snippetCounts := queryCountByModule(ctx, "snippets", &qerr)
	jaCounts := queryCountByModule(ctx, "java_actions", &qerr)
	wfCounts := queryCountByModule(ctx, "workflows", &qerr)
	odataClientCounts := queryCountByModule(ctx, "odata_clients", &qerr)
	odataServiceCounts := queryCountByModule(ctx, "odata_services", &qerr)
	beServiceCounts := queryCountByModule(ctx, "business_event_services", &qerr)
	if qerr != nil {
		return qerr
	}
	constantCounts := countByModuleFromBackend(ctx, "constants")
	scheduledEventCounts := countByModuleFromBackend(ctx, "scheduled_events")
	queueCounts := countByModuleFromBackend(ctx, "queues")
	regexCounts := countByModuleFromBackend(ctx, "regular_expressions")

	tr := &TableResult{
		Columns: []string{
			"Module", "Entities", "Enumerations", "Microflows", "Nanoflows",
			"Workflows", "Pages", "Snippets", "JavaActions", "Constants",
			"ScheduledEvents", "Queues", "RegularExpressions", "ODataClients", "ODataServices", "BusinessEventServices",
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
			queueCounts[m.Name],
			regexCounts[m.Name],
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
	result, err := ctx.Catalog.Query("select Id, Name, Source, AppStoreGuid from modules ORDER by Name")
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
	var qerr error
	// Query counts per module from catalog
	entityCounts := queryCountByModule(ctx, "entities", &qerr)
	mfCounts := queryCountByModule(ctx, flowTypeFilter(catalog.MicroflowTypeMicroflow), &qerr)
	nfCounts := queryCountByModule(ctx, flowTypeFilter(catalog.MicroflowTypeNanoflow), &qerr)
	pageCounts := queryCountByModule(ctx, "pages", &qerr)
	enumCounts := queryCountByModule(ctx, "enumerations", &qerr)
	snippetCounts := queryCountByModule(ctx, "snippets", &qerr)
	jaCounts := queryCountByModule(ctx, "java_actions", &qerr)
	wfCounts := queryCountByModule(ctx, "workflows", &qerr)
	odataClientCounts := queryCountByModule(ctx, "odata_clients", &qerr)
	odataServiceCounts := queryCountByModule(ctx, "odata_services", &qerr)
	beServiceCounts := queryCountByModule(ctx, "business_event_services", &qerr)
	if qerr != nil {
		return qerr
	}

	// Get constants and scheduled events from backend (no catalog tables)
	constantCounts := countByModuleFromBackend(ctx, "constants")
	scheduledEventCounts := countByModuleFromBackend(ctx, "scheduled_events")
	queueCounts := countByModuleFromBackend(ctx, "queues")
	regexCounts := countByModuleFromBackend(ctx, "regular_expressions")

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
		if c := queueCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "queue", "queues"))
		}
		if c := regexCounts[m.Name]; c > 0 {
			parts = append(parts, pluralize(c, "regular expression", "regular expressions"))
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

// structureQuery runs one of the structure overview's catalog queries. They are
// fixed SQL against tables the catalog builder owns, so a failure is a column
// or table drift inside mxcli, never a property of the user's project. They
// used to discard the error, which is how depth 2 and 3 queried a ParentWidget
// column widgets_data has never had and printed every page without its data
// widgets, from the initial commit on.
func structureQuery(ctx *ExecContext, sql string) (*catalog.QueryResult, error) {
	result, err := ctx.Catalog.Query(sql)
	if err != nil {
		return nil, mdlerrors.NewBackend("run structure query "+sql, err)
	}
	return result, nil
}

// queryCountByModule queries a catalog table and returns a map of module name →
// count. A failing query sets *errp (if not already set) and yields no counts,
// so a caller can issue its whole batch and check once.
func queryCountByModule(ctx *ExecContext, tableAndWhere string, errp *error) map[string]int {
	counts := make(map[string]int)
	sql := fmt.Sprintf("select ModuleName, count(*) from %s GROUP by ModuleName", tableAndWhere)
	result, err := structureQuery(ctx, sql)
	if err != nil {
		if *errp == nil {
			*errp = err
		}
		return counts
	}
	for _, row := range result.Rows {
		name := asString(row[0])
		counts[name] = toInt(row[1])
	}
	return counts
}

// flowTypeFilter selects one flow flavour from the microflows table, spelled
// with the catalog builder's own constant: a hand-typed lower-case literal here
// matched no row and hid every microflow and nanoflow count.
func flowTypeFilter(flowType string) string {
	return fmt.Sprintf("microflows where MicroflowType = '%s'", flowType)
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
	case "queues":
		if queues, err := ctx.Backend.ListQueues(); err == nil {
			for _, q := range queues {
				modID := h.FindModuleID(q.ContainerID)
				modName := h.GetModuleName(modID)
				counts[modName]++
			}
		}
	case "regular_expressions":
		if res, err := ctx.Backend.ListRegularExpressions(); err == nil {
			for _, re := range res {
				modID := h.FindModuleID(re.ContainerID)
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

	allQueues, _ := ctx.Backend.ListQueues()
	queuesByModule := make(map[string][]*types.Queue)
	for _, q := range allQueues {
		modID := h.FindModuleID(q.ContainerID)
		modName := h.GetModuleName(modID)
		queuesByModule[modName] = append(queuesByModule[modName], q)
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
		if err := writeStructureModuleNested(ctx, m.Name, func() error {

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
			if err := structurePages(ctx, m.Name); err != nil {
				return err
			}

			// Snippets (from catalog)
			if err := structureSnippets(ctx, m.Name); err != nil {
				return err
			}

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

			// Task Queues
			if queues, ok := queuesByModule[m.Name]; ok {
				sort.Slice(queues, func(i, j int) bool { return queues[i].Name < queues[j].Name })
				for _, q := range queues {
					fmt.Fprintf(ctx.Output, "  Queue %s.%s\n", m.Name, q.Name)
				}
			}

			// OData Clients
			if err := structureODataClients(ctx, m.Name); err != nil {
				return err
			}

			// OData Services
			if err := structureODataServices(ctx, m.Name); err != nil {
				return err
			}

			// Business Event Services
			if err := structureBusinessEventServices(ctx, m.Name); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
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

	allQueues, _ := ctx.Backend.ListQueues()
	queuesByModule := make(map[string][]*types.Queue)
	for _, q := range allQueues {
		modID := h.FindModuleID(q.ContainerID)
		modName := h.GetModuleName(modID)
		queuesByModule[modName] = append(queuesByModule[modName], q)
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
		if err := writeStructureModuleNested(ctx, m.Name, func() error {

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
			if err := structurePages(ctx, m.Name); err != nil {
				return err
			}

			// Snippets
			if err := structureSnippets(ctx, m.Name); err != nil {
				return err
			}

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

			// Task Queues
			if queues, ok := queuesByModule[m.Name]; ok {
				sort.Slice(queues, func(i, j int) bool { return queues[i].Name < queues[j].Name })
				for _, q := range queues {
					fmt.Fprintf(ctx.Output, "  Queue %s.%s\n", m.Name, q.Name)
				}
			}

			// OData
			if err := structureODataClients(ctx, m.Name); err != nil {
				return err
			}
			if err := structureODataServices(ctx, m.Name); err != nil {
				return err
			}

			// Business Event Services
			if err := structureBusinessEventServices(ctx, m.Name); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
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
						part += " cascade"
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

// structurePages outputs pages for a module from the catalog, each annotated
// with its outermost data widgets — `Page M.P [DataView<Customer>, Datagrid<Order>]`.
//
// "Outermost" means no data widget above it in the page's widget tree: those are
// the widgets that establish a data context. A data widget inside a layout grid
// or container still counts (they are not data contexts), and one nested inside
// another data widget does not (it is a detail of its parent). Filtering on the
// page root instead would drop nearly everything, since real pages wrap their
// content in a layout grid.
//
// widgets_data is only filled by a full catalog build, so on a fast-mode catalog
// every page prints bare.
func structurePages(ctx *ExecContext, moduleName string) error {
	result, err := structureQuery(ctx, fmt.Sprintf(
		"select Name from pages where ModuleName = '%s' ORDER by Name",
		escapeSQLString(moduleName)))
	if err != nil {
		return err
	}
	if len(result.Rows) == 0 {
		return nil
	}

	widgetResult, err := structureQuery(ctx, fmt.Sprintf(
		"select Id, ParentWidgetId, ContainerQualifiedName, WidgetType, EntityRef from widgets "+
			"where ModuleName = '%s' and ContainerType = 'PAGE' ORDER by ContainerQualifiedName, WidgetType, Id",
		escapeSQLString(moduleName)))
	if err != nil {
		return err
	}
	type pageWidget struct {
		parent, page, widgetType, entity string
	}
	byID := make(map[string]pageWidget, len(widgetResult.Rows))
	var order []string
	for _, row := range widgetResult.Rows {
		id := asString(row[0])
		byID[id] = pageWidget{parent: asString(row[1]), page: asString(row[2]), widgetType: asString(row[3]), entity: asString(row[4])}
		order = append(order, id)
	}
	hasDataAncestor := func(w pageWidget) bool {
		// ParentWidgetId chains end at the page root (""); the visited set only
		// guards against a malformed cycle.
		seen := map[string]bool{}
		for p := w.parent; p != "" && !seen[p]; p = byID[p].parent {
			seen[p] = true
			if isDataWidget(byID[p].widgetType) {
				return true
			}
		}
		return false
	}

	widgetsByPage := make(map[string][]string)
	for _, id := range order {
		w := byID[id]
		if !isDataWidget(w.widgetType) || hasDataAncestor(w) {
			continue
		}
		label := shortWidgetType(w.widgetType)
		if w.entity != "" {
			label += "<" + shortName(w.entity) + ">"
		}
		widgetsByPage[w.page] = append(widgetsByPage[w.page], label)
	}

	for _, row := range result.Rows {
		qualName := moduleName + "." + asString(row[0])
		if widgets := widgetsByPage[qualName]; len(widgets) > 0 {
			fmt.Fprintf(ctx.Output, "  Page %s [%s]\n", qualName, strings.Join(widgets, ", "))
		} else {
			fmt.Fprintf(ctx.Output, "  Page %s\n", qualName)
		}
	}
	return nil
}

// structureSnippets outputs snippets for a module from the catalog.
func structureSnippets(ctx *ExecContext, moduleName string) error {
	result, err := structureQuery(ctx, fmt.Sprintf(
		"select Name from snippets where ModuleName = '%s' ORDER by Name",
		escapeSQLString(moduleName)))
	if err != nil {
		return err
	}

	for _, row := range result.Rows {
		name := asString(row[0])
		fmt.Fprintf(ctx.Output, "  Snippet %s.%s\n", moduleName, name)
	}
	return nil
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
func structureODataClients(ctx *ExecContext, moduleName string) error {
	result, err := structureQuery(ctx, fmt.Sprintf(
		"select Name, ODataVersion from consumed_odata_services where ModuleName = '%s' ORDER by Name",
		escapeSQLString(moduleName)))
	if err != nil {
		return err
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
	return nil
}

// structureODataServices outputs OData services for a module.
func structureODataServices(ctx *ExecContext, moduleName string) error {
	result, err := structureQuery(ctx, fmt.Sprintf(
		"select Name, Path, EntitySetCount from published_odata_services where ModuleName = '%s' ORDER by Name",
		escapeSQLString(moduleName)))
	if err != nil {
		return err
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
	return nil
}

// structureBusinessEventServices outputs business event services for a module.
func structureBusinessEventServices(ctx *ExecContext, moduleName string) error {
	result, err := structureQuery(ctx, fmt.Sprintf(
		"select Name, MessageCount, PublishCount, SubscribeCount from business_event_services where ModuleName = '%s' ORDER by Name",
		escapeSQLString(moduleName)))
	if err != nil {
		return err
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
	return nil
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

// countStructureWorkflowActivities counts activity types in a workflow for
// structure output, over every flow it holds — the catalog's walk (wfnames).
func countStructureWorkflowActivities(wf *workflows.Workflow) (total, userTasks, microflowCalls, decisions int) {
	c := wfnames.CountActivities(wf)
	return c.Total, c.UserTasks, c.MicroflowCalls, c.Decisions
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
	// Built-in widgets are stored as "Forms$DataView", "Forms$ListView", …
	if idx := strings.LastIndexAny(widgetType, ".$"); idx >= 0 {
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
