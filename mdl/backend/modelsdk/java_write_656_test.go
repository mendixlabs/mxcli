// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestPatchMicroflowActionInfo_HealthyShape asserts the modelsdk engine replaces
// the encoded unit's null MicroflowActionInfo slot with the current metamodel
// shape (CodeActions$ type + four non-null binary bitmaps), so the default engine
// no longer emits the crash-triggering legacy shape (issue #656).
func TestPatchMicroflowActionInfo_HealthyShape(t *testing.T) {
	// Mimic the encoder output: a JavaAction doc with a null MicroflowActionInfo.
	contents, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "JavaActions$JavaAction"},
		{Key: "MicroflowActionInfo", Value: nil},
		{Key: "Name", Value: "JA_Test"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ja := &javaactions.JavaAction{
		MicroflowActionInfo: &javaactions.MicroflowActionInfo{
			BaseElement: model.BaseElement{ID: "22222222-2222-2222-2222-222222222222"},
			Caption:     "Repro",
			Category:    "String Utils",
		},
	}

	patched, err := patchMicroflowActionInfo(contents, ja)
	if err != nil {
		t.Fatal(err)
	}

	var doc bson.D
	if err := bson.Unmarshal(patched, &doc); err != nil {
		t.Fatal(err)
	}
	var mai bson.D
	for _, e := range doc {
		if e.Key == "MicroflowActionInfo" {
			m, ok := e.Value.(bson.D)
			if !ok {
				t.Fatalf("MicroflowActionInfo not a sub-document: %T (%v)", e.Value, e.Value)
			}
			mai = m
		}
	}
	if mai == nil {
		t.Fatal("MicroflowActionInfo not present after patch")
	}

	get := func(key string) (any, bool) {
		for _, e := range mai {
			if e.Key == key {
				return e.Value, true
			}
		}
		return nil, false
	}

	if v, _ := get("$Type"); v != "CodeActions$MicroflowActionInfo" {
		t.Errorf("$Type = %v, want CodeActions$MicroflowActionInfo", v)
	}
	if _, ok := get("Icon"); ok {
		t.Error("obsolete Icon key must not be emitted")
	}
	for _, key := range []string{"IconData", "IconDataDark", "ImageData", "ImageDataDark"} {
		v, ok := get(key)
		if !ok {
			t.Errorf("%s missing", key)
			continue
		}
		bin, isBin := v.(bson.Binary)
		if !isBin || bin.Data == nil {
			t.Errorf("%s = %v (%T), want non-null binary", key, v, v)
		}
	}
}

// TestPatchMicroflowActionInfo_NoInfoIsNoOp asserts a non-exposed action is left
// untouched (MicroflowActionInfo stays null).
func TestPatchMicroflowActionInfo_NoInfoIsNoOp(t *testing.T) {
	contents, _ := bson.Marshal(bson.D{{Key: "Name", Value: "JA_Plain"}})
	out, err := patchMicroflowActionInfo(contents, &javaactions.JavaAction{})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(contents) {
		t.Error("expected contents unchanged when no MicroflowActionInfo")
	}
}
