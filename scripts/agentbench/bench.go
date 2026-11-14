// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	// cmds are the command lines after the binary, run one after the other
	// and measured as one: {P} is the project's .mpr, {D} its directory and
	// {S} the directory holding the session's scripts. Most steps are one command. A change to a
	// running app is two, the edit and the wait for it to be live, because
	// that pair is what the agent sits through.
	cmds [][]string
	// weight is how many times a real session runs this kind of command for
	// each time the benchmark does; see weights below.
	weight int
	// noWrites marks a step that must change no unit file: the idempotence
	// canary. A candidate that writes there has a defect, not a slowdown.
	noWrites bool
	// statements is how many statements the step's script holds, for a step
	// whose cost is reported per statement (the scale session's batches).
	statements int
}

// one is a step of a single command.
func one(args ...string) [][]string { return [][]string{args} }

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
		{name: "open-cold", cmds: one("-p", "{P}", "-c", "show modules"), weight: 1},
		{name: "open", cmds: one("-p", "{P}", "-c", "show modules"), weight: 60},
		{name: "domain", cmds: one("exec", "{S}/domain.mdl", "-p", "{P}"), weight: 1},
		{name: "microflows", cmds: one("exec", "{S}/microflows.mdl", "-p", "{P}"), weight: 1},
		{name: "pages", cmds: one("exec", "{S}/pages.mdl", "-p", "{P}"), weight: 1},
		{name: "describe", cmds: one("-p", "{P}", "-c", read), weight: 60},
		{name: "catalog", cmds: one("-p", "{P}", "-c", "refresh catalog"), weight: 5},
		{name: "catalog-full", cmds: one("-p", "{P}", "-c", "refresh catalog full"), weight: 1},
		{name: "query", cmds: one("-p", "{P}", "-c", "select count(*) from CATALOG.ENTITIES"), weight: 20},
		{name: "check", cmds: one("check", "{S}/single-edit.mdl", "-p", "{P}", "--references"), weight: 90},
		{name: "single-edit", cmds: one("exec", "{S}/single-edit.mdl", "-p", "{P}"), weight: 150},
		{name: "catalog-after-edit", cmds: one("-p", "{P}", "-c", "refresh catalog full"), weight: 10},
		{name: "bulk-alter", cmds: one("exec", "{S}/bulk-alter.mdl", "-p", "{P}"), weight: 1},
		{name: "re-exec", cmds: one("exec", "{S}/bulk-alter.mdl", "-p", "{P}"), weight: 1, noWrites: true},
		{name: "rename", cmds: one("-p", "{P}", "-c", "rename entity Bench.Item01 to Item01Renamed"), weight: 1},
	}
}

// scaleModule is the module the scale session's filler microflows live in.
const scaleModule = "Scale"

// scaleSteps is the scale session: what a statement costs in a project of
// thousands of units, the measurement of mendixlabs/mxcli#1842. There, `exec`
// of an update cost time linear in the project size, about 0.27 s more per
// statement for every 1,000 units, and `check` cost seconds per run before it
// had looked at a statement.
//
// The small and large sessions cannot show either: their largest project is
// under a thousand units, where both costs are inside the noise. This one runs
// on a project grown with filler microflows and reports the batches per
// statement, so that a cost that follows the project again shows as a ratio
// here and as a number an agent would feel.
func scaleSteps(batch int) []step {
	return []step{
		// One statement, so nearly all of it is what a check costs per run. The
		// first is the first command on a fresh copy of the project and reads
		// every unit file it needs from disk; the second finds them read.
		{name: "check-cold", cmds: one("check", "{S}/scale-one.mdl", "-p", "{P}", "--references"), weight: 1, statements: 1},
		{name: "check-one", cmds: one("check", "{S}/scale-one.mdl", "-p", "{P}", "--references"), weight: 1, statements: 1},
		{name: "check-batch", cmds: one("check", "{S}/scale-updates.mdl", "-p", "{P}", "--references"), weight: 1, statements: batch},
		// Without the check `exec` runs first, so the two are not counted twice.
		{name: "exec-batch", cmds: one("exec", "{S}/scale-updates.mdl", "-p", "{P}", "--no-check"), weight: 1, statements: batch},
		{name: "re-exec-batch", cmds: one("exec", "{S}/scale-updates.mdl", "-p", "{P}", "--no-check"), weight: 1, statements: batch, noWrites: true},
		{name: "new-batch", cmds: one("exec", "{S}/scale-new.mdl", "-p", "{P}", "--no-check"), weight: 1, statements: batch},
	}
}

// writeScale writes the scale session's scripts.
func writeScale(dir string, batch int) error {
	for name, src := range map[string]string{
		"scale-one.mdl":     gen.FillerUpdates(scaleModule, 1, "one"),
		"scale-updates.mdl": gen.FillerUpdates(scaleModule, batch, "batch"),
		"scale-new.mdl":     gen.FillerNew(scaleModule, batch),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// seedScale builds the scale fixture: a copy of the base project with the
// filler microflows executed into it, by the candidate, timed once.
func seedScale(cfg config, scripts string) (string, []seedResult, error) {
	if cfg.scaleBatch > cfg.scaleMicroflows {
		return "", nil, fmt.Errorf("the scale session updates %d microflows of a project seeded with %d", cfg.scaleBatch, cfg.scaleMicroflows)
	}
	dir := filepath.Join(cfg.work, "scale-snapshot")
	if err := copyProject(cfg.base, dir); err != nil {
		return "", nil, err
	}
	mpr, err := findMPR(dir)
	if err != nil {
		return "", nil, err
	}
	script := filepath.Join(scripts, "seed-filler.mdl")
	if err := os.WriteFile(script, []byte(gen.Filler(scaleModule, cfg.scaleMicroflows)), 0o644); err != nil {
		return "", nil, err
	}
	s, err := runStep(cfg.candidate, one("exec", script, "-p", "{P}", "--no-check"), filepath.Join(dir, mpr), scripts, cfg.work)
	if err != nil {
		return "", nil, fmt.Errorf("seed-filler: %w", err)
	}
	fmt.Fprintf(cfg.log, "  seed-filler: %.1fs\n", s.WallMS/1000)
	return dir, []seedResult{{Name: "seed-filler", sample: s}}, nil
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
		"entity-edit.mdl": gen.EntityEdit(p),
		"bench.test.mdl":  gen.Tests(p),
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
	// UnitStoreHits is the reads served from the persisted unit store rather
	// than a file. nil when the binary does not report them: a re-read of a
	// unit is served from the store, so without this count a command that
	// reads every unit twice looks like one that reads it once
	// (ako/mxcli#2160).
	UnitStoreHits *int64 `json:"unit_store_hits,omitempty"`
	WritesOffered int    `json:"writes_offered"`
	WritesWritten int    `json:"writes_written"`
}

// commandTimeout bounds one command. A hung boot must fail the run with its
// output, not sit until the CI job is cancelled with none.
const commandTimeout = 20 * time.Minute

// statsSeq numbers the stats files, one per command: a detached `run` appends
// its line when it stops, minutes later, and must not land in the file of
// whichever step is running by then.
var statsSeq int

// runStep runs a step's commands in order and measures them as one. A non-zero
// exit is an error carrying the tail of the output: a failed command's time is
// not a measurement.
func runStep(bin string, cmds [][]string, project, scripts, scratch string) (sample, error) {
	before := unitDigests(filepath.Dir(project))
	var s sample
	for _, args := range cmds {
		resolved := make([]string, len(args))
		for i, a := range args {
			a = strings.ReplaceAll(a, "{P}", project)
			a = strings.ReplaceAll(a, "{D}", filepath.Dir(project))
			resolved[i] = strings.ReplaceAll(a, "{S}", scripts)
		}
		statsSeq++
		statsPath := filepath.Join(scratch, fmt.Sprintf("stats-%d.jsonl", statsSeq))

		ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
		cmd := exec.CommandContext(ctx, bin, resolved...)
		cmd.Env = append(os.Environ(), statsFileEnv+"="+statsPath, "NO_COLOR=1")
		// A detached child may keep the output pipes open; do not wait for it.
		cmd.WaitDelay = 5 * time.Second
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		start := time.Now()
		err := cmd.Run()
		wall := time.Since(start)
		cancel()
		if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
			return sample{}, fmt.Errorf("%s %s: %w\n%s", filepath.Base(bin), strings.Join(resolved, " "), err, tail(stdout.String()+stderr.String(), 20))
		}
		s.WallMS += float64(wall.Microseconds()) / 1000
		s.CPUMS += float64((cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()).Microseconds()) / 1000
		s.RSSMB = max(s.RSSMB, maxRSSMB(cmd.ProcessState))
		s.StdoutBytes += stdout.Len()
		if c := readCounts(statsPath); c != nil {
			if s.Counts == nil {
				s.Counts = &counts{}
			}
			s.Counts.add(*c)
		}
		_ = os.Remove(statsPath)
	}
	s.FilesChanged = changedFiles(before, unitDigests(filepath.Dir(project)))
	return s, nil
}

// add sums o into c. The store hits stay unreported unless a line reports
// them.
func (c *counts) add(o counts) {
	c.UnitFileReads += o.UnitFileReads
	c.WritesOffered += o.WritesOffered
	c.WritesWritten += o.WritesWritten
	if o.UnitStoreHits != nil {
		if c.UnitStoreHits == nil {
			c.UnitStoreHits = new(int64)
		}
		*c.UnitStoreHits += *o.UnitStoreHits
	}
}

// same reports whether two counts agree. Store hits are compared only when
// both sides report them: against a baseline too old to count them, their
// absence is not a difference.
func (c counts) same(o counts) bool {
	if c.UnitFileReads != o.UnitFileReads || c.WritesOffered != o.WritesOffered || c.WritesWritten != o.WritesWritten {
		return false
	}
	return c.UnitStoreHits == nil || o.UnitStoreHits == nil || *c.UnitStoreHits == *o.UnitStoreHits
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
			total.add(c)
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
		// `mxcli new` links its own binary into the project root: 100 MB a
		// copy, and not part of the project.
		if !d.Type().IsRegular() || rel == "mxcli" {
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
			(s.Counts == nil) != (r.Counts == nil) || (s.Counts != nil && !s.Counts.same(*r.Counts)) {
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
	// Statements is how many statements the step's script holds, when its
	// cost is reported per statement; 0 otherwise.
	Statements int `json:"statements,omitempty"`
}

// sessionResult is the scripted session at one project size.
type sessionResult struct {
	Size string `json:"size"`
	// Reps is how many repetitions each arm ran.
	Reps int `json:"reps"`
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
	// scaleMicroflows is how many filler microflows the scale project is
	// seeded with, and scaleBatch how many statements its batches hold.
	scaleMicroflows, scaleBatch int
	sizes                       []string
	// runtimeVersion, when set, adds the running-app session on a project
	// created by `mxcli new` for that Mendix version.
	runtimeVersion string
	runtimeReps    int
	pageCheck      bool
	threshold      float64
	log            io.Writer
}

// errInvariant is returned, after the record is complete, when the candidate
// broke something a benchmark run can state exactly rather than estimate.
var errInvariant = errors.New("the candidate broke an invariant")

// A session is a list of steps run in order against one project.
type session struct {
	name  string
	steps []step
	reps  int
	// armsInTurn runs one arm's whole session before the other's, instead of
	// alternating the arms step by step. The running-app session needs it: its
	// steps leave a runtime up, and two of them at once would share the ports
	// and the processor they are being timed on.
	armsInTurn bool
	// teardown is run, unmeasured, after each arm's repetition, whatever
	// happened in it: a runtime left behind would fail the next boot.
	teardown []string
}

// runSession measures one session. snapshot is the project every repetition of
// both arms starts from.
func runSession(cfg config, def session, snapshot, scripts string) (sessionResult, error) {
	mpr, err := findMPR(snapshot)
	if err != nil {
		return sessionResult{}, err
	}
	size, steps := def.name, def.steps
	res := sessionResult{Size: size, Reps: def.reps, Units: countUnits(snapshot)}
	candidate := make([][]sample, len(steps))
	baseline := make([][]sample, len(steps))
	baselineLive := cfg.baseline != ""

	for rep := 0; rep < def.reps; rep++ {
		arms := []string{"candidate"}
		if baselineLive {
			arms = append(arms, "baseline")
			// Alternate which arm goes first, so neither always runs on the
			// page cache the other warmed.
			if rep%2 == 1 {
				arms[0], arms[1] = arms[1], arms[0]
			}
		}
		binOf := func(arm string) string {
			if arm == "baseline" {
				return cfg.baseline
			}
			return cfg.candidate
		}
		dirs := map[string]string{}
		prepare := func(arm string) error {
			dirs[arm] = filepath.Join(cfg.work, fmt.Sprintf("%s-%s-%d", size, arm, rep))
			return copyProject(snapshot, dirs[arm])
		}
		// finish stops what the arm started and removes its project.
		finish := func(arm string) {
			if def.teardown != nil {
				_, _ = runStep(binOf(arm), [][]string{def.teardown}, filepath.Join(dirs[arm], mpr), scripts, cfg.work)
			}
			_ = os.RemoveAll(dirs[arm])
		}
		// runOne runs step i in one arm. Its error is the candidate's: a
		// baseline that fails only retires the baseline.
		runOne := func(i int, arm string) error {
			if arm == "baseline" && !baselineLive {
				return nil
			}
			st := steps[i]
			s, err := runStep(binOf(arm), st.cmds, filepath.Join(dirs[arm], mpr), scripts, cfg.work)
			switch {
			case err != nil && arm == "candidate":
				return fmt.Errorf("%s, step %s: %w", size, st.name, err)
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
			return nil
		}

		var stepErr error
		if def.armsInTurn {
			for _, arm := range arms {
				if stepErr = prepare(arm); stepErr == nil {
					for i := range steps {
						if stepErr = runOne(i, arm); stepErr != nil {
							break
						}
					}
				}
				finish(arm)
				if stepErr != nil {
					break
				}
			}
		} else {
			for _, arm := range arms {
				if stepErr = prepare(arm); stepErr != nil {
					break
				}
			}
			for i := 0; i < len(steps) && stepErr == nil; i++ {
				for _, arm := range arms {
					if stepErr = runOne(i, arm); stepErr != nil {
						break
					}
				}
			}
			for arm := range dirs {
				finish(arm)
			}
		}
		if stepErr != nil {
			return res, stepErr
		}
		fmt.Fprintf(cfg.log, "  %s: repetition %d of %d done\n", size, rep+1, def.reps)
	}

	var invariant bool
	for i, st := range steps {
		sr := stepResult{Name: st.name, Weight: st.weight, Candidate: newArmResult(candidate[i]), Statements: st.statements}
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
				(sr.Candidate.Counts != nil && b.Counts != nil && !sr.Candidate.Counts.same(*b.Counts)) {
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
		s, err := runStep(cfg.candidate, one("exec", script, "-p", "{P}"), filepath.Join(dir, mpr), scripts, cfg.work)
		if err != nil {
			return "", nil, fmt.Errorf("%s: %w", part.name, err)
		}
		fmt.Fprintf(cfg.log, "  %s: %.1fs\n", part.name, s.WallMS/1000)
		out = append(out, seedResult{Name: part.name, sample: s})
	}
	return dir, out, nil
}

// runtimeSteps is the running-app session: what an agent waits for once the
// app is up. Weights are rounded from the same measured builds as the others
// (`run wait` 95 calls in the largest, restarts 150, `docker check` 84,
// microflow test runs 35).
func runtimeSteps(pageCheck bool) []step {
	steps := []step{
		// Validation by mx itself, on a temporary copy of the project: what an
		// agent runs after a batch of changes, before it trusts them.
		{name: "mx-check", cmds: one("docker", "check", "-p", "{P}"), weight: 80},
		// The full build to a deployable package. --skip-check, or it would
		// time the check a second time.
		{name: "build", cmds: one("docker", "build", "-p", "{P}", "--skip-check", "-o", "{D}/bench-package"), weight: 2},
		// Until the app serves: the build, the runtime start, the schema.
		{name: "boot", cmds: one("run", "--local", "--watch", "--detach", "--db-type", "hsqldb",
			"--test-endpoint", "--allow-call", "-p", "{P}"), weight: 10},
		{name: "page-check", cmds: one("playwright", "check", "-p", "{P}"), weight: 30},
		// A microflow change, live: the model is reloaded, the runtime stays.
		{name: "hot-apply", cmds: [][]string{
			{"exec", "{S}/single-edit.mdl", "-p", "{P}"},
			{"run", "wait", "-p", "{P}"},
		}, weight: 60},
		{name: "test-attach", cmds: one("test", "{S}/bench.test.mdl", "-p", "{P}", "--attach"), weight: 30},
		// An entity change, live: the runtime restarts and updates the schema.
		{name: "apply-entity", cmds: [][]string{
			{"exec", "{S}/entity-edit.mdl", "-p", "{P}"},
			{"run", "wait", "-p", "{P}"},
		}, weight: 20},
		{name: "stop", cmds: one("run", "stop", "-p", "{P}"), weight: 10},
		// A test run that boots its own runtime, as one does with no app up.
		{name: "test-cold", cmds: one("test", "{S}/bench.test.mdl", "-p", "{P}", "--local", "--db-type", "hsqldb"), weight: 5},
	}
	if !pageCheck {
		steps = slices.DeleteFunc(steps, func(s step) bool { return s.name == "page-check" })
	}
	return steps
}

// prepareRuntime creates the project the running-app session starts from:
// `mxcli new` for the requested Mendix version, with the session's app
// authored into it. The fixture the other sessions use cannot be built — it
// carries no theme — so this one has to be made. `mxcli new` installs mxbuild
// when it is not cached; that is done first and separately, so the download
// is in neither the `new` figure nor the first boot.
func prepareRuntime(cfg config, scripts string) (string, []seedResult, error) {
	untimed := func(what string, args ...string) error {
		fmt.Fprintf(cfg.log, "  %s\n", what)
		out, err := exec.Command(cfg.candidate, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %w\n%s", what, err, tail(string(out), 20))
		}
		return nil
	}
	if err := untimed("installing mxbuild "+cfg.runtimeVersion+" if it is not cached", "setup", "mxbuild", "--version", cfg.runtimeVersion); err != nil {
		return "", nil, err
	}
	dir := filepath.Join(cfg.work, "runtime-snapshot")
	var seed []seedResult
	start := time.Now()
	if err := untimed("creating the project", "new", "BenchApp", "--version", cfg.runtimeVersion, "--output-dir", dir); err != nil {
		return "", nil, err
	}
	seed = append(seed, seedResult{Name: "new", sample: sample{WallMS: float64(time.Since(start).Microseconds()) / 1000}})
	mpr, err := findMPR(dir)
	if err != nil {
		return "", nil, err
	}
	project := filepath.Join(dir, mpr)
	s, err := runStep(cfg.candidate, [][]string{
		{"exec", "{S}/domain.mdl", "-p", "{P}"},
		{"exec", "{S}/microflows.mdl", "-p", "{P}"},
		{"exec", "{S}/pages.mdl", "-p", "{P}"},
	}, project, scripts, cfg.work)
	if err != nil {
		return "", nil, fmt.Errorf("authoring the app: %w", err)
	}
	seed = append(seed, seedResult{Name: "author", sample: s})
	if err := untimed("caching the runtime", "run", "--local", "--setup", "--db-type", "hsqldb", "-p", project); err != nil {
		return "", nil, err
	}
	return dir, seed, nil
}

// run is the whole benchmark: every requested session, in order. When a
// session fails, the ones that completed are still returned.
func run(cfg config) ([]sessionResult, error) {
	scripts := filepath.Join(cfg.work, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		return nil, err
	}
	if err := writeSession(scripts, cfg.sessionEntities); err != nil {
		return nil, err
	}
	names := cfg.sizes
	if cfg.runtimeVersion != "" {
		names = append(slices.Clone(names), "runtime")
	}
	var results []sessionResult
	var invariant bool
	for _, name := range names {
		fmt.Fprintf(cfg.log, "%s session\n", name)
		def := session{name: name, steps: sessionSteps(), reps: cfg.reps}
		var snapshot string
		var seed []seedResult
		var err error
		switch name {
		case "small":
			snapshot = filepath.Join(cfg.work, "small-snapshot")
			err = copyProject(cfg.base, snapshot)
		case "large":
			snapshot, seed, err = seedLarge(cfg, scripts)
		case "scale":
			def.steps = scaleSteps(cfg.scaleBatch)
			if err = writeScale(scripts, cfg.scaleBatch); err == nil {
				snapshot, seed, err = seedScale(cfg, scripts)
			}
		case "runtime":
			def = session{
				name: name, steps: runtimeSteps(cfg.pageCheck), reps: cfg.runtimeReps,
				armsInTurn: true, teardown: []string{"run", "stop", "-p", "{P}"},
			}
			snapshot, seed, err = prepareRuntime(cfg, scripts)
		default:
			err = fmt.Errorf("unknown size %q (want small, large or scale)", name)
		}
		if err != nil {
			return results, err
		}
		res, err := runSession(cfg, def, snapshot, scripts)
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
