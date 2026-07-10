// SPDX-License-Identifier: Apache-2.0

// Package executor - Widget commands (SHOW WIDGETS, UPDATE WIDGETS)
package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// execShowWidgets handles the SHOW WIDGETS statement.
func execShowWidgets(ctx *ExecContext, s *ast.ShowWidgetsStmt) error {

	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Ensure catalog is built (full mode for widgets)
	if err := ensureCatalog(ctx, true); err != nil {
		return mdlerrors.NewBackend("build catalog", err)
	}

	// Build SQL query from filters
	var query strings.Builder
	query.WriteString("select Name, WidgetType, ContainerQualifiedName, ModuleName from widgets where 1=1")
	args := []any{}

	for _, f := range s.Filters {
		col := mapWidgetFilterField(f.Field)
		if strings.EqualFold(f.Operator, "like") {
			query.WriteString(fmt.Sprintf(" and %s like ?", col))
		} else {
			query.WriteString(fmt.Sprintf(" and %s = ?", col))
		}
		args = append(args, f.Value)
	}

	if s.InModule != "" {
		query.WriteString(" and ModuleName = ?")
		args = append(args, s.InModule)
	}

	query.WriteString(" ORDER by ModuleName, ContainerQualifiedName, Name")

	// Execute query using SQLite parameterization
	result, err := executeCatalogQueryWithArgs(ctx, query.String(), args...)
	if err != nil {
		return mdlerrors.NewBackend("query widgets", err)
	}

	// Output results as table
	if result.Count == 0 {
		fmt.Fprintln(ctx.Output, "No widgets found matching the criteria")
		return nil
	}

	// Print header
	fmt.Fprintf(ctx.Output, "\n%-30s %-40s %-40s %-20s\n",
		"NAME", "widget type", "container", "module")
	fmt.Fprintln(ctx.Output, strings.Repeat("-", 130))

	// Print rows
	for _, row := range result.Rows {
		name := formatCell(row[0], 30)
		widgetType := formatCell(row[1], 40)
		container := formatCell(row[2], 40)
		module := formatCell(row[3], 20)
		fmt.Fprintf(ctx.Output, "%-30s %-40s %-40s %-20s\n", name, widgetType, container, module)
	}

	fmt.Fprintf(ctx.Output, "\n%d widget(s) found\n", result.Count)
	return nil
}

// execUpdateWidgets handles the UPDATE WIDGETS statement.
func execUpdateWidgets(ctx *ExecContext, s *ast.UpdateWidgetsStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Ensure catalog is built (full mode for widgets)
	if err := ensureCatalog(ctx, true); err != nil {
		return mdlerrors.NewBackend("build catalog", err)
	}

	// Find matching widgets
	widgets, err := findMatchingWidgets(ctx, s.Filters, s.InModule)
	if err != nil {
		return mdlerrors.NewBackend("find widgets", err)
	}

	if len(widgets) == 0 {
		fmt.Fprintln(ctx.Output, "No widgets found matching the criteria")
		return nil
	}

	// Group widgets by container
	containers := groupWidgetsByContainer(widgets)

	// Report what will be updated
	fmt.Fprintf(ctx.Output, "\nFound %d widget(s) in %d container(s) matching the criteria\n",
		len(widgets), len(containers))

	if s.DryRun {
		fmt.Fprintln(ctx.Output, "\n[dry run] The following changes would be made:")
	}

	// Process each container
	totalUpdated := 0
	for containerID, widgetRefs := range containers {
		updated, err := updateWidgetsInContainer(ctx, containerID, widgetRefs, s.Assignments, s.DryRun)
		if err != nil {
			fmt.Fprintf(ctx.Output, "Warning: Failed to update widgets in %s: %v\n", containerID, err)
			continue
		}
		totalUpdated += updated
	}

	if s.DryRun {
		fmt.Fprintf(ctx.Output, "\n[dry run] Would update %d widget(s)\n", totalUpdated)
		fmt.Fprintln(ctx.Output, "\nRun without dry run to apply changes.")
	} else {
		fmt.Fprintf(ctx.Output, "\nUpdated %d widget(s)\n", totalUpdated)
		fmt.Fprintln(ctx.Output, "\nNote: Run 'refresh catalog full force' to update the catalog with changes.")
	}

	return nil
}

// widgetRef holds information about a widget to be updated.
type widgetRef struct {
	ID            string
	Name          string
	WidgetType    string
	ContainerID   string
	ContainerName string
	ContainerType string // "page" or "snippet"
}

// findMatchingWidgets queries the catalog for widgets matching the filters.
func findMatchingWidgets(ctx *ExecContext, filters []ast.WidgetFilter, module string) ([]widgetRef, error) {
	var query strings.Builder
	query.WriteString(`select Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType
	          from widgets where 1=1`)
	args := []any{}

	for _, f := range filters {
		col := mapWidgetFilterField(f.Field)
		if strings.EqualFold(f.Operator, "like") {
			query.WriteString(fmt.Sprintf(" and %s like ?", col))
		} else {
			query.WriteString(fmt.Sprintf(" and %s = ?", col))
		}
		args = append(args, f.Value)
	}

	if module != "" {
		query.WriteString(" and ModuleName = ?")
		args = append(args, module)
	}

	result, err := executeCatalogQueryWithArgs(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}

	widgets := make([]widgetRef, 0, result.Count)
	for _, row := range result.Rows {
		widgets = append(widgets, widgetRef{
			ID:            fmt.Sprintf("%v", row[0]),
			Name:          fmt.Sprintf("%v", row[1]),
			WidgetType:    fmt.Sprintf("%v", row[2]),
			ContainerID:   fmt.Sprintf("%v", row[3]),
			ContainerName: fmt.Sprintf("%v", row[4]),
			ContainerType: fmt.Sprintf("%v", row[5]),
		})
	}

	return widgets, nil
}

// groupWidgetsByContainer groups widgets by their container ID.
func groupWidgetsByContainer(widgets []widgetRef) map[string][]widgetRef {
	containers := make(map[string][]widgetRef)
	for _, w := range widgets {
		containers[w.ContainerID] = append(containers[w.ContainerID], w)
	}
	return containers
}

// updateWidgetsInContainer updates widgets within a single page or snippet
// using the PageMutator backend (no direct BSON manipulation).
func updateWidgetsInContainer(ctx *ExecContext, containerID string, widgetRefs []widgetRef, assignments []ast.WidgetPropertyAssignment, dryRun bool) (int, error) {
	if len(widgetRefs) == 0 {
		return 0, nil
	}

	containerName := widgetRefs[0].ContainerName

	// Open the container (page, layout, or snippet) through the backend mutator.
	mutator, err := ctx.Backend.OpenPageForMutation(model.ID(containerID))
	if err != nil {
		return 0, mdlerrors.NewBackend(fmt.Sprintf("open %s for mutation", containerName), err)
	}
	if mutator == nil {
		return 0, mdlerrors.NewBackend(fmt.Sprintf("open %s for mutation", containerName),
			fmt.Errorf("backend returned nil mutator for %s", containerID))
	}

	updated := 0
	for _, ref := range widgetRefs {
		// Verify the widget exists before attempting assignments.
		if !mutator.FindWidget(ref.Name) {
			fmt.Fprintf(ctx.Output, "  Warning: Widget %q not found in %s %s\n",
				ref.Name, mutator.ContainerType(), containerName)
			continue
		}
		for _, assignment := range assignments {
			if dryRun {
				fmt.Fprintf(ctx.Output, "  Would set '%s' = %v on %s (%s) in %s\n",
					assignment.PropertyPath, assignment.Value, ref.Name, ref.WidgetType, containerName)
			} else {
				if err := mutator.SetWidgetProperty(ref.Name, assignment.PropertyPath, assignment.Value); err != nil {
					fmt.Fprintf(ctx.Output, "  Warning: Failed to set '%s' on %s: %v\n",
						assignment.PropertyPath, ref.Name, err)
				}
			}
		}
		updated++
	}

	// Persist changes via the mutator.
	if !dryRun && updated > 0 {
		if err := mutator.Save(); err != nil {
			return updated, mdlerrors.NewBackend(fmt.Sprintf("save %s", containerName), err)
		}
	}

	return updated, nil
}

// mapWidgetFilterField maps user-facing field names to catalog column names.
func mapWidgetFilterField(field string) string {
	switch strings.ToLower(field) {
	case "widgettype":
		return "WidgetType"
	case "name":
		return "Name"
	case "container":
		return "ContainerQualifiedName"
	case "module":
		return "ModuleName"
	default:
		return field
	}
}

// executeCatalogQueryWithArgs executes a parameterized SQL query against the catalog.
func executeCatalogQueryWithArgs(ctx *ExecContext, query string, args ...any) (*catalogQueryResult, error) {
	// Replace ? placeholders with values for SQLite
	// Note: This is a simplified implementation; production code should use prepared statements
	finalQuery := query
	for _, arg := range args {
		// Escape single quotes in string values
		strVal := fmt.Sprintf("%v", arg)
		strVal = strings.ReplaceAll(strVal, "'", "''")
		finalQuery = strings.Replace(finalQuery, "?", fmt.Sprintf("'%s'", strVal), 1)
	}

	result, err := ctx.Catalog.Query(finalQuery)
	if err != nil {
		return nil, err
	}

	return &catalogQueryResult{
		Columns: result.Columns,
		Rows:    result.Rows,
		Count:   result.Count,
	}, nil
}

// catalogQueryResult wraps catalog query results.
type catalogQueryResult struct {
	Columns []string
	Rows    [][]any
	Count   int
}

// formatCell formats a cell value for display, truncating if needed.
func formatCell(val any, maxLen int) string {
	s := fmt.Sprintf("%v", val)
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
