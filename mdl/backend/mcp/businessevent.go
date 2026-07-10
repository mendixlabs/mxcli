// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Business event SERVICES cannot be created over MCP: ped_create_document rejects
// the document type outright ("Document type 'BusinessEvents$BusinessEventService'
// cannot be created.") — it is off the create whitelist (and the schema is a
// $element, not a $constructor). So CREATE / CREATE OR MODIFY are rejected; reads
// (SHOW / DESCRIBE) delegate to the local .mpr; DROP of an existing service goes
// through Concord, like other standalone documents.
//
// The domain model a business-events setup relies on — a published-event entity
// (`extends BusinessEvents.PublishedBusinessEvent`, generalization is supported by
// the entity create path) and its constant — IS authorable over MCP; only the
// service document that ties them into a contract is not.

// ListBusinessEventServices delegates to the local reader so SHOW / DESCRIBE
// business events work over MCP (they were unsupported, which broke those reads).
func (b *Backend) ListBusinessEventServices() ([]*model.BusinessEventService, error) {
	return b.reader.ListBusinessEventServices()
}

// CreateBusinessEventService is gated on the capability model (capabilities.yaml /
// businessevent_create). PED won't create the service document today, so it rejects.
func (b *Backend) CreateBusinessEventService(svc *model.BusinessEventService) error {
	if !b.canAuthor(capBusinessEventCreate) {
		return b.notAuthorable("business event service", svc.Name, capBusinessEventCreate)
	}
	return errCreatePathUnbuilt("business event service", svc.Name)
}

// UpdateBusinessEventService (CREATE OR MODIFY) is gated the same way.
func (b *Backend) UpdateBusinessEventService(svc *model.BusinessEventService) error {
	if !b.canAuthor(capBusinessEventCreate) {
		return b.notAuthorable("business event service", svc.Name, capBusinessEventCreate)
	}
	return errCreatePathUnbuilt("business event service", svc.Name)
}

// DeleteBusinessEventService drops an existing service via Concord's delete_document
// (PED has no delete tool), like other standalone documents. Requires --mcp-concord.
func (b *Backend) DeleteBusinessEventService(id model.ID) error {
	services, err := b.reader.ListBusinessEventServices()
	if err != nil {
		return fmt.Errorf("resolve business event service %s for DROP: %w", id, err)
	}
	for _, s := range services {
		if s.ID == id {
			modName, err := b.moduleNameForContainer(s.ContainerID)
			if err != nil {
				return fmt.Errorf("resolve module for business event service %q: %w", s.Name, err)
			}
			return b.concordDeleteDocument(modName, s.Name)
		}
	}
	return fmt.Errorf("business event service %s not found", id)
}
