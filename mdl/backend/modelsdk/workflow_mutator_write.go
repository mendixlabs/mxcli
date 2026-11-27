// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"github.com/mendixlabs/mxcli/modelsdk/canon"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/wfmutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// OpenWorkflowForMutation loads a workflow unit and returns a WorkflowMutator
// backed by the shared wfmutator package (pure bson.D tree manipulation). The two
// storage-specific steps — serializing a new activity to bson.D and persisting
// the unit — are wired through codecWorkflowDeps (codec converters + writer).
func (b *Backend) OpenWorkflowForMutation(unitID model.ID) (backend.WorkflowMutator, error) {
	if b.writer == nil {
		return nil, fmt.Errorf("OpenWorkflowForMutation: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return nil, fmt.Errorf("OpenWorkflowForMutation: load unit: %w", err)
	}
	// The mutator edits boundary events in the 11.16 shape; the write raises
	// them back on an 11.17 project (ako/mxcli#2616).
	raw = canon.LowerBoundaryEventContainers(raw)
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("OpenWorkflowForMutation: unmarshal: %w", err)
	}
	return wfmutator.New(d, unitID, codecWorkflowDeps{b: b}), nil
}

// codecWorkflowDeps implements wfmutator.Deps for the modelsdk (codec) backend.
type codecWorkflowDeps struct{ b *Backend }

var _ wfmutator.Deps = codecWorkflowDeps{}

func (d codecWorkflowDeps) SerializeWorkflowActivity(a workflows.WorkflowActivity) bson.D {
	return serializeWorkflowActivityToBSON(a, d.b.ProjectVersion())
}

func (d codecWorkflowDeps) SaveUnit(unitID string, contents []byte) error {
	return d.b.writer.UpdateRawUnit(unitID, contents)
}

// serializeWorkflowActivityToBSON encodes a single workflow activity through the
// codec (activityToGen → Encode) and decodes the result to bson.D, so the shared
// wfmutator can splice it into the raw workflow tree. Returns nil for unsupported
// activity types.
func serializeWorkflowActivityToBSON(a workflows.WorkflowActivity, pv *types.ProjectVersion) bson.D {
	el := activityToGen(a)
	if el == nil {
		return nil
	}
	applyCallMicroflowStorageName(el, usesCallMicroflowActivityName(pv))
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return nil
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil
	}
	if !workflowDeclaresActivityGeometry(pv) {
		// The unit's own activities have none below 11.1; an inserted one
		// carried them (ako/mxcli#1754).
		d = withoutWorkflowActivityGeometry(d)
	}
	return d
}
