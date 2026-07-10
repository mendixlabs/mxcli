// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================================
// Custom/Pluggable Widget Serialization
// ============================================================================

// serializeCustomWidget serializes a CustomWidget (pluggable widget) to BSON.
// If the widget has a RawType (cloned from existing widget), use that instead.
func serializeCustomWidget(cw *pages.CustomWidget) bson.D {
	// Check if we have a raw type definition to use
	if cw.RawType != nil {
		return serializeCustomWidgetWithRawType(cw)
	}

	// Build widget type from structured data
	widgetType := serializeCustomWidgetType(cw.WidgetType)

	// Build widget object (properties)
	widgetObject := serializeWidgetObject(cw.WidgetObject)

	editable := cw.Editable
	if editable == "" {
		editable = "Always"
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(cw.ID))},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Appearance", Value: serializeAppearance(cw.Class, cw.Style, cw.DynamicClasses, cw.DesignProperties)},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Editable", Value: editable},
		{Key: "LabelTemplate", Value: serializeLabelTemplate(cw.Label)},
		{Key: "Name", Value: cw.Name},
		{Key: "Object", Value: widgetObject},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Type", Value: widgetType},
	}

	return doc
}

// serializeCustomWidgetWithRawType serializes a CustomWidget using a pre-cloned raw type definition.
func serializeCustomWidgetWithRawType(cw *pages.CustomWidget) bson.D {
	// Use the cloned RawObject if available (contains all property values)
	// Otherwise fall back to building from WidgetObject with PropertyTypeIDMap
	var widgetObject any
	if cw.RawObject != nil {
		widgetObject = cw.RawObject
	} else {
		// Build widget object (properties) - this still needs to match the raw type's PropertyType IDs
		// The ObjectTypeID is used to set the TypePointer on the WidgetObject
		widgetObject = serializeWidgetObjectForRawType(cw.WidgetObject, cw.PropertyTypeIDMap, cw.ObjectTypeID)
	}

	editable := cw.Editable
	if editable == "" {
		editable = "Always"
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(cw.ID))},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Appearance", Value: serializeAppearance(cw.Class, cw.Style, cw.DynamicClasses, cw.DesignProperties)},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Editable", Value: editable},
		{Key: "LabelTemplate", Value: serializeLabelTemplate(cw.Label)},
		{Key: "Name", Value: cw.Name},
		{Key: "Object", Value: widgetObject},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Type", Value: cw.RawType},
	}

	return doc
}

// serializeWidgetObjectForRawType serializes WidgetObject using the PropertyType IDs from a cloned type.
// The objectTypeID parameter is used to set the TypePointer which references the WidgetObjectType.
func serializeWidgetObjectForRawType(wo *pages.WidgetObject, propTypeIDMap map[string]pages.PropertyTypeIDEntry, objectTypeID string) bson.D {
	if wo == nil {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
			{Key: "Properties", Value: bson.A{int32(3)}},
			{Key: "TypePointer", Value: nil},
		}
	}

	id := string(wo.ID)
	if id == "" {
		id = generateUUID()
	}

	var properties bson.A
	if len(wo.Properties) == 0 {
		properties = bson.A{int32(3)}
	} else {
		properties = bson.A{int32(2)} // Version marker for non-empty array
		for _, prop := range wo.Properties {
			// Look up the PropertyType IDs from the map
			var propertyTypeID, valueTypeID string
			if propTypeIDMap != nil && prop.PropertyKey != "" {
				if ids, ok := propTypeIDMap[prop.PropertyKey]; ok {
					propertyTypeID = ids.PropertyTypeID
					valueTypeID = ids.ValueTypeID
				}
			}
			properties = append(properties, serializeWidgetPropertyForRawType(prop, propertyTypeID, valueTypeID))
		}
	}

	// Build TypePointer - references the WidgetObjectType from the cloned CustomWidgetType
	var typePointer any
	if objectTypeID != "" {
		typePointer = idToBsonBinary(objectTypeID)
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "Properties", Value: properties},
		{Key: "TypePointer", Value: typePointer},
	}
}

// serializeWidgetPropertyForRawType serializes a widget property using specific PropertyType and ValueType IDs.
func serializeWidgetPropertyForRawType(prop *pages.WidgetProperty, propertyTypeID, valueTypeID string) bson.D {
	if prop == nil {
		return nil
	}

	id := string(prop.ID)
	if id == "" {
		id = generateUUID()
	}

	// Use the provided IDs, or fall back to the property's TypePointer
	ptID := propertyTypeID
	if ptID == "" {
		ptID = string(prop.TypePointer)
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: idToBsonBinary(ptID)},
		{Key: "Value", Value: serializeWidgetValueForRawType(prop.Value, valueTypeID)},
	}
}

// serializeWidgetValueForRawType serializes a widget value using a specific ValueType ID.
func serializeWidgetValueForRawType(val *pages.WidgetValue, valueTypeID string) bson.D {
	if val == nil {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: serializeClientAction(nil)},
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
			{Key: "TypePointer", Value: idToBsonBinary(valueTypeID)},
			{Key: "Widgets", Value: bson.A{int32(2)}},
		}
	}

	id := string(val.ID)
	if id == "" {
		id = generateUUID()
	}

	// Serialize DataSource if present
	var dataSource any
	if val.DataSource != nil {
		dataSource = SerializeCustomWidgetDataSource(val.DataSource)
	}

	// Serialize widgets if present
	widgets := bson.A{int32(2)}
	for _, w := range val.Widgets {
		widgets = append(widgets, serializeWidget(w))
	}

	// Use the provided ValueType ID
	var typePointer any
	if valueTypeID != "" {
		typePointer = idToBsonBinary(valueTypeID)
	} else if val.TypePointer != "" {
		typePointer = idToBsonBinary(string(val.TypePointer))
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
		{Key: "Action", Value: serializeClientAction(val.Action)},
		{Key: "AttributeRef", Value: serializeAttributeRef(val.AttributeRef)},
		{Key: "DataSource", Value: dataSource},
		{Key: "EntityRef", Value: serializeEntityRef(val.EntityRef)},
		{Key: "Expression", Value: val.Expression},
		{Key: "Form", Value: val.Form},
		{Key: "Icon", Value: nil},
		{Key: "Image", Value: val.Image},
		{Key: "Microflow", Value: val.Microflow},
		{Key: "Nanoflow", Value: val.Nanoflow},
		{Key: "Objects", Value: bson.A{int32(2)}},
		{Key: "PrimitiveValue", Value: val.PrimitiveValue},
		{Key: "Selection", Value: val.Selection},
		{Key: "SourceVariable", Value: nil},
		{Key: "TextTemplate", Value: nil},
		{Key: "TranslatableValue", Value: nil},
		{Key: "TypePointer", Value: typePointer},
		{Key: "Widgets", Value: widgets},
	}
}

// serializeCustomWidgetType serializes the CustomWidgetType.
func serializeCustomWidgetType(wt *pages.CustomWidgetType) bson.D {
	if wt == nil {
		return nil
	}

	id := string(wt.ID)
	if id == "" {
		id = generateUUID()
	}

	objectTypeID := generateUUID()

	supportedPlatform := wt.SupportedPlatform
	if supportedPlatform == "" {
		supportedPlatform = "Web"
	}

	// Use the ObjectType ID from wt if available
	otID := objectTypeID
	if wt.ObjectType != nil && wt.ObjectType.ID != "" {
		otID = string(wt.ObjectType.ID)
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
		{Key: "HelpUrl", Value: wt.HelpURL},
		{Key: "ObjectType", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(otID)},
			{Key: "$Type", Value: "CustomWidgets$WidgetObjectType"},
			{Key: "PropertyTypes", Value: serializePropertyTypes(wt.ObjectType)},
		}},
		{Key: "OfflineCapable", Value: wt.OfflineCapable},
		{Key: "StudioCategory", Value: ""},
		{Key: "StudioProCategory", Value: ""},
		{Key: "SupportedPlatform", Value: supportedPlatform},
		{Key: "WidgetDescription", Value: wt.Description},
		{Key: "WidgetId", Value: wt.WidgetID},
		{Key: "WidgetName", Value: wt.Name},
		{Key: "WidgetNeedsEntityContext", Value: wt.NeedsEntityContext},
		{Key: "WidgetPluginWidget", Value: wt.PluginWidget},
	}

	return doc
}

// serializePropertyTypes serializes the property types for a widget.
func serializePropertyTypes(ot *pages.WidgetObjectType) bson.A {
	if ot == nil || len(ot.PropertyTypes) == 0 {
		return bson.A{int32(3)}
	}

	arr := bson.A{int32(2)} // Version marker for non-empty array

	for _, pt := range ot.PropertyTypes {
		id := string(pt.ID)
		if id == "" {
			id = generateUUID()
		}
		// Use the ValueTypeID if provided, otherwise generate a new one
		vtID := string(pt.ValueTypeID)
		if vtID == "" {
			vtID = generateUUID()
		}
		arr = append(arr, bson.D{
			{Key: "$ID", Value: idToBsonBinary(id)},
			{Key: "$Type", Value: "CustomWidgets$WidgetPropertyType"},
			{Key: "Caption", Value: pt.Caption},
			{Key: "Category", Value: ""},
			{Key: "Description", Value: pt.Description},
			{Key: "IsDefault", Value: pt.IsDefault},
			{Key: "PropertyKey", Value: pt.Key},
			{Key: "ValueType", Value: serializeWidgetValueType(vtID, pt.ValueType)},
		})
	}

	return arr
}

// serializeWidgetValueType serializes a WidgetValueType for a property type.
// The id parameter is the ValueTypeID that WidgetValue.TypePointer should reference.
// The valueType string is converted to the appropriate Type enum value.
func serializeWidgetValueType(id string, valueType string) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "CustomWidgets$WidgetValueType"},
		{Key: "ActionVariables", Value: bson.A{int32(2)}},
		{Key: "AllowedTypes", Value: bson.A{int32(1)}},
		{Key: "AllowNonPersistableEntities", Value: false},
		{Key: "AllowUpload", Value: false},
		{Key: "AssociationTypes", Value: bson.A{int32(1)}},
		{Key: "DataSourceProperty", Value: ""},
		{Key: "DefaultType", Value: "None"},
		{Key: "DefaultValue", Value: ""},
		{Key: "EntityProperty", Value: ""},
		{Key: "EnumerationValues", Value: bson.A{int32(2)}},
		{Key: "IsList", Value: false},
		{Key: "IsPath", Value: "No"},
		{Key: "LinkableEntityTypes", Value: bson.A{int32(1)}},
		{Key: "MicroflowActionInfo", Value: nil},
		{Key: "ObjectType", Value: nil},
		{Key: "OnChangeProperty", Value: ""},
		{Key: "PathType", Value: "None"},
		{Key: "ReturnType", Value: nil},
		{Key: "SelectableObjectsProperty", Value: ""},
		{Key: "SelectionTypes", Value: bson.A{int32(1)}},
		{Key: "SetLabel", Value: false},
		{Key: "Translations", Value: bson.A{int32(2)}},
		{Key: "Type", Value: valueType},
	}
}

// serializeWidgetObject serializes the WidgetObject (property values).
func serializeWidgetObject(wo *pages.WidgetObject) bson.D {
	if wo == nil {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
			{Key: "Properties", Value: bson.A{int32(3)}},
		}
	}

	id := string(wo.ID)
	if id == "" {
		id = generateUUID()
	}

	var properties bson.A
	if len(wo.Properties) == 0 {
		properties = bson.A{int32(3)}
	} else {
		properties = bson.A{int32(2)} // Version marker for non-empty array
		for _, prop := range wo.Properties {
			properties = append(properties, serializeWidgetProperty(prop))
		}
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "Properties", Value: properties},
	}
}

// serializeWidgetProperty serializes a single widget property.
func serializeWidgetProperty(prop *pages.WidgetProperty) bson.D {
	if prop == nil {
		return nil
	}

	id := string(prop.ID)
	if id == "" {
		id = generateUUID()
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: idToBsonBinary(string(prop.TypePointer))},
		{Key: "Value", Value: serializeWidgetValue(prop.Value)},
	}
}

// serializeWidgetValue serializes a widget property value.
func serializeWidgetValue(val *pages.WidgetValue) bson.D {
	if val == nil {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: serializeClientAction(nil)},
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
			{Key: "TypePointer", Value: nil},
			{Key: "Widgets", Value: bson.A{int32(2)}},
		}
	}

	id := string(val.ID)
	if id == "" {
		id = generateUUID()
	}

	// Serialize DataSource if present
	var dataSource any
	if val.DataSource != nil {
		dataSource = SerializeCustomWidgetDataSource(val.DataSource)
	}

	// Serialize widgets if present
	widgets := bson.A{int32(2)}
	for _, w := range val.Widgets {
		widgets = append(widgets, serializeWidget(w))
	}

	// TypePointer should be null when not set
	var typePointer any
	if val.TypePointer != "" {
		typePointer = idToBsonBinary(string(val.TypePointer))
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
		{Key: "Action", Value: serializeClientAction(val.Action)},
		{Key: "AttributeRef", Value: serializeAttributeRef(val.AttributeRef)},
		{Key: "DataSource", Value: dataSource},
		{Key: "EntityRef", Value: serializeEntityRef(val.EntityRef)},
		{Key: "Expression", Value: val.Expression},
		{Key: "Form", Value: val.Form},
		{Key: "Icon", Value: nil},
		{Key: "Image", Value: val.Image},
		{Key: "Microflow", Value: val.Microflow},
		{Key: "Nanoflow", Value: val.Nanoflow},
		{Key: "Objects", Value: bson.A{int32(2)}},
		{Key: "PrimitiveValue", Value: val.PrimitiveValue},
		{Key: "Selection", Value: val.Selection},
		{Key: "SourceVariable", Value: nil},
		{Key: "TextTemplate", Value: nil},
		{Key: "TranslatableValue", Value: nil},
		{Key: "TypePointer", Value: typePointer},
		{Key: "Widgets", Value: widgets},
	}
}
