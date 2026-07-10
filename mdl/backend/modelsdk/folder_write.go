// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genPr "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/projects"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

// CreateFolder inserts a new Projects$Folder unit under its container (module or
// parent folder). A folder serializes to just {$ID, $Type, Name} — its child
// documents/folders are linked by their own ContainerID, not stored inline.
func (b *Backend) CreateFolder(folder *model.Folder) error {
	if folder == nil {
		return fmt.Errorf("CreateFolder: nil folder")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateFolder: not connected for writing")
	}
	if folder.ID == "" {
		folder.ID = model.ID(mmpr.GenerateID())
	}
	f := genPr.NewFolder()
	f.SetID(element.ID(folder.ID))
	f.SetName(folder.Name)
	contents, err := (&codec.Encoder{}).Encode(f)
	if err != nil {
		return fmt.Errorf("CreateFolder: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(folder.ID), string(folder.ContainerID), "Folders", "Projects$Folder", contents); err != nil {
		return fmt.Errorf("CreateFolder: insert: %w", err)
	}
	return nil
}

// DeleteFolder removes a folder unit by ID. The executor enforces emptiness.
func (b *Backend) DeleteFolder(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteFolder: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// MoveFolder reparents a folder unit to a new container (module or folder).
func (b *Backend) MoveFolder(id model.ID, newContainerID model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("MoveFolder: not connected for writing")
	}
	return b.writer.MoveUnit(string(id), string(newContainerID))
}
