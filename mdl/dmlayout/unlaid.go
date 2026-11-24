// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"sort"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// NotLaidOut returns the entities that still sit where mxcli's auto-placement
// put them, and that `mxcli layout` would place differently: the trace of a
// script that created linked entities and never ran the layout.
//
// # The signature
//
// Every create path that is given no @Position calls Place (or PlaceNear), and
// Place only ever returns a GridSlot. Studio Pro positions are wherever someone
// dropped the box, so an exact grid slot is, in practice, mxcli's placement and
// not a human's. Three conditions narrow that to "a diagram nobody arranged":
//
//   - Slot 0 is excluded. It is (OriginX, OriginY), which is also where Plan
//     puts its first entity, so it cannot tell a placed model from a laid-out
//     one. Place starts at the model's entity count, so only the first entity of
//     an empty module ever lands there anyway.
//   - The entity has a link to another entity in the same module: an
//     association or a generalization, the edges Plan lays out by. An unlinked
//     helper has nothing to be laid out relative to, and Plan only moves it to a
//     band underneath, which reads no better than a grid.
//   - Plan does not put it exactly there. Plan's coordinates are sums of box
//     sizes and gutters and can, rarely, coincide with a slot; an entity Plan
//     itself placed must never be reported, or the command the finding names
//     would not clear it.
//
// The result is sorted by name, so a finding does not move between runs.
func NotLaidOut(dm *domainmodel.DomainModel) []*domainmodel.Entity {
	return NotLaidOutWith(dm, nil)
}

// NotLaidOutWith is NotLaidOut against the layout `mxcli layout` actually
// makes for a module with cross-module lines (PlanWith): an entity moved for
// the band its cross-module line needs is laid out, not unplaced, even if the
// move happens to land it on a grid slot.
func NotLaidOutWith(dm *domainmodel.DomainModel, cross CrossLines) []*domainmodel.Entity {
	if dm == nil {
		return nil
	}
	linked := linkedEntities(dm)

	var candidates []*domainmodel.Entity
	for _, e := range dm.Entities {
		if e == nil || !linked[e.ID] || e.Location == GridSlot(0) || !onGridSlot(e.Location) {
			continue
		}
		candidates = append(candidates, e)
	}
	if len(candidates) == 0 {
		return nil
	}

	plan := PlanWith(dm, cross)
	var out []*domainmodel.Entity
	for _, e := range candidates {
		if p, ok := plan[e.ID]; ok && p == e.Location {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// onGridSlot reports whether p is exactly one of GridSlot's positions.
func onGridSlot(p model.Point) bool {
	dx, dy := p.X-OriginX, p.Y-OriginY
	if dx < 0 || dy < 0 || dx%GridColumnPitch != 0 || dy%GridRowPitch != 0 {
		return false
	}
	return dx/GridColumnPitch < GridColumns
}

// linkedEntities is the set of entities with at least one association or
// generalization to another entity in the same domain model: the edges
// newGraph builds Plan from, so "linked" here means "Plan places it by its
// relationships" there.
func linkedEntities(dm *domainmodel.DomainModel) map[model.ID]bool {
	present := make(map[model.ID]bool, len(dm.Entities))
	for _, e := range dm.Entities {
		if e != nil {
			present[e.ID] = true
		}
	}
	out := map[model.ID]bool{}
	link := func(a, b model.ID) {
		if a == b || !present[a] || !present[b] {
			return
		}
		out[a], out[b] = true, true
	}
	for _, a := range dm.Associations {
		if a != nil {
			link(a.ParentID, a.ChildID)
		}
	}
	for _, e := range dm.Entities {
		if e != nil && e.GeneralizationID != "" {
			link(e.ID, e.GeneralizationID)
		}
	}
	return out
}
