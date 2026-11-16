// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

// The feature view of the software map: the app organised by what it does for
// its users rather than by module. A feature is a plan slice of the project
// brain; what belongs to it is DERIVED from its requirements' anchors and their
// footprints, never listed by hand, so there is no feature map to maintain and
// nothing to go stale. What no feature reaches is listed as unowned: the part
// of the model nobody specified.
//
// Reach, from a feature's anchors:
//
//   - flows: the anchored flows, the requirement footprints (which already
//     include the flows anchored pages, services and entities lead to), the
//     flows behind buttons of anchored pages, and the call closure of all of
//     those;
//   - pages: the anchored pages and the pages those flows open;
//   - entities: the anchored entities (and the entity of an anchored attribute
//     or association) and the entities those flows write.
//
// Reach stops there. Following the pages a feature opens on into their own
// buttons would walk the navigation and give every feature the whole app.

// MapFeature is one feature: a slice of the plan and what it reaches.
type MapFeature struct {
	Name         string          `json:"name"`
	Milestone    string          `json:"milestone,omitempty"`
	Requirements []MapFeatureReq `json:"requirements"`
	Status       map[string]int  `json:"status"`
	Modules      []string        `json:"modules"`
	Pages        []string        `json:"pages"`
	// Flows is every flow in the feature's reach; Core is the ones its
	// requirements are anchored on or whose footprint they are.
	Flows       []string          `json:"flows"`
	Core        []string          `json:"core"`
	Entities    []string          `json:"entities"`
	Other       []string          `json:"other,omitempty"`
	EntryPoints []MapFeatureEntry `json:"entryPoints"`
	Roles       []string          `json:"roles"`
	// SharesWith names the other features that reach some of the same pages,
	// flows or entities.
	SharesWith []string `json:"sharesWith,omitempty"`
}

// MapFeatureReq is a requirement of a feature.
type MapFeatureReq struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	// Milestone is set when the requirement is in a different milestone from
	// its feature.
	Milestone string `json:"milestone,omitempty"`
}

// MapFeatureEntry is a way into a feature: a flow in its reach that something
// outside the feature starts — not a button on one of its own pages.
type MapFeatureEntry struct {
	Flow   string `json:"flow"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
}

// MapUnowned is the app no feature reaches.
type MapUnowned struct {
	Pages       []string `json:"pages"`
	EntryPoints []string `json:"entryPoints"`
	Entities    []string `json:"entities"`
}

// AddFeatures derives the feature view from the traceability data of the
// project brain and adds it to the map. sliceMilestones is each slice's own
// milestone (brain.Store.SliceMilestone). With no requirements it does nothing.
func AddFeatures(d *MapData, t *TraceabilityData, sliceMilestones map[string]string) {
	if d == nil || t == nil {
		return
	}
	flows := map[string]MapFlow{}
	for _, f := range d.Flows {
		flows[f.Name] = f
	}
	pages := map[string]MapPage{}
	for _, p := range d.Pages {
		pages[p.Name] = p
	}
	entities := map[string]MapEntity{}
	writtenBy := map[string][]string{} // flow -> entities it writes
	for _, e := range d.Entities {
		entities[e.Name] = e
		for _, f := range e.WrittenBy {
			writtenBy[f] = append(writtenBy[f], e.Name)
		}
	}

	type acc struct {
		f                            *MapFeature
		pages, flows, ents, other    map[string]bool
		reachFlows, reachPages, mods map[string]bool
	}
	var order []string
	bySlice := map[string]*acc{}
	reqMilestone := map[string]string{}
	for _, m := range t.Milestones {
		for _, r := range m.Requirements {
			if m.Name != r.Slice && m.Name != notInMilestone {
				reqMilestone[r.ID] = m.Name
			}
		}
	}
	for _, m := range t.Milestones {
		for _, r := range m.Requirements {
			a := bySlice[r.Slice]
			if a == nil {
				a = &acc{f: &MapFeature{Name: r.Slice, Status: map[string]int{}},
					pages: map[string]bool{}, flows: map[string]bool{}, ents: map[string]bool{}, other: map[string]bool{},
					reachFlows: map[string]bool{}, reachPages: map[string]bool{}, mods: map[string]bool{}}
				bySlice[r.Slice] = a
				order = append(order, r.Slice)
			}
			a.f.Requirements = append(a.f.Requirements, MapFeatureReq{ID: r.ID, Title: r.Title, Status: r.Status, Milestone: reqMilestone[r.ID]})
			a.f.Status[r.Status]++
			for _, an := range r.Anchors {
				if an.State != brain.Resolved {
					continue
				}
				name := strings.TrimPrefix(an.Anchor, "@")
				switch strings.ToLower(strings.ReplaceAll(an.Kind, " ", "_")) {
				case "page", "snippet":
					a.pages[name] = true
				case "microflow", "nanoflow":
					a.flows[name] = true
				case "entity":
					a.ents[name] = true
				case "attribute", "association":
					if i := strings.LastIndexByte(name, '.'); i > 0 {
						a.ents[name[:i]] = true
					}
				default:
					a.other[name] = true
				}
			}
			for _, fp := range r.Footprint {
				a.flows[fp] = true
			}
		}
	}

	// Reach.
	owner := map[string]map[string]bool{} // element -> features reaching it
	own := func(el, feature string) {
		if owner[el] == nil {
			owner[el] = map[string]bool{}
		}
		owner[el][feature] = true
	}
	for _, slice := range order {
		a := bySlice[slice]
		var seeds []string
		for f := range a.flows {
			seeds = append(seeds, f)
		}
		for p := range a.pages {
			a.reachPages[p] = true
			seeds = append(seeds, pages[p].Flows...)
		}
		sort.Strings(seeds)
		for len(seeds) > 0 {
			n := seeds[0]
			seeds = seeds[1:]
			if a.reachFlows[n] {
				continue
			}
			f, ok := flows[n]
			if !ok {
				continue // outside the app (Marketplace, System) or not a flow
			}
			a.reachFlows[n] = true
			seeds = append(seeds, f.Calls...)
			for _, p := range f.Pages {
				if _, ok := pages[p]; ok {
					a.reachPages[p] = true
				}
			}
			for _, e := range writtenBy[n] {
				a.ents[e] = true
			}
		}
		for p := range a.reachPages {
			a.pages[p] = true
		}
		// Only what is in the app is shown: an anchor on a Marketplace page is
		// a requirement on it, not a part of this app.
		for p := range a.pages {
			if _, ok := pages[p]; !ok {
				delete(a.pages, p)
			}
		}
		for f := range a.flows {
			if _, ok := flows[f]; !ok {
				delete(a.flows, f)
			}
		}
		for e := range a.ents {
			if _, ok := entities[e]; !ok {
				delete(a.ents, e)
			}
		}
		for p := range a.pages {
			own("page:"+p, slice)
			a.mods[moduleOf(p)] = true
		}
		for f := range a.reachFlows {
			own("flow:"+f, slice)
			a.mods[moduleOf(f)] = true
		}
		for e := range a.ents {
			own("entity:"+e, slice)
			a.mods[moduleOf(e)] = true
		}
		f := a.f
		f.Pages, f.Flows, f.Core = sortedKeys(a.pages), sortedKeys(a.reachFlows), sortedKeys(a.flows)
		f.Entities, f.Other = sortedKeys(a.ents), sortedKeys(a.other)
		f.Modules = sortedKeys(a.mods)
		for _, n := range sortedKeys(a.reachFlows) {
			for _, tr := range flows[n].Triggers {
				// A button on one of the feature's own pages is how the
				// feature works inside, not a way into it.
				if a.pages[tr.Source] && (tr.Kind == "page button" || tr.Kind == "page data source") {
					continue
				}
				f.EntryPoints = append(f.EntryPoints, MapFeatureEntry{Flow: n, Kind: tr.Kind, Source: tr.Source})
			}
		}
		sort.SliceStable(f.EntryPoints, func(i, j int) bool { return kindOrder(f.EntryPoints[i].Kind) < kindOrder(f.EntryPoints[j].Kind) })
		// The user roles that may open one of its pages.
		for _, r := range d.Roles {
			mr := setOf(r.ModuleRoles)
		page:
			for p := range a.pages {
				for _, x := range pages[p].Roles {
					if mr[x] {
						f.Roles = append(f.Roles, r.Name)
						break page
					}
				}
			}
		}
		f.Milestone = sliceMilestones[slice]
		for i := range f.Requirements {
			if f.Requirements[i].Milestone == f.Milestone {
				f.Requirements[i].Milestone = ""
			}
		}
	}
	for _, slice := range order {
		a := bySlice[slice]
		shares := map[string]bool{}
		for _, fs := range owner {
			if !fs[slice] {
				continue
			}
			for o := range fs {
				if o != slice {
					shares[o] = true
				}
			}
		}
		a.f.SharesWith = sortedKeys(shares)
		d.Features = append(d.Features, *a.f)
	}

	// Unowned: the app no feature reaches.
	if len(d.Features) == 0 {
		return
	}
	u := &MapUnowned{}
	for _, p := range d.Pages {
		if owner["page:"+p.Name] == nil {
			u.Pages = append(u.Pages, p.Name)
		}
	}
	for _, f := range d.Flows {
		if len(f.Triggers) > 0 && owner["flow:"+f.Name] == nil {
			u.EntryPoints = append(u.EntryPoints, f.Name)
		}
	}
	for _, e := range d.Entities {
		if e.Persistent && owner["entity:"+e.Name] == nil {
			u.Entities = append(u.Entities, e.Name)
		}
	}
	d.Unowned = u
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// featureStatus is "3 built, 1 partial" for a feature's requirements.
func featureStatus(st map[string]int) string {
	var parts []string
	for _, s := range []string{StatusBuilt, StatusPartial, StatusChanged, StatusNotBuilt, StatusNotTraceable} {
		if n := st[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	return strings.Join(parts, ", ")
}
