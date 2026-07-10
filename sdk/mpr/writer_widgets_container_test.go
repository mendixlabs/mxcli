// SPDX-License-Identifier: Apache-2.0

// Issue #603: a Container (Forms$DivContainer) is clickable via its
// OnClickAction. serializeContainer must wire the configured action through
// instead of always emitting Forms$NoAction.

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
)

// bsonLookup returns the value of key in doc, or nil if absent.
func bsonLookup(doc bson.D, key string) any {
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// bsonSubDoc returns doc[key] as a bson.D, failing the test if it is missing or
// not a sub-document.
func bsonSubDoc(t *testing.T, doc bson.D, key string) bson.D {
	t.Helper()
	v := bsonLookup(doc, key)
	sub, ok := v.(bson.D)
	if !ok {
		t.Fatalf("field %q: want bson.D, got %T", key, v)
	}
	return sub
}

// TestSerializeContainer_DynamicClasses locks in the DynamicClasses serialization fix: a widget's
// DynamicClasses expression is serialized into its Forms$Appearance
// (previously the field was hardcoded to "").
func TestSerializeContainer_DynamicClasses(t *testing.T) {
	c := &pages.Container{}
	c.Name = "box"
	c.Class = "ss-box"
	c.DynamicClasses = "if $currentObject/Name = '' then 'ss-box--empty' else ''"

	doc := serializeContainer(c)

	appearance := bsonSubDoc(t, doc, "Appearance")
	if got := bsonLookup(appearance, "DynamicClasses"); got != c.DynamicClasses {
		t.Errorf("Appearance.DynamicClasses = %v, want %q", got, c.DynamicClasses)
	}
	if got := bsonLookup(appearance, "Class"); got != "ss-box" {
		t.Errorf("Appearance.Class = %v, want %q", got, "ss-box")
	}
}

func TestSerializeContainer_OnClickActionDefaultsToNoAction(t *testing.T) {
	c := &pages.Container{}
	c.Name = "box"

	doc := serializeContainer(c)

	action := bsonSubDoc(t, doc, "OnClickAction")
	if got := bsonLookup(action, "$Type"); got != "Forms$NoAction" {
		t.Errorf("default OnClickAction $Type = %v, want Forms$NoAction", got)
	}
}

func TestSerializeContainer_OnClickActionMicroflow(t *testing.T) {
	c := &pages.Container{
		OnClickAction: &pages.MicroflowClientAction{
			MicroflowName: "MyFirstModule.MyFirstLogic",
		},
	}
	c.Name = "box"

	doc := serializeContainer(c)

	action := bsonSubDoc(t, doc, "OnClickAction")
	if got := bsonLookup(action, "$Type"); got != "Forms$MicroflowAction" {
		t.Fatalf("OnClickAction $Type = %v, want Forms$MicroflowAction", got)
	}
	settings := bsonSubDoc(t, action, "MicroflowSettings")
	if got := bsonLookup(settings, "Microflow"); got != "MyFirstModule.MyFirstLogic" {
		t.Errorf("Microflow = %v, want MyFirstModule.MyFirstLogic", got)
	}
}
