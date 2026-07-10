// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

func encodeWidget(t *testing.T, w pages.Widget) bsonv1.D {
	t.Helper()
	el, err := widgetToGen(w)
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
	return doc
}

func docGet(doc bsonv1.D, key string) any {
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// Issue #627 — the codec used to hardcode ConditionalVisibilitySettings to null
// (silently dropping the expression). A container that carries one must now
// serialize the settings node with the expression.
func TestWidgetConditionalVisibility_Serialized(t *testing.T) {
	ctn := &pages.Container{}
	ctn.Name = "ctn"
	ctn.ConditionalVisibility = &pages.ConditionalVisibilitySettings{
		BaseElement: model.BaseElement{TypeName: "Forms$ConditionalVisibilitySettings"},
		Expression:  "$currentObject/Name != ''",
	}

	doc := encodeWidget(t, ctn)
	cvs, ok := docGet(doc, "ConditionalVisibilitySettings").(bsonv1.D)
	if !ok {
		t.Fatalf("ConditionalVisibilitySettings not serialized (got %T)", docGet(doc, "ConditionalVisibilitySettings"))
	}
	if got := docGet(cvs, "Expression"); got != "$currentObject/Name != ''" {
		t.Errorf("Expression = %v, want $currentObject/Name != ''", got)
	}
	// Studio Pro's sub-field set must be present (via TypeDefaults).
	for _, k := range []string{"Conditions", "ModuleRoles", "Attribute", "SourceVariable", "IgnoreSecurity"} {
		found := false
		for _, e := range cvs {
			if e.Key == k {
				found = true
			}
		}
		if !found {
			t.Errorf("CVS missing sub-field %q", k)
		}
	}
}

// A widget without a conditional setting still emits the null slot (unchanged).
func TestWidgetConditionalVisibility_NullWhenUnset(t *testing.T) {
	ctn := &pages.Container{}
	ctn.Name = "ctn"
	doc := encodeWidget(t, ctn)
	if v := docGet(doc, "ConditionalVisibilitySettings"); v != nil {
		t.Errorf("expected null ConditionalVisibilitySettings when unset, got %T", v)
	}
}
