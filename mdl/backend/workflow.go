// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// WorkflowBackend provides workflow operations.
type WorkflowBackend interface {
	ListWorkflows() ([]*workflows.Workflow, error)
	GetWorkflow(id model.ID) (*workflows.Workflow, error)
	CreateWorkflow(wf *workflows.Workflow) error
	UpdateWorkflow(wf *workflows.Workflow) error
	DeleteWorkflow(id model.ID) error
}
