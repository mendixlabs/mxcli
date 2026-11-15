// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
)

// newGlyphTestContext returns a context whose output is captured. These two
// commands read the embedded font table, so no project and no backend is needed
// — which is the point: `show glyphs` works before you have connected.
func newGlyphTestContext() (*ExecContext, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return &ExecContext{Output: buf}, buf
}

// `show icon collections` / `describe icon collection` cover icons that are
// model documents. A glyph is not one — `icon glyph <n>` stores a bare character
// code into a font — so there was nothing to list, and MDL078 had to spell its
// advice as numeric ranges with holes in them. These two close that.

func TestListGlyphs_FindsByName(t *testing.T) {
	ctx, out := newGlyphTestContext()
	if err := listGlyphs(ctx, "star"); err != nil {
		t.Fatalf("listGlyphs: %v", err)
	}
	got := out.String()
	for _, want := range []string{"57350", "star", "57351", "star-empty", "icon glyph 57350"} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q:\n%s", want, got)
		}
	}
	// The MDL column is the point: the answer has to be pasteable, not just
	// looked at.
	if !strings.Contains(got, "icon glyph 57350") {
		t.Error("the listing does not give the MDL to write")
	}
}

// An ALIAS has to match, or `show glyphs like 'btc'` finds nothing while
// `icon glyph 57895` is exactly what the author is looking for.
func TestListGlyphs_MatchesAnAlias(t *testing.T) {
	ctx, out := newGlyphTestContext()
	if err := listGlyphs(ctx, "btc"); err != nil {
		t.Fatalf("listGlyphs: %v", err)
	}
	if !strings.Contains(out.String(), "57895") {
		t.Errorf("an alias did not match:\n%s", out.String())
	}
}

// CONTROL: no filter lists the whole font, so a broken matcher cannot pass the
// tests above by returning everything.
func TestListGlyphs_UnfilteredListsTheWholeFont(t *testing.T) {
	ctx, out := newGlyphTestContext()
	if err := listGlyphs(ctx, ""); err != nil {
		t.Fatalf("listGlyphs: %v", err)
	}
	if !strings.Contains(out.String(), "258") {
		t.Errorf("the summary does not report the full count:\n%s",
			lastLine(out.String()))
	}
}

// A miss must say what was searched. An empty table on its own reads as "the
// font is empty", which is the wrong conclusion to hand someone.
func TestListGlyphs_EmptyResultNamesTheSearch(t *testing.T) {
	ctx, out := newGlyphTestContext()
	if err := listGlyphs(ctx, "definitely-not-an-icon"); err != nil {
		t.Fatalf("listGlyphs: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "definitely-not-an-icon") || !strings.Contains(got, "258") {
		t.Errorf("an empty result does not say what was searched or how many exist:\n%s", got)
	}
}

func TestDescribeGlyph_ByCodeAndByName(t *testing.T) {
	for _, subject := range []string{"57350", "star"} {
		ctx, out := newGlyphTestContext()
		if err := describeGlyph(ctx, subject); err != nil {
			t.Fatalf("describeGlyph(%q): %v", subject, err)
		}
		got := out.String()
		for _, want := range []string{"star", "57350", "0xE006", "icon glyph 57350"} {
			if !strings.Contains(got, want) {
				t.Errorf("describeGlyph(%q) is missing %q:\n%s", subject, want, got)
			}
		}
	}
}

// An exact name must win over a substring, or `describe glyph 'star'` answers
// about star-empty depending on table order.
func TestDescribeGlyph_ExactNameBeatsASubstring(t *testing.T) {
	ctx, out := newGlyphTestContext()
	if err := describeGlyph(ctx, "star"); err != nil {
		t.Fatalf("describeGlyph: %v", err)
	}
	if strings.Contains(out.String(), "57351") {
		t.Errorf("an exact name resolved to the substring match instead:\n%s", out.String())
	}
}

// An ambiguous name names the candidates rather than picking one — the codes
// differ, so a guess would be silently wrong.
func TestDescribeGlyph_AmbiguousNameListsTheCandidates(t *testing.T) {
	ctx, _ := newGlyphTestContext()
	err := describeGlyph(ctx, "arrow")
	if err == nil {
		t.Fatal("an ambiguous name was resolved to one glyph")
	}
	if !strings.Contains(err.Error(), "arrow-left") {
		t.Errorf("the error does not name the candidates: %v", err)
	}
}

// The two misses point back at the listing, since neither a wrong code nor a
// wrong name tells you what the right one is.
func TestDescribeGlyph_MissesPointAtTheListing(t *testing.T) {
	for _, subject := range []string{"57562", "nonesuch"} {
		ctx, _ := newGlyphTestContext()
		err := describeGlyph(ctx, subject)
		if err == nil {
			t.Fatalf("describeGlyph(%q) succeeded", subject)
		}
		if !strings.Contains(err.Error(), "show glyphs") {
			t.Errorf("describeGlyph(%q) does not point at the listing: %v", subject, err)
		}
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}
