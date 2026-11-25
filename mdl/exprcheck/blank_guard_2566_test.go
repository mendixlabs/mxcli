// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
)

// ako/mxcli#2566: E015 caught `trim($v) != empty` (always true) but said
// nothing about the far more common blank checks around it:
//
//   - E016: `$v != empty and trim($v) != ”` — the first half is dead, since
//     trim(empty) is ”, so trim($v) != ” alone decides it;
//   - E017: `$v != empty and $v != ”` — lets a value of only spaces through
//     as "has text"; the blank check is trim($v) != ”.
//
// Both build and the first behaves correctly, so both are info.
func TestBlankGuards2566(t *testing.T) {
	ctx := Context{Microflow: "M.Probe", SlotPath: "Decision"}
	cases := []struct {
		src  string
		want string // the code expected, "" for none
	}{
		{"$Sale/Code != empty and trim($Sale/Code) != ''", "E016"},
		{"$Sale/Code != empty and toLowerCase($Sale/Code) != ''", "E016"},
		{"trim($V) != '' and $V != empty", "E016"},
		{"$V != empty and substring($V, 1) != ''", "E016"},
		{"$V = empty or trim($V) = ''", "E016"},
		{"$Flag and $V != empty and trim($V) != ''", "E016"},
		{"$Sale/Code != empty and $Sale/Code != ''", "E017"},
		{"$V != '' and $V != empty", "E017"},
		{"$V = empty or $V = ''", "E017"},

		// Controls.
		{"trim($Sale/Code) != ''", ""},
		{"$V != empty", ""},
		{"$V != ''", ""},
		{"$V != empty and trim($Other) != ''", ""},
		{"$V != empty or trim($V) != ''", ""},    // a different condition
		{"$V != empty and getKey($V) != ''", ""}, // an unmeasured function
		{"($V != empty or $Flag) and trim($V) != ''", ""},
	}
	for _, tc := range cases {
		_, hs := NewParser().Parse(tc.src, ctx)
		var got []string
		for _, h := range hs {
			if h.Code == "E016" || h.Code == "E017" {
				got = append(got, h.Code)
				if h.Severity != hints.SeverityInfo {
					t.Errorf("%q: %s severity %v, want info", tc.src, h.Code, h.Severity)
				}
			}
		}
		switch {
		case tc.want == "" && len(got) > 0:
			t.Errorf("%q: reported %v, want nothing", tc.src, got)
		case tc.want != "" && (len(got) != 1 || got[0] != tc.want):
			t.Errorf("%q: reported %v, want exactly %s", tc.src, got, tc.want)
		}
	}
}
