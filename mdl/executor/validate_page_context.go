// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// validatePageContextTree checks that page-internal references are consistent:
//   - PARAMETER DataSource references match declared page/snippet parameters
//   - SELECTION DataSource references match a widget name declared in the same page
//   - Attribute bindings have an enclosing data container providing entity context
//
// This runs at check time (no MPR needed) and catches issues that would otherwise
// only surface as CE errors in Studio Pro.
func validatePageContextTree(params []ast.PageParameter, widgets []*ast.WidgetV3) []string {
	// Build param name set
	paramNames := make(map[string]bool, len(params))
	for _, p := range params {
		paramNames[p.Name] = true
	}

	// Collect all widget names (first pass) for SELECTION validation
	widgetNames := make(map[string]bool)
	collectWidgetNames(widgets, widgetNames)

	// Walk the widget tree with context tracking
	var errors []string
	walkWidgetsWithContext(widgets, paramNames, widgetNames, false, &errors)
	return errors
}

// collectWidgetNames recursively collects all widget names in the tree.
func collectWidgetNames(widgets []*ast.WidgetV3, names map[string]bool) {
	for _, w := range widgets {
		if w.Name != "" {
			names[w.Name] = true
		}
		collectWidgetNames(w.Children, names)
	}
}

// walkWidgetsWithContext validates each widget's DataSource and attribute bindings,
// tracking whether the current position is inside a data container (DataView,
// DataGrid, ListView, etc.) that provides entity context.
func walkWidgetsWithContext(widgets []*ast.WidgetV3, paramNames map[string]bool, widgetNames map[string]bool, hasEntityContext bool, errors *[]string) {
	for _, w := range widgets {
		ds := w.GetDataSource()
		childHasContext := hasEntityContext

		if ds != nil {
			switch ds.Type {
			case "parameter":
				// Strip leading $ if present
				paramRef := strings.TrimPrefix(ds.Reference, "$")
				if paramRef != "" && !paramNames[paramRef] {
					*errors = append(*errors,
						fmt.Sprintf("widget '%s': parameter DataSource references '$%s' but no such parameter is declared in Params", w.Name, paramRef))
				}
				childHasContext = true

			case "selection":
				if ds.Reference != "" && !widgetNames[ds.Reference] {
					*errors = append(*errors,
						fmt.Sprintf("widget '%s': selection DataSource references '%s' but no widget with that name exists on this page", w.Name, ds.Reference))
				}
				childHasContext = true

			case "database", "microflow", "nanoflow", "association":
				childHasContext = true
			}
		}

		// Check if this widget type is a data container that sets context
		widgetType := strings.ToLower(w.Type)
		switch widgetType {
		case "dataview", "datagrid", "listview", "gallery", "templateview":
			if ds == nil {
				// Data container without DataSource — context comes from enclosing container
				childHasContext = hasEntityContext
			}
		}

		// Validate attribute binding: needs entity context
		if attr := w.GetAttribute(); attr != "" {
			if !hasEntityContext {
				*errors = append(*errors,
					fmt.Sprintf("widget '%s': Attribute '%s' is bound but there is no enclosing data container providing entity context", w.Name, attr))
			}
		}

		// Recurse into children
		walkWidgetsWithContext(w.Children, paramNames, widgetNames, childHasContext, errors)
	}
}
