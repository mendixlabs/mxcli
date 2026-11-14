// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// recordSchema is bumped when a field changes meaning, so a chart reading a
// year of records can tell which ones it understands.
const recordSchema = 1

// record is one benchmark run: what the nightly job commits to the
// `benchmarks` branch, one file per run.
type record struct {
	Schema         int             `json:"schema"`
	Date           string          `json:"date"`
	Commit         string          `json:"commit,omitempty"`
	Candidate      string          `json:"candidate"`
	Baseline       string          `json:"baseline,omitempty"`
	Runner         runner          `json:"runner"`
	Params         params          `json:"params"`
	WeightsVersion int             `json:"weights_version"`
	Sessions       []sessionResult `json:"sessions"`
}

type runner struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
	CPUs int    `json:"cpus"`
	CPU  string `json:"cpu,omitempty"`
}

type params struct {
	Base            string  `json:"base"`
	Reps            int     `json:"reps"`
	SessionEntities int     `json:"session_entities"`
	LargeModules    int     `json:"large_modules"`
	LargeEntities   int     `json:"large_entities"`
	LargeDocuments  int     `json:"large_documents"`
	ScaleMicroflows int     `json:"scale_microflows,omitempty"`
	ScaleBatch      int     `json:"scale_batch,omitempty"`
	RuntimeVersion  string  `json:"runtime_version,omitempty"`
	Threshold       float64 `json:"threshold"`
}

func newRecord(cfg config, sessions []sessionResult) record {
	rec := record{
		Schema:    recordSchema,
		Date:      time.Now().UTC().Format(time.RFC3339),
		Commit:    os.Getenv("GITHUB_SHA"),
		Candidate: binaryVersion(cfg.candidate),
		Runner:    runner{OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), CPU: cpuModel()},
		Params: params{
			Base: filepath.Base(cfg.base), Reps: cfg.reps, SessionEntities: cfg.sessionEntities,
			LargeModules: cfg.large.Modules, LargeEntities: cfg.large.Entities,
			LargeDocuments: cfg.large.Documents(), Threshold: cfg.threshold,
			RuntimeVersion:  cfg.runtimeVersion,
			ScaleMicroflows: cfg.scaleMicroflows, ScaleBatch: cfg.scaleBatch,
		},
		WeightsVersion: weightsVersion,
		Sessions:       sessions,
	}
	if cfg.baseline != "" {
		rec.Baseline = binaryVersion(cfg.baseline)
	}
	return rec
}

func (r record) write(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// absBinary resolves a binary path and confirms there is something to run.
func absBinary(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("no mxcli at %s: %w", path, err)
	}
	return abs, nil
}

// binaryVersion is what `mxcli version` prints, the line that names the build.
func binaryVersion(bin string) string {
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "version") {
			return strings.TrimSpace(line)
		}
	}
	return strings.TrimSpace(string(out))
}

// cpuModel is the processor's name where the platform exposes it cheaply. It
// is in the record so a step change that follows a runner-fleet change can be
// told from one that follows a commit.
func cpuModel() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if name, ok := strings.CutPrefix(line, "model name"); ok {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), ":"))
		}
	}
	return ""
}

// markdown is the run as a table per session: what a person reads in the job
// summary or the terminal.
func (r record) markdown() string {
	var sb strings.Builder
	sb.WriteString("## Agent dev-loop benchmark\n\n")
	fmt.Fprintf(&sb, "- candidate: `%s`\n", r.Candidate)
	if r.Baseline != "" {
		fmt.Fprintf(&sb, "- baseline: `%s`\n", r.Baseline)
	}
	fmt.Fprintf(&sb, "- %d CPUs", r.Runner.CPUs)
	if r.Runner.CPU != "" {
		fmt.Fprintf(&sb, " (%s)", r.Runner.CPU)
	}
	sb.WriteString("\n")
	ab := r.Baseline != ""

	for _, s := range r.Sessions {
		if s.Size == "runtime" {
			fmt.Fprintf(&sb, "\n### running app, Mendix %s — %d units, median of %d\n\n", r.Params.RuntimeVersion, s.Units, s.Reps)
		} else {
			fmt.Fprintf(&sb, "\n### %s project — %d units, median of %d\n\n", s.Size, s.Units, s.Reps)
		}
		for _, seed := range s.Seed {
			fmt.Fprintf(&sb, "- %s: %s (once, candidate only)\n", seed.Name, ms(seed.WallMS))
		}
		if len(s.Seed) > 0 {
			sb.WriteString("\n")
		}
		if ab {
			sb.WriteString("| step | new | old | wall × | CPU × | reads new / old | files changed new / old | |\n")
			sb.WriteString("|---|---:|---:|---:|---:|---:|---:|---|\n")
		} else {
			sb.WriteString("| step | wall | spread | CPU | RSS | reads | files changed | |\n")
			sb.WriteString("|---|---:|---:|---:|---:|---:|---:|---|\n")
		}
		for _, st := range s.Steps {
			c := st.Candidate
			flags := strings.Join(st.Flags, ", ")
			switch {
			case ab && st.Baseline != nil:
				fmt.Fprintf(&sb, "| %s | %s | %s | %.2f | %.2f | %s / %s | %d / %d | %s |\n",
					st.Name, ms(c.Wall.Median), ms(st.Baseline.Wall.Median), st.WallRatio, st.CPURatio,
					reads(c.Counts), reads(st.Baseline.Counts), c.FilesChanged, st.Baseline.FilesChanged, flags)
			case ab:
				fmt.Fprintf(&sb, "| %s | %s | — | — | — | %s / — | %d / — | %s |\n",
					st.Name, ms(c.Wall.Median), reads(c.Counts), c.FilesChanged, flags)
			default:
				fmt.Fprintf(&sb, "| %s | %s | %s – %s | %s | %.0f MB | %s | %d | %s |\n",
					st.Name, ms(c.Wall.Median), ms(c.Wall.Min), ms(c.Wall.Max), ms(c.CPU.Median), c.RSS.Median,
					reads(c.Counts), c.FilesChanged, flags)
			}
		}
		fmt.Fprintf(&sb, "\nSession-weighted total: **%s**", ms(s.Weighted.CandidateMS))
		if s.Weighted.Ratio > 0 {
			fmt.Fprintf(&sb, " against %s — **× %.2f**", ms(s.Weighted.BaselineMS), s.Weighted.Ratio)
		}
		sb.WriteString("\n")
		if s.BaselineError != "" {
			fmt.Fprintf(&sb, "\nThe baseline stopped, so later steps have no A/B: `%s`\n", firstLine(s.BaselineError))
		}
	}
	sb.WriteString(r.scaling())
	sb.WriteString(r.perStatement())
	return sb.String()
}

// perStatement is the scale session's batches as an agent meets them: the time
// one statement takes, and how much of it there is for every 1,000 units of
// the project. A cost that follows the project shows in the last column; one
// that does not stays near zero whatever the project holds.
func (r record) perStatement() string {
	var scale *sessionResult
	for i := range r.Sessions {
		if r.Sessions[i].Size == "scale" {
			scale = &r.Sessions[i]
		}
	}
	if scale == nil || scale.Units == 0 {
		return ""
	}
	ab := r.Baseline != ""
	thousands := float64(scale.Units) / 1000
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n### Per statement — %d units\n\n", scale.Units)
	if ab {
		sb.WriteString("| step | statements | new, per statement | old, per statement | new, per statement per 1,000 units |\n|---|---:|---:|---:|---:|\n")
	} else {
		sb.WriteString("| step | statements | per statement | per statement per 1,000 units |\n|---|---:|---:|---:|\n")
	}
	for _, st := range scale.Steps {
		if st.Statements == 0 || st.Candidate == nil {
			continue
		}
		each := st.Candidate.Wall.Median / float64(st.Statements)
		switch {
		case ab && st.Baseline != nil:
			fmt.Fprintf(&sb, "| %s | %d | %s | %s | %s |\n", st.Name, st.Statements, msFine(each),
				msFine(st.Baseline.Wall.Median/float64(st.Statements)), msFine(each/thousands))
		case ab:
			fmt.Fprintf(&sb, "| %s | %d | %s | — | %s |\n", st.Name, st.Statements, msFine(each), msFine(each/thousands))
		default:
			fmt.Fprintf(&sb, "| %s | %d | %s | %s |\n", st.Name, st.Statements, msFine(each), msFine(each/thousands))
		}
	}
	return sb.String()
}

// scaling is the same session's steps at the two sizes, side by side: what
// each command costs more on a larger project. The session's own scripts are
// identical at both sizes, so the multiple is the project's doing.
func (r record) scaling() string {
	var small, large *sessionResult
	for i := range r.Sessions {
		switch r.Sessions[i].Size {
		case "small":
			small = &r.Sessions[i]
		case "large":
			large = &r.Sessions[i]
		}
	}
	if small == nil || large == nil || small.Units == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n### Scaling — the large project has × %.1f the units\n\n", float64(large.Units)/float64(small.Units))
	sb.WriteString("| step | small | large | × |\n|---|---:|---:|---:|\n")
	for i, st := range small.Steps {
		if i >= len(large.Steps) || st.Candidate.Wall.Median == 0 {
			continue
		}
		l := large.Steps[i].Candidate.Wall.Median
		fmt.Fprintf(&sb, "| %s | %s | %s | %.1f |\n", st.Name, ms(st.Candidate.Wall.Median), ms(l), l/st.Candidate.Wall.Median)
	}
	return sb.String()
}

func ms(v float64) string {
	if v >= 10000 {
		return fmt.Sprintf("%.1f s", v/1000)
	}
	if v >= 1000 {
		return fmt.Sprintf("%.2f s", v/1000)
	}
	return fmt.Sprintf("%.0f ms", v)
}

// msFine is ms for the per-statement figures, which are often a few
// milliseconds: below ten it keeps one decimal.
func msFine(v float64) string {
	if v < 10 {
		return fmt.Sprintf("%.1f ms", v)
	}
	return ms(v)
}

func reads(c *counts) string {
	if c == nil {
		return "n/a"
	}
	if c.UnitStoreHits == nil {
		return fmt.Sprint(c.UnitFileReads)
	}
	// Every read not served from memory: files, and the persisted store.
	return fmt.Sprintf("%d + %d", c.UnitFileReads, *c.UnitStoreHits)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
