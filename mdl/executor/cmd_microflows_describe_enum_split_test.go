// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestTraverseFlow_EnumSplit(t *testing.T) {
	e := newTestExecutor()

	activityMap := map[model.ID]microflows.MicroflowObject{
		mkID("split"): &microflows.ExclusiveSplit{
			BaseMicroflowObject: mkObj("split"),
			SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$Status"},
		},
		mkID("open"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("open")},
			Action:       &microflows.LogMessageAction{LogLevel: "Info", MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "open"}}},
		},
		mkID("closed"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("closed")},
			Action:       &microflows.LogMessageAction{LogLevel: "Info", MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "closed"}}},
		},
		mkID("other"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("other")},
			Action:       &microflows.LogMessageAction{LogLevel: "Info", MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "other"}}},
		},
		mkID("merge"): &microflows.ExclusiveMerge{BaseMicroflowObject: mkObj("merge")},
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("split"): {
			mkBranchFlow("split", "open", microflows.EnumerationCase{Value: "Open"}),
			mkBranchFlow("split", "closed", microflows.EnumerationCase{Value: "Closed"}),
			mkFlow("split", "other"),
		},
		mkID("open"):   {mkFlow("open", "merge")},
		mkID("closed"): {mkFlow("closed", "merge")},
		mkID("other"):  {mkFlow("other", "merge")},
	}
	splitMergeMap := map[model.ID]model.ID{mkID("split"): mkID("merge")}

	var lines []string
	visited := make(map[model.ID]bool)
	e.traverseFlow(mkID("split"), activityMap, flowsByOrigin, splitMergeMap, visited, nil, nil, &lines, 1, nil, 0, nil)

	assertContainsAny(t, lines, "case $Status")
	assertContainsAny(t, lines, "when Open")
	assertContainsAny(t, lines, "when Closed")
	assertContainsAny(t, lines, "else")
	assertContainsAny(t, lines, "end case;")
}

func TestTraverseFlow_EnumSplitPreservesExplicitCaseOrder(t *testing.T) {
	e := newTestExecutor()

	activityMap := map[model.ID]microflows.MicroflowObject{
		mkID("split"): &microflows.ExclusiveSplit{
			BaseMicroflowObject: mkObj("split"),
			SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$Status"},
		},
		mkID("empty"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("empty")},
			Action:       &microflows.LogMessageAction{LogLevel: "Info", MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "empty"}}},
		},
		mkID("ready"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("ready")},
			Action:       &microflows.LogMessageAction{LogLevel: "Info", MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "ready"}}},
		},
		mkID("blocked"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("blocked")},
			Action:       &microflows.LogMessageAction{LogLevel: "Info", MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "blocked"}}},
		},
		mkID("merge"): &microflows.ExclusiveMerge{BaseMicroflowObject: mkObj("merge")},
	}
	emptyFlow := mkBranchFlow("split", "empty", microflows.EnumerationCase{Value: ""})
	readyFlow := mkBranchFlow("split", "ready", microflows.EnumerationCase{Value: "Ready"})
	blockedFlow := mkBranchFlow("split", "blocked", microflows.EnumerationCase{Value: "Blocked"})
	applySplitCaseOrder(emptyFlow, 0)
	applySplitCaseOrder(readyFlow, 1)
	applySplitCaseOrder(blockedFlow, 2)
	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("split"):   {blockedFlow, readyFlow, emptyFlow},
		mkID("empty"):   {mkFlow("empty", "merge")},
		mkID("ready"):   {mkFlow("ready", "merge")},
		mkID("blocked"): {mkFlow("blocked", "merge")},
	}
	splitMergeMap := map[model.ID]model.ID{mkID("split"): mkID("merge")}

	var lines []string
	visited := make(map[model.ID]bool)
	e.traverseFlow(mkID("split"), activityMap, flowsByOrigin, splitMergeMap, visited, nil, nil, &lines, 1, nil, 0, nil)

	out := strings.Join(lines, "\n")
	emptyIdx := strings.Index(out, "when (empty)")
	readyIdx := strings.Index(out, "when Ready")
	blockedIdx := strings.Index(out, "when Blocked")
	if emptyIdx == -1 || readyIdx == -1 || blockedIdx == -1 {
		t.Fatalf("missing expected cases:\n%s", out)
	}
	if !(emptyIdx < readyIdx && readyIdx < blockedIdx) {
		t.Fatalf("case order was not preserved:\n%s", out)
	}
}

func TestEnumSplitRoundtripPreservesEmptyCaseBeforeNamedCases(t *testing.T) {
	out := describeBuiltEnumSplitBody(t, []ast.MicroflowStatement{
		&ast.EnumSplitStmt{
			Variable: "ImageKind",
			Cases: []ast.EnumSplitCase{
				{
					Value: "(empty)",
					Body: []ast.MicroflowStatement{
						&ast.LogStmt{
							Level:       ast.LogError,
							Message:     &ast.LiteralExpr{Kind: ast.LiteralString, Value: "missing kind"},
							Annotations: &ast.ActivityAnnotations{Position: &ast.Position{X: -300, Y: -200}},
						},
					},
				},
				{
					Value: "Cover",
					Body: []ast.MicroflowStatement{
						&ast.LogStmt{
							Level:       ast.LogInfo,
							Message:     &ast.LiteralExpr{Kind: ast.LiteralString, Value: "cover"},
							Annotations: &ast.ActivityAnnotations{Position: &ast.Position{X: -300, Y: 100}},
						},
					},
				},
				{
					Value: "Logo",
					Body: []ast.MicroflowStatement{
						&ast.LogStmt{
							Level:       ast.LogInfo,
							Message:     &ast.LiteralExpr{Kind: ast.LiteralString, Value: "logo"},
							Annotations: &ast.ActivityAnnotations{Position: &ast.Position{X: -300, Y: -40}},
						},
					},
				},
			},
		},
	})

	assertOrder(t, out, "when (empty)", "when Cover", "when Logo")
}

func TestEnumSplitRoundtripPreservesEmptyGroupedCaseBeforeTerminalCase(t *testing.T) {
	out := describeBuiltEnumSplitBody(t, []ast.MicroflowStatement{
		&ast.EnumSplitStmt{
			Variable: "Event/Type",
			Cases: []ast.EnumSplitCase{
				{
					Values: []string{"CREATE", "DELETE"},
				},
				{
					Value: "UPDATE",
					Body: []ast.MicroflowStatement{
						&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "update"}},
						&ast.ReturnStmt{},
					},
				},
				{
					Value: "(empty)",
					Body: []ast.MicroflowStatement{
						&ast.LogStmt{
							Level:   ast.LogInfo,
							Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "empty"},
							Annotations: &ast.ActivityAnnotations{
								Anchor: &ast.FlowAnchors{From: ast.AnchorSideBottom, To: ast.AnchorSideTop},
							},
						},
						&ast.ReturnStmt{
							Annotations: &ast.ActivityAnnotations{
								Anchor: &ast.FlowAnchors{To: ast.AnchorSideTop},
							},
						},
					},
				},
			},
		},
		&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "shared tail"}},
	})

	assertOrder(t, out, "when CREATE, DELETE", "when UPDATE", "when (empty)")
}

func TestEnumSplitRoundtripKeepsTerminalNestedIfElseInsideCase(t *testing.T) {
	out := describeBuiltEnumSplitBody(t, []ast.MicroflowStatement{
		&ast.EnumSplitStmt{
			Variable: "SubjectType",
			Cases: []ast.EnumSplitCase{
				{
					Value: "member",
					Body: []ast.MicroflowStatement{
						&ast.IfStmt{
							Condition: &ast.VariableExpr{Name: "Member"},
							Annotations: &ast.ActivityAnnotations{
								FalseBranchAnchor: &ast.FlowAnchors{From: ast.AnchorSideTop, To: ast.AnchorSideBottom},
							},
							ThenBody: []ast.MicroflowStatement{
								&ast.IfStmt{
									Condition: &ast.VariableExpr{Name: "Company"},
									ThenBody: []ast.MicroflowStatement{
										&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "member ok"}},
										&ast.ReturnStmt{},
									},
									ElseBody: []ast.MicroflowStatement{
										&ast.LogStmt{Level: ast.LogError, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "missing company"}},
										&ast.ReturnStmt{},
									},
								},
							},
							ElseBody: []ast.MicroflowStatement{
								&ast.LogStmt{Level: ast.LogError, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "missing member"}},
								&ast.ReturnStmt{},
							},
						},
					},
				},
				{
					Value: "app",
					Body: []ast.MicroflowStatement{
						&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "app branch"}},
						&ast.ReturnStmt{},
					},
				},
			},
		},
	})

	memberCase := strings.Index(out, "when member")
	appCase := strings.Index(out, "when app")
	missingMember := strings.Index(out, "missing member")
	if memberCase == -1 || appCase == -1 || missingMember == -1 {
		t.Fatalf("missing expected enum case output:\n%s", out)
	}
	if missingMember > appCase {
		t.Fatalf("member ELSE body leaked outside the member case:\n%s", out)
	}
	if !strings.Contains(out[memberCase:appCase], "else") || !strings.Contains(out[memberCase:appCase], "missing member") {
		t.Fatalf("member ELSE body must stay inside the nested IF:\n%s", out)
	}
}

func describeBuiltEnumSplitBody(t *testing.T, body []ast.MicroflowStatement) string {
	t.Helper()

	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
	oc := fb.buildFlowGraph(body, nil)
	mf := &microflows.Microflow{ObjectCollection: oc}
	e := newTestExecutor()

	return strings.Join(formatMicroflowActivities(e.newExecContext(t.Context()), mf, nil, nil), "\n")
}

func assertOrder(t *testing.T, out string, items ...string) {
	t.Helper()

	lastIdx := -1
	for _, item := range items {
		idx := strings.Index(out, item)
		if idx == -1 {
			t.Fatalf("missing %q in output:\n%s", item, out)
		}
		if idx < lastIdx {
			t.Fatalf("expected %q after previous item in output:\n%s", item, out)
		}
		lastIdx = idx
	}
}

func assertContainsAny(t *testing.T, lines []string, want string) {
	t.Helper()
	for _, line := range lines {
		if contains(line, want) {
			return
		}
	}
	t.Fatalf("Expected output to contain %q, got %v", want, lines)
}
