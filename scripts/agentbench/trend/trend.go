// SPDX-License-Identifier: Apache-2.0

// Package main reads the benchmark's records — one JSON file per nightly run
// on the `benchmarks` branch — and writes what a single night cannot show: how
// each step has moved over the runs (TREND.md, trend.json) and whether
// anything in it should be looked at (the alerts).
//
// A night's own summary compares the candidate with the previous nightly. That
// catches a step that got 20% slower overnight and misses one that got 3%
// slower ten nights running. The trend is the other half.
//
// It decodes only the fields it uses, so a record written by a newer driver
// with more in it is still read. See
// docs-site/src/internals/agent-benchmark.md.
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A record is one run, as far as the trend reads it.
type record struct {
	Date      string    `json:"date"`
	Commit    string    `json:"commit"`
	Candidate string    `json:"candidate"`
	Baseline  string    `json:"baseline"`
	Sessions  []session `json:"sessions"`
	// file is where the record was read from, for messages.
	file string
}

type session struct {
	Size     string `json:"size"`
	Units    int    `json:"units"`
	Steps    []step `json:"steps"`
	Weighted struct {
		CandidateMS float64 `json:"candidate_ms"`
		Ratio       float64 `json:"ratio"`
	} `json:"weighted"`
}

type step struct {
	Name       string   `json:"name"`
	Statements int      `json:"statements"`
	Candidate  *arm     `json:"candidate"`
	Baseline   *arm     `json:"baseline"`
	WallRatio  float64  `json:"wall_ratio"`
	Flags      []string `json:"flags"`
}

type arm struct {
	Wall struct {
		Median float64 `json:"median"`
	} `json:"wall_ms"`
	Counts *struct {
		UnitFileReads int64  `json:"unit_file_reads"`
		UnitStoreHits *int64 `json:"unit_store_hits"`
		WritesWritten int    `json:"writes_written"`
	} `json:"counts"`
	FilesChanged int `json:"files_changed"`
}

func (s step) flagged(flag string) bool {
	for _, f := range s.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

// loadRecords reads every .json file under dir, oldest first. A file that is
// not a record is skipped and named in skipped: one bad file must not hide
// the history around it.
func loadRecords(dir string) (records []record, skipped []string, err error) {
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var r record
		if json.Unmarshal(data, &r) != nil || r.Date == "" || len(r.Sessions) == 0 {
			skipped = append(skipped, filepath.Base(path))
			return nil
		}
		r.file = filepath.Base(path)
		records = append(records, r)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	// RFC 3339 in UTC sorts as text.
	sort.SliceStable(records, func(i, j int) bool { return records[i].Date < records[j].Date })
	sort.Strings(skipped)
	return records, skipped, nil
}

// A point is one step in one run.
type point struct {
	Date   string  `json:"date"`
	Commit string  `json:"commit"`
	WallMS float64 `json:"wall_ms"`
	// Ratio is that night's candidate/baseline; 0 when there was no baseline.
	Ratio float64 `json:"ratio,omitempty"`
}

// A series is one step of one session across the runs that have it.
type series struct {
	Session    string  `json:"session"`
	Step       string  `json:"step"`
	Statements int     `json:"statements,omitempty"`
	Points     []point `json:"points"`
}

// weightedStep is the name the session-weighted total is charted under.
const weightedStep = "(session-weighted total)"

// buildSeries turns the records into one series per session and step, in the
// order the latest record lists them, with the session-weighted total first.
func buildSeries(records []record) []series {
	index := map[string]int{}
	var out []series
	at := func(sess, name string, statements int) *series {
		key := sess + "\x00" + name
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, series{Session: sess, Step: name})
		}
		if statements > 0 {
			out[i].Statements = statements
		}
		return &out[i]
	}
	// Walk newest first so the order is the latest record's, then reverse the
	// points of each series.
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		for _, s := range r.Sessions {
			if s.Weighted.CandidateMS > 0 {
				p := at(s.Size, weightedStep, 0)
				p.Points = append(p.Points, point{Date: r.Date, Commit: short(r.Commit), WallMS: s.Weighted.CandidateMS, Ratio: s.Weighted.Ratio})
			}
			for _, st := range s.Steps {
				if st.Candidate == nil {
					continue
				}
				p := at(s.Size, st.Name, st.Statements)
				p.Points = append(p.Points, point{Date: r.Date, Commit: short(r.Commit), WallMS: st.Candidate.Wall.Median, Ratio: st.WallRatio})
			}
		}
	}
	for i := range out {
		pts := out[i].Points
		for l, r := 0, len(pts)-1; l < r; l, r = l+1, r-1 {
			pts[l], pts[r] = pts[r], pts[l]
		}
	}
	return out
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// thresholds are what an alert is judged by.
type thresholds struct {
	// slower is the night's candidate/baseline wall ratio above which a step
	// counts as slower that night.
	slower float64
	// drift is how far the median of the last driftRecent runs may sit above
	// the median of the driftBefore runs before them.
	drift float64
	// minMS keeps the drift rule off steps too short to time: a 60 ms step
	// moves by a third between two runners.
	minMS float64
}

const (
	driftRecent = 3
	driftBefore = 7
)

// An alert is one thing in the records worth a person's look.
type alert struct {
	Kind    string `json:"kind"`
	Session string `json:"session"`
	Step    string `json:"step"`
	Detail  string `json:"detail"`
}

// findAlerts judges the latest record against the ones before it.
//
//   - slower-two-runs: slower than its baseline in the latest run and in the
//     one before. One night is noise often enough that it is not reported.
//   - drift: the last three runs' median above the seven before them. This is
//     the slow regression no single night's comparison shows. It compares
//     times from different nights, and so from different runners, which is
//     why each run is first corrected for its runner (runnerFactors); it
//     needs ten runs and a step long enough to time.
//   - counts-increased: the candidate read more unit files, or landed more
//     writes, than the baseline on the same night for the same step. Counts
//     carry no noise, so the first night is reported.
//   - wrote-on-re-exec: a settled script changed a unit file. That is a
//     defect, not a slowdown.
func findAlerts(records []record, t thresholds) []alert {
	if len(records) == 0 {
		return nil
	}
	latest := records[len(records)-1]
	var prev *record
	if len(records) > 1 {
		prev = &records[len(records)-2]
	}
	var out []alert
	for _, s := range latest.Sessions {
		for _, st := range s.Steps {
			if st.Candidate == nil {
				continue
			}
			if st.flagged("wrote-on-re-exec") {
				out = append(out, alert{"wrote-on-re-exec", s.Size, st.Name,
					fmt.Sprintf("a settled script changed %d unit file(s)", st.Candidate.FilesChanged)})
			}
			if st.Baseline != nil && st.Candidate.Counts != nil && st.Baseline.Counts != nil {
				c, b := st.Candidate.Counts, st.Baseline.Counts
				// A re-read of a unit is served from the persisted store, not
				// its file, so where both arms report store hits the reads
				// compared are the two together.
				if c.UnitStoreHits != nil && b.UnitStoreHits != nil {
					if cr, br := c.UnitFileReads+*c.UnitStoreHits, b.UnitFileReads+*b.UnitStoreHits; cr > br {
						out = append(out, alert{"counts-increased", s.Size, st.Name,
							fmt.Sprintf("units read from a file or the unit store: %d, the baseline %d", cr, br)})
					}
				} else if c.UnitFileReads > b.UnitFileReads {
					out = append(out, alert{"counts-increased", s.Size, st.Name,
						fmt.Sprintf("unit files read: %d, the baseline %d", c.UnitFileReads, b.UnitFileReads)})
				}
				if c.WritesWritten > b.WritesWritten {
					out = append(out, alert{"counts-increased", s.Size, st.Name,
						fmt.Sprintf("writes landed: %d, the baseline %d", c.WritesWritten, b.WritesWritten)})
				}
			}
			if st.Baseline != nil && st.WallRatio > t.slower && prev != nil {
				if p, ok := prev.step(s.Size, st.Name); ok && p.Baseline != nil && p.WallRatio > t.slower {
					out = append(out, alert{"slower-two-runs", s.Size, st.Name,
						fmt.Sprintf("× %.2f against its baseline in the latest run and × %.2f in the one before (threshold × %.2f)", st.WallRatio, p.WallRatio, t.slower)})
				}
			}
		}
	}
	all := buildSeries(records)
	factor := runnerFactors(all, records)
	for _, sr := range all {
		if sr.Step == weightedStep || len(sr.Points) < driftRecent+driftBefore {
			continue
		}
		// Only a step the latest record still has.
		if sr.Points[len(sr.Points)-1].Date != latest.Date {
			continue
		}
		var recent, before []float64
		n := len(sr.Points)
		for _, p := range sr.Points[n-driftRecent:] {
			recent = append(recent, p.WallMS/factor[p.Date])
		}
		for _, p := range sr.Points[n-driftRecent-driftBefore : n-driftRecent] {
			before = append(before, p.WallMS/factor[p.Date])
		}
		now, was := median(recent), median(before)
		if was >= t.minMS && now > was*t.drift {
			out = append(out, alert{"drift", sr.Session, sr.Step,
				fmt.Sprintf("median of the last %d runs %s, of the %d before them %s — × %.2f (threshold × %.2f), each run corrected for its runner",
					driftRecent, ms(now), driftBefore, ms(was), now/was, t.drift)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Session != out[j].Session {
			return out[i].Session < out[j].Session
		}
		return out[i].Step < out[j].Step
	})
	return out
}

// factorSteps is how many steps a run needs before its runner is judged from
// them; with fewer, one step's change would be read as the machine's.
const factorSteps = 5

// runnerFactors estimates, for each of the last driftRecent+driftBefore runs,
// how fast its runner was against the others: the median, over every step,
// of that run's time divided by the step's median over those runs. Hosted
// runners differ by a quarter from one night to the next, and that moves
// every step together; a regression moves the steps that run the changed
// code. Dividing a time by its run's factor leaves the second.
//
// What it cannot see is a change that slows every step alike — that is
// indistinguishable from a slow machine in these numbers. The night's own
// comparison, two binaries on one runner, is what catches that.
func runnerFactors(all []series, records []record) map[string]float64 {
	window := records
	if len(window) > driftRecent+driftBefore {
		window = window[len(window)-driftRecent-driftBefore:]
	}
	in := map[string]bool{}
	factor := map[string]float64{}
	for _, r := range records {
		factor[r.Date] = 1
	}
	for _, r := range window {
		in[r.Date] = true
	}
	ratios := map[string][]float64{}
	for _, sr := range all {
		if sr.Step == weightedStep {
			continue
		}
		var walls []float64
		for _, p := range sr.Points {
			if in[p.Date] {
				walls = append(walls, p.WallMS)
			}
		}
		m := median(walls)
		if m <= 0 || len(walls) < 2 {
			continue
		}
		for _, p := range sr.Points {
			if in[p.Date] {
				ratios[p.Date] = append(ratios[p.Date], p.WallMS/m)
			}
		}
	}
	for date, v := range ratios {
		if f := median(v); len(v) >= factorSteps && f > 0 {
			factor[date] = f
		}
	}
	return factor
}

func (r *record) step(size, name string) (step, bool) {
	for _, s := range r.Sessions {
		if s.Size != size {
			continue
		}
		for _, st := range s.Steps {
			if st.Name == name && st.Candidate != nil {
				return st, true
			}
		}
	}
	return step{}, false
}

func ms(v float64) string {
	switch {
	case v >= 10000:
		return fmt.Sprintf("%.1f s", v/1000)
	case v >= 1000:
		return fmt.Sprintf("%.2f s", v/1000)
	case v < 10:
		return fmt.Sprintf("%.1f ms", v)
	}
	return fmt.Sprintf("%.0f ms", v)
}

// sparkline draws values as one character each, scaled between the smallest
// and the largest of them: the shape of a step's last runs in a table cell.
func sparkline(v []float64) string {
	const bars = "▁▂▃▄▅▆▇█"
	if len(v) == 0 {
		return ""
	}
	lo, hi := v[0], v[0]
	for _, x := range v {
		lo, hi = min(lo, x), max(hi, x)
	}
	var sb strings.Builder
	for _, x := range v {
		i := 3 // a flat line sits in the middle
		if hi > lo {
			i = int((x - lo) / (hi - lo) * 7.999)
		}
		sb.WriteString(string([]rune(bars)[i]))
	}
	return sb.String()
}
