// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// asActionMap normalizes a raw BSON sub-document (a client action) into a
// map[string]any, accepting either the already-converted map form or the
// raw primitive.M produced by the mongo driver. Returns nil if v is neither.
func asActionMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case primitive.M:
		return map[string]any(m)
	default:
		return nil
	}
}

// parseRawWidget parses a raw widget map into rawWidget structs.
// extractConditionalSettings extracts ConditionalVisibility/Editability from raw BSON.
func extractConditionalSettings(widget *rawWidget, w map[string]any) {
	if cvs, ok := w["ConditionalVisibilitySettings"].(map[string]any); ok && cvs != nil {
		if expr, ok := cvs["Expression"].(string); ok && expr != "" {
			widget.VisibleIf = expr
		}
	}
	if ces, ok := w["ConditionalEditabilitySettings"].(map[string]any); ok && ces != nil {
		if expr, ok := ces["Expression"].(string); ok && expr != "" {
			widget.EditableIf = expr
		}
	}
}

// scrollContainerRegions is the fixed slot order of a Forms$ScrollContainer.
// The BSON key and the name MDL uses for the slot differ for the centre one,
// which Mendix stores as "CenterRegion" while its four siblings are bare
// positions.
var scrollContainerRegions = []struct{ key, name string }{
	{"Top", "top"},
	{"Right", "right"},
	{"Bottom", "bottom"},
	{"Left", "left"},
	{"CenterRegion", "center"},
}

// bsonInt coerces a BSON numeric to int. Mendix stores a region's Size as
// int32, but the decoders in this package hand back whichever width the driver
// chose, so all three are accepted.
func bsonInt(v any) int {
	switch n := v.(type) {
	case int32:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

func parseRawWidget(ctx *ExecContext, w map[string]any, parentEntityContext ...string) []rawWidget {
	inheritedCtx := ""
	if len(parentEntityContext) > 0 {
		inheritedCtx = parentEntityContext[0]
	}
	typeName, _ := w["$Type"].(string)
	name, _ := w["Name"].(string)

	// ScrollContainer: children are nested inside CenterRegion.Widgets
	// rather than Widgets directly, so recurse into CenterRegion so nested
	// widget IDs are visible in DESCRIBE PAGE output.
	if typeName == "Forms$ScrollContainer" || typeName == "Pages$ScrollContainer" {
		widget := rawWidget{
			Type: typeName,
			Name: name,
		}
		if appearance, ok := w["Appearance"].(map[string]any); ok {
			if class, ok := appearance["Class"].(string); ok && class != "" {
				widget.Class = class
			}
			if style, ok := appearance["Style"].(string); ok && style != "" {
				widget.Style = style
			}
			if dc, ok := appearance["DynamicClasses"].(string); ok && dc != "" {
				widget.DynamicClasses = dc
			}
			widget.DesignProperties = extractDesignProperties(appearance)
		}
		extractConditionalSettings(&widget, w)
		// Regions are five named slots, not a list: Top, Right, Bottom, Left
		// and CenterRegion (the last spelled differently from its siblings).
		// Each occupied one becomes a synthetic intermediate widget, the same
		// way TabControl preserves its tab pages below — so the output says
		// which region a widget is in.
		//
		// Reading CenterRegion alone was survivable for pages, where the other
		// slots are usually empty, and wrong for layouts: a layout's topbar
		// lives in Top and its navigation in Left, so the two things anyone
		// describes a layout to see were the two the walk skipped.
		for _, slot := range scrollContainerRegions {
			region, ok := w[slot.key].(map[string]any)
			if !ok {
				continue
			}
			child := rawWidget{Type: "Forms$ScrollContainerRegion", Name: slot.name}
			if appearance, ok := region["Appearance"].(map[string]any); ok {
				if class, ok := appearance["Class"].(string); ok && class != "" {
					child.Class = class
				}
			}
			if sm, ok := region["SizeMode"].(string); ok {
				child.RegionSizeMode = sm
			}
			child.RegionSize = bsonInt(region["Size"])
			for _, c := range getBsonArrayElements(region["Widgets"]) {
				if cMap, ok := c.(map[string]any); ok {
					child.Children = append(child.Children, parseRawWidget(ctx, cMap, inheritedCtx)...)
				}
			}
			widget.Children = append(widget.Children, child)
		}
		// Fallback for older BSON that stored children directly on the
		// container rather than in a region.
		if len(widget.Children) == 0 {
			for _, c := range getBsonArrayElements(w["Widgets"]) {
				if cMap, ok := c.(map[string]any); ok {
					widget.Children = append(widget.Children, parseRawWidget(ctx, cMap, inheritedCtx)...)
				}
			}
		}
		return []rawWidget{widget}
	}

	// TabControl: children are grouped under TabPages[]. Preserve each tab
	// page as a synthetic intermediate widget so the output distinguishes
	// which tab each nested widget belongs to.
	if typeName == "Forms$TabControl" || typeName == "Pages$TabControl" {
		widget := rawWidget{
			Type: typeName,
			Name: name,
		}
		if appearance, ok := w["Appearance"].(map[string]any); ok {
			if class, ok := appearance["Class"].(string); ok && class != "" {
				widget.Class = class
			}
			if style, ok := appearance["Style"].(string); ok && style != "" {
				widget.Style = style
			}
			if dc, ok := appearance["DynamicClasses"].(string); ok && dc != "" {
				widget.DynamicClasses = dc
			}
			widget.DesignProperties = extractDesignProperties(appearance)
		}
		extractConditionalSettings(&widget, w)
		for _, tp := range getBsonArrayElements(w["TabPages"]) {
			tpMap, ok := tp.(map[string]any)
			if !ok {
				continue
			}
			tabPage := rawWidget{
				Type: "Pages$TabPage",
			}
			if n, ok := tpMap["Name"].(string); ok {
				tabPage.Name = n
			}
			if ct, ok := tpMap["CaptionTemplate"].(map[string]any); ok {
				tabPage.TabCaption = extractTextFromTemplate(ctx, ct)
			}
			if tabPage.TabCaption == "" {
				tabPage.TabCaption = extractTextCaption(ctx, tpMap)
			}
			for _, tw := range getBsonArrayElements(tpMap["Widgets"]) {
				if twMap, ok := tw.(map[string]any); ok {
					tabPage.Children = append(tabPage.Children, parseRawWidget(ctx, twMap, inheritedCtx)...)
				}
			}
			widget.Children = append(widget.Children, tabPage)
		}
		return []rawWidget{widget}
	}

	// Parse DivContainer as a proper CONTAINER widget with children
	if typeName == "Forms$DivContainer" || typeName == "Pages$DivContainer" ||
		typeName == "Forms$GroupBox" || typeName == "Pages$GroupBox" {
		widget := rawWidget{
			Type: typeName,
			Name: name,
		}
		// Extract CSS class, style, and design properties from Appearance
		if appearance, ok := w["Appearance"].(map[string]any); ok {
			if class, ok := appearance["Class"].(string); ok && class != "" {
				widget.Class = class
			}
			if style, ok := appearance["Style"].(string); ok && style != "" {
				widget.Style = style
			}
			if dc, ok := appearance["DynamicClasses"].(string); ok && dc != "" {
				widget.DynamicClasses = dc
			}
			widget.DesignProperties = extractDesignProperties(appearance)
		}
		// Extract the container's "On click" action (Forms$DivContainer.OnClickAction).
		// GroupBox has no click action, so only DivContainer is inspected (issue #603).
		if typeName == "Forms$DivContainer" || typeName == "Pages$DivContainer" {
			if onClick := asActionMap(w["OnClickAction"]); onClick != nil {
				widget.Action = extractButtonAction(ctx, map[string]any{"Action": onClick})
			}
		}
		// Extract GroupBox-specific properties
		if typeName == "Forms$GroupBox" || typeName == "Pages$GroupBox" {
			// Caption is stored as CaptionTemplate (Forms$ClientTemplate)
			if ct, ok := w["CaptionTemplate"].(map[string]any); ok {
				widget.Caption = extractTextFromTemplate(ctx, ct)
			} else {
				// Fallback to legacy Caption field
				widget.Caption = extractTextCaption(ctx, w)
			}
			if collapsible, ok := w["Collapsible"].(string); ok {
				widget.Collapsible = collapsible
			}
			if headerMode, ok := w["HeaderMode"].(string); ok {
				widget.HeaderMode = headerMode
			}
		}
		extractConditionalSettings(&widget, w)
		children := getBsonArrayElements(w["Widgets"])
		if children != nil {
			for _, c := range children {
				if cMap, ok := c.(map[string]any); ok {
					widget.Children = append(widget.Children, parseRawWidget(ctx, cMap, inheritedCtx)...)
				}
			}
		}
		return []rawWidget{widget}
	}

	widget := rawWidget{
		Type: typeName,
		Name: name,
	}
	extractConditionalSettings(&widget, w)

	// Extract CSS class, style, and design properties from Appearance
	if appearance, ok := w["Appearance"].(map[string]any); ok {
		if class, ok := appearance["Class"].(string); ok && class != "" {
			widget.Class = class
		}
		if style, ok := appearance["Style"].(string); ok && style != "" {
			widget.Style = style
		}
		if dc, ok := appearance["DynamicClasses"].(string); ok && dc != "" {
			widget.DynamicClasses = dc
		}
		widget.DesignProperties = extractDesignProperties(appearance)
	}

	switch typeName {
	case "Forms$LayoutGrid", "Pages$LayoutGrid":
		widget.Rows = parseLayoutGridRows(ctx, w, inheritedCtx)
		return []rawWidget{widget}

	case "Forms$NavigationTree", "Pages$NavigationTree",
		"Forms$MenuBar", "Pages$MenuBar":
		// The profile is a qualified name one level down, in a
		// Forms$NavigationSource, not a property of the tree.
		if src, ok := w["MenuSource"].(map[string]any); ok {
			if p, ok := src["NavigationProfile"].(string); ok {
				widget.NavigationProfile = p
			}
		}
		return []rawWidget{widget}

	case "Forms$DynamicText", "Pages$DynamicText":
		widget.Content = extractTextContent(ctx, w, "Content")
		widget.Parameters = extractClientTemplateParameters(ctx, w, "Content")
		if rm, ok := w["RenderMode"].(string); ok {
			widget.RenderMode = rm
		}
		return []rawWidget{widget}

	case "Forms$ActionButton", "Pages$ActionButton":
		widget.Caption = extractButtonCaption(ctx, w)
		widget.Parameters = extractButtonCaptionParameters(ctx, w)
		widget.ButtonStyle = extractButtonStyle(ctx, w)
		widget.Action = extractButtonAction(ctx, w)
		// RenderType "Link" is a linkbutton; the emitter uses this to choose the
		// `linkbutton` keyword over `actionbutton` for a clean DESCRIBE roundtrip.
		if rt, ok := w["RenderType"].(string); ok {
			widget.RenderMode = rt
		}
		widget.Icon, widget.IconType, widget.IconCode = extractButtonIcon(w)
		return []rawWidget{widget}

	case "Forms$Text", "Pages$Text":
		widget.Content = extractTextCaption(ctx, w)
		if rm, ok := w["RenderMode"].(string); ok {
			widget.RenderMode = rm
		}
		return []rawWidget{widget}

	case "Forms$Title", "Pages$Title":
		widget.Caption = extractTextCaption(ctx, w)
		return []rawWidget{widget}

	case "Forms$DataView", "Pages$DataView":
		widget.DataSource = extractDataViewDataSource(ctx, w)
		if widget.DataSource != nil && widget.DataSource.Reference != "" {
			widget.EntityContext = dataSourceEntityContext(ctx, widget.DataSource)
		} else if inheritedCtx != "" {
			widget.EntityContext = inheritedCtx
		}
		widget.LabelWidth = extractDataViewLabelWidth(w)
		widget.ShowFooter, _ = w["ShowFooter"].(bool)
		widget.Children = parseDataViewChildren(ctx, w, widget.EntityContext)
		return []rawWidget{widget}

	case "Forms$TextBox", "Pages$TextBox":
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractAttributeRef(ctx, w)
		widget.Placeholder = extractPlaceholderText(ctx, w)
		widget.Editable = extractEditable(ctx, w)
		widget.OnChange = extractOnChangeAction(ctx, w)
		return []rawWidget{widget}

	case "Forms$TextArea", "Pages$TextArea":
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractAttributeRef(ctx, w)
		widget.Editable = extractEditable(ctx, w)
		widget.OnChange = extractOnChangeAction(ctx, w)
		return []rawWidget{widget}

	case "Forms$DatePicker", "Pages$DatePicker":
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractAttributeRef(ctx, w)
		widget.Editable = extractEditable(ctx, w)
		widget.OnChange = extractOnChangeAction(ctx, w)
		return []rawWidget{widget}

	case "Forms$RadioButtons", "Pages$RadioButtons", "Forms$RadioButtonGroup", "Pages$RadioButtonGroup":
		widget.Type = "Forms$RadioButtons" // Normalize type
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractAttributeRef(ctx, w)
		widget.Editable = extractEditable(ctx, w)
		widget.OnChange = extractOnChangeAction(ctx, w)
		return []rawWidget{widget}

	case "Forms$CheckBox", "Pages$CheckBox":
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractAttributeRef(ctx, w)
		widget.Editable = extractEditable(ctx, w)
		widget.ReadOnlyStyle = extractReadOnlyStyle(ctx, w)
		widget.ShowLabel = extractShowLabel(ctx, w)
		widget.OnChange = extractOnChangeAction(ctx, w)
		return []rawWidget{widget}

	case "CustomWidgets$CustomWidget":
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractCustomWidgetAttribute(ctx, w)
		widget.RenderMode = extractCustomWidgetType(ctx, w) // Store widget type in RenderMode
		widget.WidgetID = extractCustomWidgetID(ctx, w)
		// For ComboBox, extract datasource and association attribute for association mode.
		// In association mode the Attribute binding is stored as EntityRef (not AttributeRef),
		// so we must use extractCustomWidgetPropertyAssociation instead of the generic scan.
		if widget.RenderMode == "combobox" {
			widget.DataSource = extractComboBoxDataSource(ctx, w)
			if widget.DataSource != nil {
				widget.Content = extractCustomWidgetPropertyAssociation(ctx, w, "attributeAssociation")
				widget.CaptionAttribute = extractCustomWidgetPropertyAttributeRef(ctx, w, "optionsSourceAssociationCaptionAttribute")
			}
			// The on-change action, in BOTH modes — the def maps `onChangeEvent`
			// in each, and modes are exclusive. Read outside the DataSource
			// branch so an enumeration-mode ComboBox keeps its action too.
			//
			// Without this the write path stored the action correctly and
			// describe simply never emitted it, so a describe→edit→exec cycle
			// deleted it: measured 1 Forms$MicroflowAction → 0, while the
			// datepicker beside it survived. Same shape as the DataGrid2
			// `onClick` round trip below (ledger #67).
			widget.OnChange = renderClientActionMDL(ctx, customWidgetPropertyActionMap(ctx, w, "onChangeEvent"))
		}
		// The drop-down filter's association mode is the same shape as the
		// ComboBox's, on differently-named properties: `baseType` selects it and
		// the reference is stored as an EntityRef, not an AttributeRef. Without
		// this the filter described back as a bare `dropdownfilter name` and a
		// describe→edit→exec cycle silently reverted it to attribute mode. (#830)
		if widget.RenderMode == "dropdownfilter" &&
			extractCustomWidgetPropertyString(ctx, w, "baseType") == "ref" {
			widget.DataSource = extractCustomWidgetPropertyDataSource(ctx, w, "refOptions")
			widget.Content = extractCustomWidgetPropertyAssociation(ctx, w, "refEntity")
			widget.CaptionAttribute = extractCustomWidgetPropertyAttributeRef(ctx, w, "refCaption")
		}
		// For DataGrid2, also extract datasource, columns, CONTROLBAR widgets, paging, and selection
		if widget.RenderMode == "datagrid2" {
			widget.DataSource = extractDataGrid2DataSource(ctx, w)
			widget.PageSize = extractCustomWidgetPropertyString(ctx, w, "pageSize")
			widget.Pagination = extractCustomWidgetPropertyString(ctx, w, "pagination")
			widget.PagingPosition = extractCustomWidgetPropertyString(ctx, w, "pagingPosition")
			widget.ShowPagingButtons = extractCustomWidgetPropertyString(ctx, w, "showPagingButtons")
			// showNumberOfRows: not yet fully supported in DataGrid2, skip to avoid CE0463
			widget.Selection = extractGallerySelection(ctx, w)
			if widget.DataSource != nil && widget.DataSource.Reference != "" {
				widget.EntityContext = dataSourceEntityContext(ctx, widget.DataSource)
			} else if inheritedCtx != "" {
				widget.EntityContext = inheritedCtx
			}
			widget.DataGridColumns = extractDataGrid2Columns(ctx, w, widget.EntityContext)
			widget.ControlBar = extractDataGrid2ControlBar(ctx, w)
			// onClick action (ledger #67): read the client action back with full
			// parameter mappings so a describe round-trip re-emits the onClick.
			// By SOURCE, not by the literal key — the widget may store it as
			// `onClickEvent`/`onClickAction`, which the writer accepts and the
			// literal lookup could not see (#956).
			widget.OnClick = renderClientActionMDL(ctx, customWidgetActionForSource(ctx, w, "OnClick"))
			// Slots MDL addresses by the widget's own key — DataGrid2 stores
			// onSelectionChange and onConfigurationChange (#956).
			widget.NamedActions = namedActionSlotsOf(ctx, w)
		}
		// For Gallery, extract datasource, content widgets, filter widgets, and selection mode
		if widget.RenderMode == "gallery" {
			widget.DataSource = extractGalleryDataSource(ctx, w)
			widget.Selection = extractGallerySelection(ctx, w)
			widget.DesktopColumns = extractCustomWidgetPropertyString(ctx, w, "desktopItems")
			widget.TabletColumns = extractCustomWidgetPropertyString(ctx, w, "tabletItems")
			widget.PhoneColumns = extractCustomWidgetPropertyString(ctx, w, "phoneItems")
			if widget.DataSource != nil && widget.DataSource.Reference != "" {
				widget.EntityContext = dataSourceEntityContext(ctx, widget.DataSource)
			} else if inheritedCtx != "" {
				widget.EntityContext = inheritedCtx
			}
			widget.Children = extractGalleryContent(ctx, w, widget.EntityContext)
			widget.FilterWidgets = extractGalleryFilters(ctx, w)
		}
		// For filter widgets, extract filter attributes and expression
		if widget.RenderMode == "textfilter" || widget.RenderMode == "numberfilter" || widget.RenderMode == "dropdownfilter" || widget.RenderMode == "datefilter" {
			widget.FilterAttributes = extractFilterAttributes(ctx, w)
			widget.FilterExpression = extractFilterExpression(ctx, w)
		}
		// For pluggable Image widget, extract image-specific properties
		if widget.RenderMode == "image" {
			extractImageProperties(ctx, w, &widget)
		}
		// For generic pluggable widgets (not handled by dedicated extractors above),
		// extract all non-default properties as explicit key-value pairs.
		if !isKnownCustomWidgetType(widget.RenderMode) {
			widget.ExplicitProperties = extractExplicitProperties(ctx, w)
			widget.ObjectLists = extractObjectLists(ctx, w)
			widget.ChildSlots = extractChildSlots(ctx, w, widget.EntityContext)
			widget.OmittedContainers = unreconstructedContainers(w, widget.ObjectLists, widget.ChildSlots)
			// onClick action (ledger #67 — reported on CustomChart): read the client
			// action back with full parameter mappings so a describe round-trip
			// re-emits it (the finding's original widget goes through this path).
			// By SOURCE, not by the literal key — BadgeButton stores the same slot
			// as `onClickEvent` and HeatMap as `onClickAction`, both of which the
			// writer accepts and the literal lookup could not see (#956).
			widget.OnClick = renderClientActionMDL(ctx, customWidgetActionForSource(ctx, w, "OnClick"))
			// The change slot too. This path had no OnChange read at all, so a
			// Slider/RangeSlider/StarRating stored its action correctly and
			// described back without it — the same one-way write as the click
			// slot, on the widgets that reach describe generically (#956).
			widget.OnChange = renderClientActionMDL(ctx, customWidgetActionForSource(ctx, w, "OnChange"))
			// Every remaining action slot, addressed by the widget's own key
			// (#956) — a File Uploader's createFileAction / onUploadSuccessFile,
			// a Switch's `action`, and so on.
			widget.NamedActions = namedActionSlotsOf(ctx, w)
		}
		// Generic datasource fallback for a pluggable widget with no per-widget
		// extractor above. Without it a widget whose datasource lives on a
		// property MDL reaches through the `DataSource:` keyword described back
		// with no datasource at all, and a rewrite dropped it — measured on a
		// Studio Pro-authored File Uploader 2.5.0: a page at 0 errors came back
		// as CE0642 "Property 'Associated files' is required" plus two CE1571
		// (the action's parameter has no default once its datasource is gone),
		// while the DESCRIBE text was byte-identical before and after (#956).
		//
		// Read every named source, skipping empty datasource-shaped properties.
		// firstObjectPropertyDataSource cannot do that: it stops at the first
		// property whose DataSource parses at all, and a File Uploader has one
		// carrying no reference ahead of its real one.
		if widget.DataSource == nil {
			named := namedCustomWidgetDataSources(w)
			if len(named) > 1 {
				widget.NamedDataSources = named
			} else if len(named) == 1 {
				widget.DataSource = named[0].DataSource
				if widget.EntityContext == "" {
					widget.EntityContext = dataSourceEntityContext(ctx, named[0].DataSource)
				}
			}
		}
		return []rawWidget{widget}

	case "Forms$Label", "Pages$Label":
		widget.Content = extractTextCaption(ctx, w)
		return []rawWidget{widget}

	case "Forms$NavigationList", "Pages$NavigationList":
		widget.Children = parseNavigationListItems(ctx, w)
		return []rawWidget{widget}

	case "Forms$Gallery", "Pages$Gallery":
		widget.DataSource = extractGalleryDataSource(ctx, w)
		if widget.DataSource != nil && widget.DataSource.Reference != "" {
			widget.EntityContext = dataSourceEntityContext(ctx, widget.DataSource)
		} else if inheritedCtx != "" {
			widget.EntityContext = inheritedCtx
		}
		widget.Children = parseGalleryContent(ctx, w, widget.EntityContext)
		return []rawWidget{widget}

	case "Forms$SnippetCallWidget", "Pages$SnippetCallWidget":
		widget.Content = extractSnippetRef(ctx, w)
		return []rawWidget{widget}

	case "Forms$ListView", "Pages$ListView":
		widget.DataSource = extractListViewDataSource(ctx, w)
		if widget.DataSource != nil && widget.DataSource.Reference != "" {
			widget.EntityContext = dataSourceEntityContext(ctx, widget.DataSource)
		} else if inheritedCtx != "" {
			widget.EntityContext = inheritedCtx
		}
		// Round-trip a non-default PageSize (the output formatter suppresses "20").
		if ps := extractInt(w["PageSize"]); ps > 0 {
			widget.PageSize = strconv.Itoa(ps)
		}
		// A List View's Editable is a BOOL — the property that makes the inputs
		// inside it editable — not the three-valued Always/Never/Conditional one
		// the input widgets carry in this same field. Reusing the field means the
		// shared formatter prints it; emitting it separately there duplicates the
		// property. Only `true` is carried: false is Mendix's default and an empty
		// value keeps an unchanged list view quiet.
		//
		// Without this an editable list view round-trips into a read-only one,
		// which is the failure the builder fix would otherwise reintroduce on the
		// next describe → exec (ako/CapTrackV3 FINDINGS §6).
		if editable, ok := w["Editable"].(bool); ok && editable {
			widget.Editable = "true"
		}
		widget.Children = parseListViewContent(ctx, w, widget.EntityContext)
		return []rawWidget{widget}

	default:
		// For unknown types, just note them
		return []rawWidget{widget}
	}
}

func parseLayoutGridRows(ctx *ExecContext, w map[string]any, entityContext ...string) []rawWidgetRow {
	entCtx := ""
	if len(entityContext) > 0 {
		entCtx = entityContext[0]
	}
	rows := getBsonArrayElements(w["Rows"])
	if rows == nil {
		return nil
	}

	var result []rawWidgetRow
	for _, r := range rows {
		rMap, ok := r.(map[string]any)
		if !ok {
			continue
		}
		row := rawWidgetRow{}
		cols := getBsonArrayElements(rMap["Columns"])
		for _, c := range cols {
			cMap, ok := c.(map[string]any)
			if !ok {
				continue
			}
			col := rawWidgetColumn{}
			// Get width
			if weight, ok := cMap["Weight"].(int32); ok {
				col.Width = int(weight)
			} else if weight, ok := cMap["DesktopWeight"].(int32); ok {
				col.Width = int(weight)
			}
			if tw, ok := cMap["TabletWeight"].(int32); ok {
				col.TabletWidth = int(tw)
			}
			if pw, ok := cMap["PhoneWeight"].(int32); ok {
				col.PhoneWidth = int(pw)
			}
			// Get widgets
			colWidgets := getBsonArrayElements(cMap["Widgets"])
			for _, cw := range colWidgets {
				if cwMap, ok := cw.(map[string]any); ok {
					col.Widgets = append(col.Widgets, parseRawWidget(ctx, cwMap, entCtx)...)
				}
			}
			row.Columns = append(row.Columns, col)
		}
		result = append(result, row)
	}
	return result
}

// parseNavigationListItems extracts items from a NavigationList widget.
func parseNavigationListItems(ctx *ExecContext, w map[string]any) []rawWidget {
	items := getBsonArrayElements(w["Items"])
	if items == nil {
		return nil
	}

	var result []rawWidget
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		rw := rawWidget{
			Type: "NavigationListItem",
		}

		// Read Name field
		rw.Name, _ = itemMap["Name"].(string)

		// Parse all child widgets from the Widgets array
		widgets := getBsonArrayElements(itemMap["Widgets"])
		for _, widget := range widgets {
			wMap, ok := widget.(map[string]any)
			if !ok {
				continue
			}
			parsed := parseRawWidget(ctx, wMap)
			rw.Children = append(rw.Children, parsed...)
		}

		// Extract action
		rw.Action = extractNavigationListItemAction(ctx, itemMap)

		// Extract style from Appearance class
		if appearance, ok := itemMap["Appearance"].(map[string]any); ok {
			if class, ok := appearance["Class"].(string); ok && class != "" {
				rw.ButtonStyle = class
			}
		}

		result = append(result, rw)
	}
	return result
}

// extractNavigationListItemAction extracts action from a NavigationListItem.
// NavigationListItem uses Forms$FormAction with FormSettings.Form for page references,
// which differs from ActionButton's action format.
func extractNavigationListItemAction(ctx *ExecContext, w map[string]any) string {
	action, ok := w["Action"].(map[string]any)
	if !ok {
		return ""
	}
	typeName, _ := action["$Type"].(string)
	switch typeName {
	case "Forms$FormAction", "Pages$FormAction":
		// Extract page reference from FormSettings (Studio Pro format)
		if formSettings, ok := action["FormSettings"].(map[string]any); ok {
			if formName, ok := formSettings["Form"].(string); ok && formName != "" {
				return "show_page '" + formName + "'"
			}
		}
		// Fall back to PageSettings.Form (string name)
		if pageSettings, ok := action["PageSettings"].(map[string]any); ok {
			if pageName, ok := pageSettings["Form"].(string); ok && pageName != "" {
				return "show_page '" + pageName + "'"
			}
		}
		// Fall back to Page field (binary ID from mxcli serialization)
		if pageID := extractBinaryID(action["Page"]); pageID != "" {
			pageName := getPageQualifiedName(ctx, model.ID(pageID))
			if pageName != "" {
				return "show_page '" + pageName + "'"
			}
		}
		return "show_page"
	default:
		// Delegate to the standard action extractor
		return extractButtonAction(ctx, w)
	}
}

// parseDataViewChildren extracts child widgets from a DataView.
// entityContext is the resolved entity context from the enclosing data container.
func parseDataViewChildren(ctx *ExecContext, w map[string]any, entityContext ...string) []rawWidget {
	entCtx := ""
	if len(entityContext) > 0 {
		entCtx = entityContext[0]
	}
	var result []rawWidget

	// Get main widgets
	widgets := getBsonArrayElements(w["Widgets"])
	for _, child := range widgets {
		if childMap, ok := child.(map[string]any); ok {
			result = append(result, parseRawWidget(ctx, childMap, entCtx)...)
		}
	}

	// Get footer widgets
	footerWidgets := getBsonArrayElements(w["FooterWidgets"])
	if len(footerWidgets) > 0 {
		// Create a special footer container with synthetic name
		footer := rawWidget{Type: "Footer", Name: "footer1"}
		for _, child := range footerWidgets {
			if childMap, ok := child.(map[string]any); ok {
				footer.Children = append(footer.Children, parseRawWidget(ctx, childMap, entCtx)...)
			}
		}
		result = append(result, footer)
	}

	return result
}

// extractDataViewDataSource extracts the data source from a DataView widget.
func extractDataViewDataSource(ctx *ExecContext, w map[string]any) *rawDataSource {
	ds, ok := w["DataSource"].(map[string]any)
	if !ok {
		return nil
	}
	return parseDataSource(ds)
}

func extractDataViewLabelWidth(w map[string]any) int {
	v, ok := w["LabelWidth"]
	if !ok {
		return -1
	}
	switch n := v.(type) {
	case int32:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	}
	return -1
}

// extractLabelText extracts the label text from an input widget.
func extractLabelText(ctx *ExecContext, w map[string]any) string {
	labelTemplate, ok := w["LabelTemplate"].(map[string]any)
	if !ok {
		return ""
	}
	return extractTextFromTemplate(ctx, labelTemplate)
}

// extractPlaceholderText extracts the placeholder hint text from an input widget.
// PlaceholderTemplate is a Forms$ClientTemplate, same shape as LabelTemplate.
func extractPlaceholderText(ctx *ExecContext, w map[string]any) string {
	placeholderTemplate, ok := w["PlaceholderTemplate"].(map[string]any)
	if !ok {
		return ""
	}
	return extractTextFromTemplate(ctx, placeholderTemplate)
}

// extractEditable extracts the Editable setting from an input widget.
// Returns "Always", "Never", or "Conditional".
//
// Every input widget must call this, not just CheckBox. While TextBox did not,
// `describe page` emitted no editability at all for the widget people use most,
// so a describe/exec round-trip silently normalised `Editable: Never` back to
// Always — and a describe/diff could not reveal the CREATE-path loss that
// ako/mxcli-maintenance-2 reported, because both sides printed nothing.
func extractEditable(ctx *ExecContext, w map[string]any) string {
	if editable, ok := w["Editable"].(string); ok {
		return editable
	}
	return ""
}

// extractReadOnlyStyle extracts the ReadOnlyStyle from an input widget.
// Returns "Inherit", "Control", or "Text".
func extractReadOnlyStyle(ctx *ExecContext, w map[string]any) string {
	if style, ok := w["ReadOnlyStyle"].(string); ok {
		return style
	}
	return ""
}

// extractShowLabel extracts whether the label is visible from LabelTemplate.
func extractShowLabel(ctx *ExecContext, w map[string]any) bool {
	labelTemplate, ok := w["LabelTemplate"].(map[string]any)
	if !ok {
		return true // Default to showing label
	}
	// Check for TextVisible field - false means "Show label: No"
	if textVisible, ok := labelTemplate["TextVisible"].(bool); ok {
		return textVisible
	}
	return true // Default
}

// extractTextFromTemplate extracts text from a ClientTemplate.
// ClientTemplate structure: Template.Items[] contains Texts$Translation with Text field
func extractTextFromTemplate(ctx *ExecContext, template map[string]any) string {
	lang := describeDefaultLanguage(ctx)
	// For ClientTemplate (Forms$ClientTemplate), the text is in Template.Items[].Text
	if innerTemplate, ok := template["Template"].(map[string]any); ok {
		if text := selectTranslationText(getBsonArrayElements(innerTemplate["Items"]), lang); text != "" {
			return text
		}
	}
	// Fallback: direct Items array (for legacy or different template types).
	return selectTranslationText(getBsonArrayElements(template["Items"]), lang)
}

// shortAttributeName strips the qualified prefix from a BSON attribute path.
// "Module.Entity.Attribute" → "Attribute". The entity context is established
// by the enclosing DATAVIEW, so DESCRIBE outputs only the bare name.
func shortAttributeName(attr string) string {
	if idx := strings.LastIndex(attr, "."); idx >= 0 {
		return attr[idx+1:]
	}
	return attr
}

// extractAttributeRef extracts the attribute reference from an input widget.
// Returns just the attribute name (last segment).
func extractAttributeRef(ctx *ExecContext, w map[string]any) string {
	attrRef, ok := w["AttributeRef"].(map[string]any)
	if !ok {
		return ""
	}
	attr, ok := attrRef["Attribute"].(string)
	if !ok {
		return ""
	}
	return shortAttributeName(attr)
}

// parseGalleryContent extracts the content widget from a Gallery.
// entityContext is the resolved entity context from the Gallery's datasource.
func parseGalleryContent(ctx *ExecContext, w map[string]any, entityContext ...string) []rawWidget {
	entCtx := ""
	if len(entityContext) > 0 {
		entCtx = entityContext[0]
	}
	content := w["ContentWidget"]
	if content == nil {
		return nil
	}
	contentMap, ok := content.(map[string]any)
	if !ok {
		return nil
	}
	return parseRawWidget(ctx, contentMap, entCtx)
}

// parseListViewContent extracts the content widgets from a ListView.
// entityContext is the resolved entity context from the enclosing list container.
func parseListViewContent(ctx *ExecContext, w map[string]any, entityContext ...string) []rawWidget {
	entCtx := ""
	if len(entityContext) > 0 {
		entCtx = entityContext[0]
	}
	var result []rawWidget
	for _, wgt := range getBsonArrayElements(w["Widgets"]) {
		wgtMap, ok := wgt.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, parseRawWidget(ctx, wgtMap, entCtx)...)
	}
	result = append(result, parseListViewTemplates(ctx, w)...)
	return result
}

// parseListViewTemplates reads a List View's specialization templates — the
// per-specialization bodies Studio Pro stores in a Templates array, alongside
// (not inside) the list view's own Widgets.
//
// Nothing read this array before, so a template's entire contents were absent
// from DESCRIBE with no warning, and SEARCH inherited the blind spot because the
// catalog's source table is built from DESCRIBE output. Order is preserved: it is
// authored, not derived. Issue #940.
func parseListViewTemplates(ctx *ExecContext, w map[string]any) []rawWidget {
	var result []rawWidget
	for _, tpl := range getBsonArrayElements(w["Templates"]) {
		tplMap, ok := tpl.(map[string]any)
		if !ok {
			continue
		}
		// Inside a template the context object is the specialization, so an
		// attribute it adds resolves even though the list view's own entity
		// does not have it.
		spec := extractString(tplMap["Entity"])
		wrapper := rawWidget{
			Type:           "Forms$ListViewTemplate",
			Specialization: spec,
			EntityContext:  spec,
		}
		for _, wgt := range getBsonArrayElements(tplMap["Widgets"]) {
			wgtMap, ok := wgt.(map[string]any)
			if !ok {
				continue
			}
			wrapper.Children = append(wrapper.Children, parseRawWidget(ctx, wgtMap, spec)...)
		}
		result = append(result, wrapper)
	}
	return result
}

// extractListViewDataSource extracts the datasource from a ListView widget.
func extractListViewDataSource(ctx *ExecContext, w map[string]any) *rawDataSource {
	ds, ok := w["DataSource"].(map[string]any)
	if !ok || ds == nil {
		return nil
	}
	return parseDataSource(ds)
}

func extractSnippetRef(ctx *ExecContext, w map[string]any) string {
	// First try the FormCall.Form path (used for BY_NAME_REFERENCE)
	if formCall, ok := w["FormCall"].(map[string]any); ok {
		if form, ok := formCall["Form"].(string); ok && form != "" {
			return form
		}
		// Try binary ID and resolve to name
		if formID := extractBinaryID(formCall["Form"]); formID != "" {
			// Try to resolve the snippet name from ID
			snippets, err := ctx.Backend.ListSnippets()
			if err == nil {
				for _, s := range snippets {
					if string(s.ID) == formID {
						moduleName := ""
						if modules, err := ctx.Backend.ListModules(); err == nil {
							for _, m := range modules {
								if m.ID == s.ContainerID {
									moduleName = m.Name
									break
								}
							}
						}
						if moduleName != "" {
							return moduleName + "." + s.Name
						}
						return s.Name
					}
				}
			}
		}
	}
	// Fallback to direct Snippet field
	return extractString(w["Snippet"])
}

// extractDesignProperties extracts design properties from an Appearance map.
// The DesignProperties field is a BSON array: [version, prop1, prop2, ...]
// Studio Pro uses a nested format where each prop has $Type "Forms$DesignPropertyValue"
// with Key and a Value sub-map containing the actual Toggle/Option type.
// We also handle the flat format (Toggle/Option directly) for backward compatibility.
func extractDesignProperties(appearance map[string]any) []rawDesignProp {
	dpArray := getBsonArrayElements(appearance["DesignProperties"])
	if len(dpArray) == 0 {
		return nil
	}

	var result []rawDesignProp
	for _, item := range dpArray {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if dp, ok := parseDesignProperty(itemMap); ok {
			result = append(result, dp)
		}
	}
	return result
}

// parseDesignProperty parses a single design-property item. It handles both the
// Studio Pro nested format (Forms$DesignPropertyValue wrapping a typed Value) and
// the flat format mxcli once wrote. Compound values (Forms$CompoundDesignPropertyValue)
// recurse over their Properties list (issue #668).
func parseDesignProperty(itemMap map[string]any) (rawDesignProp, bool) {
	typeName, _ := itemMap["$Type"].(string)
	key, _ := itemMap["Key"].(string)
	if key == "" {
		return rawDesignProp{}, false
	}

	switch typeName {
	case "Forms$DesignPropertyValue":
		// Studio Pro nested format: Value sub-map contains the actual type
		valueMap, ok := itemMap["Value"].(map[string]any)
		if !ok {
			return rawDesignProp{}, false
		}
		switch valueMap["$Type"].(string) {
		case "Forms$ToggleDesignPropertyValue":
			return rawDesignProp{Key: key, ValueType: "toggle"}, true
		case "Forms$OptionDesignPropertyValue":
			option, _ := valueMap["Option"].(string)
			return rawDesignProp{Key: key, ValueType: "option", Option: option}, true
		case "Forms$CustomDesignPropertyValue":
			value, _ := valueMap["Value"].(string)
			// Treat custom (ToggleButtonGroup) as option for display
			return rawDesignProp{Key: key, ValueType: "option", Option: value}, true
		case "Forms$CompoundDesignPropertyValue":
			var nested []rawDesignProp
			for _, sub := range getBsonArrayElements(valueMap["Properties"]) {
				if subMap, ok := sub.(map[string]any); ok {
					if dp, ok := parseDesignProperty(subMap); ok {
						nested = append(nested, dp)
					}
				}
			}
			return rawDesignProp{Key: key, ValueType: "compound", Nested: nested}, true
		}
	case "Forms$ToggleDesignPropertyValue":
		// Flat format (backward compat with mxcli-written pages)
		return rawDesignProp{Key: key, ValueType: "toggle"}, true
	case "Forms$OptionDesignPropertyValue":
		// Flat format (backward compat with mxcli-written pages)
		option, _ := itemMap["Option"].(string)
		return rawDesignProp{Key: key, ValueType: "option", Option: option}, true
	}
	return rawDesignProp{}, false
}
