// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// mendixlabs/mxcli#151: `alter page … set (MaxFileSize: 20) on fileManager1`
// was refused with `property "MaxFileSize" is not a property of this built-in
// widget` — a file manager's own four properties fell through to the pluggable
// setter. The stored shape is Studio Pro's (TestApp's
// WorkflowCommons.WorkflowAttachment_New).
func fileManagerDoc(maxFileSize any) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$FileManager"},
		{Key: "AllowedExtensions", Value: ""},
		{Key: "Appearance", Value: bson.D{{Key: "$Type", Value: "Forms$Appearance"}}},
		{Key: "Editable", Value: "Always"},
		{Key: "MaxFileSize", Value: maxFileSize},
		{Key: "Name", Value: "fileManager1"},
		{Key: "ShowFileInBrowser", Value: false},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Type", Value: "Upload"},
	}
}

func TestSetFileManagerProperties(t *testing.T) {
	// int64 is the width Studio Pro stores MaxFileSize at (26 of 26 across nine
	// Studio Pro-authored projects, ako/mxcli#1264); the int32 case below is a
	// document an older mxcli wrote.
	raw := makeRawPage(fileManagerDoc(int64(5)))
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	for prop, value := range map[string]any{
		"MaxFileSize":       20,
		"allowedextensions": ".xlsx;.xls", // MDL property names are case-insensitive
		"Type":              "both",       // and enum values too; stored canonical
		"ShowFileInBrowser": true,
	} {
		if err := m.SetWidgetProperty("fileManager1", prop, value); err != nil {
			t.Fatalf("set %s: %v", prop, err)
		}
	}
	w := findBsonWidget(raw, "fileManager1").widget
	if got := bsonnav.DGet(w, "MaxFileSize"); got != int64(20) {
		t.Errorf("MaxFileSize = %#v, want int64(20) — the stored width kept", got)
	}
	if got := bsonnav.DGetString(w, "AllowedExtensions"); got != ".xlsx;.xls" {
		t.Errorf("AllowedExtensions = %q", got)
	}
	if got := bsonnav.DGetString(w, "Type"); got != "Both" {
		t.Errorf("Type = %q, want Both", got)
	}
	if got := bsonnav.DGet(w, "ShowFileInBrowser"); got != true {
		t.Errorf("ShowFileInBrowser = %#v, want true", got)
	}
}

// The stored width is kept: replacing one width with the other is a change of
// its own, as for a FormattingInfo's DecimalPrecision. int64 is Studio Pro's;
// int32 is what mxcli's codec wrote before ako/mxcli#1264.
func TestSetFileManagerMaxFileSize_KeepsStoredWidth(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stored any
		want   any
	}{
		{"int64 (Studio Pro)", int64(5), int64(7)},
		{"int32 (older mxcli)", int32(5), int32(7)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := makeRawPage(fileManagerDoc(tc.stored))
			m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
			if err := m.SetWidgetProperty("fileManager1", "MaxFileSize", 7); err != nil {
				t.Fatalf("set: %v", err)
			}
			if got := bsonnav.DGet(findBsonWidget(raw, "fileManager1").widget, "MaxFileSize"); got != tc.want {
				t.Errorf("MaxFileSize = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestSetFileManager_RefusesBadValues(t *testing.T) {
	for _, tc := range []struct {
		prop  string
		value any
		want  string
	}{
		{"Type", "Uploads", "Upload, Download, Both"},
		{"MaxFileSize", -1, "MaxFileSize"},
		{"MaxFileSize", "big", "MaxFileSize"},
		{"ShowFileInBrowser", "maybe", "ShowFileInBrowser"},
		{"AllowedExtensions", 3, "AllowedExtensions"},
	} {
		m := &Mutator{rawData: makeRawPage(fileManagerDoc(int32(5))), widgetFinder: findBsonWidget}
		err := m.SetWidgetProperty("fileManager1", tc.prop, tc.value)
		if err == nil {
			t.Errorf("set %s = %v accepted", tc.prop, tc.value)
		} else if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("set %s = %v: error %q should mention %q", tc.prop, tc.value, err, tc.want)
		}
	}
}

// Control: on any other widget the keys keep their old route (the pluggable
// setter), so `Type` on a pluggable widget still means that widget's own
// property and a native widget still refuses it.
func TestSetFileManagerKeys_OtherWidgetUnchanged(t *testing.T) {
	cont := bson.D{
		{Key: "$Type", Value: "Forms$DivContainer"},
		{Key: "Name", Value: "box"},
		{Key: "Appearance", Value: bson.D{{Key: "$Type", Value: "Forms$Appearance"}}},
	}
	m := &Mutator{rawData: makeRawPage(cont), widgetFinder: findBsonWidget}
	err := m.SetWidgetProperty("box", "MaxFileSize", 5)
	if err == nil || !strings.Contains(err.Error(), "not a property of this built-in widget") {
		t.Errorf("MaxFileSize on a container: want the built-in-widget refusal, got %v", err)
	}
}
