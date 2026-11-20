// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2174 — an Image widget whose `ImageType` is `icon` was described as
//
//	image image8 (ImageType: icon, …)
//
// with no icon, so re-creating it from that output wrote an image showing
// nothing and mxbuild failed with "No icon selected." once per image. A second
// describe printed the same thing, so the loss was invisible to a diff.
//
// The icon lives on the widget's `imageIcon` property, as the WidgetValue's
// `Icon` element — the same three elements a button's `Icon` holds, measured on
// ConversationalUI.Snippet_Message_Content (Studio Pro 11.16):
//
//	image8:  Icon = Forms$ImageIcon{Image: "ConversationalUI.Image_collection.Traces_icons"}
//	image1:  Icon = Forms$IconCollectionIcon{Image: "Atlas_Core.Atlas_Filled.user"}
//
// MDL already spells all three for buttons (`Icon: 'M.C.I'`, `Icon: image
// 'M.C.I'`, `Icon: glyph N`); an image takes the same spelling.

// imageIconWidget is a stored Image CustomWidget whose `imageIcon` property
// holds icon (nil for an unset one).
func imageIconWidget(icon map[string]any) map[string]any {
	w := imagePropWidget("imageIcon", "")
	prop := w["Object"].(map[string]any)["Properties"].([]any)[0].(map[string]any)
	prop["Value"].(map[string]any)["Icon"] = icon
	return w
}

func TestDescribeImageWidget_EmitsTheIcon_2174(t *testing.T) {
	cases := []struct {
		name string
		icon map[string]any
		want string
	}{
		{"image icon", map[string]any{"$Type": "Forms$ImageIcon", "Image": "ConversationalUI.Image_collection.Traces_icons"},
			"Icon: image 'ConversationalUI.Image_collection.Traces_icons'"},
		{"collection icon", map[string]any{"$Type": "Forms$IconCollectionIcon", "Image": "Atlas_Core.Atlas_Filled.user"},
			"Icon: 'Atlas_Core.Atlas_Filled.user'"},
		{"glyph icon", map[string]any{"$Type": "Forms$GlyphIcon", "Code": int32(57377)},
			"Icon: glyph 57377"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, _ := newMockCtx(t)
			w := rawWidget{Name: "image8"}
			extractImageProperties(ctx, imageIconWidget(c.icon), &w)
			w.ImageType = "icon"
			got := strings.Join(describeImageWidgetProps(ctx, w), ", ")
			if !strings.Contains(got, c.want) {
				t.Errorf("describe = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// CONTROL: an image with no icon stored describes with no Icon clause.
func TestDescribeImageWidget_NoIconNoClause_2174(t *testing.T) {
	ctx, _ := newMockCtx(t)
	w := rawWidget{Name: "img"}
	extractImageProperties(ctx, imageIconWidget(nil), &w)
	if got := strings.Join(describeImageWidgetProps(ctx, w), ", "); strings.Contains(got, "Icon") {
		t.Errorf("describe emitted an icon for a widget with none: %q", got)
	}
}

// iconImage is `image image8 (ImageType: icon, Icon: <icon>)` as the parser
// hands it over.
func iconImage(icon *ast.WidgetIcon) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Name: "image8",
		Type: "image",
		Properties: map[string]any{
			"ImageType": "icon",
			"Icon":      icon,
		},
	}
}

// The write half: the icon lands on `imageIcon` as the element its kind names.
func TestImageWidget_IconIsWritten_2174(t *testing.T) {
	e := imageObjectEngine(t, "")
	widget, err := e.Build(realImageDef(t), iconImage(&ast.WidgetIcon{
		Kind: types.MenuIconImage, Name: "ConversationalUI.Image_collection.Traces_icons"}))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := renderedWidgetStrings(t, widget)
	if !strings.Contains(got, "Forms$ImageIcon") ||
		!strings.Contains(got, "ConversationalUI.Image_collection.Traces_icons") {
		t.Errorf("the image icon was not written:\n%s", got)
	}
}

// describe -> exec -> describe: every icon kind written by the engine reads back
// as the clause it was written from, on the property the real template declares.
func TestImageWidget_IconRoundTrips_2174(t *testing.T) {
	cases := []struct {
		icon *ast.WidgetIcon
		want string
	}{
		{&ast.WidgetIcon{Kind: types.MenuIconImage, Name: "ConversationalUI.Image_Traces.book_sparkle"},
			"Icon: image 'ConversationalUI.Image_Traces.book_sparkle'"},
		{&ast.WidgetIcon{Kind: types.MenuIconCollection, Name: "Atlas_Core.Atlas_Filled.user"},
			"Icon: 'Atlas_Core.Atlas_Filled.user'"},
		{&ast.WidgetIcon{Kind: types.MenuIconGlyph, Code: 57377},
			"Icon: glyph 57377"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			e := imageObjectEngine(t, "")
			widget, err := e.Build(realImageDef(t), iconImage(c.icon))
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			stored := storedCustomWidget(t, widget)
			ctx, _ := newMockCtx(t)
			var w rawWidget
			extractImageProperties(ctx, stored, &w)
			got := strings.Join(describeImageWidgetProps(ctx, w), ", ")
			if !strings.Contains(got, "ImageType: icon") || !strings.Contains(got, c.want) {
				t.Errorf("round trip lost the icon: describe = %q, want %q", got, c.want)
			}
		})
	}
}

// storedCustomWidget turns a built CustomWidget into the map shape DESCRIBE
// reads, via the same BSON decode GetRawUnit uses, so the TypePointer IDs
// resolve as they do on disk.
func storedCustomWidget(t *testing.T, widget pages.Widget) map[string]any {
	t.Helper()
	cw, ok := widget.(*pages.CustomWidget)
	if !ok {
		t.Fatalf("Build returned %T, want *pages.CustomWidget", widget)
	}
	raw, err := bson.Marshal(bson.D{{Key: "Type", Value: cw.RawType}, {Key: "Object", Value: cw.RawObject}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := mpr.RawDocumentMap(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}
