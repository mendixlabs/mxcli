// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
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
	h := htmlOf(t, QualityDoc(r, "Shop", r.Date, "1.0"))
	for _, want := range []string{`<div class="meter good"><i style="width:90.0%"></i>`, `.meter.good i{background:var(--good)}`,
		"Shop · Quality score · 2026-11-19", "It does not build", "Does it build?", "CE0117", "SEC001"} {
		if !strings.Contains(h, want) {
			t.Errorf("quality page lacks %s", want)
		}
	}
}
