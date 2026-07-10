// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

func (m *MockBackend) ListDomainModels() ([]*domainmodel.DomainModel, error) {
	if m.ListDomainModelsFunc != nil {
		return m.ListDomainModelsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	if m.GetDomainModelFunc != nil {
		return m.GetDomainModelFunc(moduleID)
	}
	return nil, nil
}

func (m *MockBackend) GetDomainModelByID(id model.ID) (*domainmodel.DomainModel, error) {
	if m.GetDomainModelByIDFunc != nil {
		return m.GetDomainModelByIDFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) UpdateDomainModel(dm *domainmodel.DomainModel) error {
	if m.UpdateDomainModelFunc != nil {
		return m.UpdateDomainModelFunc(dm)
	}
	return nil
}

func (m *MockBackend) CreateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	if m.CreateEntityFunc != nil {
		return m.CreateEntityFunc(domainModelID, entity)
	}
	return nil
}

func (m *MockBackend) UpdateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	if m.UpdateEntityFunc != nil {
		return m.UpdateEntityFunc(domainModelID, entity)
	}
	return nil
}

func (m *MockBackend) DeleteEntity(domainModelID model.ID, entityID model.ID) error {
	if m.DeleteEntityFunc != nil {
		return m.DeleteEntityFunc(domainModelID, entityID)
	}
	return nil
}

func (m *MockBackend) MoveEntity(entity *domainmodel.Entity, sourceDMID, targetDMID model.ID, sourceModuleName, targetModuleName string) ([]string, error) {
	if m.MoveEntityFunc != nil {
		return m.MoveEntityFunc(entity, sourceDMID, targetDMID, sourceModuleName, targetModuleName)
	}
	return nil, nil
}

func (m *MockBackend) AddAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error {
	if m.AddAttributeFunc != nil {
		return m.AddAttributeFunc(domainModelID, entityID, attr)
	}
	return nil
}

func (m *MockBackend) UpdateAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error {
	if m.UpdateAttributeFunc != nil {
		return m.UpdateAttributeFunc(domainModelID, entityID, attr)
	}
	return nil
}

func (m *MockBackend) DeleteAttribute(domainModelID model.ID, entityID model.ID, attrID model.ID) error {
	if m.DeleteAttributeFunc != nil {
		return m.DeleteAttributeFunc(domainModelID, entityID, attrID)
	}
	return nil
}

func (m *MockBackend) CreateAssociation(domainModelID model.ID, assoc *domainmodel.Association) error {
	if m.CreateAssociationFunc != nil {
		return m.CreateAssociationFunc(domainModelID, assoc)
	}
	return nil
}

func (m *MockBackend) CreateCrossAssociation(domainModelID model.ID, ca *domainmodel.CrossModuleAssociation) error {
	if m.CreateCrossAssociationFunc != nil {
		return m.CreateCrossAssociationFunc(domainModelID, ca)
	}
	return nil
}

func (m *MockBackend) DeleteAssociation(domainModelID model.ID, assocID model.ID) error {
	if m.DeleteAssociationFunc != nil {
		return m.DeleteAssociationFunc(domainModelID, assocID)
	}
	return nil
}

func (m *MockBackend) DeleteCrossAssociation(domainModelID model.ID, assocID model.ID) error {
	if m.DeleteCrossAssociationFunc != nil {
		return m.DeleteCrossAssociationFunc(domainModelID, assocID)
	}
	return nil
}

func (m *MockBackend) CreateViewEntitySourceDocument(moduleID model.ID, moduleName, docName, oqlQuery, documentation string) (model.ID, error) {
	if m.CreateViewEntitySourceDocumentFunc != nil {
		return m.CreateViewEntitySourceDocumentFunc(moduleID, moduleName, docName, oqlQuery, documentation)
	}
	return "", nil
}

func (m *MockBackend) DeleteViewEntitySourceDocument(id model.ID) error {
	if m.DeleteViewEntitySourceDocumentFunc != nil {
		return m.DeleteViewEntitySourceDocumentFunc(id)
	}
	return nil
}

func (m *MockBackend) DeleteViewEntitySourceDocumentByName(moduleName, docName string) error {
	if m.DeleteViewEntitySourceDocumentByNameFunc != nil {
		return m.DeleteViewEntitySourceDocumentByNameFunc(moduleName, docName)
	}
	return nil
}

func (m *MockBackend) FindViewEntitySourceDocumentID(moduleName, docName string) (model.ID, error) {
	if m.FindViewEntitySourceDocumentIDFunc != nil {
		return m.FindViewEntitySourceDocumentIDFunc(moduleName, docName)
	}
	return "", nil
}

func (m *MockBackend) FindAllViewEntitySourceDocumentIDs(moduleName, docName string) ([]model.ID, error) {
	if m.FindAllViewEntitySourceDocumentIDsFunc != nil {
		return m.FindAllViewEntitySourceDocumentIDsFunc(moduleName, docName)
	}
	return nil, nil
}

func (m *MockBackend) MoveViewEntitySourceDocument(sourceModuleName string, targetModuleID model.ID, docName string) error {
	if m.MoveViewEntitySourceDocumentFunc != nil {
		return m.MoveViewEntitySourceDocumentFunc(sourceModuleName, targetModuleID, docName)
	}
	return nil
}

func (m *MockBackend) UpdateOqlQueriesForMovedEntity(oldQualifiedName, newQualifiedName string) (int, error) {
	if m.UpdateOqlQueriesForMovedEntityFunc != nil {
		return m.UpdateOqlQueriesForMovedEntityFunc(oldQualifiedName, newQualifiedName)
	}
	return 0, nil
}

func (m *MockBackend) UpdateEnumerationRefsInAllDomainModels(oldQualifiedName, newQualifiedName string) error {
	if m.UpdateEnumerationRefsInAllDomainModelsFunc != nil {
		return m.UpdateEnumerationRefsInAllDomainModelsFunc(oldQualifiedName, newQualifiedName)
	}
	return nil
}
