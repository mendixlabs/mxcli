// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1476: no MDL form bound the Image widget to an image object,
// so a React-client page could not show an uploaded image (the legacy
// `dynamicimage` is CE0582 there).
//
// The reference is a page Studio Pro 11.12.1 wrote (Image widget 1.5.0): an
// image inside `dataview (DataSource: $Photo)`, where Photo extends
// System.Image, with "Image source" set to the data view's object. Decoding the
// unit before and after that one change gives exactly two differences:
//
//	imageObject          Image 'Atlas_Core.Content.Mendix' -> ''
//	                     EntityRef null -> {DomainModels$DirectEntityRef, Entity: 'MyFirstModule.Photo'}
//	defaultImageDynamic  EntityRef null -> {DomainModels$DirectEntityRef, Entity: 'MyFirstModule.Photo'}
//
// MDL spells the binding `Image: $currentObject`.

// imageObjectEngine returns an engine building inside a data view over
// contextEntity, over a domain model in which Sales.Photo extends System.Image,
// Sales.Thumbnail extends Sales.Photo, and Sales.Room extends nothing.
func imageObjectEngine(t *testing.T, contextEntity string) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-sales")}, Name: "Sales"}
	b := &mock.MockBackend{
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-sales")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-photo")}, Name: "Photo", GeneralizationRef: "System.Image"},
					{BaseElement: model.BaseElement{ID: model.ID("e-thumb")}, Name: "Thumbnail", GeneralizationRef: "Sales.Photo"},
					{BaseElement: model.BaseElement{ID: model.ID("e-room")}, Name: "Room"},
				},
			}}, nil
		},
	}
	pb := &pageBuilder{
		backend:          b,
		entityContext:    contextEntity,
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
	e := NewPluggableWidgetEngine(b, pb)
	pb.pluggableEngine = e
	return e
}

func imageObjectDef(t *testing.T) *WidgetDefinition {
	t.Helper()
	reg := LoadWidgetRegistry("")
	if reg == nil || reg.byMDLName["IMAGE"] == nil {
		t.Fatal("built-in IMAGE definition not available")
	}
	return reg.byMDLName["IMAGE"]
}

// builtImageValue returns the stored WidgetValue of one property of a built
// widget, found by its TypePointer as Studio Pro resolves it.
func builtImageValue(t *testing.T, w pages.Widget, propertyKey string) bson.D {
	t.Helper()
	cw, ok := w.(*pages.CustomWidget)
	if !ok {
		t.Fatalf("Build returned %T, want *pages.CustomWidget", w)
	}
	entry, ok := cw.PropertyTypeIDMap[propertyKey]
	if !ok {
		t.Fatalf("template has no property %q", propertyKey)
	}
	want := strings.ReplaceAll(entry.PropertyTypeID, "-", "")
	for _, e := range cw.RawObject {
		if e.Key != "Properties" {
			continue
		}
		for _, item := range e.Value.(bson.A) {
			prop, ok := item.(bson.D)
			if !ok {
				continue
			}
			var id string
			var value bson.D
			for _, f := range prop {
				switch f.Key {
				case "TypePointer":
					if bin, ok := f.Value.(primitive.Binary); ok {
						id = strings.ReplaceAll(types.BlobToUUID(bin.Data), "-", "")
					}
				case "Value":
					value, _ = f.Value.(bson.D)
				}
			}
			if id == want {
				return value
			}
		}
	}
	t.Fatalf("built widget has no value for %q", propertyKey)
	return nil
}

func imageValueField(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// directEntity returns the Entity of a DirectEntityRef, or "" for anything else.
func directEntity(v any) string {
	ref, ok := v.(bson.D)
	if !ok || imageValueField(ref, "$Type") != "DomainModels$DirectEntityRef" {
		return ""
	}
	s, _ := imageValueField(ref, "Entity").(string)
	return s
}

func boundImageWidget(extra map[string]any) *ast.WidgetV3 {
	props := map[string]any{"Image": &ast.DataSourceV3{Type: "parameter", Reference: "$currentObject"}}
	for k, v := range extra {
		props[k] = v
	}
	return &ast.WidgetV3{Name: "Photo_View_Image", Type: "image", Properties: props}
}

// The fix: both image-typed properties carry the data view's entity as a
// DirectEntityRef, and the image object's collection image is empty — the
// shape Studio Pro stores.
func TestImageObject_BindsBothImagePropertiesToTheContextEntity(t *testing.T) {
	e := imageObjectEngine(t, "Sales.Photo")
	widget, err := e.Build(imageObjectDef(t), boundImageWidget(nil))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, key := range []string{"imageObject", "defaultImageDynamic"} {
		v := builtImageValue(t, widget, key)
		if got := directEntity(imageValueField(v, "EntityRef")); got != "Sales.Photo" {
			t.Errorf("%s: EntityRef = %v, want a DirectEntityRef to Sales.Photo", key, imageValueField(v, "EntityRef"))
		}
		if got := imageValueField(v, "Image"); got != "" {
			t.Errorf("%s: Image = %v, want empty", key, got)
		}
		if got := imageValueField(v, "SourceVariable"); got != nil {
			t.Errorf("%s: SourceVariable = %v, want none — Studio Pro stores none for the data view's object", key, got)
		}
	}
	if got := imageValueField(builtImageValue(t, widget, "datasource"), "PrimitiveValue"); got != "image" {
		t.Errorf("datasource = %v, want image", got)
	}
}

// The fallback is a collection image on defaultImageDynamic, and only that: in
// the running app (Image widget 1.5.0) an EntityRef beside it made the widget
// render nothing for an object without contents, and without it the widget
// showed the fallback.
func TestImageObject_DefaultImageLandsOnTheFallbackProperty(t *testing.T) {
	e := imageObjectEngine(t, "Sales.Photo")
	widget, err := e.Build(imageObjectDef(t), boundImageWidget(map[string]any{"DefaultImage": "Atlas_Core.Content.Mendix"}))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	def := builtImageValue(t, widget, "defaultImageDynamic")
	if got := imageValueField(def, "Image"); got != "Atlas_Core.Content.Mendix" {
		t.Errorf("defaultImageDynamic Image = %v, want the fallback", got)
	}
	if got := imageValueField(def, "EntityRef"); got != nil {
		t.Errorf("defaultImageDynamic EntityRef = %v, want none beside a collection image — the runtime then shows no fallback", got)
	}
	if got := directEntity(imageValueField(builtImageValue(t, widget, "imageObject"), "EntityRef")); got != "Sales.Photo" {
		t.Errorf("imageObject EntityRef = %v, want Sales.Photo", got)
	}
	if got := imageValueField(builtImageValue(t, widget, "imageObject"), "Image"); got != "" {
		t.Errorf("imageObject Image = %v — the fallback leaked onto the image itself", got)
	}
}

// A specialisation of a specialisation is still an image object.
func TestImageObject_IndirectSpecialisationIsAccepted(t *testing.T) {
	e := imageObjectEngine(t, "Sales.Thumbnail")
	if _, err := e.Build(imageObjectDef(t), boundImageWidget(nil)); err != nil {
		t.Fatalf("Build: %v", err)
	}
}

// CONTROL: a collection image is written as before, with no EntityRef.
func TestImageObject_CollectionImageWritesNoEntityRef(t *testing.T) {
	e := imageObjectEngine(t, "Sales.Photo")
	w := &ast.WidgetV3{Name: "Logo", Type: "image", Properties: map[string]any{"Image": "Atlas_Core.Content.Mendix"}}
	widget, err := e.Build(imageObjectDef(t), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	v := builtImageValue(t, widget, "imageObject")
	if got := imageValueField(v, "Image"); got != "Atlas_Core.Content.Mendix" {
		t.Errorf("imageObject Image = %v, want the collection image", got)
	}
	for _, key := range []string{"imageObject", "defaultImageDynamic"} {
		if got := imageValueField(builtImageValue(t, widget, key), "EntityRef"); got != nil {
			t.Errorf("%s: EntityRef = %v, want none for a collection image", key, got)
		}
	}
}

// mxbuild answers an entity that is not an image with CE7247 "Widget expects
// entity of type System.Image." (measured, once per bound property); refused
// before anything is written.
func TestImageObject_NonImageEntityIsRefused(t *testing.T) {
	e := imageObjectEngine(t, "Sales.Room")
	_, err := e.Build(imageObjectDef(t), boundImageWidget(nil))
	if err == nil || !strings.Contains(err.Error(), "System.Image") {
		t.Fatalf("err = %v, want a refusal naming System.Image", err)
	}
}

// Outside a data container `$currentObject` names nothing.
func TestImageObject_NoContextIsRefused(t *testing.T) {
	e := imageObjectEngine(t, "")
	_, err := e.Build(imageObjectDef(t), boundImageWidget(nil))
	// The refusal must say there is no object, not blame an entity: with no
	// context there is no entity to judge.
	if err == nil || !strings.Contains(err.Error(), "this widget has none") {
		t.Fatalf("err = %v, want a refusal saying the widget has no enclosing object", err)
	}
}

// A page parameter or an association path is a binding whose stored shape has
// not been measured; refused rather than written as the context object.
func TestImageObject_OtherObjectsAreRefused(t *testing.T) {
	for _, ds := range []*ast.DataSourceV3{
		{Type: "parameter", Reference: "$Photo"},
		{Type: "association", ContextVariable: "currentObject", Reference: "Sales.Room_Photo"},
	} {
		e := imageObjectEngine(t, "Sales.Photo")
		w := &ast.WidgetV3{Name: "img", Type: "image", Properties: map[string]any{"Image": ds}}
		if _, err := e.Build(imageObjectDef(t), w); err == nil {
			t.Errorf("%s: built, want a refusal", formatDataSourceV3(ds))
		}
	}
}
