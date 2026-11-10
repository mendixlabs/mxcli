// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// MoveDocument reparents a top-level document unit. See
// backend.DocumentPlacementBackend for why this is type-agnostic; the writer
// handles the idempotence the interface requires.
func (b *Backend) MoveDocument(unitID, containerID model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("MoveDocument: not connected for writing")
	}
	if unitID == "" || containerID == "" {
		return fmt.Errorf("MoveDocument: unit and container are both required")
	}
	return b.writer.MoveUnit(string(unitID), string(containerID))
}

// FindDocumentUnit locates a document by module and name through the unit
// table, whatever its type.
//
// Only units contained as "Documents" are considered. A module also holds its
// domain model, its security and its settings, and folders sit in the same
// table — restricting to the document containment is what keeps a folder named
// like a document from being returned as one.
func (b *Backend) FindDocumentUnit(moduleName, name string) (*types.DocumentUnit, error) {
	docs, err := b.findDocumentUnits(moduleName, name, true)
	if err != nil || len(docs) == 0 {
		return nil, err
	}
	return docs[0], nil
}

// FindDocumentUnits returns every document named name in moduleName, of any
// type. See backend.DocumentPlacementBackend.
func (b *Backend) FindDocumentUnits(moduleName, name string) ([]*types.DocumentUnit, error) {
	return b.findDocumentUnits(moduleName, name, false)
}

func (b *Backend) findDocumentUnits(moduleName, name string, firstOnly bool) ([]*types.DocumentUnit, error) {
	if b.reader == nil {
		return nil, fmt.Errorf("FindDocumentUnit: not connected")
	}
	modules, err := b.reader.ListModules()
	if err != nil {
		return nil, fmt.Errorf("FindDocumentUnit: list modules: %w", err)
	}
	var moduleID string
	for _, m := range modules {
		if m.Name == moduleName {
			moduleID = m.ID
			break
		}
	}
	if moduleID == "" {
		return nil, nil
	}
	containers := b.containerSetForModule(moduleID)

	var found []*types.DocumentUnit
	err = b.eachDocumentUnit(func(doc *types.DocumentUnit) bool {
		if doc.Name != name || !containers[string(doc.ContainerID)] {
			return true
		}
		found = append(found, doc)
		return !firstOnly
	})
	if err != nil {
		return nil, fmt.Errorf("FindDocumentUnit: %w", err)
	}
	return found, nil
}

// ListDocumentUnits returns every top-level document in the project.
func (b *Backend) ListDocumentUnits() ([]*types.DocumentUnit, error) {
	if b.reader == nil {
		return nil, fmt.Errorf("ListDocumentUnits: not connected")
	}
	var out []*types.DocumentUnit
	if err := b.eachDocumentUnit(func(doc *types.DocumentUnit) bool {
		out = append(out, doc)
		return true
	}); err != nil {
		return nil, fmt.Errorf("ListDocumentUnits: %w", err)
	}
	return out, nil
}

// eachDocumentUnit walks every "Documents" unit, decoding just enough of each
// to name it, and stops early when visit returns false.
//
// A unit whose contents will not decode is skipped rather than failing the
// walk: a listing missing one unreadable document is more useful than no
// listing, and the alternative would let one damaged unit hide a whole project.
func (b *Backend) eachDocumentUnit(visit func(*types.DocumentUnit) bool) error {
	units, err := b.reader.ListUnits()
	if err != nil {
		return fmt.Errorf("list units: %w", err)
	}
	for _, u := range units {
		if u.ContainmentName != "Documents" {
			continue
		}
		raw, err := b.reader.GetRawUnitBytes(u.ID)
		if err != nil || len(raw) == 0 {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			continue
		}
		name := docNameOf(doc)
		if name == "" {
			continue
		}
		if !visit(&types.DocumentUnit{
			ID:          model.ID(u.ID),
			ContainerID: model.ID(u.ContainerID),
			Name:        name,
			Type:        u.Type,
			Kind:        types.DocumentKind(u.Type),
		}) {
			return nil
		}
	}
	return nil
}
