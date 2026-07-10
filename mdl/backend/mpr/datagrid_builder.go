// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"log"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/bsonutil"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/widgets"
)

// BuildFilterWidget builds a filter widget for DataGrid2.
func (b *MprBackend) BuildFilterWidget(spec backend.FilterWidgetSpec, projectPath string) (pages.Widget, error) {
	bsonD := b.buildFilterWidgetBSON(spec.WidgetID, spec.FilterName, projectPath)

	// Wrap the BSON in a CustomWidget
	w := &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CustomWidgets$CustomWidget",
			},
			Name: spec.FilterName,
		},
		Editable:  "Always",
		RawObject: getBsonField(bsonD, "Object"),
		RawType:   getBsonField(bsonD, "Type"),
	}
	return w, nil
}

// ===========================================================================
// Filter widget BSON construction
// ===========================================================================

func (b *MprBackend) buildFilterWidgetBSON(widgetID, filterName string, projectPath string) bson.D {
	rawType, rawObject, _, _, err := widgets.GetTemplateFullBSON(widgetID, types.GenerateID, projectPath)
	if err != nil || rawType == nil {
		if err != nil {
			log.Printf("warning: failed to load template for widget %s: %v; using minimal fallback", widgetID, err)
		}
		return b.buildMinimalFilterWidgetBSON(widgetID, filterName)
	}

	// A complete CustomWidget BSON requires Appearance, ConditionalEditability/
	// VisibilitySettings, LabelTemplate, and TabIndex alongside Type/Object.
	// Omitting Appearance triggers CE0463 ("definition of this widget has
	// changed") because Studio Pro requires every CustomWidget to carry the
	// full Forms$Page widget envelope, not just the inner widget-specific
	// payload. See docs/mpr-bson-shapes.md for reference.
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Appearance", Value: defaultEmptyAppearance()},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Editable", Value: "Always"},
		{Key: "LabelTemplate", Value: nil},
		{Key: "Name", Value: filterName},
		{Key: "Object", Value: rawObject},
		{Key: "TabIndex", Value: int32(0)},
		{Key: "Type", Value: rawType},
	}
}

// defaultEmptyAppearance returns the Forms$Appearance BSON for a widget that
// has no class, style, or design properties — matches what Studio Pro emits.
func defaultEmptyAppearance() bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$Appearance"},
		{Key: "Class", Value: ""},
		{Key: "DesignProperties", Value: bson.A{int32(3)}},
		{Key: "DynamicClasses", Value: ""},
		{Key: "Style", Value: ""},
	}
}

func (b *MprBackend) buildMinimalFilterWidgetBSON(widgetID, filterName string) bson.D {
	typeID := types.GenerateID()
	objectTypeID := types.GenerateID()
	objectID := types.GenerateID()

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
		{Key: "Appearance", Value: defaultEmptyAppearance()},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Editable", Value: "Always"},
		{Key: "LabelTemplate", Value: nil},
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

// ===========================================================================
// BSON field helpers
// ===========================================================================

func getBsonField(d bson.D, key string) bson.D {
	for _, elem := range d {
		if elem.Key == key {
			if nested, ok := elem.Value.(bson.D); ok {
				return nested
			}
		}
	}
	return nil
}
