// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/pagemutator"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/widgetobj"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// OpenPageForMutation loads a page/snippet/layout unit and returns a PageMutator
// backed by the shared pagemutator package. The mutator is engine-agnostic (pure
// bson.D tree manipulation over the unit's raw bytes); the codec-specific steps —
// child serialization (widget / client-action → bson via the codec converters)
// and persisting the unit — are wired through codecPageDeps.
func (b *Backend) OpenPageForMutation(unitID model.ID) (backend.PageMutator, error) {
	if b.writer == nil {
		return nil, fmt.Errorf("OpenPageForMutation: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return nil, fmt.Errorf("OpenPageForMutation: load unit: %w", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("OpenPageForMutation: unmarshal: %w", err)
	}
	return pagemutator.New(d, unitID, codecPageDeps{b: b}), nil
}

// codecPageDeps implements pagemutator.Deps for the codec engine. The three child
// serializers are inherited from codecChildSerializer (which routes through the
// codec converters and bridges v2→v1 BSON); persistence goes through the writer.
type codecPageDeps struct {
	codecChildSerializer
	b *Backend
}

var _ pagemutator.Deps = codecPageDeps{}

// BuildDataGrid2Column builds a DataGrid2 column via the shared widgetobj builder,
// serializing any custom-content child widgets through the codec child serializer.
func (d codecPageDeps) BuildDataGrid2Column(col *backend.DataGridColumnSpec, columnObjectTypeID string, columnPropertyIDs map[string]pages.PropertyTypeIDEntry) (bson.D, error) {
	return widgetobj.BuildDataGrid2Column(codecChildSerializer{}, col, columnObjectTypeID, columnPropertyIDs), nil
}

func (d codecPageDeps) SaveUnit(unitID string, contents []byte) error {
	return d.b.writer.UpdateRawUnit(unitID, contents)
}
