// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

// FindCustomWidgetType used to be generated into unimplemented_gen.go. Backend
// now shadows that stub, and the stub is STILL GENERATED (the generator emits a
// complete fallback set), so nothing but a test notices if the override is lost
// — the method would quietly go back to erroring.

import (
	"os"
	"path/filepath"
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"
)

// imageWidgetID is in the shared fixture; combobox and datagrid are too.
const imageWidgetID = "com.mendix.widget.web.image.Image"

func customWidgetFixture(t *testing.T) *Backend {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../../testdata/expr-checker")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	b := New()
	if err := b.ConnectReadOnly(filepath.Join(dst, "minimal.mpr")); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	return b
}

func TestFindCustomWidgetType_ReturnsTheStoredWidget(t *testing.T) {
	w, err := customWidgetFixture(t).FindCustomWidgetType(imageWidgetID)
	if err != nil {
		t.Fatalf("FindCustomWidgetType: %v — if this is the \"not implemented on "+
			"the model engine\" error, Backend's override was lost", err)
	}
	if w == nil {
		t.Fatal("no widget found for an id the fixture contains")
	}
	if w.WidgetID != imageWidgetID {
		t.Errorf("WidgetID = %q, want %q", w.WidgetID, imageWidgetID)
	}

	// The v1/v2 trap this method exists to absorb: modelsdk/mpr builds these with
	// the v2 driver, every caller asserts the v1 bson.D that types.RawCustomWidgetType
	// documents, and the two are unrelated types with the SAME NAME — so the
	// failure reads "widget type is bson.D, want bson.D" and a cast written to
	// silence it panics. Asserting the concrete v1 type is what pins the currency.
	typeDoc, ok := w.RawType.(bsonv1.D)
	if !ok {
		t.Fatalf("RawType is %T, want v1 bson.D — check the conversion, and note "+
			"that a v2 bson.D prints under the same name", w.RawType)
	}
	if len(typeDoc) == 0 {
		t.Error("RawType is empty — a widget's schema cannot be")
	}
	if w.RawObject != nil {
		if _, ok := w.RawObject.(bsonv1.D); !ok {
			t.Errorf("RawObject is %T, want v1 bson.D", w.RawObject)
		}
	}

	// Both halves or neither: sdk/widgets/templates/README.md is explicit that a
	// template needs Type AND Object, and a Type whose Object is missing is the
	// CE0463 shape.
	if w.RawObject == nil {
		t.Error("RawType without RawObject — an extracted template would be incomplete")
	}
}

// The not-found contract: nil, nil rather than an error, which is what callers
// branch on. Without this, an implementation that errored on every miss would
// pass the test above.
func TestFindCustomWidgetType_MissingWidgetIsNotAnError(t *testing.T) {
	w, err := customWidgetFixture(t).FindCustomWidgetType("com.example.no.such.Widget")
	if err != nil {
		t.Errorf("a widget the project does not have returned an error: %v", err)
	}
	if w != nil {
		t.Errorf("got %+v for a widget id that is not in the project, want nil", w)
	}
}
