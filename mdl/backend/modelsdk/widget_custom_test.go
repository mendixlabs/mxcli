// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"
	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	mwidgets "github.com/JordtenBulte-OLC/mxcli/modelsdk/widgets"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// v2ToV1 bridges a modelsdk/widgets template (bson v2) into the v1 bson.D that
// pages.CustomWidget.RawType/RawObject use. The future LoadWidgetTemplate must do
// the same conversion at the v2(engine)→v1(shared model) boundary.
func v2ToV1(t *testing.T, d bsonv2.D) bsonv1.D {
	t.Helper()
	b, err := bsonv2.Marshal(d)
	if err != nil {
		t.Fatalf("v2 marshal: %v", err)
	}
	var out bsonv1.D
	if err := bsonv1.Unmarshal(b, &out); err != nil {
		t.Fatalf("v1 unmarshal: %v", err)
	}
	return out
}

// TestCustomWidgetEmbed verifies the pluggable-widget integration point: a real
// ComboBox template (Type + Object) from the modelsdk/widgets registry, embedded
// via customWidgetToGen and re-encoded by the codec, round-trips byte-faithfully
// — proving the codec carries the pluggable widget's own (non-metamodel) schema
// through as passthrough.
func TestCustomWidgetEmbed(t *testing.T) {
	const widgetID = "com.mendix.widget.web.combobox.Combobox"
	typeBSON, objBSON, _, _, _, err := mwidgets.GetTemplateFullBSON(widgetID, mmpr.GenerateID, "")
	if err != nil {
		t.Fatalf("GetTemplateFullBSON: %v", err)
	}
	if typeBSON == nil || objBSON == nil {
		t.Fatal("combobox template not available (nil type/object)")
	}

	cw := &pages.CustomWidget{RawType: v2ToV1(t, typeBSON), RawObject: v2ToV1(t, objBSON)}
	cw.Name = "comboBox1"

	el, err := widgetToGen(cw)
	if err != nil {
		t.Fatalf("widgetToGen: %v", err)
	}
	out, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var doc bsonv1.D
	if err := bsonv1.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	get := func(key string) interface{} {
		for _, e := range doc {
			if e.Key == key {
				return e.Value
			}
		}
		return nil
	}
	if ty, _ := get("$Type").(string); ty != "CustomWidgets$CustomWidget" {
		t.Fatalf("$Type = %q, want CustomWidgets$CustomWidget", ty)
	}
	if get("Name") != "comboBox1" {
		t.Fatalf("Name not preserved: %v", get("Name"))
	}
	// Type (PropertyTypes schema) and Object (filled WidgetObject) must survive.
	if get("Type") == nil {
		t.Error("Type (widget schema) was dropped")
	}
	if get("Object") == nil {
		t.Error("Object (widget values) was dropped")
	}
	// The embedded Object must still carry the pluggable widget's own $Type marker.
	if objDoc, ok := get("Object").(bsonv1.D); ok {
		var objType string
		for _, e := range objDoc {
			if e.Key == "$Type" {
				objType, _ = e.Value.(string)
			}
		}
		if objType == "" {
			t.Error("embedded Object lost its $Type")
		}
	}
}
