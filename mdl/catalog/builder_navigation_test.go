// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

func TestCountMenuItems(t *testing.T) {
	tests := []struct {
		name  string
		items []*types.NavMenuItem
		want  int
	}{
		{
			name:  "nil items",
			items: nil,
			want:  0,
		},
		{
			name:  "empty items",
			items: []*types.NavMenuItem{},
			want:  0,
		},
		{
			name: "flat items",
			items: []*types.NavMenuItem{
				{Caption: "Home"},
				{Caption: "About"},
				{Caption: "Contact"},
			},
			want: 3,
		},
		{
			name: "nested items",
			items: []*types.NavMenuItem{
				{
					Caption: "Admin",
					Items: []*types.NavMenuItem{
						{Caption: "Users"},
						{Caption: "Roles"},
					},
				},
				{Caption: "Home"},
			},
			want: 4, // Admin + Users + Roles + Home
		},
		{
			name: "deeply nested",
			items: []*types.NavMenuItem{
				{
					Caption: "L1",
					Items: []*types.NavMenuItem{
						{
							Caption: "L2",
							Items: []*types.NavMenuItem{
								{Caption: "L3"},
							},
						},
					},
				},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countMenuItems(tt.items); got != tt.want {
				t.Errorf("countMenuItems() = %d, want %d", got, tt.want)
			}
		})
	}
}
