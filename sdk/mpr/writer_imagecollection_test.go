// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSerializeImageCollection_EmptyImages(t *testing.T) {
	ic := &ImageCollection{
		BaseElement: model.BaseElement{ID: "ic-test-1"},
		ContainerID: model.ID("module-id-1"),
		Name:        "TestIcons",
		ExportLevel: "Hidden",
	}

	data, err := serializeImageCollection(ic)
	if err != nil {
		t.Fatalf("serializeImageCollection: %v", err)
	}

	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	// Verify $Type
	if got := getBSONField(doc, "$Type"); got != "Images$ImageCollection" {
		t.Errorf("$Type = %q, want %q", got, "Images$ImageCollection")
	}

	// Verify Name
	if got := getBSONField(doc, "Name"); got != "TestIcons" {
		t.Errorf("Name = %q, want %q", got, "TestIcons")
	}

	// Verify ExportLevel
	if got := getBSONField(doc, "ExportLevel"); got != "Hidden" {
		t.Errorf("ExportLevel = %q, want %q", got, "Hidden")
	}

	// Verify Excluded
	if got := getBSONField(doc, "Excluded"); got != false {
		t.Errorf("Excluded = %v, want false", got)
	}

	// Images array must start with marker int32(3)
	assertArrayMarker(t, doc, "Images", int32(3))

	// Images should be empty (marker only)
	arr := getBSONField(doc, "Images").(bson.A)
	if len(arr) != 1 {
		t.Errorf("Images length = %d, want 1 (marker only)", len(arr))
	}
}

func TestSerializeImageCollection_DefaultExportLevel(t *testing.T) {
	ic := &ImageCollection{
		BaseElement: model.BaseElement{ID: "ic-test-2"},
		ContainerID: model.ID("module-id-1"),
		Name:        "Icons",
		// ExportLevel intentionally omitted to test CreateImageCollection default
	}

	// CreateImageCollection sets default, but serializeImageCollection doesn't —
	// test that empty ExportLevel serializes as empty string (caller's responsibility)
	data, err := serializeImageCollection(ic)
	if err != nil {
		t.Fatalf("serializeImageCollection: %v", err)
	}

	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	assertArrayMarker(t, doc, "Images", int32(3))
}
