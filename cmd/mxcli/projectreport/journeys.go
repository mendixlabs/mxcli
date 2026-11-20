// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// JourneyRecord is one line of a journey log: a browser journey, or one step of
// it, with the time window it ran in. A journey script (Playwright or anything
// else) appends one line per step or per journey to the file named by
// $MXCLI_JOURNEY_LOG. The window is what ties runtime spans to the journey:
// every span that starts inside it was caused by it (the app should serve
// nothing else meanwhile).
//
//	{"journey":"Swap a shift","step":"open marketplace","run":2,
//	 "start":"2026-11-12T10:00:01.120Z","end":"2026-11-12T10:00:02.410Z",
//	 "status":"passed","checks":["sees 3 offers"],"browserMs":1290,
//	 "pages":["Shifts.Marketplace"]}
//
// A line {"meta":true,"database":"fresh seed 2026-11-12","notes":["…"]} records
// the state the run used, which the page states — journeys consume seed data,
// and a run on a used database can stop on its own precondition.
type JourneyRecord struct {
	Meta      bool     `json:"meta,omitempty"`
	Database  string   `json:"database,omitempty"`
	Notes     []string `json:"notes,omitempty"`
	Journey   string   `json:"journey,omitempty"`
	Step      string   `json:"step,omitempty"`
	Run       int      `json:"run,omitempty"`
	Start     string   `json:"start,omitempty"`
	End       string   `json:"end,omitempty"`
	Status    string   `json:"status,omitempty"`
	Checks    []string `json:"checks,omitempty"`
	Message   string   `json:"message,omitempty"`
	BrowserMs float64  `json:"browserMs,omitempty"`
	Pages     []string `json:"pages,omitempty"`
}

// JourneyLog is a parsed journey log.
type JourneyLog struct {
	Database string
	Notes    []string
	Records  []JourneyRecord
}

// ReadJourneyLog reads a JSONL journey log (or a JSON array of records).
func ReadJourneyLog(path string) (*JourneyLog, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var recs []JourneyRecord
	trimmed := strings.TrimSpace(string(b))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &recs); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	} else {
		sc := bufio.NewScanner(strings.NewReader(trimmed))
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		line := 0
		for sc.Scan() {
			line++
			t := strings.TrimSpace(sc.Text())
			if t == "" {
				continue
			}
			var r JourneyRecord
			if err := json.Unmarshal([]byte(t), &r); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, line, err)
			}
			recs = append(recs, r)
		}
	}
	log := &JourneyLog{}
	for _, r := range recs {
		if r.Meta {
			if r.Database != "" {
				log.Database = r.Database
			}
			log.Notes = append(log.Notes, r.Notes...)
			continue
		}
		if r.Journey == "" {
			return nil, fmt.Errorf("%s: a record without \"journey\": every line names the journey it belongs to", path)
		}
		if _, err := parseTime(r.Start); err != nil {
			return nil, fmt.Errorf("%s: journey %q: start: %w", path, r.Journey, err)
		}
		if _, err := parseTime(r.End); err != nil {
			return nil, fmt.Errorf("%s: journey %q: end: %w", path, r.Journey, err)
		}
		log.Records = append(log.Records, r)
	}
	return log, nil
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("missing")
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z07:00", "2006-01-02 15:04:05.000"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	// unix milliseconds, as Date.now() writes them
	var ms int64
	if _, err := fmt.Sscan(s, &ms); err == nil && ms > 1e11 {
		return time.UnixMilli(ms), nil
	}
	return time.Time{}, fmt.Errorf("unreadable time %q (want RFC 3339 or unix milliseconds)", s)
}

func (r JourneyRecord) window() (int64, int64) {
	s, _ := parseTime(r.Start)
	e, _ := parseTime(r.End)
	return s.UnixNano(), e.UnixNano()
}

// Journey is a journey with all its steps merged.
type Journey struct {
	Name     string
	Start    int64
	End      int64
	Status   string
	Checks   []string
	Messages []string
	Pages    []string
	Windows  [][2]int64
}

// Journeys merges a log's records per journey, in first-seen order. Status is
// failed if any record failed, else passed if any passed.
func (l *JourneyLog) Journeys() []Journey {
	idx := map[string]int{}
	var out []Journey
	for _, r := range l.Records {
		i, ok := idx[r.Journey]
		if !ok {
			i = len(out)
			idx[r.Journey] = i
			out = append(out, Journey{Name: r.Journey, Status: "passed"})
		}
		j := &out[i]
		s, e := r.window()
		if j.Start == 0 || s < j.Start {
			j.Start = s
		}
		j.End = max(j.End, e)
		j.Windows = append(j.Windows, [2]int64{s, e})
		switch strings.ToLower(r.Status) {
		case "failed", "fail", "error", "timedout":
			j.Status = "failed"
		case "skipped":
			if j.Status != "failed" && len(j.Windows) == 1 {
				j.Status = "skipped"
			}
		}
		for _, c := range r.Checks {
			if !containsStr(j.Checks, c) {
				j.Checks = append(j.Checks, c)
			}
		}
		if r.Message != "" {
			j.Messages = append(j.Messages, r.Message)
		}
		for _, p := range r.Pages {
			if !containsStr(j.Pages, p) {
				j.Pages = append(j.Pages, p)
			}
		}
	}
	return out
}

// MicroflowsIn returns the distinct microflows whose spans start inside any of
// the windows, sorted.
func MicroflowsIn(spans []Span, windows [][2]int64) []string {
	seen := map[string]bool{}
	for _, s := range spans {
		mf := s.Microflow()
		if mf == "" || seen[mf] {
			continue
		}
		for _, w := range windows {
			if s.Start >= w[0] && s.Start <= w[1] {
				seen[mf] = true
				break
			}
		}
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}
