// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// execDefineFragment stores a fragment definition in the executor's session state.
func execDefineFragment(ctx *ExecContext, s *ast.DefineFragmentStmt) error {
	if ctx.Fragments == nil {
		ctx.Fragments = make(map[string]*ast.DefineFragmentStmt)
	}
	if _, exists := ctx.Fragments[s.Name]; exists {
		return mdlerrors.NewAlreadyExists("fragment", s.Name)
	}
	ctx.Fragments[s.Name] = s
	fmt.Fprintf(ctx.Output, "Defined fragment %s (%d widgets)\n", s.Name, len(s.Widgets))
	return nil
}

// listFragments lists all defined fragments in the current session.
func listFragments(ctx *ExecContext) error {
	if len(ctx.Fragments) == 0 {
		fmt.Fprintln(ctx.Output, "No fragments defined.")
		return nil
	}

	// Sort by name for consistent output
	names := make([]string, 0, len(ctx.Fragments))
	for name := range ctx.Fragments {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Fprintf(ctx.Output, "%-30s %s\n", "Fragment", "Widgets")
	fmt.Fprintf(ctx.Output, "%-30s %s\n", strings.Repeat("-", 30), strings.Repeat("-", 10))
	for _, name := range names {
		frag := ctx.Fragments[name]
		fmt.Fprintf(ctx.Output, "%-30s %d\n", name, len(frag.Widgets))
	}
	return nil
}

// describeFragment outputs a fragment's definition as MDL.
func describeFragment(ctx *ExecContext, name ast.QualifiedName) error {
	if ctx.Fragments == nil {
		return mdlerrors.NewNotFound("fragment", name.Name)
	}
	frag, ok := ctx.Fragments[name.Name]
	if !ok {
		return mdlerrors.NewNotFound("fragment", name.Name)
	}

	fmt.Fprintf(ctx.Output, "define fragment %s as {\n", frag.Name)
	for _, w := range frag.Widgets {
		outputASTWidgetMDL(ctx.Output, w, 1)
	}
	fmt.Fprintln(ctx.Output, "};")
	return nil
}

// describeFragmentFrom handles DESCRIBE FRAGMENT FROM PAGE/SNIPPET ... WIDGET ... command.
// It finds a named widget in a page or snippet and outputs it as MDL.
func describeFragmentFrom(ctx *ExecContext, s *ast.DescribeFragmentFromStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	var rawWidgets []rawWidget

	switch s.ContainerType {
	case "page":
		allPages, err := ctx.Backend.ListPages()
		if err != nil {
			return mdlerrors.NewBackend("list pages", err)
		}
		var foundPage *pages.Page
		for _, p := range allPages {
			modID := h.FindModuleID(p.ContainerID)
			modName := h.GetModuleName(modID)
			if p.Name == s.ContainerName.Name && (s.ContainerName.Module == "" || modName == s.ContainerName.Module) {
				foundPage = p
				break
			}
		}
		if foundPage == nil {
			return mdlerrors.NewNotFound("page", s.ContainerName.String())
		}
		rawWidgets = getPageWidgetsFromRaw(ctx, foundPage.ID)

	case "snippet":
		allSnippets, err := ctx.Backend.ListSnippets()
		if err != nil {
			return mdlerrors.NewBackend("list snippets", err)
		}
		var foundSnippet *pages.Snippet
		for _, sn := range allSnippets {
			modID := h.FindModuleID(sn.ContainerID)
			modName := h.GetModuleName(modID)
			if sn.Name == s.ContainerName.Name && (s.ContainerName.Module == "" || modName == s.ContainerName.Module) {
				foundSnippet = sn
				break
			}
		}
		if foundSnippet == nil {
			return mdlerrors.NewNotFound("snippet", s.ContainerName.String())
		}
		rawWidgets = getSnippetWidgetsFromRaw(ctx, foundSnippet.ID)
	}

	// Find the widget by name
	target := findRawWidgetByName(rawWidgets, s.WidgetName)
	if target == nil {
		return mdlerrors.NewNotFoundMsg("widget", s.WidgetName, fmt.Sprintf("not found in %s %s", strings.ToLower(s.ContainerType), s.ContainerName.String()))
	}

	// Output as MDL
	outputWidgetMDLV3(ctx, *target, 0)
	return nil
}

// findRawWidgetByName recursively searches the widget tree for a widget with the given name.
func findRawWidgetByName(widgets []rawWidget, name string) *rawWidget {
	for i := range widgets {
		if widgets[i].Name == name {
			return &widgets[i]
		}
		// Search children
		if found := findRawWidgetByName(widgets[i].Children, name); found != nil {
			return found
		}
		// Search rows (for LayoutGrid)
		for _, row := range widgets[i].Rows {
			for _, col := range row.Columns {
				if found := findRawWidgetByName(col.Widgets, name); found != nil {
					return found
				}
			}
		}
		// Search filter/controlbar widgets
		if found := findRawWidgetByName(widgets[i].FilterWidgets, name); found != nil {
			return found
		}
		if found := findRawWidgetByName(widgets[i].ControlBar, name); found != nil {
			return found
		}
	}
	return nil
}

// outputASTWidgetMDL outputs an AST WidgetV3 as MDL text.
func outputASTWidgetMDL(w io.Writer, widget *ast.WidgetV3, indent int) {
	prefix := strings.Repeat("  ", indent)

	// Widget type and name
	fmt.Fprintf(w, "%s%s %s", prefix, widget.Type, mdlIdent(widget.Name))

	// Properties (excluding internal ones like Prefix)
	props := formatASTWidgetProps(widget)
	if props != "" {
		fmt.Fprintf(w, " (%s)", props)
	}

	// Children
	if len(widget.Children) > 0 {
		fmt.Fprintln(w, " {")
		for _, child := range widget.Children {
			outputASTWidgetMDL(w, child, indent+1)
		}
		fmt.Fprintf(w, "%s}\n", prefix)
	} else {
		fmt.Fprintln(w)
	}
}

// formatASTWidgetProps formats widget properties as "Key: Value, Key: Value".
func formatASTWidgetProps(w *ast.WidgetV3) string {
	if len(w.Properties) == 0 {
		return ""
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(w.Properties))
	for k := range w.Properties {
		if k == "Prefix" {
			continue // Internal property, not MDL
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if len(keys) == 0 {
		return ""
	}

	var parts []string
	for _, k := range keys {
		v := w.Properties[k]
		parts = append(parts, fmt.Sprintf("%s: %s", k, formatASTPropertyValue(v)))
	}
	return strings.Join(parts, ", ")
}

// formatASTPropertyValue formats a property value for MDL output.
func formatASTPropertyValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("'%s'", val)
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case *ast.DataSourceV3:
		return formatDataSourceV3(val)
	case *ast.ActionV3:
		return formatActionV3(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func formatDataSourceV3(ds *ast.DataSourceV3) string {
	switch ds.Type {
	case "parameter":
		return ds.Reference
	case "database":
		return "database " + ds.Reference
	case "microflow":
		return "microflow " + ds.Reference
	case "nanoflow":
		return "nanoflow " + ds.Reference
	case "association":
		return "association " + ds.Reference
	case "selection":
		return "selection " + mdlIdent(ds.Reference)
	default:
		return ds.Reference
	}
}

func formatActionV3(a *ast.ActionV3) string {
	switch a.Type {
	case "save":
		if a.ClosePage {
			return "save_changes close_page"
		}
		return "save_changes"
	case "cancel":
		if a.ClosePage {
			return "cancel_changes close_page"
		}
		return "cancel_changes"
	case "close":
		return "close_page"
	case "delete":
		return "delete_object"
	case "showPage":
		return "show_page " + a.Target
	case "microflow":
		return "microflow " + a.Target
	case "nanoflow":
		return "nanoflow " + a.Target
	case "signOut":
		return "sign_out"
	case "completeTask":
		return "complete_task '" + strings.ReplaceAll(a.OutcomeValue, "'", "''") + "'"
	default:
		return a.Type
	}
}
