# Implementation Plan — Adopt the modelsdk engine on `main`, behind a feature flag with dual-engine comparison

**Date:** 2026-06-05
**Status:** Superseded — the engine was adopted and became the default; the legacy engine it was
flagged against was then deleted by
[`2026-09-14-retire-legacy-engine.md`](2026-09-14-retire-legacy-engine.md). Kept for the design
record; the dual-engine and feature-flag machinery it describes no longer exists.
**Implements:** the "vendor the engine into `main`" path from
[`PROPOSAL_backend_strategy.md`](../11-proposals/PROPOSAL_backend_strategy.md) (Option 3 evolved
into a sequencing plan) — *keep `main` canonical and our product surface intact; lift the one
high-value, cleanly-separable asset (the roundtrip codec) under the existing `FullBackend`
abstraction.*
**Engine reference:** [`MODELSDK_BACKEND_PIPELINE.md`](../03-development/MODELSDK_BACKEND_PIPELINE.md)

---

## 1. Strategy in one paragraph

Vendor engalar's `modelsdk/` engine onto `main` as a **second implementation of the existing
`FullBackend` interface**, selected at the single factory seam
(`cmd/mxcli/main.go:205 SetBackendFactory`). Ship it **behind a feature flag** (`MXCLI_ENGINE`
/ `--engine`) with `legacy` (`sdk/mpr`) as the default. Validate the new engine with a
**dual-engine differential harness**: run the same MDL through both engines on a throwaway copy
and diff the normalized `.mxunit` BSON. Flip the default only when the diff is clean across the
corpus and real projects. **Defer the tree-wide `mongo-driver` v1→v2 migration to the cutover**
— the two engines coexist (v1 and v2 are different module paths) and we diff at the byte level,
which is driver-agnostic.

This is deliberately the *reversible* path: at every step both engines are present, the old one
is the source of truth, and the new one is shadow-validated before it ever owns a write.

## 2. Why the feature flag is the right backbone (answer to "does it make sense?")

Yes — and it does more than A/B switching:

- **Every real command becomes a differential test.** The curated `doctype-tests` corpus is
  ~90% shared but finite; running *actual* user scripts through both engines and diffing surface
  divergences the corpus never exercises.
- **It catches the bug class `mx check` misses** — field drift, `$Type` mistakes, pointer/order
  differences — by comparing against the *known-good* legacy output, not just "does Studio Pro
  accept it."
- **It removes the v2 migration from the critical path.** Because comparison is byte-level, the
  new engine can use `mongo-driver/v2` while the rest of the tree stays on v1 until cutover.
- **It makes cutover a one-line, reversible change** (flip the default factory; keep `legacy`
  reachable for one release as an escape hatch).
- **The tooling already exists** on engalar's branch (`cmd_bson_compare.go`, `cmd_bson_dump.go`)
  — port, don't invent.

### Engine modes

| `MXCLI_ENGINE` | Behaviour | Use |
|---|---|---|
| `legacy` (default) | `sdk/mpr` writes to disk. Current behaviour, byte-for-byte. | Production until cutover |
| `modelsdk` | new engine writes to disk. | The eventual default; opt-in early adopters |
| `compare` | `legacy` writes to disk (source of truth); `modelsdk` encodes in parallel on a copy; normalized BSON diffed; divergences logged (and failed in CI). | Development + CI gate |

> **Design note — run-both, not in-process dual backend.** `compare` mode runs the script
> through each engine on its own copy of the project and diffs the results. Do **not** build an
> in-process backend that fans each mutation to two live element trees — keeping two trees in
> lockstep mid-command is fragile and buys nothing the run-both approach doesn't. The executor
> stays single-backend; comparison is an outer harness.

### The normalization problem (load-bearing detail)

Generated `$ID`s and GUIDs differ on every run, so a raw byte diff is useless. The harness needs
a **canonicalizer** that, before diffing:

1. Collects the set of `$ID`/GUID binary values from each output and remaps them to stable
   ordinal tokens (`$ID#1`, `$ID#2`, …) in document-walk order — preserving *referential
   structure* (same ID used twice → same token) while masking the random value.
2. Masks known-volatile fields (timestamps, `dataStorageGuid` where non-deterministic).
3. Diffs the resulting canonical BSON (decoded → sorted-key JSON, or `bsoncore` element walk).

A residual diff after canonicalization is a **real** divergence. This canonicalizer is the same
primitive the golden-diff harness in the proposal needs, so build it once and share it.

## 3. Phases

Each phase has an explicit exit gate. Phases 1–3 leave `legacy` as the default the entire time.

### Phase 0 — Vendor + scaffolding + flag + harness  *(Effort: M, Risk: Low)*

- Vendor `modelsdk/` (`element`, `property`, `codec`, `gen/*`, `mpr`, `widgets`) and
  `mdl/backend/unitstore` onto `main`. Additive — `sdk/mpr` untouched. Brings `mongo-driver/v2`
  as a **coexisting** dependency.
- Vendor the codegen (`cmd/modelsdk-codegen`, `internal/codegen` deltas, `supplements.json`) so
  `gen/*` is regenerable, not just a frozen blob. **Fold in the `TypeVersionInfo` type-level
  bound fix** here (see §4).
- Add the flag seam: replace the hard-wired `SetBackendFactory` at `cmd/mxcli/main.go:205` with
  a selector reading `MXCLI_ENGINE` (env) and a hidden global `--engine` flag. `legacy` default.
- Port `cmd_bson_compare.go` / `cmd_bson_dump.go`; build the **canonicalizer** (§2) and a
  `make engine-diff` target that runs the `doctype-tests` corpus through both engines and reports
  divergences. Wire the `compare` mode.
- **Gate:** binary compiles with both engines linked; `MXCLI_ENGINE=modelsdk` reaches a stub
  backend; `make engine-diff` runs end-to-end (even if everything diverges initially).

### Phase 1 — Read-path parity  *(Effort: M, Risk: Low–Med)*

- Back the new `FullBackend`'s **read** methods with `modelsdk/mpr` + codec (reuse engalar's
  `mdl/backend/mpr/convert_reader.go` as reference).
- `compare` mode diffs the *decoded model* (entities, microflows, pages, …) read by each engine
  across the corpus **and a set of real MPRs** (v1 and v2 formats).
- **Gate:** reads are model-equivalent on every corpus project and the real-MPR set; any
  divergence is explained (benign) or fixed.

### Phase 2 — Write-path vertical slice: domain model  *(Effort: L, Risk: Med–High)*

This is the crux slice — the engine/backend seam (22 commits touch both `modelsdk/` and
`mdl/backend/`).

- Port engalar's `domainmodel_modelsdk.go`, `convert.go`, `factory.go`, `script_tx.go`, and the
  `unitstore` write path for the local backend's domain-model mutations. **Use his
  `mdl/backend/mpr` as a reference implementation, adapting imports — do not import his executor
  rework.** `FullBackend` is identical on both sides; the interface is the firewall.
- `compare` mode: run domain-model MDL (`create/alter entity`, associations, attributes,
  enumerations) through both engines; canonicalize; diff per `.mxunit`.
- Validate the new-engine output additionally with `mx check` and a Studio-Pro open on a sample.
- **Gate:** domain-model corpus produces canonically-identical BSON (or every diff is documented
  as a known, Studio-Pro-accepted improvement, e.g. roundtrip-preserved unknown fields). `mx
  check` clean.

### Phase 3 — Write-path coverage expansion  *(Effort: L, Risk: Med, parallelizable)*

Doctype by doctype, each independent, each gated by the same `compare` + `mx check` pass:
microflows → pages (`page_mutator.go`) → workflows → security → navigation → services/REST →
remaining. Pipeline-style: a doctype clears as soon as its diff is clean; no global barrier.

- **Widget parity is the special case.** Keep **our v0.12 widget serialization** (it lives on
  *our* side; his is "absent"); reconcile with his real-time `.mpk` registry. `compare` mode
  *sizes the gap precisely* instead of guessing from commit history — run it before committing
  widget effort.
- **Gate (per doctype):** clean canonical diff over its corpus slice + `mx check`; widget changes
  additionally Studio-Pro-validated (CE0463 class).

### Phase 4 — Cutover  *(Effort: S, Risk: Med)*

- Flip the default factory to `modelsdk`. Keep `MXCLI_ENGINE=legacy` reachable for **one release**
  as a documented escape hatch.
- Keep `compare` available; promote a periodic `compare` run over a project zoo to nightly CI.
- **Gate:** clean diff across the full corpus + real-project zoo for two consecutive nightly runs;
  no open Studio-Pro-rejection issues.

### Phase 5 — Cleanup  *(Effort: M, Risk: Low–Med)*

- Delete the `sdk/mpr` **write** path. Migrate the remaining tree from `mongo-driver` v1 → v2
  (~129 files, mechanical), drop the v1 dependency.
- Remove the `legacy` factory and (optionally) retire the flag to a kill-switch.
- Retire `sdk/widgets` in favour of `modelsdk/widgets` once parity is proven.

## 4. Cross-cutting work

| Item | Where | Notes |
|---|---|---|
| **`TypeVersionInfo` type-level bound fix** | `modelsdk/version` + codegen emitter | The one fidelity bug surfaced by the version analysis. Parser already captures type-level `Introduced`/`Deleted` (`JsVersionInfo`); the generated struct drops it. ~½ day. Do it during Phase-0 codegen vendoring. |
| **mongo-driver v1/v2 coexistence** | go.mod | Confirm both link in one binary (different module paths — expected to work). Validate in Phase 0 before committing to the strategy. |
| **Codegen source** | `cmd/modelsdk-codegen` | Inherit engalar's regex source short-term. The `audit`/`audit-keys` mode → **promote to CI gate** (catches unregistered `$Type`s / `ByIdRef` mismatches). The `mx dump-mpr` re-point and instantiate-and-reflect are **out of scope** here — separate, deferrable (see backend-strategy § Version handling). |
| **Version gating unchanged** | `sdk/versions/*.yaml` | Byte-identical on both branches; the engine swap does not touch it. No work. |

## 5. Decisions to confirm

1. **Vendor-as-reference vs. clean rewrite of the local backend.** Recommendation: **vendor
   engalar's `mdl/backend/mpr` modelsdk files and adapt imports** (faster, battle-tested), since
   `FullBackend` is identical — rather than rewriting the write path from scratch. Confirm.
2. **Flag surface.** `MXCLI_ENGINE` env + hidden `--engine` global. Confirm naming and whether
   `--engine` should be user-visible or hidden until Phase 4.
3. **Cutover default-flip release.** Which release carries the default flip, and how long `legacy`
   stays as an escape hatch (proposed: one release).
4. **Widget strategy at cutover.** Keep ours through cutover and retire `sdk/widgets` in Phase 5
   (recommended), vs. adopt his `modelsdk/widgets` earlier. `compare` mode informs this.

## 6. Effort & risk summary

| Phase | Effort | Risk | Default engine during phase |
|---|---|---|---|
| 0 — Vendor + flag + harness | M | Low | legacy |
| 1 — Read parity | M | Low–Med | legacy |
| 2 — Write slice (domain model) | L | **Med–High** | legacy |
| 3 — Write coverage expansion | L | Med | legacy |
| 4 — Cutover | S | Med | → modelsdk |
| 5 — Cleanup (delete sdk/mpr, v2 migration) | M | Low–Med | modelsdk |

**Long poles:** Phase 2 (the engine/backend seam) and Phase 3 widget parity. Everything before
cutover is reversible; the comparison harness is the safety net that makes the cutover decision
evidence-based rather than a leap.

## 7. First concrete steps

1. Phase 0 spike: vendor `modelsdk/` + `unitstore`, confirm **v1/v2 coexistence compiles**, wire
   the `MXCLI_ENGINE` selector at the factory seam, port `cmd_bson_compare.go`, build the
   canonicalizer, get `make engine-diff` running red-to-green on a single domain-model script.
2. If coexistence compiles and the harness runs, the rest is execution against gates. If it does
   **not** compile cleanly, escalate the v2 question to the front (a forced early tree-wide bump),
   which materially changes the Phase-0 estimate.

---

## 8. Phase-0 spike results (2026-06-05)

Ran the make-or-break parts of step 1 on the `modelsdk` branch. **The strategy's two central
premises hold; the entanglement is smaller and more contained than feared.**

### ✅ v1/v2 coexistence — PROVEN

- Added `go.mongodb.org/mongo-driver/v2 v2.6.0` alongside the existing `v1.17.9`. Both resolve in
  one module (distinct module paths).
- `go build ./modelsdk/... ./mdl/backend/unitstore/...` → **rc=0.** The vendored engine compiles
  on v2 in the same binary as our v1 tree. **The "defer the tree-wide v2 migration to cutover"
  premise is validated** — the v2 dependency is confined to `modelsdk/`.

### ✅ Dependency closure — small and clean

- `modelsdk/` reaches outside its subtree only into `model` and `mdl/types`.
- **All `model.*` symbols it needs already exist on `main`** — no `model` vendoring required.
- `mdl/types` is **bson-driver-agnostic** (`RawUnit.Contents` is `[]byte`, identical both branches),
  so it does *not* drag v2 into shared code.

### ⚠️ The one real entanglement — `mdl/types` java-action refactor

`modelsdk/` needs engalar's `mdl/types` (not just 4 additive files — `UnitPatch`/`RawUnitInfo`
extras live in *modified* existing files). Adopting his `mdl/types` wholesale, then building every
package that imports it (13, excluding the embed-noisy `cmd/mxcli`):

| Result | Packages |
|---|---|
| **build OK (10)** | all `modelsdk/*` engine pkgs, `mdl/backend`, `mdl/backend/mock`, `mdl/catalog`, `mdl/linter`(+rules), `mdl/bsonutil`, `sdk/mpr/version` |
| **break (3)** | `sdk/mpr`, `mdl/backend/mpr` (transitive via sdk/mpr), `mdl/executor` |

**All 3 failures share one root cause:** engalar consolidated the code-action types
(`CodeActionReturnType`, `JavaActionParameter`, `TypeParameter`, …) into `mdl/types` with a
different unexported marker method, while our legacy code still mixes them with the separate
`sdk/javaactions` package. Call sites: `sdk/mpr/parser_misc.go` and
`mdl/executor/cmd_javascript_actions.go`.

**Reconciliation is small and already conventional.** The fix is to make `sdk/javaactions`
re-export the `mdl/types` code-action types as aliases — exactly the CLAUDE.md rule *"new shared
types in `mdl/types/`; `sdk/mpr` re-exports as type aliases."* And `sdk/mpr` is deleted at cutover
anyway, so this is a transitional patch, not permanent debt.

### Pre-existing noise (not engine-related)

`go build ./...` also trips on make-managed generated files — the ANTLR parser (`make grammar`,
run during the spike) and `cmd/mxcli` `//go:embed changelog.md`. These are normal `make build`
steps, unrelated to the engine.

### Net & progress

The Phase-0 gate is **substantially met**: engine vendored, both drivers coexist, engine builds.
No forced early v2 migration.

**Phase-0 checklist (committed on the `modelsdk` branch):**

| Item | Status | Commit |
|---|---|---|
| Vendor `modelsdk/` + `unitstore` + read fixture; `mongo-driver/v2` coexists | ✅ done | `b1536ba7` |
| Java-action `mdl/types` reconciliation (alias re-export; fixes 3 pkgs) | ✅ done | `b1536ba7` |
| `MXCLI_ENGINE` / `--engine` selection seam (legacy wired; modelsdk/compare fail-fast) | ✅ done | `1e8ec679` |
| Vendor engalar codegen + `TypeVersionInfo` type-level bound fix | ❌ **decided against vendoring** | [PROPOSAL_codegen_ownership.md](../11-proposals/PROPOSAL_codegen_ownership.md) — its input (the TypeScript SDK) has no storage names, so regenerating reintroduces 102 wrong BSON keys. Hand overrides + `keyaudit_test.go` ledger instead; `TypeVersionInfo` fix still open |
| Comparison harness — **read side** (`mdl/enginecompare` + `make engine-diff`) | ✅ done | `6a1fd4e2` |
| Comparison harness — write/BSON side (port `cmd_bson_dump`/`cmd_bson_compare`, ID-canonicalizer) | ⏳ Phase-2-gated on the modelsdk write path | — |

`make build` green; engine, backend, and affected packages tested. Legacy path verified end-to-end
against `testdata/expr-checker/minimal.mpr` (SHOW ENTITIES / SHOW MODULES); the three engine guards
(`modelsdk`, `compare`, unknown value) all fail loud with exit 2.

### Phase 1 — read slice (started 2026-06-05)

| Item | Status | Commit |
|---|---|---|
| `mdl/backend/modelsdk` (package `modelsdkbackend`): FullBackend reading via the codec engine | ✅ first slice | `43cbb3b3` |
| Wire `MXCLI_ENGINE=modelsdk` → read backend (read-only warning; writes no-op via mock) | ✅ done | `43cbb3b3` |
| Modules read (`ListModules`/`GetModule*`) — diff-identical to legacy | ✅ done | `43cbb3b3` |
| Entities read (`ListDomainModels`/`GetDomainModel` + gen→`domainmodel` adapter) | ✅ done | `7dd42a1d` |
| Container-tree reads (`ListUnits`/`ListFolders`) for module/folder resolution | ✅ done | `f7b2a020` |
| Microflows read (`ListMicroflows`/`GetMicroflow` + flow-object/param conversion) | ✅ done | `f7b2a020` |
| Pages read (`ListPages`/`GetPage` + title/template handling) | ✅ done | `fb9664da` |
| Nanoflows read (`ListNanoflows`/`GetNanoflow`, reuses microflow helpers) | ✅ done | `24c4428d` |
| Enumerations read (`ListEnumerations`/`GetEnumeration`, ports engalar's converter) | ✅ done | `0a5c532f` |
| Constants read (`ListConstants`/`GetConstant`, ports engalar's converter) | ✅ done | `82f7d7e0` |
| **Read-parity harness** (`mdl/enginecompare`, `make engine-diff`) — 6 strict cases green | ✅ done | `6a1fd4e2` |
| Read coverage beyond constants (security, scheduled events, mappings, …) | ⏳ next | — |

The harness automates what had been manual `diff`s: `make engine-diff` runs each `SHOW` query
through both engines in-process and compares padding/order-normalized output. **All 7 cases now
strict-PASS** (microflows, nanoflows, pages, enums, constants, entities, modules — the last two
with the System row filtered). Every new read type should add a case here, making subsequent type
work **self-validating** and regression-proofed. Write/BSON comparison (the ID-canonicalizer) is
the Phase-2 extension.

**Modules gap closed** (`b01a522f`): added module `Source` (decode the unit for
`FromAppStore`/`AppStoreVersion` — the generic lister can't be used because the
`Projects$ModuleImpl` alias defeats strict type resolution), `ListSnippets`, and `ListJavaActions`
(the remaining non-zero count columns). Non-System modules now match across **every** column. The
System row is filtered, not replicated: legacy injects a hardcoded 11.6.4 System module
(`BuildSystemDomainModel`) because it couldn't read the real System contents, whereas modelsdk
reads the real (sparser) unit — modelsdk is **arguably more correct**, so we treat System as a
principled difference rather than porting the hardcoded data.

Enums confirmed that engalar's `convert_reader.go` *does* have portable converters for the
non-domain-model document types (`enumToModel`/`enumValueToModel`, etc.) — unlike domain models,
where he changed the interface and left nothing to port. So the remaining `model.*`-typed reads
(constants, scheduled events, mappings, business events, …) can mostly **port engalar's
converter + wrap with `ListUnitsWithContainer`**, which is faster than writing from scratch.

Nanoflows were a clean reuse — `nanoflowFromGen` shares `splitFlowObjects`/`dataTypeFromGen`
with microflows unchanged; `SHOW NANOFLOWS` was byte-identical on the first run (13 rows). The
per-document recipe is now well-established and the remaining doc types (enums, constants,
security, etc.) are mostly mechanical, with the cross-cutting infrastructure (container tree,
text registration, prefix-match handling) already solved.

**Two more reusable lessons from pages** (both will recur across remaining doc types):

1. **Strict typing vs legacy prefix-matching.** Legacy's `listUnitsByType("Forms$Page")` is
   *prefix-matched*, so it silently sweeps in `Forms$PageTemplate` (16 pages + 46 templates = 62).
   The modelsdk reader is strict-typed and returned only the 16. To match, `ListPages` explicitly
   reads `Forms$PageTemplate` too. **Watch for other prefix collisions** (the module `ModuleImpl`
   case was the same family). The modelsdk strictness is arguably *more correct*; we replicate the
   quirk for parity and can fix both engines later.
2. **Child-element gen packages must be registered.** Page titles came back empty because
   `Texts$Text` wasn't registered — the codec decoded the title child as bare `element.Base`, and
   the interface-based `textElementToModel` silently returned nil. Fix: blank-import the child's
   gen package (`modelsdk/gen/texts`). Any converter that reaches into nested element types must
   ensure those packages' `init()` registrations have run.

**Validated:** `SHOW PAGES` byte-identical to legacy across all 62 rows (16 pages + 46 templates),
titles included.

**Third discovery — renderers need the container tree, not just the doc converter.** Microflows
read fine (16 units) but `SHOW MICROFLOWS` initially dropped *every* row: the renderer resolves
each flow's module from its `ContainerID` via `ContainerHierarchy` (`FindModuleID`/
`BuildFolderPath`), which is built from `ListModules` + **`ListUnits`** + **`ListFolders`**.
Flows are nested in folders, so without those two reads folder→module resolution fails silently.
Lesson for the remaining read domains: implementing the doc converter is necessary but not
sufficient — the supporting container/metadata reads must be present too. (`ListUnits`/`ListFolders`
are now done, so later doc types inherit working module/folder resolution.)

**Validated:** `SHOW MICROFLOWS` byte-identical to legacy across all 16 microflows and every
column (params, actions, McCabe, returns). `SHOW MODULES` aggregate counts now match legacy for
the implemented doc types (entities, microflows); the rest converge as reads land.

**Two discoveries this slice:**

1. **We own the gen→`domainmodel` adapter — there is nothing to port.** engalar changed the
   `DomainModelBackend` interface to traffic in `*genDm` types (`GetDomainModelGen`, …) and
   **deleted `sdk/domainmodel`** entirely. Keeping main's executor and `domainmodel` types
   canonical means the translation is net-new and ours. It is the concrete, recurring cost of
   "vendor, don't adopt" — each read domain is *override + delegate to `mprread` + convert*.
   The entity converter (`mdl/backend/modelsdk/domainmodel.go`) is the template.
2. **System-module injection gap.** Legacy injects the whole System module from hardcoded
   `sdk/mpr/system_module.go` (`BuildSystemDomainModel`); the codec reader returns only real
   project units, so `SHOW ENTITIES` via modelsdk omits System.*. **All 8 non-System entities
   are byte-identical to legacy** (type, Extends, every count). Closing the gap = surface the
   same synthetic System module from the modelsdk backend (reuse/relocate `BuildSystemDomainModel`,
   which is coupled to the legacy `sdk/mpr` package today). Tracked, not yet done.

**Thesis validated.** The new backend embeds `*mock.MockBackend` (satisfies all 27 sub-interfaces;
un-overridden methods are safe zero/nil stubs) and overrides only connection + module reads,
delegating to `modelsdk/mpr.Reader`. `MXCLI_ENGINE=modelsdk show modules` returns a module list
**byte-identical to legacy** (10 modules on `testdata/expr-checker`) — reads flow through
`FullBackend` on the codec engine **without importing engalar's executor/backend rework**. This is
the central "vendor, don't adopt" bet, now demonstrated on a real read. Un-ported reads (entities,
etc.) fall through to empty stubs rather than crashing.

**Next read targets** (each: override the method, delegate to `mprread`/Reader, convert gen→our
types, diff vs legacy): domain models/entities → microflows → pages. These are where the
gen→`domainmodel`/`model` conversion cost (engalar's `convert_reader.go`) gets sized for real.

## 9. Phase 2 — write-path scope (detailed, 2026-06-06)

Investigated the modelsdk write API and engalar's reference write path to size the write slice.
Phase 1 (reads) is essentially done — eight doc types + the module aggregate pass `make
engine-diff` strictly. Phase 2 turns the read-only slice into a writing engine.

### The write mechanics (how a mutation persists)

The reference flow, from engalar's `CreateEntityGen` → `UpdateDomainModelGen`:

```
domainmodel.Entity  ──[write adapter]──▶  genDm.Entity
        │
        ▼  load DM unit, assign IDs, dm.AddEntities(child)  (marks dirty)
   genDm.DomainModel (element tree)
        │  codec.Encoder.Encode(dm)        ◀── roundtrip: clean children pass
        ▼                                       through verbatim, only the new
   BSON bytes                                   child is freshly encoded
        │  Writer.UpdateRawUnit(dmID, bytes)  (entities are CHILDREN of the
        ▼                                      DomainModel unit, not their own units)
   modelsdk/mpr.Writer  ──▶  WriteTransaction / disk
```

Key facts:
- **Entities are children of the DomainModel unit.** `CreateEntity` is *mutate the DM unit*
  (`UpdateRawUnit`), not `InsertUnit`. Top-level docs (modules, microflows, pages) are
  `InsertUnit`. So two write shapes: child-mutation and new-unit.
- **The roundtrip encoder is the reliability core.** Encoding the dirty DM re-emits only the new
  child; every untouched sibling entity passes through as its original raw bytes. This is exactly
  what should make modelsdk's BSON match legacy's — and exactly what the canonicalized BSON diff
  verifies.
- Write API is small and present: `Writer.InsertUnit / UpdateRawUnit / DeleteUnit`,
  `BeginWriteTransaction`, `codec.Encoder.Encode`. `unitstore.BufferedUnitStore` batches writes
  (optional for the slice; direct writer is fine first).

### The real cost: the write-direction adapter (`domainmodel → gen`)

Just as engalar deleted `sdk/domainmodel` and we own the **read** adapter (`gen → domainmodel`),
we own the **write** adapter (`domainmodel → gen`). It is *harder* than read: read populated only
the fields a renderer shows (often counts); write must construct a **complete, valid** gen element
— every attribute with its data type, the generalization (`NoGeneralization` with persistability +
system flags, or `Generalization` with the parent ref), access rules, etc. — because that element
becomes the persisted BSON. This per-doc-type adapter is the bulk of Phase 2/3 effort; the
write infrastructure (writer, encoder, ID-gen, canonicalizer) is one-time.

### Vertical slice (smallest end-to-end proof)

`CREATE PERSISTENT ENTITY Module.Foo` with no attributes:
- exercises the full path — `domainmodel.Entity → genDm.Entity` (name + `NoGeneralization{Persistable:true}`),
  DM child-mutation, **roundtrip encode** (sibling entities preserved), `UpdateRawUnit`, commit;
- is the minimum that proves the roundtrip encoder writes a Studio-Pro-valid unit;
- validates two ways: `mx check` clean **and** canonicalized BSON diff vs legacy.

(`CREATE MODULE` is an even smaller `InsertUnit`-only warm-up if the DM-mutation path needs
de-risking first.)

### Backend changes for the slice

- `Connect` must open **read-write** (today `OpenOptions{ReadOnly: true}`) and hold a
  `Writer` / `WriteTransaction`; `Commit`/`Disconnect` flush.
- Override the write methods (`CreateEntity`, …) — they currently no-op via the embedded mock,
  which is why `--engine modelsdk` writes silently don't persist today.
- Reuse engalar's `assignEntityIDsGen` / `AddEntities` mutation helpers as reference (adapt
  imports; do **not** pull his executor rework — `FullBackend` is the firewall).

### Write/BSON comparison (extends the harness)

The read harness diffs rendered `SHOW` output; writes need the **BSON canonicalizer** (plan §2):
run the same `CREATE …` script through both engines on **separate copies** of a fixture, then for
each changed `.mxunit`: decode → remap `$ID`/GUID binaries to ordinal tokens in document-walk order
(preserving referential structure) → mask volatile fields → diff. A residual diff is a real
divergence. This is the one genuinely new piece of tooling; engalar's `cmd_bson_dump`/
`cmd_bson_compare` are the porting base.

### Risks / open questions

1. **Encoder fidelity on fresh elements.** For a *new* entity (no raw bytes) the encoder emits
   `$ID`+`$Type`+set fields in `Properties()` order. Whether that byte-for-byte matches legacy's
   hand-built BSON (field order, defaults, optional fields) is unknown until the canonicalized diff
   runs — this is the main thing Phase 2 discovers, and where subtle divergences will surface.
2. **ID nondeterminism.** New units get fresh UUIDs differing per engine/run; the canonicalizer's
   ID-remap must neutralize them (and `dataStorageGuid`-class fields) or every write "diffs".
3. **Transaction/commit semantics.** Match legacy's commit boundaries (per-statement vs per-script)
   so the persisted result and the executor's connection lifecycle agree.
4. **The 22 engine/backend-straddling commits.** The write path is where `modelsdk/` and
   `mdl/backend/` entangle most; porting must stay on the `FullBackend` side of the firewall.
5. **Write-adapter completeness.** A partial `domainmodel → gen` adapter writes an *incomplete*
   unit → Studio-Pro rejection. Unlike reads (partial is harmless), writes need full per-type
   field coverage. Size each doc type before committing to it.

### Sequencing & effort

1. **Infra (one-time):** read-write `Connect` + writer/transaction; the BSON canonicalizer +
   write-compare harness mode. *(M)*
2. **Vertical slice:** `CreateEntity` (empty persistent) → green on `mx check` + canonical BSON
   diff. *(M, the de-risking milestone — proves the encoder writes valid, legacy-matching BSON)*
3. **Domain-model breadth:** attributes (+ data types), associations, generalization, access
   rules — the write adapter grows. *(L)*
4. **Other doc types (Phase 3):** microflows, pages, … each = write adapter + harness case.

**Effort:** L overall; **Risk: Med–High**, concentrated in step 2 (encoder fidelity is the
unknown). Everything stays behind `MXCLI_ENGINE`/`compare`; `legacy` remains the default and the
source of truth until the canonical-BSON gate is consistently green.

### Phase 2 slice results — CreateEntity (2026-06-06)

Built the `CreateEntity` write slice + the BSON write-parity harness (`3d1a50f7`). The
encoder-fidelity question is **answered, and the news is good**:

- **Write path works end-to-end.** Connect opens read-write; `CreateEntity` runs the
  `domainmodel→gen` adapter, mutates the DM element, roundtrip-encodes, `UpdateRawUnit`.
  Create → reopen → the entity persists with the right properties.
- **Cross-engine compatible.** The *legacy* engine reads the modelsdk-written project
  correctly — strong structural-validity evidence absent a 10.24 `mx` locally.
- **BSON parity: every emitted field matches legacy** — `$Type` (`DomainModels$EntityImpl`),
  Documentation, ExportLevel, Location, Name, and the full `MaybeGeneralization`
  (`NoGeneralization{Persistable}`) — verified by `CanonicalizeRaw` (walks `bson.Raw`;
  `bson.M` unmarshal mis-decodes Mendix typed arrays) with IDs/GUIDs masked.
- **Found+fixed an over-emission:** the adapter initially set `HasOwner/…=false`, which
  legacy omits; now flags are set only when true → `MaybeGeneralization` matches exactly.

**The lone residual** (tracked, `write_test` self-flags when closed): modelsdk omits the
entity `GUID` and the **empty** member arrays (`Attributes/AccessRules/ValidationRules/
Indexes/Events`, which legacy emits as a single typed-array marker `[3]`). Root cause is
**engine-level, not the adapter**: `genDm.NewEntity` has a pending `applyDefaults` (engalar
Fix 4) and the encoder only emits *dirty* properties for fresh elements (so unset PartLists
and the unsettable `GUID` don't appear). Studio Pro / legacy read it fine → completeness, not
correctness. **Decision:** close it at the engine layer (applyDefaults + GUID), not by hand in
each adapter — it's shared across all write types, so fixing it once unblocks the breadth step.

**Net:** the crux risk of Phase 2 (does the codec write legacy-matching BSON?) is retired —
the engine writes *correct, parity* BSON for everything it emits; the only work left for full
parity is the shared `applyDefaults`/GUID engine fix, then growing the write adapter
(attributes, associations, …).

### Phase 2 — applyDefaults target CONFIRMED via Studio Pro (2026-06-06)

Validated against the live MCP backend (now on main) + real Studio-Pro BSON in
`mx-test-projects/test7-app` (11.x):

- The MCP backend creates entities end-to-end against a live Studio Pro
  (`mxcli --mcp … -c "CREATE PERSISTENT ENTITY …"` → "Created entity").
- `ped_get_schema DomainModels$Entity` (the creation *contract*) contains only
  name/attributes/location/generalization/source — **no GUID, ExportLevel,
  member arrays, or persistability**. Proof that Studio Pro applies those
  **internally** on create — i.e. `applyDefaults` is exactly Studio Pro's own behaviour.
- A real Studio-Pro entity on disk (`ObjListV10.Location`) carries the full set:
  `$ID, $Type=EntityImpl, Name, Documentation, ExportLevel="Hidden", GUID, Location,
  MaybeGeneralization, Attributes, AccessRules, ValidationRules, Indexes, Events`
  (empty collections encoded as the `[3]` marker).

**Conclusions:** (1) the modelsdk fresh-entity residual (missing GUID + 5 empty member
arrays) is a gap vs **real Studio Pro**, so `applyDefaults`/GUID is required for true
fidelity, not just legacy parity. (2) **Legacy's field set matches Studio Pro exactly**,
so the legacy baseline used in the read/write harness is authoritative-equivalent for
entities. (3) MCP is validated as the live oracle / a real 4th backend on the `MXCLI_ENGINE`
seam (`--mcp`/`--mcp-dial`).

### Phase 2 — applyDefaults/GUID fix DONE (2026-06-06)

Closed the fresh-element serialization gap (`09c0b6d1`):
- `codec.TypeDefaults` + `RegisterTypeDefaults` — a per-`$Type` registry of the defaults
  Studio Pro applies internally (`EmitGUID` = $ID-as-binary; `MandatoryLists` = PartList keys
  always serialized, empty = the `[3]` marker). The encoder consults it for **new elements
  only**, so it can't affect any unregistered type. Hand-maintained stand-in for codegen-from-
  reflection-data (engalar Fix 4); extensible per type.
- `modelsdkbackend` registers `DomainModels$EntityImpl` (GUID + the 5 member arrays).

**Result:** the enginecompare **write-parity case is now a STRICT gate** — modelsdk `CreateEntity`
BSON is canonically identical to legacy (byte-faithful to Studio Pro). Directly confirmed on a
real **test7 (11.x)** copy: the created entity's field set matches a real Studio-Pro entity.
codec roundtrip tests unaffected; broad sweep green.

**Remaining for write breadth:** register `DomainModels$Attribute` / `DomainModels$Association`
defaults (each carries a GUID) as those write paths land, and grow `entityToGen` to convert
attributes/associations/generalization detail. The applyDefaults *mechanism* is now in place, so
each new write type is just a converter + a registry entry.

### Phase 2/3 write breadth — generalization ✅, validations ✅, indexes ⚠️ (2026-06-06)

- **Generalization (EXTENDS)**: already worked via SetGeneralizationQualifiedName; added a
  regression test.
- **Validation rules**: implemented (Required/Unique) — attribute by qualified name, Message
  text (Texts$Text + Translations), RuleInfo. Required a vendored-gen storage-key correction:
  the message property serialized as `ErrorMessage` but Studio Pro's key is `Message` (verified
  vs test7 BSON) — patched in gen with a tracking comment; permanent fix is a supplements.json
  override once codegen is vendored. Write-parity strict.
- **Indexes ⚠️ DEFERRED — legacy baseline is unreliable here.** This is the first write type
  where the **legacy serializer disagrees with real Studio Pro**: legacy emits a `SortOrder`
  string on `IndexedAttribute`, but the authoritative MCP model schema (`ped_get_schema
  DomainModels$Index`) shows `ascending`(bool) + `type`(Normal/CreatedDate/…) + `attribute`
  (by-id) — which matches the **gen** metamodel, not legacy. Legacy's `SortOrder` is almost
  certainly stale (older Mendix). Consequences:
  - The gen-native serialization (`Ascending`/`Type`/`AttributePointer`) is very likely correct
    for 11.x, but **the write-parity-vs-legacy gate cannot confirm it** (legacy is wrong).
  - **Resolution = an MCP index-BSON capture**: add an index via MCP → save → dump the `.mxunit`
    to confirm the exact 11.x keys, then implement to match (and likely *fix legacy* too).
  - No index conversion was shipped — better to leave it unimplemented than ship a guess against
    an unreliable baseline. (Indexes are rare; the inline `index(...)` path stays legacy-only for
    now.) This is exactly the case the MCP oracle exists for.

### Index spec resolved via MCP capture (2026-06-06)

Created `MxcliDiskProbe.IdxProbe` with an index via Studio Pro (MCP entity create + UI
index add + save), then dumped the real 11.x `.mxunit`. Authoritative index BSON:

```
EntityIndex { GUID, IncludeInOffline=false, Attributes=[<marker 2>,
  IndexedAttribute { Ascending(bool), Type("Normal"), AttributePointer(bin), AssociationPointer(bin) } ] }
```

Findings (the MCP oracle earning its keep):
1. **Legacy is WRONG for 11.x** — it emits `SortOrder`(string); real Studio Pro uses
   `Ascending`(bool)+`Type`+`AttributePointer`. Legacy has a real index-serialization bug for
   11.x worth fixing in its own right (it's the default engine).
2. **gen `IndexedAttribute` is missing `AssociationPointer`** (binary) — same class as the
   ErrorMessage→Message gap; needs a gen correction / supplements entry.
3. **🔴 Per-list typed-array marker.** The inner index `Attributes` list uses marker **2**, but
   the encoder hardcodes `int32(3)` for every PartList. So the marker is per-element-list-type
   (metamodel-driven), not universal — a real encoder limitation, not a converter detail.

**Status: indexes still not shipped.** Unlike the other write types, correct index output needs
(a) a per-list-type array marker in the shared encoder (metamodel-driven), (b) the gen
`AssociationPointer` fix, and (c) the converter + EntityIndex GUID/IncludeInOffline defaults.
That's a bigger, encoder-touching change — a focused follow-up, not a quick converter add. The
spec is now captured and authoritative, so the follow-up is unblocked.

(Scratch entities `MxcliDiskProbe.IdxProbe` + `OracleTmp` were saved into test7-app during the
capture — safe to delete.)

### Indexes shipped + legacy SortOrder bug fixed (2026-06-06)

Both halves done and validated against the real Mendix validator:

**(a) modelsdk index writes** — codec gains a per-child-type PartList marker registry
(`RegisterListMarker`; IndexedAttribute lists use 2, not the default 3) and a `ZeroGUIDFields`
default (emits `AssociationPointer` as an all-zero GUID). The converter builds `EntityIndex`
(EmitGUID, IncludeInOffline=false) + `IndexedAttribute` (Ascending+Type("Normal")+AttributePointer);
the domainmodel attribute ID is carried onto the gen attribute so AttributePointer resolves.

**(b) legacy index writer** — `serializeIndex`/`serializeIndexAttribute` rewritten to emit the
11.x shape (marker 2, GUID, IncludeInOffline, Ascending+Type+AttributePointer+AssociationPointer),
replacing the stale `SortOrder` string + marker 3 + missing fields. Safe because the legacy
*parser* already reads `Ascending` (with a `SortOrder` fallback for old projects) — writer and
parser are now aligned. No version gating exists or was added; the parser's dual-read is the
back-compat mechanism.

**Validation (mx check, Mendix 11.10.0, test6-app):** baseline (no index) = legacy+index =
modelsdk+index = **17 errors** (all pre-existing OdTest/OData issues); **zero** index-related
errors in either engine. Both engines now produce byte-identical, Studio-Pro-faithful index BSON.
`TestWriteParity_Index` asserts both match the MCP-captured truth. The modelsdk write slice now
covers entity/attributes/associations/generalization/validations/indexes.

### Domain-model deletes shipped; ALTER needs lossless reads (2026-06-06)

Added `DeleteEntity` (+ legacy's cross-DM association cascade), `DeleteAttribute`, and
`UpdateEntity` to the modelsdk backend. `DROP ENTITY` passes strict parity (kept entity is
byte-faithful passthrough; dropped entity + dangling associations removed in both engines).

**Blocker surfaced for ALTER ENTITY.** The executor routes *every* ALTER op (rename, doc,
add/modify/drop attribute, generalization, index) through `UpdateEntity(domainmodel.Entity)` —
a read-modify-write. `UpdateEntity` is implemented and correct (rebuilds the target via
`entityToGen`, siblings pass through in order), but it's bottlenecked by a **lossy read
adapter**: `entityFromGen` carries only attribute *names* (no type/length/default), drops entity
`Location`, and reduces indexes/validations/access-rules/events to bare IDs. `entityToGen` also
doesn't yet rebuild access rules or event handlers. So a round-tripped entity diverges from
legacy (proven: `TestWriteParity_DropAttribute`, currently skipped).

**Next gate: a lossless modelsdk read adapter.** Make `entityFromGen`/`attributeFromGen`
round-trip-complete (attribute types incl. Length/enum-ref/default, Location, full
index/validation/access-rule/event detail) and extend `entityToGen` to match. This unblocks ALL
ALTER ENTITY ops at once. The codec's passthrough strength suggests an alternative worth
weighing: have ALTER mutate the decoded gen tree directly instead of round-tripping through the
lossy domainmodel.Entity — but that diverges from the executor's current UpdateEntity contract.

### Lossless read adapter → ALTER ENTITY shipped (2026-06-06)

Made the modelsdk read adapter round-trip-complete for the domain-model entity, unblocking ALTER:
- `attributeFromGen` — full type (incl. StringAttributeType Length, EnumerationAttributeType ref)
  via `attributeTypeFromGen`, plus Documentation and StoredValue DefaultValue.
- `entityFromGen` — entity Location (`parseLocation`).
- `indexFromGen` — index segments (AttributePointer→AttributeID + Ascending).
- `validationRuleFromGen` — attribute ref (qualified name), rule type, error Text (`textFromGen`).

`TestWriteParity_AlterEntity` is green across AddAttribute, RenameAttribute, ModifyAttribute,
SetDocumentation, AddIndex, DropIndexKeepsAttrs, AlterKeepsValidation — all strict legacy parity,
all read-modify-write (two-session) so they exercise the codec passthrough path. No read-parity
regression (TestReadParity still green).

**Remaining gap (guarded, not silent): access rules + event handlers.** `entityToGen` doesn't
rebuild them and the adapter doesn't round-trip them yet, so `UpdateEntity` now *refuses* to ALTER
an entity that has either ("use the legacy engine") rather than silently dropping them. Closing
this needs `accessRuleFromGen`/`eventHandlerFromGen` + `entityToGen` support (MemberAccess, roles,
XPath) — a focused follow-up. modelsdk write slice now: entity/attributes/associations/
generalization/validations/indexes (create) + DROP ENTITY + ALTER ENTITY (no access-rule entities).

### Access rules round-trip; full-codec decision recorded (2026-06-06)

ADR-0004 records the decision: route **all** document types through the codec (not engalar's
implicit hybrid). Investigating the engalar fork confirmed engalar never codec-ified domain-model
writes — `feat/modelsdk-core` ships the codec as a library but every domain-model write still
delegates to `sdk/mpr`. So our codec-based domain-model write path is net-new.

Closed the access-rule half of the ALTER gap:
- `accessRuleFromGen`/`memberAccessFromGen` (lossless read: module roles by name, allow flags,
  default rights, XPath, per-member accesses) and `accessRuleToGen`/`memberAccessToGen` (forward).
- The codec already handles the marker-1 `ByNameRefList` (AllowedModuleRoles), so no codec change.
- **Member-access sync**: on write, every access rule gets a MemberAccess for each attribute, new
  ones joining with the rule's default rights — verified (via raw BSON) to match what legacy /
  Studio Pro write when an attribute is added to an access-controlled entity. (Legacy's own source
  for this sync was not locatable statically; we matched the observed on-disk output and gated it
  with `TestWriteParity_AlterKeepsAccessRule`.)

`UpdateEntity` now only refuses entities with **event handlers** — the gen wires BSON keys
Event/PassEventObject while legacy emits Type/SendInputParameter, a discrepancy needing a real
Studio-Pro reference to settle (an MCP capture, like the index spec) before it can be round-tripped.
modelsdk domain-model write surface: create (entity/attrs/assoc/gen/validations/indexes) + DROP
ENTITY + ALTER ENTITY (incl. access-rule entities; event-handler entities still deferred).

### Event handlers round-trip; last ALTER guard removed (2026-06-07)

MCP capture (test7-app IdxProbe, save + dump) settled the gen-vs-legacy disagreement — and this
time **legacy was right, the gen was wrong**. Real Studio-Pro 11.x event-handler BSON:
`EntityEvent { Microflow=<by-name>, Moment, RaiseErrorOnFalse, SendInputParameter, Type }`.

Three gen storage-name bugs fixed (the gen both read and wrote the wrong keys, so event-handler
reads silently returned 0):
- `EventHandler` field `Event` → BSON key **`Type`** (init + InitFromRaw lookup).
- `EventHandler` field `PassEventObject` → BSON key **`SendInputParameter`**.
- `Entity.eventHandlers` list `EventHandlers` → BSON key **`Events`** (NewPartList + DecodeChildren;
  must match the "Events" MandatoryList default, else a dirty entity double-emitted the list).

Wired `eventHandlerFromGen`/`eventHandlerToGen`, added event handlers to `assignEntityIDs`, and
**removed the last `UpdateEntity` guard**. `TestWriteParity_AlterKeepsEventHandler` is green (setup
via legacy, ALTER via each engine, strict parity). The gen fix also corrects event-handler reads
(SHOW ENTITIES event counts). No read or write regression.

modelsdk domain-model write surface is now complete for ALTER: entity/attrs/assoc/gen/validations/
indexes (create) + DROP ENTITY + ALTER ENTITY with **no remaining guards** (access rules AND event
handlers round-trip). Two storage-name overrides (this + ErrorMessage→Message) should move to
supplements.json when the codegen is next run. (Scratch in test7-app/MxcliDiskProbe: IdxProbe now
also carries two duplicate event handlers from the capture — safe to delete.)

### Enumerations CREATE/DROP/ALTER (2026-06-07)

First top-level-document writes in modelsdk (units, not domain-model children):
- `CreateEnumeration` → build gen Enumeration + `InsertUnit` (containment "Documents").
- `DeleteEnumeration` → `DeleteUnit`. `UpdateEnumeration` → rebuild + `UpdateRawUnit`.
- `enumToGen`/`enumValueToGen` mirror the legacy serializer (Excluded=false, ExportLevel=Hidden,
  Image="", captions as Texts$Text); registered defaults: Enumeration/EnumerationValue NullFields
  (RemoteSource/RemoteValue), Texts$Text MandatoryLists [Items] (empty captions emit Items=[3]).
- Added `EnumCanonBSON` to the harness. `TestWriteParity_Enumeration` (CreatePlain, CreateCaptioned,
  AlterAddValue) + `TestWriteParity_DropEnumeration` are green — strict legacy parity.

Also wired the previously-stubbed **executor ALTER ENUMERATION** handler (engine-agnostic: ADD VALUE
/ RENAME VALUE read-modify-write through UpdateEnumeration) — so ALTER ENUMERATION now works for
*both* engines. (DROP VALUE has an executor branch but no grammar rule yet; left as defensive code.)
Updated the CLI syntax help and added a doctype-test example.

### Constants CREATE/DROP/(modify) (2026-06-07)

Second top-level-document type, same unit-write pattern as enumerations:
- `CreateConstant` (InsertUnit) / `DeleteConstant` (DeleteUnit) / `UpdateConstant` (UpdateRawUnit).
- `constToGen` mirrors the legacy serializer; `constantDataTypeToGen` (reverse of the read adapter)
  builds the DataTypes$* element (Long→IntegerType, Date→DateTimeType, per legacy).
- Verified against a real on-disk constant (test7-app FeedbackModule.ClientIdentifier): Studio Pro
  emits only the `Type` element, **no** `DataType` primitive — so `constToGen` deliberately omits it.
  No applyDefaults needed (all scalars + the Type element).
- Added `ConstCanonBSON`. `TestWriteParity_Constant` (String/Integer/Decimal/Boolean + CREATE OR
  MODIFY → UpdateConstant) and `TestWriteParity_DropConstant` are green — strict legacy parity.

Constants have no `ALTER CONSTANT` statement; the "alter" path is `CREATE OR MODIFY CONSTANT`
(already wired in the executor → UpdateConstant), so no executor changes were needed. The read
adapter was already lossless, so the modify round-trip needed no read-side work.

### Microflows: codec-native CREATE conversion — foundation (2026-06-07)

Decision (user): build the full microflowToGen CREATE conversion now (codec-native, the path to
deleting sdk/mpr); ALTER comes later via the mutator pattern (OpenMicroflowForMutation, passthrough
diff-fidelity). Microflows are create-only today, so CREATE has no passthrough benefit on its own —
it's the prerequisite for sdk/mpr removal, not an end in itself.

Foundation laid this session:
- **gen typo bug fixed + committed**: real BSON key is `ConcurrenyErrorMessage` (Mendix's typo), gen
  wired `ConcurrencyErrorMessage` on encode+decode → silently failed to round-trip. Verified vs
  test7-app ACT_Callee. (Legacy was right.)
- `MicroflowCanonBSON` added to the harness.
- Captured the real wrapper field set (test7 ACT_Callee, v11): $ID, $Type, AllowConcurrentExecution,
  AllowedModuleRoles, ApplyEntityAccess, ConcurrencyErrorMicroflow(""), ConcurrenyErrorMessage
  (Texts$Text), Documentation, Excluded, ExportLevel, Flows[3,…], MarkAsUsed, MicroflowActionInfo
  (null), MicroflowReturnType, Name, ObjectCollection, ReturnVariableName, StableId, Url(""),
  UrlSearchParameters[1], WorkflowActionInfo(null). v10+ gates ReturnVariableName/StableId/Url/
  UrlSearchParameters (legacy gates on majorVersion; fixture minimal.mpr = 11.6.6).

Reference: legacy serializeMicroflow (wrapper), serializeMicroflowObjectCollectionWithoutFlows
(22 lines; merges mf.Parameters into Objects), serializeMicroflowObject (dispatch @ line 486),
serializeSequenceFlow (170), serializeMicroflowDataType (return/param types; nil→VoidType).
gen has all wrapper props + NewMicroflow/StartEvent/EndEvent/MicroflowObjectCollection/SequenceFlow/
MicroflowParameter/ActionActivity + setters (SetObjectCollection/AddFlows/AddObjects/SetMicroflow
ReturnType/OriginID/DestinationID/…). Backend version via b.ProjectVersion().MajorVersion.

**Next step (skeleton): microflow_write.go** — CreateMicroflow/Update/Delete (unit-write pattern);
microflowToGen wrapper (all fields, v10+ gated) + microflowReturnTypeToGen + ObjectCollection
(microflowObjectToGen dispatch: StartEvent/EndEvent/MicroflowParameter first) + Flows
(sequenceFlowToGen) + assignMicroflowIDs. Parity-test the simplest microflow (start→end), then add
activity groups incrementally (object ops → splits → loops → calls → java/REST).

### Microflows: skeleton CREATE at parity (2026-06-08)

`microflow_write.go` — Create/Update/Delete (unit-write pattern) + `microflowToGen` wrapper +
`microflowObjectToGen` (StartEvent/EndEvent) + `sequenceFlowToGen` (v10+ CaseValues marker-2 + Line
BezierCurve, NoCase default) + `microflowDataTypeToGen` (return/param types) + `assignMicroflowIDs`.
`TestWriteParity_Microflow_Skeleton` (CREATE MICROFLOW … RETURNS BOOLEAN BEGIN RETURN true END,
start→end, no params) is green — byte-identical to legacy.

Two new findings while reaching parity:
- **StableId is binary, not a string** — the gen mistypes it as a string primitive (so reads were
  broken and writes emitted a string). Added a `FreshGUIDFields` codec default (emits a fresh GUID
  binary subtype 0) and registered it for Microflows$Microflow; StableId is no longer set as a string.
- AllowedModuleRoles needed explicit emission (empty marker-1 by-name list), like UrlSearchParameters.
- Registered CaseValues list marker 2 (keyed by the case child types: NoCase/Enumeration/Expression/
  Inheritance), mirroring the index IndexedAttribute marker.

Known gap deferred to the parameters group: the gen MicroflowParameter is missing Documentation/
HasVariableNameBeenChanged/IsRequired/DefaultValue (legacy emits them) — skeleton uses a no-param
microflow. Next: parameters group, then activity groups (object ops → splits → loops → calls → java/REST).
