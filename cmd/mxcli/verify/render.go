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
// findings under it.
func WriteText(w io.Writer, r Result) {
	fmt.Fprintf(w, "verify %s\n", r.Project)
	for _, s := range r.Stages {
		fmt.Fprintf(w, "  %-8s %-8s %s\n", s.Name, strings.ToUpper(s.Status), summary(s))
		for _, f := range s.New {
			fmt.Fprintf(w, "      %s\n", line(f))
		}
		for _, f := range s.Unmasked {
			fmt.Fprintf(w, "      unmasked: %s\n", line(f))
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
}

func summary(s StageResult) string {
	var parts []string
	switch s.Status {
	case StatusSkipped, StatusError:
		return s.Note
	}
	parts = append(parts, fmt.Sprintf("%d new", len(s.New)))
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
