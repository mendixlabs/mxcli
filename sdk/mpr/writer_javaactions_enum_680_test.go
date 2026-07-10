// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
)

// Issue #680: an EnumerationType parameter/return must serialize as
// CodeActions$EnumerationType with the Enumeration qualified name — never as an
// entity reference.

func TestSerializeInnerType_Enumeration(t *testing.T) {
	d := serializeInnerType(&javaactions.EnumerationType{
		BaseElement: model.BaseElement{ID: "11111111-1111-1111-1111-111111111111"},
		Enumeration: "Barcode.BarcodeFormat",
	})
	m := map[string]any{}
	for _, e := range d {
		m[e.Key] = e.Value
	}
	if m["$Type"] != "CodeActions$EnumerationType" {
		t.Errorf("$Type = %v, want CodeActions$EnumerationType", m["$Type"])
	}
	if m["Enumeration"] != "Barcode.BarcodeFormat" {
		t.Errorf("Enumeration = %v", m["Enumeration"])
	}
}

func TestSerializeReturnType_Enumeration(t *testing.T) {
	d := serializeReturnType(&javaactions.EnumerationType{
		BaseElement: model.BaseElement{ID: "22222222-2222-2222-2222-222222222222"},
		Enumeration: "M.Status",
	})
	m := map[string]any{}
	for _, e := range d {
		m[e.Key] = e.Value
	}
	if m["$Type"] != "CodeActions$EnumerationType" {
		t.Errorf("$Type = %v, want CodeActions$EnumerationType", m["$Type"])
	}
	if m["Enumeration"] != "M.Status" {
		t.Errorf("Enumeration = %v", m["Enumeration"])
	}
}
