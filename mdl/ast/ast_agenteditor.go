// SPDX-License-Identifier: Apache-2.0

package ast

// CreateModelStmt represents:
//
//	CREATE MODEL Module.Name (
//	  Provider: MxCloudGenAI,
//	  Key: @Module.SomeConstant
//	  -- optional Portal-populated fields:
//	  [, DisplayName: '...']
//	  [, KeyName: '...']
//	  [, KeyId: '...']
//	  [, Environment: '...']
//	  [, ResourceName: '...']
//	  [, DeepLinkURL: '...']
//	);
type CreateModelStmt struct {
	CreateGuard             // `create … if not exists` (ako/mxcli#731)
	Folder           string // Folder path within module (empty = leave placement alone)
	Name             QualifiedName
	Documentation    string
	DocumentationSet bool           // see mendixlabs/mxcli#1018: absent preserves, empty clears
	Provider         string         // "MxCloudGenAI" by default
	Key              *QualifiedName // qualified name of the String constant holding the Portal key
	DisplayName      string         // optional Portal-populated metadata
	KeyName          string         // optional Portal-populated metadata
	KeyID            string         // optional Portal-populated metadata
	Environment      string         // optional Portal-populated metadata
	ResourceName     string         // optional Portal-populated metadata
	DeepLinkURL      string         // optional Portal-populated metadata
	CreateOrModify   bool           // true for CREATE OR MODIFY / CREATE OR REPLACE
}

func (s *CreateModelStmt) isStatement() {}

// DropModelStmt represents: DROP MODEL Module.Name
type DropModelStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropModelStmt) isStatement() {}

// AlterModelStmt represents: ALTER MODEL Module.Name SET key = value (, key = value)*
type AlterModelStmt struct {
	Name    QualifiedName
	Changes map[string]string
}

func (s *AlterModelStmt) isStatement() {}

// CreateConsumedMCPServiceStmt represents:
//
//	CREATE CONSUMED MCP SERVICE Module.Name (
//	  ProtocolVersion: v2025_03_26,
//	  Version: '0.0.1',
//	  ConnectionTimeoutSeconds: 30,
//	  Documentation: '...'
//	);
type CreateConsumedMCPServiceStmt struct {
	CreateGuard                     // `create … if not exists` (ako/mxcli#731)
	Folder                   string // Folder path within module (empty = leave placement alone)
	Name                     QualifiedName
	OuterDocumentation       string // /** ... */ doc comment
	DocumentationSet         bool   // see mendixlabs/mxcli#1018: absent preserves, empty clears
	ProtocolVersion          string
	Version                  string
	ConnectionTimeoutSeconds int
	InnerDocumentation       string // Contents.documentation field
	CreateOrModify           bool   // true for CREATE OR MODIFY / CREATE OR REPLACE
	// Endpoint is the String constant holding the server URL (#2060);
	// AuthenticationMicroflow returns the headers sent with each request. Left
	// out, a create or modify keeps the stored reference.
	Endpoint                *QualifiedName
	AuthenticationMicroflow *QualifiedName
}

func (s *CreateConsumedMCPServiceStmt) isStatement() {}

// DropConsumedMCPServiceStmt represents: DROP CONSUMED MCP SERVICE Module.Name
type DropConsumedMCPServiceStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropConsumedMCPServiceStmt) isStatement() {}

// AlterConsumedMCPServiceStmt represents:
// ALTER CONSUMED MCP SERVICE Module.Name SET key = value (, key = value)*
type AlterConsumedMCPServiceStmt struct {
	Name    QualifiedName
	Changes map[string]string
}

func (s *AlterConsumedMCPServiceStmt) isStatement() {}

// CreatePublishedMCPServiceStmt represents (ako/mxcli#2020):
//
//	CREATE PUBLISHED MCP SERVICE Module.Name (
//	  [ServerName: 'Orders',] EndpointPath: 'orders', Version: '1.0.0',
//	  ProtocolVersion: v2025_03_26,
//	  [AuthenticationEnabled: true, AuthenticationMicroflow: Module.Auth,]
//	  [AuthorizationServerUrl: '...', Scopes: '...',]
//	  Documentation: '...'
//	) { tool name ( Microflow: Module.Tool, Description: '...', Title: '...', Schema: '...' ) };
type CreatePublishedMCPServiceStmt struct {
	CreateGuard
	Folder                  string
	Name                    QualifiedName
	OuterDocumentation      string // /** ... */ doc comment
	DocumentationSet        bool
	ServerName              string // empty: the stored value, or the document name on create
	EndpointPath            string
	Version                 string
	ProtocolVersion         string
	AuthenticationEnabled   bool
	AuthenticationMicroflow *QualifiedName
	AuthorizationServerURL  string
	Scopes                  string
	InnerDocumentation      string // Contents.documentation field
	Tools                   []PublishedMCPToolDef
	CreateOrModify          bool
}

func (s *CreatePublishedMCPServiceStmt) isStatement() {}

// PublishedMCPToolDef is a `tool name ( … )` block of a published MCP service.
type PublishedMCPToolDef struct {
	Name        string
	Microflow   *QualifiedName
	Description string
	Title       string
	Schema      string
}

// DropPublishedMCPServiceStmt represents: DROP PUBLISHED MCP SERVICE Module.Name
type DropPublishedMCPServiceStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropPublishedMCPServiceStmt) isStatement() {}

// AlterPublishedMCPServiceStmt represents:
//
//	ALTER PUBLISHED MCP SERVICE Module.Name
//	    SET key = value (, key = value)* | ADD TOOL name ( … ) | DROP TOOL name
//
// The actions apply in order.
type AlterPublishedMCPServiceStmt struct {
	Name      QualifiedName
	Sets      map[string]string
	AddTools  []PublishedMCPToolDef
	DropTools []string
}

func (s *AlterPublishedMCPServiceStmt) isStatement() {}

// CreateKnowledgeBaseStmt represents:
//
//	CREATE KNOWLEDGE BASE Module.Name (
//	  Provider: MxCloudGenAI,
//	  Key: @Module.SomeConstant
//	);
type CreateKnowledgeBaseStmt struct {
	CreateGuard             // `create … if not exists` (ako/mxcli#731)
	Folder           string // Folder path within module (empty = leave placement alone)
	Name             QualifiedName
	Documentation    string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
	Provider         string
	Key              *QualifiedName
	ModelDisplayName string
	ModelName        string
	KeyName          string
	KeyID            string
	Environment      string
	DeepLinkURL      string
	CreateOrModify   bool // true for CREATE OR MODIFY / CREATE OR REPLACE
}

func (s *CreateKnowledgeBaseStmt) isStatement() {}

// DropKnowledgeBaseStmt represents: DROP KNOWLEDGE BASE Module.Name
type DropKnowledgeBaseStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropKnowledgeBaseStmt) isStatement() {}

// AlterKnowledgeBaseStmt represents:
// ALTER KNOWLEDGE BASE Module.Name SET key = value (, key = value)*
type AlterKnowledgeBaseStmt struct {
	Name    QualifiedName
	Changes map[string]string
}

func (s *AlterKnowledgeBaseStmt) isStatement() {}

// CreateAgentStmt represents CREATE AGENT Module.Name (...) [{ body }].
type CreateAgentStmt struct {
	CreateGuard             // `create … if not exists` (ako/mxcli#731)
	Folder           string // Folder path within module (empty = leave placement alone)
	Name             QualifiedName
	Documentation    string
	DocumentationSet bool   // see mendixlabs/mxcli#1018: absent preserves, empty clears
	UsageType        string // "Task" or "Conversational"
	Description      string
	Model            *QualifiedName // reference to a Model document
	Entity           *QualifiedName // reference to a domain entity
	MaxTokens        *int
	ToolChoice       string
	// Response streaming (#2022). Left out, a create or modify keeps the
	// stored value.
	EnableBackendStreaming *bool
	EnableUIStreaming      *bool
	// The model on a multi-model resource (#2031): its id, and the display
	// name Studio Pro stores. Left out, a create or modify keeps the stored
	// one; '' clears it.
	SelectedModel     *string
	SelectedModelName *string
	Temperature       *float64
	TopP              *float64
	SystemPrompt      string
	UserPrompt        string
	Variables         []AgentVarDef
	Tools             []AgentToolDef
	KBTools           []AgentKBToolDef
	CreateOrModify    bool // true for CREATE OR MODIFY / CREATE OR REPLACE
}

func (s *CreateAgentStmt) isStatement() {}

// DropAgentStmt represents: DROP AGENT Module.Name
type DropAgentStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropAgentStmt) isStatement() {}

// AlterAgentStmt represents:
//
//	ALTER AGENT Module.Name
//	    SET key = value (, key = value)*
//	    | ADD TOOL ... | ADD MCP SERVICE ... | ADD KNOWLEDGE BASE ...
//	    | DROP TOOL name | DROP MCP SERVICE Module.Name | DROP KNOWLEDGE BASE name
//	  ;
//
// Multiple actions can be combined in one statement; they apply in order.
type AlterAgentStmt struct {
	Name            QualifiedName
	Sets            map[string]string
	AddTools        []AgentToolDef   // includes TOOL and MCP SERVICE blocks (ToolType field distinguishes)
	AddKBs          []AgentKBToolDef // KNOWLEDGE BASE blocks
	DropTools       []string         // Microflow tool block names
	DropMCPServices []QualifiedName  // MCP service qualified names
	DropKBs         []string         // KB tool block names
}

func (s *AlterAgentStmt) isStatement() {}

// AgentVarDef is a variable entry in CREATE AGENT's Variables: (...) property.
type AgentVarDef struct {
	Key                 string
	IsAttributeInEntity bool
}

// AgentToolDef represents a TOOL or MCP SERVICE block in CREATE AGENT body.
type AgentToolDef struct {
	Name        string // block identifier (MCP: document qualified name; Tool: tool name)
	ToolType    string // "MCP" or "Microflow"
	Document    *QualifiedName
	Description string
	Enabled     bool
	// Agent Editor 2.2+ (#2021). Left out, a create or modify keeps the stored
	// value: "" / nil means the statement does not say.
	UserAccessApproval string
	DisplayTitle       *string
	DisplayDescription *string
}

// AgentKBToolDef represents a KNOWLEDGE BASE block in CREATE AGENT body.
type AgentKBToolDef struct {
	Name        string // per-agent identifier
	Source      *QualifiedName
	Collection  string
	MaxResults  int
	Description string
	Enabled     bool
	// Left out, a create or modify keeps the stored value.
	MinSimilarity *float64
	IsVisible     *bool
}
