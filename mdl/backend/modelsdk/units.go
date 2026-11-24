// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
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

// GetRawUnitBytes returns a unit's raw BSON. Both this and UpdateRawUnit already
// existed on the reader/writer and are used throughout this package; they were
// simply never exposed as Backend methods, so the embedded `unimplemented` stub
// answered and every caller going through the interface was told to fall back to
// the legacy engine. That is what gated `mxcli widget sync --apply` to that
// engine while its read-only plan ran on both.
func (b *Backend) GetRawUnitBytes(id model.ID) ([]byte, error) {
	return b.reader.GetRawUnitBytes(string(id))
}

// UpdateRawUnit replaces a unit's contents. Takes a string ID to match the SDK writer
// layer convention (see backend.RawUnitBackend).
func (b *Backend) UpdateRawUnit(unitID string, contents []byte) error {
	return b.writer.UpdateRawUnit(unitID, contents)
}

// UpdateRawUnitOwningTranslations writes a unit whose contents already account
// for every translation in it — see the interface for why that is a distinct
// case from an ordinary rebuild.
func (b *Backend) UpdateRawUnitOwningTranslations(unitID string, contents []byte) error {
	return b.writer.UpdateRawUnitOwningTranslations(unitID, contents)
}

// UpdateRawUnitPatch writes a unit whose contents patch the stored bytes in
// place (canon.ContentsOwnElementIDs, canon.ContentsOwnTranslations).
func (b *Backend) UpdateRawUnitPatch(unitID string, contents []byte) error {
	return b.writer.UpdateRawUnitPatch(unitID, contents)
}

// UpdateRawUnitOwningStorageGUIDs writes a unit whose contents deliberately move
// storage GUIDs — see the interface for why that needs saying out loud.
func (b *Backend) UpdateRawUnitOwningStorageGUIDs(unitID string, contents []byte) error {
	return b.writer.UpdateRawUnitOwningStorageGUIDs(unitID, contents)
}

// ListRawUnitsByType returns every unit whose $Type has the given prefix, with
// resolved raw contents — the catalog uses this for document types that have no
// dedicated typed reader (e.g. JavaScript actions, data transformers). Delegates
// to the codec reader.
func (b *Backend) ListRawUnitsByType(typePrefix string) ([]*types.RawUnit, error) {
	return b.reader.ListRawUnitsByTypeWhere(typePrefix, b.unitScope)
}

// WithUnitScope returns a read view of the backend whose document lists hold
// only the units keep accepts, and which reads no other unit's contents on
// MPR v2. Everything else — single-document reads, settings, the unit
// table — is the backend's. A catalog build that rebuilds a few units lists
// through it, so it decodes those and not the project (ako/mxcli#1804).
//
// The view shares the backend's reader; it is for reading, and writes
// through it are not counted in the backend's write statistics.
func (b *Backend) WithUnitScope(keep func(unitID string) bool) backend.FullBackend {
	view := *b
	view.unitScope = keep
	return &view
}

// listUnits decodes the units of type T the backend reads: every one, or
// those in its unit scope (WithUnitScope).
func listUnits[T element.Element](b *Backend) ([]mprread.Unit[T], error) {
	if b.unitScope != nil {
		return mprread.ListUnitsWithContainerIn[T](b.reader, b.unitScope)
	}
	return mprread.ListUnitsWithContainer[T](b.reader)
}

// unitHeaders lists the ID, container, name and Excluded flag of every unit of
// type T without decoding any of them.
func unitHeaders[T element.Element](b *Backend) ([]types.DocumentHeader, error) {
	hs, err := mprread.ListUnitHeaders[T](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]types.DocumentHeader, len(hs))
	for i, h := range hs {
		out[i] = types.DocumentHeader(h)
	}
	return out, nil
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

// UnitContentHashes returns a hash of every unit's current contents, keyed by
// unit ID, so a caller can tell which units changed since it last looked (the
// catalog's incremental source index, ako/mxcli#1790).
func (b *Backend) UnitContentHashes() (map[string]string, error) {
	return b.reader.UnitContentHashes()
}

// AddRawUnit inserts a new unit verbatim.
//
// Exposed on the backend because the marketplace transplant copies a module's
// units between projects without decoding them, and had been reaching a
// concrete sdk/mpr writer to do it. Copying verbatim is deliberate rather than
// lazy: decoding and re-encoding would mint fresh identities, and an entity's
// GUID is what the runtime keys the database on (CLAUDE.md).
func (b *Backend) AddRawUnit(unitID, containerID, containmentName, unitType string, contents []byte) error {
	if b.writer == nil {
		return fmt.Errorf("AddRawUnit: not connected for writing")
	}
	return b.writer.InsertUnit(unitID, containerID, containmentName, unitType, contents)
}
