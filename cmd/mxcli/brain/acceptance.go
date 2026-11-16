// SPDX-License-Identifier: Apache-2.0

// acceptance.go - lint findings a project has decided to keep.
//
// Accepting a lint finding is a project decision — "Beperking has no entity
// access rules on purpose" — and so belongs in the brain, with its reason, next
// to the other decisions about that module. Recorded as free text it changed
// nothing: lint kept reporting the finding, the warning count never showed what
// was actually open, and the security report needed the same verdict written a
// second time in an annotations file (ako/mxcli#2304).
//
// The form is a line in a decision's body, which `brain capture --accepts`
// writes:
//
//	Accepts lint `SEC001` on `@Dossier.Beperking`: read only through DS_Beperkingen_Inzien, which logs every view.
//
// `mxcli lint` reads these lines: a matching finding is shown as accepted and
// left out of the counts, and an acceptance that no finding matches any more is
// reported as stale — the rule no longer fires there, so the decision is out of
// date. `brain check` validates the shape: the element must resolve, and the
// rule id must be one lint knows.
package brain

import (
	"fmt"
	"regexp"
	"strings"
)

// Acceptance is one accepted lint finding.
type Acceptance struct {
	Shard   string `json:"shard,omitempty"`
	EntryID string `json:"entry_id"`
	Title   string `json:"title,omitempty"`
	Rule    string `json:"rule"`
	// Element is the anchor the acceptance covers, without the '@': a
	// document ("Dossier.Beperking"), a member (its document is matched), a
	// module (every finding of the rule in that module), or "project" for a
	// finding about the project as a whole (security level, demo users,
	// password policy), which no anchor can name.
	Element string `json:"element"`
	Reason  string `json:"reason,omitempty"`
}

// EntryAcceptance is an acceptance as `brain list --json` reports it.
type EntryAcceptance struct {
	Rule    string `json:"rule"`
	Element string `json:"element"`
	Reason  string `json:"reason,omitempty"`
}

var acceptLine = regexp.MustCompile("(?im)^\\s*(?:[-*]\\s*)?accepts\\s+lint\\s+`?([A-Za-z][A-Za-z0-9_-]*)`?\\s+on\\s+`?@?([A-Za-z_][\\w]*(?:\\.[A-Za-z_][\\w]*){0,2})`?\\s*:?\\s*(.*)$")

// ruleID is the shape of a lint rule id: SEC001, CONV007, MDL-FLOW01.
var ruleID = regexp.MustCompile(`^[A-Z][A-Z0-9]*(-[A-Z0-9]+)*$`)

// Acceptances returns the acceptance lines in an entry's title and body.
func Acceptances(e Entry) []Acceptance {
	var out []Acceptance
	for _, m := range acceptLine.FindAllStringSubmatch(e.Title+"\n"+e.Body, -1) {
		out = append(out, Acceptance{EntryID: e.ID, Title: e.Title, Rule: strings.ToUpper(m[1]), Element: m[2], Reason: strings.TrimSpace(m[3])})
	}
	return out
}

// EntryAcceptances is Acceptances in the shape `brain list --json` gives.
func EntryAcceptances(e Entry) []EntryAcceptance {
	var out []EntryAcceptance
	for _, a := range Acceptances(e) {
		out = append(out, EntryAcceptance{Rule: a.Rule, Element: a.Element, Reason: a.Reason})
	}
	return out
}

// ProjectElement is the element of an acceptance that covers a project-wide
// finding.
const ProjectElement = "project"

func isProjectElement(el string) bool { return strings.EqualFold(el, ProjectElement) }

// AcceptanceLine is the body line `capture --accepts` writes.
func AcceptanceLine(rule, element, reason string) (string, error) {
	r := strings.ToUpper(strings.TrimSpace(rule))
	if !ruleID.MatchString(r) {
		return "", fmt.Errorf("%q is not a lint rule id (as 'mxcli lint --list-rules' shows them: SEC001, CONV007, MDL-FLOW01)", rule)
	}
	el := strings.TrimPrefix(strings.TrimSpace(element), "@")
	if isProjectElement(el) {
		line := fmt.Sprintf("Accepts lint `%s` on `%s`", r, ProjectElement)
		if s := strings.TrimSpace(reason); s != "" {
			line += ": " + s
		}
		return line, nil
	}
	if _, err := ParseAnchor(el); err != nil {
		return "", err
	}
	line := fmt.Sprintf("Accepts lint `%s` on `@%s`", r, el)
	if s := strings.TrimSpace(reason); s != "" {
		line += ": " + s
	}
	return line, nil
}

// AddAcceptances appends one acceptance line per rule spec ("SEC001" or
// "SEC001:reason") for each of the entry's anchors. A decision accepts a
// finding on the elements it is about; a line the body already has is not
// added twice.
func AddAcceptances(e Entry, specs []string) (Entry, error) {
	if len(specs) == 0 {
		return e, nil
	}
	if e.EntryKind() == KindRequirement || e.Open {
		return Entry{}, fmt.Errorf("accepting a lint finding is a decision: capture it without --slice and --open")
	}
	// An unanchored decision is about the project, and accepts a project-wide
	// finding; it is filed in project.md.
	elements := e.Anchors
	if len(elements) == 0 {
		elements = []string{ProjectElement}
	}
	for _, spec := range specs {
		rule, reason, _ := strings.Cut(spec, ":")
		for _, a := range elements {
			line, err := AcceptanceLine(rule, a, reason)
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
	}
	return e, nil
}

// LoadAcceptances reads every acceptance in the store's decisions. Open
// questions and requirements do not accept anything: an acceptance on them is
// reported as misplaced by Check.
func (s *Store) LoadAcceptances() (accepted, misplaced []Acceptance, err error) {
	if !s.Exists() {
		return nil, nil, nil
	}
	shards, err := s.ListShards()
	if err != nil {
		return nil, nil, err
	}
	for _, sh := range shards {
		entries, _, err := s.LoadShard(sh)
		if err != nil {
			return nil, nil, err
		}
		for _, e := range entries {
			for _, a := range Acceptances(e) {
				a.Shard = sh
				if e.Open || e.EntryKind() == KindRequirement {
					misplaced = append(misplaced, a)
					continue
				}
				accepted = append(accepted, a)
			}
		}
	}
	return accepted, misplaced, nil
}

// Covers reports whether the acceptance covers a finding of ruleID located in
// module and document. The document may come bare ("Beperking") or qualified
// ("Dossier.Beperking"), as rules report it either way. A module element
// covers every finding of the rule in that module; a member element covers
// findings on its document.
func (a Acceptance) Covers(ruleID, module, document string) bool {
	if !strings.EqualFold(a.Rule, ruleID) {
		return false
	}
	if isProjectElement(a.Element) {
		return module == ""
	}
	if module == "" {
		return false
	}
	doc := document
	if doc != "" && !strings.HasPrefix(doc, module+".") {
		doc = module + "." + doc
	}
	parts := strings.Split(a.Element, ".")
	switch len(parts) {
	case 1:
		return parts[0] == module
	default:
		return parts[0]+"."+parts[1] == doc
	}
}

// AcceptanceFinding is an acceptance that cannot hold, and why.
type AcceptanceFinding struct {
	Acceptance
	Problem string `json:"problem"`
}

// checkAcceptances validates the acceptances in the checked shards: placement
// and whether the element resolves. With no model the element is not judged.
func (s *Store) checkAcceptances(r Resolver, checked []string) ([]AcceptanceFinding, error) {
	accepted, misplaced, err := s.LoadAcceptances()
	if err != nil {
		return nil, err
	}
	in := map[string]bool{}
	for _, sh := range checked {
		in[sh] = true
	}
	var out []AcceptanceFinding
	for _, a := range misplaced {
		if in[a.Shard] {
			out = append(out, AcceptanceFinding{a, "accepting a lint finding is a decision; it is on an open question or a requirement"})
		}
	}
	for _, a := range accepted {
		if !in[a.Shard] || isProjectElement(a.Element) {
			continue
		}
		anchor, err := ParseAnchor(a.Element)
		if err != nil {
			out = append(out, AcceptanceFinding{a, err.Error()})
			continue
		}
		res, err := r.Resolve(anchor)
		if err != nil {
			return nil, err
		}
		if res.State == NotFound {
			out = append(out, AcceptanceFinding{a, "@" + a.Element + " is not in the model"})
		}
	}
	return out, nil
}
