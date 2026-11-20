// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

func htmlOf(t *testing.T, d Doc) string {
	t.Helper()
	var b bytes.Buffer
	if err := RenderHTML(&b, d); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// ako/mxcli#2443: the shared look — a header naming project, report and date,
// the verdict as a lede, tiles with a status edge, bars in tables, stacked
// bars with a legend, findings as cards, links between report pages.
func TestSharedLookBlocks(t *testing.T) {
	d := Doc{Title: "Security review", Project: "Shop", Kind: "Security", Generated: "2026-11-19 10:00", Mxcli: "1.0",
		Lede:  "Nothing high is open.",
		Stats: []Stat{{Label: "open high", Value: "0", Status: "good", Note: "after triage"}},
		Nav:   []NavLink{{Label: "Overview", Href: "index.html"}, {Label: "Security", Href: "security.html", Current: true}},
		Sections: []Section{{Title: "S", Blocks: []Block{
			Table{Columns: []string{"Category", "Score", "N"}, Rows: [][]string{{"Quality", "40", "3"}},
				Classes: [][]string{{"", "", "num"}}, Bars: [][]*Bar{{nil, {Pct: 40, Status: "bad"}, nil}}},
			Stacked{Legend: []StackKey{{"built, tested", "good"}, {"<untested>", "s1"}}, Rows: []StackRow{{Label: "M1", Parts: []int{3, 1}, Value: "4/4"}}},
			Card{Tag: "HIGH", Status: "good", StatusText: "fixed", Title: "Caller-trusting flow `Shop.X`", Paras: []string{"<b>triage</b>"}},
		}}}}
	h := htmlOf(t, d)
	for _, want := range []string{
		`<span class="eyebrow">Shop · Security · 2026-11-19</span>`, `<p class="lede">Nothing high is open.</p>`,
		`<div class="kpi good">`, `<a href="security.html" aria-current="page">Security</a>`,
		`<div class="meter bad"><i style="width:40.0%"></i>`, `<th class="num">N</th>`,
		`background:var(--good)`, `background:var(--s1)`, `&lt;untested&gt;`,
		`<span class="tag good">HIGH</span>`, `<code>Shop.X</code>`, `&lt;b&gt;triage&lt;/b&gt;`,
		`Generated 2026-11-19 10:00 by mxcli 1.0`, `:root[data-theme="dark"]`, `.meter.bad i{background:var(--bad)}`,
	} {
		if !strings.Contains(h, want) {
			t.Errorf("html lacks %s", want)
		}
	}
	if strings.Contains(h, "<b>triage</b>") {
		t.Error("card text is escaped")
	}
	var md bytes.Buffer
	_ = RenderMarkdown(&md, d)
	mustContain(t, md.String(), "| M1 | 3 | 1 | 4/4 |", "`HIGH` **Caller-trusting flow `Shop.X`** — fixed")
}

// The scored report's page used the linter's own HTML, whose score bars had a
// width but no fill rule: every bar was an empty grey track. On the shared
// renderer each bar has a coloured fill.
func TestQualityDocBarsHaveAFill(t *testing.T) {
	r := linter.BuildReport("Shop", "2026-11-19 10:00:00", []linter.Violation{
		{RuleID: "SEC001", Severity: linter.SeverityError, Message: "no access rules", Location: linter.Location{Module: "Shop", DocumentName: "Order"}},
	})
	r.Consistency = &linter.ConsistencyCheck{Ran: true, Errors: 2, Findings: []linter.ConsistencyFinding{
		{Severity: "error", Code: "CE0117", Message: "x", Module: "Shop", Document: "Microflow 'A'"},
		{Severity: "error", Code: "CE0117", Message: "x", Module: "Shop", Document: "Microflow 'B'"}}}
	h := htmlOf(t, QualityDoc(r, "Shop", r.Date, "1.0", nil))
	for _, want := range []string{`<div class="meter good"><i style="width:90.0%"></i>`, `.meter.good i{background:var(--good)}`,
		"Shop · Quality score · 2026-11-19", "It does not build", "Does it build?", "CE0117", "SEC001"} {
		if !strings.Contains(h, want) {
			t.Errorf("quality page lacks %s", want)
		}
	}
}

// ako/mxcli#2443: the scored page explains its number — weights and what each
// category costs the headline, the penalty against the floor, the points each
// rule's fixes would gain (scored again without them), and a verdict per rule.
func TestQualityDocExplainsTheScore(t *testing.T) {
	var vs []linter.Violation
	add := func(rule string, sev linter.Severity, n int, module string) {
		for i := 0; i < n; i++ {
			vs = append(vs, linter.Violation{RuleID: rule, Severity: sev, Message: "m", Location: linter.Location{Module: module, DocumentName: "D"}})
		}
	}
	add("MPR006", linter.SeverityInfo, 120, "Shop")   // Quality: 120 points, past the floor
	add("MPR002", linter.SeverityWarning, 40, "Core") // Quality too: 120 points on its own
	add("SEC001", linter.SeverityError, 2, "Shop")    // Security: 80
	r := linter.BuildReport("Shop", "2026-11-19 10:00:00", vs)
	r.AcceptedByRule = map[string]int{"SEC001": 1, "SEC013": 1}
	d := QualityDoc(r, "Shop", r.Date, "1.0", &Annotations{Items: map[string]Note{"rule:MPR002": {Status: "by design", Note: "placeholder flows"}}})
	var md bytes.Buffer
	_ = RenderMarkdown(&md, d)
	m := md.String()
	for _, want := range []string{
		"MPR006 × 120", "fixing them all gains nothing: Quality stays past the floor",
		"240 · 140 past", // 120 info + 40 warnings × 3
		"by design: placeholder flows", "accepted in the brain", "`SEC013`",
		"Fix `SEC001` (2 finding(s), Security): +",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("quality page lacks %q", want)
		}
	}
	if strings.Contains(m, "Fix `MPR002`") || strings.Contains(m, "Fix `MPR006`") {
		t.Error("a rule with a verdict, or whose fixes gain nothing, is not recommended")
	}
	// The gain is what scoring again without the rule's findings gives.
	var rest []linter.Violation
	for _, v := range vs {
		if v.RuleID != "SEC001" {
			rest = append(rest, v)
		}
	}
	want := linter.BuildReport("Shop", r.Date, rest).OverallScore - r.OverallScore
	if !strings.Contains(m, fmt.Sprintf("| `SEC001` | Security | error | 2 | 1 | %.1f |", want)) {
		t.Errorf("SEC001 row should gain %.1f:\n%s", want, m)
	}
}

// ako/mxcli#2443: traceability opens with stacked bars per milestone and per
// epic, and the tiles carry a status.
func TestTraceabilityGlanceAndTileStatus(t *testing.T) {
	d := TraceabilityData{HasTests: true, Milestones: []TraceMilestone{
		{Name: "M1", Requirements: []TraceRequirement{
			{ID: "a", Slice: "01-accounts", Status: StatusBuilt, Tests: []string{"T"}},
			{ID: "b", Slice: "01-accounts", Status: StatusBuilt},
			{ID: "c", Slice: "02-orders", Status: StatusPartial}}},
		{Name: "M2", Requirements: []TraceRequirement{{ID: "d", Slice: "03-pay", Status: StatusNotBuilt}}},
	}}
	env, _ := NewEnvelope("traceability", "Shop", "2026-11-19 10:00", "t", d, TraceabilityMethod, TraceabilityLimits, nil)
	doc, err := RenderTraceability(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	var md bytes.Buffer
	_ = RenderMarkdown(&md, doc)
	mustContain(t, md.String(), "## At a glance", "| M1 | 1 | 1 | 1 | 0 | 2/3 built |", "| M2 | 0 | 0 | 0 | 1 | 0/1 built |", "| 02-orders | 0 | 0 | 1 | 0 | 0/1 built |",
		// one table of every requirement, across milestones (ako/mxcli#2443)
		"## Every requirement", "| **M1 · 01-accounts** |", "| `a`  | built | 1 test |", "| **M2 · 03-pay** |", "| `d`  | not built | nothing |",
		// built with nothing to reach says why it can never be proven (ako/mxcli#2475)
		"| `b`  | built | nothing to reach (no microflow implements it) |")
	if strings.Count(md.String(), "| **M1 · 01-accounts** |") != 1 {
		t.Error("a group header is written once, where the group starts")
	}
	h := htmlOf(t, doc)
	if !strings.Contains(h, `<div class="kpi warn"><span class="v">2</span><span class="l">built</span>`) {
		t.Error("the built tile is warn while not every requirement is built")
	}
}

// ako/mxcli#2443: an unused-document annotation without the module applies
// when the name is unique; an ambiguous one still shows as matching nothing.
func TestUnusedAnnotationWithoutModule(t *testing.T) {
	d := ArchitectureData{Unused: []UnusedDoc{
		{Name: "Shop.FaultProbe", Type: "page", Module: "Shop", Verdict: "verify"},
		{Name: "Shop.Dup", Type: "page", Module: "Shop", Verdict: "unused"},
		{Name: "Core.Dup", Type: "page", Module: "Core", Verdict: "unused"},
	}}
	env, _ := NewEnvelope("architecture", "Shop", "now", "t", d, ArchitectureMethod, ArchitectureLimits, nil)
	ann := &Annotations{Items: map[string]Note{
		"unused:FaultProbe": {Status: "used", Note: "reached by URL"},
		"unused:Dup":        {Status: "used"},
	}}
	doc, err := RenderArchitecture(env, ann)
	if err != nil {
		t.Fatal(err)
	}
	var md bytes.Buffer
	_ = RenderMarkdown(&md, doc)
	mustContain(t, md.String(), "| `Shop.FaultProbe` | page | used | reached by URL |", "Annotations that matched nothing", "`unused:Dup`")
	if strings.Contains(md.String(), "`unused:FaultProbe`") {
		t.Error("the unique short key applied, so it is not listed as matching nothing")
	}
}

// At phone width a stacked bar's label wraps instead of being cut with an
// ellipsis: the bar has its own line by then, so the label has room. The
// narrow rule must come after the base .srow rules, or those win the cascade
// at equal specificity (the retest of ako/mxcli#2475 saw "M3 — Simulated
// kitchen and co…" at 390 px).
func TestStackedLabelsWrapAtPhoneWidth(t *testing.T) {
	css := pageCSS
	narrow := strings.Index(css, "@media (max-width:600px){.srow")
	base := strings.Index(css, ".srow .sl{overflow:hidden")
	if narrow < 0 || base < 0 {
		t.Fatalf("rules not found: narrow %d, base %d", narrow, base)
	}
	if narrow < base {
		t.Error("the phone-width .srow rule comes before the base rules, so they override it")
	}
	rule := css[narrow : narrow+strings.Index(css[narrow:], "}}")]
	if !strings.Contains(rule, ".srow .sl{") || !strings.Contains(rule, "white-space:normal") {
		t.Errorf("the phone-width rule lets the label wrap: %s", rule)
	}
}
