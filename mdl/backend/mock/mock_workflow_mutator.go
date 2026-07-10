// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

var _ backend.WorkflowMutator = (*MockWorkflowMutator)(nil)

// MockWorkflowMutator implements backend.WorkflowMutator. Every interface
// method is backed by a public function field. If the field is nil the
// method returns zero values / nil error (never panics).
type MockWorkflowMutator struct {
	SetPropertyFunc           func(prop string, value string) error
	SetPropertyWithEntityFunc func(prop string, value string, entity string) error
	SetActivityPropertyFunc   func(activityRef string, atPos int, prop string, value string) error
	InsertAfterActivityFunc   func(activityRef string, atPos int, activities []workflows.WorkflowActivity) error
	DropActivityFunc          func(activityRef string, atPos int) error
	ReplaceActivityFunc       func(activityRef string, atPos int, activities []workflows.WorkflowActivity) error
	InsertOutcomeFunc         func(activityRef string, atPos int, outcomeName string, activities []workflows.WorkflowActivity) error
	DropOutcomeFunc           func(activityRef string, atPos int, outcomeName string) error
	InsertPathFunc            func(activityRef string, atPos int, pathCaption string, activities []workflows.WorkflowActivity) error
	DropPathFunc              func(activityRef string, atPos int, pathCaption string) error
	InsertBranchFunc          func(activityRef string, atPos int, condition string, activities []workflows.WorkflowActivity) error
	DropBranchFunc            func(activityRef string, atPos int, branchName string) error
	InsertBoundaryEventFunc   func(activityRef string, atPos int, eventType string, delay string, activities []workflows.WorkflowActivity) error
	DropBoundaryEventFunc     func(activityRef string, atPos int) error
	SaveFunc                  func() error
}

func (m *MockWorkflowMutator) SetProperty(prop string, value string) error {
	if m.SetPropertyFunc != nil {
		return m.SetPropertyFunc(prop, value)
	}
	return nil
}

func (m *MockWorkflowMutator) SetPropertyWithEntity(prop string, value string, entity string) error {
	if m.SetPropertyWithEntityFunc != nil {
		return m.SetPropertyWithEntityFunc(prop, value, entity)
	}
	return nil
}

func (m *MockWorkflowMutator) SetActivityProperty(activityRef string, atPos int, prop string, value string) error {
	if m.SetActivityPropertyFunc != nil {
		return m.SetActivityPropertyFunc(activityRef, atPos, prop, value)
	}
	return nil
}

func (m *MockWorkflowMutator) InsertAfterActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error {
	if m.InsertAfterActivityFunc != nil {
		return m.InsertAfterActivityFunc(activityRef, atPos, activities)
	}
	return nil
}

func (m *MockWorkflowMutator) DropActivity(activityRef string, atPos int) error {
	if m.DropActivityFunc != nil {
		return m.DropActivityFunc(activityRef, atPos)
	}
	return nil
}

func (m *MockWorkflowMutator) ReplaceActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error {
	if m.ReplaceActivityFunc != nil {
		return m.ReplaceActivityFunc(activityRef, atPos, activities)
	}
	return nil
}

func (m *MockWorkflowMutator) InsertOutcome(activityRef string, atPos int, outcomeName string, activities []workflows.WorkflowActivity) error {
	if m.InsertOutcomeFunc != nil {
		return m.InsertOutcomeFunc(activityRef, atPos, outcomeName, activities)
	}
	return nil
}

func (m *MockWorkflowMutator) DropOutcome(activityRef string, atPos int, outcomeName string) error {
	if m.DropOutcomeFunc != nil {
		return m.DropOutcomeFunc(activityRef, atPos, outcomeName)
	}
	return nil
}

func (m *MockWorkflowMutator) InsertPath(activityRef string, atPos int, pathCaption string, activities []workflows.WorkflowActivity) error {
	if m.InsertPathFunc != nil {
		return m.InsertPathFunc(activityRef, atPos, pathCaption, activities)
	}
	return nil
}

func (m *MockWorkflowMutator) DropPath(activityRef string, atPos int, pathCaption string) error {
	if m.DropPathFunc != nil {
		return m.DropPathFunc(activityRef, atPos, pathCaption)
	}
	return nil
}

func (m *MockWorkflowMutator) InsertBranch(activityRef string, atPos int, condition string, activities []workflows.WorkflowActivity) error {
	if m.InsertBranchFunc != nil {
		return m.InsertBranchFunc(activityRef, atPos, condition, activities)
	}
	return nil
}

func (m *MockWorkflowMutator) DropBranch(activityRef string, atPos int, branchName string) error {
	if m.DropBranchFunc != nil {
		return m.DropBranchFunc(activityRef, atPos, branchName)
	}
	return nil
}

func (m *MockWorkflowMutator) InsertBoundaryEvent(activityRef string, atPos int, eventType string, delay string, activities []workflows.WorkflowActivity) error {
	if m.InsertBoundaryEventFunc != nil {
		return m.InsertBoundaryEventFunc(activityRef, atPos, eventType, delay, activities)
	}
	return nil
}

func (m *MockWorkflowMutator) DropBoundaryEvent(activityRef string, atPos int) error {
	if m.DropBoundaryEventFunc != nil {
		return m.DropBoundaryEventFunc(activityRef, atPos)
	}
	return nil
}

func (m *MockWorkflowMutator) Save() error {
	if m.SaveFunc != nil {
		return m.SaveFunc()
	}
	return nil
}
