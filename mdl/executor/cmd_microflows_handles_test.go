// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// handlesFixture is a flow with a duplicated output variable (so describe
// prepends a warning above the body) and a statement that occurs twice (so two
// handles need an ordinal).
func handlesFixture() *microflows.Microflow {
	act := func(id string, x int, a microflows.MicroflowAction) *microflows.ActionActivity {
		return &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{
				BaseMicroflowObject: microflows.BaseMicroflowObject{
					BaseElement: model.BaseElement{ID: model.ID(id)},
					Position:    model.Point{X: x, Y: 100},
				},
				AutoGenerateCaption: true,
			},
			Action: a,
		}
	}
	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: "split"},
			Position:    model.Point{X: 400, Y: 100},
		},
		Caption:        "Enough?",
		SplitCondition: &microflows.ExpressionSplitCondition{Expression: "$N > 1"},
	}
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.StartEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "start"}, Position: model.Point{X: 0, Y: 100}}},
			act("item1", 100, &microflows.CreateObjectAction{OutputVariable: "Item", EntityQualifiedName: "Synthetic.Item"}),
			act("item2", 200, &microflows.CreateObjectAction{OutputVariable: "Item", EntityQualifiedName: "Synthetic.Item"}),
			act("declare", 250, &microflows.CreateVariableAction{VariableName: "N", DataType: &microflows.IntegerType{}, InitialValue: "0"}),
			act("set1", 300, &microflows.ChangeVariableAction{VariableName: "N", Value: "$N + 1"}),
			split,
			act("set2", 500, &microflows.ChangeVariableAction{VariableName: "N", Value: "$N + 1"}),
			&microflows.ExclusiveMerge{BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "merge"}, Position: model.Point{X: 600, Y: 100}}},
			&microflows.EndEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "end"}, Position: model.Point{X: 700, Y: 100}}},
		},
		Flows: []*microflows.SequenceFlow{
			{OriginID: "start", DestinationID: "item1"},
			{OriginID: "item1", DestinationID: "item2"},
			{OriginID: "item2", DestinationID: "declare"},
			{OriginID: "declare", DestinationID: "set1"},
			{OriginID: "set1", DestinationID: "split"},
			{OriginID: "split", DestinationID: "set2", CaseValue: &microflows.ExpressionCase{Expression: "true"}},
			{OriginID: "split", DestinationID: "merge", CaseValue: &microflows.ExpressionCase{Expression: "false"}, OriginConnectionIndex: 2},
			{OriginID: "set2", DestinationID: "merge"},
			{OriginID: "merge", DestinationID: "end"},
		},
	}
	return &microflows.Microflow{ObjectCollection: oc}
}

// Each handle line sits directly above the activity it names — the warning
// describe prepends must not shift the handles off their statements — and
// resolves back to that activity.
func TestFormatMicroflowActivitiesWithHandles_AboveEachActivity(t *testing.T) {
	ctx := &ExecContext{}
	mf := handlesFixture()
	lines := formatMicroflowActivitiesWithHandles(ctx, mf, nil, nil)
	got := strings.Join(lines, "\n")
	if !strings.Contains(got, "-- WARNING: duplicate output variable $Item") {
		t.Fatalf("fixture should trigger the duplicate-output warning:\n%s", got)
	}

	cands, _, _, _ := microflowTargets(ctx, mf, nil, nil)
	wantAbove := map[string]string{
		"$Item @1":           "(100, 100)",
		"$Item @2":           "(200, 100)",
		"$N":                 "(250, 100)",
		"set $N = $N + 1 @1": "(300, 100)",
		"'Enough?'":          "(400, 100)",
		"set $N = $N + 1 @2": "(500, 100)",
	}
	seen := map[string]bool{}
	for i, line := range lines {
		h, ok := strings.CutPrefix(strings.TrimSpace(line), "-- handle: ")
		if !ok {
			continue
		}
		seen[h] = true
		if i+1 >= len(lines) || !strings.Contains(lines[i+1], "@position"+wantAbove[h]) {
			t.Errorf("handle %q is not directly above the activity at %s:\n%s", h, wantAbove[h], got)
		}
		c, err := mfmutator.ResolveText(cands, h)
		if err != nil {
			t.Errorf("handle %q does not resolve: %v", h, err)
			continue
		}
		p := c.Object.GetPosition()
		if want := wantAbove[h]; want != "" && want != fmt.Sprintf("(%d, %d)", p.X, p.Y) {
			t.Errorf("handle %q resolves to the activity at (%d, %d), want %s", h, p.X, p.Y, want)
		}
	}
	for h := range wantAbove {
		if !seen[h] {
			t.Errorf("missing handle %q:\n%s", h, got)
		}
	}

	// Control: take the handle lines away and what remains is exactly the
	// plain description, so `with handles` adds comments and changes nothing.
	plain := formatMicroflowActivities(ctx, handlesFixture(), nil, nil)
	var stripped []string
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "-- handle: ") {
			stripped = append(stripped, line)
		}
	}
	if strings.Join(stripped, "\n") != strings.Join(plain, "\n") {
		t.Errorf("with handles minus the handle lines differs from plain describe:\n--- with handles\n%s\n--- plain\n%s",
			strings.Join(stripped, "\n"), strings.Join(plain, "\n"))
	}
}

// The statement parses end to end: grammar, visitor and executor.
func TestDescribeMicroflowWithHandles_Statement(t *testing.T) {
	prog, errs := visitor.Build("describe microflow MyModule.ACT_Count with handles;")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	stmt, ok := prog.Statements[0].(*ast.DescribeStmt)
	if !ok || !stmt.WithHandles || stmt.Normalized {
		t.Fatalf("want a DescribeStmt with WithHandles, got %#v", prog.Statements[0])
	}

	mod := mkModule("MyModule")
	mf := handlesFixture()
	mf.BaseElement = model.BaseElement{ID: "mf"}
	mf.ContainerID = mod.ID
	mf.Name = "ACT_Count"
	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeMicroflowMode(ctx, stmt.Name, describeMicroflowOptions{Handles: true}))
	assertContainsStr(t, buf.String(), "  -- handle: 'Enough?'\n")

	// normalized shows a graph other than the stored one; its handles would
	// address nothing, so the combination is refused.
	if err := describeMicroflowMode(ctx, stmt.Name, describeMicroflowOptions{Handles: true, Normalized: true}); err == nil {
		t.Error("normalized with handles: want an error")
	}
}

// An if whose then-branch is empty is printed as stored (ako/mxcli#1963), so
// the stored condition is both the printed form and the handle.
func TestMicroflowTargets_EmptyThenIfIsAddressedAsStored(t *testing.T) {
	mf := handlesFixture()
	// Swap the branches: true now goes straight to the merge.
	for _, f := range mf.ObjectCollection.Flows {
		if f.OriginID == "split" {
			if f.DestinationID == "set2" {
				f.DestinationID = "merge"
			} else {
				f.DestinationID = "set2"
			}
		}
	}
	ctx := &ExecContext{}
	got := strings.Join(formatMicroflowActivities(ctx, mf, nil, nil), "\n")
	if !strings.Contains(got, "if $N > 1 then") || strings.Contains(got, "not(") {
		t.Fatalf("fixture should describe the split as stored:\n%s", got)
	}
	cands, _, _, _ := microflowTargets(ctx, mf, nil, nil)
	for _, target := range []string{"if $N > 1 then", "if * then"} {
		c, err := mfmutator.ResolveText(cands, target)
		if err != nil {
			t.Errorf("%s: %v", target, err)
			continue
		}
		if c.ID != "split" {
			t.Errorf("%s: resolved to %s, want split", target, c.ID)
		}
	}
	// The printed form is the one a handle shows.
	for i, c := range cands {
		if c.ID == "split" {
			c.Caption = ""
			cands[i] = c
			if h := mfmutator.Handle(cands, i); h != "if $N > 1 then" {
				t.Errorf("handle %q, want the printed form", h)
			}
		}
	}
}

// errorHandlerFixture is a flow whose custom error handler holds a statement
// that also occurs after the handled activity. Describe prints the handler's
// copy first, inside the `on error begin … end error` block.
func errorHandlerFixture() *microflows.Microflow {
	act := func(id string, x, y int, a microflows.MicroflowAction) *microflows.ActionActivity {
		return &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{
				BaseMicroflowObject: microflows.BaseMicroflowObject{
					BaseElement: model.BaseElement{ID: model.ID(id)},
					Position:    model.Point{X: x, Y: y},
				},
				AutoGenerateCaption: true,
			},
			Action: a,
		}
	}
	end := func(id string, x, y int) *microflows.EndEvent {
		return &microflows.EndEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(id)}, Position: model.Point{X: x, Y: y}}}
	}
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.StartEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "start"}, Position: model.Point{X: 0, Y: 100}}},
			act("declare", 100, 100, &microflows.CreateVariableAction{VariableName: "N", DataType: &microflows.IntegerType{}, InitialValue: "0"}),
			act("create", 200, 100, &microflows.CreateObjectAction{
				OutputVariable: "Obj", EntityQualifiedName: "Synthetic.Item",
				ErrorHandlingType: microflows.ErrorHandlingTypeCustomWithoutRollback,
			}),
			// Stored before the main-path copy, so storage order agrees with
			// describe order and cannot mask a ranking that ignores the handler.
			act("mset", 300, 100, &microflows.ChangeVariableAction{VariableName: "N", Value: "$N + 1"}),
			act("hset", 200, 250, &microflows.ChangeVariableAction{VariableName: "N", Value: "$N + 1"}),
			end("hend", 300, 250),
			end("end", 400, 100),
		},
		Flows: []*microflows.SequenceFlow{
			{OriginID: "start", DestinationID: "declare"},
			{OriginID: "declare", DestinationID: "create"},
			{OriginID: "create", DestinationID: "mset"},
			{OriginID: "create", DestinationID: "hset", IsErrorHandler: true},
			{OriginID: "hset", DestinationID: "hend"},
			{OriginID: "mset", DestinationID: "end"},
		},
	}
	return &microflows.Microflow{ObjectCollection: oc}
}

// An activity inside an `on error begin … end error` block is printed, so it gets a handle
// directly above it, and ordinals count it where it is printed: before the
// main-path copy that follows the block. Ranking handler bodies after
// everything else would make `@1` pick the activity a reader counts second.
func TestDescribeWithHandles_ErrorHandlerBody(t *testing.T) {
	ctx := &ExecContext{}
	lines := formatMicroflowActivitiesWithHandles(ctx, errorHandlerFixture(), nil, nil)
	got := strings.Join(lines, "\n")

	var setLines []int
	for i, line := range lines {
		if strings.TrimSpace(line) == "set $N = $N + 1;" {
			setLines = append(setLines, i)
		}
	}
	if len(setLines) != 2 || !strings.Contains(got, "on error without rollback begin") {
		t.Fatalf("fixture should print the handler's set inside the block, then the main one:\n%s", got)
	}

	cands, _, _, _ := microflowTargets(ctx, errorHandlerFixture(), nil, nil)
	for i, wantID := range []model.ID{"hset", "mset"} {
		target := fmt.Sprintf("set $N = $N + 1 @%d", i+1)
		c, err := mfmutator.ResolveText(cands, target)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if c.ID != wantID {
			t.Errorf("%s resolves to %s, want %s (the %s one printed)", target, c.ID, wantID, []string{"first", "second"}[i])
		}
		// The handle naming it sits directly above the printed statement.
		above := strings.TrimSpace(lines[setLines[i]-1])
		for j := setLines[i] - 1; j >= 0 && strings.HasPrefix(strings.TrimSpace(lines[j]), "@"); j-- {
			above = strings.TrimSpace(lines[j-1])
		}
		if above != "-- handle: "+target {
			t.Errorf("line above the %s set is %q, want the handle %q:\n%s", wantID, above, target, got)
		}
	}

	// Control: the handles are the only addition.
	var stripped []string
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "-- handle: ") {
			stripped = append(stripped, line)
		}
	}
	if plain := formatMicroflowActivities(ctx, errorHandlerFixture(), nil, nil); strings.Join(stripped, "\n") != strings.Join(plain, "\n") {
		t.Errorf("with handles minus the handle lines differs from plain describe:\n%s\n---\n%s", strings.Join(stripped, "\n"), strings.Join(plain, "\n"))
	}
}

// A handle has to be writable as an `alter microflow` target, and a target
// names the activity and ends before any fragment. So the `begin` describe
// prints after an activity with a custom error handler, and an empty handler's
// whole `begin end error`, are not part of its statement.
func TestPrintedStatement_ErrorHandlerBlockOpenerIsNotPartOfTheStatement(t *testing.T) {
	for _, body := range [][]string{
		{"  commit $Order on error begin", "    return false;", "  end error;"},
		{"  commit $Order on error begin end error;"},
	} {
		obj := &microflows.ActionActivity{}
		got := printedStatement(obj, body, elkSourceRange{StartLine: 0, EndLine: len(body) - 1})
		if got != "commit $Order on error" {
			t.Errorf("%q: printed statement %q, want %q", body[0], got, "commit $Order on error")
		}
		if _, err := mfmutator.ParseTarget(got); err != nil {
			t.Errorf("the handle does not parse as a target: %v", err)
		}
	}
}
