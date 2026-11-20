// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// MicroflowFragment is what an `alter microflow` insert or replace splices in:
// the objects and flows a fragment of MDL builds to, written exactly as
// `create microflow` writes the same statements, and the two objects the
// surrounding flow connects to. Entry is where the flow into the fragment
// ends; Exit is where the flow out of it starts. For a one-activity fragment
// they are the same object.
//
// Positions are the builder's own; the mutator translates the fragment into
// place (plan item 4.2c) before writing it.
type MicroflowFragment struct {
	Objects         []microflows.MicroflowObject
	Flows           []*microflows.SequenceFlow
	AnnotationFlows []*microflows.AnnotationFlow
	Entry, Exit     model.ID
	// ExitCase is the case value the flow leaving Exit carries, when Exit is
	// a decision — a guard clause last in the fragment, whose other path
	// returns (ako/mxcli#888). nil when the flow out of Exit has no case.
	ExitCase microflows.CaseValue
	// Placed says the script stated where the fragment goes (an @position on
	// its first statement): it is written where the builder put it, and
	// nothing around it is moved to make room (ako/mxcli#818). Unset, the
	// mutator translates it into the gap it is spliced into.
	Placed bool
	// Pinned are the objects whose position the script states on the canvas —
	// a note's `position:` — rather than relative to the fragment. They stay
	// where they are when the fragment is translated into place, and take no
	// part in the room it needs (mendixlabs/mxcli#1598, case 1).
	Pinned map[model.ID]bool
	// InCurve and OutCurve are the control vectors the script states for the
	// connector into the fragment and the one out of it, when it states them
	// (`create or modify`, whose declared flow is drawn whole): the splice
	// draws those two connectors with them, so the flow it leaves is the flow
	// the script describes and the next run of it finds nothing to change
	// (ako/mxcli#2311). nil keeps the splice's own vectors (an `alter`).
	InCurve, OutCurve *FlowVectors
}

// FlowVectors are a sequence flow's two bezier control vectors as stored,
// "x;y"; "0;0" is a straight end.
type FlowVectors struct {
	Origin, Destination string
}

// MicroflowMutator splices into one stored microflow or nanoflow (ADR-0012
// decision 3). Targets are activity IDs, already resolved from their content
// address by mfmutator.Resolve. Every operation edits the stored document in
// place: untouched elements stay byte-identical, and nothing is rebuilt.
// Call Save to persist.
type MicroflowMutator interface {
	// InsertAfter splices frag onto the one flow that leaves target.
	InsertAfter(target model.ID, frag *MicroflowFragment) error
	// InsertBefore splices frag onto the one flow that enters target.
	InsertBefore(target model.ID, frag *MicroflowFragment) error
	// Replace puts frag in target's place and removes target. A target
	// replaced by one activity of the same kind — the same activity and
	// action — is edited instead: the new content is written onto the stored
	// activity, which keeps its $ID and those of the elements under it that
	// pair up (ako/mxcli#1761).
	Replace(target model.ID, frag *MicroflowFragment) error
	// EditedInPlace reports whether a Replace of target edited the stored
	// activity, keeping its identity, rather than putting new elements in
	// its place — what the report of the change says.
	EditedInPlace(target model.ID) bool
	// Drop removes target and joins its incoming flows to its successor. A
	// decision of the shape `alter … insert` produces goes with its paths and
	// its merge (ako/mxcli#1694); any other decision is refused.
	Drop(target model.ID) error
	// ReplaceLoopBody replaces everything inside the loop target by the body
	// of the one loop frag holds — the loop restated with its new body, as the
	// builder made it — and keeps the loop itself: its $ID, position,
	// iterator, list and the flows and notes around it (ako/mxcli#1693). The
	// box keeps its size where the new body fits it, and grows where it does
	// not.
	ReplaceLoopBody(target model.ID, frag *MicroflowFragment) error
	// DecisionNodes returns the activities on the paths of the decision
	// target when it has the shape `alter … insert` produces — two paths of
	// plain activities, or none, rejoining at one merge — which a Drop or a
	// Replace of it takes out with it (ako/mxcli#1694); nil for any other
	// target.
	DecisionNodes(target model.ID) []model.ID
	// RemoveNotes removes the annotations attached to target, with their
	// lines, so that a Replace or Drop after it does not keep them
	// (ako/mxcli#859: a `create or modify` states an activity's notes). A
	// note also attached to another object is refused.
	RemoveNotes(target model.ID) error
	// AddNotes attaches notes to target, which stays as it is — a decision or
	// an end event, whose notes change without the node being replaced
	// (mendixlabs/mxcli#1517). frag holds one stand-in object, frag.Entry, and
	// the notes the builder attached to it: each note is drawn where it is
	// relative to the stand-in, but relative to target, and its line runs to
	// target.
	AddNotes(target model.ID, frag *MicroflowFragment) error
	// LinkNote draws a line from note, a note already in the flow, to object,
	// an object in the flow — one an earlier call added included — unless one
	// is drawn already. The note keeps its $ID, text and place; the line
	// attaches at the sides of the two that face each other where they are
	// drawn now (ako/mxcli#1695).
	LinkNote(note, object model.ID) error
	// SetFreeNotes replaces the flow's free notes — notes on the canvas,
	// attached to nothing — by the ones frag holds (its Annotation objects
	// that no line in it attaches; its other objects are ignored). Nothing
	// else changes (mendixlabs/mxcli#1517).
	SetFreeNotes(frag *MicroflowFragment) error
	// SetReturnValue sets the expression the end event target returns, in
	// place; "" is no value.
	SetReturnValue(target model.ID, value string) error
	// SetCondition sets the expression the decision target branches on, and
	// its caption, in place; the decision and its flows stay. A decision that
	// calls a rule is refused.
	SetCondition(target model.ID, expression, caption string) error
	// Move sets where the top-level node target is drawn, and nothing else:
	// its flows keep their ends, sides and curves (ako/mxcli#818).
	Move(target model.ID, to model.Point) error
	// Pin says the script states that a node is drawn at the point at — a
	// stored node, or one a fragment places there. An insert or replace that
	// would move the node drawn there to make room is refused instead, so the
	// stored flow keeps saying what the script says (mendixlabs/mxcli#1598,
	// case 6).
	Pin(at model.Point) error
	// PinInLoop is Pin for a node of the body of the loop: at is in the loop
	// box's coordinates (ako/mxcli#1693).
	PinInLoop(loop model.ID, at model.Point) error
	// SetHeader writes the document properties of declared — a
	// *microflows.Microflow or *microflows.Nanoflow built from the statement,
	// with every property it does not state carried from the stored one — onto
	// the stored document: the header (return type, URL, export level,
	// concurrency, documentation, …) and the parameters (added, retyped or
	// removed). Nothing else is touched, and an unchanged property is not
	// rewritten. It returns what it changed, for the report.
	SetHeader(declared any) ([]string, error)
	// Save writes the patched unit.
	Save() error
}

// MicroflowMutationBackend opens a microflow or nanoflow for splicing.
type MicroflowMutationBackend interface {
	// OpenMicroflowForMutation loads a Microflows$Microflow or
	// Microflows$Nanoflow unit and returns a mutator over its stored form.
	OpenMicroflowForMutation(unitID model.ID) (MicroflowMutator, error)
}
