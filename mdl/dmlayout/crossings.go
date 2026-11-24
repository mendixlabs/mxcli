// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"sort"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Line is one line Studio Pro draws on a domain model diagram, as a straight
// segment between two canvas points.
type Line struct {
	Name   string     // the association's name; empty for a generalization
	Kind   string     // "association", "generalization" or "cross-module"
	Ends   []model.ID // the entities the line attaches to, in this module
	X1, Y1 float64
	X2, Y2 float64
}

// StoredAnchors is each association's connection points as stored, or Studio
// Pro's default where nothing readable is (FROM end at 0;50, TO end at 100;50).
func StoredAnchors(dm *domainmodel.DomainModel) map[model.ID]AnchorPair {
	out := map[model.ID]AnchorPair{}
	if dm == nil {
		return out
	}
	defP := *domainmodel.ParseConnectionPoint(domainmodel.DefaultParentConnection)
	defC := *domainmodel.ParseConnectionPoint(domainmodel.DefaultChildConnection)
	for _, a := range dm.Associations {
		if a == nil {
			continue
		}
		pair := AnchorPair{Parent: defP, Child: defC}
		if a.ParentConnection != nil {
			pair.Parent = *a.ParentConnection
		}
		if a.ChildConnection != nil {
			pair.Child = *a.ChildConnection
		}
		out[a.ID] = pair
	}
	return out
}

// Lines lists every line of dm's diagram at the positions in pos: each
// association between two placed entities (self-associations are loops and
// are left out) from its anchors; each generalization, centre to centre, the
// way the arrow runs; and each cross-module line in cross, from the left side
// of its entity at i/(n+1) of its height to the canvas edge.
func Lines(dm *domainmodel.DomainModel, pos map[model.ID]model.Point, anchors map[model.ID]AnchorPair, cross CrossLines) []Line {
	var out []Line
	if dm == nil {
		return out
	}
	boxes := map[model.ID]Rect{}
	for _, e := range dm.Entities {
		if e == nil {
			continue
		}
		if p, ok := pos[e.ID]; ok {
			boxes[e.ID] = EntityBoxAt(e, p)
		}
	}
	at := func(r Rect, p model.Point) (float64, float64) {
		return float64(r.X) + float64(p.X)*float64(r.W)/100, float64(r.Y) + float64(p.Y)*float64(r.H)/100
	}
	for _, a := range dm.Associations {
		if a == nil || a.ParentID == a.ChildID {
			continue
		}
		pb, okP := boxes[a.ParentID]
		cb, okC := boxes[a.ChildID]
		pair, okA := anchors[a.ID]
		if !okP || !okC || !okA {
			continue
		}
		x1, y1 := at(pb, pair.Parent)
		x2, y2 := at(cb, pair.Child)
		out = append(out, Line{Name: a.Name, Kind: "association", Ends: []model.ID{a.ParentID, a.ChildID}, X1: x1, Y1: y1, X2: x2, Y2: y2})
	}
	for _, e := range dm.Entities {
		if e == nil || e.GeneralizationID == "" {
			continue
		}
		sb, ok1 := boxes[e.ID]
		bb, ok2 := boxes[e.GeneralizationID]
		if !ok1 || !ok2 || e.ID == e.GeneralizationID {
			continue
		}
		x1, y1 := at(sb, model.Point{X: 50, Y: 50})
		x2, y2 := at(bb, model.Point{X: 50, Y: 50})
		out = append(out, Line{Kind: "generalization", Ends: []model.ID{e.ID, e.GeneralizationID}, X1: x1, Y1: y1, X2: x2, Y2: y2})
	}
	var ids []model.ID
	for id := range cross {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		b, ok := boxes[id]
		if !ok {
			continue
		}
		names := cross[id]
		for i, n := range names {
			y := float64(lineY(b.Y, b.H, i, len(names)))
			out = append(out, Line{Name: n, Kind: "cross-module", Ends: []model.ID{id}, X1: float64(b.X), Y1: y, X2: 0, Y2: y})
		}
	}
	return out
}

// Through lists the entities whose boxes l passes through, other than its own
// ends, in dm's order. A line along an edge or touching a corner does not count.
func (l Line) Through(dm *domainmodel.DomainModel, pos map[model.ID]model.Point) []*domainmodel.Entity {
	return l.ThroughInset(dm, pos, 1)
}

// ThroughInset is Through against each box shrunk by inset pixels on every
// side: a check that reports a finding uses a few pixels, because the box sizes
// are estimates and a line grazing an estimated edge may clear the real one.
func (l Line) ThroughInset(dm *domainmodel.DomainModel, pos map[model.ID]model.Point, inset int) []*domainmodel.Entity {
	var out []*domainmodel.Entity
	for _, e := range dm.Entities {
		if e == nil || l.hasEnd(e.ID) {
			continue
		}
		p, ok := pos[e.ID]
		if !ok {
			continue
		}
		r := EntityBoxAt(e, p)
		if segmentHitsRect(l, Rect{X: r.X + inset - 1, Y: r.Y + inset - 1, W: r.W - 2*(inset-1), H: r.H - 2*(inset-1)}) {
			out = append(out, e)
		}
	}
	return out
}

// Crossings counts, for dm at pos with the given anchors and cross-module
// lines, the pairs of lines that cross (lines sharing an entity meet at its
// box, which is not a crossing) and the times a line runs through a box that
// is not one of its ends. They are what #1929's first rule asks to minimise.
func Crossings(dm *domainmodel.DomainModel, pos map[model.ID]model.Point, anchors map[model.ID]AnchorPair, cross CrossLines) (lines, boxes int) {
	ls := Lines(dm, pos, anchors, cross)
	for i := range ls {
		boxes += len(ls[i].Through(dm, pos))
		for j := i + 1; j < len(ls); j++ {
			if !ls[i].sharesEnd(ls[j]) && segmentsCross(ls[i], ls[j]) {
				lines++
			}
		}
	}
	return lines, boxes
}

func (l Line) hasEnd(id model.ID) bool {
	for _, e := range l.Ends {
		if e == id {
			return true
		}
	}
	return false
}

func (l Line) sharesEnd(o Line) bool {
	for _, e := range o.Ends {
		if l.hasEnd(e) {
			return true
		}
	}
	return false
}

// segmentsCross reports a proper crossing: each segment's ends strictly on
// opposite sides of the other. Touching and collinear overlap are not counted.
func segmentsCross(a, b Line) bool {
	d1 := orient(b.X1, b.Y1, b.X2, b.Y2, a.X1, a.Y1)
	d2 := orient(b.X1, b.Y1, b.X2, b.Y2, a.X2, a.Y2)
	d3 := orient(a.X1, a.Y1, a.X2, a.Y2, b.X1, b.Y1)
	d4 := orient(a.X1, a.Y1, a.X2, a.Y2, b.X2, b.Y2)
	return d1*d2 < 0 && d3*d4 < 0
}

func orient(ax, ay, bx, by, cx, cy float64) float64 {
	v := (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
	switch {
	case v > 1e-9:
		return 1
	case v < -1e-9:
		return -1
	}
	return 0
}

// segmentHitsRect reports whether the segment passes through the inside of r
// (shrunk by a pixel, so a line along an edge or touching a corner does not
// count): Liang–Barsky clipping.
func segmentHitsRect(l Line, r Rect) bool {
	xmin, ymin := float64(r.X)+1, float64(r.Y)+1
	xmax, ymax := float64(r.X+r.W)-1, float64(r.Y+r.H)-1
	dx, dy := l.X2-l.X1, l.Y2-l.Y1
	t0, t1 := 0.0, 1.0
	for _, c := range [4][2]float64{
		{-dx, l.X1 - xmin}, {dx, xmax - l.X1},
		{-dy, l.Y1 - ymin}, {dy, ymax - l.Y1},
	} {
		p, q := c[0], c[1]
		if p == 0 {
			if q < 0 {
				return false
			}
			continue
		}
		t := q / p
		if p < 0 {
			if t > t1 {
				return false
			}
			t0 = max(t0, t)
		} else {
			if t < t0 {
				return false
			}
			t1 = min(t1, t)
		}
	}
	return t0 < t1
}
