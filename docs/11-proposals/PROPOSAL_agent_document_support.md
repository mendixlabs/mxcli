# Proposal: Agent Document Type Support in MDL

**Status:** Draft
**Date:** 2026-04-13

## Problem Statement

Mendix 11.9 introduces **Agents** as a first-class concept for building agentic AI applications. Agents are defined in Studio Pro through the Agent Editor extension and stored as `CustomBlobDocuments$CustomBlobDocument` with `CustomDocumentType = "agenteditor.agent"`. The ecosystem includes five marketplace modules:

| Module | Role |
|--------|------|
| **GenAICommons** | Core AI types: Request/Response, DeployedModel, Tool, ToolCall, Trace, KnowledgeBase |
| **MxGenAIConnector** | Mendix Cloud AI backend (Bedrock), model config, embeddings, knowledge base collections |
| **AgentCommons** | Agent management: versioned agents, tools, knowledge bases, MCP, test cases |
| **AgentEditorCommons** | Bridge between Studio Pro Agent Editor extension and AgentCommons runtime entities |
| **MCPClient** | Model Context Protocol client: server connections, tool/prompt discovery, execution |
| **ConversationalUI** | Chat widgets, message rendering, tool approval UI, trace monitoring, token & observability dashboards |

Currently, `mxcli` has no visibility into agent documents. `SHOW STRUCTURE` reports the Agents module as empty because it only contains `CustomBlobDocument` units, which are not parsed. An AI coding agent cannot discover, inspect, or create agents via MDL.

## BSON Structure

All four agent-editor document types (Agent, Model, Knowledge Base, Consumed MCP Service) share the same outer wrapper — a generic `CustomBlobDocument` with a JSON payload in `Contents`. They're distinguished by the `CustomDocumentType` field.

### Outer Wrapper (common to all four types)

```
CustomBlobDocuments$CustomBlobDocument:
  $ID: bytes
  $Type: "CustomBlobDocuments$CustomBlobDocument"
  Name: string
  Contents: string (JSON payload — schema depends on CustomDocumentType)
  DocumentType: "agenteditor.agent"
                    | "agenteditor.model"
                    | "agenteditor.knowledgebase"
                    | "agenteditor.consumedMCPService"
  Documentation: string
  Excluded: bool
  ExportLevel: "Hidden"
  Metadata:
    $ID: bytes
    $Type: "CustomBlobDocuments$CustomBlobDocumentMetadata"
    CreatedByExtension: "extension/agent-editor"
    ReadableTypeName: "Agent" | "Model" | "Knowledge base" | "Consumed MCP service"
```

Key observations about the wrapper:
- `CustomDocumentType` is the discriminator for the inner JSON schema
- `Contents` is a JSON string (not nested BSON) — the agent editor extension owns the inner schema
- `Metadata.ReadableTypeName` is a human-friendly label (also used as the UI badge in Studio Pro)
- `Excluded` is `false` by default; can be changed to `true` by the user. Then the document cannot be used by other app logic and no errors will be shown for this document.

### MODEL — `Contents` JSON schema

Observed in `Agents.MyFirstModel`:

```json
{
  "type": "Text generation",
  "name": "",
  "displayName": "",
  "provider": "MxCloudGenAI",
  "providerFields": {
    "environment": "",
    "deepLinkURL": "",
    "keyId": "",
    "keyName": "",
    "resourceName": "",
    "key": {
      "documentId": "51b85be5-f040-4562-bf4c-086347d387a9",
      "qualifiedName": "Agents.LLMKey"
    }
  }
}
```

- `provider` is a top-level discriminator — `"MxCloudGenAI"` is the only observed value. `providerFields` shape depends on `provider`.
- `providerFields.key` references a **String constant** (holding the Mendix Cloud GenAI Portal key) by `{documentId, qualifiedName}`.
- `type`, `name`, `displayName`, `environment`, `deepLinkURL`, `keyId`, `keyName`, `resourceName` are all empty in the sample — they values are decoded from the key the user selects as a string constant. After decoding the key, a call to the backend might update the reference to the exact model if that was changed in the Mendix Cloud GenAI portal. This check can also be triggered by the user when clicking **Test Key** in Studio Pro.

### KNOWLEDGE BASE — `Contents` JSON schema

Observed in `Agents.Knowledge_base`:

```json
{
  "name": "",
  "provider": "MxCloudGenAI",
  "providerFields": {
    "environment": "",
    "deepLinkURL": "",
    "keyId": "",
    "keyName": "",
    "modelDisplayName": "",
    "modelName": "",
    "key": {
      "documentId": "51b85be5-f040-4562-bf4c-086347d38712",
      "qualifiedName": "Agents.KnowledbaseKey"
    }
  }
}
```

Same shape as Model, but `providerFields` includes embedding-model info (`modelDisplayName`, `modelName`) instead of `resourceName`. The `key` reference points to a String constant with a knowledge base key generated in the Mendix Cloud GenAI portal.

### CONSUMED MCP SERVICE — `Contents` JSON schema

Observed in `Agents.Consumed_MCP_service`:

```json
{
  "protocolVersion": "v2025_03_26",
  "documentation": " fqwef qwec qwefc",
  "version": "0.0.1",
  "connectionTimeoutSeconds": 30,
  "endpoint" : {
    "documentId": "51b85be5-f040-4562-bf4c-086347d38734",
    "qualifiedName": "Agents.MCPEndpoint"
  },
  "authenticationMicroflow" : {
    "documentId": "51b85be5-f040-4562-bf4c-086347d387ab",
    "qualifiedName": "Agents.AuthenticationMicroflow"
  }
}
```

Endpoint is a reference to a string constant with the MCP endpoint. If the server requried authentication those can be created in an authentication microflow which the user can optionally select. An authentication microflow cannot have input parameters and needs to return a list of `System.HttpHeader`.
For MCP tool discovery inside Studio Pro, user can add headers in the UI. Those will not be persisted, nor transferred into the authentication microflow to be used at runtime.

Enum values for `protocolVersion`: `"v2024_11_05"` or `"v2025_03_26"`. Use `v2025_03_26` for newer MCP servers that support streamable http transport, `v2024_11_05` for servers that only support SSE transport.

### AGENT — `Contents` JSON schema

Simple agent (observed in `AgentEditorCommons.TranslationAgent`):

```json
{
  "description": "",
  "systemPrompt": "Translate the given text into {{Description}}.",
  "userPrompt": "...",
  "usageType": "Task",
  "variables": [
    { "key": "Description", "isAttributeInEntity": true }
  ],
  "tools": [],
  "knowledgebaseTools": [],
  "entity": {
    "documentId": "83d81a7b-4a84-416e-a64f-0ffa981c8408",
    "qualifiedName": "System.Language"
  },
  "model": {
    "documentId": "3addaaa1-8bd3-4654-8cc9-2c886d0a01e9",
    "qualifiedName": "Agents.MyFirstModel"
  }
}
```

Fully-populated agent (observed in `Agents.Agent007`):

```json
{
  "description": "doing your stuff for you",
  "systemPrompt": "Do you intereesting and useful stuff that makes me money",
  "userPrompt": "Just do it",
  "usageType": "Task",
  "variables": [],
  "tools": [
    {
      "id": "044bc8c2-8ca6-4166-b8f0-9d2245aba8c7",
      "name": "",
      "description": "",
      "enabled": true,
      "toolType": "MCP",
      "document": {
        "qualifiedName": "Agents.Consumed_MCP_service",
        "documentId": "47c9987a-e922-44eb-a389-e641f325ce15"
      }
    },
    {
      "id": "044bc8c2-8ca6-4166-b8f0-9d2245aba8c7",
      "name": "GetBankHolidays",
      "description": "Gets bank holidays",
      "enabled": true,
      "toolType": "Microlfow",
      "document": {
        "qualifiedName": "Agents.GetBankHolidays",
        "documentId": "47c9987a-e922-44eb-a389-e641f325ce18"
      }
    }
  ],
  "knowledgebaseTools": [
    {
      "id": "20980c3d-399b-409e-8292-49df7d0ab533",
      "name": "My_mem",
      "description": "My memory of useful stuff",
      "enabled": true,
      "toolType": "",
      "document": {
        "qualifiedName": "Agents.Knowledge_base",
        "documentId": "cccc0b5b-7600-47a9-8f6e-5761ce2fc620"
      },
      "collectionIdentifier": "agent1-collection",
      "maxResults": 3,
      "minSimilarity": 0.5
    }
  ],
  "model": {
    "documentId": "3addaaa1-8bd3-4654-8cc9-2c886d0a01e9",
    "qualifiedName": "Agents.MyFirstModel"
  },
  "maxTokens": 16384,
  "temperature": 0.5,
  "topP": 1.0,
  "toolChoice": "Tool",
  "toolChoiceToolName": "GetBankHolidays"
}
```

Agent schema observations:
- **`model`**: Reference to a Model document by `{documentId, qualifiedName}`.
- **`entity`** the prompts might contain some variables (words in the user prompt surrounded by double curly brackets, i.e. {{Language}}). Variables will be replaced at runtime by attribute values on an initialized object of type entity. Therefore it is neccessary that the variables key is the same as an attribute on the entity.
- **`usageType`**: Defaults to `"Task"` agents which have aan optional system prompt and a required user prompt. `"Chat"` agents (introduced in v1.1.0 of the agent editor) do not have a user prompt on the agent definition since that will be provided by the user at runtime.
- **`tools[]`**: Optional. Array of tool references. Each entry has a UUID `id`, unique `name`, `description`, `enabled` boolean, and a `toolType` discriminator. Observed `toolType` values: `"MCP"` (a whole MCP service attached as tools) or a microflow-tool `"Microflow"` where `document` referencces a microflow document. Tool microflows need to return a String and can only have primitive types and GenAICommons.Request or GenAICommons.Tool objects as input parameters.
- **`knowledgebaseTools[]`**: Optional. Array of KB references. Same base fields plus `collectionIdentifier`, `maxResults` and `minSimilarity`. ToolType is irrelevant for knowledgebaseTools. MaxResults needs to be a positive integer, minSimilarity is a decimal between 0 and 1.
- **`variables[]`**: Should be left empty this will be automatically populated by the extension based on the detected variables in the user or system prompt.
- **`entity`**: Optional. Present on older agents with `isAttributeInEntity: true` variables; absent on `Agent007`.
- **`maxTokens`**: Optional. Can be set by the user to restrict the number of tokens to consume in one agent call.
- **`toolChoice`**: Optional. Agent-level inference parameters. Enum values for `toolChoice` observed: `"Auto"` (capitalized, not the lowercase `auto` used by `GenAICommons.ENUM_ToolChoice` at runtime). Other values: `"None"`, `"Any"`, `"Tool"`. If tool choice is set to `Tool`, then also the `toolChoiceToolName` needs to be set by the unique name referencing one tool in `tools[]`. Only tools where `toolType` = "Microflow" can become tool choice.
- **`temperature`**, **`topP`**: Optional. Can be set by the user to influence the randomness of the response.
- **No `UserAccessApproval`/`Access` field** on tools. That's a runtime-only concern (set on `AgentCommons.Tool` entity, not the document). **This is a correction to earlier versions of this proposal.**

### Observed documents in test3 project

| Document | Type | Notes |
|---|---|---|
| `AgentEditorCommons.InformationExtractorAgent` | Agent | Older format, `Excluded: true`, no model reference |
| `AgentEditorCommons.SummarizationAgent` | Agent | Older format |
| `AgentEditorCommons.TranslationAgent` | Agent | Older format, bound to `System.Language` |
| `AgentEditorCommons.ProductDescription` | Agent | Older format, bound to `AgentCommons.ProductDescriptionGenerator_EXAMPLE` |
| `Agents.Agent007` | Agent | New format with model reference, MCP tool, KB tool |
| `Agents.MyFirstModel` | Model | Provider `MxCloudGenAI`, key → `Agents.LLMKey` |
| `Agents.Knowledge_base` | Knowledge Base | Provider `MxCloudGenAI`, key → `Agents.LLMKey` |
| `Agents.Consumed_MCP_service` | Consumed MCP Service | Protocol v2025_03_26, timeout 30s |

## Proposed MDL Syntax

### SHOW AGENTS

```sql
SHOW AGENTS [IN Module]
```

Output:

| Qualified Name | Module | Name | UsageType | Entity | Variables |
|----------------|--------|------|-----------|--------|-----------|
| AgentEditorCommons.InformationExtractorAgent | AgentEditorCommons | InformationExtractorAgent | Task | AgentCommons.InformationExtractor_EXAMPLE | Information |
| AgentEditorCommons.SummarizationAgent | AgentEditorCommons | SummarizationAgent | Task | | |
| AgentEditorCommons.TranslationAgent | AgentEditorCommons | TranslationAgent | Task | System.Language | Description |
| AgentEditorCommons.ProductDescription | AgentEditorCommons | ProductDescription | Task | AgentCommons.ProductDescriptionGenerator_EXAMPLE | ProductName, Keywords |

### DESCRIBE AGENT

```sql
DESCRIBE AGENT AgentEditorCommons.TranslationAgent
```

Output (round-trippable MDL):

```sql
CREATE AGENT AgentEditorCommons."TranslationAgent" (
  UsageType: Task,
  Entity: System.Language,
  Variables: ("Description": EntityAttribute),
  SystemPrompt: 'Translate the given text into {{Description}}.',
  UserPrompt: 'What is a multi-agent AI system?...'
);
/
```

### CREATE AGENT

The syntax follows the same shape as `CREATE REST CLIENT`: top-level configuration in `(...)` followed by a `{...}` body containing one block per attached resource (`TOOL`, `KNOWLEDGE BASE`, `MCP SERVICE`). Simple agents with no resources omit the body entirely.

**Simple task agent (no body needed):**

```sql
CREATE AGENT MyModule."SentimentAnalyzer" (
  UsageType: Task,
  Entity: MyModule.FeedbackItem,
  Variables: ("FeedbackText": EntityAttribute),
  Model: MyModule.GPT4Model,
  SystemPrompt: 'Analyze the sentiment of {{FeedbackText}}. Classify as positive, negative, or neutral.',
  UserPrompt: '{{FeedbackText}}'
);
```

**Agent with tools, knowledge bases, and MCP services (matches `Agents.Agent007`):**

```sql
CREATE AGENT Agents."Agent007" (
  UsageType: Task,
  Model: Agents.MyFirstModel,
  MaxTokens: 16384,
  ToolChoice: Auto,
  Description: 'doing your stuff for you',
  SystemPrompt: 'Do you intereesting and useful stuff that makes me money',
  UserPrompt: 'Just do it'
)
{
  MCP SERVICE Agents.Consumed_MCP_service {
    Enabled: true
  }

  KNOWLEDGE BASE My_mem {
    Source: Agents.Knowledge_base,
    Collection: 'agent1-collection',
    MaxResults: 3,
    Description: 'My memory of useful stuff',
    Enabled: true
  }
};
```

**Block-level property reference:**

Each block maps to one entry in the agent's `Contents` JSON (`tools[]` for TOOL/CONSUMED MCP SERVICE, `knowledgebaseTools[]` for KNOWLEDGE BASE). Block IDs (the `id` UUID field in JSON) are auto-generated by the writer.

| Block | Referenced by | Properties | Maps to JSON field |
|---|---|---|---|
| `CONSUMED MCP SERVICE <QualifiedName> { ... }` | ConsumedMCPService document | `Enabled`, `Description` | `tools[]` entry with `toolType: "MCP"`, `document: {...}` |
| `TOOL <Name> { Microflow: <QualifiedName>, Description, Enabled }` | microflow name | `Microflow`, `Enabled`, `Description` | `tools[]` entry with `toolType: "Microflow"`, `document: { qualifiedName: <microflow>, documentId: <uuid> }`. Microflow tools must be microflow references (qualified name) and the target microflow must return a `String`; input parameters are limited to primitives and `GenAICommons.Request`/`GenAICommons.Tool` types. |
| `KNOWLEDGE BASE <Name> { Source: ... }` | KB document via `Source:` | `Source` (required), `Collection`, `MaxResults`, `Description`, `Enabled` | `knowledgebaseTools[]` entry |

### DROP AGENT

```sql
DROP AGENT MyModule."SentimentAnalyzer"
```

### What Goes Where: Design-Time vs. Call-Time

Everything inside `CREATE AGENT` — including the properties in the top-level `(...)` — is **design-time configuration that is stored in the agent document**. None of it is an invocation parameter. Runtime inputs are supplied at the `CALL AGENT` site.

The layering follows the same pattern as `CREATE REST CLIENT`:

| Layer | REST CLIENT | AGENT |
|---|---|---|
| **Document-level static config** (stored in document) | `BaseUrl`, `Authentication` | `UsageType`, `Description`, `Entity`, `Model`, `MaxTokens`, `ToolChoice`, `SystemPrompt`, `UserPrompt` |
| **Input contract** (what the caller must bring at call time) | `Parameters: ($id: String)` on each operation | `Variables: ("Topic": String, ...)` on the agent |
| **Attached resources** (body blocks) | `OPERATION` blocks | `TOOL` / `KNOWLEDGE BASE` / `MCP SERVICE` blocks |
| **Runtime invocation** (values supplied at call site) | `SEND REST REQUEST Mod.Api.GetItems (id = $x)` | `CALL AGENT WITH HISTORY $agent REQUEST $req CONTEXT $obj` |

In other words:

- `UsageType`, `Entity`, `SystemPrompt`, `UserPrompt` are the same kind of property as `BaseUrl` on a REST client — baked into the document, changed by editing the document.
- `Variables: (...)` is the same kind of property as `Parameters: (...)` on a REST operation — it declares the **schema** of what the caller must supply, not the values. Actual values arrive at runtime: for `EntityAttribute` variables, they're read from matching attributes on the `CONTEXT` object; for free-form variables (future extension), they'd be passed directly.
- `TOOL`, `KNOWLEDGE BASE`, `MCP SERVICE` blocks describe **capabilities the agent carries with it** — the LLM can invoke them autonomously at runtime, but they aren't something the caller passes in.

Example — all of this is stored in the agent document:

```sql
CREATE AGENT Reviews."SentimentAnalyzer" (
  UsageType: Task,                                             -- design-time mode
  Entity: Reviews.ProductReview,                               -- context entity contract
  Variables: ("ProductName": EntityAttribute,                  -- input contract; which attributes to read from the context object at runtime
              "ReviewText": EntityAttribute),
  SystemPrompt: 'Analyze the review for {{ProductName}}.',     -- prompt template
  UserPrompt: '{{ReviewText}}'                                 -- prompt template
);
```

And this is the runtime call — the only place values flow in:

```sql
CALL AGENT WITHOUT HISTORY $Agent CONTEXT $Review INTO $Response;
-- $Review is a Reviews.ProductReview instance;
-- its ProductName and ReviewText attributes satisfy the Variables contract.
```

### Syntax Design Rationale

| Decision | Rationale |
|----------|-----------|
| `AGENT` as document type keyword | Matches `Metadata.ReadableTypeName = "Agent"` and Mendix UI terminology |
| Top-level `(Key: Value)` config + `{...}` body with singular blocks | Mirrors `CREATE REST CLIENT ... (...) { OPERATION Name {...} }` exactly — same shape, same mental model |
| `Model: <QualifiedName>` in top-level config | Agent documents can reference a Model document directly via the `model` JSON field (confirmed in `Agent007`). |
| `UsageType: Task` determines if user prompt is a template or not | Task agents have a fixed user prompt, potentially containing variables, while `Chat` agents do not have a predefined userprompt, because it is determined by the user at runtime. |
| `ToolChoice: Auto` PascalCase enum literal | Matches the real JSON value (`"Auto"`), which differs from the lowercase `auto` used by `GenAICommons.ENUM_ToolChoice` at runtime. Values: `Auto`, `None`, `Any`, `Tool`. When `ToolChoice` is set to `Tool`, the `toolChoiceToolName` property must be set to the agent-local tool `name` (the `name` field in `tools[]`). The writer/validator must ensure the named tool exists on the agent, is unique, has `toolType: "Microflow"`, and is `Enabled: true`; otherwise emit a validation error. `toolChoiceToolName` selects the microflow tool the agent will prefer when `ToolChoice` is fixed to `Tool`. |
| `MaxTokens: <int>` on the agent | Matches the JSON `maxTokens` field; agent-level inference parameter |
| `TOOL`, `KNOWLEDGE BASE`, `MCP SERVICE` as singular block types | Matches the `OPERATION` singular used in REST CLIENT; each block defines one resource |
| `MCP SERVICE <QualifiedName> { Enabled, Description }` | The name is the qualified name of a ConsumedMCPService document (the whole service is attached as a bundle of tools) |
| `KNOWLEDGE BASE <Name> { Source: <doc>, Collection, MaxResults, ... }` | `<Name>` is the per-agent identifier stored in JSON `name`; `Source:` references the KB document. Matches `Agent007`'s `My_mem` KB entry |
| `TOOL <Name> { Microflow: <QualifiedName>, Description, Enabled }` | Microflow tool references a microflow by qualified name; the writer encodes this in `tools[]` with `toolType: "Microflow"` and `document: { qualifiedName: <microflow>, documentId: <uuid> }`. Target microflows must return a `String`; input parameters are restricted to primitives and `GenAICommons.Request`/`GenAICommons.Tool` types. |
| `Variables: (...)` is the input-schema analog of REST CLIENT's `Parameters: (...)` | Declares what the caller must supply; values flow in via the `CONTEXT` object at the `CALL AGENT` site. Inline form matches REST CLIENT's `Parameters: ($id: String)` |
| No `Access:` on tool blocks | `UserAccessApproval` is NOT stored in the agent document JSON — it's a runtime-only concern on the `AgentCommons.Tool` entity. (Earlier drafts of this proposal incorrectly placed it on the block.) |
| Body omitted when there are no tools/KB/MCP | Same concession REST CLIENT makes implicitly — empty bodies are awkward; drop them |
| Prompts as string literals | Consistent with other MDL string properties; `{{var}}` placeholders are just text |
| Auto-generated `id` UUIDs on block entries | Each tool/KB entry has a UUID `id` in the JSON (Studio Pro-generated). The MDL writer will generate these; they round-trip stably through `DESCRIBE` |

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | No | — |
| **BSON parser** | No (CustomBlobDocuments not parsed) | — |
| **Reader/Catalog** | No | — |
| **Grammar** | No | — |
| **AST** | No | — |
| **Visitor** | No | — |
| **Executor** | No | — |
| **Generated metamodel** | Partial | `generated/metamodel/types.go` has `CustomBlobDocuments$CustomBlobDocument` |

## Implementation Plan

### Phase 1: Read Support (SHOW/DESCRIBE)

#### 1.1 Add Model Types

In a new file `sdk/agents/types.go` (or extend an existing domain):

```go
package agents

import "github.com/nicholasgasior/modelsdk-go/model"

type Agent struct {
    ContainerID   model.ID
    ID            model.ID
    Name          string
    Documentation string

    // Parsed from Contents JSON
    Description  string
    SystemPrompt string
    UserPrompt   string
    UsageType    string        // "Task", "Conversational"
    Variables    []Variable
    Tools        []ToolRef     // tools[] array
    KBTools      []KBToolRef   // knowledgebaseTools[] array
    Model        *DocRef       // optional, points to a Model document
    Entity       *EntityRef    // optional, points to a domain entity
    MaxTokens    *int          // optional
    ToolChoice   string        // optional: "Auto", "None", "Any", "Tool"
    ToolChoiceToolName *string // optional: when ToolChoice == "Tool", the agent-local tool `name` to prefer
    Temperature  *float64      // optional, not yet observed
    TopP         *float64      // optional, not yet observed
}

type Variable struct {
    Key                 string
    IsAttributeInEntity bool // true when an attribute with name == Key is found on the referenced entity
}

type EntityRef struct {
    DocumentID    string // UUID of the entity's domain model
    QualifiedName string // Module.EntityName
}

type DocRef struct {
    DocumentID    string // UUID of the referenced CustomBlobDocument or a microflow
    QualifiedName string // Module.DocumentName
}

// Entry in the agent's tools[] array
type ToolRef struct {
    ID          string  // per-tool UUID (generated by writer)
    Name        string  // only relevant for microflow tools; unique tool name (used by `toolChoiceToolName`); Tool name must start with a letter or underscore and contain only letters, numbers, and underscores.
    Description string  // only relevant for microflow tools
    Enabled     bool    // diabled tools will be ignored at runtime
    ToolType    string  // "MCP" | "Microflow"
    Document    *DocRef // references ConsumedMCPService for ToolType=="MCP", references a microflow for ToolType=="Microflow"
}

// Entry in the agent's knowledgebaseTools[] array
type KBToolRef struct {
    ID                   string
    Name                 string
    Description          string
    Enabled              bool
    ToolType             string  // unused for knowledge base tools
    Document             *DocRef // references KnowledgeBase document
    CollectionIdentifier string
    MaxResults           int
    MinSimilarity        float64 // decimal between 0.0 and 1.0
}

// Peer document types (same wrapper, different Contents JSON)

type Model struct {
    ContainerID   model.ID
    ID            model.ID
    Name          string
    Documentation string

    Type        string                 // Portal-populated, usually empty
    DisplayName string                 // Portal-populated
    Provider    string                 // "MxCloudGenAI"
    Fields      map[string]interface{} // providerFields — shape depends on provider
    KeyConstant *ConstantRef           // providerFields.key → String constant
}

type KnowledgeBase struct {
    ContainerID   model.ID
    ID            model.ID
    Name          string
    Documentation string

    Provider    string                 // "MxCloudGenAI"
    Fields      map[string]interface{} // providerFields (includes modelDisplayName, modelName)
    KeyConstant *ConstantRef           // providerFields.key → String constant
}

type ConsumedMCPService struct {
    ContainerID              model.ID
    ID                       model.ID
    Name                     string
    Documentation            string

    ProtocolVersion          string // "v2024_11_05" | "v2025_03_26"
    Version                  string // app-specified version
    InnerDocumentation       string // Contents.documentation (free text)
    ConnectionTimeoutSeconds int
    Endpoint                 *DocRef // reference to a String constant document containing the MCP endpoint
    AuthenticationMicroflow  *DocRef // optional microflow used to produce auth headers; must have no input params and return List<System.HttpHeader>
}

type ConstantRef struct {
    DocumentID    string
    QualifiedName string // e.g. "Agents.LLMKey"
}
```

#### 1.2 Add BSON Parser

In `sdk/mpr/parser_customblob.go` (generic — handles all four document types):

- Parse `CustomBlobDocuments$CustomBlobDocument` documents
- Dispatch by `CustomDocumentType`:
  - `"agenteditor.agent"` → decode Contents JSON as `Agent`
  - `"agenteditor.model"` → decode as `Model`
  - `"agenteditor.knowledgebase"` → decode as `KnowledgeBase`
  - `"agenteditor.consumedMCPService"` → decode as `ConsumedMCPService`
  - unknown → store raw Contents, warn
- Store in per-type maps on the reader

The parser should be tolerant: unknown JSON fields in `Contents` are ignored (the agent editor extension may add fields in future versions).

#### 1.3 Add Reader Methods

```go
func (r *Reader) Agents() []*agenteditor.Agent
func (r *Reader) AgentByQualifiedName(name string) *agenteditor.Agent

func (r *Reader) Models() []*agenteditor.Model
func (r *Reader) ModelByQualifiedName(name string) *agenteditor.Model

func (r *Reader) KnowledgeBases() []*agenteditor.KnowledgeBase
func (r *Reader) KnowledgeBaseByQualifiedName(name string) *agenteditor.KnowledgeBase

func (r *Reader) ConsumedMCPServices() []*agenteditor.ConsumedMCPService
func (r *Reader) ConsumedMCPServiceByQualifiedName(name string) *agenteditor.ConsumedMCPService
```

#### 1.4 Add Catalog Tables

- `CATALOG.AGENTS` (module, name, qualified_name, usage_type, entity, model, variables, tool_count, kb_count)
- `CATALOG.MODELS` (module, name, qualified_name, provider, key_constant)
- `CATALOG.KNOWLEDGE_BASES` (module, name, qualified_name, provider, key_constant)
- `CATALOG.CONSUMED_MCP_SERVICES` (module, name, qualified_name, endpoint_constant, protocol_version, authentication_microflow, timeout_seconds)

#### 1.5 Add Grammar/AST/Visitor/Executor

- Grammar: `SHOW {AGENTS | MODELS | KNOWLEDGE BASES | CONSUMED MCP SERVICES} [IN module]`
- Grammar: `DESCRIBE {AGENT | MODEL | KNOWLEDGE BASE | CONSUMED MCP SERVICE} qualifiedName`
- AST: `ShowCustomBlobStmt`, `DescribeCustomBlobStmt` (discriminated by type enum)
- Executor: format output using standard table/MDL patterns

**Recommended implementation order** (matches user preference to start with MODEL):
1. Generic wrapper parser + `Model` type + `SHOW MODELS` + `DESCRIBE MODEL` (smallest Contents JSON)
2. `ConsumedMCPService` (also small)
3. `KnowledgeBase` (similar shape to Model)
4. `Agent` (largest, depends on the other three for resolving `model`/`document` references in its body)

### Phase 2: Write Support (CREATE/DROP)

#### 2.1 Add BSON Writer

In `sdk/mpr/writer_customblob.go` (generic wrapper for all four types):

- Serialize any of `Agent` / `Model` / `KnowledgeBase` / `ConsumedMCPService` structs to a `CustomBlobDocuments$CustomBlobDocument` BSON
- Set `CustomDocumentType` per type (`agenteditor.agent`, `agenteditor.model`, etc.)
- Set `Metadata.CreatedByExtension = "extension/agent-editor"`
- Set `Metadata.ReadableTypeName` per type (`"Agent"`, `"Model"`, `"Knowledge base"`, `"Consumed MCP service"`)
- Serialize `Contents` as a JSON string (per-type encoder)
- Set `Excluded = false`, `ExportLevel = "Hidden"` (matches the new Agent Editor defaults)
- Generate stable UUIDs for `$ID` and `Metadata.$ID`
- For `Agent`: generate UUIDs for `id` field on each `tools[]` and `knowledgebaseTools[]` entry
- For `Model` / `KnowledgeBase`: resolve the `Key` constant reference to `{documentId, qualifiedName}` by looking up the String constant in the reader

#### 2.2 Add Grammar/AST/Visitor/Executor for CREATE/DROP

- Grammar: `CREATE AGENT qualifiedName properties variablesClause?`
- AST: `CreateAgentStmt`, `DropAgentStmt`
- Executor: validate, write BSON, register in module

#### 2.3 Validation

- Entity reference must exist (if specified)
- Variables marked `EntityAttribute` must correspond to attributes on the referenced entity
- `UsageType` must be a known value (`Task` or `Chat`)
- Variable names used in `{{...}}` in prompts should match declared variables (warning, not error)

### Phase 3: Integration & Catalog

#### 3.1 Catalog Integration

- Include agents in `SHOW STRUCTURE` output
- Add `CATALOG.AGENTS` table for SQL queries
- Include agent references in `SHOW REFERENCES` / `SHOW IMPACT`
- Wire into `REFRESH CATALOG` (both fast and full modes)

#### 3.2 Version Gating

- Agent documents (`CustomBlobDocuments$CustomBlobDocument`) require Mendix 11.x
- Add to `sdk/versions/mendix-11.yaml`:
  ```yaml
  agents:
    agent_document:
      min_version: "11.9.0"
      mdl: "CREATE AGENT Module.Name (...) { TOOL ... { ... } ... }"
      notes: "Requires AgentEditorCommons marketplace module"
  ```
- Executor pre-check: `checkFeature("agent_document")` before CREATE

#### 3.3 LSP Support

- Hover on agent names shows system prompt summary
- Go-to-definition navigates to agent document
- Completion for `DESCRIBE AGENT` with agent names

### Phase 4: Supporting Document Types and Microflow Activities

Full agent support requires MDL coverage of related `CustomBlobDocument` types and the new "Call Agent" microflow activity. These are split into sub-phases but all are needed for the examples in this proposal to work end-to-end.

#### 4.1 `CREATE MODEL` Document

Models are peer `CustomBlobDocument`s that reference a Mendix Cloud GenAI Portal key stored in a **String constant**. The minimum input from the user is the provider and the constant reference — Portal metadata (`displayName`, `keyId`, `keyName`, `environment`, `resourceName`, etc.) is filled by Studio Pro when the user clicks **Test Key** and shouldn't be user-set in MDL.

Matches the observed BSON for `Agents.MyFirstModel`:

```sql
CREATE MODEL Agents."MyFirstModel" (
  Provider: MxCloudGenAI,
  Key: Agents.LLMKey
);
```

`DESCRIBE MODEL` may show Portal-populated fields when present (round-trip preserves them, but they're not user-editable in MDL):

```sql
-- What DESCRIBE produces for a model that has been activated against the Portal
CREATE MODEL Agents."MyFirstModel" (
  Provider: MxCloudGenAI,
  Key: Agents.LLMKey,
  DisplayName: 'GPT-4 Turbo',          -- Portal-populated, read-only in MDL
  KeyName: 'prod-gpt4',                -- Portal-populated, read-only in MDL
  Environment: 'production'            -- Portal-populated, read-only in MDL
);
```

At runtime, `AgentEditorCommons.ASU_AgentEditor` reads the constant and creates the corresponding `GenAICommons.DeployedModel`.

**JSON output shape:**
```json
{
  "type": "",
  "name": "",
  "displayName": "<portal-populated or empty>",
  "provider": "MxCloudGenAI",
  "providerFields": {
    "environment": "", "deepLinkURL": "", "keyId": "", "keyName": "", "resourceName": "",
    "key": { "documentId": "<uuid>", "qualifiedName": "Agents.LLMKey" }
  }
}
```

#### 4.2 `CREATE KNOWLEDGE BASE` Document

Same shape as Model, but `providerFields` carries embedding-model info instead of model-resource info. User-settable fields are just `Provider` and `Key`:

```sql
CREATE KNOWLEDGE BASE Agents."Knowledge_base" (
  Provider: MxCloudGenAI,
  Key: Agents.LLMKey
);
```

`DESCRIBE` can round-trip Portal-populated fields:

```sql
CREATE KNOWLEDGE BASE Agents."Knowledge_base" (
  Provider: MxCloudGenAI,
  Key: Agents.LLMKey,
  ModelDisplayName: 'text-embedding-3-large',   -- Portal-populated
  ModelName: 'text-embedding-3-large'           -- Portal-populated
);
```

Referenced from agents via `KNOWLEDGE BASE <Name> { Source: <QualifiedName>, ... }` blocks inside the agent body.

**JSON output shape:**
```json
{
  "name": "",
  "provider": "MxCloudGenAI",
  "providerFields": {
    "environment": "", "deepLinkURL": "", "keyId": "", "keyName": "",
    "modelDisplayName": "", "modelName": "",
    "key": { "documentId": "<uuid>", "qualifiedName": "Agents.LLMKey" }
  }
}
```

#### 4.3 `CREATE CONSUMED MCP SERVICE` Document

Matches the observed BSON for `Agents.Consumed_MCP_service`. Endpoint and credentials are **not** part of the document — they're runtime configuration on the `MCPClient.ConsumedMCPService` entity. The document only carries protocol version, app-level version, timeout, and documentation.

```sql
CREATE CONSUMED MCP SERVICE Agents."Consumed_MCP_service" (
  ProtocolVersion: v2025_03_26,
  Version: '0.0.1',
  ConnectionTimeoutSeconds: 30,
  Documentation: 'Description of what this MCP service provides'
);
```

Referenced from agents via `MCP SERVICE <QualifiedName> { ... }` blocks inside the agent body.

**JSON output shape:**
```json
{
  "protocolVersion": "v2025_03_26",
  "documentation": "Description of what this MCP service provides",
  "version": "0.0.1",
  "connectionTimeoutSeconds": 30
}
```

> **Note:** `Documentation` as a top-level property maps to the JSON `documentation` field (inside `Contents`), not to the outer BSON `Documentation` field on the CustomBlobDocument wrapper. Two different fields with the same name — the MDL writer sets both consistently.

#### 4.4 `CALL AGENT` / `NEW CHAT FOR AGENT` Microflow Activities

New MDL microflow statements mapping to the Agents Kit toolbox actions (see the "New MDL Statement" section under "Building Smart Apps" for syntax):

| MDL Statement | Java Action | Purpose |
|---------------|-------------|---------|
| `CALL AGENT WITH HISTORY $agent REQUEST $req [CONTEXT $obj] INTO $Response` | `AgentCommons.Agent_Call_WithHistory` | Call a conversational agent with chat history |
| `CALL AGENT WITHOUT HISTORY $agent [CONTEXT $obj] [REQUEST $req] [FILES $fc] INTO $Response` | `AgentCommons.Agent_Call_WithoutHistory` | Call a single-call (Task) agent |
| `NEW CHAT FOR AGENT $agent ACTION MICROFLOW <mf> [CONTEXT $obj] [MODEL $dm] INTO $ChatContext` | `AgentCommons.ChatContext_Create_ForAgent` | Create a ChatContext wired to an agent |

These need a new BSON activity type (or mapping to the generic Java action call — see Open Question 4).

#### 4.5 ALTER AGENT

Follows the same shape as `ALTER PAGE` — in-place modifications to top-level properties and body blocks:

```sql
ALTER AGENT MyModule."SentimentAnalyzer" {
  SET SystemPrompt = 'New prompt with {{Variable}}.';
  SET Variables = ("FeedbackText": EntityAttribute, "NewVar": String);
  SET Model = MyModule.OtherModel;
  SET ToolChoice = None;

  INSERT MCP SERVICE MyModule.NewMCPService {
    Enabled: true
  };

  INSERT KNOWLEDGE BASE NewKB {
    Source: MyModule.OtherKB,
    Collection: 'other-collection',
    MaxResults: 5
  };

  DROP MCP SERVICE MyModule.OldMCPService;
};
```

## Building Smart Apps with MDL: End-to-End Examples

This section demonstrates how MDL agent support, combined with the existing agentic marketplace modules (GenAICommons, AgentCommons, MCPClient, MCPServer, ConversationalUI, and one of the connectors such as MxGenAIConnector), enables building complete AI-powered applications entirely from MDL scripts.

> **Sources:** This section follows the official Mendix documentation for the [Agent Editor](https://docs.mendix.com/appstore/modules/genai/genai-for-mx/agent-editor/), [Agent Commons](https://docs.mendix.com/appstore/modules/genai/genai-for-mx/agent-commons/), [Conversational UI](https://docs.mendix.com/appstore/modules/genai/genai-for-mx/conversational-ui/), [Mendix Cloud GenAI Connector](https://docs.mendix.com/appstore/modules/genai/mx-cloud-genai/MxGenAI-connector/), [MCP Client](https://docs.mendix.com/appstore/modules/genai/mcp-modules/mcp-client/), and [MCP Server](https://docs.mendix.com/appstore/modules/genai/mcp-modules/mcp-server/).

### Architecture Overview

A "smart app" in Mendix typically has these layers, all expressible in MDL:

```
┌─────────────────────────────────────────────────────────────────┐
│                     Conversational UI                           │
│         Chat widget, tool approval, trace monitoring            │
├─────────────────────────────────────────────────────────────────┤
│                      Agent Layer                                │
│  Agent documents (CREATE AGENT) — prompts, variables,           │
│           tools, knowledge bases, MCP servers                   │
├──────────────┬────────────────────┬─────────────────────────────┤
│    Tools     │   Knowledge Bases  │      MCP Services           │
│  Microflows  │   RAG retrieval    │  External tool servers      │
├──────────────┴────────────────────┴─────────────────────────────┤
│              Model Documents + MxGenAIConnector                 │
│      Model key constant → DeployedModel at runtime              │
├─────────────────────────────────────────────────────────────────┤
│                    Domain Model                                 │
│           Entities, associations, enumerations                  │
└─────────────────────────────────────────────────────────────────┘
```

### Key Correction: How Agents Work in Mendix

Unlike the initial draft of this proposal, agents in Mendix are **not** wired up by building the request manually in an action microflow. The correct flow is:

1. **Studio Pro design time** — the developer creates agent documents (and model documents) in Studio Pro. Tools, knowledge bases, and MCP servers are **attached to the agent in the agent document itself** (not added at runtime).
2. **Model key** — a Mendix Cloud GenAI Portal key is stored in a String constant on the model document. At runtime, `ASU_AgentEditor` (registered as after-startup microflow) reads the key and auto-creates the corresponding `GenAICommons.DeployedModel`.
3. **Call Agent activity** — in a microflow, a single **"Call Agent With History"** or **"Call Agent Without History"** toolbox action does everything: resolve the agent's in-use version, select its deployed model, replace variable placeholders from the context object, wire in tools/knowledge bases/MCP servers declared on the agent, and call the LLM.
4. **Conversational UI** — to use the agent in a chat, call **"New Chat for Agent"** which creates a `ChatContext` pre-configured with the agent's deployed model, system prompt, and action microflow. The action microflow for chat just calls **"Call Agent With History"** with the request built by `Default Preprocessing`.

### Prerequisites

Before any agent can be used, the following one-time setup is required (see the [Agent Editor prerequisites](https://docs.mendix.com/appstore/modules/genai/genai-for-mx/agent-editor/)):

```sql
-- 1. Encryption key must be 32 characters (App > Settings > Configuration).
--    The Encryption module is a prerequisite; its setup is outside MDL.

-- 2. Register ASU_AgentEditor as an after-startup microflow
ALTER SETTINGS MODEL AfterStartupMicroflow = AgentEditorCommons.ASU_AgentEditor;

-- 3. A model key constant must hold a Mendix Cloud GenAI Portal key.
--    Use one constant per model. The agent editor references this constant
--    in the model document.
CREATE CONSTANT "MyApp"."DefaultModelKey" (
  Type: String,
  DefaultValue: ''   -- set via environment config or configuration UI
);

-- 4. Ensure the required module roles are assigned.
--    MxGenAIConnector.Administrator is needed to configure the connector.
--    AgentCommons.AgentAdmin is needed to manage agents in the runtime UI.

-- 5. Exclude the auto-created /agenteditor folder from version control.
--    (This is handled in .gitignore, outside MDL.)
```

### New MDL Statement: `CALL AGENT` Microflow Activity

Because "Call Agent" is a first-class Mendix toolbox activity (distinct from a generic Java action call), this proposal also introduces a corresponding MDL microflow statement:

```
CALL AGENT WITH HISTORY <agent> REQUEST <request> [CONTEXT <obj>] INTO $Response
CALL AGENT WITHOUT HISTORY <agent> [CONTEXT <obj>] [REQUEST <req>] [FILES <fc>] INTO $Response
NEW CHAT FOR AGENT <agent> ACTION MICROFLOW <microflow> [CONTEXT <obj>] [MODEL <dm>] INTO $ChatContext
```

These map directly to the `AgentCommons.Agent_Call_WithHistory`, `AgentCommons.Agent_Call_WithoutHistory`, and `AgentCommons.ChatContext_Create_ForAgent` Java actions (all exposed in the **"Agents Kit"** toolbox category). The MDL form exists so these show up as the actual "Call Agent" activity in Studio Pro rather than as opaque Java action calls.

---

### Example 1: Customer Support Agent with Tools

A conversational agent that helps customer support reps by looking up orders, checking shipment status, and drafting responses. Tools (microflows) are attached to the agent in the agent document — at runtime the "Call Agent" activity handles tool invocation automatically.

#### Step 1: Domain Model

```sql
-- Domain model for the support system

@Position(100, 100)
CREATE PERSISTENT ENTITY Support."Customer" (
  "Name": String(200) NOT NULL ERROR 'Name is required',
  "Email": String(200),
  "Phone": String(50),
  "AccountTier": Enumeration(Support.AccountTier)
);

@Position(350, 100)
CREATE PERSISTENT ENTITY Support."Order" (
  "OrderNumber": String(50) NOT NULL ERROR 'Order number is required',
  "OrderDate": DateTime,
  "TotalAmount": Decimal,
  "Status": Enumeration(Support.OrderStatus)
);

@Position(600, 100)
CREATE PERSISTENT ENTITY Support."SupportTicket" (
  "Subject": String(200),
  "Description": String(unlimited),
  "Priority": Enumeration(Support.TicketPriority),
  "Resolution": String(unlimited),
  "IsResolved": Boolean DEFAULT false
);

CREATE ASSOCIATION Support."Order_Customer"
  FROM Support."Order" TO Support."Customer";

CREATE ASSOCIATION Support."SupportTicket_Customer"
  FROM Support."SupportTicket" TO Support."Customer";

CREATE ASSOCIATION Support."SupportTicket_Order"
  FROM Support."SupportTicket" TO Support."Order";

CREATE ENUMERATION Support."OrderStatus" (
  Pending = 'Pending',
  Shipped = 'Shipped',
  Delivered = 'Delivered',
  Returned = 'Returned'
);

CREATE ENUMERATION Support."TicketPriority" (
  Low = 'Low',
  Medium = 'Medium',
  High = 'High',
  Urgent = 'Urgent'
);

CREATE ENUMERATION Support."AccountTier" (
  Standard = 'Standard',
  Premium = 'Premium',
  Enterprise = 'Enterprise'
);
```

#### Step 2: Tool Microflows

Tool microflows take the **Mendix data types that the LLM should fill in** as input parameters and must return a `String` (which becomes the tool result shown to the model). The Agent Editor infers the tool's JSON schema from the microflow signature — so parameter names and types are what the LLM sees.

```sql
/**
 * Tool microflow: Look up a customer by email address.
 * The Agent Editor will expose this as a tool with input parameter "Email".
 */
CREATE MICROFLOW Support."Tool_LookupCustomer" (
  $Email: String
)
RETURNS String
BEGIN
  RETRIEVE $Customer FROM DATABASE Support.Customer
    WHERE Email = $Email LIMIT 1;

  IF $Customer != empty THEN
    RETURN 'Customer: ' + $Customer/Name
      + ', Tier: ' + getKey($Customer/AccountTier)
      + ', Phone: ' + $Customer/Phone;
  ELSE
    RETURN 'No customer found with email: ' + $Email;
  END IF;
END;
/

/**
 * Tool microflow: Look up recent orders for a customer by name.
 */
CREATE MICROFLOW Support."Tool_GetOrders" (
  $CustomerName: String
)
RETURNS String
BEGIN
  RETRIEVE $Customer FROM DATABASE Support.Customer
    WHERE Name = $CustomerName LIMIT 1;

  IF $Customer = empty THEN
    RETURN 'Customer not found: ' + $CustomerName;
  END IF;

  RETRIEVE $OrderList FROM DATABASE Support.Order
    WHERE Support.Order_Customer = $Customer;

  DECLARE $Result String = '';
  LOOP $Order IN $OrderList
  BEGIN
    SET $Result = $Result + 'Order ' + $Order/OrderNumber
      + ' (' + formatDateTime($Order/OrderDate, 'yyyy-MM-dd') + ')'
      + ' - ' + formatDecimal($Order/TotalAmount, 2) + ' EUR'
      + ' - Status: ' + getKey($Order/Status) + '\n';
  END LOOP;

  RETURN if $Result = '' then 'No orders found' else $Result;
END;
/

/**
 * Tool microflow: Create a support ticket.
 * Multiple primitive parameters become structured input for the tool.
 */
CREATE MICROFLOW Support."Tool_CreateTicket" (
  $Subject: String,
  $Description: String,
  $Priority: ENUM Support.TicketPriority
)
RETURNS String
BEGIN
  $Ticket = CREATE Support.SupportTicket (
    Subject = $Subject,
    Description = $Description,
    Priority = $Priority,
    IsResolved = false
  );
  COMMIT $Ticket;

  RETURN 'Ticket created (ID: ' + toString($Ticket/System.id) + ').';
END;
/
```

#### Step 3: Agent Document

Tools, knowledge bases, and MCP servers are declared in the **agent document itself** — not attached at runtime in the action microflow. This is the key fix vs. the earlier draft.

```sql
-- The agent definition — stored as a CustomBlobDocument in the project
CREATE AGENT Support."CustomerSupportAgent" (
  UsageType: Conversational,
  Description: 'Customer support agent with lookup and ticketing tools',
  SystemPrompt: 'You are a helpful customer support agent for an e-commerce company.

Your capabilities:
- Look up customer information by email
- Check order history and shipment status
- Create support tickets for unresolved issues

Guidelines:
- Always verify the customer identity before sharing order details
- For Premium and Enterprise customers, prioritize their requests
- If you cannot resolve an issue, create a support ticket
- Be empathetic and professional in your responses'
)
{
  TOOL LookupCustomer {
    Microflow: Support.Tool_LookupCustomer,
    Description: 'Look up a customer by their email address',
    Access: VisibleForUser
  }

  TOOL GetOrders {
    Microflow: Support.Tool_GetOrders,
    Description: 'Get recent orders for a customer by name',
    Access: VisibleForUser
  }

  TOOL CreateTicket {
    Microflow: Support.Tool_CreateTicket,
    Description: 'Create a new support ticket with the given subject, description, and priority',
    Access: UserConfirmationRequired
  }
};
```

The `Access` property maps to `GenAICommons.ENUM_UserAccessApproval`:
- `HiddenForUser` — tool executes silently
- `VisibleForUser` — tool call is shown in the chat UI but executes automatically
- `UserConfirmationRequired` — tool call is shown and user must approve before execution

#### Step 4: Runtime Wiring — Action Microflow for Chat

With tools declared on the agent, the action microflow becomes a simple two-step: preprocess, then call the agent. The "Call Agent With History" activity handles tool invocation, knowledge base retrieval, and the LLM round-trips internally.

```sql
/**
 * Action microflow for the Customer Support chat.
 * Wired up by "New Chat for Agent" when the chat is created.
 *
 * @param $ChatContext The conversation context from the chat widget
 * @returns Boolean indicating success
 */
CREATE MICROFLOW Support."Chat_CustomerSupport" (
  $ChatContext: ConversationalUI.ChatContext
)
RETURNS Boolean
BEGIN
  -- 1. Default Preprocessing: extract user message, build Request with history
  $Request = CALL MICROFLOW ConversationalUI.ChatContext_Preprocessing(
    ChatContext = $ChatContext
  ) ON ERROR ROLLBACK;

  IF $Request = empty THEN
    RETURN false;
  END IF;

  -- 2. Retrieve the agent (created automatically from the agent document
  --    when ASU_AgentEditor runs at startup)
  RETRIEVE $Agent FROM DATABASE AgentCommons.Agent
    WHERE _QualifiedName = 'Support.CustomerSupportAgent' LIMIT 1;

  -- 3. Call Agent With History — single activity that:
  --    - Selects the in-use version + its deployed model
  --    - Wires in the agent's tools, knowledge bases, MCP servers
  --    - Calls Chat Completions
  --    - Handles tool-call round-trips
  CALL AGENT WITH HISTORY $Agent REQUEST $Request INTO $Response
    ON ERROR ROLLBACK;

  -- 4. Update the chat UI with the response (same as any ConversationalUI flow)
  IF $Response != empty AND $Response/GenAICommons.Response_Message != empty THEN
    $Message = CALL MICROFLOW ConversationalUI.ChatContext_UpdateAssistantResponse(
      ChatContext = $ChatContext,
      MessageStatus = ConversationalUI.ENUM_MessageStatus.Success,
      Response = $Response
    ) ON ERROR ROLLBACK;
    RETURN true;
  ELSE
    RETURN false;
  END IF;
END;
/
```

#### Step 5: Page with "Start Chat" Button

The chat page uses `New Chat for Agent` to create a `ChatContext` pre-configured with the agent's model, system prompt, and action microflow. This replaces the manual ProviderConfig wiring from the earlier draft.

```sql
/**
 * Microflow that opens a support chat. Called from a "Start Chat" button.
 * Uses "New Chat for Agent" to create a ChatContext configured with this agent.
 */
CREATE MICROFLOW Support."ACT_StartSupportChat" ()
BEGIN
  RETRIEVE $Agent FROM DATABASE AgentCommons.Agent
    WHERE _QualifiedName = 'Support.CustomerSupportAgent' LIMIT 1;

  NEW CHAT FOR AGENT $Agent
    ACTION MICROFLOW Support.Chat_CustomerSupport
    INTO $ChatContext
    ON ERROR ROLLBACK;

  SHOW PAGE Support.SupportChat($ChatContext = $ChatContext);
END;
/

/**
 * Customer support chat page.
 * Data source is the ChatContext passed from ACT_StartSupportChat.
 */
CREATE PAGE Support."SupportChat" (
  Title: 'Customer Support',
  Layout: Atlas_Core.Atlas_Default
) {
  HEADER h1 {
    DYNAMICTEXT title (Caption: 'AI Customer Support')
  }
  DATAVIEW chatView (DataSource: CONTEXT ConversationalUI.ChatContext) {
    -- The ConversationalUI chat snippet renders the conversation,
    -- send box, tool call approvals, and message history
    SNIPPETCALL chatWidget (Snippet: ConversationalUI.Snippet_Output_WithHistory)
  }
};
/
```

#### Step 6: Security

```sql
-- Module roles
CREATE MODULE ROLE Support."User";
CREATE MODULE ROLE Support."Admin";

-- Entity access
GRANT Support.User ON Support.Customer (READ *);
GRANT Support.User ON Support.Order (READ *);
GRANT Support.User ON Support.SupportTicket (CREATE, READ *, WRITE *);
GRANT Support.Admin ON Support.Customer (CREATE, DELETE, READ *, WRITE *);
GRANT Support.Admin ON Support.Order (CREATE, DELETE, READ *, WRITE *);
GRANT Support.Admin ON Support.SupportTicket (CREATE, DELETE, READ *, WRITE *);

-- Microflow access
GRANT EXECUTE ON MICROFLOW Support.ACT_StartSupportChat TO Support.User;
GRANT EXECUTE ON MICROFLOW Support.Chat_CustomerSupport TO Support.User;
-- Tool microflows must be callable because the agent invokes them
GRANT EXECUTE ON MICROFLOW Support.Tool_LookupCustomer TO Support.User;
GRANT EXECUTE ON MICROFLOW Support.Tool_GetOrders TO Support.User;
GRANT EXECUTE ON MICROFLOW Support.Tool_CreateTicket TO Support.User;

-- Page access
GRANT VIEW ON PAGE Support.SupportChat TO Support.User;
```

---

### Example 2: MCP-Powered Research Agent

An agent that connects to external MCP servers to access tools like web search, file reading, and database queries. The key insight: the consumed MCP service is a **document** (created in Studio Pro alongside the agent), and it's attached to the agent document directly — no runtime wiring needed.

#### Step 1: Domain Model

```sql
CREATE PERSISTENT ENTITY Research."ResearchProject" (
  "Title": String(200),
  "Objective": String(unlimited),
  "Status": Enumeration(Research.ProjectStatus),
  "Summary": String(unlimited)
);

CREATE ENUMERATION Research."ProjectStatus" (
  InProgress = 'In Progress',
  Completed = 'Completed',
  OnHold = 'On Hold'
);
```

#### Step 2: Credentials Microflow

Before defining the consumed MCP service, create a microflow that returns the HTTP headers needed to authenticate to the server. The microflow must take no parameters and return `List<System.HttpHeader>`.

```sql
/**
 * Returns HTTP headers used to authenticate to the research MCP server.
 * Referenced from the ConsumedMCPService document.
 */
CREATE MICROFLOW Research."MCP_GetCredentials" ()
RETURNS List of System.HttpHeader
BEGIN
  DECLARE $Headers List of System.HttpHeader = empty;
  $AuthHeader = CREATE System.HttpHeader (
    Key = 'Authorization',
    Value = 'Bearer ' + @Research.ResearchMCPToken
  );
  SET $Headers = $Headers + $AuthHeader;
  RETURN $Headers;
END;
/
```

#### Step 3: Consumed MCP Service Document

The ConsumedMCPService is a model document (like the agent itself), not a runtime-created entity. In this proposal's Phase 4 future extensions, we would also add MDL for it:

```sql
-- Proposed (future extension): declare a consumed MCP service as a document
CREATE CONSUMED MCP SERVICE Research."ResearchTools" (
  Endpoint: 'https://mcp.example.com/research',
  ProtocolVersion: v2025_03_26,
  GetCredentialsMicroflow: Research.MCP_GetCredentials,
  ConnectionTimeOutInSeconds: 30
);
```

At runtime, `ASU_AgentEditor` syncs this document into a `MCPClient.ConsumedMCPService` entity and discovers its tools.

#### Step 4: Agent with MCP Service Attached

The agent references the consumed MCP service by qualified name. At runtime, "Call Agent" automatically adds all (enabled) tools from the attached MCP server to the request.

```sql
CREATE AGENT Research."ResearchAssistant" (
  UsageType: Conversational,
  Description: 'Research assistant with web search and document analysis via MCP',
  Entity: Research.ResearchProject,
  Variables: ("Title": EntityAttribute, "Objective": EntityAttribute),
  SystemPrompt: 'You are a research assistant helping with project: {{Title}}.

Objective: {{Objective}}

Use the available tools to:
1. Search the web for relevant information
2. Read and analyze documents
3. Summarize findings

Always cite your sources. Present findings in a structured format.'
)
{
  MCP SERVICE Research.ResearchTools {
    Access: VisibleForUser
  }
};
```

#### Step 5: Action Microflow — Same Simple Pattern

Because tools and MCP services are declared on the agent document, the action microflow stays minimal. "Call Agent With History" takes care of wiring MCP tools into the request automatically.

```sql
/**
 * Action microflow for the research chat.
 * Because MCP services are attached to the agent document, no MCP-specific
 * code is needed here — "Call Agent" handles it.
 */
CREATE MICROFLOW Research."Chat_Research" (
  $ChatContext: ConversationalUI.ChatContext
)
RETURNS Boolean
BEGIN
  $Request = CALL MICROFLOW ConversationalUI.ChatContext_Preprocessing(
    ChatContext = $ChatContext
  ) ON ERROR ROLLBACK;

  IF $Request = empty THEN
    RETURN false;
  END IF;

  RETRIEVE $Agent FROM DATABASE AgentCommons.Agent
    WHERE _QualifiedName = 'Research.ResearchAssistant' LIMIT 1;

  -- Retrieve the context object passed from the page (ResearchProject).
  -- Variables "Title" and "Objective" are replaced from this object's
  -- attributes by Call Agent automatically.
  RETRIEVE $ProjectList FROM $ChatContext/ConversationalUI.ChatContext_Owner
    /System.User; -- simplified; real apps pass project via extension entity
  DECLARE $Project Research.ResearchProject;

  CALL AGENT WITH HISTORY $Agent REQUEST $Request CONTEXT $Project INTO $Response
    ON ERROR ROLLBACK;

  IF $Response != empty AND $Response/GenAICommons.Response_Message != empty THEN
    CALL MICROFLOW ConversationalUI.ChatContext_UpdateAssistantResponse(
      ChatContext = $ChatContext,
      MessageStatus = ConversationalUI.ENUM_MessageStatus.Success,
      Response = $Response
    ) ON ERROR ROLLBACK;
    RETURN true;
  ELSE
    RETURN false;
  END IF;
END;
/
```

---

### Example 3: Single-Call Agent for Data Processing

Not all agents need a chat interface. A `Task` (single-call) agent processes one request and returns a result — useful for batch operations, background processing, and microflow-embedded AI. The "Call Agent Without History" activity handles everything in one step.

#### Step 1: Domain Model

```sql
@Position(100, 100)
CREATE PERSISTENT ENTITY Reviews."ProductReview" (
  "ProductName": String(200),
  "ReviewText": String(unlimited),
  "Sentiment": String(50),
  "KeyThemes": String(unlimited),
  "IsProcessed": Boolean DEFAULT false
);
```

#### Step 2: Agent Document

```sql
CREATE AGENT Reviews."SentimentAnalyzer" (
  UsageType: Task,
  Description: 'Single-call agent that extracts sentiment and themes from a product review',
  Entity: Reviews.ProductReview,
  Variables: ("ProductName": EntityAttribute, "ReviewText": EntityAttribute),
  SystemPrompt: 'Analyze the following product review for {{ProductName}}.

Extract:
1. Overall sentiment (Positive, Negative, Neutral, Mixed)
2. Key themes mentioned (comma-separated)

Respond in this exact format:
Sentiment: <sentiment>
Themes: <theme1>, <theme2>, <theme3>',
  UserPrompt: '{{ReviewText}}'
);
```

#### Step 3: Processing Microflow — One Activity

The context object (`$Review`) carries the attribute values that replace `{{ProductName}}` and `{{ReviewText}}` in the prompts. "Call Agent Without History" resolves everything and returns the `Response` in a single activity.

```sql
/**
 * Process a single product review using the SentimentAnalyzer agent.
 * Called from a batch microflow, a button action, or a scheduled event.
 *
 * @param $Review The review to analyze — its attributes replace prompt variables
 */
CREATE MICROFLOW Reviews."ProcessReview" (
  $Review: Reviews.ProductReview
)
BEGIN
  RETRIEVE $Agent FROM DATABASE AgentCommons.Agent
    WHERE _QualifiedName = 'Reviews.SentimentAnalyzer' LIMIT 1;

  -- One activity: resolve version, deployed model, prompts, and call the LLM
  CALL AGENT WITHOUT HISTORY $Agent CONTEXT $Review INTO $Response
    ON ERROR ROLLBACK;

  IF $Response = empty OR $Response/GenAICommons.Response_Message = empty THEN
    LOG WARNING NODE 'Reviews' 'Sentiment analysis failed for review: '
      + toString($Review/System.id);
    RETURN;
  END IF;

  DECLARE $ResponseText String = CALL MICROFLOW
    GenAICommons.Response_GetModelResponseString(Response = $Response);

  CHANGE $Review (
    Sentiment = $ResponseText,
    IsProcessed = true
  );
  COMMIT $Review;
END;
/

/**
 * Batch process all unprocessed reviews.
 */
CREATE MICROFLOW Reviews."ProcessAllReviews" (
  $ReviewList: List of Reviews.ProductReview
)
BEGIN
  LOOP $Review IN $ReviewList
  BEGIN
    IF $Review/IsProcessed = false THEN
      CALL MICROFLOW Reviews.ProcessReview(Review = $Review) ON ERROR CONTINUE;
    END IF;
  END LOOP;
END;
/
```

---

### Example 4: Agent with User-Approved Tools

A finance agent where sensitive tool calls (approving expenses) require user confirmation. The user-confirmation flow is **configured declaratively on the agent's tools**, not built in the action microflow — ConversationalUI renders the approval dialog automatically.

#### Step 1: Tool Microflows

```sql
/**
 * Read-only tool: Look up pending expense reports.
 * Safe for auto-execution (VisibleForUser).
 */
CREATE MICROFLOW Finance."Tool_LookupExpenses" (
  $Department: String
)
RETURNS String
BEGIN
  RETRIEVE $ExpenseList FROM DATABASE Finance.ExpenseReport
    WHERE Department = $Department AND Status = Finance.ExpenseStatus.Pending;

  DECLARE $Result String = '';
  LOOP $Expense IN $ExpenseList
  BEGIN
    SET $Result = $Result + 'Expense #' + $Expense/ReportNumber
      + ' by ' + $Expense/SubmittedBy
      + ' - ' + formatDecimal($Expense/Amount, 2) + ' EUR: '
      + $Expense/Description + '\n';
  END LOOP;

  RETURN if $Result = '' then 'No pending expenses' else $Result;
END;
/

/**
 * Write tool: Approve an expense report.
 * Requires user confirmation before execution.
 */
CREATE MICROFLOW Finance."Tool_ApproveExpense" (
  $ReportNumber: String,
  $ApprovalNote: String
)
RETURNS String
BEGIN
  RETRIEVE $Expense FROM DATABASE Finance.ExpenseReport
    WHERE ReportNumber = $ReportNumber LIMIT 1;

  IF $Expense = empty THEN
    RETURN 'Expense report not found: ' + $ReportNumber;
  END IF;

  CHANGE $Expense (
    Status = Finance.ExpenseStatus.Approved,
    ApprovalNote = $ApprovalNote,
    ApprovedDate = [%CurrentDateTime%]
  );
  COMMIT $Expense;

  RETURN 'Expense ' + $ReportNumber + ' approved.';
END;
/
```

#### Step 2: Agent with Mixed Access Levels

The `ACCESS` modifier per tool controls what ConversationalUI does at runtime:
- `VisibleForUser` — tool call shown in chat, executes automatically
- `UserConfirmationRequired` — chat shows an approval dialog, user clicks Approve/Decline
- `HiddenForUser` — executes silently (use for internal/lookup tools)

```sql
CREATE AGENT Finance."ExpenseApprovalAgent" (
  UsageType: Conversational,
  Description: 'Review and approve expense reports with user confirmation for writes',
  SystemPrompt: 'You are a financial assistant that helps managers review and approve expense reports.

You have access to tools that can:
- Look up expense report details (auto-executes)
- Approve expense reports (requires user confirmation)

IMPORTANT: Always show the expense details before recommending approval.
Never approve expenses that exceed typical department limits without explicit user instruction.'
)
{
  TOOL LookupExpenses {
    Microflow: Finance.Tool_LookupExpenses,
    Description: 'List pending expense reports for a department',
    Access: VisibleForUser
  }

  TOOL ApproveExpense {
    Microflow: Finance.Tool_ApproveExpense,
    Description: 'Approve a specific expense report by report number',
    Access: UserConfirmationRequired
  }
};
```

#### Step 3: Action Microflow — Unchanged

Because tool approval is declared on the agent, the action microflow is identical to Example 1 — `Call Agent With History` handles tool-call round-trips and cooperates with ConversationalUI's approval widget automatically.

```sql
CREATE MICROFLOW Finance."Chat_ExpenseApproval" (
  $ChatContext: ConversationalUI.ChatContext
)
RETURNS Boolean
BEGIN
  $Request = CALL MICROFLOW ConversationalUI.ChatContext_Preprocessing(
    ChatContext = $ChatContext
  ) ON ERROR ROLLBACK;

  IF $Request = empty THEN RETURN false; END IF;

  RETRIEVE $Agent FROM DATABASE AgentCommons.Agent
    WHERE _QualifiedName = 'Finance.ExpenseApprovalAgent' LIMIT 1;

  CALL AGENT WITH HISTORY $Agent REQUEST $Request INTO $Response
    ON ERROR ROLLBACK;

  IF $Response != empty AND $Response/GenAICommons.Response_Message != empty THEN
    CALL MICROFLOW ConversationalUI.ChatContext_UpdateAssistantResponse(
      ChatContext = $ChatContext,
      MessageStatus = ConversationalUI.ENUM_MessageStatus.Success,
      Response = $Response
    ) ON ERROR ROLLBACK;
    RETURN true;
  ELSE
    RETURN false;
  END IF;
END;
/
```

---

### Example 5: Knowledge Base RAG Agent

An agent that uses a knowledge base (vector store) for Retrieval-Augmented Generation. As with tools and MCP services, the knowledge base is attached to the agent **document** — "Call Agent" performs retrieval automatically before invoking the LLM, and source references flow through to the chat UI.

#### Step 1: Knowledge Base Document

A knowledge base is a separate model document that references a Mendix Cloud GenAI Knowledge Base resource via its key. The underlying `GenAICommons.ConsumedKnowledgeBase` is auto-created by `ASU_AgentEditor` at startup from the document.

```sql
-- Proposed (future extension): declare a knowledge base as a document
CREATE KNOWLEDGE BASE HelpDesk."ProductDocsKB" (
  DisplayName: 'Product Documentation',
  Architecture: 'MxCloud',
  KeyConstant: HelpDesk.ProductDocsKBKey     -- String constant with the KB resource key
);
```

#### Step 2: Agent with Knowledge Base Attached

```sql
CREATE AGENT HelpDesk."ProductExpert" (
  UsageType: Conversational,
  Description: 'Answers product questions from the documentation knowledge base',
  SystemPrompt: 'You are a product expert for our software platform.

Answer questions using ONLY the information from the provided knowledge base context.
If the knowledge base does not contain relevant information, say so clearly.
Always include the source document reference in your answer.

Do not make up information that is not in the context.'
)
{
  KNOWLEDGE BASE HelpDesk.ProductDocsKB {
    Collection: 'product-documentation',
    MaxResults: 5,
    MinSimilarity: 0.7
  }
};
```

#### Step 3: Action Microflow — Identical to the Simple Pattern

Because the knowledge base is attached to the agent, RAG retrieval happens inside "Call Agent With History". Source references are automatically added to the `Response/Message`, and `ChatContext_UpdateAssistantResponse` already handles rendering them (it calls `Source_Create` internally).

```sql
CREATE MICROFLOW HelpDesk."Chat_ProductExpert" (
  $ChatContext: ConversationalUI.ChatContext
)
RETURNS Boolean
BEGIN
  $Request = CALL MICROFLOW ConversationalUI.ChatContext_Preprocessing(
    ChatContext = $ChatContext
  ) ON ERROR ROLLBACK;

  IF $Request = empty THEN RETURN false; END IF;

  RETRIEVE $Agent FROM DATABASE AgentCommons.Agent
    WHERE _QualifiedName = 'HelpDesk.ProductExpert' LIMIT 1;

  -- RAG retrieval happens inside "Call Agent With History" automatically
  CALL AGENT WITH HISTORY $Agent REQUEST $Request INTO $Response
    ON ERROR ROLLBACK;

  IF $Response != empty AND $Response/GenAICommons.Response_Message != empty THEN
    CALL MICROFLOW ConversationalUI.ChatContext_UpdateAssistantResponse(
      ChatContext = $ChatContext,
      MessageStatus = ConversationalUI.ENUM_MessageStatus.Success,
      Response = $Response
    ) ON ERROR ROLLBACK;
    RETURN true;
  ELSE
    RETURN false;
  END IF;
END;
/
```

Notice that Examples 1, 2, 4, and 5 all have **the same shape** for the action microflow — only the agent reference changes. This is the power of declarative agent documents: the capabilities (tools, KB, MCP) are metadata the "Call Agent" activity consumes, not code the developer writes.

---

### Example 6: Building an MCP Server in Mendix

Mendix apps can also act as MCP servers, exposing their microflows as tools that external AI systems (Claude, ChatGPT, or another Mendix app) can call. This is done via the **MCPServer** marketplace module — not a custom Published REST Service. The module provides `Create MCP Server` and `Add Tool` toolbox actions that handle the MCP protocol.

Each tool microflow must accept **primitives or an `MCPServer.Tool` object** as input and return either `String` or `TextContent`. The Agent Editor / MCP Server infers the JSON schema from the signature.

```sql
-- Tool microflow: The MCP server will expose this as a tool named "lookup_customer"
CREATE MICROFLOW Support."MCP_LookupCustomer" (
  $Email: String
)
RETURNS String
BEGIN
  RETRIEVE $Customer FROM DATABASE Support.Customer
    WHERE Email = $Email LIMIT 1;
  IF $Customer = empty THEN
    RETURN 'No customer found';
  END IF;
  RETURN 'Customer: ' + $Customer/Name + ', Tier: ' + getKey($Customer/AccountTier);
END;
/

CREATE MICROFLOW Support."MCP_GetOrderStatus" (
  $OrderNumber: String
)
RETURNS String
BEGIN
  RETRIEVE $Order FROM DATABASE Support.Order
    WHERE OrderNumber = $OrderNumber LIMIT 1;
  IF $Order = empty THEN
    RETURN 'Order not found: ' + $OrderNumber;
  END IF;
  RETURN 'Order ' + $Order/OrderNumber + ' status: ' + getKey($Order/Status);
END;
/

/**
 * Set up the MCP server at startup and register the tools.
 * Register this as (part of) the after-startup microflow.
 */
CREATE MICROFLOW Support."ASU_SetupMCPServer" ()
BEGIN
  -- 1. Create the MCP server instance (Mendix runtime listens for MCP requests)
  $Server = CALL JAVA ACTION MCPServer.CreateMCPServer(
    Name = 'CustomerSupportMCP',
    Version = '1.0',
    ProtocolVersion = MCPServer.ENUM_ProtocolVersion.v2025_03_26
  ) ON ERROR ROLLBACK;

  -- 2. Expose each tool microflow. The MCP Server module builds the JSON
  --    schema from each microflow's signature.
  CALL JAVA ACTION MCPServer.AddTool(
    Server = $Server,
    Name = 'lookup_customer',
    Description = 'Look up customer information by email address',
    Microflow = 'Support.MCP_LookupCustomer'
  ) ON ERROR ROLLBACK;

  CALL JAVA ACTION MCPServer.AddTool(
    Server = $Server,
    Name = 'get_order_status',
    Description = 'Get the status of an order by order number',
    Microflow = 'Support.MCP_GetOrderStatus'
  ) ON ERROR ROLLBACK;

  LOG INFO NODE 'MCP' 'MCP server started with 2 tools';
END;
/
```

External agents (a Claude Desktop client, another Mendix app using MCPClient, or any MCP-compliant system) can now connect to this server, discover the tools, and call them. The MCPServer module handles protocol framing, authentication, and dispatch to the appropriate microflow.

---

### Example 7: Full Smart App Script — IT Help Desk

This script creates a complete AI-powered IT help desk application in a single MDL file, showing all the correct pieces end-to-end.

```sql
-- =============================================================
-- IT Help Desk Smart App
-- A complete AI-powered help desk built with MDL
-- Prerequisites: Encryption module configured, model key in constant
-- =============================================================

-- 1. Module
CREATE MODULE ITHelp;

-- 2. Domain Model
@Position(100, 100)
CREATE PERSISTENT ENTITY ITHelp."Ticket" (
  "Subject": String(200) NOT NULL ERROR 'Subject is required',
  "Description": String(unlimited),
  "Category": Enumeration(ITHelp.Category),
  "Status": Enumeration(ITHelp.TicketStatus),
  "AssignedTo": String(200),
  "Resolution": String(unlimited)
);

@Position(100, 300)
CREATE PERSISTENT ENTITY ITHelp."KBArticle" (
  "Title": String(200),
  "Content": String(unlimited),
  "Category": Enumeration(ITHelp.Category),
  "ViewCount": Integer DEFAULT 0
);

CREATE ENUMERATION ITHelp."Category" (
  Network = 'Network',
  Hardware = 'Hardware',
  Software = 'Software',
  Access = 'Access & Permissions',
  Other = 'Other'
);

CREATE ENUMERATION ITHelp."TicketStatus" (
  New = 'New',
  InProgress = 'In Progress',
  WaitingOnUser = 'Waiting on User',
  Resolved = 'Resolved',
  Closed = 'Closed'
);

-- 3. Model Key Constant (set via environment or Configuration_Overview page)
CREATE CONSTANT ITHelp."ModelKey" (
  Type: String,
  DefaultValue: ''
);

-- 4. Tool microflows — signatures become the tool JSON schemas
CREATE MICROFLOW ITHelp."Tool_SearchKB" ($Query: String)
RETURNS String
BEGIN
  RETRIEVE $Articles FROM DATABASE ITHelp.KBArticle
    WHERE contains(Title, $Query) OR contains(Content, $Query);

  DECLARE $Result String = '';
  LOOP $Article IN $Articles
  BEGIN
    SET $Result = $Result + '## ' + $Article/Title + '\n'
      + $Article/Content + '\n\n';
  END LOOP;

  RETURN if $Result = '' then 'No articles found for: ' + $Query else $Result;
END;
/

CREATE MICROFLOW ITHelp."Tool_CreateTicket" (
  $Subject: String,
  $Description: String,
  $Category: ENUM ITHelp.Category
)
RETURNS String
BEGIN
  $Ticket = CREATE ITHelp.Ticket (
    Subject = $Subject,
    Description = $Description,
    Category = $Category,
    Status = ITHelp.TicketStatus.New
  );
  COMMIT $Ticket;
  RETURN 'Ticket ' + toString($Ticket/System.id) + ' created.';
END;
/

CREATE MICROFLOW ITHelp."Tool_GetTicketStatus" ($TicketId: String)
RETURNS String
BEGIN
  RETRIEVE $Ticket FROM DATABASE ITHelp.Ticket WHERE System.id = $TicketId LIMIT 1;
  IF $Ticket = empty THEN RETURN 'Ticket not found'; END IF;
  RETURN 'Status: ' + getKey($Ticket/Status)
    + ', Assigned to: ' + $Ticket/AssignedTo;
END;
/

-- 5. Agent document — tools declared here, not at runtime
CREATE AGENT ITHelp."ITSupportAgent" (
  UsageType: Conversational,
  Description: 'AI-powered first-line IT support',
  SystemPrompt: 'You are an IT support agent for a corporate help desk.

Capabilities (use these tools):
1. Search the knowledge base for solutions to common problems
2. Create support tickets when issues need escalation
3. Check the status of existing tickets

Always try the knowledge base first before creating a ticket.
Be patient and ask clarifying questions when the issue is unclear.
For password resets and access requests, always create a ticket.'
)
{
  TOOL SearchKB {
    Microflow: ITHelp.Tool_SearchKB,
    Description: 'Search the knowledge base for articles matching a query',
    Access: VisibleForUser
  }

  TOOL CreateTicket {
    Microflow: ITHelp.Tool_CreateTicket,
    Description: 'Create a new support ticket with subject, description, and category',
    Access: UserConfirmationRequired
  }

  TOOL GetTicketStatus {
    Microflow: ITHelp.Tool_GetTicketStatus,
    Description: 'Get current status of an existing support ticket by ID',
    Access: VisibleForUser
  }
};

-- 6. Chat action microflow — uniform pattern with Call Agent
CREATE MICROFLOW ITHelp."Chat_ITSupport" (
  $ChatContext: ConversationalUI.ChatContext
)
RETURNS Boolean
BEGIN
  $Request = CALL MICROFLOW ConversationalUI.ChatContext_Preprocessing(
    ChatContext = $ChatContext
  ) ON ERROR ROLLBACK;

  IF $Request = empty THEN RETURN false; END IF;

  RETRIEVE $Agent FROM DATABASE AgentCommons.Agent
    WHERE _QualifiedName = 'ITHelp.ITSupportAgent' LIMIT 1;

  CALL AGENT WITH HISTORY $Agent REQUEST $Request INTO $Response
    ON ERROR ROLLBACK;

  IF $Response != empty AND $Response/GenAICommons.Response_Message != empty THEN
    CALL MICROFLOW ConversationalUI.ChatContext_UpdateAssistantResponse(
      ChatContext = $ChatContext,
      MessageStatus = ConversationalUI.ENUM_MessageStatus.Success,
      Response = $Response
    ) ON ERROR ROLLBACK;
    RETURN true;
  ELSE
    RETURN false;
  END IF;
END;
/

-- 7. Entry-point microflow uses "New Chat for Agent"
CREATE MICROFLOW ITHelp."ACT_StartHelpChat" ()
BEGIN
  RETRIEVE $Agent FROM DATABASE AgentCommons.Agent
    WHERE _QualifiedName = 'ITHelp.ITSupportAgent' LIMIT 1;

  NEW CHAT FOR AGENT $Agent
    ACTION MICROFLOW ITHelp.Chat_ITSupport
    INTO $ChatContext
    ON ERROR ROLLBACK;

  SHOW PAGE ITHelp.HelpDesk($ChatContext = $ChatContext);
END;
/

-- 8. Pages
CREATE PAGE ITHelp."Home" (
  Title: 'IT Help Desk',
  Layout: Atlas_Core.Atlas_Default
) {
  HEADER h1 { DYNAMICTEXT t (Caption: 'IT Help Desk') }
  CONTAINER c {
    ACTIONBUTTON startChat (
      Caption: 'Start Chat with IT Support',
      Action: MICROFLOW ITHelp.ACT_StartHelpChat()
    )
  }
};
/

CREATE PAGE ITHelp."HelpDesk" (
  Title: 'IT Support Chat',
  Layout: Atlas_Core.Atlas_Default
) {
  HEADER h1 { DYNAMICTEXT t (Caption: 'IT Support') }
  DATAVIEW chatDv (DataSource: CONTEXT ConversationalUI.ChatContext) {
    SNIPPETCALL chat (Snippet: ConversationalUI.Snippet_Output_WithHistory)
  }
};
/

CREATE PAGE ITHelp."TicketOverview" (
  Title: 'Support Tickets',
  Layout: Atlas_Core.Atlas_Default
) {
  HEADER h1 { DYNAMICTEXT t (Caption: 'Support Tickets') }
  DATAGRID ticketGrid (DataSource: DATABASE ITHelp.Ticket) {
    COLUMN col1 (Attribute: Subject, Caption: 'Subject')
    COLUMN col2 (Attribute: Category, Caption: 'Category')
    COLUMN col3 (Attribute: Status, Caption: 'Status')
    COLUMN col4 (Attribute: AssignedTo, Caption: 'Assigned To')
  }
};
/

-- 9. Security
CREATE MODULE ROLE ITHelp."User";
CREATE MODULE ROLE ITHelp."Admin";

GRANT ITHelp.User ON ITHelp.Ticket (CREATE, READ *, WRITE (ITHelp.Ticket.Description));
GRANT ITHelp.User ON ITHelp.KBArticle (READ *);
GRANT ITHelp.Admin ON ITHelp.Ticket (CREATE, DELETE, READ *, WRITE *);
GRANT ITHelp.Admin ON ITHelp.KBArticle (CREATE, DELETE, READ *, WRITE *);

GRANT EXECUTE ON MICROFLOW ITHelp.ACT_StartHelpChat TO ITHelp.User;
GRANT EXECUTE ON MICROFLOW ITHelp.Chat_ITSupport TO ITHelp.User;
GRANT EXECUTE ON MICROFLOW ITHelp.Tool_SearchKB TO ITHelp.User;
GRANT EXECUTE ON MICROFLOW ITHelp.Tool_CreateTicket TO ITHelp.User;
GRANT EXECUTE ON MICROFLOW ITHelp.Tool_GetTicketStatus TO ITHelp.User;
GRANT VIEW ON PAGE ITHelp.Home TO ITHelp.User;
GRANT VIEW ON PAGE ITHelp.HelpDesk TO ITHelp.User;
GRANT VIEW ON PAGE ITHelp.TicketOverview TO ITHelp.User, ITHelp.Admin;

-- 10. After-startup microflow registration
ALTER SETTINGS MODEL AfterStartupMicroflow = AgentEditorCommons.ASU_AgentEditor;
-- (Add custom setup to a composite microflow if needed.)

-- 11. Navigation
CREATE OR REPLACE NAVIGATION Responsive_web
  HOME PAGE ITHelp.Home FOR ITHelp.User
  MENU (
    ITEM 'Help Desk' PAGE ITHelp.Home,
    ITEM 'Tickets' PAGE ITHelp.TicketOverview,
    ITEM 'Agent Admin' PAGE AgentCommons.Agent_Overview
  );
```

---

### Summary: What MDL Agent Support Enables

| Capability | Without MDL Agent Support | With MDL Agent Support |
|------------|---------------------------|------------------------|
| **Discover agents** | Open Studio Pro, navigate to Agent Editor | `SHOW AGENTS` in CLI or script |
| **Inspect agent prompts** | Click through Agent Editor UI | `DESCRIBE AGENT Module.Name` |
| **Create agents** | Only via Studio Pro Agent Editor | `CREATE AGENT` in MDL scripts |
| **Version control** | Binary CustomBlobDocument diffs | Human-readable MDL diffs |
| **AI-assisted development** | AI cannot see or create agents | AI generates complete smart apps |
| **Batch operations** | Manual, one agent at a time | Script creates multiple agents |
| **Code review** | Cannot review agent changes in PR | MDL changes are reviewable text |
| **Migration** | Manual recreation in new project | Copy/paste MDL scripts |
| **Documentation** | Screenshots of Agent Editor | `DESCRIBE AGENT` produces docs |
| **Testing** | Manual testing in Studio Pro | Scriptable test cases with mxcli |

The combination of `CREATE AGENT` (document definition), tool microflows (business logic), MCP connections (external tools), knowledge bases (RAG), and ConversationalUI (chat interface) means an AI coding agent can scaffold an entire smart app from a natural-language description — creating all layers from domain model to navigation in a single MDL session.

## Open Questions

1. **CustomBlobDocument extensibility** *(answered)*: Mendix uses `CustomBlobDocument` as a general extension pattern. Four `CustomDocumentType` values observed so far: `agenteditor.agent`, `agenteditor.model`, `agenteditor.knowledgebase`, `agenteditor.consumedMCPService`. The parser dispatches by `CustomDocumentType` rather than hardcoding agent-specific logic. Future types (other extensions, other agent-editor documents) plug in naturally.

2. **Contents JSON schema for tools/KB/MCP** *(answered for MCP tools and KB tools, still open for microflow tools)*: The `Agents.Agent007` document in the test3 project gave us the schema for MCP-type tools and knowledge base tools (see BSON Structure section). **Still open**: the JSON shape for a microflow-type tool — none observed yet. Expected: `toolType: "Microflow"` plus a `microflow: { qualifiedName, microflowId }` reference, but the exact key names and nesting need a real sample. Implementation should capture one before finalizing the microflow-tool writer.

3. **Separate document types for Model, Knowledge Base, and MCP Service** *(answered)*: Confirmed. Phase 4 of the implementation plan covers `CREATE MODEL`, `CREATE KNOWLEDGE BASE`, `CREATE CONSUMED MCP SERVICE` with schemas matching the observed BSON.

4. **`CALL AGENT` activity BSON format**: The proposed `CALL AGENT WITH HISTORY` / `CALL AGENT WITHOUT HISTORY` / `NEW CHAT FOR AGENT` MDL statements need to map to a Studio Pro microflow activity. Is this a dedicated activity type in BSON, or does Studio Pro render a generic `JavaActionCallAction` (pointing at `AgentCommons.Agent_Call_WithHistory`) as "Call Agent"? If it's the latter, MDL can emit a standard Java action call; if the former, we need to identify the new activity BSON `$Type`. **Action:** inspect a Studio Pro microflow that uses "Call Agent" to resolve.

5. **ASU_AgentEditor behavior**: `AgentEditorCommons.ASU_AgentEditor` is the after-startup microflow that syncs agent documents to runtime `AgentCommons.Agent` entities. Does `CREATE AGENT` via MDL need to trigger this sync, or does it happen automatically on next app startup? What happens if MDL creates an agent and the app is already running?

6. **Module placement** *(partially answered)*: The new documents in test3 live in a user-created `Agents` module (not in `AgentEditorCommons`), confirming that users **can** place agent-editor documents in their own modules. The older 4 agents in `AgentEditorCommons` appear to be samples shipped with the marketplace module.

7. **Cross-document UUID stability**: Agent documents reference model/KB/MCP documents by both `qualifiedName` AND `documentId` (UUID). When MDL creates a document, the generated UUID must be stable so that subsequent `CREATE AGENT` statements can correctly fill the `documentId` field. If an `ALTER` or re-create changes the UUID, all referring agents break. The writer must either (a) preserve existing UUIDs on update or (b) allow the agent's `documentId` field to be left empty and resolved at app-startup time by qualified name.

8. **Portal-populated fields on Model/KB**: Fields like `displayName`, `keyId`, `keyName`, `environment`, `resourceName`, `modelName`, `modelDisplayName` are populated by Studio Pro after the user clicks "Test Key". Should MDL `CREATE MODEL` write them as empty strings (letting Studio Pro fill them on next open), preserve them if provided by `DESCRIBE` round-trip, or outright reject user-supplied values? Current proposal: accept-and-round-trip but document them as read-only.

7. **Non-Mendix-Cloud model providers (e.g., OpenRouter)**: The [Agent Editor docs](https://docs.mendix.com/appstore/modules/genai/genai-for-mx/agent-editor/) state that model documents require "a String constant that contains the key for a **Text Generation resource**... obtained in the **Mendix Cloud GenAI Portal**" — so the model document format is currently locked to Mendix Cloud GenAI. Meanwhile, `GenAICommons.DeployedModel` is provider-agnostic (it's just `DisplayName` + `Architecture` + a `Microflow` pointer), and marketplace connectors exist for OpenAI, Amazon Bedrock, Google Gemini, and Mistral. This creates a split:
   - Users who want OpenAI-compatible endpoints like **OpenRouter** (including its free models: `google/gemini-flash-1.5-8b:free`, `mistralai/mistral-7b-instruct:free`, etc.) cannot use the Agent Editor's model documents today. Workarounds: (a) reconfigure the OpenAI Connector's base URL to OpenRouter; (b) build a custom microflow-based `DeployedModel`; (c) skip the Agent Editor and create `AgentCommons.Agent` / `Version` entities at runtime instead.
   - Option (c) means losing the design-time benefits of agent documents (MDL support, version control in the project, LLM-friendly static configuration). `CREATE AGENT` in MDL therefore won't help these users until Mendix opens the model document format to other providers.
   - **Implications for this proposal**: The proposed `CREATE MODEL` document (Phase 4) should not hard-code `Architecture: 'MxCloud'`. If/when Mendix supports third-party architectures in model documents, the `CREATE MODEL` syntax must accept `Architecture: 'OpenAI' | 'OpenRouter' | 'Bedrock' | ...` and a connector-specific configuration block. The `CREATE AGENT` body is already model-provider-agnostic (it references a model document by name, not by architecture), so no changes needed there.
   - **Track this externally**: Monitor Mendix release notes for the Agent Editor opening to additional providers. If that happens, the MDL grammar already has room for it — we'd just add more valid `Architecture` values to `CREATE MODEL`.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Microflow-tool JSON shape in `Contents.tools[]` still unobserved | Medium | Medium | Observed MCP-tool and KB-tool shapes; capture a microflow-tool sample before finalizing that writer path |
| `CALL AGENT` activity is a new BSON `$Type` | Medium | Medium | Inspect a Studio Pro microflow that uses "Call Agent" before implementing |
| Cross-document UUIDs become stale when documents are re-created | High | High | Preserve UUIDs on update; validate referring agents on `CREATE`/`DROP` of a referenced document |
| Contents JSON schema changes in future Mendix versions | Medium | Medium | Parse tolerantly (ignore unknown fields), version-gate new fields |
| CustomBlobDocument format changes | Low | High | Monitor Mendix release notes, BSON schema comparison |
| Studio Pro fails to open MDL-created documents | Medium | High | Test with `mx check` and Studio Pro after creation; compare BSON byte-for-byte with editor-created documents (`Agents.MyFirstModel` etc.) |
| Portal-populated fields overwritten by MDL round-trip | Medium | Medium | On `CREATE MODEL` / `CREATE KNOWLEDGE BASE`, preserve any existing Portal fields if the document already exists; write empty strings only on fresh creates |
| Prerequisites (Encryption, ASU_AgentEditor) not set up before CREATE AGENT | Medium | Medium | MDL `CREATE AGENT` should warn/pre-check that prerequisites are configured |
| Agent document + matching Model/KB/MCP documents out of sync | Medium | Medium | `mxcli check` should validate cross-document references when `--references` is passed |
| Users want third-party LLM providers (OpenRouter, custom OpenAI-compatible) but Agent Editor model documents are Mendix-Cloud-only | High | Low (out of scope) | Document the workarounds (reconfigure OpenAI connector, custom microflow DeployedModel, skip agent documents); keep `CREATE MODEL` syntax open to future `Provider` values |

## References

- Test project: `mx-test-projects/test3-app/test3.mpr` (Mendix 11.9.0)
- **Older agent documents (AgentEditorCommons module)**: `InformationExtractorAgent`, `ProductDescription`, `SummarizationAgent`, `TranslationAgent` — `Excluded: true`, no model reference, no tools/KB
- **New Agent Editor sample documents (Agents module)**:
  - `Agents.Agent007` — fully populated agent with model, MCP tool, KB tool (`mprcontents/e0/72/e072318a-...mxunit`)
  - `Agents.MyFirstModel` — Model document, provider `MxCloudGenAI` (`mprcontents/3a/dd/3addaaa1-...mxunit`)
  - `Agents.Knowledge_base` — Knowledge Base document (`mprcontents/cc/cc/cccc0b5b-...mxunit`)
  - `Agents.Consumed_MCP_service` — Consumed MCP Service document (`mprcontents/47/c9/47c9987a-...mxunit`)
- Agent Editor extension manifest: `.mendix-cache/modules/agenteditor.mxmodule/extensions/agent-editor/manifest.json`
- AgentCommons module: Marketplace v3.1.0 (31 entities, 226 microflows)
- AgentEditorCommons module: Marketplace v1.0.0 (9 entities, 32 microflows, including `ASU_AgentEditor`)
- MCPClient module: Marketplace v3.0.1 (20 entities, 35 microflows)
- GenAICommons module: Marketplace v6.1.0 (34 entities, 112 microflows)
- ConversationalUI module: Marketplace v6.1.0 (17 entities, 152 microflows)
- [Mendix Agent Editor documentation](https://docs.mendix.com/appstore/modules/genai/genai-for-mx/agent-editor/)
