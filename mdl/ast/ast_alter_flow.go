// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// ALTER MICROFLOW / ALTER NANOFLOW — a graph splice into the stored flow
// ============================================================================

// AlterFlowStmt represents:
//
//	alter microflow|nanoflow Module.Name {
//	  insert after|before <target> begin <statements> end;
//	  replace <target> with begin <statements> end;
//	  replace body of <loop> with begin <statements> end;
//	  drop <target>;
//	}
//
// (ADR-0012 decision 3). Targets are content addresses, resolved against the
// flow as stored before any operation applies.
type AlterFlowStmt struct {
	Nanoflow   bool
	Name       QualifiedName
	Operations []*AlterFlowOperation
}

func (s *AlterFlowStmt) isStatement() {}

// Kind is "microflow" or "nanoflow".
func (s *AlterFlowStmt) Kind() string {
	if s.Nanoflow {
		return "nanoflow"
	}
	return "microflow"
}

// AlterFlowOpKind names an operation of an AlterFlowStmt.
type AlterFlowOpKind string

const (
	AlterFlowInsertAfter  AlterFlowOpKind = "insert after"
	AlterFlowInsertBefore AlterFlowOpKind = "insert before"
	AlterFlowReplace      AlterFlowOpKind = "replace"
	AlterFlowDrop         AlterFlowOpKind = "drop"
	// AlterFlowReplaceBody replaces what is inside a stored loop and keeps
	// the loop itself — its $ID, position, iterator and list
	// (ako/mxcli#1693). Target addresses the loop; Body is its new body.
	AlterFlowReplaceBody AlterFlowOpKind = "replace body of"
)

// AlterFlowOperation is one operation of an AlterFlowStmt.
type AlterFlowOperation struct {
	Op AlterFlowOpKind
	// Target is the content address as written (`$IsValidEmail`,
	// `'Email is Valid?'`, `log * node 'Debug' *`, with an optional `@n`).
	// mfmutator.ParseTarget reads it.
	Target string
	// Body is the fragment, for insert and replace.
	Body []MicroflowStatement
	// ReplaceNotes, on a replace or drop, says the notes attached to the
	// target go with it instead of being kept on the replacement (or left
	// behind): Body carries the notes the statement states. Set only by
	// `create or modify`, whose declared statements state every activity's
	// notes (ako/mxcli#859); an `alter` replace keeps them.
	ReplaceNotes bool
	// SetCondition, on a replace of a decision, says Body is a single `if`
	// whose condition is the decision's new one, set in place: the decision,
	// its flows and both of its paths stay (ako/mxcli#888). Set only by
	// `create or modify`, for an `if` that differs from the stored one in its
	// condition; the branches are diffed by operations of their own.
	SetCondition bool
	// KeepHandler, on a replace of an activity with a custom error handler,
	// says Body's one statement states the stored handler unchanged: the
	// activity alone is replaced, in place, and the stored handler — its
	// activities, its error flow and the merge it rejoins at — stays
	// (mendixlabs/mxcli#1489). Set by `create or modify`, which diffs the
	// handler's body by operations of its own; an `alter` replace that restates
	// the stored handler is recognised when it is applied.
	KeepHandler bool
	// SetNotes, on a replace of a decision or an end event, says Body is one
	// statement whose notes are the node's new notes: the stored notes are
	// taken off and these attached, and the node itself stays — it is changed
	// in place, never replaced (mendixlabs/mxcli#1517). Set only by
	// `create or modify`.
	SetNotes bool
	// StatesCurves, on an insert, says InCurve and OutCurve are the curves the
	// script states for the connector into the inserted statements (on the
	// statement before them) and the one out of them (on their last), nil
	// meaning a straight line, as describe prints none for one. Set only by
	// `create or modify`, whose declared flow is drawn whole; an `alter`
	// insert states no curves (ako/mxcli#2311).
	StatesCurves      bool
	InCurve, OutCurve *FlowCurve
}
