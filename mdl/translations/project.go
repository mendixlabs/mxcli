// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// Project is the slice of a backend this package needs: enumerate the units,
// read one, write one. Deliberately narrow — nothing here knows what kind of
// document a unit holds, which is what makes the walk cover document types mxcli
// cannot otherwise read or write.
type Project interface {
	ListUnits() ([]*types.UnitInfo, error)
	GetRawUnitBytes(id model.ID) ([]byte, error)
	// UpdateRawUnitOwningTranslations rather than UpdateRawUnit: this package
	// patches the STORED bytes, so its output is authoritative about every
	// translation in them. The ordinary write path carries stored translations
	// back onto a write — right for a rebuild, which cannot express them, and
	// exactly wrong here, where a missing translation was removed on purpose.
	UpdateRawUnitOwningTranslations(unitID string, contents []byte) error
}

// Scope decides which units take part. A nil Scope means the whole project.
type Scope func(unitID model.ID) bool

func (s Scope) includes(id model.ID) bool { return s == nil || s(id) }

// Dictionary is source string → translation, for one language. An empty value
// means "present but not translated yet".
type Dictionary map[string]string

// matcher finds a source's entry in a Dictionary, exactly or with line
// endings ignored: Marketplace texts carry CRLF ("{1} days\r\n"), and a file
// whose CRs an editor, an agent or git normalised away named each of them as
// unknown (ako/mxcli#2383).
type matcher struct {
	dict Dictionary
	byLF map[string]string // source with CRLF as LF → the dictionary key
}

func (d Dictionary) matcher() matcher {
	m := matcher{dict: d, byLF: make(map[string]string, len(d))}
	for k := range d {
		m.byLF[lf(k)] = k
	}
	return m
}

// lookup returns the dictionary key naming src, its translation, and whether
// there is one. A translation for a CRLF source takes CRLF line breaks, as
// Studio Pro stores them.
func (m matcher) lookup(src string) (key, translation string, ok bool) {
	if t, ok := m.dict[src]; ok {
		return src, t, true
	}
	key, ok = m.byLF[lf(src)]
	if !ok {
		return "", "", false
	}
	t := m.dict[key]
	if strings.Contains(src, "\r\n") && !strings.Contains(t, "\r") {
		t = strings.ReplaceAll(t, "\n", "\r\n")
	}
	return key, t, true
}

func lf(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// Stats is what an Apply did across the project.
type Stats struct {
	Set     int
	Removed int
	Units   int // units actually written
	// RemovedSources names every source string whose translation was deleted,
	// deduplicated and sorted. CREATE OR REPLACE can delete work somebody did in
	// Studio Pro, so a caller must be able to say what went rather than only how
	// much (guard-don't-drop, ADR-0005).
	RemovedSources []string
	// Unmatched names every dictionary key that matched no text in the project.
	// The signal that a source string was edited after the file was written —
	// see SuggestDrift.
	Unmatched []string
	// Written lists every unit Apply wrote, with what it changed there, in
	// walk order. A caller that has to say WHERE the writes landed — an
	// unscoped run reaching into Marketplace modules (ako/mxcli#970) — needs
	// the unit, not just the total.
	Written []UnitWrite
}

// UnitWrite is one unit Apply wrote.
type UnitWrite struct {
	ID      model.ID
	Type    string // the unit's stored $Type, e.g. "Forms$PageTemplate"
	Set     int
	Removed int
}

// Collect returns every translatable text in scope, deduplicated by source
// string. Where the same source occurs several times with different translations
// the first is kept and the source is reported in Conflicts, because no single
// dictionary entry can describe both.
func Collect(p Project, sourceLang string, scope Scope) (entries []Entry, conflicts []string, err error) {
	units, err := p.ListUnits()
	if err != nil {
		return nil, nil, fmt.Errorf("list units: %w", err)
	}
	bySource := map[string]map[string]string{}
	conflicting := map[string]bool{}
	for _, u := range units {
		if u == nil || !scope.includes(u.ID) {
			continue
		}
		raw, err := p.GetRawUnitBytes(u.ID)
		if err != nil || len(raw) == 0 {
			continue // a unit that cannot be read contributes nothing
		}
		for _, e := range CollectFromUnit(raw, sourceLang) {
			prev, seen := bySource[e.Source]
			if !seen {
				bySource[e.Source] = e.Targets
				continue
			}
			if !sameTargets(prev, e.Targets) && len(e.Targets) > 0 && len(prev) > 0 {
				conflicting[e.Source] = true
				continue
			}
			// One side is empty: prefer the one that says something.
			if len(prev) == 0 {
				bySource[e.Source] = e.Targets
			}
		}
	}
	for src, targets := range bySource {
		entries = append(entries, Entry{Source: src, Targets: targets})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Source < entries[j].Source })
	for src := range conflicting {
		conflicts = append(conflicts, src)
	}
	sort.Strings(conflicts)
	return entries, conflicts, nil
}

// Languages returns every language used by texts in scope, sorted.
func Languages(p Project, scope Scope) ([]string, error) {
	units, err := p.ListUnits()
	if err != nil {
		return nil, fmt.Errorf("list units: %w", err)
	}
	seen := map[string]bool{}
	for _, u := range units {
		if u == nil || !scope.includes(u.ID) {
			continue
		}
		raw, err := p.GetRawUnitBytes(u.ID)
		if err != nil {
			continue
		}
		for _, l := range LanguagesInUnit(raw) {
			seen[l] = true
		}
	}
	out := make([]string, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	sort.Strings(out)
	return out, nil
}

// Apply writes a dictionary across the project. Units whose translations did not
// change are not written at all, so re-running a file is a no-op down to the
// filesystem.
func Apply(p Project, sourceLang, lang string, dict Dictionary, mode Mode, scope Scope) (Stats, error) {
	var stats Stats
	units, err := p.ListUnits()
	if err != nil {
		return stats, fmt.Errorf("list units: %w", err)
	}

	matched := map[string]bool{}
	removed := map[string]bool{}
	m := dict.matcher()
	for _, u := range units {
		if u == nil || !scope.includes(u.ID) {
			continue
		}
		raw, err := p.GetRawUnitBytes(u.ID)
		if err != nil || len(raw) == 0 {
			continue
		}
		for _, e := range CollectFromUnit(raw, sourceLang) {
			if key, _, ok := m.lookup(e.Source); ok {
				matched[key] = true
			}
		}
		out, us := PatchUnit(raw, sourceLang, lang, dict, mode)
		if !us.touched() {
			continue
		}
		if err := p.UpdateRawUnitOwningTranslations(string(u.ID), out); err != nil {
			return stats, fmt.Errorf("write unit %s: %w", u.ID, err)
		}
		stats.Set += us.Set
		stats.Removed += us.Removed
		stats.Units++
		stats.Written = append(stats.Written, UnitWrite{ID: u.ID, Type: u.Type, Set: us.Set, Removed: us.Removed})
		for _, s := range us.RemovedSources {
			removed[s] = true
		}
	}

	for src := range dict {
		if !matched[src] {
			stats.Unmatched = append(stats.Unmatched, src)
		}
	}
	sort.Strings(stats.Unmatched)
	for s := range removed {
		stats.RemovedSources = append(stats.RemovedSources, s)
	}
	sort.Strings(stats.RemovedSources)
	return stats, nil
}

func sameTargets(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
