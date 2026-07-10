// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// TestSerializeActionButton_RenderType verifies that a link-rendered action
// button (authored as `linkbutton`) serializes with RenderType "Link", while a
// normal action button keeps "Button". Previously RenderType was hardcoded to
// "Button", so linkbutton output was indistinguishable from actionbutton.
func TestSerializeActionButton_RenderType(t *testing.T) {
	cases := []struct {
		name   string
		render pages.ButtonRenderMode
		want   string
	}{
		{"linkbutton", pages.ButtonRenderModeLink, "Link"},
		{"actionbutton", pages.ButtonRenderModeButton, "Button"},
		{"default empty", "", "Button"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ab := &pages.ActionButton{
				BaseWidget: pages.BaseWidget{
					BaseElement: model.BaseElement{ID: "11111111-1111-1111-1111-111111111111"},
					Name:        "btn",
				},
				RenderMode: tc.render,
			}
			doc := serializeActionButton(ab)
			got := ""
			for _, e := range doc {
				if e.Key == "RenderType" {
					got, _ = e.Value.(string)
				}
			}
			if got != tc.want {
				t.Errorf("RenderType = %q, want %q", got, tc.want)
			}
			if doc[0].Key != "$ID" {
				t.Errorf("first key = %q, want $ID", doc[0].Key)
			}
		})
	}
}
