// SPDX-License-Identifier: Apache-2.0

// Package dmlayout decides where entities sit in the domain-model editor.
//
// It has two entry points, for the two moments a position is needed.
//
// GridSlot answers "an entity is being created and the script said nothing
// about where" — a wrapping grid, no knowledge of the model. Plan answers "lay
// this module out properly", using the association graph so related entities end
// up near each other.
//
// # Why a package rather than a formula at the call site
//
// The default used to be one line in the CREATE ENTITY handler:
//
//	location = model.Point{X: 100 + len(dm.Entities)*150, Y: 100}
//
// Every entity on one row at y=100. A 40-entity model is a 6,000px line, and
// 150px is narrower than an entity box, so the boxes touch. Both halves of the
// fix want the same notion of "how big is a box and how far apart do they go",
// so it lives here once.
//
// # What is NOT known
//
// An entity stores only its Location. There is no Size in the model — Studio Pro
// derives the box from the entity's name and member list when it draws, and
// mxcli never sees the result. So every dimension below is an ESTIMATE from the
// model, deliberately generous: too much space is a diagram that scrolls, too
// little is the overlap this package exists to remove.
//
// An entity's Location is the box's TOP-LEFT corner. It is NOT the centre:
// microflow objects store a RelativeMiddlePoint, and this package used to borrow
// that convention, which put every box half a box further down and right than
// planned and let tall entities run into the one below (mendixlabs/mxcli#1479,
// measured in Studio Pro 11.12.5). mxcli's own SVG renderer
// (mdl/executor/cmd_domainmodel_svg.go) already drew boxes from the top-left.
//
// # Three entry points
//
// GridSlot is the bare grid. Place is what a create path calls: the first grid
// slot from the entity's count onwards where the new box, at its estimated size,
// clears every box already in the model — a row pitch alone cannot do that,
// because one entity with twenty attributes is taller than any row. Plan lays a
// whole module out; ResolveOverlaps moves only the entities that collide and
// leaves every other position, hand-placed or not, exactly where it was.
package dmlayout

import (
	"sort"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Grid geometry for auto-placed entities.
//
// GridColumns is fixed rather than derived from the entity count: the count is
// not known when the first entity of a script is created, and a column count
// that grew mid-script would move entities that were already placed.
const (
	OriginX = 100
	OriginY = 100

	GridColumns     = 6
	GridColumnPitch = 260
	GridRowPitch    = 280
)

// GridSlot returns the position for the n-th entity placed without an explicit
// @Position, counting from 0.
//
// Stable by construction: slot n does not depend on anything but n, so adding an
// entity never moves one already written.
func GridSlot(n int) model.Point {
	if n < 0 {
		n = 0
	}
	return model.Point{
		X: OriginX + (n%GridColumns)*GridColumnPitch,
		Y: OriginY + (n/GridColumns)*GridRowPitch,
	}
}

// Box size estimates, from measurements in Studio Pro: classic viewer and the
// 11.16 beta viewer, each screenshot calibrated against the positions stored in
// its project (PROPOSAL_domain_model_readable_layout.md, "Measurements").
//
// A project is opened in either viewer, so each dimension takes the larger:
//
//   - Width is FIXED: 150 classic, 158 beta, whatever the names, which are cut
//     off with "…". It does not grow with a long name; an estimate that did
//     (8 px per character, before) only spread the columns apart.
//   - Height is the header plus one row per attribute, with a minimum of about
//     three rows: classic max(84, 36 + 16n), beta max(~76, 28 + 16n). The 4 px
//     over the classic header absorbs the few px of measurement error, so no
//     drawn box is taller than its estimate. The minimum matters most: a 0–2
//     attribute entity is drawn 84 tall, and estimating 60 let the box below it
//     touch it (MPR016, Place).
const (
	boxWidth      = 160
	pixelsPerChar = 8  // rough advance width of the editor's font, for annotation captions
	headerHeight  = 40 // the entity's name bar, with margin
	memberHeight  = 16 // one attribute row
	minBoxHeight  = 84 // three rows: what Studio Pro draws for 0–3 attributes

	columnGutter = 90 // horizontal space between layers
	rowGutter    = 40 // vertical space between entities within a layer
	bandGutter   = 140
)

// Plan computes a position for every entity in one domain model.
//
// The layout is layered on the association graph. An entity that references
// nothing else in the module is layer 0; anything else sits one layer past the
// furthest thing it references. Layers become columns, so the things everything
// points at — the lookup tables — end up on the left and the leaves on the
// right, with most association lines running the same way.
//
// Entities with no local edge at all (a non-persistent helper, a freshly created
// entity nothing points at yet) are not lookups and do not belong in layer 0
// beside them; they go in their own band underneath.
//
// The result is deterministic for a given model: every iteration order here is
// sorted, because a map range would produce a different diagram on every run and
// re-writing the unit each time is exactly the churn ADR-0008 exists to prevent.
func Plan(dm *domainmodel.DomainModel) map[model.ID]model.Point {
	if dm == nil {
		return nil
	}
	g := newGraph(dm)
	if len(g.order) == 0 {
		return map[model.ID]model.Point{}
	}

	layers, isolated := g.layers()
	out := make(map[model.ID]model.Point, len(g.order))

	x := OriginX
	maxY := OriginY
	for _, layer := range layers {
		widest := 0
		y := OriginY
		for _, id := range layer {
			e := g.byID[id]
			w, h := boxSize(e)
			if w > widest {
				widest = w
			}
			out[id] = model.Point{X: x, Y: y}
			y += h + rowGutter
		}
		if y > maxY {
			maxY = y
		}
		x += widest + columnGutter
	}

	// The isolated band: a plain grid under everything else, so it reads as a
	// separate group rather than as another layer of the graph.
	if len(isolated) > 0 {
		// Rows are as tall as their tallest member, not a fixed pitch: a fixed
		// pitch is the #1479 overlap all over again for a 20-attribute helper.
		//
		// Columns are as wide as their widest member for the same reason: a long
		// entity name is wider than GridColumnPitch.
		colX := make([]int, GridColumns)
		colW := make([]int, GridColumns)
		for i, id := range isolated {
			if w, _ := boxSize(g.byID[id]); w > colW[i%GridColumns] {
				colW[i%GridColumns] = w
			}
		}
		x := OriginX
		for c := range colX {
			colX[c] = x
			x += colW[c] + columnGutter
		}
		y := maxY + bandGutter
		rowHeight := 0
		for i, id := range isolated {
			if i > 0 && i%GridColumns == 0 {
				y += rowHeight + rowGutter
				rowHeight = 0
			}
			_, h := boxSize(g.byID[id])
			if h > rowHeight {
				rowHeight = h
			}
			out[id] = model.Point{X: colX[i%GridColumns], Y: y}
		}
	}
	return out
}

// boxSize estimates the drawn size of an entity from its attribute count. The
// name does not matter: Studio Pro draws every entity the same width.
func boxSize(e *domainmodel.Entity) (w, h int) {
	n := 0
	if e != nil {
		n = len(e.Attributes)
	}
	return boxWidth, max(minBoxHeight, headerHeight+n*memberHeight)
}

// graph is the module's entities plus the local edges between them.
type graph struct {
	byID  map[model.ID]*domainmodel.Entity
	order []model.ID // entity ids, sorted by name — the determinism anchor
	out   map[model.ID][]model.ID
	deg   map[model.ID]int // total edges, either direction
}

// newGraph indexes the domain model. An edge runs FROM the entity that holds the
// reference TO the entity referenced — the same direction MDL's `from`/`to`
// spells, which for an association is ParentID -> ChildID (see CLAUDE.md on the
// inverted parent/child naming).
//
// A generalization is an edge too: a specialisation belongs beside its base, and
// the direction matches (the specialisation depends on the base).
//
// Cross-module associations are skipped: their target is not in this domain
// model, so it cannot be placed relative to anything here.
func newGraph(dm *domainmodel.DomainModel) *graph {
	g := &graph{
		byID: make(map[model.ID]*domainmodel.Entity, len(dm.Entities)),
		out:  map[model.ID][]model.ID{},
		deg:  map[model.ID]int{},
	}
	for _, e := range dm.Entities {
		if e == nil {
			continue
		}
		g.byID[e.ID] = e
	}
	for id := range g.byID {
		g.order = append(g.order, id)
	}
	sort.Slice(g.order, func(i, j int) bool {
		return g.byID[g.order[i]].Name < g.byID[g.order[j]].Name
	})

	add := func(from, to model.ID) {
		if from == to || g.byID[from] == nil || g.byID[to] == nil {
			return
		}
		g.out[from] = append(g.out[from], to)
		g.deg[from]++
		g.deg[to]++
	}
	for _, a := range dm.Associations {
		if a != nil {
			add(a.ParentID, a.ChildID)
		}
	}
	for _, e := range dm.Entities {
		if e != nil && e.GeneralizationID != "" {
			add(e.ID, e.GeneralizationID)
		}
	}
	for from := range g.out {
		sort.Slice(g.out[from], func(i, j int) bool {
			return g.byID[g.out[from][i]].Name < g.byID[g.out[from][j]].Name
		})
	}
	return g
}

// layers assigns each connected entity a layer and returns the layers in order,
// plus the entities that have no local edge at all.
func (g *graph) layers() (layers [][]model.ID, isolated []model.ID) {
	depth := make(map[model.ID]int, len(g.order))
	const (
		unvisited = 0
		active    = 1
		done      = 2
	)
	state := make(map[model.ID]int, len(g.order))

	// Longest path to a sink. A cycle would make that undefined, so an edge back
	// into the current path contributes nothing — the entities in the cycle land
	// in the same layer, which is where they belong anyway.
	var visit func(id model.ID) int
	visit = func(id model.ID) int {
		switch state[id] {
		case done:
			return depth[id]
		case active:
			return 0
		}
		state[id] = active
		best := 0
		for _, t := range g.out[id] {
			if d := visit(t) + 1; d > best {
				best = d
			}
		}
		state[id] = done
		depth[id] = best
		return best
	}

	maxDepth := 0
	for _, id := range g.order {
		if g.deg[id] == 0 {
			isolated = append(isolated, id)
			continue
		}
		if d := visit(id); d > maxDepth {
			maxDepth = d
		}
	}

	layers = make([][]model.ID, maxDepth+1)
	for _, id := range g.order {
		if g.deg[id] == 0 {
			continue
		}
		layers[depth[id]] = append(layers[depth[id]], id)
	}

	// Within a layer, order by where an entity's targets already sit, so lines
	// run roughly straight instead of crossing the diagram. Ties — and layer 0,
	// which has no targets — fall back to the name order g.order is already in,
	// which is what keeps the result deterministic.
	pos := map[model.ID]int{}
	for li, layer := range layers {
		if li > 0 {
			sort.SliceStable(layer, func(i, j int) bool {
				return g.barycentre(layer[i], pos) < g.barycentre(layer[j], pos)
			})
		}
		for i, id := range layer {
			pos[id] = i
		}
	}
	return layers, isolated
}

// barycentre is the mean row of an entity's already-placed targets, or a large
// sentinel when it has none placed yet so those sink to the bottom of the layer
// rather than jostling the ones that do.
func (g *graph) barycentre(id model.ID, pos map[model.ID]int) float64 {
	sum, n := 0, 0
	for _, t := range g.out[id] {
		if p, ok := pos[t]; ok {
			sum += p
			n++
		}
	}
	if n == 0 {
		return 1 << 20
	}
	return float64(sum) / float64(n)
}
