// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// PerformanceInput is what the performance report measures from.
type PerformanceInput struct {
	Journeys *JourneyLog
	Spans    []Span
	Plans    []Plan
	Baseline *Envelope // an earlier performance report: the "before"
	// Candidates are static N+1 shapes from the catalog; nil without a project.
	Candidates []Candidate
	Top        int
}

// PerformanceData is the collected performance report.
type PerformanceData struct {
	Database          string      `json:"database,omitempty"`
	Notes             []string    `json:"notes,omitempty"`
	Steps             []StepStats `json:"steps"`
	BaselineGenerated string      `json:"baselineGenerated,omitempty"`
	Baseline          []StepStats `json:"baseline,omitempty"`
	Plans             []Plan      `json:"plans,omitempty"`
	Candidates        []Candidate `json:"candidates,omitempty"`
	CandidatesChecked bool        `json:"candidatesChecked"`
}

// StepStats is one journey step, measured as the median of its warm runs.
type StepStats struct {
	Journey       string       `json:"journey"`
	Step          string       `json:"step"`
	Runs          int          `json:"runs"`
	WarmRuns      int          `json:"warmRuns"`
	BrowserMs     float64      `json:"browserMs"`
	ServerMs      float64      `json:"serverMs"`
	DBMs          float64      `json:"dbMs"`
	Queries       int          `json:"queries"`
	TopMicroflows []NameMs     `json:"topMicroflows,omitempty"`
	Statements    []Statement  `json:"statements,omitempty"`
	WindowMs      float64      `json:"windowMs"`
	Flame         []FlameEntry `json:"flame,omitempty"`
}

func (s StepStats) key() string { return s.Journey + " / " + s.Step }

// NameMs is a name with a time.
type NameMs struct {
	Name  string  `json:"name"`
	Ms    float64 `json:"ms"`
	Count int     `json:"count"`
}

// Statement is a normalised SQL statement with how often it ran in a step.
type Statement struct {
	SQL   string  `json:"sql"`
	Count int     `json:"count"`
	Ms    float64 `json:"ms"`
}

// FlameEntry is a span placed on the representative run's flame chart.
type FlameEntry struct {
	Name  string  `json:"name"`
	Start float64 `json:"start"`
	Dur   float64 `json:"dur"`
	Depth int     `json:"depth"`
	Kind  string  `json:"kind"`
}

// Plan is one auto_explain entry.
type Plan struct {
	Time       string  `json:"time,omitempty"`
	DurationMs float64 `json:"durationMs"`
	JITMs      float64 `json:"jitMs,omitempty"`
	Node       string  `json:"node"`
	Relation   string  `json:"relation,omitempty"`
	Query      string  `json:"query"`
	Params     string  `json:"params,omitempty"`
	Step       string  `json:"step,omitempty"`
}

// Candidate is a static N+1 shape found in the model.
type Candidate struct {
	Kind   string `json:"kind"`
	Where  string `json:"where"`
	Detail string `json:"detail"`
}

// CollectPerformance measures each journey step from spans.
func CollectPerformance(in PerformanceInput) (*PerformanceData, error) {
	if in.Journeys == nil || len(in.Journeys.Records) == 0 {
		return nil, fmt.Errorf("no journey steps: pass --journeys (or --journeys-cmd) with one record per step and run")
	}
	if in.Top <= 0 {
		in.Top = 8
	}
	d := &PerformanceData{Database: in.Journeys.Database, Notes: in.Journeys.Notes, Plans: in.Plans,
		Candidates: in.Candidates, CandidatesChecked: in.Candidates != nil}
	type group struct {
		journey, step string
		recs          []JourneyRecord
	}
	var order []string
	groups := map[string]*group{}
	for _, r := range in.Journeys.Records {
		step := nonEmpty(r.Step, r.Journey)
		k := r.Journey + "\x00" + step
		g := groups[k]
		if g == nil {
			g = &group{journey: r.Journey, step: step}
			groups[k] = g
			order = append(order, k)
		}
		g.recs = append(g.recs, r)
	}
	for _, k := range order {
		g := groups[k]
		recs := g.recs
		sort.SliceStable(recs, func(i, j int) bool { return recs[i].Run < recs[j].Run })
		// The first run warms caches, the JIT and connection pools: it is
		// measured but not counted, unless it is the only one.
		warm := recs
		if len(recs) > 1 {
			warm = recs[1:]
		}
		type run struct {
			browser, server, db float64
			queries             int
			spans               []Span
			s, e                int64
		}
		var runs []run
		for _, r := range warm {
			s, e := r.window()
			sp := spansIn(in.Spans, s, e)
			rr := run{s: s, e: e, spans: sp, browser: r.BrowserMs}
			if rr.browser == 0 {
				rr.browser = float64(e-s) / 1e6
			}
			rr.server = rootUnionMs(sp)
			for _, x := range sp {
				if x.IsDB() {
					rr.db += x.DurationMs()
					rr.queries++
				}
			}
			runs = append(runs, rr)
		}
		st := StepStats{Journey: g.journey, Step: g.step, Runs: len(recs), WarmRuns: len(warm)}
		var bs, ss, ds, qs []float64
		for _, r := range runs {
			bs, ss, ds, qs = append(bs, r.browser), append(ss, r.server), append(ds, r.db), append(qs, float64(r.queries))
		}
		st.BrowserMs, st.ServerMs, st.DBMs, st.Queries = round1(median(bs)), round1(median(ss)), round1(median(ds)), int(median(qs)+0.5)
		// The representative run is the one closest to the median server time.
		rep := 0
		for i, r := range runs {
			if abs(r.server-st.ServerMs) < abs(runs[rep].server-st.ServerMs) {
				rep = i
			}
		}
		if len(runs) > 0 {
			r := runs[rep]
			st.WindowMs = round1(float64(r.e-r.s) / 1e6)
			st.TopMicroflows = topMicroflows(r.spans, in.Top)
			st.Statements = topStatements(r.spans, in.Top)
			st.Flame = flame(r.spans, r.s, 3000)
		}
		d.Steps = append(d.Steps, st)
	}
	// Plans belong to the step whose window holds their log time.
	for i := range d.Plans {
		t, err := time.Parse("2006-01-02 15:04:05.000 MST", d.Plans[i].Time)
		if err != nil {
			continue
		}
		for _, r := range in.Journeys.Records {
			s, e := r.window()
			if t.UnixNano() >= s && t.UnixNano() <= e+int64(time.Second) {
				d.Plans[i].Step = r.Journey + " / " + nonEmpty(r.Step, r.Journey)
				break
			}
		}
	}
	sort.SliceStable(d.Plans, func(i, j int) bool { return d.Plans[i].DurationMs > d.Plans[j].DurationMs })
	if in.Baseline != nil {
		var b PerformanceData
		if err := in.Baseline.Decode(&b); err != nil {
			return nil, fmt.Errorf("baseline: %w", err)
		}
		d.BaselineGenerated = in.Baseline.Generated
		for _, s := range b.Steps {
			s.Flame, s.Statements, s.TopMicroflows = nil, nil, nil
			d.Baseline = append(d.Baseline, s)
		}
	}
	return d, nil
}

func spansIn(spans []Span, s, e int64) []Span {
	var out []Span
	for _, x := range spans {
		if x.Start >= s && x.Start <= e {
			out = append(out, x)
		}
	}
	return out
}

// rootUnionMs is the time any request was being served: the union of the
// intervals of spans whose parent is not in the set. Summing them would count
// concurrent requests twice.
func rootUnionMs(spans []Span) float64 {
	ids := map[string]bool{}
	for _, s := range spans {
		ids[s.SpanID] = true
	}
	var iv [][2]int64
	for _, s := range spans {
		if s.ParentID == "" || !ids[s.ParentID] {
			iv = append(iv, [2]int64{s.Start, s.End})
		}
	}
	sort.Slice(iv, func(i, j int) bool { return iv[i][0] < iv[j][0] })
	var total int64
	var cs, ce int64 = -1, -1
	for _, x := range iv {
		if x[0] > ce {
			if ce > cs {
				total += ce - cs
			}
			cs, ce = x[0], x[1]
		} else if x[1] > ce {
			ce = x[1]
		}
	}
	if ce > cs {
		total += ce - cs
	}
	return float64(total) / 1e6
}

func topMicroflows(spans []Span, n int) []NameMs {
	by := map[string]*NameMs{}
	for _, s := range spans {
		mf := s.Microflow()
		if mf == "" {
			continue
		}
		x := by[mf]
		if x == nil {
			x = &NameMs{Name: mf}
			by[mf] = x
		}
		x.Ms += s.DurationMs()
		x.Count++
	}
	var out []NameMs
	for _, x := range by {
		x.Ms = round1(x.Ms)
		out = append(out, *x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ms > out[j].Ms })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

var (
	sqlString = regexp.MustCompile(`'(?:[^']|'')*'`)
	// A number not preceded by $ (a bind placeholder, kept) or a word
	// character (part of a name such as t1).
	sqlNumber = regexp.MustCompile(`(^|[^$\w])\d+(\.\d+)?\b`)
	sqlInList = regexp.MustCompile(`\(\s*\?(\s*,\s*\?)+\s*\)`)
	sqlSpace  = regexp.MustCompile(`\s+`)
)

// NormalizeSQL replaces literals so the same statement with different
// arguments groups as one — the shape an N+1 has.
func NormalizeSQL(s string) string {
	s = sqlString.ReplaceAllString(s, "?")
	s = sqlNumber.ReplaceAllString(s, "${1}?")
	s = sqlInList.ReplaceAllString(s, "(?…)")
	return strings.TrimSpace(sqlSpace.ReplaceAllString(s, " "))
}

func topStatements(spans []Span, n int) []Statement {
	by := map[string]*Statement{}
	for _, s := range spans {
		if !s.IsDB() {
			continue
		}
		k := NormalizeSQL(s.Statement())
		x := by[k]
		if x == nil {
			x = &Statement{SQL: k}
			by[k] = x
		}
		x.Count++
		x.Ms += s.DurationMs()
	}
	var out []Statement
	for _, x := range by {
		x.Ms = round1(x.Ms)
		out = append(out, *x)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Ms > out[j].Ms
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// flame places a run's spans: depth from the span tree, and concurrent
// requests packed into separate bands so they do not overdraw each other.
func flame(spans []Span, windowStart int64, limit int) []FlameEntry {
	ids := map[string]Span{}
	children := map[string][]Span{}
	for _, s := range spans {
		ids[s.SpanID] = s
	}
	var roots []Span
	for _, s := range spans {
		if s.ParentID == "" || ids[s.ParentID].SpanID == "" {
			roots = append(roots, s)
		} else {
			children[s.ParentID] = append(children[s.ParentID], s)
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Start < roots[j].Start })
	var depthOf func(s Span) int
	depthOf = func(s Span) int {
		d := 0
		for _, c := range children[s.SpanID] {
			d = max(d, depthOf(c)+1)
		}
		return d
	}
	type band struct {
		end    int64
		height int
	}
	var bands []band
	bandOf := make([]int, len(roots))
	for i, r := range roots {
		h := depthOf(r) + 1
		placed := false
		for b := range bands {
			if bands[b].end <= r.Start {
				bands[b].end = r.End
				bands[b].height = max(bands[b].height, h)
				bandOf[i] = b
				placed = true
				break
			}
		}
		if !placed {
			bands = append(bands, band{end: r.End, height: h})
			bandOf[i] = len(bands) - 1
		}
	}
	offset := make([]int, len(bands))
	for b := 1; b < len(bands); b++ {
		offset[b] = offset[b-1] + bands[b-1].height
	}
	var out []FlameEntry
	var walk func(s Span, depth int)
	walk = func(s Span, depth int) {
		if len(out) >= limit {
			return
		}
		kind := "other"
		switch {
		case s.Microflow() != "":
			kind = "microflow"
		case s.IsDB():
			kind = "db"
		case depth == 0 || s.Kind == 2:
			kind = "request"
		}
		out = append(out, FlameEntry{Name: s.Name, Start: round1(float64(s.Start-windowStart) / 1e6), Dur: round1(s.DurationMs()), Depth: depth, Kind: kind})
		for _, c := range children[s.SpanID] {
			walk(c, depth+1)
		}
	}
	for i, r := range roots {
		walk(r, offset[bandOf[i]])
	}
	return out
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// ---------------------------------------------------------------- auto_explain

var explainHead = regexp.MustCompile(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d(?:\.\d+)? \w+).*duration: ([\d.]+) ms\s+plan:\s*(.*)$`)

// ReadAutoExplain reads plans from a Postgres log written with auto_explain in
// JSON format (auto_explain.log_format = json). The JIT block is kept: on the
// prototype, 171 of a "slow query's" 197 ms was JIT compiling a plan whose scan
// took 28 ms (ako/mxcli#2225).
func ReadAutoExplain(path string) ([]Plan, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var out []Plan
	var cur *Plan
	var buf strings.Builder
	depth := 0
	finish := func() {
		if cur == nil {
			return
		}
		parsePlanJSON(cur, buf.String())
		out = append(out, *cur)
		cur = nil
		buf.Reset()
		depth = 0
	}
	for sc.Scan() {
		line := sc.Text()
		if m := explainHead.FindStringSubmatch(line); m != nil {
			finish()
			cur = &Plan{Time: m[1], DurationMs: atof(m[2])}
			line = m[3]
		} else if cur == nil {
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if depth <= 0 && strings.Contains(buf.String(), "{") {
			finish()
		}
	}
	finish()
	return out, sc.Err()
}

func parsePlanJSON(p *Plan, s string) {
	var doc struct {
		QueryText   string         `json:"Query Text"`
		QueryParams string         `json:"Query Parameters"`
		Plan        map[string]any `json:"Plan"`
		JIT         struct {
			Timing struct {
				Total any `json:"Total"`
			} `json:"Timing"`
		} `json:"JIT"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &doc); err != nil {
		p.Query = strings.TrimSpace(s)
		return
	}
	p.Query, p.Params = doc.QueryText, doc.QueryParams
	p.Node, _ = doc.Plan["Node Type"].(string)
	p.Relation, _ = doc.Plan["Relation Name"].(string)
	if p.Relation == "" {
		p.Relation = firstRelation(doc.Plan)
	}
	switch t := doc.JIT.Timing.Total.(type) {
	case float64:
		p.JITMs = round1(t)
	case map[string]any: // PG ≥ 16 reports {"Total": {"Generation": …}}? keep the sum
		var sum float64
		for _, v := range t {
			if f, ok := v.(float64); ok {
				sum += f
			}
		}
		p.JITMs = round1(sum)
	}
}

func firstRelation(n map[string]any) string {
	if r, ok := n["Relation Name"].(string); ok && r != "" {
		return r
	}
	if kids, ok := n["Plans"].([]any); ok {
		for _, k := range kids {
			if m, ok := k.(map[string]any); ok {
				if r := firstRelation(m); r != "" {
					return r
				}
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------- static N+1

// CollectCandidates finds two N+1 shapes no index fixes (ako/mxcli#1263): a
// retrieve inside a loop, and a page data source microflow that returns a whole
// list (the runtime checks read access per returned object).
func CollectCandidates(q Querier, scope Scope) ([]Candidate, error) {
	out := []Candidate{}
	app := sqlList(scope.App)
	loops, err := rows(q, `SELECT MicroflowQualifiedName, COALESCE(EntityRef, '') AS EntityRef, RetrieveSource, LoopDepth
		FROM activities WHERE LoopDepth > 0 AND RetrieveSource IN ('association', 'database') AND ModuleName IN (`+app+`)
		ORDER BY MicroflowQualifiedName`)
	if err != nil {
		return nil, err
	}
	for _, r := range loops {
		out = append(out, Candidate{Kind: "retrieve in loop", Where: r["MicroflowQualifiedName"],
			Detail: fmt.Sprintf("%s retrieve of %s inside a loop (depth %s): one query per iteration — about five per row by association, counting access checks",
				r["RetrieveSource"], nonEmpty(r["EntityRef"], "an entity"), r["LoopDepth"])})
	}
	ds, err := rows(q, `SELECT r.SourceName AS Page, r.TargetName AS Microflow, m.ReturnType FROM refs r
		JOIN microflows m ON m.QualifiedName = r.TargetName
		WHERE r.RefKind = 'datasource' AND r.TargetType = 'MICROFLOW' AND m.ReturnType LIKE 'List%'
		AND m.ModuleName IN (`+app+`) ORDER BY r.TargetName`)
	if err != nil {
		return nil, err
	}
	for _, r := range ds {
		out = append(out, Candidate{Kind: "list data source", Where: r["Microflow"],
			Detail: fmt.Sprintf("data source of %s returns %s: the runtime checks read access per returned object, so N objects cost N queries. Filter or page in the database instead", r["Page"], r["ReturnType"])})
	}
	return out, nil
}

// ---------------------------------------------------------------- render

var PerformanceMethod = []string{
	"Each journey step is the time window its journey log record gives: from the action until its last `/xas/` request answered and nothing new started for a short quiet period (the journey script decides). Every span that starts inside the window belongs to the step.",
	"Browser time is the record's `browserMs` (or the window length). Server time is the union of the step's root spans — concurrent requests are not counted twice. Database time and query count are the database spans (`db.system` / `db.statement`) in the window.",
	"Each step ran several times; the first run warms caches and the JIT and is not counted. Figures are the median of the warm runs; the flame chart and statements are from the run closest to the median.",
	"Plans come from Postgres `auto_explain` in JSON format (`--plans <postgres log>`), with the JIT block reported next to the plan.",
	"Before/after compares against `--baseline`, an earlier performance report of the same journeys on the same seed.",
}

var PerformanceLimits = []string{
	"Results are only as representative as the seed: volume, distribution and dates. Seed dates relative to today — a seed dated from a fixed day made a \"revertible\" list about 120× too long and led to a wrong conclusion until checked.",
	"A fair before/after needs the committed model as the \"before\", on the same seed: revert the change, restart and measure, rather than reuse an older run.",
	"Access rules steer plans: an index on a status column can go unused because the access-rule subquery drives from an association, and a Mendix index cannot include an association. Such a finding needs a model change, not another index.",
	"Spans show the runtime's work, not the browser's rendering; the difference between browser and server time is network and client.",
	"Static N+1 candidates are shapes, not measurements: confirm each in a step's query count before changing it.",
}

// RenderPerformance builds the performance page.
func RenderPerformance(env *Envelope, ann *Annotations) (Doc, error) {
	var d PerformanceData
	if err := env.Decode(&d); err != nil {
		return Doc{}, err
	}
	base := map[string]StepStats{}
	for _, s := range d.Baseline {
		base[s.key()] = s
	}
	var server, db float64
	queries := 0
	for _, s := range d.Steps {
		server += s.ServerMs
		db += s.DBMs
		queries += s.Queries
	}
	jit := 0
	for _, p := range d.Plans {
		if p.JITMs > 0 && p.JITMs >= 0.3*p.DurationMs {
			jit++
		}
	}
	doc := Doc{Title: "Performance", Subtitle: "Where time goes in the main journeys at production volume"}
	doc.Stats = []Stat{
		{Label: "steps measured", Value: fmt.Sprint(len(d.Steps))},
		{Label: "server time, all steps", Value: fmt.Sprintf("%.0f ms", server)},
		{Label: "database time", Value: fmt.Sprintf("%.0f ms", db), Note: fmt.Sprintf("%d queries", queries)},
		{Label: "plans dominated by JIT", Value: fmt.Sprint(jit), Note: fmt.Sprintf("of %d", len(d.Plans))},
	}
	var env0 []Block
	if d.Database != "" {
		env0 = append(env0, Callout{Kind: "info", Text: "Database: " + d.Database})
	} else {
		env0 = append(env0, Callout{Kind: "warn", Text: "The journey log does not say what data the run used. Record the seed (volume, date) with a `{\"meta\":true,\"database\":\"…\"}` line; without it the numbers cannot be compared."})
	}
	if len(d.Notes) > 0 {
		env0 = append(env0, Para{Text: "Changes made for the measurement (and restored afterwards):"}, Bullets{Items: d.Notes})
	}

	// Steps table, with before/after when there is a baseline.
	cols := []string{"Journey / step", "Runs", "Browser ms", "Server ms", "DB ms", "Queries"}
	if len(d.Baseline) > 0 {
		cols = []string{"Journey / step", "Browser ms", "Server ms", "DB ms", "Queries"}
	}
	st := Table{Columns: append(cols, "Note")}
	for _, s := range d.Steps {
		note := ""
		if n, ok := ann.Get("step:" + s.key()); ok {
			note = joinNote(n.Status, n.Note, "")
		}
		if len(d.Baseline) == 0 {
			st.Rows = append(st.Rows, []string{s.key(), fmt.Sprintf("%d (%d warm)", s.Runs, s.WarmRuns), ms(s.BrowserMs), ms(s.ServerMs), ms(s.DBMs), fmt.Sprint(s.Queries), note})
			st.Classes = append(st.Classes, []string{"", "num", "num", "num", "num", "num", ""})
			continue
		}
		b, ok := base[s.key()]
		if !ok {
			st.Rows = append(st.Rows, []string{s.key(), "– → " + ms(s.BrowserMs), "– → " + ms(s.ServerMs), "– → " + ms(s.DBMs), fmt.Sprintf("– → %d", s.Queries), joinNote("", note, "not in the baseline")})
			st.Classes = append(st.Classes, []string{"", "num", "num", "num", "num", ""})
			continue
		}
		st.Rows = append(st.Rows, []string{s.key(), arrow(b.BrowserMs, s.BrowserMs), arrow(b.ServerMs, s.ServerMs), arrow(b.DBMs, s.DBMs),
			fmt.Sprintf("%d → %d", b.Queries, s.Queries), note})
		st.Classes = append(st.Classes, []string{"", deltaClass(b.BrowserMs, s.BrowserMs), deltaClass(b.ServerMs, s.ServerMs), deltaClass(b.DBMs, s.DBMs),
			deltaClass(float64(b.Queries), float64(s.Queries)), ""})
	}
	intro := "Median of the warm runs per step."
	if len(d.Baseline) > 0 {
		intro = "Before → after, median of the warm runs. Baseline generated " + d.BaselineGenerated + "."
	}
	doc.Sections = append(doc.Sections, Section{ID: "steps", Title: "Journey steps", Intro: intro, Blocks: append(env0, st)})

	// Per-step detail: flame chart, microflows, statements.
	for _, s := range d.Steps {
		var bl []Block
		if len(s.Flame) > 0 {
			fs := make([]flameSpan, len(s.Flame))
			for i, f := range s.Flame {
				fs[i] = flameSpan{Name: f.Name, Start: f.Start, Dur: f.Dur, Depth: f.Depth, Kind: f.Kind}
			}
			bl = append(bl, Figure{SVG: flameSVG(fs, s.WindowMs), Caption: "Spans of the representative run; concurrent requests in separate bands. Blue microflow, amber database, green request."})
		}
		if len(s.TopMicroflows) > 0 {
			mt := Table{Caption: "Microflows (inclusive time)", Columns: []string{"Microflow", "Calls", "ms"}}
			for _, m := range s.TopMicroflows {
				mt.Rows = append(mt.Rows, []string{"`" + m.Name + "`", fmt.Sprint(m.Count), ms(m.Ms)})
			}
			bl = append(bl, mt)
		}
		if len(s.Statements) > 0 {
			sq := Table{Caption: "Statements, most frequent first", Columns: []string{"SQL (literals replaced)", "Count", "ms"}}
			for _, x := range s.Statements {
				sq.Rows = append(sq.Rows, []string{"`" + truncate(x.SQL, 220) + "`", fmt.Sprint(x.Count), ms(x.Ms)})
			}
			bl = append(bl, sq)
		}
		if len(bl) == 0 {
			bl = append(bl, Para{Text: "No spans fell inside this step's window."})
		}
		doc.Sections = append(doc.Sections, Section{ID: "", Title: "Step: " + s.key(), Blocks: bl})
	}

	// Plans.
	pt := Table{Columns: []string{"ms", "JIT ms", "Node", "Relation", "Step", "Query"}}
	for i, p := range d.Plans {
		if i >= 25 {
			break
		}
		q := "`" + truncate(NormalizeSQL(p.Query), 240) + "`"
		if p.Params != "" {
			q += " — params: " + truncate(p.Params, 120)
		}
		pt.Rows = append(pt.Rows, []string{ms(p.DurationMs), ms(p.JITMs), p.Node, p.Relation, p.Step, q})
		jc := ""
		if p.JITMs > 0 && p.JITMs >= 0.3*p.DurationMs {
			jc = "warn"
		}
		pt.Classes = append(pt.Classes, []string{"num", jc, "", "", "", ""})
	}
	plansIntro := "Slowest statements as Postgres planned them. When JIT is a large share, the plan is cheap and compiling it is not: raise `jit_above_cost` or turn JIT off for this workload rather than add an index. auto_explain logs parameterised SQL with the parameters apart; bind them before re-running EXPLAIN."
	if len(d.Plans) == 0 {
		plansIntro = "No plans were given (`--plans <postgres log>` with auto_explain in JSON format)."
	}
	doc.Sections = append(doc.Sections, Section{ID: "plans", Title: "Query plans", Intro: plansIntro, Blocks: []Block{pt}})

	// Static candidates.
	ct := Table{Columns: []string{"Shape", "Where", "Detail", "Verdict"}}
	for _, c := range d.Candidates {
		v := ""
		if n, ok := ann.Get("candidate:" + c.Where); ok {
			v = joinNote(n.Status, n.Note, "")
		}
		ct.Rows = append(ct.Rows, []string{c.Kind, "`" + c.Where + "`", c.Detail, v})
	}
	cIntro := "Two N+1 shapes no index fixes, found in the model. Annotate `candidate:<Microflow>`."
	if !d.CandidatesChecked {
		cIntro = "Not checked: the report ran without a project."
	}
	doc.Sections = append(doc.Sections, Section{ID: "candidates", Title: "N+1 candidates in the model", Intro: cIntro, Blocks: []Block{ct}})
	return Frame(env, ann, doc), nil
}

func ms(f float64) string {
	if f == 0 {
		return "0"
	}
	if f < 10 {
		return fmt.Sprintf("%.1f", f)
	}
	return fmt.Sprintf("%.0f", f)
}

func arrow(before, after float64) string { return ms(before) + " → " + ms(after) }

func deltaClass(before, after float64) string {
	switch {
	case before == 0 && after == 0:
		return "num"
	case after <= before*0.8:
		return "ok"
	case after >= before*1.2:
		return "bad"
	}
	return "num"
}
