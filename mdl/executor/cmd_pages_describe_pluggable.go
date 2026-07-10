// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
)

// buildPropertyTypeKeyMap builds a map from PropertyType $ID to PropertyKey for a CustomWidget.
// This resolves TypePointer references in Object.Properties back to their property names.
// If withFallback is true, also checks widgetType["PropertyTypes"] directly (for widgets like
// Gallery/DataGrid2 that may store PropertyTypes at different nesting levels).
func buildPropertyTypeKeyMap(w map[string]any, withFallback bool) map[string]string {
	propTypeKeyMap := make(map[string]string)
	widgetType, ok := w["Type"].(map[string]any)
	if !ok {
		return propTypeKeyMap
	}
	var propTypes []any
	if objType, ok := widgetType["ObjectType"].(map[string]any); ok {
		propTypes = getBsonArrayElements(objType["PropertyTypes"])
	}
	if withFallback && len(propTypes) == 0 {
		propTypes = getBsonArrayElements(widgetType["PropertyTypes"])
	}
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key := extractString(ptMap["PropertyKey"])
		if key == "" {
			continue
		}
		id := extractBinaryID(ptMap["$ID"])
		if id != "" {
			propTypeKeyMap[id] = key
		}
	}
	return propTypeKeyMap
}

// extractCustomWidgetAttribute extracts the attribute from a CustomWidget (e.g., ComboBox).
// Specifically looks for attributeAssociation or attributeEnumeration properties by key,
// avoiding false matches from other properties that also have AttributeRef (e.g., CaptionAttribute).
func extractCustomWidgetAttribute(ctx *ExecContext, w map[string]any) string {
	// Try association attribute first, then enumeration attribute
	for _, key := range []string{"attributeAssociation", "attributeEnumeration"} {
		if attr := extractCustomWidgetPropertyAttributeRef(ctx, w, key); attr != "" {
			return attr
		}
	}
	return ""
}

// extractCustomWidgetType extracts the widget type ID from a CustomWidget.
func extractCustomWidgetType(ctx *ExecContext, w map[string]any) string {
	typeObj, ok := w["Type"].(map[string]any)
	if !ok {
		return ""
	}
	if widgetID, ok := typeObj["WidgetId"].(string); ok {
		// Return short name based on widget ID (uppercase for MDL keywords)
		switch widgetID {
		case "com.mendix.widget.web.combobox.Combobox":
			return "combobox"
		case "com.mendix.widget.web.datagrid.Datagrid":
			return "datagrid2"
		case "com.mendix.widget.web.gallery.Gallery":
			return "gallery"
		case "com.mendix.widget.web.datagridtextfilter.DatagridTextFilter":
			return "textfilter"
		case "com.mendix.widget.web.datagridnumberfilter.DatagridNumberFilter":
			return "numberfilter"
		case "com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter":
			return "dropdownfilter"
		case "com.mendix.widget.web.datagriddatefilter.DatagridDateFilter":
			return "datefilter"
		case "com.mendix.widget.web.dropdownsort.DropdownSort":
			return "dropdownsort"
		case "com.mendix.widget.web.image.Image":
			return "image"
		default:
			// Extract last part of widget ID and uppercase it
			parts := strings.Split(widgetID, ".")
			if len(parts) > 0 {
				return strings.ToLower(parts[len(parts)-1])
			}
			return strings.ToLower(widgetID)
		}
	}
	return ""
}

// extractComboBoxDataSource extracts the datasource from a ComboBox CustomWidget in association mode.
// Returns nil for enumeration mode (no datasource).
func extractComboBoxDataSource(ctx *ExecContext, w map[string]any) *rawDataSource {
	// Check if optionsSourceType is "association" first
	sourceType := extractCustomWidgetPropertyString(ctx, w, "optionsSourceType")
	if sourceType != "association" {
		return nil
	}

	// Extract datasource from optionsSourceAssociationDataSource property
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}

	propTypeKeyMap := buildPropertyTypeKeyMap(w, false)

	// Search through properties for optionsSourceAssociationDataSource
	props := getBsonArrayElements(obj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		typePointerID := extractBinaryID(propMap["TypePointer"])
		propKey := propTypeKeyMap[typePointerID]
		if propKey != "optionsSourceAssociationDataSource" {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		dsVal, hasDS := value["DataSource"]
		if !hasDS {
			continue
		}
		if ds, ok := dsVal.(map[string]any); ok && ds != nil {
			return parseCustomWidgetDataSource(ctx, ds)
		}
	}
	return nil
}

// gridSortDirection reads a grid sort item's direction. A Pages/Forms$GridSortItem
// (pluggable DataGrid2/Gallery, ListView) stores it under SortDirection — the name
// Studio Pro and the modelsdk engine write. The legacy sdk/mpr writer used the
// SortOrder key (correct only for Microflows$SortItem / DocumentTemplates), so we
// fall back to it to keep pre-fix files readable. Returns the raw Mendix enum value
// ("Ascending"/"Descending"), or "" when unset.
func gridSortDirection(sortItem map[string]any) string {
	if v := extractString(sortItem["SortDirection"]); v != "" {
		return v
	}
	return extractString(sortItem["SortOrder"])
}

// extractDataGrid2DataSource extracts the datasource from a DataGrid2 CustomWidget.
func extractDataGrid2DataSource(ctx *ExecContext, w map[string]any) *rawDataSource {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}

	// Search through properties for datasource
	props := getBsonArrayElements(obj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		// Check for DataSource
		ds, ok := value["DataSource"].(map[string]any)
		if !ok || ds == nil {
			continue
		}

		dsType := extractString(ds["$Type"])
		switch dsType {
		case "Forms$DatabaseSource":
			entityRef, ok := ds["EntityRef"].(map[string]any)
			if ok && entityRef != nil {
				entity := extractString(entityRef["Entity"])
				if entity != "" {
					return &rawDataSource{Type: "database", Reference: entity}
				}
			}
		case "CustomWidgets$CustomWidgetXPathSource":
			// CustomWidget datasource format - EntityRef contains Entity as qualified name
			result := &rawDataSource{Type: "database"}
			entityRef, ok := ds["EntityRef"].(map[string]any)
			if ok && entityRef != nil {
				result.Reference = extractString(entityRef["Entity"])
			}
			// Extract XPathConstraint
			result.XPathConstraint = extractString(ds["XPathConstraint"])
			// Extract sorting from SortBar - support multiple sort columns
			if sortBar, ok := ds["SortBar"].(map[string]any); ok {
				sortItems := getBsonArrayElements(sortBar["SortItems"])
				for _, item := range sortItems {
					sortItem, ok := item.(map[string]any)
					if !ok {
						continue
					}
					col := rawSortColumn{Order: "asc"}
					// Extract attribute from AttributeRef
					if attrRef, ok := sortItem["AttributeRef"].(map[string]any); ok {
						col.Attribute = shortAttributeName(extractString(attrRef["Attribute"]))
					}
					// Extract sort order
					sortOrder := gridSortDirection(sortItem)
					if sortOrder == "Descending" {
						col.Order = "desc"
					}
					if col.Attribute != "" {
						result.SortColumns = append(result.SortColumns, col)
					}
				}
			}
			if result.Reference != "" {
				return result
			}
		case "Forms$MicroflowSource":
			microflow := extractString(ds["Microflow"])
			if microflow != "" {
				return &rawDataSource{Type: "microflow", Reference: microflow}
			}
		case "Forms$EntityPathSource", "Forms$DataViewSource":
			entityPath := extractString(ds["EntityPath"])
			if entityPath != "" {
				return &rawDataSource{Type: "parameter", Reference: entityPath}
			}
		}
	}
	return nil
}

// extractDataGrid2Columns extracts the columns from a DataGrid2 CustomWidget.
// entityContext is the resolved entity context from the DataGrid2's datasource.
func extractDataGrid2Columns(ctx *ExecContext, w map[string]any, entityContext ...string) []rawDataGridColumn {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}

	// Build column property key map from Type.ObjectType.PropertyTypes -> columns -> ValueType.ObjectType.PropertyTypes
	colPropKeyMap := buildColumnPropertyKeyMap(ctx, w)

	// Search through properties for columns
	props := getBsonArrayElements(obj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		// Check for Objects array (columns are stored as Objects)
		objects := getBsonArrayElements(value["Objects"])
		if len(objects) == 0 {
			continue
		}

		entCtx := ""
		if len(entityContext) > 0 {
			entCtx = entityContext[0]
		}
		var columns []rawDataGridColumn
		for _, colObj := range objects {
			colMap, ok := colObj.(map[string]any)
			if !ok {
				continue
			}
			col := extractDataGrid2Column(ctx, colMap, colPropKeyMap, entCtx)
			if col.Attribute != "" || col.Caption != "" {
				columns = append(columns, col)
			}
		}
		if len(columns) > 0 {
			return columns
		}
	}
	return nil
}

// buildColumnPropertyKeyMap builds a map from TypePointer ID to property key
// for column-level properties (alignment, wrapText, etc.) from the widget Type.
func buildColumnPropertyKeyMap(ctx *ExecContext, w map[string]any) map[string]string {
	result := make(map[string]string)
	widgetType, ok := w["Type"].(map[string]any)
	if !ok {
		return result
	}
	objType, ok := widgetType["ObjectType"].(map[string]any)
	if !ok {
		return result
	}
	// Find the "columns" property type
	propTypes := getBsonArrayElements(objType["PropertyTypes"])
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key := extractString(ptMap["PropertyKey"])
		if key != "columns" {
			continue
		}
		// Get ValueType.ObjectType.PropertyTypes for column-level properties
		valueType, ok := ptMap["ValueType"].(map[string]any)
		if !ok {
			break
		}
		colObjType, ok := valueType["ObjectType"].(map[string]any)
		if !ok {
			break
		}
		colPropTypes := getBsonArrayElements(colObjType["PropertyTypes"])
		for _, cpt := range colPropTypes {
			cptMap, ok := cpt.(map[string]any)
			if !ok {
				continue
			}
			colKey := extractString(cptMap["PropertyKey"])
			if colKey == "" {
				continue
			}
			id := extractBinaryID(cptMap["$ID"])
			if id != "" {
				result[id] = colKey
			}
		}
		break
	}
	return result
}

// extractDataGrid2Column extracts a single column's info from its WidgetObject.
// DataGrid2 columns have several properties:
// - "header": TextTemplate for column header caption (with optional parameters)
// - "attribute": AttributeRef for the attribute binding
// - "showContentAs": enum value ("attribute", "dynamicText", "customContent")
// - "content": Widgets array for custom content
// - "dynamicText": TextTemplate for dynamic text (when showContentAs = "dynamicText")
// - "alignment": enum value ("left", "center", "right")
// - "wrapText": boolean ("true", "false")
// columnAttributeFromRef reconstructs a DataGrid2 column's `attribute:` value
// from its DomainModels$AttributeRef. For an own-entity attribute it returns the
// short attribute name; for an attribute navigated over associations
// (AttributeRef.EntityRef = IndirectEntityRef of steps) it returns
// `Assoc/.../Attr` using SHORT association names — the column-attribute grammar
// (attributePathV3) accepts bare segments only, so a module-qualified association
// would not re-parse.
func columnAttributeFromRef(attrRef map[string]any) string {
	attr := shortAttributeName(extractString(attrRef["Attribute"]))
	entityRef, ok := attrRef["EntityRef"].(map[string]any)
	if !ok || entityRef == nil || extractString(entityRef["$Type"]) != "DomainModels$IndirectEntityRef" {
		return attr
	}
	steps := getBsonArrayElements(entityRef["Steps"])
	assocs := make([]string, 0, len(steps))
	for _, s := range steps {
		sm, ok := s.(map[string]any)
		if !ok {
			return attr
		}
		a := extractString(sm["Association"])
		if a == "" {
			return attr
		}
		assocs = append(assocs, shortAttributeName(a)) // drop module prefix
	}
	if len(assocs) == 0 || attr == "" {
		return attr
	}
	return strings.Join(assocs, "/") + "/" + attr
}

func extractDataGrid2Column(ctx *ExecContext, colObj map[string]any, colPropKeyMap map[string]string, entityContext string) rawDataGridColumn {
	col := rawDataGridColumn{}

	// Track if we've found the header to avoid overwriting with dynamicText's TextTemplate
	foundHeader := false

	props := getBsonArrayElements(colObj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}

		// Resolve property key via TypePointer if available
		propKey := ""
		if len(colPropKeyMap) > 0 {
			typePointerID := extractBinaryID(propMap["TypePointer"])
			propKey = colPropKeyMap[typePointerID]
		}

		// Extract alignment and wrapText by property key
		if propKey == "alignment" {
			if primVal := extractString(value["PrimitiveValue"]); primVal != "" {
				col.Alignment = primVal
			}
			continue
		}
		if propKey == "wrapText" {
			if primVal := extractString(value["PrimitiveValue"]); primVal != "" {
				col.WrapText = primVal
			}
			continue
		}
		if propKey == "sortable" {
			if primVal := extractString(value["PrimitiveValue"]); primVal != "" {
				col.Sortable = primVal
			}
			continue
		}
		if propKey == "resizable" {
			if primVal := extractString(value["PrimitiveValue"]); primVal != "" {
				col.Resizable = primVal
			}
			continue
		}
		if propKey == "draggable" {
			if primVal := extractString(value["PrimitiveValue"]); primVal != "" {
				col.Draggable = primVal
			}
			continue
		}
		if propKey == "hidable" {
			if primVal := extractString(value["PrimitiveValue"]); primVal != "" {
				col.Hidable = primVal
			}
			continue
		}
		if propKey == "width" {
			if primVal := extractString(value["PrimitiveValue"]); primVal != "" {
				col.ColumnWidth = primVal
			}
			continue
		}
		if propKey == "size" {
			if primVal := extractString(value["PrimitiveValue"]); primVal != "" {
				col.Size = primVal
			}
			continue
		}
		if propKey == "visible" {
			if expr := extractString(value["Expression"]); expr != "" {
				col.Visible = expr
			}
			continue
		}
		if propKey == "columnClass" {
			if expr := extractString(value["Expression"]); expr != "" {
				col.DynamicCellClass = expr
			}
			continue
		}
		if propKey == "tooltip" {
			if textTemplate, ok := value["TextTemplate"].(map[string]any); ok && textTemplate != nil {
				if template, ok := textTemplate["Template"].(map[string]any); ok && template != nil {
					items := getBsonArrayElements(template["Items"])
					for _, item := range items {
						itemMap, ok := item.(map[string]any)
						if !ok {
							continue
						}
						if text := extractString(itemMap["Text"]); text != "" {
							col.Tooltip = text
							break
						}
					}
				}
			}
			continue
		}
		// Route header / dynamicText by property key so the read result doesn't
		// depend on property iteration order. The writer emits properties
		// alphabetically, which puts dynamicText before header — the old
		// "first TextTemplate is the header" heuristic was wrong for that order.
		if propKey == "header" {
			if text, tt := extractTextTemplateText(value); text != "" {
				col.Caption = text
				col.CaptionParams = extractTextTemplateParameters(ctx, tt)
				foundHeader = true
			}
			continue
		}
		if propKey == "dynamicText" {
			if text, tt := extractTextTemplateText(value); text != "" {
				col.DynamicText = text
				col.DynamicTextParams = extractTextTemplateParameters(ctx, tt)
			}
			continue
		}

		// Check for AttributeRef (attribute property)
		if col.Attribute == "" {
			if attrRef, ok := value["AttributeRef"].(map[string]any); ok && attrRef != nil {
				col.Attribute = columnAttributeFromRef(attrRef)
			}
		}

		// Check for PrimitiveValue (could be showContentAs enum)
		if col.ShowContentAs == "" {
			if primVal := extractString(value["PrimitiveValue"]); primVal != "" {
				// Check if it's a showContentAs enum value
				if primVal == "attribute" || primVal == "dynamicText" || primVal == "customContent" {
					col.ShowContentAs = primVal
				}
			}
		}

		// Check for Widgets array (content property for custom widgets)
		if len(col.ContentWidgets) == 0 {
			widgets := getBsonArrayElements(value["Widgets"])
			if len(widgets) > 0 {
				for _, w := range widgets {
					if wMap, ok := w.(map[string]any); ok {
						col.ContentWidgets = append(col.ContentWidgets, parseRawWidget(ctx, wMap, entityContext)...)
					}
				}
			}
		}

		// Check for TextTemplate (could be header or dynamicText property)
		if textTemplate, ok := value["TextTemplate"].(map[string]any); ok && textTemplate != nil {
			template, ok := textTemplate["Template"].(map[string]any)
			if ok && template != nil {
				items := getBsonArrayElements(template["Items"])
				for _, item := range items {
					itemMap, ok := item.(map[string]any)
					if !ok {
						continue
					}
					if text := extractString(itemMap["Text"]); text != "" {
						if !foundHeader {
							// First TextTemplate with text is the header
							col.Caption = text
							col.CaptionParams = extractTextTemplateParameters(ctx, textTemplate)
							foundHeader = true
						} else if col.DynamicText == "" {
							// Second TextTemplate is dynamicText (if showContentAs = dynamicText)
							col.DynamicText = text
							col.DynamicTextParams = extractTextTemplateParameters(ctx, textTemplate)
						}
						break
					}
				}
			}
		}
	}
	return col
}

// extractDataGrid2ControlBar extracts the CONTROLBAR widgets from a DataGrid2 CustomWidget.
// DataGrid2 stores header/filter widgets in the 'filtersPlaceholder' property, same as Gallery.
func extractDataGrid2ControlBar(ctx *ExecContext, w map[string]any) []rawWidget {
	return extractGalleryWidgetsByPropertyKey(ctx, w, "filtersPlaceholder")
}

// extractTextTemplateText finds the first non-empty Text item inside a
// WidgetValue's TextTemplate (Forms$ClientTemplate → Texts$Text → Items[]).
// Returns the text and the TextTemplate map so callers can also pull
// Parameters from it. Returns ("", nil) when the value has no template text.
func extractTextTemplateText(value map[string]any) (string, map[string]any) {
	textTemplate, ok := value["TextTemplate"].(map[string]any)
	if !ok || textTemplate == nil {
		return "", nil
	}
	template, ok := textTemplate["Template"].(map[string]any)
	if !ok || template == nil {
		return "", nil
	}
	for _, item := range getBsonArrayElements(template["Items"]) {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if text := extractString(itemMap["Text"]); text != "" {
			return text, textTemplate
		}
	}
	return "", nil
}

// extractTextTemplateParameters extracts parameters from a TextTemplate (Forms$ClientTemplate).
func extractTextTemplateParameters(ctx *ExecContext, textTemplate map[string]any) []string {
	params := getBsonArrayElements(textTemplate["Parameters"])
	if params == nil || len(params) == 0 {
		return nil
	}
	var result []string
	for _, p := range params {
		pMap, ok := p.(map[string]any)
		if !ok {
			continue
		}
		// Check for Expression first (literal value)
		if expr, ok := pMap["Expression"].(string); ok && expr != "" {
			result = append(result, expr)
			continue
		}

		// Check for SourceVariable (page parameter / local variable / snippet parameter)
		sourceVarName := ""
		isLocalVariable := false
		if srcVar, ok := pMap["SourceVariable"].(map[string]any); ok && srcVar != nil {
			// Local page-level variable — Studio Pro UI emits this form for
			// $varName references that bind to a Variables entry.
			if name, ok := srcVar["LocalVariable"].(string); ok && name != "" {
				sourceVarName = name
				isLocalVariable = true
			} else if name, ok := srcVar["PageParameter"].(string); ok && name != "" {
				sourceVarName = name
			} else if name, ok := srcVar["SnippetParameter"].(string); ok && name != "" {
				sourceVarName = name
			}
		}

		// Local variable: emit as bare $varName (no .attribute suffix).
		if isLocalVariable {
			result = append(result, "$"+sourceVarName)
			continue
		}

		// Check for AttributeRef
		if attrRef, ok := pMap["AttributeRef"].(map[string]any); ok && attrRef != nil {
			if attr, ok := attrRef["Attribute"].(string); ok {
				if sourceVarName != "" {
					// Has SourceVariable - this is a page parameter reference
					parts := strings.Split(attr, ".")
					attrName := parts[len(parts)-1]
					result = append(result, "$"+sourceVarName+"."+attrName)
				} else {
					// No SourceVariable - use short attribute name
					result = append(result, shortAttributeName(attr))
				}
				continue
			}
		}
		// Parameter exists but has no binding
		result = append(result, "<unbound>")
	}
	return result
}

// extractGalleryDataSource extracts the datasource from a Gallery widget.
// Handles both Forms$Gallery and CustomWidgets$CustomWidget Gallery formats.
func extractGalleryDataSource(ctx *ExecContext, w map[string]any) *rawDataSource {
	// First check for CustomWidget Gallery format (datasource in Object.Properties)
	if obj, ok := w["Object"].(map[string]any); ok {
		props := getBsonArrayElements(obj["Properties"])
		for _, prop := range props {
			propMap, ok := prop.(map[string]any)
			if !ok {
				continue
			}
			value, ok := propMap["Value"].(map[string]any)
			if !ok {
				continue
			}
			// Check for DataSource field in Value - only process if not nil
			dsVal, hasDS := value["DataSource"]
			if !hasDS {
				continue
			}
			if ds, ok := dsVal.(map[string]any); ok && ds != nil {
				result := parseCustomWidgetDataSource(ctx, ds)
				if result != nil {
					return result
				}
			}
		}
	}

	// Fall back to Forms$Gallery format (DataSource at top level)
	ds, ok := w["DataSource"].(map[string]any)
	if !ok || ds == nil {
		return nil
	}

	dsType := extractString(ds["$Type"])
	switch dsType {
	case "Forms$DatabaseSource":
		result := &rawDataSource{Type: "database"}
		entityRef, ok := ds["EntityRef"].(map[string]any)
		if ok && entityRef != nil {
			result.Reference = extractString(entityRef["Entity"])
		}
		result.XPathConstraint = extractString(ds["XPathConstraint"])
		// Extract sorting
		if sortBar, ok := ds["SortBar"].(map[string]any); ok {
			sortItems := getBsonArrayElements(sortBar["SortItems"])
			for _, item := range sortItems {
				sortItem, ok := item.(map[string]any)
				if !ok {
					continue
				}
				col := rawSortColumn{Order: "asc"}
				if attrRef, ok := sortItem["AttributeRef"].(map[string]any); ok {
					col.Attribute = shortAttributeName(extractString(attrRef["Attribute"]))
				}
				sortOrder := gridSortDirection(sortItem)
				if sortOrder == "Descending" {
					col.Order = "desc"
				}
				if col.Attribute != "" {
					result.SortColumns = append(result.SortColumns, col)
				}
			}
		}
		if result.Reference != "" {
			return result
		}
	case "Forms$MicroflowSource":
		microflow := extractString(ds["Microflow"])
		if microflow != "" {
			return &rawDataSource{Type: "microflow", Reference: microflow}
		}
	case "Forms$EntityPathSource", "Forms$DataViewSource":
		entityPath := extractString(ds["EntityPath"])
		if entityPath != "" {
			return &rawDataSource{Type: "parameter", Reference: entityPath}
		}
	}
	return nil
}

// parseCustomWidgetDataSource parses datasource from CustomWidget property format.
func parseCustomWidgetDataSource(ctx *ExecContext, ds map[string]any) *rawDataSource {
	dsType := extractString(ds["$Type"])
	switch dsType {
	case "CustomWidgets$CustomWidgetXPathSource":
		result := &rawDataSource{Type: "database"}
		entityRef, ok := ds["EntityRef"].(map[string]any)
		if ok && entityRef != nil {
			result.Reference = extractString(entityRef["Entity"])
		}
		result.XPathConstraint = extractString(ds["XPathConstraint"])
		// Extract sorting if present
		if sortBar, ok := ds["SortBar"].(map[string]any); ok {
			sortItems := getBsonArrayElements(sortBar["SortItems"])
			for _, item := range sortItems {
				sortItem, ok := item.(map[string]any)
				if !ok {
					continue
				}
				col := rawSortColumn{Order: "asc"}
				if attrRef, ok := sortItem["AttributeRef"].(map[string]any); ok {
					col.Attribute = shortAttributeName(extractString(attrRef["Attribute"]))
				}
				sortOrder := gridSortDirection(sortItem)
				if sortOrder == "Descending" {
					col.Order = "desc"
				}
				if col.Attribute != "" {
					result.SortColumns = append(result.SortColumns, col)
				}
			}
		}
		return result
	case "Forms$MicroflowSource":
		// Pluggable widgets use Forms$MicroflowSource with MicroflowSettings
		if settings, ok := ds["MicroflowSettings"].(map[string]any); ok {
			microflow := extractString(settings["Microflow"])
			if microflow != "" {
				return &rawDataSource{Type: "microflow", Reference: microflow}
			}
		}
	case "Forms$NanoflowSource":
		// Pluggable widgets use Forms$NanoflowSource with NanoflowSettings
		if settings, ok := ds["NanoflowSettings"].(map[string]any); ok {
			nanoflow := extractString(settings["Nanoflow"])
			if nanoflow != "" {
				return &rawDataSource{Type: "nanoflow", Reference: nanoflow}
			}
		}
	case "CustomWidgets$CustomWidgetNanoflowSource":
		nanoflow := extractString(ds["Nanoflow"])
		if nanoflow != "" {
			return &rawDataSource{Type: "nanoflow", Reference: nanoflow}
		}
	}
	return nil
}

// extractGalleryContent extracts the content widgets from a CustomWidget Gallery.
// entityContext is the resolved entity context from the Gallery's datasource.
func extractGalleryContent(ctx *ExecContext, w map[string]any, entityContext ...string) []rawWidget {
	entCtx := ""
	if len(entityContext) > 0 {
		entCtx = entityContext[0]
	}
	return extractGalleryWidgetsByPropertyKey(ctx, w, "content", entCtx)
}

// extractGalleryWidgetsByPropertyKey extracts widgets from a named property of a CustomWidget Gallery.
// entityContext is the resolved entity context to propagate to child widgets.
func extractGalleryWidgetsByPropertyKey(ctx *ExecContext, w map[string]any, targetKey string, entityContext ...string) []rawWidget {
	entCtx := ""
	if len(entityContext) > 0 {
		entCtx = entityContext[0]
	}
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}

	// Build a map from PropertyType ID to PropertyKey (with fallback for Gallery/DataGrid2)
	propTypeKeyMap := buildPropertyTypeKeyMap(w, true)

	// Search through properties for the named property
	props := getBsonArrayElements(obj["Properties"])

	// First pass: try to match by property key
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}

		// Check property key via TypePointer - can be string, binary, or map with $Subtype
		typePointerID := extractBinaryID(propMap["TypePointer"])
		propKey := propTypeKeyMap[typePointerID]

		// Skip if not the target property
		if propKey != targetKey {
			continue
		}

		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		// Check for Widgets array
		widgetsArr := getBsonArrayElements(value["Widgets"])
		if len(widgetsArr) == 0 {
			continue
		}

		var result []rawWidget
		for _, wgt := range widgetsArr {
			wgtMap, ok := wgt.(map[string]any)
			if !ok {
				continue
			}
			result = append(result, parseRawWidget(ctx, wgtMap, entCtx)...)
		}
		return result
	}

	// Fallback: if no property key map, scan all properties with Widgets
	// This handles cases where PropertyKey field isn't available
	if len(propTypeKeyMap) == 0 && targetKey == "content" {
		for _, prop := range props {
			propMap, ok := prop.(map[string]any)
			if !ok {
				continue
			}
			value, ok := propMap["Value"].(map[string]any)
			if !ok {
				continue
			}
			// Check for Widgets array
			widgetsArr := getBsonArrayElements(value["Widgets"])
			if len(widgetsArr) == 0 {
				continue
			}
			var result []rawWidget
			for _, wgt := range widgetsArr {
				wgtMap, ok := wgt.(map[string]any)
				if !ok {
					continue
				}
				result = append(result, parseRawWidget(ctx, wgtMap, entCtx)...)
			}
			if len(result) > 0 {
				return result
			}
		}
	}

	return nil
}

// extractGalleryFilters extracts the filter widgets from a CustomWidget Gallery.
func extractGalleryFilters(ctx *ExecContext, w map[string]any) []rawWidget {
	return extractGalleryWidgetsByPropertyKey(ctx, w, "filtersPlaceholder")
}

// extractGallerySelection extracts the selection mode from a CustomWidget Gallery.
func extractGallerySelection(ctx *ExecContext, w map[string]any) string {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return ""
	}

	// Search through properties for one with Selection != "None"
	props := getBsonArrayElements(obj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		// Check for Selection field
		if sel, ok := value["Selection"].(string); ok && sel != "None" && sel != "" {
			return sel
		}
	}
	return ""
}

// extractFilterAttributes extracts the filter attributes from a TextFilter/NumberFilter widget.
func extractFilterAttributes(ctx *ExecContext, w map[string]any) []string {
	// Use the generic property extraction helper
	return extractCustomWidgetPropertyAttributes(ctx, w, "attributes")
}

// extractFilterExpression extracts the default filter expression from a TextFilter widget.
func extractFilterExpression(ctx *ExecContext, w map[string]any) string {
	return extractCustomWidgetPropertyString(ctx, w, "defaultFilter")
}

// extractCustomWidgetPropertyAttributeRef extracts an AttributeRef value from a named CustomWidget property.
func extractCustomWidgetPropertyAttributeRef(ctx *ExecContext, w map[string]any, propertyKey string) string {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return ""
	}

	propTypeKeyMap := buildPropertyTypeKeyMap(w, false)

	// Search through properties for the named property
	props := getBsonArrayElements(obj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		typePointerID := extractBinaryID(propMap["TypePointer"])
		propKey := propTypeKeyMap[typePointerID]
		if propKey != propertyKey {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		if attrRef, ok := value["AttributeRef"].(map[string]any); ok && attrRef != nil {
			if attr, ok := attrRef["Attribute"].(string); ok && attr != "" {
				return shortAttributeName(attr)
			}
		}
	}
	return ""
}

// extractCustomWidgetPropertyAssociation extracts an association name from a named
// CustomWidget property that was written by opAssociation (setAssociationRef).
// The association is stored as EntityRef.Steps[1].Association (qualified path);
// this function returns only the short name (last segment after the final dot).
//
// This is the symmetric counterpart of extractCustomWidgetPropertyAttributeRef,
// handling the EntityRef storage format instead of AttributeRef.
func extractCustomWidgetPropertyAssociation(ctx *ExecContext, w map[string]any, propertyKey string) string {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return ""
	}

	propTypeKeyMap := buildPropertyTypeKeyMap(w, false)

	// Find the named property and extract EntityRef.Steps[1].Association
	props := getBsonArrayElements(obj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		typePointerID := extractBinaryID(propMap["TypePointer"])
		if propTypeKeyMap[typePointerID] != propertyKey {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		entityRef, ok := value["EntityRef"].(map[string]any)
		if !ok || entityRef == nil {
			return ""
		}
		steps := getBsonArrayElements(entityRef["Steps"])
		// Steps layout: [int32(2), step0, step1, ...] — first element is version marker
		for _, step := range steps {
			stepMap, ok := step.(map[string]any)
			if !ok {
				continue
			}
			if assoc := extractString(stepMap["Association"]); assoc != "" {
				return shortAttributeName(assoc)
			}
		}
	}
	return ""
}

// extractCustomWidgetPropertyString extracts a string property value from a CustomWidget.
func extractCustomWidgetPropertyString(ctx *ExecContext, w map[string]any, propertyKey string) string {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return ""
	}

	propTypeKeyMap := buildPropertyTypeKeyMap(w, false)

	// Search through properties for the named property
	props := getBsonArrayElements(obj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}

		// Check property key via TypePointer
		typePointerID := extractBinaryID(propMap["TypePointer"])
		propKey := propTypeKeyMap[typePointerID]
		if propKey != propertyKey {
			continue
		}

		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}

		// Extract PrimitiveValue for string properties
		if pv, ok := value["PrimitiveValue"].(string); ok && pv != "" {
			return pv
		}
	}
	return ""
}

// extractCustomWidgetPropertyAttributes extracts attribute references from a CustomWidget property.
func extractCustomWidgetPropertyAttributes(ctx *ExecContext, w map[string]any, propertyKey string) []string {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}

	propTypeKeyMap := buildPropertyTypeKeyMap(w, false)

	// Search through properties for the named property
	props := getBsonArrayElements(obj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}

		// Check property key via TypePointer
		typePointerID := extractBinaryID(propMap["TypePointer"])
		propKey := propTypeKeyMap[typePointerID]
		if propKey != propertyKey {
			continue
		}

		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}

		// Extract from Objects array (each object has an AttributeRef)
		objects := getBsonArrayElements(value["Objects"])
		var result []string
		for _, objItem := range objects {
			objMap, ok := objItem.(map[string]any)
			if !ok {
				continue
			}
			// Look for Properties inside each object
			objProps := getBsonArrayElements(objMap["Properties"])
			for _, objProp := range objProps {
				objPropMap, ok := objProp.(map[string]any)
				if !ok {
					continue
				}
				objValue, ok := objPropMap["Value"].(map[string]any)
				if !ok {
					continue
				}
				// Check for AttributeRef
				if attrRef, ok := objValue["AttributeRef"].(map[string]any); ok && attrRef != nil {
					if attr, ok := attrRef["Attribute"].(string); ok && attr != "" {
						result = append(result, shortAttributeName(attr))
					}
				}
			}
		}
		return result
	}
	return nil
}

// extractCustomWidgetID extracts the full widget ID from a CustomWidget (e.g. "com.mendix.widget.custom.switch.Switch").
func extractCustomWidgetID(ctx *ExecContext, w map[string]any) string {
	typeObj, ok := w["Type"].(map[string]any)
	if !ok {
		return ""
	}
	if widgetID, ok := typeObj["WidgetId"].(string); ok {
		return widgetID
	}
	return ""
}

// isKnownCustomWidgetType returns true for widget types that have dedicated DESCRIBE extractors.
func isKnownCustomWidgetType(widgetType string) bool {
	switch widgetType {
	case "combobox", "datagrid2", "gallery", "image",
		"textfilter", "numberfilter", "dropdownfilter", "datefilter",
		"dropdownsort":
		return true
	}
	return false
}

// extractExplicitProperties extracts non-default property values from a CustomWidget BSON.
// Returns attribute references and primitive values for properties that differ from defaults.
func extractExplicitProperties(ctx *ExecContext, w map[string]any) []rawExplicitProp {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}

	propTypeKeyMap := buildPropertyTypeKeyMap(w, false)
	if len(propTypeKeyMap) == 0 {
		return nil
	}

	var result []rawExplicitProp
	props := getBsonArrayElements(obj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		typePointerID := extractBinaryID(propMap["TypePointer"])
		propKey := propTypeKeyMap[typePointerID]
		if propKey == "" {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}

		// Check for AttributeRef (attribute binding)
		if attrRef, ok := value["AttributeRef"].(map[string]any); ok && attrRef != nil {
			if attr := extractString(attrRef["Attribute"]); attr != "" {
				result = append(result, rawExplicitProp{
					Key:   propKey,
					Value: shortAttributeName(attr),
					IsRef: true,
				})
				continue
			}
		}

		// Check for non-default PrimitiveValue
		if pv := extractString(value["PrimitiveValue"]); pv != "" {
			// Skip common defaults
			if pv == "true" || pv == "false" {
				continue
			}
			result = append(result, rawExplicitProp{
				Key:   propKey,
				Value: pv,
			})
		}
	}
	return result
}

// extractImageProperties extracts properties from a pluggable Image CustomWidget.
func extractImageProperties(ctx *ExecContext, w map[string]any, widget *rawWidget) {
	widget.ImageType = extractCustomWidgetPropertyString(ctx, w, "datasource")
	widget.ImageUrl = extractCustomWidgetPropertyTextTemplate(ctx, w, "imageUrl")
	widget.AlternativeText = extractCustomWidgetPropertyTextTemplate(ctx, w, "alternativeText")
	widget.ImageWidth = extractCustomWidgetPropertyString(ctx, w, "width")
	widget.ImageHeight = extractCustomWidgetPropertyString(ctx, w, "height")
	widget.WidthUnit = extractCustomWidgetPropertyString(ctx, w, "widthUnit")
	widget.HeightUnit = extractCustomWidgetPropertyString(ctx, w, "heightUnit")
	widget.DisplayAs = extractCustomWidgetPropertyString(ctx, w, "displayAs")
	widget.Responsive = extractCustomWidgetPropertyString(ctx, w, "responsive")
	widget.OnClickType = extractCustomWidgetPropertyString(ctx, w, "onClickType")
	widget.Action = extractCustomWidgetPropertyAction(ctx, w, "onClick")
}

// extractCustomWidgetPropertyTextTemplate extracts text from a TextTemplate property of a CustomWidget.
func extractCustomWidgetPropertyTextTemplate(ctx *ExecContext, w map[string]any, propertyKey string) string {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return ""
	}

	propTypeKeyMap := buildPropertyTypeKeyMap(w, false)

	props := getBsonArrayElements(obj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		typePointerID := extractBinaryID(propMap["TypePointer"])
		propKey := propTypeKeyMap[typePointerID]
		if propKey != propertyKey {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		// Extract text from TextTemplate
		if textTemplate, ok := value["TextTemplate"].(map[string]any); ok && textTemplate != nil {
			if template, ok := textTemplate["Template"].(map[string]any); ok && template != nil {
				items := getBsonArrayElements(template["Items"])
				for _, item := range items {
					itemMap, ok := item.(map[string]any)
					if !ok {
						continue
					}
					if text := extractString(itemMap["Text"]); text != "" {
						return text
					}
				}
			}
		}
	}
	return ""
}

// extractCustomWidgetPropertyAction extracts an action description from a CustomWidget property.
// Returns a formatted string like "CALL_MICROFLOW Module.Flow" or "SHOW_PAGE Module.Page".
func extractCustomWidgetPropertyAction(ctx *ExecContext, w map[string]any, propertyKey string) string {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return ""
	}

	propTypeKeyMap := buildPropertyTypeKeyMap(w, false)

	props := getBsonArrayElements(obj["Properties"])
	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		typePointerID := extractBinaryID(propMap["TypePointer"])
		propKey := propTypeKeyMap[typePointerID]
		if propKey != propertyKey {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		action, ok := value["Action"].(map[string]any)
		if !ok || action == nil {
			continue
		}
		actionType := extractString(action["$Type"])
		switch actionType {
		case "Forms$MicroflowAction", "Pages$MicroflowClientAction":
			if settings, ok := action["MicroflowSettings"].(map[string]any); ok {
				if mf := extractString(settings["Microflow"]); mf != "" {
					return "microflow " + mf
				}
			}
		case "Forms$CallNanoflowClientAction", "Pages$CallNanoflowClientAction":
			if settings, ok := action["NanoflowSettings"].(map[string]any); ok {
				if nf := extractString(settings["Nanoflow"]); nf != "" {
					return "nanoflow " + nf
				}
			}
		case "Forms$FormAction", "Pages$FormAction":
			if settings, ok := action["PageSettings"].(map[string]any); ok {
				if page := extractString(settings["Page"]); page != "" {
					return "show_page " + page
				}
			}
		case "Forms$NoAction", "Pages$NoAction":
			return ""
		}
	}
	return ""
}

func (e *Executor) extractCustomWidgetPropertyAssociation(w map[string]any, propertyKey string) string {
	return extractCustomWidgetPropertyAssociation(e.newExecContext(context.Background()), w, propertyKey)
}
