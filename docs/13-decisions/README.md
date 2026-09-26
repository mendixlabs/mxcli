# Architecture Decision Records

This folder records cross-cutting architectural decisions for mxcli — the
*why* behind durable patterns that shape more than one feature.

ADRs complement, but do not replace:

- **Proposals** (`docs/11-proposals/`) — feature-specific *what to build*.
- **Wiki** (`docs-wiki/`) — synthesized mental model, citing ADRs as sources.
- **CLAUDE.md** — active rules and invariants enforced during work.
- **Skills** (`.claude/skills/`) — step-by-step procedures.

See [ADR-0001](0001-record-architecture-decisions.md) for the bootstrap
decision to use ADRs at all.

## When to write an ADR

Write one when **all** of these hold:

- The decision is **cross-cutting** — shapes more than one feature.
- The decision is **durable** — expected to hold for years.
- The *why* would be hard to reconstruct from PR history six months later.
- A future contributor could plausibly propose to undo it without knowing
  the context.

Do NOT write an ADR for:

| Situation | Use this instead |
|-----------|------------------|
| Feature-specific design | Proposal in `docs/11-proposals/` |
| Active rule or invariant | CLAUDE.md (cite the ADR if one exists) |
| Step-by-step procedure | Skill in `.claude/skills/` |
| How to use a feature | User manual in `docs-site/` |
| Synthesized "current model" | Wiki page in `docs-wiki/` |

## File naming and numbering

- Files: `NNNN-short-slug.md`, e.g. `0007-backend-abstraction.md`.
- Numbers are sequential, never reused, no gaps. The next ADR is the
  highest existing number plus one.
- The slug names the decision, not the topic — `backend-abstraction` is
  good; `executor` is too broad.

## Template

```markdown
# ADR-NNNN: <Decision title>

- **Status**: Proposed | Accepted | Superseded by [ADR-XXXX](...) | Deprecated
- **Date**: YYYY-MM-DD
- **Deciders**: <optional>
- **Related**: <optional — proposal, PR, issue>

## Context

What forces are at play? What problem are we solving? Why now?

## Decision

What did we decide? One paragraph max — the decision itself, not the
justification.

## Consequences

Positive, negative, and neutral. Be honest about the downsides — a future
contributor weighing whether to revisit needs the full picture.

## Alternatives considered

What else was on the table, and why was it not chosen?
```

## Immutability and supersession

An accepted ADR is immutable. Typo and clarification edits are fine;
content changes are not. When the decision changes, write a new ADR that:

1. Begins with `**Supersedes**: [ADR-NNNN](...)`.
2. Updates the old ADR's status to `Superseded by [ADR-MMMM](...)`.

This preserves the audit trail.

## Cross-references with other artifacts

- A new ADR that introduces a load-bearing rule should be cited from
  CLAUDE.md (one line, e.g. `See ADR-0007: backend abstraction.`).
- A new ADR is a source for the wiki's `rationale/` pages — re-sync the
  affected page via `/mxcli-dev:wiki-sync` so the wiki reflects current
  reasoning.
- A proposal that crystallises into a cross-cutting decision should
  reference the resulting ADR in its frontmatter.

## Index

| # | Title | Status |
|---|-------|--------|
| [0001](0001-record-architecture-decisions.md) | Record Architecture Decisions | Accepted |
| [0002](0002-backend-abstraction.md) | Backend Abstraction Layer | Accepted |
| [0003](0003-mdl-is-sql-shaped.md) | MDL is SQL-shaped | Accepted |
| [0004](0004-full-codec-engine.md) | Route all document types through the codec engine | Accepted |
| [0005](0005-semantic-model-interface-currency.md) | Semantic model is the backend-interface currency; storage formats are swappable adapters | Accepted |
| [0006](0006-mcp-capability-model.md) | Version-aware MCP capability model | Proposed |
| [0007](0007-mcp-read-model-session-overlay.md) | MCP backend read model — disk base with session overlay | Proposed |
| [0008](0008-identity-and-idempotence.md) | Skip unchanged writes; never renumber element IDs in place | Accepted |
| [0009](0009-tunnel-is-linux-only.md) | The embedded tunnel ships in Linux builds only | Accepted |
| [0010](0010-mdl-canonical-syntax-rules.md) | MDL has one canonical form, governed by twelve syntax rules | Accepted |
| [0011](0011-mdl-language-versioning.md) | MDL evolves through deprecation aliases and a language header; meaning changes only across versions | Accepted |
| [0012](0012-mdl-first-and-data-first-editing.md) | MDL-first and data-first editing share one syntax and one patch engine | Accepted |

## Candidates to back-fill

These cross-cutting decisions are in effect today but lack ADRs. Draft
them opportunistically when the relevant area is touched and the context
can be captured accurately. Do not bulk-backfill; each ADR needs real
research into its context and alternatives.

- Pure-Go SQLite (no CGO, `modernc.org/sqlite`)
- ANTLR4 grammar; generated parser files not committed to git
- `mdl/types` shared package to break `sdk/mpr` import cycles
- Pluggable widget WidgetEngine + `.def.json` over hardcoded BSON builders

Note: inverted association pointer semantics and storage-name vs qualified-name translation are **invariants of Mendix**, not decisions mxcli made — they belong in `docs-wiki/models/` (as mental-model pages) and in CLAUDE.md (as active rules), not as ADRs.
