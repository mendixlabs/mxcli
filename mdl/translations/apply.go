// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Mode is what a statement does to the translations already in the project.
// The three map onto MDL's CREATE verbs, with the LANGUAGE as the thing that
// exists (PROPOSAL_translations.md).
type Mode int

const (
	// ModeCreate is bare CREATE: the language must have no translations yet.
	// The caller checks that; the write itself behaves as ModeMerge.
	ModeCreate Mode = iota
	// ModeMerge is CREATE OR MODIFY: sources named are set, sources not named
	// are left alone.
	ModeMerge
	// ModeReplace is CREATE OR REPLACE: the dictionary is authoritative, so a
	// translation whose source is not in it — or whose entry is empty — is
	// REMOVED. This is what makes a translation file and the project unable to
	// drift apart, and it is why the caller must report what it removes.
	ModeReplace
)

// UnitStats is what one unit's patch did.
type UnitStats struct {
	Set     int // translations added or changed
	Removed int // translations deleted (ModeReplace only)
	// RemovedSources are the source strings whose translation was removed, so a
	// caller can name the work it is about to delete rather than only count it.
	RemovedSources []string
}

func (s UnitStats) touched() bool { return s.Set > 0 || s.Removed > 0 }

// CollectFromUnit returns every translatable text in a unit, keyed by its string
// in sourceLang. Texts with no sourceLang string are skipped: there is nothing
// to translate from, and nothing a dictionary could address them by.
//
// Entries are returned per occurrence, not deduplicated — the caller merges
// across units, because deduplication is only meaningful project-wide.
func CollectFromUnit(raw []byte, sourceLang string) []Entry {
	var doc bson.D
	if bson.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var out []Entry
	for _, t := range findTexts(doc) {
		src := t.Source(sourceLang)
		if src == "" {
			continue
		}
		targets := map[string]string{}
		for lang, txt := range t.byLang {
			if lang != sourceLang && txt != "" {
				targets[lang] = txt
			}
		}
		out = append(out, Entry{Source: src, Targets: targets})
	}
	return out
}

// PatchUnit applies a dictionary to one unit's raw BSON, returning the new bytes
// and what it did. The bytes are unchanged (and stats zero) when nothing matched,
// so a caller can skip the write entirely.
//
// dict maps a source string to its translation in lang. An EMPTY value means
// "present in the file, not translated yet": under merge it is skipped, and
// under replace it is treated as no translation — the same as being absent —
// because both say the file has nothing for that string.
func PatchUnit(raw []byte, sourceLang, lang string, dict map[string]string, mode Mode) ([]byte, UnitStats) {
	var stats UnitStats
	var doc bson.D
	if bson.Unmarshal(raw, &doc) != nil {
		return raw, stats
	}

	m := Dictionary(dict).matcher()
	for _, t := range findTexts(doc) {
		src := t.Source(sourceLang)
		if src == "" {
			continue
		}
		_, want, named := m.lookup(src)

		if want != "" {
			if t.setTranslation(lang, want) {
				stats.Set++
			}
			continue
		}
		// No translation in the file for this source. Under replace the file is
		// authoritative, so an existing one is removed; under merge it is left
		// alone. `named` is deliberately not consulted: an entry with an empty
		// value and an absent entry both say the file has nothing here.
		_ = named
		if mode == ModeReplace {
			if t.removeTranslation(lang) {
				stats.Removed++
				stats.RemovedSources = append(stats.RemovedSources, src)
			}
		}
	}

	if !stats.touched() {
		return raw, stats
	}
	out, err := bson.Marshal(doc)
	if err != nil {
		return raw, UnitStats{}
	}
	sort.Strings(stats.RemovedSources)
	return out, stats
}

// LanguagesInUnit returns every language a unit's texts carry, sorted. Used to
// answer "does this language exist yet", which is what bare CREATE refuses on.
func LanguagesInUnit(raw []byte) []string {
	var doc bson.D
	if bson.Unmarshal(raw, &doc) != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, t := range findTexts(doc) {
		for _, l := range t.Languages() {
			seen[l] = true
		}
	}
	out := make([]string, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}
