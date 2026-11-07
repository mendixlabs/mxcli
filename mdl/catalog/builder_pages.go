// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"encoding/base64"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func (b *Builder) buildPages() error {
	// Get all pages (cached — reused by buildReferences and buildStrings)
	pageList, err := b.cachedPages()
	if err != nil {
		return err
	}

	pageStmt, err := b.tx.Prepare(`
		INSERT INTO pages_data (Id, Name, QualifiedName, ModuleName, Folder, Title, URL, LayoutRef,
			Description, ParameterCount, WidgetCount, Excluded, Class, Style, DynamicClasses,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer pageStmt.Close()

	// Prepare widget statement only in full mode
	var widgetStmt *Stmt
	if b.fullMode {
		widgetStmt, err = b.tx.Prepare(`
			INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType,
				ModuleName, Folder, EntityRef, AttributeRef, MicroflowRef, NanoflowRef, PageRef, Description,
				ParentWidgetId, Depth, Class, Style, DynamicClasses, ActionType, HasConfirmation,
				ProjectId, SnapshotId)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer widgetStmt.Close()
	}

	projectID, snapshotID := b.snapshotMeta()

	pageCount := 0
	widgetCount := 0

	for _, pg := range pageList {
		if !b.own(string(pg.ID)) {
			continue
		}
		// Get module name
		moduleID := b.hierarchy.findModuleID(pg.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + pg.Name
		folder := b.hierarchy.buildFolderPath(pg.ContainerID)

		// The title in the project's default language. Ranging over the
		// translation map and taking the first entry made the column vary
		// between builds of an unchanged project (mendixlabs/mxcli#1262).
		title := ""
		if pg.Title != nil {
			title = pickTranslation(pg.Title.Translations, b.defaultLanguage())
		}

		// Get layout ref and widgets from raw BSON data
		layoutRef := ""
		widgetCnt := 0
		var rawWidgets []rawWidgetInfo
		var class, style, dynamicClasses string

		if b.fullMode {
			rawData, _ := b.reader.GetRawUnit(pg.ID)
			if rawData != nil {
				layoutRef = extractLayoutRef(rawData)
				rawWidgets = extractPageWidgets(rawData, string(pg.ID))
				widgetCnt = len(rawWidgets)
				class, style, dynamicClasses = widgetAppearance(rawData)
			}
		}

		_, err = pageStmt.Exec(
			string(pg.ID),
			pg.Name,
			qualifiedName,
			moduleName,
			folder,
			title,
			pg.URL,
			layoutRef,
			pg.Documentation,
			len(pg.Parameters),
			widgetCnt,
			pg.Excluded,
			class, style, dynamicClasses,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
		pageCount++

		// Insert widgets only in full mode
		if b.fullMode && len(rawWidgets) > 0 {
			for _, w := range rawWidgets {
				if _, err := widgetStmt.Exec(
					w.ID,
					w.Name,
					w.WidgetType,
					string(pg.ID),
					qualifiedName,
					"PAGE",
					moduleName,
					folder,
					w.EntityRef,
					w.AttributeRef,
					w.MicroflowRef,
					w.NanoflowRef,
					w.PageRef,
					"",
					w.ParentID, w.Depth, w.Class, w.Style, w.DynamicClasses,
					w.ActionType, w.HasConfirmation,
					projectID, snapshotID,
				); err != nil {
					return fmt.Errorf("insert widget %s for page %s: %w", w.Name, qualifiedName, err)
				}
				widgetCount++
				b.collectWidgetExtraRefs("PAGE", qualifiedName, moduleName, w)
			}
		}
	}
	b.disown()

	b.report("Pages", pageCount)
	if b.fullMode {
		b.report("Widgets", widgetCount)
	}
	return nil
}

func (b *Builder) buildSnippets() error {
	// Get all snippets
	snippetList, err := b.lister().ListSnippets()
	if err != nil {
		return err
	}

	snippetStmt, err := b.tx.Prepare(`
		INSERT INTO snippets_data (Id, Name, QualifiedName, ModuleName, Folder, Description,
			ParameterCount, WidgetCount,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer snippetStmt.Close()

	// Snippet widgets feed the same widgets_data table as page widgets (full mode
	// only), with ContainerType=SNIPPET, so the refs projection in buildReferences
	// emits snippet → entity/microflow/nanoflow edges automatically.
	var widgetStmt *Stmt
	if b.fullMode {
		widgetStmt, err = b.tx.Prepare(`
			INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType,
				ModuleName, Folder, EntityRef, AttributeRef, MicroflowRef, NanoflowRef, PageRef, Description,
				ParentWidgetId, Depth, Class, Style, DynamicClasses, ActionType, HasConfirmation,
				ProjectId, SnapshotId)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer widgetStmt.Close()
	}

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	widgetCount := 0
	for _, sn := range snippetList {
		if !b.own(string(sn.ID)) {
			continue
		}
		// Get module name
		moduleID := b.hierarchy.findModuleID(sn.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + sn.Name
		folder := b.hierarchy.buildFolderPath(sn.ContainerID)

		var rawWidgets []rawWidgetInfo
		if b.fullMode {
			if rawData, _ := b.reader.GetRawUnit(sn.ID); rawData != nil {
				rawWidgets = extractSnippetWidgets(rawData)
			}
		}

		if _, err := snippetStmt.Exec(
			string(sn.ID),
			sn.Name,
			qualifiedName,
			moduleName,
			folder,
			sn.Documentation,
			0, // ParameterCount
			len(rawWidgets),
			projectID, snapshotID,
		); err != nil {
			return err
		}
		count++

		if b.fullMode {
			for _, w := range rawWidgets {
				if _, err := widgetStmt.Exec(
					w.ID, w.Name, w.WidgetType,
					string(sn.ID), qualifiedName, "SNIPPET",
					moduleName, folder,
					w.EntityRef, w.AttributeRef, w.MicroflowRef, w.NanoflowRef, w.PageRef, "",
					w.ParentID, w.Depth, w.Class, w.Style, w.DynamicClasses,
					w.ActionType, w.HasConfirmation,
					projectID, snapshotID,
				); err != nil {
					return fmt.Errorf("insert widget %s for snippet %s: %w", w.Name, qualifiedName, err)
				}
				widgetCount++
				b.collectWidgetExtraRefs("SNIPPET", qualifiedName, moduleName, w)
			}
		}
	}
	b.disown()

	b.report("Snippets", count)
	if b.fullMode {
		b.report("Snippet widgets", widgetCount)
	}
	return nil
}

// buildBuildingBlocks populates building_blocks_data. Building blocks are
// read-only copy-templates (deep-copied onto pages with no live link), so —
// unlike snippets — their widgets are NOT emitted into widgets_data for
// cross-reference; only a WidgetCount is recorded. The widget count is read from
// the raw unit's top-level Widgets array (same shape as a snippet), so it is
// only available in full mode.
func (b *Builder) buildBuildingBlocks() error {
	blocks, err := b.lister().ListBuildingBlocks()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO building_blocks_data (Id, Name, QualifiedName, ModuleName, Folder, Description,
			DisplayName, Platform, Category, WidgetCount,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, bb := range blocks {
		if !b.own(string(bb.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(bb.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + bb.Name
		folder := b.hierarchy.buildFolderPath(bb.ContainerID)

		widgetCount := 0
		if b.fullMode {
			if rawData, _ := b.reader.GetRawUnit(bb.ID); rawData != nil {
				widgetCount = len(extractSnippetWidgets(rawData))
			}
		}

		if _, err := stmt.Exec(
			string(bb.ID),
			bb.Name,
			qualifiedName,
			moduleName,
			folder,
			bb.Documentation,
			bb.DisplayName,
			bb.Platform,
			bb.TemplateCategory,
			widgetCount,
			projectID, snapshotID,
		); err != nil {
			return err
		}
		count++
	}
	b.disown()

	b.report("Building blocks", count)
	return nil
}

// rawWidgetInfo represents a widget extracted from raw BSON data.
type rawWidgetInfo struct {
	ID           string
	Name         string
	WidgetType   string
	EntityRef    string // datasource entity (DomainModels$DirectEntityRef.Entity)
	AttributeRef string
	MicroflowRef string // action/datasource microflow (Forms$MicroflowSettings.Microflow, …)
	NanoflowRef  string // action/datasource nanoflow
	PageRef      string // action page (Forms$PageSettings.Form) — see scanWidgetOwnRefs

	// AllRefs is every document the widget names in its own BSON. The four
	// single columns above hold one of each kind (the smallest, see
	// scanWidgetOwnRefs) for `show widgets` and Starlark; a widget can name
	// more — a data grid 2 with a microflow datasource AND an on-click
	// microflow — and the refs table needs all of them (mendixlabs/mxcli#1513).
	AllRefs widgetOwnRefs

	// Tree position (mendixlabs/mxcli#1268). ParentID is the nearest INDEXED
	// ancestor, so a wrapper the walk skips (the synthetic
	// conditionalVisibilityWidget DivContainer) and the non-widget holders in
	// between (a layout grid's rows and columns, a tab page, a pluggable
	// widget's property or object-list item) are transparent: a widget in a
	// layout grid column or a data grid 2 column is parented to the grid.
	// Depth is the number of indexed ancestors — 0 at the page or snippet root.
	// A list view template IS indexed (issue #940), so it is a level of its
	// own: list view d, template d+1, the template's widgets d+2. The walk
	// does not enter a snippet call, so depth restarts at 0 inside the snippet.
	ParentID string
	Depth    int

	// Appearance (Forms$Appearance — the only home of these since Mendix 8,
	// pluggable widgets included).
	Class          string
	Style          string
	DynamicClasses string

	// ActionType is the stored $Type of the widget's primary action, read from
	// the first of types.WidgetClickActionKeys the widget holds. Raw, e.g.
	// "Forms$DeleteClientAction", "Forms$MicroflowAction", "Forms$NoAction".
	// HasConfirmation reports a ConfirmationInfo on that action; only
	// microflow, nanoflow and workflow calls can carry one — a delete action
	// has no confirmation setting in the model at all.
	ActionType      string
	HasConfirmation bool
}

// widgetPrimaryAction returns the $Type of a widget's primary action and whether
// that action asks for confirmation. A microflow call keeps its confirmation in
// MicroflowSettings.ConfirmationInfo; nanoflow and workflow calls hold
// ConfirmationInfo directly. Unset, the property is stored as null.
func widgetPrimaryAction(w map[string]any) (actionType string, hasConfirmation bool) {
	for _, key := range types.WidgetClickActionKeys {
		action, ok := w[key].(map[string]any)
		if !ok {
			continue
		}
		actionType = extractString(action["$Type"])
		if actionType == "" {
			continue
		}
		if _, ok := action["ConfirmationInfo"].(map[string]any); ok {
			hasConfirmation = true
		} else if settings, ok := action["MicroflowSettings"].(map[string]any); ok {
			_, hasConfirmation = settings["ConfirmationInfo"].(map[string]any)
		}
		return actionType, hasConfirmation
	}
	return "", false
}

// widgetAppearance reads a widget's Class, Style and DynamicClasses.
func widgetAppearance(w map[string]any) (class, style, dynamicClasses string) {
	appearance, ok := w["Appearance"].(map[string]any)
	if !ok {
		return "", "", ""
	}
	return extractString(appearance["Class"]), extractString(appearance["Style"]),
		extractString(appearance["DynamicClasses"])
}

// widgetChildKeys are the keys under which a widget nests *other* widgets. The
// per-widget ref scan skips them so a container doesn't absorb its children's
// references (each child is visited separately by extractWidgetsRecursive).
var widgetChildKeys = map[string]bool{
	"Widgets": true, "Rows": true, "FooterWidgets": true, "TabPages": true,
	// A List View's specialization templates. Missing here, the list view's own
	// scan descended into them and collected each template's Entity — then
	// returned the lexicographically smallest candidate, so a specialization
	// sorting before the list view's real datasource silently replaced it. The
	// widgets table recorded the wrong entity for the list view, decided by
	// alphabetical accident. Each template is indexed as a container in its own
	// right instead (see extractWidgetsRecursive). Issue #940.
	"Templates": true,
}

// widgetOwnRefs is every entity, microflow, nanoflow and page a widget names in
// its own (non-child-widget) BSON, each list sorted and without duplicates.
type widgetOwnRefs struct {
	Entities, Microflows, Nanoflows, Pages []string
}

// scanWidgetOwnRefs returns ONE entity/microflow/nanoflow/page per kind from a
// widget's own BSON: the lexicographically smallest, so the result is
// deterministic regardless of BSON map iteration order. This is what the single
// widgets_data columns (EntityRef, MicroflowRef, NanoflowRef, PageRef) show —
// `show widgets` and Starlark's microflow_ref. It is NOT complete: a widget can
// name two microflows (a datasource and an on-click), and the one not picked
// here would have no reference at all if the refs table were built from these
// columns alone — QUAL004 then called a live datasource microflow unused
// (mendixlabs/mxcli#1513). The refs table takes every name, from
// scanWidgetAllRefs.
func scanWidgetOwnRefs(w map[string]any) (entity, microflow, nanoflow, page string) {
	all := scanWidgetAllRefs(w)
	first := func(xs []string) string {
		if len(xs) == 0 {
			return ""
		}
		return xs[0]
	}
	return first(all.Entities), first(all.Microflows), first(all.Nanoflows), first(all.Pages)
}

// scanWidgetAllRefs collects every entity/microflow/nanoflow/page a widget
// references in its own (non-child-widget) BSON — datasource entity, datasource
// microflow, on-click microflow, etc.
func scanWidgetAllRefs(w map[string]any) widgetOwnRefs {
	var ents, mfs, nfs, forms []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if s, ok := x["Entity"].(string); ok && s != "" {
				ents = append(ents, s)
			}
			if s, ok := x["Microflow"].(string); ok && s != "" {
				mfs = append(mfs, s)
			}
			if s, ok := x["Nanoflow"].(string); ok && s != "" {
				nfs = append(nfs, s)
			}
			// A page reference is stored under "Form" — Mendix's original word for
			// a page, the same rename behind ShowFormAction/CloseFormAction. Not
			// collecting it is why an action button that opens a page created no
			// reference, so the page reported "(no callers found)" however many
			// buttons pointed at it (issue #773).
			if s, ok := x["Form"].(string); ok && s != "" {
				forms = append(forms, s)
			}
			for k, val := range x {
				if widgetChildKeys[k] {
					continue
				}
				walk(val)
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(w)
	return widgetOwnRefs{
		Entities:   sortedUnique(ents),
		Microflows: sortedUnique(mfs),
		Nanoflows:  sortedUnique(nfs),
		Pages:      sortedUnique(forms),
	}
}

// sortedUnique sorts xs and drops repeats; nil stays nil.
func sortedUnique(xs []string) []string {
	if len(xs) == 0 {
		return nil
	}
	sort.Strings(xs)
	out := xs[:1]
	for _, s := range xs[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

// extractLayoutRef extracts the layout reference from raw page BSON. Regular
// pages (Forms$Form) carry the layout under FormCall; page templates
// (Forms$PageTemplate) carry it under LayoutCall instead. Both nest the same
// Form (string qualified name) / Layout (binary GUID) fields, so try FormCall
// first and fall back to LayoutCall — without the fallback every page template
// reports no layout (e.g. all 46 Atlas_Web_Content templates).
func extractLayoutRef(rawData map[string]any) string {
	call, ok := rawData["FormCall"].(map[string]any)
	if !ok {
		call, ok = rawData["LayoutCall"].(map[string]any)
	}
	if !ok {
		return ""
	}

	// Try Form field first (string layout name)
	if formName, ok := call["Form"].(string); ok && formName != "" {
		return formName
	}

	// Try Layout field (binary GUID) - extract and format
	if layoutID := extractBinaryID(call["Layout"]); layoutID != "" {
		return layoutID // Will be a GUID string
	}

	return ""
}

// extractPageWidgets extracts all widgets from raw page BSON data.
func extractPageWidgets(rawData map[string]any, containerID string) []rawWidgetInfo {
	formCall, ok := rawData["FormCall"].(map[string]any)
	if !ok {
		return nil
	}

	// Get Arguments array
	args := getBsonArrayElements(formCall["Arguments"])
	if args == nil {
		return nil
	}

	var widgets []rawWidgetInfo
	for _, arg := range args {
		argMap, ok := arg.(map[string]any)
		if !ok {
			continue
		}
		// Each argument has a Widgets array
		argWidgets := getBsonArrayElements(argMap["Widgets"])
		for _, w := range argWidgets {
			if wMap, ok := w.(map[string]any); ok {
				widgets = append(widgets, extractWidgetsRecursive(wMap)...)
			}
		}
	}
	return widgets
}

// isTransparentDivWrapper reports whether w is the synthetic
// "conditionalVisibilityWidget*" DivContainer that mxcli / Studio Pro insert as a
// layout placeholder. Such a wrapper carries no user styling and is not an
// authored widget, so the catalog omits it (its children are still indexed) —
// matching DESCRIBE PAGE's isConditionalVisibilityWrapper. A DivContainer that is
// named differently, or that carries any appearance, is a real user container and
// IS indexed.
func isTransparentDivWrapper(w map[string]any, widgetType string) bool {
	if widgetType != "Forms$DivContainer" && widgetType != "Pages$DivContainer" {
		return false
	}
	if !strings.HasPrefix(extractString(w["Name"]), "conditionalVisibilityWidget") {
		return false
	}
	// A styled wrapper is a real container the user cares about, not a placeholder.
	appearance, _ := w["Appearance"].(map[string]any)
	if appearance == nil {
		return true
	}
	if extractString(appearance["Class"]) != "" ||
		extractString(appearance["Style"]) != "" ||
		extractString(appearance["DynamicClasses"]) != "" {
		return false
	}
	return len(getBsonArrayElements(appearance["DesignProperties"])) == 0
}

// extractWidgetsRecursive recursively extracts widgets from a widget map that
// sits at the root of its page or snippet.
func extractWidgetsRecursive(w map[string]any) []rawWidgetInfo {
	return walkWidget(w, "", 0)
}

// walkWidget extracts w and every widget nested in it. parentID and depth are
// those w gets if it is indexed — the nearest indexed ancestor and the number
// of indexed ancestors — and are passed through unchanged to its children when
// it is not.
func walkWidget(w map[string]any, parentID string, depth int) []rawWidgetInfo {
	var result []rawWidgetInfo

	// Extract this widget's info
	widget := rawWidgetInfo{
		ID:         extractBsonID(w["$ID"]),
		Name:       extractString(w["Name"]),
		WidgetType: extractString(w["$Type"]),
	}

	// Handle CustomWidgets - get the actual widget type ID
	if widget.WidgetType == "CustomWidgets$CustomWidget" {
		if typeObj, ok := w["Type"].(map[string]any); ok {
			if widgetID := extractString(typeObj["WidgetId"]); widgetID != "" {
				widget.WidgetType = widgetID
			}
		}
	}

	// Extract attribute reference for input widgets
	if attrRef, ok := w["AttributeRef"].(map[string]any); ok {
		widget.AttributeRef = extractString(attrRef["Attribute"])
	}

	// Extract datasource entity + action microflow/nanoflow references from this
	// widget's own content (not its child widgets).
	widget.EntityRef, widget.MicroflowRef, widget.NanoflowRef, widget.PageRef = scanWidgetOwnRefs(w)
	widget.AllRefs = scanWidgetAllRefs(w)
	widget.Class, widget.Style, widget.DynamicClasses = widgetAppearance(w)
	widget.ActionType, widget.HasConfirmation = widgetPrimaryAction(w)
	widget.ParentID, widget.Depth = parentID, depth

	// Index user-authored containers, but skip the synthetic
	// "conditionalVisibilityWidget*" wrapper that mxcli / Studio Pro insert as a
	// transparent layout placeholder (mirrors DESCRIBE's isConditionalVisibilityWrapper).
	// Previously ALL DivContainers were dropped, so real `container` widgets —
	// which carry Class/Style/DynamicClasses/DesignProperties and OnClick — were
	// invisible to `show widgets` and untargetable by `update widgets`.
	childParent, childDepth := parentID, depth
	if !isTransparentDivWrapper(w, widget.WidgetType) {
		result = append(result, widget)
		childParent, childDepth = widget.ID, depth+1
	}

	// Recurse into child widgets
	childWidgets := getBsonArrayElements(w["Widgets"])
	for _, child := range childWidgets {
		if childMap, ok := child.(map[string]any); ok {
			result = append(result, walkWidget(childMap, childParent, childDepth)...)
		}
	}

	// Handle LayoutGrid rows/columns
	rows := getBsonArrayElements(w["Rows"])
	for _, row := range rows {
		if rowMap, ok := row.(map[string]any); ok {
			cols := getBsonArrayElements(rowMap["Columns"])
			for _, col := range cols {
				if colMap, ok := col.(map[string]any); ok {
					colWidgets := getBsonArrayElements(colMap["Widgets"])
					for _, cw := range colWidgets {
						if cwMap, ok := cw.(map[string]any); ok {
							result = append(result, walkWidget(cwMap, childParent, childDepth)...)
						}
					}
				}
			}
		}
	}

	// Handle List View specialization templates. A template is not a widget, but
	// it is a container that carries its own entity — the specialization it
	// renders — so it is walked as one: the row it produces holds that entity,
	// and its children are indexed like any other nested widgets.
	//
	// Without this, every widget inside a template was absent from the index, and
	// because the refs projection is built from the widgets table, an entity used
	// only inside a template reported 0 pages and 0 widgets — so anything using
	// reference counts to decide "unused, safe to delete" would delete a document
	// in active use. Issue #940.
	for _, tpl := range getBsonArrayElements(w["Templates"]) {
		if tplMap, ok := tpl.(map[string]any); ok {
			result = append(result, walkWidget(tplMap, childParent, childDepth)...)
		}
	}

	// Handle DataView/ListView footer widgets
	footerWidgets := getBsonArrayElements(w["FooterWidgets"])
	for _, fw := range footerWidgets {
		if fwMap, ok := fw.(map[string]any); ok {
			result = append(result, walkWidget(fwMap, childParent, childDepth)...)
		}
	}

	// Handle TabContainer tab pages
	tabPages := getBsonArrayElements(w["TabPages"])
	for _, tp := range tabPages {
		if tpMap, ok := tp.(map[string]any); ok {
			tpWidgets := getBsonArrayElements(tpMap["Widgets"])
			for _, tw := range tpWidgets {
				if twMap, ok := tw.(map[string]any); ok {
					result = append(result, walkWidget(twMap, childParent, childDepth)...)
				}
			}
		}
	}

	// Handle CustomWidget nested widgets in properties — both kinds of container.
	if obj, ok := w["Object"].(map[string]any); ok {
		result = append(result, widgetsInPropertyBag(obj, childParent, childDepth)...)
	}

	// Handle NavigationList items
	items := getBsonArrayElements(w["Items"])
	for _, item := range items {
		if itemMap, ok := item.(map[string]any); ok {
			itemWidgets := getBsonArrayElements(itemMap["Widgets"])
			for _, iw := range itemWidgets {
				if iwMap, ok := iw.(map[string]any); ok {
					result = append(result, walkWidget(iwMap, childParent, childDepth)...)
				}
			}
		}
	}

	// Handle the containers that keep their widgets under other keys: table
	// cells, a header's and a split pane's sides, a scroll container's regions.
	// Each was skipped, so everything inside was absent from the index while the
	// XPath walk, which descends into every key, named it (ako/mxcli#1895).
	var parts []map[string]any
	for _, cell := range getBsonArrayElements(w["Cells"]) {
		if cellMap, ok := cell.(map[string]any); ok {
			parts = append(parts, cellMap)
		}
	}
	if widget.WidgetType == "Forms$ScrollContainer" {
		for _, key := range []string{"CenterRegion", "Top", "Left", "Right", "Bottom"} {
			if region, ok := w[key].(map[string]any); ok {
				parts = append(parts, region)
			}
		}
	}
	for _, part := range parts {
		for _, pw := range partWidgets(part, "Widgets", "Widget") {
			result = append(result, walkWidget(pw, childParent, childDepth)...)
		}
	}
	for _, side := range [][2]string{{"LeftWidgets", "LeftWidget"}, {"RightWidgets", "RightWidget"},
		{"FirstWidgets", "FirstWidget"}, {"SecondWidgets", "SecondWidget"}} {
		for _, sw := range partWidgets(w, side[0], side[1]) {
			result = append(result, walkWidget(sw, childParent, childDepth)...)
		}
	}

	return result
}

// partWidgets returns the widgets m holds under a list key or, in older models,
// a single-widget key.
func partWidgets(m map[string]any, listKey, singleKey string) []map[string]any {
	var out []map[string]any
	for _, v := range getBsonArrayElements(m[listKey]) {
		if vm, ok := v.(map[string]any); ok {
			out = append(out, vm)
		}
	}
	if single, ok := m[singleKey].(map[string]any); ok {
		out = append(out, single)
	}
	return out
}

// widgetsInPropertyBag walks a pluggable widget's stored property bag and
// indexes every widget inside it, through BOTH kinds of container:
//
//	Value.Widgets   a child slot   — a Gallery's content, an HTML Element's body
//	Value.Objects   an object list — a DataGrid2 column, a chart series
//
// Only the first was walked, so anything placed in a column, a gallery item or
// a series was invisible to the catalog. Measured on a real project: a page
// holding 19 chart sparklines inside datagrid columns did not appear under
// "which pages use VegaChart?", while the grid around them did.
//
// The consequence is wider than the widget edge, because CATALOG.REFS is a
// projection of this table: an entity or microflow used ONLY inside a column
// template reported zero references, so anything using reference counts to
// decide "unused, safe to delete" would delete a document in active use. That is
// issue #940's failure mode — fixed for List View templates, left open here.
//
// An object-list item is itself a property bag, so the walk recurses: a column
// holding a nested widget that has its own object list is covered without a
// second case.
//
// parentID and depth belong to the pluggable widget's children: a property and
// an object-list item are not widgets, so a widget in a column is parented to
// the grid.
func widgetsInPropertyBag(bag map[string]any, parentID string, depth int) []rawWidgetInfo {
	var result []rawWidgetInfo
	for _, prop := range getBsonArrayElements(bag["Properties"]) {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		for _, pw := range getBsonArrayElements(value["Widgets"]) {
			if pwMap, ok := pw.(map[string]any); ok {
				result = append(result, walkWidget(pwMap, parentID, depth)...)
			}
		}
		for _, obj := range getBsonArrayElements(value["Objects"]) {
			if objMap, ok := obj.(map[string]any); ok {
				result = append(result, widgetsInPropertyBag(objMap, parentID, depth)...)
			}
		}
	}
	return result
}

// extractSnippetWidgets extracts all widgets from raw snippet BSON data.
func extractSnippetWidgets(rawData map[string]any) []rawWidgetInfo {
	// Handle both snippet formats:
	// - Studio Pro uses "Widgets" (plural): a top-level array of widgets
	// - mxcli uses "Widget" (singular): a single container whose "Widgets" field holds children
	var widgetsArray []any
	if wa := getBsonArrayElements(rawData["Widgets"]); wa != nil {
		widgetsArray = wa
	} else if widgetContainer, ok := rawData["Widget"].(map[string]any); ok {
		widgetsArray = getBsonArrayElements(widgetContainer["Widgets"])
	}
	if widgetsArray == nil {
		return nil
	}

	var widgets []rawWidgetInfo
	for _, w := range widgetsArray {
		if wMap, ok := w.(map[string]any); ok {
			widgets = append(widgets, extractWidgetsRecursive(wMap)...)
		}
	}
	return widgets
}

// getBsonArrayElements extracts array elements from BSON array format.
// BSON arrays have format [typeIndicator, item1, item2, ...] where typeIndicator is a number.
func getBsonArrayElements(v any) []any {
	arr := toBsonArray(v)
	if len(arr) == 0 {
		return nil
	}
	// Check if first element is a type indicator (integer)
	if _, ok := arr[0].(int32); ok {
		return arr[1:]
	}
	if _, ok := arr[0].(int); ok {
		return arr[1:]
	}
	// No type indicator, return as-is
	return arr
}

// toBsonArray converts various BSON array types to []interface{}.
func toBsonArray(v any) []any {
	switch arr := v.(type) {
	case []any:
		return arr
	case bson.A:
		return []any(arr)
	default:
		return nil
	}
}

// extractString extracts a string value from an interface.
func extractString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// extractBsonID extracts an ID from a BSON field (can be string or binary).
func extractBsonID(v any) string {
	if v == nil {
		return ""
	}

	// Try string first
	if s, ok := v.(string); ok {
		return s
	}

	// Try BSON binary format: {"Subtype": 0, "Data": "base64..."}
	if m, ok := v.(map[string]any); ok {
		if data, ok := m["Data"].(string); ok {
			// Data is base64-encoded GUID, decode and format
			return decodeBase64GUID(data)
		}
	}

	// Try bson.Binary
	if bin, ok := v.(bson.Binary); ok {
		return formatGUID(bin.Data)
	}

	return ""
}

// decodeBase64GUID decodes a base64-encoded GUID and formats it.
func decodeBase64GUID(data string) string {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil || len(decoded) != 16 {
		return data // Return as-is if can't decode
	}
	return formatGUID(decoded)
}

// extractBinaryID extracts a UUID string from a BSON binary or string value.
func extractBinaryID(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return formatGUID(val)
	case bson.Binary:
		return formatGUID(val.Data)
	default:
		return ""
	}
}

// formatGUID converts a 16-byte GUID to its string representation with proper byte ordering.
func formatGUID(data []byte) string {
	if len(data) != 16 {
		return string(data)
	}
	// Reverse first 4 bytes (group 1)
	g1 := []byte{data[3], data[2], data[1], data[0]}
	// Reverse next 2 bytes (group 2)
	g2 := []byte{data[5], data[4]}
	// Reverse next 2 bytes (group 3)
	g3 := []byte{data[7], data[6]}
	// Last 8 bytes stay in order
	return strings.ToLower(strings.Join([]string{
		bytesToHex(g1),
		bytesToHex(g2),
		bytesToHex(g3),
		bytesToHex(data[8:10]),
		bytesToHex(data[10:16]),
	}, "-"))
}

// bytesToHex converts a byte slice to a hex string.
func bytesToHex(data []byte) string {
	const hex = "0123456789abcdef"
	result := make([]byte, len(data)*2)
	for i, b := range data {
		result[i*2] = hex[b>>4]
		result[i*2+1] = hex[b&0x0f]
	}
	return string(result)
}

// layoutPlatform is "Native" for a native-mobile layout and "Web" otherwise.
// Pages inherit it from their layout; the React-client CE0582 rules apply to
// web pages only (ako/mxcli#953).
func layoutPlatform(l *pages.Layout) string {
	if l.Native {
		return LayoutPlatformNative
	}
	return LayoutPlatformWeb
}

// The values of layouts.Platform.
const (
	LayoutPlatformWeb    = "Web"
	LayoutPlatformNative = "Native"
)

func (b *Builder) buildLayouts() error {
	// Get all layouts
	layoutList, err := b.lister().ListLayouts()
	if err != nil {
		return err
	}

	layoutStmt, err := b.tx.Prepare(`
		INSERT INTO layouts_data (Id, Name, QualifiedName, ModuleName, Folder, LayoutType, Platform,
			Description, Class, Style, DynamicClasses, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer layoutStmt.Close()

	// A layout's widgets feed widgets_data like a page's, with
	// ContainerType=LAYOUT (full mode only). Without them a "no inline styles"
	// rule passed the styling that reaches every page on the layout, and a
	// microflow only a layout's button calls had no caller (ako/mxcli#1302).
	var widgetStmt *Stmt
	if b.fullMode {
		widgetStmt, err = b.tx.Prepare(`
			INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType,
				ModuleName, Folder, EntityRef, AttributeRef, MicroflowRef, NanoflowRef, PageRef, Description,
				ParentWidgetId, Depth, Class, Style, DynamicClasses, ActionType, HasConfirmation,
				ProjectId, SnapshotId)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer widgetStmt.Close()
	}

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	widgetCount := 0
	for _, l := range layoutList {
		if !b.own(string(l.ID)) {
			continue
		}
		// Get module name
		moduleID := b.hierarchy.findModuleID(l.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + l.Name
		folder := b.hierarchy.buildFolderPath(l.ContainerID)

		var rawWidgets []rawWidgetInfo
		var class, style, dynamicClasses string
		if b.fullMode {
			if rawData, _ := b.reader.GetRawUnit(l.ID); rawData != nil {
				rawWidgets = extractLayoutWidgets(rawData)
				class, style, dynamicClasses = widgetAppearance(rawData)
			}
		}

		_, err := layoutStmt.Exec(
			string(l.ID),
			l.Name,
			qualifiedName,
			moduleName,
			folder,
			string(l.LayoutType),
			layoutPlatform(l),
			l.Documentation,
			class, style, dynamicClasses,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
		count++

		for _, w := range rawWidgets {
			if _, err := widgetStmt.Exec(
				w.ID, w.Name, w.WidgetType,
				string(l.ID), qualifiedName, "LAYOUT",
				moduleName, folder,
				w.EntityRef, w.AttributeRef, w.MicroflowRef, w.NanoflowRef, w.PageRef, "",
				w.ParentID, w.Depth, w.Class, w.Style, w.DynamicClasses,
				w.ActionType, w.HasConfirmation,
				projectID, snapshotID,
			); err != nil {
				return fmt.Errorf("insert widget %s for layout %s: %w", w.Name, qualifiedName, err)
			}
			widgetCount++
			b.collectWidgetExtraRefs("LAYOUT", qualifiedName, moduleName, w)
		}
	}
	b.disown()

	b.report("Layouts", count)
	if b.fullMode {
		b.report("Layout widgets", widgetCount)
	}
	return nil
}

// extractLayoutWidgets extracts every widget of a raw layout. A Forms$Layout
// keeps its tree under Content (Forms$WebLayoutContent or
// Forms$NativeLayoutContent): its own Widgets, or — for a layout built on
// another one — the arguments of Content.LayoutCall, shaped like a page's
// FormCall. A native layout adds the header placeholder and the sidebar.
func extractLayoutWidgets(rawData map[string]any) []rawWidgetInfo {
	content, ok := rawData["Content"].(map[string]any)
	if !ok {
		return nil
	}
	var roots []map[string]any
	roots = append(roots, partWidgets(content, "Widgets", "Widget")...)
	if call, ok := content["LayoutCall"].(map[string]any); ok {
		for _, arg := range getBsonArrayElements(call["Arguments"]) {
			if argMap, ok := arg.(map[string]any); ok {
				roots = append(roots, partWidgets(argMap, "Widgets", "Widget")...)
			}
		}
	}
	if ph, ok := content["RightHeaderPlaceholder"].(map[string]any); ok {
		roots = append(roots, ph)
	}
	roots = append(roots, partWidgets(content, "SidebarWidgets", "")...)

	var widgets []rawWidgetInfo
	for _, w := range roots {
		widgets = append(widgets, extractWidgetsRecursive(w)...)
	}
	return widgets
}

// widgetExtraRef is a page/snippet → document edge a widget makes that its
// single widgets_data column does not carry (see rawWidgetInfo.AllRefs).
type widgetExtraRef struct {
	owner                                  string
	containerType, containerQN, moduleName string
	targetType, targetName, refKind        string
}

// collectWidgetExtraRefs records, for buildReferences, every name in w.AllRefs
// that is not already the widget's single column for that kind — those are
// projected from widgets_data directly.
func (b *Builder) collectWidgetExtraRefs(containerType, containerQN, moduleName string, w rawWidgetInfo) {
	add := func(names []string, primary, targetType, refKind string) {
		for _, n := range names {
			if n == primary {
				continue
			}
			b.widgetExtraRefs = append(b.widgetExtraRefs, widgetExtraRef{
				b.currentOwner(), containerType, containerQN, moduleName, targetType, n, refKind,
			})
		}
	}
	add(w.AllRefs.Entities, w.EntityRef, RefObjectEntity, RefKindDatasource)
	add(w.AllRefs.Microflows, w.MicroflowRef, RefObjectMicroflow, RefKindAction)
	add(w.AllRefs.Nanoflows, w.NanoflowRef, RefObjectNanoflow, RefKindAction)
	add(w.AllRefs.Pages, w.PageRef, RefObjectPage, RefKindShowPage)
}
