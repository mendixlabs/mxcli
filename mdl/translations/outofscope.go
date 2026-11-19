// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"fmt"
	"sort"
)

// OutOfScope returns the dictionary keys that match a translatable text in a
// unit the scope EXCLUDES, sorted.
//
// The reason it exists (ledger #137): the NAVIGATION is a project-level
// document, not a module one, so `create or modify translations in Ledger …`
// never reaches it. The app's page text switched to Dutch and the sidebar did
// not, while the run reported
//
//	Set 212 nl_NL translation(s) across 20 document(s)
//
// — a success and a document count, and the count is the only thing that would
// have given it away, if you knew what number to expect.
//
// The question answered here is deliberately narrow: which of THIS FILE's own
// entries did the scope keep it from reaching. "The project has other strings"
// is true of every scoped run and would warn forever, which is exactly the
// per-module workflow the scoping exists to support.
//
// A nil scope excludes nothing, so an unscoped run gets an empty result without
// walking anything.
func OutOfScope(p Project, sourceLang string, scope Scope, dict Dictionary) ([]string, error) {
	if scope == nil || len(dict) == 0 {
		return nil, nil
	}
	units, err := p.ListUnits()
	if err != nil {
		return nil, fmt.Errorf("list units: %w", err)
	}
	missed := map[string]bool{}
	m := dict.matcher()
	for _, u := range units {
		if u == nil || scope.includes(u.ID) {
			continue
		}
		raw, err := p.GetRawUnitBytes(u.ID)
		if err != nil || len(raw) == 0 {
			continue // a unit that cannot be read contributes nothing
		}
		for _, e := range CollectFromUnit(raw, sourceLang) {
			if key, _, named := m.lookup(e.Source); named {
				missed[key] = true
			}
		}
	}
	out := make([]string, 0, len(missed))
	for src := range missed {
		out = append(out, src)
	}
	sort.Strings(out)
	return out, nil
}
