# Agent, Model, Knowledge Base & Consumed MCP Service Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/386)

## Setup

> See [AGENT-TESTING.md](./AGENT-TESTING.md) for build, execution methods, and verification patterns.

---

## 1. SHOW AGENTS

### 1.1 List all agents

```
show agents;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Usage | Model | Tools | KBs`. Summary line `(N agents)`. Sorted alphabetically.

### 1.2 List agents in a module

```
show agents in MyModule;
```

**Expected:** Only agents from `MyModule`. Same column format.

### 1.3 Empty module

```
show agents in NonExistentModule;
```

**Expected:** Error or empty result with `(0 agents)`.

---

## 2. DESCRIBE AGENT

### 2.1 Agent with all block types

Find or create an agent with tools, MCP services, and knowledge bases. Verify all three block types appear in `describe` output.

### 2.2 Dollar-quoted prompts

Verify `SystemPrompt` and `UserPrompt` use `$$...$$` quoting. Prompts with single quotes, newlines, and special characters must survive roundtrip.

### 2.3 Non-existent agent

```
describe agent Fake.Missing;
```

**Expected:** Error — agent not found.

---

## 3. CREATE AGENT

### 3.1 Minimal agent

```
create agent MyModule.SimpleAgent (
  UsageType: chat,
  Model: MyModule.GPT4o,
  SystemPrompt: $$Hello.$$
);
```

**Expected:** Agent created. `show agents` lists it. `describe` matches input.

### 3.2 Auto-create module

```
create agent NewModule.Agent1 (
  UsageType: chat,
  Model: MyModule.GPT4o,
  SystemPrompt: $$Test.$$
);
```

**Expected:** `NewModule` auto-created if it does not exist. Agent created inside it.

### 3.3 Model reference resolution

```
create agent MyModule.RefTest (
  UsageType: chat,
  Model: OtherModule.SomeModel,
  SystemPrompt: $$Test.$$
);
```

**Expected:** Resolves `OtherModule.SomeModel` to existing model document. Error if model not found.

### 3.4 Entity reference resolution

```
create agent MyModule.EntityRef (
  UsageType: chat,
  Model: MyModule.GPT4o,
  Entity: MyModule.ChatSession,
  SystemPrompt: $$Test.$$
);
```

**Expected:** Resolves `MyModule.ChatSession` to existing entity. Error if entity not found.

### 3.5 Duplicate agent

```
create agent MyModule.SimpleAgent (
  UsageType: chat,
  Model: MyModule.GPT4o,
  SystemPrompt: $$Duplicate.$$
);
```

**Expected:** Error — agent already exists.

---

## 4. DROP AGENT

### 4.1 Drop existing agent

```
drop agent MyModule.SimpleAgent;
```

**Expected:** Agent removed. `show agents` no longer lists it.

### 4.2 Drop non-existent agent

```
drop agent MyModule.NonExistent;
```

**Expected:** Error — agent not found.

---

## 5. SHOW MODELS

### 5.1 List all models

```
show models;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Provider | Key Constant | Display Name`. Summary line `(N models)`. Sorted alphabetically.

### 5.2 List models in a module

```
show models in MyModule;
```

**Expected:** Only models from `MyModule`. Same column format.

### 5.3 Empty module

```
show models in NonExistentModule;
```

**Expected:** Error or empty result with `(0 models)`.

---

## 6. DESCRIBE MODEL

### 6.1 Basic model

```
describe model MyModule.GPT4o;
```

**Expected:** MDL output:

```mdl
create model MyModule.GPT4o (
  Provider: MxCloudGenAI,
  Key: MyModule.OpenAIKey,
  DisplayName: 'GPT-4o'
);
```

### 6.2 Model with all properties

Find a model with additional properties beyond Provider, Key, and DisplayName. Verify all appear in `describe` output.

### 6.3 Non-existent model

```
describe model Fake.Missing;
```

**Expected:** Error — model not found.

---

## 7. CREATE MODEL

### 7.1 Minimal model (default provider)

```
create model MyModule.BasicModel (
  Key: MyModule.ApiKey,
  DisplayName: 'Basic Model'
);
```

**Expected:** Model created. Provider defaults to `MxCloudGenAI`. `describe` matches.

### 7.2 Explicit provider

```
create model MyModule.CustomModel (
  Provider: MxCloudGenAI,
  Key: MyModule.ApiKey,
  DisplayName: 'Custom Model'
);
```

**Expected:** Model created with explicit provider.

### 7.3 Constant reference resolution

```
create model MyModule.RefModel (
  Key: MyModule.NonExistentConst,
  DisplayName: 'Test'
);
```

**Expected:** Error — constant `MyModule.NonExistentConst` not found.

### 7.4 Auto-create module

```
create model NewModule.Model1 (
  Key: MyModule.ApiKey,
  DisplayName: 'New Module Model'
);
```

**Expected:** `NewModule` auto-created if it does not exist.

### 7.5 Duplicate model

```
create model MyModule.BasicModel (
  Key: MyModule.ApiKey,
  DisplayName: 'Duplicate'
);
```

**Expected:** Error — model already exists.

---

## 8. DROP MODEL

### 8.1 Drop existing model

```
drop model MyModule.BasicModel;
```

**Expected:** Model removed. `show models` no longer lists it.

### 8.2 Drop non-existent model

```
drop model MyModule.NonExistent;
```

**Expected:** Error — model not found.

---

## 9. SHOW KNOWLEDGE BASES

### 9.1 List all knowledge bases

```
show knowledge bases;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Provider | Key Constant | Embedding Model`. Summary line `(N knowledge bases)`. Sorted alphabetically.

### 9.2 List knowledge bases in a module

```
show knowledge bases in MyModule;
```

**Expected:** Only knowledge bases from `MyModule`. Same column format.

### 9.3 Empty module

```
show knowledge bases in NonExistentModule;
```

**Expected:** Error or empty result with `(0 knowledge bases)`.

---

## 10. DESCRIBE KNOWLEDGE BASE

### 10.1 Basic knowledge base

```
describe knowledge base MyModule.DocumentationKB;
```

**Expected:** MDL output:

```mdl
create knowledge base MyModule.DocumentationKB (
  Provider: MxCloudGenAI,
  Key: MyModule.KBKey,
  ModelDisplayName: 'text-embedding-ada-002',
  ModelName: 'text-embedding-ada-002'
);
```

### 10.2 Knowledge base with all properties

Find a knowledge base with additional properties. Verify all appear in `describe` output.

### 10.3 Non-existent knowledge base

```
describe knowledge base Fake.Missing;
```

**Expected:** Error — knowledge base not found.

---

## 11. CREATE KNOWLEDGE BASE

### 11.1 Minimal knowledge base

```
create knowledge base MyModule.TestKB (
  Key: MyModule.KBKey,
  ModelDisplayName: 'text-embedding-ada-002',
  ModelName: 'text-embedding-ada-002'
);
```

**Expected:** Knowledge base created. `describe` matches.

### 11.2 Explicit provider

```
create knowledge base MyModule.FullKB (
  Provider: MxCloudGenAI,
  Key: MyModule.KBKey,
  ModelDisplayName: 'text-embedding-3-small',
  ModelName: 'text-embedding-3-small'
);
```

**Expected:** Knowledge base created with explicit provider.

### 11.3 Constant reference resolution

```
create knowledge base MyModule.BadKB (
  Key: MyModule.NonExistentConst,
  ModelDisplayName: 'test',
  ModelName: 'test'
);
```

**Expected:** Error — constant `MyModule.NonExistentConst` not found.

### 11.4 Auto-create module

```
create knowledge base NewModule.KB1 (
  Key: MyModule.KBKey,
  ModelDisplayName: 'test',
  ModelName: 'test'
);
```

**Expected:** `NewModule` auto-created if it does not exist.

### 11.5 Duplicate knowledge base

```
create knowledge base MyModule.TestKB (
  Key: MyModule.KBKey,
  ModelDisplayName: 'test',
  ModelName: 'test'
);
```

**Expected:** Error — knowledge base already exists.

---

## 12. DROP KNOWLEDGE BASE

### 12.1 Drop existing knowledge base

```
drop knowledge base MyModule.TestKB;
```

**Expected:** Knowledge base removed. `show knowledge bases` no longer lists it.

### 12.2 Drop non-existent knowledge base

```
drop knowledge base MyModule.NonExistent;
```

**Expected:** Error — knowledge base not found.

---

## 13. SHOW CONSUMED MCP SERVICES

### 13.1 List all consumed MCP services

```
show consumed mcp services;
```

**Expected:** Table with columns `Qualified Name | Module | Name | Protocol | Version | Timeout`. Summary line `(N consumed mcp services)`. Sorted alphabetically.

### 13.2 List consumed MCP services in a module

```
show consumed mcp services in MyModule;
```

**Expected:** Only services from `MyModule`. Same column format.

### 13.3 Empty module

```
show consumed mcp services in NonExistentModule;
```

**Expected:** Error or empty result with `(0 consumed mcp services)`.

---

## 14. DESCRIBE CONSUMED MCP SERVICE

### 14.1 Basic consumed MCP service

```
describe consumed mcp service MyModule.ExternalSvc;
```

**Expected:** MDL output:

```mdl
create consumed mcp service MyModule.ExternalSvc (
  ProtocolVersion: 2024-11-05,
  Version: '1.0.0',
  ConnectionTimeoutSeconds: 30,
  Documentation: 'External data provider for weather information'
);
```

### 14.2 Service with all properties

Find a consumed MCP service with all available properties. Verify all appear in `describe` output.

### 14.3 Non-existent consumed MCP service

```
describe consumed mcp service Fake.Missing;
```

**Expected:** Error — consumed MCP service not found.

---

## 15. CREATE CONSUMED MCP SERVICE

### 15.1 Minimal consumed MCP service

```
create consumed mcp service MyModule.WeatherSvc (
  ProtocolVersion: 2024-11-05,
  Version: '1.0.0',
  ConnectionTimeoutSeconds: 30
);
```

**Expected:** Service created. `describe` matches.

### 15.2 With documentation

```
create consumed mcp service MyModule.DataSvc (
  ProtocolVersion: 2024-11-05,
  Version: '2.0.0',
  ConnectionTimeoutSeconds: 60,
  Documentation: 'Provides access to external data sources'
);
```

**Expected:** Service created with documentation. `describe` shows `Documentation` property.

### 15.3 Auto-create module

```
create consumed mcp service NewModule.Svc1 (
  ProtocolVersion: 2024-11-05,
  Version: '1.0.0',
  ConnectionTimeoutSeconds: 30
);
```

**Expected:** `NewModule` auto-created if it does not exist.

### 15.4 Duplicate consumed MCP service

```
create consumed mcp service MyModule.WeatherSvc (
  ProtocolVersion: 2024-11-05,
  Version: '1.0.0',
  ConnectionTimeoutSeconds: 30
);
```

**Expected:** Error — consumed MCP service already exists.

---

## 16. DROP CONSUMED MCP SERVICE

### 16.1 Drop existing consumed MCP service

```
drop consumed mcp service MyModule.WeatherSvc;
```

**Expected:** Service removed. `show consumed mcp services` no longer lists it.

### 16.2 Drop non-existent consumed MCP service

```
drop consumed mcp service MyModule.NonExistent;
```

**Expected:** Error — consumed MCP service not found.

---

## 17. ROUNDTRIP

Test that CREATE → DESCRIBE → CREATE (from output) produces identical results.

### 17.1 Model roundtrip

```
create model RtTest.Model1 (
  Provider: MxCloudGenAI,
  Key: RtTest.ApiKey,
  DisplayName: 'Test Model'
);
```

1. `describe model RtTest.Model1`
2. Drop: `drop model RtTest.Model1`
3. Execute described MDL
4. `describe` again

**Expected:** Identical output.

### 17.2 Knowledge base roundtrip

```
create knowledge base RtTest.DocsKB (
  Provider: MxCloudGenAI,
  Key: RtTest.KBKey,
  ModelDisplayName: 'text-embedding-ada-002',
  ModelName: 'text-embedding-ada-002'
);
```

1. `describe knowledge base RtTest.DocsKB`
2. Drop: `drop knowledge base RtTest.DocsKB`
3. Execute described MDL
4. `describe` again

**Expected:** Identical output.

### 17.3 Consumed MCP service roundtrip

```
create consumed mcp service RtTest.ExternalSvc (
  ProtocolVersion: 2024-11-05,
  Version: '1.0.0',
  ConnectionTimeoutSeconds: 30,
  Documentation: 'Test service'
);
```

1. `describe consumed mcp service RtTest.ExternalSvc`
2. Drop: `drop consumed mcp service RtTest.ExternalSvc`
3. Execute described MDL
4. `describe` again

**Expected:** Identical output.

---

## 18. MULTI-STEP WORKFLOWS

### 18.1 Cross-module references

```
create constant ModA.Key type String default 'key';
create model ModA.SharedModel (Key: ModA.Key, DisplayName: 'Shared');
create agent ModB.CrossAgent (
  UsageType: chat,
  Model: ModA.SharedModel,
  SystemPrompt: $$Cross-module test.$$
);
describe agent ModB.CrossAgent;
```

**Expected:** Agent in `ModB` references model in `ModA`. `describe` shows fully qualified model name.

---

## 19. FAILURE MODES & ERROR RECOVERY

### 19.1 Not connected

Run any command without `-p` flag or REPL session.

**Expected:** Error — not connected to a project.

### 19.2 Agent already exists

```
create agent MyModule.TestAgent (...);
create agent MyModule.TestAgent (...);
```

**Expected:** Second create fails — agent already exists.

### 19.3 Model not found during agent creation

```
create agent MyModule.BadAgent (
  UsageType: chat,
  Model: MyModule.NonExistentModel,
  SystemPrompt: $$Test.$$
);
```

**Expected:** Error — model `MyModule.NonExistentModel` not found.

### 19.4 Constant not found during model creation

```
create model MyModule.BadModel (
  Key: MyModule.FakeConst,
  DisplayName: 'Bad'
);
```

**Expected:** Error — constant not found.

### 19.5 Constant not found during KB creation

```
create knowledge base MyModule.BadKB (
  Key: MyModule.FakeConst,
  ModelDisplayName: 'test',
  ModelName: 'test'
);
```

**Expected:** Error — constant not found.

### 19.6 Drop model referenced by agent

```
drop model MyModule.TestModel;
```

**Expected:** Warning about agent references. Model dropped (agent may have dangling reference).

---

## 20. BOUNDARY & STRESS

### 20.1 Agent with many tools (10+)

Create an agent with 10+ tool blocks. Verify `describe` lists all tools.

### 20.2 Agent with many knowledge bases (5+)

Create an agent with 5+ knowledge base blocks. Verify `describe` lists all.

### 20.3 Long dollar-quoted prompt

Create an agent with a `SystemPrompt` exceeding 4000 characters. Verify `describe` preserves full text.

### 20.4 Special characters in prompts

```
create agent MyModule.SpecialChars (
  UsageType: chat,
  Model: MyModule.GPT4o,
  SystemPrompt: $$Prompt with 'single quotes', "double quotes",
backslash \, dollar $, and unicode: café résumé$$
);
```

**Expected:** All characters preserved in `describe` output.

### 20.5 Long service documentation

```
create consumed mcp service MyModule.DocSvc (
  ProtocolVersion: 2024-11-05,
  Version: '1.0.0',
  ConnectionTimeoutSeconds: 30,
  Documentation: 'Very long documentation string...(1000+ chars)'
);
```

**Expected:** Full documentation preserved in `describe`.

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Product |
|-----------|:---:|:---:|:---:|
| SHOW AGENTS | x | x | x |
| DESCRIBE AGENT | x | x | x |
| CREATE AGENT | x | | |
| DROP AGENT | x | | |
| SHOW MODELS | x | x | x |
| DESCRIBE MODEL | x | x | |
| CREATE MODEL | x | | |
| DROP MODEL | x | | |
| SHOW KNOWLEDGE BASES | x | x | x |
| DESCRIBE KNOWLEDGE BASE | x | x | |
| CREATE KNOWLEDGE BASE | x | | |
| DROP KNOWLEDGE BASE | x | | |
| SHOW CONSUMED MCP SERVICES | x | x | x |
| DESCRIBE CONSUMED MCP SERVICE | x | x | |
| CREATE CONSUMED MCP SERVICE | x | | |
| DROP CONSUMED MCP SERVICE | x | | |

Read operations tested on all projects. Write operations on copies of one project.

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. SHOW AGENTS | Mock tests | |
| 2. DESCRIBE AGENT | Mock tests | All block types |
| 3. CREATE AGENT | Mock + roundtrip | Reference resolution |
| 4. DROP AGENT | Mock tests | |
| 5. SHOW MODELS | Mock tests | |
| 6. DESCRIBE MODEL | Mock tests | |
| 7. CREATE MODEL | Mock tests | Constant resolution |
| 8. DROP MODEL | Mock tests | |
| 9. SHOW KNOWLEDGE BASES | Mock tests | |
| 10. DESCRIBE KNOWLEDGE BASE | Mock tests | |
| 11. CREATE KNOWLEDGE BASE | Mock tests | Constant resolution |
| 12. DROP KNOWLEDGE BASE | Mock tests | |
| 13. SHOW CONSUMED MCP SERVICES | Mock tests | |
| 14. DESCRIBE CONSUMED MCP SERVICE | Mock tests | |
| 15. CREATE CONSUMED MCP SERVICE | Mock tests | |
| 16. DROP CONSUMED MCP SERVICE | Mock tests | |
| 17. Roundtrip | Roundtrip tests | Complex agents |
| 18. Multi-step | | All manual |
| 19. Failure modes | Partial | Edge cases |
| 20. Boundary | | All manual |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | SHOW AGENTS | List all | | | | |
| 1.2 | SHOW AGENTS | Filter by module | | | | |
| 1.3 | SHOW AGENTS | Empty module | | | | |
| 2.1 | DESCRIBE AGENT | All block types | | | | |
| 2.2 | DESCRIBE AGENT | Dollar-quoted prompts | | | | |
| 2.3 | DESCRIBE AGENT | Not found | | | | |
| 3.1 | CREATE AGENT | Minimal | | | | |
| 3.2 | CREATE AGENT | Auto-create module | | | | |
| 3.3 | CREATE AGENT | Model reference | | | | |
| 3.4 | CREATE AGENT | Entity reference | | | | |
| 3.5 | CREATE AGENT | Duplicate error | | | | |
| 4.1 | DROP AGENT | Existing | | | | |
| 4.2 | DROP AGENT | Non-existent | | | | |
| 5.1 | SHOW MODELS | List all | | | | |
| 5.2 | SHOW MODELS | Filter by module | | | | |
| 5.3 | SHOW MODELS | Empty module | | | | |
| 6.1 | DESCRIBE MODEL | Basic | | | | |
| 6.2 | DESCRIBE MODEL | All properties | | | | |
| 6.3 | DESCRIBE MODEL | Not found | | | | |
| 7.1 | CREATE MODEL | Minimal (default provider) | | | | |
| 7.2 | CREATE MODEL | Explicit provider | | | | |
| 7.3 | CREATE MODEL | Constant not found | | | | |
| 7.4 | CREATE MODEL | Auto-create module | | | | |
| 7.5 | CREATE MODEL | Duplicate error | | | | |
| 8.1 | DROP MODEL | Existing | | | | |
| 8.2 | DROP MODEL | Non-existent | | | | |
| 9.1 | SHOW KBS | List all | | | | |
| 9.2 | SHOW KBS | Filter by module | | | | |
| 9.3 | SHOW KBS | Empty module | | | | |
| 10.1 | DESCRIBE KB | Basic | | | | |
| 10.2 | DESCRIBE KB | All properties | | | | |
| 10.3 | DESCRIBE KB | Not found | | | | |
| 11.1 | CREATE KB | Minimal | | | | |
| 11.2 | CREATE KB | Explicit provider | | | | |
| 11.3 | CREATE KB | Constant not found | | | | |
| 11.4 | CREATE KB | Auto-create module | | | | |
| 11.5 | CREATE KB | Duplicate error | | | | |
| 12.1 | DROP KB | Existing | | | | |
| 12.2 | DROP KB | Non-existent | | | | |
| 13.1 | SHOW MCP | List all | | | | |
| 13.2 | SHOW MCP | Filter by module | | | | |
| 13.3 | SHOW MCP | Empty module | | | | |
| 14.1 | DESCRIBE MCP | Basic | | | | |
| 14.2 | DESCRIBE MCP | All properties | | | | |
| 14.3 | DESCRIBE MCP | Not found | | | | |
| 15.1 | CREATE MCP | Minimal | | | | |
| 15.2 | CREATE MCP | With documentation | | | | |
| 15.3 | CREATE MCP | Auto-create module | | | | |
| 15.4 | CREATE MCP | Duplicate error | | | | |
| 16.1 | DROP MCP | Existing | | | | |
| 16.2 | DROP MCP | Non-existent | | | | |
| 17.1 | ROUNDTRIP | Model | | | | |
| 17.2 | ROUNDTRIP | Knowledge base | | | | |
| 17.3 | ROUNDTRIP | Consumed MCP service | | | | |
| 18.1 | MULTI-STEP | Cross-module refs | | | | |
| 19.1 | FAILURE | Not connected | | | | |
| 19.2 | FAILURE | Already exists | | | | |
| 19.3 | FAILURE | Model not found | | | | |
| 19.4 | FAILURE | Constant not found (model) | | | | |
| 19.5 | FAILURE | Constant not found (KB) | | | | |
| 19.6 | FAILURE | Drop referenced model | | | | |
| 20.1 | BOUNDARY | Many tools | | | | |
| 20.2 | BOUNDARY | Many KBs | | | | |
| 20.3 | BOUNDARY | Long prompt | | | | |
| 20.4 | BOUNDARY | Special characters | | | | |
| 20.5 | BOUNDARY | Long documentation | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
