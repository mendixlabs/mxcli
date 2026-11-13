// SPDX-License-Identifier: Apache-2.0

package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The partial's real shape: one `@each $weight` block per vendored family.
const probePartial = `
// ---------------------------------------------------------------------------
// Fonts. Vendored (SIL OFL 1.1, see mxcli-fonts/OFL-signal.txt) rather than pulled
// from a CDN: no @import ordering trap, no third-party request at runtime.
// ---------------------------------------------------------------------------
@each $weight in (400, 500, 600, 700) {
  @font-face {
    font-family: "IBM Plex Sans";
    src: url("./mxcli-fonts/ibm-plex-sans-latin-#{$weight}-normal.woff2") format("woff2");
    font-weight: $weight;
  }
}

@each $weight in (400, 500, 600) {
  @font-face {
    font-family: "IBM Plex Mono";
    src: url("./mxcli-fonts/ibm-plex-mono-latin-#{$weight}-normal.woff2") format("woff2");
    font-weight: $weight;
  }
}
`

func tokensWith(pairs map[string]string) *Tokens {
	set := TokenSet{}
	for k, v := range pairs {
		set[k] = v
	}
	return &Tokens{Source: "design.css", Base: set}
}

func TestVendoredFamilies(t *testing.T) {
	got := vendoredFamilies(probePartial)
	want := []string{"IBM Plex Sans", "IBM Plex Mono"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("family %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// probeBase is the base theme's own font stacks, shaped like Signal's and
// Console's: the heading follows the body font by var() reference.
var probeBase = TokenSet{
	"--mxt-font":         `"IBM Plex Sans", system-ui, sans-serif`,
	"--mxt-font-heading": `var(--mxt-font)`,
	"--mxt-font-mono":    `"IBM Plex Mono", ui-monospace, monospace`,
}

func TestUnusedVendoredFamilies(t *testing.T) {
	tests := []struct {
		name   string
		tokens *Tokens
		want   []string
	}{
		{
			name:   "fonts not seeded at all — nothing is dropped",
			tokens: tokensWith(map[string]string{"--mxt-brand": "#10069F"}),
			want:   nil,
		},
		{
			name:   "no tokens at all — scaffolding from a theme",
			tokens: nil,
			want:   nil,
		},
		{
			name: "a brand font replaces both",
			tokens: tokensWith(map[string]string{
				"--mxt-font":      `"Neue Haas Grotesk", Helvetica, sans-serif`,
				"--mxt-font-mono": `ui-monospace, Menlo, monospace`,
			}),
			want: []string{"IBM Plex Sans", "IBM Plex Mono"},
		},
		{
			name: "keeping the mono font keeps only its faces",
			tokens: tokensWith(map[string]string{
				"--mxt-font":      `"Neue Haas Grotesk", Helvetica, sans-serif`,
				"--mxt-font-mono": `"IBM Plex Mono", ui-monospace, monospace`,
			}),
			want: []string{"IBM Plex Sans"},
		},
		{
			name: "a heading font that still names the family keeps it",
			tokens: tokensWith(map[string]string{
				"--mxt-font":         `"Neue Haas Grotesk", sans-serif`,
				"--mxt-font-heading": `"IBM Plex Sans", sans-serif`,
				"--mxt-font-mono":    `ui-monospace, monospace`,
			}),
			want: []string{"IBM Plex Mono"},
		},
		// ako/mxcli#2210: a token the design leaves alone keeps the base's
		// value, so the family that value names is still loaded.
		{
			name:   "seeding only the body font keeps the mono the base still names",
			tokens: tokensWith(map[string]string{"--mxt-font": `Inter, system-ui, sans-serif`}),
			want:   []string{"IBM Plex Sans"},
		},
		{
			name:   "seeding only the heading keeps both base families",
			tokens: tokensWith(map[string]string{"--mxt-font-heading": `Inter, sans-serif`}),
			want:   nil,
		},
		{
			name: "a seeded heading of var(--mxt-font) follows the base body font",
			tokens: tokensWith(map[string]string{
				"--mxt-font-heading": `var(--mxt-font)`,
				"--mxt-font-mono":    `ui-monospace, monospace`,
			}),
			want: []string{"IBM Plex Mono"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := unusedVendoredFamilies(probePartial, probeBase, tc.tokens)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDropFontFaces(t *testing.T) {
	// Dropping one family leaves the other, and leaves the banner explaining it.
	one := dropFontFaces(probePartial, []string{"IBM Plex Sans"})
	if fams := vendoredFamilies(one); len(fams) != 1 || fams[0] != "IBM Plex Mono" {
		t.Errorf("after dropping Sans: %v, want [IBM Plex Mono]", fams)
	}
	if strings.Contains(one, "ibm-plex-sans-latin") {
		t.Error("the dropped family's src urls must go with it")
	}
	if !strings.Contains(one, "Fonts. Vendored") {
		t.Error("the banner still explains the family that remains")
	}

	// Dropping all of them takes the banner too: a comment about vendored fonts
	// a theme does not ship reads as fonts having gone missing by accident.
	all := dropFontFaces(probePartial, []string{"IBM Plex Sans", "IBM Plex Mono"})
	if fams := vendoredFamilies(all); len(fams) != 0 {
		t.Errorf("after dropping both: %v, want none", fams)
	}
	if strings.Contains(all, "Fonts. Vendored") {
		t.Error("with no families left the banner describes nothing and must go")
	}
	if strings.Contains(all, "@font-face") {
		t.Error("no @font-face may survive")
	}
}

// Per-family file dropping. Shipping four unused Sans weights because Mono
// survived is the same dead weight in smaller print.
func TestDropsFontFile(t *testing.T) {
	r := &rewriter{droppedFonts: []string{"IBM Plex Sans"}, keptAnyFont: true}
	for path, want := range map[string]bool{
		"theme/web/mxcli-fonts/ibm-plex-sans-latin-400-normal.woff2": true,
		"theme/web/mxcli-fonts/ibm-plex-mono-latin-400-normal.woff2": false,
		"theme/web/mxcli-fonts/OFL-signal.txt":                       false, // covers the family that stayed
		"theme/web/custom-variables.scss":                            false,
	} {
		if got := r.dropsFontFile(path); got != want {
			t.Errorf("dropsFontFile(%q) = %v, want %v", path, got, want)
		}
	}

	// With nothing loaded, everything under mxcli-fonts/ goes — licence too,
	// since it covers fonts the theme no longer ships.
	none := &rewriter{droppedFonts: []string{"IBM Plex Sans", "IBM Plex Mono"}, keptAnyFont: false}
	for _, p := range []string{
		"theme/web/mxcli-fonts/ibm-plex-mono-latin-400-normal.woff2",
		"theme/web/mxcli-fonts/OFL-signal.txt",
	} {
		if !none.dropsFontFile(p) {
			t.Errorf("with no families kept, %q must be dropped", p)
		}
	}
	if none.dropsFontFile("theme/web/main.scss") {
		t.Error("a non-font file must never be dropped")
	}
}

// The colorway is what `mxcli theme list` shows, and an inherited one shows the
// base theme's colours for a theme that no longer has them.
func TestDeriveColorway(t *testing.T) {
	base := &Theme{Colorway: []string{"#0F6E6B", "#1F5FA8", "#1F7A4D", "#B45309", "#B42318", "#5A6572"}}

	// A design that pins only some entries keeps the base's for the rest — a
	// partial seed is normal, and a hole in the swatch row reads as a defect.
	got := deriveColorway(base, tokensWith(map[string]string{
		"--mxt-brand": "#10069F",
		"--mxt-info":  "#1297E4",
	}))
	if got[0] != "#10069F" || got[1] != "#1297E4" {
		t.Errorf("seeded entries not used: %v", got)
	}
	if got[4] != "#B42318" || got[5] != "#5A6572" {
		t.Errorf("unseeded entries must fall back to the base: %v", got)
	}
	if len(got) != len(base.Colorway) {
		t.Errorf("length changed: %d, want %d", len(got), len(base.Colorway))
	}

	// Scaffolding from a theme seeds nothing, so the base's colorway stands.
	if same := deriveColorway(base, nil); len(same) != len(base.Colorway) || same[0] != base.Colorway[0] {
		t.Errorf("with no tokens the base colorway must be kept, got %v", same)
	}
}

func TestSeededSummaryNamesTheSource(t *testing.T) {
	s := seededSummary(tokensWith(map[string]string{"--mxt-brand": "#10069F"}))
	if !strings.Contains(s, "design.css") {
		t.Errorf("summary must name where the palette came from: %q", s)
	}
	if strings.Contains(strings.ToLower(s), "teal") || strings.Contains(strings.ToLower(s), "slate") {
		t.Errorf("summary must not describe the BASE theme's colours: %q", s)
	}
	if seededSummary(nil) != "" {
		t.Error("with no tokens there is nothing to say, and the base's summary stands")
	}
}

// The invariant that matters most, at the level it actually breaks: after a
// scaffold, every @font-face URL must resolve to a file the theme ships, and
// every file it ships must be loaded by one.
//
// Dropping a family touches BOTH halves — the @font-face block and the woff2
// files — and getting either alone wrong is silent: a surviving rule for a
// deleted file 404s in the browser, and a surviving file nothing loads is the
// dead weight this fix exists to remove. Unit tests on each half cannot catch a
// mismatch between them.
func TestScaffoldedThemeShipsExactlyTheFontsItLoads(t *testing.T) {
	tests := map[string]struct {
		base       string // "" scaffolds from the default base
		css        string
		wantLoaded []string
	}{
		"a brand font replaces both": {
			css: `:root {
				--mxt-brand: #10069F;
				--mxt-font: "Neue Haas Grotesk", Helvetica, sans-serif;
				--mxt-font-mono: ui-monospace, Menlo, monospace;
			}`,
			wantLoaded: nil,
		},
		"keeping mono keeps only its files": {
			css: `:root {
				--mxt-brand: #10069F;
				--mxt-font: "Neue Haas Grotesk", Helvetica, sans-serif;
				--mxt-font-mono: "IBM Plex Mono", ui-monospace, monospace;
			}`,
			wantLoaded: []string{"IBM Plex Mono"},
		},
		"a design that says nothing about fonts keeps both": {
			css:        `:root { --mxt-brand: #10069F; }`,
			wantLoaded: []string{"IBM Plex Sans", "IBM Plex Mono"},
		},
		// ako/mxcli#2210: seeding only the body font left --mxt-font-mono at
		// console's "JetBrains Mono" while its @font-face and woff2 files were
		// dropped, because only the SEEDED stacks were consulted. The theme's
		// resulting stacks decide: mono still names JetBrains Mono, and the
		// heading (var(--mxt-font)) now resolves to Inter, so only Space
		// Grotesk goes.
		"console: seeding only the body font keeps the mono the base still names": {
			base:       "console",
			css:        `:root { --mxt-font: Inter, "Segoe UI", system-ui, -apple-system, Roboto, sans-serif; }`,
			wantLoaded: []string{"JetBrains Mono"},
		},
		"console: seeding the mono font too drops it": {
			base: "console",
			css: `:root {
				--mxt-font: Inter, system-ui, sans-serif;
				--mxt-font-mono: "Fira Code", ui-monospace, monospace;
			}`,
			wantLoaded: nil,
		},
		"console: a heading that names the base body font keeps it": {
			base: "console",
			css: `:root {
				--mxt-font: Inter, system-ui, sans-serif;
				--mxt-font-heading: "Space Grotesk", sans-serif;
			}`,
			wantLoaded: []string{"Space Grotesk", "JetBrains Mono"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := newProject(t)
			design := filepath.Join(dir, "design.css")
			write(t, design, tc.css)

			if _, err := Create(dir, "probe", CreateOptions{From: design, Base: tc.base}); err != nil {
				t.Fatal(err)
			}
			themeDir := filepath.Join(dir, "theme", "mxcli-themes", "probe", "files", "theme", "web")
			partial := read(t, filepath.Join(themeDir, "_mxcli-probe.scss"))

			if got := vendoredFamilies(partial); strings.Join(got, ",") != strings.Join(tc.wantLoaded, ",") {
				t.Errorf("families loaded = %v, want %v", got, tc.wantLoaded)
			}

			// Every URL resolves to a shipped file.
			shipped := map[string]bool{}
			entries, _ := os.ReadDir(filepath.Join(themeDir, "mxcli-fonts"))
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".woff2") {
					shipped[e.Name()] = true
				}
			}
			for _, name := range fontRefs(partial) {
				if !shipped[name] {
					t.Errorf("@font-face loads %s but the theme does not ship it", name)
				}
				delete(shipped, name)
			}
			// And nothing is shipped that no rule loads.
			for leftover := range shipped {
				t.Errorf("%s is shipped but no @font-face loads it", leftover)
			}
		})
	}
}

// Dropping a family left its `@each` block's closing brace behind: the pattern
// stopped at the first `}` after the family name, which is the one closing the
// `#{$weight}` interpolation in `src:`, so the `@font-face` close was taken for
// the `@each` close. `theme create --from … --base console` wrote a partial with
// 27 `{` and 29 `}`. Every shipped base has the same shape; check each.
func TestDropFontFacesKeepsBracesBalanced(t *testing.T) {
	balanced := func(t *testing.T, label, s string) {
		t.Helper()
		if o, c := strings.Count(s, "{"), strings.Count(s, "}"); o != c {
			t.Errorf("%s: %d `{` against %d `}`", label, o, c)
		}
	}
	balanced(t, "probe, one family", dropFontFaces(probePartial, []string{"IBM Plex Sans"}))
	balanced(t, "probe, all families", dropFontFaces(probePartial, []string{"IBM Plex Sans", "IBM Plex Mono"}))

	partials, _ := filepath.Glob("assets/*/files/theme/web/_mxcli-*.scss")
	checked := 0
	for _, p := range partials {
		body, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		fams := vendoredFamilies(string(body))
		if len(fams) == 0 {
			continue
		}
		checked++
		balanced(t, p+" (as shipped)", string(body))
		got := dropFontFaces(string(body), fams)
		balanced(t, p+" (all families dropped)", got)
		if strings.Contains(got, "@font-face") {
			t.Errorf("%s: @font-face survived dropping every family", p)
		}
	}
	if checked == 0 {
		t.Fatal("no shipped partial declares a vendored family — the glob no longer finds the assets")
	}
}

// A seeded family mxcli does not vendor (Inter) got no @font-face and no file,
// and `theme create --from` said nothing (#944), so the theme rendered in a
// fallback font everywhere Inter was not installed. Create must name each such
// family; the controls are a vendored family and a generic stack, which ship or
// need nothing and must not be named.
func TestCreate_NamesSeededFontsItDoesNotVendor(t *testing.T) {
	tests := map[string]struct {
		css  string
		want []string
	}{
		"an unvendored body font is named": {
			css:  `:root { --mxt-font: "Inter", system-ui, sans-serif; }`,
			want: []string{"Inter"},
		},
		"each unvendored stack is named once": {
			css: `:root {
				--mxt-font: 'Inter', sans-serif;
				--mxt-font-heading: Inter, sans-serif;
				--mxt-font-mono: "JetBrains Mono", monospace;
			}`,
			want: []string{"Inter", "JetBrains Mono"},
		},
		"control: a vendored family is not named": {
			css:  `:root { --mxt-font-mono: "IBM Plex Mono", ui-monospace, monospace; }`,
			want: nil,
		},
		"control: a generic stack is not named": {
			css:  `:root { --mxt-font: system-ui, -apple-system, sans-serif; }`,
			want: nil,
		},
		"control: no fonts seeded": {
			css:  `:root { --mxt-brand: #10069F; }`,
			want: nil,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := newProject(t)
			design := filepath.Join(dir, "design.css")
			write(t, design, tc.css)
			res, err := Create(dir, "probe", CreateOptions{From: design})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(res.UnvendoredFonts, ",") != strings.Join(tc.want, ",") {
				t.Errorf("UnvendoredFonts = %v, want %v", res.UnvendoredFonts, tc.want)
			}
		})
	}
}
