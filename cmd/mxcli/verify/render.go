// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// WriteJSON writes the result as one JSON document.
func WriteJSON(w io.Writer, r Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteText writes a short human summary: one line per stage, then the new
// findings under it that would fail the gate at failOn ("error" or "warning").
// Findings below that threshold are counted by severity, not listed: on a project
// whose baseline predates most of its model they are hundreds of lines that bury
// the one that matters, and an agent learns to skip a gate that always prints
// them. The JSON keeps every finding.
func WriteText(w io.Writer, r Result, failOn string) {
	opts := Options{FailOn: failOn}
	fmt.Fprintf(w, "verify %s\n", r.Project)
	for _, n := range r.Notes {
		fmt.Fprintf(w, "  note: %s\n", n)
	}
	below := 0
	var quietStages []string
	for _, s := range r.Stages {
		if s.Status == StatusSkipped && s.Note == notSelectedNote {
			continue // the JSON keeps it; a text line per stage nobody asked for is noise
		}
		listed, quiet := splitByThreshold(s.New, opts)
		fmt.Fprintf(w, "  %-8s %-8s %s\n", s.Name, strings.ToUpper(s.Status), summary(s, len(quiet), countBySeverity(quiet)))
		for _, f := range listed {
			fmt.Fprintf(w, "      %s\n", line(f))
		}
		for _, f := range s.Unmasked {
			fmt.Fprintf(w, "      unmasked: %s\n", line(f))
		}
		below += len(quiet)
		if len(quiet) > 0 {
			quietStages = append(quietStages, s.Name)
		}
	}
	switch {
	case r.Exit == ExitOK:
		fmt.Fprintln(w, "Result: passed")
	case r.Exit == ExitCouldNotRun:
		fmt.Fprintf(w, "Result: could not run (exit %d)\n", r.Exit)
	default:
		fmt.Fprintf(w, "Result: FAILED (exit %d)\n", r.Exit)
	}
	if below > 0 {
		fmt.Fprintf(w, "%d new finding(s) are below --fail-on=%s and do not fail the gate. Review them (--format json lists\n", below, opts.failOn())
		// Scoped to the stages that have them: --update-baseline records every
		// selected stage, and accepting lint's infos must not also accept mx check errors.
		fmt.Fprintf(w, "every finding); `mxcli verify -p %s --stages %s --update-baseline` accepts them.\n", r.Project, strings.Join(quietStages, ","))
	}
}

// splitByThreshold separates the findings that fail the gate at opts.FailOn from
// those below it.
func splitByThreshold(fs []Finding, opts Options) (listed, quiet []Finding) {
	for _, f := range fs {
		if severityRank(f.Severity) >= severityRank(opts.failOn()) {
			listed = append(listed, f)
		} else {
			quiet = append(quiet, f)
		}
	}
	return
}

func countBySeverity(fs []Finding) string {
	n := map[string]int{}
	for _, f := range fs {
		n[f.Severity]++
	}
	var parts []string
	for _, sev := range []string{"warning", "info"} {
		if n[sev] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n[sev], sev))
		}
	}
	return strings.Join(parts, ", ")
}

func summary(s StageResult, quiet int, quietDetail string) string {
	var parts []string
	switch s.Status {
	case StatusSkipped, StatusError:
		return s.Note
	}
	if quiet > 0 {
		parts = append(parts, fmt.Sprintf("%d new (%d below the failure threshold: %s)", len(s.New), quiet, quietDetail))
	} else {
		parts = append(parts, fmt.Sprintf("%d new", len(s.New)))
	}
	if len(s.Unmasked) > 0 {
		parts = append(parts, fmt.Sprintf("%d unmasked (an earlier error in the baseline hid them)", len(s.Unmasked)))
	}
	parts = append(parts, fmt.Sprintf("%d pre-existing", s.PreExisting))
	if s.Resolved > 0 {
		parts = append(parts, fmt.Sprintf("%d resolved", s.Resolved))
	}
	if s.Hidden > 0 {
		parts = append(parts, fmt.Sprintf("%d hidden by a new earlier-stage error, not resolved", s.Hidden))
	}
	out := strings.Join(parts, ", ")
	if s.Recorded {
		out += " (baseline recorded)"
	}
	if s.Note != "" {
		out += " — " + s.Note
	}
	return out
}

func line(f Finding) string {
	loc := f.Document
	if f.Element != "" {
		if loc != "" {
			loc += " / "
		}
		loc += f.Element
	}
	if loc != "" {
		loc += ": "
	}
	return fmt.Sprintf("[%s] %s %s%s", f.Severity, f.Code, loc, f.Message)
}
