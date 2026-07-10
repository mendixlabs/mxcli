// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// MicroflowBackend provides microflow and nanoflow operations.
type MicroflowBackend interface {
	ListMicroflows() ([]*microflows.Microflow, error)
	GetMicroflow(id model.ID) (*microflows.Microflow, error)
	CreateMicroflow(mf *microflows.Microflow) error
	UpdateMicroflow(mf *microflows.Microflow) error
	DeleteMicroflow(id model.ID) error
	MoveMicroflow(mf *microflows.Microflow) error

	// ParseMicroflowFromRaw builds a Microflow from an already-unmarshalled
	// map. Used by diff-local and other callers that have raw map data.
	ParseMicroflowFromRaw(raw map[string]any, unitID, containerID model.ID) *microflows.Microflow

	// ParseMicroflowBSON parses raw microflow BSON bytes into a Microflow.
	// Used by the executor to inspect microflows it has not necessarily
	// loaded via ListMicroflows (e.g. to resolve a CALL MICROFLOW's return
	// type from its raw unit).
	ParseMicroflowBSON(contents []byte, unitID, containerID model.ID) (*microflows.Microflow, error)

	ListNanoflows() ([]*microflows.Nanoflow, error)
	GetNanoflow(id model.ID) (*microflows.Nanoflow, error)
	CreateNanoflow(nf *microflows.Nanoflow) error
	UpdateNanoflow(nf *microflows.Nanoflow) error
	DeleteNanoflow(id model.ID) error
	MoveNanoflow(nf *microflows.Nanoflow) error

	// IsRule reports whether the given qualified name refers to a rule
	// (Microflows$Rule) rather than a microflow. The flow builder uses this
	// to decide whether an IF condition that looks like a function call
	// (Module.Name(...)) should be serialized as a RuleSplitCondition.
	IsRule(qualifiedName string) (bool, error)
}
