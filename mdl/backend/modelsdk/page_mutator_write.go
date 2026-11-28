// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/mdl/backend/widgetobj"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/sdk/pages"
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

// SerializeParameter builds a new page or snippet parameter through the same
// converters CREATE writes with, so ALTER gets the same version gate on
// IsRequired/DefaultValue (11.5+) and the same ParameterType shapes. A page
// parameter MDL declares is required unless it says `optional`, and has no
// default, as on CREATE.
func (d codecPageDeps) SerializeParameter(container backend.ContainerKind, p backend.PageParameterSpec) (bson.D, error) {
	var el element.Element
	switch container {
	case backend.ContainerPage:
		el = pageParameterToGen(&pages.PageParameter{
			Name:       p.Name,
			TypeName:   p.PrimitiveType,
			EntityName: p.EntityName,
			IsRequired: !p.Optional,
		}, d.b.ProjectVersion())
	case backend.ContainerSnippet:
		el = snippetParameterToGen(&pages.SnippetParameter{
			Name:       p.Name,
			Type:       p.PrimitiveType,
			EntityName: p.EntityName,
		})
	default:
		return nil, fmt.Errorf("a %s has no parameters", container)
	}
	out := genToV1BSON(el)
	if out == nil {
		if err := takeChildSerializeErr(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("serialize parameter $%s: no output", p.Name)
	}
	return out, nil
}

func (d codecPageDeps) SaveUnit(unitID string, contents []byte) error {
	return d.b.writer.UpdateRawUnit(unitID, contents)
}

// SerializeFormDataSource writes a built-in widget's flow data source as create
// page does (ako/mxcli#2634).
func (d codecPageDeps) SerializeFormDataSource(ds pages.DataSource, list bool) bson.D {
	build := dataViewSourceToGen
	if list {
		build = listViewSourceToGen
	}
	el, err := build(ds)
	if err != nil {
		recordChildSerializeErr(fmt.Errorf("serialize data source %T: %w", ds, err))
		return nil
	}
	if el == nil {
		return nil
	}
	return genToV1BSON(el)
}
