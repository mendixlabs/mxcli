// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"sort"

	"github.com/mendixlabs/mxcli/model"
)

// pullLookups moves each lookup (an entity that references nothing in the
// module and is referenced) towards the median row of the entities
// referencing it. Columns are placed left to right, so a lookup was stacked
// before any of its referrers had a row; phase 1 then lined the referrers up
// with their own targets, and phase 3 pushed some down for their cross-module
// lines, leaving lookups at the top of their column with long lines down to
// what uses them.
//
// A lookup only moves to a y that is free in its column, keeps its own
// cross-module lines clear of the boxes to its left, stays out of the
// cross-module lines of every entity to its right (those run left across this
// column), and is closer to the median than where it was. Lookups are taken
// column by column, then by name, so the result is deterministic.
func (g *graph) pullLookups(layers [][]model.ID, layerOf map[model.ID]int, out map[model.ID]model.Point, cross CrossLines) {
	for _, layer := range layers {
		for _, id := range layer {
			if len(g.out[id]) > 0 || len(g.in[id]) == 0 {
				continue
			}
			g.pullLookup(id, layers[layerOf[id]], out, cross)
		}
	}
}

func (g *graph) pullLookup(id model.ID, layer []model.ID, out map[model.ID]model.Point, cross CrossLines) {
	var ys []int
	for _, r := range g.in[id] {
		if p, ok := out[r]; ok {
			ys = append(ys, p.Y)
		}
	}
	if len(ys) == 0 {
		return
	}
	sort.Ints(ys)
	want := ys[(len(ys)-1)/2]
	cur := out[id]
	if cur.Y == want {
		return
	}
	_, h := boxSize(g.byID[id])

	// The rest of its column, as it stands.
	var col column
	for _, o := range layer {
		if o == id {
			continue
		}
		_, oh := boxSize(g.byID[o])
		p := out[o]
		col.spans = append(col.spans, [2]int{p.Y, p.Y + oh + rowGutter})
	}

	// Every other box, and the cross-module line strips of entities to the
	// right of this column.
	var left []Rect
	type strip struct{ lo, hi int }
	var strips []strip
	for _, o := range g.order {
		p, ok := out[o]
		if !ok || o == id {
			continue
		}
		ow, oh := boxSize(g.byID[o])
		if p.X < cur.X {
			left = append(left, Rect{X: p.X, Y: p.Y, W: ow, H: oh})
		}
		if n := len(cross[o]); n > 0 && p.X > cur.X {
			for i := 0; i < n; i++ {
				ly := lineY(p.Y, oh, i, n)
				strips = append(strips, strip{ly - labelHalf, ly + labelHalf})
			}
		}
	}
	n := len(cross[id])
	ok := func(y int) bool {
		for _, s := range strips {
			if y < s.hi && y+h > s.lo {
				return false
			}
		}
		return n == 0 || bandClear(left, cur.X, y, h, n)
	}
	extra := bandCandidates(left, cur.X, h, n, want)
	for _, s := range strips {
		extra = append(extra, s.hi, s.lo-h)
	}

	y, found := col.placeWhere(want, h, extra, ok)
	if !found || abs(y-want) >= abs(cur.Y-want) {
		return
	}
	out[id] = model.Point{X: cur.X, Y: y}
}
