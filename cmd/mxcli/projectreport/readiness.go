// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// The readiness report rolls the other reports up into one go/no-go page
// (ako/mxcli#2427). It measures nothing itself: each item comes from a
// report's data, and a report that was not given is "not assessed", never
// passing.

// ReadinessInput is the other reports' data. A nil entry was not given.
type ReadinessInput struct {
	Quality       *linter.JSONReport
	Architecture  *ArchitectureData
	Security      *SecurityData
	Configuration *ConfigurationData
	Dependencies  *DependencyData
	Tests         *TestsData
	Traceability  *TraceabilityData
	Performance   *PerformanceData
	Release       *ReleaseData
	// Annotations per kind: the verdicts recorded on each report (a fixed
	// security finding, an accepted vulnerability or release change).
	Annotations map[string]*Annotations
	// SignOff is the readiness page's own annotations: readiness:<key> with
	// status "signed off" moves a blocking item to Accepted.
	SignOff *Annotations
	// Failed names the reports that were given their inputs but could not
	// run, with why; Warnings are each report's own warnings. Both turn a
	// "not given" into what actually happened (ako/mxcli#2452).
	Failed   map[string]string
	Warnings map[string][]string
	// Milestone, when set, makes its unbuilt requirements blocking.
	Milestone string
	// BudgetMs is the server time a journey step may take; 0 means none.
	BudgetMs float64
}

// ReadinessData is the verdict and its reasons.
type ReadinessData struct {
	Verdict    string            `json:"verdict"` // not ready | not assessed in full | ready, with risks | ready
	Rule       string            `json:"rule"`
	Blocking   []ReadinessItem   `json:"blocking"`
	Risks      []ReadinessItem   `json:"risks"`
	Accepted   []ReadinessItem   `json:"accepted"`
	Admin      []ReadinessItem   `json:"admin"`
	Sources    []ReadinessSource `json:"sources"`
	NotCovered []string          `json:"notCovered"`
	Milestone  string            `json:"milestone,omitempty"`
	BudgetMs   float64           `json:"budgetMs,omitempty"`
}

// ReadinessItem is one reason, with the report it came from.
type ReadinessItem struct {
	Key    string `json:"key"`
	Source string `json:"source"` // report kind
	Text   string `json:"text"`
	Note   string `json:"note,omitempty"`
}

// ReadinessSource says whether a report was read, and if not, why that
// matters.
type ReadinessSource struct {
	Kind     string `json:"kind"`
	Assessed bool   `json:"assessed"`
	Note     string `json:"note,omitempty"`
	// Decides is true for a report whose absence leaves a blocking question
	// open; the verdict cannot be "ready" without it.
	Decides bool `json:"decides"`
}

// Readiness verdicts.
const (
	VerdictNotReady    = "not ready"
	VerdictIncomplete  = "not assessed in full"
	VerdictReadyRisks  = "ready, with risks"
	VerdictReady       = "ready"
	ReadinessRule      = "Any open blocking item: not ready. Otherwise, when a report that decides a blocking question was not given or could not check: not assessed in full. Otherwise: ready, with risks when there are risks, else ready. A blocking item signed off (readiness:<key>, status \"signed off\") counts as accepted."
	readinessSignedOff = "signed off"
)

// ReadinessNotCovered is what no model report can tell, listed every time.
var ReadinessNotCovered = []string{
	"Behaviour on the production infrastructure, and load beyond the measured journeys.",
	"Hosting: TLS, HTTP security headers, network rules, the deployment's own constant values and scheduled-event switches.",
	"Backups, and a restore that was actually tried.",
	"Licences: the Mendix runtime licence and third-party terms.",
	"A penetration test, and a review of Java and JavaScript code the model does not show.",
}

// ComputeReadiness rolls the reports up.
func ComputeReadiness(in ReadinessInput) *ReadinessData {
	d := &ReadinessData{Rule: ReadinessRule, NotCovered: ReadinessNotCovered, Milestone: in.Milestone, BudgetMs: in.BudgetMs,
		Blocking: []ReadinessItem{}, Risks: []ReadinessItem{}, Accepted: []ReadinessItem{}, Admin: []ReadinessItem{}}
	ann := func(kind string) *Annotations { return in.Annotations[kind] }
	block := func(src, key, text string) {
		it := ReadinessItem{Key: key, Source: src, Text: text}
		if n, ok := in.SignOff.Get("readiness:" + key); ok && strings.EqualFold(n.Status, readinessSignedOff) {
			it.Note = joinNote("signed off", n.Note, "")
			d.Accepted = append(d.Accepted, it)
			return
		}
		d.Blocking = append(d.Blocking, it)
	}
	risk := func(src, key, text string) {
		d.Risks = append(d.Risks, ReadinessItem{Key: key, Source: src, Text: text})
	}
	accept := func(src, key, text, note string) {
		d.Accepted = append(d.Accepted, ReadinessItem{Key: key, Source: src, Text: text, Note: note})
	}
	source := func(kind string, given, decides bool, note string) {
		switch {
		case !given && in.Failed[kind] != "":
			// Given, and broke: a different action than evidence never collected.
			note = "failed: " + firstLine(in.Failed[kind])
		case note != "" && len(in.Warnings[kind]) > 0:
			note += " — " + firstLine(in.Warnings[kind][0])
		}
		d.Sources = append(d.Sources, ReadinessSource{Kind: kind, Assessed: given && note == "", Decides: decides, Note: note})
	}
	resolved := func(kind, key string) (Note, bool) {
		n, ok := ann(kind).Peek(key)
		if !ok {
			return n, false
		}
		switch strings.ToLower(n.Status) {
		case "fixed", "accepted", "intended":
			return n, true
		}
		return n, false
	}

	// Builds: the scored report's consistency check.
	switch q := in.Quality; {
	case q == nil:
		source("quality", false, true, "not given: consistency errors and lint findings not assessed")
	default:
		note := ""
		switch c := q.Consistency; {
		case c == nil || !c.Ran:
			note = "the consistency check (mx check) did not run"
			if c != nil && c.NotRun != "" {
				note += ": " + firstLine(c.NotRun)
			}
		case c.Errors > 0:
			block("quality", "builds", fmt.Sprintf("%d consistency error(s) (mx check): a project with errors cannot be deployed", c.Errors))
		case c.Incomplete != "":
			note = "the consistency check's output could not be read in full: " + c.Incomplete
		}
		source("quality", true, true, note)
		for _, c := range q.Categories {
			if c.Errors+c.Warnings > 0 {
				risk("quality", "lint:"+c.Name, fmt.Sprintf("%s: %d error(s), %d warning(s) in the lint rules (score %.0f)", c.Name, c.Errors, c.Warnings, c.Score))
			}
		}
		rules := make([]string, 0, len(q.AcceptedByRule))
		for r := range q.AcceptedByRule {
			rules = append(rules, r)
		}
		sort.Strings(rules)
		for _, r := range rules {
			accept("quality", "rule:"+r, fmt.Sprintf("%d finding(s) of %s", q.AcceptedByRule[r], r), "accepted by a decision in the project brain (mxcli lint --show-accepted)")
		}
	}

	// Release: what the deploy does to production data.
	if r := in.Release; r == nil {
		source("release", false, true, "not given: what the deploy does to production data is not assessed (report release --against <deployed>)")
	} else {
		source("release", true, true, "")
		for _, c := range r.Changes {
			n, ok := resolved("release", c.Key())
			switch {
			case c.Effect == EffectDestroys && ok:
				accept("release", c.Key(), c.What, joinNote(n.Status, n.Note, ""))
			case c.Effect == EffectDestroys:
				block("release", c.Key(), c.What+" — destroys data stored in production")
			case c.Effect == EffectVerify && !ok:
				risk("release", c.Key(), c.What+" — verify against production data")
			}
		}
	}

	// Dependencies.
	if dep := in.Dependencies; dep == nil {
		source("dependencies", false, true, "not given: vulnerable libraries and the Mendix release line not assessed")
	} else {
		note := ""
		if !dep.VulnChecked {
			note = "vulnerabilities were not checked: " + nonEmpty(dep.VulnNote, "no advisory source")
		}
		source("dependencies", true, true, note)
		for _, l := range dep.Libraries {
			for _, v := range l.Vulns {
				if !v.Blocking() {
					continue
				}
				key := "vuln:" + v.ID
				if n, ok := resolved("dependencies", key); ok {
					accept("dependencies", key, v.ID+" in "+l.Coordinate(), joinNote(n.Status, n.Note, ""))
					continue
				}
				block("dependencies", key, fmt.Sprintf("%s (%s) in %s", v.ID, strings.ToLower(v.Severity), l.Coordinate()))
			}
		}
		switch dep.Line {
		case "monthly":
			risk("dependencies", "mendix-line", "Mendix "+dep.MendixVersion+" is a monthly release, supported only until the next one; production apps usually run an LTS or MTS version")
		case "unknown", "":
			risk("dependencies", "mendix-line", "Mendix "+dep.MendixVersion+"'s release line is not in mxcli's registry: check its support status")
		}
		behind := 0
		for _, m := range dep.Modules {
			if m.Status == DepNewer {
				behind++
			}
		}
		if behind > 0 {
			risk("dependencies", "modules-behind", fmt.Sprintf("%d Marketplace module(s) have a newer compatible version", behind))
		}
	}

	// Configuration.
	if c := in.Configuration; c == nil {
		source("configuration", false, true, "not given: locations fixed in the model and the administrator's checklist not assessed")
	} else {
		source("configuration", true, true, "")
		for _, e := range c.Endpoints {
			if e.Fixed && e.Host != "external" {
				block("configuration", "endpoint:"+e.Where, fmt.Sprintf("`%s`: %s fixed to %s (`%s`)", e.Where, e.Kind, hostPhrase(e.Host), e.Value))
			}
		}
		for _, cr := range c.Credentials {
			block("configuration", "credential:"+cr.Where, "`"+cr.Where+"`: "+cr.What)
		}
		for _, k := range c.Constants {
			switch k.Need {
			case NeedMustSet, NeedOverride:
				d.Admin = append(d.Admin, ReadinessItem{Key: "constant:" + k.Name, Source: "configuration", Text: "Set `" + k.Name + "` (" + k.Need + ")"})
			case NeedSecret:
				d.Admin = append(d.Admin, ReadinessItem{Key: "constant:" + k.Name, Source: "configuration", Text: "Set `" + k.Name + "` and rotate the committed value"})
			}
		}
		for _, s := range c.Scheduled {
			d.Admin = append(d.Admin, ReadinessItem{Key: "scheduled:" + s.Name, Source: "configuration", Text: "Enable or disable `" + s.Name + "` (" + nonEmpty(s.Interval, "scheduled") + ")"})
		}
	}

	// Security.
	if s := in.Security; s == nil {
		source("security", false, true, "not given: open security findings not assessed")
	} else {
		source("security", true, true, "")
		for _, f := range s.Findings {
			key := "finding:" + f.ID
			n, ok := ann("security").Peek(key)
			st := strings.ToLower(n.Status)
			switch {
			case ok && st == "fixed":
				continue
			case ok && (st == "accepted" || st == "intended"):
				accept("security", key, f.Title, joinNote(n.Status, n.Note, ""))
			case f.Severity == "high":
				block("security", key, f.Title)
			case f.Severity == "medium":
				risk("security", key, f.Title)
			}
		}
		triage := 0
		for _, f := range s.Flows {
			if f.Risk == "medium" {
				if _, ok := ann("security").Peek("flow:" + f.Name); !ok {
					triage++
				}
			}
		}
		if triage > 0 {
			risk("security", "flows", fmt.Sprintf("%d client-callable flow(s) at medium risk without a verdict", triage))
		}
	}

	// Tests.
	if t := in.Tests; t == nil {
		source("tests", false, true, "not given: test results not assessed")
	} else {
		note := ""
		if !t.HasResults {
			note = "no test results were read (report tests --run, or --junit)"
		}
		source("tests", true, true, note)
		for _, x := range t.Tests {
			if x.Status == "failed" || x.Status == "error" {
				block("tests", "test:"+x.Name, "microflow test `"+x.Name+"` "+x.Status)
			}
		}
		for _, j := range t.Journeys {
			if j.Status == "failed" {
				block("tests", "journey:"+j.Name, "journey `"+j.Name+"` failed")
			}
		}
		for _, g := range t.GapKinds {
			if g.Kind == GapCallable {
				risk("tests", "gaps", fmt.Sprintf("%d microflow(s) a user role may call are reached by no test or journey", len(g.Microflows)))
			}
		}
	}

	// Traceability.
	if tr := in.Traceability; tr == nil || tr.ModelAbsent {
		source("traceability", false, in.Milestone != "", "not given: whether the specification was built is not assessed")
	} else {
		note := ""
		found := in.Milestone == ""
		for _, m := range tr.Milestones {
			releasing := in.Milestone != "" && strings.EqualFold(m.Name, in.Milestone)
			found = found || releasing
			notBuilt, untested := 0, 0
			for _, r := range m.Requirements {
				switch {
				case r.Status == StatusNotBuilt || r.Status == StatusNotTraceable || r.Status == StatusPartial:
					if releasing {
						block("traceability", "requirement:"+r.ID, fmt.Sprintf("%s %s is %s (milestone %s)", r.ID, r.Title, r.Status, m.Name))
					} else {
						notBuilt++
					}
				case r.Status == StatusBuilt && len(r.Tests)+len(r.Journeys) == 0:
					untested++
				}
			}
			if notBuilt > 0 {
				risk("traceability", "milestone:"+m.Name, fmt.Sprintf("milestone %s: %d requirement(s) not built in full", m.Name, notBuilt))
			}
			if untested > 0 {
				risk("traceability", "untested:"+m.Name, fmt.Sprintf("milestone %s: %d requirement(s) built but proven by no test or journey", m.Name, untested))
			}
		}
		if !found {
			note = "milestone " + in.Milestone + " is not in the plan"
		}
		source("traceability", true, in.Milestone != "", note)
	}

	// Performance: a risk, never blocking — the journeys are not production load.
	if p := in.Performance; p == nil {
		source("performance", false, false, "not given: no journeys were measured")
	} else {
		source("performance", true, false, "")
		if in.BudgetMs > 0 {
			for _, s := range p.Steps {
				if s.ServerMs > in.BudgetMs {
					risk("performance", "step:"+s.key(), fmt.Sprintf("%s takes %.0f ms on the server (budget %.0f ms)", s.key(), s.ServerMs, in.BudgetMs))
				}
			}
		}
	}

	// Architecture: a risk.
	if a := in.Architecture; a == nil {
		source("architecture", false, false, "not given")
	} else {
		source("architecture", true, false, "")
		for _, c := range a.Cycles {
			key := "cycle:" + strings.Join(c.Modules, "+")
			if _, ok := resolved("architecture", key); !ok {
				risk("architecture", key, "modules "+strings.Join(c.Modules, ", ")+" depend on each other (a domain cycle)")
			}
		}
	}

	open := 0
	for _, s := range d.Sources {
		if s.Decides && !s.Assessed {
			open++
		}
	}
	switch {
	case len(d.Blocking) > 0:
		d.Verdict = VerdictNotReady
	case open > 0:
		d.Verdict = VerdictIncomplete
	case len(d.Risks) > 0:
		d.Verdict = VerdictReadyRisks
	default:
		d.Verdict = VerdictReady
	}
	return d
}

// ---------------------------------------------------------------- render

var ReadinessMethod = []string{
	"Rolled up from the other reports' JSON, as `mxcli report all` writes them (or run for this page): quality (with Mendix's consistency check), release, dependencies, configuration, security, tests, traceability, performance and architecture. Each item names the report it came from; open that report for the evidence.",
	"Blocking: consistency errors; a release change that destroys production data; a high or critical vulnerability; a location fixed to localhost, a private address or a test host, or a literal credential; an open high security finding; a failing microflow test or journey; a requirement of the milestone being released (`--milestone`) that is not built in full.",
	"Risks: lint warnings and errors by category; release changes to verify; a monthly or unknown Mendix release line; Marketplace modules behind; medium security findings and flows without a verdict; callable microflows no test reaches; requirements not built or not proven; journey steps over the server-time budget (`--budget-ms`); domain cycles.",
	"Accepted: findings a project-brain decision accepts, verdicts recorded on the other reports (fixed, accepted, intended), and blocking items signed off on this page.",
}

var ReadinessLimits = []string{
	"The verdict is only as complete as the reports it read: one not given is listed as not assessed, and a report that decides a blocking question keeps the verdict at \"not assessed in full\".",
	"Everything under \"Not covered by the model\" has to be checked elsewhere, every time.",
}

// RenderReadiness builds the page.
func RenderReadiness(env *Envelope, ann *Annotations) (Doc, error) {
	var d ReadinessData
	if err := env.Decode(&d); err != nil {
		return Doc{}, err
	}
	doc := Doc{Title: "Production readiness", Subtitle: "One go/no-go page, rolled up from the other reports"}
	status := map[string]string{VerdictNotReady: "bad", VerdictIncomplete: "warn", VerdictReadyRisks: "warn", VerdictReady: "good"}[d.Verdict]
	notAssessed := 0
	for _, s := range d.Sources {
		if !s.Assessed {
			notAssessed++
		}
	}
	doc.Lede = fmt.Sprintf("Verdict: %s. %d blocking, %d risk(s), %d accepted; %d of %d reports not assessed in full.",
		d.Verdict, len(d.Blocking), len(d.Risks), len(d.Accepted), notAssessed, len(d.Sources))
	doc.Stats = []Stat{
		{Label: "verdict", Value: d.Verdict, Status: status, Note: "by the rule under Blocking"},
		{Label: "blocking", Value: fmt.Sprint(len(d.Blocking)), Status: statusIf(len(d.Blocking) > 0, false), Note: "any one makes it not ready"},
		{Label: "risks", Value: fmt.Sprint(len(d.Risks)), Status: statusIf(false, len(d.Risks) > 0), Note: "decide, or accept on their report"},
		{Label: "accepted", Value: fmt.Sprint(len(d.Accepted)), Note: "recorded decisions and verdicts"},
		{Label: "not assessed", Value: fmt.Sprint(notAssessed), Status: statusIf(false, notAssessed > 0), Note: fmt.Sprintf("of %d reports: never counted as passing", len(d.Sources))},
	}

	items := func(xs []ReadinessItem, empty string, withKey bool) Block {
		if len(xs) == 0 {
			return Para{Text: empty}
		}
		cols := []string{"Item", "From"}
		if withKey {
			cols = append(cols, "Sign-off key")
		}
		t := Table{Columns: cols}
		for _, x := range xs {
			text := x.Text
			if x.Note != "" {
				text += " — " + x.Note
			}
			row := []string{text, "[" + KindLabel(x.Source) + "](" + x.Source + ".html)"}
			if withKey {
				row = append(row, "`readiness:"+x.Key+"`")
			}
			t.Rows = append(t.Rows, row)
		}
		return t
	}
	intro := "The verdict rule: " + d.Rule
	if d.Milestone != "" {
		intro += " Milestone being released: " + d.Milestone + "."
	}
	doc.Sections = append(doc.Sections,
		Section{ID: "blocking", Title: "Blocking", Intro: intro, Blocks: []Block{items(d.Blocking, "Nothing blocking was found in the reports that were read.", true)}},
		Section{ID: "risks", Title: "Risks", Intro: "Not blocking on their own; each is a decision to take, or to record as accepted on its report.",
			Blocks: []Block{items(d.Risks, "No risks were found in the reports that were read.", false)}},
		Section{ID: "accepted", Title: "Accepted", Intro: "Visible, not hidden: each accepted item with the decision or verdict that accepts it. " +
			"Only recorded acceptances count: a verdict on the item's own report (`--annotations`), a lint finding accepted by a brain decision captured with `mxcli brain capture --accepts <RULE>:<why>`, or a sign-off here. " +
			"A decision written only as prose in the brain is not read as accepting anything.",
			Blocks: []Block{items(d.Accepted, "Nothing is accepted.", false)}},
	)
	admin := []Block{items(d.Admin, "The model asks nothing of an environment.", false)}
	doc.Sections = append(doc.Sections, Section{ID: "admin", Title: "For the administrator",
		Intro: "What each environment must supply, from the configuration report; sign it off there.", Blocks: admin})

	st := Table{Columns: []string{"Report", "Assessed", "Note"}}
	for _, s := range d.Sources {
		a, cls := "yes", "ok"
		if !s.Assessed {
			a, cls = "not assessed", "warn"
			if !s.Decides {
				cls = "info"
			}
		}
		st.Rows = append(st.Rows, []string{"[" + KindLabel(s.Kind) + "](" + s.Kind + ".html)", a, s.Note})
		st.Classes = append(st.Classes, []string{"", cls, ""})
	}
	doc.Sections = append(doc.Sections, Section{ID: "sources", Title: "What was assessed",
		Intro: "A report not given, or one that could not check, is not assessed — never passing.", Blocks: []Block{st}})
	doc.Sections = append(doc.Sections, Section{ID: "not-covered", Title: "Not covered by the model",
		Intro: "Checks to do elsewhere, every time: no report on this page can make them.", Blocks: []Block{Bullets{Items: d.NotCovered}}})
	return Frame(env, ann, doc), nil
}

// ReadinessKinds are the reports readiness reads, as `report all` names their
// files (<kind>.json; the scored report is quality.json).
var ReadinessKinds = []string{"quality", "release", "dependencies", "configuration", "security", "tests", "traceability", "performance", "architecture"}

// ReadinessInputFrom decodes the other reports' envelopes. A kind missing
// from envs stays nil: not given.
func ReadinessInputFrom(envs map[string]*Envelope, quality *linter.JSONReport, ann map[string]*Annotations) (ReadinessInput, error) {
	in := ReadinessInput{Quality: quality, Annotations: ann, Warnings: map[string][]string{}}
	for kind, env := range envs {
		if env != nil && len(env.Warnings) > 0 {
			in.Warnings[kind] = env.Warnings
		}
	}
	var errs []string
	note := func(err error) {
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	var err error
	in.Architecture, err = decodeKind[ArchitectureData](envs, "architecture")
	note(err)
	in.Security, err = decodeKind[SecurityData](envs, "security")
	note(err)
	in.Configuration, err = decodeKind[ConfigurationData](envs, "configuration")
	note(err)
	in.Dependencies, err = decodeKind[DependencyData](envs, "dependencies")
	note(err)
	in.Tests, err = decodeKind[TestsData](envs, "tests")
	note(err)
	in.Traceability, err = decodeKind[TraceabilityData](envs, "traceability")
	note(err)
	in.Performance, err = decodeKind[PerformanceData](envs, "performance")
	note(err)
	in.Release, err = decodeKind[ReleaseData](envs, "release")
	note(err)
	if len(errs) > 0 {
		return in, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return in, nil
}

// decodeKind decodes one report's data; nil when it was not given.
func decodeKind[T any](envs map[string]*Envelope, kind string) (*T, error) {
	env, ok := envs[kind]
	if !ok || env == nil {
		return nil, nil
	}
	v := new(T)
	if err := env.Decode(v); err != nil {
		return nil, fmt.Errorf("%s: %w", kind, err)
	}
	return v, nil
}
