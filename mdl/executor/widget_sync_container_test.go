// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#1997: `widget sync --page` is documented as taking a qualified
// name, but it was compared with the bare document name, so `--page
// ModW.PageD` matched nothing and the sync reported no drift.
func TestFilterSyncContainer(t *testing.T) {
	inst := func(module, unit string) *types.CustomWidgetInstance {
		return &types.CustomWidgetInstance{ModuleName: module, UnitName: unit, UnitID: module + "/" + unit}
	}
	all := []*types.CustomWidgetInstance{
		inst("ModW", "PageD"), inst("ModW", "PageD"), // two widgets on one page
		inst("ModX", "PageD"),
		inst("ModW", "PageA"),
	}
	ids := func(in []*types.CustomWidgetInstance) string {
		var s []string
		for _, i := range in {
			s = append(s, i.UnitID)
		}
		return strings.Join(s, ",")
	}
	for _, tc := range []struct {
		want, got, err string
	}{
		{"", "ModW/PageD,ModW/PageD,ModX/PageD,ModW/PageA", ""},
		{"ModW.PageD", "ModW/PageD,ModW/PageD", ""},
		{"modx.paged", "ModX/PageD", ""},
		{"ModW.Missing", "", ""},
		{"PageA", "ModW/PageA", ""},             // bare and unambiguous: still accepted
		{"PageD", "", "ModW.PageD, ModX.PageD"}, // bare and ambiguous: refused, matches named
	} {
		got, err := filterSyncContainer(all, tc.want)
		if tc.err != "" {
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("%q: want an error naming %q, got %v", tc.want, tc.err, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tc.want, err)
			continue
		}
		if ids(got) != tc.got {
			t.Errorf("%q matched %q, want %q", tc.want, ids(got), tc.got)
		}
	}
}
