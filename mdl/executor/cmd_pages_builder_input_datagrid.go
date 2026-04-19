// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// colPropBool reads a bool/string property from col.Properties and returns "true"/"false".
func colPropBool(props map[string]any, key string, defaultVal string) string {
	if props == nil {
		return defaultVal
	}
	v, ok := props[key]
	if !ok {
		return defaultVal
	}
	switch bv := v.(type) {
	case bool:
		if bv {
			return "true"
		}
		return "false"
	case string:
		lower := strings.ToLower(bv)
		if lower == "true" || lower == "false" {
			return lower
		}
		return defaultVal
	default:
		return defaultVal
	}
}

// colPropString reads a string property from col.Properties and lowercases it.
func colPropString(props map[string]any, key string, defaultVal string) string {
	if props == nil {
		return defaultVal
	}
	v, ok := props[key]
	if !ok {
		return defaultVal
	}
	if sv, isStr := v.(string); isStr && sv != "" {
		return strings.ToLower(sv)
	}
	return defaultVal
}

// colPropInt reads an int/float/string property from col.Properties and returns its string form.
func colPropInt(props map[string]any, key string, defaultVal string) string {
	if props == nil {
		return defaultVal
	}
	v, ok := props[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return fmt.Sprintf("%d", n)
	case int64:
		return fmt.Sprintf("%d", n)
	case float64:
		return fmt.Sprintf("%d", int(n))
	case string:
		if n != "" {
			return n
		}
		return defaultVal
	default:
		return defaultVal
	}
}

// buildDataGrid2Property creates a WidgetProperty BSON for DataGrid2.
func (pb *pageBuilder) buildDataGrid2Property(entry pages.PropertyTypeIDEntry, datasource pages.DataSource, attrRef string, primitiveValue string) bson.D {
	// Build the datasource BSON if provided
	var datasourceBSON any
	if datasource != nil {
		datasourceBSON = pb.widgetBackend.SerializeDataSourceToOpaque(datasource)
	}

	// Build attribute ref if provided
	var attrRefBSON any
	if attrRef != "" {
		attrRefBSON = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "DomainModels$AttributeRef"},
			{Key: "Attribute", Value: attrRef},
			{Key: "EntityRef", Value: nil},
		}
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NoAction"},
				{Key: "DisabledDuringExecution", Value: true},
			}},
			{Key: "AttributeRef", Value: attrRefBSON},
			{Key: "DataSource", Value: datasourceBSON},
			{Key: "EntityRef", Value: nil},
			{Key: "Expression", Value: ""},
			{Key: "Form", Value: ""},
			{Key: "Icon", Value: nil},
			{Key: "Image", Value: ""},
			{Key: "Microflow", Value: ""},
			{Key: "Nanoflow", Value: ""},
			{Key: "Objects", Value: bson.A{int32(2)}},
			{Key: "PrimitiveValue", Value: primitiveValue},
			{Key: "Selection", Value: "None"},
			{Key: "SourceVariable", Value: nil},
			{Key: "TextTemplate", Value: nil},
			{Key: "TranslatableValue", Value: nil},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
			{Key: "Widgets", Value: bson.A{int32(2)}},
			{Key: "XPathConstraint", Value: ""},
		}},
	}
}

func (pb *pageBuilder) updateDataGrid2Object(templateObject bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, datasource pages.DataSource, columns []ast.DataGridColumnDef, headerWidgets []bson.D) bson.D {
	// Clone the template object with new IDs
	result := make(bson.D, 0, len(templateObject))

	for _, elem := range templateObject {
		if elem.Key == "$ID" {
			// Generate new ID for the object
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Properties" {
			// Update properties
			if propsArr, ok := elem.Value.(bson.A); ok {
				updatedProps := pb.updateDataGrid2Properties(propsArr, propertyTypeIDs, datasource, columns, headerWidgets)
				result = append(result, bson.E{Key: "Properties", Value: updatedProps})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}

	return result
}

func (pb *pageBuilder) updateDataGrid2Properties(props bson.A, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, datasource pages.DataSource, columns []ast.DataGridColumnDef, headerWidgets []bson.D) bson.A {
	result := bson.A{int32(2)} // Version marker

	// Get the property type IDs for datasource, columns, and filtersPlaceholder
	datasourceEntry := propertyTypeIDs["datasource"]
	columnsEntry := propertyTypeIDs["columns"]
	filtersPlaceholderEntry := propertyTypeIDs["filtersPlaceholder"]

	// Process each property from the template
	for _, propVal := range props {
		// Skip version markers
		if _, ok := propVal.(int32); ok {
			continue
		}

		propMap, ok := propVal.(bson.D)
		if !ok {
			continue
		}

		// Check if this is the datasource, columns, or filtersPlaceholder property by matching TypePointer
		typePointer := pb.getTypePointerFromProperty(propMap)

		if typePointer == datasourceEntry.PropertyTypeID {
			// Replace with our datasource
			result = append(result, pb.buildDataGrid2Property(datasourceEntry, datasource, "", ""))
		} else if typePointer == columnsEntry.PropertyTypeID {
			// Clone the columns property and update with our column data
			result = append(result, pb.cloneAndUpdateColumnsProperty(propMap, columnsEntry, propertyTypeIDs, columns))
		} else if typePointer == filtersPlaceholderEntry.PropertyTypeID && len(headerWidgets) > 0 {
			// Replace with our header widgets (for HEADER section support)
			result = append(result, pb.buildFiltersPlaceholderProperty(filtersPlaceholderEntry, headerWidgets))
		} else {
			// Keep the template property as-is, but regenerate IDs
			result = append(result, pb.clonePropertyWithNewIDs(propMap))
		}
	}

	return result
}

func (pb *pageBuilder) cloneAndUpdateColumnsProperty(templateProp bson.D, columnsEntry pages.PropertyTypeIDEntry, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, columns []ast.DataGridColumnDef) bson.D {
	// Extract template column object from the property
	var templateColumnObj bson.D
	for _, elem := range templateProp {
		if elem.Key == "Value" {
			if valMap, ok := elem.Value.(bson.D); ok {
				for _, ve := range valMap {
					if ve.Key == "Objects" {
						if objArr, ok := ve.Value.(bson.A); ok {
							// Find first WidgetObject in the array
							for _, obj := range objArr {
								if colObj, ok := obj.(bson.D); ok {
									templateColumnObj = colObj
									break
								}
							}
						}
					}
				}
			}
		}
	}

	// Build column objects by cloning template and updating
	columnObjects := bson.A{int32(2)} // Version marker
	for i := range columns {
		col := &columns[i]
		if templateColumnObj != nil {
			// Clone template column and update (preserves all template properties)
			columnObjects = append(columnObjects, pb.cloneAndUpdateColumnObject(templateColumnObj, col, columnsEntry.NestedPropertyIDs))
		} else {
			// Build from scratch (no template available)
			columnObjects = append(columnObjects, pb.buildDataGrid2ColumnObject(col, columnsEntry.ObjectTypeID, columnsEntry.NestedPropertyIDs))
		}
	}

	// Clone the property structure with new IDs and our column objects
	result := make(bson.D, 0, len(templateProp))
	for _, elem := range templateProp {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Value" {
			if valMap, ok := elem.Value.(bson.D); ok {
				newVal := make(bson.D, 0, len(valMap))
				for _, ve := range valMap {
					if ve.Key == "$ID" {
						newVal = append(newVal, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
					} else if ve.Key == "Objects" {
						newVal = append(newVal, bson.E{Key: "Objects", Value: columnObjects})
					} else if ve.Key == "Action" {
						// Clone Action with new ID
						if actionMap, ok := ve.Value.(bson.D); ok {
							newVal = append(newVal, bson.E{Key: "Action", Value: pb.cloneWithNewID(actionMap)})
						} else {
							newVal = append(newVal, ve)
						}
					} else {
						newVal = append(newVal, ve)
					}
				}
				result = append(result, bson.E{Key: "Value", Value: newVal})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}

	return result
}

func (pb *pageBuilder) cloneAndUpdateColumnObject(templateCol bson.D, col *ast.DataGridColumnDef, columnPropertyIDs map[string]pages.PropertyTypeIDEntry) bson.D {
	attrPath := pb.resolveAttributePath(col.Attribute)
	caption := col.Caption
	if caption == "" {
		caption = col.Attribute
	}

	// Build content widgets if there are child widgets
	var contentWidgets []bson.D
	for _, child := range col.ChildrenV3 {
		widgetBSON, err := pb.buildWidgetV3ToBSON(child)
		if err != nil {
			// Log error and continue (don't fail the entire column)
			continue
		}
		if widgetBSON != nil {
			contentWidgets = append(contentWidgets, widgetBSON)
		}
	}

	result := make(bson.D, 0, len(templateCol))
	for _, elem := range templateCol {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Properties" {
			// Update properties
			if propsArr, ok := elem.Value.(bson.A); ok {
				result = append(result, bson.E{Key: "Properties", Value: pb.cloneAndUpdateColumnProperties(propsArr, columnPropertyIDs, col, attrPath, caption, contentWidgets)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func (pb *pageBuilder) cloneAndUpdateColumnProperties(templateProps bson.A, columnPropertyIDs map[string]pages.PropertyTypeIDEntry, col *ast.DataGridColumnDef, attrPath, caption string, contentWidgets []bson.D) bson.A {
	result := bson.A{int32(2)} // Version marker

	// Track which properties were added
	addedProps := make(map[string]bool)

	hasCustomContent := len(contentWidgets) > 0

	for _, propVal := range templateProps {
		if _, ok := propVal.(int32); ok {
			continue // Skip version markers
		}
		propMap, ok := propVal.(bson.D)
		if !ok {
			continue
		}

		typePointer := pb.getTypePointerFromProperty(propMap)

		// Find which property key this TypePointer corresponds to
		propKey := ""
		for key, entry := range columnPropertyIDs {
			if entry.PropertyTypeID == typePointer {
				addedProps[key] = true
				propKey = key
				break
			}
		}

		// Clone template properties, adjusting for the column mode.
		//
		// The editorConfig.js in the widget mpk defines mode-dependent visibility:
		//   attribute mode:      tooltip VISIBLE, content/allowEventPropagation/exportValue HIDDEN
		//   customContent mode:  tooltip HIDDEN, content/allowEventPropagation/exportValue VISIBLE
		//
		// Properties must have mode-appropriate values or CE0463 is triggered.
		// See docs/03-development/PAGE_BSON_SERIALIZATION.md for details.
		switch propKey {
		case "showContentAs":
			if hasCustomContent {
				result = append(result, pb.clonePropertyWithPrimitiveValue(propMap, "customContent"))
			} else {
				result = append(result, pb.clonePropertyWithNewIDs(propMap))
			}
		case "attribute":
			if attrPath != "" {
				entry := columnPropertyIDs["attribute"]
				result = append(result, pb.buildColumnAttributeProperty(entry, attrPath))
			} else {
				result = append(result, pb.clonePropertyWithNewIDs(propMap))
			}
		case "header":
			entry := columnPropertyIDs["header"]
			result = append(result, pb.buildColumnHeaderProperty(entry, caption))
		case "content":
			if hasCustomContent {
				entry := columnPropertyIDs["content"]
				result = append(result, pb.buildColumnContentProperty(entry, contentWidgets))
			} else {
				result = append(result, pb.clonePropertyWithNewIDs(propMap))
			}
		case "visible":
			visExpr := "true"
			if col.Properties != nil {
				if v, ok := col.Properties["Visible"]; ok {
					if sv, isStr := v.(string); isStr && sv != "" {
						visExpr = sv
					}
				}
			}
			result = append(result, pb.clonePropertyWithExpression(propMap, visExpr))

		case "columnClass":
			classExpr := ""
			if col.Properties != nil {
				if v, ok := col.Properties["DynamicCellClass"]; ok {
					if sv, isStr := v.(string); isStr {
						classExpr = sv
					}
				}
			}
			result = append(result, pb.clonePropertyWithExpression(propMap, classExpr))

		// Mode-dependent properties: adjust for customContent vs attribute mode
		case "tooltip":
			if hasCustomContent {
				// tooltip is HIDDEN in customContent mode — clear TextTemplate
				result = append(result, pb.clonePropertyClearingTextTemplate(propMap))
			} else {
				tooltipText := ""
				if col.Properties != nil {
					if v, ok := col.Properties["Tooltip"]; ok {
						if sv, isStr := v.(string); isStr {
							tooltipText = sv
						}
					}
				}
				if tooltipText != "" {
					entry := columnPropertyIDs["tooltip"]
					result = append(result, pb.buildColumnHeaderProperty(entry, tooltipText))
				} else {
					result = append(result, pb.clonePropertyWithNewIDs(propMap))
				}
			}
		case "exportValue":
			if hasCustomContent {
				// exportValue is VISIBLE in customContent mode — ensure it has a TextTemplate
				entry := columnPropertyIDs["exportValue"]
				result = append(result, pb.buildColumnHeaderProperty(entry, ""))
			} else {
				result = append(result, pb.clonePropertyWithNewIDs(propMap))
			}
		case "allowEventPropagation":
			// allowEventPropagation is VISIBLE in customContent mode (hidden in attribute mode).
			// Clone from template preserving its default value.
			result = append(result, pb.clonePropertyWithNewIDs(propMap))

		case "sortable":
			defaultSortable := "false"
			if attrPath != "" {
				defaultSortable = "true"
			}
			sortVal := colPropBool(col.Properties, "Sortable", defaultSortable)
			result = append(result, pb.clonePropertyWithPrimitiveValue(propMap, sortVal))

		case "resizable":
			resVal := colPropBool(col.Properties, "Resizable", "true")
			result = append(result, pb.clonePropertyWithPrimitiveValue(propMap, resVal))

		case "draggable":
			dragVal := colPropBool(col.Properties, "Draggable", "true")
			result = append(result, pb.clonePropertyWithPrimitiveValue(propMap, dragVal))

		case "hidable":
			hidVal := colPropString(col.Properties, "Hidable", "yes")
			result = append(result, pb.clonePropertyWithPrimitiveValue(propMap, hidVal))

		case "width":
			widthVal := colPropString(col.Properties, "ColumnWidth", "autoFill")
			result = append(result, pb.clonePropertyWithPrimitiveValue(propMap, widthVal))

		case "size":
			sizeVal := colPropInt(col.Properties, "Size", "1")
			result = append(result, pb.clonePropertyWithPrimitiveValue(propMap, sizeVal))

		case "wrapText":
			wrapVal := "false"
			if col.Properties != nil {
				if v, ok := col.Properties["WrapText"]; ok {
					if bv, isBool := v.(bool); isBool && bv {
						wrapVal = "true"
					} else if sv, isStr := v.(string); isStr {
						wrapVal = strings.ToLower(sv)
					}
				}
			}
			result = append(result, pb.clonePropertyWithPrimitiveValue(propMap, wrapVal))

		case "alignment":
			alignVal := "left"
			if col.Properties != nil {
				if v, ok := col.Properties["Alignment"]; ok {
					if sv, isStr := v.(string); isStr && sv != "" {
						alignVal = strings.ToLower(sv)
					}
				}
			}
			result = append(result, pb.clonePropertyWithPrimitiveValue(propMap, alignVal))

		default:
			// Clone all other properties from template with regenerated IDs
			result = append(result, pb.clonePropertyWithNewIDs(propMap))
		}
	}

	// Add required properties that were missing from template
	if !addedProps["visible"] {
		if visibleEntry, ok := columnPropertyIDs["visible"]; ok {
			visExpr := "true"
			if col.Properties != nil {
				if v, ok := col.Properties["Visible"]; ok {
					if sv, isStr := v.(string); isStr && sv != "" {
						visExpr = sv
					}
				}
			}
			result = append(result, pb.buildColumnExpressionProperty(visibleEntry, visExpr))
		}
	}

	return result
}

func (pb *pageBuilder) buildDataGrid2Object(propertyTypeIDs map[string]pages.PropertyTypeIDEntry, objectTypeID string, datasource pages.DataSource, columns []ast.DataGridColumnDef, headerWidgets []bson.D) bson.D {
	properties := bson.A{int32(2)} // Version marker for non-empty array

	// Create properties for ALL entries in propertyTypeIDs
	// This ensures Studio Pro can display the widget's properties panel
	for key, entry := range propertyTypeIDs {
		switch key {
		case "datasource":
			// Use actual datasource value
			properties = append(properties, pb.buildDataGrid2Property(entry, datasource, "", ""))
		case "columns":
			// Use actual columns
			properties = append(properties, pb.buildDataGrid2ColumnsProperty(entry, propertyTypeIDs, columns))
		case "filtersPlaceholder":
			// Use header widgets if provided (for HEADER section support)
			if len(headerWidgets) > 0 {
				properties = append(properties, pb.buildFiltersPlaceholderProperty(entry, headerWidgets))
			} else {
				properties = append(properties, pb.buildDataGrid2DefaultProperty(entry))
			}
		default:
			// Create property with default value from template
			properties = append(properties, pb.buildDataGrid2DefaultProperty(entry))
		}
	}

	// Build TypePointer - references the WidgetObjectType
	var typePointer any
	if objectTypeID != "" {
		typePointer = bsonutil.IDToBsonBinary(objectTypeID)
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "Properties", Value: properties},
		{Key: "TypePointer", Value: typePointer},
	}
}

func (pb *pageBuilder) buildDataGrid2DefaultProperty(entry pages.PropertyTypeIDEntry) bson.D {
	// Determine default values based on value type
	var selectionValue string = "None"
	if entry.ValueType == "Selection" {
		// Selection type defaults to "None"
		selectionValue = "None"
	}

	// For TextTemplate properties, create a proper Forms$ClientTemplate structure
	// Studio Pro expects this even for empty values, otherwise it shows "widget definition changed"
	var textTemplate any
	if entry.ValueType == "TextTemplate" {
		textTemplate = pb.buildEmptyClientTemplate()
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NoAction"},
				{Key: "DisabledDuringExecution", Value: true},
			}},
			{Key: "AttributeRef", Value: nil},
			{Key: "DataSource", Value: nil},
			{Key: "EntityRef", Value: nil},
			{Key: "Expression", Value: ""},
			{Key: "Form", Value: ""},
			{Key: "Icon", Value: nil},
			{Key: "Image", Value: ""},
			{Key: "Microflow", Value: ""},
			{Key: "Nanoflow", Value: ""},
			{Key: "Objects", Value: bson.A{int32(2)}},
			{Key: "PrimitiveValue", Value: entry.DefaultValue},
			{Key: "Selection", Value: selectionValue},
			{Key: "SourceVariable", Value: nil},
			{Key: "TextTemplate", Value: textTemplate},
			{Key: "TranslatableValue", Value: nil},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
			{Key: "Widgets", Value: bson.A{int32(2)}},
			{Key: "XPathConstraint", Value: ""},
		}},
	}
}

func (pb *pageBuilder) buildEmptyClientTemplate() bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}}, // Empty items with version marker
		}},
		{Key: "Parameters", Value: bson.A{int32(2)}}, // Empty parameters
		{Key: "Template", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}}, // Empty items with version marker
		}},
	}
}

func (pb *pageBuilder) buildClientTemplateWithText(text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Template", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{
				int32(3),
				bson.D{
					{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
					{Key: "$Type", Value: "Texts$Translation"},
					{Key: "LanguageCode", Value: "en_US"},
					{Key: "Text", Value: text},
				},
			}},
		}},
	}
}

func (pb *pageBuilder) buildFiltersPlaceholderProperty(entry pages.PropertyTypeIDEntry, widgetsBSON []bson.D) bson.D {
	// Build the Widgets array with version marker
	widgetsArray := bson.A{int32(2)} // Version marker for non-empty array
	for _, w := range widgetsBSON {
		widgetsArray = append(widgetsArray, w)
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NoAction"},
				{Key: "DisabledDuringExecution", Value: true},
			}},
			{Key: "AttributeRef", Value: nil},
			{Key: "DataSource", Value: nil},
			{Key: "EntityRef", Value: nil},
			{Key: "Expression", Value: ""},
			{Key: "Form", Value: ""},
			{Key: "Icon", Value: nil},
			{Key: "Image", Value: ""},
			{Key: "Microflow", Value: ""},
			{Key: "Nanoflow", Value: ""},
			{Key: "Objects", Value: bson.A{int32(2)}},
			{Key: "PrimitiveValue", Value: ""},
			{Key: "Selection", Value: "None"},
			{Key: "SourceVariable", Value: nil},
			{Key: "TextTemplate", Value: nil},
			{Key: "TranslatableValue", Value: nil},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
			{Key: "Widgets", Value: widgetsArray},
			{Key: "XPathConstraint", Value: ""},
		}},
	}
}

func (pb *pageBuilder) buildDataGrid2ColumnsProperty(entry pages.PropertyTypeIDEntry, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, columns []ast.DataGridColumnDef) bson.D {
	// Build column objects using nested property IDs
	columnObjects := bson.A{int32(2)} // Version marker
	for i := range columns {
		columnObjects = append(columnObjects, pb.buildDataGrid2ColumnObject(&columns[i], entry.ObjectTypeID, entry.NestedPropertyIDs))
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NoAction"},
				{Key: "DisabledDuringExecution", Value: true},
			}},
			{Key: "AttributeRef", Value: nil},
			{Key: "DataSource", Value: nil},
			{Key: "EntityRef", Value: nil},
			{Key: "Expression", Value: ""},
			{Key: "Form", Value: ""},
			{Key: "Icon", Value: nil},
			{Key: "Image", Value: ""},
			{Key: "Microflow", Value: ""},
			{Key: "Nanoflow", Value: ""},
			{Key: "Objects", Value: columnObjects},
			{Key: "PrimitiveValue", Value: ""},
			{Key: "Selection", Value: "None"},
			{Key: "SourceVariable", Value: nil},
			{Key: "TextTemplate", Value: nil},
			{Key: "TranslatableValue", Value: nil},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
			{Key: "Widgets", Value: bson.A{int32(2)}},
			{Key: "XPathConstraint", Value: ""},
		}},
	}
}

func (pb *pageBuilder) buildDataGrid2ColumnObject(col *ast.DataGridColumnDef, columnObjectTypeID string, columnPropertyIDs map[string]pages.PropertyTypeIDEntry) bson.D {
	attrPath := pb.resolveAttributePath(col.Attribute)

	// Build content widgets if there are child widgets
	var contentWidgets []bson.D
	for _, child := range col.ChildrenV3 {
		widgetBSON, err := pb.buildWidgetV3ToBSON(child)
		if err != nil {
			// Log error and continue (don't fail the entire column)
			continue
		}
		if widgetBSON != nil {
			contentWidgets = append(contentWidgets, widgetBSON)
		}
	}
	hasCustomContent := len(contentWidgets) > 0

	// Column properties array - MUST include ALL properties from columnPropertyIDs
	properties := bson.A{int32(2)} // Version marker

	// Iterate through ALL column property types and create each one
	for key, entry := range columnPropertyIDs {
		switch key {
		case "showContentAs":
			// Set to "customContent" if we have custom widgets, otherwise "attribute"
			if hasCustomContent {
				properties = append(properties, pb.buildColumnPrimitiveProperty(entry, "customContent"))
			} else {
				properties = append(properties, pb.buildColumnPrimitiveProperty(entry, "attribute"))
			}

		case "attribute":
			// The actual attribute path
			if attrPath != "" {
				properties = append(properties, pb.buildColumnAttributeProperty(entry, attrPath))
			} else {
				properties = append(properties, pb.buildColumnDefaultProperty(entry))
			}

		case "header":
			// Caption for the column (TextTemplate type)
			if col.Caption != "" {
				properties = append(properties, pb.buildColumnHeaderProperty(entry, col.Caption))
			} else {
				// Use attribute name as default caption
				properties = append(properties, pb.buildColumnHeaderProperty(entry, col.Attribute))
			}

		case "content":
			// Content property with widgets (if any)
			if hasCustomContent {
				properties = append(properties, pb.buildColumnContentProperty(entry, contentWidgets))
			} else {
				properties = append(properties, pb.buildColumnContentProperty(entry, nil))
			}

		case "filter":
			// Filter property should have empty widget arrays (like Studio Pro)
			properties = append(properties, pb.buildColumnContentProperty(entry, nil))

		case "visible":
			// Expression-type property
			visExpr := "true"
			if col.Properties != nil {
				if v, ok := col.Properties["Visible"]; ok {
					if sv, isStr := v.(string); isStr && sv != "" {
						visExpr = sv
					}
				}
			}
			properties = append(properties, pb.buildColumnExpressionProperty(entry, visExpr))

		case "columnClass":
			// Expression-type property
			classExpr := ""
			if col.Properties != nil {
				if v, ok := col.Properties["DynamicCellClass"]; ok {
					if sv, isStr := v.(string); isStr {
						classExpr = sv
					}
				}
			}
			properties = append(properties, pb.buildColumnExpressionProperty(entry, classExpr))

		case "sortable":
			defaultSortable := "false"
			if attrPath != "" {
				defaultSortable = "true"
			}
			sortVal := colPropBool(col.Properties, "Sortable", defaultSortable)
			properties = append(properties, pb.buildColumnPrimitiveProperty(entry, sortVal))

		case "resizable":
			resVal := colPropBool(col.Properties, "Resizable", "true")
			properties = append(properties, pb.buildColumnPrimitiveProperty(entry, resVal))

		case "draggable":
			dragVal := colPropBool(col.Properties, "Draggable", "true")
			properties = append(properties, pb.buildColumnPrimitiveProperty(entry, dragVal))

		case "wrapText":
			wrapVal := colPropBool(col.Properties, "WrapText", "false")
			properties = append(properties, pb.buildColumnPrimitiveProperty(entry, wrapVal))

		case "alignment":
			alignVal := colPropString(col.Properties, "Alignment", "left")
			properties = append(properties, pb.buildColumnPrimitiveProperty(entry, alignVal))

		case "width":
			widthVal := colPropString(col.Properties, "ColumnWidth", "autoFill")
			properties = append(properties, pb.buildColumnPrimitiveProperty(entry, widthVal))

		case "minWidth":
			// Enumeration-type property - "auto", "setByContent", or "manual"
			properties = append(properties, pb.buildColumnPrimitiveProperty(entry, "auto"))

		case "size":
			sizeVal := colPropInt(col.Properties, "Size", "1")
			properties = append(properties, pb.buildColumnPrimitiveProperty(entry, sizeVal))

		case "hidable":
			hidVal := colPropString(col.Properties, "Hidable", "yes")
			properties = append(properties, pb.buildColumnPrimitiveProperty(entry, hidVal))

		case "tooltip":
			if hasCustomContent {
				// tooltip is HIDDEN in customContent mode — use empty TextTemplate
				properties = append(properties, pb.buildColumnDefaultProperty(entry))
			} else {
				tooltipText := ""
				if col.Properties != nil {
					if v, ok := col.Properties["Tooltip"]; ok {
						if sv, isStr := v.(string); isStr {
							tooltipText = sv
						}
					}
				}
				if tooltipText != "" {
					properties = append(properties, pb.buildColumnHeaderProperty(entry, tooltipText))
				} else {
					properties = append(properties, pb.buildColumnDefaultProperty(entry))
				}
			}

		default:
			// All other properties: use default value based on valueType
			switch entry.ValueType {
			case "Expression":
				// Expression properties need an expression value
				properties = append(properties, pb.buildColumnExpressionProperty(entry, ""))
			case "TextTemplate":
				// TextTemplate properties need proper structure
				properties = append(properties, pb.buildColumnDefaultProperty(entry))
			default:
				// Other types use default builder
				properties = append(properties, pb.buildColumnDefaultProperty(entry))
			}
		}
	}

	// Column ObjectType pointer
	var typePointer any
	if columnObjectTypeID != "" {
		typePointer = bsonutil.IDToBsonBinary(columnObjectTypeID)
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "Properties", Value: properties},
		{Key: "TypePointer", Value: typePointer},
	}
}

func (pb *pageBuilder) buildColumnDefaultProperty(entry pages.PropertyTypeIDEntry) bson.D {
	// For TextTemplate properties, create a proper Forms$ClientTemplate structure
	var textTemplate any
	if entry.ValueType == "TextTemplate" {
		textTemplate = pb.buildEmptyClientTemplate()
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NoAction"},
				{Key: "DisabledDuringExecution", Value: true},
			}},
			{Key: "AttributeRef", Value: nil},
			{Key: "DataSource", Value: nil},
			{Key: "EntityRef", Value: nil},
			{Key: "Expression", Value: ""},
			{Key: "Form", Value: ""},
			{Key: "Icon", Value: nil},
			{Key: "Image", Value: ""},
			{Key: "Microflow", Value: ""},
			{Key: "Nanoflow", Value: ""},
			{Key: "Objects", Value: bson.A{int32(2)}},
			{Key: "PrimitiveValue", Value: entry.DefaultValue},
			{Key: "Selection", Value: "None"},
			{Key: "SourceVariable", Value: nil},
			{Key: "TextTemplate", Value: textTemplate},
			{Key: "TranslatableValue", Value: nil},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
			{Key: "Widgets", Value: bson.A{int32(2)}},
			{Key: "XPathConstraint", Value: ""},
		}},
	}
}

func (pb *pageBuilder) buildColumnPrimitiveProperty(entry pages.PropertyTypeIDEntry, value string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NoAction"},
				{Key: "DisabledDuringExecution", Value: true},
			}},
			{Key: "AttributeRef", Value: nil},
			{Key: "DataSource", Value: nil},
			{Key: "EntityRef", Value: nil},
			{Key: "Expression", Value: ""},
			{Key: "Form", Value: ""},
			{Key: "Icon", Value: nil},
			{Key: "Image", Value: ""},
			{Key: "Microflow", Value: ""},
			{Key: "Nanoflow", Value: ""},
			{Key: "Objects", Value: bson.A{int32(2)}},
			{Key: "PrimitiveValue", Value: value},
			{Key: "Selection", Value: "None"},
			{Key: "SourceVariable", Value: nil},
			{Key: "TextTemplate", Value: nil},
			{Key: "TranslatableValue", Value: nil},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
			{Key: "Widgets", Value: bson.A{int32(2)}},
			{Key: "XPathConstraint", Value: ""},
		}},
	}
}

func (pb *pageBuilder) buildColumnExpressionProperty(entry pages.PropertyTypeIDEntry, expression string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NoAction"},
				{Key: "DisabledDuringExecution", Value: true},
			}},
			{Key: "AttributeRef", Value: nil},
			{Key: "DataSource", Value: nil},
			{Key: "EntityRef", Value: nil},
			{Key: "Expression", Value: expression},
			{Key: "Form", Value: ""},
			{Key: "Icon", Value: nil},
			{Key: "Image", Value: ""},
			{Key: "Microflow", Value: ""},
			{Key: "Nanoflow", Value: ""},
			{Key: "Objects", Value: bson.A{int32(2)}},
			{Key: "PrimitiveValue", Value: ""},
			{Key: "Selection", Value: "None"},
			{Key: "SourceVariable", Value: nil},
			{Key: "TextTemplate", Value: nil},
			{Key: "TranslatableValue", Value: nil},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
			{Key: "Widgets", Value: bson.A{int32(2)}},
			{Key: "XPathConstraint", Value: ""},
		}},
	}
}

func (pb *pageBuilder) buildColumnAttributeProperty(entry pages.PropertyTypeIDEntry, attrPath string) bson.D {
	// AttributeRef requires a fully qualified path (Module.Entity.Attribute, 2+ dots).
	// If the path is not fully qualified, set AttributeRef to nil to avoid Studio Pro crash.
	var attributeRef any
	if strings.Count(attrPath, ".") >= 2 {
		attributeRef = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "DomainModels$AttributeRef"},
			{Key: "Attribute", Value: attrPath},
			{Key: "EntityRef", Value: nil},
		}
	}
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NoAction"},
				{Key: "DisabledDuringExecution", Value: true},
			}},
			{Key: "AttributeRef", Value: attributeRef},
			{Key: "DataSource", Value: nil},
			{Key: "EntityRef", Value: nil},
			{Key: "Expression", Value: ""},
			{Key: "Form", Value: ""},
			{Key: "Icon", Value: nil},
			{Key: "Image", Value: ""},
			{Key: "Microflow", Value: ""},
			{Key: "Nanoflow", Value: ""},
			{Key: "Objects", Value: bson.A{int32(2)}},
			{Key: "PrimitiveValue", Value: ""},
			{Key: "Selection", Value: "None"},
			{Key: "SourceVariable", Value: nil},
			{Key: "TextTemplate", Value: nil},
			{Key: "TranslatableValue", Value: nil},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
			{Key: "Widgets", Value: bson.A{int32(2)}},
			{Key: "XPathConstraint", Value: ""},
		}},
	}
}

func (pb *pageBuilder) buildColumnHeaderProperty(entry pages.PropertyTypeIDEntry, caption string) bson.D {
	// Create the text template with the caption
	textTemplate := pb.buildClientTemplateWithText(caption)

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NoAction"},
				{Key: "DisabledDuringExecution", Value: true},
			}},
			{Key: "AttributeRef", Value: nil},
			{Key: "DataSource", Value: nil},
			{Key: "EntityRef", Value: nil},
			{Key: "Expression", Value: ""},
			{Key: "Form", Value: ""},
			{Key: "Icon", Value: nil},
			{Key: "Image", Value: ""},
			{Key: "Microflow", Value: ""},
			{Key: "Nanoflow", Value: ""},
			{Key: "Objects", Value: bson.A{int32(2)}},
			{Key: "PrimitiveValue", Value: ""},
			{Key: "Selection", Value: "None"},
			{Key: "SourceVariable", Value: nil},
			{Key: "TextTemplate", Value: textTemplate},
			{Key: "TranslatableValue", Value: nil},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
			{Key: "Widgets", Value: bson.A{int32(2)}},
			{Key: "XPathConstraint", Value: ""},
		}},
	}
}

func (pb *pageBuilder) buildColumnContentProperty(entry pages.PropertyTypeIDEntry, widgets any) bson.D {
	// Widgets array containing the widgets
	widgetsArray := bson.A{int32(2)}
	switch w := widgets.(type) {
	case bson.D:
		if w != nil {
			widgetsArray = append(widgetsArray, w)
		}
	case []bson.D:
		for _, widget := range w {
			widgetsArray = append(widgetsArray, widget)
		}
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NoAction"},
				{Key: "DisabledDuringExecution", Value: true},
			}},
			{Key: "AttributeRef", Value: nil},
			{Key: "DataSource", Value: nil},
			{Key: "EntityRef", Value: nil},
			{Key: "Expression", Value: ""},
			{Key: "Form", Value: ""},
			{Key: "Icon", Value: nil},
			{Key: "Image", Value: ""},
			{Key: "Microflow", Value: ""},
			{Key: "Nanoflow", Value: ""},
			{Key: "Objects", Value: bson.A{int32(2)}},
			{Key: "PrimitiveValue", Value: ""},
			{Key: "Selection", Value: "None"},
			{Key: "SourceVariable", Value: nil},
			{Key: "TextTemplate", Value: nil},
			{Key: "TranslatableValue", Value: nil},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
			{Key: "Widgets", Value: widgetsArray},
			{Key: "XPathConstraint", Value: ""},
		}},
	}
}
