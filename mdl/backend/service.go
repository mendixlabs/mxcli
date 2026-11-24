// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/model"
)

// ServiceBackend provides operations for OData, REST, business event,
// database connection, and data transformer services.
type ServiceBackend interface {
	ODataBackend
	RESTBackend
	BusinessEventBackend
	ChangeDataCaptureBackend
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
	// MovePublishedODataService reparents the service document to an
	// already-updated ContainerID, leaving its contents alone.
	MovePublishedODataService(svc *model.PublishedODataService) error
	DeletePublishedODataService(id model.ID) error
}

// RESTBackend manages consumed and published REST services.
type RESTBackend interface {
	ListConsumedRestServices() ([]*model.ConsumedRestService, error)
	ListPublishedRestServices() ([]*model.PublishedRestService, error)
	CreateConsumedRestService(svc *model.ConsumedRestService) error
	UpdateConsumedRestService(svc *model.ConsumedRestService) error
	// EditConsumedRestService edits a stored consumed REST service in place
	// (`alter consumed rest service`): what the edit does not name is written
	// back as stored.
	EditConsumedRestService(id model.ID, edit *ConsumedRestServiceEdit) error
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

// ChangeDataCaptureBackend reads entity change data capture services.
//
// Read-only for now: the write path is part C of the Kafka proposal, and it is not a
// small one — the baseline identifies what it tracks by GUID, the identity the
// runtime keys the database on, so a writer that mints those instead of reading
// them from the domain model loses a table's rows on the next deploy with no
// build error (CLAUDE.md).
type ChangeDataCaptureBackend interface {
	ListChangeDataCaptureServices() ([]*model.ChangeDataCaptureService, error)
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

// ConsumedRestServiceEdit is an in-place edit of a stored consumed REST
// service. Every operation it does not drop or modify is written back as its
// stored bytes, keeping whatever Studio Pro gave it that MDL has no words for
// (mendixlabs/mxcli#1608).
type ConsumedRestServiceEdit struct {
	// BaseUrl, when non-nil, replaces the service's base URL.
	BaseUrl *string
	// SetAuthentication replaces the authentication scheme with
	// Authentication; nil is none.
	SetAuthentication bool
	Authentication    *model.RestAuthentication
	// Drop names the stored operations to remove.
	Drop []string
	// Modify rewrites some properties of stored operations.
	Modify []ConsumedRestOperationEdit
	// Add appends new operations after the stored ones, in order.
	Add []*model.RestClientOperation
}

// ConsumedRestOperationEdit rewrites some properties of one stored operation.
type ConsumedRestOperationEdit struct {
	// Name is the stored operation's name.
	Name string
	// Operation is the operation as it reads after the edit.
	Operation *model.RestClientOperation
	// Keys are the MDL properties to write from Operation — Method, Path,
	// Parameters, Query, Headers, Body, Timeout, Response. Every other stored
	// property of the operation is kept.
	Keys []string
}
