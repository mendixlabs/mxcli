// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
)

// A native mobile widget is described under the keyword `widget init` gave it
// (`nativegallery`, `nativeactivityindicator`, …), or — with no definition to
// resolve against — the `pluggablewidget '<id>'` form. Never under the last
// segment of its id.
//
// Measured on PedApp (11.13.0) with Native Mobile Resources 12.6.0 installed:
// a native widget with no setting away from its default took the bare branch
// of describe, which printed the id's last segment. Three of those segments
// are WEB keywords — `gallery`, `badge`, `image` — so re-running the
// description rebuilt the page with the web widget in place of the native one,
// and `check` passed it. Seven more (`activityindicator`, `qrcode`, …) named no
// widget at all and were refused (MDL-WIDGET25). `gallery` and `image` were
// worse still: they are matched on that segment to the WEB widget's dedicated
// branch, so a native gallery with a data source was described as the web one
// too.
func TestDescribeNativeWidget_UsesItsOwnKeyword(t *testing.T) {
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("no registry — the embedded definitions must load, or this test proves nothing")
	}
	natives := map[string]string{
		"com.mendix.widget.native.gallery.Gallery":                     "nativegallery",
		"com.mendix.widget.native.badge.Badge":                         "nativebadge",
		"com.mendix.widget.native.image.Image":                         "nativeimage",
		"com.mendix.widget.native.activityindicator.ActivityIndicator": "nativeactivityindicator",
		"com.mendix.widget.native.qrcode.QRCode":                       "nativeqrcode",
		"com.mendix.widget.native.repeater.Repeater":                   "nativerepeater",
	}
	for id, kw := range natives {
		def := &WidgetDefinition{WidgetID: id, MDLName: strings.ToUpper(kw), WidgetKind: "pluggable"}
		registry.byWidgetID[id] = def
		registry.byMDLName[strings.ToUpper(kw)] = def
	}

	for id, kw := range natives {
		t.Run(kw, func(t *testing.T) {
			got := describeBareCustomWidget(t, registry, id, "w1")
			if want := kw + " w1"; !strings.HasPrefix(got, want) {
				t.Errorf("described as %q, want %q — the keyword that builds %s", got, want, id)
			}
		})
	}
}

// Without a project the registry holds only the embedded (web) definitions, so
// a native widget has no keyword to resolve to: the id form is the one that
// builds it again. The last segment would name the web widget or nothing.
func TestDescribeNativeWidget_NoDefinitionUsesTheIDForm(t *testing.T) {
	registry := LoadWidgetRegistry("")
	for _, id := range []string{
		"com.mendix.widget.native.gallery.Gallery",
		"com.mendix.widget.native.badge.Badge",
		"com.mendix.widget.native.signature.Signature",
	} {
		got := describeBareCustomWidget(t, registry, id, "w1")
		if want := "pluggablewidget '" + id + "' w1"; !strings.HasPrefix(got, want) {
			t.Errorf("described as %q, want %q", got, want)
		}
	}
}

// The control: the web widgets keep their keyword — gallery and image through
// their dedicated branches, the badge through the definition `widget init`
// writes for it (without one, it takes the id form, as it does when it has
// settings).
func TestDescribeWebWidget_KeepsItsKeyword(t *testing.T) {
	registry := LoadWidgetRegistry("")
	badge := &WidgetDefinition{WidgetID: "com.mendix.widget.custom.badge.Badge", MDLName: "BADGE", WidgetKind: "pluggable"}
	registry.byWidgetID[badge.WidgetID] = badge
	registry.byMDLName["BADGE"] = badge
	for id, kw := range map[string]string{
		"com.mendix.widget.web.gallery.Gallery": "gallery",
		"com.mendix.widget.web.image.Image":     "image",
		"com.mendix.widget.custom.badge.Badge":  "badge",
	} {
		got := describeBareCustomWidget(t, registry, id, "w1")
		if want := kw + " w1"; !strings.HasPrefix(got, want) {
			t.Errorf("%s described as %q, want %q", id, got, want)
		}
	}
}

// extractCustomWidgetType picks the dedicated describe branch. Only the web
// widget a branch was written for may land in it.
func TestExtractCustomWidgetType_NativeIDIsNotTheWebWidget(t *testing.T) {
	for _, id := range []string{
		"com.mendix.widget.native.gallery.Gallery",
		"com.mendix.widget.native.image.Image",
		"com.acme.widgets.combobox.Combobox",
	} {
		got := extractCustomWidgetType(nil, map[string]any{"Type": map[string]any{"WidgetId": id}})
		if isKnownCustomWidgetType(got) {
			t.Errorf("%s routed to the dedicated %q branch, which describes a different widget", id, got)
		}
	}
	// Control: the web gallery is still routed to its branch.
	if got := extractCustomWidgetType(nil, map[string]any{"Type": map[string]any{
		"WidgetId": "com.mendix.widget.web.gallery.Gallery"}}); got != "gallery" {
		t.Errorf("web gallery routed to %q, want gallery", got)
	}
}

// describeBareCustomWidget describes a stored pluggable widget that holds no
// setting away from its default.
func describeBareCustomWidget(t *testing.T, registry *WidgetRegistry, widgetID, name string) string {
	t.Helper()
	stored := map[string]any{
		"$Type": "CustomWidgets$CustomWidget",
		"Name":  name,
		"Type": map[string]any{
			"WidgetId":   widgetID,
			"ObjectType": map[string]any{"PropertyTypes": []any{int32(3)}},
		},
		"Object": map[string]any{"Properties": []any{int32(3)}},
	}
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(nil)
	ctx.Output = &buf
	ctx.widgetRegistry, ctx.widgetRegistryLoaded = registry, true
	raws := parseRawWidget(ctx, stored)
	if len(raws) != 1 {
		t.Fatalf("parsed %d widgets, want 1", len(raws))
	}
	outputWidgetMDLV3(ctx, raws[0], 0)
	return strings.TrimSpace(buf.String())
}
