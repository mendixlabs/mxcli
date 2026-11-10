// SPDX-License-Identifier: Apache-2.0

// Package agenteditor defines types for documents created by the Mendix
// Studio Pro Agent Editor extension.
//
// All four document types (Agent, Model, Knowledge Base, Consumed MCP
// Service) share the same outer BSON wrapper — a generic
// CustomBlobDocuments$CustomBlobDocument — and are distinguished by the
// wrapper's CustomDocumentType field. The inner Contents field holds a
// JSON payload whose schema depends on the document type.
//
// See docs/11-proposals/PROPOSAL_agent_document_support.md for the
// observed BSON and JSON schemas.
package types

import "github.com/mendixlabs/mxcli/model"

// CustomDocumentType values observed in test3 project.
const (
	CustomTypeAgent              = "agenteditor.agent"
	CustomTypeModel              = "agenteditor.model"
	CustomTypeKnowledgeBase      = "agenteditor.knowledgebase"
	CustomTypeConsumedMCPService = "agenteditor.consumedMCPService"
	// Agent Editor 2.6+ with MCP Server 5.x (ako/mxcli#2020).
	CustomTypePublishedMCPService = "agenteditor.publishedMCPService"
)

// ReadableTypeName values (BSON wrapper Metadata.ReadableTypeName).
const (
	ReadableAgent               = "Agent"
	ReadableModel               = "Model"
	ReadableKnowledgeBase       = "Knowledge base"
	ReadableConsumedMCPService  = "Consumed MCP service"
	ReadablePublishedMCPService = "Published MCP service"
)

// CreatedByExtensionID is the value used for the Metadata.CreatedByExtension
// field on all agent-editor documents.
const CreatedByExtensionID = "extension/agent-editor"

// DocRef references another CustomBlobDocument by its document ID and
// qualified name. Used throughout agent-editor JSON schemas for inter-
// document references.
type DocRef struct {
	DocumentID    string `json:"documentId"`
	QualifiedName string `json:"qualifiedName"`
}

// ConstantRef references a String constant by its document ID and
// qualified name. Used by Model and KnowledgeBase documents to point
// at the constant holding the Mendix Cloud GenAI Portal key.
type ConstantRef struct {
	DocumentID    string `json:"documentId"`
	QualifiedName string `json:"qualifiedName"`
}

// Model represents an agent-editor Model document
// (CustomDocumentType = "agenteditor.model").
//
// Contents JSON schema:
//
//	{
//	  "type": "",
//	  "name": "",
//	  "displayName": "",
//	  "provider": "MxCloudGenAI",
//	  "providerFields": {
//	    "environment": "",
//	    "deepLinkURL": "",
//	    "keyId": "",
//	    "keyName": "",
//	    "resourceName": "",
//	    "key": { "documentId": "...", "qualifiedName": "Module.Const" }
//	  }
//	}
type Model struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
	Excluded      bool     `json:"excluded,omitempty"`
	ExportLevel   string   `json:"exportLevel,omitempty"`

	// Portal-populated fields — usually empty on freshly-created documents.
	// They are filled by Studio Pro after the user clicks "Test Key".
	Type        string `json:"type,omitempty"`
	InnerName   string `json:"innerName,omitempty"` // Contents.name field
	DisplayName string `json:"displayName,omitempty"`

	// User-set: provider discriminator. Only observed value: "MxCloudGenAI".
	Provider string `json:"provider,omitempty"`

	// Portal-populated providerFields (subset that varies by provider).
	Environment  string `json:"environment,omitempty"`
	DeepLinkURL  string `json:"deepLinkURL,omitempty"`
	KeyID        string `json:"keyId,omitempty"`
	KeyName      string `json:"keyName,omitempty"`
	ResourceName string `json:"resourceName,omitempty"`

	// User-set: reference to the String constant that holds the Portal key.
	Key *ConstantRef `json:"key,omitempty"`
}

// GetName returns the model's name.
func (m *Model) GetName() string { return m.Name }

// GetContainerID returns the container ID (the module this model lives in).
func (m *Model) GetContainerID() model.ID { return m.ContainerID }

// KnowledgeBase represents an agent-editor Knowledge Base document
// (CustomDocumentType = "agenteditor.knowledgebase"). Scaffolded for future
// implementation — parsing not yet wired.
type KnowledgeBase struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
	Excluded      bool     `json:"excluded,omitempty"`
	ExportLevel   string   `json:"exportLevel,omitempty"`

	InnerName        string       `json:"innerName,omitempty"` // Contents.name: the Portal knowledge base name
	Provider         string       `json:"provider,omitempty"`
	Environment      string       `json:"environment,omitempty"`
	DeepLinkURL      string       `json:"deepLinkURL,omitempty"`
	KeyID            string       `json:"keyId,omitempty"`
	KeyName          string       `json:"keyName,omitempty"`
	ModelDisplayName string       `json:"modelDisplayName,omitempty"`
	ModelName        string       `json:"modelName,omitempty"`
	Key              *ConstantRef `json:"key,omitempty"`
}

// GetName returns the knowledge base's name.
func (k *KnowledgeBase) GetName() string { return k.Name }

// GetContainerID returns the container ID.
func (k *KnowledgeBase) GetContainerID() model.ID { return k.ContainerID }

// ConsumedMCPService represents an agent-editor Consumed MCP Service document
// (CustomDocumentType = "agenteditor.consumedMCPService"). Scaffolded for
// future implementation — parsing not yet wired.
type ConsumedMCPService struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
	Excluded      bool     `json:"excluded,omitempty"`
	ExportLevel   string   `json:"exportLevel,omitempty"`

	ProtocolVersion          string `json:"protocolVersion,omitempty"`
	Version                  string `json:"version,omitempty"`
	InnerDocumentation       string `json:"innerDocumentation,omitempty"` // Contents.documentation
	ConnectionTimeoutSeconds int    `json:"connectionTimeoutSeconds,omitempty"`
	// Endpoint is the String constant holding the server URL; without one
	// AgentEditorCommons does not import the service (#2060).
	Endpoint *DocRef `json:"endpoint,omitempty"`
	// AuthenticationMicroflow returns the HTTP headers sent to the server
	// (no parameters, List of System.HttpHeader).
	AuthenticationMicroflow *DocRef `json:"authenticationMicroflow,omitempty"`
}

// GetName returns the MCP service's name.
func (c *ConsumedMCPService) GetName() string { return c.Name }

// GetContainerID returns the container ID.
func (c *ConsumedMCPService) GetContainerID() model.ID { return c.ContainerID }

// PublishedMCPService represents an agent-editor Published MCP Service document
// (CustomDocumentType = "agenteditor.publishedMCPService", Agent Editor 2.6+
// with MCP Server 5.x). At startup AgentEditorCommons reads it and calls
// MCPServer.CreateMCPServer and MCPServer.AddTool, so each field is one of
// those actions' parameters (ako/mxcli#2020).
//
// Contents JSON schema (Studio Pro 11.16, Agent Editor 2.6.0):
//
//	{
//	  "serverName": "...", "endpointPath": "...", "version": "1.0.0",
//	  "protocolVersion": "v2025_03_26", "authenticationEnabled": false,
//	  "tools": [{ "uuid", "name", "description",
//	              "sourceMicroflow": { "qualifiedName", "documentId" }, "title" }],
//	  "documentation": "..."
//	}
//
// With authentication on, Studio Pro appends authenticationMicroflow
// { "documentId", "qualifiedName" } after documentation. The runtime reader
// also takes authorizationServerUrl, scopes and a tool's schema; no saved
// document holds those yet. ServerName is stored but not shown in the editor:
// a duplicated document keeps the original's.
type PublishedMCPService struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
	Excluded      bool     `json:"excluded,omitempty"`
	ExportLevel   string   `json:"exportLevel,omitempty"`

	ServerName              string             `json:"serverName,omitempty"`
	EndpointPath            string             `json:"endpointPath,omitempty"`
	Version                 string             `json:"version,omitempty"`
	ProtocolVersion         string             `json:"protocolVersion,omitempty"`
	AuthenticationEnabled   bool               `json:"authenticationEnabled,omitempty"`
	AuthenticationMicroflow *DocRef            `json:"authenticationMicroflow,omitempty"`
	AuthorizationServerURL  string             `json:"authorizationServerUrl,omitempty"`
	Scopes                  string             `json:"scopes,omitempty"`
	Tools                   []PublishedMCPTool `json:"tools,omitempty"`
	InnerDocumentation      string             `json:"innerDocumentation,omitempty"` // Contents.documentation
}

// GetName returns the published MCP service's name.
func (p *PublishedMCPService) GetName() string { return p.Name }

// GetContainerID returns the container ID.
func (p *PublishedMCPService) GetContainerID() model.ID { return p.ContainerID }

// PublishedMCPTool is an entry in a published MCP service's `tools` array: a
// microflow exposed as an MCP tool.
type PublishedMCPTool struct {
	UUID            string  `json:"uuid,omitempty"`
	Name            string  `json:"name,omitempty"`
	Description     string  `json:"description,omitempty"`
	SourceMicroflow *DocRef `json:"sourceMicroflow,omitempty"`
	Title           string  `json:"title,omitempty"`
	Schema          string  `json:"schema,omitempty"`
}

// Agent represents an agent-editor Agent document
// (CustomDocumentType = "agenteditor.agent"). Scaffolded for future
// implementation — parsing not yet wired.
type Agent struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
	Excluded      bool     `json:"excluded,omitempty"`
	ExportLevel   string   `json:"exportLevel,omitempty"`

	Description  string        `json:"description,omitempty"`
	SystemPrompt string        `json:"systemPrompt,omitempty"`
	UserPrompt   string        `json:"userPrompt,omitempty"`
	UsageType    string        `json:"usageType,omitempty"`
	Variables    []AgentVar    `json:"variables,omitempty"`
	Tools        []AgentTool   `json:"tools,omitempty"`
	KBTools      []AgentKBTool `json:"knowledgebaseTools,omitempty"`
	Model        *DocRef       `json:"model,omitempty"`
	Entity       *DocRef       `json:"entity,omitempty"`
	MaxTokens    *int          `json:"maxTokens,omitempty"`
	ToolChoice   string        `json:"toolChoice,omitempty"`
	Temperature  *float64      `json:"temperature,omitempty"`
	TopP         *float64      `json:"topP,omitempty"`
	// Response streaming (Agent Editor 2.2.1+, #2022), stored on the agent,
	// not the model. nil when the document holds none.
	EnableBackendStreaming *bool `json:"enableBackendStreaming,omitempty"`
	EnableUIStreaming      *bool `json:"enableUIStreaming,omitempty"`
	// SelectedModel is the model the agent uses on a multi-model resource: its
	// id, which the import matches against DeployedModel.Model, and the display
	// name Studio Pro shows. Agent Editor 2.2+ refuses to import an agent
	// without one (#2031). nil when the document holds none.
	SelectedModel *AgentSelectedModel `json:"selectedModel,omitempty"`
}

// AgentSelectedModel is the agent's `selectedModel` JSON object.
type AgentSelectedModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetName returns the agent's name.
func (a *Agent) GetName() string { return a.Name }

// GetContainerID returns the container ID.
func (a *Agent) GetContainerID() model.ID { return a.ContainerID }

// AgentVar is an entry in the agent's `variables` JSON array.
type AgentVar struct {
	Key                 string `json:"key"`
	IsAttributeInEntity bool   `json:"isAttributeInEntity"` // Agent Editor writes false too
}

// AgentTool is an entry in the agent's `tools` JSON array.
type AgentTool struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name,omitempty"`
	Description string  `json:"description,omitempty"`
	Enabled     bool    `json:"enabled,omitempty"`
	ToolType    string  `json:"toolType,omitempty"` // "MCP" | "Microflow" | ""
	Document    *DocRef `json:"document,omitempty"`
	// Agent Editor 2.2+ (#2021): whether the user sees the tool call, and
	// whether it waits for the user's approval — HiddenForUser,
	// VisibleForUser or UserConfirmationRequired (GenAICommons.
	// ENUM_UserAccessApproval). "" when the document holds none.
	UserAccessApproval string `json:"userAccessApproval,omitempty"`
	// The title and description shown to the user for the call; the model
	// sees Name and Description. nil when the document holds none.
	DisplayTitle       *string `json:"displayTitle,omitempty"`
	DisplayDescription *string `json:"displayDescription,omitempty"`
}

// AgentKBTool is an entry in the agent's `knowledgebaseTools` JSON array.
type AgentKBTool struct {
	ID                   string  `json:"id,omitempty"`
	Name                 string  `json:"name,omitempty"`
	Description          string  `json:"description,omitempty"`
	Enabled              bool    `json:"enabled,omitempty"`
	ToolType             string  `json:"toolType,omitempty"`
	Document             *DocRef `json:"document,omitempty"`
	CollectionIdentifier string  `json:"collectionIdentifier,omitempty"`
	MaxResults           int     `json:"maxResults,omitempty"`
	// The least similarity a chunk needs to be returned, and whether the
	// user sees the retrieval. nil when the document holds none.
	MinSimilarity *float64 `json:"minSimilarity,omitempty"`
	IsVisible     *bool    `json:"isVisible,omitempty"`
}
