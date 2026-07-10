// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// ListUnits and ListFolders expose the container tree. The executor's
// ContainerHierarchy (FindModuleID / BuildFolderPath) is built from
// ListModules + ListUnits + ListFolders, so renderers that resolve a
// document's module/folder from its ContainerID (e.g. SHOW MICROFLOWS, where
// flows are nested in folders) need these — without them the rows are silently
// dropped because folder→module resolution fails.

// GetRawUnit returns a unit's decoded BSON as an ordered map (catalog +
// raw-inspection paths). Delegates to the codec reader.
func (b *Backend) GetRawUnit(id model.ID) (map[string]any, error) {
	return b.reader.GetRawUnit(id)
}

// ListRawUnitsByType returns every unit whose $Type has the given prefix, with
// resolved raw contents — the catalog uses this for document types that have no
// dedicated typed reader (e.g. JavaScript actions, data transformers). Delegates
// to the codec reader.
func (b *Backend) ListRawUnitsByType(typePrefix string) ([]*types.RawUnit, error) {
	return b.reader.ListRawUnitsByType(typePrefix)
}

func (b *Backend) ListUnits() ([]*types.UnitInfo, error) {
	us, err := b.reader.ListUnits()
	if err != nil {
		return nil, err
	}
	out := make([]*types.UnitInfo, 0, len(us))
	for _, u := range us {
		out = append(out, &types.UnitInfo{
			ID:              model.ID(u.ID),
			ContainerID:     model.ID(u.ContainerID),
			ContainmentName: u.ContainmentName,
			Type:            u.Type,
		})
	}
	return out, nil
}

func (b *Backend) ListFolders() ([]*types.FolderInfo, error) {
	fs, err := b.reader.ListFolders()
	if err != nil {
		return nil, err
	}
	out := make([]*types.FolderInfo, 0, len(fs))
	for _, f := range fs {
		out = append(out, &types.FolderInfo{
			ID:          model.ID(f.ID),
			ContainerID: model.ID(f.ContainerID),
			Name:        f.Name,
		})
	}
	return out, nil
}
