# PED / MCP Tool Capabilities per Studio Pro Version

Studio Pro ships an embedded **MCP server** (the "PED" — Progressive Element
Disclosure — server, exposing Mendix's "Maia" agent tools) on a local HTTP
port. The mxcli **MCP backend** (`mdl/backend/mcp/`) is a client of this server:
it routes model writes through PED so Studio Pro stays the authoritative
serializer while the project stays open. See
[`PROPOSAL_mcp_backend.md`](../11-proposals/PROPOSAL_mcp_backend.md) for the why.

**The tool surface changes between Studio Pro versions.** Each release can add,
remove, or change tools — which directly expands or limits what the MCP backend
can do. This document is the canonical record of *which PED tools exist in which
Studio Pro version* and *what capability gaps each version has*. Update it
whenever a new Studio Pro version is onboarded (procedure at the bottom).

> **Direction:** [ADR-0006](../13-decisions/0006-mcp-capability-model.md) decides to
> make this knowledge machine-readable — a version-keyed capability table merged
> with a live `tools/list` probe — so the backend gates on it and an agent-facing
> capability report is generated from it. This document then becomes the
> human-readable narrative over that table. Until that lands, this doc + the
> scattered per-type rejections are the source of truth.

This is a developer reference. It is the sibling of
[`WIDGET_BSON_VERSION_COMPATIBILITY.md`](WIDGET_BSON_VERSION_COMPATIBILITY.md)
for the MCP transport instead of on-disk BSON. For the **cross-layer** view —
how MCP coverage compares to the MPR backend, MDL, and the Mendix platform — see
the Backend Coverage section of
[`../01-project/MDL_FEATURE_MATRIX.md`](../01-project/MDL_FEATURE_MATRIX.md); this
document is the MCP column's deep-dive.

> **Provenance.** Every row below was captured live with `cmd/mcpprobe` against a
> running Studio Pro, not copied from documentation. Raw fixtures live in
> `mdl/backend/mcp/testdata/`: tool surfaces (`tools.json`, `tools-11.13.json`,
> `tools-11.14.json`) and entity schemas (`schema-entity.json` = 11.11,
> `schema-entity-11.14.json`). Re-capture per the onboarding procedure; do not
> hand-edit claims you have not verified.
>
> **`ped_get_schema`'s response format changed between 11.11 and 11.14**, from a
> structured JSON schema (`{"schemas":[{elementType, schema:{properties:…}}]}`) to a
> TypeScript-like source text (`{"kind":"constructor","schema":"constructor type
> 'DomainModels$Entity' = {…}"}`). **Which release did it is not established here** —
> the 11.11 fixture and the 11.14 capture are the only two samples, and 11.13's
> addition of the `kind` argument makes 11.13 the likelier culprit. Do not cite this
> as an 11.14 change. It costs the backend nothing either way: `ensureSchema` calls
> the tool only to satisfy PED's fetch-before-create contract and discards the body.

## Server identity

| Studio Pro | MCP server present | `serverInfo` | MCP protocol | Captured |
|------------|--------------------|--------------|--------------|----------|
| ≤ 11.10    | **No**             | —            | —            | —        |
| 11.11      | Yes                | `mendix-studio-pro` 1.0.0 | `2025-06-18` | 2026-06-05 |
| 11.12      | Yes                | `mendix-studio-pro` 1.0.0 | `2025-06-18` | 2026-06-23 |
| 11.13      | Yes                | `mendix-studio-pro` 1.0.0 | `2025-06-18` | 2026-08-11 |
| 11.14      | Yes                | `mendix-studio-pro` 1.0.0 | `2025-06-18` | 2026-08-25 |

> **`serverInfo.version` is frozen at `1.0.0` across 11.11, 11.12, 11.13 **and 11.14**
> even though the tool surface and behaviour changed in every one of them.** So the
> server version is **not** a reliable discriminator between Studio Pro releases — four
> releases in, treat this as settled rather than provisional. The machine-readable
> [`capabilities.yaml`](../../mdl/backend/mcp/capabilities.yaml) keys `available_since`
> on the server version and therefore **cannot express an 11.12-only capability** —
> features that vary by Studio Pro version must be gated on the **project's Mendix
> version** instead (e.g. `gateAttributeDefaults` → `ProjectVersion().IsAtLeast(11,12)`).
> Until the table grows a Studio-Pro-version dimension, this per-version doc is the
> source of truth for the 11.11→11.12, 11.12→11.13 and 11.13→11.14 deltas below.
>
> **Gate a per-release *argument* on a live schema probe, not on any version.**
> `Client.SupportsToolArg(tool, arg)` answers from a cached `tools/list` (which now
> captures each tool's `inputSchema.properties`). Tool schemas are declared
> `additionalProperties:false`, so sending an argument an older server does not know
> fails the whole call — "unknown" must mean "do not send". This is what keeps
> `pg_read_page`'s 11.13-only `depth` off 11.11/11.12 servers.

`serverInfo.version` is the MCP server's own version, distinct from the Studio
Pro version. The MCP server first appears in **11.11**; earlier versions have no
endpoint and the backend must fail with an actionable error on connect.

## Tool matrix

A tool's cell is the first Studio Pro version it was observed in (✓ = present,
blank = absent, `—` = n/a). `cmd/mcpprobe -method tools/list` is the source.

| Tool | 11.11 | Purpose / used by the backend |
|------|:-----:|-------------------------------|
| `ped_get_schema` | ✓ | Schemas for element types (`$constructor` for create, `$element` for read). Backend: fetched before create/add. |
| `ped_find_document` | ✓ | Find docs by module + type. **Must NOT be used for `DomainModels$DomainModel`** (always exists, nameless). |
| `ped_read_document` | ✓ | Progressive read; JSON-pointer `paths` to descend. Backend: dirty-set read reconstruction. |
| `ped_list_folder` | ✓ | Immediate contents of a module/folder. (Not yet used.) |
| `ped_create_module` | ✓ | Create a module (+ its domain model). Flushes to disk immediately. |
| `ped_create_document` | ✓ | Create standalone documents (enum, microflow, page, …). "Never create domain models." Backend: `CreateEnumeration`. |
| `ped_update_document` | ✓ | Operation-based set/add/remove at JSON paths. Backend: entities, attributes, associations (incl. removes). |
| `ped_check_errors` | ✓ | Validate documents (run after the final write). Backend: after every write — but see **Error-list lag** below. |
| `pg_read_page` | ✓ | **Pages only** — separate read path. (Not yet used.) |
| `pg_write_page` | ✓ | **Pages only** — separate write path; PED is *forbidden* for pages. (Not yet used.) |
| `oql_generate` | ✓ | NL → OQL for a module. (Agent helper; not used.) |
| `search_mendix_knowledge_base` | ✓ | Docs/KB search. (Agent helper; not used.) |
| `read_skill` | ✓ | Load a Maia skill. (Agent helper; not used.) |
| `glob` | ✓ | List files in a virtual file domain. (Agent helper; not used.) |
| `read_file` | ✓ | Read a file in a virtual file domain. (Agent helper; not used.) |
| `write_file` | ✓ | Write Java/JS/CSS in a virtual file domain. **Intentionally not used** — these exist for Maia (the in-IDE agent with no disk access). Claude Code / mxcli run with direct filesystem access to the project, so source files are edited on disk directly; routing through the virtual FS would be pure overhead. Not a capability gap. |

`initialize` instructs clients to first read the resource
`mendix://studio-pro/system-prompt` (the Maia system prompt + PED contract).

## 11.12 changes (delta vs 11.11)

Captured live 2026-06-23 (`cmd/mcpprobe -method tools/list`). The 11.11 matrix above is otherwise unchanged; this section records only what differs. Tool count 16 → 18.

| Change | Tool | Effect on the backend |
|--------|------|-----------------------|
| **Removed** | `pg_write_page` | **Breaking** (was: MCP page authoring broken on 11.12). **Migrated (#697):** `page.go`'s `pgWritePage` now calls `pg_patch_page` instead. Verified live — page create round-trips. |
| **Added** | `pg_patch_page` | Pages via JSON Patch (RFC 6902): create/whole-page-write = one `{op:"replace", path:"", value:<full LightPage>}` (the `value` is the same LightPage `pg_write_page` took, so the content builders are unchanged); patch existing = targeted ops (path must reference an existing element). mxcli routes both CreatePage and the mutator's `Save()` through the root-replace form (#697). Targeted-op ALTER (vs. the current read-modify-replace-whole) is a possible future optimisation. |
| **Added** | `list_modules` | Returns `[{moduleName, writable, fromMarketplace}]`. **Closes the "No list-modules tool" gap** (see below) — pure-MCP module enumeration is now possible, reducing the hard dependency on a matching local `.mpr`. |
| **Added** | `install_marketplace_module` | `{moduleName, versionId, conflictResolution}` — installs a Marketplace module into the open project. (Not used by the backend yet.) |

Tools already present in 11.11 and unchanged (do **not** re-add as "new"): `ped_list_folder`, `oql_generate`, `search_mendix_knowledge_base`, `read_skill`, `glob`, `read_file`, `write_file`.


**Error-list lag — `ped_check_errors` is not synchronous with the write (issue #945).**
It reads the error list Studio Pro maintains on a **background thread**, so it
reports the model as of some moment *before* the call. Measured live (PED 1.0.0)
with a 20ms poll interval, **26 samples** spanning both directions and three
write-load levels (0, 10 and 30 preceding writes):

| | min | median | max |
|---|---|---|---|
| write → error becomes **visible** | 77ms | 82ms | 115ms |
| fix → error stops being **shown** | 75ms | 83ms | 128ms |

It is one symmetric debounce, and it did **not** grow with load. A settled check
itself costs ~11ms, so re-asking is cheap; only the initial wait is not.

> **Measure it with a fine poll interval.** The first pass at this reported
> 170-350ms and concluded the lag "grows under load". Both were artifacts of
> polling at 50-100ms — the granularity, not the lag. Re-measuring at 20ms
> collapsed the spread to 77-128ms and removed the load dependence entirely.

Both directions are damaging, so waiting is the only fix:

- asked too early it answers `No errors found.` for a document that was just
  broken — a **silent** miss, and `ped_update_document` is no backstop because it
  reports op-level failures only (a duplicate name comes back as `SUCCESS`);
- it equally still reports an error a just-applied op has already **cleared**,
  which fails a perfectly good statement on a leftover verdict. A multi-op ALTER
  passes through intermediate states that are legitimately invalid, so this is
  not hypothetical.

**`Backend.pedCheckDocument` owns the pacing for every call site**: sleep
`settleDelay` (250ms, ~2x the observed max) before the first ask, then re-ask
across `settleWindow` (250ms) at `settleInterval` while the answer stays clean,
short-circuiting on an error. Re-asking is only safe *after* the delay — past it
a dirty verdict is current rather than left over. It lives in the shared helper
rather than in each caller so a new write path cannot forget it; the cost is one
settle per backend operation, and no operation validates more than once (checked:
`renameAttribute` and `applyInPlaceEntityChanges` are early-return branches of
`UpdateEntity`, not a loop).

Verified live against the immediate check as a control, 6 trials each: the
immediate check missed a real error **6/6** and falsely failed on a cleared
transient **6/6**; the settled check was **0/6** on both.

**New authoring capability — attribute default values (implemented, `domainmodel.go`).** PED's `DomainModels$StoredValue.defaultValue` is settable via a `ped_update_document` path-op (`/entities/N/attributes/M/value/defaultValue`); the create constructor still can't carry it, so it's set as a follow-up after the attribute exists (`applyAttributeDefaults`). Verified live on 11.12: enums store **bare** (`Draft`, not `MES.WorkOrderStatus.Draft`); PED accepts bare or qualified input but normalises to bare. **Gated on the project Mendix version (11.12+), not the server version** (frozen at 1.0.0 — see the caveat under Server identity). The entity/attribute `$constructor` schema is otherwise unchanged from 11.11; its text now hints `DomainModels$Index` and `DomainModels$ValidationRule` are addable the same constructor-then-path-op way — candidates to wire next, like defaults.

**New authoring capability — navigation (implemented, `navigation.go`; issue #699).** The project-level `Navigation$NavigationDocument` has no dedicated PED tool, but it is reachable through the **generic** document tools (`ped_read_document` / `ped_update_document` with `documentType` set and `documentName` **omitted** — project-level). `CREATE OR REPLACE NAVIGATION` on a **web** profile is authored as path-ops: scalar leaves are `set` in place (`/profiles/N/homePage/page`, `/profiles/N/loginPageSettings/page`), a currently-null element (`notFoundHomepage`) is `set` whole, and the menu (`menuItemCollection.items`, an array) is **cleared then rebuilt** — removes and adds go in **separate** calls (PED forbids batching add+remove on one array path). Menu items are `Menus$MenuItem` constructors: `caption` is a **plain string** (PED wraps it into `Texts$Text`), the action is `Pages$PageClientAction` (page ref under `pageSettings.page`), `Pages$MicroflowClientAction` (`microflowSettings.microflow`), or `Pages$NoClientAction` (container); sub-items recurse. Verified live on 11.12 by reproducing a full menu (3 page items + an Admin container with 2 children). Gates/limits: `ped_check_errors` cannot address the project-level nav doc, so the `ped_update_document` result is the validation gate (a bad page ref fails the op); **native** profiles and **role-based home pages** are rejected (not wired); and Studio Pro often answers a nav write with `-32000 Request timed out` while the edit still applies (slow nav re-render) — the backend surfaces an actionable hint rather than auto-retrying the non-idempotent op.

**New authoring capability — entity access rules (implemented, `security.go`).** "Security" is two different things over PED. The security **documents** are sealed: `ped_read_document` on `Security$ProjectSecurity` and `Security$ModuleSecurity` both return **"Unknown document type"**, so **module roles, user roles, demo users, and project security settings cannot be authored over MCP**. But an entity's **access rules** are not in the security document — they live on `DomainModels$Entity.accessRules` (the domain-model document PED already authors). Verified live on 11.12: a rule's `moduleRoles`, per-member `attribute`/`association` refs, and access rights are the **same qualified names** mxcli already builds (`ExpenseApproval.Expense.Title`, `ExpenseApproval.Expense_Employee`, `ExpenseApproval.Manager`), so `EntityAccessRuleParams` maps 1:1 onto a `DomainModels$AccessRule` constructor `add`. The referenced module role must already exist. **Hard limit — PED is add/modify-only for access control:** `DomainModels$AccessRule` and `DomainModels$MemberAccess` can be `add`ed and their leaves `set`, but **never removed** ("Element of type … cannot be removed"). So mxcli can GRANT a new rule but **rejects** REVOKE and replacing an existing rule in place (it can't remove the old rule/members) — do those in Studio Pro. The executor builds the complete member-access list (every attribute + FROM-side associations + system owner/changedBy, per the CE0066 FROM-entity rule), so the `add` passes `ped_check_errors`.

## 11.13 changes (delta vs 11.12)

Captured live 2026-08-11 (`cmd/mcpprobe -method tools/list`, fixture
`mdl/backend/mcp/testdata/tools-11.13.json`). Tool count stays 18, but the
composition changed. **The 11.13 release notes announce none of the items in this
section** — they cover only auto-port-selection, a status-bar port indicator, and
four fixes to Studio Pro's MCP *client*. This is the second release in a row where
the authoring surface moved silently (11.12 removed `pg_write_page`, #697), so the
live probe — including **input schemas**, not just tool names — is the only
trustworthy source.

| Change | Tool | Effect on the backend |
|--------|------|-----------------------|
| **Removed** | `oql_generate` | Not used by the backend (LLM-backed; view entities are authored from user OQL verbatim). Its disappearance lines up with 11.13's new **"OQL Generation Toggle"** preference, so treat it as **configuration-dependent, not removed** — see the federation note below. |
| **Added** | `mcp_mendix-marketplace_Component_GetComponentIDsByCriteria` | A **proxied** tool, not a Studio Pro one — see federation below. Not used by the backend. |
| **Changed** | `pg_read_page` | **Breaking — was: ALTER PAGE broken on 11.13.** Gained `depth` (**default 4**) and `paths`; nodes below the limit become the literal string `"..."`. Fixed in `pgReadPage`: request `pgReadFullDepth` when the server advertises the argument, and refuse a read that still carries the sentinel. See the truncation note below. |
| **Changed** | `ped_get_schema` | Gained `kind` (`constructor` \| `element`, default `constructor`), making explicit the two shapes the system prompt always described. **No backend change needed** — `ensureSchema` calls it only to satisfy PED's fetch-before-create contract and discards the body. |
| **Changed** | `ped_update_document` | Description-only. Now states the rule mxcli's `navigation.go` discovered empirically: an element-valued property can be `set` **only while it is null/undefined**; a non-null one cannot be re-set. |
| **Changed** | `ped_create_document` | Description-only (`documentContent`). Its text references a tool named `get_document_schema`, which does not exist in `tools/list` — an upstream naming slip, not a tool we are missing. |

**Studio Pro now federates the MCP servers it is a client of.** The system prompt
says it outright: *"Capabilities can be extended via MCP (Model Context Protocol)
tools provided by the user. MCP tools are prefixed `mcp_{serverName}_{toolName}`."*
The `mcp_mendix-marketplace_*` entry is the Marketplace MCP server re-exposed
through Studio Pro's own server. Two consequences: the tool surface now varies
**per user configuration** as well as per release, and `tools.listChanged: true`
means it can change within a session. Combined with a togglable `oql_generate`,
**tool presence must be probe-gated, never table-gated** — `capabilities.yaml` can
describe what mxcli does with a tool, but not whether it is there.

**Observed, and the reason this matters.** The *same* Studio Pro session reported
**18 tools including `mcp_mendix-marketplace_*`, then 17 without it an hour later**,
with no restart (2026-08-11). The federated tool comes and goes with Studio Pro's own
client connection to the Marketplace server — the very thing 11.13's "repeated
disconnect/reconnect" fix addresses. A table asserting that tool's presence would
have been wrong within the hour. The fixture `testdata/tools-11.13.json` captures the
18-tool state; treat its federated entry as a **sample, not a constant**, and do not
write a test that asserts a federated tool is present.

**Page reads are depth-truncated by default (the 11.13 regression).** Measured on
`Administration.Account_Overview`: 32,594 bytes at full depth, **1,052 bytes** at
the default, the entire widget tree collapsed to
`{"widgets":[{"$Type":"Pages$Content","slot":"Main","widgets":["...","..."]}]}`.
This is not an edge case — all three pages sampled from `PgTest` truncated too.
Because ALTER PAGE is read-modify-**replace-whole-page**, the truncated read went
back as the new page body; `pg_patch_page` **rejected** it (`PROP_NOT_PRIMITIVE:
Property 'widgets' is not a primitive property`) and left the page intact, so the
symptom was a broken ALTER PAGE rather than data loss. `CREATE PAGE` was never
affected (it does not read). The fix has two halves, and the second is the durable
one: `pgReadPage` asks for `pgReadFullDepth`, **and** `hasTruncationSentinel`
refuses any read still carrying a placeholder, so the next change to the server's
truncation default fails loudly instead of silently (ADR-0005 guard-don't-drop).
The sentinel is matched only as an **array element** — a caption or title that
legitimately reads `"..."` is real page content and must not trip the guard.

**Unchanged gaps, re-verified live on 11.13:** still **no delete-document tool**,
still **no save/flush tool**, reads still expose `$QualifiedName` but **not `$ID`**,
and the security documents are still sealed.

### Re-probe 2026-08-24 (11.13.0) — mappings, and a second federation sample

Probed from a devcontainer through a host-side
`socat TCP6-LISTEN:7790,reuseaddr,fork,ipv6only=0 'TCP6:[::1]:7782'`, dialled as
`-dial host.docker.internal:7790` with the `Host` header left at `localhost:7782`.

**Tool surface: 17, identical to `tools-11.13.json` minus
`mcp_mendix-marketplace_Component_GetComponentIDsByCriteria`.** That is the third
observation of the federated tool coming and going on 11.13 with no restart, and
it settles the "sample, not a constant" call above — no table row should ever
assert it.

**Import/export mappings are schema-visible but document-inaccessible** — the same
shape as the security documents, and the second confirmed case where
`ped_read_document` is the reliable probe:

| tool | `ImportMappings$ImportMapping` / `ExportMappings$ExportMapping` |
|------|----------------------------------------------------------------|
| `ped_get_schema` (`element`) | **works** — full schemas for both documents and for `Import/ExportObjectMappingElement`, `Import/ExportValueMappingElement`, `Mappings$MappingMicroflowCall`, `Mappings$MappingMicroflowParameter`, `Mappings$ElementPath` |
| `ped_get_schema` (`constructor`) | **works**, but is `{ $Type, name }` only — no way to populate a created mapping |
| `ped_list_folder` | **works** — mappings are listed with their document type |
| `ped_check_errors` | **works, and genuinely validates mapping content** — a keyless `find` mapping is reported as `Object element must have a key defined if object handling is set to 'Search for an object'. (at locations: /rootMappingElements/0)`, i.e. mxbuild's CE0250 with a JSON-pointer location instead of a code |
| `ped_read_document` | `Unknown document type 'ImportMappings$ImportMapping'.` |
| `ped_find_document` | `Unknown document type 'ImportMappings$ImportMapping'.` |
| `ped_update_document` | `Document type ImportMappings$ImportMapping is not supported.` |

Controls, so the rejections are not a bad invocation: the same calls against
`Microflows$Microflow` and `JsonStructures$JsonStructure` succeed, and
`ped_update_document` against a **nonexistent** microflow returns
`Document '…' not found` rather than "not supported" — so the probe reaches
document resolution before failing.

`JsonStructures$JsonStructure` is **fully supported** (read returns `jsonSnippet`;
its constructor takes `name` + `jsonSnippet`), so a mapping's *source* document is
reachable over MCP even though the mapping is not.

**What the mapping schemas are worth even without document access.** They are
Studio Pro's own metamodel, and they pin enums that the on-disk BSON only implies:

```
objectHandling:       'Parameter' | 'Create' | 'Find' | 'Custom'   = "Create"
objectHandlingBackup: 'Create' | 'Ignore' | 'Error'                = "Create"
nullValueOption:      'SendAsNil' | 'LeaveOutElement'              = "LeaveOutElement"
elementType: 'Undefined' | 'Inheritance' | 'Choice' | 'Object' | 'Value'
           | 'Sequence' | 'All' | 'NamedArray' | 'Array' | 'Wrapper' = "Undefined"
```

This is how the mxcli defect in
[`PROPOSAL_mapping_coverage.md`](../11-proposals/PROPOSAL_mapping_coverage.md) §6.1
was confirmed — both writers set `ObjectHandlingBackup` to `Find`/`Parameter`,
neither of which is in the union. **`ped_get_schema` is therefore useful as an enum
oracle for types PED will not otherwise touch**, which is a cheaper check than
opening a written project.

**`ped_check_errors` is the second oracle, and the more decisive one.** It needs no
read access to the document — just module-qualified name plus type — so a mapping
authored on disk by mxcli can be validated by Studio Pro itself after a reload.
Measured 2026-08-24 on 11.13.0 with three mxcli-written mappings: the legal
`Find`/`Create` and the **off-enum `Find`/`Find`** both returned "No errors found",
while a keyless `find` in the same folder returned the CE0250 text above. So the
checker does inspect `/rootMappingElements/0` — the element carrying the off-enum
value — and tolerates it, and **the project opens**: an out-of-enum *value* is not
the "Sequence contains no matching element" failure that an unknown *property*
causes. Whether Studio Pro normalises the value to the declared default on load is
untested (it needs a save, and a save only rewrites units it considers changed).

Two practical notes for anyone repeating this. `ped_check_errors` reads the
**in-memory** model, so an on-disk write is invisible until the project is
reloaded — and reloading must not be preceded by a save, or Studio Pro's
in-memory model overwrites the write. And **PED reports no project path**: with
two sessions each holding a project named `PedApp.mpr`, `list_modules` was the
only way to tell which was open — worth a guard before any write.

Two naming notes, both instances of the storage-name split in `CLAUDE.md`: PED
speaks the **SDK** names (`ImportMappings$ImportObjectMappingElement`,
`mappingMicroflowCall`) where the BSON uses storage names
(`ImportMappings$ObjectMappingElement`, `CustomHandlerCall`). And
`Mappings$ElementPath` is an **opaque element** (`{ $Type }`, no properties), so
the `(Object)|a|b` mapping paths would not come through even if the document tools
accepted mappings.


## 11.14 changes (delta vs 11.13)

Captured live 2026-08-25 (`cmd/mcpprobe -method tools/list`, fixture
`mdl/backend/mcp/testdata/tools-11.14.json`). **Tool count and composition are
unchanged — the same 18 names, none added, none removed.** This is the first
release where a name-only diff would have reported "nothing happened", and it
would have been badly wrong: six tools changed their descriptions or input
schemas, `ped_update_document` gained an operation type, its **atomicity
guarantee was weakened**, and one long-standing capability gap closed. Diff the
`inputSchema` *and* the description text, or you will miss the release.

| Change | Tool | Effect on the backend |
|--------|------|-----------------------|
| **Changed** | `ped_update_document` | **Behavioural, three ways.** (1) A new `call` operation type invokes a schema-declared method on an element. (2) `set` now **replaces a non-null element-valued property** — 11.13 allowed it only while the property was null/undefined. (3) Atomicity is weaker; see below. No backend change is *required* (mxcli sends no `call` ops, and its `set` sites all target primitives or null elements), but (2) closes a gap — see the gaps table. |
| **Changed** | `ped_get_schema` | `kind: "element"` now also prints **methods** (`method name(args: PlainObject<{…}>): output`) and **validation constraints carrying a Mendix error code**. No backend change — `ensureSchema` still discards the body. |
| **Changed** | `ped_read_document` | Description-only: names `Settings$ProjectSettings` as a project-level document. **The server rejects it** — see the advertised-but-absent note below. |
| **Changed** | `ped_find_document` | Description-only: adds `Security$ModuleSecurity` to the "singleton, read it directly" carve-out beside `DomainModels$DomainModel`. **The server rejects it too.** |
| **Changed** | `glob` | Two new file domains: **`/version-control`** (`status.json`, `commits.json` with per-commit model changes, `changes.json` with pending changes) and **`/edc-schemas`** (live DB schema metadata for `DatabaseConnection` documents). Not used by the backend; `/version-control` is a plausible source for a future `--mcp` drift command. |
| **Changed** | `read_skill` | Skill inventory only: adds `version-control` and `send-email-common`. |

**No tool gained a new argument, and no default changed.** Measured across all 18
tools: no added or removed top-level `inputSchema` property, no changed `default`,
no changed `required` set. So 11.13's `pg_read_page` `depth` trap — a new argument
with a default that the server applies whether or not the client knows about it —
**did not recur**, and no new `Client.SupportsToolArg` gate is needed. This is a
measured negative, not an assumption; re-run the same scan next release.

**Atomicity was weakened, and the docs now say so.** 11.13: *"Operations are
validated and applied atomically; stops on first error."* 11.14:

> Operations are applied independently in the specified order … If an operation
> fails, no changes are saved. **However, removing, overwriting, or renaming
> elements can have side effects that persist even when another operation fails.**

Two consequences. First, a batch containing a remove/overwrite/rename is no longer
all-or-nothing, so a failed batch can leave the model partly mutated — the caller
has to re-read rather than assume the prior state. `navigation.go` already splits
its menu clear and rebuild into separate calls (PED's same-array rule forced that),
so no mxcli write path batches a remove with a dependent add; the stale claim was
in a *comment*, corrected in the same commit as this section. Second, `mxcli`'s
error surfacing improves for free — see below.

**Better error reporting (the visible half of the release).** Failures are now
reported per operation, with the operation's index and path, instead of stopping at
the first one:

```
ERROR: Update operation failed with errors (in format [<operation index>]<operationPath>: <output>):
'[0]/entities': Property 'entities' is an array. Use 'add' or 'remove' operation instead.
```

Schemas now carry the **Mendix error code** a violated constraint will raise, as the
second argument of the constraint type — so the CE number is knowable *before* the
write:

```
name: string & MinLength<1, 'CE0725'>;          // Workflows$Workflow
description: string & MinLength<1, 'MW0006'>;   // Workflows$WorkflowEventHandler
```

And an unknown element type now gets suggestions: `Pages$NoSuchWidget` →
`Did you mean: Pages$Widget, Pages$ClientAction, Pages$GridSortItem?`
`ped_check_errors` itself is **unchanged** — the improvement is in the write and
schema paths, not the checker.

**Three document types are advertised but not supported — do not "fix" the
capability notes from the tool text.** 11.14's descriptions and schemas name
`Settings$ProjectSettings`, `Security$ProjectSecurity` and `Security$ModuleSecurity`
as supported document types, and `ped_get_schema` returns full, richly documented
element schemas for all three. The document API rejects every one, with a control:

| `documentType` | `ped_update_document` result |
|---|---|
| `Navigation$NavigationDocument` (known-good control) | reaches the operation layer → `'[0]/zzzNoSuchProp': …` |
| `Settings$ProjectSettings` | `ERROR: Document type Settings$ProjectSettings is not supported.` |
| `Security$ProjectSecurity` | `ERROR: Document type Security$ProjectSecurity is not supported.` |
| `Security$ModuleSecurity` | `ERROR: Unknown document type` (on read/find) |

The control is what makes this a finding rather than a guess: the rejection happens
at the *document* layer, before any operation is looked at. So the "no security
document ops" and "no project-settings write path" gaps are **still open**, and
`capabilities.yaml` stays as it is. A reader who trusts the 11.14 tool text alone
would conclude the opposite.

**The system prompt no longer mentions the `pg_*` page tools at all.** The
"two write protocols" rule (pages via `pg_*`, everything else via `ped_*`) was
stated in 11.13's system prompt; in 11.14's it is gone — the tool list section
names only the eight `ped_*`/file tools. `pg_read_page` and `pg_patch_page` are
still in `tools/list`, and `ped_read_document` on a `Pages$Page` now returns a
full page document. Whether `ped_update_document` can *write* a page is
**untested** — it needs a real mutation, so measure it against a throwaway project
before moving the page path off `pg_*`. Until then the backend keeps using `pg_*`,
which still works.

**New system-prompt sections worth knowing** (they describe how Studio Pro's own
agent is told to behave, which is the best available guide to what the server will
tolerate): *Element methods* (how to invoke a `call` op), *Safe operations* vs
*Radical operations* (never remove elements not named in an error or the user's
query; never recreate documents from scratch), an *Error-Fixing Protocol* imposing
a single-fix rule, and *Version Control Guidelines*.

**Security: nothing opened, but one refusal's *reason* expired.** Security is the
area where 11.14's descriptions moved most and its behaviour moved least, so the
three layers have to be kept apart.

1. **The security documents are still sealed.** `Security$ProjectSecurity`,
   `Security$ModuleSecurity` → `Unknown document type` on read/find, `Document type
   … is not supported.` on update. Module roles, user roles, demo users and the
   project security level remain unauthorable over MCP. This is *despite* 11.14's
   tool text naming them (see the advertised-but-absent table above).
2. **`ped_get_schema` now returns ten fully documented security types** —
   `ProjectSecurity`, `ModuleSecurity`, `UserRole`, `ModuleRole`, `DemoUser`,
   `PasswordPolicySettings`, `FileDocumentAccessRuleContainer`,
   `ImageAccessRuleContainer`, plus `AccessRule`/`MemberAccess`. Schema *presence*
   is not new (11.13 already knew them as nested elements) and it is **not** a write
   path — do not read a schema as a capability.
3. **No security type declares a method**, so 11.14's new `call` operation opens
   nothing here.

**What did change is the in-place replace of an entity access rule.** mxcli refuses
to replace an existing `DomainModels$AccessRule` today, and the stated reason is
that replacing one *"would need member removal"*, which PED forbids. On 11.14 that
reasoning no longer holds: `set` can replace a non-null element outright, so a rule
can be overwritten **without removing anything**. Measured on
`/entities/0/accessRules/0`, three probes deep, each with a value that cannot land:

| probe | result |
|---|---|
| `set` a bogus `$Type` | `Expected type DomainModels$AccessRule, got DomainModels$NoSuchAccessRule` |
| `set` a well-formed rule naming a nonexistent role | `{"/moduleRoles/0":"Reference with qualified name MyFirstModule.NoSuchRole of type Security$ModuleRole not found."}` |
| re-read the rule afterwards | unchanged — still `MyFirstModule.User`, one `memberAccess` |

The second probe is the informative one: the operation clears the document layer
**and** shape validation, and fails only at *reference resolution*. Nothing but a
valid role stands between that call and a successful replace. The third is the
control proving the probes mutated nothing.

**REVOKE is a different question and is still unanswered.** Removing an
`AccessRule` or a `MemberAccess` could not be measured without actually deleting
one: an out-of-bounds index is rejected by the **bounds check before** any
removability check (`Index 5 is out of bounds for array 'accessRules' with length
1`), and routing the same op at a non-writable module is rejected by the
**writability check first** (`Module 'Administration' is not writeable.`) — both
mask the answer. So 11.13's `"Element of type … cannot be removed"` is **neither
confirmed nor refuted on 11.14**. It needs a valid-index remove against a throwaway
project. Note that `set` cannot substitute: it may replace an element but *never*
target an array, so there is no removal-free spelling of REVOKE.

The practical upshot: on 11.14 **re-GRANT (updating an existing rule) looks
reachable while REVOKE does not**, which is the opposite of how the two are paired
in `security.go` today — both are refused there for the same reason. `capabilities.yaml`
is unchanged because mxcli's *behaviour* is unchanged; what expired is the
justification, and acting on it needs the throwaway-project measurement first.

**Gap re-check, measured live on 11.14.** Still **no delete-document tool**, still
**no save/flush tool**, reads still expose `$QualifiedName` but **not `$ID`**, the
security documents are still sealed, and the `ped_create_document` whitelist still
rejects `Microflows$Nanoflow` (*"Did you mean: Microflows$Microflow?"*),
`Projects$Folder` and `JavaActions$JavaAction`. Two gaps moved — see the gaps table
for both.

### Workflows: constructor shape, AI agent tasks, handlers (measured live 2026-09-15)

Captured against Studio Pro 11.14 with ako/TestApp open, via `cmd/mcpprobe`
(`ped_get_schema`, `ped_create_document`, `ped_check_errors`, `ped_read_document`).

- **The `Workflows$Workflow` constructor changed shape.** It takes the context
  entity as `context: Reference<'DomainModels$Entity'>` and `workflowName` /
  `caption` as plain strings, and rejects the older payload outright:
  `{"/context":"Expected reference (string), got undefined","/workflowName":"Expected
  string, got object"}` — so **every workflow create over MCP failed** on 11.14.
  The *element* shape used by updates (`/parameter/entity`, `/workflowName/text`) is
  unchanged. Which release made the change is not established (11.13 was not
  re-probed), so the backend reads the constructor schema instead of gating on a
  version (`workflowConstructorTakesContext`).
- **`Workflows$AIAgentTaskActivity`** has the call-microflow fields — `name`,
  `caption`, `microflow`, `outcomes`, `parameterMappings`, `boundaryEvents` (timer
  events, `MaxLength<5, 'CE6696'>`). Created and read back clean.
- **`onWorkflowEvent`** takes `Workflows$WorkflowEventHandler` (`description`,
  `documentation`, `eventTypes`, `microflowEventHandler{microflow}`); a user task's
  `onCreatedEvent` takes `Workflows$MicroflowBasedEvent{microflow}`. The MCP mapper
  used to send `NoEvent` and no handlers, silently dropping both.
- **`eventTypes` is a schema enum of the same 42 names** mxcli validates, and PED
  refuses an invented one at create (`Expected one of [WorkflowCompleted, …]`) —
  where mxbuild accepts it at 0 errors.
- **Signature rules match mxbuild, message for message:** an on-created microflow
  with the handler signature and a handler with the on-created signature each
  report the CE6683 / CE6691 text; an agent task without parameter mappings reports
  "The parameters of the selected microflow have changed".
- **`description` is typed `MinLength<1, 'MW0006'>`**, but a handler with an empty
  one produced no MW0006 in `ped_check_errors` output — inconclusive whether the tool
  omits warnings.
- **The error-list lag bites here too:** both probe documents first checked "No
  errors found." and reported their errors only on a later call.

### Microflows: the constructor is a canvas skeleton (measured live 2026-09-15)

- **The `Microflows$Microflow` constructor changed shape, and every microflow create
  over MCP failed on 11.14:** `{"/flows/0/$Type":"Expected an element with $Type
  property.","/returnType":"Expected one of [Void, Boolean, …], got
  {\"type\":\"Void\"}"}`. Flows are now `$Type`d elements (`Microflows$SequenceFlow`
  / `Microflows$AnnotationFlow`), `returnType` is a bare enum with
  `returnTypeEntity` / `returnTypeEnumeration` beside it, and parameters go in a
  separate `parameters` list.
- **The two rejected keys are not the whole change.** Every object constructor
  (`StartEvent`, `EndEvent`, `ActionActivity`, `ExclusiveSplit`, `LoopedActivity`, …)
  now declares only `x`/`y` plus a `caption` or `loopType`. A create that fixes the
  flows and return type but still sends `relativeMiddlePoint`, `action`,
  `returnValue` or a split/loop source **succeeds with those silently dropped**:
  positions read back 0,0, `"action": null` ("No action defined"), an empty return
  value. Only a read-back shows it.
- **Behaviour is set by path-ops after the create**, and they round-trip:
  `set` on `/objectCollection/objects/N/action`, `…/returnValue`, `…/splitCondition`
  (`Microflows$ExpressionSplitCondition{expression}`) and `…/loopSource`
  (`Microflows$IterableList{listVariableName, variableName}`), nesting through a
  loop's `objectCollection`. Stored parameters occupy the first object slots, so `N`
  counts them while the constructor's `$id(/objects/N)` does not.
- The backend detects the shape from the constructor schema text (`returnTypeEntity`)
  rather than a version (`microflowConstructorTakesSkeleton`), and on a skeleton server
  sends the canvas in the create and one `ped_update_document` with the rest
  (`adaptMicroflowSkeleton`). Verified live: a trivial microflow, Boolean, entity and
  list returns, and a parameter + split + loop microflow all created with
  `ped_check_errors` clean on two successive calls, and read back with positions,
  actions, conditions, loop sources and return types intact. The while-loop source
  (`Microflows$WhileLoopCondition`) is mapped from its schema but was not exercised live.

## Capability gaps (established 11.11, status re-checked each release)

These are the *absences* that bound what the backend can do. They are as
important as the tools that exist — re-check each when onboarding a new version,
since a gap closing (e.g. a delete or save tool appearing) unlocks features.
The right-hand column carries the latest measured status, not the 11.11 one.

| Gap | Consequence for the backend | Status to recheck each version |
|-----|-----------------------------|-------------------------------|
| **No delete-document tool** | `DROP` of any *standalone document* (enum, microflow, page) is impossible. Only entities/associations delete, via a `ped_update_document` remove op on the domain-model array. Test docs created via MCP cannot be cleaned up — they persist until the user closes Studio Pro without saving. | Watch for a `ped_delete_document` / equivalent. |
| **No list-modules tool** ⟶ **CLOSED in 11.12** | PED cannot enumerate modules. The backend must read modules/structure from the local mounted `.mpr` (hybrid model); `-p` must be the same project Studio Pro has open. | **11.12 adds `list_modules`** (`[{moduleName, writable, fromMarketplace}]`) → pure-MCP module enumeration is now possible; the local-`.mpr` dependency for *module listing* can be lifted on 11.12+. Other reads (structure, IDs) still use the hybrid model. |
| **No save/flush tool** | `ped_update_document` edits stay in Studio Pro's in-memory model; the on-disk `.mpr` is stale until the user saves. Drives the dirty-set read router. **Refined on 11.12 (observed live, test8):** *created* documents (`ped_create_document`, `pg_patch_page` page-create) flush to `mprcontents/` immediately — no Concord `save_all` needed for them — but *update* ops still stay in memory (an `alter entity add attribute` was visible to `ped_check_errors` yet absent on disk → CE1613 on an `mx check` of a mid-session disk copy). (`ped_create_module` already flushed immediately on 11.11.) | Watch for a save tool / autosave; recheck the create-flush behavior each version. |
| **Reads omit `$ID`** | Reads expose `$QualifiedName` only. Association refs need entity GUIDs, recovered from the local reader (= live `$ID` for saved entities). Reconstructed reads use synthetic IDs mapped to names. | Recheck whether reads expose `$ID`. |
| **Array reads omit primitive types** | A `/entities/N/attributes` read gives attribute names (`$QualifiedName`) but not their primitive type or documentation — those need a per-*leaf* read (`/entities/N/attributes/M/type` → a `DomainModels$*AttributeType` constructor; `/…/documentation` → string). Reconstruction recovers them with a single **batched** leaf read (`enrichReconstructedEntities`) so a dirty/session module reports real types (correct DESCRIBE + a reliable ALTER diff); it falls back to placeholder `String` only if the read fails. | **11.14 — partially CLOSED.** An attributes-array read now carries each attribute's `type.$Type`, `value.$Type` and `documentation` inline (`{"$Type":"DomainModels$Attribute","$QualifiedName":"…Title","type":{"$Type":"DomainModels$StringAttributeType"},"value":{"$Type":"DomainModels$StoredValue"},"name":"Title","documentation":""}`). But the type's **parameters are still dropped**: the same attribute's leaf read returns `{"$Type":"DomainModels$StringAttributeType","length":200}` — `length` appears **only** in the leaf read. So `enrichReconstructedEntities` is still required for `length` / enumeration refs, but the type *kind* is now free. Not yet exploited; measured on one String attribute, so widen the sample before relying on it. |
| **No in-place attribute-type change** | `set /entities/N/attributes/M/type` is rejected (`"only allowed to set primitive or reference properties directly"` — the type is a nested element), and a whole-attribute replace is rejected too (`"only allowed to update elements …"`). The only route to a new type is remove+add, which drops the attribute's `$ID` → **drops the column data**. So `ALTER ENTITY … MODIFY ATTRIBUTE <type>` is rejected over MCP; do it against a local `.mpr` (Studio Pro does an in-place migration PED can't). | **11.14 — the structural refusal is GONE.** 11.14's `set` explicitly *"can set or replace element-valued properties"*. Probed with a deliberately invalid value so it could not land: `set /entities/0/attributes/0/type` = `{"$Type":"DomainModels$NoSuchAttributeType"}` is now rejected as a **value validation** error (`"Type … is not a valid concrete subtype of DomainModels$AttributeType."`), not with 11.13's structural *"only allowed to set primitive or reference properties directly"*. That means the operation now reaches validation — i.e. a **valid** subtype would plausibly be accepted. **Not confirmed:** proving it needs a successful mutation against a throwaway project, and the data-safety question behind the gap (does an in-place type change migrate the column, or drop it?) is unanswered and is the *real* gate on `ALTER ENTITY … MODIFY ATTRIBUTE`. `capabilities.yaml` therefore still reports it unauthorable. |
| **Two write protocols** | Pages **must** use `pg_*`; everything else uses `ped_*`. The system prompt forbids PED for pages. | Stable, but reconfirm. |
| **`ped_create_document` doc-type whitelist** | The create tool accepts only certain document types; some model documents are rejected even though they have a `$constructor` schema. Confirmed off the whitelist: **`Microflows$Nanoflow`** (`"… Did you mean: Microflows$Microflow?"`), **`Projects$Folder`** (empty folders), and **`BusinessEvents$BusinessEventService`** (a `$element`, not a `$constructor` — its CREATE/ALTER are rejected in `businessevent.go`, but SHOW/DESCRIBE read the `.mpr`, DROP goes via Concord, and the published-event entities/constants it relies on are creatable). So nanoflows aren't creatable over MCP (CREATE/ALTER rejected — `nanoflow.go`; DROP works via Concord), and an *empty* folder can't be created. **But `ped_create_document` accepts a `folderPath` per document, which auto-creates the whole folder path** — so a document is placed in a (possibly nested) folder at create time (`folder.go`/`resolveDocContainer`). What you can't do: re-parent an *existing* document — `/folderPath` is not a settable property (so `MOVE`/`DROP FOLDER`/`MOVE FOLDER` are rejected), and pages can't be foldered (`pg_write_page` takes no folderPath). | Re-probe the create whitelist each version. |
| **No Java action document creation** | `ped_create_document` rejects `JavaActions$JavaAction` outright (`"Document type 'JavaActions$JavaAction' cannot be created."`) — a Java action is backed by a `.java` source file Studio Pro generates/manages, so the model document can't be created standalone. `CREATE/ALTER JAVA ACTION` are rejected with an actionable error (`mdl/backend/mcp/javaaction.go`); *calling* a Java action from a microflow is unaffected. | Watch for a Java-action create tool. |
| **No mapping document ops** | PED's `ped_read/find/update_document` reject `ImportMappings$ImportMapping` and `ExportMappings$ExportMapping` ("Unknown document type" / "not supported"), while `ped_get_schema` returns full `element` **and** `constructor` schemas and `ped_list_folder` lists them — the security-document shape exactly. The constructor is `{ $Type, name }` only, so even a whitelisted create would yield an empty shell with no way to populate it. mxcli's MCP backend matches: all ten mapping methods are `unsupportedBackend` stubs (`unsupported_gen.go`). `JsonStructures$JsonStructure` — the mapping's *source* — **is** fully supported, and `ped_check_errors` **does** accept a mapping by name, so an on-disk-authored mapping can still be error-checked by Studio Pro. Re-probed 11.13.0, 2026-08-24. | Watch for the mapping types appearing in `ped_read_document`. |
| **No security document ops** | PED's `ped_find/read/update/create_document` reject every security type (`Security$ModuleSecurity`, `Security$ProjectSecurity`, …) as "Unknown document type" — only `ped_get_schema` knows them as nested *elements*. Concord exposes only security *reads* (`audit_security`, `read_entity_access_rules`, `read_microflow_security`, `read_security_info`). So the security **documents** cannot be authored via MCP (module/user roles, demo users, project security level) — neither server has a write path. **Entity access rules are the exception and are authorable**: a `DomainModels$AccessRule` lives on the domain model, not the security document, so `GRANT` works (ADD-only — PED refuses to remove a rule or a member access, so `REVOKE` and replacing a rule in place are rejected). Security **reads** are served from the local `.mpr` by the backend's reader, so `SHOW MODULE ROLES` / `USER ROLES` / `PROJECT SECURITY` work over `--mcp` and `GRANT` can validate the role it references — routing that read to PED instead is what made every `GRANT --mcp` abort before its first tool call (#900). Determine support with `ped_read_document`, NOT `ped_find_document`: `find` also reports the (supported) nameless `DomainModels$DomainModel` as "Unknown", so it is not a reliable probe. | **11.14 — still sealed, and now actively mis-advertised.** The tool descriptions name `Settings$ProjectSettings`, `Security$ProjectSecurity` and `Security$ModuleSecurity` as supported and `ped_get_schema` returns ten fully documented security element types, but every document-API call is refused (`Unknown document type` / `Document type … is not supported.`), against a `Navigation$NavigationDocument` control that reaches the operation layer in the same probe. No security type declares a **method**, so the new `call` op opens nothing. **One thing did move:** replacing an existing `DomainModels$AccessRule` in place no longer needs member removal — 11.14's `set` replaces a non-null element, and the probe clears shape validation and fails only at reference resolution. So re-GRANT looks reachable while **REVOKE is still unmeasured** (bounds and writability checks both mask removability). See the 11.14 delta for the probes and the control. |

## Transport (per environment, not per version)

The server binds **IPv6 loopback only** (`[::1]:7782` observed) and enforces a
DNS-rebinding guard: the `/mcp` route requires HTTP `Host: localhost` (bare, no
port). From a devcontainer:

- Some sessions are reachable directly at `host.docker.internal:7782`.
- Otherwise bridge on the **host**. Where `host.docker.internal` resolves to an
  **IPv6** address (confirmed on the 11.13 Mac host: `fdc4:f303:9324::254`), a
  `TCP4-LISTEN` bridge is not reachable from the container — the listener must
  accept both families:

  ```bash
  socat TCP6-LISTEN:7790,reuseaddr,fork,ipv6only=0 'TCP6:[::1]:7782'
  ```

  Then dial `host.docker.internal:7790`. Quote the target: zsh glob-expands the
  bare `[::1]` and fails with `no matches found`.

**Finding the port on 11.13+.** Studio Pro now **auto-selects a free port** when the
default is taken, and shows the active one in the **status bar** (Preferences > AI >
MCP Server also configures it). There is no documented file or endpoint exposing it,
so the port is a per-session lookup. A scan is a workable fallback — the server
answers `initialize` with `serverInfo.name: mendix-studio-pro`, which identifies it
unambiguously among other listeners.

`cmd/mcpprobe` and the backend client pin the dial target while keeping the
`Host` header `localhost` (`-url http://localhost/mcp -dial host.docker.internal:<port>`).
The port can change between Studio Pro sessions — confirm with `lsof` on the host.

## How the mxcli MCP backend uses this surface

Implemented (11.11): `CREATE MODULE`, `CREATE/ALTER/DROP ENTITY`,
`CREATE/DROP ASSOCIATION`, `CREATE ENUMERATION`, `CREATE/ALTER/DROP CONSTANT`,
`CREATE VIEW ENTITY`, `CREATE
MICROFLOW` (broad activity + control-flow coverage), `CREATE PAGE` + `ALTER PAGE`
(INSERT/DROP/REPLACE/SET property/DataSource/Layout), and `CREATE WORKFLOW` +
`DROP WORKFLOW` + `ALTER WORKFLOW`, with a dirty-set read router that makes
in-session edits visible.

**CREATE/ALTER CONSTANT** maps onto `Constants$Constant` ({name, type,
defaultValue, exposedToClient}). Two PED-shape facts: the constructor `type` is a
plain enum string limited to **String / Integer / Decimal / Boolean / DateTime**
(Long / Enumeration / Binary are rejected, not coerced), and there is **no
documentation field** (dropped). CREATE OR MODIFY sets the `defaultValue` and
`exposedToClient` leaves in place; a *type* change is rejected because the model's
`type` is a nested `DataTypes$*Type` element PED can't set directly (same constraint
as an attribute's type — `UpdateConstant` reads the live type and refuses a
mismatch). DROP goes through Concord's `delete_document` (no PED delete tool), like
enumerations.

**CREATE MICROFLOW** maps a broad set of actions (variable/object/list changes,
create/commit/delete/rollback, aggregate, list operations, retrieve, call
micro/nano/java/**javascript** action, log/validation/download/close-page,
**show home page**) and list operations (head/tail/filter/find/sort/union/
intersect/subtract, **contains**, **equals**). Some actions can't be expressed
through PED's *simplified* action constructors and are rejected rather than
mis-built: **show page** (the constructor omits the target page — pages go through
`pg_*`), **cast** (the `CastAction` constructor exposes only `outputVariableName`,
not the input variable or target type), and **retrieve sorting / custom range** and
the **list-range** operation (PED's `byDatabaseQuery` input is `entity` +
`xPathConstraint` + `takeOnlyFirst` only, and `Microflows$Range` exposes no
settable fields). These would need a post-create element update PED's simplified
constructors don't support.

On 11.14 the create itself is two-phase: the constructor carries only the canvas and
each object's action, return value, split condition and loop source are `set` by a
follow-up `ped_update_document` (see *Microflows: the constructor is a canvas
skeleton* under the 11.14 changes). Whether that path-op route also opens the
rejected actions above has not been probed.

**ALTER ENTITY** diffs the executor's rebuilt entity against the live model
(name-keyed) and routes by the diff's shape: adds-only → ADD ATTRIBUTE, removes-only
→ DROP ATTRIBUTE, one-add-one-remove → **RENAME** (set the `name` leaf in place,
preserving the attribute's `$ID`/column data), and no structural change → in-place
**entity & attribute documentation** (primitive-property sets). An attribute
**type** change is rejected (see the type-change gap above) rather than silently
no-op'd; reliable detection depends on the enriched reconstruction carrying real
types, so a documentation edit on a dirty module is never mistaken for a type change.

`CREATE WORKFLOW` maps the executor's workflow onto `Workflows$Workflow`
(parameter + a linear `Workflows$Flow` of activities Start … End, connected via
condition outcomes that may nest sub-flows). Activity-type coverage: Start/End,
**CallMicroflow** (outcomes), single **UserTask** (task page, XPath/Microflow user
targeting, task name/description, named outcomes each with a recursive sub-flow),
**Decision** (ExclusiveSplit: expression + boolean/enum outcomes), **ParallelSplit**
(concurrent paths), **JumpTo**, **WaitForTimer**, **CallWorkflow** (sub-workflow
ref + parameter mappings — PED binds `$WorkflowContext` implicitly, so there is no
`parameterExpression` field), **WaitForNotification**, **MultiUserTask**, and
**boundary events** (interrupting/non-interrupting timers, with handler sub-flows)
on user-task and call-microflow activities. **MultiUserTask gotcha:** unlike the
single variant, its `pageReference` is a bare string (not a `taskPage` element),
`participiantInput` is required (defaulted to `AllTargetUsers`), and its `outcomes`
are plain value strings — so a multi-task outcome carries no per-outcome sub-flow
(one with activities is rejected rather than silently dropped). Still rejected:
AIAgentTask, annotations, and other niche activity types (clear error each). **Type-name gotcha:** PED's element type for the call-microflow
activity is `Workflows$CallMicroflowActivity`, NOT the on-disk BSON `$Type`
`Workflows$CallMicroflowTask` — they differ; use `ped_get_schema
Workflows$WorkflowActivity` to enumerate the valid concrete subtypes. Workflow
documents are addressed by qualified name (`<module>.<workflow>`), not a bare
name. Entity references into a session-created module (a workflow's context
entity, a microflow parameter) resolve because `ListDomainModels`/`GetDomainModel`
reconstruct the session module's live entities from PED.

**ALTER WORKFLOW** uses `ped_update_document` path operations (NOT the page-style
read-modify-whole-tree, because `ped_read_document` collapses nested workflow
elements to their `$Type`). Implemented: workflow-level SET — `display` →
`/workflowName/text` + `/title`, `description` → `/workflowDescription/text`,
`due_date` → `/dueDate`, `parameter` → `/parameter/entity`. Note nested template/
parameter elements must be set by their leaf field (`/workflowName/text`), not
replaced wholesale (PED rejects a whole-element set). Activity-level structural ops are wired via ref→index resolution (a shallow
`/flow/activities` read matching caption/name, with `@N` disambiguation, top-level
activities only): **INSERT** activity (ped add at index+1), **DROP** activity (ped
remove at index), **REPLACE** activity (remove the slot, then add — a whole-element
set by index is rejected). The outcome/path/branch/boundary-event ops follow the
same array add/remove pattern one level deeper — on the activity's nested
`/flow/activities/<i>/outcomes` (user-task outcomes, decision branches, parallel
paths all share this array, differing only by element `$Type`) or
`/flow/activities/<i>/boundaryEvents`; DROP reads the array to resolve the
match→index first. **SetActivityProperty** sets primitive/reference leaves
(`page` → `/taskPage/page`, `description` → `/taskDescription/text`, `due_date`);
changing the *kind* of user targeting (XPath↔Microflow) is rejected because PED
can't replace the nested element. Activity references resolve **anywhere in the
flow tree**, not just the top level: `resolve` walks the flow depth-first (each
activity, then its outcome sub-flows, then its boundary-event sub-flows — the same
order as DESCRIBE, so `@N` numbering matches), reading each `…/outcomes` /
`…/boundaryEvents` array (an outcome read exposes a collapsed `flow` field that
signals a sub-flow to descend into) and returning the matched activity's *containing
array path*. Every op then targets that path — e.g. `insert after NestedCall` adds
into `/flow/activities/1/outcomes/0/flow/activities`, not the top level. So
`ALTER WORKFLOW` is **complete** over MCP: workflow-level SET, all activity-level
structural and property ops, on activities at any nesting depth.

**CREATE OR REPLACE WORKFLOW** rewrites an existing workflow in place via
`ped_update_document` (PED has no document-replace tool), preserving the `$ID`.
The workflow keeps its structural Start/End (PED refuses to remove either, and an
index-less `add` lands *after* End in the array), so only the *middle* activities
are swapped: the originals (indices 1..n-2) are removed, then the new middles are
inserted just after Start. Each insert targets **index 1 in reverse order** — an
explicit `add` index is validated against the array's *original* length (so an
incrementing index can't grow the flow), but index 1 is always valid; reverse
insertion leaves the middles in sequence. Workflow-level properties are set as in
ALTER (title/workflowName/description/dueDate/documentation/parameter). Verified
live (1-middle → 2-middle replace → `[Start, NewB, NewC, End]`, validates clean).

`CREATE MODULE` routes through `ped_create_module` (which flushes to disk
immediately) and registers the module in a session list merged into
`ListModules`/`GetModule(ByName)`, so a later op in the same run — e.g. `create
module X; create enumeration X.Y`, or `create module X; create entity X.Foo` —
resolves the freshly created module (the local reader does not yet know about it).
For entities, `GetDomainModel` returns an empty **synthetic** domain model for a
session module (ID `mcp~dm~<module>`, which `moduleNameForDomainModel` decodes
back to the module name) since the reader has no on-disk domain model to read.
**Quirk:** a module created via `ped_create_module` lags briefly before
`ped_update_document` can mutate it (errors `Module ... not found` even though the
create flushed), so `pedUpdateDoc` retries on that transient with a short backoff. The standalone-doc create paths
(enumeration/page/microflow) resolve their module via `GetModule` (session-aware)
rather than the reader directly. Note `ped_create_module`'s success text is
`"Module 'X' created successfully."`, NOT the `SUCCESS`-prefix the document ops
use, so it has its own success check (contains "success").

**ALTER PAGE** is a read-modify-write on the pg tree: `OpenPageForMutation` loads
the page via `pg_read_page`, the mutator edits the in-memory tree, and `Save()`
writes it back via `pg_patch_page` (a root-replace patch). Supported in-place ops:
INSERT (before/after a widget), DROP widget, REPLACE widget, SET DataSource, SET
Layout, **SET widget property** (`set (prop = value, …) on <widget>`), and
**page-level SET Title** (`set (Title = '…')` with no `on` clause) — plus the
introspection the executor needs (FindWidget, WidgetScope, ParamScope,
EnclosingEntity). The widget ref is the widget name (recursive tree search). The
executor passes the AST position token (`"AFTER"`/`"BEFORE"`), so the mutator
compares case-insensitively. SET maps the MDL property name (also
case-insensitive) to its pg key: Class/Style → the widget's `appearance`;
Caption/Content/Label → the `ct:`-prefixed client templates; ButtonStyle → pg's
normalized enum; TabIndex/RenderMode/Editable/Name → direct keys; Visible → a
conditional-visibility expression. Page-level SET Title maps onto the LightPage's
top-level `title` string. Not yet mapped: page-level Url and pop-up settings
(PopupWidth/Height/Resizable/CloseAction) — the LightPage schema is
`additionalProperties:false`, so they aren't on the pg shape and are rejected
before `Save()` (set them in Studio Pro); also unknown SET properties, column
INSERT/REPLACE/property, design properties, pluggable-property SET, and page
variables — each returns a clear error.

Pages use a **separate protocol**: `pg_patch_page` / `pg_read_page` (PED is
forbidden for pages). The backend maps the executor's `pages.Page` (shell +
LayoutCall slots + widget tree) onto the high-level page content. Note pg's
container type is **`Pages$DivContainer`** (not `Container`); page reads
(layouts/snippets/folders) delegate to the local reader because the executor
resolves the layout through the container hierarchy. Validation success is
signalled by a result text containing "success" (not the PED "SUCCESS"-prefix
convention), and — unlike PED — there is **no pg validation tool**, so a bad
attribute/page reference still writes "successfully" but shows a CE error in
Studio Pro.

Widgets so far: DivContainer, LayoutGrid/Row/Column, TabContainer/TabPage,
ActionButton, DynamicText, DataView, ListView, TextBox, TextArea, CheckBox,
RadioButtonGroup, DatePicker (+ No/Microflow/Page/CreateObject client actions;
page-variable / direct-entity / database / microflow data sources). Button styles
are normalized to pg's canonical enum (`primary` → `Primary`); an unknown style
falls back to `Default` (pg rejects unknown values). A DataGrid 2 control bar is
just the `filtersPlaceholder` slot holding action buttons. TextArea and the executor's
RadioButtons (→ `Pages$RadioButtonGroup`) are attribute-bound inputs that share
the same minimal `attributeRef` + `ct:labelTemplate` shape as TextBox; the server
fills in the rest of their defaults (rows, render direction, placeholder, …),
which are not yet mapped. **Conditional visibility** (`visible: [xpath]`, i.e.
VISIBLE IF) maps onto a `Pages$ConditionalVisibilitySettings {expression}` and
is attached uniformly to every mapped widget; the MDL `visible:` property only
ever produces an expression, so module-role / attribute / source-variable
conditions are not mapped. (The static `visible: false` form sets a separate
`Visible` value, not conditional visibility, and is not yet mapped.) Tab-page
captions use the `t:caption` key (a plain
string the server wraps in `Texts$Text`), not the `ct:` ClientTemplate prefix
that button captions use. pg's widget
union (from the tool schema) is the limit of native support: ActionButton,
CheckBox, Content, DataView, DatePicker, DivContainer, DynamicText, LayoutGrid/
Row/Column, ListView, RadioButtonGroup, TabContainer/TabPage, TextArea, TextBox,
plus `CustomWidgets$CustomWidget` (pluggable). **No `Pages$DataGrid`** — the
legacy DataGrid is rejected; DataGrid 2 is a pluggable custom widget. Coverage
grows one widget/data-source type at a time.

**Pluggable widgets.** The reference/dropdown selector — the Mendix 11
ComboBox (`com.mendix.widget.web.combobox.Combobox`) — is supported in both
enumeration and association modes. Crucially, the MCP path does *not* build the
BSON widget template the MPR writer must: it implements `LoadWidgetTemplate` with
an `mcpWidgetBuilder` that records the engine's semantic property operations
(`SetAttribute`/`SetAssociation`/`SetPrimitive`/`SetDataSource`) into a high-level
pg `object`, and Studio Pro expands every default on `pg_write_page` (37 props
filled from ~5 for ComboBox; 34 object + 19/column for DataGrid 2). **This
sidesteps the entire CE0463 "widget definition changed" template-mismatch class
of bugs** that the on-disk BSON writer hits, because the server owns
serialization. One ComboBox quirk: the def.json enum mode maps only
`attributeEnumeration` (the MPR template carries `optionsSourceType`'s default),
so the MCP backend infers `optionsSourceType: "enumeration"` — otherwise pg
defaults it to `association` and prunes the enum binding.

**Acceptance is registry-driven, not a curated list** (Phases 1+2 of
`PROPOSAL_mcp_pluggable_widget_authoring.md`, live-validated on 11.12): any
widget the executor's shared registry resolves (project `.mxcli/widgets/*.def.json`
→ global → embedded — including defs produced by `mxcli widget extract`) is
authorable over MCP. `mdl/backend/mcp/widgets.def.json` is demoted to an
auto-datasource *hint* table (which property of a built-in widget is its
DataSource, so the engine's auto-datasource pass fires for e.g. DataGrid 2); it
is still MCP-owned and **deliberately not** merged into the shared widget
registry, so it cannot change the MPR datagrid path. A widget absent from the
hint table is still authorable — it just maps its datasource explicitly via its
def.json. The builder translates the shared engine's storage-agnostic calls:
- `SetDataSource` → `CustomWidgets$CustomWidgetXPathSource` (DataGrid 2 reaches it
  via auto-datasource, which reads the DataSource property `PropertyTypeIDs`
  reports from the def; ComboBox/Gallery map it explicitly in their shared
  def.json). A `sort by` clause becomes the `Pages$GridSortBar` (`sortItems` with
  `attributeRef` + `sortDirection`), and a `where [...]` clause becomes the
  source's `xPathConstraint`. (Page datasources have no grouping concept.)

`sort`/`where` are supported wherever the **official metamodel** has a source
type that carries them (verified against the `modelsdk` branch's generated
types): `GridXPathSource` (= pg `CustomWidgetXPathSource`, DataGrid 2 / Gallery /
association ComboBox) and `ListViewXPathSource` (pg `Pages$ListViewXPathSource`,
list views with a database source) both have `xPathConstraint` + `sortBar`. A
**DataView**, by contrast, has *no* XPath source type — `DataViewSource` is
context/parameter-only (`pageParameter`/`snippetParameter`/`entityRef`) — so a
constraint/sort on a data-view database source is correctly rejected. This is a
metamodel fact, not a pg limitation: emit the right source `$Type` and pg expands
it. (List-view database sources must use `Pages$ListViewXPathSource`, NOT
`Pages$DataViewSource`, or pg drops the constraint/sort.)
- `SetObjectList` → generic object-list items (DataGrid 2 `columns`): operation
  kind → pg shape, text-template keys take pg's `ct:` prefix.
- `SetChildWidgets` → Widgets-typed slots (Gallery `content` template), mapped
  recursively through the normal widget mapper so nested pluggable widgets and
  conditional visibility work inside a slot.
- An object-list item's own Widgets-typed sub-slots are mapped recursively too,
  which gives **DataGrid 2 column filters** (`textfilter`/`numberfilter`/
  `datefilter`/`dropdownfilter` → the column's `filter` slot) and custom-content
  cells (the column's `content` slot). The filter widgets are added to
  `widgets.def.json`. Their def.json always sets `attrChoice: "auto"`, under which
  Studio Pro auto-binds the filter to the column attribute and rejects a non-empty
  `attributes` list — so `SetAttributeObjects` is a deliberate no-op (emitting the
  derived attribute would drop the widget).

- `SetExpression` → plain string key in the custom-widget `object` (Phase 2,
  verified live: ComboBox `customEditabilityExpression`).
- `SetTextTemplate` → `ct:`-prefixed plain string; Studio Pro expands it to a
  full `Pages$ClientTemplate`. `SetTextTemplateWithParams` rewrites `{AttrName}`
  placeholders to `{1}` refs backed by `Pages$ClientTemplateParameter`
  `attributeRef`s resolved against the entity context (the shared engine routes
  placeholder-bearing def-mapped texttemplates here — emitted literally, the
  braces fail Studio Pro's translatable-text parser).
- `SetAction` → `Pages$NoClientAction` / `Pages$MicroflowClientAction` /
  `Pages$PageClientAction` via `customWidgetClientAction`. **Shape deviation
  from the native LightPage constructors:** inside a custom widget's `object`,
  the microflow reference must nest in `microflowSettings` — pg *silently
  drops* a flat `microflow` key there (verified live: flat → "Select a
  microflow"; nested → clean). Actions with parameter mappings, and other
  action kinds (save/cancel/close/delete/create/open-link/nanoflow), are
  rejected loudly — their pg value shapes are not yet pinned.

The authoritative pg shape sources are published **by the server itself**:
`read_skill` (`page-gen-common` + `references/common-objects.md` /
`references/actions.md`) and per-widget schemas at
`/pagegen/customWidgetsVFS/<widgetId>.schema.json` (via `glob`/`read_file`).
Consult these before probing new shapes. Widget-specific gotcha class: pg
prunes properties made irrelevant by a selector primitive's default — the
ComboBox `optionsSourceType` quirk above, and the Image widget dropping
`ct:imageUrl` unless `datasource` is set to `"imageUrl"` (MDL:
`ImageType: 'imageUrl'`).

Client templates with `{N}` parameters (common in Gallery/DataGrid cells) emit a
full `Pages$ClientTemplate` with `attributeRef`/`expression`/`sourceVariable`
parameters — otherwise the literal `{1}` would show. DataGrid 2 columns with
custom-content child widgets or parameterised header templates, and any property
op the builder doesn't translate, are rejected, not silently emitted with
missing content. The broader consolidation (removing the hardcoded Go maps in
`widget_defs.go` and migrating the MPR path to def.json) is deferred until after
the engine replacement merges.

Data sources for DataView/ListView: page-variable (`Pages$PageVariable`),
direct-entity (`DomainModels$DirectEntityRef`), and **microflow**
(`Pages$MicroflowSource` wrapping `Pages$MicroflowSettings {microflow,
parameterMappings:[], outputMappings:[], progressBar:"None", asynchronous:false,
formValidations:"All"}`). Microflow sources with parameter mappings, and
database sources with XPath/sorting, are not yet mapped.

Microflow support is now broad: name, parameters, return type, and a recursive
object/flow graph (positions reused from the executor's layout engine, so the
MCP-authored canvas matches the file-written one). Supported activities:
declare/set variable; create/change/commit/delete/retrieve/rollback object;
create list, change list, aggregate, list operations (head/tail, filter/find by
expression or attribute, sort, union/intersect/subtract); show message; log;
call microflow / nanoflow / java action; download file; close page; validation
feedback. Control flow: if/else ExclusiveSplit + ExclusiveMerge, for-each/while
LoopedActivity + break/continue. Rejected (PED can't express them faithfully):
show page (constructor omits the page ref — pages are pg_*), cast, inheritance
splits, rule-split conditions, contains/equals/range list ops, queue settings.

View-entity choreography (verified): `ped_create_document
DomainModels$ViewEntitySourceDocument {name}` → `ped_update_document` set
`/oql` → entity add with `source: {OqlViewEntitySource, sourceDocument:
"<qualified>"}` and each attribute carrying `value: {OqlViewValue, reference:
<column>}` (without the OqlViewValue the entity is "out of sync", CE-6770). The
source document's name must equal the view entity's name. Because there is no
delete-document tool, dropping a view entity removes the entity but orphans its
source document, and `CREATE OR REPLACE` of an existing view entity fails at
the duplicate source-document create. See `mdl/backend/mcp/` and the
[proposal](../11-proposals/PROPOSAL_mcp_backend.md). Operations outside the slice
return a clear "not supported by the MCP backend" error via the generated
`unsupportedBackend` base.

## Concord (optional second client — gap-filler)

> **Concord is Windows-only — it does not run on macOS** (confirmed 2026-08-11 on
> the 11.13 Mac host). On macOS every Concord-backed capability is simply absent,
> so `DROP` of a standalone document (enumeration, microflow, page) has **no path
> at all**: PED has no delete tool and Concord is the only gap-filler. `check_model`
> is likewise unavailable; `ped_check_errors` remains for per-document validation.
> This also means MCP-authored test documents cannot be cleaned up on macOS —
> remove them in Studio Pro, or close without saving.

Some deployments run a second MCP server, **Concord** (a Studio Pro extension;
`concord-mcp`), alongside the built-in PED server. Concord is **not** an authoring
server — it has none of the `ped_*`/`pg_*` create tools — but it provides
operational/refactor capabilities PED lacks. The backend uses **PED for authoring
by default** and reaches for Concord **only** for these gaps. Configure it with
`--mcp-concord` / `--mcp-concord-dial` (a second `Client`, dialed independently);
it stays `nil` when not configured, and every Concord-backed op errors clearly if
it's missing.

Wired so far:
- **`delete_document`** — real `DROP` of standalone documents (enumeration,
  microflow, page), which PED cannot delete at all. `DROP ENUMERATION/MICROFLOW/
  PAGE` resolves the document's module + name and calls
  `delete_document {module_name, document_name}`. Unlike `save_all` this is
  **model-based, not keystroke automation**, so it is robust. (Entities and
  associations still delete via PED's array-element removal — no Concord needed.)
- **`check_model`** (`--mcp-check`) — domain-model consistency check after writes
  (PED has no validation for the live model). Parses `{success, healthy, summary
  {errorCount, warningCount, …}, errors[], warnings[]}` and prints a report to
  stderr on Disconnect. Model-based (robust). Note: `healthy: true` means zero
  *errors*, not zero warnings — the report shows both.
- **`save_all`** (`--mcp-save`) — PED has no save tool, so writes live only in
  Studio Pro's in-memory model until the user saves. `--mcp-save` flushes via
  Concord's `save_all` on Disconnect. **Unreliable — keystroke automation.**
  Concord's `save_all` synthesizes a macOS Cmd+S (osascript → System Events), with
  two failure modes observed against 11.11 Beta:
  1. **Permission.** Needs macOS **Accessibility** on the *responsible* process.
     If Studio Pro is launched from a shell that exec's the binary directly, the
     responsible process is the **terminal**, not Studio Pro — relaunch via
     `open -n -a "<app>" --args …` (launchd → app is its own responsible process)
     and grant Studio Pro Accessibility. Otherwise it fails `osascript is not
     allowed to send keystrokes (1002)`.
  2. **The keystroke does not save, and hangs Studio Pro — confirmed broken in
     11.11 Beta (2026-06-08).** First observed from a devcontainer
     (`{"status":"save_command_sent"}`, no disk change). Then re-tested the
     authoritative way — from Claude Code in the Concord terminal, **single active
     Studio Pro instance, app frontmost** — and it still did **not** persist *and*
     **hung Studio Pro** while Concord tried to drive the save. So it is **not**
     the two-instance ambiguity; it is a genuine **Concord/Studio Pro `save_all`
     bug** (the synthetic Cmd-S hangs the IDE). **Do not rely on `--mcp-save`;
     save manually (Cmd-S) in Studio Pro, and report `save_all` upstream to the
     Concord/Studio Pro team.** The backend wiring is correct and will work
     unchanged once `save_all` is fixed; a silently-no-op `save_command_sent` it
     cannot detect. Model-based gap-closers (`delete_document`, `check_model`) are
     unaffected — they do not use keystroke automation.

- **`get_app_status`** — the API call works and returns well-formed
  `{data:{running, runningUrl, projectName, …}}` (exposed as `GetAppStatus()`,
  printed by `--mcp-run`). **But `running`/`runningUrl` are effectively a
  port/process probe**, not the current session's console-managed runtime: it
  reported `running | :8080` while the Studio Pro runtime console was empty —
  because an **orphaned runtime from a previous run** was still bound to `:8080`
  (restarting Studio Pro doesn't kill the separate runtime process; `:8080` was
  confirmed listening). So trust the API shape, but treat `running: running` as
  "something is bound to the runtime port," which may be stale.
- **`run_app` / `stop_app`** (`--mcp-run` starts the app) — ⚠️ **same
  UI-automation failure as `save_all`.** They are "click the Run/Stop button"
  automations: `stop_app` returned `{"status":"command_sent"}` but the app stayed
  running across repeated `get_app_status` polls (2026-06-08, 11.11 Beta). So like
  `save_all` they report success without taking effect; `run_app` almost certainly
  behaves the same. Wired correctly (will work once Concord's UI automation is
  fixed), but **don't rely on `--mcp-run`/`stop_app` — start/stop the app manually
  in Studio Pro.** Report upstream.

**Pattern (important):** Concord's **model-editing** tools work (`delete_document`,
`check_model`); its **UI-automation** tools that synthesize button clicks /
keystrokes do **not** in this environment (`save_all`, `stop_app`, `run_app` all
return a `*_command_sent` status with no actual effect, and `save_all` can hang
Studio Pro). `get_app_status` is read-only and returns valid data but reflects raw
port state (can be a stale runtime). Net: only `delete_document` and
`check_model` are dependable today.

### Full tool inventory (44 tools, captured 2026-06-15)

Concord identity: `concord-mcp` (proto `2025-03-26`), port 7783 (directly
container-reachable; no socat). **Two behaviors to know:**

- **`tools/list` is context-curated.** After `concord__set_task_context`, a
  subsequent `tools/list` returns only a *curated subset* relevant to the declared
  task. The full 44 below come from a fresh `tools/list` with no task context set.
  Always re-probe without a task context to see everything.
- **No Maia / agent-invoke tool.** Like PED, Concord exposes *tools an agent uses*,
  not a "run the Maia agent" entrypoint. The `concord__*` tools are agent **self-
  management** helpers (calibration, friction journal, tool discovery), not Maia.

Wired/tested status: ✅ dependable · ⚠️ UI-automation, unreliable (see above) ·
○ present, not yet wired/vetted. `R` = readOnlyHint.

| Category | Tool | R/W | Status / note |
|----------|------|-----|---------------|
| Domain model | `rename_entity` | W | ○ in-place, identity-preserving rename (PED only does `set /name`) |
| Domain model | `rename_attribute` | W | ○ identity-preserving |
| Domain model | `rename_association` | W | ○ identity-preserving |
| Domain model | `rename_module` | W | ○ identity-preserving |
| Domain model | `rename_document` | W | ○ identity-preserving |
| Domain model | `set_documentation` | W | ○ set element documentation |
| Domain model | `arrange_domain_model` | W | ○ auto-layout entities/associations |
| Domain model | `delete_model_element` | W | ○ delete entity/attribute/association (PED already removes these via array op) |
| Constants/enums | `rename_enumeration_value` | W | ○ identity-preserving |
| Microflows | `modify_microflow_activity` | W | ○ mutate an activity by 1-based position |
| Microflows | `insert_before_activity` | W | ○ sequence an activity |
| Microflows | `set_microflow_url` | W | ○ |
| Pages | `delete_document` | W | ✅ real `DROP` of standalone docs (wired) |
| Pages | `exclude_document` | W | ○ |
| Pages | `generate_overview_pages` | W | ○ CRUD list + new-edit pages |
| Navigation | `manage_navigation` | W | ○ menu structure, home page, role gating (Concord convenience; mxcli now authors web-profile nav directly via generic `ped_update_document` on `Navigation$NavigationDocument` — see below) |
| Project settings | `read_configurations` | R | ○ |
| Project settings | `set_configuration` | W | ○ |
| Project settings | `read_runtime_settings` | R | ○ |
| Project settings | `set_runtime_settings` | W | ○ |
| Project settings | `get_active_run_configuration` | R | ○ which run config will run |
| Validation/diag | `check_model` | R | ✅ domain-model consistency check (wired, `--mcp-check`) |
| Validation/diag | `check_project_errors` | R | ○ full-project "Check All Errors" — reported stubbed in Concord |
| Validation/diag | `analyze_project_patterns` | R | ○ architectural pattern/anti-pattern scan |
| Validation/diag | `get_last_error` | R | ○ |
| Validation/diag | `get_studio_pro_logs` | R | ○ |
| Security (read) | `audit_security` | R | ○ full security audit (intended) |
| Security (read) | `read_entity_access_rules` | R | ○ |
| Security (read) | `read_microflow_security` | R | ○ |
| Security (read) | `read_security_info` | R | ○ project security overview |
| App runtime | `get_app_status` | R | ✅ valid data, but raw port probe (can be a stale runtime) |
| App runtime | `run_app` | W | ⚠️ UI automation — `command_sent`, no effect |
| App runtime | `stop_app` | W | ⚠️ UI automation — `command_sent`, app stays running |
| App runtime | `save_all` | W | ⚠️ UI automation — does not persist and **hangs Studio Pro**; save manually |
| Project | `refresh_project` | W | ○ force re-scan of project dir for external file changes |
| Concord meta | `concord__session_context` | R | ○ session-start briefing (call once) |
| Concord meta | `concord__set_task_context` | W | ○ declares task → curates subsequent `tools/list` |
| Concord meta | `concord__find_tool` | R | ○ free-text "which tool for this task" |
| Concord meta | `concord__verify` | R | ○ verify a high-level workflow (e.g. `entity_created`) actually applied |
| Concord meta | `concord__diagnose_project` | R | ○ parallel project-state probes (SP running? .mpr locked?) |
| Concord meta | `concord__calibration_summary` | R | ○ C4 calibration journal summary |
| Concord meta | `concord__record_calibration` | W | ○ append (claimed, actual) calibration entry |
| Concord meta | `concord__silence_friction` | W | ○ suppress friction-wire emission for a tool |
| Concord meta | `concord__tried_approaches` | R | ○ prior friction occurrences for a pattern/tool |

**Highest-value unwired gap-closers** (would fill PED limits the matrix currently
marks unsupported, pending probe + identity-preservation vetting per
[ADR-0002](../13-decisions/0002-backend-abstraction.md) and the "don't fake
identity ops" rule): the `rename_*` family (true renames vs PED's `set /name`).
(Navigation is no longer in this list — mxcli authors web-profile navigation
directly over PED; see the navigation note below.) `check_project_errors` and
`refresh_project` are also candidates. `delete_model_element` is low priority (PED
already removes entities/associations).

## Onboarding a new Studio Pro version

1. Open a project in the new Studio Pro; establish transport (direct or socat).
2. `go run ./cmd/mcpprobe -url http://localhost/mcp -dial host.docker.internal:<port> -method tools/list`
   → save to `mdl/backend/mcp/testdata/tools-<version>.json`.
3. **Diff against the previous `tools-<version>.json`** — added/removed/renamed
   tools, each surviving tool's `inputSchema`, **and each tool's description text**.
   Three releases have now proved a name-only diff insufficient, each in a different
   way:
   - **11.12** removed `pg_write_page` (a name change — the only one a name diff
     would have caught).
   - **11.13** changed no name mxcli calls, yet `pg_read_page` gained a `depth`
     argument defaulting to 4 that broke ALTER PAGE. Treat a **new argument with a
     default** as a behaviour change to the existing call, since the server applies
     it whether or not the client knows about it. A new argument mxcli must send has
     to be gated with `Client.SupportsToolArg` — never sent unconditionally.
   - **11.14** changed no name and no argument at all, yet `ped_update_document`
     gained an operation type and **downgraded its atomicity guarantee** — the whole
     of it visible only in prose. Read the description diffs; a contract can change
     with the schema byte-identical.

   The argument scan that produced 11.14's measured negative (no added/removed
   property, no changed `default`, no changed `required` set) is worth re-running
   verbatim — it is what licenses "no new `SupportsToolArg` gate needed":

   ```python
   old = {t['name']: t for t in json.load(open('testdata/tools-<prev>.json'))['tools']}
   new = {t['name']: t for t in json.load(open('testdata/tools-<next>.json'))['tools']}
   for n in sorted(set(old) & set(new)):
       a = (old[n].get('inputSchema') or {}).get('properties', {})
       b = (new[n].get('inputSchema') or {}).get('properties', {})
       assert set(a) == set(b), (n, set(b) - set(a), set(a) - set(b))
       assert all(a[k].get('default') == b[k].get('default') for k in a), n
   ```

   Also read the **system prompt** (`resources/read` on
   `mendix://studio-pro/system-prompt`). It states rules the tool descriptions do
   not — 11.14 silently dropped the "pages must use `pg_*`" instruction from it.
4. Update the **server identity** and **tool matrix** tables above (new column).
5. Re-run the **capability gaps** checks — especially delete / save / modules /
   `$ID` exposure. Any gap that closed is a feature to build; note it here and
   in `docs/11-proposals/PROPOSAL_mcp_backend.md`.

   **Probe a gap without mutating the user's open project.** Send the operation with
   a value that *cannot* succeed, and read which layer rejects it. A **structural**
   refusal ("only allowed to set primitive or reference properties directly") means
   the gap is open; a **value-validation** refusal ("not a valid concrete subtype
   of …") means the operation now reaches validation and the gap has moved. This is
   how 11.14's attribute-type gap was measured without writing anything. Always run a
   **known-good control** in the same batch — without one, "it was rejected" cannot
   distinguish an unsupported document type from a rejected operation, which is
   exactly the trap the advertised-but-absent `Settings$ProjectSettings` sets.
   A probe that could *land* needs a throwaway project: there is still no
   delete-document tool, so anything created over MCP persists until the user closes
   Studio Pro without saving.
6. Re-capture changed schemas (`ped_get_schema`) for doctypes the backend uses;
   refresh `testdata/` fixtures and any affected tests.
7. If a tool's input schema changed, update the backend call sites and the
   `version-awareness` skill if a workaround is needed for older versions.
