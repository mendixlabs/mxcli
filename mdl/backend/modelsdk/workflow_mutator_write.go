// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/wfmutator"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
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
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("OpenWorkflowForMutation: unmarshal: %w", err)
	}
	return wfmutator.New(d, unitID, codecWorkflowDeps{b: b}), nil
}

// SerializeWorkflowActivity converts a domain WorkflowActivity to its raw bson.D
// form via the codec converters (used by the ALTER WORKFLOW insert/replace paths).
func (b *Backend) SerializeWorkflowActivity(a workflows.WorkflowActivity) (any, error) {
	d := serializeWorkflowActivityToBSON(a)
	if d == nil {
		return nil, fmt.Errorf("SerializeWorkflowActivity: unsupported activity %T", a)
	}
	return d, nil
}

// codecWorkflowDeps implements wfmutator.Deps for the modelsdk (codec) backend.
type codecWorkflowDeps struct{ b *Backend }

var _ wfmutator.Deps = codecWorkflowDeps{}

func (d codecWorkflowDeps) SerializeWorkflowActivity(a workflows.WorkflowActivity) bson.D {
	return serializeWorkflowActivityToBSON(a)
}

func (d codecWorkflowDeps) SaveUnit(unitID string, contents []byte) error {
	return d.b.writer.UpdateRawUnit(unitID, contents)
}

// serializeWorkflowActivityToBSON encodes a single workflow activity through the
// codec (activityToGen → Encode) and decodes the result to bson.D, so the shared
// wfmutator can splice it into the raw workflow tree. Returns nil for unsupported
// activity types.
func serializeWorkflowActivityToBSON(a workflows.WorkflowActivity) bson.D {
	el := activityToGen(a)
	if el == nil {
		return nil
	}
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return nil
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil
	}
	return d
}
