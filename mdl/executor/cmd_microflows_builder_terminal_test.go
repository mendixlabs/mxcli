// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestLastStmtIsReturn_EmptyBody(t *testing.T) {
	if lastStmtIsReturn(nil) {
		t.Error("empty body must not be terminal")
	}
}

func TestLastStmtIsReturn_PlainReturn(t *testing.T) {
	body := []ast.MicroflowStatement{&ast.ReturnStmt{}}
	if !lastStmtIsReturn(body) {
		t.Error("body ending in ReturnStmt must be terminal")
	}
}

func TestLastStmtIsReturn_RaiseError(t *testing.T) {
	body := []ast.MicroflowStatement{&ast.RaiseErrorStmt{}}
	if !lastStmtIsReturn(body) {
		t.Error("body ending in RaiseErrorStmt must be terminal")
	}
}

func TestLastStmtIsReturn_BreakAndContinue(t *testing.T) {
	for _, stmt := range []ast.MicroflowStatement{&ast.BreakStmt{}, &ast.ContinueStmt{}} {
		if !lastStmtIsReturn([]ast.MicroflowStatement{stmt}) {
			t.Errorf("body ending in %T must be terminal", stmt)
		}
	}
}

func TestLastStmtIsReturn_IfWithoutElse_NotTerminal(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{ThenBody: []ast.MicroflowStatement{&ast.ReturnStmt{}}},
	}
	if lastStmtIsReturn(body) {
		t.Error("IF without ELSE must not be terminal (false path falls through)")
	}
}

func TestLastStmtIsReturn_IfBothBranchesReturn_Terminal(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			ThenBody: []ast.MicroflowStatement{&ast.ReturnStmt{}},
			ElseBody: []ast.MicroflowStatement{&ast.ReturnStmt{}},
		},
	}
	if !lastStmtIsReturn(body) {
		t.Error("IF/ELSE where both branches return must be terminal")
	}
}

func TestLastStmtIsReturn_IfOnlyThenReturns_NotTerminal(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			ThenBody: []ast.MicroflowStatement{&ast.ReturnStmt{}},
			ElseBody: []ast.MicroflowStatement{&ast.LogStmt{}}, // non-terminal
		},
	}
	if lastStmtIsReturn(body) {
		t.Error("IF/ELSE where only THEN terminates must not be terminal")
	}
}

func TestLastStmtIsReturn_NestedIfChain_Terminal(t *testing.T) {
	// if { return } else if { return } else { return }
	inner := &ast.IfStmt{
		ThenBody: []ast.MicroflowStatement{&ast.ReturnStmt{}},
		ElseBody: []ast.MicroflowStatement{&ast.ReturnStmt{}},
	}
	outer := &ast.IfStmt{
		ThenBody: []ast.MicroflowStatement{&ast.ReturnStmt{}},
		ElseBody: []ast.MicroflowStatement{inner},
	}
	if !lastStmtIsReturn([]ast.MicroflowStatement{outer}) {
		t.Error("else-if chain where every terminal branch returns must be terminal")
	}
}

func TestLastStmtIsReturn_RaiseErrorMixed_Terminal(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			ThenBody: []ast.MicroflowStatement{&ast.ReturnStmt{}},
			ElseBody: []ast.MicroflowStatement{&ast.RaiseErrorStmt{}},
		},
	}
	if !lastStmtIsReturn(body) {
		t.Error("IF/ELSE with return on one side and raise error on the other must be terminal")
	}
}

func TestLastStmtIsReturn_IfBreakContinueBranches_Terminal(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			ThenBody: []ast.MicroflowStatement{&ast.ContinueStmt{}},
			ElseBody: []ast.MicroflowStatement{&ast.BreakStmt{}},
		},
	}
	if !lastStmtIsReturn(body) {
		t.Error("IF/ELSE with continue on one side and break on the other must be terminal")
	}
}

func TestLastStmtIsReturn_LoopNotTerminal(t *testing.T) {
	// A LOOP whose body only returns is still non-terminal — BREAK can exit.
	body := []ast.MicroflowStatement{
		&ast.LoopStmt{Body: []ast.MicroflowStatement{&ast.ReturnStmt{}}},
	}
	if lastStmtIsReturn(body) {
		t.Error("LOOP must never be terminal (BREAK path)")
	}
}

func TestBuildFlowGraph_LoopIfPreservesBreakAndContinue(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.LoopStmt{
			LoopVariable: "Item",
			ListVariable: "Items",
			Body: []ast.MicroflowStatement{
				&ast.IfStmt{
					Condition: &ast.VariableExpr{Name: "Changed"},
					ThenBody:  []ast.MicroflowStatement{&ast.ContinueStmt{}},
					ElseBody:  []ast.MicroflowStatement{&ast.BreakStmt{}},
				},
			},
		},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"Items": "List of MyModule.Item"},
		declaredVars: map[string]string{"Changed": "Boolean"},
		measurer:     &layoutMeasurer{},
	}
	oc := fb.buildFlowGraph(body, nil)

	var loop *microflows.LoopedActivity
	for _, obj := range oc.Objects {
		if l, ok := obj.(*microflows.LoopedActivity); ok {
			loop = l
			break
		}
	}
	if loop == nil || loop.ObjectCollection == nil {
		t.Fatal("expected loop with object collection")
	}

	var hasBreak, hasContinue bool
	for _, obj := range loop.ObjectCollection.Objects {
		switch obj.(type) {
		case *microflows.BreakEvent:
			hasBreak = true
		case *microflows.ContinueEvent:
			hasContinue = true
		}
	}
	if !hasBreak || !hasContinue {
		t.Fatalf("expected break and continue events in loop body, got break=%v continue=%v", hasBreak, hasContinue)
	}
}

func TestBuildFlowGraph_ManualWhileTrueContinueUsesBackEdgeMerge(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.WhileStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			Body: []ast.MicroflowStatement{
				&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "retry"}},
				&ast.ContinueStmt{},
			},
		},
	}

	fb := &flowBuilder{
		posX:     100,
		posY:     100,
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
	}
	oc := fb.buildFlowGraph(body, nil)

	var merge *microflows.ExclusiveMerge
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.LoopedActivity:
			t.Fatal("manual while true with continue must not be rebuilt as LoopedActivity")
		case *microflows.ContinueEvent:
			t.Fatal("manual while true back-edge must not emit ContinueEvent outside a LoopedActivity")
		case *microflows.ExclusiveMerge:
			merge = o
		}
	}
	if merge == nil {
		t.Fatal("expected manual loop header ExclusiveMerge")
	}

	var backEdges int
	for _, flow := range oc.Flows {
		if flow.DestinationID == merge.ID {
			backEdges++
		}
	}
	if backEdges == 0 {
		t.Fatal("expected continue branch to connect back to the manual-loop merge")
	}
}

func TestBuildFlowGraph_ManualWhileTrueTerminalDoesNotAddFallthroughEnd(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.WhileStmt{
			Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
			Body: []ast.MicroflowStatement{
				&ast.IfStmt{
					Condition: &ast.VariableExpr{Name: "Done"},
					ThenBody:  []ast.MicroflowStatement{&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true}}},
				},
				&ast.ContinueStmt{},
			},
		},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		declaredVars: map[string]string{"Done": "Boolean"},
		measurer:     &layoutMeasurer{},
	}
	oc := fb.buildFlowGraph(body, &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeBoolean}})

	for _, obj := range oc.Objects {
		end, ok := obj.(*microflows.EndEvent)
		if !ok {
			continue
		}
		if end.ReturnValue == "" {
			t.Fatal("manual while true ending in continue must not add a fallthrough EndEvent without return value")
		}
	}
}

func TestLastStmtIsReturn_EnumSplitAllBranchesReturn_Terminal(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.EnumSplitStmt{
			Cases: []ast.EnumSplitCase{
				{Values: []string{"Open"}, Body: []ast.MicroflowStatement{&ast.ReturnStmt{}}},
				{Values: []string{"Closed"}, Body: []ast.MicroflowStatement{&ast.RaiseErrorStmt{}}},
			},
			ElseBody: []ast.MicroflowStatement{&ast.ReturnStmt{}},
		},
	}
	if !lastStmtIsReturn(body) {
		t.Error("ENUM split where all cases and ELSE terminate must be terminal")
	}
}

func TestLastStmtIsReturn_EnumSplitWithoutElseAllBranchesReturn_Terminal(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.EnumSplitStmt{
			Cases: []ast.EnumSplitCase{
				{Values: []string{"Open"}, Body: []ast.MicroflowStatement{&ast.ReturnStmt{}}},
				{Values: []string{"Closed"}, Body: []ast.MicroflowStatement{&ast.ReturnStmt{}}},
			},
		},
	}
	if !lastStmtIsReturn(body) {
		t.Error("ENUM split without ELSE must be terminal when every emitted case terminates")
	}
}

func TestLastStmtIsReturn_EnumSplitWithoutElseNonTerminalCase_NotTerminal(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.EnumSplitStmt{
			Cases: []ast.EnumSplitCase{
				{Values: []string{"Open"}, Body: []ast.MicroflowStatement{&ast.ReturnStmt{}}},
				{Values: []string{"Closed"}, Body: []ast.MicroflowStatement{&ast.LogStmt{}}},
			},
		},
	}
	if lastStmtIsReturn(body) {
		t.Error("ENUM split without ELSE must not be terminal when any emitted case can continue")
	}
}

func TestBuildFlowGraph_NonTerminalCustomHandlerRejoinsContinuation(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.CallMicroflowStmt{
			MicroflowName: ast.QualifiedName{Module: "SampleSync", Name: "RefreshExternalData"},
			ErrorHandling: &ast.ErrorHandlingClause{
				Type: ast.ErrorHandlingCustomWithoutRollback,
				Body: []ast.MicroflowStatement{
					&ast.LogStmt{Level: ast.LogError, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "refresh failed"}},
				},
			},
		},
		&ast.CallMicroflowStmt{
			MicroflowName: ast.QualifiedName{Module: "SampleSync", Name: "ContinueWithNextBatch"},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
	oc := fb.buildFlowGraph(body, nil)

	var sourceID, handlerLogID, nextID model.ID
	for _, obj := range oc.Objects {
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		switch action := activity.Action.(type) {
		case *microflows.MicroflowCallAction:
			if action.MicroflowCall != nil && action.MicroflowCall.Microflow == "SampleSync.RefreshExternalData" {
				sourceID = activity.ID
			}
			if action.MicroflowCall != nil && action.MicroflowCall.Microflow == "SampleSync.ContinueWithNextBatch" {
				nextID = activity.ID
			}
		case *microflows.LogMessageAction:
			if action.LogLevel == "Error" {
				handlerLogID = activity.ID
			}
		}
	}
	if sourceID == "" || handlerLogID == "" || nextID == "" {
		t.Fatalf("expected source call, handler log, and continuation; got source=%q log=%q next=%q", sourceID, handlerLogID, nextID)
	}
	if !flowPathExists(oc.Flows, handlerLogID, nextID) {
		t.Fatal("non-terminal custom error handler should rejoin the next safe continuation")
	}
	for _, flow := range oc.Flows {
		if flow.IsErrorHandler && flow.OriginID == sourceID && flow.DestinationID == nextID {
			t.Fatal("custom handler must execute its body before rejoining")
		}
	}
}

func TestBuildFlowGraph_ConsecutiveCustomHandlersEachRejoinsContinuation(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.CallMicroflowStmt{
			MicroflowName: ast.QualifiedName{Module: "SampleSync", Name: "RetryFirstBatch"},
			ErrorHandling: &ast.ErrorHandlingClause{
				Type: ast.ErrorHandlingCustomWithoutRollback,
				Body: []ast.MicroflowStatement{
					&ast.LogStmt{Level: ast.LogError, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "first failed"}},
				},
			},
		},
		&ast.CallMicroflowStmt{
			MicroflowName: ast.QualifiedName{Module: "SampleSync", Name: "RetrySecondBatch"},
			ErrorHandling: &ast.ErrorHandlingClause{
				Type: ast.ErrorHandlingCustomWithoutRollback,
				Body: []ast.MicroflowStatement{
					&ast.LogStmt{Level: ast.LogError, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "second failed"}},
				},
			},
		},
		&ast.CallMicroflowStmt{
			MicroflowName: ast.QualifiedName{Module: "SampleSync", Name: "RetryFinalBatch"},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
	oc := fb.buildFlowGraph(body, nil)

	callIDs := map[string]model.ID{}
	logIDs := map[string]model.ID{}
	for _, obj := range oc.Objects {
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		switch action := activity.Action.(type) {
		case *microflows.MicroflowCallAction:
			if action.MicroflowCall != nil {
				callIDs[action.MicroflowCall.Microflow] = activity.ID
			}
		case *microflows.LogMessageAction:
			if action.MessageTemplate != nil {
				logIDs[action.MessageTemplate.Translations["en_US"]] = activity.ID
			}
		}
	}

	firstLog := logIDs["first failed"]
	secondLog := logIDs["second failed"]
	secondCall := callIDs["SampleSync.RetrySecondBatch"]
	finalCall := callIDs["SampleSync.RetryFinalBatch"]
	if firstLog == "" || secondLog == "" || secondCall == "" || finalCall == "" {
		t.Fatalf("expected all handler logs and continuation calls; logs=%#v calls=%#v", logIDs, callIDs)
	}
	if !flowPathExists(oc.Flows, firstLog, secondCall) {
		t.Fatal("first pending handler must rejoin before the second call instead of being overwritten")
	}
	if !flowPathExists(oc.Flows, secondLog, finalCall) {
		t.Fatal("second pending handler must rejoin before the final continuation")
	}
}

func TestBodyHasContinuingCustomErrorHandler_CoversActionStatements(t *testing.T) {
	continuingHandler := &ast.ErrorHandlingClause{
		Type: ast.ErrorHandlingCustomWithoutRollback,
		Body: []ast.MicroflowStatement{
			&ast.LogStmt{Level: ast.LogError, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "failed"}},
		},
	}

	tests := []struct {
		name string
		stmt ast.MicroflowStatement
	}{
		{name: "retrieve", stmt: &ast.RetrieveStmt{ErrorHandling: continuingHandler}},
		{name: "create object", stmt: &ast.CreateObjectStmt{ErrorHandling: continuingHandler}},
		{name: "commit", stmt: &ast.MfCommitStmt{ErrorHandling: continuingHandler}},
		{name: "delete", stmt: &ast.DeleteObjectStmt{ErrorHandling: continuingHandler}},
		{name: "call microflow", stmt: &ast.CallMicroflowStmt{ErrorHandling: continuingHandler}},
		{name: "call nanoflow", stmt: &ast.CallNanoflowStmt{ErrorHandling: continuingHandler}},
		{name: "call java action", stmt: &ast.CallJavaActionStmt{ErrorHandling: continuingHandler}},
		{name: "call javascript action", stmt: &ast.CallJavaScriptActionStmt{ErrorHandling: continuingHandler}},
		{name: "call web service", stmt: &ast.CallWebServiceStmt{ErrorHandling: continuingHandler}},
		{name: "execute database query", stmt: &ast.ExecuteDatabaseQueryStmt{ErrorHandling: continuingHandler}},
		{name: "call external action", stmt: &ast.CallExternalActionStmt{ErrorHandling: continuingHandler}},
		{name: "download file", stmt: &ast.DownloadFileStmt{ErrorHandling: continuingHandler}},
		{name: "rest call", stmt: &ast.RestCallStmt{ErrorHandling: continuingHandler}},
		{name: "send rest request", stmt: &ast.SendRestRequestStmt{ErrorHandling: continuingHandler}},
		{name: "import mapping", stmt: &ast.ImportFromMappingStmt{ErrorHandling: continuingHandler}},
		{name: "export mapping", stmt: &ast.ExportToMappingStmt{ErrorHandling: continuingHandler}},
		{name: "transform json", stmt: &ast.TransformJsonStmt{ErrorHandling: continuingHandler}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bodyHasContinuingCustomErrorHandler([]ast.MicroflowStatement{tt.stmt}) {
				t.Fatalf("%T must be visible to continuing custom handler detection", tt.stmt)
			}
		})
	}
}

func TestStatementReferencesVar_CoversActionStatementInputs(t *testing.T) {
	ref := &ast.VariableExpr{Name: "SkippedOutput"}
	tests := []struct {
		name string
		stmt ast.MicroflowStatement
	}{
		{name: "call nanoflow", stmt: &ast.CallNanoflowStmt{Arguments: []ast.CallArgument{{Name: "value", Value: ref}}}},
		{name: "call javascript action", stmt: &ast.CallJavaScriptActionStmt{Arguments: []ast.CallArgument{{Name: "value", Value: ref}}}},
		{name: "call web service timeout", stmt: &ast.CallWebServiceStmt{Timeout: ref}},
		{name: "execute database query argument", stmt: &ast.ExecuteDatabaseQueryStmt{Arguments: []ast.CallArgument{{Name: "value", Value: ref}}}},
		{name: "execute database query connection argument", stmt: &ast.ExecuteDatabaseQueryStmt{ConnectionArguments: []ast.CallArgument{{Name: "value", Value: ref}}}},
		{name: "call external action", stmt: &ast.CallExternalActionStmt{Arguments: []ast.CallArgument{{Name: "value", Value: ref}}}},
		{name: "rest auth", stmt: &ast.RestCallStmt{Auth: &ast.RestAuth{Username: ref}}},
		{name: "send rest request parameter", stmt: &ast.SendRestRequestStmt{Parameters: []ast.SendRestParamDef{{Name: "value", Expression: "$SkippedOutput/Name"}}}},
		{name: "send rest request body", stmt: &ast.SendRestRequestStmt{BodyVariable: "SkippedOutput"}},
		{name: "import mapping", stmt: &ast.ImportFromMappingStmt{SourceVariable: "SkippedOutput"}},
		{name: "export mapping", stmt: &ast.ExportToMappingStmt{SourceVariable: "SkippedOutput"}},
		{name: "transform json", stmt: &ast.TransformJsonStmt{InputVariable: "SkippedOutput"}},
		{name: "download file", stmt: &ast.DownloadFileStmt{FileDocument: "SkippedOutput"}},
		{name: "validation feedback target", stmt: &ast.ValidationFeedbackStmt{AttributePath: &ast.AttributePathExpr{Variable: "SkippedOutput", Path: []string{"Name"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !statementReferencesVar(tt.stmt, "SkippedOutput") {
				t.Fatalf("%T must expose input variable references for custom handler routing", tt.stmt)
			}
		})
	}
}

func TestBuildFlowGraph_EmptyOutputHandlerTerminatesBeforeOutputDependentTail(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.CallJavaActionStmt{
			OutputVariable: "ProcessedCount",
			ActionName:     ast.QualifiedName{Module: "SampleMigration", Name: "CountProcessedItems"},
			ErrorHandling:  &ast.ErrorHandlingClause{Type: ast.ErrorHandlingCustomWithoutRollback},
		},
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "processed {1}"},
			Template: []ast.TemplateParam{{
				Index: 1,
				Value: &ast.VariableExpr{Name: "ProcessedCount"},
			}},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
	oc := fb.buildFlowGraph(body, nil)

	var javaID, logID model.ID
	endIDs := map[model.ID]bool{}
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ActionActivity:
			switch action := o.Action.(type) {
			case *microflows.JavaActionCallAction:
				if action.ResultVariableName == "ProcessedCount" {
					javaID = o.ID
				}
			case *microflows.LogMessageAction:
				if action.MessageTemplate != nil && action.MessageTemplate.Translations["en_US"] == "processed {1}" {
					logID = o.ID
				}
			}
		case *microflows.EndEvent:
			endIDs[o.ID] = true
		}
	}
	if javaID == "" || logID == "" || len(endIDs) == 0 {
		t.Fatalf("expected java action, output-dependent log, and end event; got java=%q log=%q ends=%v", javaID, logID, endIDs)
	}

	var errorFlowTerminates bool
	for _, flow := range oc.Flows {
		if !flow.IsErrorHandler || flow.OriginID != javaID {
			continue
		}
		if flowPathExists(oc.Flows, flow.DestinationID, logID) {
			t.Fatal("empty output handler must not rejoin before a statement that reads the missing output")
		}
		for endID := range endIDs {
			if flow.DestinationID == endID || flowPathExists(oc.Flows, flow.DestinationID, endID) {
				errorFlowTerminates = true
			}
		}
	}
	if !errorFlowTerminates {
		t.Fatal("empty output handler should terminate at an EndEvent before the output-dependent tail")
	}
}

func TestBuildFlowGraph_OutputHandlerTerminatesBeforeDeclareReferencingOutput(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.CallMicroflowStmt{
			OutputVariable: "CreatedRecord",
			MicroflowName:  ast.QualifiedName{Module: "SampleSync", Name: "CreateRecord"},
			ErrorHandling: &ast.ErrorHandlingClause{
				Type: ast.ErrorHandlingCustomWithoutRollback,
				Body: []ast.MicroflowStatement{
					&ast.LogStmt{Level: ast.LogError, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "create failed"}},
				},
			},
		},
		&ast.DeclareStmt{
			Variable: "SuccessMessage",
			Type:     ast.DataType{Kind: ast.TypeString},
			InitialValue: &ast.BinaryExpr{
				Left:     &ast.LiteralExpr{Kind: ast.LiteralString, Value: "Created "},
				Operator: "+",
				Right:    &ast.AttributePathExpr{Variable: "CreatedRecord", Path: []string{"Name"}},
			},
		},
	}

	if !statementReferencesVar(body[1], "CreatedRecord") {
		t.Fatal("DECLARE initial values must be visible to custom handler skip-var routing")
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
	oc := fb.buildFlowGraph(body, nil)

	var callID, declareID model.ID
	endIDs := map[model.ID]bool{}
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ActionActivity:
			switch action := o.Action.(type) {
			case *microflows.MicroflowCallAction:
				if action.ResultVariableName == "CreatedRecord" {
					callID = o.ID
				}
			case *microflows.CreateVariableAction:
				if action.VariableName == "SuccessMessage" {
					declareID = o.ID
				}
			}
		case *microflows.EndEvent:
			endIDs[o.ID] = true
		}
	}
	if callID == "" || declareID == "" || len(endIDs) == 0 {
		t.Fatalf("expected call, output-dependent declare, and end event; got call=%q declare=%q ends=%v", callID, declareID, endIDs)
	}

	var errorFlowTerminates bool
	for _, flow := range oc.Flows {
		if !flow.IsErrorHandler || flow.OriginID != callID {
			continue
		}
		if flowPathExists(oc.Flows, flow.DestinationID, declareID) {
			t.Fatal("custom handler must not rejoin before a DECLARE that reads the missing output")
		}
		for endID := range endIDs {
			if flow.DestinationID == endID || flowPathExists(oc.Flows, flow.DestinationID, endID) {
				errorFlowTerminates = true
			}
		}
	}
	if !errorFlowTerminates {
		t.Fatal("custom handler should terminate at an EndEvent before the output-dependent declare")
	}
}

func TestBuildFlowGraph_OutputHandlerInReturningBranchSkipsDerivedSuccessTail(t *testing.T) {
	successMessage := &ast.VariableExpr{Name: "SuccessMessage"}
	body := []ast.MicroflowStatement{
		&ast.DeclareStmt{
			Variable:     "ErrorMessage",
			Type:         ast.DataType{Kind: ast.TypeString},
			InitialValue: &ast.LiteralExpr{Kind: ast.LiteralString, Value: ""},
		},
		&ast.IfStmt{
			Condition: &ast.VariableExpr{Name: "CanCreate"},
			ThenBody: []ast.MicroflowStatement{
				&ast.CallMicroflowStmt{
					OutputVariable: "CreatedRecord",
					MicroflowName:  ast.QualifiedName{Module: "SampleService", Name: "CreateRecord"},
					ErrorHandling: &ast.ErrorHandlingClause{
						Type: ast.ErrorHandlingCustomWithoutRollback,
						Body: []ast.MicroflowStatement{
							&ast.MfSetStmt{
								Target: "ErrorMessage",
								Value:  &ast.LiteralExpr{Kind: ast.LiteralString, Value: "create failed"},
							},
						},
					},
				},
				&ast.DeclareStmt{
					Variable: "SuccessMessage",
					Type:     ast.DataType{Kind: ast.TypeString},
					InitialValue: &ast.BinaryExpr{
						Left:     &ast.LiteralExpr{Kind: ast.LiteralString, Value: "Created "},
						Operator: "+",
						Right:    &ast.AttributePathExpr{Variable: "CreatedRecord", Path: []string{"Name"}},
					},
				},
				&ast.LogStmt{
					Level:   ast.LogInfo,
					Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "{1}"},
					Template: []ast.TemplateParam{
						{Index: 1, Value: successMessage},
					},
				},
				&ast.ShowMessageStmt{
					Message:      &ast.LiteralExpr{Kind: ast.LiteralString, Value: "{1}"},
					Type:         "Information",
					TemplateArgs: []ast.Expression{successMessage},
				},
				&ast.ClosePageStmt{},
				&ast.ReturnStmt{},
			},
			ElseBody: []ast.MicroflowStatement{
				&ast.MfSetStmt{
					Target: "ErrorMessage",
					Value:  &ast.LiteralExpr{Kind: ast.LiteralString, Value: "not found"},
				},
			},
		},
		&ast.LogStmt{
			Level:   ast.LogError,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "{1}"},
			Template: []ast.TemplateParam{
				{Index: 1, Value: &ast.VariableExpr{Name: "ErrorMessage"}},
			},
		},
		&ast.ShowMessageStmt{
			Message:      &ast.LiteralExpr{Kind: ast.LiteralString, Value: "{1}"},
			Type:         "Error",
			TemplateArgs: []ast.Expression{&ast.VariableExpr{Name: "ErrorMessage"}},
		},
		&ast.ReturnStmt{},
	}

	if !statementReferencesVar(body[1].(*ast.IfStmt).ThenBody[3], "SuccessMessage") {
		t.Fatal("SHOW MESSAGE arguments must be visible to custom handler skip-var routing")
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		declaredVars: map[string]string{"CanCreate": "Boolean"},
		measurer:     &layoutMeasurer{},
	}
	oc := fb.buildFlowGraph(body, nil)

	var callID, handlerSetID, successDeclareID, errorLogID model.ID
	for _, obj := range oc.Objects {
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		switch action := activity.Action.(type) {
		case *microflows.MicroflowCallAction:
			if action.ResultVariableName == "CreatedRecord" {
				callID = activity.ID
			}
		case *microflows.ChangeVariableAction:
			if action.VariableName == "ErrorMessage" && strings.Contains(action.Value, "create failed") {
				handlerSetID = activity.ID
			}
		case *microflows.CreateVariableAction:
			if action.VariableName == "SuccessMessage" {
				successDeclareID = activity.ID
			}
		case *microflows.LogMessageAction:
			if action.LogLevel == "Error" {
				errorLogID = activity.ID
			}
		}
	}
	if callID == "" || handlerSetID == "" || successDeclareID == "" || errorLogID == "" {
		t.Fatalf("expected source call, handler set, success declare, and error log; got call=%q handler=%q declare=%q errorLog=%q", callID, handlerSetID, successDeclareID, errorLogID)
	}
	if !flowPathExists(oc.Flows, callID, handlerSetID) {
		t.Fatal("source call must connect to the custom handler body")
	}
	if flowPathExists(oc.Flows, handlerSetID, successDeclareID) {
		t.Fatal("custom handler must skip the output-derived success tail")
	}
	if !flowPathExists(oc.Flows, handlerSetID, errorLogID) {
		t.Fatal("custom handler in a returning branch must rejoin at the shared safe continuation")
	}
}

func TestBuildFlowGraph_EmptyNoOutputHandlerRejoinsAtNextAction(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.CallMicroflowStmt{
			MicroflowName: ast.QualifiedName{Module: "SampleMigration", Name: "RefreshCache"},
			ErrorHandling: &ast.ErrorHandlingClause{Type: ast.ErrorHandlingCustomWithoutRollback},
		},
		&ast.CallJavaActionStmt{
			OutputVariable: "ProcessedCount",
			ActionName:     ast.QualifiedName{Module: "SampleMigration", Name: "CountProcessedItems"},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
	oc := fb.buildFlowGraph(body, nil)

	var callID, javaID model.ID
	for _, obj := range oc.Objects {
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		switch action := activity.Action.(type) {
		case *microflows.MicroflowCallAction:
			if action.MicroflowCall != nil && action.MicroflowCall.Microflow == "SampleMigration.RefreshCache" {
				callID = activity.ID
			}
		case *microflows.JavaActionCallAction:
			if action.ResultVariableName == "ProcessedCount" {
				javaID = activity.ID
			}
		}
	}
	if callID == "" || javaID == "" {
		t.Fatalf("expected no-output call and output-producing java action; got call=%q java=%q", callID, javaID)
	}
	for _, flow := range oc.Flows {
		if flow.IsErrorHandler && flow.OriginID == callID && flowPathExists(oc.Flows, flow.DestinationID, javaID) {
			return
		}
	}
	t.Fatal("empty no-output handler should rejoin at the next action")
}

func TestBuildFlowGraph_EmptyNoOutputHandlerInIfBranchRejoinsAtNextAction(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.VariableExpr{Name: "ShouldRefresh"},
			HasElse:   true,
			ThenBody: []ast.MicroflowStatement{
				&ast.CallMicroflowStmt{
					MicroflowName: ast.QualifiedName{Module: "Synthetic", Name: "RefreshCache"},
					ErrorHandling: &ast.ErrorHandlingClause{Type: ast.ErrorHandlingCustomWithoutRollback},
				},
				&ast.CallJavaActionStmt{
					OutputVariable: "ProcessedCount",
					ActionName:     ast.QualifiedName{Module: "Synthetic", Name: "CountProcessedItems"},
				},
			},
			ElseBody: []ast.MicroflowStatement{
				&ast.ReturnStmt{},
			},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
	oc := fb.buildFlowGraph(body, nil)

	var callID, javaID model.ID
	for _, obj := range oc.Objects {
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		switch action := activity.Action.(type) {
		case *microflows.MicroflowCallAction:
			if action.MicroflowCall != nil && action.MicroflowCall.Microflow == "Synthetic.RefreshCache" {
				callID = activity.ID
			}
		case *microflows.JavaActionCallAction:
			if action.ResultVariableName == "ProcessedCount" {
				javaID = activity.ID
			}
		}
	}
	if callID == "" || javaID == "" {
		t.Fatalf("expected no-output call and branch continuation; got call=%q java=%q", callID, javaID)
	}
	for _, flow := range oc.Flows {
		if flow.IsErrorHandler && flow.OriginID == callID && flowPathExists(oc.Flows, flow.DestinationID, javaID) {
			return
		}
	}
	t.Fatal("empty no-output handler inside IF branch should rejoin at the next branch action")
}

func TestBuildFlowGraph_ErrorHandlerEmptyElseKeepsFalseCaseOnRejoin(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.CallMicroflowStmt{
			MicroflowName: ast.QualifiedName{Module: "Synthetic", Name: "PatchRemoteState"},
			ErrorHandling: &ast.ErrorHandlingClause{
				Type: ast.ErrorHandlingCustom,
				Body: []ast.MicroflowStatement{
					&ast.IfStmt{
						Condition: &ast.VariableExpr{Name: "HasResponse"},
						HasElse:   true,
						ThenBody: []ast.MicroflowStatement{
							&ast.ReturnStmt{Value: &ast.VariableExpr{Name: "ErrorResponse"}},
						},
					},
				},
			},
		},
		&ast.CallMicroflowStmt{
			MicroflowName: ast.QualifiedName{Module: "Synthetic", Name: "ContinueAfterPatch"},
		},
		&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralNull}},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		declaredVars: map[string]string{"HasResponse": "Boolean", "ErrorResponse": "Synthetic.Error"},
		measurer:     &layoutMeasurer{},
	}
	oc := fb.buildFlowGraph(body, &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeEntity, EntityRef: &ast.QualifiedName{Module: "Synthetic", Name: "Error"}}})

	var splitID, continuationID model.ID
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ExclusiveSplit:
			if condition, ok := o.SplitCondition.(*microflows.ExpressionSplitCondition); ok && condition.Expression == "$HasResponse" {
				splitID = o.ID
			}
		case *microflows.ActionActivity:
			if action, ok := o.Action.(*microflows.MicroflowCallAction); ok && action.MicroflowCall != nil && action.MicroflowCall.Microflow == "Synthetic.ContinueAfterPatch" {
				continuationID = o.ID
			}
		}
	}
	if splitID == "" || continuationID == "" {
		t.Fatalf("expected error-handler split and continuation call, got split=%q continuation=%q", splitID, continuationID)
	}

	for _, flow := range oc.Flows {
		if flow.OriginID != splitID || flowCaseString(flow.CaseValue) != "false" {
			continue
		}
		if flow.DestinationID == continuationID || flowPathExists(oc.Flows, flow.DestinationID, continuationID) {
			return
		}
	}
	t.Fatal("expected deferred custom error-handler ELSE path to retain CaseValue=false when rejoining")
}

func TestBuildFlowGraph_ExplicitEmptyElseProvidesFalseContinuation(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.IfStmt{
			Condition: &ast.VariableExpr{Name: "HasResponse"},
			HasElse:   true,
			ThenBody: []ast.MicroflowStatement{
				&ast.ReturnStmt{Value: &ast.VariableExpr{Name: "ErrorResponse"}},
			},
		},
		&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralNull}},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		declaredVars: map[string]string{"HasResponse": "Boolean", "ErrorResponse": "Synthetic.Error"},
		measurer:     &layoutMeasurer{},
	}
	oc := fb.buildFlowGraph(body, &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeEntity, EntityRef: &ast.QualifiedName{Module: "Synthetic", Name: "Error"}}})

	var splitID, emptyEndID model.ID
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ExclusiveSplit:
			splitID = o.ID
		case *microflows.EndEvent:
			if o.ReturnValue == "empty" {
				emptyEndID = o.ID
			}
		}
	}
	if splitID == "" || emptyEndID == "" {
		t.Fatalf("expected split and empty return end event, got split=%q emptyEnd=%q", splitID, emptyEndID)
	}

	for _, flow := range oc.Flows {
		if flow.OriginID != splitID || flowCaseString(flow.CaseValue) != "false" {
			continue
		}
		if flowPathExists(oc.Flows, flow.DestinationID, emptyEndID) {
			return
		}
	}
	t.Fatal("expected explicit empty else to produce a false-path continuation")
}

func flowCaseString(caseValue microflows.CaseValue) string {
	switch c := caseValue.(type) {
	case microflows.EnumerationCase:
		return c.Value
	case *microflows.EnumerationCase:
		if c != nil {
			return c.Value
		}
	case *microflows.ExpressionCase:
		if c != nil {
			return c.Expression
		}
	}
	return ""
}

func flowPathExists(flows []*microflows.SequenceFlow, startID, targetID model.ID) bool {
	if startID == "" || targetID == "" {
		return false
	}
	seen := map[model.ID]bool{}
	queue := []model.ID{startID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == targetID {
			return true
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, flow := range flows {
			if flow.OriginID == id && !seen[flow.DestinationID] {
				queue = append(queue, flow.DestinationID)
			}
		}
	}
	return false
}
