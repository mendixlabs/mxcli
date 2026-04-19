// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
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

	ListNanoflows() ([]*microflows.Nanoflow, error)
	GetNanoflow(id model.ID) (*microflows.Nanoflow, error)
	CreateNanoflow(nf *microflows.Nanoflow) error
	UpdateNanoflow(nf *microflows.Nanoflow) error
	DeleteNanoflow(id model.ID) error
	MoveNanoflow(nf *microflows.Nanoflow) error
}
