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
package agenteditor

import "github.com/JordtenBulte-OLC/mxcli/model"

// CustomDocumentType values observed in test3 project.
const (
	CustomTypeAgent              = "agenteditor.agent"
	CustomTypeModel              = "agenteditor.model"
	CustomTypeKnowledgeBase      = "agenteditor.knowledgebase"
	CustomTypeConsumedMCPService = "agenteditor.consumedMCPService"
)

// ReadableTypeName values (BSON wrapper Metadata.ReadableTypeName).
const (
	ReadableAgent              = "Agent"
	ReadableModel              = "Model"
	ReadableKnowledgeBase      = "Knowledge base"
	ReadableConsumedMCPService = "Consumed MCP service"
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
}

// GetName returns the MCP service's name.
func (c *ConsumedMCPService) GetName() string { return c.Name }

// GetContainerID returns the container ID.
func (c *ConsumedMCPService) GetContainerID() model.ID { return c.ContainerID }

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
}

// GetName returns the agent's name.
func (a *Agent) GetName() string { return a.Name }

// GetContainerID returns the container ID.
func (a *Agent) GetContainerID() model.ID { return a.ContainerID }

// AgentVar is an entry in the agent's `variables` JSON array.
type AgentVar struct {
	Key                 string `json:"key"`
	IsAttributeInEntity bool   `json:"isAttributeInEntity,omitempty"`
}

// AgentTool is an entry in the agent's `tools` JSON array.
type AgentTool struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name,omitempty"`
	Description string  `json:"description,omitempty"`
	Enabled     bool    `json:"enabled,omitempty"`
	ToolType    string  `json:"toolType,omitempty"` // "MCP" | "Microflow" | ""
	Document    *DocRef `json:"document,omitempty"`
	// Microflow-tool-specific fields are not yet observed in the wild.
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
}
