// SPDX-License-Identifier: Apache-2.0

//go:build unix

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeMxcli writes a shell script that stands in for mxcli, so the driver's
// own behaviour — interleaving, pairing, flagging, giving up on a baseline —
// is tested without a project or a build. It takes about 30 ms, so that the
// scheduler's jitter is small beside it, exits 0 and reports fixed counts,
// except:
//
//	slowOn  a command line containing this sleeps first
//	failOn  a command line containing this exits non-zero
//	writeOn a command line containing this rewrites a unit file with new bytes
func fakeMxcli(t *testing.T, name, slowOn, failOn, writeOn string) string {
	t.Helper()
	return fakeMxcliLogging(t, name, slowOn, failOn, writeOn, "")
}

// fakeMxcliLogging is fakeMxcli that also appends "<name> <command line>" to
// callLog for every command, and creates a project for `new`, so the order in
// which the driver ran two binaries can be read back.
func fakeMxcliLogging(t *testing.T, name, slowOn, failOn, writeOn, callLog string) string {
	t.Helper()
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = version ]; then echo "mxcli version %[1]s"; exit 0; fi
match() { [ -n "$1" ] && case "$ARGS" in *"$1"*) return 0;; esac; return 1; }
ARGS="$*"
if [ -n %[5]q ]; then echo "%[1]s $ARGS" >> %[5]q; fi
project=""
outdir=""
while [ $# -gt 0 ]; do
  if [ "$1" = -p ]; then project="$2"; fi
  if [ "$1" = --output-dir ]; then outdir="$2"; fi
  shift
done
if [ -n "$outdir" ]; then
  mkdir -p "$outdir/mprcontents" && echo mpr > "$outdir/App.mpr" && echo unit > "$outdir/mprcontents/unit.mxunit"
fi
sleep 0.03
if match %[2]q; then sleep 0.25; fi
if match %[3]q; then echo "unknown statement" >&2; exit 3; fi
if match %[4]q; then date +%%s%%N >> "$(dirname "$project")/mprcontents/unit.mxunit"; fi
if [ -n "$MXCLI_STATS_FILE" ]; then
  echo '{"unit_file_reads":7,"writes_offered":2,"writes_written":1}' >> "$MXCLI_STATS_FILE"
fi
echo ok
`, name, slowOn, failOn, writeOn, callLog)
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeProject is the smallest thing the driver accepts as a base project.
func fakeProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "mprcontents"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"App.mpr": "mpr", "mprcontents/unit.mxunit": "unit"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func testConfig(t *testing.T, candidate, baseline string) config {
	t.Helper()
	return config{
		candidate: candidate, baseline: baseline, base: fakeProject(t), work: t.TempDir(),
		reps: 3, sessionEntities: 1, sizes: []string{"small"}, threshold: 1.5, log: io.Discard,
	}
}

func stepNamed(t *testing.T, s sessionResult, name string) stepResult {
	t.Helper()
	for _, st := range s.Steps {
		if st.Name == name {
			return st
		}
	}
	t.Fatalf("no step %q in the session", name)
	return stepResult{}
}

// The benchmark's reason to exist: a candidate that is slower in one step is
// reported slower in that step and in no other. The control is the baseline,
// which is the same script without the delay — so a driver that flagged
// nothing, or everything, fails here.
func TestRun_FlagsExactlyTheStepThatGotSlower(t *testing.T) {
	t.Parallel()
	candidate := fakeMxcli(t, "candidate", "rename entity", "", "")
	baseline := fakeMxcli(t, "baseline", "", "", "")
	sessions, err := run(testConfig(t, candidate, baseline))
	if err != nil {
		t.Fatal(err)
	}
	var slower []string
	for _, st := range sessions[0].Steps {
		if slices.Contains(st.Flags, "slower") {
			slower = append(slower, st.Name)
		}
	}
	if want := []string{"rename"}; !slices.Equal(slower, want) {
		t.Fatalf("flagged slower: %v, want %v", slower, want)
	}
	if r := stepNamed(t, sessions[0], "rename").WallRatio; r < 3 {
		t.Errorf("rename ratio %.2f, want a 250 ms delay on a 30 ms command to read well above 3", r)
	}
}

// A baseline that cannot run a step — an older build that does not know a
// statement the session uses — is not this run's failure. The steps before it
// keep their A/B; it and the ones after have none, because the baseline's
// project no longer holds what the candidate's does.
func TestRun_BaselineThatFailsLosesItsLaterSteps(t *testing.T) {
	t.Parallel()
	candidate := fakeMxcli(t, "candidate", "", "", "")
	baseline := fakeMxcli(t, "baseline", "", "refresh catalog full", "")
	sessions, err := run(testConfig(t, candidate, baseline))
	if err != nil {
		t.Fatalf("a failing baseline failed the run: %v", err)
	}
	s := sessions[0]
	if !strings.Contains(s.BaselineError, "catalog-full") {
		t.Errorf("baseline error %q does not name the step it stopped at", s.BaselineError)
	}
	if stepNamed(t, s, "pages").Baseline == nil {
		t.Error("pages ran before the failure and lost its baseline")
	}
	for _, name := range []string{"catalog-full", "single-edit", "rename"} {
		if st := stepNamed(t, s, name); st.Baseline != nil || st.WallRatio != 0 {
			t.Errorf("%s has a baseline measurement after the baseline stopped: %+v", name, st.Baseline)
		}
		if st := stepNamed(t, s, name); len(st.Candidate.Wall.Samples) != 3 {
			t.Errorf("%s: candidate has %d samples, want all 3 repetitions", name, len(st.Candidate.Wall.Samples))
		}
	}
	if s.Weighted.Ratio != 0 {
		t.Errorf("weighted ratio %.2f over steps only one arm ran", s.Weighted.Ratio)
	}
}

// A candidate that fails a step fails the run: a failed command returns fast,
// and recording that as a time would report a regression as a speed-up.
func TestRun_CandidateThatFailsFailsTheRun(t *testing.T) {
	t.Parallel()
	candidate := fakeMxcli(t, "candidate", "", "pages.mdl", "")
	_, err := run(testConfig(t, candidate, ""))
	if err == nil || !strings.Contains(err.Error(), "step pages") || !strings.Contains(err.Error(), "unknown statement") {
		t.Fatalf("got %v, want an error naming the step and carrying the command's output", err)
	}
}

// Re-running a settled script must change no unit file. A candidate that does
// is reported with its record intact, and the run ends in error.
func TestRun_WritingOnReExecIsAnInvariantFailure(t *testing.T) {
	t.Parallel()
	writes := fakeMxcli(t, "candidate", "", "", "bulk-alter.mdl")
	sessions, err := run(testConfig(t, writes, ""))
	if !errors.Is(err, errInvariant) {
		t.Fatalf("got %v, want errInvariant", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("the record was dropped with the error: %d sessions", len(sessions))
	}
	if st := stepNamed(t, sessions[0], "re-exec"); !slices.Contains(st.Flags, "wrote-on-re-exec") || st.Candidate.FilesChanged != 1 {
		t.Errorf("re-exec: flags %v, files changed %d", st.Flags, st.Candidate.FilesChanged)
	}

	// Control: the same session with a candidate that leaves the file alone.
	quiet := fakeMxcli(t, "candidate", "", "", "")
	if _, err := run(testConfig(t, quiet, "")); err != nil {
		t.Fatalf("a candidate that writes nothing on re-exec: %v", err)
	}
}

// Counts come from the stats file and are summed over its lines; a step whose
// count differs between the arms is flagged whatever its time did.
func TestRun_CountsAreRecordedAndComparedAcrossArms(t *testing.T) {
	t.Parallel()
	candidate := fakeMxcli(t, "candidate", "", "", "pages.mdl")
	baseline := fakeMxcli(t, "baseline", "", "", "")
	sessions, err := run(testConfig(t, candidate, baseline))
	if err != nil {
		t.Fatal(err)
	}
	open := stepNamed(t, sessions[0], "open")
	if c := open.Candidate.Counts; c == nil || c.UnitFileReads != 7 || c.WritesWritten != 1 {
		t.Errorf("open counts %+v, want the stats file's 7 reads and 1 write", c)
	}
	if slices.Contains(open.Flags, "counts-changed") {
		t.Errorf("open flagged counts-changed with equal counts in both arms")
	}
	if pages := stepNamed(t, sessions[0], "pages"); !slices.Contains(pages.Flags, "counts-changed") {
		t.Errorf("pages changed a file in one arm only and is not flagged: %v", pages.Flags)
	}
}

func TestPairedRatioAndMedian(t *testing.T) {
	if got := median([]float64{5, 1, 3}); got != 3 {
		t.Errorf("median of three: %v", got)
	}
	if got := median([]float64{4, 1, 3, 2}); got != 2.5 {
		t.Errorf("median of four: %v", got)
	}
	if got := median(nil); got != 0 {
		t.Errorf("median of none: %v", got)
	}
	// Repetition 2 ran while the machine was busy: both arms doubled. The
	// paired ratio is unmoved; the ratio of the medians would be too, here,
	// but only the pairing guarantees it.
	if got := pairedRatio([]float64{110, 220, 110}, []float64{100, 200, 100}); got < 1.099 || got > 1.101 {
		t.Errorf("paired ratio: %v, want 1.1", got)
	}
	// The baseline stopped after one repetition: only that pair counts.
	if got := pairedRatio([]float64{150, 100, 100}, []float64{100}); got != 1.5 {
		t.Errorf("paired ratio over unequal lengths: %v, want 1.5", got)
	}
}

// A copy must not carry the caches a previous command left in the project:
// the arm whose binary wrote them would start warm.
func TestCopyProject_LeavesTheCachesBehind(t *testing.T) {
	src := fakeProject(t)
	if err := os.MkdirAll(filepath.Join(src, ".mxcli"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, ".mxcli", "unit-index-2.idx"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "copy")
	if err := copyProject(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".mxcli")); !os.IsNotExist(err) {
		t.Errorf(".mxcli was copied (%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "mprcontents", "unit.mxunit")); err != nil {
		t.Errorf("the unit file was not copied: %v", err)
	}
}

// The running-app session leaves a runtime up between its steps, so the two
// arms must not be interleaved: one arm's whole session, its stop, then the
// other's. And the stop must happen whatever the session did — here the
// baseline fails half way — or the next boot finds the ports taken.
func TestRun_RuntimeSessionRunsArmsInTurnAndAlwaysStops(t *testing.T) {
	t.Parallel()
	callLog := filepath.Join(t.TempDir(), "calls.log")
	candidate := fakeMxcliLogging(t, "candidate", "", "", "", callLog)
	baseline := fakeMxcliLogging(t, "baseline", "", "entity-edit.mdl", "", callLog)
	cfg := testConfig(t, candidate, baseline)
	cfg.sizes, cfg.runtimeVersion, cfg.runtimeReps, cfg.pageCheck = nil, "11.13.0", 1, true

	sessions, err := run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Size != "runtime" {
		t.Fatalf("sessions: %+v, want the runtime session alone", sessions)
	}
	s := sessions[0]
	var seeds []string
	for _, seed := range s.Seed {
		seeds = append(seeds, seed.Name)
	}
	if !slices.Equal(seeds, []string{"new", "author"}) {
		t.Errorf("seed steps %v, want the project's creation and its authoring", seeds)
	}

	data, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatal(err)
	}
	// The session proper starts at the first boot; before it is preparation.
	var arms, stops []string
	started := false
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		arm, cmd, _ := strings.Cut(line, " ")
		if strings.HasPrefix(cmd, "run --local --watch") {
			started = true
		}
		if !started {
			if arm != "candidate" {
				t.Errorf("the baseline took part in preparing the project: %s", line)
			}
			continue
		}
		if len(arms) == 0 || arms[len(arms)-1] != arm {
			arms = append(arms, arm)
		}
		if strings.HasPrefix(cmd, "run stop") {
			stops = append(stops, arm)
		}
	}
	if !slices.Equal(arms, []string{"candidate", "baseline"}) {
		t.Errorf("arms ran in the order %v, want each arm's whole session in turn\n%s", arms, data)
	}
	// The candidate stops twice (its own stop step, then the teardown); the
	// baseline never reached its stop step, so its one stop is the teardown.
	if !slices.Equal(stops, []string{"candidate", "candidate", "baseline"}) {
		t.Errorf("run stop was called by %v, want the baseline stopped after it failed\n%s", stops, data)
	}

	if st := stepNamed(t, s, "hot-apply"); st.Baseline == nil {
		t.Error("hot-apply ran before the baseline failed and has no baseline")
	} else if one := stepNamed(t, s, "test-attach").Candidate.Wall.Median; st.Candidate.Wall.Median < 1.3*one {
		// Two commands, the edit and the wait, measured as one step.
		t.Errorf("hot-apply took %.0f ms against %.0f ms for a one-command step, want the two commands summed", st.Candidate.Wall.Median, one)
	}
	for _, name := range []string{"apply-entity", "stop", "test-cold"} {
		if stepNamed(t, s, name).Baseline != nil {
			t.Errorf("%s has a baseline measurement after the baseline failed", name)
		}
	}
}

// The session validates and builds before it boots, and the build does not
// repeat the check it follows.
func TestRuntimeSteps_ValidateAndBuildComeBeforeBoot(t *testing.T) {
	var names []string
	for _, st := range runtimeSteps(true) {
		names = append(names, st.name)
	}
	if want := []string{"mx-check", "build", "boot"}; len(names) < 3 || !slices.Equal(names[:3], want) {
		t.Fatalf("the session starts %v, want %v", names, want)
	}
	build := runtimeSteps(true)[1].cmds[0]
	if !slices.Contains(build, "--skip-check") {
		t.Errorf("the build step %v runs the check again, which mx-check already timed", build)
	}
	if !slices.Contains(build, "{D}/bench-package") {
		t.Errorf("the build step %v does not write its package inside the arm's own project copy", build)
	}
}

// Without the page check — a machine with no browser — the session is the
// same minus that step.
func TestRuntimeSteps_PageCheckIsOptional(t *testing.T) {
	with, without := runtimeSteps(true), runtimeSteps(false)
	if len(with) != len(without)+1 {
		t.Fatalf("%d steps with the page check, %d without", len(with), len(without))
	}
	for _, st := range without {
		if st.name == "page-check" {
			t.Error("page-check is still in the session")
		}
	}
}

// The scale session runs its own steps on a project seeded with filler
// microflows, records how many statements each batch holds, and reports them
// per statement. A batch larger than the seed is refused before anything runs.
func TestRun_ScaleSessionReportsItsBatchesPerStatement(t *testing.T) {
	// Not parallel: it times nothing, and run beside the tests that do, its
	// thirty processes take the processor they are measuring 30 ms commands on.
	log := filepath.Join(t.TempDir(), "calls.log")
	candidate := fakeMxcliLogging(t, "candidate", "", "", "", log)
	cfg := testConfig(t, candidate, "")
	cfg.sizes, cfg.reps, cfg.scaleMicroflows, cfg.scaleBatch = []string{"scale"}, 1, 9, 4
	sessions, err := run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Size != "scale" {
		t.Fatalf("got %d sessions, want the scale session", len(sessions))
	}
	var names []string
	for _, st := range sessions[0].Steps {
		names = append(names, st.Name)
	}
	want := []string{"check-cold", "check-one", "check-batch", "exec-batch", "re-exec-batch", "new-batch"}
	if !slices.Equal(names, want) {
		t.Errorf("steps %v, want %v", names, want)
	}
	if got := stepNamed(t, sessions[0], "exec-batch").Statements; got != 4 {
		t.Errorf("exec-batch records %d statements, want 4", got)
	}
	if got := stepNamed(t, sessions[0], "check-one").Statements; got != 1 {
		t.Errorf("check-one records %d statements, want 1", got)
	}
	if len(sessions[0].Seed) != 1 || sessions[0].Seed[0].Name != "seed-filler" {
		t.Errorf("seed %+v, want the filler seed timed once", sessions[0].Seed)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(string(calls), "\n", 2)[0]
	if !strings.Contains(first, "seed-filler.mdl") {
		t.Errorf("the first command was %q, want the filler seed", first)
	}
	seed, err := os.ReadFile(filepath.Join(cfg.work, "scripts", "seed-filler.mdl"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(seed), "create or modify microflow"); got != 9 {
		t.Errorf("the seed holds %d microflows, want 9", got)
	}

	md := newRecord(cfg, sessions).markdown()
	if !strings.Contains(md, "### Per statement") || !strings.Contains(md, "| exec-batch | 4 |") {
		t.Errorf("the summary has no per-statement row for the batch:\n%s", md)
	}

	cfg.work, cfg.scaleBatch = t.TempDir(), 10
	if _, err := run(cfg); err == nil || !strings.Contains(err.Error(), "seeded with 9") {
		t.Errorf("a batch of 10 on a seed of 9 gave %v, want a refusal", err)
	}
}
