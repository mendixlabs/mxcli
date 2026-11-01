// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Nanoflows cannot be *created* over MCP: PED's ped_create_document rejects the
// document type outright ("Document type 'Microflows$Nanoflow' cannot be created.
// Did you mean: Microflows$Microflow?") — its create whitelist covers microflows
// but not nanoflows, despite the two sharing a structure. CREATE / CREATE OR MODIFY
// are therefore rejected with an actionable error. DROP of an existing nanoflow
// uses ped_delete_document (11.16+), like microflows; reads delegate to the
// local .mpr.

// CreateNanoflow is gated on the capability model (capabilities.yaml /
// nanoflow_create). Today PED's create whitelist excludes Microflows$Nanoflow, so it
// is not authorable and this rejects; if a future server lifts that, flip the table.
func (b *Backend) CreateNanoflow(nf *microflows.Nanoflow) error {
	if !b.canAuthor(capNanoflowCreate) {
		return b.notAuthorable("nanoflow", nf.Name, capNanoflowCreate)
	}
	return errCreatePathUnbuilt("nanoflow", nf.Name)
}

// UpdateNanoflow (CREATE OR MODIFY) is gated the same way.
func (b *Backend) UpdateNanoflow(nf *microflows.Nanoflow) error {
	if !b.canAuthor(capNanoflowCreate) {
		return b.notAuthorable("nanoflow", nf.Name, capNanoflowCreate)
	}
	return errCreatePathUnbuilt("nanoflow", nf.Name)
}

// DeleteNanoflow drops an existing nanoflow via deleteDocument: ped_delete_document
// (Studio Pro 11.16+).
func (b *Backend) DeleteNanoflow(id model.ID) error {
	nf, err := b.GetNanoflow(id)
	if err != nil {
		return fmt.Errorf("resolve nanoflow %s for DROP: %w", id, err)
	}
	modName, err := b.moduleNameForContainer(nf.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for nanoflow %q: %w", nf.Name, err)
	}
	return b.deleteDocument(nanoflowDocType, modName, nf.Name)
}

// Nanoflow reads delegate to the local reader (nanoflows can't be created over MCP,
// so there is nothing session-local to merge).
func (b *Backend) ListNanoflows() ([]*microflows.Nanoflow, error) { return b.reader.ListNanoflows() }

func (b *Backend) GetNanoflow(id model.ID) (*microflows.Nanoflow, error) {
	return b.reader.GetNanoflow(id)
}
