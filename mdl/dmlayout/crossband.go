// SPDX-License-Identifier: Apache-2.0

package dmlayout

import (
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// CrossLines maps an entity of one module to the names of the cross-module
// associations Studio Pro draws at it in that module's diagram, sorted.
//
// Measured in Studio Pro (classic and 11.16 beta; PROPOSAL_domain_model_
// readable_layout.md, "Measurements"): a cross-module association is drawn
// from the LEFT side of each local end — the target too, not only the entity
// holding the reference — straight to the canvas's left edge (x = 0), and n of
// them on one entity sit at i/(n+1) of its height. The connection points are
// not stored and cannot be moved, so the only way to keep such a line out of
// other boxes is to leave its strip empty (mendixlabs/mxcli#1929, rule 2).
type CrossLines map[model.ID][]string

// labelHalf is half the height a cross-module line needs clear around it: the
// line plus its name label, which sits on it.
const labelHalf = 10

// CrossLinesFor collects the cross-module lines drawn in module's diagram: the
// FROM entities of dm's own cross-module associations, and the entities of dm
// that another module's cross-module association names as its TO end
// ("Module.Entity", compared case-insensitively like every Mendix name). all
// is every domain model of the project; dm may be among them.
func CrossLinesFor(module string, dm *domainmodel.DomainModel, all []*domainmodel.DomainModel) CrossLines {
	out := CrossLines{}
	if dm == nil {
		return out
	}
	byName := map[string]model.ID{}
	present := map[model.ID]bool{}
	for _, e := range dm.Entities {
		if e != nil {
			byName[strings.ToLower(e.Name)] = e.ID
			present[e.ID] = true
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca != nil && present[ca.ParentID] {
			out[ca.ParentID] = append(out[ca.ParentID], ca.Name)
		}
	}
	prefix := strings.ToLower(module) + "."
	for _, other := range all {
		if other == nil {
			continue
		}
		for _, ca := range other.CrossAssociations {
			if ca == nil {
				continue
			}
			ref := strings.ToLower(ca.ChildRef)
			if !strings.HasPrefix(ref, prefix) {
				continue
			}
			if id, ok := byName[strings.TrimPrefix(ref, prefix)]; ok {
				out[id] = append(out[id], ca.Name)
			}
		}
	}
	for id := range out {
		sort.Strings(out[id])
	}
	return out
}

// EntityBoxAt is the estimated box e occupies at p.
func EntityBoxAt(e *domainmodel.Entity, p model.Point) Rect {
	w, h := boxSize(e)
	return Rect{X: p.X, Y: p.Y, W: w, H: h}
}

// lineY is the y of the i-th of n cross-module lines on a box at top y, height h.
func lineY(y, h, i, n int) int {
	return y + (i+1)*h/(n+1)
}

// bandClear reports whether n cross-module lines on a box at (x, y) of height h
// miss every box in placed that lies to its left.
func bandClear(placed []Rect, x, y, h, n int) bool {
	for i := 0; i < n; i++ {
		ly := lineY(y, h, i, n)
		for _, r := range placed {
			if r.X < x && r.Y < ly+labelHalf && r.Y+r.H > ly-labelHalf {
				return false
			}
		}
	}
	return true
}

// bandCandidates are the y values at which one of the n lines just clears a
// box to the left: above or below each box, for each line. With want and the
// column's own free spots, the first that passes bandClear wins.
func bandCandidates(placed []Rect, x, h, n, want int) []int {
	var out []int
	for _, r := range placed {
		if r.X >= x {
			continue
		}
		for i := 0; i < n; i++ {
			off := (i + 1) * h / (n + 1)
			for _, y := range []int{r.Y + r.H + labelHalf - off, r.Y - labelHalf - off} {
				if y >= want {
					out = append(out, y)
				}
			}
		}
	}
	return out
}

// belowLeft is the first y at which every line of a box at x clears every box
// to its left by running underneath it: always a valid band.
func belowLeft(placed []Rect, x int) int {
	y := OriginY
	for _, r := range placed {
		if r.X < x {
			y = max(y, r.Y+r.H+labelHalf)
		}
	}
	return y
}
