// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// The Image widget's image can be the image object in context — a data view's
// or a list's object whose entity specialises System.Image — and not only an
// entry of an image collection. MDL spells it `Image: $currentObject`
// (mendixlabs/mxcli#1476).
//
// Measured on Studio Pro 11.12.1 with the Image widget 1.5.0: setting "Image
// source" to the data view's object changes exactly two values in the stored
// page. `imageObject` loses its collection image (Image '') and gains an
// EntityRef, a DirectEntityRef to the data view's entity; `defaultImageDynamic`
// gains the same EntityRef. No source variable is stored. The entity is
// therefore the enclosing data container's, which is what `$currentObject`
// already means everywhere else in a page.
//
// `DefaultImage:` is not part of that measurement: the reference page set none.
// It is written as a plain collection image on `defaultImageDynamic`, WITHOUT
// the EntityRef. Measured in the running app (Image widget 1.5.0, an object
// whose HasContents is false): with the EntityRef beside the collection image
// the widget renders hidden with no src; without it, it shows the collection
// image. Both pass mx check.
//
// Before this, MDL could only point `imageObject` at a collection image, so a
// React-client page had no way to show an uploaded image: the legacy
// `dynamicimage` is refused there with CE0582.

// isImageContextRef reports whether ds is the context object, `$currentObject`.
func isImageContextRef(ds *ast.DataSourceV3) bool {
	return ds != nil && ds.Type == "parameter" &&
		strings.EqualFold(strings.TrimPrefix(ds.Reference, "$"), "currentObject")
}

// isImageObjectValue reports whether the widget's property key holds an
// object rather than a collection image's name — any variable-led value, the
// supported `$currentObject` and the forms imageObjectBinding refuses alike.
func isImageObjectValue(w *ast.WidgetV3, key string) bool {
	v, ok := lookupProperty(w.Properties, key)
	if !ok {
		return false
	}
	_, isDS := v.(*ast.DataSourceV3)
	return isDS
}

// imageObjectBinding returns the entity of the image object the widget binds
// an image-typed property to, or "" when every image-typed property names a
// collection image or nothing.
//
// Only the enclosing data container's object is accepted. Studio Pro also
// offers a page parameter and, presumably, an associated object; their stored
// shapes have not been measured, so they are refused here rather than written
// in a form Studio Pro might not read back.
func (e *PluggableWidgetEngine) imageObjectBinding(mappings []PropertyMapping, w *ast.WidgetV3) (string, error) {
	for _, m := range mappings {
		if m.Operation != "image" || m.Source == "" {
			continue
		}
		v, ok := lookupProperty(w.Properties, m.Source)
		if !ok {
			continue
		}
		ds, isDS := v.(*ast.DataSourceV3)
		if !isDS {
			continue
		}
		if !isImageContextRef(ds) {
			return "", mdlerrors.NewValidationf(
				"widget `%s` property `%s`: %s — an image object is bound only as `$currentObject`, "+
					"the object of the enclosing data view or list; place the widget in a data view "+
					"over the object you want to show (`dataview v (DataSource: $Photo) { image i (Image: $currentObject) }`)",
				widgetLabelForError(w), m.Source, formatDataSourceV3(ds))
		}
		entity := e.outerEntityContext
		if entity == "" {
			return "", mdlerrors.NewValidationf(
				"widget `%s` property `%s`: `$currentObject` names the object of the enclosing data view "+
					"or list, and this widget has none — place it inside a data view, list view or gallery "+
					"whose entity specialises System.Image",
				widgetLabelForError(w), m.Source)
		}
		// mxbuild refuses any other entity with CE7247 "Widget expects entity of
		// type System.Image.", once per bound property — measured on 11.12.1.
		if !e.pageBuilder.isSpecializationOf(entity, systemImageEntity) {
			return "", mdlerrors.NewValidationf(
				"widget `%s` property `%s`: `$currentObject` is a %s, which does not specialise %s — "+
					"the Image widget shows only an image object (mxbuild: CE7247 \"Widget expects entity "+
					"of type System.Image.\"); bind it inside a data view over an entity that extends %s",
				widgetLabelForError(w), m.Source, entity, systemImageEntity, systemImageEntity)
		}
		return entity, nil
	}
	return "", nil
}

const systemImageEntity = "System.Image"
