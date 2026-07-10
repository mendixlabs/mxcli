// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"log"

	bsonv1 "go.mongodb.org/mongo-driver/bson"
	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/widgetobj"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// LoadWidgetTemplate loads a pluggable widget template from the modelsdk/widgets
// registry and returns the shared widgetobj.Builder, wired with the codec child
// serializer. The registry speaks bson v2; the builder + pages.CustomWidget speak
// v1, so the template is bridged here.
func (b *Backend) LoadWidgetTemplate(widgetID string, projectPath string) (backend.WidgetObjectBuilder, error) {
	typeBSON, objBSON, propIDs, objectTypeID, _, err := mwidgets.GetTemplateFullBSON(widgetID, mmpr.GenerateID, projectPath)
	if err != nil {
		return nil, err
	}
	if typeBSON == nil || objBSON == nil {
		return nil, nil
	}
	return widgetobj.New(widgetID, v2ToV1BSON(typeBSON), v2ToV1BSON(objBSON),
		convertPropTypeIDs(propIDs), objectTypeID, codecChildSerializer{}), nil
}

// SerializeWidgetToOpaque converts a domain widget to its raw BSON form (for
// embedding as a pluggable widget's child content) via the codec converters.
func (b *Backend) SerializeWidgetToOpaque(w pages.Widget) any {
	return codecChildSerializer{}.SerializeWidget(w)
}

// SerializeDataSourceToOpaque converts a domain data source to raw BSON.
func (b *Backend) SerializeDataSourceToOpaque(ds pages.DataSource) any {
	return codecChildSerializer{}.SerializeCustomWidgetDataSource(ds)
}

// BuildCreateAttributeObject builds an attribute object for filter widgets (pure,
// shared with the legacy engine).
func (b *Backend) BuildCreateAttributeObject(attributePath string, objectTypeID, propertyTypeID, valueTypeID string) (any, error) {
	return widgetobj.CreateAttributeObject(attributePath, objectTypeID, propertyTypeID, valueTypeID)
}

// BuildFilterWidget builds a DataGrid2 filter widget (text / number / date /
// dropdown) as a pages.CustomWidget whose Type/Object come from the
// modelsdk/widgets registry. Mirrors the MPR backend's BuildFilterWidget; the rest
// of the CustomWidget envelope (Appearance, editability, …) is added when the
// widget is serialized via customWidgetToGen.
func (b *Backend) BuildFilterWidget(spec backend.FilterWidgetSpec, projectPath string) (pages.Widget, error) {
	typeBSON, objBSON, _, _, _, err := mwidgets.GetTemplateFullBSON(spec.WidgetID, mmpr.GenerateID, projectPath)
	if err != nil {
		return nil, fmt.Errorf("BuildFilterWidget: load template %s: %w", spec.WidgetID, err)
	}
	if typeBSON == nil || objBSON == nil {
		return nil, fmt.Errorf("BuildFilterWidget: no template for widget %s", spec.WidgetID)
	}
	return &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CustomWidgets$CustomWidget",
			},
			Name: spec.FilterName,
		},
		Editable:  "Always",
		RawObject: v2ToV1BSON(objBSON),
		RawType:   v2ToV1BSON(typeBSON),
	}, nil
}

// codecChildSerializer implements widgetobj.ChildSerializer by routing child
// content through the modelsdk codec converters, then bridging v2→v1 BSON.
type codecChildSerializer struct{}

func (codecChildSerializer) SerializeWidget(w pages.Widget) bsonv1.D {
	el, err := widgetToGen(w)
	if err != nil {
		log.Printf("modelsdk: serialize child widget %T: %v", w, err)
		return nil
	}
	return genToV1BSON(el)
}

func (codecChildSerializer) SerializeClientAction(a pages.ClientAction) bsonv1.D {
	el, err := clientActionToGen(a)
	if err != nil {
		log.Printf("modelsdk: serialize client action %T: %v", a, err)
		return nil
	}
	return genToV1BSON(el)
}

func (codecChildSerializer) SerializeCustomWidgetDataSource(ds pages.DataSource) bsonv1.D {
	el, err := customWidgetDataSourceToGen(ds)
	if err != nil {
		log.Printf("modelsdk: serialize custom widget data source %T: %v", ds, err)
		return nil
	}
	if el == nil {
		return nil
	}
	return genToV1BSON(el)
}

// genToV1BSON encodes a gen element with the codec (bson v2 bytes) and decodes it
// back into a v1 bson.D for the shared (v1) builder.
func genToV1BSON(el element.Element) bsonv1.D {
	out, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		log.Printf("modelsdk: encode child element: %v", err)
		return nil
	}
	var d bsonv1.D
	if err := bsonv1.Unmarshal([]byte(out), &d); err != nil {
		log.Printf("modelsdk: v1 decode child element: %v", err)
		return nil
	}
	return d
}

// v2ToV1BSON bridges a modelsdk/widgets template document (bson v2) to v1.
func v2ToV1BSON(d bsonv2.D) bsonv1.D {
	b, err := bsonv2.Marshal(d)
	if err != nil {
		return nil
	}
	var out bsonv1.D
	if err := bsonv1.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// convertPropTypeIDs maps the registry's PropertyTypeIDEntry (types form) to the
// pages form the builder uses. NestedKeyOrder carries the template PropertyTypes
// order for object-list widgets (DataGrid2 columns); without it the builder falls
// back to alphabetical order and Studio Pro raises CE0463.
func convertPropTypeIDs(src map[string]types.PropertyTypeIDEntry) map[string]pages.PropertyTypeIDEntry {
	out := make(map[string]pages.PropertyTypeIDEntry, len(src))
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
			entry.NestedPropertyIDs = convertPropTypeIDs(v.NestedPropertyIDs)
			entry.NestedKeyOrder = append([]string(nil), v.NestedKeyOrder...)
		}
		out[k] = entry
	}
	return out
}
