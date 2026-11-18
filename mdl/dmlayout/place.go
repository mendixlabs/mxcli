// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// clearance is the minimum gap kept between two boxes when placing or repairing.
// Touching boxes are as unreadable as overlapping ones, and every size here is an
// estimate, so the margin absorbs some of the error.
const clearance = 20

// Rect is an estimated box on the domain-model canvas, top-left anchored.
type Rect struct {
	X, Y, W, H int
}

// TooClose reports whether two boxes come closer than clearance. This is the
// predicate for PLACING: touching boxes are as unreadable as overlapping ones,
// so a placer separates them.
//
// It is deliberately NOT the predicate for checking a stored model. Two boxes
// 10px apart satisfy TooClose and do not overlap, so a lint rule that used this
// would report an overlap that is not there — see Intersects.
func (r Rect) TooClose(o Rect) bool {
	return r.X < o.X+o.W+clearance && o.X < r.X+r.W+clearance &&
		r.Y < o.Y+o.H+clearance && o.Y < r.Y+r.H+clearance
}

// Intersects reports whether two boxes actually cover each other, with no
// clearance margin. This is the predicate for CHECKING a model that already
// exists (linter rule MPR016): a finding has to be true, and "closer than I
// would have placed them" is not an overlap.
//
// Both boxes are still estimates (see the package comment), so a finding of a
// few pixels in one axis may not be visible in Studio Pro. The estimates are
// generous rather than tight, which errs towards reporting; MPR016 therefore
// reports a cluster rather than a pixel count, and the repair is idempotent.
func (r Rect) Intersects(o Rect) bool {
	return r.X < o.X+o.W && o.X < r.X+r.W &&
		r.Y < o.Y+o.H && o.Y < r.Y+r.H
}

// EntityBox is the estimated box an entity occupies at its stored Location.
func EntityBox(e *domainmodel.Entity) Rect {
	w, h := boxSize(e)
	return Rect{X: e.Location.X, Y: e.Location.Y, W: w, H: h}
}

// Annotation sizing. Studio Pro stores an annotation's Width but not its height,
// which grows with the caption; estimate it the way boxSize does for entities.
const (
	defaultAnnotationWidth = 200
	annotationLineHeight   = 16
	annotationPadding      = 20
)

// AnnotationBox is the estimated box an annotation occupies. Annotations are
// obstacles only: nothing here ever moves one.
func AnnotationBox(a *domainmodel.Annotation) Rect {
	w := a.Width
	if w <= 0 {
		w = defaultAnnotationWidth
	}
	charsPerLine := (w - annotationPadding) / pixelsPerChar
	if charsPerLine < 1 {
		charsPerLine = 1
	}
	lines := 0
	for _, l := range strings.Split(a.Caption, "\n") {
		lines += 1 + len(l)/charsPerLine
	}
	return Rect{X: a.Location.X, Y: a.Location.Y, W: w, H: lines*annotationLineHeight + annotationPadding}
}

// obstacles is every box in the model except the entity being placed.
func obstacles(dm *domainmodel.DomainModel, skip *domainmodel.Entity) []Rect {
	var out []Rect
	if dm == nil {
		return out
	}
	for _, e := range dm.Entities {
		if e != nil && e != skip && (skip == nil || e.ID == "" || e.ID != skip.ID) {
			out = append(out, EntityBox(e))
		}
	}
	for _, a := range dm.Annotations {
		if a != nil {
			out = append(out, AnnotationBox(a))
		}
	}
	return out
}

func clear(r Rect, obs []Rect) bool {
	for _, o := range obs {
		if r.TooClose(o) {
			return false
		}
	}
	return true
}

// maxSlotSearch bounds Place's walk down the grid. It is far past any real
// module; reaching it means the estimates are nonsense, and the fallback slot
// is still a valid position.
const maxSlotSearch = 10000

// Place returns the position for a new entity created without an explicit
// @Position: the first grid slot, starting from the model's entity count, where
// e's estimated box clears every entity and annotation already in dm.
//
// Starting from the count keeps the common case identical to GridSlot — a script
// of small entities lands on the same slots it always did. The search is what
// GridSlot alone could not do: a slot is free only if the box FITS there, so a
// 20-attribute entity above pushes the next one down a row instead of under it
// (mendixlabs/mxcli#1479).
//
// e need not be in dm; if it is (matched by pointer or ID) it is not its own
// obstacle. Its Location is ignored.
func Place(dm *domainmodel.DomainModel, e *domainmodel.Entity) model.Point {
	w, h := boxSize(e)
	obs := obstacles(dm, e)
	start := 0
	if dm != nil {
		start = len(dm.Entities)
	}
	for n := start; n < start+maxSlotSearch; n++ {
		p := GridSlot(n)
		if clear(Rect{X: p.X, Y: p.Y, W: w, H: h}, obs) {
			return p
		}
	}
	return GridSlot(start)
}

// PlaceNear returns want if e's box fits there, else the nearest free position
// to it. For an entity that belongs beside another one — an OData primitive
// collection's helper beside its parent — where a grid slot would scatter it.
func PlaceNear(dm *domainmodel.DomainModel, e *domainmodel.Entity, want model.Point) model.Point {
	w, h := boxSize(e)
	r := Rect{X: want.X, Y: want.Y, W: w, H: h}
	obs := obstacles(dm, e)
	if clear(r, obs) {
		return want
	}
	return nearestFree(r, obs)
}

// ResolveOverlaps moves only the entities whose boxes collide, and returns their
// new positions; an entity absent from the result keeps its Location.
//
// The entities are settled in reading order — top to bottom, then left to right,
// then by name — and the first of any colliding group stays put: it is the one
// most likely to have been placed on purpose, and moving the later one is the
// smallest change that separates them. A moved entity goes to the nearest free
// position to where it was, so a hand-arranged diagram keeps its shape.
// Annotations are obstacles and never move.
//
// Deterministic for a given model, for the same reason Plan is.
func ResolveOverlaps(dm *domainmodel.DomainModel) map[model.ID]model.Point {
	out := map[model.ID]model.Point{}
	if dm == nil {
		return out
	}
	var ents []*domainmodel.Entity
	for _, e := range dm.Entities {
		if e != nil {
			ents = append(ents, e)
		}
	}
	sort.SliceStable(ents, func(i, j int) bool {
		a, b := ents[i].Location, ents[j].Location
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		if a.X != b.X {
			return a.X < b.X
		}
		return ents[i].Name < ents[j].Name
	})

	var settled []Rect
	for _, a := range dm.Annotations {
		if a != nil {
			settled = append(settled, AnnotationBox(a))
		}
	}
	for _, e := range ents {
		r := EntityBox(e)
		if !clear(r, settled) {
			p := nearestFree(r, settled)
			out[e.ID] = p
			r.X, r.Y = p.X, p.Y
		}
		settled = append(settled, r)
	}
	return out
}

// searchStep is the granularity of the nearest-free-position search, and
// maxRing bounds it (maxRing*searchStep px in every direction — far past any
// real diagram, since below and right of everything is always free).
const (
	searchStep = 20
	maxRing    = 1000
)

// nearestFree finds the free position closest to r's current one, searching
// rings of growing radius. Within a ring, candidates are taken in order of
// distance, then down before right before up before left — a stable order, and
// the directions a reader expects a displaced box to have gone.
func nearestFree(r Rect, obs []Rect) model.Point {
	type cand struct{ dx, dy, d2 int }
	for ring := 1; ring <= maxRing; ring++ {
		var cs []cand
		for dx := -ring; dx <= ring; dx++ {
			for dy := -ring; dy <= ring; dy++ {
				if max(abs(dx), abs(dy)) != ring {
					continue
				}
				cs = append(cs, cand{dx, dy, dx*dx + dy*dy})
			}
		}
		sort.Slice(cs, func(i, j int) bool {
			if cs[i].d2 != cs[j].d2 {
				return cs[i].d2 < cs[j].d2
			}
			if cs[i].dy != cs[j].dy {
				return cs[i].dy > cs[j].dy
			}
			return cs[i].dx > cs[j].dx
		})
		for _, c := range cs {
			x, y := r.X+c.dx*searchStep, r.Y+c.dy*searchStep
			if x < 0 || y < 0 {
				continue
			}
			if clear(Rect{X: x, Y: y, W: r.W, H: r.H}, obs) {
				return model.Point{X: x, Y: y}
			}
		}
	}
	return model.Point{X: r.X, Y: r.Y}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
