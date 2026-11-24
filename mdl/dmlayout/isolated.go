// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"sort"

	"github.com/mendixlabs/mxcli/model"
)

// packIsolated places the entities with no association in the module (out
// already holds the graph) in the free space to the right of the graph or
// below it, keeping them a group apart from the graph's columns.
//
// A fixed band under the graph wasted the room beside it: in PizzaV1's Shop
// one unconnected entity (TrackView) is taller than the whole graph, and the
// band put it, and the rest, underneath, doubling the height.
//
// Each entity takes the candidate position that keeps the diagram's bounding
// box closest to a landscape frame — the smallest max(width, 1.5 × height),
// then the smallest area, then the highest, then the leftmost — among those
// where
//   - its box is clear of every placed box;
//   - its own cross-module lines clear every box to its left;
//   - it is not in the cross-module lines of an entity to its right;
//   - it is outside the graph's area, so no association line of the graph,
//     which runs within that area, passes through it.
//
// Entities with cross-module lines go first, shortest first: a taller box's
// lines, at fractions of its height, then run below its shorter neighbours.
// The rest follow, tallest first, the usual order for packing.
func (g *graph) packIsolated(ids []model.ID, cross CrossLines, x0 int, out map[model.ID]model.Point) {
	if len(ids) == 0 {
		return
	}
	var placed []Rect
	graphRight, graphBottom := 0, 0
	for _, id := range g.order {
		if p, ok := out[id]; ok {
			w, h := boxSize(g.byID[id])
			placed = append(placed, Rect{X: p.X, Y: p.Y, W: w, H: h})
			graphRight, graphBottom = max(graphRight, p.X+w), max(graphBottom, p.Y+h)
		}
	}
	graphPlaced := len(placed) > 0

	height := func(id model.ID) int { _, h := boxSize(g.byID[id]); return h }
	var withLines, without []model.ID
	for _, id := range ids {
		if len(cross[id]) > 0 {
			withLines = append(withLines, id)
		} else {
			without = append(without, id)
		}
	}
	sort.SliceStable(withLines, func(i, j int) bool { return height(withLines[i]) < height(withLines[j]) })
	sort.SliceStable(without, func(i, j int) bool { return height(without[i]) > height(without[j]) })

	type strip struct{ x, lo, hi int }
	var strips []strip // the cross-module lines of placed isolated entities
	pitch := boxWidth + columnGutter

	for _, id := range append(withLines, without...) {
		w, h := boxSize(g.byID[id])
		n := len(cross[id])

		// Candidate columns: the grid from x0 below the graph, and from just
		// past the graph to its right. Candidate rows: the top, the graph's
		// bottom, every placed box's bottom, and where this entity's lines
		// just clear a box.
		var xs []int
		for k := 0; k < GridColumns+len(ids); k++ {
			xs = append(xs, x0+k*pitch)
			if len(placed) > 0 {
				xs = append(xs, graphRight+columnGutter+k*pitch)
			}
		}
		ys := []int{OriginY}
		if len(placed) > 0 {
			ys = append(ys, graphBottom+bandGutter)
		}
		for _, r := range placed {
			ys = append(ys, r.Y+r.H+rowGutter)
		}

		bestCost, bestArea, bx, by := -1, -1, 0, 0
		consider := func(x, y int) {
			r := Rect{X: x, Y: y, W: w, H: h}
			if graphPlaced && x < graphRight+columnGutter && y < graphBottom+bandGutter {
				return // inside the graph's area: keep the groups apart
			}
			for _, o := range placed {
				if r.TooClose(o) {
					return
				}
			}
			if n > 0 && !bandClear(placed, x, y, h, n) {
				return
			}
			for _, s := range strips {
				if s.x > x && y < s.hi && y+h > s.lo {
					return
				}
			}
			minX, minY, maxX, maxY := x, y, x+w, y+h
			for _, o := range placed {
				minX, minY = min(minX, o.X), min(minY, o.Y)
				maxX, maxY = max(maxX, o.X+o.W), max(maxY, o.Y+o.H)
			}
			bw, bh := maxX-minX, maxY-minY
			cost, area := max(2*bw, 3*bh), bw*bh // max(w, 1.5h), doubled to stay integer
			better := bestCost < 0 || cost < bestCost ||
				(cost == bestCost && (area < bestArea || (area == bestArea && (y < by || (y == by && x < bx)))))
			if better {
				bestCost, bestArea, bx, by = cost, area, x, y
			}
		}
		for _, x := range xs {
			cands := append([]int(nil), ys...)
			if n > 0 {
				for _, y := range bandCandidates(placed, x, h, n, OriginY) {
					cands = append(cands, y)
				}
			}
			for _, y := range cands {
				consider(x, y)
			}
		}
		if bestCost < 0 {
			// Under everything, at the left: always clear.
			by = OriginY
			for _, o := range placed {
				by = max(by, o.Y+o.H+bandGutter)
			}
			bx = x0
		}
		out[id] = model.Point{X: bx, Y: by}
		placed = append(placed, Rect{X: bx, Y: by, W: w, H: h})
		for i := 0; i < n; i++ {
			ly := lineY(by, h, i, n)
			strips = append(strips, strip{bx, ly - labelHalf, ly + labelHalf})
		}
	}
}
