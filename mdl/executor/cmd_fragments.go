// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
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

	params := ""
	if len(frag.Params) > 0 {
		parts := make([]string, 0, len(frag.Params))
		for _, p := range frag.Params {
			parts = append(parts, "$"+p.Name+": "+p.Kind)
		}
		params = " (" + strings.Join(parts, ", ") + ")"
	}
	fmt.Fprintf(ctx.Output, "create fragment %s%s as {\n", frag.Name, params)
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

	// The visitor stores ContainerType uppercase ("PAGE"/"SNIPPET"), as it
	// does for DESCRIBE/ALTER STYLING and ALTER PAGE. Normalise here, and make
	// an unrecognised value an error: falling through the switch leaves no
	// widgets, which then reads as "widget not found" — the disguise this
	// casing mismatch wore here and in cmd_styling.go before it.
	containerType := strings.ToLower(s.ContainerType)
	switch containerType {
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

	default:
		return mdlerrors.NewUnsupported("describe fragment from: unsupported container type " + s.ContainerType)
	}

	// Find the widget by name
	target := findRawWidgetByName(rawWidgets, s.WidgetName)
	if target == nil {
		return mdlerrors.NewNotFoundMsg("widget", s.WidgetName, fmt.Sprintf("widget %s not found in %s %s", s.WidgetName, containerType, s.ContainerName.String()))
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
		return quoteExpressionLiteral(val)
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
		return formatActionV3(val) + formatFlowReturnsV3(val.Returns) + formatActionSettingsV3(val.Settings)
	case *ast.WidgetExpressionV3:
		return val.Text
	default:
		return fmt.Sprintf("%v", val)
	}
}

// databaseSourceFrom is what follows `database from`: the entity, or the
// `$ctx/Assoc/Entity` path of a source reached over an association
// (ako/mxcli#721 L5).
func databaseSourceFrom(ds *ast.DataSourceV3) string {
	if ds.AssociationPath != "" {
		return "$" + ds.ContextVariable + "/" + ds.AssociationPath
	}
	return ds.Reference
}

func formatDataSourceV3(ds *ast.DataSourceV3) string {
	switch ds.Type {
	case "parameter":
		return ds.Reference
	case "database":
		return "database " + databaseSourceFrom(ds)
	case "microflow":
		if ds.FormValidations != "" {
			return "microflow " + ds.Reference + formatFlowArgsV3(ds.Args) + " with (FormValidations: " + ds.FormValidations + ")"
		}
		return "microflow " + ds.Reference + formatFlowArgsV3(ds.Args)
	case "nanoflow":
		return "nanoflow " + ds.Reference + formatFlowArgsV3(ds.Args)
	case "association":
		return "association " + ds.Reference
	case "selection":
		return "selection " + mdlIdent(ds.Reference)
	default:
		return ds.Reference
	}
}

// formatFlowArgsV3 renders a flow or page call's arguments as written,
// `(Name = value, …)`, or "" when there are none.
func formatFlowArgsV3(args []ast.FlowArgV3) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, visitor.ParameterNameSpelling(arg.Name)+" = "+fmt.Sprint(arg.Value))
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func formatActionV3(a *ast.ActionV3) string {
	switch a.Type {
	case "none":
		return "nothing"
	case "save":
		if a.ClosePage {
			return "save changes close page"
		}
		return "save changes"
	case "cancel":
		if a.ClosePage {
			return "cancel changes close page"
		}
		return "cancel changes"
	case "close":
		return "close page"
	case "delete":
		out := "delete"
		if a.SourceVariable != "" {
			out += " $" + a.SourceVariable
		}
		if a.ClosePage {
			out += " close page"
		}
		return out
	case "param":
		return "$" + a.Target
	case "create":
		if a.ThenAction != nil {
			return "create object " + a.Target + " then " + formatActionV3(a.ThenAction)
		}
		return "create object " + a.Target
	case "showPage":
		return "show page " + a.Target + formatFlowArgsV3(a.Args)
	case "microflow":
		return "call microflow " + a.Target + formatFlowArgsV3(a.Args)
	case "nanoflow":
		return "call nanoflow " + a.Target + formatFlowArgsV3(a.Args)
	case "openLink":
		if a.LinkVariable != "" {
			return "open link " + a.LinkVariable + "/" + a.LinkAttribute
		}
		return "open link " + quoteExpressionLiteral(a.LinkURL)
	case "signOut":
		return "sign out"
	case "completeTask":
		return "complete task '" + strings.ReplaceAll(a.OutcomeValue, "'", "''") + "'"
	case "openUserTask":
		return "open user task"
	case "openWorkflow":
		return "open workflow"
	case "callWorkflow":
		return "call workflow " + a.Target
	default:
		return a.Type
	}
}
