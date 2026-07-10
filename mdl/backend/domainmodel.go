// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// DomainModelBackend provides domain model, entity, attribute, and
// association operations.
type DomainModelBackend interface {
	// Domain models
	ListDomainModels() ([]*domainmodel.DomainModel, error)
	GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error)
	GetDomainModelByID(id model.ID) (*domainmodel.DomainModel, error)
	UpdateDomainModel(dm *domainmodel.DomainModel) error

	// Entities
	CreateEntity(domainModelID model.ID, entity *domainmodel.Entity) error
	UpdateEntity(domainModelID model.ID, entity *domainmodel.Entity) error
	DeleteEntity(domainModelID model.ID, entityID model.ID) error
	MoveEntity(entity *domainmodel.Entity, sourceDMID, targetDMID model.ID, sourceModuleName, targetModuleName string) ([]string, error)

	// Attributes
	AddAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error
	UpdateAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error
	DeleteAttribute(domainModelID model.ID, entityID model.ID, attrID model.ID) error

	// Associations
	CreateAssociation(domainModelID model.ID, assoc *domainmodel.Association) error
	CreateCrossAssociation(domainModelID model.ID, ca *domainmodel.CrossModuleAssociation) error
	DeleteAssociation(domainModelID model.ID, assocID model.ID) error
	DeleteCrossAssociation(domainModelID model.ID, assocID model.ID) error

	// View entities
	CreateViewEntitySourceDocument(moduleID model.ID, moduleName, docName, oqlQuery, documentation string) (model.ID, error)
	DeleteViewEntitySourceDocument(id model.ID) error
	DeleteViewEntitySourceDocumentByName(moduleName, docName string) error
	FindViewEntitySourceDocumentID(moduleName, docName string) (model.ID, error)
	FindAllViewEntitySourceDocumentIDs(moduleName, docName string) ([]model.ID, error)
	MoveViewEntitySourceDocument(sourceModuleName string, targetModuleID model.ID, docName string) error
	UpdateOqlQueriesForMovedEntity(oldQualifiedName, newQualifiedName string) (int, error)
	UpdateEnumerationRefsInAllDomainModels(oldQualifiedName, newQualifiedName string) error
}
