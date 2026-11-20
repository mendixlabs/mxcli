# Agent Editor Statements

Statements for managing AI agent editor documents. Requires Mendix 11.9+ and the `AgentEditorCommons` module.

The Mendix Agent Editor introduces four document types that must be set up in dependency order:

1. **Model** — an LLM configuration referencing a Mendix Cloud GenAI Portal resource key
2. **Knowledge Base** — a vector knowledge base for retrieval-augmented generation
3. **Consumed MCP Service** — an external tool server (Model Context Protocol)
4. **Agent** — an AI agent that references a model and optionally uses knowledge bases and MCP services

A fifth, the **Published MCP Service** (Agent Editor 2.6+, MCP Server 5.x), is the other direction: an MCP server the app publishes, exposing its microflows as tools.

## Statements

| Statement | Description |
|-----------|-------------|
| [CREATE AI MODEL](create-model.md) | Define an LLM model configuration |
| [CREATE KNOWLEDGE BASE](create-knowledge-base.md) | Define a vector knowledge base |
| [CREATE CONSUMED MCP SERVICE](create-consumed-mcp-service.md) | Register an external MCP tool server |
| [CREATE PUBLISHED MCP SERVICE](create-published-mcp-service.md) | Publish the app's microflows as an MCP server |
| [CREATE AGENT](create-agent.md) | Define an AI agent with prompts and optional tools |

## Related Statements

| Statement | Syntax |
|-----------|--------|
| List AI models | `LIST AI MODELS [IN module]` |
| List knowledge bases | `LIST KNOWLEDGE BASES [IN module]` |
| List consumed MCP services | `LIST CONSUMED MCP SERVICES [IN module]` |
| List published MCP services | `LIST PUBLISHED MCP SERVICES [IN module]` |
| List agents | `LIST AGENTS [IN module]` |
| Describe | `DESCRIBE { MODEL \| KNOWLEDGE BASE \| CONSUMED MCP SERVICE \| PUBLISHED MCP SERVICE \| AGENT } module.Name` |

## Drop Statements

```sql
mdl 1;
DROP AI MODEL module.Name;
DROP KNOWLEDGE BASE module.Name;
DROP CONSUMED MCP SERVICE module.Name;
DROP PUBLISHED MCP SERVICE module.Name;
DROP AGENT module.Name;
```

## Prerequisites

- Mendix 11.9+
- `AgentEditorCommons` module installed
- Encryption module configured (32-character key)
- `ASU_AgentEditor` registered as after-startup microflow

## Version Gate

All agent editor statements are version-gated to Mendix 11.9+. Running them against an older project produces:

```
feature not available: agent editor requires Mendix >= 11.9
```
