// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testThresholds = thresholds{slower: 1.2, drift: 1.3, minMS: 100}

// stepNames are the steps of the synthetic session: enough of them that a
// run's runner can be judged.
var stepNames = []string{"open", "domain", "microflows", "pages", "describe", "catalog", "check", "re-exec"}

// night is one synthetic record: every step takes base ms × runner, and the
// baseline arm the same, so every ratio is 1 until a test says otherwise.
func night(n int, runner float64) record {
	r := record{
		Date:      fmt.Sprintf("2026-01-%02dT03:00:00Z", n),
		Commit:    fmt.Sprintf("%040d", n),
		Candidate: fmt.Sprintf("mxcli c%d", n),
		Baseline:  fmt.Sprintf("mxcli c%d", n-1),
	}
	s := session{Size: "small", Units: 370}
	for i, name := range stepNames {
		wall := float64(200*(i+1)) * runner
		s.Steps = append(s.Steps, step{Name: name, Candidate: newArm(wall, 100, 0), Baseline: newArm(wall, 100, 0), WallRatio: 1})
	}
	s.Weighted.CandidateMS = 10000 * runner
	s.Weighted.Ratio = 1
	r.Sessions = []session{s}
	return r
}

func newArm(wall float64, reads int64, written int) *arm {
	a := &arm{}
	a.Wall.Median = wall
	a.Counts = &struct {
		UnitFileReads int64  `json:"unit_file_reads"`
		UnitStoreHits *int64 `json:"unit_store_hits"`
		WritesWritten int    `json:"writes_written"`
	}{reads, nil, written}
	return a
}

func stepOf(r *record, name string) *step {
	for i := range r.Sessions[0].Steps {
		if r.Sessions[0].Steps[i].Name == name {
			return &r.Sessions[0].Steps[i]
		}
	}
	panic("no step " + name)
}

func nights(n int) []record {
	var out []record
	for i := 1; i <= n; i++ {
		out = append(out, night(i, 1))
	}
	return out
}

func kinds(alerts []alert) string {
	var out []string
	for _, a := range alerts {
		out = append(out, a.Kind+":"+a.Step)
	}
	return strings.Join(out, " ")
}

func TestAlerts_AQuietHistoryReportsNothing(t *testing.T) {
	if got := findAlerts(nights(12), testThresholds); len(got) != 0 {
		t.Fatalf("alerts on a history where nothing moved: %s", kinds(got))
	}
}

// One slow night is not reported; the second in a row is. The control is the
// first half: the same ratio, one night only.
func TestAlerts_SlowerIsReportedOnTheSecondNight(t *testing.T) {
	recs := nights(4)
	stepOf(&recs[3], "pages").WallRatio = 1.5
	if got := findAlerts(recs, testThresholds); len(got) != 0 {
		t.Fatalf("one slow night was reported: %s", kinds(got))
	}
	stepOf(&recs[2], "pages").WallRatio = 1.4
	if got := kinds(findAlerts(recs, testThresholds)); got != "slower-two-runs:pages" {
		t.Fatalf("two slow nights: got %q, want slower-two-runs:pages", got)
	}
	// A night with no baseline has no ratio to be slower by.
	stepOf(&recs[2], "pages").Baseline = nil
	if got := findAlerts(recs, testThresholds); len(got) != 0 {
		t.Fatalf("the night before had no baseline, and was still counted: %s", kinds(got))
	}
}

func TestAlerts_CountsAreReportedOnTheFirstNight(t *testing.T) {
	recs := nights(2)
	stepOf(&recs[1], "check").Candidate.Counts.UnitFileReads = 140
	stepOf(&recs[1], "domain").Candidate.Counts.WritesWritten = 3
	// Fewer is an improvement, not an alert.
	stepOf(&recs[1], "open").Candidate.Counts.UnitFileReads = 10
	if got := kinds(findAlerts(recs, testThresholds)); got != "counts-increased:check counts-increased:domain" {
		t.Fatalf("got %q", got)
	}
}

// A unit read a second time is served from the persisted store, not its file:
// the file reads are equal and only the store hits show it. Where both arms
// report them the two are compared together; against a baseline that does
// not, the file reads are compared as before.
func TestAlerts_AReReadShowsInTheStoreHits(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	recs := nights(2)
	st := stepOf(&recs[1], "check")
	st.Candidate.Counts.UnitStoreHits, st.Baseline.Counts.UnitStoreHits = n(454), n(278)
	if got := kinds(findAlerts(recs, testThresholds)); got != "counts-increased:check" {
		t.Fatalf("got %q, want counts-increased:check", got)
	}
	// Control: the same hits on both arms.
	st.Candidate.Counts.UnitStoreHits = n(278)
	if got := findAlerts(recs, testThresholds); len(got) != 0 {
		t.Fatalf("equal reads were reported: %s", kinds(got))
	}
	// A baseline that does not report them.
	st.Candidate.Counts.UnitStoreHits, st.Baseline.Counts.UnitStoreHits = n(454), nil
	if got := findAlerts(recs, testThresholds); len(got) != 0 {
		t.Fatalf("an unreported count was compared: %s", kinds(got))
	}
	// Files moved to the store, the total the same: not an increase.
	st.Candidate.Counts.UnitFileReads, st.Candidate.Counts.UnitStoreHits = 0, n(378)
	st.Baseline.Counts.UnitStoreHits = n(278)
	if got := findAlerts(recs, testThresholds); len(got) != 0 {
		t.Fatalf("reads moving from files to the store were reported: %s", kinds(got))
	}
}

func TestAlerts_AWriteOnReExecIsReported(t *testing.T) {
	recs := nights(1)
	st := stepOf(&recs[0], "re-exec")
	st.Flags = []string{"wrote-on-re-exec"}
	st.Candidate.FilesChanged = 2
	got := findAlerts(recs, testThresholds)
	if kinds(got) != "wrote-on-re-exec:re-exec" || !strings.Contains(got[0].Detail, "2 unit file") {
		t.Fatalf("got %+v", got)
	}
}

// A step that creeps up 6% a night never crosses the nightly threshold and
// is what the drift rule exists for.
func TestAlerts_DriftIsSeenThoughNoNightWasSlow(t *testing.T) {
	recs := nights(12)
	wall := stepOf(&recs[0], "pages").Candidate.Wall.Median
	for i := 4; i < len(recs); i++ {
		wall *= 1.06
		st := stepOf(&recs[i], "pages")
		st.Candidate.Wall.Median = wall
		st.WallRatio = 1.06
	}
	if got := kinds(findAlerts(recs, testThresholds)); got != "drift:pages" {
		t.Fatalf("got %q, want drift:pages", got)
	}
	// Not with fewer runs than the rule compares.
	if got := findAlerts(recs[:9], testThresholds); len(got) != 0 {
		t.Fatalf("drift judged on %d runs: %s", 9, kinds(got))
	}
}

// Three nights on a runner half as fast move every step by more than the
// drift threshold. That is the machine, and must not be reported — while the
// one step that also regressed still is.
func TestAlerts_ASlowRunnerIsNotDrift(t *testing.T) {
	var recs []record
	for i := 1; i <= 12; i++ {
		runner := 1.0
		if i > 9 {
			runner = 1.6
		}
		recs = append(recs, night(i, runner))
	}
	if got := findAlerts(recs, testThresholds); len(got) != 0 {
		t.Fatalf("a slow runner was reported as drift: %s", kinds(got))
	}
	for i := 9; i < 12; i++ {
		stepOf(&recs[i], "check").Candidate.Wall.Median *= 1.5
	}
	if got := kinds(findAlerts(recs, testThresholds)); got != "drift:check" {
		t.Fatalf("got %q, want drift:check", got)
	}
}

func TestAlerts_DriftLeavesShortStepsAlone(t *testing.T) {
	recs := nights(12)
	for i := range recs {
		st := stepOf(&recs[i], "open")
		st.Candidate.Wall.Median = 40
		if i > 8 {
			st.Candidate.Wall.Median = 70
		}
	}
	if got := findAlerts(recs, testThresholds); len(got) != 0 {
		t.Fatalf("a 40 ms step was judged for drift: %s", kinds(got))
	}
}

func TestLoadRecords_OrdersByDateAndSkipsWhatIsNotARecord(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string, v any) {
		t.Helper()
		data, _ := json.Marshal(v)
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("2026/02/z-first.json", night(1, 1))
	write("2026/01/a-second.json", night(2, 1))
	write("2026/01/other.json", map[string]any{"hello": "world"})
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	recs, skipped, err := loadRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].file != "z-first.json" || recs[1].file != "a-second.json" {
		t.Fatalf("records: %+v", recs)
	}
	if strings.Join(skipped, ",") != "broken.json,other.json" {
		t.Fatalf("skipped: %v", skipped)
	}
}

func TestTrendMarkdown_ChartsATotalAndListsEveryStep(t *testing.T) {
	recs := nights(3)
	stepOf(&recs[2], "pages").Candidate.Wall.Median = 1600
	md := trendMarkdown(recs, []string{"other.json"}, []alert{{"drift", "small", "pages", "detail"}})
	for _, want := range []string{
		"3 runs, 2026-01-01 to 2026-01-03",
		"## small project — 370 units",
		"xychart-beta\n    title \"small: session-weighted total (seconds)\"",
		`plotColorPalette: "#2f81f7"`,
		`x-axis ["01-01", "01-02", "01-03"]`,
		"line [10.00, 10.00, 10.00]",
		"- **drift**, small session, step `pages`: detail",
		"other.json",
		"| pages | 1.60 s | 800 ms | 800 ms | × 2.00 | × 1.00 | ▁▁█ |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("TREND.md lacks %q\n%s", want, md)
		}
	}
	for _, name := range stepNames {
		if !strings.Contains(md, "| "+name+" |") {
			t.Errorf("no row for step %s", name)
		}
	}
	if got := trendMarkdown(nil, nil, nil); !strings.Contains(got, "No records yet") {
		t.Errorf("empty history: %q", got)
	}
}

// A batch step is charted per statement, the figure the session exists for.
func TestTrendMarkdown_ABatchStepIsChartedPerStatement(t *testing.T) {
	recs := nights(2)
	for i := range recs {
		recs[i].Sessions[0].Size = "scale"
		st := stepOf(&recs[i], "open")
		st.Name, st.Statements = "exec-batch", 100
		st.Candidate.Wall.Median = 2000
	}
	md := trendMarkdown(recs, nil, nil)
	if !strings.Contains(md, `title "exec-batch, per statement (milliseconds)"`) || !strings.Contains(md, "line [20.00, 20.00]") {
		t.Fatalf("no per-statement chart:\n%s", md)
	}
}

func TestAlertsMarkdown_IsEmptyWithoutAlerts(t *testing.T) {
	recs := nights(2)
	if got := alertsMarkdown(recs, nil); got != "" {
		t.Fatalf("a body with nothing to report: %q", got)
	}
	got := alertsMarkdown(recs, []alert{{"drift", "small", "pages", "detail"}})
	if !strings.Contains(got, "step `pages`") || !strings.Contains(got, "mxcli c2") {
		t.Fatalf("body: %s", got)
	}
}

func TestSparkline(t *testing.T) {
	if got := sparkline([]float64{1, 2, 3}); got != "▁▄█" {
		t.Errorf("got %q", got)
	}
	if got := sparkline([]float64{5, 5}); got != "▄▄" {
		t.Errorf("flat: got %q", got)
	}
}
