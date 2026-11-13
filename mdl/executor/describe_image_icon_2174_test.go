// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2174: an `ImageType: icon` image was described without its icon,
// so re-creating it from the description failed the build with "No icon
// selected." The icon is the imageIcon property's value, stored as an action
// button's icon is, and printed with the same `Icon:` spelling.
func TestDescribeImageIcon(t *testing.T) {
	cw := map[string]any{
		"Type": map[string]any{"ObjectType": map[string]any{"PropertyTypes": []any{int32(2),
			map[string]any{"$ID": "pt-icon", "PropertyKey": "imageIcon"},
		}}},
		"Object": map[string]any{"Properties": []any{int32(2),
			map[string]any{"TypePointer": "pt-icon", "Value": map[string]any{
				"Icon": map[string]any{"$Type": "Forms$GlyphIcon", "Code": int32(57377)},
			}},
		}},
	}
	name, typ, code := extractCustomWidgetPropertyIcon(cw, "imageIcon")
	if typ != "Forms$GlyphIcon" || code != 57377 || name != "" {
		t.Fatalf("read (%q, %q, %d), want a glyph 57377", name, typ, code)
	}

	w := rawWidget{ImageType: "icon", IconType: typ, IconCode: code}
	got := strings.Join(describeImageWidgetProps(nil, w), ", ")
	if !strings.Contains(got, "Icon: glyph 57377") {
		t.Errorf("describe printed %q, want an Icon: glyph 57377", got)
	}
	// Control: an image of another type prints no icon.
	w.ImageType = "imageUrl"
	if got := strings.Join(describeImageWidgetProps(nil, w), ", "); strings.Contains(got, "Icon:") {
		t.Errorf("a non-icon image printed an icon: %q", got)
	}
}
