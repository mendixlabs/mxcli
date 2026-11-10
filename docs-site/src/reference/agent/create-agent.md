# CREATE AGENT

## Synopsis

```sql
CREATE [ OR MODIFY ] AGENT module.Name (
    UsageType: { Task | Conversational },
    model: module.ModelName,
    SelectedModel: 'model id',
    SystemPrompt: 'prompt' | $$ multi-line prompt $$,
    UserPrompt: 'prompt' | $$ multi-line prompt $$
    [, description: 'description' ]
    [, variables: ( "Key": EntityAttribute [, ...] ) ]
    [, MaxTokens: 16384 ]
    [, Temperature: 0.7 ]
    [, ToolChoice: { Auto | None | Required } ]
)
[ {
    [ MCP SERVICE module.McpServiceName (
        Enabled: true
    ) ]
    [ KNOWLEDGE BASE alias (
        source: module.KBName,
        collection: 'collection-name',
        MaxResults: 5,
        description: 'description',
        Enabled: true
    ) ]
} ]
;

DROP AGENT module.Name;
```

Requires Mendix 11.9+.

## Description

Creates an agent document in the Mendix Agent Editor. An agent defines how an AI assistant behaves: which model it uses, what instructions it follows, and which tools and knowledge bases it can access.

### Usage Types

| Value | Description |
|-------|-------------|
| `Task` | Single-turn task completion |
| `Conversational` | Multi-turn conversational agent |

### Prompts

Both `SystemPrompt` and `UserPrompt` accept single-quoted strings or dollar-quoted (`$$...$$`) multi-line strings. Dollar-quoted strings preserve line breaks and are useful for structured prompts.

### Variables

The `variables` clause declares named template variables that can be referenced in prompts using `{{VariableName}}` syntax. Each variable maps a name to a Mendix type (e.g., `EntityAttribute`).

```sql
variables: ("Language": EntityAttribute)
SystemPrompt: 'Translate the text into {{Language}}.'
```

### Body Blocks

The optional body block (inside `{ }`) attaches tools and knowledge bases to the agent:

- **MCP SERVICE** — attaches a consumed MCP service by its qualified name
- **KNOWLEDGE BASE** — attaches a knowledge base with retrieval settings
- **TOOL** — attaches a microflow tool; `Document: Module.Microflow` is the microflow it runs

Each attachment is a child of the agent, so its properties are in `( )`. The older brace form (`TOOL Name { … }`) still parses but warns (MDL-DEPR071); `mxcli fmt --upgrade` rewrites it.

### OR MODIFY

If `OR MODIFY` is specified and the agent already exists, its properties and body blocks are updated in place. The document UUID is preserved, and so is each tool's and knowledge base's identity: a tool keeps its stored entry when its type and name match (an MCP service when the service matches). Settings that MDL cannot spell keep their stored values, both on `OR MODIFY` and on `ALTER AGENT`: a tool's user approval and display title and description, the agent's streaming switches and selected model, and a knowledge base tool's minimum similarity.

## Parameters

`module.Name`
:   The qualified name of the agent document.

`UsageType`
:   Whether the agent is `Task` (single-turn) or `Conversational` (multi-turn).

`model: module.ModelName`
:   Qualified reference to the model document the agent uses.

`SelectedModel`
:   The id of the model the agent uses, one the Model document's resource offers (e.g. `'eu.anthropic.claude-sonnet-5'`). Required in practice from Agent Editor 2.2: its import refuses an agent without one ("Model with name  is referenced but could not be found for the Agent"), and that refusal stops the import of every agent document at startup. `mxcli check` warns on a `create agent` without it (MDL-AGENT02). Only the id is read at runtime; it is matched against `GenAICommons.DeployedModel.Model`. To list the ids, with the app running: `mxcli oql "select Model from MxGenAIConnector.MxCloudDeployedModel"` (reported to work in ako/mxcli#2031). `''` clears it.

`SelectedModelName`
:   Optional. The display name Studio Pro stores next to the id (e.g. `'Anthropic Claude Sonnet 5'`); nothing at runtime reads it. Defaults to the id, or keeps the stored name while the id is unchanged.

`SystemPrompt`
:   Instructions for the AI model's behavior and persona.

`UserPrompt`
:   Default user message or prompt template.

`description`
:   Optional human-readable description of the agent's purpose.

`variables`
:   Optional template variable declarations. Keys are quoted strings; values are Mendix type keywords.

`MaxTokens`
:   Optional maximum number of tokens in the response.

`Temperature`
:   Optional sampling temperature (0.0–1.0). Higher values produce more creative output.

`ToolChoice`
:   Optional. `Auto` (let the model decide), `None` (no tools), or `Required` (always use a tool).

`EnableBackendStreaming`, `EnableUIStreaming`
:   Optional, `true` or `false`. Response streaming for the agent (Agent Editor 2.2.1+). Streaming is set on the agent, not on its model. The Studio Pro playground does not show it; it works at runtime with a chat agent in Conversational UI.

### Tool and MCP service settings

`UserAccessApproval`
:   Optional. `HiddenForUser`: the call runs without being shown. `VisibleForUser`: the call is shown and runs. `UserConfirmationRequired`: the call is shown and waits for the user's approval. Agent Editor 2.2+.

`DisplayTitle`, `DisplayDescription`
:   Optional. The title and description shown to the user for the call. The model still sees the tool's name and `Description`.

### Knowledge base settings

`MinSimilarity`
:   Optional. The least similarity, between 0 and 1, a chunk needs to be returned.

`IsVisible`
:   Optional, `true` or `false`. Whether the user sees the retrieval.

On `OR MODIFY`, a setting the statement leaves out keeps its stored value.

## Examples

### Simple task agent

```sql
CREATE AGENT MyModule."Summarizer" (
    UsageType: Task,
    model: MyModule.GPT4Model,
    SelectedModel: 'eu.anthropic.claude-sonnet-5',
    SystemPrompt: 'Summarize the given text in 3 sentences.',
    UserPrompt: 'Enter text to summarize.'
);

```

### Agent with variable substitution

```sql
CREATE AGENT MyModule."Translator" (
    UsageType: Task,
    model: MyModule.GPT4Model,
    SelectedModel: 'eu.anthropic.claude-sonnet-5',
    variables: ("Language": EntityAttribute),
    SystemPrompt: 'Translate the text into {{Language}}.',
    UserPrompt: 'Hello world'
);

```

### Agent whose tool needs the user's approval

```sql
CREATE AGENT MyModule."Support" (
    UsageType: Chat,
    model: MyModule.GPT4Model,
    SelectedModel: 'eu.anthropic.claude-sonnet-5',
    EnableBackendStreaming: true,
    SystemPrompt: 'Help the customer with their orders.'
)
{
    TOOL CancelOrder (
        Document: MyModule.Tool_CancelOrder,
        Description: 'Cancel an order by its number',
        UserAccessApproval: UserConfirmationRequired,
        DisplayTitle: 'Cancel order'
    )
    KNOWLEDGE BASE Faq (Source: MyModule.FaqKB, MaxResults: 5, MinSimilarity: 0.3)
};
```

### Agent with multi-line dollar-quoted prompt

```sql
CREATE AGENT MyModule."CodeReviewer" (
    UsageType: Task,
    model: MyModule.GPT4Model,
    SelectedModel: 'eu.anthropic.claude-sonnet-5',
    SystemPrompt: $$You are a code review assistant.

Review the provided code for:
1. Security vulnerabilities
2. Performance issues
3. Code style and readability

Provide specific, actionable feedback.$$,
    UserPrompt: $$func main() {
    fmt.Println("Hello, World!")
}$$
);

```

### Conversational agent with MCP service and knowledge base

```sql
CREATE AGENT MyModule."ResearchAssistant" (
    UsageType: Conversational,
    description: 'Research assistant with web search and documentation',
    model: MyModule.GPT4Model,
    SelectedModel: 'eu.anthropic.claude-sonnet-5',
    MaxTokens: 16384,
    ToolChoice: Auto,
    SystemPrompt: 'You are a research assistant. Use available tools.',
    UserPrompt: 'Find information about quantum computing.'
)
{
    MCP SERVICE MyModule.WebSearch (
        Enabled: true
    )

    KNOWLEDGE BASE ProductKB (
        source: MyModule.ProductDocs,
        collection: 'product-docs',
        MaxResults: 5,
        description: 'Product documentation',
        Enabled: true
    )
};

```

### Idempotent upsert

```sql
CREATE OR MODIFY AGENT MyModule."ResearchAssistant" (
    UsageType: Task,
    model: MyModule.GPT4Model,
    MaxTokens: 8192,
    Temperature: 0.5,
    SystemPrompt: $$You are an updated research assistant.$$,
    UserPrompt: 'How can I help you today?'
);

```

## Notes

- Agents must be dropped before their referenced models, knowledge bases, and MCP services.
- The model and knowledge base documents must exist before the agent is created.
- Dollar-quoted prompts (`$$...$$`) preserve all whitespace including newlines.

## See Also

[CREATE AI MODEL](create-model.md), [CREATE KNOWLEDGE BASE](create-knowledge-base.md), [CREATE CONSUMED MCP SERVICE](create-consumed-mcp-service.md)
