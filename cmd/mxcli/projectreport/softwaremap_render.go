// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

var MapMethod = []string{
	"Everything is read from the mxcli catalog (`refresh catalog full`): modules, documents, the reference graph (`CATALOG.REFS`), access rules, navigation and task queues. Nothing ran against the app.",
	"Domain use and screen links (`layout`, `show_page`, `menu_item`, `home_page`, `widget`, `snippet_call`) are counted apart; the module most others use is the hub and is hidden from the module map by default.",
	"Entry points are grouped by what starts them, from the reference that reaches the flow: a page button or data source, a menu item, a schedule, a published service, a project setting (startup), a workflow, a task queue (`queue`), an entity event, or a constant whose value is the flow's name (`by_name`).",
	"Page flows per role start at the role's home pages and the menu, then follow show-page actions, page buttons and the flows behind them, through snippets and layouts, limited to pages the role may view and flows it may execute.",
	"Index: attribute and enumeration-value references are folded into their entity or enumeration.",
	"Module purposes come from the module's domain-model documentation, or from `--annotations` (`module:<Name>` notes) when the model has none.",
	"Features (when the project has a brain with a plan) are its slices. A feature reaches its anchored flows and the requirements' footprints (as `report traceability` computes them), the flows behind buttons of its anchored pages, and the call closure of all of those; the pages anchored or opened by those flows; the entities anchored or written by them. Entry points are the reached flows something outside the feature starts; roles are the user roles that may open one of its pages. Milestones come from the plan.",
}

var MapLimits = []string{
	"Calls from Java actions are not in the reference graph (ako/mxcli#1819): a flow called only from Java has no caller here.",
	"A page the role may open but no click path reaches is listed, not judged: workflow task pages and links from notifications are reached that way on purpose.",
	"Page flows follow the model, not the running app: a button hidden by a condition still counts as a path.",
	"Clients of published services and the systems behind consumed ones are outside the model; the map names the services, not who calls them.",
	"A feature is only as complete as its anchors: a requirement anchored on one microflow of a larger flow shows that flow's closure, not the pages that lead to it. \"Reached by no feature\" is what no anchor leads to — unspecified, or reached only through navigation the plan does not anchor.",
}

// applyPurposes copies annotation notes keyed module:<Name> onto modules that
// have no purpose of their own, or that the person wants to overrule.
func applyPurposes(d *MapData, ann *Annotations) {
	for i := range d.Modules {
		if n, ok := ann.Get("module:" + d.Modules[i].Name); ok && n.Note != "" {
			d.Modules[i].Purpose = n.Note
		}
	}
}

// RenderMap builds the markdown summary of the map. The interactive views —
// graphs, flow closures, per-role page flows and the searchable index — are
// the HTML page's; markdown lists what fits in tables.
func RenderMap(env *Envelope, ann *Annotations) (Doc, error) {
	var d MapData
	if err := env.Decode(&d); err != nil {
		return Doc{}, err
	}
	applyPurposes(&d, ann)
	entries := 0
	for _, f := range d.Flows {
		if len(f.Triggers) > 0 {
			entries++
		}
	}
	doc := Doc{Title: "Software map", Subtitle: "What the app contains and how it fits together"}
	doc.Stats = []Stat{
		{Label: "app modules", Value: fmt.Sprint(len(d.Modules)), Note: nonEmpty(hubNote(d.Hub), "")},
		{Label: "entities", Value: fmt.Sprint(len(d.Entities))},
		{Label: "microflows and nanoflows", Value: fmt.Sprint(len(d.Flows)), Note: fmt.Sprintf("%d entry points", entries)},
		{Label: "pages", Value: fmt.Sprint(len(d.Pages))},
		{Label: "documents indexed", Value: fmt.Sprint(len(d.Index))},
	}

	// Landscape.
	rt := Table{Caption: "User roles", Columns: []string{"Role", "Starts at", "Pages reached", "May open, no path reaches"}}
	for _, r := range d.Roles {
		name := r.Name
		if r.Anonymous {
			name += " (anonymous)"
		}
		rt.Rows = append(rt.Rows, []string{name, strings.Join(codeList(r.Starts), ", "), fmt.Sprintf("%d of %d", len(r.Reachable), r.Allowed), fmt.Sprint(len(r.Unreached))})
	}
	st := Table{Caption: "Systems around the app", Columns: []string{"Kind", "Name", "Detail"}}
	for _, s := range d.Systems {
		st.Rows = append(st.Rows, []string{s.Kind, "`" + s.Name + "`", s.Detail})
	}
	bt := Table{Caption: "Runs without a user", Columns: []string{"Started by", "Source", "Flow"}}
	for _, t := range d.Background {
		bt.Rows = append(bt.Rows, []string{t.Kind, "`" + t.Source + "`", "`" + t.Flow + "`"})
	}
	doc.Sections = append(doc.Sections, Section{ID: "landscape", Title: "Landscape", Blocks: []Block{rt, st, bt}})

	if len(d.Features) > 0 {
		doc.Sections = append(doc.Sections, featureSection(d))
	}

	// Modules.
	mt := Table{Columns: []string{"Module", "Purpose", "Entities", "Microflows", "Pages", "Entry points"}}
	for _, m := range d.Modules {
		var eps []string
		for _, k := range EntryKinds {
			if n := m.EntryPoints[k]; n > 0 {
				eps = append(eps, fmt.Sprintf("%s %d", k, n))
			}
		}
		mt.Rows = append(mt.Rows, []string{m.Name, m.Purpose, fmt.Sprint(m.Counts["entity"]), fmt.Sprint(m.Counts["microflow"]), fmt.Sprint(m.Counts["page"]), strings.Join(eps, ", ")})
	}
	et := Table{Caption: "Module dependencies", Columns: []string{"From", "To", "Domain refs", "Screen links"}}
	for _, e := range d.ModuleEdges {
		et.Rows = append(et.Rows, []string{e.From, e.To, fmt.Sprint(e.Domain), fmt.Sprint(e.Navigation)})
	}
	doc.Sections = append(doc.Sections, Section{ID: "modules", Title: "Modules",
		Intro:  "Annotate `module:<Name>` with one sentence for a module whose domain model has no documentation.",
		Blocks: []Block{mt, et, Figure{Fallback: "The interactive module map, per-module data models and flow views are in the HTML page (`--format html`)."}}})

	// Data across modules.
	ct := Table{Columns: []string{"Modules", "Association", "From", "To", "Type"}}
	for _, a := range crossModuleAssocs(d.Assocs) {
		ct.Rows = append(ct.Rows, []string{moduleOf(a.From) + " → " + moduleOf(a.To), "`" + a.Name + "`", "`" + a.From + "`", "`" + a.To + "`", a.Type})
	}
	doc.Sections = append(doc.Sections, Section{ID: "data", Title: "Data across modules", Blocks: []Block{ct}})

	// Conventions.
	pt := Table{Caption: "Naming prefixes", Columns: []string{"Prefix", "Flows", "Modules"}}
	for _, p := range d.Prefixes {
		pt.Rows = append(pt.Rows, []string{p.Prefix + "_", fmt.Sprint(p.Total), countList(p.ByModule)})
	}
	sh := Table{Caption: "Flows called from three or more modules", Columns: []string{"Flow", "Called from"}}
	for _, s := range d.SharedFlows {
		sh.Rows = append(sh.Rows, []string{"`" + s.Flow + "`", strings.Join(s.Callers, ", ")})
	}
	em := Table{Caption: "Entry points × module", Columns: append([]string{"Module"}, EntryKinds...)}
	for _, m := range d.Modules {
		row := []string{m.Name}
		for _, k := range EntryKinds {
			row = append(row, cellCount(m.EntryPoints[k]))
		}
		em.Rows = append(em.Rows, row)
	}
	doc.Sections = append(doc.Sections, Section{ID: "conventions", Title: "Logic conventions", Blocks: []Block{pt, sh, em}})

	// APIs and background.
	ot := Table{Caption: "Published REST operations", Columns: []string{"Service", "Operation", "Microflow", "Call closure"}}
	for _, o := range d.Published {
		ot.Rows = append(ot.Rows, []string{"`" + o.Service + "`", o.Method + " " + o.Path, "`" + o.Microflow + "`", fmt.Sprint(o.Closure)})
	}
	cn := Table{Caption: "Consumed services and import mappings", Columns: []string{"Document", "What"}}
	for _, c := range d.Consumed {
		cn.Rows = append(cn.Rows, []string{"`" + c.Name + "`", c.Note})
	}
	hc := Table{Caption: "Flows making HTTP calls", Columns: []string{"Flow", "Calls"}}
	for _, h := range d.HTTPCalls {
		hc.Rows = append(hc.Rows, []string{"`" + h.From + "`", h.To})
	}
	qt := Table{Caption: "Task queues", Columns: []string{"Queue", "Runs", "Put there by"}}
	for _, x := range d.Queues {
		qt.Rows = append(qt.Rows, []string{"`" + x.Name + "`", strings.Join(codeList(x.Runs), ", "), strings.Join(codeList(x.Callers), ", ")})
	}
	jt := Table{Caption: "Java actions", Columns: []string{"Java action", "Does"}}
	for _, j := range d.JavaActions {
		jt.Rows = append(jt.Rows, []string{"`" + j.Name + "`", j.Note})
	}
	kt := Table{Caption: "Constants", Columns: []string{"Constant", "Type", "Default", "Names flow"}}
	for _, c := range d.Constants {
		kt.Rows = append(kt.Rows, []string{"`" + c.Name + "`", c.Type, c.Default, c.NamesFlow})
	}
	doc.Sections = append(doc.Sections, Section{ID: "apis", Title: "APIs and background work", Blocks: []Block{ot, cn, hc, qt, jt, kt}})
	return Frame(env, ann, doc), nil
}

// featureSection is the app by feature: one row per plan slice, and what no
// feature reaches.
func featureSection(d MapData) Section {
	ft := Table{Columns: []string{"Feature", "Milestone", "Requirements", "Modules", "Pages", "Flows", "Entities", "Entry points", "Roles that may open it", "Shares with"}}
	for _, f := range d.Features {
		var eps []string
		for _, e := range f.EntryPoints {
			eps = append(eps, e.Kind+" `"+e.Flow+"` ("+e.Source+")")
		}
		ft.Rows = append(ft.Rows, []string{f.Name, f.Milestone, fmt.Sprintf("%d (%s)", len(f.Requirements), featureStatus(f.Status)),
			strings.Join(f.Modules, ", "), strings.Join(codeList(f.Pages), ", "), fmt.Sprintf("%d (%d anchored)", len(f.Flows), len(f.Core)),
			strings.Join(codeList(f.Entities), ", "), strings.Join(eps, ", "), strings.Join(f.Roles, ", "), strings.Join(f.SharesWith, ", ")})
	}
	blocks := []Block{ft}
	if u := d.Unowned; u != nil {
		ut := Table{Caption: "Reached by no feature", Columns: []string{"Kind", "Count", "Documents"}}
		for _, r := range []struct {
			kind string
			xs   []string
		}{{"pages", u.Pages}, {"entry points", u.EntryPoints}, {"persistent entities", u.Entities}} {
			ut.Rows = append(ut.Rows, []string{r.kind, fmt.Sprint(len(r.xs)), strings.Join(codeList(r.xs), ", ")})
		}
		blocks = append(blocks, ut)
	}
	return Section{ID: "features", Title: "Features",
		Intro:  "Each feature is a slice of the project brain's plan. What belongs to it is derived from its requirements' anchors: the anchored flows and their footprints with their call closure, the pages anchored or opened by those flows, and the entities anchored or written by them. What no feature reaches is the model nobody specified.",
		Blocks: blocks}
}

func hubNote(hub string) string {
	if hub == "" {
		return ""
	}
	return "hub: " + hub
}

func crossModuleAssocs(as []MapAssoc) []MapAssoc {
	var out []MapAssoc
	for _, a := range as {
		if moduleOf(a.From) != moduleOf(a.To) {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := moduleOf(out[i].From)+moduleOf(out[i].To), moduleOf(out[j].From)+moduleOf(out[j].To)
		if pi != pj {
			return pi < pj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func countList(m map[string]int) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		if m[ks[i]] != m[ks[j]] {
			return m[ks[i]] > m[ks[j]]
		}
		return ks[i] < ks[j]
	})
	var out []string
	for _, k := range ks {
		out = append(out, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(out, ", ")
}

// RenderMapHTML writes the interactive page: the data embedded as JSON, the
// views drawn by a small script with no external requests.
func RenderMapHTML(w io.Writer, env *Envelope, ann *Annotations) error {
	var d MapData
	if err := env.Decode(&d); err != nil {
		return err
	}
	applyPurposes(&d, ann)
	title := "Software map — " + env.Project
	if ann != nil && ann.Title != "" {
		title = ann.Title
	}
	payload := struct {
		Title     string   `json:"title"`
		Project   string   `json:"project"`
		Generated string   `json:"generated"`
		Mxcli     string   `json:"mxcli"`
		Summary   string   `json:"summary,omitempty"`
		Method    []string `json:"method"`
		Limits    []string `json:"limits"`
		Warnings  []string `json:"warnings,omitempty"`
		Kinds     []string `json:"entryKinds"`
		Nav       []string `json:"navigationKinds"`
		Data      MapData  `json:"data"`
	}{title, env.Project, env.Generated, env.Mxcli, "", env.Method, env.Limits, env.Warnings, EntryKinds, NavigationKinds, d}
	if ann != nil {
		payload.Summary = ann.Summary
	}
	// json.Marshal escapes <, > and &, so the payload cannot close the script
	// element it is embedded in.
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title><style>%s%s</style></head>
<body><main id="app"><header><h1>%s</h1><p class="sub" id="sub"></p></header>
<nav class="tabs" id="tabs"></nav><div id="view"></div></main>
<script id="map-data" type="application/json">%s</script>
<script>%s</script></body></html>
`, esc(title), pageCSS, mapCSS, esc(title), b, mapJS)
	return err
}
