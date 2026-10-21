// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"sync"

	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
)

// explicitPassWritableTypes are the property value types the widget engine's
// explicit-property pass (PluggableWidgetEngine.Build, step 4.6) writes by
// storage key: Expression, TextTemplate, Attribute and Selection through their
// own setters, and the scalar types as a primitive. A value of any other type
// (objects, widgets, icons, images, actions, datasources…) has no correct
// route there, so MDL-WIDGET06 stays true for it.
var explicitPassWritableTypes = map[string]bool{
	"expression": true, "texttemplate": true, "attribute": true, "selection": true,
	"string": true, "boolean": true, "integer": true, "decimal": true, "enumeration": true,
}

var widgetPropertyTypesCache sync.Map // widgetID → map[lowercased key]lowercased type

// widgetPropertyTypes returns each property key's declared value type, from
// the widget's embedded template. Empty when there is no template.
func widgetPropertyTypes(widgetID string) map[string]string {
	if v, ok := widgetPropertyTypesCache.Load(widgetID); ok {
		return v.(map[string]string)
	}
	out := map[string]string{}
	if tmpl, err := mwidgets.GetTemplate(widgetID); err == nil && tmpl != nil {
		for _, p := range propsFromTemplate(tmpl.Type) {
			if p.Key != "" && p.Type != "" {
				out[strings.ToLower(p.Key)] = strings.ToLower(p.Type)
			}
		}
	}
	widgetPropertyTypesCache.Store(widgetID, out)
	return out
}

// persistedByExplicitPass reports whether a key MDL-WIDGET06 would call "not
// yet persisted" is in fact written by the explicit-property pass.
//
// It was not: `optionsSourceAssociationCaptionType: expression` and
// `optionsSourceAssociationCaptionExpression: '…'` on a ComboBox are both
// written, and the page builds clean (measured on Mendix 11.13.0, #664) — yet
// check warned that each "will be dropped". DESCRIBE now emits exactly those
// keys for an expression caption, so the false warning would land on every
// round trip of Administration.Account_New.
//
// An unknown type keeps the warning: saying "not persisted" for something that
// is, is the lesser error than the reverse.
func persistedByExplicitPass(widgetID, lowerKey string) bool {
	return explicitPassWritableTypes[widgetPropertyTypes(widgetID)[lowerKey]]
}
