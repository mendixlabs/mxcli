// SPDX-License-Identifier: Apache-2.0

// Package verify is the gate behind `mxcli verify` (docs/11-proposals/
// PROPOSAL_verify_gate.md): it runs the check, lint and mx check stages in cost
// order and reports what a change made worse, not what the project already had.
//
// The stages themselves are injected (see Stage), so the orchestration, the
// baseline comparison and the exit codes are testable without a project or mx.
package verify

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Stage names, in pipeline order.
const (
	StageCheck   = "check"
	StageExec    = "exec" // applies the scripts; runs only with Options.Apply
	StageLint    = "lint"
	StageMxCheck = "mxcheck"
)

// AllStages is the pipeline order the stages run in.
var AllStages = []string{StageCheck, StageExec, StageLint, StageMxCheck}

// Through returns the stages up to and including last, in pipeline order: the
// caller's answer to "how far should this run?".
func Through(last string) ([]string, error) {
	for i, s := range AllStages {
		if s == last {
			return append([]string{}, AllStages[:i+1]...), nil
		}
	}
	return nil, fmt.Errorf("unknown stage %q (stages: %s)", last, strings.Join(AllStages, ", "))
}

// Statuses of a stage.
const (
	StatusPassed  = "passed"
	StatusFailed  = "failed"  // the stage ran and the change made something worse
	StatusError   = "error"   // the stage could not run (no mx, no runtime): never "passed"
	StatusSkipped = "skipped" // not selected, nothing to check, or an earlier stage failed with --fail-fast
)

// Exit codes. A failing gate sets ExitFailedBase plus one bit per failed stage, so
// several failed stages are one code and "failed" can never be read as "could not
// run" (ExitCouldNotRun) or as a usage error.
const (
	ExitOK          = 0
	ExitUsage       = 1
	ExitCouldNotRun = 2
	ExitFailedBase  = 16
)

var stageBit = map[string]int{StageCheck: 1, StageLint: 2, StageMxCheck: 4, StageExec: 8}

// Finding is one problem a stage reports. Key is the identity a baseline matches
// on; it must not contain the message or any name that renaming changes.
type Finding struct {
	Stage      string `json:"stage"`
	Code       string `json:"code"`
	Severity   string `json:"severity"` // "error", "warning", "info"
	Document   string `json:"document,omitempty"`
	Element    string `json:"element,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
	Key        string `json:"key"`
	// MxStage is MxBuild's report stage (0..3) for an mxcheck finding, else -1.
	MxStage int `json:"mxStage"`
	// Unit scopes what an mxcheck finding can hide: stages 1 and 2 hide the later
	// ones in the same unit (microflow).
	Unit string `json:"unit,omitempty"`
}

// StageResult is one stage's outcome.
type StageResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMs int64  `json:"durationMs"`
	// Note says why a stage was skipped or could not run, or that it had no
	// baseline to compare with.
	Note string `json:"note,omitempty"`
	// New are the findings the change made worse, the only ones that fail the gate.
	New []Finding `json:"new"`
	// Unmasked are findings that appeared only because the baseline had an
	// earlier-stage error hiding them (mxcheck). They are reported and fail the
	// gate only with Options.Strict.
	Unmasked []Finding `json:"unmasked,omitempty"`
	// Resolved and Hidden are counts: the stages' baselines record keys, not
	// whole findings, and a count is the honest type for lint.
	Resolved    int `json:"resolved"`
	Hidden      int `json:"hidden,omitempty"`
	PreExisting int `json:"preExisting"`
	// PreExistingBySeverity says what the pre-existing findings are: a count alone
	// ("63 pre-existing") reads the same whether they are infos or errors the
	// baseline accepted. Empty when the stage cannot say.
	PreExistingBySeverity map[string]int `json:"preExistingBySeverity,omitempty"`
	// Recorded is true when --update-baseline recorded this stage's baseline.
	Recorded bool `json:"recorded,omitempty"`
}

// Result is the whole run.
type Result struct {
	Project string `json:"project"`
	// Applied is true when the exec stage ran and applied the scripts. Without
	// it, lint and mxcheck describe the project as it was before any script.
	Applied bool          `json:"applied"`
	Stages  []StageResult `json:"stages"`
	// Notes are things a reader of the whole result must not miss (scripts that
	// were checked but not applied).
	Notes []string `json:"notes,omitempty"`
	Exit  int      `json:"exit"`
}

// Outcome is what a Stage returns, before the orchestrator decides pass or fail.
type Outcome struct {
	// New are the findings not in the baseline (all of them without one).
	New         []Finding
	Unmasked    []Finding
	Resolved    int
	Hidden      int
	PreExisting int
	// PreExistingBy is PreExisting by severity ("error", "warning", "info").
	PreExistingBy map[string]int
	Recorded      bool
	Note          string
	// Incomplete says the stage ran but cannot vouch for its result: rules the
	// baseline expects did not run, or the configuration differs from the one it
	// was recorded under. A stage with new failing findings still fails; one
	// without them is an error, never a pass, because "nothing new" would mean
	// "nothing was checked".
	Incomplete string
	// Applied is set by the exec stage when it applied the scripts.
	Applied bool
}

// notSelectedNote is the note of a stage the caller did not select.
const notSelectedNote = "not selected"

// Stage is one runnable stage.
type Stage func() (Outcome, error)

// Options steer the orchestrator.
type Options struct {
	Project  string
	Stages   []string // selected, in any order; run in AllStages order
	FailFast bool
	// FailOn is the lowest severity of a new finding that fails the gate:
	// "error" (default) or "warning".
	FailOn string
	// Strict makes unmasked findings fail the gate too.
	Strict bool
	// Skip lists stages that have nothing to do, with the reason (e.g. check
	// without scripts); they report skipped, never passed.
	Skip map[string]string
	// Apply says the scripts are applied (the exec stage runs). It also makes
	// the stages depend on each other: a change that failed check is not
	// written, and lint and mxcheck of a change that was not applied would
	// describe the project without it, so they are skipped.
	Apply bool
	// ScriptsGiven says scripts were passed. Without Apply they are checked and
	// not applied, which the result says in Notes.
	ScriptsGiven bool
}

// ScriptsNotAppliedNote is what a run that checked scripts without applying
// them says: lint and mxcheck judge the project as it is.
const ScriptsNotAppliedNote = "scripts were checked but not applied: lint and mxcheck describe the project without them (pass --apply to apply them first)"

// Run executes the selected stages in cost order and returns the result.
// stages maps a stage name to its runner.
func Run(opts Options, stages map[string]Stage) Result {
	selected := map[string]bool{}
	for _, s := range opts.Stages {
		selected[s] = true
	}
	res := Result{Project: opts.Project}
	if opts.ScriptsGiven && !opts.Apply {
		res.Notes = append(res.Notes, ScriptsNotAppliedNote)
	}
	failed, errored, stop := false, false, false
	blocked := "" // why the stages after a failed check or exec do not run (--apply only)
	mask := 0
	for _, name := range AllStages {
		sr := StageResult{Name: name, New: []Finding{}}
		switch {
		case !selected[name]:
			sr.Status, sr.Note = StatusSkipped, notSelectedNote
		case opts.Skip[name] != "":
			sr.Status, sr.Note = StatusSkipped, opts.Skip[name]
		case blocked != "":
			sr.Status, sr.Note = StatusSkipped, blocked
		case stop:
			sr.Status, sr.Note = StatusSkipped, "an earlier stage failed (--fail-fast)"
		default:
			t0 := time.Now()
			out, err := stages[name]()
			sr.DurationMs = time.Since(t0).Milliseconds()
			if err != nil {
				sr.Status, sr.Note = StatusError, err.Error()
				errored = true
				stop = opts.FailFast
				blocked = blockAfter(name, opts)
				break
			}
			sr.New = append(sr.New, out.New...)
			sr.Unmasked, sr.Resolved, sr.Hidden = out.Unmasked, out.Resolved, out.Hidden
			sr.PreExisting, sr.Recorded, sr.Note = out.PreExisting, out.Recorded, out.Note
			sr.PreExistingBySeverity = out.PreExistingBy
			switch {
			case fails(sr, opts):
				sr.Status = StatusFailed
				if out.Incomplete != "" {
					sr.Note = joinNote(sr.Note, out.Incomplete)
				}
				failed = true
				mask |= stageBit[name]
				stop = opts.FailFast
				blocked = blockAfter(name, opts)
			case out.Incomplete != "":
				sr.Status, sr.Note = StatusError, joinNote(sr.Note, out.Incomplete)
				errored = true
				stop = opts.FailFast
			default:
				sr.Status = StatusPassed
				if name == StageExec && out.Applied {
					res.Applied = true
				}
			}
		}
		res.Stages = append(res.Stages, sr)
	}
	switch {
	case errored:
		res.Exit = ExitCouldNotRun
	case failed:
		res.Exit = ExitFailedBase | mask
	default:
		res.Exit = ExitOK
	}
	// A stage that failed while another could not run is still a failure the
	// caller must see; the failure code wins because it is actionable.
	if errored && failed {
		res.Exit = ExitFailedBase | mask
	}
	return res
}

func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// blockAfter says why nothing after a failed stage runs, or "" when the stages
// stay independent. Only a run that applies scripts has dependencies: a failed
// check writes nothing, and a change that was not applied is not what lint and
// mxcheck would judge.
func blockAfter(failed string, opts Options) string {
	if !opts.Apply {
		return ""
	}
	switch failed {
	case StageCheck:
		return "check failed; nothing was written"
	case StageExec:
		return "exec did not apply the scripts; lint and mxcheck would describe the project without them"
	}
	return ""
}

func fails(sr StageResult, opts Options) bool {
	for _, f := range sr.New {
		if severityRank(f.Severity) >= severityRank(opts.failOn()) {
			return true
		}
	}
	return opts.Strict && len(sr.Unmasked) > 0
}

func (o Options) failOn() string {
	if o.FailOn == "warning" {
		return "warning"
	}
	return "error"
}

func severityRank(s string) int {
	switch s {
	case "error":
		return 2
	case "warning":
		return 1
	}
	return 0
}

// ParseStages validates a --stages list.
func ParseStages(list []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, s := range list {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || seen[s] {
			continue
		}
		if _, ok := stageBit[s]; !ok {
			return nil, fmt.Errorf("unknown stage %q (stages: %s)", s, strings.Join(AllStages, ", "))
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no stages selected")
	}
	return out, nil
}

// SortFindings orders findings deterministically (document, code, element, key).
func SortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Document != b.Document {
			return a.Document < b.Document
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Element != b.Element {
			return a.Element < b.Element
		}
		return a.Key < b.Key
	})
}
