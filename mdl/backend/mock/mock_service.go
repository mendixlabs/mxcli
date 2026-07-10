// SPDX-License-Identifier: Apache-2.0

package mock

import "github.com/JordtenBulte-OLC/mxcli/model"

func (m *MockBackend) ListConsumedODataServices() ([]*model.ConsumedODataService, error) {
	if m.ListConsumedODataServicesFunc != nil {
		return m.ListConsumedODataServicesFunc()
	}
	return nil, nil
}

func (m *MockBackend) ListPublishedODataServices() ([]*model.PublishedODataService, error) {
	if m.ListPublishedODataServicesFunc != nil {
		return m.ListPublishedODataServicesFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateConsumedODataService(svc *model.ConsumedODataService) error {
	if m.CreateConsumedODataServiceFunc != nil {
		return m.CreateConsumedODataServiceFunc(svc)
	}
	return nil
}

func (m *MockBackend) UpdateConsumedODataService(svc *model.ConsumedODataService) error {
	if m.UpdateConsumedODataServiceFunc != nil {
		return m.UpdateConsumedODataServiceFunc(svc)
	}
	return nil
}

func (m *MockBackend) DeleteConsumedODataService(id model.ID) error {
	if m.DeleteConsumedODataServiceFunc != nil {
		return m.DeleteConsumedODataServiceFunc(id)
	}
	return nil
}

func (m *MockBackend) CreatePublishedODataService(svc *model.PublishedODataService) error {
	if m.CreatePublishedODataServiceFunc != nil {
		return m.CreatePublishedODataServiceFunc(svc)
	}
	return nil
}

func (m *MockBackend) UpdatePublishedODataService(svc *model.PublishedODataService) error {
	if m.UpdatePublishedODataServiceFunc != nil {
		return m.UpdatePublishedODataServiceFunc(svc)
	}
	return nil
}

func (m *MockBackend) DeletePublishedODataService(id model.ID) error {
	if m.DeletePublishedODataServiceFunc != nil {
		return m.DeletePublishedODataServiceFunc(id)
	}
	return nil
}

func (m *MockBackend) ListConsumedRestServices() ([]*model.ConsumedRestService, error) {
	if m.ListConsumedRestServicesFunc != nil {
		return m.ListConsumedRestServicesFunc()
	}
	return nil, nil
}

func (m *MockBackend) ListPublishedRestServices() ([]*model.PublishedRestService, error) {
	if m.ListPublishedRestServicesFunc != nil {
		return m.ListPublishedRestServicesFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateConsumedRestService(svc *model.ConsumedRestService) error {
	if m.CreateConsumedRestServiceFunc != nil {
		return m.CreateConsumedRestServiceFunc(svc)
	}
	return nil
}

func (m *MockBackend) UpdateConsumedRestService(svc *model.ConsumedRestService) error {
	if m.UpdateConsumedRestServiceFunc != nil {
		return m.UpdateConsumedRestServiceFunc(svc)
	}
	return nil
}

func (m *MockBackend) DeleteConsumedRestService(id model.ID) error {
	if m.DeleteConsumedRestServiceFunc != nil {
		return m.DeleteConsumedRestServiceFunc(id)
	}
	return nil
}

func (m *MockBackend) CreatePublishedRestService(svc *model.PublishedRestService) error {
	if m.CreatePublishedRestServiceFunc != nil {
		return m.CreatePublishedRestServiceFunc(svc)
	}
	return nil
}

func (m *MockBackend) UpdatePublishedRestService(svc *model.PublishedRestService) error {
	if m.UpdatePublishedRestServiceFunc != nil {
		return m.UpdatePublishedRestServiceFunc(svc)
	}
	return nil
}

func (m *MockBackend) DeletePublishedRestService(id model.ID) error {
	if m.DeletePublishedRestServiceFunc != nil {
		return m.DeletePublishedRestServiceFunc(id)
	}
	return nil
}

func (m *MockBackend) ListBusinessEventServices() ([]*model.BusinessEventService, error) {
	if m.ListBusinessEventServicesFunc != nil {
		return m.ListBusinessEventServicesFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateBusinessEventService(svc *model.BusinessEventService) error {
	if m.CreateBusinessEventServiceFunc != nil {
		return m.CreateBusinessEventServiceFunc(svc)
	}
	return nil
}

func (m *MockBackend) UpdateBusinessEventService(svc *model.BusinessEventService) error {
	if m.UpdateBusinessEventServiceFunc != nil {
		return m.UpdateBusinessEventServiceFunc(svc)
	}
	return nil
}

func (m *MockBackend) DeleteBusinessEventService(id model.ID) error {
	if m.DeleteBusinessEventServiceFunc != nil {
		return m.DeleteBusinessEventServiceFunc(id)
	}
	return nil
}

func (m *MockBackend) ListDatabaseConnections() ([]*model.DatabaseConnection, error) {
	if m.ListDatabaseConnectionsFunc != nil {
		return m.ListDatabaseConnectionsFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateDatabaseConnection(conn *model.DatabaseConnection) error {
	if m.CreateDatabaseConnectionFunc != nil {
		return m.CreateDatabaseConnectionFunc(conn)
	}
	return nil
}

func (m *MockBackend) UpdateDatabaseConnection(conn *model.DatabaseConnection) error {
	if m.UpdateDatabaseConnectionFunc != nil {
		return m.UpdateDatabaseConnectionFunc(conn)
	}
	return nil
}

func (m *MockBackend) MoveDatabaseConnection(conn *model.DatabaseConnection) error {
	if m.MoveDatabaseConnectionFunc != nil {
		return m.MoveDatabaseConnectionFunc(conn)
	}
	return nil
}

func (m *MockBackend) DeleteDatabaseConnection(id model.ID) error {
	if m.DeleteDatabaseConnectionFunc != nil {
		return m.DeleteDatabaseConnectionFunc(id)
	}
	return nil
}

func (m *MockBackend) ListDataTransformers() ([]*model.DataTransformer, error) {
	if m.ListDataTransformersFunc != nil {
		return m.ListDataTransformersFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateDataTransformer(dt *model.DataTransformer) error {
	if m.CreateDataTransformerFunc != nil {
		return m.CreateDataTransformerFunc(dt)
	}
	return nil
}

func (m *MockBackend) UpdateDataTransformer(dt *model.DataTransformer) error {
	if m.UpdateDataTransformerFunc != nil {
		return m.UpdateDataTransformerFunc(dt)
	}
	return nil
}

func (m *MockBackend) DeleteDataTransformer(id model.ID) error {
	if m.DeleteDataTransformerFunc != nil {
		return m.DeleteDataTransformerFunc(id)
	}
	return nil
}
