// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"math"
	"sort"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// AnchorPair is where an association's line attaches to its two boxes: Parent
// on the FROM entity (the one holding the reference, stored as
// ParentConnection) and Child on the TO entity (ChildConnection). Each is a
// whole-number percentage of its box, x 0 = left … 100 = right, y 0 = top …
// 100 = bottom, and always on the box's edge.
type AnchorPair struct {
	Parent, Child model.Point
}

const (
	// headerAnchorPx is how far below a box's top a straight line attaches when
	// the two boxes share their top rows: inside the name bar, which is the same
	// height on every box, so an error in the estimated height moves it by
	// rounding at most.
	headerAnchorPx = 20
	// minBand is the shared span two boxes need for a straight line between
	// them: less, and the line would graze a corner.
	minBand = 24
	// parallelGap separates the straight lines of several associations between
	// the same two entities, enough for each to keep its own label.
	parallelGap = 20
	// spreadClearance keeps a spread line this many percent from a straight one
	// on the same side, so the two do not meet at one point.
	spreadClearance = 5
)

type side int

const (
	sideLeft side = iota
	sideRight
	sideTop
	sideBottom
)

type anchorBox struct {
	x, y, w, h int
}

func (b anchorBox) cx() float64 { return float64(b.x) + float64(b.w)/2 }
func (b anchorBox) cy() float64 { return float64(b.y) + float64(b.h)/2 }

// end is one end of one association on one box.
type end struct {
	assoc    model.ID
	parent   bool // the FROM end
	entity   model.ID
	side     side
	fixed    bool    // a straight line's end, already placed
	pct      int     // along the side, when fixed
	farOrder float64 // the far end's position along this side's axis, to sort by
	name     string  // association name, the tie-break
}

// Anchors computes the connection points of every association between two
// different entities of dm at the positions in pos (mendixlabs/mxcli#1929,
// rules 4 and 5):
//
//   - Each end attaches on the side of its box that faces the other box: top and
//     bottom when the boxes overlap horizontally, left and right otherwise.
//   - Where the two boxes share a span along that side, both ends sit at the
//     same point of it, so the line is straight. Boxes sharing their top rows
//     connect in the header, which keeps the line straight whatever their
//     heights; otherwise the middle of the shared span.
//   - The other lines are spread evenly along their side, in the order of their
//     far ends, so no two meet at one point and none cross at the box.
//
// Self-associations are not returned (their loop is the author's to place), nor
// associations whose ends are not both in pos. Studio Pro stores no anchors for
// a generalization, so there is nothing to compute for one.
//
// Deterministic for a given model and positions.
func Anchors(dm *domainmodel.DomainModel, pos map[model.ID]model.Point) map[model.ID]AnchorPair {
	out := map[model.ID]AnchorPair{}
	if dm == nil {
		return out
	}
	boxes := map[model.ID]anchorBox{}
	for _, e := range dm.Entities {
		if e == nil {
			continue
		}
		p, ok := pos[e.ID]
		if !ok {
			continue
		}
		w, h := boxSize(e)
		boxes[e.ID] = anchorBox{p.X, p.Y, w, h}
	}

	nth := parallelIndex(dm.Associations)
	var all []anchorBox
	for _, e := range dm.Entities {
		if e != nil {
			if b, ok := boxes[e.ID]; ok {
				all = append(all, b)
			}
		}
	}

	var ends []*end
	for _, a := range dm.Associations {
		if a == nil || a.ID == "" || a.ParentID == a.ChildID {
			continue
		}
		pb, okP := boxes[a.ParentID]
		cb, okC := boxes[a.ChildID]
		if !okP || !okC {
			continue
		}
		pe := &end{assoc: a.ID, parent: true, entity: a.ParentID, name: a.Name}
		ce := &end{assoc: a.ID, entity: a.ChildID, name: a.Name}
		connect(pe, ce, pb, cb, nth[a.ID], all)
		ends = append(ends, pe, ce)
	}

	spread(ends)

	for _, e := range ends {
		pt := pointOn(e.side, e.pct)
		pair := out[e.assoc]
		if e.parent {
			pair.Parent = pt
		} else {
			pair.Child = pt
		}
		out[e.assoc] = pair
	}
	return out
}

// connect picks the facing sides of a and b, and places both ends when a
// straight line fits between them.
//
// k is the association's index among those joining the same two entities: the
// k-th straight line runs parallelGap*k further along the shared span, so two
// associations between one pair are two lines, not one.
//
// A straight line is only drawn where it passes no other box (all holds every
// box, a and b included): a line between columns that are not adjacent would
// otherwise run straight through the boxes between them.
func connect(ea, eb *end, a, b anchorBox, k int, all []anchorBox) {
	xOverlap := a.x < b.x+b.w && b.x < a.x+a.w
	if xOverlap {
		// One above the other.
		if a.cy() <= b.cy() {
			ea.side, eb.side = sideBottom, sideTop
		} else {
			ea.side, eb.side = sideTop, sideBottom
		}
		ea.farOrder, eb.farOrder = b.cx(), a.cx()
		lo, hi := max(a.x, b.x), min(a.x+a.w, b.x+b.w)
		at := float64(lo+hi)/2 + float64(k*parallelGap)
		gapLo, gapHi := min(a.y+a.h, b.y+b.h), max(a.y, b.y)
		if hi-lo >= minBand && at <= float64(hi-minBand/2) &&
			clearPath(all, a, b, at, float64(gapLo), float64(gapHi), true) {
			straight(ea, eb, a.x, a.w, b.x, b.w, at)
		}
		return
	}
	if a.cx() <= b.cx() {
		ea.side, eb.side = sideRight, sideLeft
	} else {
		ea.side, eb.side = sideLeft, sideRight
	}
	ea.farOrder, eb.farOrder = b.cy(), a.cy()
	lo, hi := max(a.y, b.y), min(a.y+a.h, b.y+b.h)
	if hi-lo < minBand {
		return
	}
	at := float64(lo + headerAnchorPx + k*parallelGap)
	if at > float64(hi-minBand/2) {
		at = float64(lo+hi)/2 + float64(k*parallelGap)
	}
	if at > float64(hi-minBand/2) {
		return // no room for another straight line here: spread it instead
	}
	gapLo, gapHi := min(a.x+a.w, b.x+b.w), max(a.x, b.x)
	if !clearPath(all, a, b, at, float64(gapLo), float64(gapHi), false) {
		return
	}
	straight(ea, eb, a.y, a.h, b.y, b.h, at)
}

// clearPath reports whether the straight segment at coordinate at, from from to
// to along the other axis, misses every box but a and b. vertical says the
// segment runs up and down (at is an x); otherwise it runs across (at is a y).
func clearPath(all []anchorBox, a, b anchorBox, at, from, to float64, vertical bool) bool {
	for _, c := range all {
		if c == a || c == b {
			continue
		}
		var cLo, cHi, sLo, sHi float64
		if vertical {
			cLo, cHi = float64(c.x), float64(c.x+c.w)
			sLo, sHi = float64(c.y), float64(c.y+c.h)
		} else {
			cLo, cHi = float64(c.y), float64(c.y+c.h)
			sLo, sHi = float64(c.x), float64(c.x+c.w)
		}
		if at >= cLo && at <= cHi && sLo < to && sHi > from {
			return false
		}
	}
	return true
}

// parallelIndex numbers the associations joining the same two entities, in
// name order (either direction counts as the same pair): 0 for the first, 1
// for the second, and so on.
func parallelIndex(assocs []*domainmodel.Association) map[model.ID]int {
	type pair struct{ a, b model.ID }
	byPair := map[pair][]*domainmodel.Association{}
	for _, a := range assocs {
		if a == nil || a.ID == "" || a.ParentID == a.ChildID {
			continue
		}
		k := pair{a.ParentID, a.ChildID}
		if k.b < k.a {
			k.a, k.b = k.b, k.a
		}
		byPair[k] = append(byPair[k], a)
	}
	out := map[model.ID]int{}
	for _, list := range byPair {
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Name != list[j].Name {
				return list[i].Name < list[j].Name
			}
			return list[i].ID < list[j].ID
		})
		for i, a := range list {
			out[a.ID] = i
		}
	}
	return out
}

// straight places both ends at the same absolute coordinate, at, along their
// sides (start and length of each box along that axis). The coordinate is
// first moved onto the LONGER box's whole-percent grid: a percent of an 856-px
// box is 8.6 px, so rounding there would tilt the line visibly, while rounding
// the shorter box's percent is worth half a pixel or so.
func straight(ea, eb *end, aStart, aLen, bStart, bLen int, at float64) {
	long, short := ea, eb
	lStart, lLen, sStart, sLen := aStart, aLen, bStart, bLen
	if bLen > aLen {
		long, short = eb, ea
		lStart, lLen, sStart, sLen = bStart, bLen, aStart, aLen
	}
	lp := clampPct(math.Round((at - float64(lStart)) * 100 / float64(lLen)))
	exact := float64(lStart) + float64(lp)*float64(lLen)/100
	sp := clampPct(math.Round((exact - float64(sStart)) * 100 / float64(sLen)))
	long.pct, long.fixed = lp, true
	short.pct, short.fixed = sp, true
}

// spread places the ends that are not part of a straight line, per box side,
// evenly along the side in the order of their far ends, keeping clear of the
// straight lines' ends on the same side.
func spread(ends []*end) {
	type key struct {
		entity model.ID
		side   side
	}
	groups := map[key][]*end{}
	var keys []key
	for _, e := range ends {
		k := key{e.entity, e.side}
		if _, seen := groups[k]; !seen {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], e)
	}
	for _, k := range keys {
		var fixed []int
		var free []*end
		for _, e := range groups[k] {
			if e.fixed {
				fixed = append(fixed, e.pct)
			} else {
				free = append(free, e)
			}
		}
		sort.SliceStable(free, func(i, j int) bool {
			if free[i].farOrder != free[j].farOrder {
				return free[i].farOrder < free[j].farOrder
			}
			return free[i].name < free[j].name
		})
		n := len(free)
		for i, e := range free {
			e.pct = clearOf((i+1)*100/(n+1), fixed)
		}
	}
}

// clearOf moves p away from any fixed point closer than spreadClearance,
// towards the middle of the side.
func clearOf(p int, fixed []int) int {
	for _, f := range fixed {
		if d := p - f; d > -spreadClearance && d < spreadClearance {
			if f < 50 {
				p = f + spreadClearance
			} else {
				p = f - spreadClearance
			}
		}
	}
	return clampPct(float64(p))
}

// clampPct keeps a point off the box's corners, where a line would read as
// belonging to the neighbouring side.
func clampPct(p float64) int {
	return int(math.Max(1, math.Min(99, p)))
}

func pointOn(s side, pct int) model.Point {
	switch s {
	case sideLeft:
		return model.Point{X: 0, Y: pct}
	case sideRight:
		return model.Point{X: 100, Y: pct}
	case sideTop:
		return model.Point{X: pct, Y: 0}
	default:
		return model.Point{X: pct, Y: 100}
	}
}
