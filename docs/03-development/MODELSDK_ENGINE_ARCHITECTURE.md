# modelsdk engine architecture & patterns

How the `modelsdk` write/read engine is structured, and the canonical patterns to
follow when extending it. Read this before adding a document type or a microflow
activity group. Decisions behind it: [ADR-0002](../13-decisions/0002-backend-abstraction.md)
(backend abstraction), [ADR-0004](../13-decisions/0004-full-codec-engine.md) (full codec),
[ADR-0005](../13-decisions/0005-semantic-model-interface-currency.md) (model is the interface currency).

## Layers

```
front-ends:  MDL executor ─┐
             fluent api/   ─┼─► SEMANTIC MODEL ──► backend interface (model-typed) ──┐
             (importers…)  ─┘   (sdk/microflows,                                      │
                                 sdk/domainmodel, model)                  ┌───────────┼────────────┐
                                                                      MPR backend  MCP/PED      (future fmt)
                                                                      gen+codec    JSON ops      adapter
                                                                          │
                                                                        BSON
```

- **The backend interface speaks the semantic model**, never gen/AST/BSON types. gen+codec
  (`modelsdk/gen`, `modelsdk/codec`) are the **MPR backend's internal storage adapter** — one of
  several (MPR, MCP/PED, a future format Mendix is considering). Do not leak gen into `mdl/backend/*.go`.
- The `modelsdk` MPR backend lives in `mdl/backend/modelsdk/`; selected via `MXCLI_ENGINE=modelsdk`
  (default is `legacy`). Parity against `legacy` is the correctness gate during migration.

## Write patterns

**Unit-write pattern (top-level documents: microflows, enums, constants, pages, …).**
A document is its own unit. `CreateX` → build gen → `assignXIDs` → `codec.Encoder.Encode` →
`writer.InsertUnit(unitID, containerID, "Documents", "<$Type>", contents)`. `DeleteX` →
`writer.DeleteUnit(id)`. `UpdateX` (CREATE OR REPLACE) → rebuild → `writer.UpdateRawUnit(id, contents)`.

**Child-element writes (entities/associations live inside the domain-model unit).**
Load the DM gen unit (`loadDomainModelGen`), mutate it (`AddEntities`, `RemoveEntities`, …), re-encode
the whole unit, `UpdateRawUnit`. Clean siblings pass through their original bytes (codec passthrough).

**CREATE = model→gen.** Write an `xToGen(model) *genX` converter mirroring the legacy serializer's
field set. Use engalar/dev's `flowbuilder_*_gen.go` / `*_gen.go` as the **reference** for which gen
setters to call (it targets the gen API directly) — but never adopt engalar's gen-typed interface
(see Harvest rule). Then `assignXIDs` walks the tree assigning fresh IDs to elements lacking one
(`assignID` is no-op on non-empty IDs; carry domainmodel IDs where references must resolve).

**Reads = gen→model.** Write `xFromGen(*genX) *model.X`. Make it **lossless** for any field that an
ALTER read-modify-write path round-trips (else ALTER silently drops data — see the entity history).

**ALTER.** Two sanctioned approaches (ADR-0005):
- **Mutator (preferred for fidelity-sensitive / large docs):** `OpenXForMutation(id) (XMutator, error)`
  loads the decoded gen unit and exposes semantic mutation ops; untouched sub-elements pass through
  byte-for-byte → clean git diffs. Template: `PageMutator` / `WorkflowMutator` in `mdl/backend/mutation.go`.
  **Microflow ALTER, when added, MUST use this pattern** (don't repeat entity's model round-trip).
- **Model round-trip (acceptable for small/simple docs):** `GetX` → mutate model → `UpdateX`. Used today
  for entities/enums. Correct but lossy-by-construction (needs a lossless read adapter) and churns IDs/
  order, so reserve it for docs where diff-fidelity doesn't matter.

## Codec mechanisms a converter author needs

- **`codec.RegisterTypeDefaults($Type, TypeDefaults{…})`** — fields Studio Pro emits on create that the
  gen constructor doesn't set. Knobs: `EmitGUID` (GUID = element $ID, binary), `MandatoryLists` (empty
  PartList emitted as `[marker]`), `NullFields` (emit BSON null), `ZeroGUIDFields` (all-zero GUID binary,
  e.g. an unset pointer), `FreshGUIDFields` (fresh random GUID binary, e.g. `StableId`).
- **`codec.RegisterListMarker(childType, n)`** — typed-array version marker. Default is **3**; some lists
  use **2** (index `IndexedAttribute`, sequence-flow `CaseValues`, change-action `ChangeActionItem`) or
  **1** (by-name lists like `AllowedModuleRoles`/`UrlSearchParameters`). Verify against real BSON.
- **Storage-name overrides.** The gen sometimes wires the SDK name, not the BSON storage name. Patch the
  gen `init` + `InitFromRaw` (both encode and decode) with a `STORAGE-NAME OVERRIDE` comment; the permanent
  fix is `internal/codegen/supplements.json` + regenerate. Known cases: `ErrorMessage`→`Message`,
  EventHandler `Event`→`Type` / `PassEventObject`→`SendInputParameter` / list `EventHandlers`→`Events`,
  microflow `ConcurrencyErrorMessage`→`ConcurrenyErrorMessage` (Mendix typo), `StableId` is binary-not-string
  (handled via `FreshGUIDFields`). gen MicroflowParameter was also missing fields (completed by hand).

## A listed element may be shared: read it, or change it and write it

`mprread`'s listings (`ListUnitsWithContainer` and the lookups beside it — what
the backend's `listUnits` calls) keep the decoded form of every unit they have
decoded twice, and hand the **same** element to every caller until the unit's
stored bytes change (`modelsdk/mprread/decode_cache.go`, ako/mxcli#2160;
the decision and its alternatives are
[ADR-0015](../13-decisions/0015-content-keyed-unit-caches.md)). A
script used to cost statements × project, because each statement listed whole
document types and each listing decoded every unit again.

What that means for a converter or a write path:

- **Reading is free and safe**, from any goroutine: every lazily decoded value
  is loaded before an element is shared, so a read writes nothing.
- **A setter on a listed element is seen by no other caller.** The element
  becomes dirty, and a dirty entry is decoded afresh for the next caller. Set
  it, encode it, write it — as `UpdateOqlQueriesForMovedEntity` does.
- **Do not change a listed element by any route that does not mark it dirty**
  (a field assignment, a slice edited in place). Nothing detects that, and the
  next caller would read the change.
- **Never hold a listed element across your own write and compare it with a
  fresh listing** expecting two objects: when nothing changed they are the
  same one.

The cache is keyed by a unit's stored bytes, so no write path invalidates it
and none can forget to. `MXCLI_DECODE_CACHE=off` turns it off for a process, to
rule it in or out when a result looks wrong.

## Harvest rule (engalar/dev)

engalar/dev has a complete, tested gen-native write suite, but built **AST-direct with a gen-typed
interface**. Harvest its `flowbuilder_*_gen.go` only as a **reference for `model→gen` construction**.
Never adopt its gen-typed backend interface or AST-direct executor — that would weld the contract to BSON
and break the MCP backend + the future format (ADR-0005). See `docs/11-proposals/ASSESSMENT_harvest_engalar_writes.md`.

## Recipe: add a document type or activity group

1. **Get a Studio Pro-authored reference document** of the type — from a Marketplace module that uses
   it, or by asking for one to be created in Studio Pro. This is the field set + ordering spec. There is
   no second engine to copy from any more, and there is nothing else that will tell you the truth.
2. **Read its BSON**: `mxcli bson dump -p app.mpr --type <type> --object "Mod.Name"`, or the MCP/PED
   probe (`cmd/mcpprobe`) against a live Studio Pro.
3. **Write `xToGen`** (+ `xFromGen` if reads/ALTER need it), registering any TypeDefaults / list markers.
   An owner/key marker (`codec.RegisterPropertyListMarker`) is applied by every write to the stored lists
   it does not rebuild as well (`canon.NormalizeListMarkers`), so register only a measured one:
   `TestIssue1640_RegisteredListMarkersAreWhatStudioProStores` holds each against the fixtures.
4. **`assignXIDs`** walks new sub-elements.
5. **Pin the document against the reference** — re-serialize the reference element by element and assert
   the keys, markers and value types match. `mdl/scheduledevents` and `mdl/regularexpressions` are the
   worked examples; both found gen wrong about a property that way.
6. **Iterate until the diff is empty**, then build it (`mxcli docker check`) and, where the construct
   renders or runs, verify it there too — `mx check` tolerates unknown properties, so a clean build is
   not evidence the document is right.
7. **gofmt any hand-edited gen file** or `TestGeneratedCodeIsFormatted` fails.

## Verification truth

**A Studio Pro-authored document is the arbiter.** There used to be a second engine to diff against;
`sdk/mpr` was deleted ([ADR-0004](../13-decisions/0004-full-codec-engine.md)) and it had been wrong often
enough that it was never the real baseline anyway (stale index `SortOrder` serializer).

Where `modelsdk/gen` and `generated/metamodel` disagree about a property key, **`generated/metamodel` is
the arbiter** — it is built from reflection data carrying storage names, while gen's generator reads the
TypeScript SDK, which has none, and patches them back by hand. The caveat is that it is a snapshot of
11.6.0, so it says nothing about properties introduced later; for those, get a real document. See
CLAUDE.md, "`modelsdk/gen` Binds Some Properties Under the Wrong BSON Key".

Capture, don't guess — and note what a green build does *not* tell you: mxbuild accepts properties the
project's metamodel does not declare, while Studio Pro throws `InvalidOperationException` at
`MprProperty.cs`. Measured on 10.24.25 with two 11.5-only keys present: 0 errors.

## `modelsdk/gen` Binds Some Properties Under the Wrong BSON Key

The storage-name table above is about `$Type`. The **same split exists per
property**, and `modelsdk/gen` gets it wrong in **102 properties across 65
types** — the ledger is `modelsdk/gen/keyaudit_test.go`. Mendix's reflection
data carries two names per property — an SDK `Name` and a BSON `StorageName` —
and the in-repo generator (`cmd/codegen` → `generated/metamodel`) keeps them
apart, tag from storage name:

```go
// generated/metamodel/types.go — correct
RegularExpression model.QualifiedName `json:"regExIdentifier,omitempty"`
//   ^ SDK name                                ^ storage name
```

The generator behind `modelsdk/gen` reads a **different input** — the TypeScript
SDK's compiled JS, which does not contain storage names at all (measured:
`regExIdentifier` occurs 0 times in `mendixmodelsdk` 4.114.0) — and patches them
back via a hand-maintained `PropertyKeyOverrides` table.

**`generated/metamodel` is therefore the arbiter when the two disagree**, with
one caveat: it is a **snapshot of 11.6.0** (see its header), so it is sound for
the properties it contains but says nothing about ones introduced later — for
those, get a real document. It has been right in every case checked that way
(`RegularExpression.Expression`, `RegExRuleInfo.RegExIdentifier`, `Attribute.GUID`).
`TestGenPropertyKeysAgainstMetamodel` fails when a NEW mismatch appears (a
re-vendored gen that dropped an override) or when a listed one is fixed without
being struck off. Why the generator is not simply brought in-tree, and what it
would take: [PROPOSAL_codegen_ownership.md](docs/11-proposals/PROPOSAL_codegen_ownership.md).

`cmd/modelsdk-codegen` and `internal/codegen/supplements.json` — named in every
gen file's `DO NOT EDIT` header — have **never existed in this repo**
(`git log --all` is empty for both), and `/reference/` is gitignored, so the
generator's input is absent too. gen is vendored output that cannot be
regenerated here; see `docs/plans/2026-06-05-adopt-modelsdk-engine.md` §4, where
"vendor engalar codegen" is still an open Phase-0 item.

So the fix for a wrong key is a **hand-applied override in the `init<Type>`
function**, commented in the house style (grep `STORAGE-NAME OVERRIDE` for the
four precedents). Two rules:

1. **Patch both sides.** The encode key (`init<Type>`) and the decode key
   (`InitFromRaw`) are separate literals. Patching one gives a document that
   writes one key and reads another — which the entity-rewrite guard then
   refuses, so the symptom is a puzzling refusal rather than a wrong file.
2. **`gofmt` the file**, or `TestGeneratedCodeIsFormatted` fails.

Not every wrong key is worth patching — leave the ones nothing writes, and note
why. `mx check` is a weak signal here either way: it caught the RegEx one
(CE0135) but tolerates unknown properties in general, and Studio Pro is stricter
than mxbuild.

## Overlay Writes: Never Invent a Key, Branch on `$Type`

When a write overlays fields onto preserved BSON (`mdl/settingsoverlay`, and any
future storage that follows ADR-0005 guard-don't-drop), two rules are load-bearing.
Breaking either produces a document `mx check` accepts and **Studio Pro cannot
open**: it resolves every stored property against the type's property list and
throws `System.InvalidOperationException: Sequence contains no matching element`
at `MprProperty.cs`. mxbuild's deserializer tolerates unknown properties, so the
build is not a safety net here.

1. **Write only keys the document already carries.** Property names are
   version-specific — Mendix renamed `JavaVersion` (`"Java21"`) to
   `JavaMajorVersion` (`"21"`) and `Tracing` to `OpenTelemetry` between 11.6 and
   11.12. Read the key off the stored document and write back to that same key;
   when neither is present, write neither (an absent optional property is filled
   in on load). See `settingsoverlay.JavaVersionKey` (#759).
2. **A polymorphic child must be dispatched on `$Type` before any field
   assignment.** Variants can differ in *arity*, not just field values:
   `Settings$SharedValue` carries a `Value`, while `Settings$PrivateValue` is a
   bare marker with no properties at all (the value lives on the developer's
   workstation). Assigning `Value` to whichever node is there corrupts the marker.

The same reasoning bans authoring what the model does not own: mxcli preserves a
constant override's shared/private choice and refuses statements that would flip
it, rather than silently converting one to the other.

Enum-valued properties are the sibling trap: validate against
`generated/metamodel` (e.g. `SettingsDatabaseType` is `Hsqldb`, never `HSQLDB`)
rather than passing a user string through.

**On a CREATE there is no stored document to read the key off.** Rule 1 then
becomes: branch on the project's Mendix version and write exactly one spelling —
never both as a hedge. `mdl/dbconnector` does this for the 11.13 rename of
`DatabaseQuery.QueryType` (int) to `Type` (string enum), which mxbuild *does*
catch, as CE5277 on every activity using the query. To learn the target shape
without guessing, run the new mxbuild's own migration over an old project
(`mx convert -p -s <project>`) and diff the BSON: Mendix ships a one-time
conversion per renamed property, so the converted document is authoritative.
