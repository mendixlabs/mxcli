---
title: MCP Backend (live Studio Pro writes)
category: architecture
last-synced: cfcf1ea12
sources:
  - docs/03-development/PED_MCP_CAPABILITIES.md
  - mdl/backend/mcp/backend.go
  - mdl/backend/mcp/read_router.go
  - mdl/backend/mcp/delete.go
  - mdl/backend/mcp/app.go
---

> **Do not duplicate**: the tool matrix, transport setup, and capability gaps
> (`docs/03-development/PED_MCP_CAPABILITIES.md` is canonical), the backend
> interface contract ([[rationale/backend-abstraction]] / ADR-0002), or per-method
> behaviour (read source). This page frames *what kind of backend* this is.

## What this is

A second implementation of the executor's backend interface (alongside the MPR
backend) that does **not** write BSON to disk. Instead it routes model mutations to
a **live, running Studio Pro** through that IDE's embedded MCP server — codenamed
**PED** — while still reading structure from the local `.mpr` on disk. It is a
*hybrid* backend: disk for reads, the live IDE for writes. Deleting a document and
running the app go through the same server: `ped_delete_document` and the
`mendix_app` handler, both new in Studio Pro 11.16.

## How it fits

The same MDL pipeline (grammar → AST → visitor → executor) runs unchanged; only the
backend differs, which is exactly what the backend abstraction is for. Choosing the
MCP backend means the executor's writes become `ped_*` tool calls against Studio
Pro's in-memory model rather than MPR writer calls — so the edits appear live
in the open project instead of being serialised to the file.

The hybrid split creates a **consistency problem the backend has to close itself**.
PED can't enumerate modules and (mostly) doesn't flush to disk, so reads come from
the local reader — but once a write lands, that reader is stale for the touched
module. The backend tracks this with a **dirty set** and a **read router**: a module
edited this session is reconstructed from PED's live model rather than read from the
now-stale `.mpr`, and modules/workflows *created* this session (which aren't on disk
at all) are held in session lists and merged into list/get results. Because PED's
reads collapse nested elements to their `$Type`, reconstruction enriches what it
needs (e.g. real attribute types) with targeted leaf reads. Pages are the one
sub-area that uses a different protocol entirely — `pg_*` read-modify-write on the
widget tree — because PED's page tools, not its generic document tools, own pages.

The shape of what this backend can do is governed less by the model than by PED's
API surface: simplified constructors, a constrained patch API, and a handful of
counter-intuitive mutation rules. That surface — and the discipline of probing it,
mapping exactly, and rejecting the inexpressible — is its own concept; see
[[models/ped-mutation-constraints]]. The scope is deliberately bounded: the MCP
backend wires *existing* MDL to PED; it does not extend the MDL language.

## See also

- [../../docs/03-development/PED_MCP_CAPABILITIES.md](../../docs/03-development/PED_MCP_CAPABILITIES.md) — canonical tool matrix, transport per environment, capability gaps, and onboarding a new Studio Pro version
- [[models/ped-mutation-constraints]] — the invariants that bound what writes are possible
- [[architecture/mpr-read-write]] — the on-disk write backend this one parallels
- [[rationale/backend-abstraction]] — why a second backend drops in without touching the executor
