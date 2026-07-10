// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestNestedIfPreservesCaptions is a regression test for the bug where
// the outer IF's @caption would be overwritten by the inner IF's @caption
// because pendingAnnotations is shared mutable state across recursive
// addStatement calls.
//
// Before the fix:
//   - outer ExclusiveSplit received caption "Right format?" (from inner IF)
//   - inner ExclusiveSplit kept its condition expression as caption
//   - inner IF's @annotation got attached to the outer split
//
// After the fix:
//   - addIfStatement consumes its own pendingAnnotations right after
//     creating its split, so outer and inner captions stay bound to the
//     correct splits.
func TestNestedIfPreservesCaptions(t *testing.T) {
	// Build an AST equivalent to:
	//   if $S != empty            @caption 'String not empty?'
	//     if isMatch($S, 'x')     @caption 'Right format?'
	//       return true
	//     else
	//       return false
	//   else
	//     return false
	innerIf := &ast.IfStmt{
		Condition: &ast.FunctionCallExpr{
			Name:      "isMatch",
			Arguments: []ast.Expression{&ast.VariableExpr{Name: "S"}, &ast.LiteralExpr{Value: "x", Kind: ast.LiteralString}},
		},
		ThenBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: true, Kind: ast.LiteralBoolean}},
		},
		ElseBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: false, Kind: ast.LiteralBoolean}},
		},
		Annotations: &ast.ActivityAnnotations{Caption: "Right format?"},
	}
	outerIf := &ast.IfStmt{
		Condition: &ast.BinaryExpr{
			Left:     &ast.VariableExpr{Name: "S"},
			Operator: "!=",
			Right:    &ast.LiteralExpr{Value: nil, Kind: ast.LiteralNull},
		},
		ThenBody: []ast.MicroflowStatement{innerIf},
		ElseBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: false, Kind: ast.LiteralBoolean}},
		},
		Annotations: &ast.ActivityAnnotations{Caption: "String not empty?"},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"S": "String"},
		declaredVars: map[string]string{"S": "String"},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{outerIf}, nil)

	// Collect ExclusiveSplits with their captions. The outer split is created
	// first, so objects[1] is the outer split (objects[0] is the StartEvent).
	var splits []*microflows.ExclusiveSplit
	for _, obj := range fb.objects {
		if sp, ok := obj.(*microflows.ExclusiveSplit); ok {
			splits = append(splits, sp)
		}
	}

	if len(splits) != 2 {
		t.Fatalf("expected 2 ExclusiveSplits, got %d", len(splits))
	}

	// Splits are appended in creation order: outer first (from outerIf),
	// then inner (when recursion into ThenBody creates the nested IF's split).
	outerSplit, innerSplit := splits[0], splits[1]

	if outerSplit.Caption != "String not empty?" {
		t.Errorf("outer split caption: got %q, want %q", outerSplit.Caption, "String not empty?")
	}
	if innerSplit.Caption != "Right format?" {
		t.Errorf("inner split caption: got %q, want %q", innerSplit.Caption, "Right format?")
	}
}

// TestIfCaptionWithoutNesting confirms a single IF with @caption still gets
// the right caption after the fix (baseline sanity check).
func TestIfCaptionWithoutNesting(t *testing.T) {
	ifStmt := &ast.IfStmt{
		Condition: &ast.BinaryExpr{
			Left:     &ast.VariableExpr{Name: "S"},
			Operator: "!=",
			Right:    &ast.LiteralExpr{Value: nil, Kind: ast.LiteralNull},
		},
		ThenBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: true, Kind: ast.LiteralBoolean}},
		},
		ElseBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: false, Kind: ast.LiteralBoolean}},
		},
		Annotations: &ast.ActivityAnnotations{Caption: "String not empty?"},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"S": "String"},
		declaredVars: map[string]string{"S": "String"},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{ifStmt}, nil)

	for _, obj := range fb.objects {
		if sp, ok := obj.(*microflows.ExclusiveSplit); ok {
			if sp.Caption != "String not empty?" {
				t.Errorf("split caption: got %q, want %q", sp.Caption, "String not empty?")
			}
			return
		}
	}
	t.Fatal("no ExclusiveSplit found")
}

// TestIfAnnotationStaysWithCorrectSplit confirms @annotation on a nested IF
// attaches to that IF's split, not to the outer one.
func TestIfAnnotationStaysWithCorrectSplit(t *testing.T) {
	innerIf := &ast.IfStmt{
		Condition: &ast.FunctionCallExpr{
			Name:      "isMatch",
			Arguments: []ast.Expression{&ast.VariableExpr{Name: "S"}, &ast.LiteralExpr{Value: "x", Kind: ast.LiteralString}},
		},
		ThenBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: true, Kind: ast.LiteralBoolean}},
		},
		ElseBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: false, Kind: ast.LiteralBoolean}},
		},
		Annotations: &ast.ActivityAnnotations{
			Caption:        "Right format?",
			AnnotationText: "Inner IF note",
		},
	}
	outerIf := &ast.IfStmt{
		Condition: &ast.BinaryExpr{
			Left:     &ast.VariableExpr{Name: "S"},
			Operator: "!=",
			Right:    &ast.LiteralExpr{Value: nil, Kind: ast.LiteralNull},
		},
		ThenBody: []ast.MicroflowStatement{innerIf},
		ElseBody: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: false, Kind: ast.LiteralBoolean}},
		},
		Annotations: &ast.ActivityAnnotations{
			Caption:        "String not empty?",
			AnnotationText: "Outer IF note",
		},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"S": "String"},
		declaredVars: map[string]string{"S": "String"},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{outerIf}, nil)

	var splits []*microflows.ExclusiveSplit
	var annotations []*microflows.Annotation
	for _, obj := range fb.objects {
		switch o := obj.(type) {
		case *microflows.ExclusiveSplit:
			splits = append(splits, o)
		case *microflows.Annotation:
			annotations = append(annotations, o)
		}
	}

	if len(splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(splits))
	}
	if len(annotations) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(annotations))
	}

	outerSplit, innerSplit := splits[0], splits[1]

	// AnnotationFlow links Annotation -> activity. Verify each flow points
	// from the annotation with the expected text to the expected split.
	var outerNoteDestID, innerNoteDestID string
	for _, af := range fb.annotationFlows {
		// Find the Annotation referenced by OriginID
		for _, ann := range annotations {
			if ann.ID != af.OriginID {
				continue
			}
			switch ann.Caption {
			case "Outer IF note":
				outerNoteDestID = string(af.DestinationID)
			case "Inner IF note":
				innerNoteDestID = string(af.DestinationID)
			}
		}
	}

	if outerNoteDestID != string(outerSplit.ID) {
		t.Errorf("outer note destination: got %q, want %q (outer split)", outerNoteDestID, outerSplit.ID)
	}
	if innerNoteDestID != string(innerSplit.ID) {
		t.Errorf("inner note destination: got %q, want %q (inner split)", innerNoteDestID, innerSplit.ID)
	}
}

func TestLoopBodyIfAnnotationPromotedToParentFlows(t *testing.T) {
	nestedIf := &ast.IfStmt{
		Condition: &ast.VariableExpr{Name: "IsActive"},
		ThenBody: []ast.MicroflowStatement{
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "active"}},
		},
		Annotations: &ast.ActivityAnnotations{
			AnnotationText: "Nested decision note",
		},
	}
	loop := &ast.LoopStmt{
		LoopVariable: "Item",
		ListVariable: "Items",
		Body:         []ast.MicroflowStatement{nestedIf},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"Items": "List of Synthetic.Item", "IsActive": "Boolean"},
		declaredVars: map[string]string{"Items": "List of Synthetic.Item", "IsActive": "Boolean"},
	}
	oc := fb.buildFlowGraph([]ast.MicroflowStatement{loop}, nil)

	var splitID model.ID
	for _, obj := range oc.Objects {
		loopObj, ok := obj.(*microflows.LoopedActivity)
		if !ok || loopObj.ObjectCollection == nil {
			continue
		}
		for _, nested := range loopObj.ObjectCollection.Objects {
			if split, ok := nested.(*microflows.ExclusiveSplit); ok {
				splitID = split.ID
			}
		}
	}
	if splitID == "" {
		t.Fatal("expected nested ExclusiveSplit inside loop body")
	}

	annotations := buildAnnotationsByTarget(oc)
	if got := annotations[splitID]; len(got) != 1 || got[0] != "Nested decision note" {
		t.Fatalf("annotations for nested split = %#v, want Nested decision note", got)
	}
}

// TestLoopCaptionPreserved covers the loop caption case — previously untested
// per PR review. The fix for the outer-IF caption contamination bug also applied
// the same snapshot/restore pattern to addLoopStatement and addWhileStatement.
func TestLoopCaptionPreserved(t *testing.T) {
	innerReturn := &ast.ReturnStmt{Value: &ast.LiteralExpr{Value: true, Kind: ast.LiteralBoolean}}
	loop := &ast.LoopStmt{
		LoopVariable: "item",
		ListVariable: "items",
		Body:         []ast.MicroflowStatement{innerReturn},
		Annotations:  &ast.ActivityAnnotations{Caption: "Process each item"},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"items": "List of MyMod.Item"},
		declaredVars: map[string]string{"items": "List of MyMod.Item"},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{loop}, nil)

	var loops []*microflows.LoopedActivity
	for _, obj := range fb.objects {
		if l, ok := obj.(*microflows.LoopedActivity); ok {
			loops = append(loops, l)
		}
	}

	if len(loops) != 1 {
		t.Fatalf("expected 1 LoopedActivity, got %d", len(loops))
	}
	if loops[0].Caption != "Process each item" {
		t.Errorf("loop caption: got %q, want %q", loops[0].Caption, "Process each item")
	}
}

// TestWhileLoopCaptionPreserved — same coverage for the WHILE shape.
func TestWhileLoopCaptionPreserved(t *testing.T) {
	whileStmt := &ast.WhileStmt{
		Condition: &ast.BinaryExpr{
			Left:     &ast.VariableExpr{Name: "n"},
			Operator: "<",
			Right:    &ast.LiteralExpr{Value: int64(10), Kind: ast.LiteralInteger},
		},
		Body: []ast.MicroflowStatement{
			&ast.ReturnStmt{Value: &ast.LiteralExpr{Value: true, Kind: ast.LiteralBoolean}},
		},
		Annotations: &ast.ActivityAnnotations{Caption: "Until n >= 10"},
	}

	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"n": "Integer"},
		declaredVars: map[string]string{"n": "Integer"},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{whileStmt}, nil)

	var loops []*microflows.LoopedActivity
	for _, obj := range fb.objects {
		if l, ok := obj.(*microflows.LoopedActivity); ok {
			loops = append(loops, l)
		}
	}

	if len(loops) != 1 {
		t.Fatalf("expected 1 LoopedActivity (WHILE), got %d", len(loops))
	}
	if loops[0].Caption != "Until n >= 10" {
		t.Errorf("while caption: got %q, want %q", loops[0].Caption, "Until n >= 10")
	}
}

// TestInheritanceSplitCaptionApplied — InheritanceSplit is not produced by the
// executor builder (only parsed from BSON for roundtrip), but applyAnnotations
// gained an InheritanceSplit case in the fix. Test the applicator directly.
func TestInheritanceSplitCaptionApplied(t *testing.T) {
	split := &microflows.InheritanceSplit{}
	split.ID = "inh-split-1"

	fb := &flowBuilder{}
	fb.objects = append(fb.objects, split)

	fb.applyAnnotations(split.ID, &ast.ActivityAnnotations{Caption: "Customer type?"})

	if split.Caption != "Customer type?" {
		t.Errorf("inheritance split caption: got %q, want %q", split.Caption, "Customer type?")
	}
}

func TestFreeAnnotationBeforePositionStaysUnattached(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "message"},
			Annotations: &ast.ActivityAnnotations{
				FreeAnnotations: []string{"free synthetic note"},
				Position:        &ast.Position{X: 120, Y: 240},
			},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
	oc := fb.buildFlowGraph(body, nil)

	freeAnnotations := collectFreeAnnotations(oc)
	if len(freeAnnotations) != 1 || freeAnnotations[0] != "free synthetic note" {
		t.Fatalf("free annotations = %#v, want one free note", freeAnnotations)
	}

	attached := buildAnnotationsByTarget(oc)
	for activityID, captions := range attached {
		for _, caption := range captions {
			if caption == "free synthetic note" {
				t.Fatalf("free note was attached to activity %s", activityID)
			}
		}
	}
}

func TestMultipleFreeAnnotationsBeforePositionStayUnattached(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "message"},
			Annotations: &ast.ActivityAnnotations{
				FreeAnnotations: []string{"first free note", "second free note"},
				Position:        &ast.Position{X: 120, Y: 240},
			},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
	oc := fb.buildFlowGraph(body, nil)

	freeAnnotations := collectFreeAnnotations(oc)
	want := []string{"first free note", "second free note"}
	if len(freeAnnotations) != len(want) {
		t.Fatalf("free annotations = %#v, want %#v", freeAnnotations, want)
	}
	for i, wantText := range want {
		if freeAnnotations[i] != wantText {
			t.Fatalf("free annotation %d = %q, want %q", i, freeAnnotations[i], wantText)
		}
	}
}

func TestIfBranchActionCaptionStaysWithAction(t *testing.T) {
	ifStmt := &ast.IfStmt{
		Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
		ThenBody: []ast.MicroflowStatement{
			&ast.LogStmt{
				Level:       ast.LogInfo,
				Message:     &ast.LiteralExpr{Kind: ast.LiteralString, Value: "branch"},
				Annotations: &ast.ActivityAnnotations{Caption: "Branch activity"},
			},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
	fb.buildFlowGraph([]ast.MicroflowStatement{ifStmt}, nil)

	for _, obj := range fb.objects {
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		if _, ok := activity.Action.(*microflows.LogMessageAction); ok {
			if activity.Caption != "Branch activity" {
				t.Fatalf("branch activity caption = %q, want Branch activity", activity.Caption)
			}
			if activity.AutoGenerateCaption {
				t.Fatal("branch activity caption should not be autogenerated")
			}
			return
		}
	}
	t.Fatal("expected branch log activity")
}

func TestIfBranchActionAnnotationStaysWithAction(t *testing.T) {
	ifStmt := &ast.IfStmt{
		Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
		ThenBody: []ast.MicroflowStatement{
			&ast.LogStmt{
				Level:   ast.LogInfo,
				Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "branch"},
				Annotations: &ast.ActivityAnnotations{
					AnnotationText: "Branch note",
				},
			},
		},
	}

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
	oc := fb.buildFlowGraph([]ast.MicroflowStatement{ifStmt}, nil)

	var logID model.ID
	for _, obj := range oc.Objects {
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		if _, ok := activity.Action.(*microflows.LogMessageAction); ok {
			logID = activity.ID
			break
		}
	}
	if logID == "" {
		t.Fatal("expected branch log activity")
	}

	attached := buildAnnotationsByTarget(oc)
	if got := attached[logID]; len(got) != 1 || got[0] != "Branch note" {
		t.Fatalf("branch log annotations = %#v, want [Branch note]", got)
	}
}
