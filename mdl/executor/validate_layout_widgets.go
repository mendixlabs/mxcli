// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// validateLayoutWidgets refuses the widget placements mxbuild rejects in a
// layout (and a sidebar toggle anywhere but one), which passed check and exec
// and failed a build later (ako/mxcli#2461). Measured with mx check 11.12.6;
// each layout was alone in the project, so no page binding is involved:
//
//	dataview in a Responsive, Tablet, ModalPopup layout
//	                          → CE6086 "Data views are not allowed on web layouts."
//	dataview in a Default (native) layout
//	                          → CE6086 "Data views are not allowed on native layouts."
//	region with ToggleMode other than None, no sidebartoggle in the layout
//	                          → CE0611 "A sidebar toggle button is required if a
//	                            region can toggle." (a toggle inside a snippet
//	                            the layout calls does not count)
//	sidebartoggle in a layout whose top level is anything but one scroll
//	container with a toggling region (a container around it, the toggle beside
//	it, no region that toggles)
//	                          → CE0628 "For the sidebar toggle button to work, the
//	                            outermost widget on the layout must be a scroll
//	                            container with a region that can be toggled."
//	sidebartoggle on a page or snippet
//	                          → CE6086 "Sidebar toggles are not allowed on web
//	                            pages." / "… on web snippets."
//
// Every rule reads the statement alone, so this runs in the project-free pass.
// A layout built on another one (`Layout:`) is checked only for data views: its
// widgets sit in the outer layout's placeholders, and where the outermost
// widget is then was not measured.
func validateLayoutWidgets(stmt ast.Statement) []linter.Violation {
	switch s := stmt.(type) {
	case *ast.CreateLayoutStmt:
		return validateLayoutStmtWidgets(s)
	case *ast.CreatePageStmtV3:
		return sidebarTogglesOutsideLayout("page", s.Name, allPageWidgets(s))
	case *ast.CreateSnippetStmtV3:
		return sidebarTogglesOutsideLayout("snippet", s.Name, s.Widgets)
	}
	return nil
}

func validateLayoutStmtWidgets(layout *ast.CreateLayoutStmt) []linter.Violation {
	qn := layout.Name.String()
	loc := linter.Location{Module: layout.Name.Module, DocumentType: "layout", DocumentName: layout.Name.Name}
	var out []linter.Violation
	platform := "web"
	t := (&ast.WidgetV3{Properties: layout.Properties}).GetStringProp("LayoutType")
	if pages.ValidLayoutType(pages.LayoutType(t), true) {
		platform = "native"
	}
	for _, w := range layoutWidgetsOfKind(layout.AllWidgets(), "dataview", nil) {
		out = append(out, linter.Violation{
			RuleID:   "MDL-LAYOUT01",
			Severity: linter.SeverityError,
			Location: loc,
			Message: fmt.Sprintf(
				"layout '%s': data view %s is not allowed in a layout, so mxbuild fails it with "+
					"CE6086 (\"Data views are not allowed on %s layouts.\")", qn, quoteWidgetName(w), platform),
			Suggestion: "put the data view on the page that uses this layout, inside its `placeholder Main { … }`; " +
				"a layout frames pages and has no object of its own.",
		})
	}
	if platform == "native" || layout.OuterLayout() != "" {
		return out
	}

	toggles := layoutWidgetsOfKind(layout.Widgets, "sidebartoggle", nil)
	var togglingRegions []string
	for _, sc := range layoutWidgetsOfKind(layout.Widgets, "scrollcontainer", nil) {
		for _, r := range togglingRegionsOf(sc) {
			togglingRegions = append(togglingRegions, fmt.Sprintf("%s of %s", r, quoteWidgetName(sc)))
		}
	}
	if len(toggles) == 0 && len(togglingRegions) > 0 {
		out = append(out, linter.Violation{
			RuleID:   "MDL-LAYOUT02",
			Severity: linter.SeverityError,
			Location: loc,
			Message: fmt.Sprintf(
				"layout '%s': region %s can toggle but the layout has no sidebartoggle, so mxbuild fails it "+
					"with CE0611 (\"A sidebar toggle button is required if a region can toggle.\")",
				qn, strings.Join(togglingRegions, ", ")),
			Suggestion: "add `sidebartoggle <name> (Caption: 'Menu')` to a region of the scroll container " +
				"(a toggle inside a called snippet does not count), or set the region's `ToggleMode: 'None'`.",
		})
	}
	if len(toggles) > 0 && !outermostIsTogglingScrollContainer(layout.Widgets) {
		out = append(out, linter.Violation{
			RuleID:   "MDL-LAYOUT03",
			Severity: linter.SeverityError,
			Location: loc,
			Message: fmt.Sprintf(
				"layout '%s': sidebar toggle %s needs the layout's only top-level widget to be a scroll container "+
					"with a region that can toggle, so mxbuild fails it with CE0628 (\"For the sidebar toggle button "+
					"to work, the outermost widget on the layout must be a scroll container with a region that can be toggled.\")",
				qn, quoteWidgetName(toggles[0])),
			Suggestion: "make `scrollcontainer` the layout's one top-level widget, put the toggle in one of its regions " +
				"(`region top { sidebartoggle … }`), and give the side region a `ToggleMode` other than 'None'.",
		})
	}
	return out
}

// sidebarTogglesOutsideLayout refuses a sidebar toggle on a page or snippet.
func sidebarTogglesOutsideLayout(kind string, name ast.QualifiedName, widgets []*ast.WidgetV3) []linter.Violation {
	toggles := layoutWidgetsOfKind(widgets, "sidebartoggle", nil)
	if len(toggles) == 0 {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-LAYOUT04",
		Severity: linter.SeverityError,
		Location: linter.Location{Module: name.Module, DocumentType: kind, DocumentName: name.Name},
		Message: fmt.Sprintf(
			"%s '%s': sidebar toggle %s is allowed only in a layout, so mxbuild fails it with CE6086 "+
				"(\"Sidebar toggles are not allowed on web %ss.\")", kind, name.String(), quoteWidgetName(toggles[0]), kind),
		Suggestion: "put the sidebartoggle in the layout, in a region of the scroll container whose side region toggles.",
	}}
}

// layoutWidgetsOfKind collects the widgets of a kind anywhere in the tree.
func layoutWidgetsOfKind(ws []*ast.WidgetV3, kind string, acc []*ast.WidgetV3) []*ast.WidgetV3 {
	for _, w := range ws {
		if w == nil {
			continue
		}
		if strings.EqualFold(w.Type, kind) {
			acc = append(acc, w)
		}
		acc = layoutWidgetsOfKind(w.Children, kind, acc)
	}
	return acc
}

// togglingRegionsOf names the regions of a scroll container whose ToggleMode
// is stated and is not None.
func togglingRegionsOf(sc *ast.WidgetV3) []string {
	var out []string
	for _, r := range sc.Children {
		if r == nil || !strings.EqualFold(r.Type, "region") {
			continue
		}
		if mode := r.GetStringProp("ToggleMode"); mode != "" && !strings.EqualFold(mode, "None") {
			out = append(out, r.Name)
		}
	}
	return out
}

func outermostIsTogglingScrollContainer(top []*ast.WidgetV3) bool {
	var real []*ast.WidgetV3
	for _, w := range top {
		if w != nil {
			real = append(real, w)
		}
	}
	return len(real) == 1 && strings.EqualFold(real[0].Type, "scrollcontainer") && len(togglingRegionsOf(real[0])) > 0
}

func quoteWidgetName(w *ast.WidgetV3) string {
	if w.Name == "" {
		return "(unnamed)"
	}
	return "'" + w.Name + "'"
}
