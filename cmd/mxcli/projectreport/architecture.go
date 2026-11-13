// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// NavigationKinds are the reference kinds that wire screens together rather
// than express a domain dependency. Counted with everything else they put the
// whole app in one cycle: the shell's menu links every screen and every screen
// uses the shell's layout — measured on a 14-module app, where leaving these
// out left two real cycles, each with a concrete cause (ako/mxcli#2222).
var NavigationKinds = []string{"layout", "show_page", "menu_item", "home_page", "widget"}

// ArchitectureOptions configures the architecture collector.
type ArchitectureOptions struct {
	Scope ScopeOptions
	Top   int
	// SettingsMicroflows are the after-startup, before-shutdown and health-check
	// microflows. Project settings are not a source in the refs table, so
	// without these the after-startup microflow reads as unused.
	SettingsMicroflows []string
}

// ArchitectureData is the collected architecture report.
type ArchitectureData struct {
	Scope           Scope          `json:"scope"`
	NavigationKinds []string       `json:"navigationKinds"`
	Modules         []ModuleHealth `json:"modules"`
	DomainEdges     []ModuleEdge   `json:"domainEdges"`
	NavigationEdges []ModuleEdge   `json:"navigationEdges"`
	OutsideEdges    []ModuleEdge   `json:"outsideEdges"`
	Cycles          []Cycle        `json:"cycles"`
	// ModulesInCyclesWithNavigation is how many app modules sit in a cycle
	// when navigation kinds are counted too — the number that makes the
	// domain/navigation split necessary, so the page shows it.
	ModulesInCyclesWithNavigation int         `json:"modulesInCyclesWithNavigation"`
	CommunitiesComputed           bool        `json:"communitiesComputed"`
	Communities                   []Community `json:"communities"`
	SmallCommunities              int         `json:"smallCommunities"`
	GodNodes                      []Asset     `json:"godNodes"`
	Bridges                       []Asset     `json:"bridges"`
	Hotspots                      []Hotspot   `json:"hotspots"`
	Unused                        []UnusedDoc `json:"unused"`
}

// ModuleHealth is one app module's size and coupling.
type ModuleHealth struct {
	Name        string `json:"name"`
	Layer       int    `json:"layer"`
	Documents   int    `json:"documents"`
	Internal    int    `json:"internal"`
	ExternalOut int    `json:"externalOut"`
	ExternalIn  int    `json:"externalIn"`
	// Cohesion is a percentage, or -1 when the module makes no references.
	Cohesion float64 `json:"cohesion"`
	FanIn    int     `json:"fanIn"`
	FanOut   int     `json:"fanOut"`
	NavShare float64 `json:"navShare"`
	InCycle  bool    `json:"inCycle"`
	// Role names why a low-cohesion module is low by design (a shell, an
	// admin UI, an adapter), so the page explains it instead of flagging it.
	Role       string `json:"role,omitempty"`
	RoleReason string `json:"roleReason,omitempty"`
}

// ModuleEdge is the references from one module to another.
type ModuleEdge struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Refs    int      `json:"refs"`
	Kinds   []string `json:"kinds"`
	Samples []string `json:"samples,omitempty"`
	InCycle bool     `json:"inCycle,omitempty"`
}

// Cycle is a set of modules that depend on each other through domain edges.
type Cycle struct {
	Modules []string     `json:"modules"`
	Edges   []ModuleEdge `json:"edges"`
	// Lightest is the edges carrying the fewest references — usually the
	// accidental direction, and the first place to look for the cut.
	Lightest []string `json:"lightest"`
}

// Community is one Leiden cluster restricted to the app's modules.
type Community struct {
	Key       string         `json:"key"`
	Label     string         `json:"label"`
	Ambiguous bool           `json:"ambiguous,omitempty"`
	Size      int            `json:"size"`
	KeyMember string         `json:"keyMember"`
	Modules   []ModuleCount  `json:"modules"`
	Members   []string       `json:"members"`
	byModule  map[string]int // not serialized; rebuilt on render
}

// ModuleCount is a module's share of a community.
type ModuleCount struct {
	Module string `json:"module"`
	Count  int    `json:"count"`
}

// Asset is a god node or bridge.
type Asset struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Module      string  `json:"module"`
	In          int     `json:"in"`
	Out         int     `json:"out"`
	Degree      int     `json:"degree"`
	Betweenness float64 `json:"betweenness"`
}

// Hotspot is an entity used by many flows.
type Hotspot struct {
	Entity  string `json:"entity"`
	Flows   int    `json:"flows"`
	Modules string `json:"modules"`
}

// UnusedDoc is a document with no inbound reference, with a verdict on
// whether that is believable.
type UnusedDoc struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Module  string `json:"module"`
	Verdict string `json:"verdict"` // unused | verify | used
	Reason  string `json:"reason,omitempty"`
}

type refRow struct {
	src, srcType, tgt, tgtType, kind string
}

// CollectArchitecture measures module structure from the catalog. It needs a
// FULL refresh (refs) and, for the bounded-context section, the COMMUNITIES
// pass.
func CollectArchitecture(q Querier, opt ArchitectureOptions) (*ArchitectureData, error) {
	if opt.Top <= 0 {
		opt.Top = 15
	}
	scope, err := LoadScope(q, opt.Scope)
	if err != nil {
		return nil, err
	}
	d := &ArchitectureData{Scope: scope, NavigationKinds: NavigationKinds}
	app := setOf(scope.App)
	known := setOf(append(append([]string{}, scope.App...), scope.Outside...))
	nav := setOf(NavigationKinds)

	rs, err := rows(q, `SELECT SourceName, SourceType, TargetName, TargetType, RefKind FROM refs
		WHERE instr(SourceName, '.') > 0 AND instr(TargetName, '.') > 0`)
	if err != nil {
		return nil, err
	}
	refs := make([]refRow, 0, len(rs))
	for _, r := range rs {
		refs = append(refs, refRow{r["SourceName"], r["SourceType"], r["TargetName"], r["TargetType"], r["RefKind"]})
	}

	health := map[string]*ModuleHealth{}
	for _, m := range scope.App {
		health[m] = &ModuleHealth{Name: m}
	}
	type key struct{ from, to string }
	type agg struct {
		refs    int
		kinds   map[string]bool
		samples []string
	}
	domain, navig, outside := map[key]*agg{}, map[key]*agg{}, map[key]*agg{}
	bump := func(m map[key]*agg, k key, r refRow) {
		a := m[k]
		if a == nil {
			a = &agg{kinds: map[string]bool{}}
			m[k] = a
		}
		a.refs++
		a.kinds[r.kind] = true
		s := fmt.Sprintf("%s → %s (%s)", r.src, r.tgt, r.kind)
		if len(a.samples) < 3 && !containsStr(a.samples, s) {
			a.samples = append(a.samples, s)
		}
	}
	navOut := map[string]int{}
	for _, r := range refs {
		sm, tm := moduleOf(r.src), moduleOf(r.tgt)
		sIn, tIn := app[sm], app[tm]
		// A navigation document's name ("Navigation.Responsive") has a
		// prefix that is no module; it is not a dependency between modules.
		if (!sIn && !tIn) || !known[sm] || !known[tm] {
			continue
		}
		if sm == tm {
			health[sm].Internal++
			continue
		}
		if sIn {
			health[sm].ExternalOut++
			if nav[r.kind] {
				navOut[sm]++
			}
		}
		if tIn {
			health[tm].ExternalIn++
		}
		switch {
		case sIn && tIn && nav[r.kind]:
			bump(navig, key{sm, tm}, r)
		case sIn && tIn:
			bump(domain, key{sm, tm}, r)
		default:
			bump(outside, key{sm, tm}, r)
		}
	}
	toEdges := func(m map[key]*agg) []ModuleEdge {
		var out []ModuleEdge
		for k, a := range m {
			e := ModuleEdge{From: k.from, To: k.to, Refs: a.refs, Samples: a.samples}
			for kind := range a.kinds {
				e.Kinds = append(e.Kinds, kind)
			}
			sort.Strings(e.Kinds)
			out = append(out, e)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Refs != out[j].Refs {
				return out[i].Refs > out[j].Refs
			}
			return out[i].From+out[i].To < out[j].From+out[j].To
		})
		return out
	}
	d.DomainEdges, d.NavigationEdges, d.OutsideEdges = toEdges(domain), toEdges(navig), toEdges(outside)

	// Domain graph: layers and cycles.
	g := newDigraph(scope.App)
	for _, e := range d.DomainEdges {
		g.add(e.From, e.To, e.Refs)
	}
	layer := g.layers()
	cycleOf := map[string]int{}
	for _, comp := range g.sccs() {
		if len(comp) < 2 {
			continue
		}
		c := Cycle{Modules: comp}
		in := setOf(comp)
		for i := range d.DomainEdges {
			e := &d.DomainEdges[i]
			if in[e.From] && in[e.To] {
				e.InCycle = true
				c.Edges = append(c.Edges, *e)
			}
		}
		minRefs := math.MaxInt
		for _, e := range c.Edges {
			minRefs = min(minRefs, e.Refs)
		}
		for _, e := range c.Edges {
			if e.Refs == minRefs {
				c.Lightest = append(c.Lightest, e.From+" → "+e.To)
			}
		}
		for _, m := range comp {
			cycleOf[m] = len(d.Cycles)
		}
		d.Cycles = append(d.Cycles, c)
	}
	// The same question with navigation counted, for the page's comparison.
	gAll := newDigraph(scope.App)
	for _, e := range d.DomainEdges {
		gAll.add(e.From, e.To, e.Refs)
	}
	for _, e := range d.NavigationEdges {
		gAll.add(e.From, e.To, e.Refs)
	}
	for _, comp := range gAll.sccs() {
		if len(comp) > 1 {
			d.ModulesInCyclesWithNavigation += len(comp)
		}
	}

	// Documents per module, and the object kinds that decide a module's role.
	docs := map[string]map[string]int{}
	ors, err := rows(q, `SELECT ModuleName, ObjectType, COUNT(*) AS N FROM objects
		WHERE ModuleName != '' AND ObjectType NOT IN ('MODULE', 'ATTRIBUTE', 'ASSOCIATION', 'ENUMERATION_VALUE')
		GROUP BY ModuleName, ObjectType`)
	if err != nil {
		return nil, err
	}
	for _, r := range ors {
		if docs[r["ModuleName"]] == nil {
			docs[r["ModuleName"]] = map[string]int{}
		}
		docs[r["ModuleName"]][r["ObjectType"]] = atoi(r["N"])
	}
	fanIn, fanOut := map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, e := range d.DomainEdges {
		if fanOut[e.From] == nil {
			fanOut[e.From] = map[string]bool{}
		}
		if fanIn[e.To] == nil {
			fanIn[e.To] = map[string]bool{}
		}
		fanOut[e.From][e.To] = true
		fanIn[e.To][e.From] = true
	}
	for _, m := range scope.App {
		h := health[m]
		h.Layer = layer[m]
		for _, n := range docs[m] {
			h.Documents += n
		}
		// -1: no references at all, so cohesion is undefined rather than 0.
		h.Cohesion = -1
		if tot := h.Internal + h.ExternalOut; tot > 0 {
			h.Cohesion = round1(100 * float64(h.Internal) / float64(tot))
		}
		if h.ExternalOut > 0 {
			h.NavShare = round1(100 * float64(navOut[m]) / float64(h.ExternalOut))
		}
		h.FanIn, h.FanOut = len(fanIn[m]), len(fanOut[m])
		_, h.InCycle = cycleOf[m]
		h.Role, h.RoleReason = moduleRole(m, h, docs[m])
		d.Modules = append(d.Modules, *h)
	}
	sort.SliceStable(d.Modules, func(i, j int) bool {
		if d.Modules[i].Layer != d.Modules[j].Layer {
			return d.Modules[i].Layer > d.Modules[j].Layer
		}
		return d.Modules[i].Name < d.Modules[j].Name
	})

	if err := collectCommunities(q, d, app); err != nil {
		return nil, err
	}
	appList := sqlList(scope.App)
	top := fmt.Sprint(opt.Top)
	gn, err := rows(q, `SELECT Asset, COALESCE(ObjectType, '') AS ObjectType, ModuleName, InDegree, OutDegree, Degree,
		COALESCE(Betweenness, 0) AS Betweenness FROM graph_god_nodes WHERE ModuleName IN (`+appList+`)
		ORDER BY Degree DESC, Asset LIMIT `+top)
	if err != nil {
		return nil, err
	}
	d.GodNodes = toAssets(gn)
	br, err := rows(q, `SELECT Asset, COALESCE(ObjectType, '') AS ObjectType, ModuleName, InDegree, OutDegree, Degree,
		COALESCE(Betweenness, 0) AS Betweenness FROM graph_god_nodes WHERE ModuleName IN (`+appList+`)
		AND COALESCE(Betweenness, 0) > 0 ORDER BY Betweenness DESC, Asset LIMIT `+top)
	if err != nil {
		return nil, err
	}
	d.Bridges = toAssets(br)
	hs, err := rows(q, `SELECT Entity, UsedByFlows, COALESCE(AcrossModules, '') AS AcrossModules FROM graph_entity_hotspots
		WHERE substr(Entity, 1, instr(Entity, '.') - 1) IN (`+appList+`) ORDER BY UsedByFlows DESC, Entity LIMIT `+top)
	if err != nil {
		return nil, err
	}
	for _, r := range hs {
		d.Hotspots = append(d.Hotspots, Hotspot{Entity: r["Entity"], Flows: atoi(r["UsedByFlows"]), Modules: r["AcrossModules"]})
	}
	if d.Unused, err = collectUnused(q, appList, opt.SettingsMicroflows); err != nil {
		return nil, err
	}
	return d, nil
}

func toAssets(rs []map[string]string) []Asset {
	var out []Asset
	for _, r := range rs {
		out = append(out, Asset{Name: r["Asset"], Type: r["ObjectType"], Module: r["ModuleName"],
			In: atoi(r["InDegree"]), Out: atoi(r["OutDegree"]), Degree: atoi(r["Degree"]), Betweenness: round1(atof(r["Betweenness"]))})
	}
	return out
}

var adminName = regexp.MustCompile(`(?i)admin|beheer|management|backoffice`)

// moduleRole explains a module whose low cohesion is its job. It only names a
// role when the evidence is in the model; anything else is left for the reader
// (or an annotation) to judge.
func moduleRole(name string, h *ModuleHealth, docs map[string]int) (string, string) {
	if h.Cohesion >= 50 || h.Cohesion < 0 {
		return "", ""
	}
	integration := docs["CONSUMED_REST_SERVICE"] + docs["PUBLISHED_REST_SERVICE"] + docs["IMPORT_MAPPING"] +
		docs["EXPORT_MAPPING"] + docs["JSON_STRUCTURE"] + docs["CONSUMED_ODATA_SERVICE"] + docs["PUBLISHED_ODATA_SERVICE"] +
		docs["MESSAGE_DEFINITION_COLLECTION"] + docs["BUSINESS_EVENT_SERVICE"]
	switch {
	case h.NavShare >= 50 || (docs["LAYOUT"] > 0 && h.ExternalIn > h.ExternalOut):
		return "shell", fmt.Sprintf("%.0f%% of its outbound references are navigation, or it owns the layouts other modules use", h.NavShare)
	case integration > 0 && integration*3 >= h.Documents:
		return "adapter", fmt.Sprintf("%d of its %d documents are services, mappings or message definitions", integration, h.Documents)
	case adminName.MatchString(name):
		return "admin UI", "named as an administration module; screens over other modules' data"
	}
	return "", ""
}

func collectCommunities(q Querier, d *ArchitectureData, app map[string]bool) error {
	rs, err := rows(q, `SELECT c.AssetName, c.ModuleName, c.CommunityId, COALESCE(gc.PageRank, 0) AS PageRank
		FROM communities c LEFT JOIN graph_centrality_data gc ON gc.AssetName = c.AssetName
		ORDER BY c.CommunityId, PageRank DESC, c.AssetName`)
	if err != nil {
		return err
	}
	d.CommunitiesComputed = len(rs) > 0
	byID := map[string]*Community{}
	var order []string
	for _, r := range rs {
		if !app[r["ModuleName"]] {
			continue
		}
		id := r["CommunityId"]
		c := byID[id]
		if c == nil {
			c = &Community{byModule: map[string]int{}, KeyMember: r["AssetName"]}
			byID[id] = c
			order = append(order, id)
		}
		c.Size++
		c.byModule[r["ModuleName"]]++
		if len(c.Members) < 12 {
			c.Members = append(c.Members, r["AssetName"])
		}
	}
	labels := map[string]int{}
	for _, id := range order {
		c := byID[id]
		if c.Size < 3 {
			d.SmallCommunities++
			continue
		}
		for m, n := range c.byModule {
			c.Modules = append(c.Modules, ModuleCount{m, n})
		}
		sort.Slice(c.Modules, func(i, j int) bool {
			if c.Modules[i].Count != c.Modules[j].Count {
				return c.Modules[i].Count > c.Modules[j].Count
			}
			return c.Modules[i].Module < c.Modules[j].Module
		})
		c.Key = "cluster:" + c.KeyMember
		c.Label = c.Modules[0].Module
		labels[c.Label]++
		d.Communities = append(d.Communities, *c)
	}
	// The catalog's label is the largest member module, so three clusters of
	// one big module were all called by its name. Disambiguate by the most
	// central member and say a person should name them.
	for i := range d.Communities {
		c := &d.Communities[i]
		if labels[c.Label] > 1 {
			c.Ambiguous = true
			c.Label += " · " + shortName(c.KeyMember)
		}
	}
	sort.SliceStable(d.Communities, func(i, j int) bool { return d.Communities[i].Size > d.Communities[j].Size })
	return nil
}

func shortName(qn string) string {
	if i := strings.LastIndexByte(qn, '.'); i >= 0 {
		return qn[i+1:]
	}
	return qn
}

// collectUnused lists documents nothing references, and gives each one a
// second look: two of three "dead" entries in the prototype were in use, one
// placed in a layout and one named in a constant's value (ako/mxcli#2222).
func collectUnused(q Querier, appList string, settingsFlows []string) ([]UnusedDoc, error) {
	dead, err := rows(q, `SELECT QualifiedName, ObjectType, ModuleName FROM graph_dead_assets
		WHERE ModuleName IN (`+appList+`) ORDER BY ModuleName, ObjectType, QualifiedName`)
	if err != nil || len(dead) == 0 {
		return nil, err
	}
	layoutRefs, err := rows(q, `SELECT COUNT(*) AS N FROM refs WHERE SourceType = 'LAYOUT'`)
	if err != nil {
		return nil, err
	}
	noLayoutRefs := len(layoutRefs) == 0 || atoi(layoutRefs[0]["N"]) == 0
	consts, err := rows(q, `SELECT QualifiedName AS C, COALESCE(DefaultValue, '') AS V FROM constants
		UNION ALL SELECT ConstantName AS C, COALESCE(Value, '') AS V FROM constant_values`)
	if err != nil {
		return nil, err
	}
	homes, _ := rows(q, `SELECT COALESCE(Page, '') AS Page, COALESCE(Microflow, '') AS Microflow FROM navigation_role_homes`)
	isHome := map[string]bool{}
	for _, h := range homes {
		isHome[h["Page"]], isHome[h["Microflow"]] = true, true
	}
	settings := setOf(settingsFlows)
	var out []UnusedDoc
	for _, r := range dead {
		u := UnusedDoc{Name: r["QualifiedName"], Type: r["ObjectType"], Module: r["ModuleName"], Verdict: "unused"}
		switch {
		case settings[u.Name]:
			u.Verdict, u.Reason = "used", "set in project settings (after startup, before shutdown or health check), which the reference graph does not record"
		case isHome[u.Name]:
			u.Verdict, u.Reason = "used", "a user role's home page in navigation"
		default:
			for _, c := range consts {
				if c["V"] != "" && strings.Contains(c["V"], u.Name) {
					u.Verdict, u.Reason = "verify", "named in the value of constant "+c["C"]+"; no graph follows a name in a string"
					break
				}
			}
			if u.Verdict == "unused" && u.Type == "SNIPPET" && noLayoutRefs {
				u.Verdict, u.Reason = "verify", "the catalog records no references from layouts, so a snippet placed in a layout region looks unused"
			}
			if u.Verdict == "unused" && (u.Type == "MICROFLOW" || u.Type == "NANOFLOW") {
				phrase := strings.NewReplacer(`"`, ``, `'`, ``).Replace(u.Name)
				txt, err := rows(q, `SELECT QualifiedName FROM strings WHERE strings MATCH '"`+phrase+`"'
					AND QualifiedName != '`+phrase+`' LIMIT 1`)
				if err == nil && len(txt) > 0 {
					u.Verdict, u.Reason = "verify", "its name appears in text in "+nonEmpty(txt[0]["QualifiedName"], "a system text")+" — possibly called by name"
				}
			}
		}
		out = append(out, u)
	}
	return out, nil
}

func nonEmpty(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// ---------------------------------------------------------------- render

// ArchitectureMethod and ArchitectureLimits are stated on every page.
var ArchitectureMethod = []string{
	"Every number is read from the mxcli catalog (`refresh catalog full`, plus the `communities` pass for clusters, layers and centrality). Each section is a query over `CATALOG.REFS` or a `CATALOG.GRAPH_*` view and can be re-run by hand.",
	"The app is the project's own modules: Marketplace modules, `System` and the test runner's `MxTest` are outside unless `--include-marketplace` or `--modules` says otherwise. Their edges from app modules are listed under \"Outside the app\".",
	"Domain dependencies are every reference kind except navigation (`layout`, `show_page`, `menu_item`, `home_page`, `widget`). The map, layers, matrix and cycles use domain edges only; navigation is reported separately.",
	"A module's layer is the longest path from it down the condensed domain graph to a module with no dependencies; modules in a cycle share a layer.",
	"Cohesion is internal references divided by internal plus outbound references, over every kind.",
	"Clusters are Leiden communities over the structural kinds (`CATALOG.GRAPH_ANALYSIS_SCOPE`), restricted to app modules; clusters smaller than three documents are counted, not shown.",
}

var ArchitectureLimits = []string{
	"References made from Java actions are not in the graph (ako/mxcli#1819): a microflow called only from Java reads as unused, and a dependency through Java is invisible.",
	"A name in a string — a constant's value, a `call microflow` by name, a URL — is not a reference. Such documents are marked \"verify\", never \"used\".",
	"Layers and cycles are about module dependencies, not runtime behaviour: a cycle closed by one reference is as much a cycle as one closed by a hundred.",
	"Cluster labels are derived (dominant module, then most central member); they are not names. Name them with `--annotations`.",
}

// RenderArchitecture builds the page from collected data.
func RenderArchitecture(env *Envelope, ann *Annotations) (Doc, error) {
	var d ArchitectureData
	if err := env.Decode(&d); err != nil {
		return Doc{}, err
	}
	modsInCycles := 0
	for _, c := range d.Cycles {
		modsInCycles += len(c.Modules)
	}
	maxLayer := 0
	for _, m := range d.Modules {
		maxLayer = max(maxLayer, m.Layer)
	}
	verify := 0
	for _, u := range d.Unused {
		if u.Verdict == "verify" {
			verify++
		}
	}
	doc := Doc{Title: "Architecture", Subtitle: "Module layers, coupling, bounded contexts and cycles"}
	doc.Stats = []Stat{
		{Label: "app modules", Value: fmt.Sprint(len(d.Modules)), Note: fmt.Sprintf("%d outside", len(d.Scope.Outside))},
		{Label: "layers", Value: fmt.Sprint(maxLayer + 1)},
		{Label: "domain cycles", Value: fmt.Sprint(len(d.Cycles)), Note: fmt.Sprintf("%d modules; %d with navigation counted", modsInCycles, d.ModulesInCyclesWithNavigation)},
		{Label: "clusters", Value: fmt.Sprint(len(d.Communities))},
		{Label: "unused documents", Value: fmt.Sprint(len(d.Unused)), Note: fmt.Sprintf("%d to verify", verify)},
	}

	// 1. Map.
	health := map[string]ModuleHealth{}
	for _, m := range d.Modules {
		health[m.Name] = m
	}
	var layersMD []string
	for l := maxLayer; l >= 0; l-- {
		var names []string
		for _, m := range d.Modules {
			if m.Layer == l {
				names = append(names, m.Name)
			}
		}
		if len(names) > 0 {
			layersMD = append(layersMD, fmt.Sprintf("Layer %d: %s", l, strings.Join(names, ", ")))
		}
	}
	doc.Sections = append(doc.Sections, Section{ID: "map", Title: "Layered module map",
		Intro: "Each app module on the layer its domain dependencies put it on: arrows point from a module to what it uses, thicker for more references. Red dashed arrows close a cycle. Fill shows cohesion.",
		Blocks: []Block{
			Figure{SVG: layeredMapSVG(d.Modules, d.DomainEdges), Caption: "Domain dependencies between app modules, highest layer at the top.",
				Fallback: strings.Join(layersMD, "  \n")},
		}})

	// 2. Health.
	ht := Table{Columns: []string{"Module", "Layer", "Documents", "Internal refs", "Outbound", "Inbound", "Cohesion", "Fan-in", "Fan-out", "Role / note"}}
	for _, m := range d.Modules {
		note := m.Role
		if m.RoleReason != "" {
			note += ": " + m.RoleReason
		}
		if n, ok := ann.Get("module:" + m.Name); ok {
			note = joinNote(n.Name, n.Note, note)
		}
		cls := "num"
		switch {
		case m.Cohesion < 0:
			cls = "muted"
		case m.Cohesion < 40 && m.Role == "" && note == "":
			cls = "warn"
		case m.Cohesion < 40:
			cls = "info"
		}
		coh := fmt.Sprintf("%.0f%%", m.Cohesion)
		if m.Cohesion < 0 {
			coh = "n/a"
		}
		if m.InCycle {
			note = strings.TrimSpace(note + " (in a cycle)")
		}
		ht.Rows = append(ht.Rows, []string{m.Name, fmt.Sprint(m.Layer), fmt.Sprint(m.Documents), fmt.Sprint(m.Internal),
			fmt.Sprint(m.ExternalOut), fmt.Sprint(m.ExternalIn), coh, fmt.Sprint(m.FanIn), fmt.Sprint(m.FanOut), note})
		ht.Classes = append(ht.Classes, []string{"", "num", "num", "num", "num", "num", cls, "num", "num", ""})
	}
	doc.Sections = append(doc.Sections, Section{ID: "health", Title: "Module health",
		Intro:  "Low cohesion is flagged only where nothing explains it. A shell, an admin UI or an adapter is low-cohesion by design; annotate `module:<Name>` to explain another.",
		Blocks: []Block{ht}})

	// 3. Matrix.
	doc.Sections = append(doc.Sections, Section{ID: "matrix", Title: "Coupling matrix (domain)",
		Intro:  "Rows use columns. Modules are in layer order, highest first, so a layered design is a triangle above the diagonal; a filled cell below it is a dependency upward — part of a cycle.",
		Blocks: []Block{couplingMatrix(d.Modules, d.DomainEdges)}})

	// 4. Navigation.
	nt := Table{Columns: []string{"From", "To", "References", "Kinds"}}
	for _, e := range d.NavigationEdges {
		nt.Rows = append(nt.Rows, []string{e.From, e.To, fmt.Sprint(e.Refs), strings.Join(e.Kinds, ", ")})
	}
	doc.Sections = append(doc.Sections, Section{ID: "navigation", Title: "Navigation dependencies",
		Intro:  fmt.Sprintf("Menus, layouts and page links between app modules. Counted as dependencies they put %d modules in cycles; that is how screens are wired, not how the domain is layered.", d.ModulesInCyclesWithNavigation),
		Blocks: []Block{nt}})

	// 5. Cycles.
	var cyc []Block
	if len(d.Cycles) == 0 {
		cyc = append(cyc, Para{Text: "No domain cycles between app modules."})
	}
	for _, c := range d.Cycles {
		key := "cycle:" + strings.Join(c.Modules, "+")
		var items []string
		for _, e := range c.Edges {
			items = append(items, fmt.Sprintf("`%s → %s`: %d reference(s) (%s), e.g. %s", e.From, e.To, e.Refs, strings.Join(e.Kinds, ", "), strings.Join(codeList(e.Samples), "; ")))
		}
		cyc = append(cyc, Para{Text: fmt.Sprintf("**%s** — key `%s`", strings.Join(c.Modules, " ↔ "), key)}, Bullets{Items: items})
		text := "Lightest edge(s): " + strings.Join(c.Lightest, ", ") + " — the usual place to cut."
		if n, ok := ann.Get(key); ok {
			text = joinNote(n.Status, n.Note, text)
		}
		cyc = append(cyc, Callout{Kind: "info", Text: text})
	}
	doc.Sections = append(doc.Sections, Section{ID: "cycles", Title: "Cycles", Intro: "Each cycle with the exact edges that close it. A cycle is real when both directions carry domain meaning; it is accidental when one direction is a helper that belongs on the other side.", Blocks: cyc})

	// 6. Bounded contexts.
	doc.Sections = append(doc.Sections, contextsSection(d, ann))

	// 7. God nodes, bridges, hotspots.
	at := func(as []Asset) Table {
		t := Table{Columns: []string{"Element", "Type", "Module", "In", "Out", "Betweenness"}}
		for _, a := range as {
			t.Rows = append(t.Rows, []string{"`" + a.Name + "`", a.Type, a.Module, fmt.Sprint(a.In), fmt.Sprint(a.Out), fmt.Sprintf("%.1f", a.Betweenness)})
		}
		return t
	}
	hst := Table{Columns: []string{"Entity", "Used by flows", "From modules"}}
	for _, h := range d.Hotspots {
		hst.Rows = append(hst.Rows, []string{"`" + h.Entity + "`", fmt.Sprint(h.Flows), h.Modules})
	}
	doc.Sections = append(doc.Sections, Section{ID: "hubs", Title: "God nodes, bridges and entity hotspots",
		Intro:  "God nodes are what the most references hang on; bridges sit on the most shortest paths between other elements (betweenness) and are where a split would cut; hotspots are entities used by the most flows.",
		Blocks: []Block{captioned(at(d.GodNodes), "God nodes"), captioned(at(d.Bridges), "Bridges"), captioned(hst, "Entity hotspots")}})

	// 8. Outside.
	ot := Table{Columns: []string{"From", "To", "References", "Kinds"}}
	for _, e := range d.OutsideEdges {
		ot.Rows = append(ot.Rows, []string{e.From, e.To, fmt.Sprint(e.Refs), strings.Join(e.Kinds, ", ")})
	}
	doc.Sections = append(doc.Sections, Section{ID: "outside", Title: "Outside the app",
		Intro: "References between app modules and Marketplace or platform modules. These are dependencies to keep upgradeable, not part of the app's layering.", Blocks: []Block{ot}})

	// 9. Unused.
	ut := Table{Columns: []string{"Document", "Type", "Verdict", "Why"}}
	for _, u := range d.Unused {
		verdict, why := u.Verdict, u.Reason
		if n, ok := ann.Get("unused:" + u.Name); ok {
			verdict, why = nonEmpty(n.Status, verdict), joinNote("", n.Note, why)
		}
		cls := map[string]string{"unused": "warn", "verify": "info", "used": "ok"}[verdict]
		ut.Rows = append(ut.Rows, []string{"`" + u.Name + "`", u.Type, verdict, why})
		ut.Classes = append(ut.Classes, []string{"", "", cls, ""})
	}
	doc.Sections = append(doc.Sections, Section{ID: "unused", Title: "Unused documents",
		Intro: "Documents of a kind that should be referenced if used, with no inbound reference. \"verify\" means the graph cannot see the way it may be used; check before deleting. Annotate `unused:<Name>`.", Blocks: []Block{ut}})

	return Frame(env, ann, doc), nil
}

func contextsSection(d ArchitectureData, ann *Annotations) Section {
	s := Section{ID: "contexts", Title: "Bounded contexts",
		Intro: "Clusters of tightly connected documents, against the modules they come from. A module spread over several clusters may hold more than one context; a cluster spanning modules may be one context split across them."}
	if !d.CommunitiesComputed {
		s.Blocks = []Block{Callout{Kind: "warn", Text: "Clusters were not computed. Run `refresh catalog communities` (the report command does this unless `--no-communities`)."}}
		return s
	}
	var mods []string
	for _, m := range d.Modules {
		mods = append(mods, m.Name)
	}
	t := Table{Columns: append([]string{"Cluster", "Key", "Size"}, mods...)}
	t.Columns = append(t.Columns, "Note")
	spread := map[string]int{}
	totals := map[string]int{}
	for _, c := range d.Communities {
		for _, mc := range c.Modules {
			totals[mc.Module] += mc.Count
		}
	}
	for _, c := range d.Communities {
		label, note := c.Label, ""
		if c.Ambiguous {
			note = "label derived — name it"
		}
		if n, ok := ann.Get(c.Key); ok {
			label = nonEmpty(n.Name, label)
			note = n.Note
		}
		row := []string{label, "`" + c.Key + "`", fmt.Sprint(c.Size)}
		heat := []float64{0, 0, 0}
		counts := map[string]int{}
		for _, mc := range c.Modules {
			counts[mc.Module] = mc.Count
		}
		for _, m := range mods {
			n := counts[m]
			if n > 0 && totals[m] > 0 && float64(n)/float64(totals[m]) >= 0.2 {
				spread[m]++
			}
			row = append(row, cellCount(n))
			h := 0.0
			if c.Size > 0 {
				h = float64(n) / float64(c.Size)
			}
			heat = append(heat, h)
		}
		row = append(row, note)
		t.Rows = append(t.Rows, row)
		t.Heat = append(t.Heat, append(heat, 0))
	}
	s.Blocks = append(s.Blocks, t)
	var split []string
	for _, m := range mods {
		if spread[m] > 1 {
			split = append(split, fmt.Sprintf("`%s` has at least a fifth of its clustered documents in each of %d clusters", m, spread[m]))
		}
	}
	if len(split) > 0 {
		s.Blocks = append(s.Blocks, Para{Text: "Modules that may hold more than one context:"}, Bullets{Items: split})
	}
	if d.SmallCommunities > 0 {
		s.Blocks = append(s.Blocks, Para{Text: fmt.Sprintf("%d cluster(s) of fewer than three documents not shown.", d.SmallCommunities)})
	}
	return s
}

func captioned(t Table, caption string) Table {
	t.Caption = caption
	return t
}

func cellCount(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}

func couplingMatrix(mods []ModuleHealth, edges []ModuleEdge) Table {
	idx := map[string]int{}
	cols := []string{"uses →"}
	for i, m := range mods {
		idx[m.Name] = i
		cols = append(cols, m.Name)
	}
	w := map[[2]int]int{}
	maxW := 1
	for _, e := range edges {
		i, ok1 := idx[e.From]
		j, ok2 := idx[e.To]
		if ok1 && ok2 {
			w[[2]int{i, j}] = e.Refs
			maxW = max(maxW, e.Refs)
		}
	}
	t := Table{Columns: cols}
	for i, m := range mods {
		row := []string{m.Name}
		cls := []string{""}
		heat := []float64{0}
		for j := range mods {
			n := w[[2]int{i, j}]
			c := ""
			switch {
			case i == j:
				c = "muted"
			case n > 0 && j < i:
				c = "bad"
			}
			row = append(row, cellCount(n))
			cls = append(cls, c)
			h := 0.0
			if n > 0 && c != "bad" {
				h = math.Log1p(float64(n)) / math.Log1p(float64(maxW))
			}
			heat = append(heat, h)
		}
		t.Rows = append(t.Rows, row)
		t.Classes = append(t.Classes, cls)
		t.Heat = append(t.Heat, heat)
	}
	return t
}

// joinNote combines an annotation's parts with derived text, annotation first.
func joinNote(status, note, derived string) string {
	var parts []string
	for _, p := range []string{status, note} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return derived
	}
	s := strings.Join(parts, ": ")
	if derived != "" {
		s += " (" + derived + ")"
	}
	return s
}
