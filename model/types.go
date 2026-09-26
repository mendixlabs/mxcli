// SPDX-License-Identifier: Apache-2.0

// Package model provides core types for Mendix model elements.
package model

import (
	"encoding/json"
	"strings"
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
	ContainerID   ID     `json:"containerId"`
	Name          string `json:"name"`
	Documentation string `json:"documentation,omitempty"`
	// Excluded mirrors Studio Pro's "Exclude from project". It is model state
	// that MDL cannot express for an enumeration, so every write path must
	// carry the stored value rather than default it to false (#914).
	Excluded bool               `json:"excluded,omitempty"`
	Values   []EnumerationValue `json:"values,omitempty"`
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

// RegularExpression represents a named regex document
// (RegularExpressions$RegularExpression).
//
// It is a document rather than a string on a validation rule because Mendix
// stores the rule's reference by qualified name
// (DomainModels$RegExRuleInfo.RegExIdentifier), so one pattern is shared by
// every attribute that validates against it.
type RegularExpression struct {
	BaseElement
	ContainerID   ID     `json:"containerId"`
	Name          string `json:"name"`
	Documentation string `json:"documentation,omitempty"`
	// Expression is the pattern. Mendix stores it under the BSON key
	// "Expression" — NOT "RegEx", which is what modelsdk/gen binds.
	Expression  string `json:"expression"`
	ExportLevel string `json:"exportLevel,omitempty"`
	Excluded    bool   `json:"excluded,omitempty"`
}

// GetName returns the regular expression's name.
func (r *RegularExpression) GetName() string {
	return r.Name
}

// GetContainerID returns the ID of the containing element.
func (r *RegularExpression) GetContainerID() ID {
	return r.ContainerID
}

// ScheduleKind identifies which ScheduledEvents$*Schedule variant a schedule is.
// The variants differ in which fields they carry, so the kind has to be decided
// before any field is read or written — see Schedule.
type ScheduleKind string

const (
	ScheduleMinute       ScheduleKind = "Minute"
	ScheduleHour         ScheduleKind = "Hour"
	ScheduleDay          ScheduleKind = "Day"
	ScheduleWeek         ScheduleKind = "Week"
	ScheduleMonthDate    ScheduleKind = "MonthDate"
	ScheduleMonthWeekday ScheduleKind = "MonthWeekday"
	ScheduleYearDate     ScheduleKind = "YearDate"
	ScheduleYearWeekday  ScheduleKind = "YearWeekday"
)

// Schedule is the repeat rule of a scheduled event — the polymorphic
// ScheduledEvents$Schedule child, flattened into one struct with Kind saying
// which variant it is.
//
// It is flat rather than eight types because the fields overlap heavily and the
// consumers (a formatter, a serializer, a describe) all switch on the kind
// anyway. Only the fields listed for a kind are written; the rest are ignored:
//
//	Minute        Multiplier
//	Hour          Multiplier, MinuteOffset
//	Day           HourOfDay, MinuteOfHour
//	Week          Weekdays, HourOfDay, MinuteOfHour
//	MonthDate     Multiplier, MonthOffset, DayOfMonth, HourOfDay, MinuteOfHour
//	MonthWeekday  Multiplier, MonthOffset, DaySelector, Weekday, HourOfDay, MinuteOfHour
//	YearDate      Month, DayOfMonth, HourOfDay, MinuteOfHour
//	YearWeekday   Month, DaySelector, Weekday, HourOfDay, MinuteOfHour
type Schedule struct {
	Kind ScheduleKind `json:"kind"`

	// Multiplier is the repeat count ("every N minutes/hours/months").
	// Day, Week and the two Year variants have no multiplier in the metamodel.
	Multiplier int `json:"multiplier,omitempty"`
	// MinuteOffset is the minute within the hour, for the Hour variant only.
	MinuteOffset int `json:"minuteOffset,omitempty"`
	// MonthOffset selects which month of a multi-month cycle fires.
	MonthOffset int `json:"monthOffset,omitempty"`

	HourOfDay    int `json:"hourOfDay,omitempty"`
	MinuteOfHour int `json:"minuteOfHour,omitempty"`

	// DayOfMonth is 1-31; Month is 1-12.
	DayOfMonth int `json:"dayOfMonth,omitempty"`
	Month      int `json:"month,omitempty"`

	// Weekdays holds the seven per-day flags of the Week variant, Sunday first.
	Weekdays [7]bool `json:"weekdays,omitempty"`
	// DaySelector is First|Second|Third|Fourth|Last, Weekday is Sunday..Saturday.
	DaySelector string `json:"daySelector,omitempty"`
	Weekday     string `json:"weekday,omitempty"`
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
	// Schedule is the repeat rule. Every Studio Pro-authored event carries one;
	// nil means the document did not have the child.
	Schedule *Schedule `json:"schedule,omitempty"`
	// OnOverlap is SkipNext or DelayNext — what happens when a run is still
	// going when the next one is due. This is a scheduled event's own
	// concurrency control; it does not go through a task queue.
	OnOverlap   string `json:"onOverlap,omitempty"`
	ExportLevel string `json:"exportLevel,omitempty"`
	Excluded    bool   `json:"excluded,omitempty"`
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

	// Microflow reference (BY_NAME). Studio Pro's "Configuration source"
	// dropdown selects a configuration microflow. The BSON storage field was
	// renamed across Mendix versions (see ODataConfigMicroflowBSONKey): Studio
	// Pro >= 11.10 stores it under `ConfigurationEntityMicroflow` and ignores
	// the pre-11.10 `ConfigurationMicroflow` key, silently showing "Constants
	// only" (issue #728). The MDL `HeadersMicroflow` keyword is an alias that
	// writes to this same field.
	ConfigurationMicroflow string `json:"configurationMicroflow,omitempty"` // BSON: version-gated, see ODataConfigMicroflowBSONKey
	// HeadersMicroflow is the "Headers microflow" dropdown option (returns a
	// list of System.HttpHeader). It is a DIFFERENT storage slot from the
	// configuration microflow on Mendix >= 11.10 (see ODataHeadersMicroflowBSONKey):
	// mapping it to the configuration slot makes Studio Pro demand a
	// System.ConsumedODataConfiguration return type (CE6808).
	HeadersMicroflow       string `json:"headersMicroflow,omitempty"`       // BSON: version-gated, see ODataHeadersMicroflowBSONKey
	ErrorHandlingMicroflow string `json:"errorHandlingMicroflow,omitempty"` // BSON: ErrorHandlingMicroflow

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

// ODataConfigMicroflowBSONKey returns the BSON storage field for a consumed
// OData service's configuration microflow, which Mendix renamed across versions.
// From the Rest$ConsumedODataService reflection metadata:
//
//	configurationMicroflow:       introduced 10.12.0, DELETED 11.10.0
//	configurationEntityMicroflow: introduced 11.10.0
//
// Writing the pre-11.10 key on a >= 11.10 project makes Studio Pro ignore the
// unknown field and fall back to "Constants only" (issue #728). Callers pass the
// project's major/minor version.
func ODataConfigMicroflowBSONKey(major, minor int) string {
	if major > 11 || (major == 11 && minor >= 10) {
		return "ConfigurationEntityMicroflow"
	}
	return "ConfigurationMicroflow"
}

// ODataConfigMicroflowBSONKeys lists every BSON field name the configuration
// microflow has been stored under, across Mendix versions, so readers can accept
// a service authored by any version (or by Studio Pro on a different version).
func ODataConfigMicroflowBSONKeys() []string {
	return []string{"ConfigurationEntityMicroflow", "ConfigurationMicroflow", "HeadersMicroflow"}
}

// ODataHeadersMicroflowBSONKey returns the BSON storage field for a consumed
// OData service's "Headers microflow" (returns list of System.HttpHeader). On
// Mendix >= 11.10 this is a distinct slot, HeaderListMicroflow; before 11.10 the
// configuration and headers microflows shared the single ConfigurationMicroflow
// field (Studio Pro distinguished them by the microflow's return type). Writing
// a headers microflow into the configuration slot triggers CE6808 (issue #728).
func ODataHeadersMicroflowBSONKey(major, minor int) string {
	if major > 11 || (major == 11 && minor >= 10) {
		return "HeaderListMicroflow"
	}
	return "ConfigurationMicroflow"
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
	ContainerID         ID     `json:"containerId"`
	Name                string `json:"name"`
	Documentation       string `json:"documentation,omitempty"`
	Path                string `json:"path,omitempty"`
	Namespace           string `json:"namespace,omitempty"`
	ServiceName         string `json:"serviceName,omitempty"`
	Version             string `json:"version,omitempty"`
	ODataVersion        string `json:"odataVersion,omitempty"`
	Summary             string `json:"summary,omitempty"`
	Description         string `json:"description,omitempty"`
	PublishAssociations bool   `json:"publishAssociations,omitempty"`
	// SupportsGraphQL publishes the same resources over GraphQL as well as
	// OData. One boolean, one extra endpoint; the OData surface is unchanged.
	SupportsGraphQL     bool                   `json:"supportsGraphQL,omitempty"`
	UseGeneralization   bool                   `json:"useGeneralization,omitempty"`
	AuthenticationTypes []string               `json:"authenticationTypes,omitempty"`
	AuthMicroflow       string                 `json:"authMicroflow,omitempty"`
	EntityTypes         []*PublishedEntityType `json:"entityTypes,omitempty"`
	EntitySets          []*PublishedEntitySet  `json:"entitySets,omitempty"`
	// Microflows are published as OData actions (ActionImport in $metadata).
	Microflows         []*PublishedMicroflow `json:"microflows,omitempty"`
	AllowedModuleRoles []string              `json:"allowedModuleRoles,omitempty"`
	Excluded           bool                  `json:"excluded,omitempty"`
}

// PublishedMicroflow is a microflow exposed as an OData action.
type PublishedMicroflow struct {
	BaseElement
	Microflow   string `json:"microflow,omitempty"`   // BY_NAME ref, Module.Microflow
	ExposedName string `json:"exposedName,omitempty"` // the ActionImport name
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
	// The microflow's own return type, read off the microflow rather than
	// authored so the two cannot drift. Kind and Ref are kept apart on purpose:
	// "Module.X" alone cannot say whether X is an entity or an enumeration —
	// the ambiguity CLAUDE.md documents for the MDL visitor.
	ReturnTypeKind string                         `json:"returnTypeKind,omitempty"` // Boolean|String|…|Object|List|Enumeration
	ReturnTypeRef  string                         `json:"returnTypeRef,omitempty"`  // qualified name for Object/List/Enumeration
	Parameters     []*PublishedMicroflowParameter `json:"parameters,omitempty"`
}

// PublishedMicroflowParameter is one argument of a published microflow.
type PublishedMicroflowParameter struct {
	BaseElement
	// MicroflowParameter is a BY_NAME ref of the form Module.Microflow.Param,
	// the same shape the published-REST writer uses.
	MicroflowParameter string `json:"microflowParameter,omitempty"`
	ExposedName        string `json:"exposedName,omitempty"`
	DataTypeKind       string `json:"dataTypeKind,omitempty"`
	DataTypeRef        string `json:"dataTypeRef,omitempty"`
	CanBeEmpty         bool   `json:"canBeEmpty,omitempty"`
	Summary            string `json:"summary,omitempty"`
	Description        string `json:"description,omitempty"`
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

	// OData query options. nil means "not specified" and is written as Mendix's
	// own default of true; only an explicit false turns one off.
	Countable     *bool `json:"countable,omitempty"`
	SkipSupported *bool `json:"skipSupported,omitempty"`
	TopSupported  *bool `json:"topSupported,omitempty"`
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

	// EdmType is the OData EDM type the attribute is published as (e.g.
	// "Edm.String"). Studio Pro stores this on every ODataPublish$PublishedAttribute;
	// without it `mx check` reports CE5016 ("published as ."). Attribute members only.
	EdmType string `json:"edmType,omitempty"`

	// EnumerationAsString publishes an enumeration attribute as Edm.String
	// rather than as its own EDM enum type. The two are one setting, not two:
	// with this false, Mendix expects the enumeration itself to be published in
	// the service and rejects Edm.String (CE5016 + CE4583 "Enumeration is not
	// published in this service"). Attribute members only.
	EnumerationAsString bool `json:"enumerationAsString,omitempty"`

	// Association-specific fields (Kind == "association"). Studio Pro's
	// ODataPublish$PublishedAssociationEnd records both the association
	// target entity (qualified name) and the bare association name
	// alongside ExposedName (the navigation property name).
	AssociationTargetEntity string `json:"associationTargetEntity,omitempty"`
	ExposedAssociationName  string `json:"exposedAssociationName,omitempty"`

	// IsMany is the exposed navigation's multiplicity (to-many vs to-one).
	// Studio Pro stores it on every PublishedAssociationEnd; without it
	// `mx check` reports CE5022 ("changed multiplicity"). Association members only.
	IsMany bool `json:"isMany,omitempty"`
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
	Name      string `json:"name"`
	QueryType int    `json:"queryType"` // 1 = custom SQL (Mendix <= 11.12 storage)
	// QueryTypeName is the Mendix 11.13+ `Type` enum member ("Select" /
	// "NonSelect" / "Unknown"), which replaced the integer QueryType. Empty on a
	// project that stores the legacy key, or on a query mxcli has just authored;
	// see mdl/dbconnector.
	QueryTypeName string                    `json:"queryTypeName,omitempty"`
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
	ContainerID  ID                       `json:"containerId"`
	Name         string                   `json:"name"`
	Path         string                   `json:"path,omitempty"`
	Version      string                   `json:"version,omitempty"`
	ServiceName  string                   `json:"serviceName,omitempty"`
	Excluded     bool                     `json:"excluded,omitempty"`
	AllowedRoles []string                 `json:"allowedRoles,omitempty"`
	Resources    []*PublishedRestResource `json:"resources,omitempty"`
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
	Path       string   `json:"path,omitempty"`
	HTTPMethod string   `json:"httpMethod,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Microflow  string   `json:"microflow,omitempty"`
	Deprecated bool     `json:"deprecated,omitempty"`
	Parameters []string `json:"parameters,omitempty"` // path parameter names extracted from {param} in Path
	// OperationParameters are the operation's parameters as Studio Pro derives them from
	// its microflow. Empty when the microflow could not be read; the writer then writes
	// the path placeholders alone, as String path parameters.
	OperationParameters []*PublishedRestOperationParameter `json:"operationParameters,omitempty"`
}

// PathParameterNames returns the names of the {name} placeholders in the operation's
// path, in order.
func (op *PublishedRestOperation) PathParameterNames() []string {
	var names []string
	path := op.Path
	for {
		start := strings.Index(path, "{")
		if start < 0 {
			break
		}
		end := strings.Index(path[start:], "}")
		if end < 0 {
			break
		}
		names = append(names, path[start+1:start+end])
		path = path[start+end+1:]
	}
	return names
}

// PublishedRestOperationParameter is one parameter of a published REST operation. It
// binds to the operation microflow's parameter of the same name.
type PublishedRestOperationParameter struct {
	Name          string `json:"name"`
	ParameterType string `json:"parameterType"`           // "Path", "Query" or "Body"
	DataType      string `json:"dataType"`                // the microflow parameter's type: "String", "Integer", "Object", ...
	QualifiedName string `json:"qualifiedName,omitempty"` // the entity of an Object or List, or the enumeration
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
	OpenApiContent string                 `json:"openApiContent,omitempty"` // raw spec text (stored in OpenApiFile.Content BSON field)
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
	Tags             []string               `json:"tags,omitempty"`            // resource group labels (from OpenAPI tags[0])
	Parameters       []*RestClientParameter `json:"parameters,omitempty"`      // path parameters
	QueryParameters  []*RestClientParameter `json:"queryParameters,omitempty"` // query parameters
	Headers          []*RestClientHeader    `json:"headers,omitempty"`
	BodyType         string                 `json:"bodyType,omitempty"`     // "JSON", "FILE", "TEMPLATE", "EXPORT_MAPPING", ""
	BodyVariable     string                 `json:"bodyVariable,omitempty"` // variable name, template expression, or entity name
	BodyMappings     []*RestResponseMapping `json:"bodyMappings,omitempty"` // export mapping tree (Entity → JSON) for EXPORT_MAPPING bodies
	ResponseType     string                 `json:"responseType"`           // "JSON", "STRING", "FILE", "STATUS", "NONE", "MAPPING"
	ResponseVariable string                 `json:"responseVariable,omitempty"`
	ResponseEntity   string                 `json:"responseEntity,omitempty"`   // target entity for implicit mapping response
	ResponseMappings []*RestResponseMapping `json:"responseMappings,omitempty"` // JSON field → entity attribute
	Timeout          int                    `json:"timeout,omitempty"`          // 0 = default (300s)
}

// RestResponseMapping represents one element in a response mapping tree.
// It is either a value mapping (Attribute set, Entity empty) or an object mapping
// (Entity set, with its own Children).
type RestResponseMapping struct {
	// Value mapping: maps a JSON field to an entity attribute
	Attribute   string `json:"attribute,omitempty"` // entity attribute short name
	ExposedName string `json:"exposedName"`         // JSON field name
	JsonPath    string `json:"jsonPath,omitempty"`  // e.g. "(Object)|args|queryparam_1"

	// Object mapping: nested entity linked by association
	Entity      string                 `json:"entity,omitempty"`      // child entity (e.g. "RestDemo.Args")
	Association string                 `json:"association,omitempty"` // e.g. "RestDemo.Args_PostDemo2Response"
	Children    []*RestResponseMapping `json:"children,omitempty"`    // recursive children
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
// Data Transformers (Mendix 11.9+)
// ============================================================================

// DataTransformer represents a DataTransformers$DataTransformer document.
type DataTransformer struct {
	BaseElement
	ContainerID ID                     `json:"containerId"`
	Name        string                 `json:"name"`
	SourceType  string                 `json:"sourceType,omitempty"` // "JSON", "XML"
	SourceJSON  string                 `json:"sourceJson,omitempty"` // source content
	Steps       []*DataTransformerStep `json:"steps,omitempty"`
	Excluded    bool                   `json:"excluded,omitempty"`
}

// GetName returns the transformer's name.
func (t *DataTransformer) GetName() string { return t.Name }

// GetContainerID returns the ID of the containing element.
func (t *DataTransformer) GetContainerID() ID { return t.ContainerID }

// DataTransformerStep represents a single transformation step.
type DataTransformerStep struct {
	Technology string `json:"technology"` // "JSLT", "XSLT"
	Expression string `json:"expression"` // the transformation expression
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
	Value      string `json:"value"`      // The overridden value (empty when IsPrivate)
	// IsPrivate marks an override whose value is private: the stored
	// SharedOrPrivateValue is a Settings$PrivateValue, a marker type with no
	// properties, because the value lives on the developer's workstation and is
	// deliberately kept out of the shared model. Value is therefore always empty
	// here — "" means "not in the model", not "overridden with the empty string".
	// mxcli preserves the choice and never authors it.
	IsPrivate bool `json:"isPrivate,omitempty"`
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
	DefaultTimeZoneCode                string `json:"defaultTimeZoneCode,omitempty"`
	FirstDayOfWeek                     string `json:"firstDayOfWeek,omitempty"`
	DecimalScale                       int    `json:"decimalScale,omitempty"`
	EnableDataStorageOptimisticLocking bool   `json:"enableDataStorageOptimisticLocking"`
	UseDatabaseForeignKeyConstraints   bool   `json:"useDatabaseForeignKeyConstraints"`
	UseOQLVersion2                     bool   `json:"useOQLVersion2"`
	UseSystemContextForBackgroundTasks bool   `json:"useSystemContextForBackgroundTasks"`
	SslCertificateAlgorithm            string `json:"sslCertificateAlgorithm,omitempty"`
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
	DefaultLanguageCode string     `json:"defaultLanguageCode,omitempty"`
	Languages           []Language `json:"languages,omitempty"`
}

// Language represents a Texts$Language entry in the project language settings.
// The Languages slice is populated by parseLanguageSettings and is available
// for use by settings describers and future language-aware commands.
type Language struct {
	Code                 string `json:"code"`
	CheckCompleteness    bool   `json:"checkCompleteness,omitempty"`
	CustomDateFormat     string `json:"customDateFormat,omitempty"`
	CustomDateTimeFormat string `json:"customDateTimeFormat,omitempty"`
	CustomTimeFormat     string `json:"customTimeFormat,omitempty"`
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
	// Groups are the workflow groups from App > Settings > Workflows > Groups.
	// The runtime materialises one System.WorkflowGroup per entry, which is what
	// a user task's group targeting selects from.
	Groups []WorkflowGroup `json:"groups,omitempty"`
	// GroupsIncomplete records that the stored Groups list held an element the
	// read could not convert. The write path rebuilds the list from Groups, so
	// rewriting it would drop that element; UpdateProjectSettings refuses
	// instead (ADR-0005 guard-don't-drop).
	GroupsIncomplete bool `json:"-"`
}

// WorkflowGroup represents a Settings$WorkflowGroup: one entry of the workflows
// settings part's Groups list. Name and Description are the only two properties
// the type declares (modelsdk/gen, generated/metamodel and mendixmodelsdk 4.115.0
// all agree), and there is no separate identifier — Name is the group's identity,
// which is how ALTER SETTINGS WORKFLOWS ... GROUP addresses one.
type WorkflowGroup struct {
	BaseElement
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
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
// Import Mappings
// ============================================================================

// ImportMapping represents an ImportMappings$ImportMapping document.
// WebServiceMappingSource is a mapping's SOAP binding: the imported web service
// document plus which service, operation and root element of it the mapping
// covers.
//
// It is read-only. mxcli cannot author a SOAP mapping, and the point of reading
// it is precisely that it cannot: a rewrite that dropped these turned a working
// integration into CE6896 "A mapping must have exactly one schema source" and
// CE0270 "No root element could be found in the schema" (ako/mxcli#365).
//
// ParameterName and IsHeader exist on EXPORT mappings only — which SOAP message
// part the mapping produces, and whether it is a header — and are carried for
// the same reason.
type WebServiceMappingSource struct {
	ImportedWebService string `json:"importedWebService,omitempty"` // stored as wsdlFile
	ServiceName        string `json:"serviceName,omitempty"`
	OperationName      string `json:"operationName,omitempty"`
	RootElementName    string `json:"rootElementName,omitempty"` // stored as xsdRootElementName
	ParameterName      string `json:"parameterName,omitempty"`   // export only
	IsHeader           bool   `json:"isHeader,omitempty"`        // export only
}

// IsSet reports whether the mapping is sourced from a web service. The imported
// service is the discriminator: the other fields only qualify which part of it.
func (w WebServiceMappingSource) IsSet() bool { return w.ImportedWebService != "" }

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
	// MessageDefinition2 is a version-introduced sibling (11.10+) that mxcli
	// CARRIES rather than derives: nil means the stored document does not have
	// the key, which is not the same as present-and-empty (ako/mxcli#279).
	MessageDefinition2 *string `json:"messageDefinition2,omitempty"`
	// WebServiceSource is the imported web service (SOAP) a mapping can be
	// sourced from — a FOURTH source kind beside JSON structure, XML schema and
	// message definition. mxcli does not author one, but it must not destroy
	// one: the properties are read so a rewrite can be refused rather than
	// silently dropping the binding (ako/mxcli#365).
	WebServiceSource WebServiceMappingSource `json:"webServiceSource,omitempty"`
	// ParameterEntity is the entity of the mapping's INPUT object, stored as
	// ParameterType — a DataTypes$ObjectType naming it. Empty means the mapping
	// takes none, which Mendix stores as the DataTypes$UnknownType marker rather
	// than by omitting the property (#265).
	ParameterEntity string `json:"parameterEntity,omitempty"`
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
	// "Object", "Value", or "Array"
	Kind string `json:"kind"`
	// Object mapping fields
	Entity         string `json:"entity,omitempty"`         // qualified entity name
	ObjectHandling string `json:"objectHandling,omitempty"` // "Create", "Find", "FindOrCreate", "Custom"
	// ObjectHandlingBackup is what happens when the object is not found:
	// "Create", "Error" or "Ignore" — the only three Mendix accepts (#261).
	// Empty means the writer picks the default for the handling.
	ObjectHandlingBackup string `json:"objectHandlingBackup,omitempty"`
	// BackupAllowOverride sets ObjectHandlingBackupAllowOverride.
	BackupAllowOverride bool   `json:"backupAllowOverride,omitempty"`
	Association         string `json:"association,omitempty"` // qualified association name
	// CustomHandler is set when ObjectHandling is "Custom": a microflow
	// resolves the object instead of Create/Find (#264).
	CustomHandler *MappingMicroflowCall `json:"customHandler,omitempty"`
	// Value mapping fields
	Attribute string `json:"attribute,omitempty"` // qualified attribute name (Module.Entity.Attr)
	DataType  string `json:"dataType,omitempty"`  // "String", "Integer", "Boolean", etc.
	IsKey     bool   `json:"isKey,omitempty"`
	// Converter is a microflow the value passes through on its way to the
	// attribute (#266). The element carries only this — there is no separate
	// parameter path, because the microflow's input IS the member the element
	// already binds.
	Converter string `json:"converter,omitempty"` // qualified microflow name
	// Schema fields (cloned from JSON structure element)
	ExposedName string `json:"exposedName,omitempty"`
	JsonPath    string `json:"jsonPath,omitempty"`
	// XmlPath is set for a mapping over an XML schema or a MESSAGE DEFINITION,
	// which store BOTH path families — "Emails|Email|From" beside the JSON
	// projection "(Array)|(Object)|From" (#263). Empty for a JSON-structure
	// mapping.
	XmlPath        string `json:"xmlPath,omitempty"`
	MinOccurs      int    `json:"minOccurs,omitempty"`
	MaxOccurs      int    `json:"maxOccurs,omitempty"` // 0 = default from JSON structure
	Nillable       bool   `json:"nillable,omitempty"`
	OriginalValue  string `json:"originalValue,omitempty"`
	FractionDigits int    `json:"fractionDigits,omitempty"` // -1 = unset
	TotalDigits    int    `json:"totalDigits,omitempty"`    // -1 = unset
	MaxLength      int    `json:"maxLength,omitempty"`      // -1 = unset for non-string
	// Children
	Children []*ImportMappingElement `json:"children,omitempty"`
}

// ============================================================================
// Mapping custom object handling
// ============================================================================

// MappingMicroflowCall is the microflow that resolves a mapping element's object
// when its handling is Custom (#264) — Studio Pro's "call a microflow" option.
//
// Stored as Mappings$MappingMicroflowCallImpl on the object element's
// CustomHandlerCall property.
type MappingMicroflowCall struct {
	Microflow  string                       `json:"microflow"`
	Parameters []*MappingMicroflowParameter `json:"parameters,omitempty"`
}

// MappingMicroflowParameter binds one of the called microflow's parameters.
//
// Four sources occur across the demo apps, and the stored shape distinguishes
// them by a path marker plus LevelOfParent rather than by a kind field:
//
//	Source     stored path        LevelOfParent
//	parent     "(parent)"         -1     the enclosing mapped object
//	parameter  "(parameter)"      -1     the mapping's own input object
//	ancestor   ""                 1..N   N levels up (export mappings)
//	path       a JSON value path  -1     a value from the payload
type MappingMicroflowParameter struct {
	// Parameter is the qualified parameter reference,
	// "Module.Microflow.ParamName".
	Parameter string `json:"parameter"`
	// Source is one of "parent", "parameter", "ancestor", "path".
	Source string `json:"source"`
	// LevelOfParent is meaningful for "ancestor" only; -1 otherwise.
	LevelOfParent int `json:"levelOfParent"`
	// ValuePath is the JSON path for "path" only.
	ValuePath string `json:"valuePath,omitempty"`
	// XmlValuePath mirrors ValuePath for XML/message-definition mappings. The
	// marker sources write the SAME marker into both path properties; a value
	// path writes the JSON one and leaves the XML one empty.
	XmlValuePath string `json:"xmlValuePath,omitempty"`
}

// ============================================================================
// Message Definitions
// ============================================================================

// MessageDefinitionCollection represents a
// MessageDefinitions$MessageDefinitionCollection document — the unit. The
// individual definitions live inside it, which is why a mapping's
// MessageDefinition reference is THREE parts: Module.Collection.Definition.
type MessageDefinitionCollection struct {
	BaseElement
	ContainerID ID     `json:"containerId"`
	Name        string `json:"name"`
	// Documentation, Excluded and ExportLevel are carried so a CREATE OR MODIFY
	// preserves what the statement does not restate. ExportLevel is "Hidden" on
	// every collection measured, but it is read rather than assumed.
	Documentation string               `json:"documentation,omitempty"`
	Excluded      bool                 `json:"excluded,omitempty"`
	ExportLevel   string               `json:"exportLevel,omitempty"`
	Definitions   []*MessageDefinition `json:"definitions,omitempty"`
}

// MessageDefinition is one EntityMessageDefinition inside a collection.
type MessageDefinition struct {
	Name string `json:"name"`
	// Root is the exposed entity the definition is built on. Nil for a
	// definition kind this reader does not model.
	Root *MessageDefinitionElement `json:"root,omitempty"`
}

// MessageDefinitionElement is a node of a definition's exposed tree.
//
// Unlike a JSON structure, a message definition is derived from the DOMAIN
// MODEL: every node names an entity or an attribute. It carries two names —
// ExposedName is the node's name, ExposedItemName the singular one an unbounded
// node's items get — and a mapping over it needs both, because it stores an
// XmlPath built from them alongside the JSON projection.
type MessageDefinitionElement struct {
	// "Entity" or "Attribute".
	Kind string `json:"kind"`
	// Entity/Association are set on an Entity node; Attribute on an Attribute
	// node. Association is set only when the node is reached through one, and
	// Entity is then its TARGET — both are needed to rebuild or describe the
	// node, because the stored MaxOccurs depends on the direction of traversal
	// and cannot be recovered from the association alone.
	Entity      string `json:"entity,omitempty"`
	Association string `json:"association,omitempty"`
	Attribute   string `json:"attribute,omitempty"`

	ExposedName     string `json:"exposedName,omitempty"`
	ExposedItemName string `json:"exposedItemName,omitempty"`
	// OriginalName is the member's own name — the entity's, the attribute's, or
	// for an association node the TARGET entity's. Stored beside ExposedName
	// because the two differ routinely (52 of 56 roots, 406 of 933
	// associations) and Mendix keeps both.
	OriginalName string `json:"originalName,omitempty"`
	// Example is author-set free text. Rare — 1 of 4,707 elements across the
	// demo corpus and ako/TestApp — but hardcoding it empty would silently drop
	// the one that exists, so it is carried like any other authored value.
	Example string `json:"example,omitempty"`
	// Path is the definition's own path ("Email|From"). It is NOT the mapping's
	// XmlPath, which is built from the exposed names — the definition root's
	// path is the ITEM name while the mapping's is "Emails|Email".
	Path          string `json:"path,omitempty"`
	MinOccurs     int    `json:"minOccurs,omitempty"`
	MaxOccurs     int    `json:"maxOccurs,omitempty"`
	PrimitiveType string `json:"primitiveType,omitempty"`

	Children []*MessageDefinitionElement `json:"children,omitempty"`
}

// Unbounded reports whether the node repeats (0..*), which is what makes the
// mapping project it as an array on both path families.
func (e *MessageDefinitionElement) Unbounded() bool { return e != nil && e.MaxOccurs == -1 }

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
	// MessageDefinition2 is a version-introduced sibling (11.10+) that mxcli
	// CARRIES rather than derives: nil means the stored document does not have
	// the key, which is not the same as present-and-empty (ako/mxcli#279).
	MessageDefinition2 *string `json:"messageDefinition2,omitempty"`
	// NullValueOption controls how null values are serialized: "LeaveOutElement" or "SendAsNil"
	NullValueOption string `json:"nullValueOption,omitempty"`
	// WebServiceSource is the imported web service (SOAP) a mapping can be
	// sourced from — a FOURTH source kind beside JSON structure, XML schema and
	// message definition. mxcli does not author one, but it must not destroy
	// one: the properties are read so a rewrite can be refused rather than
	// silently dropping the binding (ako/mxcli#365).
	WebServiceSource WebServiceMappingSource `json:"webServiceSource,omitempty"`
	Elements         []*ExportMappingElement `json:"elements,omitempty"`
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
	Entity         string `json:"entity,omitempty"`         // qualified entity name
	Association    string `json:"association,omitempty"`    // qualified association name
	ObjectHandling string `json:"objectHandling,omitempty"` // "Parameter" for root, "Find" for children
	// CustomHandler — see the note on ImportMappingElement (#264).
	CustomHandler *MappingMicroflowCall `json:"customHandler,omitempty"`
	// 1 for Object, -1 for Array (unbounded). NOT "0 = default": Mendix reads
	// MaxOccurs=0 literally as "never occurs", and cross-validates this against
	// the bound JSON structure element (CE5015). Mirror the schema (#841).
	MaxOccurs int `json:"maxOccurs,omitempty"`
	// MinOccurs, MaxLength and IsKey mirror the BOUND SCHEMA ELEMENT, exactly as
	// the import twin does. The export writers used to hardcode MinOccurs 0 and
	// MaxLength 0 and omit IsKey, so no export mapping mxcli wrote matched its
	// Studio Pro original: a schema root has MinOccurs 1, and MaxLength is 0 for
	// a string element but -1 for a numeric one (ako/mxcli#277, #279).
	MinOccurs int  `json:"minOccurs,omitempty"`
	MaxLength int  `json:"maxLength,omitempty"`
	IsKey     bool `json:"isKey,omitempty"`
	// Value mapping fields
	Attribute string `json:"attribute,omitempty"` // qualified attribute name (Module.Entity.Attr)
	DataType  string `json:"dataType,omitempty"`  // "String", "Integer", "Boolean", etc.
	// Converter — see the note on ImportMappingElement (#266).
	Converter string `json:"converter,omitempty"`
	// Shared fields
	ExposedName string `json:"exposedName,omitempty"`
	JsonPath    string `json:"jsonPath,omitempty"`
	// OriginalValue is the sample parsed out of the JSON structure's snippet.
	// Carried rather than derived: whether a mapping stores it is a per-document
	// property mxcli cannot compute, so a rewrite preserves what was there
	// instead of choosing (ako/mxcli#379).
	OriginalValue string `json:"originalValue,omitempty"`
	// XmlPath — see the note on ImportMappingElement.
	XmlPath  string                  `json:"xmlPath,omitempty"`
	Children []*ExportMappingElement `json:"children,omitempty"`
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
