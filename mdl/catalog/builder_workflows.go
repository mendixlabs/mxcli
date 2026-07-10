// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

func (b *Builder) buildWorkflows() error {
	wfs, err := b.cachedWorkflows()
	if err != nil {
		return err
	}

	if len(wfs) == 0 {
		return nil
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO workflows_data (Id, Name, QualifiedName, ModuleName, Folder, Description,
			ExportLevel, ParameterEntity, ActivityCount, UserTaskCount, MicroflowCallCount, DecisionCount,
			DueDate, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, wf := range wfs {
		moduleID := b.hierarchy.findModuleID(wf.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + wf.Name
		folderPath := b.hierarchy.buildFolderPath(wf.ContainerID)

		paramEntity := ""
		if wf.Parameter != nil {
			paramEntity = wf.Parameter.EntityRef
		}

		// Count activities by type
		actCount, utCount, mfCount, decCount := countWorkflowActivityTypes(wf)

		_, err = stmt.Exec(
			string(wf.ID),
			wf.Name,
			qualifiedName,
			moduleName,
			folderPath,
			wf.Documentation,
			wf.ExportLevel,
			paramEntity,
			actCount,
			utCount,
			mfCount,
			decCount,
			wf.DueDate,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
		count++
	}

	b.report("Workflows", count)
	return nil
}

// countWorkflowActivityTypes counts activity types in a workflow.
func countWorkflowActivityTypes(wf *workflows.Workflow) (total, userTasks, microflowCalls, decisions int) {
	if wf.Flow == nil {
		return
	}
	countFlowActivityTypes(wf.Flow, &total, &userTasks, &microflowCalls, &decisions)
	return
}

// countFlowActivityTypes recursively counts activity types in a flow.
func countFlowActivityTypes(flow *workflows.Flow, total, userTasks, microflowCalls, decisions *int) {
	if flow == nil {
		return
	}
	for _, act := range flow.Activities {
		*total++
		switch a := act.(type) {
		case *workflows.UserTask:
			*userTasks++
			for _, outcome := range a.Outcomes {
				countFlowActivityTypes(outcome.Flow, total, userTasks, microflowCalls, decisions)
			}
		case *workflows.CallMicroflowTask:
			*microflowCalls++
			for _, outcome := range a.Outcomes {
				countOutcomeFlowActivities(outcome, total, userTasks, microflowCalls, decisions)
			}
		case *workflows.SystemTask:
			*microflowCalls++
			for _, outcome := range a.Outcomes {
				countOutcomeFlowActivities(outcome, total, userTasks, microflowCalls, decisions)
			}
		case *workflows.ExclusiveSplitActivity:
			*decisions++
			for _, outcome := range a.Outcomes {
				countOutcomeFlowActivities(outcome, total, userTasks, microflowCalls, decisions)
			}
		case *workflows.ParallelSplitActivity:
			for _, outcome := range a.Outcomes {
				countFlowActivityTypes(outcome.Flow, total, userTasks, microflowCalls, decisions)
			}
		}
	}
}

// countOutcomeFlowActivities counts activities in a condition outcome's flow.
func countOutcomeFlowActivities(outcome workflows.ConditionOutcome, total, userTasks, microflowCalls, decisions *int) {
	if outcome == nil {
		return
	}
	countFlowActivityTypes(outcome.GetFlow(), total, userTasks, microflowCalls, decisions)
}
