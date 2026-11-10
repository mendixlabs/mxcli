// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// asActionMap normalizes a raw BSON sub-document (a client action) into a
// map[string]any, accepting either the already-converted map form or the
// raw bson.M produced by the mongo driver. Returns nil if v is neither.
func asActionMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case bson.M:
		return map[string]any(m)
	default:
		return nil
	}
}

// parseRawWidget parses a raw widget map into rawWidget structs.
// extractConditionalSettings extracts ConditionalVisibility/Editability from raw BSON.
func extractConditionalSettings(ctx *ExecContext, widget *rawWidget, w map[string]any) {
	if cvs, ok := w["ConditionalVisibilitySettings"].(map[string]any); ok && cvs != nil {
		if expr, ok := cvs["Expression"].(string); ok && expr != "" {
			widget.VisibleIf = expr
		}
		// Attribute-based: one Enumerations$Condition per value. Without this the
		// setting was never read, so describe → exec wrote the widget always
		// visible (Administration.Account_Edit: 8 conditions → 0).
		if attr, ok := cvs["Attribute"].(string); ok && attr != "" {
			widget.VisibleAttr = describeAttr(ctx, attr)
			// Read from a page or snippet parameter (TestApp
			// Snip_UserTask_NameColumnWithIcon): `$Param.Attr in (…)`, or from
			// an enclosing data view (FactoryManagement
			// ConversationalUI.Trace_Details): `$dataView6.Attr in (…)`.
			if param := namedObjectSourceName(cvs["SourceVariable"]); param != "" {
				widget.VisibleAttr = parameterAttributeMDL(param, attr)
			}
			for _, c := range getBsonArrayElements(cvs["Conditions"]) {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				if shown, _ := cm["EditableVisible"].(bool); !shown {
					continue
				}
				v, _ := cm["AttributeValue"].(string)
				if v == emptyConditionValue {
					v = "empty"
				}
				widget.VisibleValues = append(widget.VisibleValues, v)
			}
		}
		// "Visible for module roles": by-name Module.Role references, stored
		// beside (not instead of) the condition above. Never read, so describe
		// → exec removed the restriction (mendixlabs/mxcli#1460).
		for _, r := range getBsonArrayElements(cvs["ModuleRoles"]) {
			if name, ok := r.(string); ok && name != "" {
				widget.VisibleRoles = append(widget.VisibleRoles, name)
			}
		}
	}
	if ces, ok := w["ConditionalEditabilitySettings"].(map[string]any); ok && ces != nil {
		if expr, ok := ces["Expression"].(string); ok && expr != "" {
			widget.EditableIf = expr
		}
		// Attribute-based, stored as the visibility form is: one
		// Enumerations$Condition per value, EditableVisible marking the values
		// that make the widget editable. Never read, so describe printed a bare
		// `Editable: Conditional` that check refuses (ako/mxcli#2111).
		if attr, ok := ces["Attribute"].(string); ok && attr != "" {
			widget.EditableAttr = describeAttr(ctx, attr)
			if param := parameterSourceName(ces["SourceVariable"]); param != "" {
				widget.EditableAttr = parameterAttributeMDL(param, attr)
			}
			for _, c := range getBsonArrayElements(ces["Conditions"]) {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				if editable, _ := cm["EditableVisible"].(bool); !editable {
					continue
				}
				v, _ := cm["AttributeValue"].(string)
				if v == emptyConditionValue {
					v = "empty"
				}
				widget.EditableValues = append(widget.EditableValues, v)
			}
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
		// Which regions run the full width (ako/mxcli#1703).
		widget.ScrollLayoutMode, _ = w["LayoutMode"].(string)
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
		extractConditionalSettings(ctx, &widget, w)
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
			if tm, ok := region["ToggleMode"].(string); ok {
				child.RegionToggleMode = tm
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
		extractConditionalSettings(ctx, &widget, w)
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
			// Forms$TabPage carries its own visibility (#1465); it has no
			// editability, so only the visibility half is read.
			extractConditionalSettings(ctx, &tabPage, tpMap)
			readTabPageBadge(ctx, tpMap, &tabPage)
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
			// The HTML element it renders as (mendixlabs/mxcli#1582).
			if rm, ok := w["RenderMode"].(string); ok {
				widget.RenderMode = rm
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
		extractConditionalSettings(ctx, &widget, w)
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
	extractConditionalSettings(ctx, &widget, w)

	// Extract CSS class, style, and design properties from Appearance
	extractAppearance(&widget, w)

	switch typeName {
	case "Forms$LayoutGrid", "Pages$LayoutGrid":
		widget.LayoutGridWidth = extractString(w["Width"])
		widget.Rows = parseLayoutGridRows(ctx, w, inheritedCtx)
		return []rawWidget{widget}

	case "Forms$NavigationTree", "Pages$NavigationTree",
		"Forms$MenuBar", "Pages$MenuBar",
		"Forms$SimpleMenuBar", "Pages$SimpleMenuBar":
		// What the widget renders is one level down, in MenuSource: a profile
		// in a Forms$NavigationSource, or a menu document in a
		// Forms$MenuDocumentSource — Atlas's phone bottom bar is the latter.
		if src, ok := w["MenuSource"].(map[string]any); ok {
			if p, ok := src["NavigationProfile"].(string); ok {
				widget.NavigationProfile = p
			}
			if m, ok := src["Menu"].(string); ok {
				widget.Menu = m
			}
		}
		if o, ok := w["Orientation"].(string); ok {
			widget.MenuOrientation = o
		}
		return []rawWidget{widget}

	case "Forms$DynamicText", "Pages$DynamicText":
		widget.Content = extractTextContent(ctx, w, "Content")
		widget.Parameters = extractClientTemplateParameters(ctx, w, "Content")
		if rm, ok := w["RenderMode"].(string); ok {
			widget.RenderMode = rm
		}
		widget.NativeTextStyle = extractString(w["NativeTextStyle"])
		return []rawWidget{widget}

	case "Forms$ActionButton", "Pages$ActionButton":
		widget.Caption = extractButtonCaption(ctx, w)
		widget.Parameters = extractButtonCaptionParameters(ctx, w)
		widget.Tooltip = extractTextsText(ctx, w, "Tooltip")
		widget.ButtonStyle = extractButtonStyle(ctx, w)
		widget.Action = extractButtonAction(ctx, w)
		// RenderType "Link" is a linkbutton; the emitter uses this to choose the
		// `linkbutton` keyword over `actionbutton` for a clean DESCRIBE roundtrip.
		if rt, ok := w["RenderType"].(string); ok {
			widget.RenderMode = rt
		}
		widget.Icon, widget.IconType, widget.IconCode = extractButtonIcon(w)
		widget.UnsayableOutputMappings = unsayableOutputMappings(w["Action"])
		return []rawWidget{widget}

	case "Forms$Header", "Pages$Header":
		// A phone/tablet layout's header bar: two widget lists, described as
		// its two sides `region left` / `region right`.
		for _, side := range []struct{ key, name string }{{"LeftWidgets", "left"}, {"RightWidgets", "right"}} {
			child := rawWidget{Type: "Forms$HeaderSide", Name: side.name}
			for _, c := range getBsonArrayElements(w[side.key]) {
				if cMap, ok := c.(map[string]any); ok {
					child.Children = append(child.Children, parseRawWidget(ctx, cMap, inheritedCtx)...)
				}
			}
			if len(child.Children) > 0 {
				widget.Children = append(widget.Children, child)
			}
		}
		return []rawWidget{widget}

	case "Forms$SidebarToggleButton", "Pages$SidebarToggleButton":
		// An action button's face without the action (mendixlabs/mxcli#1220).
		widget.Caption = extractButtonCaption(ctx, w)
		widget.Parameters = extractButtonCaptionParameters(ctx, w)
		widget.Tooltip = extractTextsText(ctx, w, "Tooltip")
		widget.ButtonStyle = extractButtonStyle(ctx, w)
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
		// A data view stores `Editability`, where input widgets store `Editable`.
		// Unread, a read-only data view described as an editable one and
		// re-executing the description widened it (ako/mxcli#1676).
		widget.Editable = extractString(w["Editability"])
		// Below Mendix 10.7 it is the boolean Editable (ako/mxcli#1209).
		if editable, ok := w["Editable"].(bool); ok && !editable && widget.Editable == "" {
			widget.Editable = "Never"
		}
		widget.ReadOnlyStyle = extractReadOnlyStyle(ctx, w)
		widget.ShowFooter, _ = w["ShowFooter"].(bool)
		widget.Children = withQualifiedAttrs(ctx, widget.DataSource, func() []rawWidget {
			return parseDataViewChildren(ctx, w, widget.EntityContext)
		})
		return []rawWidget{widget}

	case "Forms$TextBox", "Pages$TextBox":
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractInputAttribute(ctx, w)
		widget.Placeholder = extractPlaceholderText(ctx, w)
		widget.Editable = extractEditable(ctx, w)
		widget.ReadOnlyStyle = extractReadOnlyStyle(ctx, w) // ako/mxcli#977
		widget.IsPassword, _ = w["IsPasswordBox"].(bool)
		widget.KeyboardType, _ = w["KeyboardType"].(string)
		widget.ValidationExpression, widget.ValidationMessage = extractWidgetValidation(ctx, w)
		widget.SubmitBehaviour, widget.SubmitOnInputDelay = extractSubmitBehaviour(w)
		widget.OnChange = extractOnChangeAction(ctx, w)
		widget.OnEnter, widget.OnLeave = extractFocusEventActions(ctx, w)
		// Unread, describe → exec replaced a stored enter-key action with
		// the empty one (ako/mxcli#1743).
		widget.OnEnterKeyPress = renderClientActionMDL(ctx, actionMapForKey(w, "OnEnterKeyPressAction"))
		widget.Formatting = describeInputFormatting(ctx, "textbox", w)
		return []rawWidget{widget}

	case "Forms$TextArea", "Pages$TextArea":
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractInputAttribute(ctx, w)
		// Read like the textbox's; it was not, so describe -> exec deleted a
		// textarea's placeholder with all its translations (ako/mxcli#705).
		widget.Placeholder = extractPlaceholderText(ctx, w)
		widget.Editable = extractEditable(ctx, w)
		widget.ReadOnlyStyle = extractReadOnlyStyle(ctx, w) // ako/mxcli#977
		widget.SubmitBehaviour, widget.SubmitOnInputDelay = extractSubmitBehaviour(w)
		widget.OnChange = extractOnChangeAction(ctx, w)
		widget.OnEnter, widget.OnLeave = extractFocusEventActions(ctx, w)
		if v, ok := w["NumberOfLines"]; ok {
			n := int64(extractInt(v))
			widget.NumberOfLines = &n
		}
		if b, ok := w["Autocomplete"].(bool); ok {
			widget.Autocomplete = &b
		}
		return []rawWidget{widget}

	case "Forms$FileManager", "Pages$FileManager":
		// Printed as a comment until mendixlabs/mxcli#151, so describe -> exec
		// dropped every file manager on the page.
		widget.Caption = extractLabelText(ctx, w)
		widget.Editable = extractEditable(ctx, w)
		widget.FileManagerProps = describeFileManagerProps(ctx, w)
		return []rawWidget{widget}

	case "Forms$DatePicker", "Pages$DatePicker":
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractInputAttribute(ctx, w)
		widget.Editable = extractEditable(ctx, w)
		widget.ReadOnlyStyle = extractReadOnlyStyle(ctx, w) // ako/mxcli#977
		widget.OnChange = extractOnChangeAction(ctx, w)
		widget.OnEnter, widget.OnLeave = extractFocusEventActions(ctx, w)
		widget.Formatting = describeInputFormatting(ctx, "datepicker", w)
		return []rawWidget{widget}

	case "Forms$RadioButtons", "Pages$RadioButtons", "Forms$RadioButtonGroup", "Pages$RadioButtonGroup":
		widget.Type = "Forms$RadioButtons" // Normalize type
		if h, ok := w["RenderHorizontal"].(bool); ok && !h {
			widget.RenderVertical = true // ako/mxcli#2106
		}
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractInputAttribute(ctx, w)
		widget.Editable = extractEditable(ctx, w)
		widget.ReadOnlyStyle = extractReadOnlyStyle(ctx, w) // ako/mxcli#977
		widget.OnChange = extractOnChangeAction(ctx, w)
		widget.OnEnter, widget.OnLeave = extractFocusEventActions(ctx, w)
		return []rawWidget{widget}

	case "Forms$CheckBox", "Pages$CheckBox":
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractInputAttribute(ctx, w)
		widget.Editable = extractEditable(ctx, w)
		widget.ReadOnlyStyle = extractReadOnlyStyle(ctx, w)
		widget.ShowLabel = extractShowLabel(ctx, w)
		widget.LabelPosition, _ = w["LabelPosition"].(string) // ako/mxcli#2106
		widget.OnChange = extractOnChangeAction(ctx, w)
		widget.OnEnter, widget.OnLeave = extractFocusEventActions(ctx, w)
		return []rawWidget{widget}

	case "CustomWidgets$CustomWidget":
		widget.Caption = extractLabelText(ctx, w)
		widget.Content = extractCustomWidgetAttribute(ctx, w)
		widget.RenderMode = extractCustomWidgetType(ctx, w) // Store widget type in RenderMode
		widget.WidgetID = extractCustomWidgetID(ctx, w)
		// A pluggable widget declaring the Editability system property stores
		// it on the CustomWidget, like a text box. Unread, a combo box's
		// `Editable: Never` described as nothing and re-exec widened it.
		widget.Editable = extractEditable(ctx, w)
		// For ComboBox, extract datasource and association attribute for association mode.
		// In association mode the Attribute binding is stored as EntityRef (not AttributeRef),
		// so we must use extractCustomWidgetPropertyAssociation instead of the generic scan.
		if widget.RenderMode == "combobox" && comboBoxIsDatabaseMode(ctx, w) {
			describeComboBoxDatabaseMode(ctx, w, &widget)
		} else if widget.RenderMode == "combobox" {
			widget.DataSource = extractComboBoxDataSource(ctx, w)
			if widget.DataSource != nil {
				widget.Content = associationRefForContext(extractCustomWidgetPropertyAssociationQN(ctx, w, "attributeAssociation"), inheritedCtx)
				// The association starts at a page or snippet parameter:
				// `$Param.Module.Assoc`, qualified, since no entity is in scope
				// to shorten it against.
				if param := customWidgetPropertyParameter(w, "attributeAssociation"); param != "" && widget.Content != "" {
					widget.Content = "$" + param + "." + extractCustomWidgetPropertyAssociationQN(ctx, w, "attributeAssociation")
				}
				widget.CaptionAttribute = extractCustomWidgetPropertyAttributeRef(ctx, w, "optionsSourceAssociationCaptionAttribute")
				// The caption can be an EXPRESSION instead (Studio Pro's "Caption
				// type: Expression"). Reading only the attribute form dropped it,
				// and exec wrote a combobox with no caption — CE0642 "Property
				// 'Caption' is required" on Administration.Account_New (#664).
				if extractCustomWidgetPropertyString(ctx, w, "optionsSourceAssociationCaptionType") == "expression" {
					widget.CaptionExpression = extractCustomWidgetPropertyExpression(w, "optionsSourceAssociationCaptionExpression")
					widget.CaptionAttribute = ""
				}
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
		// The combo box's other action slots — onEnterEvent, onLeaveEvent,
		// onChangeFilterInputEvent — in every mode, by their own keys
		// (ako/mxcli#854). The change slot of either mode is not among them:
		// it is the `OnChange:` read above.
		if widget.RenderMode == "combobox" {
			widget.NamedActions = namedActionSlotsOf(ctx, w)
		}
		// The drop-down filter's association mode is the same shape as the
		// ComboBox's, on differently-named properties: `baseType` selects it and
		// the reference is stored as an EntityRef, not an AttributeRef. Without
		// this the filter described back as a bare `dropdownfilter name` and a
		// describe→edit→exec cycle silently reverted it to attribute mode. (#830)
		if widget.RenderMode == "dropdownfilter" &&
			extractCustomWidgetPropertyString(ctx, w, "baseType") == "ref" {
			widget.DataSource = extractCustomWidgetPropertyDataSource(ctx, w, "refOptions")
			widget.Content = associationRefForContext(extractCustomWidgetPropertyAssociationQN(ctx, w, "refEntity"), inheritedCtx)
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
			// Every other grid setting that differs from its default, by the
			// widget's own key, and the widget slots other than the control bar
			// (the empty-state placeholder). Without them a describe -> exec
			// cycle reset the personalization, selection, paging and label
			// settings to the widget's defaults.
			widget.ExplicitProperties, widget.OmittedProps = extractDataGrid2ByKeyProps(ctx, w, inheritedCtx, widget.EntityContext)
			for _, slot := range extractChildSlots(ctx, w, widget.EntityContext) {
				if slot.PropertyKey != "filtersPlaceholder" {
					widget.ChildSlots = append(widget.ChildSlots, slot)
				}
			}
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
			widget.Children = withQualifiedAttrs(ctx, widget.DataSource, func() []rawWidget {
				return extractGalleryContent(ctx, w, widget.EntityContext)
			})
			widget.FilterWidgets = extractGalleryFilters(ctx, w)
			// Pagination, page size, state storage and the rest of the gallery's
			// settings that differ from their defaults, by the widget's own key,
			// and the slots other than `template` and `filter` (the empty-state
			// placeholder).
			widget.ExplicitProperties, widget.OmittedProps = extractGalleryByKeyProps(ctx, w, inheritedCtx, widget.EntityContext)
			for _, slot := range extractChildSlots(ctx, w, widget.EntityContext) {
				if slot.PropertyKey != "content" && slot.PropertyKey != "filtersPlaceholder" {
					widget.ChildSlots = append(widget.ChildSlots, slot)
				}
			}
			// The row click and the other two action slots, read the way the
			// data grid's are: the click by SOURCE, the rest by the widget's own
			// key. Without them the write fix for #842 was one-way — describe →
			// exec deleted the row action — and the pluggable passthrough
			// (#721 L4), which keeps a stored widget whose statement equals its
			// description, could not tell a statement that removed the action
			// from one that kept it.
			widget.OnClick = renderClientActionMDL(ctx, customWidgetActionForSource(ctx, w, "OnClick"))
			widget.NamedActions = namedActionSlotsOf(ctx, w)
			// The two settings that make a row click legal (MDL-WIDGET36): a
			// selection of None, which extractGallerySelection reads as unset
			// while exec's default is Single, and a double-click trigger, which
			// exec writes as single unless told otherwise.
			if widget.Selection == "" && gallerySelectionIsNone(w) {
				widget.Selection = "None"
			}
			if trig := extractCustomWidgetPropertyString(ctx, w, "onClickTrigger"); trig != "" && trig != "single" {
				widget.OnClickTrigger = trig
			}
		}
		// For filter widgets, extract filter attributes and expression
		if widget.RenderMode == "textfilter" || widget.RenderMode == "numberfilter" || widget.RenderMode == "dropdownfilter" || widget.RenderMode == "datefilter" {
			widget.FilterAttributes = extractFilterAttributes(ctx, w)
			widget.FilterExpression = extractFilterExpression(ctx, w)
			// The filter's `onChange`, which `OnChange:` writes (ako/mxcli#854).
			// Unread, a Studio Pro-authored filter action described as nothing:
			// the rewrite patch kept it only while the statement left the slot
			// alone, and no statement could say what it held.
			widget.OnChange = renderClientActionMDL(ctx, customWidgetActionForSource(ctx, w, "OnChange"))
			// The drop-down filter keeps its ONE custom attribute on `attr`
			// (attrChoice "linked"), not in an `attributes` list. Read it back
			// as the `attributes: [A]` it is written with, or a describe→exec
			// cycle loses it (#1519). Under "auto" the column supplies it.
			if widget.RenderMode == "dropdownfilter" && len(widget.FilterAttributes) == 0 &&
				extractCustomWidgetPropertyString(ctx, w, "attrChoice") == "linked" {
				if attr := extractCustomWidgetPropertyAttributeRef(ctx, w, "attr"); attr != "" {
					widget.FilterAttributes = []string{attr}
				}
			}
		}
		// Every other setting of a combo box or a data grid filter, by the
		// widget's own key. Their readers above know only the keys MDL has a
		// keyword for, so a stored `customEditabilityExpression` or a filter's
		// `defaultValue` expression — written by exec's explicit-property pass
		// — described back as nothing, and a describe → exec cycle reset it
		// (mendixlabs/mxcli#1586).
		if skip, ok := sharedBranchByKeySkip[widget.RenderMode]; ok {
			ownEntity := ""
			if widget.DataSource != nil {
				ownEntity = dataSourceEntityContext(ctx, widget.DataSource)
			}
			widget.ExplicitProperties, widget.OmittedProps = extractSharedBranchByKeyProps(ctx, w, skip, inheritedCtx, ownEntity)
			if widget.RenderMode == "combobox" {
				database := comboBoxIsDatabaseMode(ctx, w)
				if database {
					ownEntity = ""
					if len(widget.NamedDataSources) > 0 {
						ownEntity = dataSourceEntityContext(ctx, widget.NamedDataSources[0].DataSource)
					}
					widget.ExplicitProperties, widget.OmittedProps = extractSharedBranchByKeyProps(ctx, w, comboBoxDatabaseByKeySkip, inheritedCtx, ownEntity)
					widget.ExplicitProperties = append(widget.ExplicitProperties, comboBoxDatabaseSelection(ctx, w)...)
				}
				widget.ExplicitProperties, widget.OmittedProps = withoutUndescribedComboBoxModes(widget.ExplicitProperties, widget.OmittedProps, database)
				// The custom content of the mode described, as the block that
				// writes it, its widgets bound to the option entity
				// (ako/mxcli#1744); content of any other slot is still named.
				widget.ChildSlots = comboBoxCustomContent(ctx, w, comboBoxDescribedMode(&widget, database), ownEntity)
				widget.HiddenContent = withoutDescribedContent(comboBoxHiddenContent(w), widget.ChildSlots)
			}
		}
		// For pluggable Image widget, extract image-specific properties
		if widget.RenderMode == "image" {
			extractImageProperties(ctx, w, &widget)
		}
		// For generic pluggable widgets (not handled by dedicated extractors above),
		// extract all non-default properties as explicit key-value pairs.
		if !isKnownCustomWidgetType(widget.RenderMode) {
			widget.ExplicitProperties = extractExplicitProperties(ctx, w, inheritedCtx)
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
		// namedCustomWidgetDataSources, not firstObjectPropertyDataSource: the
		// latter stops at the first property whose DataSource parses at all, and
		// a File Uploader has one carrying no reference ahead of its real one.
		//
		// One configured datasource keeps the generic `DataSource:` clause it has
		// always been described with — which is every widget shipped today, so
		// their output is unchanged. Several are emitted under their own schema
		// keys, because one clause cannot say which binding is which.
		//
		// This runs even when a per-widget extractor above already found one,
		// because those extractors each know about ONE of their widget's
		// datasources: the drop-down filter's reads `refOptions` and never
		// `linkedDs`, so a filter carrying both described back as single-source
		// and a rewrite dropped the grid's list. Whether a widget is
		// multi-source is a property of the stored document, not of which
		// extractor happened to run.
		switch named := namedCustomWidgetDataSources(w); {
		case len(named) > 1:
			widget.NamedDataSources = named
			// One context for a widget with several is a guess, and it is only a
			// guess about how CHILD widgets render. The widget's own attribute
			// properties are emitted as bare names (shortAttributeName takes the
			// last segment and never consults this), and on re-exec each binds
			// against its own datasource's entity through the template's
			// DataSourceProperty link — which is what makes this round trip at all.
			if widget.EntityContext == "" {
				widget.EntityContext = dataSourceEntityContext(ctx, named[0].DataSource)
			}
			// w.DataSource is left as an extractor set it: several emitter
			// branches are gated on it being non-nil, and
			// appendWidgetDataSources prefers the named set, so it is not
			// emitted twice.
		case len(named) == 1 && widget.DataSource == nil:
			widget.DataSource = named[0].DataSource
			if widget.EntityContext == "" {
				widget.EntityContext = dataSourceEntityContext(ctx, named[0].DataSource)
			}
			// One CONFIGURED is not one DECLARED. The builder refuses the
			// generic clause on a widget whose definition maps several
			// datasources, whatever is configured — a File Uploader in files
			// mode still declares `associatedImages` — so the generic spelling
			// made every such page's description unexecutable (#1199). Name the
			// key whenever the schema declares more than one the author can set.
			if named[0].Key != "" && declaresSeveralAuthorableDataSources(w) {
				widget.NamedDataSources = named
			}
		}
		return []rawWidget{widget}

	case "Forms$StaticImageViewer", "Pages$StaticImageViewer":
		// Mendix stores WHICH image on a by-name reference to Images$Image: the
		// three-part Module.Collection.Image name of an entry in an image
		// collection, as a plain string (unset is "", never null). This is the
		// same reference the pluggable `image` widget carries on its
		// WidgetValue, so it re-uses ImageObject and emits the same clause.
		//
		// The widget had no case here at all, so DESCRIBE fell through to the
		// unknown-type note and a Selection helper's three mandatory custom
		// slots came back empty — CE0642 on rebuild (mendixlabs/mxcli#1057).
		if image, ok := w["Image"].(string); ok && image != "" {
			widget.ImageObject = image
		}
		if width := extractInt(w["Width"]); width > 0 {
			widget.ImageWidth = strconv.Itoa(width)
		}
		if height := extractInt(w["Height"]); height > 0 {
			widget.ImageHeight = strconv.Itoa(height)
		}
		// The units and Responsive round-trip too, because the writer honours
		// them as of this change. Emitting the widget WITHOUT them would trade
		// the old visible note for a silent normalisation to Auto/responsive.
		if u, ok := w["WidthUnit"].(string); ok {
			widget.WidthUnit = strings.ToLower(u)
		}
		if u, ok := w["HeightUnit"].(string); ok {
			widget.HeightUnit = strings.ToLower(u)
		}
		if responsive, ok := w["Responsive"].(bool); ok && !responsive {
			widget.Responsive = "false"
		}
		// Forms$StaticImageViewer.ClickAction. The builder started filling it in
		// ako/mxcli#512 and the writer already serialised it; nothing read it
		// back, because nothing read this widget back at all.
		if onClick := asActionMap(w["ClickAction"]); onClick != nil {
			widget.Action = extractButtonAction(ctx, map[string]any{"Action": onClick})
		}
		return []rawWidget{widget}

	case "Forms$ImageViewer", "Pages$ImageViewer":
		// The DYNAMIC image. Its binding is a Forms$ImageViewerSource, which the
		// shared datasource reader already knows (entityBackedSourceTypes), so
		// the entity CE0489 asks for costs nothing to read back.
		//
		// Nothing read this widget at all before, and the writer bound it to no
		// entity, so every dynamic image mxcli authored failed the build and
		// every stored one was dropped by describe -> exec.
		if ds, ok := w["DataSource"].(map[string]any); ok {
			widget.DataSource = parseDataSource(ds)
			if widget.DataSource != nil && widget.DataSource.Reference != "" {
				widget.EntityContext = dataSourceEntityContext(ctx, widget.DataSource)
			}
		}
		if widget.EntityContext == "" {
			widget.EntityContext = inheritedCtx
		}
		// The fallback image, a by-name reference like the static image's.
		if fallback, ok := w["DefaultImage"].(string); ok && fallback != "" {
			widget.DefaultImage = fallback
		}
		if width := extractInt(w["Width"]); width > 0 {
			widget.ImageWidth = strconv.Itoa(width)
		}
		if height := extractInt(w["Height"]); height > 0 {
			widget.ImageHeight = strconv.Itoa(height)
		}
		if u, ok := w["WidthUnit"].(string); ok {
			widget.WidthUnit = strings.ToLower(u)
		}
		if u, ok := w["HeightUnit"].(string); ok {
			widget.HeightUnit = strings.ToLower(u)
		}
		if responsive, ok := w["Responsive"].(bool); ok && !responsive {
			widget.Responsive = "false"
		}
		// ShowAsThumbnail and OnClickEnlarge reuse the vocabulary the PLUGGABLE
		// image widget already describes with (DisplayAs, OnClickType), so one
		// property name means one thing across all three image widgets.
		if thumb, ok := w["ShowAsThumbnail"].(bool); ok && thumb {
			widget.DisplayAs = "thumbnail"
		}
		if enlarge, ok := w["OnClickEnlarge"].(bool); ok && enlarge {
			widget.OnClickType = "enlarge"
		}
		if onClick := asActionMap(w["ClickAction"]); onClick != nil {
			widget.Action = extractButtonAction(ctx, map[string]any{"Action": onClick})
		}
		return []rawWidget{widget}

	case "Forms$DataGrid":
		// Unauthorable, but its parts are addressable by their stored names
		// (mendixlabs/mxcli#1459); outputLegacyGridV3 prints them as comments.
		if ds, ok := w["DataSource"].(map[string]any); ok {
			widget.DataSource = parseDataSource(ds)
		}
		widget.LegacyGrid = parseLegacyGrid(ctx, w)
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
		widget.SnippetParams = extractSnippetCallParams(w)
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
		// "On click" — Pages$ListView.ClickAction. The write half landed without
		// this and the action vanished on the next describe → exec, which is the
		// half-shell trap this repo keeps recording: a construct written, valid,
		// and silently dropped on read-back (ako/mxcli#512).
		if onClick := asActionMap(w["ClickAction"]); onClick != nil {
			widget.Action = extractButtonAction(ctx, map[string]any{"Action": onClick})
		}
		widget.Children = withQualifiedAttrs(ctx, widget.DataSource, func() []rawWidget {
			return parseListViewContent(ctx, w, widget.EntityContext)
		})
		return []rawWidget{widget}

	case "Forms$Table":
		widget.TableCells = parseTableCells(w)
		return []rawWidget{widget}

	default:
		// For unknown types, just note them
		return []rawWidget{widget}
	}
}

// parseTableCells reads a legacy table's cells in stored order. A cell's
// TopRowIndex / LeftColumnIndex are 0-based; the address ALTER PAGE takes is
// 1-based.
func parseTableCells(w map[string]any) []rawTableCell {
	var cells []rawTableCell
	for _, c := range getBsonArrayElements(w["Cells"]) {
		cell, ok := c.(map[string]any)
		if !ok {
			continue
		}
		rc := rawTableCell{Row: extractInt(cell["TopRowIndex"]) + 1, Column: extractInt(cell["LeftColumnIndex"]) + 1}
		for _, cw := range getBsonArrayElements(cell["Widgets"]) {
			m, ok := cw.(map[string]any)
			if !ok {
				continue
			}
			label, _ := m["Name"].(string)
			if label == "" {
				label, _ = m["$Type"].(string)
			}
			rc.Widgets = append(rc.Widgets, label)
		}
		cells = append(cells, rc)
	}
	return cells
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
		row := rawWidgetRow{SpacingBetweenColumns: true}
		// A row's and a column's appearance and alignment were never read, so a
		// describe → exec reset every one of them (the Atlas "Flex container" /
		// "Column gap" / "Cards style" a Studio Pro page sets on its grid).
		extractAppearance(&row.Appearance, rMap)
		// A row's own visibility (mendixlabs/mxcli#1461); a row has no
		// editability, and a column no visibility, in the metamodel.
		extractConditionalSettings(ctx, &row.Appearance, rMap)
		row.VerticalAlignment, _ = rMap["VerticalAlignment"].(string)
		row.HorizontalAlignment, _ = rMap["HorizontalAlignment"].(string)
		if v, ok := rMap["SpacingBetweenColumns"].(bool); ok {
			row.SpacingBetweenColumns = v
		}
		cols := getBsonArrayElements(rMap["Columns"])
		for _, c := range cols {
			cMap, ok := c.(map[string]any)
			if !ok {
				continue
			}
			col := rawWidgetColumn{}
			extractAppearance(&col.Appearance, cMap)
			col.VerticalAlignment, _ = cMap["VerticalAlignment"].(string)
			// Widths: 1..12, -1 auto-fill, -2 auto-fit content. Studio Pro
			// stores them as int64, so read them width-agnostically — an
			// `.(int32)` here missed every stored width and describe printed
			// AutoFill for all of them (ako/mxcli#721 L1).
			if v, ok := cMap["Weight"]; ok {
				col.Width = bsonInt(v)
			} else if v, ok := cMap["DesktopWeight"]; ok {
				col.Width = bsonInt(v)
			}
			col.TabletWidth = bsonInt(cMap["TabletWeight"])
			col.PhoneWidth = bsonInt(cMap["PhoneWeight"])
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

// extractAppearance copies the CSS class, inline style, dynamic classes and
// design properties of an element's Forms$Appearance onto dst.
func extractAppearance(dst *rawWidget, w map[string]any) {
	appearance, ok := w["Appearance"].(map[string]any)
	if !ok {
		return
	}
	if class, ok := appearance["Class"].(string); ok && class != "" {
		dst.Class = class
	}
	if style, ok := appearance["Style"].(string); ok && style != "" {
		dst.Style = style
	}
	if dc, ok := appearance["DynamicClasses"].(string); ok && dc != "" {
		dst.DynamicClasses = dc
	}
	dst.DesignProperties = extractDesignProperties(appearance)
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

		// The item's own Forms$Appearance: Class, Style, DynamicClasses. It was
		// read into ButtonStyle, a property an item does not have, so describe
		// printed `ButtonStyle: <class>` and exec dropped the class.
		if appearance, ok := itemMap["Appearance"].(map[string]any); ok {
			rw.Class, _ = appearance["Class"].(string)
			rw.Style, _ = appearance["Style"].(string)
			rw.DynamicClasses, _ = appearance["DynamicClasses"].(string)
		}
		// The item's own visibility — 13 of the report's 97 role-locked
		// widgets were items of one navigation list (#1460).
		extractConditionalSettings(ctx, &rw, itemMap)

		result = append(result, rw)
	}
	return result
}

// extractNavigationListItemAction renders a NavigationListItem's action. It is
// the same client action a button stores (a page action is a Forms$FormAction
// whose page sits in FormSettings or PageSettings), so it goes through the
// shared renderer. A private copy here printed the legacy `show_page 'M.P'`,
// which does not parse (ako/mxcli#950).
func extractNavigationListItemAction(ctx *ExecContext, w map[string]any) string {
	return extractButtonAction(ctx, w)
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
		// The footer is a region of the data view, stored without a name: it is
		// printed unnamed and addressed as `<dataview>.footer` (ako/mxcli#528).
		// The `footer1` printed here before named nothing ALTER could find.
		footer := rawWidget{Type: "Footer"}
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
//
// Below Mendix 10.11 the widget stores Placeholder, a plain Texts$Text, instead
// (ako/mxcli#1209); extractTextFromTemplate reads its Items the same way.
func extractPlaceholderText(ctx *ExecContext, w map[string]any) string {
	placeholderTemplate, ok := w["PlaceholderTemplate"].(map[string]any)
	if !ok {
		placeholderTemplate, ok = w["Placeholder"].(map[string]any)
	}
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

// extractWidgetValidation reads the two fields of a Forms$WidgetValidation: the
// expression Mendix evaluates over $value, and the message shown when it fails.
//
// An empty expression means the widget has no validation — Studio Pro stores the
// element either way — so both come back empty and the describer emits nothing.
// Emitting a clause for a stored-but-empty validation would be the "invents"
// shape: it puts something in the user's script that they did not write.
func extractWidgetValidation(ctx *ExecContext, w map[string]any) (expression, message string) {
	v, ok := w["Validation"].(map[string]any)
	if !ok || v == nil {
		return "", ""
	}
	expression = extractString(v["Expression"])
	if expression == "" {
		return "", ""
	}
	// Message is a bare Texts$Text (Items[] of translations), not a
	// Forms$ClientTemplate — extractTextFromTemplate's fallback branch handles
	// exactly that shape.
	if msg, ok := v["Message"].(map[string]any); ok {
		message = extractTextFromTemplate(ctx, msg)
	}
	return expression, message
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

// extractAttributeRef extracts the attribute reference from an input widget as
// the short form MDL accepts: a bare name for an own attribute, or
// `Assoc/.../Attr` when the binding navigates associations.
//
// This used to return the last segment of AttributeRef.Attribute and ignore
// AttributeRef.EntityRef entirely, which silently dropped every association
// hop. Measured on ako/TestApp's Rules.RuleAction_NewEdit, whose text box binds
// Rules.BusinessRule.Name over Rules.RuleAction_BusinessRule: DESCRIBE emitted
// `Attribute: Name`, and Rules.RuleAction has no Name — so a describe → exec
// round trip rebound the widget to nothing. `check` stayed clean and the damage
// surfaced at build time as CE1613, or in a browser as a blank field
// (ako/mxcli#529).
//
// columnAttributeFromRef already did this correctly for DataGrid2 columns (bug
// 7), so one page could round-trip a grid column and destroy a text box beside
// it. Sharing that function is the point: two readers of one BSON shape is how
// the halves drifted apart to begin with.
func extractAttributeRef(ctx *ExecContext, w map[string]any) string {
	attrRef, ok := w["AttributeRef"].(map[string]any)
	if !ok {
		return ""
	}
	if _, ok := attrRef["Attribute"].(string); !ok {
		return ""
	}
	path := columnAttributeFromRef(ctx, attrRef)
	if !strings.Contains(path, "/") {
		// A plain binding: qualified where no entity is in scope (describeAttr).
		return describeAttr(ctx, extractString(attrRef["Attribute"]))
	}
	return path
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

// extractSnippetCallParams renders a snippet call's stored
// Forms$SnippetParameterMapping list as `Param = $variable, …`, or "" when the
// call has none (it is then bound by its data context). Describe printed
// nothing for them, so describe → exec lost every mapping — or refused the call
// outright, the parameter being required: TestApp's WorkflowCommons has 27
// passing a snippet parameter on and 5 passing a page parameter (#826).
//
// A mapping whose variable is only a Widget is left out: MDL has no argument
// syntax for it, and no Studio Pro-authored mapping measured has one.
func extractSnippetCallParams(w map[string]any) string {
	formCall, ok := w["FormCall"].(map[string]any)
	if !ok {
		return ""
	}
	var params []string
	for _, m := range getBsonArrayElements(formCall["ParameterMappings"]) {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		ref := extractString(mm["Parameter"])
		name := ref[strings.LastIndex(ref, ".")+1:]
		// flowArgVariableValue: a data view's object is named in Widget
		// (ako/mxcli#1968), which pageVariableArgValue does not read.
		value := flowArgVariableValue(mm["Variable"])
		if value == "" {
			value = strings.TrimSpace(extractString(mm["Argument"]))
		}
		if name == "" || value == "" {
			continue
		}
		params = append(params, visitor.ParameterNameSpelling(name)+" = "+value)
	}
	return strings.Join(params, ", ")
}

// extractInputAttribute is an input widget's `Attribute:` value. A binding read
// through a named data view — Studio Pro's SourceVariable {Widget: dataView1,
// PageParameter: Account} (PedApp Administration.Account_Edit) — prints as
// `$dataView1.FullName`, which the builder resolves back to the same pair;
// describe used to print the bare attribute and exec dropped the variable
// (ako/mxcli#826). Every other binding reads as before.
//
// An input bound directly to a page variable stores no AttributeRef and names
// the variable in SourceVariable.LocalVariable; it prints as `$ShowAll`, the
// spelling the builder takes. describe used to print it unbound, so a round
// trip silently cut the binding (mendixlabs/mxcli#1235).
func extractInputAttribute(ctx *ExecContext, w map[string]any) string {
	sv, _ := w["SourceVariable"].(map[string]any)
	widget := extractString(sv["Widget"])
	attrRef, _ := w["AttributeRef"].(map[string]any)
	if local := extractString(sv["LocalVariable"]); local != "" && widget == "" && attrRef == nil {
		return "$" + local
	}
	// Read straight from a page or snippet parameter — a widget outside every
	// data container (TestApp Snip_WorkflowUserTaskView_Details): `$Param.Attr`.
	// Bare, it re-executed with no SourceVariable, or was refused at the root.
	if param := parameterSourceName(w["SourceVariable"]); param != "" && attrRef != nil {
		if attr := extractString(attrRef["Attribute"]); attr != "" && attrRef["EntityRef"] == nil {
			return parameterAttributeMDL(param, attr)
		}
	}
	// An association path read from a named object — a parameter, or a data
	// view through its Widget slot — is `$Root/Assoc/Attr`. Printed without
	// the root it was refused at the snippet root (MDL-WIDGET34), or bound
	// against the wrong context (ako/mxcli#833).
	if root := firstNonEmpty(widget, parameterSourceName(w["SourceVariable"])); root != "" && attrRef != nil && attrRef["EntityRef"] != nil {
		if path := columnAttributeFromRef(ctx, attrRef); strings.Contains(path, "/") {
			return "$" + root + "/" + path
		}
	}
	if widget == "" || attrRef == nil {
		return extractAttributeRef(ctx, w)
	}
	path := columnAttributeFromRef(ctx, attrRef)
	if path == "" || strings.Contains(path, "/") {
		return extractAttributeRef(ctx, w)
	}
	return "$" + widget + "." + path
}

func extractSnippetRef(ctx *ExecContext, w map[string]any) string {
	// First try the FormCall.Form path (used for BY_NAME_REFERENCE)
	if formCall, ok := w["FormCall"].(map[string]any); ok {
		if form, ok := formCall["Form"].(string); ok && form != "" {
			return form
		}
		// Try binary ID and resolve to name
		if formID := extractBinaryID(formCall["Form"]); formID != "" {
			// Try to resolve the snippet name from ID: by its header,
			// without decoding every snippet (#1850).
			snippets, err := snippetHeaders(ctx.Backend)
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
			return parseFlatDesignProperty(key, itemMap)
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

// parseFlatDesignProperty reads a design property in the shape Mendix stored
// before 10.2: Type ("Toggle" or "DropDown"), StringValue and BooleanValue on
// the entry itself, as Studio Pro 9.24.30 writes them (ako/mxcli#1209). The
// writer lowers to this shape on such a project (canon.LowerToProjectShape).
// A toggle stored off, or an option with no value, is not set.
func parseFlatDesignProperty(key string, itemMap map[string]any) (rawDesignProp, bool) {
	switch itemMap["Type"] {
	case "Toggle":
		if on, _ := itemMap["BooleanValue"].(bool); on {
			return rawDesignProp{Key: key, ValueType: "toggle"}, true
		}
	case "DropDown":
		if option, _ := itemMap["StringValue"].(string); option != "" {
			return rawDesignProp{Key: key, ValueType: "option", Option: option}, true
		}
	}
	return rawDesignProp{}, false
}

// describeFileManagerProps renders a file manager's Type, MaxFileSize,
// AllowedExtensions and ShowFileInBrowser, leaving out the ones that equal the
// builder's defaults — which are the Model SDK's — so nothing is invented and
// the omitted ones round-trip.
func describeFileManagerProps(ctx *ExecContext, w map[string]any) []string {
	var props []string
	if t, _ := w["Type"].(string); t != "" && t != string(pages.FileManagerTypeBoth) {
		props = append(props, "Type: "+t)
	}
	if n, ok := w["MaxFileSize"]; ok && bsonInt(n) != 5 {
		props = append(props, fmt.Sprintf("MaxFileSize: %d", bsonInt(n)))
	}
	if ext, _ := w["AllowedExtensions"].(string); ext != "" {
		props = append(props, "AllowedExtensions: "+mdlQuote(ctx, ext))
	}
	if show, _ := w["ShowFileInBrowser"].(bool); show {
		props = append(props, "ShowFileInBrowser: true")
	}
	return props
}
