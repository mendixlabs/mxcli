// SPDX-License-Identifier: Apache-2.0

// Package model provides core types for Mendix model elements.
package model

import (
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// ID represents a unique identifier for model elements.
// In Mendix, these are typically UUIDs.
type ID string

// QualifiedName represents a fully qualified name in the format "Module.Element".
type QualifiedName string

// Point represents a position in 2D space.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Size represents dimensions in 2D space.
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Element is the base interface for all model elements.
type Element interface {
	GetID() ID
	GetTypeName() string
}

// NamedElement is an element with a name.
type NamedElement interface {
	Element
	GetName() string
}

// ContainedElement is an element that belongs to a container.
type ContainedElement interface {
	Element
	GetContainerID() ID
}

// BaseElement provides common fields for all model elements.
type BaseElement struct {
	ID       ID     `json:"$ID"`
	TypeName string `json:"$Type"`
}

// GetID returns the element's unique identifier.
func (e *BaseElement) GetID() ID {
	return e.ID
}

// GetTypeName returns the element's type name.
func (e *BaseElement) GetTypeName() string {
	return e.TypeName
}

// Unit represents a document unit in the Mendix model.
// Units are top-level elements like DomainModel, Microflow, Page, etc.
type Unit struct {
	BaseElement
	ContainerID ID     `json:"containerId"`
	Name        string `json:"name,omitempty"`
}

// GetName returns the unit's name.
func (u *Unit) GetName() string {
	return u.Name
}

// GetContainerID returns the ID of the containing element.
func (u *Unit) GetContainerID() ID {
	return u.ContainerID
}

// Module represents a Mendix module.
type Module struct {
	BaseElement
	Name                string `json:"name"`
	Documentation       string `json:"documentation,omitempty"`
	Excluded            bool   `json:"excluded,omitempty"`
	FromAppStore        bool   `json:"fromAppStore,omitempty"`
	AppStoreVersion     string `json:"appStoreVersion,omitempty"`
	AppStoreGuid        string `json:"appStoreGuid,omitempty"`
	IsReusableComponent bool   `json:"isReusableComponent,omitempty"`

	// Contained units
	DomainModelID ID   `json:"domainModelId,omitempty"`
	Documents     []ID `json:"documents,omitempty"`
}

// GetName returns the module's name.
func (m *Module) GetName() string {
	return m.Name
}

// Project represents a Mendix project.
type Project struct {
	BaseElement
	Name            string    `json:"name"`
	MendixVersion   string    `json:"mendixVersion"`
	ProjectID       string    `json:"projectId,omitempty"`
	IsSystemProject bool      `json:"isSystemProject,omitempty"`
	CreatedDate     time.Time `json:"createdDate,omitempty"`

	// Project-level settings
	Modules          []ID `json:"modules,omitempty"`
	ProjectDocuments []ID `json:"projectDocuments,omitempty"`
}

// GetName returns the project's name.
func (p *Project) GetName() string {
	return p.Name
}

// Folder represents a folder within a module for organizing documents.
type Folder struct {
	BaseElement
	ContainerID ID     `json:"containerId"`
	Name        string `json:"name"`
	Documents   []ID   `json:"documents,omitempty"`
	Folders     []ID   `json:"folders,omitempty"`
}

// GetName returns the folder's name.
func (f *Folder) GetName() string {
	return f.Name
}

// GetContainerID returns the ID of the containing element.
func (f *Folder) GetContainerID() ID {
	return f.ContainerID
}

// Text represents localized text.
type Text struct {
	BaseElement
	Translations map[string]string `json:"translations,omitempty"`
}

// GetTranslation returns the translation for a given language code.
func (t *Text) GetTranslation(languageCode string) string {
	if t.Translations == nil {
		return ""
	}
	return t.Translations[languageCode]
}

// Image represents an image reference.
type Image struct {
	BaseElement
	Name      string `json:"name,omitempty"`
	ImageData []byte `json:"imageData,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

// ConstantDataType represents the data type of a constant.
type ConstantDataType struct {
	Kind      string `json:"kind"`                // "String", "Integer", "Long", "Decimal", "Boolean", "DateTime", "Enumeration", "Binary"
	EnumRef   string `json:"enumRef,omitempty"`   // For Enumeration type: qualified name of the enumeration
	EntityRef string `json:"entityRef,omitempty"` // For Object/List types: qualified name of the entity
}

// Constant represents a constant value.
type Constant struct {
	BaseElement
	ContainerID     ID               `json:"containerId"`
	Name            string           `json:"name"`
	Documentation   string           `json:"documentation,omitempty"`
	Type            ConstantDataType `json:"type"`
	DefaultValue    string           `json:"defaultValue,omitempty"`
	ExposedToClient bool             `json:"exposedToClient,omitempty"`
	Excluded        bool             `json:"excluded,omitempty"`
	ExportLevel     string           `json:"exportLevel,omitempty"` // "Hidden" or "API"
}

// GetName returns the constant's name.
func (c *Constant) GetName() string {
	return c.Name
}

// GetContainerID returns the ID of the containing element.
func (c *Constant) GetContainerID() ID {
	return c.ContainerID
}

// Enumeration represents an enumeration type.
type Enumeration struct {
	BaseElement
	ContainerID   ID                 `json:"containerId"`
	Name          string             `json:"name"`
	Documentation string             `json:"documentation,omitempty"`
	Values        []EnumerationValue `json:"values,omitempty"`
}

// GetName returns the enumeration's name.
func (e *Enumeration) GetName() string {
	return e.Name
}

// GetContainerID returns the ID of the containing element.
func (e *Enumeration) GetContainerID() ID {
	return e.ContainerID
}

// EnumerationValue represents a value in an enumeration.
type EnumerationValue struct {
	BaseElement
	Name    string `json:"name"`
	Caption *Text  `json:"caption,omitempty"`
	Image   *Image `json:"image,omitempty"`
}

// GetName returns the enumeration value's name.
func (v *EnumerationValue) GetName() string {
	return v.Name
}

// RegularExpression represents a regular expression constraint.
type RegularExpression struct {
	BaseElement
	ContainerID   ID     `json:"containerId"`
	Name          string `json:"name"`
	Documentation string `json:"documentation,omitempty"`
	Expression    string `json:"expression"`
}

// GetName returns the regular expression's name.
func (r *RegularExpression) GetName() string {
	return r.Name
}

// GetContainerID returns the ID of the containing element.
func (r *RegularExpression) GetContainerID() ID {
	return r.ContainerID
}

// ScheduledEvent represents a scheduled event.
type ScheduledEvent struct {
	BaseElement
	ContainerID   ID         `json:"containerId"`
	Name          string     `json:"name"`
	Documentation string     `json:"documentation,omitempty"`
	MicroflowID   ID         `json:"microflowId,omitempty"`
	StartDateTime *time.Time `json:"startDateTime,omitempty"`
	TimeZone      string     `json:"timeZone,omitempty"`
	Interval      int        `json:"interval,omitempty"`
	IntervalType  string     `json:"intervalType,omitempty"`
	Enabled       bool       `json:"enabled"`
}

// GetName returns the scheduled event's name.
func (s *ScheduledEvent) GetName() string {
	return s.Name
}

// GetContainerID returns the ID of the containing element.
func (s *ScheduledEvent) GetContainerID() ID {
	return s.ContainerID
}

// MarshalJSON provides custom JSON marshaling.
func (e *BaseElement) MarshalJSON() ([]byte, error) {
	type Alias BaseElement
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	})
}

// DocumentType represents the type of a document.
type DocumentType string

const (
	DocumentTypeDomainModel           DocumentType = "DomainModels$DomainModel"
	DocumentTypeMicroflow             DocumentType = "Microflows$Microflow"
	DocumentTypeNanoflow              DocumentType = "Microflows$Nanoflow"
	DocumentTypePage                  DocumentType = "Pages$Page"
	DocumentTypeLayout                DocumentType = "Pages$Layout"
	DocumentTypeSnippet               DocumentType = "Pages$Snippet"
	DocumentTypeConstant              DocumentType = "Constants$Constant"
	DocumentTypeEnumeration           DocumentType = "Enumerations$Enumeration"
	DocumentTypeScheduledEvent        DocumentType = "ScheduledEvents$ScheduledEvent"
	DocumentTypeJavaAction            DocumentType = "JavaActions$JavaAction"
	DocumentTypeRule                  DocumentType = "Rules$Rule"
	DocumentTypeConsumedODataService  DocumentType = "Rest$ConsumedODataService"
	DocumentTypePublishedODataService DocumentType = "ODataPublish$PublishedODataService2"
	DocumentTypeJsonStructure         DocumentType = "JsonStructures$JsonStructure"
	DocumentTypeImportMapping         DocumentType = "ImportMappings$ImportMapping"
	DocumentTypeExportMapping         DocumentType = "ExportMappings$ExportMapping"
)

// ConsumedODataService represents a consumed OData service (OData client).
type ConsumedODataService struct {
	BaseElement
	ContainerID       ID     `json:"containerId"`
	Name              string `json:"name"`
	Documentation     string `json:"documentation,omitempty"`
	Version           string `json:"version,omitempty"`
	ServiceName       string `json:"serviceName,omitempty"`
	ODataVersion      string `json:"odataVersion,omitempty"`
	MetadataUrl       string `json:"metadataUrl,omitempty"`
	TimeoutExpression string `json:"timeoutExpression,omitempty"`
	ProxyType         string `json:"proxyType,omitempty"`
	Description       string `json:"description,omitempty"`
	Validated         bool   `json:"validated,omitempty"`
	Excluded          bool   `json:"excluded,omitempty"`

	// HTTP configuration (nested Microflows$HttpConfiguration part)
	HttpConfiguration *HttpConfiguration `json:"httpConfiguration,omitempty"`

	// Microflow references (BY_NAME)
	ConfigurationMicroflow string `json:"configurationMicroflow,omitempty"` // Microflow for configuring requests
	ErrorHandlingMicroflow string `json:"errorHandlingMicroflow,omitempty"` // Microflow for handling errors

	// Proxy constant references (BY_NAME to Constants$Constant)
	ProxyHost     string `json:"proxyHost,omitempty"`
	ProxyPort     string `json:"proxyPort,omitempty"`
	ProxyUsername string `json:"proxyUsername,omitempty"`
	ProxyPassword string `json:"proxyPassword,omitempty"`

	// Cached contract metadata (from $metadata endpoint)
	Metadata     string `json:"metadata,omitempty"`     // Full $metadata XML (EDMX/CSDL)
	MetadataHash string `json:"metadataHash,omitempty"` // SHA-256 hash of metadata for change detection

	// Mendix Catalog integration
	ApplicationId   string `json:"applicationId,omitempty"`
	EndpointId      string `json:"endpointId,omitempty"`
	CatalogUrl      string `json:"catalogUrl,omitempty"`
	EnvironmentType string `json:"environmentType,omitempty"`
}

// HttpConfiguration represents the HTTP transport configuration (Microflows$HttpConfiguration).
type HttpConfiguration struct {
	BaseElement
	UseAuthentication bool               `json:"useAuthentication,omitempty"`
	Username          string             `json:"username,omitempty"`          // Expression for username
	Password          string             `json:"password,omitempty"`          // Expression for password
	HttpMethod        string             `json:"httpMethod,omitempty"`        // Get, Post, Put, Patch, Delete, Head, Options
	OverrideLocation  bool               `json:"overrideLocation,omitempty"`  // Whether to use custom location
	CustomLocation    string             `json:"customLocation,omitempty"`    // Custom URL expression
	ClientCertificate string             `json:"clientCertificate,omitempty"` // Client certificate identifier
	HeaderEntries     []*HttpHeaderEntry `json:"headerEntries,omitempty"`
}

// HttpHeaderEntry represents a custom HTTP header (Microflows$HttpHeaderEntry).
type HttpHeaderEntry struct {
	BaseElement
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"` // Expression for value
}

// GetName returns the service's name.
func (s *ConsumedODataService) GetName() string {
	return s.Name
}

// GetContainerID returns the ID of the containing module.
func (s *ConsumedODataService) GetContainerID() ID {
	return s.ContainerID
}

// PublishedODataService represents a published OData service.
type PublishedODataService struct {
	BaseElement
	ContainerID         ID                     `json:"containerId"`
	Name                string                 `json:"name"`
	Documentation       string                 `json:"documentation,omitempty"`
	Path                string                 `json:"path,omitempty"`
	Namespace           string                 `json:"namespace,omitempty"`
	ServiceName         string                 `json:"serviceName,omitempty"`
	Version             string                 `json:"version,omitempty"`
	ODataVersion        string                 `json:"odataVersion,omitempty"`
	Summary             string                 `json:"summary,omitempty"`
	Description         string                 `json:"description,omitempty"`
	PublishAssociations bool                   `json:"publishAssociations,omitempty"`
	UseGeneralization   bool                   `json:"useGeneralization,omitempty"`
	AuthenticationTypes []string               `json:"authenticationTypes,omitempty"`
	AuthMicroflow       string                 `json:"authMicroflow,omitempty"`
	EntityTypes         []*PublishedEntityType `json:"entityTypes,omitempty"`
	EntitySets          []*PublishedEntitySet  `json:"entitySets,omitempty"`
	AllowedModuleRoles  []string               `json:"allowedModuleRoles,omitempty"`
	Excluded            bool                   `json:"excluded,omitempty"`
}

// GetName returns the service's name.
func (s *PublishedODataService) GetName() string {
	return s.Name
}

// GetContainerID returns the ID of the containing module.
func (s *PublishedODataService) GetContainerID() ID {
	return s.ContainerID
}

// PublishedEntityType represents an entity type published in an OData service.
type PublishedEntityType struct {
	BaseElement
	Entity      string             `json:"entity,omitempty"`      // BY_NAME reference to entity
	ExposedName string             `json:"exposedName,omitempty"` // Name exposed in OData service
	Summary     string             `json:"summary,omitempty"`
	Description string             `json:"description,omitempty"`
	Members     []*PublishedMember `json:"members,omitempty"`
}

// PublishedEntitySet represents an entity set published in an OData service.
type PublishedEntitySet struct {
	BaseElement
	ExposedName    string `json:"exposedName,omitempty"`
	EntityTypeName string `json:"entityTypeName,omitempty"` // Resolved entity type name
	ReadMode       string `json:"readMode,omitempty"`
	InsertMode     string `json:"insertMode,omitempty"`
	UpdateMode     string `json:"updateMode,omitempty"`
	DeleteMode     string `json:"deleteMode,omitempty"`
	UsePaging      bool   `json:"usePaging,omitempty"`
	PageSize       int    `json:"pageSize,omitempty"`
}

// PublishedMember represents a member (attribute/association/id) published in an OData entity type.
type PublishedMember struct {
	BaseElement
	Kind        string `json:"kind,omitempty"`        // "attribute", "association", "id"
	Name        string `json:"name,omitempty"`        // BY_NAME reference to attribute/association
	ExposedName string `json:"exposedName,omitempty"` // Name exposed in OData service
	Filterable  bool   `json:"filterable,omitempty"`
	Sortable    bool   `json:"sortable,omitempty"`
	IsPartOfKey bool   `json:"isPartOfKey,omitempty"`
}

// ============================================================================
// Database Connection (DatabaseConnector marketplace module)
// ============================================================================

// DatabaseConnection represents a DatabaseConnector$DatabaseConnection document.
type DatabaseConnection struct {
	BaseElement
	ContainerID          ID               `json:"containerId"`
	Name                 string           `json:"name"`
	DatabaseType         string           `json:"databaseType"`         // "PostgreSQL", "MSSQL", "Oracle"
	ConnectionString     string           `json:"connectionString"`     // BY_NAME ref to constant: "Module.ConstantName"
	ConnectionInputValue string           `json:"connectionInputValue"` // Actual JDBC URL for Studio Pro dev
	UserName             string           `json:"userName"`             // BY_NAME ref to constant
	Password             string           `json:"password"`             // BY_NAME ref to constant
	Documentation        string           `json:"documentation,omitempty"`
	Excluded             bool             `json:"excluded,omitempty"`
	ExportLevel          string           `json:"exportLevel,omitempty"`
	Queries              []*DatabaseQuery `json:"queries,omitempty"`
}

// DatabaseQuery represents a DatabaseConnector$DatabaseQuery.
type DatabaseQuery struct {
	BaseElement
	Name          string                    `json:"name"`
	QueryType     int                       `json:"queryType"`     // 1 = custom SQL
	SQL           string                    `json:"sql,omitempty"` // extracted from TableMappings
	TableMappings []*DatabaseTableMapping   `json:"tableMappings,omitempty"`
	Parameters    []*DatabaseQueryParameter `json:"parameters,omitempty"`
}

// DatabaseQueryParameter represents a DatabaseConnector$QueryParameter.
type DatabaseQueryParameter struct {
	BaseElement
	ParameterName         string `json:"parameterName"`
	DataType              string `json:"dataType"`              // e.g. "DataTypes$IntegerType", "DataTypes$StringType"
	DefaultValue          string `json:"defaultValue"`          // test value for Studio Pro
	EmptyValueBecomesNull bool   `json:"emptyValueBecomesNull"` // true = test with NULL
}

// DatabaseTableMapping represents a DatabaseConnector$TableMapping.
type DatabaseTableMapping struct {
	BaseElement
	Entity    string                   `json:"entity"`    // BY_NAME entity ref: "Module.Entity"
	TableName string                   `json:"tableName"` // SQL table name
	Columns   []*DatabaseColumnMapping `json:"columns,omitempty"`
}

// DatabaseColumnMapping represents a DatabaseConnector$ColumnMapping.
type DatabaseColumnMapping struct {
	BaseElement
	Attribute   string `json:"attribute"`   // BY_NAME attribute ref: "Module.Entity.Attr"
	ColumnName  string `json:"columnName"`  // SQL column name
	SqlDataType string `json:"sqlDataType"` // simplified type name for display
}

// ============================================================================
// Business Events
// ============================================================================

// BusinessEventService represents a BusinessEvents$BusinessEventService document.
type BusinessEventService struct {
	BaseElement
	ContainerID              ID                       `json:"containerId"`
	Name                     string                   `json:"name"`
	Documentation            string                   `json:"documentation,omitempty"`
	Excluded                 bool                     `json:"excluded,omitempty"`
	ExportLevel              string                   `json:"exportLevel,omitempty"`
	Definition               *BusinessEventDefinition `json:"definition,omitempty"`
	OperationImplementations []*ServiceOperation      `json:"operationImplementations,omitempty"`

	// Cached AsyncAPI contract (for consumed/client services)
	Document string `json:"document,omitempty"` // AsyncAPI YAML document
}

// GetName returns the service's name.
func (s *BusinessEventService) GetName() string {
	return s.Name
}

// GetContainerID returns the ID of the containing module.
func (s *BusinessEventService) GetContainerID() ID {
	return s.ContainerID
}

// BusinessEventDefinition represents BusinessEvents$BusinessEventDefinition.
type BusinessEventDefinition struct {
	BaseElement
	ServiceName     string                  `json:"serviceName"`
	EventNamePrefix string                  `json:"eventNamePrefix,omitempty"`
	Description     string                  `json:"description,omitempty"`
	Summary         string                  `json:"summary,omitempty"`
	Channels        []*BusinessEventChannel `json:"channels,omitempty"`
}

// BusinessEventChannel represents BusinessEvents$Channel.
type BusinessEventChannel struct {
	BaseElement
	ChannelName string                  `json:"channelName"`
	Description string                  `json:"description,omitempty"`
	Messages    []*BusinessEventMessage `json:"messages,omitempty"`
}

// BusinessEventMessage represents BusinessEvents$Message.
type BusinessEventMessage struct {
	BaseElement
	MessageName  string                    `json:"messageName"`
	Description  string                    `json:"description,omitempty"`
	CanPublish   bool                      `json:"canPublish"`
	CanSubscribe bool                      `json:"canSubscribe"`
	Attributes   []*BusinessEventAttribute `json:"attributes,omitempty"`
}

// BusinessEventAttribute represents BusinessEvents$MessageAttribute.
type BusinessEventAttribute struct {
	BaseElement
	AttributeName string `json:"attributeName"`
	AttributeType string `json:"attributeType"` // "Long", "String", "Integer", "Boolean", "DateTime", "Decimal"
	Description   string `json:"description,omitempty"`
}

// ServiceOperation represents BusinessEvents$ServiceOperation.
type ServiceOperation struct {
	BaseElement
	MessageName string `json:"messageName"`
	Operation   string `json:"operation"`           // "publish" or "subscribe"
	Entity      string `json:"entity"`              // BY_NAME qualified ref: "Module.EntityName"
	Microflow   string `json:"microflow,omitempty"` // BY_NAME qualified ref (optional handler)
}

// ============================================================================
// Published REST Services
// ============================================================================

// PublishedRestService represents a Rest$PublishedRestService document.
type PublishedRestService struct {
	BaseElement
	ContainerID ID                       `json:"containerId"`
	Name        string                   `json:"name"`
	Path        string                   `json:"path,omitempty"`
	Version     string                   `json:"version,omitempty"`
	ServiceName string                   `json:"serviceName,omitempty"`
	Excluded    bool                     `json:"excluded,omitempty"`
	Resources   []*PublishedRestResource `json:"resources,omitempty"`
}

// GetName returns the service's name.
func (s *PublishedRestService) GetName() string {
	return s.Name
}

// GetContainerID returns the ID of the containing element.
func (s *PublishedRestService) GetContainerID() ID {
	return s.ContainerID
}

// PublishedRestResource represents a Rest$PublishedRestServiceResource.
type PublishedRestResource struct {
	BaseElement
	Name       string                    `json:"name"`
	Operations []*PublishedRestOperation `json:"operations,omitempty"`
}

// PublishedRestOperation represents a Rest$PublishedRestServiceOperation.
type PublishedRestOperation struct {
	BaseElement
	Path       string `json:"path,omitempty"`
	HTTPMethod string `json:"httpMethod,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Microflow  string `json:"microflow,omitempty"`
	Deprecated bool   `json:"deprecated,omitempty"`
}

// ============================================================================
// Consumed REST Services
// ============================================================================

// ConsumedRestService represents a Rest$ConsumedRestService document.
type ConsumedRestService struct {
	BaseElement
	ContainerID    ID                     `json:"containerId"`
	Name           string                 `json:"name"`
	Documentation  string                 `json:"documentation,omitempty"`
	Excluded       bool                   `json:"excluded,omitempty"`
	BaseUrl        string                 `json:"baseUrl"`
	Authentication *RestAuthentication    `json:"authentication,omitempty"`
	Operations     []*RestClientOperation `json:"operations,omitempty"`
}

// GetName returns the service's name.
func (s *ConsumedRestService) GetName() string {
	return s.Name
}

// GetContainerID returns the ID of the containing element.
func (s *ConsumedRestService) GetContainerID() ID {
	return s.ContainerID
}

// RestAuthentication represents authentication configuration for a consumed REST service.
type RestAuthentication struct {
	Scheme   string `json:"scheme"`             // "Basic"
	Username string `json:"username,omitempty"` // literal value or constant reference
	Password string `json:"password,omitempty"` // literal value or constant reference
}

// RestClientOperation represents a single operation in a consumed REST service.
type RestClientOperation struct {
	Name             string                 `json:"name"`
	Documentation    string                 `json:"documentation,omitempty"`
	HttpMethod       string                 `json:"httpMethod"`                // "GET", "POST", etc.
	Path             string                 `json:"path"`                      // e.g. "/pet/{petId}"
	Parameters       []*RestClientParameter `json:"parameters,omitempty"`      // path parameters
	QueryParameters  []*RestClientParameter `json:"queryParameters,omitempty"` // query parameters
	Headers          []*RestClientHeader    `json:"headers,omitempty"`
	BodyType         string                 `json:"bodyType,omitempty"`     // "JSON", "FILE", ""
	BodyVariable     string                 `json:"bodyVariable,omitempty"` // variable name
	ResponseType     string                 `json:"responseType"`           // "JSON", "STRING", "FILE", "STATUS", "NONE"
	ResponseVariable string                 `json:"responseVariable,omitempty"`
	Timeout          int                    `json:"timeout,omitempty"` // 0 = default (300s)
}

// RestClientParameter represents a path or query parameter.
type RestClientParameter struct {
	Name     string `json:"name"`     // parameter name (without $ prefix)
	DataType string `json:"dataType"` // "String", "Integer", "Boolean", "Decimal"
}

// RestClientHeader represents an HTTP header in a REST client operation.
type RestClientHeader struct {
	Name  string `json:"name"`  // header name, e.g. "Accept"
	Value string `json:"value"` // literal, $var, or expression like "'Bearer ' + $Token"
}

// ============================================================================
// Project Settings
// ============================================================================

// ProjectSettings represents the single Settings$ProjectSettings document.
type ProjectSettings struct {
	BaseElement
	// Settings parts (polymorphic, dispatched by $Type)
	WebUI         *WebUISettings         `json:"webUI,omitempty"`
	Integration   *IntegrationSettings   `json:"integration,omitempty"`
	Configuration *ConfigurationSettings `json:"configuration,omitempty"`
	Model         *ModelSettings         `json:"model,omitempty"`
	Convention    *ConventionSettings    `json:"convention,omitempty"`
	Language      *LanguageSettings      `json:"language,omitempty"`
	Certificate   *CertificateSettings   `json:"certificate,omitempty"`
	Workflows     *WorkflowsSettings     `json:"workflows,omitempty"`
	JarDeployment *JarDeploymentSettings `json:"jarDeployment,omitempty"`
	Distribution  *DistributionSettings  `json:"distribution,omitempty"`
	// RawParts preserves the original BSON for round-trip fidelity
	RawParts []map[string]any `json:"-"`
}

// WebUISettings represents Forms$WebUIProjectSettingsPart.
type WebUISettings struct {
	BaseElement
	EnableMicroflowReachabilityAnalysis bool   `json:"enableMicroflowReachabilityAnalysis"`
	UseOptimizedClient                  string `json:"useOptimizedClient,omitempty"`
	UrlPrefix                           string `json:"urlPrefix,omitempty"`
}

// IntegrationSettings represents Settings$IntegrationProjectSettingsPart.
type IntegrationSettings struct {
	BaseElement
}

// ConfigurationSettings represents Settings$ConfigurationSettings.
type ConfigurationSettings struct {
	BaseElement
	Configurations []*ServerConfiguration `json:"configurations,omitempty"`
}

// ServerConfiguration represents Settings$ServerConfiguration.
type ServerConfiguration struct {
	BaseElement
	Name                          string           `json:"name"`
	DatabaseType                  string           `json:"databaseType,omitempty"`
	DatabaseUrl                   string           `json:"databaseUrl,omitempty"`
	DatabaseName                  string           `json:"databaseName,omitempty"`
	DatabaseUserName              string           `json:"databaseUserName,omitempty"`
	DatabasePassword              string           `json:"databasePassword,omitempty"`
	DatabaseUseIntegratedSecurity bool             `json:"databaseUseIntegratedSecurity"`
	HttpPortNumber                int              `json:"httpPortNumber,omitempty"`
	ServerPortNumber              int              `json:"serverPortNumber,omitempty"`
	ApplicationRootUrl            string           `json:"applicationRootUrl,omitempty"`
	MaxJavaHeapSize               int              `json:"maxJavaHeapSize,omitempty"`
	ExtraJvmParameters            string           `json:"extraJvmParameters,omitempty"`
	OpenAdminPort                 bool             `json:"openAdminPort"`
	OpenHttpPort                  bool             `json:"openHttpPort"`
	ConstantValues                []*ConstantValue `json:"constantValues,omitempty"`
}

// ConstantValue represents Settings$ConstantValue (constant override per configuration).
type ConstantValue struct {
	BaseElement
	ConstantId string `json:"constantId"` // Qualified name: "BusinessEvents.ServerUrl"
	Value      string `json:"value"`      // The overridden value
}

// ModelSettings represents Settings$ModelSettings.
type ModelSettings struct {
	BaseElement
	AfterStartupMicroflow              string `json:"afterStartupMicroflow,omitempty"`
	BeforeShutdownMicroflow            string `json:"beforeShutdownMicroflow,omitempty"`
	HealthCheckMicroflow               string `json:"healthCheckMicroflow,omitempty"`
	AllowUserMultipleSessions          bool   `json:"allowUserMultipleSessions"`
	HashAlgorithm                      string `json:"hashAlgorithm,omitempty"`
	BcryptCost                         int    `json:"bcryptCost,omitempty"`
	JavaVersion                        string `json:"javaVersion,omitempty"`
	RoundingMode                       string `json:"roundingMode,omitempty"`
	ScheduledEventTimeZoneCode         string `json:"scheduledEventTimeZoneCode,omitempty"`
	FirstDayOfWeek                     string `json:"firstDayOfWeek,omitempty"`
	DecimalScale                       int    `json:"decimalScale,omitempty"`
	EnableDataStorageOptimisticLocking bool   `json:"enableDataStorageOptimisticLocking"`
	UseDatabaseForeignKeyConstraints   bool   `json:"useDatabaseForeignKeyConstraints"`
}

// ConventionSettings represents Settings$ConventionSettings.
type ConventionSettings struct {
	BaseElement
	LowerCaseMicroflowVariables bool   `json:"lowerCaseMicroflowVariables"`
	DefaultAssociationStorage   string `json:"defaultAssociationStorage,omitempty"`
}

// LanguageSettings represents Settings$LanguageSettings.
type LanguageSettings struct {
	BaseElement
	DefaultLanguageCode string `json:"defaultLanguageCode,omitempty"`
}

// CertificateSettings represents Settings$CertificateSettings.
type CertificateSettings struct {
	BaseElement
}

// WorkflowsSettings represents Settings$WorkflowsProjectSettingsPart.
type WorkflowsSettings struct {
	BaseElement
	UserEntity                string `json:"userEntity,omitempty"`
	DefaultTaskParallelism    int    `json:"defaultTaskParallelism,omitempty"`
	WorkflowEngineParallelism int    `json:"workflowEngineParallelism,omitempty"`
}

// JarDeploymentSettings represents Settings$JarDeploymentSettings.
type JarDeploymentSettings struct {
	BaseElement
}

// DistributionSettings represents Settings$DistributionSettings.
type DistributionSettings struct {
	BaseElement
	IsDistributable bool   `json:"isDistributable"`
	Version         string `json:"version,omitempty"`
}

// ============================================================================
// JSON Structures
// ============================================================================

// JsonStructure represents a JsonStructures$JsonStructure document.
type JsonStructure struct {
	BaseElement
	ContainerID   ID             `json:"containerId"`
	Name          string         `json:"name"`
	Documentation string         `json:"documentation,omitempty"`
	Excluded      bool           `json:"excluded,omitempty"`
	ExportLevel   string         `json:"exportLevel,omitempty"`
	JsonSnippet   string         `json:"jsonSnippet,omitempty"`
	Elements      []*JsonElement `json:"elements,omitempty"`
}

// GetName returns the JSON structure's name.
func (j *JsonStructure) GetName() string { return j.Name }

// GetContainerID returns the ID of the containing module.
func (j *JsonStructure) GetContainerID() ID { return j.ContainerID }

// JsonElement represents a single element in a JSON structure's element tree.
type JsonElement struct {
	BaseElement
	ExposedName     string         `json:"exposedName,omitempty"`
	ExposedItemName string         `json:"exposedItemName,omitempty"`
	ElementType     string         `json:"elementType,omitempty"`   // "Object", "Array", "Value"
	PrimitiveType   string         `json:"primitiveType,omitempty"` // "String", "Integer", "Boolean", "Decimal", "DateTime", "Unknown"
	Path            string         `json:"path,omitempty"`
	OriginalValue   string         `json:"originalValue,omitempty"`
	MinOccurs       int            `json:"minOccurs,omitempty"`
	MaxOccurs       int            `json:"maxOccurs,omitempty"`
	FractionDigits  int            `json:"fractionDigits,omitempty"`
	TotalDigits     int            `json:"totalDigits,omitempty"`
	MaxLength       int            `json:"maxLength,omitempty"`
	Nillable        bool           `json:"nillable,omitempty"`
	IsDefaultType   bool           `json:"isDefaultType,omitempty"`
	Children        []*JsonElement `json:"children,omitempty"`
}

// ============================================================================
// Import Mappings
// ============================================================================

// ImportMapping represents an ImportMappings$ImportMapping document.
type ImportMapping struct {
	BaseElement
	ContainerID   ID     `json:"containerId"`
	Name          string `json:"name"`
	Documentation string `json:"documentation,omitempty"`
	Excluded      bool   `json:"excluded,omitempty"`
	ExportLevel   string `json:"exportLevel,omitempty"`
	// Schema source (at most one is set)
	JsonStructure     string `json:"jsonStructure,omitempty"`     // qualified name
	XmlSchema         string `json:"xmlSchema,omitempty"`         // qualified name
	MessageDefinition string `json:"messageDefinition,omitempty"` // qualified name
	// Mapping tree (top-level elements, usually one root)
	Elements []*ImportMappingElement `json:"elements,omitempty"`
}

// GetName returns the import mapping's name.
func (m *ImportMapping) GetName() string { return m.Name }

// GetContainerID returns the ID of the containing module.
func (m *ImportMapping) GetContainerID() ID { return m.ContainerID }

// ImportMappingElement represents either an object or value mapping element.
type ImportMappingElement struct {
	BaseElement
	// "Object" or "Value"
	Kind string `json:"kind"`
	// Object mapping fields
	Entity         string `json:"entity,omitempty"`         // qualified entity name
	ObjectHandling string `json:"objectHandling,omitempty"` // "Create", "Find", "FindOrCreate", "Custom"
	Association    string `json:"association,omitempty"`    // qualified association name
	// Value mapping fields
	Attribute string `json:"attribute,omitempty"` // qualified attribute name (Module.Entity.Attr)
	DataType  string `json:"dataType,omitempty"`  // "String", "Integer", "Boolean", etc.
	IsKey     bool   `json:"isKey,omitempty"`
	// Shared fields
	ExposedName string                  `json:"exposedName,omitempty"`
	JsonPath    string                  `json:"jsonPath,omitempty"`
	Children    []*ImportMappingElement `json:"children,omitempty"`
}

// ============================================================================
// Export Mappings
// ============================================================================

// ExportMapping represents an ExportMappings$ExportMapping document.
type ExportMapping struct {
	BaseElement
	ContainerID   ID     `json:"containerId"`
	Name          string `json:"name"`
	Documentation string `json:"documentation,omitempty"`
	Excluded      bool   `json:"excluded,omitempty"`
	ExportLevel   string `json:"exportLevel,omitempty"`
	// Schema source (at most one is set)
	JsonStructure     string `json:"jsonStructure,omitempty"`     // qualified name
	XmlSchema         string `json:"xmlSchema,omitempty"`         // qualified name
	MessageDefinition string `json:"messageDefinition,omitempty"` // qualified name
	// NullValueOption controls how null values are serialized: "LeaveOutElement" or "SendAsNil"
	NullValueOption string                  `json:"nullValueOption,omitempty"`
	Elements        []*ExportMappingElement `json:"elements,omitempty"`
}

// GetName returns the export mapping's name.
func (m *ExportMapping) GetName() string { return m.Name }

// GetContainerID returns the ID of the containing module.
func (m *ExportMapping) GetContainerID() ID { return m.ContainerID }

// ExportMappingElement represents either an object or value mapping element in an export mapping.
type ExportMappingElement struct {
	BaseElement
	// "Object" or "Value"
	Kind string `json:"kind"`
	// Object mapping fields
	Entity      string `json:"entity,omitempty"`      // qualified entity name
	Association string `json:"association,omitempty"` // qualified association name (VIA clause)
	// Value mapping fields
	Attribute string `json:"attribute,omitempty"` // qualified attribute name (Module.Entity.Attr)
	DataType  string `json:"dataType,omitempty"`  // "String", "Integer", "Boolean", etc.
	// Shared fields
	ExposedName string                  `json:"exposedName,omitempty"`
	JsonPath    string                  `json:"jsonPath,omitempty"`
	Children    []*ExportMappingElement `json:"children,omitempty"`
}

// UnknownElement is a generic fallback for BSON elements with unrecognized $Type values.
// It preserves all raw BSON fields so developers can diagnose unimplemented types
// without silent data loss.
//
// FieldKinds maps each raw field name to its inferred Mendix property kind
// (e.g. "primitive", "part", "by-name-reference", "collection:part-primary").
// This guides implementors in writing a proper parser without inspecting the
// mendixmodelsdk JS source manually.
type UnknownElement struct {
	BaseElement
	Position   Point             `json:"position,omitempty"`
	Name       string            `json:"name,omitempty"`
	Caption    string            `json:"caption,omitempty"`
	RawDoc     bson.D            `json:"-"`
	FieldKinds map[string]string `json:"-"`
}

// GetPosition returns the element's position (satisfies microflows.MicroflowObject).
func (u *UnknownElement) GetPosition() Point { return u.Position }

// SetPosition sets the element's position (satisfies microflows.MicroflowObject).
func (u *UnknownElement) SetPosition(p Point) { u.Position = p }

// GetName returns the element's name (satisfies workflows.WorkflowActivity).
func (u *UnknownElement) GetName() string { return u.Name }

// GetCaption returns the element's caption (satisfies workflows.WorkflowActivity).
func (u *UnknownElement) GetCaption() string { return u.Caption }

// ActivityType returns the type name (satisfies workflows.WorkflowActivity).
func (u *UnknownElement) ActivityType() string { return u.TypeName }
