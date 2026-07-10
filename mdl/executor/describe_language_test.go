// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

func transItem(lang, text string) map[string]any {
	return map[string]any{"$Type": "Texts$Translation", "LanguageCode": lang, "Text": text}
}

func TestSelectTranslationText(t *testing.T) {
	// The #702 case: a non-default translation ('Tekst') listed first must NOT win
	// when the preferred language is en_US.
	items := []any{int32(3), transItem("nl_NL", "Tekst"), transItem("en_US", "Dashboard")}
	if got := selectTranslationText(items, "en_US"); got != "Dashboard" {
		t.Errorf("preferred en_US = %q, want Dashboard", got)
	}
	// A Dutch-default project selects the nl_NL translation.
	items2 := []any{int32(3), transItem("en_US", "Text"), transItem("nl_NL", "Overzicht")}
	if got := selectTranslationText(items2, "nl_NL"); got != "Overzicht" {
		t.Errorf("preferred nl_NL = %q, want Overzicht", got)
	}
	// Preferred language absent → fall back to en_US.
	items3 := []any{int32(3), transItem("nl_NL", "Tekst"), transItem("en_US", "Dashboard")}
	if got := selectTranslationText(items3, "de_DE"); got != "Dashboard" {
		t.Errorf("missing preferred falls back to en_US = %q, want Dashboard", got)
	}
	// Preferred present but empty → skip to en_US.
	items4 := []any{int32(3), transItem("nl_NL", ""), transItem("en_US", "Dashboard")}
	if got := selectTranslationText(items4, "nl_NL"); got != "Dashboard" {
		t.Errorf("empty preferred falls back to en_US = %q, want Dashboard", got)
	}
	// No en_US either → first non-empty.
	items5 := []any{int32(3), transItem("nl_NL", ""), transItem("fr_FR", "Tableau")}
	if got := selectTranslationText(items5, "de_DE"); got != "Tableau" {
		t.Errorf("no preferred/en_US → first non-empty = %q, want Tableau", got)
	}
	// Single-language project (the common mxcli-authored case) still works.
	if got := selectTranslationText([]any{int32(3), transItem("en_US", "Settings")}, "en_US"); got != "Settings" {
		t.Errorf("single en_US = %q, want Settings", got)
	}
}

func TestPickTextTranslation(t *testing.T) {
	txt := &model.Text{Translations: map[string]string{"nl_NL": "Tekst", "en_US": "Dashboard"}}
	if got := pickTextTranslation(txt, "en_US"); got != "Dashboard" {
		t.Errorf("preferred en_US = %q, want Dashboard", got)
	}
	if got := pickTextTranslation(txt, "nl_NL"); got != "Tekst" {
		t.Errorf("preferred nl_NL = %q, want Tekst", got)
	}
	if got := pickTextTranslation(txt, "de_DE"); got != "Dashboard" {
		t.Errorf("missing preferred falls back to en_US = %q, want Dashboard", got)
	}
	if got := pickTextTranslation(nil, "en_US"); got != "" {
		t.Errorf("nil text = %q, want empty", got)
	}
}

// End-to-end: extractTextContent must return the default-language translation,
// not the first Items entry. Reproduces and fixes issue #702.
func TestExtractTextContent_LanguageAware(t *testing.T) {
	content := map[string]any{
		"$Type": "Forms$ClientTemplate",
		"Template": map[string]any{
			"$Type": "Texts$Text",
			"Items": []any{int32(3), transItem("nl_NL", "Tekst"), transItem("en_US", "Dashboard")},
		},
	}
	w := map[string]any{"$Type": "Forms$DynamicText", "Content": content}
	// ctx with the default language pre-cached to en_US (no backend needed).
	ctx := &ExecContext{Cache: &executorCache{defaultLang: "en_US", defaultLangLoaded: true}}
	if got := extractTextContent(ctx, w, "Content"); got != "Dashboard" {
		t.Fatalf("extractTextContent = %q, want Dashboard (issue #702)", got)
	}
}
