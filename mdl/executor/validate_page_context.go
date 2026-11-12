// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// validatePageContextTree checks that page-internal references are consistent:
//   - PARAMETER DataSource references match declared page/snippet parameters
//   - SELECTION DataSource references match a widget name declared in the same page
//   - Attribute bindings have an enclosing data container providing entity context
//
// This runs at check time (no MPR needed) and catches issues that would otherwise
// only surface as CE errors in Studio Pro.
func validatePageContextTree(ctx *ExecContext, params []ast.PageParameter, widgets []*ast.WidgetV3) []string {
	// Build param name set
	paramNames := make(map[string]bool, len(params))
	for _, p := range params {
		paramNames[p.Name] = true
	}

	// The names the page will hold are the names AFTER `use building block` /
	// `use fragment` expansion: a reference is not a widget, and a prefixed
	// copy is named by its prefix (ako/mxcli#559).
	built := expandUseRefsForCheck(ctx, widgets)

	// Collect all widget names (first pass) for SELECTION validation
	widgetNames := make(map[string]bool)
	collectWidgetNames(built, widgetNames)

	// Walk the widget tree with context tracking
	var errors []string
	errors = append(errors, checkDuplicateWidgetNames(built, pageContextWidgetRegistry(ctx))...)
	walkWidgetsWithContext(widgets, paramNames, widgetNames, false, pageContextWidgetRegistry(ctx), nil, &errors)
	return errors
}

// expandUseRefsForCheck returns the widget tree with each `use building block`
// and `use fragment` reference replaced by the widgets exec will write for it,
// prefix applied — the tree whose names Mendix will see. The input is not
// modified.
//
// The reference itself carries the BLOCK's name in Name, and counting that as a
// widget name refused four prefixed copies of one block as a CE0495 duplicate
// of 'Atlas_Web_Content.Card' while exec and mx check had none (ako/mxcli#559).
// Expanding instead of merely skipping keeps the real collision — the same
// block placed twice with no prefix — which exec refuses as a duplicate name.
//
// A reference that cannot be expanded here (no project, an unknown block, a
// fragment defined in the script being checked) contributes no names; a
// fragment's content-slot payload is kept, since those are page widgets
// whatever the fragment holds. Expansion errors are exec's to report.
func expandUseRefsForCheck(ctx *ExecContext, widgets []*ast.WidgetV3) []*ast.WidgetV3 {
	var pb *pageBuilder
	if ctx != nil && ctx.Connected() {
		pb = &pageBuilder{ctx: ctx, backend: ctx.Backend, fragments: ctx.Fragments}
	}
	var walk func(ws []*ast.WidgetV3) []*ast.WidgetV3
	walk = func(ws []*ast.WidgetV3) []*ast.WidgetV3 {
		var out []*ast.WidgetV3
		for _, w := range ws {
			if w == nil {
				continue
			}
			switch w.Type {
			case "USE_BUILDING_BLOCK", "USE_FRAGMENT":
				if pb != nil {
					if expanded, err := pb.expandIfFragment(w); err == nil {
						out = append(out, walk(expanded)...)
						continue
					}
				}
				out = append(out, walk(w.Children)...)
			case "SLOT":
				out = append(out, walk(w.Children)...)
			default:
				c := *w
				c.Children = walk(w.Children)
				out = append(out, &c)
			}
		}
		return out
	}
	return walk(widgets)
}

// pageContextWidgetRegistry returns the widget registry for the run, or nil.
// The duplicate-name rule needs it to tell an object-list item from a widget;
// every other check here is registry-free, and a nil ctx (unit tests) is fine.
func pageContextWidgetRegistry(ctx *ExecContext) *WidgetRegistry {
	if ctx == nil {
		return nil
	}
	return ctx.GetWidgetRegistry()
}

// widgetKindsWithoutStoredNames are the widget kinds Mendix stores with no Name
// at all, so MDL's name for one is mxcli's own — derived at describe time to give
// ALTER PAGE something to address.
//
// Measured on a stored page: Forms$LayoutGridRow and Forms$LayoutGridColumn carry
// no `Name` key. A DataGrid2 column is the same, and mxcli says so itself in
// MDL-WIDGET16 ("DataGrid 2 stores no column names"), deriving one from the bound
// attribute — so two columns over the same attribute derive the same name and no
// renaming fixes that without breaking the documented addressing.
//
// A name the model does not hold cannot be a CE0495 duplicate, and reporting one
// made DESCRIBE output fail mxcli's own check (upstream #978).
var widgetKindsWithoutStoredNames = map[string]bool{
	"row":    true,
	"column": true,
}

// unstoredContainerKinds returns the lowercased MDL container keywords the
// widget's definition declares as object lists (a BarChart's `series`, an
// Accordion's `group`) or as child slots (a Gallery's `template` and `filter`).
// Children with one of those types are ITEMS or SLOT BLOCKS, not widgets: an
// object-list item is a WidgetObject with no name, and applyChildSlots builds
// only a slot block's children into the slot property, discarding the block's
// own name. Either way the model holds no name for it — which is why DESCRIBE
// synthesises `series1` and `template1` — so it cannot be a CE0495 duplicate
// (upstream #978: four galleries described as `template template1 { … }`).
//
// Returns nil for anything that does not resolve — a built-in widget, an unknown
// name, or no registry at all — so the caller's behaviour is unchanged there.
func unstoredContainerKinds(registry *WidgetRegistry, w *ast.WidgetV3) map[string]bool {
	if registry == nil || w == nil {
		return nil
	}
	def := lookupWidgetDef(w, registry)
	if def == nil {
		return nil
	}
	out := make(map[string]bool, len(def.ObjectLists))
	for _, ol := range def.ObjectLists {
		if ol.MDLContainer != "" {
			out[strings.ToLower(ol.MDLContainer)] = true
		}
	}
	// WidgetMode carries no ObjectLists — object lists are declared once on the
	// definition — but it does carry ChildSlots. Every mode's slots count: which
	// mode applies depends on properties the builder evaluates, and a slot
	// keyword is a slot block in whichever mode declares it.
	addSlots := func(slots []ChildSlotMapping) {
		for _, s := range slots {
			if s.MDLContainer != "" {
				out[strings.ToLower(s.MDLContainer)] = true
			}
		}
	}
	addSlots(def.ChildSlots)
	for i := range def.Modes {
		addSlots(def.Modes[i].ChildSlots)
	}
	return out
}

// checkDuplicateWidgetNames flags any widget name that appears more than once on a
// page. Mendix requires widget names to be unique per page and rejects duplicates
// with CE0495 "Duplicate name" — which mxcli check otherwise passed (FINDINGS #15).
// Each duplicate name is reported once, in first-seen order.
//
// Widget kinds Mendix stores without a name are skipped: see
// widgetKindsWithoutStoredNames. So are OBJECT-LIST ITEMS — a chart `series`, a
// gallery `customitem` — for the same reason and with the same evidence: a page
// authored with `series sRegion (…)` comes back from DESCRIBE as
// `series series1 (…)`, because the stored WidgetObject carries no name and
// DESCRIBE has to synthesise one. Measured on mxbuild 11.6.6, three charts on
// one page each holding a `series s` is 0 errors; mxcli called it CE0495 and,
// since a reference error fails the run, refused to execute the script. And so
// are CHILD-SLOT BLOCKS — a gallery's `template` and `filter` — whose name
// applyChildSlots discards (see unstoredContainerKinds).
//
// registry may be nil (check runs with no project in CI). Then no parent
// resolves, itemKinds is empty everywhere, and the rule behaves as it did
// before — a false positive is preferable to a silent one, and the enumerated
// widget types are unaffected either way.
func checkDuplicateWidgetNames(widgets []*ast.WidgetV3, registry *WidgetRegistry) []string {
	counts := make(map[string]int)
	var order []string
	// itemKinds are the container keywords the ENCLOSING widget declares as
	// object lists. Read from that widget's definition rather than from a list
	// of keywords: the containers are def-driven, so a table here would be the
	// second list this proposal exists to remove.
	var walk func(ws []*ast.WidgetV3, itemKinds map[string]bool)
	walk = func(ws []*ast.WidgetV3, itemKinds map[string]bool) {
		for _, w := range ws {
			kind := strings.ToLower(w.Type)
			// `container <slotName> { … }` is the other spelling of a slot block:
			// applyChildSlots routes it by NAME, and stores no name for it either.
			slotByName := kind == "container" && itemKinds[strings.ToLower(w.Name)]
			if w.Name != "" && !widgetKindsWithoutStoredNames[kind] && !itemKinds[kind] && !slotByName {
				if counts[w.Name] == 0 {
					order = append(order, w.Name)
				}
				counts[w.Name]++
			}
			walk(w.Children, unstoredContainerKinds(registry, w))
		}
	}
	walk(widgets, nil)

	var errors []string
	for _, name := range order {
		if counts[name] > 1 {
			errors = append(errors,
				fmt.Sprintf("duplicate widget name '%s' (used %d times) — Mendix requires unique widget names per page (CE0495)", name, counts[name]))
		}
	}
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
//
// itemKinds are the container keywords the enclosing widget's definition
// declares (unstoredContainerKinds): a child of one of those types is an
// object-list ITEM or a slot block, not a widget, and its properties are the
// item's own. An item property may be named `attribute` without being an
// attribute binding — AccessibilityHelper's attributesList.attribute is the
// HTML attribute's name, a string — and reading it as one refused the
// describe output of every such widget (ako/mxcli#904). registry may be nil;
// then no item resolves and the rule behaves as before.
func walkWidgetsWithContext(widgets []*ast.WidgetV3, paramNames map[string]bool, widgetNames map[string]bool, hasEntityContext bool, registry *WidgetRegistry, itemKinds map[string]bool, errors *[]string) {
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
		if attr := w.GetAttribute(); attr != "" && !itemKinds[strings.ToLower(w.Type)] {
			// `$Param.Attr` names its object; validateNamedObjectBindings
			// judges the name.
			if _, _, named := namedObjectBinding(attr); !hasEntityContext && !named {
				*errors = append(*errors,
					fmt.Sprintf("widget '%s': Attribute '%s' is bound but there is no enclosing data container providing entity context", w.Name, attr))
			}
		}

		// Recurse into children
		walkWidgetsWithContext(w.Children, paramNames, widgetNames, childHasContext, registry, unstoredContainerKinds(registry, w), errors)
	}
}

// alwaysNamedKinds are the built-in widget kinds Mendix stores a Name for
// wherever they appear. A missing name on one is refused at check time with no
// project needed; the page builder refuses the rest (checkWidgetHasName), where
// the parent's widget definition is known.
var alwaysNamedKinds = map[string]bool{
	"textbox": true, "textarea": true, "datepicker": true, "dropdown": true, "combobox": true,
	"checkbox": true, "radiobuttons": true, "referenceselector": true, "filemanager": true, "actionbutton": true,
	"linkbutton": true, "title": true, "label": true, "dynamictext": true, "statictext": true,
	"snippetcall": true, "dataview": true, "listview": true, "datagrid": true, "gallery": true,
	"container": true, "customcontainer": true, "groupbox": true, "tabcontainer": true,
	"layoutgrid": true, "navigationlist": true, "staticimage": true, "dynamicimage": true,
	"image": true, "scrollcontainer": true, "navigationtree": true, "menubar": true,
	"simplemenubar": true, "sidebartoggle": true, "layoutheader": true, "textfilter": true, "numberfilter": true, "dropdownfilter": true,
	"datefilter": true, "dropdownsort": true,
}

// checkMissingWidgetNames refuses a widget written without a name where Mendix
// stores one (ako/mxcli#749). The name is optional in the grammar because a
// layout grid's rows and columns, a data grid's columns and control bar, and a
// gallery's template and filter have none; a `row` or `column` anywhere else
// is built as a container, which does.
func checkMissingWidgetNames(widgets []*ast.WidgetV3) []string {
	var errs []string
	var walk func(ws []*ast.WidgetV3, parent string)
	walk = func(ws []*ast.WidgetV3, parent string) {
		for _, w := range ws {
			kind := strings.ToLower(w.Type)
			if w.Name == "" && w.Specialization == "" && !w.TypeIsGeneric {
				nameless := false
				switch kind {
				case "row":
					nameless = parent == "layoutgrid"
				case "column":
					// A row's columns are layout grid columns, standalone row
					// or not (buildContainerWithRowV3).
					nameless = parent == "row" || parent == "datagrid"
				default:
					nameless = !alwaysNamedKinds[kind]
				}
				if !nameless {
					errs = append(errs, fmt.Sprintf(
						"%s needs a name: Mendix stores one for it (`%s %sName …`). Only a layout grid's rows and "+
							"columns, a data grid's columns and control bar, and a gallery's template and filter "+
							"are written without one (ako/mxcli#749)", kind, kind, kind))
				}
			}
			walk(w.Children, kind)
		}
	}
	walk(widgets, "")
	return errs
}

// widgetLabel names a widget in a message: "widget `name` (kind)", or "a kind"
// for an element Mendix stores no name for, such as a DataGrid 2 column (#749).
func widgetLabel(name, kind string) string {
	if name == "" {
		return "a " + strings.ToLower(kind)
	}
	return fmt.Sprintf("widget `%s` (%s)", name, kind)
}

// fileManagersWithoutContext refuses a file manager that sits in no data
// container (mendixlabs/mxcli#151). It uploads into the container's object, so
// with none there is nothing to store the file in: mxbuild CE6810 "A file
// manager must be placed in a data view or snippet that is connected to entity
// 'System.FileDocument' or a specialization", measured on 11.14.0. Whether the
// container's entity IS a file needs the project — buildFileManagerV3 checks
// that. topLevelHasContext is true for a snippet, whose object is decided where
// it is used.
func fileManagersWithoutContext(widgets []*ast.WidgetV3, topLevelHasContext bool) []string {
	var errs []string
	var walk func(ws []*ast.WidgetV3, hasContext bool)
	walk = func(ws []*ast.WidgetV3, hasContext bool) {
		for _, w := range ws {
			if w == nil {
				continue
			}
			if strings.EqualFold(w.Type, "filemanager") && !hasContext {
				errs = append(errs, fmt.Sprintf(
					"widget '%s': a file manager must be inside a data view (or snippet) whose object is a "+
						"System.FileDocument specialization — here it has no data container (mxbuild CE6810)", w.Name))
			}
			walk(w.Children, hasContext || w.GetDataSource() != nil)
		}
	}
	walk(widgets, topLevelHasContext)
	return errs
}
