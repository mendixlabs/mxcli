// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// The default grid is a fixed pitch, and a box is as tall as its attribute list:
// from 16 attributes up an entity is taller than GridRowPitch and runs into the
// one below it. Plan fixes that by laying the whole module out again, which also
// throws away every position somebody chose. ResolveOverlaps is the narrow
// repair: it moves the entities whose boxes collide and nothing else.
//
// # Strategy
//
// Boxes are the estimates boxSize gives (the same ones Plan uses), hanging from
// the entity's Location, which is the box's top-left corner, the convention this
// package states. A collision is a strict intersection: boxes that merely touch,
// or sit close, are left alone.
//
// Which entity gives way. Entities are taken in the order they appear in the
// domain model, which is creation order. An entity stays if its box clears every
// entity that has already been kept and every annotation; otherwise it is a
// mover. So of two colliding entities the earlier one stays (it is the one more
// likely to have been placed on purpose), a chain of collisions costs the fewest
// moves a greedy pass can manage, and an entity nothing collides with is always
// kept, because there is nothing it could fail against. Annotations are notes the
// author put on the canvas on purpose: they are obstacles, never movers.
//
// Where a mover goes. Each mover, in the same order, is put on the nearest free
// point to where it was. "Free" means its box, grown by placementGutter on every
// side, intersects no kept entity, no annotation and no mover placed before it.
// The search walks outwards from the old position in square rings of
// placementStep pixels and takes, within the first ring that has a free point,
// the one closest to the old position (ties: smaller y, then smaller x). Nothing
// in it depends on map order, so the same model always gives the same moves.
//
// A mover is never placed left of or above the canvas origin, where the
// entities of a generated model start.
//
// Idempotence follows from the two rules: kept entities clear each other by
// construction and every mover clears everything placed before it with a margin,
// so the result has no collision, and a model with no collision has no mover.
//
// # Annotations
//
// An annotation stores a position and a width but no height or origin convention
// this package can rely on, so its obstacle is a deliberately generous estimate:
// the width as stored (defaultAnnotationWidth when absent), a height from the
// caption's line count, and a rectangle that covers the box whether Location is
// its centre or its top-left corner.

// placementGutter is the clear space a relocated entity keeps to every other
// box. It is the gutter Plan keeps between stacked entities.
const placementGutter = rowGutter

const (
	// placementStep is the search lattice, in pixels.
	placementStep = 10

	defaultAnnotationWidth = 440 // what CREATE ANNOTATION stores when no width is given
)

// Overlap is one pair of colliding boxes found in a domain model.
type Overlap struct {
	// Entity is the name of the entity that gives way.
	Entity string
	// With is what it collides with: another entity's name, or the first line of
	// an annotation's caption when WithAnnotation is set.
	With           string
	WithAnnotation bool
}

// Move is one entity that ResolveOverlaps relocates.
type Move struct {
	ID       model.ID
	Name     string
	From, To model.Point
}

// OverlapPlan is the result of ResolveOverlaps.
type OverlapPlan struct {
	// Pairs lists every collision in the model as it was handed in. For two
	// entities, Entity is the later one in model order — the one that gives way
	// unless an earlier collision already moved the other.
	Pairs []Overlap
	// Moves lists the entities to relocate, in the order they were placed.
	Moves []Move
}

// rect is a half-open box: x1 <= x < x2, y1 <= y < y2.
type rect struct{ x1, y1, x2, y2 int }

// intersects reports whether the two boxes share area once a is grown by pad on
// every side. Boxes that only touch do not intersect.
func (a rect) intersects(b rect, pad int) bool {
	return a.x1-pad < b.x2 && b.x1 < a.x2+pad && a.y1-pad < b.y2 && b.y1 < a.y2+pad
}

// entityRect is the estimated box of e when its Location is at.
func entityRect(e *domainmodel.Entity, at model.Point) rect {
	w, h := boxSize(e)
	return rect{at.X, at.Y, at.X + w, at.Y + h}
}

// annotationRect is the generous obstacle for a note; see "Annotations" above.
func annotationRect(a *domainmodel.Annotation) rect {
	w := a.Width
	if w <= 0 {
		w = defaultAnnotationWidth
	}
	perLine := (w - 2*boxPadding) / pixelsPerChar
	if perLine < 1 {
		perLine = 1
	}
	lines := 0
	for _, l := range strings.Split(strings.ReplaceAll(a.Caption, "\r\n", "\n"), "\n") {
		lines += 1 + (len(l)-1)/perLine // an empty line still takes one
	}
	h := 2*boxPadding + lines*memberHeight
	if h < minBoxHeight {
		h = minBoxHeight
	}
	// Covers Location as the centre and Location as the top-left corner.
	return rect{a.Location.X - w/2, a.Location.Y - h/2, a.Location.X + w, a.Location.Y + h}
}

// annotationTitle is the first non-empty line of a caption, which is how a note
// is called everywhere else.
func annotationTitle(caption string) string {
	for _, l := range strings.Split(strings.ReplaceAll(caption, "\r\n", "\n"), "\n") {
		if t := strings.TrimSpace(l); t != "" {
			return t
		}
	}
	return ""
}

// ResolveOverlaps finds the entities whose estimated boxes collide and plans a
// new position for the ones that have to give way; see the strategy above. It
// does not modify dm.
func ResolveOverlaps(dm *domainmodel.DomainModel) OverlapPlan {
	var plan OverlapPlan
	if dm == nil {
		return plan
	}
	var ents []*domainmodel.Entity
	for _, e := range dm.Entities {
		if e != nil {
			ents = append(ents, e)
		}
	}
	var notes []*domainmodel.Annotation
	var noteRects []rect
	for _, a := range dm.Annotations {
		if a != nil {
			notes = append(notes, a)
			noteRects = append(noteRects, annotationRect(a))
		}
	}
	boxes := make([]rect, len(ents))
	for i, e := range ents {
		boxes[i] = entityRect(e, e.Location)
	}

	// Every collision, for reporting.
	for j := range ents {
		for i := 0; i < j; i++ {
			if boxes[i].intersects(boxes[j], 0) {
				plan.Pairs = append(plan.Pairs, Overlap{Entity: ents[j].Name, With: ents[i].Name})
			}
		}
		for k, n := range notes {
			if boxes[j].intersects(noteRects[k], 0) {
				plan.Pairs = append(plan.Pairs, Overlap{Entity: ents[j].Name, With: annotationTitle(n.Caption), WithAnnotation: true})
			}
		}
	}
	if len(plan.Pairs) == 0 {
		return plan
	}

	// Who gives way: greedy over model order, against what has been kept.
	obstacles := append([]rect(nil), noteRects...)
	var movers []int
	for i := range ents {
		clear := true
		for _, o := range obstacles {
			if boxes[i].intersects(o, 0) {
				clear = false
				break
			}
		}
		if clear {
			obstacles = append(obstacles, boxes[i])
		} else {
			movers = append(movers, i)
		}
	}

	// Where they go: nearest free point, each mover seeing those placed before it.
	for _, i := range movers {
		w, h := boxSize(ents[i])
		to := nearestFree(w, h, ents[i].Location, obstacles)
		obstacles = append(obstacles, entityRect(ents[i], to))
		plan.Moves = append(plan.Moves, Move{ID: ents[i].ID, Name: ents[i].Name, From: ents[i].Location, To: to})
	}
	return plan
}

// nearestFree returns the point closest to from at which a w x h box whose
// top-left corner is there, grown by placementGutter, intersects none of
// obstacles.
func nearestFree(w, h int, from model.Point, obstacles []rect) model.Point {
	free := func(p model.Point) bool {
		// A mover never goes left of or above the canvas origin.
		if p.X < 0 || p.Y < 0 {
			return false
		}
		b := rect{p.X, p.Y, p.X + w, p.Y + h}
		for _, o := range obstacles {
			if b.intersects(o, placementGutter) {
				return false
			}
		}
		return true
	}
	if free(from) {
		return from
	}
	start := model.Point{X: max(from.X, 0), Y: max(from.Y, 0)}

	// Past every obstacle there is always room, so the search is bounded: the
	// first ring that reaches beyond the rightmost edge has a free point.
	right := from.X
	for _, o := range obstacles {
		if o.x2 > right {
			right = o.x2
		}
	}
	maxRing := (right-from.X+w+h+2*placementGutter)/placementStep + 2

	type cand struct {
		p  model.Point
		d2 int
	}
	for r := 1; r <= maxRing; r++ {
		var ring []cand
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if abs(dx) != r && abs(dy) != r {
					continue
				}
				x, y := dx*placementStep, dy*placementStep
				ring = append(ring, cand{model.Point{X: from.X + x, Y: from.Y + y}, x*x + y*y})
			}
		}
		sort.Slice(ring, func(a, b int) bool {
			if ring[a].d2 != ring[b].d2 {
				return ring[a].d2 < ring[b].d2
			}
			if ring[a].p.Y != ring[b].p.Y {
				return ring[a].p.Y < ring[b].p.Y
			}
			return ring[a].p.X < ring[b].p.X
		})
		for _, c := range ring {
			if free(c.p) {
				return c.p
			}
		}
	}
	// Unreachable by the bound above; the point is clear of everything anyway.
	return model.Point{X: right + placementGutter + placementStep, Y: start.Y}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
