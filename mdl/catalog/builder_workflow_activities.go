// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"github.com/mendixlabs/mxcli/mdl/backend/wfnames"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// workflowActivityRow is one row of workflow_activities (ako/mxcli#1295).
type workflowActivityRow struct {
	ID, Name, Caption, ActivityType                                         string
	Microflow, UserSourceMicroflow, OnCreatedMicroflow, CompletionMicroflow string
	Page, UserTaskEntity, CalledWorkflow                                    string
}

// workflowActivityRows lists a workflow's activities in walk order with the
// documents each one names. A lint rule that wants "every microflow a system
// task runs" or "every page a user task opens" reads it, where refs says only
// that the workflow calls the microflow, not from which kind of activity.
func workflowActivityRows(wf *workflows.Workflow) []workflowActivityRow {
	var out []workflowActivityRow
	wfnames.WalkActivities(wf, func(act workflows.WorkflowActivity) {
		r := workflowActivityRow{
			ID:           string(act.GetID()),
			Name:         act.GetName(),
			Caption:      act.GetCaption(),
			ActivityType: act.ActivityType(),
		}
		switch a := act.(type) {
		case *workflows.UserTask:
			if a.IsMulti {
				r.ActivityType = "MultiUserTask"
			}
			r.Page, r.UserTaskEntity, r.OnCreatedMicroflow = a.Page, a.UserTaskEntity, a.OnCreated
			switch us := a.UserSource.(type) {
			case *workflows.MicroflowBasedUserSource:
				r.UserSourceMicroflow = us.Microflow
			case *workflows.MicroflowGroupSource:
				r.UserSourceMicroflow = us.Microflow
			}
			if a.CompletionCriteria != nil {
				r.CompletionMicroflow = a.CompletionCriteria.Microflow
			}
		case *workflows.CallMicroflowTask:
			if a.IsAgent {
				r.ActivityType = "AIAgentTask"
			}
			r.Microflow = a.Microflow
		case *workflows.SystemTask:
			r.Microflow = a.Microflow
		case *workflows.CallWorkflowActivity:
			r.CalledWorkflow = a.Workflow
		}
		out = append(out, r)
	})
	return out
}

// buildWorkflowActivities writes workflow_activities.
func (b *Builder) buildWorkflowActivities(wfs []*workflows.Workflow) (int, error) {
	stmt, err := b.tx.Prepare(`
		INSERT INTO workflow_activities_data (Id, Sequence, WorkflowId, WorkflowQualifiedName, ModuleName,
			Name, Caption, ActivityType, Microflow, UserSourceMicroflow, OnCreatedMicroflow,
			CompletionMicroflow, Page, UserTaskEntity, CalledWorkflow, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, wf := range wfs {
		if !b.own(string(wf.ID)) {
			continue
		}
		moduleName := b.hierarchy.getModuleName(b.hierarchy.findModuleID(wf.ContainerID))
		qualifiedName := moduleName + "." + wf.Name
		for i, r := range workflowActivityRows(wf) {
			if _, err := stmt.Exec(r.ID, i, string(wf.ID), qualifiedName, moduleName,
				r.Name, r.Caption, r.ActivityType,
				nullIfEmpty(r.Microflow), nullIfEmpty(r.UserSourceMicroflow), nullIfEmpty(r.OnCreatedMicroflow),
				nullIfEmpty(r.CompletionMicroflow), nullIfEmpty(r.Page), nullIfEmpty(r.UserTaskEntity),
				nullIfEmpty(r.CalledWorkflow), projectID, snapshotID); err != nil {
				return count, err
			}
			count++
		}
	}
	b.disown()
	return count, nil
}

// nullIfEmpty is a reference column: the referenced document's qualified
// name, or NULL when the slot is empty — one rule since schema 47: absent is
// NULL (ako/mxcli#2093, docRefName).
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
