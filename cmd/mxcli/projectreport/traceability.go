// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

// Milestones groups plan slices — and single requirements placed apart from
// their slice — into the specification's milestones. It is the one input the
// traceability report cannot derive: the brain knows slices, the PRD knows
// milestones.
//
//	{"milestones": [
//	  {"name": "M1 — accounts", "slices": ["01-accounts"]},
//	  {"name": "M2 — approvals", "slices": ["02-approvals"], "requirements": ["a1b2c3"]}
//	]}
type Milestones struct {
	Milestones []MilestoneSpec `json:"milestones"`
}

// MilestoneSpec is one milestone.
type MilestoneSpec struct {
	Name         string   `json:"name"`
	Slices       []string `json:"slices,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
}

// ReadMilestones reads a milestones file.
func ReadMilestones(path string) (*Milestones, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Milestones
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

// TraceabilityInput is what the traceability report reads.
type TraceabilityInput struct {
	Requirements []brain.RequirementResult
	Entries      []brain.ListedEntry
	Milestones   *Milestones
	Evidence     *TestsData // nil: no tests or journeys were read
	ModelAbsent  bool
	// HelperCallers is the caller count above which a callee is a shared
	// helper and not part of any one requirement's footprint.
	HelperCallers int
}

// TraceabilityData is the collected traceability report.
type TraceabilityData struct {
	ModelAbsent bool             `json:"modelAbsent,omitempty"`
	HasTests    bool             `json:"hasTests"`
	HasJourneys bool             `json:"hasJourneys"`
	Milestones  []TraceMilestone `json:"milestones"`
	Questions   []TraceQuestion  `json:"questions"`
	// Uncaptured is milestone file entries that name no slice or requirement.
	Uncaptured []string `json:"uncaptured,omitempty"`
}

// TraceMilestone is a milestone and its requirements.
type TraceMilestone struct {
	Name         string             `json:"name"`
	Requirements []TraceRequirement `json:"requirements"`
}

// TraceRequirement is one requirement with its status and evidence.
type TraceRequirement struct {
	ID            string               `json:"id"`
	Slice         string               `json:"slice"`
	Title         string               `json:"title"`
	Spec          string               `json:"spec,omitempty"`
	Status        string               `json:"status"`
	StatusReason  string               `json:"statusReason,omitempty"`
	Anchors       []brain.AnchorResult `json:"anchors"`
	Footprint     []string             `json:"footprint"`
	FootprintNote string               `json:"footprintNote,omitempty"`
	Tests         []string             `json:"tests"`
	Journeys      []string             `json:"journeys"`
	Decisions     []string             `json:"decisions"`
	Questions     []string             `json:"questions"`
}

// TraceQuestion is an open question that bears on one or more requirements.
type TraceQuestion struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Requirements []string `json:"requirements"`
}

// Requirement statuses. They are derived, never ticked off by hand.
const (
	StatusBuilt        = "built"
	StatusPartial      = "partial"
	StatusChanged      = "changed"
	StatusNotBuilt     = "not built"
	StatusNotTraceable = "not traceable"
)

var specRef = regexp.MustCompile(`\(((?:slide|p\.|page|§)\s*[\w.-]+)\)\s*$`)

// footprintKinds names, per anchor kind, which microflows implement it. A kind
// not listed here (role, enumeration, constant, module) has no microflow
// footprint, and the page says so rather than calling it untested.
func footprint(q Querier, a brain.AnchorResult, mfModule map[string]string, g CallGraph) ([]string, error) {
	name := strings.TrimPrefix(a.Anchor, "@")
	lit := strings.ReplaceAll(name, "'", "''")
	var sql string
	switch strings.ToLower(a.Kind) {
	case "microflow":
		// The microflow plus what it calls within its own module: helpers in
		// other modules are other requirements' (or nobody's) work.
		mod := mfModule[name]
		var out []string
		for _, m := range g.Closure([]string{name}) {
			if mfModule[m] == mod {
				out = append(out, m)
			}
		}
		return out, nil
	case "page", "snippet", "workflow", "scheduled_event", "published_rest_service", "published_odata_service",
		"published_rest_operation", "business_event_service", "rest_operation", "import_mapping", "export_mapping":
		sql = `SELECT DISTINCT TargetName FROM refs WHERE TargetType = 'MICROFLOW'
			AND (SourceName = '` + lit + `' OR SourceName LIKE '` + lit + `.%')`
	case "entity":
		sql = `SELECT DISTINCT r.SourceName AS TargetName FROM refs r WHERE r.SourceType = 'MICROFLOW' AND r.TargetName = '` + lit + `'
			AND (r.RefKind IN ('create', 'change', 'delete', 'commit')
				OR (r.RefKind = 'retrieve' AND EXISTS (SELECT 1 FROM entities e WHERE e.QualifiedName = '` + lit + `'
					AND upper(COALESCE(e.EntityType, '')) LIKE '%VIEW%')))`
	case "attribute", "association":
		sql = `SELECT DISTINCT SourceName AS TargetName FROM refs WHERE SourceType = 'MICROFLOW' AND TargetName = '` + lit + `'`
	default:
		return nil, nil
	}
	rs, err := rows(q, sql)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range rs {
		out = append(out, r["TargetName"])
	}
	return out, nil
}

// CollectTraceability derives each requirement's status and evidence.
func CollectTraceability(q Querier, in TraceabilityInput) (*TraceabilityData, error) {
	if in.HelperCallers <= 0 {
		in.HelperCallers = 3
	}
	d := &TraceabilityData{ModelAbsent: in.ModelAbsent}
	if in.Evidence != nil {
		d.HasTests = len(in.Evidence.Tests) > 0
		d.HasJourneys = len(in.Evidence.Journeys) > 0
	}
	g := CallGraph{}
	mfModule := map[string]string{}
	callers := map[string]map[string]bool{}
	if q != nil && !in.ModelAbsent {
		var err error
		if g, err = LoadCallGraph(q); err != nil {
			return nil, err
		}
		for src, tgts := range g {
			for _, t := range tgts {
				if callers[t] == nil {
					callers[t] = map[string]bool{}
				}
				callers[t][src] = true
			}
		}
		ms, err := rows(q, `SELECT QualifiedName, ModuleName FROM microflows WHERE MicroflowType = 'MICROFLOW'`)
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			mfModule[m["QualifiedName"]] = m["ModuleName"]
		}
	}

	entry := map[string]brain.ListedEntry{}
	var decisions, questions []brain.ListedEntry
	verdicts := map[string][]brain.VerdictRef{}
	for _, e := range in.Entries {
		entry[e.ID] = e
		if e.EntryKind() == brain.KindRequirement && !e.Open {
			continue
		}
		if e.Open {
			questions = append(questions, e)
		} else {
			decisions = append(decisions, e)
			for _, v := range brain.Verdicts(e.Entry) {
				verdicts[v.RequirementID] = append(verdicts[v.RequirementID], v)
			}
		}
	}

	reqs := map[string]TraceRequirement{}
	questionReqs := map[string][]string{}
	for _, r := range in.Requirements {
		e := entry[r.ID]
		tr := TraceRequirement{ID: r.ID, Slice: r.Slice, Title: r.Title, Anchors: r.Anchors}
		if m := specRef.FindStringSubmatch(r.Title); m != nil {
			tr.Spec = m[1]
		}
		if e.Source != "" {
			tr.Spec = strings.TrimSpace(e.Source + " " + tr.Spec)
		}
		// Status.
		notFound := false
		for _, a := range r.Anchors {
			if a.State == brain.NotFound || a.State == brain.Unverifiable {
				notFound = true
			}
		}
		switch {
		case len(r.Anchors) == 0:
			tr.Status, tr.StatusReason = StatusNotTraceable, "no anchor: nothing in the model can be checked against it"
		case !r.Built || notFound:
			tr.Status = StatusNotBuilt
			var missing []string
			for _, a := range r.Anchors {
				if a.State != brain.Resolved && a.State != brain.NotIndexable {
					missing = append(missing, a.Anchor)
				}
			}
			tr.StatusReason = "not in the model: " + strings.Join(missing, ", ")
			if in.ModelAbsent {
				tr.StatusReason = "no model to check against yet"
			}
		default:
			tr.Status = StatusBuilt
		}
		for _, v := range verdicts[r.ID] {
			switch {
			case v.Verdict == brain.VerdictChanges:
				tr.Status, tr.StatusReason = StatusChanged, "decision "+v.EntryID+": "+v.Reason
			case v.Verdict == brain.VerdictPartly && tr.Status != StatusChanged:
				tr.Status, tr.StatusReason = StatusPartial, "decision "+v.EntryID+": "+v.Reason
			}
		}
		// Footprint.
		fp := map[string]bool{}
		hasFootprintKind := false
		for _, a := range r.Anchors {
			if a.State != brain.Resolved {
				continue
			}
			ms, err := footprint(q, a, mfModule, g)
			if err != nil {
				return nil, err
			}
			if ms != nil || isFootprintKind(a.Kind) {
				hasFootprintKind = true
			}
			anchored := strings.TrimPrefix(a.Anchor, "@")
			for _, m := range ms {
				if m != anchored && len(callers[m]) > in.HelperCallers {
					continue // a shared helper, not this requirement's work
				}
				fp[m] = true
			}
		}
		for m := range fp {
			tr.Footprint = append(tr.Footprint, m)
		}
		sort.Strings(tr.Footprint)
		if len(tr.Footprint) == 0 && tr.Status != StatusNotBuilt && tr.Status != StatusNotTraceable {
			if hasFootprintKind {
				tr.FootprintNote = "no microflow implements it yet (nothing writes or calls what it is anchored on)"
			} else {
				tr.FootprintNote = "no microflow implements this: it is anchored on a role, enumeration, constant or module"
			}
		}
		// Proof.
		if in.Evidence != nil {
			for _, t := range in.Evidence.Tests {
				if intersects(t.Reached, fp) {
					tr.Tests = append(tr.Tests, t.Name)
				}
			}
			for _, j := range in.Evidence.Journeys {
				if intersects(j.Executed, fp) || namesAnchoredPage(j.Pages, r.Anchors) {
					tr.Journeys = append(tr.Journeys, j.Name)
				}
			}
		}
		// Context.
		for _, dcs := range decisions {
			if sharesAnchor(dcs.Anchors, r.Anchors) || citesID(dcs.Entry, r.ID) {
				tr.Decisions = append(tr.Decisions, dcs.ID+": "+dcs.Title)
			}
		}
		for _, qs := range questions {
			if sharesAnchor(qs.Anchors, r.Anchors) || citesID(qs.Entry, r.ID) || (qs.Shard == brain.PlanShard(r.Slice)) {
				tr.Questions = append(tr.Questions, qs.ID+": "+qs.Title)
				questionReqs[qs.ID] = append(questionReqs[qs.ID], r.ID)
			}
		}
		reqs[r.ID] = tr
	}

	// Milestones.
	placed := map[string]bool{}
	bySlice := map[string][]string{}
	var sliceOrder []string
	for _, r := range in.Requirements {
		if _, ok := bySlice[r.Slice]; !ok {
			sliceOrder = append(sliceOrder, r.Slice)
		}
		bySlice[r.Slice] = append(bySlice[r.Slice], r.ID)
	}
	if in.Milestones != nil && len(in.Milestones.Milestones) > 0 {
		// Requirements placed individually win over their slice's milestone.
		moved := map[string]bool{}
		for _, m := range in.Milestones.Milestones {
			for _, id := range m.Requirements {
				moved[id] = true
			}
		}
		for _, m := range in.Milestones.Milestones {
			tm := TraceMilestone{Name: m.Name}
			for _, sl := range m.Slices {
				if _, ok := bySlice[sl]; !ok {
					d.Uncaptured = append(d.Uncaptured, fmt.Sprintf("milestone %q names slice %q, which has no requirements", m.Name, sl))
				}
				for _, id := range bySlice[sl] {
					if !moved[id] && !placed[id] {
						tm.Requirements = append(tm.Requirements, reqs[id])
						placed[id] = true
					}
				}
			}
			for _, id := range m.Requirements {
				if tr, ok := reqs[id]; ok && !placed[id] {
					tm.Requirements = append(tm.Requirements, tr)
					placed[id] = true
				} else if !ok {
					d.Uncaptured = append(d.Uncaptured, fmt.Sprintf("milestone %q names requirement %s, which the plan does not have", m.Name, id))
				}
			}
			d.Milestones = append(d.Milestones, tm)
		}
		var rest TraceMilestone
		rest.Name = "Not in any milestone"
		for _, sl := range sliceOrder {
			for _, id := range bySlice[sl] {
				if !placed[id] {
					rest.Requirements = append(rest.Requirements, reqs[id])
				}
			}
		}
		if len(rest.Requirements) > 0 {
			d.Milestones = append(d.Milestones, rest)
		}
	} else {
		for _, sl := range sliceOrder {
			tm := TraceMilestone{Name: sl}
			for _, id := range bySlice[sl] {
				tm.Requirements = append(tm.Requirements, reqs[id])
			}
			d.Milestones = append(d.Milestones, tm)
		}
	}
	for _, qs := range questions {
		if ids := questionReqs[qs.ID]; len(ids) > 0 {
			d.Questions = append(d.Questions, TraceQuestion{ID: qs.ID, Title: qs.Title, Requirements: ids})
		}
	}
	return d, nil
}

func isFootprintKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "microflow", "page", "snippet", "workflow", "entity", "attribute", "association", "scheduled_event",
		"published_rest_service", "published_odata_service", "published_rest_operation":
		return true
	}
	return false
}

func intersects(xs []string, set map[string]bool) bool {
	for _, x := range xs {
		if set[x] {
			return true
		}
	}
	return false
}

func namesAnchoredPage(pages []string, anchors []brain.AnchorResult) bool {
	for _, a := range anchors {
		if strings.EqualFold(a.Kind, "page") && containsStr(pages, strings.TrimPrefix(a.Anchor, "@")) {
			return true
		}
	}
	return false
}

func sharesAnchor(anchors []string, req []brain.AnchorResult) bool {
	for _, a := range anchors {
		an := "@" + strings.TrimPrefix(a, "@")
		for _, r := range req {
			// A module anchor on either side is too broad to link on.
			if an == r.Anchor && strings.Contains(an, ".") {
				return true
			}
		}
	}
	return false
}

func citesID(e brain.Entry, id string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(id) + `\b`).MatchString(e.Title + "\n" + e.Body)
}

// ---------------------------------------------------------------- render

var TraceabilityMethod = []string{
	"Requirements, decisions and open questions are read from the project brain (`docs/brain/`) with `brain list`; each requirement's anchor resolutions come from the same check `brain plan` uses, so \"built\" is defined in one place.",
	"Status is derived, never ticked off: built when every anchor resolves in the model; not built when one does not; not traceable without anchors; partial or changed when a decision says ``Partly meets requirement `<id>`: …`` or ``Changes requirement `<id>`: …`` (validated by `brain check`).",
	"Footprint: the microflows that implement a requirement — an anchored microflow plus its same-module callees; microflows a page, workflow, scheduled event or published service anchor calls; microflows that write an anchored entity or retrieve an anchored view entity; microflows that use an anchored attribute or association. Helpers with more than three callers are left out.",
	"A microflow test proves a requirement when what it calls, with their call closure, touches the footprint. A journey proves it when a footprint microflow appears in its spans, or its log names an anchored page.",
	"Decisions bear on a requirement when they share an anchor (module anchors excluded) or cite its id; open questions also when filed on its slice.",
	"Milestones come from `docs/brain/plan/traceability.json` (or `--milestones`); without one, each slice is a milestone.",
}

var TraceabilityLimits = []string{
	"A requirement appears only once it is captured in the brain. A PRD item nobody captured is invisible here — the report pushes the brain to be complete, it does not read the PRD.",
	"\"Built\" means its anchors exist, not that they do what was asked; that is what the tests column and the decisions are for.",
	"Pages whose data comes only from database data sources run no microflow, so spans cannot show a journey reaching them unless the journey log names the page.",
	"Footprints miss microflows reached only from Java actions (ako/mxcli#1819) and nanoflows.",
}

// RenderTraceability builds the traceability page.
func RenderTraceability(env *Envelope, ann *Annotations) (Doc, error) {
	var d TraceabilityData
	if err := env.Decode(&d); err != nil {
		return Doc{}, err
	}
	count := map[string]int{}
	total := 0
	var notBuilt, untested []string
	for _, m := range d.Milestones {
		for _, r := range m.Requirements {
			total++
			count[r.Status]++
			label := fmt.Sprintf("`%s` %s — %s", r.ID, r.Title, r.Status)
			if r.StatusReason != "" {
				label += " (" + r.StatusReason + ")"
			}
			if r.Status != StatusBuilt {
				notBuilt = append(notBuilt, label)
			}
			if (r.Status == StatusBuilt || r.Status == StatusPartial || r.Status == StatusChanged) &&
				len(r.Footprint) > 0 && len(r.Tests) == 0 && len(r.Journeys) == 0 {
				untested = append(untested, fmt.Sprintf("`%s` %s — footprint %s", r.ID, r.Title, strings.Join(codeList(r.Footprint), ", ")))
			}
		}
	}
	doc := Doc{Title: "Requirements traceability", Subtitle: "Each requirement: built, partial or changed, what implements it and what proves it"}
	doc.Stats = []Stat{
		{Label: "requirements", Value: fmt.Sprint(total)},
		{Label: "built", Value: fmt.Sprint(count[StatusBuilt]), Note: pct(count[StatusBuilt], total)},
		{Label: "partial / changed", Value: fmt.Sprintf("%d / %d", count[StatusPartial], count[StatusChanged])},
		{Label: "not built / not traceable", Value: fmt.Sprintf("%d / %d", count[StatusNotBuilt], count[StatusNotTraceable])},
		{Label: "built but untested", Value: fmt.Sprint(len(untested))},
	}
	var pre []Block
	if d.ModelAbsent {
		pre = append(pre, Callout{Kind: "warn", Text: "There is no model yet, so nothing counts as built."})
	}
	if !d.HasTests && !d.HasJourneys {
		pre = append(pre, Callout{Kind: "warn", Text: "No tests or journeys were read, so no requirement shows proof. Pass `--tests` / `--junit` and `--journeys` / `--spans`."})
	}
	for _, u := range d.Uncaptured {
		pre = append(pre, Callout{Kind: "warn", Text: "Milestones file: " + u})
	}
	if len(pre) > 0 {
		doc.Sections = append(doc.Sections, Section{ID: "inputs", Title: "Inputs", Blocks: pre})
	}
	for i, m := range d.Milestones {
		t := Table{Columns: []string{"Requirement", "Status", "Implemented by", "Tests", "Journeys", "Decisions", "Open questions"}}
		for _, r := range m.Requirements {
			title := fmt.Sprintf("`%s` %s", r.ID, r.Title)
			if r.Spec != "" && !strings.Contains(r.Title, r.Spec) {
				title += " (" + r.Spec + ")"
			}
			status := r.Status
			if r.StatusReason != "" {
				status += ": " + r.StatusReason
			}
			if n, ok := ann.Get("req:" + r.ID); ok {
				status = joinNote("", n.Note, status)
			}
			impl := strings.Join(codeList(r.Footprint), ", ")
			if impl == "" {
				impl = r.FootprintNote
			}
			t.Rows = append(t.Rows, []string{title, status, impl, strings.Join(r.Tests, "; "), strings.Join(r.Journeys, "; "),
				strings.Join(r.Decisions, "; "), strings.Join(r.Questions, "; ")})
			t.Classes = append(t.Classes, []string{"", statusClass(r.Status), "", proofClass(r), proofClass(r), "", ""})
		}
		doc.Sections = append(doc.Sections, Section{ID: fmt.Sprintf("m%d", i+1), Title: m.Name, Blocks: []Block{t}})
	}
	doc.Sections = append(doc.Sections,
		Section{ID: "gap-built", Title: "Not fully built as asked", Blocks: []Block{bulletsOrNone(notBuilt)}},
		Section{ID: "gap-untested", Title: "Built, but no test reaches it", Blocks: []Block{bulletsOrNone(untested)}},
	)
	var qs []string
	for _, q := range d.Questions {
		qs = append(qs, fmt.Sprintf("`%s` %s — affects %s", q.ID, q.Title, strings.Join(codeList(q.Requirements), ", ")))
	}
	doc.Sections = append(doc.Sections, Section{ID: "gap-questions", Title: "Open questions that affect a requirement", Blocks: []Block{bulletsOrNone(qs)}})
	return Frame(env, ann, doc), nil
}

func proofClass(r TraceRequirement) string {
	if r.Status != StatusBuilt && r.Status != StatusPartial && r.Status != StatusChanged {
		return ""
	}
	if len(r.Footprint) > 0 && len(r.Tests) == 0 && len(r.Journeys) == 0 {
		return "warn"
	}
	return ""
}

func bulletsOrNone(items []string) Block {
	if len(items) == 0 {
		return Para{Text: "None."}
	}
	return Bullets{Items: items}
}
