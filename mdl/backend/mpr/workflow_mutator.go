// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/wfmutator"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// openWorkflowForMutation loads a workflow unit and returns a WorkflowMutator
// backed by the shared wfmutator package. The mutation logic is engine-agnostic
// (pure bson.D tree manipulation over the unit's raw bytes); the two
// storage-specific steps — serializing a new activity to bson.D and persisting
// the unit — are wired through mprWorkflowDeps.
func (b *MprBackend) openWorkflowForMutation(unitID model.ID) (backend.WorkflowMutator, error) {
	rawBytes, err := b.reader.GetRawUnitBytes(unitID)
	if err != nil {
		return nil, fmt.Errorf("load raw unit bytes: %w", err)
	}
	var rawData bson.D
	if err := bson.Unmarshal(rawBytes, &rawData); err != nil {
		return nil, fmt.Errorf("unmarshal workflow BSON: %w", err)
	}
	return wfmutator.New(rawData, unitID, &mprWorkflowDeps{backend: b}), nil
}

// mprWorkflowDeps implements wfmutator.Deps for the MPR backend, delegating to
// the sdk/mpr activity serializer and the raw-unit writer.
type mprWorkflowDeps struct{ backend *MprBackend }

var _ wfmutator.Deps = (*mprWorkflowDeps)(nil)

func (d *mprWorkflowDeps) SerializeWorkflowActivity(a workflows.WorkflowActivity) bson.D {
	return mpr.SerializeWorkflowActivity(a)
}

func (d *mprWorkflowDeps) SaveUnit(unitID string, contents []byte) error {
	return d.backend.writer.UpdateRawUnit(unitID, contents)
}
