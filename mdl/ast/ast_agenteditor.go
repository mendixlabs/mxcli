// SPDX-License-Identifier: Apache-2.0

package ast

// CreateModelStmt represents:
//
//	CREATE MODEL Module.Name (
//	  Provider: MxCloudGenAI,
//	  Key: Module.SomeConstant
//	  -- optional Portal-populated fields:
//	  [, DisplayName: '...']
//	  [, KeyName: '...']
//	  [, KeyId: '...']
//	  [, Environment: '...']
//	  [, ResourceName: '...']
//	  [, DeepLinkURL: '...']
//	);
type CreateModelStmt struct {
	Name          QualifiedName
	Documentation string
	Provider      string         // "MxCloudGenAI" by default
	Key           *QualifiedName // qualified name of the String constant holding the Portal key
	DisplayName   string         // optional Portal-populated metadata
	KeyName       string         // optional Portal-populated metadata
	KeyID         string         // optional Portal-populated metadata
	Environment   string         // optional Portal-populated metadata
	ResourceName  string         // optional Portal-populated metadata
	DeepLinkURL   string         // optional Portal-populated metadata
}

func (s *CreateModelStmt) isStatement() {}

// DropModelStmt represents: DROP MODEL Module.Name
type DropModelStmt struct {
	Name QualifiedName
}

func (s *DropModelStmt) isStatement() {}

// CreateConsumedMCPServiceStmt represents:
//
//	CREATE CONSUMED MCP SERVICE Module.Name (
//	  ProtocolVersion: v2025_03_26,
//	  Version: '0.0.1',
//	  ConnectionTimeoutSeconds: 30,
//	  Documentation: '...'
//	);
type CreateConsumedMCPServiceStmt struct {
	Name                     QualifiedName
	OuterDocumentation       string // /** ... */ doc comment
	ProtocolVersion          string
	Version                  string
	ConnectionTimeoutSeconds int
	InnerDocumentation       string // Contents.documentation field
}

func (s *CreateConsumedMCPServiceStmt) isStatement() {}

// DropConsumedMCPServiceStmt represents: DROP CONSUMED MCP SERVICE Module.Name
type DropConsumedMCPServiceStmt struct {
	Name QualifiedName
}

func (s *DropConsumedMCPServiceStmt) isStatement() {}

// CreateKnowledgeBaseStmt represents:
//
//	CREATE KNOWLEDGE BASE Module.Name (
//	  Provider: MxCloudGenAI,
//	  Key: Module.SomeConstant
//	);
type CreateKnowledgeBaseStmt struct {
	Name             QualifiedName
	Documentation    string
	Provider         string
	Key              *QualifiedName
	ModelDisplayName string
	ModelName        string
	KeyName          string
	KeyID            string
	Environment      string
	DeepLinkURL      string
}

func (s *CreateKnowledgeBaseStmt) isStatement() {}

// DropKnowledgeBaseStmt represents: DROP KNOWLEDGE BASE Module.Name
type DropKnowledgeBaseStmt struct {
	Name QualifiedName
}

func (s *DropKnowledgeBaseStmt) isStatement() {}

// CreateAgentStmt represents CREATE AGENT Module.Name (...) [{ body }].
type CreateAgentStmt struct {
	Name          QualifiedName
	Documentation string
	UsageType     string // "Task" or "Conversational"
	Description   string
	Model         *QualifiedName // reference to a Model document
	Entity        *QualifiedName // reference to a domain entity
	MaxTokens     *int
	ToolChoice    string
	Temperature   *float64
	TopP          *float64
	SystemPrompt  string
	UserPrompt    string
	Variables     []AgentVarDef
	Tools         []AgentToolDef
	KBTools       []AgentKBToolDef
}

func (s *CreateAgentStmt) isStatement() {}

// DropAgentStmt represents: DROP AGENT Module.Name
type DropAgentStmt struct {
	Name QualifiedName
}

func (s *DropAgentStmt) isStatement() {}

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
}

// AgentKBToolDef represents a KNOWLEDGE BASE block in CREATE AGENT body.
type AgentKBToolDef struct {
	Name        string // per-agent identifier
	Source      *QualifiedName
	Collection  string
	MaxResults  int
	Description string
	Enabled     bool
}
