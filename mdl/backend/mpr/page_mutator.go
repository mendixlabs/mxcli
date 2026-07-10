// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/pagemutator"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/widgetobj"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// OpenPageForMutation loads a page/snippet/layout unit and returns a PageMutator
// backed by the shared pagemutator package. The mutator is engine-agnostic (pure
// bson.D tree manipulation); the MPR-specific steps — child serialization, the
// DataGrid2 column builder, and persisting the unit — are wired through
// mprPageDeps.
func (b *MprBackend) OpenPageForMutation(unitID model.ID) (backend.PageMutator, error) {
	rawBytes, err := b.reader.GetRawUnitBytes(unitID)
	if err != nil {
		return nil, fmt.Errorf("load raw unit bytes: %w", err)
	}
	var rawData bson.D
	if err := bson.Unmarshal(rawBytes, &rawData); err != nil {
		return nil, fmt.Errorf("unmarshal unit BSON: %w", err)
	}
	return pagemutator.New(rawData, unitID, &mprPageDeps{backend: b}), nil
}

// mprPageDeps implements pagemutator.Deps for the MPR backend, delegating to the
// sdk/mpr serializers, the DataGrid2 column builder, and the raw-unit writer.
type mprPageDeps struct{ backend *MprBackend }

var _ pagemutator.Deps = (*mprPageDeps)(nil)

func (d *mprPageDeps) SerializeWidget(w pages.Widget) bson.D {
	return mpr.SerializeWidget(w)
}

func (d *mprPageDeps) SerializeClientAction(a pages.ClientAction) bson.D {
	return mpr.SerializeClientAction(a)
}

func (d *mprPageDeps) SerializeCustomWidgetDataSource(ds pages.DataSource) bson.D {
	return mpr.SerializeCustomWidgetDataSource(ds)
}

func (d *mprPageDeps) BuildDataGrid2Column(col *backend.DataGridColumnSpec, columnObjectTypeID string, columnPropertyIDs map[string]pages.PropertyTypeIDEntry) (bson.D, error) {
	return widgetobj.BuildDataGrid2Column(mprChildSerializer{}, col, columnObjectTypeID, columnPropertyIDs), nil
}

func (d *mprPageDeps) SaveUnit(unitID string, contents []byte) error {
	return d.backend.writer.UpdateRawUnit(unitID, contents)
}
