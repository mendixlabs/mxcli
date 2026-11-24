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

	columnGutter = 90 // horizontal space between layers, at least
	rowGutter    = 40 // vertical space between entities within a layer
	bandGutter   = 140

	// An association's name label and the 1/* markers at its two ends share
	// the line between two boxes; a shorter line cuts the label off or draws
	// it on a box (mendixlabs/mxcli#1929, rule 5). Measured in Studio Pro
	// (classic and 11.16 beta): about 5 px per character plus ~12 of padding.
	// 5.5 leaves room for wide glyphs.
	labelPxPerChar2 = 11 // 5.5 px per character, kept in integer halves
	labelPadding    = 12
	markerAllowance = 40 // the 1 / * markers and their margin, both ends
)

// labelWidth estimates the drawn width of an association's name label.
func labelWidth(name string) int {
	return (len(name)*labelPxPerChar2+1)/2 + labelPadding
}

// Plan computes a position for every entity in one domain model.
//
// The layout is layered on the association graph. An entity sits one layer
// past the furthest thing it references, and a lookup (one that references
// nothing in the module) one layer left of its nearest referrer. Layers become
// columns, so what an entity points at is to its left and the leaves are on
// the right, with most association lines running the same way. After placing,
// each lookup follows its referrers' row where there is room (pullLookups).
//
// Entities with no local edge at all (a non-persistent helper, a freshly created
// entity nothing points at yet) are not lookups and do not belong among them;
// they go in their own band underneath.
//
// The result is deterministic for a given model: every iteration order here is
// sorted, because a map range would produce a different diagram on every run and
// re-writing the unit each time is exactly the churn ADR-0008 exists to prevent.
func Plan(dm *domainmodel.DomainModel) map[model.ID]model.Point {
	return PlanWith(dm, nil)
}

// PlanWith is Plan for a module whose diagram also draws cross-module lines:
// cross names, per entity, the lines Studio Pro draws from its left side to
// the canvas edge (see CrossLines). Each such entity is placed where all of
// its lines, label height included, miss every box to its left (rule 2 of
// mendixlabs/mxcli#1929): in its column, at its target's row when that is
// clear, otherwise the first clear y below. Column 0 starts far enough right
// for the longest name drawn from it, since the label sits between the canvas
// edge and the box (rule 3). An entity in the isolated band with such a line
// gets a row of its own at the left.
//
// PlanWith(dm, nil) is Plan(dm).
func PlanWith(dm *domainmodel.DomainModel, cross CrossLines) map[model.ID]model.Point {
	if dm == nil {
		return nil
	}
	g := newGraph(dm)
	if len(g.order) == 0 {
		return map[model.ID]model.Point{}
	}

	base, isolated := g.layers()

	// Rule 1 of mendixlabs/mxcli#1929: as few crossings as possible. The order
	// of the entities within each column decides many of them. Starting from
	// the order the layering made, swap two neighbours in a column and keep the
	// swap when the whole placement, scored on the lines it actually draws,
	// has strictly fewer crossings; repeat until no swap helps. A swap is kept
	// only on a strict improvement, so the result is never worse than the
	// layering's order by this measure, and it stays deterministic.
	//
	// Whole-column barycentre sweeps (the textbook step) were tried first and
	// dropped: on ako/PizzaV1 and on thousands of random models they never
	// produced a better order than this start, because row alignment and the
	// cross-module bands decide most positions; single swaps did (PizzaV1
	// Catalog: 8 crossings to 6).
	score := func(layers [][]model.ID) (map[model.ID]model.Point, int) {
		pos := g.place(layers, isolated, cross)
		lines, boxes := Crossings(dm, pos, Anchors(dm, pos), cross)
		// A line through a box hides what is in it; a crossing only costs a
		// moment's tracing.
		return pos, lines + 2*boxes
	}
	bestLayers := base
	best, bestScore := score(base)
	// The budget grows with the model, so a large module stays fast.
	budget := 8 * len(g.order)
	for improved := true; improved && budget > 0 && bestScore > 0; {
		improved = false
		for li := range bestLayers {
			for i := 0; i+1 < len(bestLayers[li]) && budget > 0; i++ {
				cand := copyLayers(bestLayers)
				cand[li][i], cand[li][i+1] = cand[li][i+1], cand[li][i]
				budget--
				if pos, sc := score(cand); sc < bestScore {
					bestLayers, best, bestScore = cand, pos, sc
					improved = true
				}
			}
		}
	}
	return best
}

// place lays out one ordering of the layers, plus the isolated band: the body
// of PlanWith for a fixed order of the entities within each column.
func (g *graph) place(layers [][]model.ID, isolated []model.ID, cross CrossLines) map[model.ID]model.Point {
	out := make(map[model.ID]model.Point, len(g.order))

	layerOf := make(map[model.ID]int, len(g.order))
	for li, layer := range layers {
		for _, id := range layer {
			layerOf[id] = li
		}
	}
	gaps := g.columnGaps(layerOf, len(layers))

	// The isolated entities with a cross-module line go first in the band, each
	// in a row of its own at the left (see below).
	var crossIso, plainIso []model.ID
	for _, id := range isolated {
		if len(cross[id]) > 0 {
			crossIso = append(crossIso, id)
		} else {
			plainIso = append(plainIso, id)
		}
	}

	// Column 0 is where the canvas edge is closest: its cross-module labels
	// need the room between x = 0 and the box.
	x0 := OriginX
	var leftmost []model.ID
	if len(layers) > 0 {
		leftmost = append(leftmost, layers[0]...)
	}
	for _, id := range append(leftmost, crossIso...) {
		for _, name := range cross[id] {
			x0 = max(x0, labelWidth(name)+markerAllowance)
		}
	}

	var placed []Rect
	x := x0
	maxY := OriginY
	for li, layer := range layers {
		widest := 0
		var col column
		for _, id := range layer {
			w, h := boxSize(g.byID[id])
			widest = max(widest, w)
			// On the row of the entity it is most linked to, so the line between
			// them runs straight across (rule 4). Layer 0 references nothing
			// placed yet, so it stacks; so does an entity whose only targets
			// share its layer (a cycle).
			want := col.bottom()
			if t, ok := g.primaryTarget(id, out); ok {
				want = out[t].Y
			}
			var y int
			if n := len(cross[id]); n > 0 {
				// The first spot where every one of its cross-module lines is
				// clear of the boxes to its left; under all of them if nothing
				// nearer is, which always is.
				clear := func(y int) bool { return bandClear(placed, x, y, h, n) }
				var ok bool
				if y, ok = col.placeWhere(want, h, bandCandidates(placed, x, h, n, want), clear); !ok {
					y = col.place(max(want, belowLeft(placed, x)), h)
				}
			} else {
				y = col.place(want, h)
			}
			out[id] = model.Point{X: x, Y: y}
			placed = append(placed, Rect{X: x, Y: y, W: w, H: h})
		}
		maxY = max(maxY, col.bottom())
		if li < len(gaps) {
			x += widest + gaps[li]
		}
	}

	// Lookups were stacked before anything referencing them was placed; now
	// they can follow their referrers' rows.
	g.pullLookups(layers, layerOf, out, cross)
	for _, id := range g.order {
		if p, ok := out[id]; ok {
			_, h := boxSize(g.byID[id])
			maxY = max(maxY, p.Y+h+rowGutter)
		}
	}

	// The isolated band: a plain grid under everything else, so it reads as a
	// separate group rather than as another layer of the graph. Entities in it
	// with a cross-module line come first, one per row at the left edge, the
	// only place in a grid row where nothing is to their left.
	y0 := maxY + bandGutter
	for _, id := range crossIso {
		_, h := boxSize(g.byID[id])
		out[id] = model.Point{X: x0, Y: y0}
		y0 += h + rowGutter
	}
	if len(crossIso) > 0 {
		maxY = y0 - bandGutter
	}
	isolated = plainIso
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
		x := x0
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
	edges []edge     // every local edge, in model order
	out   map[model.ID][]model.ID
	in    map[model.ID][]model.ID // the entities referencing each one
	deg   map[model.ID]int        // total edges, either direction
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
		in:   map[model.ID][]model.ID{},
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

	add := func(from, to model.ID, label string) {
		if from == to || g.byID[from] == nil || g.byID[to] == nil {
			return
		}
		g.out[from] = append(g.out[from], to)
		g.in[to] = append(g.in[to], from)
		g.deg[from]++
		g.deg[to]++
		g.edges = append(g.edges, edge{from: from, to: to, label: label})
	}
	for _, a := range dm.Associations {
		if a != nil {
			add(a.ParentID, a.ChildID, a.Name)
		}
	}
	// A generalization is drawn as an arrow with no name and no multiplicity,
	// so it asks nothing of the gap it crosses.
	for _, e := range dm.Entities {
		if e != nil && e.GeneralizationID != "" {
			add(e.ID, e.GeneralizationID, "")
		}
	}
	for from := range g.out {
		sort.Slice(g.out[from], func(i, j int) bool {
			return g.byID[g.out[from][i]].Name < g.byID[g.out[from][j]].Name
		})
	}
	return g
}

// edge is one local association or generalization, FROM the referencing entity
// TO the referenced one. label is the association's name, drawn on the line;
// empty for a generalization.
type edge struct {
	from, to model.ID
	label    string
}

// columnGaps returns the horizontal gap after each layer but the last: the
// gutter, or wider where an association between that layer and the next needs
// room for its name and both multiplicity markers (rule 5).
//
// Only associations between ADJACENT layers size a gap. A line spanning several
// layers has its label over a middle column whatever the gaps are, which is a
// crossing to avoid (rule 1), not a gap to widen.
func (g *graph) columnGaps(layerOf map[model.ID]int, n int) []int {
	if n < 2 {
		return nil
	}
	gaps := make([]int, n-1)
	for i := range gaps {
		gaps[i] = columnGutter
	}
	for _, e := range g.edges {
		if e.label == "" {
			continue
		}
		a, b := layerOf[e.from], layerOf[e.to]
		if a > b {
			a, b = b, a
		}
		if b-a != 1 {
			continue
		}
		gaps[a] = max(gaps[a], labelWidth(e.label)+markerAllowance)
	}
	return gaps
}

// primaryTarget is the already-placed entity that id references most, ties
// broken by name: the one whose row id takes. g.out is sorted by name and holds
// one entry per edge, so the first to reach the highest count wins.
func (g *graph) primaryTarget(id model.ID, placed map[model.ID]model.Point) (model.ID, bool) {
	counts := map[model.ID]int{}
	var best model.ID
	bestN := 0
	for _, t := range g.out[id] {
		if _, ok := placed[t]; !ok {
			continue
		}
		counts[t]++
		if counts[t] > bestN {
			best, bestN = t, counts[t]
		}
	}
	return best, bestN > 0
}

// column is the vertical spans already taken in one layer, each box with the
// row gutter under it.
type column struct {
	spans [][2]int // [top, bottom+rowGutter)
}

// bottom is where the next box stacked under everything would go.
func (c *column) bottom() int {
	y := OriginY
	for _, s := range c.spans {
		y = max(y, s[1])
	}
	return y
}

// place takes the free y nearest at or below want for a box of height h, and
// records it.
func (c *column) place(want, h int) int {
	y, _ := c.placeWhere(want, h, nil, nil)
	return y
}

// placeWhere is place with a further condition: the lowest candidate at or
// below want that is free in the column AND passes ok (nil: always) wins.
// Candidates are want, the end of every span below it, and extra. When none
// passes ok it reports false and records nothing; the column-free fallback
// below everything is always free, so with ok nil it always succeeds.
func (c *column) placeWhere(want, h int, extra []int, ok func(y int) bool) (int, bool) {
	want = max(want, OriginY)
	cands := []int{want}
	for _, s := range c.spans {
		if s[1] > want {
			cands = append(cands, s[1])
		}
	}
	for _, y := range extra {
		if y >= want {
			cands = append(cands, y)
		}
	}
	cands = append(cands, max(want, c.bottom()))
	sort.Ints(cands)
	for _, y := range cands {
		if !c.free(y, h) || (ok != nil && !ok(y)) {
			continue
		}
		c.spans = append(c.spans, [2]int{y, y + h + rowGutter})
		return y, true
	}
	return 0, false
}

// free reports whether a box of height h at y clears every span in the column.
func (c *column) free(y, h int) bool {
	for _, s := range c.spans {
		if y < s[1] && s[0] < y+h+rowGutter {
			return false
		}
	}
	return true
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

	// A lookup (references nothing, is referenced) goes in the column just left
	// of its nearest referrer rather than always in column 0: a lookup only a
	// deep entity uses would otherwise sit columns away from it, its line
	// crossing everything between. Only lookups move, so every other layer
	// stays one past what it references; a lookup's referrers are all at least
	// one layer deeper than it, so the result is still at least 0 and still
	// left of every referrer.
	for _, id := range g.order {
		if g.deg[id] == 0 || len(g.out[id]) > 0 || len(g.in[id]) == 0 {
			continue
		}
		nearest := -1
		for _, r := range g.in[id] {
			if nearest < 0 || depth[r] < nearest {
				nearest = depth[r]
			}
		}
		depth[id] = max(0, nearest-1)
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
