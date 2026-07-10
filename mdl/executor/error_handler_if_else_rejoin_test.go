package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// Empty custom error handler followed by an IF that checks the call's
// output variable: the error edge must rejoin the fallback path at the
// first ELSE activity, sharing a merge with the split's `false` branch.
// Without this rejoin, terminatePendingErrorHandlersAtEnd synthesises a
// phantom EndEvent at the microflow tail (CE0079); wiring the error edge
// straight to the IF split bypasses the output-variable declaration and
// triggers CE0108 "variable not in scope" downstream.
func TestEmptyErrorHandlerRejoinsAtIfElseFirstActivity(t *testing.T) {
	fb := &flowBuilder{
		spacing:      HorizontalSpacing,
		measurer:     &layoutMeasurer{},
		declaredVars: map[string]string{"R": "String"},
	}

	oc := fb.buildFlowGraph([]ast.MicroflowStatement{
		&ast.CallMicroflowStmt{
			MicroflowName:  ast.QualifiedName{Module: "M", Name: "Helper"},
			OutputVariable: "R",
			ErrorHandling: &ast.ErrorHandlingClause{
				Type: ast.ErrorHandlingCustomWithoutRollback,
				Body: nil,
			},
		},
		&ast.IfStmt{
			Condition: &ast.VariableExpr{Name: "R"},
			ThenBody: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "ok"}},
				&ast.ReturnStmt{},
			},
			ElseBody: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogError, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "fail"}},
				&ast.ReturnStmt{},
			},
		},
	}, nil)

	// Count EndEvents — should be 2 (one per IF branch), no phantom at mf end.
	endEvents := 0
	for _, obj := range oc.Objects {
		if _, ok := obj.(*microflows.EndEvent); ok {
			endEvents++
		}
	}
	if endEvents != 2 {
		t.Errorf("got %d EndEvents, want 2", endEvents)
	}

	// Find call activity, if split, first-else activity
	var callID, splitID, firstElseID model.ID
	logCount := 0
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ActionActivity:
			if _, ok := o.Action.(*microflows.MicroflowCallAction); ok {
				callID = o.ID
			}
			if _, ok := o.Action.(*microflows.LogMessageAction); ok {
				logCount++
				if logCount == 2 {
					// Second log in visitation order = the else body log ("fail")
					firstElseID = o.ID
				}
			}
		case *microflows.ExclusiveSplit:
			splitID = o.ID
		}
	}
	if callID == "" || splitID == "" || firstElseID == "" {
		t.Fatal("missing call/split/firstElse")
	}

	// Error flow must originate at call and terminate at a merge reachable from split(false).
	// Trace error flow from call:
	var errorDest model.ID
	for _, f := range oc.Flows {
		if f.OriginID == callID && f.IsErrorHandler {
			errorDest = f.DestinationID
			break
		}
	}
	if errorDest == "" {
		t.Fatal("no error-handler flow from call")
	}
	// errorDest should be a merge that also receives the split false branch and leads to firstElseID
	mergeReachesFirstElse := false
	splitFalseReachesMerge := false
	for _, f := range oc.Flows {
		if f.OriginID == errorDest && f.DestinationID == firstElseID {
			mergeReachesFirstElse = true
		}
		if f.OriginID == splitID && f.DestinationID == errorDest {
			splitFalseReachesMerge = true
		}
	}
	if !mergeReachesFirstElse {
		t.Errorf("merge %s does not flow to first-else activity %s", string(errorDest)[:6], string(firstElseID)[:6])
	}
	if !splitFalseReachesMerge {
		t.Errorf("split %s (false) does not flow to merge %s", string(splitID)[:6], string(errorDest)[:6])
	}
}
