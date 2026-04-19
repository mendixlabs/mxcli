// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// WorkflowBackend provides workflow operations.
type WorkflowBackend interface {
	ListWorkflows() ([]*workflows.Workflow, error)
	GetWorkflow(id model.ID) (*workflows.Workflow, error)
	CreateWorkflow(wf *workflows.Workflow) error
	DeleteWorkflow(id model.ID) error
}
