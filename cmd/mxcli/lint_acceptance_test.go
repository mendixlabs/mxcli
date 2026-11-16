// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

func viol(rule, module, doc string) linter.Violation {
	return linter.Violation{RuleID: rule, Severity: linter.SeverityWarning, Message: rule + " on " + doc,
		Location: linter.Location{Module: module, DocumentName: doc}}
}

// ako/mxcli#2304: an accepted finding leaves the open findings (and so the
// counts and the exit code), is listed on request with the decision that
// accepts it, and an acceptance no finding matches any more is stale.
func TestBrainAcceptancesApplyAndStale(t *testing.T) {
	b := &brainAcceptances{used: map[int]bool{}, all: []brain.Acceptance{
		{EntryID: "d1", Rule: "SEC001", Element: "Dossier.Beperking"},
		{EntryID: "d2", Rule: "SEC013", Element: "project"},
		{EntryID: "d3", Rule: "SEC001", Element: "Rooster.Dienst"}, // the rule no longer fires here
		{EntryID: "d4", Rule: "CONV001", Element: "Rooster"},       // CONV001 did not run
	}}
	open := b.apply([]linter.Violation{
		viol("SEC001", "Dossier", "Beperking"),
		viol("SEC001", "Dossier", "Other"),
		viol("SEC013", "", "ProjectSecurity"),
		{RuleID: "SEC001", RuleFailure: true, Location: linter.Location{Module: "Dossier", DocumentName: "Beperking"}},
	})
	if len(open) != 2 || open[0].Location.DocumentName != "Other" || !open[1].RuleFailure {
		t.Errorf("open = %+v (a rule failure is never accepted)", open)
	}
	if len(b.accepted) != 2 {
		t.Fatalf("accepted = %+v", b.accepted)
	}
	ran := func(rule string) bool { return rule != "CONV001" }
	stale := b.stale(ran, func(string) bool { return true })
	if len(stale) != 1 || stale[0].EntryID != "d3" {
		t.Errorf("stale = %+v", stale)
	}
	var out bytes.Buffer
	b.report(&out, true, stale)
	for _, want := range []string{"2 finding(s) accepted", "accepted (brain d1)  [SEC001] Dossier.Beperking", "Stale acceptance (brain d3): SEC001 no longer fires on @Rooster.Dienst"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
}

// A reason with a comma is one value: string-slice flags split on commas,
// which turned "demo data only, never deployed" into a second rule id.
func TestBrainVerdictAndAcceptanceFlagsKeepCommas(t *testing.T) {
	for _, f := range []struct {
		cmd  string
		name string
	}{{"capture", "accepts"}, {"capture", "changes"}, {"capture", "partly-meets"}, {"edit", "changes"}, {"edit", "partly-meets"}} {
		c := brainCaptureCmd
		if f.cmd == "edit" {
			c = brainEditCmd
		}
		fl := c.Flags().Lookup(f.name)
		if fl == nil || fl.Value.Type() != "stringArray" {
			t.Errorf("brain %s --%s must be a stringArray (got %v)", f.cmd, f.name, fl)
		}
	}
}
