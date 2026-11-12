// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"github.com/mendixlabs/mxcli/mdl/backend/wfnames"
	"github.com/mendixlabs/mxcli/sdk/workflows"
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
		if !b.own(string(wf.ID)) {
			continue
		}
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
	b.disown()

	b.report("Workflows", count)

	activities, err := b.buildWorkflowActivities(wfs)
	if err != nil {
		return err
	}
	b.report("Workflow activities", activities)
	return nil
}

// countWorkflowActivityTypes counts activity types in a workflow, over every
// flow it holds (see wfnames.WalkActivities).
func countWorkflowActivityTypes(wf *workflows.Workflow) (total, userTasks, microflowCalls, decisions int) {
	c := wfnames.CountActivities(wf)
	return c.Total, c.UserTasks, c.MicroflowCalls, c.Decisions
}
