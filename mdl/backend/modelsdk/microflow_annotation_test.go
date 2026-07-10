// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestMicroflowObjectToGen_Annotation guards that a microflow annotation note
// (from @annotation) is serialized. The modelsdk write path previously had no
// case for it, so @annotation was silently dropped on the default engine.
func TestMicroflowObjectToGen_Annotation(t *testing.T) {
	ann := &microflows.Annotation{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID("ann-1")},
			Position:    model.Point{X: 100, Y: 100},
			Size:        model.Size{Width: 200, Height: 50},
		},
		Caption: "Process products",
	}
	g := microflowObjectToGen(ann)
	if g == nil {
		t.Fatal("microflowObjectToGen(Annotation) returned nil — annotation dropped")
	}
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	r := bson.Raw(raw)
	if got := r.Lookup("$Type").StringValue(); got != "Microflows$Annotation" {
		t.Errorf("$Type = %q, want Microflows$Annotation", got)
	}
	if got := r.Lookup("Caption").StringValue(); got != "Process products" {
		t.Errorf("Caption = %q, want Process products", got)
	}
}

// TestAnnotationFlowToGen guards the AnnotationFlow that connects a note to its
// activity, including the version-specific line shape (Line/BezierCurve on 10+).
func TestAnnotationFlowToGen(t *testing.T) {
	af := &microflows.AnnotationFlow{
		BaseElement:   model.BaseElement{ID: model.ID("af-1")},
		OriginID:      model.ID("ann-1"),
		DestinationID: model.ID("act-1"),
	}
	raw, err := (&codec.Encoder{}).Encode(annotationFlowToGen(af, 11))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	r := bson.Raw(raw)
	if got := r.Lookup("$Type").StringValue(); got != "Microflows$AnnotationFlow" {
		t.Errorf("$Type = %q, want Microflows$AnnotationFlow", got)
	}
	if _, err := r.LookupErr("OriginPointer"); err != nil {
		t.Errorf("OriginPointer missing: %v", err)
	}
	if _, err := r.LookupErr("DestinationPointer"); err != nil {
		t.Errorf("DestinationPointer missing: %v", err)
	}
	if _, err := r.LookupErr("Line"); err != nil {
		t.Errorf("Line (BezierCurve) missing on major>=10: %v", err)
	}
}
