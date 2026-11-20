// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// NullTemplatesOfUnsetDataSources stores TextTemplate: null for an unset
// TextTemplate property bound to a data source (widget.xml `dataSource="X"`)
// when X holds no data source (ako/mxcli#1967, mendixlabs/mxcli#1745). Studio
// Pro writes it that way, and `mx update-widgets` changes mxcli's empty
// Forms$ClientTemplate to null and nothing else; the empty template is CE0463
// "The definition of this widget has changed". With X set, an unset template
// stays empty (the reporter's control), so the rule keys on X, not on the
// template.
//
// Only an UNSET template is nulled: authored text is the statement's, and is
// never dropped here. Only the widget's own properties: a binding to the
// enclosing widget's data source ("../datasource", a column's) names one that
// is always set.
func NullTemplatesOfUnsetDataSources(object bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry) bson.D {
	values := map[string]bson.D{}
	for _, elem := range object {
		if elem.Key != "Properties" {
			continue
		}
		arr, _ := elem.Value.(bson.A)
		for _, item := range arr {
			if prop, ok := item.(bson.D); ok {
				if id := propertyTypePointerID(prop); id != "" {
					values[id] = widgetValueOfProperty(prop)
				}
			}
		}
	}
	valueOf := func(key string) (bson.D, bool) {
		entry, ok := propertyTypeIDs[key]
		if !ok {
			return nil, false
		}
		v, ok := values[strings.ReplaceAll(entry.PropertyTypeID, "-", "")]
		return v, ok && v != nil
	}
	var keys []string
	for key, entry := range propertyTypeIDs {
		ds := entry.DataSourceProperty
		if !strings.EqualFold(entry.ValueType, "TextTemplate") || ds == "" || strings.Contains(ds, "/") {
			continue
		}
		dsValue, ok := valueOf(ds)
		if !ok || !bsonFieldIsNil(dsValue, "DataSource") {
			continue // the data source is set, or not known: leave the template
		}
		if tv, ok := valueOf(key); ok && isUnsetClientTemplate(tv) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		object = updateWidgetPropertyValue(object, propertyTypeIDs, key, func(val bson.D) bson.D {
			return setBSONField(val, "TextTemplate", nil)
		})
	}
	return object
}

// isUnsetClientTemplate reports whether a WidgetValue's TextTemplate is an
// empty Forms$ClientTemplate: no translation in Template, and no parameters.
// null is not "unset" here — there is nothing to change.
func isUnsetClientTemplate(val bson.D) bool {
	var tmpl bson.D
	for _, e := range val {
		if e.Key == "TextTemplate" {
			tmpl, _ = e.Value.(bson.D)
		}
	}
	if tmpl == nil {
		return false
	}
	for _, e := range tmpl {
		switch e.Key {
		case "Parameters":
			if arr, ok := e.Value.(bson.A); ok && len(arr) > 1 {
				return false
			}
		case "Template":
			text, _ := e.Value.(bson.D)
			for _, te := range text {
				if te.Key == "Items" {
					if arr, ok := te.Value.(bson.A); ok && len(arr) > 1 {
						return false
					}
				}
			}
		}
	}
	return true
}

// NullTemplatesOfUnsetDataSources applies the rule to the object being built.
func (ob *Builder) NullTemplatesOfUnsetDataSources() {
	ob.object = NullTemplatesOfUnsetDataSources(ob.object, ob.propertyTypeIDs)
}
