// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The software map (ako/mxcli#2297) describes what an app contains and how it
// fits together, for someone who did not build it. Where the architecture
// report judges structure, the map is a guide: the landscape around the app,
// the module graph, each module's data, entry points and flows, page flows per
// role, cross-module data, conventions, APIs and background work, and an index
// of every document.
//
// Everything is read from the catalog. The data is collected once into
// MapData; the HTML page embeds it and draws the diagrams in the browser, one
// module or one entry point at a time — the prototype measured that any
// "everything at once" graph is unreadable beyond ~30 nodes.

// MapData is the collected software map.
type MapData struct {
	Scope       Scope          `json:"scope"`
	Hub         string         `json:"hub,omitempty"`
	Modules     []MapModule    `json:"modules"`
	ModuleEdges []MapEdge      `json:"moduleEdges"`
	Roles       []MapRole      `json:"roles"`
	Systems     []MapSystem    `json:"systems"`
	Background  []MapTrigger   `json:"background"`
	Entities    []MapEntity    `json:"entities"`
	Assocs      []MapAssoc     `json:"associations"`
	Flows       []MapFlow      `json:"flows"`
	Pages       []MapPage      `json:"pages"`
	PageEdges   []MapLink      `json:"pageEdges"`
	JavaActions []MapDoc       `json:"javaActions"`
	Constants   []MapConstant  `json:"constants"`
	Published   []MapOperation `json:"published"`
	Consumed    []MapDoc       `json:"consumed"`
	HTTPCalls   []MapLink      `json:"httpCalls"`
	Queues      []MapQueue     `json:"queues"`
	Prefixes    []MapPrefix    `json:"prefixes"`
	SharedFlows []MapShared    `json:"sharedFlows"`
	Index       []MapIndexDoc  `json:"index"`
	// Features is the app by plan slice, when the project has a brain with
	// requirements (AddFeatures); Unowned is what no feature reaches.
	Features []MapFeature `json:"features,omitempty"`
	Unowned  *MapUnowned  `json:"unowned,omitempty"`
}

// MapModule is one app module.
type MapModule struct {
	Name        string         `json:"name"`
	Purpose     string         `json:"purpose,omitempty"`
	Counts      map[string]int `json:"counts"`
	Integration bool           `json:"integration,omitempty"`
	EntryPoints map[string]int `json:"entryPoints"`
}

// MapEdge is module → module references, split by class.
type MapEdge struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Domain     int    `json:"domain"`
	Navigation int    `json:"navigation"`
}

// MapRole is a user role with where it starts and what it reaches.
type MapRole struct {
	Name        string   `json:"name"`
	ModuleRoles []string `json:"moduleRoles"`
	Anonymous   bool     `json:"anonymous,omitempty"`
	Starts      []string `json:"starts"`
	Reachable   []string `json:"reachable"`
	// Unreached are pages the role may open but no click path from its
	// starts reaches: workflow task pages, notification links, dead pages.
	Unreached []string `json:"unreached"`
	Allowed   int      `json:"allowed"`
}

// MapSystem is something outside the app it talks to.
type MapSystem struct {
	Kind   string `json:"kind"` // identity | published REST | published OData | consumed REST | consumed OData | business events | integration module
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
}

// MapTrigger is a way a flow runs without a user.
type MapTrigger struct {
	Kind   string `json:"kind"` // startup | schedule | task queue | workflow | entity event | constant
	Source string `json:"source"`
	Flow   string `json:"flow"`
}

// MapEntity is an entity with its attributes and writers.
type MapEntity struct {
	Name       string   `json:"name"`
	Module     string   `json:"module"`
	Persistent bool     `json:"persistent"`
	Generalize string   `json:"generalization,omitempty"`
	Attributes []string `json:"attributes"`
	WrittenBy  []string `json:"writtenBy"`
}

// MapAssoc is an association.
type MapAssoc struct {
	Name   string `json:"name"`
	From   string `json:"from"`
	To     string `json:"to"`
	Type   string `json:"type"`
	Module string `json:"module"`
}

// MapFlow is a microflow or nanoflow with its calls and what starts it.
type MapFlow struct {
	Name     string       `json:"name"`
	Module   string       `json:"module"`
	Nanoflow bool         `json:"nanoflow,omitempty"`
	Calls    []string     `json:"calls,omitempty"`
	Pages    []string     `json:"pages,omitempty"`
	Triggers []MapTrigger `json:"triggers,omitempty"`
}

// MapPage is a page.
type MapPage struct {
	Name   string   `json:"name"`
	Module string   `json:"module"`
	Title  string   `json:"title,omitempty"`
	Popup  bool     `json:"popup,omitempty"`
	Roles  []string `json:"roles,omitempty"`
	Flows  []string `json:"flows,omitempty"`
}

// MapLink is a named edge.
type MapLink struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind,omitempty"`
}

// MapDoc is a named document with a module and a note.
type MapDoc struct {
	Name   string `json:"name"`
	Module string `json:"module"`
	Note   string `json:"note,omitempty"`
}

// MapConstant is a constant.
type MapConstant struct {
	Name      string `json:"name"`
	Module    string `json:"module"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	NamesFlow string `json:"namesFlow,omitempty"`
}

// MapOperation is a published REST operation.
type MapOperation struct {
	Service   string `json:"service"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Microflow string `json:"microflow"`
	Closure   int    `json:"closure"`
}

// MapQueue is a task queue with what runs in it and who puts it there.
type MapQueue struct {
	Name    string   `json:"name"`
	Runs    []string `json:"runs"`
	Callers []string `json:"callers"`
}

// MapPrefix is a naming prefix with its count per module.
type MapPrefix struct {
	Prefix   string         `json:"prefix"`
	Total    int            `json:"total"`
	ByModule map[string]int `json:"byModule"`
}

// MapShared is a flow called from several modules.
type MapShared struct {
	Flow    string   `json:"flow"`
	Callers []string `json:"callerModules"`
}

// MapIndexDoc is one document with what it uses and what uses it.
type MapIndexDoc struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Module string   `json:"module"`
	Uses   []string `json:"uses,omitempty"`
	UsedBy []string `json:"usedBy,omitempty"`
}

// Entry-point kinds, in the order the page lists them.
var EntryKinds = []string{"page button", "page data source", "menu", "schedule", "REST / service", "startup",
	"workflow", "task queue", "entity event", "constant", "calculated attribute"}

func entryKind(kind, srcType string) string {
	switch kind {
	case "action":
		return "page button"
	case "datasource":
		return "page data source"
	case "menu_item", "home_page":
		return "menu"
	case "schedule":
		return "schedule"
	case "publish":
		return "REST / service"
	case "settings":
		return "startup"
	case "event":
		return "entity event"
	case "by_name":
		return "constant"
	case "queue":
		return "task queue"
	case "calculate":
		return "calculated attribute"
	case "call":
		if srcType == "WORKFLOW" {
			return "workflow"
		}
	}
	return ""
}

var flowPrefix = regexp.MustCompile(`^([A-Za-z]{2,6})_`)

// CollectMap reads the catalog into a software map.
func CollectMap(q Querier, opt ScopeOptions, purposes map[string]string) (*MapData, error) {
	scope, err := LoadScope(q, opt)
	if err != nil {
		return nil, err
	}
	d := &MapData{Scope: scope}
	app := setOf(scope.App)
	appList := sqlList(scope.App)
	known := setOf(append(append([]string{}, scope.App...), scope.Outside...))
	nav := setOf(NavigationKinds)
	inApp := func(qn string) bool { return app[moduleOf(qn)] }

	// ---- modules
	mods, err := rows(q, `SELECT Name, COALESCE(DomainModelDocumentation, '') AS Doc, COALESCE(Description, '') AS Descr FROM modules_data`)
	if err != nil {
		return nil, err
	}
	purpose := map[string]string{}
	for _, m := range mods {
		purpose[m["Name"]] = strings.TrimSpace(nonEmpty(m["Descr"], m["Doc"]))
	}
	counts := map[string]map[string]int{}
	cs, err := rows(q, `SELECT ModuleName, ObjectType, COUNT(*) AS N FROM objects WHERE ModuleName IN (`+appList+`) GROUP BY 1, 2`)
	if err != nil {
		return nil, err
	}
	for _, r := range cs {
		if counts[r["ModuleName"]] == nil {
			counts[r["ModuleName"]] = map[string]int{}
		}
		counts[r["ModuleName"]][strings.ToLower(r["ObjectType"])] = atoi(r["N"])
	}
	modIdx := map[string]int{}
	for _, m := range scope.App {
		c := counts[m]
		if c == nil {
			c = map[string]int{}
		}
		integ := c["import_mapping"] + c["export_mapping"] + c["consumed_rest_service"] + c["published_rest_service"] +
			c["consumed_odata_service"] + c["published_odata_service"] + c["json_structure"] + c["business_event_service"]
		p := purpose[m]
		if v := purposes[m]; v != "" {
			p = v
		}
		modIdx[m] = len(d.Modules)
		d.Modules = append(d.Modules, MapModule{Name: m, Purpose: firstSentence(p), Counts: c, Integration: integ > 0, EntryPoints: map[string]int{}})
	}

	// ---- refs, read once
	refRows, err := rows(q, `SELECT SourceType, SourceName, TargetType, TargetName, RefKind FROM refs WHERE SourceName != '' AND TargetName != ''`)
	if err != nil {
		return nil, err
	}

	// ---- module edges
	type pair struct{ a, b string }
	edges := map[pair]*MapEdge{}
	fanIn := map[string]map[string]bool{}
	for _, r := range refRows {
		sm, tm := moduleOf(r["SourceName"]), moduleOf(r["TargetName"])
		if sm == tm || !app[sm] || !app[tm] || !known[sm] {
			continue
		}
		e := edges[pair{sm, tm}]
		if e == nil {
			e = &MapEdge{From: sm, To: tm}
			edges[pair{sm, tm}] = e
		}
		if nav[r["RefKind"]] {
			e.Navigation++
		} else {
			e.Domain++
			if fanIn[tm] == nil {
				fanIn[tm] = map[string]bool{}
			}
			fanIn[tm][sm] = true
		}
	}
	for _, e := range edges {
		d.ModuleEdges = append(d.ModuleEdges, *e)
	}
	sort.Slice(d.ModuleEdges, func(i, j int) bool {
		return d.ModuleEdges[i].From+"\x00"+d.ModuleEdges[i].To < d.ModuleEdges[j].From+"\x00"+d.ModuleEdges[j].To
	})
	// The hub is the module most others use: it is hidden by default because
	// every module's arrow to it turns the graph into a hairball.
	best := 0
	for _, m := range scope.App {
		if n := len(fanIn[m]); n > best && n*2 >= len(scope.App)-1 && len(scope.App) > 3 {
			best, d.Hub = n, m
		}
	}

	// ---- flows
	fl, err := rows(q, `SELECT QualifiedName, ModuleName, MicroflowType FROM microflows_data
		WHERE ModuleName IN (`+appList+`) AND MicroflowType IN ('MICROFLOW', 'NANOFLOW') ORDER BY QualifiedName`)
	if err != nil {
		return nil, err
	}
	flowIdx := map[string]int{}
	for _, r := range fl {
		flowIdx[r["QualifiedName"]] = len(d.Flows)
		d.Flows = append(d.Flows, MapFlow{Name: r["QualifiedName"], Module: r["ModuleName"], Nanoflow: r["MicroflowType"] == "NANOFLOW"})
	}
	callerMods := map[string]map[string]bool{}
	for _, r := range refRows {
		src, tgt, kind := r["SourceName"], r["TargetName"], r["RefKind"]
		i, isFlow := flowIdx[tgt]
		if r["SourceType"] == "MICROFLOW" || r["SourceType"] == "NANOFLOW" {
			if si, ok := flowIdx[src]; ok {
				switch {
				case kind == "call" && (r["TargetType"] == "MICROFLOW" || r["TargetType"] == "NANOFLOW"):
					d.Flows[si].Calls = appendUnique(d.Flows[si].Calls, tgt)
				case kind == "show_page" && r["TargetType"] == "PAGE":
					d.Flows[si].Pages = appendUnique(d.Flows[si].Pages, tgt)
				}
			}
			if kind == "call" && isFlow && moduleOf(src) != moduleOf(tgt) {
				if callerMods[tgt] == nil {
					callerMods[tgt] = map[string]bool{}
				}
				callerMods[tgt][moduleOf(src)] = true
			}
			continue
		}
		if !isFlow {
			continue
		}
		if ek := entryKind(kind, r["SourceType"]); ek != "" {
			t := MapTrigger{Kind: ek, Source: src, Flow: tgt}
			if !containsTrigger(d.Flows[i].Triggers, t) {
				d.Flows[i].Triggers = append(d.Flows[i].Triggers, t)
			}
		}
	}
	// Menu items that call a microflow are not always refs rows; add them.
	mi, err := rows(q, `SELECT COALESCE(TargetMicroflow, '') AS M, Caption FROM navigation_menu_items WHERE COALESCE(TargetMicroflow, '') != ''`)
	if err != nil {
		return nil, err
	}
	for _, r := range mi {
		if i, ok := flowIdx[r["M"]]; ok {
			t := MapTrigger{Kind: "menu", Source: "menu: " + r["Caption"], Flow: r["M"]}
			if !containsTrigger(d.Flows[i].Triggers, t) {
				d.Flows[i].Triggers = append(d.Flows[i].Triggers, t)
			}
		}
	}
	for _, f := range d.Flows {
		seen := map[string]bool{}
		for _, t := range f.Triggers {
			if !seen[t.Kind] {
				seen[t.Kind] = true
				d.Modules[modIdx[f.Module]].EntryPoints[t.Kind]++
			}
			switch t.Kind {
			case "startup", "schedule", "task queue", "workflow", "entity event", "constant":
				d.Background = append(d.Background, t)
			}
		}
	}
	sort.SliceStable(d.Background, func(i, j int) bool { return kindOrder(d.Background[i].Kind) < kindOrder(d.Background[j].Kind) })
	for flow, ms := range callerMods {
		if len(ms) >= 3 && inApp(flow) {
			var callers []string
			for m := range ms {
				callers = append(callers, m)
			}
			sort.Strings(callers)
			d.SharedFlows = append(d.SharedFlows, MapShared{Flow: flow, Callers: callers})
		}
	}
	sort.Slice(d.SharedFlows, func(i, j int) bool {
		if len(d.SharedFlows[i].Callers) != len(d.SharedFlows[j].Callers) {
			return len(d.SharedFlows[i].Callers) > len(d.SharedFlows[j].Callers)
		}
		return d.SharedFlows[i].Flow < d.SharedFlows[j].Flow
	})

	// ---- naming prefixes
	pre := map[string]*MapPrefix{}
	for _, f := range d.Flows {
		m := flowPrefix.FindStringSubmatch(shortName(f.Name))
		if m == nil {
			continue
		}
		p := strings.ToUpper(m[1])
		x := pre[p]
		if x == nil {
			x = &MapPrefix{Prefix: p, ByModule: map[string]int{}}
			pre[p] = x
		}
		x.Total++
		x.ByModule[f.Module]++
	}
	for _, x := range pre {
		if x.Total >= 2 {
			d.Prefixes = append(d.Prefixes, *x)
		}
	}
	sort.Slice(d.Prefixes, func(i, j int) bool {
		if d.Prefixes[i].Total != d.Prefixes[j].Total {
			return d.Prefixes[i].Total > d.Prefixes[j].Total
		}
		return d.Prefixes[i].Prefix < d.Prefixes[j].Prefix
	})

	if err := collectMapData(q, d, appList, refRows, flowIdx); err != nil {
		return nil, err
	}
	if err := collectMapPages(q, d, appList, refRows); err != nil {
		return nil, err
	}
	if err := collectMapLandscape(q, d, appList, flowIdx); err != nil {
		return nil, err
	}
	d.Index = buildMapIndex(q, d, refRows, inApp)
	return d, nil
}

func containsTrigger(ts []MapTrigger, t MapTrigger) bool {
	for _, x := range ts {
		if x.Kind == t.Kind && x.Source == t.Source {
			return true
		}
	}
	return false
}

func kindOrder(k string) int {
	for i, e := range EntryKinds {
		if e == k {
			return i
		}
	}
	return len(EntryKinds)
}

func firstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if i := strings.Index(s, ". "); i > 0 && i < 300 {
		return s[:i+1]
	}
	return truncate(s, 300)
}

// collectMapData reads entities, attributes, associations and their writers.
func collectMapData(q Querier, d *MapData, appList string, refRows []map[string]string, flowIdx map[string]int) error {
	ents, err := rows(q, `SELECT QualifiedName, ModuleName, COALESCE(EntityType, '') AS T, COALESCE(Generalization, '') AS G
		FROM entities_data WHERE ModuleName IN (`+appList+`) ORDER BY QualifiedName`)
	if err != nil {
		return err
	}
	idx := map[string]int{}
	for _, e := range ents {
		idx[e["QualifiedName"]] = len(d.Entities)
		d.Entities = append(d.Entities, MapEntity{Name: e["QualifiedName"], Module: e["ModuleName"],
			Persistent: strings.EqualFold(e["T"], "PERSISTENT"), Generalize: e["G"]})
	}
	attrs, err := rows(q, `SELECT EntityQualifiedName, Name, COALESCE(DataType, '') AS T FROM attributes_data
		WHERE ModuleName IN (`+appList+`) ORDER BY EntityQualifiedName, rowid`)
	if err != nil {
		return err
	}
	for _, a := range attrs {
		if i, ok := idx[a["EntityQualifiedName"]]; ok {
			d.Entities[i].Attributes = append(d.Entities[i].Attributes, a["Name"]+": "+a["T"])
		}
	}
	as, err := rows(q, `SELECT QualifiedName, ModuleName, FromEntity, ToEntity, COALESCE(AssociationType, '') AS T FROM associations_data
		WHERE ModuleName IN (`+appList+`) OR substr(ToEntity, 1, instr(ToEntity, '.') - 1) IN (`+appList+`) ORDER BY QualifiedName`)
	if err != nil {
		return err
	}
	for _, a := range as {
		d.Assocs = append(d.Assocs, MapAssoc{Name: a["QualifiedName"], From: a["FromEntity"], To: a["ToEntity"], Type: a["T"], Module: a["ModuleName"]})
	}
	for _, r := range refRows {
		switch r["RefKind"] {
		case "create", "change", "delete", "commit":
		default:
			continue
		}
		if i, ok := idx[r["TargetName"]]; ok {
			if _, isFlow := flowIdx[r["SourceName"]]; isFlow || r["SourceType"] == "MICROFLOW" {
				d.Entities[i].WrittenBy = appendUnique(d.Entities[i].WrittenBy, r["SourceName"])
			}
		}
	}
	for i := range d.Entities {
		sort.Strings(d.Entities[i].WrittenBy)
	}
	return nil
}

// collectMapPages reads pages, their links and the page flows per role.
func collectMapPages(q Querier, d *MapData, appList string, refRows []map[string]string) error {
	ps, err := rows(q, `SELECT p.QualifiedName, p.ModuleName, COALESCE(p.Title, '') AS Title,
			COALESCE(l.LayoutType, '') AS LT FROM pages_data p LEFT JOIN layouts_data l ON l.QualifiedName = p.LayoutRef
		WHERE p.ModuleName IN (`+appList+`) ORDER BY p.QualifiedName`)
	if err != nil {
		return err
	}
	pageIdx := map[string]int{}
	for _, p := range ps {
		pageIdx[p["QualifiedName"]] = len(d.Pages)
		d.Pages = append(d.Pages, MapPage{Name: p["QualifiedName"], Module: p["ModuleName"], Title: p["Title"],
			Popup: strings.Contains(strings.ToLower(p["LT"]), "popup")})
	}

	// Who may open what.
	rm, err := rows(q, `SELECT UserRoleName, ModuleRoleName FROM role_mappings ORDER BY UserRoleName, ModuleRoleName`)
	if err != nil {
		return err
	}
	ur, err := rows(q, `SELECT Name, COALESCE(IsAnonymous, 0) AS A FROM user_roles ORDER BY Name`)
	if err != nil {
		return err
	}
	modRoles := map[string][]string{}
	var roleNames []string
	anon := map[string]bool{}
	for _, r := range ur {
		roleNames = append(roleNames, r["Name"])
		anon[r["Name"]] = r["A"] == "1"
	}
	for _, r := range rm {
		if _, ok := modRoles[r["UserRoleName"]]; !ok && !containsStr(roleNames, r["UserRoleName"]) {
			roleNames = append(roleNames, r["UserRoleName"])
		}
		modRoles[r["UserRoleName"]] = append(modRoles[r["UserRoleName"]], r["ModuleRoleName"])
	}
	perms, err := rows(q, `SELECT ModuleRoleName, ElementName, ElementType FROM permissions
		WHERE (ElementType = 'PAGE' AND AccessType = 'VIEW') OR (ElementType IN ('MICROFLOW', 'NANOFLOW') AND AccessType = 'EXECUTE')`)
	if err != nil {
		return err
	}
	grant := map[string]map[string]bool{} // module role → element
	for _, p := range perms {
		if grant[p["ModuleRoleName"]] == nil {
			grant[p["ModuleRoleName"]] = map[string]bool{}
		}
		grant[p["ModuleRoleName"]][p["ElementName"]] = true
		if p["ElementType"] == "PAGE" {
			if i, ok := pageIdx[p["ElementName"]]; ok {
				d.Pages[i].Roles = appendUnique(d.Pages[i].Roles, p["ModuleRoleName"])
			}
		}
	}

	// The click graph: page → page (show_page), page/snippet/layout → flow
	// (action), flow → flow (call), flow → page (show_page), page → snippet and
	// layout → snippet (snippet_call), page → layout.
	next := map[string][]string{}
	add := func(a, b string) {
		if !containsStr(next[a], b) {
			next[a] = append(next[a], b)
		}
	}
	for _, r := range refRows {
		st, tt, k := r["SourceType"], r["TargetType"], r["RefKind"]
		src, tgt := r["SourceName"], r["TargetName"]
		switch {
		case (st == "PAGE" || st == "SNIPPET" || st == "LAYOUT") && (k == "show_page" || k == "action" || k == "snippet_call" || k == "layout"):
			add(src, tgt)
			if st == "PAGE" && (tt == "PAGE") {
				d.PageEdges = append(d.PageEdges, MapLink{From: src, To: tgt, Kind: "show page"})
			}
			if st == "PAGE" && (tt == "MICROFLOW" || tt == "NANOFLOW") {
				if i, ok := pageIdx[src]; ok {
					d.Pages[i].Flows = appendUnique(d.Pages[i].Flows, tgt)
				}
			}
		case (st == "MICROFLOW" || st == "NANOFLOW") && (k == "call" || k == "show_page"):
			add(src, tgt)
			if k == "show_page" {
				d.PageEdges = append(d.PageEdges, MapLink{From: src, To: tgt, Kind: "flow opens"})
			}
		}
	}
	kindOf := map[string]string{}
	for _, r := range refRows {
		kindOf[r["TargetName"]] = r["TargetType"]
		kindOf[r["SourceName"]] = r["SourceType"]
	}
	homes, err := rows(q, `SELECT UserRole, COALESCE(Page, '') AS Page, COALESCE(Microflow, '') AS Microflow FROM navigation_role_homes`)
	if err != nil {
		return err
	}
	menu, err := rows(q, `SELECT COALESCE(TargetPage, '') AS Page, COALESCE(TargetMicroflow, '') AS Microflow FROM navigation_menu_items`)
	if err != nil {
		return err
	}
	var defaultHomes []string
	for _, r := range refRows {
		if r["RefKind"] == "home_page" || r["RefKind"] == "menu_item" {
			defaultHomes = appendUnique(defaultHomes, r["TargetName"])
		}
	}
	for _, r := range menu {
		for _, t := range []string{r["Page"], r["Microflow"]} {
			if t != "" {
				defaultHomes = appendUnique(defaultHomes, t)
			}
		}
	}
	for _, name := range roleNames {
		role := MapRole{Name: name, ModuleRoles: modRoles[name], Anonymous: anon[name]}
		may := func(el string) bool {
			for _, mr := range modRoles[name] {
				if grant[mr][el] {
					return true
				}
			}
			return false
		}
		allowedPage := func(p string) bool { return may(p) }
		allowedFlow := func(f string) bool { return kindOf[f] == "NANOFLOW" || may(f) }
		var starts []string
		for _, h := range homes {
			if h["UserRole"] == name {
				starts = appendUnique(starts, nonEmpty(h["Page"], h["Microflow"]))
			}
		}
		for _, h := range defaultHomes {
			starts = appendUnique(starts, h)
		}
		reach := map[string]bool{}
		var queue []string
		visit := func(n string) {
			if reach[n] || n == "" {
				return
			}
			switch kindOf[n] {
			case "PAGE":
				if !allowedPage(n) {
					return
				}
			case "MICROFLOW", "NANOFLOW":
				// A flow called from another flow runs with the caller's
				// rights; only the first one needs the grant. Close enough
				// for a guide: require it for flows reached from a page.
			}
			reach[n] = true
			queue = append(queue, n)
		}
		for _, s := range starts {
			if kindOf[s] == "PAGE" && !allowedPage(s) {
				continue
			}
			if (kindOf[s] == "MICROFLOW" || kindOf[s] == "NANOFLOW") && !allowedFlow(s) {
				continue
			}
			role.Starts = append(role.Starts, s)
			visit(s)
		}
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			for _, m := range next[n] {
				if (kindOf[n] == "PAGE" || kindOf[n] == "SNIPPET" || kindOf[n] == "LAYOUT") &&
					(kindOf[m] == "MICROFLOW" || kindOf[m] == "NANOFLOW") && !allowedFlow(m) {
					continue
				}
				visit(m)
			}
		}
		for _, p := range d.Pages {
			if allowedPage(p.Name) {
				role.Allowed++
				if reach[p.Name] {
					role.Reachable = append(role.Reachable, p.Name)
				} else {
					role.Unreached = append(role.Unreached, p.Name)
				}
			}
		}
		d.Roles = append(d.Roles, role)
	}
	return nil
}

// collectMapLandscape reads the systems around the app, APIs, queues, Java
// actions and constants.
func collectMapLandscape(q Querier, d *MapData, appList string, flowIdx map[string]int) error {
	closure := func(root string) int {
		seen := map[string]bool{}
		stack := []string{root}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[n] {
				continue
			}
			seen[n] = true
			if i, ok := flowIdx[n]; ok {
				stack = append(stack, d.Flows[i].Calls...)
			}
		}
		return len(seen)
	}
	ops, err := rows(q, `SELECT ServiceQualifiedName, COALESCE(HttpMethod, '') AS M, COALESCE(Path, '') AS P, COALESCE(Microflow, '') AS F
		FROM published_rest_operations_data WHERE ModuleName IN (`+appList+`) ORDER BY ServiceQualifiedName, Path`)
	if err != nil {
		return err
	}
	svc := map[string]int{}
	for _, o := range ops {
		d.Published = append(d.Published, MapOperation{Service: o["ServiceQualifiedName"], Method: o["M"], Path: o["P"], Microflow: o["F"], Closure: closure(o["F"])})
		svc[o["ServiceQualifiedName"]]++
	}
	for s, n := range svc {
		d.Systems = append(d.Systems, MapSystem{Kind: "published REST", Name: s, Detail: fmt.Sprintf("%d operation(s); its clients are outside the model", n)})
	}
	type src struct{ kind, sql string }
	for _, s := range []src{
		{"published OData", `SELECT QualifiedName AS N, COALESCE(Path, '') AS D FROM odata_services_data WHERE ModuleName IN (` + appList + `)`},
		{"consumed REST", `SELECT QualifiedName AS N, COALESCE(BaseUrl, '') AS D FROM rest_clients_data WHERE ModuleName IN (` + appList + `)`},
		{"consumed OData", `SELECT QualifiedName AS N, COALESCE(MetadataUrl, '') AS D FROM odata_clients_data WHERE ModuleName IN (` + appList + `)`},
		{"business events", `SELECT QualifiedName AS N, COALESCE(ServiceName, '') AS D FROM business_event_services_data WHERE ModuleName IN (` + appList + `)`},
	} {
		rs, err := rows(q, s.sql)
		if err != nil {
			return err
		}
		for _, r := range rs {
			d.Systems = append(d.Systems, MapSystem{Kind: s.kind, Name: r["N"], Detail: r["D"]})
			if s.kind == "consumed REST" || s.kind == "consumed OData" {
				d.Consumed = append(d.Consumed, MapDoc{Name: r["N"], Module: moduleOf(r["N"]), Note: s.kind + " " + r["D"]})
			}
		}
	}
	im, err := rows(q, `SELECT QualifiedName, ModuleName FROM import_mappings_data WHERE ModuleName IN (`+appList+`) ORDER BY QualifiedName`)
	if err != nil {
		return err
	}
	for _, r := range im {
		d.Consumed = append(d.Consumed, MapDoc{Name: r["QualifiedName"], Module: r["ModuleName"], Note: "import mapping"})
	}
	// Identity: the modules that sign users in, from their names; Marketplace
	// modules count here, since that is where an identity provider lives.
	idp := regexp.MustCompile(`(?i)^(oidc|saml|sso|ldap|azuread|entra|keycloak|okta|auth0)`)
	for _, m := range append(append([]string{}, d.Scope.App...), d.Scope.Outside...) {
		if idp.MatchString(m) {
			d.Systems = append(d.Systems, MapSystem{Kind: "identity", Name: m, Detail: "signs users in"})
		}
	}
	for _, m := range d.Modules {
		if m.Integration {
			d.Systems = append(d.Systems, MapSystem{Kind: "integration module", Name: m.Name})
		}
	}
	sort.SliceStable(d.Systems, func(i, j int) bool {
		if d.Systems[i].Kind != d.Systems[j].Kind {
			return d.Systems[i].Kind < d.Systems[j].Kind
		}
		return d.Systems[i].Name < d.Systems[j].Name
	})
	http, err := rows(q, `SELECT DISTINCT MicroflowQualifiedName AS F, COALESCE(NULLIF(ServiceRef, ''), ActionType) AS T FROM activities_data
		WHERE ModuleName IN (`+appList+`) AND (ActionType LIKE '%Rest%' OR ActionType LIKE '%WebService%' OR ActionType LIKE '%Http%')
		ORDER BY 1`)
	if err != nil {
		return err
	}
	for _, r := range http {
		d.HTTPCalls = append(d.HTTPCalls, MapLink{From: r["F"], To: r["T"], Kind: "HTTP call"})
	}
	qs, err := rows(q, `SELECT QualifiedName FROM queues_data WHERE ModuleName IN (`+appList+`) ORDER BY QualifiedName`)
	if err != nil {
		return err
	}
	qi := map[string]int{}
	for _, r := range qs {
		qi[r["QualifiedName"]] = len(d.Queues)
		d.Queues = append(d.Queues, MapQueue{Name: r["QualifiedName"]})
	}
	calls, err := rows(q, `SELECT DISTINCT QueueRef, ActionRef, MicroflowQualifiedName FROM activities_data WHERE COALESCE(QueueRef, '') != ''`)
	if err != nil {
		return err
	}
	for _, r := range calls {
		i, ok := qi[r["QueueRef"]]
		if !ok {
			i = len(d.Queues)
			qi[r["QueueRef"]] = i
			d.Queues = append(d.Queues, MapQueue{Name: r["QueueRef"]})
		}
		d.Queues[i].Runs = appendUnique(d.Queues[i].Runs, r["ActionRef"])
		d.Queues[i].Callers = appendUnique(d.Queues[i].Callers, r["MicroflowQualifiedName"])
	}
	ja, err := rows(q, `SELECT QualifiedName, ModuleName, COALESCE(Documentation, '') AS D FROM java_actions_data WHERE ModuleName IN (`+appList+`) ORDER BY QualifiedName`)
	if err != nil {
		return err
	}
	for _, r := range ja {
		d.JavaActions = append(d.JavaActions, MapDoc{Name: r["QualifiedName"], Module: r["ModuleName"], Note: firstSentence(r["D"])})
	}
	cs, err := rows(q, `SELECT QualifiedName, ModuleName, COALESCE(DataType, '') AS T, COALESCE(DefaultValue, '') AS V FROM constants_data
		WHERE ModuleName IN (`+appList+`) ORDER BY QualifiedName`)
	if err != nil {
		return err
	}
	for _, r := range cs {
		c := MapConstant{Name: r["QualifiedName"], Module: r["ModuleName"], Type: r["T"]}
		v := strings.TrimSpace(r["V"])
		if _, ok := flowIdx[v]; ok {
			c.NamesFlow = v
		}
		// Defaults are shown only when short and not secret-like: the map is
		// for reading, and a key in it would be one more copy of the key.
		if len(v) <= 60 && !LooksLikeKeyMaterial(v) && !secretName.MatchString(shortName(c.Name)) {
			c.Default = v
		}
		d.Constants = append(d.Constants, c)
	}
	return nil
}

// buildMapIndex lists every app document with what it uses and what uses it.
// Attribute and enumeration-value references fold into their entity or
// enumeration, so an entity's "used by" is everything that touches it.
func buildMapIndex(q Querier, d *MapData, refRows []map[string]string, inApp func(string) bool) []MapIndexDoc {
	docs, err := rows(q, `SELECT QualifiedName, ObjectType, ModuleName FROM objects
		WHERE ObjectType NOT IN ('MODULE', 'ATTRIBUTE', 'ENUMERATION_VALUE') ORDER BY QualifiedName`)
	if err != nil {
		return nil
	}
	idx := map[string]int{}
	var out []MapIndexDoc
	for _, r := range docs {
		if !inApp(r["QualifiedName"]) {
			continue
		}
		idx[r["QualifiedName"]] = len(out)
		out = append(out, MapIndexDoc{Name: r["QualifiedName"], Type: r["ObjectType"], Module: r["ModuleName"]})
	}
	fold := func(name, typ string) string {
		if typ == "ATTRIBUTE" || typ == "ENUMERATION_VALUE" {
			if i := strings.LastIndexByte(name, '.'); i > 0 {
				return name[:i]
			}
		}
		return name
	}
	for _, r := range refRows {
		if r["TargetType"] == "WIDGET" {
			continue
		}
		src, tgt := fold(r["SourceName"], r["SourceType"]), fold(r["TargetName"], r["TargetType"])
		if src == tgt {
			continue
		}
		if i, ok := idx[src]; ok {
			out[i].Uses = appendUnique(out[i].Uses, tgt)
		}
		if i, ok := idx[tgt]; ok {
			out[i].UsedBy = appendUnique(out[i].UsedBy, src)
		}
	}
	for i := range out {
		sort.Strings(out[i].Uses)
		sort.Strings(out[i].UsedBy)
	}
	return out
}
