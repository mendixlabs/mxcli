// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRootUnionCountsConcurrentRequestsOnce(t *testing.T) {
	ms := int64(time.Millisecond)
	spans := []Span{
		{SpanID: "a", Start: 0, End: 100 * ms},
		{SpanID: "b", Start: 50 * ms, End: 150 * ms}, // overlaps a
		{SpanID: "c", ParentID: "a", Start: 10 * ms, End: 90 * ms},
		{SpanID: "d", Start: 200 * ms, End: 250 * ms},
	}
	if got := rootUnionMs(spans); got != 200 {
		t.Errorf("server time = %v, want 200 (150 + 50)", got)
	}
}

func TestNormalizeSQL(t *testing.T) {
	got := NormalizeSQL("SELECT  * FROM t1 WHERE id IN (1, 2, 3) AND name = 'it''s' AND x = $1 AND y > 2.5")
	want := "SELECT * FROM t1 WHERE id IN (?…) AND name = ? AND x = $1 AND y > ?"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestPerformanceMedianOfWarmRuns(t *testing.T) {
	t0 := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	var recs []JourneyRecord
	var spans []Span
	// Run 0 is the cold one (900 ms); warm runs take 100, 300 and 200 ms.
	for run, dur := range []int{900, 100, 300, 200} {
		s := t0.Add(time.Duration(run) * 10 * time.Second)
		recs = append(recs, JourneyRecord{Journey: "Market", Step: "open", Run: run, Start: s.Format(time.RFC3339Nano),
			End: s.Add(2 * time.Second).Format(time.RFC3339Nano), BrowserMs: float64(dur + 50)})
		root := Span{SpanID: string(rune('a' + run)), Name: "POST /xas/", Start: s.Add(10 * time.Millisecond).UnixNano(),
			End: s.Add(time.Duration(10+dur) * time.Millisecond).UnixNano()}
		spans = append(spans, root,
			Span{SpanID: root.SpanID + "q", ParentID: root.SpanID, Name: "SELECT", Attrs: map[string]string{"db.statement": "SELECT 1"},
				Start: root.Start, End: root.Start + int64(5*time.Millisecond)})
	}
	d, err := CollectPerformance(PerformanceInput{Journeys: &JourneyLog{Records: recs}, Spans: spans})
	if err != nil {
		t.Fatal(err)
	}
	s := d.Steps[0]
	if s.Runs != 4 || s.WarmRuns != 3 || s.ServerMs != 200 || s.BrowserMs != 250 || s.Queries != 1 || s.DBMs != 5 {
		t.Errorf("step = %+v", s)
	}
	if len(s.Flame) != 2 || s.Flame[1].Depth != 1 || s.Flame[1].Kind != "db" {
		t.Errorf("flame = %+v", s.Flame)
	}

	// Before/after against itself renders arrows.
	env, _ := NewEnvelope("performance", "x", "now", "t", d, PerformanceMethod, PerformanceLimits, nil)
	d2, err := CollectPerformance(PerformanceInput{Journeys: &JourneyLog{Records: recs}, Spans: spans, Baseline: env})
	if err != nil {
		t.Fatal(err)
	}
	env2, _ := NewEnvelope("performance", "x", "now", "t", d2, PerformanceMethod, PerformanceLimits, nil)
	doc, _ := RenderPerformance(env2, nil)
	var md bytes.Buffer
	_ = RenderMarkdown(&md, doc)
	mustContain(t, md.String(), "200 → 200", "1 → 1", "does not say what data the run used")
}

func TestReadAutoExplainKeepsJIT(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pg.log")
	_ = os.WriteFile(p, []byte(`2026-11-13 10:00:01.123 UTC [1] LOG:  duration: 197.300 ms  plan:
	{
	  "Query Text": "SELECT * FROM shop$order WHERE status = $1",
	  "Query Parameters": "$1 = 'open'",
	  "Plan": {"Node Type": "Hash Join", "Plans": [{"Node Type": "Seq Scan", "Relation Name": "shop$order"}]},
	  "JIT": {"Functions": 9, "Timing": {"Generation": 2.1, "Total": 171.4}}
	}
2026-11-13 10:00:02.000 UTC [1] LOG:  checkpoint starting
`), 0o644)
	ps, err := ReadAutoExplain(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].JITMs != 171.4 || ps[0].DurationMs != 197.3 || ps[0].Relation != "shop$order" || ps[0].Params != "$1 = 'open'" {
		t.Errorf("%+v", ps)
	}
}
