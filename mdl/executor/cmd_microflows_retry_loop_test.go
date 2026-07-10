// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// Retry-loop error handler: a call activity with a custom error handler
// whose body ends with an IF where the ELSE raises and the THEN performs
// retry actions must produce a merge placed before the source activity,
// with the handler tail looping back to the merge. Without this topology
// the merge falls after the source and subsequent activities referencing
// the call's output variable trigger CE0108 "variable not in scope".
func TestRetryLoopErrorHandlerLoopsBackToSource(t *testing.T) {
	fb := &flowBuilder{
		spacing:      HorizontalSpacing,
		measurer:     &layoutMeasurer{},
		declaredVars: map[string]string{"R": "String", "RetryCount": "Integer"},
	}

	oc := fb.buildFlowGraph([]ast.MicroflowStatement{
		&ast.DeclareStmt{
			Variable:     "RetryCount",
			Type:         ast.DataType{Kind: ast.TypeInteger},
			InitialValue: &ast.LiteralExpr{Kind: ast.LiteralInteger, Value: "0"},
		},
		&ast.CallMicroflowStmt{
			MicroflowName:  ast.QualifiedName{Module: "M", Name: "Fetch"},
			OutputVariable: "R",
			ErrorHandling: &ast.ErrorHandlingClause{
				Type: ast.ErrorHandlingCustomWithoutRollback,
				Body: []ast.MicroflowStatement{
					&ast.LogStmt{Level: ast.LogError, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "fetch failed"}},
					&ast.IfStmt{
						Condition: &ast.BinaryExpr{
							Operator: "<",
							Left:     &ast.VariableExpr{Name: "RetryCount"},
							Right:    &ast.LiteralExpr{Kind: ast.LiteralInteger, Value: "3"},
						},
						ThenBody: []ast.MicroflowStatement{
							&ast.MfSetStmt{
								Target: "RetryCount",
								Value: &ast.BinaryExpr{
									Operator: "+",
									Left:     &ast.VariableExpr{Name: "RetryCount"},
									Right:    &ast.LiteralExpr{Kind: ast.LiteralInteger, Value: "1"},
								},
							},
						},
						ElseBody: []ast.MicroflowStatement{
							&ast.RaiseErrorStmt{},
						},
					},
				},
			},
		},
		&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "ok"}},
		&ast.ReturnStmt{},
	}, nil)

	// Find the call activity, the retry-body log/set activities, and the merge.
	var callID, logAfterID model.ID
	var mergeIDs []model.ID
	logSeen := 0
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ActionActivity:
			if _, ok := o.Action.(*microflows.MicroflowCallAction); ok {
				callID = o.ID
			}
			if _, ok := o.Action.(*microflows.LogMessageAction); ok {
				logSeen++
				// logSeen==1 is the error-handler log inside the handler body.
				// logSeen==2 is the main-path log after the call.
				if logSeen == 2 {
					logAfterID = o.ID
				}
			}
		case *microflows.ExclusiveMerge:
			mergeIDs = append(mergeIDs, o.ID)
		}
	}
	if callID == "" {
		t.Fatal("no call activity found")
	}
	if logAfterID == "" {
		t.Fatal("no post-call log activity found")
	}
	if len(mergeIDs) == 0 {
		t.Fatal("no merge node found — retry-loop topology was not built")
	}

	// The merge must sit on the call's inbound path: there must be an edge
	// prev->merge (not prev->call) and an edge merge->call. The handler
	// tail loops back to the merge as a plain SequenceFlow (not marked
	// IsErrorHandler — the error marker applies only to the source→first
	// handler activity edge, which addErrorHandlerFlow emits separately).
	var mergeToCallFound bool
	var prevToMergeFound bool
	var tailToMergeFound bool
	// Find the retry-branch tail activity (the ChangeVariable that
	// increments the retry counter in the test fixture).
	var tailID model.ID
	for _, obj := range oc.Objects {
		if a, ok := obj.(*microflows.ActionActivity); ok {
			if _, isChange := a.Action.(*microflows.ChangeVariableAction); isChange {
				tailID = a.ID
			}
		}
	}
	for _, f := range oc.Flows {
		for _, mID := range mergeIDs {
			if f.DestinationID == callID && f.OriginID == mID {
				mergeToCallFound = true
			}
			if f.DestinationID == mID && f.OriginID == tailID && !f.IsErrorHandler {
				tailToMergeFound = true
			}
			if f.DestinationID == mID && !f.IsErrorHandler && f.OriginID != mID && f.OriginID != tailID {
				prevToMergeFound = true
			}
		}
		// No flow should terminate directly at the call from a non-merge origin.
		if f.DestinationID == callID && !f.IsErrorHandler {
			originIsMerge := false
			for _, mID := range mergeIDs {
				if f.OriginID == mID {
					originIsMerge = true
				}
			}
			if !originIsMerge {
				t.Errorf("normal inbound flow to call %s came from non-merge origin %s; retry-loop merge was not inserted",
					shortID(callID), shortID(f.OriginID))
			}
		}
	}
	if !mergeToCallFound {
		t.Error("missing merge -> call flow")
	}
	if !prevToMergeFound {
		t.Error("missing prev -> merge normal flow")
	}
	if !tailToMergeFound {
		t.Error("missing handler-tail -> merge (loop-back) flow")
	}
}

// Non-retry error handlers (both branches return, neither branch raises,
// or no trailing IF) must still use the forward-merge path.
func TestNonRetryErrorHandlerUsesForwardMerge(t *testing.T) {
	fb := &flowBuilder{
		spacing:      HorizontalSpacing,
		measurer:     &layoutMeasurer{},
		declaredVars: map[string]string{"R": "String"},
	}

	oc := fb.buildFlowGraph([]ast.MicroflowStatement{
		&ast.CallMicroflowStmt{
			MicroflowName:  ast.QualifiedName{Module: "M", Name: "Fetch"},
			OutputVariable: "R",
			ErrorHandling: &ast.ErrorHandlingClause{
				Type: ast.ErrorHandlingCustomWithoutRollback,
				Body: []ast.MicroflowStatement{
					// Single log — no trailing IF, no retry shape.
					&ast.LogStmt{Level: ast.LogError, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "failed"}},
				},
			},
		},
		&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "ok"}},
		&ast.ReturnStmt{},
	}, nil)

	// The call should have an inbound flow from the previous activity (the
	// StartEvent or a fb-generated start), NOT via a merge. If a merge were
	// incorrectly inserted, every such non-retry microflow would gain an
	// extra unnecessary node.
	var callID model.ID
	for _, obj := range oc.Objects {
		if a, ok := obj.(*microflows.ActionActivity); ok {
			if _, isCall := a.Action.(*microflows.MicroflowCallAction); isCall {
				callID = a.ID
			}
		}
	}
	if callID == "" {
		t.Fatal("no call activity")
	}
	for _, f := range oc.Flows {
		if f.DestinationID == callID && !f.IsErrorHandler {
			// Origin should be a non-merge (StartEvent or a plain activity).
			for _, obj := range oc.Objects {
				if obj.GetID() == f.OriginID {
					if _, isMerge := obj.(*microflows.ExclusiveMerge); isMerge {
						t.Errorf("non-retry handler unexpectedly inserted a merge before call")
					}
				}
			}
		}
	}
}

func shortID(id model.ID) string {
	s := string(id)
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
