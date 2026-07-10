// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// execAlterWorkflow handles ALTER WORKFLOW Module.Name { operations }.
func execAlterWorkflow(ctx *ExecContext, s *ast.AlterWorkflowStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Version pre-check: workflows require Mendix 9.12+
	if err := checkFeature(ctx, "workflows", "basic",
		"alter workflow",
		"upgrade your project to Mendix 9.12+ to use workflows"); err != nil {
		return err
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find workflow by qualified name
	allWorkflows, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return mdlerrors.NewBackend("list workflows", err)
	}

	var wfID model.ID
	for _, wf := range allWorkflows {
		modID := h.FindModuleID(wf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && wf.Name == s.Name.Name {
			wfID = wf.ID
			break
		}
	}
	if wfID == "" {
		return mdlerrors.NewNotFound("workflow", s.Name.Module+"."+s.Name.Name)
	}

	// Open mutator
	mutator, err := ctx.Backend.OpenWorkflowForMutation(wfID)
	if err != nil {
		return mdlerrors.NewBackend("open workflow for mutation", err)
	}

	// Apply operations sequentially
	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.SetWorkflowPropertyOp:
			switch o.Property {
			case "overview_page":
				// overview_page uses Entity as the page qualified name (Value is unused).
				qn := o.Entity.Module + "." + o.Entity.Name
				if qn == "." {
					qn = ""
				}
				if err := mutator.SetPropertyWithEntity(o.Property, qn, qn); err != nil {
					return mdlerrors.NewBackend("set "+o.Property, err)
				}
			case "parameter":
				// PARAMETER uses Value as the variable name and Entity as the entity qualified name.
				qn := o.Entity.Module + "." + o.Entity.Name
				if qn == "." {
					qn = ""
				}
				if err := mutator.SetPropertyWithEntity(o.Property, o.Value, qn); err != nil {
					return mdlerrors.NewBackend("set "+o.Property, err)
				}
			default:
				if err := mutator.SetProperty(o.Property, o.Value); err != nil {
					return mdlerrors.NewBackend("set "+o.Property, err)
				}
			}

		case *ast.SetActivityPropertyOp:
			value := o.Value
			switch o.Property {
			case "page":
				value = o.PageName.Module + "." + o.PageName.Name
			case "targeting_microflow":
				value = o.Microflow.Module + "." + o.Microflow.Name
			}
			if err := mutator.SetActivityProperty(o.ActivityRef, o.AtPosition, o.Property, value); err != nil {
				return mdlerrors.NewBackend("set activity", err)
			}

		case *ast.InsertAfterOp:
			acts := buildAndBindActivities(ctx, []ast.WorkflowActivityNode{o.NewActivity})
			if len(acts) == 0 {
				return mdlerrors.NewValidation("failed to build new activity")
			}
			if err := mutator.InsertAfterActivity(o.ActivityRef, o.AtPosition, acts); err != nil {
				return mdlerrors.NewBackend("insert after", err)
			}

		case *ast.DropActivityOp:
			if err := mutator.DropActivity(o.ActivityRef, o.AtPosition); err != nil {
				return mdlerrors.NewBackend("drop activity", err)
			}

		case *ast.ReplaceActivityOp:
			acts := buildAndBindActivities(ctx, []ast.WorkflowActivityNode{o.NewActivity})
			if len(acts) == 0 {
				return mdlerrors.NewValidation("failed to build replacement activity")
			}
			if err := mutator.ReplaceActivity(o.ActivityRef, o.AtPosition, acts); err != nil {
				return mdlerrors.NewBackend("replace activity", err)
			}

		case *ast.InsertOutcomeOp:
			acts := buildAndBindActivities(ctx, o.Activities)
			if err := mutator.InsertOutcome(o.ActivityRef, o.AtPosition, o.OutcomeName, acts); err != nil {
				return mdlerrors.NewBackend("insert outcome", err)
			}

		case *ast.DropOutcomeOp:
			if err := mutator.DropOutcome(o.ActivityRef, o.AtPosition, o.OutcomeName); err != nil {
				return mdlerrors.NewBackend("drop outcome", err)
			}

		case *ast.InsertPathOp:
			acts := buildAndBindActivities(ctx, o.Activities)
			if err := mutator.InsertPath(o.ActivityRef, o.AtPosition, "", acts); err != nil {
				return mdlerrors.NewBackend("insert path", err)
			}

		case *ast.DropPathOp:
			if err := mutator.DropPath(o.ActivityRef, o.AtPosition, o.PathCaption); err != nil {
				return mdlerrors.NewBackend("drop path", err)
			}

		case *ast.InsertBranchOp:
			acts := buildAndBindActivities(ctx, o.Activities)
			if err := mutator.InsertBranch(o.ActivityRef, o.AtPosition, o.Condition, acts); err != nil {
				return mdlerrors.NewBackend("insert branch", err)
			}

		case *ast.DropBranchOp:
			if err := mutator.DropBranch(o.ActivityRef, o.AtPosition, o.BranchName); err != nil {
				return mdlerrors.NewBackend("drop branch", err)
			}

		case *ast.InsertBoundaryEventOp:
			acts := buildAndBindActivities(ctx, o.Activities)
			if err := mutator.InsertBoundaryEvent(o.ActivityRef, o.AtPosition, o.EventType, o.Delay, acts); err != nil {
				return mdlerrors.NewBackend("insert boundary event", err)
			}

		case *ast.DropBoundaryEventOp:
			if err := mutator.DropBoundaryEvent(o.ActivityRef, o.AtPosition); err != nil {
				return mdlerrors.NewBackend("drop boundary event", err)
			}

		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf("unknown alter workflow operation type: %T", op))
		}
	}

	// Save
	if err := mutator.Save(); err != nil {
		return mdlerrors.NewBackend("save modified workflow", err)
	}

	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Altered workflow %s.%s\n", s.Name.Module, s.Name.Name)
	return nil
}

// buildAndBindActivities builds workflow activities from AST nodes and auto-binds parameters.
func buildAndBindActivities(ctx *ExecContext, nodes []ast.WorkflowActivityNode) []workflows.WorkflowActivity {
	acts := buildWorkflowActivities(nodes)
	autoBindActivitiesInFlow(ctx, acts)
	return acts
}
