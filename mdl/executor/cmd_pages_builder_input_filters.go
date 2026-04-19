// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/widgets"
	"go.mongodb.org/mongo-driver/bson"
)

func (pb *pageBuilder) getFilterWidgetIDForAttribute(attrPath string) string {
	attrType := pb.findAttributeType(attrPath)
	if attrType == nil {
		return pages.WidgetIDDataGridTextFilter // Default to text filter
	}

	switch attrType.(type) {
	case *domainmodel.StringAttributeType:
		return pages.WidgetIDDataGridTextFilter
	case *domainmodel.IntegerAttributeType, *domainmodel.LongAttributeType,
		*domainmodel.DecimalAttributeType, *domainmodel.AutoNumberAttributeType:
		return pages.WidgetIDDataGridNumberFilter
	case *domainmodel.DateTimeAttributeType, *domainmodel.DateAttributeType:
		return pages.WidgetIDDataGridDateFilter
	case *domainmodel.BooleanAttributeType, *domainmodel.EnumerationAttributeType:
		return pages.WidgetIDDataGridDropdownFilter
	default:
		return pages.WidgetIDDataGridTextFilter
	}
}

func (pb *pageBuilder) findAttributeType(attrPath string) domainmodel.AttributeType {
	if attrPath == "" {
		return nil
	}

	// Parse the attribute path
	parts := strings.Split(attrPath, ".")
	var entityName, attrName string

	if len(parts) >= 3 {
		// Format: Module.Entity.Attribute
		entityName = parts[0] + "." + parts[1]
		attrName = parts[len(parts)-1]
	} else if len(parts) == 2 {
		// Could be Entity.Attribute or Module.Entity - use context
		if pb.entityContext != "" {
			entityName = pb.entityContext
			attrName = parts[len(parts)-1]
		} else {
			// Assume Module.Entity format without attribute
			return nil
		}
	} else {
		// Just attribute name, use entity context
		if pb.entityContext != "" {
			entityName = pb.entityContext
			attrName = parts[0]
		} else {
			return nil
		}
	}

	// Find the entity and attribute
	domainModels, err := pb.getDomainModels()
	if err != nil {
		return nil
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return nil
	}

	// Parse entity qualified name
	entityParts := strings.Split(entityName, ".")
	if len(entityParts) < 2 {
		return nil
	}
	moduleName := entityParts[0]
	entityShortName := entityParts[1]

	// Find the entity
	for _, dm := range domainModels {
		modName := h.GetModuleName(dm.ContainerID)
		if modName != moduleName {
			continue
		}
		for _, entity := range dm.Entities {
			if entity.Name == entityShortName {
				attr := entity.FindAttributeByName(attrName)
				if attr != nil {
					return attr.Type
				}
				return nil
			}
		}
	}

	return nil
}

func (pb *pageBuilder) buildFilterWidgetBSON(widgetID, filterName string) bson.D {
	// Load the filter widget template
	rawType, rawObject, propertyTypeIDs, objectTypeID, err := widgets.GetTemplateFullBSON(widgetID, types.GenerateID, pb.reader.Path())
	if err != nil || rawType == nil {
		// Fallback: create minimal filter widget structure
		return pb.buildMinimalFilterWidgetBSON(widgetID, filterName)
	}

	// The widget structure is: CustomWidgets$CustomWidget with Type and Object
	widgetBSON := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Editable", Value: "Inherited"},
		{Key: "Name", Value: filterName},
		{Key: "Object", Value: rawObject},
		{Key: "TabIndex", Value: int32(0)},
		{Key: "Type", Value: rawType},
	}

	// Set the "linkedDs" property to "auto" mode (which links to parent datasource)
	if propertyTypeIDs != nil && rawObject != nil {
		widgetBSON = pb.setFilterWidgetLinkedDsAuto(widgetBSON, propertyTypeIDs, objectTypeID)
	}

	return widgetBSON
}

func (pb *pageBuilder) setFilterWidgetLinkedDsAuto(widget bson.D, propertyTypeIDs map[string]widgets.PropertyTypeIDEntry, objectTypeID string) bson.D {
	// The filter widgets have an "attrChoice" property that should be set to "auto"
	// which makes them automatically link to the parent datasource
	return widget
}

func (pb *pageBuilder) buildMinimalFilterWidgetBSON(widgetID, filterName string) bson.D {
	typeID := types.GenerateID()
	objectTypeID := types.GenerateID()
	objectID := types.GenerateID()

	// Get widget type name based on ID
	var widgetTypeName string
	switch widgetID {
	case pages.WidgetIDDataGridTextFilter:
		widgetTypeName = "Text filter"
	case pages.WidgetIDDataGridNumberFilter:
		widgetTypeName = "Number filter"
	case pages.WidgetIDDataGridDateFilter:
		widgetTypeName = "Date filter"
	case pages.WidgetIDDataGridDropdownFilter:
		widgetTypeName = "Drop-down filter"
	default:
		widgetTypeName = "Text filter"
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Editable", Value: "Inherited"},
		{Key: "Name", Value: filterName},
		{Key: "Object", Value: bson.D{
			{Key: "$ID", Value: bsonutil.IDToBsonBinary(objectID)},
			{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
			{Key: "Properties", Value: bson.A{int32(2)}},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(objectTypeID)},
		}},
		{Key: "TabIndex", Value: int32(0)},
		{Key: "Type", Value: bson.D{
			{Key: "$ID", Value: bsonutil.IDToBsonBinary(typeID)},
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "HelpUrl", Value: ""},
			{Key: "ObjectType", Value: bson.D{
				{Key: "$ID", Value: bsonutil.IDToBsonBinary(objectTypeID)},
				{Key: "$Type", Value: "CustomWidgets$WidgetObjectType"},
				{Key: "PropertyTypes", Value: bson.A{int32(2)}},
			}},
			{Key: "OfflineCapable", Value: true},
			{Key: "StudioCategory", Value: "Data Controls"},
			{Key: "StudioProCategory", Value: "Data controls"},
			{Key: "SupportedPlatform", Value: "Web"},
			{Key: "WidgetDescription", Value: ""},
			{Key: "WidgetId", Value: widgetID},
			{Key: "WidgetName", Value: widgetTypeName},
			{Key: "WidgetNeedsEntityContext", Value: false},
			{Key: "WidgetPluginWidget", Value: true},
		}},
	}
}
