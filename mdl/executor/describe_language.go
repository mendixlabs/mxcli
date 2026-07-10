// SPDX-License-Identifier: Apache-2.0

// Language-aware text selection for DESCRIBE (issue #702). Mendix texts
// (Texts$Text) can carry a translation per language; a multi-language project
// therefore stores several Texts$Translation entries per widget caption/content.
// DESCRIBE must show the project's *default* language, not whichever translation
// happens to be listed first (which is often Studio Pro's auto-seeded placeholder,
// e.g. the Dutch default "Tekst").
package executor

import "github.com/JordtenBulte-OLC/mxcli/model"

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
	return lang
}

// selectTranslationText picks the best translation string from a Texts$Text
// Items array ([marker, Texts$Translation{LanguageCode, Text}, …]): the preferred
// language, else en_US, else the first non-empty translation. This replaces the
// old "return the first item" behaviour that surfaced the wrong language (#702).
func selectTranslationText(items []any, preferredLang string) string {
	byLang := make(map[string]string)
	firstNonEmpty := ""
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text, _ := m["Text"].(string)
		if lang, _ := m["LanguageCode"].(string); lang != "" {
			if _, exists := byLang[lang]; !exists {
				byLang[lang] = text
			}
		}
		if firstNonEmpty == "" && text != "" {
			firstNonEmpty = text
		}
	}
	if preferredLang != "" && byLang[preferredLang] != "" {
		return byLang[preferredLang]
	}
	if byLang[fallbackLanguageCode] != "" {
		return byLang[fallbackLanguageCode]
	}
	return firstNonEmpty
}

// pickTextTranslation selects the best translation from a model.Text map: the
// preferred language, else en_US, else the first non-empty. Mirrors
// selectTranslationText for the model-level texts (e.g. page Title).
func pickTextTranslation(t *model.Text, preferredLang string) string {
	if t == nil || len(t.Translations) == 0 {
		return ""
	}
	if preferredLang != "" {
		if v := t.Translations[preferredLang]; v != "" {
			return v
		}
	}
	if v := t.Translations[fallbackLanguageCode]; v != "" {
		return v
	}
	for _, v := range t.Translations {
		if v != "" {
			return v
		}
	}
	return ""
}
