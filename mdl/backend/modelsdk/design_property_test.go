// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// TestAppearanceDesignProperties verifies the codec emits flat and compound
// design properties (#668). Each is a Forms$DesignPropertyValue wrapper (Key +
// typed Value); compound nests a Forms$CompoundDesignPropertyValue whose
// Properties are themselves wrappers.
func TestAppearanceDesignProperties(t *testing.T) {
	dps := []pages.DesignPropertyValue{
		{Key: "Column gap", ValueType: "option", Option: "Medium"},
		{Key: "Cards style", ValueType: "toggle"},
		{Key: "Spacing", ValueType: "compound", Compound: []pages.DesignPropertyValue{
			{Key: "margin-top", ValueType: "option", Option: "Large"},
		}},
	}

	out, err := (&codec.Encoder{}).Encode(newAppearance("", "", "", dps))
	if err != nil {
		t.Fatalf("encode appearance: %v", err)
	}

	// $Type strings and string values are stored as UTF-8 in the BSON, so a
	// substring check confirms the structure was serialized.
	for _, want := range []string{
		"Forms$DesignPropertyValue", "Column gap",
		"Forms$OptionDesignPropertyValue", "Medium",
		"Cards style", "Forms$ToggleDesignPropertyValue",
		"Spacing", "Forms$CompoundDesignPropertyValue", "margin-top", "Large",
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("encoded appearance missing %q\nBSON: %x", want, out)
		}
	}
}

// TestAppearanceDynamicClasses locks in the DynamicClasses fix: the codec serializes the
// Forms$Appearance.DynamicClasses expression when the widget carries one
// (previously hardcoded to ""), so the runtime class list is not dropped.
func TestAppearanceDynamicClasses(t *testing.T) {
	expr := "if $currentObject/Name = 'Astute' then 'ss-chip--astute' else ''"
	out, err := (&codec.Encoder{}).Encode(newAppearance("ss-chip", "", expr, nil))
	if err != nil {
		t.Fatalf("encode appearance: %v", err)
	}
	if !bytes.Contains(out, []byte(expr)) {
		t.Errorf("encoded appearance missing DynamicClasses expression %q\nBSON: %x", expr, out)
	}
}
