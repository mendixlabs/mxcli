// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/widgetobj"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/widgets"
)

// The pluggable-widget object builder now lives in the engine-agnostic
// mdl/backend/widgetobj package. This file keeps only the MprBackend-specific
// glue: loading templates from sdk/widgets, the legacy child serializer, and the
// opaque-serialization helpers.

// mprChildSerializer routes the builder's child-content serialization through the
// legacy sdk/mpr serializers (byte-identical to before the extraction).
type mprChildSerializer struct{}

func (mprChildSerializer) SerializeWidget(w pages.Widget) bson.D { return mpr.SerializeWidget(w) }
func (mprChildSerializer) SerializeClientAction(a pages.ClientAction) bson.D {
	return mpr.SerializeClientAction(a)
}
func (mprChildSerializer) SerializeCustomWidgetDataSource(ds pages.DataSource) bson.D {
	return mpr.SerializeCustomWidgetDataSource(ds)
}

// LoadWidgetTemplate loads a widget template by ID and returns a builder.
func (b *MprBackend) LoadWidgetTemplate(widgetID string, projectPath string) (backend.WidgetObjectBuilder, error) {
	embeddedType, embeddedObject, embeddedIDs, objectTypeID, err :=
		widgets.GetTemplateFullBSON(widgetID, types.GenerateID, projectPath)
	if err != nil {
		return nil, err
	}
	if embeddedType == nil || embeddedObject == nil {
		return nil, nil
	}
	return widgetobj.New(widgetID, embeddedType, embeddedObject,
		convertPropertyTypeIDs(embeddedIDs), objectTypeID, mprChildSerializer{}), nil
}

// SerializeWidgetToOpaque converts a domain Widget to opaque BSON form.
func (b *MprBackend) SerializeWidgetToOpaque(w pages.Widget) any {
	return mpr.SerializeWidget(w)
}

// SerializeDataSourceToOpaque converts a domain DataSource to opaque BSON form.
func (b *MprBackend) SerializeDataSourceToOpaque(ds pages.DataSource) any {
	return mpr.SerializeCustomWidgetDataSource(ds)
}

// BuildCreateAttributeObject creates an attribute object for filter widgets.
func (b *MprBackend) BuildCreateAttributeObject(attributePath string, objectTypeID, propertyTypeID, valueTypeID string) (any, error) {
	return widgetobj.CreateAttributeObject(attributePath, objectTypeID, propertyTypeID, valueTypeID)
}

// convertPropertyTypeIDs maps the sdk/widgets template's property-type IDs to the
// pages form the builder uses.
func convertPropertyTypeIDs(src map[string]widgets.PropertyTypeIDEntry) map[string]pages.PropertyTypeIDEntry {
	result := make(map[string]pages.PropertyTypeIDEntry)
	for k, v := range src {
		entry := pages.PropertyTypeIDEntry{
			PropertyTypeID: v.PropertyTypeID,
			ValueTypeID:    v.ValueTypeID,
			DefaultValue:   v.DefaultValue,
			ValueType:      v.ValueType,
			Required:       v.Required,
			ObjectTypeID:   v.ObjectTypeID,
		}
		if len(v.NestedPropertyIDs) > 0 {
			entry.NestedPropertyIDs = convertPropertyTypeIDs(v.NestedPropertyIDs)
			entry.NestedKeyOrder = append([]string(nil), v.NestedKeyOrder...)
		}
		result[k] = entry
	}
	return result
}
