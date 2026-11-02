// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A classic file manager (mendixlabs/mxcli#151) had no case in widgetToGen, so
// nothing could write one. The property set is Forms$FileManager's on Mendix
// 10/11 per the Model SDK (mendixmodelsdk 4.116.0) and the version table: the
// InputWidget's LabelTemplate / ScreenReaderLabel, no attribute — the widget
// acts on the enclosing data view's System.FileDocument — and its own four.
func TestFileManagerToGen_WritesPropertySet(t *testing.T) {
	g, err := widgetToGen(&pages.FileManager{
		BaseWidget:        pages.BaseWidget{Name: "fileManager1"},
		Label:             "Excel file",
		Type:              pages.FileManagerTypeUpload,
		MaxFileSize:       10,
		AllowedExtensions: ".xlsx",
		ShowFileInBrowser: true,
	})
	if err != nil {
		t.Fatalf("widgetToGen: %v", err)
	}
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	doc := bson.Raw(raw)

	if ty := doc.Lookup("$Type").StringValue(); ty != "Forms$FileManager" {
		t.Errorf("$Type = %q, want Forms$FileManager", ty)
	}
	if v := doc.Lookup("Name").StringValue(); v != "fileManager1" {
		t.Errorf("Name = %q", v)
	}
	if v := doc.Lookup("Type").StringValue(); v != "Upload" {
		t.Errorf("Type = %q, want Upload", v)
	}
	// int64 is what Studio Pro stores: 26 of 26 Forms$FileManager.MaxFileSize
	// values across nine Studio Pro-authored projects (ako/mxcli#1264).
	if v, ok := doc.Lookup("MaxFileSize").Int64OK(); !ok || v != 10 {
		t.Errorf("MaxFileSize = %v, want int64 10", doc.Lookup("MaxFileSize"))
	}
	if v := doc.Lookup("AllowedExtensions").StringValue(); v != ".xlsx" {
		t.Errorf("AllowedExtensions = %q", v)
	}
	if v, ok := doc.Lookup("ShowFileInBrowser").BooleanOK(); !ok || !v {
		t.Errorf("ShowFileInBrowser = %v, want true", doc.Lookup("ShowFileInBrowser"))
	}
	if v := doc.Lookup("Editable").StringValue(); v != "Always" {
		t.Errorf("Editable = %q, want Always", v)
	}
	if lt, ok := doc.Lookup("LabelTemplate").DocumentOK(); !ok {
		t.Error("LabelTemplate not written as a document")
	} else if ty := lt.Lookup("$Type").StringValue(); ty != "Forms$ClientTemplate" {
		t.Errorf("LabelTemplate.$Type = %q", ty)
	}
	if _, ok := doc.Lookup("Appearance").DocumentOK(); !ok {
		t.Error("Appearance not written")
	}
	// Written as null slots, as on every other input widget. Absent keys are
	// what a rewrite cannot carry stored values into (rule 4, #1373).
	for _, key := range []string{"ScreenReaderLabel", "ConditionalVisibilitySettings", "ConditionalEditabilitySettings"} {
		v, err := doc.LookupErr(key)
		if err != nil {
			t.Errorf("%s not written: %v", key, err)
		} else if v.Type != bson.TypeNull {
			t.Errorf("%s = %v, want null", key, v)
		}
	}
	// Properties deleted before Mendix 8/9 must not appear.
	for _, key := range []string{"Label", "Class", "Style", "AccessibilitySettings", "AttributeRef"} {
		if _, err := doc.LookupErr(key); err == nil {
			t.Errorf("%s written, but Forms$FileManager has no such property on Mendix 10/11", key)
		}
	}
}

// Control: without a label the template slot is null, as on a check box.
func TestFileManagerToGen_NoLabelIsNull(t *testing.T) {
	g, err := widgetToGen(&pages.FileManager{BaseWidget: pages.BaseWidget{Name: "f"}, Type: pages.FileManagerTypeBoth, MaxFileSize: 5})
	if err != nil {
		t.Fatalf("widgetToGen: %v", err)
	}
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if v, err := bson.Raw(raw).LookupErr("LabelTemplate"); err != nil || v.Type != bson.TypeNull {
		t.Errorf("LabelTemplate = %v (%v), want null", v, err)
	}
}
