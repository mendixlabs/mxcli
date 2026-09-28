// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// makeDynamicText builds a stored dynamic text widget the way Studio Pro writes
// it: a RenderMode string alongside the Content client template.
func makeDynamicText(name, renderMode string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$DynamicText"},
		{Key: "Name", Value: name},
		{Key: "NativeTextStyle", Value: "Text"},
		{Key: "RenderMode", Value: renderMode},
	}
}

// `alter page P { set RenderMode = H1 on compTitle }` on a dynamic text failed
// with
//
//	failed to set RenderMode on compTitle: property "RenderMode" not found (widget has no pluggable Object)
//
// while `create page … dynamictext x (…, RenderMode: H1)` and `replace x with
// { dynamictext x (…, RenderMode: H1) }` both accept it. RenderMode is a
// first-class Forms$DynamicText property, so the SET must write it in place.
func TestSetWidgetProperty_DynamicTextRenderMode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"H1", "H1"},
		{"H6", "H6"},
		{"h2", "H2"}, // property values arrive as typed — case-insensitive, like the name
		{"Paragraph", "Paragraph"},
		{"text", "Text"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			rawData := makeRawPage(makeDynamicText("compTitle", "Text"))
			m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
			if err := m.SetWidgetProperty("compTitle", "RenderMode", tc.in); err != nil {
				t.Fatalf("SetWidgetProperty(RenderMode=%s) failed: %v", tc.in, err)
			}
			got := bsonnav.DGetString(findBsonWidget(rawData, "compTitle").widget, "RenderMode")
			if got != tc.want {
				t.Errorf("RenderMode = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("lowercase property name", func(t *testing.T) {
		rawData := makeRawPage(makeDynamicText("compTitle", "Text"))
		m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("compTitle", "rendermode", "H3"); err != nil {
			t.Fatalf("SetWidgetProperty(rendermode) failed: %v", err)
		}
		if got := bsonnav.DGetString(findBsonWidget(rawData, "compTitle").widget, "RenderMode"); got != "H3" {
			t.Errorf("RenderMode = %q, want H3", got)
		}
	})
}

// An invalid value is refused with the accepted list, and the stored value is
// left alone — writing an unknown enum member gives a document Studio Pro
// cannot open.
func TestSetWidgetProperty_DynamicTextRenderModeInvalid(t *testing.T) {
	for _, bad := range []any{"H7", "Div", "", 1, true} {
		rawData := makeRawPage(makeDynamicText("compTitle", "H1"))
		m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
		err := m.SetWidgetProperty("compTitle", "RenderMode", bad)
		if err == nil {
			t.Fatalf("RenderMode = %v must be refused", bad)
		}
		msg := err.Error()
		if strings.Contains(msg, "pluggable Object") {
			t.Errorf("RenderMode = %v: the message describes mxcli internals: %q", bad, msg)
		}
		if !strings.Contains(msg, "Paragraph") || !strings.Contains(msg, "H6") {
			t.Errorf("RenderMode = %v: the message does not list the accepted values: %q", bad, msg)
		}
		if got := bsonnav.DGetString(findBsonWidget(rawData, "compTitle").widget, "RenderMode"); got != "H1" {
			t.Errorf("RenderMode = %v: stored value changed to %q on a refused write", bad, got)
		}
	}
}
