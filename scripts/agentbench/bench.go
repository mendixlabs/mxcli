// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/scripts/agentbench/gen"
)

// statsFileEnv is executor.StatsFileEnv, repeated so the driver does not link
// the code it measures.
const statsFileEnv = "MXCLI_STATS_FILE"

// A step is one mxcli process, started as an agent starts it. The steps of a
// session run in order against one project, each on the state the previous
// ones left.
type step struct {
	name string
	// args is the command line after the binary; {P} is the project's .mpr and
	// {S} the directory holding the session's scripts.
	args []string
	// weight is how many times a real session runs this kind of command for
	// each time the benchmark does; see weights below.
	weight int
	// noWrites marks a step that must change no unit file: the idempotence
	// canary. A candidate that writes there has a defect, not a slowdown.
	noWrites bool
}

// weightsVersion identifies the weight column. A record carries it so a change
// of weights is never read as a change of speed.
const weightsVersion = 1

// The session. Weights are per-session call counts rounded from the builds
// measured in docs/11-proposals/PROPOSAL_agent_loop_efficiency.md (`exec` 85 to
// 201 calls, `-c` 111 to 120, `check` 63 to 90 per session): a one-document
// exec, a check and a read each stand for many, the bulk steps for themselves.
func sessionSteps() []step {
	const read = "describe entity Bench.Item02; describe microflow Bench.ACT_Item02_Save; describe page Bench.Item02_Overview"
	return []step{
		// The first command on a project builds its unit index; the second is
		// what every later command pays to start and open.
		{name: "open-cold", args: []string{"-p", "{P}", "-c", "show modules"}, weight: 1},
		{name: "open", args: []string{"-p", "{P}", "-c", "show modules"}, weight: 60},
		{name: "domain", args: []string{"exec", "{S}/domain.mdl", "-p", "{P}"}, weight: 1},
		{name: "microflows", args: []string{"exec", "{S}/microflows.mdl", "-p", "{P}"}, weight: 1},
		{name: "pages", args: []string{"exec", "{S}/pages.mdl", "-p", "{P}"}, weight: 1},
		{name: "describe", args: []string{"-p", "{P}", "-c", read}, weight: 60},
		{name: "catalog", args: []string{"-p", "{P}", "-c", "refresh catalog"}, weight: 5},
		{name: "catalog-full", args: []string{"-p", "{P}", "-c", "refresh catalog full"}, weight: 1},
		{name: "query", args: []string{"-p", "{P}", "-c", "select count(*) from CATALOG.ENTITIES"}, weight: 20},
		{name: "check", args: []string{"check", "{S}/single-edit.mdl", "-p", "{P}", "--references"}, weight: 90},
		{name: "single-edit", args: []string{"exec", "{S}/single-edit.mdl", "-p", "{P}"}, weight: 150},
		{name: "catalog-after-edit", args: []string{"-p", "{P}", "-c", "refresh catalog full"}, weight: 10},
		{name: "bulk-alter", args: []string{"exec", "{S}/bulk-alter.mdl", "-p", "{P}"}, weight: 1},
		{name: "re-exec", args: []string{"exec", "{S}/bulk-alter.mdl", "-p", "{P}"}, weight: 1, noWrites: true},
		{name: "rename", args: []string{"-p", "{P}", "-c", "rename entity Bench.Item01 to Item01Renamed"}, weight: 1},
	}
}

// writeSession writes the session's scripts for an app of the given size.
func writeSession(dir string, entities int) error {
	p := gen.Params{Prefix: "Bench", Modules: 1, Entities: entities}
	for name, src := range map[string]string{
		"domain.mdl":      gen.Domain(p),
		"microflows.mdl":  gen.Microflows(p),
		"pages.mdl":       gen.Pages(p),
		"single-edit.mdl": gen.SingleEdit(p),
		"bulk-alter.mdl":  gen.BulkAlter(p),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// A sample is one measured process.
type sample struct {
	WallMS float64 `json:"wall_ms"`
	// CPUMS is user plus system time of the process. It is less sensitive to a
	// busy runner than wall time, and blind to waiting on the disk.
	CPUMS  float64 `json:"cpu_ms"`
	RSSMB  float64 `json:"rss_mb"`
	Counts *counts `json:"counts,omitempty"`
	// FilesChanged is how many unit files the command added, rewrote or
	// removed, measured from outside by comparing the project before and
	// after. Unlike Counts it needs nothing from the binary, so it exists for
	// a baseline too, and it sees a created document, which mxcli's own write
	// count (reconciled rewrites of stored units) does not.
	FilesChanged int `json:"files_changed"`
	// StdoutBytes is how much the command printed: what an agent then pays to
	// read.
	StdoutBytes int `json:"stdout_bytes"`
}

// counts are a process's machine-independent figures, summed over the
// connections it and its children closed. nil when the binary does not report
// them (a baseline older than the stats file).
type counts struct {
	UnitFileReads int64 `json:"unit_file_reads"`
	WritesOffered int   `json:"writes_offered"`
	WritesWritten int   `json:"writes_written"`
}

// runStep runs one process and measures it. A non-zero exit is an error
// carrying the tail of the output: a failed command's time is not a
// measurement.
func runStep(bin string, args []string, project, scripts, scratch string) (sample, error) {
	resolved := make([]string, len(args))
	for i, a := range args {
		a = strings.ReplaceAll(a, "{P}", project)
		resolved[i] = strings.ReplaceAll(a, "{S}", scripts)
	}
	statsPath := filepath.Join(scratch, "stats.jsonl")
	_ = os.Remove(statsPath)

	before := unitDigests(filepath.Dir(project))
	cmd := exec.Command(bin, resolved...)
	cmd.Env = append(os.Environ(), statsFileEnv+"="+statsPath, "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	err := cmd.Run()
	wall := time.Since(start)
	if err != nil {
		return sample{}, fmt.Errorf("%s %s: %w\n%s", filepath.Base(bin), strings.Join(resolved, " "), err, tail(stdout.String()+stderr.String(), 20))
	}
	s := sample{
		WallMS:       float64(wall.Microseconds()) / 1000,
		CPUMS:        float64((cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()).Microseconds()) / 1000,
		StdoutBytes:  stdout.Len(),
		Counts:       readCounts(statsPath),
		FilesChanged: changedFiles(before, unitDigests(filepath.Dir(project))),
	}
	s.RSSMB = maxRSSMB(cmd.ProcessState)
	return s, nil
}

func readCounts(path string) *counts {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var total counts
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c counts
		if json.Unmarshal(sc.Bytes(), &c) == nil {
			total.UnitFileReads += c.UnitFileReads
			total.WritesOffered += c.WritesOffered
			total.WritesWritten += c.WritesWritten
		}
	}
	return &total
}

func tail(s string, lines int) string {
	all := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

// copyProject copies a project directory, leaving out .mxcli — the unit index
// and catalog a previous command cached there. Both arms must start from the
// same bytes and build their own caches, or the arm whose binary wrote the
// snapshot starts warm.
func copyProject(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".mxcli" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(filepath.Join(dst, rel))
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// findMPR returns the one .mpr in dir.
func findMPR(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.mpr"))
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("%s: want exactly one .mpr, found %d", dir, len(matches))
	}
	return filepath.Base(matches[0]), nil
}

// countUnits is how many unit files a v2 project holds: the size a record
// reports for the project a session ran against.
func countUnits(dir string) int {
	n := 0
	_ = filepath.WalkDir(filepath.Join(dir, "mprcontents"), func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			n++
		}
		return nil
	})
	return n
}

// unitDigests maps every unit file of a v2 project to a digest of its bytes.
func unitDigests(dir string) map[string][sha256.Size]byte {
	out := map[string][sha256.Size]byte{}
	_ = filepath.WalkDir(filepath.Join(dir, "mprcontents"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return nil
		}
		if data, err := os.ReadFile(path); err == nil {
			out[path] = sha256.Sum256(data)
		}
		return nil
	})
	return out
}

// changedFiles counts the files added, rewritten with different bytes, or
// removed between two unitDigests.
func changedFiles(before, after map[string][sha256.Size]byte) int {
	n := 0
	for path, digest := range after {
		if old, ok := before[path]; !ok || old != digest {
			n++
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			n++
		}
	}
	return n
}

// dist summarises one step's samples in one arm.
type dist struct {
	Median  float64   `json:"median"`
	Min     float64   `json:"min"`
	Max     float64   `json:"max"`
	Samples []float64 `json:"samples"`
}

func newDist(v []float64) dist {
	d := dist{Samples: v, Median: median(v)}
	if len(v) > 0 {
		d.Min, d.Max = v[0], v[0]
		for _, x := range v {
			d.Min, d.Max = min(d.Min, x), max(d.Max, x)
		}
	}
	return d
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if n := len(s); n%2 == 1 {
		return s[n/2]
	} else {
		return (s[n/2-1] + s[n/2]) / 2
	}
}

// pairedRatio is the median of candidate/baseline over the repetitions both
// arms completed. Each pair ran back to back on the same machine, so a slow
// minute on the runner slows both and cancels; the ratio of two medians taken
// minutes apart would not have that property.
func pairedRatio(candidate, baseline []float64) float64 {
	n := min(len(candidate), len(baseline))
	ratios := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		if baseline[i] > 0 {
			ratios = append(ratios, candidate[i]/baseline[i])
		}
	}
	return median(ratios)
}

// armResult is one arm's measurements of one step.
type armResult struct {
	Wall dist `json:"wall_ms"`
	CPU  dist `json:"cpu_ms"`
	RSS  dist `json:"rss_mb"`
	// Counts are the first repetition's. CountsStable is false when a later
	// repetition disagreed — which a deterministic session should never show.
	Counts       *counts `json:"counts,omitempty"`
	FilesChanged int     `json:"files_changed"`
	CountsStable bool    `json:"counts_stable"`
	StdoutBytes  int     `json:"stdout_bytes"`
}

func newArmResult(samples []sample) *armResult {
	if len(samples) == 0 {
		return nil
	}
	var wall, cpu, rss []float64
	r := &armResult{
		Counts: samples[0].Counts, FilesChanged: samples[0].FilesChanged,
		CountsStable: true, StdoutBytes: samples[0].StdoutBytes,
	}
	for _, s := range samples {
		wall, cpu, rss = append(wall, s.WallMS), append(cpu, s.CPUMS), append(rss, s.RSSMB)
		if s.FilesChanged != r.FilesChanged ||
			(s.Counts == nil) != (r.Counts == nil) || (s.Counts != nil && *s.Counts != *r.Counts) {
			r.CountsStable = false
		}
	}
	r.Wall, r.CPU, r.RSS = newDist(wall), newDist(cpu), newDist(rss)
	return r
}

// stepResult is one step of one session, both arms.
type stepResult struct {
	Name      string     `json:"name"`
	Weight    int        `json:"weight"`
	Candidate *armResult `json:"candidate"`
	Baseline  *armResult `json:"baseline,omitempty"`
	// WallRatio and CPURatio are candidate/baseline; 0 when there is no
	// baseline measurement of this step.
	WallRatio float64 `json:"wall_ratio,omitempty"`
	CPURatio  float64 `json:"cpu_ratio,omitempty"`
	// Flags are what a reader should look at: "slower", "faster",
	// "counts-changed", "counts-unstable", "wrote-on-re-exec".
	Flags []string `json:"flags,omitempty"`
}

// sessionResult is the scripted session at one project size.
type sessionResult struct {
	Size string `json:"size"`
	// Units is how many unit files the project held when the session started.
	Units int `json:"units"`
	// Seed is the one-off build of the large fixture, candidate only: it is
	// run once, so it is a trend figure, not an A/B.
	Seed  []seedResult `json:"seed,omitempty"`
	Steps []stepResult `json:"steps"`
	// BaselineError is why the baseline arm stopped, when it did. Steps from
	// that one on have no ratio: the baseline's project no longer holds what
	// the candidate's does.
	BaselineError string `json:"baseline_error,omitempty"`
	// Weighted is the session's steps multiplied by their weights, in
	// milliseconds of median wall time: one figure for "what a session costs".
	Weighted weighted `json:"weighted"`
}

type seedResult struct {
	Name string `json:"name"`
	sample
}

type weighted struct {
	CandidateMS float64 `json:"candidate_ms"`
	BaselineMS  float64 `json:"baseline_ms,omitempty"`
	Ratio       float64 `json:"ratio,omitempty"`
}

// config is one benchmark run.
type config struct {
	candidate, baseline string
	base                string // the starting project's directory
	work                string
	reps                int
	sessionEntities     int
	large               gen.Params
	sizes               []string
	threshold           float64
	log                 io.Writer
}

// errInvariant is returned, after the record is complete, when the candidate
// broke something a benchmark run can state exactly rather than estimate.
var errInvariant = errors.New("the candidate broke an invariant")

// runSession measures the session at one size. snapshot is the project every
// repetition of both arms starts from.
func runSession(cfg config, size, snapshot, scripts string) (sessionResult, error) {
	mpr, err := findMPR(snapshot)
	if err != nil {
		return sessionResult{}, err
	}
	steps := sessionSteps()
	res := sessionResult{Size: size, Units: countUnits(snapshot)}
	candidate := make([][]sample, len(steps))
	baseline := make([][]sample, len(steps))
	baselineLive := cfg.baseline != ""

	for rep := 0; rep < cfg.reps; rep++ {
		dirs := map[string]string{}
		arms := []string{"candidate"}
		if baselineLive {
			arms = append(arms, "baseline")
			// Alternate which arm goes first, so neither always runs on the
			// page cache the other warmed.
			if rep%2 == 1 {
				arms[0], arms[1] = arms[1], arms[0]
			}
		}
		for _, arm := range arms {
			dir := filepath.Join(cfg.work, fmt.Sprintf("%s-%s-%d", size, arm, rep))
			if err := copyProject(snapshot, dir); err != nil {
				return res, err
			}
			dirs[arm] = dir
		}
		for i, st := range steps {
			for _, arm := range arms {
				if arm == "baseline" && !baselineLive {
					continue
				}
				bin := cfg.candidate
				if arm == "baseline" {
					bin = cfg.baseline
				}
				s, err := runStep(bin, st.args, filepath.Join(dirs[arm], mpr), scripts, cfg.work)
				switch {
				case err != nil && arm == "candidate":
					return res, fmt.Errorf("%s, step %s: %w", size, st.name, err)
				case err != nil:
					// The baseline may not know a statement the session uses.
					// That is not this run's failure, but its project has now
					// diverged, so no later step of it is comparable.
					baselineLive = false
					res.BaselineError = fmt.Sprintf("step %s: %v", st.name, err)
					fmt.Fprintf(cfg.log, "  baseline stopped at %s; later steps have no A/B\n", st.name)
				case arm == "candidate":
					candidate[i] = append(candidate[i], s)
				default:
					baseline[i] = append(baseline[i], s)
				}
			}
		}
		for _, dir := range dirs {
			_ = os.RemoveAll(dir)
		}
		fmt.Fprintf(cfg.log, "  %s: repetition %d of %d done\n", size, rep+1, cfg.reps)
	}

	var invariant bool
	for i, st := range steps {
		sr := stepResult{Name: st.name, Weight: st.weight, Candidate: newArmResult(candidate[i])}
		res.Weighted.CandidateMS += sr.Candidate.Wall.Median * float64(st.weight)
		if b := newArmResult(baseline[i]); b != nil {
			sr.Baseline = b
			sr.WallRatio = pairedRatio(sr.Candidate.Wall.Samples, b.Wall.Samples)
			sr.CPURatio = pairedRatio(sr.Candidate.CPU.Samples, b.CPU.Samples)
			res.Weighted.BaselineMS += b.Wall.Median * float64(st.weight)
			switch {
			case sr.WallRatio > cfg.threshold:
				sr.Flags = append(sr.Flags, "slower")
			case sr.WallRatio > 0 && sr.WallRatio < 1/cfg.threshold:
				sr.Flags = append(sr.Flags, "faster")
			}
			if sr.Candidate.FilesChanged != b.FilesChanged ||
				(sr.Candidate.Counts != nil && b.Counts != nil && *sr.Candidate.Counts != *b.Counts) {
				sr.Flags = append(sr.Flags, "counts-changed")
			}
		}
		if !sr.Candidate.CountsStable {
			sr.Flags = append(sr.Flags, "counts-unstable")
		}
		if st.noWrites && sr.Candidate.FilesChanged != 0 {
			sr.Flags = append(sr.Flags, "wrote-on-re-exec")
			invariant = true
		}
		res.Steps = append(res.Steps, sr)
	}
	// A weighted ratio only means something over the same steps in both arms.
	if res.BaselineError == "" && res.Weighted.BaselineMS > 0 {
		res.Weighted.Ratio = res.Weighted.CandidateMS / res.Weighted.BaselineMS
	} else {
		res.Weighted.BaselineMS = 0
	}
	if invariant {
		return res, errInvariant
	}
	return res, nil
}

// seedLarge builds the large fixture: a copy of the base project with the
// generated app executed into it, by the candidate. Its three scripts are
// timed once each.
func seedLarge(cfg config, scripts string) (string, []seedResult, error) {
	dir := filepath.Join(cfg.work, "large-snapshot")
	if err := copyProject(cfg.base, dir); err != nil {
		return "", nil, err
	}
	mpr, err := findMPR(dir)
	if err != nil {
		return "", nil, err
	}
	var out []seedResult
	for _, part := range []struct{ name, src string }{
		{"seed-domain", gen.Domain(cfg.large)},
		{"seed-microflows", gen.Microflows(cfg.large)},
		{"seed-pages", gen.Pages(cfg.large)},
	} {
		script := filepath.Join(scripts, part.name+".mdl")
		if err := os.WriteFile(script, []byte(part.src), 0o644); err != nil {
			return "", nil, err
		}
		s, err := runStep(cfg.candidate, []string{"exec", script, "-p", "{P}"}, filepath.Join(dir, mpr), scripts, cfg.work)
		if err != nil {
			return "", nil, fmt.Errorf("%s: %w", part.name, err)
		}
		fmt.Fprintf(cfg.log, "  %s: %.1fs\n", part.name, s.WallMS/1000)
		out = append(out, seedResult{Name: part.name, sample: s})
	}
	return dir, out, nil
}

// run is the whole benchmark: every requested size, in order.
func run(cfg config) ([]sessionResult, error) {
	scripts := filepath.Join(cfg.work, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		return nil, err
	}
	if err := writeSession(scripts, cfg.sessionEntities); err != nil {
		return nil, err
	}
	var results []sessionResult
	var invariant bool
	for _, size := range cfg.sizes {
		fmt.Fprintf(cfg.log, "%s session\n", size)
		var snapshot string
		var seed []seedResult
		switch size {
		case "small":
			snapshot = filepath.Join(cfg.work, "small-snapshot")
			if err := copyProject(cfg.base, snapshot); err != nil {
				return results, err
			}
		case "large":
			var err error
			if snapshot, seed, err = seedLarge(cfg, scripts); err != nil {
				return results, err
			}
		default:
			return results, fmt.Errorf("unknown size %q (want small or large)", size)
		}
		res, err := runSession(cfg, size, snapshot, scripts)
		res.Seed = seed
		if errors.Is(err, errInvariant) {
			invariant = true
		} else if err != nil {
			return results, err
		}
		results = append(results, res)
	}
	if invariant {
		return results, errInvariant
	}
	return results, nil
}
