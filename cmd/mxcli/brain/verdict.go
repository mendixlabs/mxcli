// SPDX-License-Identifier: Apache-2.0

// verdict.go - decisions that change or partly meet a requirement.
//
// A requirement is "built" when its anchors resolve, which says the thing
// exists but not that it is what was asked. The difference is a decision —
// "we did X instead", "only part of it" — and it belongs with the decision
// rather than on the requirement, because it is knowledge about why the model
// looks the way it does. The convention is a line in the decision's body:
//
//	Changes requirement `a1b2c3`: approval is per team, not per person.
//	Partly meets requirement `a1b2c3`: only the web channel, not mobile.
//
// It is parsed here so `brain check` can refuse a verdict on an id the plan
// does not have, and `report traceability` reads the same parse rather than a
// second regex (ako/mxcli#2226).
package brain

import (
	"regexp"
	"strings"
)

// Verdict kinds.
const (
	VerdictChanges = "changes"
	VerdictPartly  = "partly meets"
)

// VerdictRef is one verdict line in a decision.
type VerdictRef struct {
	Shard         string `json:"shard,omitempty"`
	EntryID       string `json:"entry_id"`
	Title         string `json:"title,omitempty"`
	Verdict       string `json:"verdict"`
	RequirementID string `json:"requirement_id"`
	Reason        string `json:"reason,omitempty"`
}

var verdictLine = regexp.MustCompile("(?im)^\\s*(?:[-*]\\s*)?(changes|partly meets|partially meets)\\s+requirement\\s+`?([0-9a-f]{6})`?\\s*:?\\s*(.*)$")

// Verdicts returns the verdict lines in an entry's title and body.
func Verdicts(e Entry) []VerdictRef {
	var out []VerdictRef
	for _, m := range verdictLine.FindAllStringSubmatch(e.Title+"\n"+e.Body, -1) {
		v := VerdictChanges
		if !strings.EqualFold(m[1], "changes") {
			v = VerdictPartly
		}
		out = append(out, VerdictRef{EntryID: e.ID, Title: e.Title, Verdict: v, RequirementID: strings.ToLower(m[2]), Reason: strings.TrimSpace(m[3])})
	}
	return out
}

// unknownVerdictRefs lists verdicts in a decision shard naming no requirement.
func (s *Store) unknownVerdictRefs(shard string, entries []Entry) ([]VerdictRef, error) {
	var refs []VerdictRef
	for _, e := range entries {
		for _, v := range Verdicts(e) {
			v.Shard = shard
			refs = append(refs, v)
		}
	}
	if len(refs) == 0 {
		return nil, nil
	}
	known, err := s.requirementIDs()
	if err != nil {
		return nil, err
	}
	var out []VerdictRef
	for _, v := range refs {
		if !known[v.RequirementID] {
			out = append(out, v)
		}
	}
	return out, nil
}

func (s *Store) requirementIDs() (map[string]bool, error) {
	slices, err := s.ListSlices()
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, sh := range slices {
		entries, _, err := s.LoadShard(sh)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			ids[e.ID] = true
		}
	}
	return ids, nil
}
