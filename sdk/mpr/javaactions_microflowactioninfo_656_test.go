// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func maiField(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// TestMicroflowActionInfoBSON_HealthyShape asserts the writer emits the current
// metamodel shape — CodeActions$ type, all four icon/image bitmaps present as
// non-null binaries, and no obsolete Icon key — even when every bitmap is empty.
// A null or absent ImageData crashes Studio Pro's UnitWriter (issue #656).
func TestMicroflowActionInfoBSON_HealthyShape(t *testing.T) {
	d := microflowActionInfoBSON(&javaactions.MicroflowActionInfo{
		BaseElement: model.BaseElement{ID: "11111111-1111-1111-1111-111111111111"},
		Caption:     "My Action",
		Category:    "My Category",
	})

	if v, _ := maiField(d, "$Type"); v != "CodeActions$MicroflowActionInfo" {
		t.Errorf("$Type = %v, want CodeActions$MicroflowActionInfo", v)
	}
	if _, ok := maiField(d, "Icon"); ok {
		t.Error("obsolete Icon key must not be emitted")
	}
	for _, key := range []string{"IconData", "IconDataDark", "ImageData", "ImageDataDark"} {
		v, ok := maiField(d, key)
		if !ok {
			t.Errorf("%s missing — must always be present", key)
			continue
		}
		bin, isBin := v.(primitive.Binary)
		if !isBin {
			t.Errorf("%s = %T, want primitive.Binary (never null/string)", key, v)
			continue
		}
		if bin.Data == nil {
			t.Errorf("%s.Data is nil, want empty (non-null) binary", key)
		}
	}
}

// TestParseMicroflowActionInfo_ToleratesLegacyShape asserts the parser reads the
// broken legacy shape (JavaActions$ type, Icon string, null ImageData) without
// error, and that re-serializing the result yields the healthy shape — i.e. a
// corrupted unit loads and self-repairs on rewrite (issue #656).
func TestParseMicroflowActionInfo_ToleratesLegacyShape(t *testing.T) {
	legacy := map[string]any{
		"$ID":       primitive.Binary{Subtype: 0, Data: make([]byte, 16)},
		"$Type":     "JavaActions$MicroflowActionInfo",
		"Caption":   "Old Action",
		"Category":  "Old Category",
		"Icon":      "",  // obsolete string key
		"ImageData": nil, // the crash-triggering null
	}

	mai := parseMicroflowActionInfo(legacy)
	if mai.Caption != "Old Action" || mai.Category != "Old Category" {
		t.Fatalf("parsed Caption/Category wrong: %+v", mai)
	}
	if mai.ImageData != nil {
		t.Errorf("null ImageData should parse to nil, got %v", mai.ImageData)
	}

	// Rewriting must produce the healthy CodeActions$ shape with non-null binaries.
	d := microflowActionInfoBSON(mai)
	if v, _ := maiField(d, "$Type"); v != "CodeActions$MicroflowActionInfo" {
		t.Errorf("rewrite $Type = %v, want CodeActions$MicroflowActionInfo", v)
	}
	if v, ok := maiField(d, "ImageData"); !ok {
		t.Error("rewrite missing ImageData")
	} else if bin, isBin := v.(primitive.Binary); !isBin || bin.Data == nil {
		t.Errorf("rewrite ImageData = %v, want non-null binary", v)
	}
}

// TestParseMicroflowActionInfo_RoundTripsBinaries asserts real icon/image
// bitmaps survive a parse→write round-trip (no longer silently stripped).
func TestParseMicroflowActionInfo_RoundTripsBinaries(t *testing.T) {
	icon := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	raw := map[string]any{
		"$ID":      primitive.Binary{Subtype: 0, Data: make([]byte, 16)},
		"$Type":    "CodeActions$MicroflowActionInfo",
		"Caption":  "Has Icon",
		"Category": "Cat",
		"IconData": primitive.Binary{Subtype: 0, Data: icon},
	}
	mai := parseMicroflowActionInfo(raw)
	if string(mai.IconData) != string(icon) {
		t.Fatalf("IconData not preserved: got %v", mai.IconData)
	}
	d := microflowActionInfoBSON(mai)
	v, _ := maiField(d, "IconData")
	bin, _ := v.(primitive.Binary)
	if string(bin.Data) != string(icon) {
		t.Errorf("IconData round-trip lost data: got %v", bin.Data)
	}
}
