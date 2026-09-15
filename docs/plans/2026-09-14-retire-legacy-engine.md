# Implementation Plan — Retire the legacy engine

**Date:** 2026-09-14
**Status:** Phases 1–3 complete (2026-09-15). Phase 3 closed by *deciding* rather than by porting:
the six remaining abstraction bypasses are raw-unit debugging commands and are **accepted as
deliberate** — see [§7.4](#phase-3-closed-the-last-six-bypasses-are-accepted). Phase 4 (the
mongo-driver migration) is the only phase outstanding, and it is what gates deleting `sdk/mpr`.
See [§7 What landed](#7-what-landed) for the record, including the two places the plan was wrong.
**Continues:** [`2026-06-05-adopt-modelsdk-engine.md`](2026-06-05-adopt-modelsdk-engine.md), which
stops at the cutover. That plan still reads as though `legacy` were the default; it is not, and has
not been since the codec engine took over. This plan covers what the earlier one deferred to
"Phase 5 — Cleanup" and never specified.
**Related:** [ADR-0004](../13-decisions/0004-full-codec-engine.md) (route all document types through
the codec), [ADR-0002](../13-decisions/0002-backend-abstraction.md) (the seam this all hangs off).

---

## 1. The plan in one paragraph

Retiring the legacy engine is **three separable removals wearing one name**, and the whole value of
writing this down is refusing to treat them as one job. Deleting the legacy *backend*
(`mdl/backend/mpr`, 2,808 lines) is small, unblocked and reversible — that is Phase 1 and it can
start today. Deleting the legacy *serializer* underneath it (`sdk/mpr`, 41,418 lines) is not a
serializer problem at all: it is blocked by two callers that **bypass the backend abstraction**
rather than by the serializer's size, and measuring them (§Phase 3) put that work at a 17-method
port plus six unimplemented methods, not a rewrite. The mongo-driver v1→v2 migration the earlier
plan promised at cutover is **gated on the second, not the first** — the opposite of what that plan
assumed, and the reason it never started.

## 2. Why this is not one job (the measurement that reorders everything)

The 2026-06-05 plan deferred the tree-wide driver migration "to the cutover", on the reasoning that
the two engines could coexist because "v1 and v2 are different module paths". They can, and they
still do. But the split does not fall where that plan implies:

| Package | mongo-driver v1 | v2 | What it is |
|---|---|---|---|
| `modelsdk/` | **0** | **117** | the codec engine |
| `sdk/mpr` | **114** | **0** | the legacy serializer |
| `mdl/backend/modelsdk` | 44 | 45 | the adapter where the two meet |
| `mdl/executor` | 18 | 5 | |

The driver split maps almost exactly onto the **serializer** split, not onto the engine flag. The
codec is already wholly on v2; `sdk/mpr` is wholly on v1; the adapter straddles both because it
converts between semantic types and gen documents. So **deleting the legacy backend does not move
the driver migration at all** — what unblocks v2 is deleting `sdk/mpr`, which is Phase 3.

Stating this is the point of the plan. Sequenced the other way round, Phase 1 looks like it owes a
41k-line migration and never gets started.

## 3. What is already true (verified 2026-09-15 against main, not assumed)

Every one of these was checked rather than inherited from the earlier plan:

- **No feature falls back to legacy.** No "not supported by the modelsdk engine" message exists
  anywhere in the tree. The last five widget gaps are closed
  (`mdl/backend/modelsdk/widget_write_legacy_gaps.go` records which were real: the name-based scan
  said twenty-three, the reachable set was five, and one of those five turned out to be a Mendix
  type that does not exist).
- **`errUnimplemented` cannot fire on a default run.** Of 19 unported `FullBackend` methods, a
  per-method build probe found 17 dead and 2 live; the live pair is implemented and the dead set is
  pinned by `mdl/backend/modelsdk/unimplemented_reachability_test.go`, which passes.
- **The dependency runs one way.** `mdl/backend/modelsdk` does not import `mdl/backend/mpr`; its
  reader is `modelsdk/mpr`. (A grep says otherwise — it matches a comment. Check the import block.)
- **Legacy is strictly weaker.** Rules, menus, layouts, message definitions and regular expressions
  all refuse on it.
- **The legacy backend has five production importers**: `cmd/mxcli/engine.go` (the factory seam),
  `mdl/repl/repl.go`, `mdl/enginecompare/compare.go`, and two `examples/`. Plus six test files.

## 4. Phases

### Phase 1 — Delete the legacy backend and its flag *(Effort: S, Risk: Low)*

Everything in §3 says this is unblocked. It is the phase that delivers the stated goal.

1. Remove `engineLegacy` and `engineCompare` from `cmd/mxcli/engine.go`; the flag and
   `MXCLI_ENGINE` become either absent or a one-value no-op (see §5, decision A).
2. Delete `mdl/backend/mpr/` and its tests.
3. Delete `mdl/enginecompare/` and the `engine-diff` make target. The package compares two engines
   and has no meaning with one; its own header still describes write comparison as "Phase 2", which
   never landed.
4. Drop the `engines` matrix leg from `.github/workflows/nightly.yml`. Its comment already says
   *"drop this line only together with the engine itself"* — this is that moment. Per-push CI
   already runs modelsdk alone.
5. Repoint the two `examples/` and the six test files at the codec backend.
6. **`mdl/repl.New()` hardcodes `mprbackend.New()` as its factory default.** Not a live bug —
   `cmd/mxcli/main.go:171` overrides it immediately — but it must go with the package, and a
   caller that forgets to override should not silently get a deleted engine.

**Done when**: `grep -r "MXCLI_ENGINE=legacy"` returns nothing outside history, the full gate passes,
and the doctype corpus runs green on the one remaining engine.

**Reversibility**: total, until Phase 3. `sdk/mpr` is untouched and every deleted file is one
`git revert` away.

### Phase 2 — Fix what goes stale the moment legacy leaves *(Effort: S, Risk: Low)*

These currently misdescribe the engine and would become actively false, so they belong with Phase 1
rather than after it:

- `mdl/backend/modelsdk`'s **package doc still says the engine is a read slice**: *"Phase 1 … is a
  READ slice … Write methods are NOT implemented yet — callers must not rely on them persisting;
  the CLI prints a read-only warning when this engine is selected."* All three clauses are false,
  and the read-only warning no longer exists in the code.
- The 2026-06-05 plan needs a status line pointing here; it reads as current and is not.
- `PROPOSAL_backend_strategy.md` is still `status: draft` from 2026-05-31 and describes a
  multi-backend future in which legacy is one of the backends.
- CLAUDE.md's `--engine` line, and the flag's help text.

### Phase 3 — Route the bypass sites through the backend abstraction *(Effort: M, Risk: Low–Med)*

**Revised 2026-09-15 after measuring it.** The first draft called this "decide the fate of
`sdk/mpr`", sized it L, and gated it on a product decision about breaking the public API. Measuring
the two consumers overturned all three.

#### The `unreachableUnimplemented` list is a census of who bypasses the abstraction

`mdl/backend/modelsdk/unimplemented_reachability_test.go` pins 16 `FullBackend` methods as having no
caller *through a backend value*. Read its reason column as a map rather than a list and it names
exactly the sites that hold a concrete `sdk/mpr` reader or writer instead:

| Bypass site | Methods it is the reason for |
|---|---|
| `api/` | `AddAttribute`, `UpdateAttribute`, `ExportJSON` |
| `mdl/backend/mcp` | `GetDomainModelByID`, `GetWorkflow`, `ListNavigationDocuments` |
| `cmd/mxcli` commands holding a reader | `FindCustomWidgetType`, `ListAllUnitIDs`, `ListRawUnits`, … |

These methods are on the interface *because* those callers exist, and they are unreachable
*because* those callers do not use a backend value. That circularity is the actual finding: the list
is not dead weight to delete, it is the work item. Close the bypasses and the methods either become
reachable and implemented, or become genuinely deletable.

#### `api/` — 3,287 lines, but a 17-method surface

`api/` **does not import `mdl/backend` at all** (measured: zero files). It is not "dependent on the
legacy backend"; it sidesteps the abstraction entirely, holding a concrete `*mpr.Writer` handed to
`api.New`. The whole dependency is five symbols — `mpr.NewWriter`, `Writer`, `Reader`, `Open`,
`GenerateID` — and seventeen methods called through them.

Of those seventeen, **fifteen are already implemented on the modelsdk backend** and all seventeen
are already declared on `FullBackend`:

    already on modelsdk   GetModuleByName, GetDomainModel, ListModules, UpdateEnumeration,
                          ListPages, ListMicroflows, DeleteAttribute, ListLayouts,
                          ListEnumerations, GetModule, CreatePage, CreateMicroflow,
                          CreateEnumeration, CreateEntity, CreateAssociation
    missing               AddAttribute, UpdateAttribute

and the two missing ones are missing *because `api/` is their only caller*. The template for
implementing them already exists: `ALTER ENTITY` does attribute mutation through the mutator.

So the port is: change `api.New` to take a `backend.FullBackend`, swap seventeen call sites, and
implement two methods. The public signatures of the builders (`CreateEntity(...).persistent()
.WithStringAttribute(...)`) need not change at all. **A breaking change to the published library is
not required**, so "does anything outside this repo depend on `api/`?" stops being a gate and
becomes a courtesy check on one parameter type (§5, decision D).

#### `mdl/backend/mcp` — stays, and is compatible

The MCP backend is in active use and its usage is expected to grow, so it is a fixed constraint
rather than something to migrate away. That constraint is satisfiable: its entire `sdk/mpr`
dependency is **one call**, `mpr.Open(path)`, for a deliberately read-only reader — writes already
go over MCP to Studio Pro, which is the whole point of the backend.

The naive port fails and the reason is worth recording: MCP calls **37 methods** on that reader and
`modelsdk/mpr.Reader` has **7** of them. But that is the wrong comparison — `modelsdk/mpr` is a
unit/raw reader, and the semantic decoding lives one layer up. Against the modelsdk **backend**,
**33 of the 37 are already implemented**; the gaps are `GetDomainModelByID`, `GetWorkflow`,
`ListNavigationDocuments` (the three the census above already attributes to MCP) and `Close`, which
is lifecycle rather than a read.

So MCP composes the codec backend for its reads instead of opening its own reader. Its 15,619 lines
are almost entirely the MCP protocol surface and are untouched by this.

#### Sequence

1. Implement the six methods the census attributes to `api/` and MCP.
2. Point MCP's reads at a composed codec backend; delete its `mpr.Open`.
3. Change `api.New` to accept `backend.FullBackend`; swap the seventeen call sites.
4. Re-run the reachability probe. What remains unreachable is the `cmd/mxcli` bson/diag commands,
   which hold a concrete reader **on purpose** — decide then whether they justify keeping a
   reader-only `sdk/mpr`, or whether they move too.

**Only after step 4 is `sdk/mpr`'s fate a question at all**, and by then it is a small one.

### Phase 4 — mongo-driver v1 → v2 *(superseded by §7.5)*

> **This section's premise did not survive Phase 3.** It opens "with [`sdk/mpr`] gone", and Phase 3
> closed by *keeping* it. Its sizing is also wrong on its own terms: the v1 files in `mdl/` do not
> "exist to bridge the two worlds", and do not shrink when `sdk/mpr` goes. See **§7.5**, which
> re-measures and splits this into two independent migrations. Kept unedited for the record.

Reachable once Phase 3's step 4 settles whether anything still needs `sdk/mpr`. With it gone the
remaining v1 files are the `mdl/backend/modelsdk` adapter's 44 and `mdl/executor`'s 18 — both exist
to bridge the two worlds and shrink as the semantic types move to v2, so the real size of this
phase is not knowable until Phase 3 lands. Not worth sequencing before then.

## 5. Decisions to confirm before Phase 1 starts

- **A. Does `--engine` survive as a no-op?** Keeping it as an accepted-and-ignored value is kinder
  to scripts in the wild; removing it is honest. The current code makes an unrecognised value
  **fatal** so typos are loud, which argues for an explicit, friendly error naming the removal
  rather than silence.
- **B. One release of deprecation, or delete now?** The earlier plan promised legacy would stay
  "reachable for one release as an escape hatch" after cutover. Whether that release has passed is
  a release-history question, not a code one.
- **C. Does `mxcli bson compare` still make sense?** It is a user-facing command whose purpose was
  comparing engine output. It may have a second life as a Studio-Pro-vs-mxcli diff, which is a
  different feature wearing the same name.
- **D. Does `api.New`'s signature change, or does it keep taking a concrete writer?** This is the
  only user-visible question in Phase 3 and it is much narrower than the first draft implied: the
  builder surface is unaffected either way (§Phase 3). Taking `backend.FullBackend` is the honest
  shape; keeping a concrete parameter and adapting inside preserves source compatibility for any
  out-of-tree caller.

**Fixed constraint, not a decision:** `mdl/backend/mcp` stays. It is in active use and its usage is
expected to grow. Phase 3 is written to satisfy that rather than to migrate away from it.

## 6. What could go wrong

- **The nightly is the only thing exercising legacy.** It is also the only thing that would notice
  if some path still reaches it. Delete the matrix leg and the backend in the **same** change, so
  there is no window where an untested engine is still shippable — the `#808` failure mode (an
  integration test that had only ever skipped) is exactly this shape.
- **A concrete-reader caller is not an engine caller.** Several `cmd/mxcli` commands hold a
  `*mpr.Reader` directly. They survive Phase 1 untouched and must not be swept into it; conflating
  them is what makes Phase 1 look big.
- **Deleting `mdl/enginecompare` deletes a measurement capability**, not just a test. If a future
  change needs "does the codec still agree with a known-good serializer", that ability leaves with
  Phase 1 — which is an argument for doing it *after* any outstanding codec parity work, not
  before.

## 7. First concrete steps

1. Answer decisions A and B (§5). They are one-line answers and they gate the diff's shape.
2. Phase 1 as a single PR, with the CI matrix change in it.
3. Phase 2 in the same PR — the stale docs are wrong the moment Phase 1 lands.
4. Phase 3 is now sequenced in place (it was going to be a proposal until measuring it shrank it).
   Its first step — implementing the six methods the census attributes to `api/` and MCP — is
   independent of Phases 1–2 and could be done first or in parallel.

---

## 7. What landed

Recorded as it happened, because two of the estimates in this plan turned out wrong in ways worth
keeping.

### Phase 3, out of order and smaller than sized (2026-09-15)

Phase 3 was written as an `L` gated on "breaking a public API". Both were wrong, and the
correction is the plan's main lesson:

- **`api/` was one signature, not a rewrite.** It held a concrete `*mpr.Writer` and imported
  `mdl/backend` zero times. Taking a `backend.FullBackend` instead (plus an `api.Open` that owns
  its connection) routed the whole package through the abstraction, which made `AddAttribute` and
  `UpdateAttribute` *reachable* and therefore worth implementing — the two methods the codec engine
  had left to the stub because nothing called them through a backend value.
- **`api/`'s integration suite had only ever skipped.** All ten tests pointed at a path that does
  not exist in this repo, so `go test ./api/` was green and verified nothing — the #808 shape
  again. Repointed at a committed fixture, with a missing fixture now fatal instead of a skip.
- **The MCP backend composes this backend now.** It kept a concrete `*mpr.Reader` for three reads
  the codec engine did not offer (`GetDomainModelByID`, `GetWorkflow`, `ListNavigationDocuments`);
  implementing those let it hold a `backend.FullBackend` instead. It needed a new
  `ConnectReadOnly`, because `Connect` opens read-write and MCP must not lock the file Studio Pro
  owns.
- **`Connect` had no test** — 190 in that package, not one called it — so the swap would have
  landed unverified with the suite green. It has one now, with the read-only constraint proved by
  a revert control.

**The organising insight, which is the reusable part:** `unreachableUnimplemented` in
`mdl/backend/modelsdk/unimplemented_reachability_test.go` is a **census of who bypasses the
abstraction**, not dead interface surface. A method is on it *because* some caller reaches it while
holding a concrete reader or writer, and unreachable *because* that caller does not use a backend
value. So the list shrinks by closing a bypass, never by deleting methods.

> **Corrected 2026-09-15 — see [§7.3](#phase-3-step-4-the-census-has-three-causes-not-one).** That
> last sentence is true of a bypass and false of the other two things on the list. Five of the
> eleven remaining entries had no caller anywhere, or callers using a different signature, and were
> deleted rather than ported. It went 11 entries
lighter over these two steps (5 struck off), and what remains names exactly the work left in
Phase 3: the `cmd/mxcli` bson/diag/extract-templates commands.

### Phases 1 and 2 (2026-09-15)

Went as written, at the sizes given. Three things the plan did not anticipate:

- **`--engine` is a warning-only no-op, not a removal.** Deleting the flag would fail a script
  pinning `legacy` at argument parsing with "unknown flag", which says nothing about what changed.
  It now warns once and proceeds. `bson compare` was dropped outright, per the same decision round.
- **`errUnimplemented` still told users to rerun on the deleted engine.** A runtime message
  naming a fallback that no longer exists is worse than no fallback; it now asks for a bug report,
  which is what reaching it actually means.
- **`setupTestEnv` defaulted to the legacy engine**, so most of `mdl/executor`'s integration tests
  were exercising the retired engine rather than the one users get. Deleting legacy moved them onto
  the codec engine — coverage that was always intended and had silently not been happening.
- **One integration test had to go, and it named itself.** `TestCustomHandlerLegacyRefuses`
  asserted that the legacy backend *refuses* a custom import-mapping handler rather than writing
  `CustomHandlerCall` as nil and dropping the microflow silently. With no legacy engine there is no
  refusal to assert, and the construct's positive coverage on the codec engine is intact, so the
  test was deleted rather than repointed. It was the only failure across the whole executor
  integration suite after the switch — 562s green, down from 642s now that the doctype gate runs
  once instead of twice.
- **The cross-engine tests split two ways.** `TestODataService_EngineWriteParity` compared two
  writers; with one engine the comparison is vacuous, so the differential half was dropped and the
  property it was a means to (a published service keeps its role grants, invisible to `mx check`)
  kept as a single-engine test. The doctype gate's engine matrix was *not* deleted: with one entry
  it still turns a stale `MXCLI_TEST_ENGINES=legacy` into a loud failure instead of a gate that
  runs nothing and reports success.

### Phase 3 step 4 — the census has three causes, not one (2026-09-15)

Re-running the probe over what was left produced a correction to the paragraph above, which is the
part of this plan most likely to be reused and was wrong.

`scripts/backend-reachability.sh` reports **DEAD** for "nothing calls this through a backend
value". That single verdict covers three situations that want opposite fixes:

| cause | what it means | fix |
|---|---|---|
| **bypass** | a caller wants it but holds a concrete reader/writer | port the caller |
| **orphan** | nothing anywhere calls it, under any type | delete the method |
| **duplicate** | callers exist, but through a narrower package-local interface with a *different signature* | delete the method |

The probe cannot separate them — that is what a grep for callers under **any** type is for. The
duplicate case is the one that misleads: the name has plenty of call sites, so it reads as a
bypass until you compare signatures.

Measured, all six DEAD: the whole **`WidgetSerializationBackend`** interface (`SerializeWidget`,
`SerializeClientAction`, `SerializeDataSource`, `SerializeWorkflowActivity`), plus `GetUnitTypes`
and `UpdateLayout`.

- `SerializeWidget` / `SerializeDataSource` are superseded by `WidgetBuilderBackend`'s
  `SerializeWidgetToOpaque` / `SerializeDataSourceToOpaque` — whose own doc comment says *"This
  replaces the direct mpr.SerializeWidget call"*, so the supersession was known and the old pair
  simply never removed.
- `SerializeClientAction` and `SerializeWorkflowActivity` are reached through `pagemutator` and
  `wfmutator`'s own `deps` interfaces, which declare them returning `bson.D` rather than
  `(any, error)`. The `*Backend` copy of `SerializeWorkflowActivity` even carried a comment claiming
  the ALTER WORKFLOW paths used it; they use `codecWorkflowDeps`.
- `GetUnitTypes` exists only on `modelsdk/mpr.Reader`; `UpdateLayout` was superseded by the page
  mutator when ALTER LAYOUT landed.

Deleted from the interface, both generated stub files regenerated (276 → 270 methods), mock stubs
dropped. **Census 11 → 6**, and all six that remain are genuine bypasses — the `cmd/mxcli`
bson/diag/extract-templates commands and `examples/read_project`, which hold a concrete reader
deliberately. Whether those should be ported at all is the open question for the rest of Phase 3;
they are debugging tools whose whole job is raw access, so "leave them" is a defensible answer that
the earlier framing did not allow for.

### Phase 3 closed — the last six bypasses are accepted (2026-09-15)

After the orphans and duplicates were deleted, the census held six entries and they were all the
same kind of caller: `cmd/mxcli`'s `bson dump` / `bson discover` / `diag` / `extract-templates` and
`examples/read_project`. Every one is a **raw-unit debugging or export command**, holding a concrete
`sdk/mpr` reader because raw access is the thing it exists to provide.

> **Corrected 2026-09-15 (§7.6).** "All the bypasses are raw tools" was wrong, and the census is
> why: it lists only methods with **no implementation**, so a caller holding a concrete reader while
> calling only *implemented* methods never appears in it. `cmd/mxcli/project_tree.go` is exactly
> that — 36 semantic reads, every one on `FullBackend` — and it is a bypass the census could not
> see. **The complete list of bypasses is the `sdk/mpr` importer list, not the census.**

Porting them was considered and **declined**. The backend interface speaks the semantic model by
[ADR-0005](../13-decisions/0005-semantic-model-interface-currency.md); routing a BSON dumper through
it would either widen that interface with raw accessors — undoing the decision — or make the tools
worse at their only job. The earlier framing ("the list shrinks by closing a bypass") did not offer
this option, which is a second way that framing was too narrow: some bypasses are correct.

`unreachableUnimplemented` therefore becomes a **standing record rather than a to-do list**, and its
header says so. It keeps its value as a tripwire: a *new* entry still means either a new bypass
appeared or a method was added that nothing calls, and the fix depends on which — establish the
cause rather than adding a row to silence the failure.

**`sdk/mpr` does not go away with this — and it is imported far more widely than the census
suggests.** The census tracks `FullBackend` *methods* with no caller through a backend value; it
says nothing about who imports the package. Measured: **28 non-test files** import `sdk/mpr`,
including `cmd/mxcli/docker/` (7 files in the run/build pipeline), two `mdl/executor` validators,
and — most consequentially — **`modelsdk.go`, the published library's root API**, whose `Reader` and
`Writer` are type *aliases* to `sdk/mpr`'s. An earlier draft of this section said "those six
commands still import it", which was wrong by a factor of four and pointed at the wrong files.

What Phase 3 delivered is narrower and still worth having: no *engine* path reaches `sdk/mpr`. The
executor's write paths, the backends and `api/` are clean. Removing the serializer is Phase 4's
job, and §7.5 measures what that actually takes.

### Phase 4 re-measured — it is two independent migrations, not one (2026-09-15)

§4's Phase 4 assumed one job, gated on `sdk/mpr` being gone. Measuring it after Phase 3 closed
shows **two migrations that do not gate each other**, and the one worth doing first is not the
driver migration at all.

**Where v1 actually lives** (249 files import v1, 174 import v2; only **6** import both, all of them
in `mdl/backend/modelsdk`, and the crossing is safe because the interchange is *bytes* — v1
`Marshal` → `v2.Raw`, and BSON bytes carry no driver version):

| tree | v1 | v2 | what it is |
|---|---|---|---|
| `modelsdk/` | 0 | 116 | the codec — already pure v2 |
| `sdk/` | 117 | 0 | the legacy serializer |
| `mdl/` | 107 | 58 | mixed; the mutator layer is v1 |
| `cmd/`, `examples/`, `scripts/`, `model/` | 25 | 0 | callers |

#### 4a — port the 28 `sdk/mpr` callers (the one that pays)

**23 of the 28 use only `mpr.Open`.** The dependency is overwhelmingly on one read-only
constructor, not on the serializer's 41k lines. The rest use `mpr.NewWriter` (6) or a small helper
(`GenerateID`, `BlobToUUID`, `MPRVersionV`, `ParseMicroflowBSON`).

The naive objection is that `sdk/mpr.Reader` has **140 methods** and `modelsdk/mpr.Reader` has
**47**, so the port looks impossible. That is the same wrong comparison the MCP port already
disproved: `modelsdk/mpr` is a *raw/unit* reader and the semantic decoding is a layer up, on the
**codec backend**. Every method these callers invoke — `ListModules`, `GetDomainModel`,
`ListMicroflows`, `GetProjectSecurity` … — is a `FullBackend` method.

So 4a is **the MCP port repeated**: swap `mpr.Open(path)` for `modelsdkbackend.New()` +
`ConnectReadOnly(path)`. That move is already written, already tested, and already has a read-only
guard with a revert control.

**Its one real gate is `modelsdk.go`.** The published library's `Reader` and `Writer` are type
aliases to `sdk/mpr`'s, so the root API *is* the legacy serializer. Changing it is a breaking change
of a different order from `api.New`'s — that one had zero in-repo callers and a fluent surface that
did not move; this one is the documented entry point in README and CLAUDE.md. **Decide this before
starting 4a**, not during.

#### 4b — convert `mdl/`'s mutator layer to v2

`mdl/backend/modelsdk` has **19 non-test v1 files**, using `bson.D` (70), `bson.A` (25),
`bson.M` (14), `Marshal`/`Unmarshal` (29). These are not a bridge to `sdk/mpr` and **do not shrink
when it goes** — §4's sizing claim is wrong here. They are v1 because the *mutator layer's*
currency is v1 `bson.D`: `pagemutator`, `wfmutator` and `widgetobj` all declare their `deps`
interfaces in those terms, and the codec backend implements them.

That makes 4b independent of 4a and of the serializer entirely. It is also the half with **no
user-visible benefit until both are done**, since v1 leaves `go.mod` only when the last importer
does.

#### Recommended order

**4a, then the `modelsdk.go` decision, then 4c (delete `sdk/mpr`), then 4b.** 4a is a proven move
that deletes 41k lines; 4b is internal churn whose payoff is gated on 4a finishing anyway. Doing 4b
first would convert a mutator layer that 4c might reshape.

### Phase 4a, first slice — `cmd/mxcli`'s readers (2026-09-15)

Five files off `sdk/mpr`, and the slice split into two ports rather than one, because the Phase 3
decision ("raw tools keep raw access") does not mean "raw tools keep `sdk/mpr`":

- **Raw tools → `modelsdk/mpr`**, the **v2 raw reader**. `bson dump`, `bson discover` and `diag`
  moved by import swap alone: `ListRawUnits`, `GetRawUnitByName`, `GetRawMicroflowByName`,
  `ListAllUnitIDs` and `ContentsDir` have **identical signatures** on both readers, and
  `RawUnitInfo` is field-for-field identical (`modelsdk/mpr` aliases `types.RawUnitInfo`;
  `sdk/mpr` declares a duplicate struct, which its own rule in CLAUDE.md forbids). This honours
  Phase 3 *and* moves those files to v2 — 4a and 4b at once for them.
- **Semantic bypass → the backend.** `project_tree.go` and `debug_resolve.go`. The first calls 36
  semantic reads; the second needs `ParseMicroflowBSON`, which is a backend method, so straddling
  two readers would have been the alternative.

**A regression the port introduced, caught only by a baseline diff.** `project-tree` silently lost
`System.VerifyPassword` — 131 bytes out of 78KB of JSON. The System module's Java actions are
platform built-ins with **no stored unit**; `sdk/mpr` synthesized them and the codec backend did
not. The definitions now live in `modelsdk/meta` beside the virtual System module's entities, both
codec listings append them, and `sdk/mpr` delegates instead of holding a second copy.

The technique is the transferable part: **build a binary from the pre-port commit first**
(`git stash -u; make build; cp bin/mxcli /tmp/before`) and require byte-identical output from every
command whose reader changed. No test caught this and none would have. Generalising: a
**synthesized** element is what a reader swap loses, because it lives in one reader's code rather
than in the data — grep the old reader for `virtual`, `not stored in`, and `Build*` helpers before
trusting any port.

**Importers: 27 → 22.**

### Phase 4a, second slice — `cmd/mxcli/docker` (2026-09-15)

Seven files plus five test files. **Importers 22 → 15.** Everything the package used —
`GetProjectSecurity`, `GetProjectSettings`, `ListModuleSettings`, `ListUnits`, `GetRawUnitBytes`,
`ProjectVersion`, `Version`, `AddDemoUser`, `SetProjectDemoUsersEnabled`, `UpdateRawUnit` — was
already on `FullBackend`; only `Close` (→ `Disconnect`) and `Reader()` (unnecessary, the backend is
both) had to change. Two package-local helpers, `openReadOnly` and `openForWriting`, are now the
only way this package opens a project.

One semantic check before porting the writers, because getting it wrong would be silent: both
`sdk/mpr.Writer.UpdateRawUnit` and `modelsdk/mpr.Writer.UpdateRawUnit` call `updateUnit` with **no
options**, i.e. the ordinary translation-carrying path, so the harvest's behaviour is unchanged.
`UpdateRawUnitOwningTranslations` is the other case and neither uses it.

**The verification lesson, which is the opposite of the last slice's.** A baseline diff said
`docker check` left all 421 files byte-identical — and proved nothing, because the run had not
written anything: the widget harvest is a no-op on a clean fixture. *A byte-identical baseline diff
is strong evidence for a read port and near-worthless for a write port, because the natural control
(nothing changed) is also what a no-op produces.*

`go test -coverprofile` + `go tool cover -func`, grepped for the ported functions, answers "did my
port's code even run" in one command where a passing suite does not. It separated `applyHarvest`
(**76.9%**, genuinely exercised including its `UpdateRawUnit`) from `ensureDemoUsers` (**0.0%**) in
the same package — so the gap was specific, not a general absence of tests. `ensureDemoUsers` now
has tests and sits at **76.5%**.

Two traps inside that fix, both already familiar: the shared fixture **already has two demo users**,
so a create-path test that skipped when any existed would never run (#808's shape — set the
precondition up, don't skip past it); and the read-back must use a **fresh connection**, since
asserting on the value the writer still holds passes against a write that never reached disk.

### Phase 4a, third slice — `mdl/executor` (2026-09-15)

Two validators, and the only slice so far that fixes a **stated rule violation** rather than
tidying: CLAUDE.md's backend-abstraction checklist says *"executor files must not call `sdk/mpr`
writer/parser types directly"*, and these did. **Importers 15 → 13.**

They open their **own** short-lived read-only connection rather than using `ctx.Backend`, because
`ValidateProgram` takes a project **path**, not a backend — `mxcli check --references` validates a
script against a project it never connects an executor to. Threading a backend down would change a
public signature and every caller for no gain. `openProjectForValidation` is now the package's one
way to do it.

**The verification problem here is a third distinct shape**, after the read port (§7.5) and the
write port (§7.6). Both validators **fail open**: an unreadable project returns nil and silences
the rule, which is right — a check should not fail on something it could not inspect — and it makes
a broken reader **silent**. The rule simply stops firing, and that is indistinguishable from a
project the rule does not apply to.

Coverage confirmed the risk was real: `offlineProfilesIn`, `projectEntityFacts` and
`openProjectForValidation` were all at **0.0%**, because the existing tests exercised only the pure
helpers or passed an empty path. Now 78%, 72% and 100%.

> **For a fail-open path, the test must assert the rule FIRES** on a project that should trigger it.
> Asserting it stays quiet proves nothing, because quiet is also the failure mode.

Two setup details decided whether that test was real, and the first attempt got both wrong: the
fixture ships only an **online** profile, so the offline rule is inert on it either way and the test
has to seed one — and Mendix **fixes the legal profile names** (`Responsive`/`Phone`/`Tablet` plus
the `*Offline` variants), so an invented name is refused by the executor. The stock-fixture control
runs first, so a reader that invented a profile is caught before the positive assertion.

### Phase 4a, fourth slice — the rest of `cmd/mxcli` (2026-09-15)

Five files — `cmd_new`, `setup` (two readers, not one), `serve`, `sync_java_deps`,
`cmd_check_post_migration` — plus one refused. **Importers 13 → 8.** Every method they call is
on `FullBackend`; `catalog.CatalogReader` already documents itself as satisfied by it, and
`executor.NewContainerHierarchyFromBackend` already existed. `openProjectReadOnly` is now this
package's one way in, and the two inline `ConnectReadOnly` sites from the first slice use it too.

**`cmd_extract_templates.go` stays on `sdk/mpr`, deliberately.** It calls
`FindCustomWidgetType`, which is **unimplemented on the codec backend** — measured at runtime, it
returns *"not implemented on the model engine. This should be unreachable."* Porting it would have
compiled and failed for anyone extracting a widget template.

> The type error was the lucky part. `RawType`/`RawObject` are `bson.D` on `sdk/mpr` and `any` on
> `types.RawCustomWidgetType`, so the port would not build — and **the one-line cast that silences
> that is the only thing standing between this and a runtime break.** When a port hits a type
> mismatch at a backend boundary, check whether the method is implemented *before* reconciling the
> types.

Note the direction: the unimplemented method's error says *"should be unreachable"*, and porting a
caller to the backend is exactly what **makes** it reachable. §7.5's census blind spot — callers
holding a concrete reader are invisible — cuts both ways.

**A measurement trap worth naming**, because it looked like a regression and was not: a baseline
diff of `check --post-migration` showed 50 lines disappearing. The **first** run built and cached a
catalog inside the project, and the second reused it. Each binary needs its own fresh copy of the
fixture — the same discipline a write port needs, because a command that caches into the project
directory makes consecutive runs non-independent even when nothing is being written on purpose.

With that fixed the run is **63 identical lines**, catalog build and legacy-widget scan included.
But "No legacy native widgets found" is §7.7's fail-open shape again — a reader handing back zero
pages prints it too — so `openProjectReadOnly` has tests asserting the reads the commands depend on
actually return data.

### Phase 4a, fifth slice — `FindCustomWidgetType`, and `cmd/mxcli` is clear (2026-09-15)

The refusal from the fourth slice, resolved. **Importers 8 → 7**, and **nothing that ships in the
binary imports `sdk/mpr` any more**.

**The implementation was never missing.** `modelsdk/mpr.Reader` has had `FindCustomWidgetType`,
`FindAllCustomWidgetTypes` and the `collectCustomWidgets` walker all along — and its version
populates `UnitName`/`WidgetName`, which the one I started writing from scratch would have left
empty. Only the *wiring* onto `Backend` was absent, which is precisely what the old error meant by
*"this should be unreachable"*.

> **Grep for an existing implementation before writing one.** A method listed in
> `unimplemented_gen.go` says nothing about whether the logic exists a layer down.

**A straight delegation then extracted 0 of 6 templates**, reporting for each widget:

```
[SKIP] Combo box: widget type is bson.D, want bson.D
```

`modelsdk/mpr` builds `RawType`/`RawObject` with the **v2** BSON driver; `sdk/mpr` and every caller
use **v1**. They are unrelated Go types that print under the same name, and
`types.RawCustomWidgetType` declares the fields `any` — so neither the compiler nor the error text
can tell them apart.

> An `any` field crossing an engine boundary can carry the right type **name** and the wrong
> **package**. When an assertion fails with identical names on both sides of "want", the question is
> which import path each came from. A cast written to silence it panics at runtime.

Fixed by converting at the boundary with the package's existing `v2ToV1BSON`, as
`widget_pluggable_write.go` already does for writes. Verified: all six templates extract
**byte-for-byte identically** to the pre-change binary, `datagrid.json` at 1.2 MB included.

Two maintenance notes. `unimplemented_gen.go` **still emits the stub** after a method is implemented
— the generator writes a complete fallback set and `Backend`'s own method shadows it — so the thing
to update is `unreachableUnimplemented` in `unimplemented_reachability_test.go`, which fails loudly
when a listed method becomes implemented. It did, which is how the stale entry was caught.

### Phase 4a, sixth slice — the last seven, and the count reaches **zero** (2026-09-15)

`examples/` (5) and `scripts/mprsnapshot` (2). **Importers 7 → 0.**

Three shapes, not one. Four examples held the **writer** (`mpr.NewWriter` → `Backend.Connect`,
`Close` → `Disconnect`, and `writer.Reader()` dropped because the backend is both halves, with
`CreateEntity`/`CreateAssociation`/`DeleteEntity`/`DeleteAssociation`/`CreatePage` identical in
signature on both). Two held a **reader**. The rest were two utilities, `GenerateID` and
`BlobToUUID` — and `sdk/mpr`'s copies are already one-line delegations to `mdl/types`, so pointing
the call sites at `types` is **provably** the same function rather than a same-named one, which is
the distinction §7.9's v1/v2 BSON trap turned on.

Verified per shape. `mprsnapshot` is a canonicalisation tool, so its output is the evidence:
**identical in all four modes** — default (2,870 lines), `-refs` (12,707), `-canon` (374) and
`-all` (43,979). The write examples cannot be diffed that way, so `add_entities` was run under both
binaries and the resulting projects compared with `mprsnapshot`: every element path and type
matches, and once UUIDs are normalised the only remaining difference is the domain model's content
**hash**, which digests bytes that embed those UUIDs. Fresh identities on new elements are required
(§"A GUID Is the Database's Identity"), so that is the correct result, not a discrepancy.

**The count is now guarded.** `TestNothingImportsTheLegacyEngine` (`mdl/backend/`) parses every
`.go` file's imports and fails naming any file that imports `sdk/mpr`. Without it the invariant
lived in this document and a habit, and one import would restore exactly the blind spot Phase 4a
existed to close — a caller holding a concrete reader is invisible to the census.

> A zero-count invariant needs **two positive controls** or it passes vacuously forever, because
> every way of breaking it is silent: a wrong root, an over-broad skip rule and an import-parsing
> mistake all report "0 importers". So assert that a plausible number of files was scanned (2,551)
> **and** that the detector can see imports at all, by counting one the repo definitely has
> (`mdl/backend`, 120 files). Only then does 0 mean zero — the same reasoning as
> `scripts/check-tunnel-deps.sh`, which proves chisel *is* in the linux graph before proving it is
> absent from the others.

**Phase 4a is complete.** `sdk/mpr` has no importers outside itself; deleting it is now a
scheduling decision rather than a risk assessment.
