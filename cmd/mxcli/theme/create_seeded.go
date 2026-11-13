// SPDX-License-Identifier: Apache-2.0

package theme

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

// A theme scaffolded with `--from <design-file>` gets the design's palette and
// used to keep everything else from the base theme verbatim. Two consequences,
// both reported by ako/ChipCoV1:
//
//   - A brand-seeded theme still described itself in `mxcli theme list` as
//     "Cool slate, one teal signal colour", and still showed Signal's six
//     swatches — so the one command whose job is to tell themes apart showed the
//     wrong one. Corrected by hand there; it should not need correcting.
//   - It vendored ~500 KB of IBM Plex woff2 that the seeded `--mxt-font` never
//     names, plus the @font-face rules loading them and a SIL OFL licence for
//     fonts the theme does not use.
//
// Both are the same mistake: inheriting a statement ABOUT the base theme into a
// theme whose palette is no longer the base's. A value that describes the design
// is derived; a value that cannot be derived is dropped rather than inherited,
// because an inherited one is a confident lie and an absent one is merely
// missing.

// colorwayTokens are the palette entries a colorway shows, in swatch order.
// This is the mapping the built-in themes already use — Signal's colorway is
// exactly its brand/info/success/warning/danger/ink-muted — so deriving one
// reproduces the hand-written value rather than inventing a new convention.
var colorwayTokens = []string{
	"--mxt-brand",
	"--mxt-info",
	"--mxt-success",
	"--mxt-warning",
	"--mxt-danger",
	"--mxt-ink-muted",
}

// deriveColorway builds the swatch list from the seeded palette, falling back
// to the base theme's value for any entry the design did not declare — a
// partial seed is normal (a brand usually pins its brand colour and leaves the
// semantic ones alone), and a hole in the swatch row would read as a defect.
func deriveColorway(base *Theme, tokens *Tokens) []string {
	if tokens == nil {
		return base.Colorway
	}
	out := make([]string, 0, len(colorwayTokens))
	for i, name := range colorwayTokens {
		if v, ok := tokens.Base[name]; ok && strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
			continue
		}
		if i < len(base.Colorway) {
			out = append(out, base.Colorway[i])
		}
	}
	if len(out) == 0 {
		return base.Colorway
	}
	return out
}

// seededSummary replaces the base theme's summary, which describes the base's
// colours and is wrong the moment a palette is seeded over it.
//
// It states what is true and checkable — where the palette came from and how
// many declarations it carried — rather than trying to describe colours in
// prose. `--summary` overrides it for a theme that wants a real description.
func seededSummary(tokens *Tokens) string {
	if tokens == nil {
		return ""
	}
	n := tokens.Count()
	decl := "declarations"
	if n == 1 {
		decl = "declaration"
	}
	return fmt.Sprintf("Project theme; palette seeded from %s (%d %s).", tokens.Source, n, decl)
}

// fontFaceFamilyRe finds the family each @font-face block declares. The built-in
// themes group faces by family in one `@each $weight` block per family, so the
// family name is what identifies a block to keep or drop.
var fontFaceFamilyRe = regexp.MustCompile(`(?s)@each\s+\$weight[^{]*\{\s*@font-face\s*\{[^}]*font-family:\s*"([^"]+)"`)

// vendoredFamilies lists the font families a theme partial loads with
// @font-face, in the order they appear.
func vendoredFamilies(scss string) []string {
	var out []string
	for _, m := range fontFaceFamilyRe.FindAllStringSubmatch(scss, -1) {
		out = append(out, m[1])
	}
	return out
}

// fontTokens are the tokens a theme's font stacks are declared under.
var fontTokens = []string{"--mxt-font", "--mxt-font-heading", "--mxt-font-mono"}

// fontVarRe matches a var() reference to one of the font tokens, with any
// fallback.
var fontVarRe = regexp.MustCompile(`var\(\s*(--mxt-font[a-z-]*)\s*(?:,[^)]*)?\)`)

// resultingFontStacks returns the font stack each font token will hold in the
// new theme: the seeded value where the design sets the token, otherwise the
// base theme's value. A var() reference to another font token is resolved
// against those same resulting stacks, so a base heading of var(--mxt-font)
// follows a seeded body font.
func resultingFontStacks(base TokenSet, tokens *Tokens) map[string]string {
	raw := map[string]string{}
	for _, name := range fontTokens {
		if v, ok := tokens.Base[name]; ok && strings.TrimSpace(v) != "" {
			raw[name] = v
		} else if v, ok := base[name]; ok {
			raw[name] = v
		}
	}
	var resolve func(name string, depth int) string
	resolve = func(name string, depth int) string {
		v := raw[name]
		if depth > len(fontTokens) {
			return v // a reference cycle; leave it unresolved
		}
		return fontVarRe.ReplaceAllStringFunc(v, func(m string) string {
			ref := fontVarRe.FindStringSubmatch(m)[1]
			return m + " " + resolve(ref, depth+1)
		})
	}
	out := map[string]string{}
	for name := range raw {
		out[name] = resolve(name, 0)
	}
	return out
}

// unusedVendoredFamilies returns the vendored families none of the new theme's
// font stacks name.
//
// A design that does not mention fonts at all seeds none, every family stays,
// and the theme keeps working — which is the right default, because dropping
// a font nobody asked to change would break the scaffold's own rendering.
//
// When fonts ARE seeded, the test is on the RESULTING stacks, not the seeded
// ones: a token the design leaves alone keeps the base's value, and if that
// value names a vendored family the family is still loaded. Comparing only the
// seeded stacks dropped console's JetBrains Mono when a design set just
// --mxt-font, leaving --mxt-font-mono naming a font with no @font-face
// (ako/mxcli#2210). base is the base theme's own declarations.
func unusedVendoredFamilies(scss string, base TokenSet, tokens *Tokens) []string {
	if tokens == nil {
		return nil
	}
	seeded := false
	for _, name := range fontTokens {
		if _, ok := tokens.Base[name]; ok {
			seeded = true
		}
	}
	if !seeded {
		return nil // fonts were not seeded; nothing to drop
	}
	stacks := resultingFontStacks(base, tokens)
	var parts []string
	for _, name := range fontTokens {
		parts = append(parts, stacks[name])
	}
	declared := strings.ToLower(strings.Join(parts, " | "))

	var unused []string
	for _, fam := range vendoredFamilies(scss) {
		if !strings.Contains(declared, strings.ToLower(fam)) {
			unused = append(unused, fam)
		}
	}
	return unused
}

// genericFontFamilies are the CSS generic families and system-font aliases: a
// stack led by one of these asks for whatever the platform has, so there is no
// file to ship and nothing to warn about.
var genericFontFamilies = map[string]bool{
	"serif": true, "sans-serif": true, "monospace": true, "cursive": true,
	"fantasy": true, "math": true, "emoji": true, "fangsong": true,
	"system-ui": true, "ui-serif": true, "ui-sans-serif": true,
	"ui-monospace": true, "ui-rounded": true,
	"-apple-system": true, "blinkmacsystemfont": true,
	"inherit": true, "initial": true, "unset": true, "revert": true,
}

// unvendoredSeededFamilies returns the primary family of each seeded font stack
// that the base partial does not load with @font-face (#944).
//
// Only the first family counts: it is the one the design chose, and the rest
// of the stack is the fallback that by definition renders "where installed". A
// stack led by a generic family or a var() reference names no font file to
// ship. A family is vendored when the partial loads it, compared without case.
func unvendoredSeededFamilies(scss string, tokens *Tokens) []string {
	if tokens == nil {
		return nil
	}
	vendored := map[string]bool{}
	for _, fam := range vendoredFamilies(scss) {
		vendored[strings.ToLower(fam)] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, name := range []string{"--mxt-font", "--mxt-font-heading", "--mxt-font-mono"} {
		v, ok := tokens.Base[name]
		if !ok {
			continue
		}
		first := strings.TrimSpace(strings.SplitN(v, ",", 2)[0])
		first = strings.Trim(first, `"'`)
		key := strings.ToLower(first)
		if first == "" || strings.HasPrefix(key, "var(") || genericFontFamilies[key] ||
			vendored[key] || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, first)
	}
	return out
}

// dropFontFaces removes the @font-face blocks for the named families, and the

// section comment when nothing is left to explain.
func dropFontFaces(scss string, families []string) string {
	for _, fam := range families {
		scss = dropFontFaceBlock(scss, fam)
	}
	if len(vendoredFamilies(scss)) == 0 {
		// The banner explains vendored fonts; with none left it describes
		// nothing, and a comment about files the theme does not ship is how the
		// next reader concludes the fonts went missing by accident.
		scss = fontSectionCommentRe.ReplaceAllString(scss, "")
	}
	return strings.TrimRight(scss, "\n") + "\n"
}

// eachWeightRe finds the start of a per-family `@each $weight` block, with the
// blank lines before it.
var eachWeightRe = regexp.MustCompile(`\n*@each\s+\$weight[^{]*\{`)

// dropFontFaceBlock removes every `@each $weight` block that declares family.
//
// The block's end is found by counting braces, not by a pattern: its `src:`
// interpolates `#{$weight}`, whose `}` a `[^}]*\}` pattern took for the
// @font-face close — so the @each's own `}` was left behind and the partial no
// longer compiled. An interpolation's braces are balanced, so depth counting
// steps over them.
func dropFontFaceBlock(scss, family string) string {
	familyRe := regexp.MustCompile(`font-family:\s*"` + regexp.QuoteMeta(family) + `"`)
	var out strings.Builder
	rest := scss
	for {
		loc := eachWeightRe.FindStringIndex(rest)
		if loc == nil {
			break
		}
		end := closingBrace(rest, loc[1]-1)
		if end < 0 {
			break // unbalanced: leave the rest as written rather than guess
		}
		out.WriteString(rest[:loc[0]])
		if !familyRe.MatchString(rest[loc[1]:end]) {
			out.WriteString(rest[loc[0] : end+1])
		}
		rest = rest[end+1:]
	}
	out.WriteString(rest)
	return out.String()
}

// closingBrace returns the index of the `}` matching the `{` at open, or -1.
func closingBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// fontSectionCommentRe matches the vendored-fonts banner comment.
var fontSectionCommentRe = regexp.MustCompile(`(?m)\n*^// -+\n(?:^// .*\n)*?^// Fonts\. Vendored.*\n(?:^//.*\n)*^// -+\n`)

// isVendoredFontPath reports whether a scaffold-relative path is one of the
// vendored font files or their licence.
func isVendoredFontPath(rel string) bool {
	return strings.Contains(rel, "mxcli-fonts/")
}

// planFonts decides which vendored families the new theme keeps, before any
// file is written.
//
// It reads the base theme's partial — the one file that carries @font-face —
// rather than waiting to meet it during the walk, so the outcome does not
// depend on WalkDir reaching the partial before the woff2 files it loads.
func (r *rewriter) planFonts(src source, root string, tokens *Tokens) error {
	if tokens == nil {
		return nil // nothing seeded; the base theme's fonts stand
	}
	partial := root + "/theme/web/_mxcli-" + r.baseName + ".scss"
	body, err := fs.ReadFile(src.fsys, partial)
	if err != nil {
		// A base theme with no such partial simply has no vendored fonts to
		// reason about. That is not an error — it is a theme that already
		// relies on system fonts. Every seeded family is then unvendored.
		r.unvendoredFonts = unvendoredSeededFamilies("", tokens)
		return nil
	}
	scss := string(body)
	r.unvendoredFonts = unvendoredSeededFamilies(scss, tokens)

	// The base's own font stacks stand wherever the design does not seed
	// one, so they decide what is still loaded as much as the seeded ones.
	// A base without a readable custom-variables.scss contributes nothing,
	// which degrades to comparing the seeded stacks alone.
	var baseTokens TokenSet
	if vars, err := fs.ReadFile(src.fsys, root+"/theme/web/custom-variables.scss"); err == nil {
		baseTokens = ExtractTokens("custom-variables.scss", string(vars)).Base
	}
	r.droppedFonts = unusedVendoredFamilies(scss, baseTokens, tokens)
	r.keptAnyFont = len(r.droppedFonts) < len(vendoredFamilies(scss))
	return nil
}

// dropsFontFile reports whether a scaffold-relative path is a font file the new
// theme no longer loads.
//
// Per family, not all-or-nothing: a brand theme routinely changes its body font
// and keeps the mono one for code, and shipping four unused Sans weights
// because Mono survived is the same dead weight in smaller print. The licence
// and any other non-woff2 file under mxcli-fonts/ is kept as long as ANY family
// is still loaded, because it covers the ones that remain.
func (r *rewriter) dropsFontFile(rel string) bool {
	if !isVendoredFontPath(rel) {
		return false
	}
	if !r.keptAnyFont {
		return true // nothing loads any of them, licence included
	}
	base := strings.ToLower(path.Base(rel))
	for _, fam := range r.droppedFonts {
		if strings.HasPrefix(base, fontFileSlug(fam)+"-") {
			return true
		}
	}
	return false
}

// fontFileSlug turns a family name into the filename stem the vendored files
// use: "IBM Plex Sans" -> "ibm-plex-sans".
func fontFileSlug(family string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(family), " ", "-"))
}
