// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// TestCreateImageCollection_RoundTrip creates a collection with one embedded
// image and confirms it (and the binary data) round-trips through
// ListImageCollections.
func TestCreateImageCollection_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	ic := &types.ImageCollection{
		ContainerID: mod.ID,
		Name:        "ZzLogos",
		Images:      []types.Image{{Name: "cli", Format: "png", Data: []byte{0x89, 0x50, 0x4e, 0x47, 1, 2, 3}}},
	}
	ic.ID = model.ID("")
	if err := b.CreateImageCollection(ic); err != nil {
		t.Fatalf("CreateImageCollection: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	all, err := b2.ListImageCollections()
	if err != nil {
		t.Fatalf("ListImageCollections: %v", err)
	}
	var got *types.ImageCollection
	for _, c := range all {
		if c.Name == "ZzLogos" {
			got = c
		}
	}
	if got == nil {
		t.Fatalf("ZzLogos not found after create")
	}
	if len(got.Images) != 1 || got.Images[0].Name != "cli" || got.Images[0].Format != "png" {
		t.Fatalf("image metadata not round-tripped: %+v", got.Images)
	}
	if len(got.Images[0].Data) != 7 || got.Images[0].Data[0] != 0x89 {
		t.Errorf("binary image data not round-tripped: %v", got.Images[0].Data)
	}
}
