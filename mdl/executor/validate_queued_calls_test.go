// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// storedCall builds the shape a stored MicroflowCall has once a task queue is
// bound to it in Studio Pro. The binding lives two levels down (Queues$QueueSettings
// inside the call's QueueSettings property), so the walk has to be recursive.
func storedCall(queueSettings map[string]any, topLevelQueue any) map[string]any {
	return map[string]any{
		"$Type": "Microflows$Microflow",
		"ObjectCollection": map[string]any{
			"Objects": []any{
				map[string]any{
					"$Type": "Microflows$ActionActivity",
					"Action": map[string]any{
						"$Type":         "Microflows$MicroflowCall",
						"Microflow":     "Q.Target",
						"Queue":         topLevelQueue,
						"QueueSettings": queueSettings,
					},
				},
			},
		},
	}
}

func TestQueuedCallTargets_FindsQueueSettings(t *testing.T) {
	got := queuedCallTargets(storedCall(map[string]any{
		"$Type": "Queues$QueueSettings",
		"Queue": "Q.MyQueue",
	}, nil))
	if len(got) != 1 || got[0] != "Q.MyQueue" {
		t.Fatalf("queuedCallTargets = %v, want [Q.MyQueue]", got)
	}
}

func TestQueuedCallTargets_NoBinding(t *testing.T) {
	if got := queuedCallTargets(storedCall(nil, nil)); len(got) != 0 {
		t.Fatalf("queuedCallTargets = %v, want none for an unqueued call", got)
	}
}

// A call can carry a bare Queue with QueueSettings null. Measured on 11.13 that
// is inert (mx check does not complain), but it is still authored state, and a
// rewrite would drop it.
func TestQueuedCallTargets_BareQueue(t *testing.T) {
	got := queuedCallTargets(storedCall(nil, "Q.MyQueue"))
	if len(got) != 1 || got[0] != "Q.MyQueue" {
		t.Fatalf("queuedCallTargets = %v, want [Q.MyQueue]", got)
	}
}

func TestQueuedCallTargets_Dedupes(t *testing.T) {
	doc := map[string]any{"Objects": []any{
		map[string]any{"QueueSettings": map[string]any{"Queue": "Q.A"}},
		map[string]any{"QueueSettings": map[string]any{"Queue": "Q.A"}},
		map[string]any{"QueueSettings": map[string]any{"Queue": "Q.B"}},
	}}
	got := queuedCallTargets(doc)
	if len(got) != 2 {
		t.Fatalf("queuedCallTargets = %v, want 2 distinct queues", got)
	}
}

// TestCheckNoQueuedCalls_Refuses is the guard against the data loss itself: a
// CREATE OR REPLACE of a microflow with a queued call used to succeed and write
// QueueSettings back as null, so mx check went from CE1613 to 0 errors by
// deleting the user's configuration.
func TestCheckNoQueuedCalls_Refuses(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedCall(map[string]any{"Queue": "Q.MyQueue"}, nil), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", nil)
	if err == nil {
		t.Fatal("expected a refusal for a microflow with a queued call")
	}
	msg := err.Error()
	for _, want := range []string{"Q.ACT_Caller", "Q.MyQueue", "task queue"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
}

func TestCheckNoQueuedCalls_AllowsUnqueued(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedCall(nil, nil), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	if err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", nil); err != nil {
		t.Fatalf("unqueued microflow must still be rewritable: %v", err)
	}
}

// An unreadable unit is not this guard's business — the rewrite path reports its
// own errors, and failing here would block writes for an unrelated reason.
func TestCheckNoQueuedCalls_UnreadableUnitDoesNotBlock(t *testing.T) {
	ctx, _ := newMockCtx(t) // default mock: GetRawUnit is not configured, so it errors
	if err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", nil); err != nil {
		t.Fatalf("unreadable unit must not block the write: %v", err)
	}
}

// TestCreateOrModifyMicroflow_RefusesQueuedCall drives the guard through the
// real CREATE OR MODIFY path. This is the test that fails if the call site in
// execCreateMicroflow is removed — checkNoQueuedCalls passing on its own proves
// nothing about whether anything calls it.
func TestCreateOrModifyMicroflow_RefusesQueuedCall(t *testing.T) {
	mod := mkModule("Q")
	h := mkHierarchy(mod)

	existing := &microflows.Microflow{Name: "ACT_Caller"}
	existing.ID = "mf-existing"
	existing.ContainerID = mod.ID

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{existing}, nil
		},
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedCall(map[string]any{"Queue": "Q.MyQueue"}, nil), nil
		},
		UpdateMicroflowFunc: func(mf *microflows.Microflow) error {
			t.Error("UpdateMicroflow must not run: the rewrite would drop the queue binding")
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateMicroflow(ctx, &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "Q", Name: "ACT_Caller"},
		CreateOrModify: true,
	})
	if err == nil {
		t.Fatal("expected a refusal, got a successful rewrite")
	}
	if !strings.Contains(err.Error(), "Q.MyQueue") {
		t.Errorf("error should name the queue that would be lost:\n%s", err)
	}
}

// `IN QUEUE` makes the binding authorable, so the guard changes shape: a script
// that restates every stored queue is the normal way to edit a microflow with a
// queued call and must go through. Before the clause existed, every such rewrite
// was refused because there was nothing to restate it with.
func TestCheckNoQueuedCalls_AllowsWhenScriptRestatesQueue(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedCall(map[string]any{"Queue": "Q.MyQueue"}, nil), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	restating := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Q", Name: "ACT_Caller"},
		Body: []ast.MicroflowStatement{
			&ast.CallMicroflowStmt{
				MicroflowName: ast.QualifiedName{Module: "Q", Name: "Target"},
				Queue:         &ast.QualifiedName{Module: "Q", Name: "MyQueue"},
			},
		},
	}
	if err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", restating); err != nil {
		t.Fatalf("a script that restates the queue must be allowed: %v", err)
	}

	// Dropping it is still refused, and the message must name the clause that
	// fixes it — the whole point of the guard is that the loss is invisible.
	dropping := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Q", Name: "ACT_Caller"},
		Body: []ast.MicroflowStatement{
			&ast.CallMicroflowStmt{MicroflowName: ast.QualifiedName{Module: "Q", Name: "Target"}},
		},
	}
	err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", dropping)
	if err == nil {
		t.Fatal("a rewrite that drops the binding must still be refused")
	}
	for _, want := range []string{"Q.MyQueue", "IN QUEUE"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message missing %q:\n%s", want, err.Error())
		}
	}
}

// A call nested inside IF/LOOP/error-handler bodies still counts as restating.
// The walk is reflective precisely so a newly added nesting cannot silently stop
// being searched — a hand-written switch would.
func TestAuthoredQueueTargets_FindsNestedCalls(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Body: []ast.MicroflowStatement{
			&ast.IfStmt{
				ThenBody: []ast.MicroflowStatement{
					&ast.LoopStmt{
						Body: []ast.MicroflowStatement{
							&ast.CallJavaActionStmt{
								ActionName: ast.QualifiedName{Module: "Q", Name: "Work"},
								Queue:      &ast.QualifiedName{Module: "Q", Name: "Deep"},
							},
						},
					},
				},
			},
		},
	}
	got := authoredQueueTargets(stmt)
	if !got["q.deep"] {
		t.Fatalf("nested IN QUEUE not found: %v", got)
	}
}

// Restating the queue without the retry is not enough — the rewrite would
// reset the retry. That must still refuse, and say how to restate it.
func TestCheckNoQueuedCalls_RefusesStoredRetry(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedCall(map[string]any{
				"$Type": "Queues$QueueSettings",
				"Queue": "Q.MyQueue",
				"Retry": map[string]any{"$Type": "Queues$QueueFixedRetry", "Retries": 3},
			}, nil), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	restating := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Q", Name: "ACT_Caller"},
		Body: []ast.MicroflowStatement{
			&ast.CallMicroflowStmt{
				MicroflowName: ast.QualifiedName{Module: "Q", Name: "Target"},
				Queue:         &ast.QualifiedName{Module: "Q", Name: "MyQueue"},
			},
		},
	}
	err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", restating)
	if err == nil {
		t.Fatal("a stored retry policy must refuse the rewrite even when the queue is restated")
	}
	if !strings.Contains(err.Error(), "retry") {
		t.Errorf("error should name the retry:\n%s", err.Error())
	}
}

// TestQueuedCallTargets_HandlesBsonArrays is the engine-parity guard.
//
// The two backends hand GetRawUnit back in different shapes: the modelsdk
// reader yields `[]interface{}` for arrays, the legacy (mpr) reader yields
// `bson.A`. `bson.A` is a NAMED slice type, so `case []any:` does not match it
// — the walk never descended into ObjectCollection.Objects under legacy, found
// no queued calls, and let the rewrite through.
//
// The consequence was the exact data loss this guard exists to prevent, still
// live on `--engine legacy`: measured on 11.13, a Studio-Pro-made queue binding
// was silently dropped by `CREATE OR REPLACE MICROFLOW`, and `mx check` went
// quiet because the configuration its CE1613 referred to had been deleted.
func TestQueuedCallTargets_HandlesBsonArrays(t *testing.T) {
	call := map[string]any{
		"$Type":     "Microflows$MicroflowCall",
		"Microflow": "Q.Target",
		"QueueSettings": map[string]any{
			"$Type": "Queues$QueueSettings",
			"Queue": "Q.MyQueue",
		},
	}
	activity := map[string]any{"$Type": "Microflows$ActionActivity", "Action": call}

	// Same document, the two shapes the two readers produce.
	shapes := map[string]any{
		"modelsdk ([]any)": map[string]any{
			"ObjectCollection": map[string]any{"Objects": []any{activity}},
		},
		"legacy (bson.A)": map[string]any{
			"ObjectCollection": map[string]any{"Objects": bson.A{activity}},
		},
	}

	for name, doc := range shapes {
		t.Run(name, func(t *testing.T) {
			got := queuedCallTargets(doc)
			if len(got) != 1 || got[0] != "Q.MyQueue" {
				t.Fatalf("queuedCallTargets = %v, want [Q.MyQueue] — a rewrite would "+
					"silently drop the binding on this engine", got)
			}
		})
	}
}

// The retry walk shares the traversal and so shares the blind spot.
func TestStoredQueueRetries_HandlesBsonArrays(t *testing.T) {
	doc := map[string]any{"Objects": bson.A{map[string]any{
		"QueueSettings": map[string]any{
			"Queue": "Q.MyQueue",
			"Retry": map[string]any{"$Type": "Queues$QueueFixedRetry", "Retries": 3},
		},
	}}}
	if got := storedQueueRetries(doc); len(got) != 1 {
		t.Fatalf("storedQueueRetries = %v, want [Q.MyQueue]", got)
	}
}

// mendixlabs/mxcli#1729: a flow with a retry was locked for MDL. A script that
// restates a retry on that queue is saying what it wants, and goes through.
func TestCheckNoQueuedCalls_AllowsRestatedRetry(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedCall(map[string]any{
				"$Type": "Queues$QueueSettings",
				"Queue": "Q.MyQueue",
				"Retry": map[string]any{"$Type": "Queues$QueueFixedRetry", "Retries": 3},
			}, nil), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Q", Name: "ACT_Caller"},
		Body: []ast.MicroflowStatement{
			&ast.IfStmt{ThenBody: []ast.MicroflowStatement{ // nested: the walk must reach it
				&ast.CallMicroflowStmt{
					MicroflowName: ast.QualifiedName{Module: "Q", Name: "Target"},
					Queue:         &ast.QualifiedName{Module: "Q", Name: "MyQueue"},
					QueueRetry:    &ast.QueueRetry{Kind: "Fixed", Retries: 3, Interval: 10, IntervalType: "Minutes"},
				},
			}},
		},
	}
	if err := checkNoQueuedCalls(ctx, "mf-1", "Q.ACT_Caller", stmt); err != nil {
		t.Fatalf("a script restating the retry must be allowed: %v", err)
	}
}

// The splice path calls checkNoDroppedQueueRetries directly: a call still bound
// to the queue without its retry is refused; a script that removed every call
// to that queue has said so, and is let through.
func TestCheckNoDroppedQueueRetries(t *testing.T) {
	raw := storedCall(map[string]any{
		"$Type": "Queues$QueueSettings",
		"Queue": "Q.MyQueue",
		"Retry": map[string]any{"$Type": "Queues$QueueExponentialRetry", "Retries": 3},
	}, nil)
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{IsConnectedFunc: func() bool { return true }}))
	call := func(retry *ast.QueueRetry) *ast.CreateMicroflowStmt {
		return &ast.CreateMicroflowStmt{Body: []ast.MicroflowStatement{&ast.CallMicroflowStmt{
			MicroflowName: ast.QualifiedName{Module: "Q", Name: "Target"},
			Queue:         &ast.QualifiedName{Module: "Q", Name: "MyQueue"},
			QueueRetry:    retry,
		}}}
	}
	if err := checkNoDroppedQueueRetries(ctx, "mf", "Q.F", call(nil), raw); err == nil {
		t.Error("a call restated without its stored retry must be refused")
	}
	if err := checkNoDroppedQueueRetries(ctx, "mf", "Q.F",
		call(&ast.QueueRetry{Kind: "Fixed", Retries: 1, Interval: 1, IntervalType: "Hours"}), raw); err != nil {
		t.Errorf("a changed, stated retry must be allowed: %v", err)
	}
	if err := checkNoDroppedQueueRetries(ctx, "mf", "Q.F", &ast.CreateMicroflowStmt{}, raw); err != nil {
		t.Errorf("removing the queued call must be allowed: %v", err)
	}
}
