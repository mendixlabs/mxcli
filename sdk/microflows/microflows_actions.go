// SPDX-License-Identifier: Apache-2.0

// Package microflows - Action types for microflows
package microflows

import (
	"github.com/mendixlabs/mxcli/model"
)

// MicroflowAction is the base interface for all microflow actions.
type MicroflowAction interface {
	isMicroflowAction()
}

// Object Actions

// CreateObjectAction creates a new object.
type CreateObjectAction struct {
	model.BaseElement
	EntityID            model.ID        `json:"entityId"`
	EntityQualifiedName string          `json:"entityQualifiedName"` // BY_NAME_REFERENCE
	OutputVariable      string          `json:"outputVariable,omitempty"`
	Commit              CommitType      `json:"commit"`
	InitialMembers      []*MemberChange `json:"initialMembers,omitempty"`
}

func (CreateObjectAction) isMicroflowAction() {}

// ChangeObjectAction changes an existing object.
type ChangeObjectAction struct {
	model.BaseElement
	ChangeVariable  string          `json:"changeVariable"`
	Commit          CommitType      `json:"commit"`
	RefreshInClient bool            `json:"refreshInClient"`
	Changes         []*MemberChange `json:"changes,omitempty"`
}

func (ChangeObjectAction) isMicroflowAction() {}

// DeleteObjectAction deletes an object.
type DeleteObjectAction struct {
	model.BaseElement
	DeleteVariable  string `json:"deleteVariable"`
	RefreshInClient bool   `json:"refreshInClient"`
}

func (DeleteObjectAction) isMicroflowAction() {}

// CommitObjectsAction commits one or more objects.
type CommitObjectsAction struct {
	model.BaseElement
	ErrorHandlingType ErrorHandlingType `json:"errorHandlingType,omitempty"`
	CommitVariable    string            `json:"commitVariable"`
	WithEvents        bool              `json:"withEvents"`
	RefreshInClient   bool              `json:"refreshInClient"`
}

func (CommitObjectsAction) isMicroflowAction() {}

// RollbackObjectAction rolls back an object.
type RollbackObjectAction struct {
	model.BaseElement
	RollbackVariable string `json:"rollbackVariable"`
	RefreshInClient  bool   `json:"refreshInClient"`
}

func (RollbackObjectAction) isMicroflowAction() {}

// MemberChange represents a change to a member (attribute or association).
type MemberChange struct {
	model.BaseElement
	AttributeID              model.ID         `json:"attributeId,omitempty"`
	AttributeQualifiedName   string           `json:"attributeQualifiedName,omitempty"` // BY_NAME_REFERENCE: Module.Entity.Attribute
	AssociationID            model.ID         `json:"associationId,omitempty"`
	AssociationQualifiedName string           `json:"associationQualifiedName,omitempty"` // BY_NAME_REFERENCE: Module.Association
	Type                     MemberChangeType `json:"type"`
	Value                    string           `json:"value,omitempty"`
}

// MemberChangeType represents how a member is changed.
type MemberChangeType string

const (
	MemberChangeTypeSet    MemberChangeType = "Set"
	MemberChangeTypeAdd    MemberChangeType = "Add"
	MemberChangeTypeRemove MemberChangeType = "Remove"
)

// CommitType represents how objects are committed.
type CommitType string

const (
	CommitTypeYes           CommitType = "Yes"
	CommitTypeNo            CommitType = "No"
	CommitTypeYesWithEvents CommitType = "YesWithEvents"
	CommitTypeNoEvent       CommitType = "NoEvent"
)

// Retrieve Actions

// RetrieveAction retrieves objects from the database.
type RetrieveAction struct {
	model.BaseElement
	OutputVariable string         `json:"outputVariable"`
	Source         RetrieveSource `json:"source,omitempty"`
}

func (RetrieveAction) isMicroflowAction() {}

// RetrieveSource represents the source for a retrieve action.
type RetrieveSource interface {
	isRetrieveSource()
}

// DatabaseRetrieveSource retrieves from the database.
type DatabaseRetrieveSource struct {
	model.BaseElement
	EntityID            model.ID    `json:"entityId"`
	EntityQualifiedName string      `json:"entityQualifiedName"` // BY_NAME_REFERENCE
	XPathConstraint     string      `json:"xPathConstraint,omitempty"`
	Range               *Range      `json:"range,omitempty"`
	Sorting             []*SortItem `json:"sorting,omitempty"`
}

func (DatabaseRetrieveSource) isRetrieveSource() {}

// AssociationRetrieveSource retrieves via association.
type AssociationRetrieveSource struct {
	model.BaseElement
	StartVariable            string   `json:"startVariable"`
	AssociationID            model.ID `json:"associationId"`
	AssociationQualifiedName string   `json:"associationQualifiedName,omitempty"` // BY_NAME_REFERENCE
}

func (AssociationRetrieveSource) isRetrieveSource() {}

// Range specifies a range for retrieval.
type Range struct {
	model.BaseElement
	RangeType RangeType `json:"rangeType"`
	Limit     string    `json:"limit,omitempty"`
	Offset    string    `json:"offset,omitempty"`
}

// RangeType represents the type of range.
type RangeType string

const (
	RangeTypeAll    RangeType = "All"
	RangeTypeFirst  RangeType = "First"
	RangeTypeCustom RangeType = "Custom"
)

// SortItem represents a sort specification.
type SortItem struct {
	model.BaseElement
	AttributeID            model.ID      `json:"attributeId"`
	AttributeQualifiedName string        `json:"attributeQualifiedName,omitempty"` // BY_NAME_REFERENCE: Module.Entity.Attribute
	Direction              SortDirection `json:"direction"`
}

// SortDirection represents sort order.
type SortDirection string

const (
	SortDirectionAscending  SortDirection = "Ascending"
	SortDirectionDescending SortDirection = "Descending"
)

// AggregateListAction aggregates a list.
type AggregateListAction struct {
	model.BaseElement
	InputVariable          string            `json:"inputVariable"`
	OutputVariable         string            `json:"outputVariable"`
	Function               AggregateFunction `json:"function"`
	AttributeID            model.ID          `json:"attributeId,omitempty"`
	AttributeQualifiedName string            `json:"attributeQualifiedName,omitempty"` // BY_NAME_REFERENCE: Module.Entity.Attribute
}

func (AggregateListAction) isMicroflowAction() {}

// AggregateFunction represents an aggregate function.
type AggregateFunction string

const (
	AggregateFunctionCount   AggregateFunction = "Count"
	AggregateFunctionSum     AggregateFunction = "Sum"
	AggregateFunctionAverage AggregateFunction = "Average"
	AggregateFunctionMin     AggregateFunction = "Minimum"
	AggregateFunctionMax     AggregateFunction = "Maximum"
)

// ListOperationAction performs list operations.
type ListOperationAction struct {
	model.BaseElement
	Operation      ListOperation `json:"operation,omitempty"`
	OutputVariable string        `json:"outputVariable,omitempty"`
}

func (ListOperationAction) isMicroflowAction() {}

// ListOperation represents a list operation.
type ListOperation interface {
	isListOperation()
}

// HeadOperation gets the first element.
type HeadOperation struct {
	model.BaseElement
	ListVariable string `json:"listVariable"`
}

func (HeadOperation) isListOperation() {}

// TailOperation gets all but the first element.
type TailOperation struct {
	model.BaseElement
	ListVariable string `json:"listVariable"`
}

func (TailOperation) isListOperation() {}

// FindOperation finds an element.
type FindOperation struct {
	model.BaseElement
	ListVariable string `json:"listVariable"`
	Expression   string `json:"expression"`
}

func (FindOperation) isListOperation() {}

// FilterOperation filters a list.
type FilterOperation struct {
	model.BaseElement
	ListVariable string `json:"listVariable"`
	Expression   string `json:"expression"`
}

func (FilterOperation) isListOperation() {}

// SortOperation sorts a list.
type SortOperation struct {
	model.BaseElement
	ListVariable string      `json:"listVariable"`
	Sorting      []*SortItem `json:"sorting,omitempty"`
}

func (SortOperation) isListOperation() {}

// UnionOperation unions two lists.
type UnionOperation struct {
	model.BaseElement
	ListVariable1 string `json:"listVariable1"`
	ListVariable2 string `json:"listVariable2"`
}

func (UnionOperation) isListOperation() {}

// IntersectOperation intersects two lists.
type IntersectOperation struct {
	model.BaseElement
	ListVariable1 string `json:"listVariable1"`
	ListVariable2 string `json:"listVariable2"`
}

func (IntersectOperation) isListOperation() {}

// SubtractOperation subtracts one list from another.
type SubtractOperation struct {
	model.BaseElement
	ListVariable1 string `json:"listVariable1"`
	ListVariable2 string `json:"listVariable2"`
}

func (SubtractOperation) isListOperation() {}

// ContainsOperation checks if a list contains an object.
type ContainsOperation struct {
	model.BaseElement
	ListVariable   string `json:"listVariable"`
	ObjectVariable string `json:"objectVariable"`
}

func (ContainsOperation) isListOperation() {}

// EqualsOperation checks if two lists are equal.
type EqualsOperation struct {
	model.BaseElement
	ListVariable1 string `json:"listVariable1"`
	ListVariable2 string `json:"listVariable2"`
}

func (EqualsOperation) isListOperation() {}

// List Actions

// CreateListAction creates an empty list.
type CreateListAction struct {
	model.BaseElement
	EntityID            model.ID `json:"entityId,omitempty"`
	EntityQualifiedName string   `json:"entityQualifiedName,omitempty"`
	OutputVariable      string   `json:"outputVariable"`
}

func (CreateListAction) isMicroflowAction() {}

// ChangeListAction modifies a list (add, remove, clear, set).
type ChangeListAction struct {
	model.BaseElement
	ChangeVariable string         `json:"changeVariable"`
	Type           ChangeListType `json:"type"`
	Value          string         `json:"value,omitempty"`
}

func (ChangeListAction) isMicroflowAction() {}

// ChangeListType represents a list change operation.
type ChangeListType string

const (
	ChangeListTypeAdd    ChangeListType = "Add"
	ChangeListTypeClear  ChangeListType = "Clear"
	ChangeListTypeRemove ChangeListType = "Remove"
	ChangeListTypeSet    ChangeListType = "Set"
)

// Variable Actions

// CreateVariableAction creates a variable.
type CreateVariableAction struct {
	model.BaseElement
	VariableName string   `json:"variableName"`
	DataType     DataType `json:"dataType,omitempty"`
	InitialValue string   `json:"initialValue,omitempty"`
}

func (CreateVariableAction) isMicroflowAction() {}

// ChangeVariableAction changes a variable.
type ChangeVariableAction struct {
	model.BaseElement
	VariableName string `json:"variableName"`
	Value        string `json:"value"`
}

func (ChangeVariableAction) isMicroflowAction() {}

// CastAction casts an object to a more specific type.
type CastAction struct {
	model.BaseElement
	ObjectVariable string `json:"objectVariable"`
	OutputVariable string `json:"outputVariable"`
}

func (CastAction) isMicroflowAction() {}

// Client Actions

// ShowPageAction shows a page.
type ShowPageAction struct {
	model.BaseElement
	PageID                model.ID                `json:"pageId,omitempty"`
	PageName              string                  `json:"pageName,omitempty"` // Qualified name for BY_NAME_REFERENCE (e.g., "Module.PageName")
	FormSettingsID        model.ID                `json:"formSettingsId,omitempty"`
	PageSettings          *PageSettings           `json:"pageSettings,omitempty"`
	PassedObject          string                  `json:"passedObject,omitempty"`
	OverridePageTitle     *model.Text             `json:"overridePageTitle,omitempty"`
	PageParameterMappings []*PageParameterMapping `json:"pageParameterMappings,omitempty"`
}

func (ShowPageAction) isMicroflowAction() {}

// PageParameterMapping maps a page parameter to an argument.
type PageParameterMapping struct {
	model.BaseElement
	Parameter string `json:"parameter,omitempty"` // Parameter qualified name (Module.Page.ParameterName)
	Argument  string `json:"argument,omitempty"`  // Expression string
}

// PageSettings represents page display settings.
type PageSettings struct {
	model.BaseElement
	Location  PageLocation `json:"location"`
	ModalForm bool         `json:"modalForm"`
}

// PageLocation represents where a page is shown.
type PageLocation string

const (
	PageLocationContent PageLocation = "Content"
	PageLocationPopup   PageLocation = "Popup"
	PageLocationModal   PageLocation = "Modal"
)

// ShowHomePageAction shows the home page.
type ShowHomePageAction struct {
	model.BaseElement
}

func (ShowHomePageAction) isMicroflowAction() {}

// ClosePageAction closes the current page.
type ClosePageAction struct {
	model.BaseElement
	NumberOfPages int `json:"numberOfPages"`
}

func (ClosePageAction) isMicroflowAction() {}

// ShowMessageAction shows a message to the user.
type ShowMessageAction struct {
	model.BaseElement
	Template           *model.Text `json:"template,omitempty"`
	Type               MessageType `json:"type"`
	Blocking           bool        `json:"blocking"`
	TemplateParameters []string    `json:"templateParameters,omitempty"` // Expressions for {1}, {2}, etc.
}

func (ShowMessageAction) isMicroflowAction() {}

// MessageType represents the type of message.
type MessageType string

const (
	MessageTypeInformation MessageType = "Information"
	MessageTypeWarning     MessageType = "Warning"
	MessageTypeError       MessageType = "Error"
)

// ValidationFeedbackAction shows validation feedback.
type ValidationFeedbackAction struct {
	model.BaseElement
	ObjectVariable     string      `json:"objectVariable"`
	AttributeName      string      `json:"attributeName,omitempty"`   // BY_NAME_REFERENCE (e.g., "Module.Entity.Attribute")
	AssociationName    string      `json:"associationName,omitempty"` // BY_NAME_REFERENCE (e.g., "Module.AssociationName")
	AttributeID        model.ID    `json:"attributeId,omitempty"`     // Deprecated: use AttributeName
	AssociationID      model.ID    `json:"associationId,omitempty"`   // Deprecated: use AssociationName
	Template           *model.Text `json:"template,omitempty"`
	TemplateParameters []string    `json:"templateParameters,omitempty"` // Expressions for {1}, {2}, etc. placeholders
}

func (ValidationFeedbackAction) isMicroflowAction() {}

// DownloadFileAction downloads a file.
type DownloadFileAction struct {
	model.BaseElement
	FileDocument  string `json:"fileDocument"`
	ShowInBrowser bool   `json:"showInBrowser"`
}

func (DownloadFileAction) isMicroflowAction() {}

// Integration Actions

// MicroflowCallAction calls another microflow.
type MicroflowCallAction struct {
	model.BaseElement
	ErrorHandlingType  ErrorHandlingType `json:"errorHandlingType,omitempty"`
	MicroflowCall      *MicroflowCall    `json:"microflowCall,omitempty"`
	ResultVariableName string            `json:"resultVariableName,omitempty"`
	UseReturnVariable  bool              `json:"useReturnVariable"`
}

func (MicroflowCallAction) isMicroflowAction() {}

// MicroflowCall represents a call to a microflow with its parameters.
type MicroflowCall struct {
	model.BaseElement
	Microflow         string                           `json:"microflow,omitempty"` // Qualified name string
	ParameterMappings []*MicroflowCallParameterMapping `json:"parameterMappings,omitempty"`
}

// MicroflowCallParameterMapping maps a parameter to an argument.
type MicroflowCallParameterMapping struct {
	model.BaseElement
	Parameter string `json:"parameter,omitempty"` // Parameter qualified name
	Argument  string `json:"argument,omitempty"`
}

// JavaActionCallAction calls a Java action.
type JavaActionCallAction struct {
	model.BaseElement
	ErrorHandlingType  ErrorHandlingType             `json:"errorHandlingType,omitempty"`
	JavaAction         string                        `json:"javaAction,omitempty"` // Qualified name string
	ParameterMappings  []*JavaActionParameterMapping `json:"parameterMappings,omitempty"`
	ResultVariableName string                        `json:"resultVariableName,omitempty"`
	UseReturnVariable  bool                          `json:"useReturnVariable"`
}

func (JavaActionCallAction) isMicroflowAction() {}

// JavaActionParameterMapping maps a Java action parameter.
type JavaActionParameterMapping struct {
	model.BaseElement
	Parameter string                   `json:"parameter,omitempty"` // Parameter qualified name
	Value     CodeActionParameterValue `json:"value,omitempty"`
}

// CodeActionParameterValue is the base interface for parameter values.
type CodeActionParameterValue interface {
	isCodeActionParameterValue()
}

// StringTemplateParameterValue is a string template parameter value.
type StringTemplateParameterValue struct {
	model.BaseElement
	TypedTemplate *TypedTemplate `json:"typedTemplate,omitempty"`
}

func (StringTemplateParameterValue) isCodeActionParameterValue() {}

// TypedTemplate represents a typed template with arguments.
type TypedTemplate struct {
	model.BaseElement
	Arguments []string `json:"arguments,omitempty"`
	Text      string   `json:"text,omitempty"`
}

// ExpressionBasedCodeActionParameterValue is an expression-based parameter value.
type ExpressionBasedCodeActionParameterValue struct {
	model.BaseElement
	Expression string `json:"expression,omitempty"`
}

func (ExpressionBasedCodeActionParameterValue) isCodeActionParameterValue() {}

// BasicCodeActionParameterValue is a basic parameter value with an argument expression.
type BasicCodeActionParameterValue struct {
	model.BaseElement
	Argument string `json:"argument,omitempty"`
}

func (BasicCodeActionParameterValue) isCodeActionParameterValue() {}

// EntityTypeCodeActionParameterValue is an entity type passed at a call site for a type parameter.
type EntityTypeCodeActionParameterValue struct {
	model.BaseElement
	Entity string `json:"entity,omitempty"` // BY_NAME_REFERENCE: qualified entity name
}

func (EntityTypeCodeActionParameterValue) isCodeActionParameterValue() {}

// CallExternalAction calls an external action on a consumed OData service.
type CallExternalAction struct {
	model.BaseElement
	ErrorHandlingType    ErrorHandlingType                 `json:"errorHandlingType,omitempty"`
	ConsumedODataService string                            `json:"consumedODataService,omitempty"` // BY_NAME reference
	Name                 string                            `json:"name,omitempty"`                 // External action name
	ParameterMappings    []*ExternalActionParameterMapping `json:"parameterMappings,omitempty"`
	ResultVariableName   string                            `json:"resultVariableName,omitempty"`
	UseReturnVariable    bool                              `json:"useReturnVariable"`
}

func (CallExternalAction) isMicroflowAction() {}

// ExternalActionParameterMapping maps a parameter for an external action call.
type ExternalActionParameterMapping struct {
	model.BaseElement
	ParameterName string `json:"parameterName,omitempty"`
	Argument      string `json:"argument,omitempty"` // Expression
	CanBeEmpty    bool   `json:"canBeEmpty,omitempty"`
}

// WebServiceCallAction calls a web service.
type WebServiceCallAction struct {
	model.BaseElement
	ServiceID         model.ID `json:"serviceId,omitempty"`
	OperationName     string   `json:"operationName,omitempty"`
	SendMappingID     model.ID `json:"sendMappingId,omitempty"`
	ReceiveMappingID  model.ID `json:"receiveMappingId,omitempty"`
	OutputVariable    string   `json:"outputVariable,omitempty"`
	UseReturnVariable bool     `json:"useReturnVariable"`
	TimeoutExpression string   `json:"timeoutExpression,omitempty"`
}

func (WebServiceCallAction) isMicroflowAction() {}

// RestCallAction calls a REST service.
type RestCallAction struct {
	model.BaseElement
	HttpConfiguration *HttpConfiguration `json:"httpConfiguration,omitempty"`
	RequestHandling   RequestHandling    `json:"requestHandling,omitempty"`
	ResultHandling    ResultHandling     `json:"resultHandling,omitempty"`
	ErrorHandling     *ErrorHandling     `json:"errorHandling,omitempty"`
	ErrorHandlingType ErrorHandlingType  `json:"errorHandlingType,omitempty"`
	OutputVariable    string             `json:"outputVariable,omitempty"`
	UseReturnVariable bool               `json:"useReturnVariable"`
	TimeoutExpression string             `json:"timeoutExpression,omitempty"`
}

func (RestCallAction) isMicroflowAction() {}

// RestOperationCallAction calls a consumed REST service operation.
// BSON type: Microflows$RestOperationCallAction
type RestOperationCallAction struct {
	model.BaseElement
	ErrorHandlingType ErrorHandlingType `json:"errorHandlingType,omitempty"`
	Operation         string            `json:"operation,omitempty"`      // BY_NAME: Module.Service.Operation
	OutputVariable    *RestOutputVar    `json:"outputVariable,omitempty"` // null or Microflows$OutputVariable
	BodyVariable      *RestBodyVar      `json:"bodyVariable,omitempty"`   // null or nested object
}

func (RestOperationCallAction) isMicroflowAction() {}

// RestOutputVar represents Microflows$OutputVariable in a REST operation call.
type RestOutputVar struct {
	model.BaseElement
	VariableName string `json:"variableName,omitempty"`
}

// RestBodyVar represents the body variable in a REST operation call.
type RestBodyVar struct {
	model.BaseElement
	VariableName string `json:"variableName,omitempty"`
}

// HttpConfiguration represents HTTP configuration for a REST call.
type HttpConfiguration struct {
	model.BaseElement
	HttpMethod        HttpMethod    `json:"httpMethod"`
	LocationTemplate  string        `json:"locationTemplate,omitempty"`
	LocationParams    []string      `json:"locationParams,omitempty"` // Expression parameters for URL template
	CustomLocation    string        `json:"customLocation,omitempty"`
	CustomHeaders     []*HttpHeader `json:"customHeaders,omitempty"`
	UseAuthentication bool          `json:"useAuthentication,omitempty"`
	Username          string        `json:"username,omitempty"`
	Password          string        `json:"password,omitempty"`
}

// HttpMethod represents an HTTP method.
type HttpMethod string

const (
	HttpMethodGet    HttpMethod = "Get"
	HttpMethodPost   HttpMethod = "Post"
	HttpMethodPut    HttpMethod = "Put"
	HttpMethodPatch  HttpMethod = "Patch"
	HttpMethodDelete HttpMethod = "Delete"
)

// HttpHeader represents an HTTP header.
type HttpHeader struct {
	model.BaseElement
	Name  string `json:"name"`
	Value string `json:"value"`
}

// RequestHandling represents how a request is handled.
type RequestHandling interface {
	isRequestHandling()
}

// SimpleRequestHandling represents simple request handling.
type SimpleRequestHandling struct {
	model.BaseElement
	ParameterEntityID model.ID `json:"parameterEntityId,omitempty"`
}

func (SimpleRequestHandling) isRequestHandling() {}

// MappingRequestHandling uses an export mapping.
type MappingRequestHandling struct {
	model.BaseElement
	MappingID         model.ID `json:"mappingId"`
	ContentType       string   `json:"contentType,omitempty"`
	ParameterVariable string   `json:"parameterVariable,omitempty"`
}

func (MappingRequestHandling) isRequestHandling() {}

// CustomRequestHandling uses a custom body.
type CustomRequestHandling struct {
	model.BaseElement
	Template       string   `json:"template,omitempty"`
	TemplateParams []string `json:"templateParams,omitempty"`
}

func (CustomRequestHandling) isRequestHandling() {}

// BinaryRequestHandling uses a binary expression.
type BinaryRequestHandling struct {
	model.BaseElement
	Expression string `json:"expression,omitempty"`
}

func (BinaryRequestHandling) isRequestHandling() {}

// FormDataRequestHandling uses form data.
type FormDataRequestHandling struct {
	model.BaseElement
}

func (FormDataRequestHandling) isRequestHandling() {}

// ResultHandling represents how a result is handled.
type ResultHandling interface {
	isResultHandling()
}

// ResultHandlingNone represents no result handling.
type ResultHandlingNone struct {
	model.BaseElement
}

func (ResultHandlingNone) isResultHandling() {}

// ResultHandlingString returns the result as a string.
type ResultHandlingString struct {
	model.BaseElement
	VariableName string `json:"variableName,omitempty"`
}

func (ResultHandlingString) isResultHandling() {}

// ResultHandlingHttpResponse returns the result as an HttpResponse object.
type ResultHandlingHttpResponse struct {
	model.BaseElement
	VariableName string `json:"variableName,omitempty"`
}

func (ResultHandlingHttpResponse) isResultHandling() {}

// ResultHandlingMapping uses an import mapping.
type ResultHandlingMapping struct {
	model.BaseElement
	MappingID      model.ID `json:"mappingId"`
	ResultEntityID model.ID `json:"resultEntityId,omitempty"`
	ResultVariable string   `json:"resultVariable,omitempty"`
	SingleObject   bool     `json:"singleObject,omitempty"` // true when mapping returns a single object (not a list)
}

func (ResultHandlingMapping) isResultHandling() {}

// ErrorHandling represents error handling configuration.
type ErrorHandling struct {
	model.BaseElement
	ErrorHandlingType ErrorHandlingType `json:"errorHandlingType"`
}

// LogMessageAction logs a message.
type LogMessageAction struct {
	model.BaseElement
	LogLevel              LogLevel    `json:"logLevel"`
	LogNodeName           string      `json:"logNodeName,omitempty"`
	MessageTemplate       *model.Text `json:"messageTemplate,omitempty"`
	TemplateParameters    []string    `json:"templateParameters,omitempty"` // Expressions for {1}, {2}, etc. placeholders
	IncludeLastStackTrace bool        `json:"includeLastStackTrace"`
}

func (LogMessageAction) isMicroflowAction() {}

// LogLevel represents a log level.
type LogLevel string

const (
	LogLevelTrace    LogLevel = "Trace"
	LogLevelDebug    LogLevel = "Debug"
	LogLevelInfo     LogLevel = "Info"
	LogLevelWarning  LogLevel = "Warning"
	LogLevelError    LogLevel = "Error"
	LogLevelCritical LogLevel = "Critical"
)

// ImportMappingCallAction calls an import mapping.
type ImportMappingCallAction struct {
	model.BaseElement
	MappingID      model.ID `json:"mappingId"`
	SourceVariable string   `json:"sourceVariable,omitempty"`
	OutputVariable string   `json:"outputVariable,omitempty"`
	ContentType    string   `json:"contentType,omitempty"`
}

func (ImportMappingCallAction) isMicroflowAction() {}

// ExportMappingCallAction calls an export mapping.
type ExportMappingCallAction struct {
	model.BaseElement
	MappingID      model.ID `json:"mappingId"`
	SourceVariable string   `json:"sourceVariable,omitempty"`
	OutputVariable string   `json:"outputVariable,omitempty"`
	ContentType    string   `json:"contentType,omitempty"`
}

func (ExportMappingCallAction) isMicroflowAction() {}

// ExecuteDatabaseQueryAction represents a DatabaseConnector$ExecuteDatabaseQueryAction.
// Used for executing queries against external databases via the Database Connector.
type ExecuteDatabaseQueryAction struct {
	model.BaseElement
	ErrorHandlingType           ErrorHandlingType                     `json:"errorHandlingType,omitempty"`
	OutputVariableName          string                                `json:"outputVariableName,omitempty"`
	Query                       string                                `json:"query,omitempty"`        // BY_NAME ref: "Module.Connection.QueryName"
	DynamicQuery                string                                `json:"dynamicQuery,omitempty"` // Raw SQL expression (overrides Query)
	ParameterMappings           []*DatabaseQueryParameterMapping      `json:"parameterMappings,omitempty"`
	ConnectionParameterMappings []*DatabaseConnectionParameterMapping `json:"connectionParameterMappings,omitempty"`
}

func (ExecuteDatabaseQueryAction) isMicroflowAction() {}

// DatabaseQueryParameterMapping maps a query parameter to a microflow expression.
type DatabaseQueryParameterMapping struct {
	model.BaseElement
	ParameterName string `json:"parameterName"`
	Value         string `json:"value"` // Microflow expression
}

// DatabaseConnectionParameterMapping maps a connection parameter to a value.
type DatabaseConnectionParameterMapping struct {
	model.BaseElement
	ParameterName string `json:"parameterName"`
	Value         string `json:"value"`
}

// UnknownAction represents an action type that is not yet implemented.
// It stores the type name for debugging purposes.
type UnknownAction struct {
	model.BaseElement
	TypeName string `json:"typeName,omitempty"`
}

func (UnknownAction) isMicroflowAction() {}
