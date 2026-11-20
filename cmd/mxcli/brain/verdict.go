// SPDX-License-Identifier: Apache-2.0

// verdict.go - entries that change or partly meet a requirement.
//
// A requirement is "built" when its anchors resolve, which says the thing
// exists but not that it is what was asked. The difference is recorded on a
// decision — "we did X instead", "only part of it" — or on an open question
// that holds it ("partial until we hear back"), never on the requirement
// itself: it is knowledge about why the model looks the way it does. The form
// is a line in the entry's body, which `brain capture --changes / --partly-meets`
// writes:
//
//	Changes requirement `a1b2c3`: approval is per team, not per person.
//	Partly meets requirement `a1b2c3` until answered: only mock adapters exist.
//
// The rules, enforced by `brain check` (ako/mxcli#2301):
//
//   - the id must be a requirement the plan has;
//   - a verdict counts on a decision and on an open question alike; on a
//     requirement entry it is misplaced and refused;
//   - a requirement may carry several verdicts of one kind, but not both
//     "changes" and "partly meets" — a reader could not tell which holds.
//
// The effective status of each requirement — built, partial, changed, not
// built, not traceable — is derived here once (EffectiveStatus) and reported
// by `brain plan --json`; `report traceability` reads it rather than parsing
// bodies a second time.
package brain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Verdict kinds.
const (
	VerdictChanges = "changes"
	VerdictPartly  = "partly meets"
)

// Effective requirement statuses.
const (
	StatusBuilt        = "built"
	StatusPartial      = "partial"
	StatusChanged      = "changed"
	StatusNotBuilt     = "not built"
	StatusNotTraceable = "not traceable"
)

// VerdictRef is one verdict line in an entry.
type VerdictRef struct {
	Shard         string `json:"shard,omitempty"`
	EntryID       string `json:"entry_id"`
	Title         string `json:"title,omitempty"`
	Verdict       string `json:"verdict"`
	RequirementID string `json:"requirement_id"`
	Reason        string `json:"reason,omitempty"`
	// Open marks a verdict on an open question: partial (or changed) until
	// the question is answered.
	Open bool `json:"open,omitempty"`
}

// EntryVerdict is a verdict as `brain list --json` reports it on its entry.
type EntryVerdict struct {
	Requirement string `json:"requirement"`
	Kind        string `json:"kind"`
	Reason      string `json:"reason,omitempty"`
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
		out = append(out, VerdictRef{EntryID: e.ID, Title: e.Title, Verdict: v, RequirementID: strings.ToLower(m[2]),
			Reason: strings.TrimSpace(m[3]), Open: e.Open})
	}
	return out
}

// EntryVerdicts is Verdicts in the shape `brain list --json` gives.
func EntryVerdicts(e Entry) []EntryVerdict {
	var out []EntryVerdict
	for _, v := range Verdicts(e) {
		out = append(out, EntryVerdict{Requirement: v.RequirementID, Kind: v.Verdict, Reason: v.Reason})
	}
	return out
}

// VerdictLine is the body line `capture --changes / --partly-meets` writes.
func VerdictLine(kind, requirementID, reason string) (string, error) {
	id := strings.ToLower(strings.TrimSpace(requirementID))
	if !regexp.MustCompile(`^[0-9a-f]{6}$`).MatchString(id) {
		return "", fmt.Errorf("%q is not a requirement id (six hex characters, as 'brain list' shows)", requirementID)
	}
	word := "Changes"
	if kind == VerdictPartly {
		word = "Partly meets"
	}
	line := fmt.Sprintf("%s requirement `%s`", word, id)
	if r := strings.TrimSpace(reason); r != "" {
		line += ": " + r
	}
	return line, nil
}

// verdictSet is every verdict in the store, read once.
type verdictSet struct {
	byReq map[string][]VerdictRef
	order []string
	// misplaced are verdict lines in a requirement entry: a requirement
	// cannot record a verdict on itself or on another requirement.
	misplaced    []VerdictRef
	requirements map[string]bool
}

// loadVerdicts reads every shard: decisions and open questions in module and
// project shards, and the open questions filed on a slice, which live in the
// plan shards beside the requirements.
func (s *Store) loadVerdicts() (*verdictSet, error) {
	shards, err := s.ListShards()
	if err != nil {
		return nil, err
	}
	vs := &verdictSet{byReq: map[string][]VerdictRef{}, requirements: map[string]bool{}}
	for _, sh := range shards {
		entries, _, err := s.LoadShard(sh)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			isRequirement := IsPlanShard(sh) && !e.Open
			if isRequirement {
				vs.requirements[e.ID] = true
			}
			for _, v := range Verdicts(e) {
				v.Shard = sh
				if isRequirement {
					vs.misplaced = append(vs.misplaced, v)
					continue
				}
				if _, ok := vs.byReq[v.RequirementID]; !ok {
					vs.order = append(vs.order, v.RequirementID)
				}
				vs.byReq[v.RequirementID] = append(vs.byReq[v.RequirementID], v)
			}
		}
	}
	return vs, nil
}

// VerdictConflict is a requirement that entries say both changed and partly
// met. A consumer has to pick one, so the store refuses it instead
// (ako/mxcli#2299). Two verdicts of the same kind are not a conflict: two
// decisions can each say "partly", for different reasons (ako/mxcli#2301).
type VerdictConflict struct {
	RequirementID string       `json:"requirement_id"`
	Verdicts      []VerdictRef `json:"verdicts"`
}

// findings returns the verdict defects that touch one of the checked shards,
// so `check --changed` reports what the change could have caused: verdicts
// naming no requirement, conflicting kinds, and verdicts on requirements.
func (vs *verdictSet) findings(checked []string) (unknown []VerdictRef, conflicts []VerdictConflict, misplaced []VerdictRef) {
	in := map[string]bool{}
	for _, sh := range checked {
		in[sh] = true
	}
	for _, id := range vs.order {
		refs := vs.byReq[id]
		kinds, touched := map[string]bool{}, false
		for _, v := range refs {
			kinds[v.Verdict] = true
			touched = touched || in[v.Shard]
			if !vs.requirements[id] && in[v.Shard] {
				unknown = append(unknown, v)
			}
		}
		if len(kinds) > 1 && touched {
			conflicts = append(conflicts, VerdictConflict{RequirementID: id, Verdicts: refs})
		}
	}
	for _, v := range vs.misplaced {
		if in[v.Shard] {
			misplaced = append(misplaced, v)
		}
	}
	return unknown, conflicts, misplaced
}

// EffectiveStatus derives a requirement's status: its anchors say whether it
// is built; a verdict says it was built differently ("changed") or only in
// part ("partial"). A conflicting pair is reported by check and read here as
// "changed", the stronger claim. The reason names where the status comes from.
func EffectiveStatus(r RequirementResult, verdicts []VerdictRef, modelAbsent bool) (string, string) {
	status, reason := StatusBuilt, ""
	switch {
	case len(r.Anchors) == 0:
		status, reason = StatusNotTraceable, "no anchor: nothing in the model can be checked against it"
	case !r.Built:
		var missing []string
		for _, a := range r.Anchors {
			if a.State != Resolved && a.State != NotIndexable {
				missing = append(missing, a.Anchor)
			}
		}
		status, reason = StatusNotBuilt, "not in the model: "+strings.Join(missing, ", ")
		if modelAbsent {
			reason = "no model to check against yet"
		}
	}
	sorted := append([]VerdictRef(nil), verdicts...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Verdict == VerdictChanges && sorted[j].Verdict != VerdictChanges })
	for _, v := range sorted {
		src := "decision " + v.EntryID
		if v.Open {
			src = "open question " + v.EntryID
		}
		if v.Verdict == VerdictChanges {
			return StatusChanged, src + ": " + v.Reason
		}
		return StatusPartial, src + ": " + v.Reason
	}
	return status, reason
}

// AddVerdicts appends verdict lines to an entry's body, one per spec. A spec is
// a requirement id, optionally followed by ":reason" ("a1b2c3:web only"); with
// no reason the line carries none. A line the body already has is not added
// twice, so re-running a capture or edit changes nothing.
func AddVerdicts(e Entry, kind string, specs []string) (Entry, error) {
	if len(specs) == 0 {
		return e, nil
	}
	if e.EntryKind() == KindRequirement && !e.Open {
		return Entry{}, fmt.Errorf("a verdict belongs on the decision or open question that explains it, not on a requirement: capture it without --slice (or with --open)")
	}
	for _, spec := range specs {
		id, reason, _ := strings.Cut(spec, ":")
		line, err := VerdictLine(kind, id, reason)
		if err != nil {
			return Entry{}, err
		}
		if strings.Contains(e.Body, line) {
			continue
		}
		if strings.TrimSpace(e.Body) == "" {
			e.Body = line
		} else {
			e.Body = strings.TrimRight(e.Body, "\n") + "\n" + line
		}
	}
	return e, nil
}

// RequirementIDs is the set of requirement ids the plan has.
func (s *Store) RequirementIDs() (map[string]bool, error) {
	vs, err := s.loadVerdicts()
	if err != nil {
		return nil, err
	}
	return vs.requirements, nil
}

// StripVerdicts removes an entry's verdict lines from its body and returns
// them. Resolving a question filed on a slice turns it into a requirement,
// where a verdict would be misplaced: the answer is what settles it.
func StripVerdicts(e Entry) (Entry, []VerdictRef) {
	vs := Verdicts(e)
	if len(vs) == 0 {
		return e, nil
	}
	var keep []string
	for _, line := range strings.Split(e.Body, "\n") {
		if !verdictLine.MatchString(line) {
			keep = append(keep, line)
		}
	}
	e.Body = strings.TrimSpace(strings.Join(keep, "\n"))
	return e, vs
}
