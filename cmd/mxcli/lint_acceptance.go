// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Lint findings the project brain accepts (ako/mxcli#2304). A decision with
// "Accepts lint `SEC001` on `@Dossier.Beperking`: …" takes the finding out of
// what lint counts as open, keeps it visible on request, and turns into a stale
// acceptance the moment the rule stops firing there.

// acceptedFinding is a lint finding a brain decision accepts.
type acceptedFinding struct {
	Violation linter.Violation
	By        brain.Acceptance
}

// brainAcceptances is the store's acceptances, applied to one lint run.
type brainAcceptances struct {
	all      []brain.Acceptance
	accepted []acceptedFinding
	// used marks the acceptances that covered at least one finding.
	used map[int]bool
}

// loadBrainAcceptances reads the acceptances of the brain beside the project.
// No store, or one that cannot be read, is no acceptances: lint must not fail
// on the brain.
func loadBrainAcceptances(projectDir string) *brainAcceptances {
	all, _, err := brain.NewStore(projectDir).LoadAcceptances()
	if err != nil {
		return &brainAcceptances{}
	}
	return &brainAcceptances{all: all, used: map[int]bool{}}
}

// apply splits the findings into the ones still open and the accepted ones. A
// rule failure is never accepted: it says nothing about the model.
func (b *brainAcceptances) apply(vs []linter.Violation) []linter.Violation {
	if len(b.all) == 0 {
		return vs
	}
	var kept []linter.Violation
	for _, v := range vs {
		hit := -1
		if !v.RuleFailure {
			for i, a := range b.all {
				if a.Covers(v.RuleID, v.Location.Module, v.Location.DocumentName) {
					hit = i
					break
				}
			}
		}
		if hit < 0 {
			kept = append(kept, v)
			continue
		}
		b.used[hit] = true
		b.accepted = append(b.accepted, acceptedFinding{Violation: v, By: b.all[hit]})
	}
	return kept
}

// stale lists the acceptances no finding matched although their rule ran and
// their module was linted: the rule no longer fires there, so the decision is
// out of date. A run narrowed by --modules, --documents or --rules cannot tell
// and reports none.
func (b *brainAcceptances) stale(ran func(rule string) bool, linted func(module string) bool) []brain.Acceptance {
	var out []brain.Acceptance
	for i, a := range b.all {
		if b.used[i] || !ran(a.Rule) {
			continue
		}
		if !strings.EqualFold(a.Element, brain.ProjectElement) && !linted(strings.SplitN(a.Element, ".", 2)[0]) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// report prints the accepted count (and with show, each one) and the stale
// acceptances, on w — stderr in practice, so --format json/sarif stays clean.
func (b *brainAcceptances) report(w io.Writer, show bool, stale []brain.Acceptance) {
	if n := len(b.accepted); n > 0 {
		fmt.Fprintf(w, "%d finding(s) accepted by decisions in the project brain, not counted above", n)
		if !show {
			fmt.Fprint(w, " (--show-accepted lists them)")
		}
		fmt.Fprintln(w)
		if show {
			sort.SliceStable(b.accepted, func(i, j int) bool { return b.accepted[i].Violation.RuleID < b.accepted[j].Violation.RuleID })
			for _, a := range b.accepted {
				loc := strings.TrimPrefix(a.Violation.Location.Module+"."+strings.TrimPrefix(a.Violation.Location.DocumentName, a.Violation.Location.Module+"."), ".")
				fmt.Fprintf(w, "  accepted (brain %s)  [%s] %s: %s\n", a.By.EntryID, a.Violation.RuleID, loc, a.Violation.Message)
			}
		}
	}
	for _, s := range stale {
		fmt.Fprintf(w, "Stale acceptance (brain %s): %s no longer fires on @%s — remove the line from the decision, or the decision itself\n",
			s.EntryID, s.Rule, s.Element)
	}
}

// knownLintRuleIDs is every rule id lint knows for this project: the built-in
// and bundled rules and the project's own Starlark rules.
func knownLintRuleIDs(projectDir string) map[string]bool {
	ids := map[string]bool{}
	for _, r := range projectLintRules(projectDir, io.Discard) {
		ids[strings.ToUpper(r.ID())] = true
	}
	return ids
}
