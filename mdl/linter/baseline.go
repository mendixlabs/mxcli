// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// A baseline records a project's lint findings at one moment, so a later run
// can report only what is new (mendixlabs/mxcli#1455, ako/mxcli#1328). On a
// mature app the existing backlog buries the findings about the change just
// made; teams worked around it from SARIF output, each with their own
// fingerprint, and those broke whenever a rule's message text changed.
//
// # What makes two findings the same
//
// A finding is keyed on its rule and its document — the document's id where the
// rule gives one, so a rename does not make its findings new, and its qualified
// name otherwise. The message is NOT part of the key, because it is exactly what
// changes when a rule is reworded.
//
// One rule can report one document several times (one finding per loop, per
// attribute), so within a key the findings are matched in two passes: first by
// identical message, then — for the ones left over on both sides — by count,
// taking an unmatched baseline finding as the reworded form of an unmatched
// current one. What remains on the current side is new. A third CONV011 in a
// flow that had two is therefore new, while a reworded QUAL001 is not.
//
// A rule failure is never baselined: it says nothing about the model, and
// absorbing it would let a broken rule read as "nothing new".

// baselineVersion is the file format. A file of another version is refused
// rather than half-read.
const baselineVersion = 1

// Baseline is a loaded baseline file.
type Baseline struct {
	byKey map[string][]string // key -> messages, one per recorded finding
	total int
	// byRule counts the recorded findings of each rule.
	byRule map[string]int
	// rules are the rules that ran when it was recorded (rulesRecorded is false
	// for a baseline written before that was recorded), and config is the
	// fingerprint of the configuration they ran under ("" when not recorded).
	rules         map[string]bool
	rulesRecorded bool
	config        string
}

// BaselineComparison is what a run found against a baseline.
type BaselineComparison struct {
	New       []Violation // not in the baseline, in the order the run reported them
	Unchanged int         // current findings the baseline already had
	Fixed     int         // baseline findings of a rule that ran and no longer reports them

	// NotChecked counts baseline findings whose rule did not run this time (not
	// installed, disabled, or it failed). They are not fixed: nothing looked.
	NotChecked int
	// RulesNotRun are the rules the baseline holds findings for, or recorded as
	// having run, that did not run now; RuleFailures are those that ran into an
	// error. NewRules ran now and are not in the baseline's recorded set, so all
	// their findings are new.
	RulesNotRun  []string
	RuleFailures []string
	NewRules     []string
	// ConfigChanged says the lint configuration differs from the one the
	// baseline was recorded under, which changes what the rules report.
	ConfigChanged bool
}

// Incomplete reports whether the run cannot be trusted to compare with the
// baseline: a rule it holds findings for did not run, a rule failed, or the
// configuration differs. A green result then means "nothing was checked", not
// "nothing is new".
func (c BaselineComparison) Incomplete() bool {
	return len(c.RulesNotRun) > 0 || len(c.RuleFailures) > 0 || c.ConfigChanged
}

// BaselineMeta is what a baseline records besides its findings: the rules that
// ran and the fingerprint of the configuration they ran under.
type BaselineMeta struct {
	Rules  []string
	Config string
}

// RunInfo says how the run being compared went. The zero value is a run that
// cannot say (the comparison then trusts every rule ran, as it always did).
type RunInfo struct {
	Known  bool
	Ran    []string // rules that executed to completion
	Failed []string // rules that executed and reported they could not run
	Config string
	// Scoped is a run narrowed to some rules, modules or documents: rules that
	// did not run are expected, so none of it is held against the run.
	Scoped bool
}

type baselineFile struct {
	Version int `json:"version"`
	// Rules and Config say what the findings were produced by: which rules ran
	// and under which configuration (ako/mxcli#2455). Optional, so a baseline
	// written before they were recorded still loads, and an older mxcli still
	// reads a newer file.
	Rules    []string          `json:"rules,omitempty"`
	Config   string            `json:"config,omitempty"`
	Findings []baselineFinding `json:"findings"`
}

type baselineFinding struct {
	Rule     string `json:"rule"`
	Document string `json:"document"`             // qualified name, for a reader
	ID       string `json:"documentId,omitempty"` // the key when present
	Type     string `json:"documentType,omitempty"`
	Message  string `json:"message"`
}

func baselineKey(rule, docType, id, document string) string {
	if id != "" {
		return rule + "\x00id\x00" + id
	}
	return rule + "\x00" + docType + "\x00" + document
}

func keyOf(v Violation) string {
	return baselineKey(v.RuleID, v.Location.DocumentType, v.Location.DocumentID, v.Location.QualifiedName())
}

// WriteBaseline records findings as a baseline at path. Rule failures are left
// out. The file is sorted, so re-recording an unchanged project rewrites the
// same bytes and a committed baseline diffs only where findings changed.
func WriteBaseline(path string, findings []Violation) error {
	return WriteBaselineWith(path, findings, BaselineMeta{})
}

// WriteBaselineWith is WriteBaseline that also records which rules ran and the
// configuration they ran under, so a later run can tell a finding that was fixed
// from one whose rule was not there to report it.
func WriteBaselineWith(path string, findings []Violation, meta BaselineMeta) error {
	rules := append([]string(nil), meta.Rules...)
	sort.Strings(rules)
	f := baselineFile{Version: baselineVersion, Rules: rules, Config: meta.Config, Findings: []baselineFinding{}}
	for _, v := range findings {
		if v.RuleFailure {
			continue
		}
		f.Findings = append(f.Findings, baselineFinding{
			Rule:     v.RuleID,
			Document: v.Location.QualifiedName(),
			ID:       v.Location.DocumentID,
			Type:     v.Location.DocumentType,
			Message:  v.Message,
		})
	}
	sort.Slice(f.Findings, func(i, j int) bool {
		a, b := f.Findings[i], f.Findings[j]
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Document != b.Document {
			return a.Document < b.Document
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Message < b.Message
	})
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// LoadBaseline reads a baseline written by WriteBaseline. A missing, corrupt or
// other-version file is an error: comparing against nothing would report the
// whole backlog as new, or — worse, with a half-read file — part of it.
func LoadBaseline(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading lint baseline: %w", err)
	}
	var f baselineFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("lint baseline %s is not a baseline file: %w", path, err)
	}
	if f.Version != baselineVersion {
		return nil, fmt.Errorf("lint baseline %s has format version %d; this mxcli reads version %d — re-record it with --update-baseline",
			path, f.Version, baselineVersion)
	}
	b := &Baseline{byKey: map[string][]string{}, total: len(f.Findings), byRule: map[string]int{},
		rules: map[string]bool{}, rulesRecorded: len(f.Rules) > 0, config: f.Config}
	for _, r := range f.Rules {
		b.rules[r] = true
	}
	for _, x := range f.Findings {
		k := baselineKey(x.Rule, x.Type, x.ID, x.Document)
		b.byKey[k] = append(b.byKey[k], x.Message)
		b.byRule[x.Rule]++
	}
	return b, nil
}

// Len is the number of findings the baseline records.
func (b *Baseline) Len() int { return b.total }

// Compare splits a run's findings into new and already-known ones, and counts
// the baseline findings the run no longer reports.
func (b *Baseline) Compare(findings []Violation) BaselineComparison {
	return b.CompareRun(findings, RunInfo{})
}

// CompareRun is Compare that knows how the run went. A baseline finding whose rule
// did not run is NotChecked, not Fixed: a checkout without the project's custom
// rules reported all their findings as fixed and passed the gate. Rule failures
// are never new findings (they are about the rule, not the project); they are
// listed in RuleFailures.
func (b *Baseline) CompareRun(findings []Violation, run RunInfo) BaselineComparison {
	var out BaselineComparison

	groups := map[string][]int{} // key -> indexes into findings
	var order []string
	for i, v := range findings {
		if v.RuleFailure {
			continue
		}
		k := keyOf(v)
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], i)
	}

	isNew := make([]bool, len(findings))
	for i, v := range findings {
		isNew[i] = v.RuleFailure
	}
	failed := map[string]bool{}
	for _, v := range findings {
		if v.RuleFailure {
			failed[v.RuleID] = true
		}
	}
	matched := 0
	for _, k := range order {
		// Pass 1: identical messages.
		left := map[string]int{}
		for _, m := range b.byKey[k] {
			left[m]++
		}
		remainingBaseline := len(b.byKey[k])
		var unmatched []int
		for _, i := range groups[k] {
			if left[findings[i].Message] > 0 {
				left[findings[i].Message]--
				remainingBaseline--
				matched++
				continue
			}
			unmatched = append(unmatched, i)
		}
		// Pass 2: an unmatched baseline finding is taken as the reworded form
		// of an unmatched current one; the rest are new.
		for _, i := range unmatched {
			if remainingBaseline > 0 {
				remainingBaseline--
				matched++
				continue
			}
			isNew[i] = true
		}
	}

	for i, v := range findings {
		if isNew[i] {
			out.New = append(out.New, v)
		}
	}
	out.Unchanged = matched
	out.Fixed = b.total - matched
	for id := range failed {
		out.RuleFailures = append(out.RuleFailures, id)
	}
	sort.Strings(out.RuleFailures)
	if !run.Known || run.Scoped {
		return out
	}

	ran := map[string]bool{}
	for _, id := range run.Ran {
		ran[id] = true
	}
	// Rules the baseline expects: those it has findings for, and those it
	// recorded as having run.
	expected := map[string]bool{}
	for id := range b.byRule {
		expected[id] = true
	}
	for id := range b.rules {
		expected[id] = true
	}
	for id := range expected {
		if !ran[id] {
			out.RulesNotRun = append(out.RulesNotRun, id)
			out.NotChecked += b.byRule[id]
		}
	}
	sort.Strings(out.RulesNotRun)
	out.Fixed = b.total - matched - out.NotChecked
	if b.rulesRecorded {
		for id := range ran {
			if !b.rules[id] {
				out.NewRules = append(out.NewRules, id)
			}
		}
		sort.Strings(out.NewRules)
	}
	out.ConfigChanged = b.config != "" && run.Config != "" && b.config != run.Config
	return out
}
