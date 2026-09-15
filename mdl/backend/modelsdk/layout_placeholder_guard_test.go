// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The write-time guard used to ask only "is there A placeholder?", under any
// name — a faithful implementation of mxcli's own docs, which called the name
// `Main` a convention. mxbuild validates it as a rule (mendixlabs/mxcli#1063),
// so the guard let through exactly the document that fails CE0848.
//
// This asserts on layoutToGen, the single choke point every layout CREATE goes
// through, so the refusal cannot be bypassed by a caller that skips `mxcli check`.
func TestLayoutToGen_PlaceholderRule(t *testing.T) {
	cases := []struct {
		name    string
		phs     []string
		wantErr string
	}{
		{name: "exactly one Main builds", phs: []string{"Main"}},
		{name: "extra names alongside Main are fine", phs: []string{"Main", "Side"}},
		{name: "no Main", phs: []string{"Content"}, wantErr: "CE0848"},
		{name: "no placeholder at all", phs: nil, wantErr: "CE0848"},
		{name: "two Mains", phs: []string{"Main", "Main"}, wantErr: "CE0849"},
		{name: "duplicate other name", phs: []string{"Main", "Side", "Side"}, wantErr: "CE0495"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := layoutToGen(layoutWith(tc.phs...))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("a layout that builds was refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("placeholders %v were accepted", tc.phs)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not cite %s", err, tc.wantErr)
			}
		})
	}
}

// layoutWith builds a layout whose placeholders sit inside a scroll container
// region, the shape a real layout has.
func layoutWith(names ...string) *pages.Layout {
	region := &pages.ScrollContainerRegion{Slot: pages.ScrollSlotCenter}
	for _, n := range names {
		ph := &pages.LayoutPlaceholder{}
		ph.Name = n
		region.Widgets = append(region.Widgets, ph)
	}
	sc := &pages.ScrollContainer{Regions: []*pages.ScrollContainerRegion{region}}
	sc.Name = "layoutContainer"
	return &pages.Layout{
		Name:       "App_X",
		LayoutType: "Responsive",
		Widgets:    []pages.Widget{sc},
	}
}
