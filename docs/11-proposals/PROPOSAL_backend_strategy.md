---
title: Backend Strategy — adopt engalar's modelsdk base + multi-backend (MCP first)
status: implemented
date: 2026-05-31
status-note: >
  The modelsdk codec was adopted and is now the only local engine; the legacy sdk/mpr
  backend this proposal treats as one of several was deleted by
  docs/plans/2026-09-14-retire-legacy-engine.md. The multi-backend framing still holds —
  MCP/PED is a live second backend — but read "legacy" here as history.
author: Generated with Claude Code
related:
  - PROPOSAL_mcp_backend
  - UNIFIED_SCHEMA_REGISTRY
  - PROPOSAL_schema_extract
  - 0002-backend-abstraction
---

# Backend Strategy — adopt engalar's modelsdk base + multi-backend (MCP first)

**Status:** Draft
**Date:** 2026-05-31
**Author:** Generated with Claude Code

## TL;DR / Decision

- **Adopt engalar's `modelsdk` foundation** as the base rather than merging 1109
  commits into `main`. The backend abstraction ([`FullBackend`](../13-decisions/0002-backend-abstraction.md))
  is preserved on his branch, so this is a base-switch + port, not a rewrite.
- **Grow backends on the *target* axis.** `FullBackend` should vary by **target**
  (local `.mpr` / live Studio Pro / mock); the `sdk/mpr`→`modelsdk` change is an
  *engine* swap one layer below, not a new backend.
- **Highest priority: the MCP backend** (live Studio Pro via the PED server). Detailed
  design in [`PROPOSAL_mcp_backend`](PROPOSAL_mcp_backend.md); this doc sets its place
  in the architecture and its priority.
- **Re-point codegen** from the regex/npm-SDK source to an authoritative
  PED/mxunit extraction (fixes the reliability gap retran flagged). Can lag the base
  switch — inherit engalar's source short-term.
- **Port the product surface** that his branch lacks: LSP + VS Code extension,
  `docs-site`, and the v0.12 widget serialization.

> This is an options + direction document, not a ratified decision. Once the team
> commits, the architectural choice (adopt engalar base; target-axis backends)
> should be recorded as an **ADR**, superseding the relevant parts of ADR-0002.

> For how the inherited engine actually works — the read→mutate→encode pipeline,
> template extraction, and the roundtrip field-preservation that motivates this
> switch — see the companion
> [`MODELSDK_BACKEND_PIPELINE.md`](../03-development/MODELSDK_BACKEND_PIPELINE.md).

---

## Investigation insights (evidence base)

All measured this session against `engalar/dev @ ed978f09`, our `main`, and a live
Studio Pro 11.11 (`test7-app`).

| Area | Finding |
|---|---|
| **Fork shape** | `engalar/dev` = fork of this repo, **1109 ahead / 142 behind** `main`, 8216 files changed, own tags v0.1–v0.17. Same module path. |
| **modelsdk** | His own Go package, explicitly *"a replacement for `sdk/`"*: 3-layer (element dirty-bitmap → lazy property decode → codec `TypeRegistry`), **roundtrip-safe** (unknown fields survive read/write), **53-domain** generated metamodel. `sdk/mpr` deleted; `mdl/backend/mpr` rebuilt on modelsdk behind a `unitstore` persistence seam. |
| **Backend abstraction** | `FullBackend` + `mock`/`mpr` + `BackendFactory` **preserved**. modelsdk is the *engine* under `mpr`, **not** a parallel backend. `unitstore.UnitPersistence` is the swappable I/O seam (= the "Option A" seam in the MCP proposal). |
| **MCP/PED server** | Exposes `ped_*` (document CRUD; **domain models update-only**, pages **forbidden**) + `pg_*` (pages) + agent helpers. Operation-based `ped_update_document` (set/add/remove) maps onto ALTER/mutators. Mandatory `find→get_schema→create/update→check_errors`. |
| **MCP transport** | Server binds **IPv6 `[::1]:7782`** + `Host: localhost` guard. From a devcontainer: bridge with a host-side `socat` IPv4→`[::1]` forwarder; client pins `Host: localhost`. `cmd/mcpprobe` (built this session) completes the handshake. |
| **MCP write semantics** | **On-disk files are stale-by-default while Studio Pro is open.** `ped_update_document` edits live in memory (shown unsaved) until the user saves; `ped_create_module` flushes immediately; `ped_check_errors` does not flush; no save tool. |
| **Widgets** | His `modelsdk/widgets` is, by his own spec, an *early, less-complete fork* of our `sdk/widgets` (which he deleted); serialization *"does not change"*. Our **v0.12 serialization is absent** (~15 commits). His **real-time `.mpk` registry supersedes** our `generatorVersion` staleness fix. |
| **Codegen source** | `dtsparser/jsparser.go` is **still regex** (23 patterns, no AST) over the public `mendixmodelsdk` npm. retran (PR #335) called it a *"fragile foundation"*; engalar agreed; only an `audit` gap-detector was added (test-only). |
| **Source reliability** | `mendixmodelsdk` IS current (4.112.0, 2026-05-21, ~monthly) **but structurally omits new doc types**: latest package has 55 domains, **zero agent/AI** → Agent Editor is **hand-coded**. `mendixmodellib` (internal Studio Pro source) is **404 on npm**. |
| **Reverse delta (what switching loses)** | Integration features are **present/ahead** on his branch (he even shipped 3 microflow statements we only proposed). Genuine losses: **`docs-site` (305 files)**, **LSP + `vscode-mdl`**, **`api/`**, **v0.12 widget serialization**, **~7 CI workflows**. |
| **doctype-tests** | ~90% shared corpus (45 vs 47 files, same numbering, mostly small diffs) → MDL syntax did **not** fork hard; usable as a parity harness. |

---

## Architecture: the two axes (the load-bearing idea)

The mistake to avoid is putting the modelsdk-vs-sdk engine choice on the same axis
as "where writes go." They are different axes:

```
FullBackend   (TARGET axis — where the model lives / how it is written)
  ├── localBackend   → modelsdk codec + metamodel
  │                      └── unitstore.BufferedUnitStore → UnitPersistence (disk I/O)
  ├── mcpBackend     → Studio Pro PED:  ped_* (docs) / pg_* (pages)   ◄── NEW, highest priority
  └── mockBackend    → in-memory test double
```

- **`FullBackend` impls vary by target.** Local file, live Studio Pro, mock.
- **The engine (modelsdk) + `unitstore` sit *under* the local backend**, not beside it.
  Swapping `sdk/mpr`→`modelsdk` is an engine change, not a new backend.
- **MCP is a genuinely different target** → a legitimate new `FullBackend`. It is
  operation-based (`ped_update_document`), so it does **not** implement
  `UnitPersistence`; it sits one layer up, as its own backend.

This is why "should modelsdk be a new backend next to mpr?" is answered **no** (engine,
not target) while "should MCP be a new backend next to mpr?" is answered **yes**.

---

## Options considered

### Option 1 — Merge both streams into `main`
Cherry-pick / reconcile engalar's 1109 commits onto `main`.
- **Pros:** keeps `main` as the canonical line; nothing to re-home.
- **Cons:** the executor/backend rework is **619 entangled commits** on hot files;
  ~3 weeks of parallel evolution on the same code. Enormous conflict surface.
- **Effort:** XL. **Risk:** High. **Verdict:** rejected — disproportionate.

### Option 2 — Adopt engalar's branch as base, port our deltas  ✅ (chosen)
Switch the base to his foundation; re-land the product surface he lacks.
- **Pros:** inherits the superior foundation (53-domain roundtrip-safe codec, more
  features) cheaply; the painful merge disappears because the *foundation* is already
  built; losses are bounded and mostly self-contained.
- **Cons:** re-own porting LSP/VS Code, docs-site, v0.12 widgets; inherit the regex
  codegen-source debt until re-pointed; depends on the engalar-collaboration model.
- **Effort:** L (concentrated in LSP + widgets). **Risk:** Medium.

### Option 3 — Keep forks separate, harvest selectively
Vendor only `modelsdk/` (88 isolated, additive-by-design commits) into `main`; leave
the rest.
- **Pros:** lowest risk; `main` stays in control; gets the codec/metamodel win.
- **Cons:** you *don't* get his executor/feature lead; you re-do the backend migration
  yourself; two lines drift.
- **Effort:** M. **Risk:** Low. **Verdict:** viable fallback if Option 2's collaboration
  model doesn't hold.

### Option 4 — Status quo: build MCP backend on `main`/`sdk/mpr`
Ignore the fork; add the MCP backend to current `main`.
- **Pros:** no base change.
- **Cons:** keeps the 5–10 domain `sdk/mpr` limitation and the BSON-fidelity bug class
  the roundtrip codec eliminates; forgoes his feature lead.
- **Effort:** M (MCP only). **Risk:** Low but low-ceiling. **Verdict:** rejected — leaves
  value on the table.

**Direction (per maintainer):** **Option 2** — adopt engalar's approach; **MCP backend is
the top-priority new backend.**

---

## The backends we need (target axis), prioritized

1. **MCP backend — HIGHEST PRIORITY.** Live Studio Pro via PED. Full design in
   [`PROPOSAL_mcp_backend`](PROPOSAL_mcp_backend.md). On the new base it lands as a new
   `FullBackend` beside engalar's local backend. Key constraints from this session:
   - Two write protocols: `ped_*` (most doctypes; domain models **update-only**) and
     `pg_*` (pages — PED forbidden for pages).
   - Transport: dial host gateway, pin `Host: localhost`; host-side IPv4→`[::1]` bridge
     in a devcontainer. `cmd/mcpprobe` already speaks the handshake.
   - **Consistency hole:** on-disk reads are stale while Studio Pro is open. Use a
     **dirty-set router** — read documents written this session back via
     `ped_read_document`; bulk/catalog from local files.
2. **Local backend (modelsdk).** Inherited from engalar; the default target.
3. **Mock backend.** Exists; extend for new methods.
4. **(Future targets):** Team-Server/Platform, read-only remote inspection — out of scope now.

---

## Work breakdown & effort

| Workstream | Effort | Risk | Notes |
|---|---|---|---|
| **Adopt base** (switch branch / establish working build on engalar's foundation) | M | Med | Mostly build + CI wiring; decide branch topology with engalar |
| **MCP backend — vertical slice** (domain model via `ped_update_document` + `ped_check_errors`, hybrid read) | M | Med | Top priority; `cmd/mcpprobe` core → `mcp/client.go`; transport bridge |
| **MCP backend — full** (microflows, pages via `pg_*`, workflows, security, dirty-set router) | L | Med | Phased per doctype after slice proves out |
| **Port LSP + `vscode-mdl`** | L | Med-High | Hardest port — rewire to his executor/grammar internals |
| **Port `docs-site`** (305 files) | S–M | Low | Largely copy; review for changed CLI behaviour |
| **Widget v0.12 serialization parity** | M–L | Med | Re-apply intent onto `modelsdk/widgets`; **golden-diff** to size gaps; keep his real-time registry |
| **Re-point codegen → PED/mxunit (or `.mxcore`) source** | L | Med | Resolves retran #1/#2; keeps new doc types (Agent Editor) current; can defer |
| **Port `api/` fluent package** | S | Low | Rewire onto modelsdk or drop if unused |
| **CI workflows** (nightly, push-test, docs, AI bots) | S | Low | Copy + adjust |
| **Reconcile MDL syntax diffs** | S–M | Low | 11 grammar files, moderate; corpus ~90% shared |

Sequencing: **adopt base → MCP slice → (parallel) docs-site/CI/api + widget parity →
MCP full + LSP → codegen re-point.** The MCP slice and the cheap copies can run
concurrently; LSP and codegen-re-point are the long poles and can trail.

---

## Testing approach

1. **Golden-diff as the spine.** Run the **shared `doctype-tests` corpus** (~90% common)
   through the old (`sdk/mpr`) and new (`modelsdk`) engines and diff the resulting
   `.mxunit` BSON. This validates the base switch is behaviour-preserving and quantifies
   widget-serialization gaps precisely (vs. guessing from commit history). engalar's
   `goldenfs` scope suggests harness scaffolding already exists to reuse.
2. **MCP backend round-trip.** Execute MDL via the MCP backend against a running Studio
   Pro + `test7`-style fixture; assert `ped_check_errors` clean, then verify the artifact
   via `ped_read_document` (in-memory truth) **and**, after a save, via local `mxcli
   describe` (disk). Explicitly test the **stale-read / dirty-set** behaviour.
3. **MCP as a BSON oracle.** Per [`PROPOSAL_mcp_bson_benchmark`](PROPOSAL_mcp_bson_benchmark.md):
   diff local-engine output against Studio-Pro-authored BSON for the same operation — a
   correctness oracle that catches field/`$type`/pointer mistakes `mx check` misses.
4. **Codegen audit + multi-version.** Keep engalar's `audit`/`audit-keys` (scan real MPRs
   for unregistered `$Type`s / `ByIdRef` key mismatches) — **promote it from test-only to
   a CI gate** — and run extraction against multiple source versions (retran's 4.100–4.111
   ask). When codegen is re-pointed at PED/mxunit, the oracle and the source converge.
5. **`mx check` / Studio Pro validation.** Every new doctype example in
   `mdl-examples/doctype-tests/` must pass `mx check`; widget changes validated by opening
   in Studio Pro (CE0463 class).
6. **Don't claim green without coverage.** A passing gate proves nothing unless it
   exercises the specific construct against an equivalent baseline (the v0.12.0 "no drift"
   false-negative lesson).

---

## Risks & open questions

1. **Collaboration model with engalar.** Is `engalar/dev` upstreamed, co-owned, or an
   external fork we track? Determines whether "adopt base" means merging his line or
   rebasing onto it. **Needs a human decision.**
2. **MCP write persistence.** No save tool; edits stay in memory until the user saves.
   The dirty-set router handles reads, but a true "apply & persist" path may need Mendix
   to expose a save/flush tool. Track as a Studio Pro ask.
3. **Codegen source debt.** If not re-pointed, new Studio Pro doc types (Agent Editor and
   successors) keep needing hand-coding. The PED/mxunit extraction we validated is the fix
   but is itself L effort. **Refined 2026-06-05** — the debt is **missing doc types**, not
   missing *version* data; the npm SDK's version metadata is closed-world complete for the
   supported range. See [Version handling & the schema-registry overlap](#version-handling--the-schema-registry-overlap-investigation-2026-06-05).
4. **Widget parity uncertainty.** Until the golden-diff runs, the exact v0.12 gap on his
   engine is inferred, not measured. Run it before committing widget effort.
5. **LSP/grammar drift.** The LSP binds to executor/grammar internals he rewrote; the port
   is the highest-uncertainty item.
6. **MDL syntax reconciliation.** Moderate divergence across 11 grammar files; pick a
   canonical set per statement where they differ.

---

## Version handling & the schema-registry overlap (investigation 2026-06-05)

A follow-up investigation triggered by two questions: *does adopting the engine make the
v0.13 [Unified Schema Registry](UNIFIED_SCHEMA_REGISTRY.md) redundant, and how does the
inherited engine actually handle Mendix versions?* Measured against `engalar/dev` and the
npm `mendixmodelsdk` source (4.105 / 4.111). The short answer reshapes both the registry
roadmap and the "codegen source debt" risk above.

### The v0.13 schema registry is subsumed except for *source* + *inspection/migration*

The Unified Schema Registry bundles four jobs. The roundtrip codec absorbs the urgent one
and leaves three thinner, orthogonal ones — so the registry should be **descoped to a small
"inspection + migration on top of the codec" proposal**, not built as a new subsystem.

| Registry job | Subsumed by the codec? | Disposition |
|---|---|---|
| **(a)** Roundtrip-safe / field-complete serialization (Studio Pro stops rejecting output) | **Yes — fully.** Unknown fields pass through verbatim (§4 of the pipeline doc); partial metamodel coverage is *safe*. This was the registry's urgent driver and its hardest part. | Retired into the engine |
| **(b)** Authoritative *source* (`mx dump-mpr`) replacing regex-npm + `supplements.json` | **No** — engalar inherits the identical source. | Survives; **path-independent** (already the "re-point codegen" risk) |
| **(c)** Inspection surface — `schema show/list/diff`, catalog tables, LSP hover/completion | **No** — orthogonal, but now cheap to build on the generated `gen/*` metadata | Survives as thin features |
| **(d)** Migration tooling, dual-stack dispatch, object-list widgets (`check --post-migration`) | **No** — orthogonal product features | Survives as thin features |

Because the codec makes partial field coverage safe, the registry's whole architecture
(Phases 4–5, chasing ~100% coverage, dropping `supplements.json`) collapses. What's left is
**(b)+(c)+(d)** layered on `gen/*`.

### Two independent version systems — only one is active, and it didn't fork

| System | Granularity | State | Forked from `main`? |
|---|---|---|---|
| Executor `sdk/versions/*.yaml` + `registry.go` | per-MDL-**feature** ("can I use this statement on vX") | **Active, enforced** via `checkFeature()` before every BSON write, with actionable hints | **Byte-identical** on `main` and `engalar/dev` (`mendix-{9,10,11}.yaml`, `registry.go`) |
| Codec `modelsdk/gen/*/version.go` | per-BSON-**property** ("does field X exist in vX") | **Latent** — generated, but **zero non-test consumers** | engalar-only (part of the engine) |

The version support users actually feel is the **YAML feature registry, which is identical
on both branches** — so the adopt-vs-vendor decision is **neutral** on it; neither plan ports
or changes it. The engine *adds* a finer-grained per-property table that is strategically
valuable (it's the raw material `schema diff` / `check --post-migration` would consume) but
inert today.

### `version.go` is closed-world *complete* for 9–11 — not "incomplete"

The per-property table looks sparse (~21% of properties carry an explicit `Introduced`/
`Deleted`), but **absence is information, not a gap**:

- Mendix's `StructureVersionInfo` is a **change-log**: a property carries `introduced` only
  if it was added *after* the type's baseline. No bound ⇒ "present since the type's baseline."
- The SDK's tracking floor is **Mendix 7** (source carries `introduced: "7.0.0"`+). mxcli
  supports 9/10/11, so any property added anywhere in that range *necessarily* carries an
  explicit `introduced`. Absence reliably means "present throughout 9–11."
- This is a **guarantee, not a convention**: the SDK's own runtime `isAvailable` gating reads
  `versionInfo`; an untagged post-floor addition would mis-gate the SDK itself.

**Consequence:** for per-property existence-gating across the supported range, `version.go`
is already complete. Neither a better parser nor `mx dump-mpr` is required for it.

### The regex parser is a *robustness* liability, not a *coverage* one

Comparing source to generated output: the npm source (4.105) holds **734 property-level**
`introduced:` annotations (the other 667 of its 1401 are type/enum-level); engalar's
generated `version.go` (from 4.111) holds **809** property-level `Introduced:` — i.e. it
already captures **≥100%** of the property-level version data the source carries. The regex
is *not* lossy on current content. Its real debt is fragility to **source *format* changes**
(minification, restructure), which `retran` (#335) correctly flagged.

- "Improve the regex to get more version info" has **no target** — the data is already out.
- The robust replacement, if desired, is **instantiate-and-reflect**: `require('mendixmodelsdk')`
  in a Node extraction step (already a codegen dependency) and read the live object graph —
  yielding format-robustness *plus* constructor-computed defaults, type-level `versionInfo`,
  and property descriptors (ref kinds, list encodings) the regex approximates. Pure-Go `goja`
  is the no-Node alternative but costs CommonJS-`require` shimming and JS-compat risk. This is
  optional tech-debt reduction, **not** a version-coverage lever.

### The one real fix, and the one real gap

| Concern | Status | Action |
|---|---|---|
| Per-property existence gating, v9–11 | **Already complete** (closed-world, SDK-guaranteed) | None |
| Type-floor inheritance for unbounded props | Parser captures type-level `Introduced`/`Deleted` (`JsVersionInfo`), but the generated `TypeVersionInfo struct { Properties map[...] }` **drops it** | **Small emitter+struct fix** — add `Introduced`/`Deleted` to `TypeVersionInfo` and emit them; no parser or source work |
| Regex robustness | Fragile to *format* changes, not lossy on content | Optional — instantiate-and-reflect |
| Doc types the SDK omits entirely (Agent/AI) | **True coverage gap** — closed-world does **not** apply (absence = "not modeled") | `mx dump-mpr` *or* hand-code + promote the codegen `audit`/`audit-keys` mode to a CI gate |

The only fidelity bug here is the type-level bound the emitter discards (a ~half-day change).
The only genuine coverage gap is *missing types*, not *missing version data* — which is why
Risk #3 above is refined from "codegen source debt (version)" to "missing doc types," and the
`audit` gate is the safety net for it.

## Recommended next steps

> **Concrete plan (2026-06-05):** the *vendor-the-engine-into-`main`* path (Option 3 evolved) now
> has a phased, feature-flagged implementation plan with a dual-engine comparison harness:
> [`docs/plans/2026-06-05-adopt-modelsdk-engine.md`](../plans/2026-06-05-adopt-modelsdk-engine.md).
> It keeps `legacy` (`sdk/mpr`) as the default and shadow-validates `modelsdk` via normalized
> BSON diff until cutover — sidestepping the upfront `mongo-driver` v2 migration.

1. Settle the **collaboration/branch model** with engalar (blocks "adopt base").
2. Establish a **working build on his foundation** + wire the golden-diff harness.
3. Land the **MCP backend vertical slice** (domain model) on that base — top priority —
   reusing `cmd/mcpprobe`'s client core and the host-side transport bridge.
4. In parallel, **copy back the cheap losses** (docs-site, CI, `api/`) and run the
   **widget golden-diff** to size that port.
5. Schedule the **LSP/VS Code port** and the **codegen source re-point** as the trailing
   long-poles.
6. Once committed, **record the decision as an ADR** (supersede the relevant part of
   ADR-0002 to add the target-axis backend model).
