// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// moveUnit reparents a top-level document unit to its (already-updated) container.
// All the typed Move* document methods reduce to this: the executor mutates the
// document's ContainerID, then calls the matching Move*, which persists the new
// containment row. The document contents are untouched.
func (b *Backend) moveUnit(id, containerID model.ID, what string) error {
	if b.writer == nil {
		return fmt.Errorf("Move%s: not connected for writing", what)
	}
	return b.writer.MoveUnit(string(id), string(containerID))
}

func (b *Backend) MoveMicroflow(mf *microflows.Microflow) error {
	if mf == nil {
		return fmt.Errorf("MoveMicroflow: nil microflow")
	}
	return b.moveUnit(mf.ID, mf.ContainerID, "Microflow")
}

func (b *Backend) MoveNanoflow(nf *microflows.Nanoflow) error {
	if nf == nil {
		return fmt.Errorf("MoveNanoflow: nil nanoflow")
	}
	return b.moveUnit(nf.ID, nf.ContainerID, "Nanoflow")
}

func (b *Backend) MovePage(page *pages.Page) error {
	if page == nil {
		return fmt.Errorf("MovePage: nil page")
	}
	return b.moveUnit(page.ID, page.ContainerID, "Page")
}

func (b *Backend) MoveSnippet(snippet *pages.Snippet) error {
	if snippet == nil {
		return fmt.Errorf("MoveSnippet: nil snippet")
	}
	return b.moveUnit(snippet.ID, snippet.ContainerID, "Snippet")
}

func (b *Backend) MoveImportMapping(im *model.ImportMapping) error {
	if im == nil {
		return fmt.Errorf("MoveImportMapping: nil mapping")
	}
	return b.moveUnit(im.ID, im.ContainerID, "ImportMapping")
}

func (b *Backend) MoveExportMapping(em *model.ExportMapping) error {
	if em == nil {
		return fmt.Errorf("MoveExportMapping: nil mapping")
	}
	return b.moveUnit(em.ID, em.ContainerID, "ExportMapping")
}

func (b *Backend) MoveDatabaseConnection(conn *model.DatabaseConnection) error {
	if conn == nil {
		return fmt.Errorf("MoveDatabaseConnection: nil connection")
	}
	return b.moveUnit(conn.ID, conn.ContainerID, "DatabaseConnection")
}
