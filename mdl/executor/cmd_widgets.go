// SPDX-License-Identifier: Apache-2.0

// Package executor - Widget commands (SHOW WIDGETS, UPDATE WIDGETS)
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
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
		return writeEmptyResult(ctx, result.Columns, "No widgets found matching the criteria")
	}
	if ctx.Format == FormatJSON {
		return writeResult(ctx, &TableResult{Columns: result.Columns, Rows: result.Rows})
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
	var total updateOutcome
	for containerID, widgetRefs := range containers {
		outcome, err := updateWidgetsInContainer(ctx, containerID, widgetRefs, s.Assignments, s.DryRun)
		if err != nil {
			fmt.Fprintf(ctx.Output, "Warning: Failed to update widgets in %s: %v\n", containerID, err)
			continue
		}
		total.add(outcome)
	}

	verb := "Updated"
	if s.DryRun {
		verb = "[dry run] Would update"
	}
	fmt.Fprintf(ctx.Output, "\n%s %d widget(s)\n", verb, total.WidgetsChanged)
	// Name the two ways a matched widget is not an updated one, so a run that
	// changed less than it matched says so rather than rounding to the headline.
	if total.WidgetsUnchanged > 0 {
		fmt.Fprintf(ctx.Output, "%d widget(s) matched but had no property that could be set\n",
			total.WidgetsUnchanged)
	}
	if total.WidgetsMissing > 0 {
		fmt.Fprintf(ctx.Output, "%d widget(s) are in the catalog but not in the document — "+
			"run 'refresh catalog full force' and try again\n", total.WidgetsMissing)
	}

	if s.DryRun {
		fmt.Fprintln(ctx.Output, "\nRun without dry run to apply changes.")
		return nil
	}
	// The catalog note is only true when something changed; printing it after a
	// run that wrote nothing told the reader there were changes to pick up.
	if total.WidgetsChanged > 0 {
		fmt.Fprintln(ctx.Output, "\nNote: Run 'refresh catalog full force' to update the catalog with changes.")
	}
	// A statement that matched widgets and wrote none of them has not succeeded.
	// Reporting that as an error is what stops a script silently doing nothing —
	// the failure mode ako/mxcli#520 was filed for.
	if total.changedNothing() {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"no widget was updated: %d assignment(s) could not be applied. "+
				"A design property (Atlas styling) is not a pluggable widget property and "+
				"cannot be set this way — see `mxcli syntax page.styling`",
			len(total.Failures)))
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
	// Pages and snippets only. The catalog also records a layout's widgets
	// (ako/mxcli#1302), for the linter; writing to a layout through this
	// statement is a separate decision, and a read-side change must not make it.
	query.WriteString(`select Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType
	          from widgets where ContainerType in ('PAGE', 'SNIPPET')`)
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

// updateOutcome is what a run actually did, which is three numbers and not one.
//
// It used to be a single `updated` count that meant "widgets found", and was
// reported as "Updated N widget(s)" — so a run where every assignment was
// refused still claimed success, right after warning about each refusal
// (ako/mxcli#520). Splitting the outcome is the fix: a widget nothing could be
// written to is not updated, and a widget the document does not carry is neither
// updated nor a property failure.
type updateOutcome struct {
	WidgetsChanged   int      // at least one assignment landed
	WidgetsUnchanged int      // found in the document, nothing could be set
	WidgetsMissing   int      // listed by the catalog, absent from the document
	Failures         []string // one per refused assignment, "'prop' on widget"
}

func (o *updateOutcome) add(other updateOutcome) {
	o.WidgetsChanged += other.WidgetsChanged
	o.WidgetsUnchanged += other.WidgetsUnchanged
	o.WidgetsMissing += other.WidgetsMissing
	o.Failures = append(o.Failures, other.Failures...)
}

// changedNothing reports a run that matched widgets and wrote none of them.
// Distinct from an empty match, which is reported before we get here.
func (o *updateOutcome) changedNothing() bool {
	return o.WidgetsChanged == 0 && (o.WidgetsUnchanged > 0 || o.WidgetsMissing > 0)
}

// updateWidgetsInContainer updates widgets within a single page or snippet
// using the PageMutator backend (no direct BSON manipulation).
func updateWidgetsInContainer(ctx *ExecContext, containerID string, widgetRefs []widgetRef, assignments []ast.WidgetPropertyAssignment, dryRun bool) (updateOutcome, error) {
	var out updateOutcome
	if len(widgetRefs) == 0 {
		return out, nil
	}

	containerName := widgetRefs[0].ContainerName

	// Open the container (page, layout, or snippet) through the backend mutator.
	mutator, err := ctx.Backend.OpenPageForMutation(model.ID(containerID))
	if err != nil {
		return out, mdlerrors.NewBackend(fmt.Sprintf("open %s for mutation", containerName), err)
	}
	if mutator == nil {
		return out, mdlerrors.NewBackend(fmt.Sprintf("open %s for mutation", containerName),
			fmt.Errorf("backend returned nil mutator for %s", containerID))
	}

	// A dry run attempts the same assignments against a DISCARDABLE COPY of the
	// document, so the preview reports what would actually happen rather than
	// assuming every assignment lands. Without this the preview told the same
	// lie one step earlier — and the syntax help says to run it first, which is
	// exactly when a user is relying on it (ako/mxcli#520).
	//
	// Best-effort: a backend whose mutator offers no probe keeps the optimistic
	// preview it had, which is no worse than before.
	target := mutator
	if dryRun {
		if p, ok := mutator.(interface {
			Probe() (backend.PageMutator, error)
		}); ok {
			if probe, perr := p.Probe(); perr == nil && probe != nil {
				target = probe
			}
		}
	}

	for _, ref := range widgetRefs {
		// Verify the widget exists before attempting assignments.
		if !target.FindWidget(ref.Name) {
			fmt.Fprintf(ctx.Output, "  Warning: Widget %q not found in %s %s\n",
				ref.Name, mutator.ContainerType(), containerName)
			out.WidgetsMissing++
			continue
		}
		landed := 0
		for _, assignment := range assignments {
			err := target.SetWidgetProperty(ref.Name, assignment.PropertyPath, assignment.Value)
			if err != nil {
				out.Failures = append(out.Failures,
					fmt.Sprintf("'%s' on %s (%s) in %s: %v",
						assignment.PropertyPath, ref.Name, ref.WidgetType, containerName, err))
				verb := "Failed to set"
				if dryRun {
					verb = "Cannot set"
				}
				fmt.Fprintf(ctx.Output, "  Warning: %s '%s' on %s: %v\n",
					verb, assignment.PropertyPath, ref.Name, err)
				continue
			}
			landed++
			if dryRun {
				fmt.Fprintf(ctx.Output, "  Would set '%s' = %v on %s (%s) in %s\n",
					assignment.PropertyPath, assignment.Value, ref.Name, ref.WidgetType, containerName)
			}
		}
		// A widget nothing could be written to is not an updated widget. That
		// distinction is the whole of ako/mxcli#520.
		if landed > 0 {
			out.WidgetsChanged++
		} else {
			out.WidgetsUnchanged++
		}
	}

	// Persist only when something actually changed. Gating on the found-count
	// offered a write for a container whose every assignment was refused;
	// idempotent-write elision (ADR-0008) discarded the bytes, so the damage was
	// confined to the summary — but offering it at all is what produced the
	// summary.
	if !dryRun && out.WidgetsChanged > 0 {
		if err := mutator.Save(); err != nil {
			return out, mdlerrors.NewBackend(fmt.Sprintf("save %s", containerName), err)
		}
	}

	return out, nil
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
