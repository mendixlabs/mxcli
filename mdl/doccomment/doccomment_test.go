// SPDX-License-Identifier: Apache-2.0

package doccomment

import (
	"strings"
	"testing"
)

func TestEscape_HoldsNoCommentEnd(t *testing.T) {
	for _, doc := range []string{
		"max-height: 400px; /* Adjust the value to your preference */",
		"*/",
		"**/",
		"a */ b */ c",
		`already *\/ escaped`,
		`two *\\/ backslashes`,
		"no comment end at all",
	} {
		got := Escape(doc)
		if strings.Contains(got, "*/") {
			t.Errorf("Escape(%q) = %q, still holds */", doc, got)
		}
		if back := Unescape(got); back != doc {
			t.Errorf("Unescape(Escape(%q)) = %q", doc, back)
		}
	}
}

func TestEscape_LeavesOtherTextAlone(t *testing.T) {
	for _, doc := range []string{"plain", "a * b / c", `back\slash`, "/* opener only"} {
		if got := Escape(doc); got != doc {
			t.Errorf("Escape(%q) = %q, want unchanged", doc, got)
		}
	}
}
