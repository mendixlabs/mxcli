// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

func TestShowWorkflows_Mock(t *testing.T) {
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "ApproveOrder")

	h := mkHierarchy(mod)
	withContainer(h, wf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listWorkflows(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "Qualified Name")
	assertContainsStr(t, out, "Sales.ApproveOrder")
}

func TestDescribeWorkflow_Mock(t *testing.T) {
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "ApproveOrder")
	wf.Parameter = &workflows.WorkflowParameter{EntityRef: "Sales.Order"}

	h := mkHierarchy(mod)
	withContainer(h, wf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeWorkflow(ctx, ast.QualifiedName{Module: "Sales", Name: "ApproveOrder"}))

	out := buf.String()
	assertContainsStr(t, out, "create workflow")
	assertContainsStr(t, out, "Sales.ApproveOrder")

	// Roundtrip: DESCRIBE output must be parseable as valid MDL (issue #478)
	_, parseErrs := visitor.Build(out)
	if len(parseErrs) > 0 {
		t.Errorf("describe workflow output is not valid MDL: %v\nOutput:\n%s", parseErrs[0], out)
	}
}

func TestDescribeWorkflow_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeWorkflow(ctx, ast.QualifiedName{Module: "X", Name: "NoSuch"}))
}

func TestShowWorkflows_FilterByModule(t *testing.T) {
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "ApproveOrder")

	h := mkHierarchy(mod)
	withContainer(h, wf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listWorkflows(ctx, "Sales"))
	assertContainsStr(t, buf.String(), "Sales.ApproveOrder")
}
