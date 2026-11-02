// SPDX-License-Identifier: Apache-2.0

// Language-aware text selection for DESCRIBE (issue #702). Mendix texts
// (Texts$Text) can carry a translation per language; a multi-language project
// therefore stores several Texts$Translation entries per widget caption/content.
// DESCRIBE must show the project's *default* language, not whichever translation
// happens to be listed first (which is often Studio Pro's auto-seeded placeholder,
// e.g. the Dutch default "Tekst").
package executor

import (
	"sort"

	"github.com/mendixlabs/mxcli/model"
)

const fallbackLanguageCode = "en_US"

// describeDefaultLanguage returns the project's default language code for text
// selection, caching it on the executor cache. Falls back to en_US when settings
// aren't available (e.g. no connection). Pre-warm it (call once) before any
// parallel widget extraction so subsequent reads are race-free.
func describeDefaultLanguage(ctx *ExecContext) string {
	if ctx == nil || ctx.Cache == nil {
		return fallbackLanguageCode
	}
	if ctx.Cache.defaultLangLoaded {
		return ctx.Cache.defaultLang
	}
	lang := fallbackLanguageCode
	if ctx.Backend != nil {
		if ps, err := ctx.Backend.GetProjectSettings(); err == nil &&
			ps != nil && ps.Language != nil && ps.Language.DefaultLanguageCode != "" {
			lang = ps.Language.DefaultLanguageCode
		}
	}
	ctx.Cache.defaultLang = lang
	ctx.Cache.defaultLangLoaded = true
	// Publish it to the writer layers. Their Texts$Text leaves cannot be handed a
	// language without changing 183 function signatures (see
	// model.AuthoringLanguage), so this single resolution point is also where the
	// process learns it — before any write, which is the contract.
	model.SetAuthoringLanguage(lang)
	return lang
}

// selectTranslationText picks the best translation string from a Texts$Text
// Items array ([marker, Texts$Translation{LanguageCode, Text}, …]) by
// model.SelectTranslation: the preferred language (even when empty), else
// en_US, else the first non-empty translation. This replaces the old "return
// the first item" behaviour that surfaced the wrong language (#702), and is the
// rule navigation and menu captions are read with too.
func selectTranslationText(items []any, preferredLang string) string {
	var entries []model.TranslationEntry
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text, _ := m["Text"].(string)
		lang, _ := m["LanguageCode"].(string)
		entries = append(entries, model.TranslationEntry{LanguageCode: lang, Text: text})
	}
	return model.SelectTranslation(entries, preferredLang)
}

// pickTextTranslation selects the best translation from a model.Text map: the
// preferred language, else en_US, else the non-empty one with the lowest
// language code. Mirrors selectTranslationText for the model-level texts (e.g.
// page Title, enumeration value Caption).
// As there, a preferred language that is present but empty wins.
func pickTextTranslation(t *model.Text, preferredLang string) string {
	if lang, ok := pickTextLanguage(t, preferredLang); ok {
		return t.Translations[lang]
	}
	return ""
}

// pickTextLanguage is pickTextTranslation's choice as a language code, for a
// caller that must also name the languages it did not print. ok is false when
// no translation qualifies.
func pickTextLanguage(t *model.Text, preferredLang string) (string, bool) {
	if t == nil || len(t.Translations) == 0 {
		return "", false
	}
	if preferredLang != "" {
		if _, ok := t.Translations[preferredLang]; ok {
			return preferredLang, true
		}
	}
	if t.Translations[fallbackLanguageCode] != "" {
		return fallbackLanguageCode, true
	}
	// Last resort: any stored translation beats reporting a caption Studio Pro
	// plainly shows as empty. Pick it by sorted language code — iterating the map
	// directly makes DESCRIBE output vary run to run, which breaks diffing and
	// makes the describe -> exec round trip non-deterministic.
	langs := make([]string, 0, len(t.Translations))
	for lang := range t.Translations {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	for _, lang := range langs {
		if t.Translations[lang] != "" {
			return lang, true
		}
	}
	return "", false
}

// authoringLanguage is the language a bare MDL string (Caption:, Title:,
// Content:, Label:, …) is stored under when a document is CREATED. Mendix has no
// language-neutral text: every Texts$Translation carries a LanguageCode, so a
// writer must pick one, and mxcli picked "en_US" everywhere
// (mendixlabs/mxcli#970). On a project that does not have en_US enabled, Studio
// Pro then shows the empty-caption placeholder and warns there is no translation
// for the project's language — while mxcli, mx check and mxbuild all report
// success, so nothing catches it.
//
// The project's DefaultLanguageCode is the right choice, and it is by definition
// one of the enabled languages, so this can never produce the unenabled-language
// case that unenabledLanguageWarning reports. It deliberately shares
// describeDefaultLanguage's cache and fallback: DESCRIBE must read back the
// language CREATE wrote, or describe -> exec stops round-tripping on any project
// whose default is not en_US.
//
// Editing an existing text (ALTER PAGE … SET Caption/Title/Label/Content) writes
// this language's translation too: it updates the entry in this language, or
// adds one, and keeps every other language's. Editing "whatever LanguageCode is
// already there" left a de_DE project's en_US-only tab caption empty in de_DE
// (mxbuild CE4899) after the very statement meant to fill it.
func authoringLanguage(ctx *ExecContext) string { return describeDefaultLanguage(ctx) }
