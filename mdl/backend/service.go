// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// ServiceBackend provides operations for OData, REST, business event,
// database connection, and data transformer services.
type ServiceBackend interface {
	ODataBackend
	RESTBackend
	BusinessEventBackend
	DatabaseConnectionBackend
	DataTransformerBackend
}

// ODataBackend manages consumed and published OData services.
type ODataBackend interface {
	ListConsumedODataServices() ([]*model.ConsumedODataService, error)
	ListPublishedODataServices() ([]*model.PublishedODataService, error)
	CreateConsumedODataService(svc *model.ConsumedODataService) error
	UpdateConsumedODataService(svc *model.ConsumedODataService) error
	DeleteConsumedODataService(id model.ID) error
	CreatePublishedODataService(svc *model.PublishedODataService) error
	UpdatePublishedODataService(svc *model.PublishedODataService) error
	DeletePublishedODataService(id model.ID) error
}

// RESTBackend manages consumed and published REST services.
type RESTBackend interface {
	ListConsumedRestServices() ([]*model.ConsumedRestService, error)
	ListPublishedRestServices() ([]*model.PublishedRestService, error)
	CreateConsumedRestService(svc *model.ConsumedRestService) error
	UpdateConsumedRestService(svc *model.ConsumedRestService) error
	DeleteConsumedRestService(id model.ID) error
	CreatePublishedRestService(svc *model.PublishedRestService) error
	UpdatePublishedRestService(svc *model.PublishedRestService) error
	DeletePublishedRestService(id model.ID) error
}

// BusinessEventBackend manages business event services.
type BusinessEventBackend interface {
	ListBusinessEventServices() ([]*model.BusinessEventService, error)
	CreateBusinessEventService(svc *model.BusinessEventService) error
	UpdateBusinessEventService(svc *model.BusinessEventService) error
	DeleteBusinessEventService(id model.ID) error
}

// DatabaseConnectionBackend manages database connections.
type DatabaseConnectionBackend interface {
	ListDatabaseConnections() ([]*model.DatabaseConnection, error)
	CreateDatabaseConnection(conn *model.DatabaseConnection) error
	UpdateDatabaseConnection(conn *model.DatabaseConnection) error
	MoveDatabaseConnection(conn *model.DatabaseConnection) error
	DeleteDatabaseConnection(id model.ID) error
}

// DataTransformerBackend manages data transformers.
type DataTransformerBackend interface {
	ListDataTransformers() ([]*model.DataTransformer, error)
	CreateDataTransformer(dt *model.DataTransformer) error
	UpdateDataTransformer(dt *model.DataTransformer) error
	DeleteDataTransformer(id model.ID) error
}
