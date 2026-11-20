// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
	// Each median carries the min–max of its warm runs (ako/mxcli#2443).
	mustContain(t, md.String(), "200 (100–300) → 200 (100–300)", "250 (150–350)", "1 → 1", "does not say what data the run used")
	if s.ServerMin != 100 || s.ServerMax != 300 || s.BrowserMin != 150 || s.BrowserMax != 350 {
		t.Errorf("ranges: server %v–%v, browser %v–%v", s.ServerMin, s.ServerMax, s.BrowserMin, s.BrowserMax)
	}

	// Steps grouped by the finding that explains them: one card per finding,
	// with its steps' before → after (ako/mxcli#2443).
	doc, _ = RenderPerformance(env2, &Annotations{Items: map[string]Note{
		"finding:F1":         {Name: "Housekeeping held a long transaction", Status: "fixed", Note: "split into batches of 500"},
		"finding:F2":         {Name: "Menu query scans every order", Status: "open"},
		"finding:F3":         {Name: "Traffic generator lacked an index", Status: "fixed", Scope: "background"},
		"finding:F4":         {Name: "Payment waits on a simulated gateway", Status: "by design"},
		"step:Market / open": {Finding: "F1", Note: "F1: the step the fix targets"},
	}})
	md.Reset()
	_ = RenderMarkdown(&md, doc)
	mustContain(t, md.String(), "## Findings", "Housekeeping held a long transaction", "split into batches of 500",
		// DB time on the card, and a change inside the spread said to be one (ako/mxcli#2475)
		"Steps: Market / open: server 200 → 200 ms (within the run-to-run spread), DB 5.0 → 5.0 ms (no clear change), 1 → 1 queries.",
		"Menu query scans every order", "No step names this finding",
		// a background finding says so; a by-design one is not "open"
		"Background work: no journey step runs it.", "by design",
		// the row names the finding by name, and its ID once
		"finding F1 — Housekeeping held a long transaction: the step the fix targets",
		// no plans read: no "0 of 0"
		"no plans read (no Postgres log given)")
	if strings.Contains(md.String(), "F1: F1:") {
		t.Error("the finding ID is written twice")
	}

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

// ako/mxcli#2443: a step counts the traces its own HTTP requests started, not
// every span in its window. Measured on ako/PizzaV1: the simulator's queued
// clock tick (a 1.5 s sleep) made "place order and pay" read 4,884 ms of server
// time; the step's requests took 280 ms.
func TestPerformanceCountsOnlyTheStepsOwnRequests(t *testing.T) {
	t0 := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	at := func(ms int) int64 { return t0.Add(time.Duration(ms) * time.Millisecond).UnixNano() }
	http := func(port string) map[string]string {
		return map[string]string{"http.request.method": "POST", "server.port": port}
	}
	db := map[string]string{"db.statement": "SELECT 1", "db.system": "postgresql"}
	var spans []Span
	// Many app requests make 8080 the app port; one is in the window.
	for i := 0; i < 5; i++ {
		spans = append(spans, Span{TraceID: "warm" + string(rune('a'+i)), SpanID: "w" + string(rune('a'+i)), Name: "POST /xas/", Attrs: http("8080"), Start: at(-60000), End: at(-59990)})
	}
	spans = append(spans,
		// The step's request: 100 ms, one query, a child that outlasts the window.
		Span{TraceID: "req", SpanID: "r", Name: "POST /xas/", Attrs: http("8080"), Start: at(10), End: at(110)},
		Span{TraceID: "req", SpanID: "rq", ParentID: "r", Name: "SELECT", Attrs: db, Start: at(20), End: at(30)},
		// Background in the same window: a queued task with its own query,
		// an admin-port request and an orphan polling query.
		Span{TraceID: "bg", SpanID: "b", Name: "Call queued task Sim.Tick", Start: at(5), End: at(1505)},
		Span{TraceID: "bg", SpanID: "bq", ParentID: "b", Name: "SELECT", Attrs: db, Start: at(40), End: at(90)},
		Span{TraceID: "admin", SpanID: "a", Name: "POST /", Attrs: http("8090"), Start: at(50), End: at(60)},
		Span{TraceID: "poll", SpanID: "p", Name: "SELECT", Attrs: db, Start: at(70), End: at(75)},
	)
	recs := []JourneyRecord{{Journey: "Order", Step: "pay", Start: t0.Format(time.RFC3339Nano), End: t0.Add(200 * time.Millisecond).Format(time.RFC3339Nano)}}
	d, err := CollectPerformance(PerformanceInput{Journeys: &JourneyLog{Records: recs}, Spans: spans})
	if err != nil {
		t.Fatal(err)
	}
	s := d.Steps[0]
	if d.Attribution != AttributionRequests || s.ServerMs != 100 || s.DBMs != 10 || s.Queries != 1 {
		t.Errorf("only the step's request counts: attribution %q, %+v", d.Attribution, s)
	}
	// Background: the task (1500 ms), the admin request and the polling query
	// all start in the window; the union is the task's 1500 ms.
	if s.BackgroundMs != 1500 {
		t.Errorf("background = %v, want 1500", s.BackgroundMs)
	}
	env, _ := NewEnvelope("performance", "Shop", "now", "t", d, PerformanceMethod, PerformanceLimits, nil)
	doc, _ := RenderPerformance(env, nil)
	var b bytes.Buffer
	_ = RenderMarkdown(&b, doc)
	mustContain(t, b.String(), "Background work ran during some steps", "Order / pay: 1500 ms")

	// The control: spans without request attributes are attributed by window,
	// as before, and the page says so.
	var bare []Span
	for _, x := range spans {
		x.Attrs = nil
		bare = append(bare, x)
	}
	d, _ = CollectPerformance(PerformanceInput{Journeys: &JourneyLog{Records: recs}, Spans: bare})
	if d.Attribution != AttributionWindow || d.Steps[0].ServerMs < 1500 {
		t.Errorf("without request attributes every span in the window counts: %q %+v", d.Attribution, d.Steps[0])
	}
	env, _ = NewEnvelope("performance", "Shop", "now", "t", d, PerformanceMethod, PerformanceLimits, nil)
	doc, _ = RenderPerformance(env, nil)
	b.Reset()
	_ = RenderMarkdown(&b, doc)
	mustContain(t, b.String(), "No span carries HTTP request attributes")
}
