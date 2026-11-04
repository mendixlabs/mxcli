// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// activityBoxWidth and activityBoxHeight are the pixel dimensions of a Mendix
// microflow activity box, used for a node that stores no size of its own.
const activityBoxWidth = 120
const activityBoxHeight = 60

// eventSize is the side of an event's box (start, end, error, break, continue)
// for one that stores no size: Studio Pro stores 20x20 on every event it draws
// (all 712 in ako/TestApp and PedApp), and so does mxcli.
const eventSize = 20

// OverlappingActivitiesRule flags microflow activities whose canvas positions overlap.
//
// The most common cause is writing multiple MDL statements after a single @position
// annotation — e.g. a DECLARE followed immediately by a SET with no second @position.
// The executor auto-places the un-annotated statement only 150px to the right (less
// than one activity width from the next explicitly annotated activity), producing
// overlapping boxes in Studio Pro.
type OverlappingActivitiesRule struct{}

func NewOverlappingActivitiesRule() *OverlappingActivitiesRule {
	return &OverlappingActivitiesRule{}
}

func (r *OverlappingActivitiesRule) ID() string                       { return "MPR008" }
func (r *OverlappingActivitiesRule) Name() string                     { return "OverlappingActivities" }
func (r *OverlappingActivitiesRule) Category() string                 { return "correctness" }
func (r *OverlappingActivitiesRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }
func (r *OverlappingActivitiesRule) Description() string {
	return "Microflow activities whose canvas positions overlap, typically caused by missing @position annotations in MDL"
}

func (r *OverlappingActivitiesRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}

	var violations []linter.Violation

	for mf := range ctx.Microflows() {
		if ctx.IsExcluded(mf.ModuleName) {
			continue
		}

		fullMF, err := ctx.FullMicroflow(model.ID(mf.ID))
		if err != nil || fullMF == nil || fullMF.ObjectCollection == nil {
			continue
		}

		planes := overlapPlanes(fullMF.ObjectCollection.Objects)

		// Check all pairs for overlapping positions, WITHIN a plane.
		// Skip activities at the origin (0,0) — these are unpositioned/default.
		reported := make(map[string]bool)
		for _, activities := range planes {
			for i := 0; i < len(activities); i++ {
				for j := i + 1; j < len(activities); j++ {
					a, b := activities[i], activities[j]
					if (a.x == 0 && a.y == 0) || (b.x == 0 && b.y == 0) {
						continue
					}
					if boxesOverlap(a, b) {
						key := fmt.Sprintf("%d,%d|%d,%d", a.x, a.y, b.x, b.y)
						if reported[key] {
							continue
						}
						reported[key] = true
						msg, suggestion := overlapViolationText(a, b, mf.DocumentNoun(), mf.ModuleName, mf.Name)
						violations = append(violations, linter.Violation{
							RuleID:   r.ID(),
							Severity: r.DefaultSeverity(),
							Message:  msg,
							Location: linter.Location{
								Module:       mf.ModuleName,
								DocumentType: mf.DocumentNoun(),
								DocumentName: mf.Name,
								DocumentID:   mf.ID,
							},
							Suggestion: suggestion,
						})
					}
				}
			}
		}
	}

	return violations
}

// overlapViolationText words an MPR008 finding for one overlapping pair.
//
// When one box is a loop, the overlap is almost never two statements sharing one
// @position: a loop's box grows with its body, so a loop placed at an ordinary
// activity spacing reaches into the activity before or after it. Saying "each
// statement needs its own @position" there sent users adding annotations that
// were already present (ako/mxcli#1839, mendixlabs/mxcli#1690), so the message
// names the loop, its size, and the neighbour it covers instead.
func overlapViolationText(a, b actInfo, noun, module, name string) (message, suggestion string) {
	loop, other := a, b
	if !loop.loop {
		loop, other = b, a
	}
	if loop.loop {
		message = fmt.Sprintf(
			"The box of loop '%s' (%d,%d, %dx%d) overlaps '%s' (%d,%d) in %s '%s.%s'. "+
				"A loop's box grows with its body, so a gap sized for an ordinary activity is not enough.",
			loop.caption, loop.x, loop.y, loop.w, loop.h, other.caption, other.x, other.y,
			noun, module, name,
		)
		suggestion = fmt.Sprintf(
			"Widen the gap: the @position x values of the loop and '%s' must differ by more than half of both widths "+
				"(%dpx here). Or drop the @position annotations around the loop and let the layout place it.",
			other.caption, (loop.w+other.w)/2,
		)
		return message, suggestion
	}
	message = fmt.Sprintf(
		"Activities '%s' (%d,%d) and '%s' (%d,%d) overlap in %s '%s.%s'. "+
			"Each MDL statement that creates a canvas activity needs its own @position annotation.",
		a.caption, a.x, a.y, b.caption, b.x, b.y, noun, module, name,
	)
	return message, "Add a separate @position(x, y) annotation before each statement. Use 190px spacing between activities."
}

// actInfo is one positioned node on a canvas: its middle point (Mendix stores
// RelativeMiddlePoint) and its box size. loop marks a LoopedActivity, whose box
// is sized by its body.
type actInfo struct {
	x, y    int
	w, h    int
	caption string
	loop    bool
}

// boxesOverlap reports whether two nodes' boxes intersect. Each node is
// measured with its own stored size: judging a 40x40 merge as a 120x60
// activity reported merges that only touch, which auto-layout places that way
// on purpose. A node with no stored size is taken to be an activity box.
func boxesOverlap(a, b actInfo) bool {
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	return abs(a.x-b.x)*2 < a.w+b.w && abs(a.y-b.y)*2 < a.h+b.h
}

// newActInfo records a node's middle point and size.
func newActInfo(obj microflows.MicroflowObject, caption string) actInfo {
	p := obj.GetPosition()
	w, h := activityBoxWidth, activityBoxHeight
	switch obj.(type) {
	case *microflows.StartEvent, *microflows.EndEvent, *microflows.ErrorEvent,
		*microflows.BreakEvent, *microflows.ContinueEvent:
		w, h = eventSize, eventSize
	}
	if s, ok := obj.(interface{ GetSize() model.Size }); ok {
		if sz := s.GetSize(); sz.Width > 0 && sz.Height > 0 {
			w, h = sz.Width, sz.Height
		}
	}
	return actInfo{x: p.X, y: p.Y, w: w, h: h, caption: caption}
}

// overlapPlanes splits a microflow's objects into one plane per CANVAS, rather
// than flattening them into a single list.
//
// A LoopedActivity's children are positioned RELATIVE to the loop container,
// while everything on the microflow's own canvas is absolute. Flattening the two
// compares coordinates from different spaces and reports overlaps that cannot
// happen on screen — verified in the stored BSON: an outer activity at 200;230
// alongside a loop child at 141;130, where 141;130 is measured from the loop's
// own frame. A false "these overlap" is worse than silence here, because the
// point of the rule is to be trusted about positions. (upstream #884)
//
// The container itself belongs to its PARENT's plane; only its children get a
// new one.
func overlapPlanes(objects []microflows.MicroflowObject) [][]actInfo {
	var planes [][]actInfo
	var collect func(objs []microflows.MicroflowObject)
	collect = func(objs []microflows.MicroflowObject) {
		var plane []actInfo
		var nested [][]microflows.MicroflowObject
		for _, obj := range objs {
			switch act := obj.(type) {
			case *microflows.ActionActivity:
				caption := act.Caption
				if caption == "" {
					caption = "(unnamed)"
				}
				plane = append(plane, newActInfo(act, caption))
			case *microflows.LoopedActivity:
				caption := act.Caption
				if caption == "" {
					caption = "(loop)"
				}
				info := newActInfo(act, caption)
				info.loop = true
				plane = append(plane, info)
				if act.ObjectCollection != nil {
					nested = append(nested, act.ObjectCollection.Objects)
				}
			case *microflows.ExclusiveSplit:
				plane = append(plane, newActInfo(act, act.Caption))
			case *microflows.InheritanceSplit:
				plane = append(plane, newActInfo(act, act.Caption))
			case *microflows.ExclusiveMerge:
				plane = append(plane, newActInfo(act, "(merge)"))
			// Events are nodes on the canvas too. Leaving them out hid an
			// activity drawn on a handler's end event: two error handler rows
			// laid on top of each other were reported only when an activity
			// happened to land on an activity (ako/mxcli#1768).
			case *microflows.StartEvent:
				plane = append(plane, newActInfo(act, "(start)"))
			case *microflows.EndEvent:
				plane = append(plane, newActInfo(act, "(end)"))
			case *microflows.ErrorEvent:
				plane = append(plane, newActInfo(act, "(error)"))
			case *microflows.BreakEvent:
				plane = append(plane, newActInfo(act, "(break)"))
			case *microflows.ContinueEvent:
				plane = append(plane, newActInfo(act, "(continue)"))
			}
		}
		planes = append(planes, plane)
		for _, o := range nested {
			collect(o)
		}
	}
	collect(objects)
	return planes
}
