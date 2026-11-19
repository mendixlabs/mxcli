// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// QualityDoc is the scored report (`mxcli report`) as a page in the shared
// look, so it reads like every other kind (ako/mxcli#2443). Its own HTML had a
// style of its own and score bars with no fill rule: every bar rendered as an
// empty grey track. Markdown and JSON stay linter's.
//
// The page explains the number (ako/mxcli#2443): each category's weight and
// what it costs the headline, how far its penalty is past the floor (a 0 that
// a few fixes cannot move), a verdict per rule (brain acceptances and
// `rule:<ID>` annotations), and recommendations ranked by the points each
// rule's fixes would gain, computed by scoring again without them.
func QualityDoc(r *linter.Report, project, generated, mxcli string, ann *Annotations) Doc {
	d := Doc{Title: "Quality score", Project: project, Kind: KindLabel("quality"), Generated: generated, Mxcli: mxcli}
	d.Lede = qualityLede(r)

	scoreStatus := statusFor(r.OverallScore)
	plain := 0.0
	for _, c := range r.Categories {
		plain += c.Score
	}
	if len(r.Categories) > 0 {
		plain /= float64(len(r.Categories))
	}
	d.Stats = append(d.Stats, Stat{Label: "overall score", Value: fmt.Sprintf("%.1f/100", r.OverallScore), Status: scoreStatus,
		Note: fmt.Sprintf("a weighted mean of the categories; a plain mean would read %.1f", plain)})
	rules := ruleStats(r, ann)
	if c := r.Consistency; c != nil {
		st := "good"
		switch {
		case c.Errors > 0:
			st = "bad"
		case !c.Builds():
			st = "warn"
		}
		d.Stats = append(d.Stats, Stat{Label: "builds", Value: c.Verdict(), Status: st, Note: "Mendix's own consistency check, not part of the score"})
	}
	errSt := "good"
	if r.Summary.Errors > 0 {
		errSt = "bad"
	}
	d.Stats = append(d.Stats,
		Stat{Label: "lint errors", Value: fmt.Sprint(r.Summary.Errors), Status: errSt},
		Stat{Label: "warnings · info", Value: fmt.Sprintf("%d · %d", r.Summary.Warnings, r.Summary.Infos)})
	if len(rules) > 0 && r.Summary.Total > 0 {
		top := rules[0]
		for _, x := range rules {
			if x.Count > top.Count {
				top = x
			}
		}
		note := fmt.Sprintf("%.0f%% of all findings; fixing them all gains %.1f points", 100*float64(top.Count)/float64(r.Summary.Total), top.Gain)
		if top.Gain < 0.1 {
			note = fmt.Sprintf("%.0f%% of all findings, yet fixing them all gains nothing: %s stays past the floor", 100*float64(top.Count)/float64(r.Summary.Total), top.Category)
		}
		d.Stats = append(d.Stats, Stat{Label: "most frequent rule", Value: fmt.Sprintf("%s × %d", top.ID, top.Count), Note: note})
	}
	if r.Accepted > 0 {
		d.Stats = append(d.Stats, Stat{Label: "accepted", Value: fmt.Sprint(r.Accepted), Note: "findings decisions in the project brain accept; not scored"})
	}

	if len(r.Modules) > 0 {
		d.Sections = append(d.Sections, Section{ID: "scope", Title: "Scope", Blocks: []Block{Callout{Kind: "info",
			Text: "Only these modules are scored: " + strings.Join(r.Modules, ", ") + ". Findings outside them, and project-wide ones, are left out."}}})
	}
	if c := r.Consistency; c != nil {
		d.Sections = append(d.Sections, consistencySection(c, len(r.Modules) > 0))
	}

	totalW := 0.0
	for _, c := range r.Categories {
		totalW += linter.CategoryWeight(c.Name)
	}
	ct := Table{Columns: []string{"Category", "Score", "Weight", "Overall lost", "Errors", "Warnings", "Info", "Penalty"}}
	for _, c := range r.Categories {
		w := linter.CategoryWeight(c.Name)
		lost := 0.0
		if totalW > 0 {
			lost = (100 - c.Score) * w / totalW
		}
		pen := c.Penalty()
		penText := fmt.Sprintf("%.0f", pen)
		penSt := "good"
		switch {
		case pen > 100:
			penText += fmt.Sprintf(" · %.0f past", pen-100)
			penSt = "bad"
		case pen > 0:
			penText += fmt.Sprintf(" · %.0f left", 100-pen)
			penSt = "warn"
		}
		ct.Rows = append(ct.Rows, []string{c.Name, fmt.Sprintf("%.0f", c.Score), fmt.Sprintf("%.0f%%", 100*w/totalW), fmt.Sprintf("%.1f", lost),
			fmt.Sprint(c.Errors), fmt.Sprint(c.Warnings), fmt.Sprint(c.Infos), penText})
		ct.Classes = append(ct.Classes, []string{"", "", "num", "num", "num", "num", "num", ""})
		penPct := pen
		if penPct > 100 {
			penPct = 100
		}
		ct.Bars = append(ct.Bars, []*Bar{nil, {Pct: c.Score, Status: statusFor(c.Score)}, nil, nil, nil, nil, nil, {Pct: penPct, Status: penSt}})
	}
	d.Sections = append(d.Sections, Section{ID: "categories", Title: "Categories",
		Intro: fmt.Sprintf("Each category starts at 100 and loses %d points per error, %d per warning and %d per info, floored at 0. "+
			"The overall score weighs the categories; \"overall lost\" is what each costs the headline. "+
			"Penalty is the points its findings cost, and how many are left before the floor or how far past it they go: past the floor, fixes do not move the score until it comes back under 100.",
			linter.PenaltyError, linter.PenaltyWarning, linter.PenaltyInfo),
		Blocks: []Block{ct}})

	// Recommendations: the rules whose fixes gain the most.
	var recs []string
	byGain := append([]ruleStat(nil), rules...)
	sort.SliceStable(byGain, func(i, j int) bool { return byGain[i].Gain > byGain[j].Gain })
	for _, x := range byGain {
		if x.Gain < 0.1 || x.Verdict != "" || len(recs) == 5 {
			continue
		}
		recs = append(recs, fmt.Sprintf("Fix `%s` (%d finding(s), %s): +%.1f points. Where: %s.", x.ID, x.Count, x.Category, x.Gain, x.Where))
	}
	var recBlocks []Block
	if len(recs) > 0 {
		recBlocks = append(recBlocks, Bullets{Items: recs})
	}
	var acts []string
	for _, c := range r.Categories {
		for _, a := range c.TopActions {
			acts = append(acts, c.Name+": "+a)
		}
	}
	if len(acts) > 0 {
		recBlocks = append(recBlocks, Para{Text: "The rules' own suggestions:"}, Bullets{Items: acts})
	}
	if len(recBlocks) > 0 {
		d.Sections = append(d.Sections, Section{ID: "recommendations", Title: "Recommendations",
			Intro:  "Ranked by how far fixing every finding of a rule would raise the score (computed by scoring again without them). Rules with a verdict are left out.",
			Blocks: recBlocks})
	}

	if len(rules) > 0 {
		rt := Table{Columns: []string{"Rule", "Category", "Severity", "Findings", "Accepted", "Gain if fixed", "Verdict", "Where"}}
		for _, x := range rules {
			v := x.Verdict
			if v == "" {
				v = "open"
			}
			rt.Rows = append(rt.Rows, []string{"`" + x.ID + "`", x.Category, x.Severity, fmt.Sprint(x.Count), fmt.Sprint(x.Accepted), fmt.Sprintf("%.1f", x.Gain), v, x.Where})
			cls := ""
			if x.Verdict == "" {
				cls = severityClassName(x.Severity)
			}
			rt.Classes = append(rt.Classes, []string{"", "", "", "num", "num", "num", cls, ""})
		}
		d.Sections = append(d.Sections, Section{ID: "rules", Title: "Rules",
			Intro:  "Every rule that found something, most findings first. A verdict comes from a brain decision that accepts the finding, or from `rule:<ID>` in the annotations (status and note).",
			Blocks: []Block{rt}})
	}

	if len(r.Violations) > 0 {
		vs := append([]linter.Violation(nil), r.Violations...)
		sort.SliceStable(vs, func(i, j int) bool { return vs[i].Severity > vs[j].Severity })
		ft := Table{Columns: []string{"Rule", "Severity", "Location", "Message"}}
		for _, v := range vs {
			loc := v.Location.QualifiedName()
			if loc == "" {
				loc = v.Location.DocumentName
			}
			ft.Rows = append(ft.Rows, []string{v.RuleID, v.Severity.String(), loc, v.Message})
			ft.Classes = append(ft.Classes, []string{"", severityClass(v.Severity), "", ""})
		}
		d.Sections = append(d.Sections, Section{ID: "findings", Title: "Findings", Blocks: []Block{ft}})
	}
	if len(r.RuleFailures) > 0 {
		rt := Table{Columns: []string{"Rule", "Problem"}}
		for _, v := range r.RuleFailures {
			rt.Rows = append(rt.Rows, []string{v.RuleID, v.Message})
		}
		d.Sections = append(d.Sections, Section{ID: "rule-failures", Title: "Rules that could not run",
			Intro: "Not counted in the score: these are problems with the lint tooling, not the project.", Blocks: []Block{rt}})
	}
	return d
}

// ruleStat is one rule's findings on the scored report.
type ruleStat struct {
	ID, Category, Severity, Where, Verdict string
	Count, Accepted                        int
	Gain                                   float64
}

// ruleStats groups the scored findings by rule, most findings first, with the
// points fixing each rule's findings would gain and its verdict.
func ruleStats(r *linter.Report, ann *Annotations) []ruleStat {
	by := map[string]*ruleStat{}
	worst := map[string]linter.Severity{}
	docs := map[string]map[string]int{}
	var order []string
	for _, v := range r.Violations {
		x := by[v.RuleID]
		if x == nil {
			x = &ruleStat{ID: v.RuleID, Category: linter.CategoryOf(v.RuleID)}
			by[v.RuleID] = x
			order = append(order, v.RuleID)
			docs[v.RuleID] = map[string]int{}
		}
		x.Count++
		if v.Severity > worst[v.RuleID] {
			worst[v.RuleID] = v.Severity
		}
		loc := v.Location.Module
		if loc == "" {
			loc = "project"
		}
		docs[v.RuleID][loc]++
	}
	out := make([]ruleStat, 0, len(order))
	for _, id := range order {
		x := by[id]
		x.Severity = worst[id].String()
		// Score again without this rule's findings.
		var rest []linter.Violation
		for _, v := range r.Violations {
			if v.RuleID != id {
				rest = append(rest, v)
			}
		}
		x.Gain = roundTenth(linter.BuildReport(r.ProjectName, r.Date, rest).OverallScore - r.OverallScore)
		x.Where = topCounts(docs[id], 3)
		x.Accepted = r.AcceptedByRule[id]
		if n, ok := ann.Get("rule:" + id); ok {
			x.Verdict = joinNote(n.Status, n.Note, "")
		}
		out = append(out, *x)
	}
	// A rule whose every finding a brain decision accepts is not among the
	// scored findings; it is listed with its verdict all the same.
	var acceptedOnly []string
	for id := range r.AcceptedByRule {
		if by[id] == nil {
			acceptedOnly = append(acceptedOnly, id)
		}
	}
	sort.Strings(acceptedOnly)
	for _, id := range acceptedOnly {
		v := "accepted in the brain"
		if n, ok := ann.Get("rule:" + id); ok {
			v = joinNote(n.Status, n.Note, "")
		}
		out = append(out, ruleStat{ID: id, Category: linter.CategoryOf(id), Accepted: r.AcceptedByRule[id], Verdict: v})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

func roundTenth(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

// topCounts is "Shop 54, Core 12, Catalog 3 and 2 more".
func topCounts(m map[string]int, n int) string {
	type kv struct {
		k string
		v int
	}
	var xs []kv
	for k, v := range m {
		xs = append(xs, kv{k, v})
	}
	sort.Slice(xs, func(i, j int) bool {
		if xs[i].v != xs[j].v {
			return xs[i].v > xs[j].v
		}
		return xs[i].k < xs[j].k
	})
	var parts []string
	for i, x := range xs {
		if i == n {
			parts = append(parts, fmt.Sprintf("and %d more", len(xs)-n))
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", x.k, x.v))
	}
	return strings.Join(parts, ", ")
}

func severityClassName(s string) string {
	switch s {
	case "error":
		return "bad"
	case "warning":
		return "warn"
	}
	return ""
}

func qualityLede(r *linter.Report) string {
	s := fmt.Sprintf("The project scores %.1f out of 100 on the lint rules", r.OverallScore)
	if len(r.Categories) > 0 {
		low := r.Categories[0]
		for _, c := range r.Categories {
			if c.Score < low.Score {
				low = c
			}
		}
		if low.Score < 100 {
			s += fmt.Sprintf("; %s scores lowest at %.0f", low.Name, low.Score)
		}
	}
	s += "."
	if c := r.Consistency; c != nil {
		switch {
		case c.Errors > 0:
			s += fmt.Sprintf(" It does not build: Mendix's consistency check found %d error(s), whatever the score says.", c.Errors)
		case !c.Ran:
			s += " Whether it builds was not checked."
		}
	}
	return s
}

func consistencySection(c *linter.ConsistencyCheck, scoped bool) Section {
	var bl []Block
	switch {
	case !c.Ran:
		bl = append(bl, Callout{Kind: "warn", Text: "Mendix's consistency check (`mx check`) was not run, so this page does not say whether the project builds: " + c.NotRun})
	case c.Errors > 0:
		bl = append(bl, Callout{Kind: "warn", Text: fmt.Sprintf("The project does not build: `mx check` found %d error(s). A project with errors cannot be deployed. The score covers the lint rules only.", c.Errors)})
	case c.Incomplete != "":
		bl = append(bl, Callout{Kind: "warn", Text: "The consistency check ran, but " + c.Incomplete + "."})
	default:
		bl = append(bl, Callout{Kind: "info", Text: "Mendix's consistency check (`mx check`) found no errors."})
	}
	if c.Ran && scoped {
		bl = append(bl, Para{Text: "The check covers the whole project, not only the selected modules."})
	}
	if groups := c.Groups(); len(groups) > 0 {
		t := Table{Columns: []string{"Severity", "Code", "Count", "Message", "Where"}}
		for _, g := range groups {
			where := strings.Join(g.Locations, "; ")
			if g.More > 0 {
				where += fmt.Sprintf("; and %d more", g.More)
			}
			t.Rows = append(t.Rows, []string{g.Severity, g.Code, fmt.Sprint(g.Count), g.Message, where})
			cls := ""
			switch g.Severity {
			case "error":
				cls = "bad"
			case "warning":
				cls = "warn"
			}
			t.Classes = append(t.Classes, []string{cls, "", "num", "", ""})
		}
		bl = append(bl, t)
	}
	return Section{ID: "builds", Title: "Does it build?", Blocks: bl}
}

func statusFor(score float64) string {
	switch {
	case score >= 75:
		return "good"
	case score >= 50:
		return "warn"
	default:
		return "bad"
	}
}

func severityClass(s linter.Severity) string {
	switch s {
	case linter.SeverityError:
		return "bad"
	case linter.SeverityWarning:
		return "warn"
	default:
		return "muted"
	}
}
