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
func QualityDoc(r *linter.Report, project, generated, mxcli string) Doc {
	d := Doc{Title: "Quality score", Project: project, Kind: KindLabel("quality"), Generated: generated, Mxcli: mxcli}
	d.Lede = qualityLede(r)

	scoreStatus := statusFor(r.OverallScore)
	d.Stats = append(d.Stats, Stat{Label: "overall score", Value: fmt.Sprintf("%.1f/100", r.OverallScore), Status: scoreStatus,
		Note: "a weighted mean of the category scores"})
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

	ct := Table{Columns: []string{"Category", "Score", "Errors", "Warnings", "Info"}}
	for _, c := range r.Categories {
		ct.Rows = append(ct.Rows, []string{c.Name, fmt.Sprintf("%.0f", c.Score), fmt.Sprint(c.Errors), fmt.Sprint(c.Warnings), fmt.Sprint(c.Infos)})
		ct.Classes = append(ct.Classes, []string{"", "", "num", "num", "num"})
		ct.Bars = append(ct.Bars, []*Bar{nil, {Pct: c.Score, Status: statusFor(c.Score)}, nil, nil, nil})
	}
	d.Sections = append(d.Sections, Section{ID: "categories", Title: "Categories",
		Intro:  "Each category starts at 100 and loses points for every finding in it, floored at 0.",
		Blocks: []Block{ct}})

	var recs []Block
	for _, c := range r.Categories {
		if len(c.TopActions) == 0 {
			continue
		}
		recs = append(recs, Para{Text: "**" + c.Name + "**"}, Bullets{Items: c.TopActions})
	}
	if len(recs) > 0 {
		d.Sections = append(d.Sections, Section{ID: "recommendations", Title: "Recommendations", Blocks: recs})
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
