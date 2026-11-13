// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// TestSource is a microflow test as written: what it calls and asserts.
type TestSource struct {
	Name       string   `json:"name"`
	File       string   `json:"file"`
	Calls      []string `json:"calls"`
	Assertions int      `json:"assertions"`
}

var callMicroflow = regexp.MustCompile(`(?i)\bcall\s+microflow\s+([A-Za-z_]\w*\.[A-Za-z_]\w*)`)

// CallsInMDL returns the microflows an MDL test body calls, in order, once each.
func CallsInMDL(mdl string, setups []string) []string {
	var out []string
	for _, s := range setups {
		if !containsStr(out, s) {
			out = append(out, s)
		}
	}
	for _, m := range callMicroflow.FindAllStringSubmatch(mdl, -1) {
		if !containsStr(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

// TestResult is one JUnit test case.
type TestResult struct {
	Name       string  `json:"name"`
	Class      string  `json:"class,omitempty"`
	File       string  `json:"file,omitempty"`
	Status     string  `json:"status"` // passed | failed | error | skipped
	Message    string  `json:"message,omitempty"`
	Seconds    float64 `json:"seconds"`
	Assertions int     `json:"assertions"`
}

// ReadJUnit reads a JUnit XML file, as `mxcli test --junit` writes it — with
// the per-test "assertions" property, so a passing test with no checks is
// distinguishable from one with six.
func ReadJUnit(path string) ([]TestResult, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	type prop struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	}
	type msg struct {
		Message string `xml:"message,attr"`
		Body    string `xml:",chardata"`
	}
	type tc struct {
		Name       string `xml:"name,attr"`
		Class      string `xml:"classname,attr"`
		File       string `xml:"file,attr"`
		Time       string `xml:"time,attr"`
		Properties []prop `xml:"properties>property"`
		Failure    *msg   `xml:"failure"`
		Error      *msg   `xml:"error"`
		Skipped    *msg   `xml:"skipped"`
	}
	type suite struct {
		Cases  []tc    `xml:"testcase"`
		Suites []suite `xml:"testsuite"`
	}
	var root suite
	if err := xml.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var out []TestResult
	var walk func(s suite)
	walk = func(s suite) {
		for _, c := range s.Cases {
			r := TestResult{Name: c.Name, Class: c.Class, File: c.File, Status: "passed"}
			r.Seconds, _ = strconv.ParseFloat(c.Time, 64)
			for _, p := range c.Properties {
				if p.Name == "assertions" {
					r.Assertions, _ = strconv.Atoi(p.Value)
				}
			}
			switch {
			case c.Failure != nil:
				r.Status, r.Message = "failed", firstLine(nonEmpty(c.Failure.Message, c.Failure.Body))
			case c.Error != nil:
				r.Status, r.Message = "error", firstLine(nonEmpty(c.Error.Message, c.Error.Body))
			case c.Skipped != nil:
				r.Status, r.Message = "skipped", firstLine(c.Skipped.Message)
			}
			out = append(out, r)
		}
		for _, sub := range s.Suites {
			walk(sub)
		}
	}
	walk(root)
	return out, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// TestsInput is everything the tests report reads besides the catalog.
type TestsInput struct {
	Scope        ScopeOptions
	Sources      []TestSource
	Results      []TestResult
	Journeys     *JourneyLog
	Spans        []Span
	StartupFlows []string // after startup, before shutdown, health check
}

// TestsData is the collected tests report.
type TestsData struct {
	Scope       Scope            `json:"scope"`
	HasResults  bool             `json:"hasResults"`
	HasJourneys bool             `json:"hasJourneys"`
	HasSpans    bool             `json:"hasSpans"`
	Database    string           `json:"database,omitempty"`
	Notes       []string         `json:"notes,omitempty"`
	Tests       []TestInfo       `json:"tests"`
	Journeys    []JourneyInfo    `json:"journeys"`
	Microflows  int              `json:"microflows"`
	Modules     []ModuleCoverage `json:"modules"`
	Totals      ModuleCoverage   `json:"totals"`
	Gaps        []ModuleGaps     `json:"gaps"`
	// Reach maps each app microflow to its category, for consumers (the
	// traceability report) that need more than the counts.
	Reach map[string]string `json:"reach"`
	// UnknownCalls are microflows a test calls that the catalog does not
	// have — renamed or deleted since the test was written.
	UnknownCalls []string `json:"unknownCalls,omitempty"`
}

// TestInfo is one microflow test with its result and reach.
type TestInfo struct {
	Name       string   `json:"name"`
	File       string   `json:"file,omitempty"`
	Status     string   `json:"status"`
	Message    string   `json:"message,omitempty"`
	Assertions int      `json:"assertions"`
	Seconds    float64  `json:"seconds,omitempty"`
	Calls      []string `json:"calls"`
	Reached    []string `json:"reached"`
}

// JourneyInfo is one browser journey with what it executed.
type JourneyInfo struct {
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Checks   []string `json:"checks"`
	Messages []string `json:"messages,omitempty"`
	Pages    []string `json:"pages,omitempty"`
	Executed []string `json:"executed"`
	// ExecutedOutside counts microflows outside the app's modules (Marketplace,
	// System) the journey ran; they are not part of the coverage figures.
	ExecutedOutside int `json:"executedOutside"`
}

// ModuleCoverage is one module's microflows by category.
type ModuleCoverage struct {
	Module       string `json:"module"`
	Total        int    `json:"total"`
	Both         int    `json:"both"`
	TestsOnly    int    `json:"testsOnly"`
	JourneysOnly int    `json:"journeysOnly"`
	Startup      int    `json:"startup"`
	Neither      int    `json:"neither"`
}

// ModuleGaps lists a module's microflows reached by nothing.
type ModuleGaps struct {
	Module     string   `json:"module"`
	Microflows []string `json:"microflows"`
}

// Coverage categories.
const (
	ReachBoth     = "both"
	ReachTests    = "tests only"
	ReachJourneys = "journeys only"
	ReachStartup  = "startup or scheduled"
	ReachNeither  = "neither"
)

// CallGraph is microflow → microflows it calls, from CATALOG.REFS.
type CallGraph map[string][]string

// LoadCallGraph reads the microflow call graph.
func LoadCallGraph(q Querier) (CallGraph, error) {
	rs, err := rows(q, `SELECT DISTINCT SourceName, TargetName FROM refs
		WHERE RefKind = 'call' AND SourceType = 'MICROFLOW' AND TargetType = 'MICROFLOW'`)
	if err != nil {
		return nil, err
	}
	g := CallGraph{}
	for _, r := range rs {
		g[r["SourceName"]] = append(g[r["SourceName"]], r["TargetName"])
	}
	return g, nil
}

// Closure returns roots plus everything they call, transitively, sorted.
func (g CallGraph) Closure(roots []string) []string {
	seen := map[string]bool{}
	stack := append([]string(nil), roots...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[n] {
			continue
		}
		seen[n] = true
		stack = append(stack, g[n]...)
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// CollectTests computes results and microflow reach per module.
func CollectTests(q Querier, in TestsInput) (*TestsData, error) {
	scope, err := LoadScope(q, in.Scope)
	if err != nil {
		return nil, err
	}
	d := &TestsData{Scope: scope, HasResults: len(in.Results) > 0, HasJourneys: in.Journeys != nil && len(in.Journeys.Records) > 0,
		HasSpans: len(in.Spans) > 0, Reach: map[string]string{}}
	if in.Journeys != nil {
		d.Database, d.Notes = in.Journeys.Database, in.Journeys.Notes
	}
	mfs, err := rows(q, `SELECT QualifiedName, ModuleName FROM microflows
		WHERE MicroflowType = 'MICROFLOW' AND ModuleName IN (`+sqlList(scope.App)+`) ORDER BY QualifiedName`)
	if err != nil {
		return nil, err
	}
	allMF, err := rows(q, `SELECT QualifiedName FROM microflows WHERE MicroflowType = 'MICROFLOW'`)
	if err != nil {
		return nil, err
	}
	exists := map[string]bool{}
	for _, r := range allMF {
		exists[r["QualifiedName"]] = true
	}
	g, err := LoadCallGraph(q)
	if err != nil {
		return nil, err
	}

	// Microflow tests: static reach.
	byTest := map[string]TestResult{}
	for _, r := range in.Results {
		byTest[r.Name] = r
	}
	byTests := map[string]bool{}
	seenSource := map[string]bool{}
	unknown := map[string]bool{}
	for _, s := range in.Sources {
		seenSource[s.Name] = true
		t := TestInfo{Name: s.Name, File: s.File, Calls: s.Calls, Assertions: s.Assertions, Status: "not run"}
		if r, ok := byTest[s.Name]; ok {
			t.Status, t.Message, t.Seconds = r.Status, r.Message, r.Seconds
			if r.Assertions > 0 || t.Assertions == 0 {
				t.Assertions = r.Assertions
			}
		}
		for _, c := range s.Calls {
			if !exists[c] {
				unknown[c] = true
			}
		}
		t.Reached = g.Closure(s.Calls)
		for _, m := range t.Reached {
			byTests[m] = true
		}
		d.Tests = append(d.Tests, t)
	}
	// Results with no source (a JUnit file from elsewhere) are still listed.
	for _, r := range in.Results {
		if !seenSource[r.Name] {
			d.Tests = append(d.Tests, TestInfo{Name: r.Name, File: r.File, Status: r.Status, Message: r.Message,
				Assertions: r.Assertions, Seconds: r.Seconds})
		}
	}
	for u := range unknown {
		d.UnknownCalls = append(d.UnknownCalls, u)
	}
	sort.Strings(d.UnknownCalls)

	// Journeys: runtime reach.
	app := setOf(scope.App)
	byJourneys := map[string]bool{}
	if in.Journeys != nil {
		for _, j := range in.Journeys.Journeys() {
			ji := JourneyInfo{Name: j.Name, Status: j.Status, Checks: j.Checks, Messages: j.Messages, Pages: j.Pages}
			for _, m := range MicroflowsIn(in.Spans, j.Windows) {
				if app[moduleOf(m)] {
					ji.Executed = append(ji.Executed, m)
					byJourneys[m] = true
				} else {
					ji.ExecutedOutside++
				}
			}
			d.Journeys = append(d.Journeys, ji)
		}
	}

	// Startup and scheduled: reached without any test, and not a gap.
	var roots []string
	roots = append(roots, in.StartupFlows...)
	se, err := rows(q, `SELECT COALESCE(Microflow, '') AS Microflow FROM scheduled_events WHERE COALESCE(Microflow, '') != ''`)
	if err != nil {
		return nil, err
	}
	for _, r := range se {
		roots = append(roots, r["Microflow"])
	}
	startup := setOf(g.Closure(roots))

	cov := map[string]*ModuleCoverage{}
	gaps := map[string][]string{}
	for _, r := range mfs {
		name, mod := r["QualifiedName"], r["ModuleName"]
		c := cov[mod]
		if c == nil {
			c = &ModuleCoverage{Module: mod}
			cov[mod] = c
		}
		c.Total++
		cat := ReachNeither
		switch {
		case byTests[name] && byJourneys[name]:
			cat, c.Both = ReachBoth, c.Both+1
		case byTests[name]:
			cat, c.TestsOnly = ReachTests, c.TestsOnly+1
		case byJourneys[name]:
			cat, c.JourneysOnly = ReachJourneys, c.JourneysOnly+1
		case startup[name]:
			cat, c.Startup = ReachStartup, c.Startup+1
		default:
			c.Neither++
			gaps[mod] = append(gaps[mod], name)
		}
		d.Reach[name] = cat
	}
	d.Microflows = len(mfs)
	for _, m := range scope.App {
		c, ok := cov[m]
		if !ok {
			continue
		}
		d.Modules = append(d.Modules, *c)
		d.Totals.Total += c.Total
		d.Totals.Both += c.Both
		d.Totals.TestsOnly += c.TestsOnly
		d.Totals.JourneysOnly += c.JourneysOnly
		d.Totals.Startup += c.Startup
		d.Totals.Neither += c.Neither
		if len(gaps[m]) > 0 {
			d.Gaps = append(d.Gaps, ModuleGaps{Module: m, Microflows: gaps[m]})
		}
	}
	return d, nil
}

// ---------------------------------------------------------------- render

var TestsMethod = []string{
	"Microflow test results and per-test assertion counts come from `mxcli test --junit` (the `assertions` property), run by this command with `--run` or read from `--junit`.",
	"Static reach: the microflows each `.test.mdl`/`.test.md` test calls (`call microflow` and its setups), plus everything those call in turn through the catalog's `call` references.",
	"Runtime reach: with `--journeys`, every `Microflow <name>` span that starts inside a journey's time window counts as executed by that journey. This is measured, not estimated — the app was running with `run --local --trace-otlp` into the span collector.",
	"\"Startup or scheduled\" is the after-startup, before-shutdown and health-check microflows and every scheduled event's microflow, with what they call. Those run without a test and are not counted as gaps.",
	"The app is the project's own modules; Marketplace modules, `System` and `MxTest` are outside.",
}

var TestsLimits = []string{
	"Static reach overstates: a microflow that a test enters may have branches no test takes. \"Reached\" means entered, not exercised.",
	"Nanoflows and page logic are not counted. A page whose data source is a database query runs no microflow, so journeys cannot be seen to reach it through spans.",
	"Microflows called from Java actions are invisible to the call graph (ako/mxcli#1819), so static reach misses them; runtime reach does not.",
	"Entity event handlers a test triggers by committing are not followed statically.",
	"Journeys consume seed data. A run on a database earlier runs used can stop on its own precondition; the database state is stated above when the journey log records it.",
}

// RenderTests builds the tests page.
func RenderTests(env *Envelope, ann *Annotations) (Doc, error) {
	var d TestsData
	if err := env.Decode(&d); err != nil {
		return Doc{}, err
	}
	passed, checks := 0, 0
	for _, t := range d.Tests {
		if t.Status == "passed" {
			passed++
		}
		checks += t.Assertions
	}
	jPassed, jChecks := 0, 0
	for _, j := range d.Journeys {
		if j.Status == "passed" {
			jPassed++
		}
		jChecks += len(j.Checks)
	}
	reached := d.Totals.Both + d.Totals.TestsOnly + d.Totals.JourneysOnly
	doc := Doc{Title: "Tests", Subtitle: "What the tests check, whether they passed, and which microflows they reach"}
	testsValue := fmt.Sprintf("%d/%d", passed, len(d.Tests))
	if !d.HasResults {
		testsValue = fmt.Sprintf("%d (not run)", len(d.Tests))
	}
	doc.Stats = []Stat{
		{Label: "microflow tests passed", Value: testsValue, Note: fmt.Sprintf("%d checks", checks)},
		{Label: "journeys passed", Value: fmt.Sprintf("%d/%d", jPassed, len(d.Journeys)), Note: fmt.Sprintf("%d checks", jChecks)},
		{Label: "microflows reached", Value: pct(reached, d.Microflows), Note: fmt.Sprintf("%d of %d", reached, d.Microflows)},
		{Label: "startup or scheduled only", Value: fmt.Sprint(d.Totals.Startup)},
		{Label: "reached by neither", Value: fmt.Sprint(d.Totals.Neither)},
	}
	var pre []Block
	if d.HasJourneys {
		if d.Database != "" {
			pre = append(pre, Callout{Kind: "info", Text: "Database state for the journeys: " + d.Database})
		} else {
			pre = append(pre, Callout{Kind: "warn", Text: "The journey log does not record which database state the run used. Journeys consume seed data; reseed before a full run and record it with a `{\"meta\":true,\"database\":\"…\"}` line."})
		}
		if d.HasJourneys && !d.HasSpans {
			pre = append(pre, Callout{Kind: "warn", Text: "Journeys ran but no spans were read, so runtime reach is empty. Run the app with `mxcli run --local --trace-otlp http://127.0.0.1:4318` while the collector listens."})
		}
	} else {
		pre = append(pre, Callout{Kind: "info", Text: "No journeys were given (`--journeys`), so coverage is static reach from microflow tests only."})
	}
	for _, n := range d.Notes {
		pre = append(pre, Callout{Kind: "info", Text: n})
	}

	// Coverage per module.
	var labels []string
	var bars [][]barSegment
	ct := Table{Columns: []string{"Module", "Microflows", "Both", "Tests only", "Journeys only", "Startup / scheduled", "Neither", "Reached"}}
	for _, m := range append(d.Modules, withName(d.Totals, "All app modules")) {
		labels = append(labels, m.Module)
		bars = append(bars, []barSegment{
			{ReachBoth, m.Both, "--ok"}, {ReachTests, m.TestsOnly, "--info"}, {ReachJourneys, m.JourneysOnly, "--accent"},
			{ReachStartup, m.Startup, "--muted"}, {ReachNeither, m.Neither, "--bad"},
		})
		ct.Rows = append(ct.Rows, []string{m.Module, fmt.Sprint(m.Total), fmt.Sprint(m.Both), fmt.Sprint(m.TestsOnly),
			fmt.Sprint(m.JourneysOnly), fmt.Sprint(m.Startup), fmt.Sprint(m.Neither), pct(m.Both+m.TestsOnly+m.JourneysOnly, m.Total)})
		ct.Classes = append(ct.Classes, []string{"", "num", "num", "num", "num", "num", "num", "num"})
	}
	doc.Sections = append(doc.Sections, Section{ID: "coverage", Title: "Microflow coverage per module",
		Intro:  "Each app microflow in one category: reached by both layers, by microflow tests only, by journeys only, run only at startup or on a schedule (not a gap), or by neither.",
		Blocks: append(pre, Figure{SVG: stackedBarsSVG(labels, bars)}, ct)})

	// Tests.
	tt := Table{Columns: []string{"Test", "Status", "Checks", "Calls", "Reaches", "Note"}}
	var noCalls []string
	for _, t := range d.Tests {
		note := t.Message
		if len(t.Calls) == 0 && t.Reached != nil {
			noCalls = append(noCalls, t.Name)
		}
		if t.Assertions == 0 && t.Status == "passed" {
			note = joinNote("", note, "passes without checking anything")
		}
		tt.Rows = append(tt.Rows, []string{t.Name, t.Status, fmt.Sprint(t.Assertions), strings.Join(codeList(t.Calls), ", "), fmt.Sprint(len(t.Reached)), note})
		tt.Classes = append(tt.Classes, []string{"", statusClass(t.Status), "num", "", "num", ""})
	}
	tb := []Block{tt}
	if len(noCalls) > 0 {
		tb = append(tb, Para{Text: "Tests that call no microflow (they only create, retrieve or assert) add nothing to reach:"}, Bullets{Items: noCalls})
	}
	if len(d.UnknownCalls) > 0 {
		tb = append(tb, Callout{Kind: "warn", Text: "Tests call microflows the model does not have (renamed or deleted?): " + strings.Join(codeList(d.UnknownCalls), ", ")})
	}
	doc.Sections = append(doc.Sections, Section{ID: "tests", Title: "Microflow tests", Blocks: tb})

	// Journeys.
	jt := Table{Columns: []string{"Journey", "Status", "Checks", "Microflows run", "What it checks"}}
	for _, j := range d.Journeys {
		what := strings.Join(j.Checks, "; ")
		if n, ok := ann.Get("journey:" + j.Name); ok {
			what = joinNote("", n.Note, what)
		}
		if j.Status == "failed" && len(j.Messages) > 0 {
			what = joinNote("failed", j.Messages[len(j.Messages)-1], what)
		}
		jt.Rows = append(jt.Rows, []string{j.Name, j.Status, fmt.Sprint(len(j.Checks)), fmt.Sprint(len(j.Executed)), what})
		jt.Classes = append(jt.Classes, []string{"", statusClass(j.Status), "num", "num", ""})
	}
	doc.Sections = append(doc.Sections, Section{ID: "journeys", Title: "Browser journeys",
		Intro:  "What each journey checks is taken from the journey script's own check messages, not written from its name — prose written from a name drifts from what the script verifies. Annotate `journey:<name>` to add context.",
		Blocks: []Block{jt}})

	// Gaps.
	gt := Table{Columns: []string{"Module", "Microflows reached by neither", "Verdict", "Key"}}
	for _, g := range d.Gaps {
		verdict := ""
		key := "gap:" + g.Module
		if n, ok := ann.Get(key); ok {
			verdict = joinNote(n.Status, n.Note, "")
		}
		gt.Rows = append(gt.Rows, []string{g.Module, strings.Join(codeList(g.Microflows), ", "), verdict, "`" + key + "`"})
	}
	doc.Sections = append(doc.Sections, Section{ID: "gaps", Title: "Gaps",
		Intro:  "Microflows no test and no journey reaches, by module. Not every gap deserves a test: give each group a verdict with `--annotations` (e.g. `needs a test`, `admin only`, `dead — remove`).",
		Blocks: []Block{gt}})
	return Frame(env, ann, doc), nil
}

func withName(c ModuleCoverage, name string) ModuleCoverage {
	c.Module = name
	return c
}

func pct(n, total int) string {
	if total == 0 {
		return "–"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(n)/float64(total))
}

func statusClass(s string) string {
	switch s {
	case "passed", "built", "fixed", "ok":
		return "ok"
	case "failed", "error", "not built", "high", "open":
		return "bad"
	case "skipped", "not run", "partial", "changed", "medium", "verify":
		return "warn"
	}
	return ""
}
