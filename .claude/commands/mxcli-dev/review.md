---
description: Review the branch's changes against the CLAUDE.md PR checklist
---

# /mxcli-dev:review — PR Review

Run a structured review of the current branch's changes against the CLAUDE.md
checklist, then check the recurring findings table below for patterns that have
burned us before.

## Steps

1. Run `gh pr view` and `gh pr diff` (or `git diff main...HEAD`) to read the change.
2. Work through CLAUDE.md's "Working Rules for a Change" (the evidence bar) and
   the subsystem checklists at the end of this file, in full.
3. Then check every row in the Recurring Findings table below — flag any match.
4. Report: blockers first, then moderate issues, then minor. Include a concrete fix
   option for every blocker (not just "this is wrong").
5. After the review: **add a row** to the Recurring Findings table for any new
   pattern not already covered.

---

## Recurring Findings

Patterns caught in real reviews. Each row is a class of mistake worth checking
proactively. Add a row after every review that surfaces something new.

| # | Finding | Category | Canonical fix |
|---|---------|----------|---------------|
| 1 | Formatter emits a keyword not present in `MDLParser.g4` → DESCRIBE output won't re-parse (e.g. `RANGE(...)`) | DESCRIBE roundtrip | Grep grammar before assuming a keyword is valid; if construct can't be expressed yet, emit `-- TypeName(field=value) — not yet expressible in MDL` |
| 2 | Output uses `$currentObject/Attr` prefix — non-idiomatic; Studio Pro uses bare attribute names | Idiomatic output | Verify against a real Studio Pro BSON sample before choosing a prefix convention |
| 3 | Malformed BSON field (missing key, wrong type) produces silent garbage output (e.g. `RANGE($x, , )`) | Error handling | Default missing numeric fields to `"0"`; or emit `-- malformed <TypeName>` rather than broken MDL |
| 4 | No DESCRIBE roundtrip test — grammar gap went undetected until human review | Test coverage | Add roundtrip test: format struct → MDL string → parse → confirm no error |
| 5 | Hardcoded personal path in committed file (e.g. `/c/Users/Ylber.Sadiku/...`) | Docs quality | Use bare commands (`go test ./...`) without absolute paths in any committed doc or skill |
| 6 | Docs-only PR cites an unmerged PR as a "model example" — cited PR had blockers | Docs quality | Only cite merged, verified PRs; or annotate with known gaps if citing in-flight work |
| 7 | Skill/doc table references a function that doesn't exist (e.g. `formatActionStatement()` vs `formatAction()`) | Docs quality | Grep function names before writing: `grep -r "func formatA" mdl/executor/` |
| 8 | "Always X" rule is too absolute for trivial edge cases (e.g. "always write failing test first" for one-char typos) | Docs quality | Soften to "prefer X" or add an exception clause; include the reasoning so readers can judge edge cases |
| 9 | Doc comment promises a fallback/feature that doesn't exist in the code (e.g., "raw-map fallback in the client" when no such fallback was implemented) | Docs quality | Grep for function/type names referenced in doc comments to confirm they exist before committing |
| 10 | BSON array items decoded by mongo driver are `primitive.D`, not `map[string]any` — bare type assertion `item.(map[string]any)` always fails silently, causing silent data loss (e.g. Languages not parsed, issue #480) | BSON parsing | Always use `extractBsonMap(item)` instead of `item.(map[string]any)`; write a parser unit test with `primitive.D` items to catch this class of bug |
| 11 | `execShow` switch missing a case for a new `ShowXxx` constant — executor handler is wired but never dispatched, command silently does nothing | Dispatch gap | After adding a new `Show*` constant and handler, grep `executor_query.go` to confirm the case is present; add a mock test that calls the handler directly |
| 12 | Mock test constructs a `Kind` value (e.g. `"Array"`) that `parseImportMappingElement` can never produce — parser only sets `"Object"` or `"Value"` — giving false assurance for a code path that is dead against real MPR data | Test coverage | Before writing a mock test for a fallback path, verify the parser can actually produce the mocked value; if not, either extend the parser or remove the dead fallback |
| 13 | Go type switch: inserting `case TypeB:` between `case TypeA:` and its body silently empties TypeA — unlike regular switch, type switch has no fallthrough, so an empty case is a no-op (e.g. EnumSplitStmt handler stolen by InheritanceSplitStmt in PR #475) | Code correctness | Always give each type switch case its own complete block; never share a body by relying on fall-through |
| 14 | Visitor test for `CREATE JAVA ACTION` omits `AS $$ ... $$` body, causing opaque parse error `no viable alternative at input '...'` — the body is mandatory, not optional | Test coverage / grammar | The grammar rule ends with `AS DOLLAR_STRING SEMICOLON?`; always include a minimal body (`as $$ return false; $$;`) even in tests |
| 15 | New MDL document type or `OR MODIFY` variant added but `cmd/mxcli/syntax/features_*.go` not updated — `mxcli syntax <topic>` and REPL `help` show stale syntax | Docs quality | Add/update `SyntaxFeature` entries: new type → new `Register(...)` block; changed syntax → update `Syntax` field of existing topic; grep `Path:` to confirm topic exists |
| 16 | Bug-fix PR missing `mdl-examples/bug-tests/<issue>-description.mdl` — checklist requires one per fix so Studio Pro can validate the regression case | Test coverage | Add minimal MDL that reproduces the symptom; commit alongside the fix; the PR description often contains the exact reproduction snippet already |
| 17 | Commit message claims a change (e.g. `"PERF001": "Performance"` mapping in `report.go`) that is not present in the diff — git body overstates the actual change, often referencing an example rule as if it were shipped | Docs quality | Diff the file the commit names (`git show <sha> -- <file>`); if the change isn't there, fix the commit body so it doesn't imply shipped behavior |

| 18 | A generated artifact hardcodes a value that an exported constant also declares (e.g. `SwitcherStorageKey = "mxcli-theme"` beside four literal `"mxcli-theme"` in the template) — the constant and the artifact can drift, and a test asserting `Contains(output, TheConst)` keeps passing because it is checking the literal, not the link | Test coverage | Substitute the constant into the template (`{{KEY}}` + `strings.NewReplacer`) so there is one source of truth; assert the placeholder is expanded *and* the expected occurrence count |
| 19 | Docs rewritten in one section while an earlier section still points at the removed content — e.g. "Copy the scaffold below" left in place after the scaffold was replaced by "do not hand-roll a scaffold", producing a direct contradiction two paragraphs apart in a skill `mxcli init` syncs into every user project | Docs quality | After deleting or replacing a doc section, grep the whole file for phrases that referred to it ("below", "scaffold", the old heading) and for the old anchor in the Contents list |
| 20 | Asset-driven feature (themes, templates) whose correctness depends on a toolchain the Go tests never run — SCSS that must compile, a mixin whose name must match its `@include`. A broken asset ships and fails at the user's build, not in CI | Test coverage | Assert the naming/structural contract in Go (`@mixin X {` and `@include X;` both present, every `url()` resolves to a shipped file); compile once by hand against a real project and record it in the proposal |
| 21 | A registry that reads only one source (an `embed.FS`) has no extension point, so the only way to customise its output is to hand-edit the generated block — which a digest fence then refuses to touch on the next run. The feature ends up hostile to the exact case it was built for | API design | When a guard refuses a user's edit, ask whether the thing being edited should have been authorable. Introduce a source abstraction (`fs.FS` + root) and let a project-local directory shadow the embedded set — the walk functions usually already take a root, so the change is contained |
| 22 | A test helper that expands a template cross-products every list against every reference (e.g. every `@each $weight` list against every `url()`), inventing artefacts no asset ever shipped and failing on correct input | Test coverage | Scan positionally: a reference must be expanded against the loop it actually sits under. Prove the helper both fires on a real break (delete one shipped file) and stays quiet on correct input — a helper only checked against green code has not been shown to detect anything |
| 23 | A copy-to-scaffold path renames files but not the identifiers built from the name (`@mixin mxcli-<name>-<alt>`, `@import "mxcli-<name>"`), so two artefacts collide the moment both exist — and the symptom is a rule that silently compiles to nothing | Code correctness | Assert the structural contract on the *generated* artefact, not just the shipped ones: factor the built-in's contract test into a helper and run the scaffold through it. Verify once end to end against the real toolchain and record it |
| 24 | A skill or command still instructs work into a package the repo has deleted (`sdk/mpr` test locations, a symptom table moved to `findings/*.jsonl` years prior) — the doc reads as authoritative and every instruction in it is a compile error or a no-op | Docs quality | When a package is deleted or a doc is restructured, grep `.claude/` for its name in the same PR. A deletion that leaves the guidance behind is worse than no guidance, because the reader trusts it |
| 25 | A doc or skill shows a CLI invocation nobody ran — a command name that does not exist (`mxcli dump-bson` for `mxcli bson dump`), or a flag form the parser rejects (`--compare "A" "B"` where `--compare` is a StringSlice needing `"A,B"`). Worst when copied FROM the command's own `--help`, which had the same error, so the doc looks sourced | Docs quality | Run every command a doc shows, against a real project, before committing it. If it came from `--help`, run that form too — the example in the help text is not evidence that it works |
| 26 | A clause added to a SHARED grammar rule (a datasource, a widget-property list) is written by only ONE of the constructs that rule serves — the others parse it, `exec` reports success, and DESCRIBE does not echo it back. A silent drop, often shipped by the very change that was fixing silent drops | Code correctness | Enumerate the other constructs the rule serves and RUN one. The round-trip that proved the feature on its intended target says nothing about them. Refuse it where it cannot be stored, naming the construct that can — an error, not a warning, when the metamodel decides it and no future package can make it valid |
| 27 | A metamodel-sync or list-coverage test asserts that a GAP still exists (`clickCapableInMendix["listview"]`, "a template for the list view's own entity is the base case Mendix permits") — so it passes throughout and FAILS on the correct fix, and the belief it encodes was never measured | Test coverage | Invert such a test rather than deleting it: keep the half that is still true (the metamodel really does carry the field) and flip the half that is not. When a test justifies itself by what a helper returns rather than by a measurement, treat it as a claim to check, not as evidence |
| 28 | A describe emitter added beside a shared property formatter duplicates a field the formatter already prints (`Editable: true` twice on one widget) — invisible when the round-trip only covers the page the change was written against | DESCRIBE roundtrip | Round-trip a page OTHER than the one under test, and assert occurrence COUNT (`strings.Count(out, x) != 1`), not presence. `Unchanged page` on re-exec of the describe output is the evidence that the emitted MDL rebuilds the stored document; `Check passed!` is not |
| 29 | A predicate that names ONE cause of a build error is read as if it named the error (`mem.IsCalculated` for CE6592, which an autonumber also triggers) — the half that is covered works, so every test passes and the gap is invisible until a user hits the other half | Code correctness | When a guard cites a CE number, enumerate what the PLATFORM rejects, not what the current code checks. Put the rule in one named place (`types.WriteRightsForbidden`) rather than a bare boolean at each site, so the second cause has somewhere to go. And fix every pass that can re-derive the value — a reconcile running after every program re-broke a grant the user had corrected by hand |
| 30 | Two commands compute the same thing from two copies of the setup (`report` re-implementing `lint`'s rule list and skipping its config), so they disagree about a project — and a SCORE carries no provenance, so neither number looks wrong | Code correctness | Extract the shared setup and route both through it. A value test cannot guard this when the copies live inside cobra `RunE` bodies: use a structural check on the source, with a positive control asserted FIRST so it cannot pass vacuously |
| 31 | A test helper that needs a heavyweight object only to satisfy a signature (`NewLintContext(nil, nil)`, which panics) invites a nil-guard added purely to make the test compile — behaviour nothing in production needs, defended forever | Test coverage | Narrow the signature instead: if the helper does not use the parameter, drop it and let the caller apply the part it owns. A test that cannot construct an argument is usually telling you the argument does not belong |
| 32 | A fix adds a diagnostic for a capability the model lacks while leaving in place the code that asserts the capability EXISTS — MDL042 telling the author a loop's `@caption` is dropped, while `cmd_microflows_builder_annotations.go` still ran `case *microflows.LoopedActivity: activity.Caption = ann.Caption` under the comment "LOOP / WHILE activities can carry a caption just like splits", and the describer still emitted one. Nothing read either back. The next reader trusts the code over the warning, deletes the check, and reopens the bug from the other side. The reason it survives is that it usually has TESTS — three here asserted the caption was carried, all of them against the semantic object and none against storage, so they passed throughout and failed only on the correct fix | Code correctness | `generated/metamodel` is the arbiter: a field on the semantic type it does not declare cannot survive a write, so an assignment to it is dead by construction. Grep the writer, the describer and the semantic struct and delete (or re-comment) whatever sets it. MEASURE before deleting — `exec` then `describe` on a real project, with the UNMODIFIED build, so the deletion rests on the stored document rather than on reading the codec. Invert the tests that defended it rather than deleting them, keeping any half still true (escaping coverage belongs on a type that can carry a caption), and check the inverted test fails when the assignment is put back |
| 33 | A column added to `createTables` without bumping `CatalogSchemaVersion` — the version guard only drops tables when it CHANGES, and `CREATE TABLE IF NOT EXISTS` never adds a column, so every user with a cached catalog keeps a table the new SELECT cannot read. Measured on #1181: `activities_for` yielded 302 on `main` and **0** on the branch against the same cache, `no such column: UseRequestTimeout`, `mxcli lint` exit 1. `mxcli report` runs the same rules and never checks `QueryErrors()`, so there it would score silently | Code correctness | Bump the constant in the same commit — its doc comment says so and `62913741` is the precedent (four columns + 11→12 + a builder test). To REPRODUCE, the cache must actually be reused: build it with the old binary and run the new one with the **same spelling of `-p`**, because a relative-vs-absolute path invalidates on "MPR path changed" and hides the bug; confirm the run says "Loading cached catalog … (from cache)" before believing a green result |

---

## After Every Review

- [ ] All blockers have a concrete fix option stated.
- [ ] Recurring Findings table updated with any new pattern.
- [ ] If docs-only PR: every function name, path, and PR reference verified against
      live code before approving.

## The subsystem checklists

These moved out of CLAUDE.md, where they were re-read into every session but only
apply when a change touches that subsystem. The evidence bar for a bug fix, and
the one-thing-per-commit rule, stay there because they govern how the work is
done rather than how it is reviewed.

### Overlap & duplication
- [ ] Check `docs/11-proposals/` for existing proposals covering the same functionality
- [ ] Search the codebase for existing implementations (grep for key function names, command names, types)
- [ ] Check `mdl-examples/doctype-tests/` for existing test coverage of the feature area
- [ ] Verify the PR doesn't re-document already-shipped features as new

### Syntax design for MDL features
New or modified MDL syntax must follow the design guidelines. See [ADR-0003: MDL is SQL-shaped](docs/13-decisions/0003-mdl-is-sql-shaped.md) for the underlying decision and rejected alternatives; the design checklist below operationalises it.
- [ ] **Design skill consulted** — read `.claude/skills/design-mdl-syntax.md` before designing syntax
- [ ] **Follows standard patterns** — uses `create`/`alter`/`drop`/`show`/`describe`, not custom verbs
- [ ] **Reads as English** — a business analyst understands the statement on first reading
- [ ] **Qualified names** — uses `Module.Element` everywhere, no implicit module context
- [ ] **Property format** — uses `( key: value, ... )` with colon separators, one per line
- [ ] **LLM-friendly** — one example is sufficient for an LLM to generate correct variants
- [ ] **Diff-friendly** — adding one property is a one-line diff

### Version compatibility
New features that depend on a specific Mendix version must be version-gated:
- [ ] **Registry entry** — feature added to `sdk/versions/mendix-{9,10,11}.yaml` with correct `min_version`
- [ ] **Executor pre-check** — `checkFeature()` called before BSON writes, with actionable error and hint
- [ ] **Test coverage** — version-gated tests use `-- @version:` directives or `requireMinVersion()`
- [ ] **Skill updated** — `.claude/skills/version-awareness.md` updated if the feature has a workaround for older versions

### Backend abstraction compliance
All executor code must go through the backend abstraction layer. **`sdk/mpr` no longer exists** — the package was deleted once its importer count reached zero, so reaching past the abstraction is now a compile error rather than a rule to remember. See [ADR-0002: Backend Abstraction Layer](docs/13-decisions/0002-backend-abstraction.md) for the context and alternatives. The codec (`modelsdk`) engine is the only local engine — the legacy `sdk/mpr` backend was deleted (`docs/plans/2026-09-14-retire-legacy-engine.md`), and `--engine`/`MXCLI_ENGINE` survive only as a warning-only no-op. It routes **all** document types — domain models included — through the codec, not a codec/legacy hybrid; see [ADR-0004: Full codec engine](docs/13-decisions/0004-full-codec-engine.md). Where the codec path cannot yet reproduce a construct, the backend **refuses** the op rather than dropping data. The backend interface speaks the **semantic model**, not gen/BSON or AST types — gen+codec are the MPR backend's internal storage adapter, one of several (MPR, MCP/PED, a future storage format); see [ADR-0005](docs/13-decisions/0005-semantic-model-interface-currency.md). CREATE is model→gen; fidelity-sensitive ALTER uses backend-internal gen-mutation, not a model round-trip.
- [ ] **No engine internals in the executor** — executor files must not reach into `modelsdk/mpr`, `modelsdk/codec` or `modelsdk/gen` directly; use `ctx.Backend.*` instead. A method missing from the backend gets implemented there, not bypassed
- [ ] **New backend methods on the interface** — any new data access or mutation goes in the appropriate interface in `mdl/backend/` (e.g., `DomainModelBackend`, `MicroflowBackend`), not as a direct SDK call
- [ ] **MPR implementation in `mdl/backend/mpr/`** — the concrete implementation lives here; all BSON/reader/writer logic stays in this package
- [ ] **Mock stub in `mdl/backend/mock/`** — every new backend method has a `Func`-field stub with a descriptive `"MockBackend.X not configured"` error default (not `nil, nil`)
- [ ] **Compile-time interface check** — new backend implementations have `var _ backend.SomeInterface = (*impl)(nil)`
- [ ] **ALTER operations use mutator pattern** — page/workflow mutations go through `ctx.Backend.OpenPageForMutation()` / `OpenWorkflowForMutation()`, not inline BSON construction
- [ ] **New shared types in `mdl/types/`** — a type used by more than one layer goes in `mdl/types/` and the others alias it (`type Foo = types.Foo`), never as duplicate definitions. A same-shape duplicate compiles and tests green; it shows up only as an assignment failure *across* the boundary, naming the same type on both sides of "want". `modelsdk/mpr/version.ProjectVersion` was that case and is now an alias — the guard is a compile-time assertion (`var _ *types.ProjectVersion = (*version.ProjectVersion)(nil)`, `version_alias_test.go`), which builds only under an alias and so is stronger than anything a test body can assert
- [ ] **Map iteration is deterministic** — any map iterated for serialization output must sort keys first (`sort.Strings(keys)` pattern); non-deterministic output causes flaky diffs and BSON instability
- [ ] **Pluggable widgets via WidgetEngine** — new pluggable widget support uses `.def.json` + `WidgetRegistry`; no hardcoded BSON widget builders in the executor

### Full-stack consistency for MDL features
New MDL commands or language features must be wired through the full pipeline:
- [ ] **Grammar** — rule added to `MDLParser.g4` (and `MDLLexer.g4` if new tokens)
- [ ] **Parser regenerated** — `make grammar` run; generated files in `mdl/grammar/parser/` are **not** committed (they are regenerated by `make` at build time)
- [ ] **AST** — node type added in `mdl/ast/`
- [ ] **Visitor** — ANTLR listener bridges parse tree to AST in `mdl/visitor/`
- [ ] **Executor** — thin handler in `mdl/executor/` dispatches to `ctx.Backend.*`; no BSON in the handler
- [ ] **Backend method** — data access or mutation wired through `mdl/backend/` interface and implemented in `mdl/backend/mpr/`
- [ ] **LSP** — if the feature adds formatting, diagnostics, or navigation targets, wire it into `cmd/mxcli/lsp.go` and register the capability
- [ ] **DESCRIBE roundtrip** — if the feature creates artifacts, `describe` should output re-executable MDL
- [ ] **VS Code extension** — if new LSP capabilities are added, update `vscode-mdl/package.json`

### Test coverage
- [ ] New packages have test files
- [ ] New executor commands have MDL examples in `mdl-examples/doctype-tests/`
- [ ] **MDL syntax changes** — any PR that adds or modifies MDL syntax must include working examples in `mdl-examples/doctype-tests/`
- [ ] **Bug fixes** — every bug fix should include an MDL test script in `mdl-examples/bug-tests/` that reproduces the issue, so the fix can be verified in Studio Pro if applicable. **Three numbering namespaces meet in that directory**: the historical files are named after `mendixlabs/mxcli` **PR** numbers (`261-mx9-microflow-roundtrip.mdl` is upstream PR #261), issues filed on the fork are `ako/mxcli` numbers — and the two sequences already collide on 261–266 — while a few names are a **Mendix version** with the dot dropped (`1113-database-query-type-enum.mdl` is Mendix 11.13, not issue 1113). Name a file after a fork issue with a topic prefix (`mapping-261-object-handling-backup.mdl`) and write the reference qualified (`ako/mxcli#261`) wherever it appears, or the number silently resolves to the wrong thing
- [ ] Integration paths (not just helpers) are tested
- [ ] Tests don't rely on `time.Sleep` for synchronization — use channels or polling with timeout

### Security & robustness
- [ ] Unix sockets use restrictive permissions (`os.Chmod(path, 0600)`)
- [ ] File I/O is not in hot paths (event loops, per-keystroke handlers) — cache in memory
- [ ] No silent side effects on typos (e.g., auto-creating resources on misspelled names should be flagged)
- [ ] Method receivers are correct (pointer vs value) for mutations

### Documentation
- [ ] **Skills** — new features documented in `.claude/skills/` (syntax, examples, gotchas)
- [ ] **CLI help (Cobra)** — `mxcli` subcommand help text updated (Cobra `Short`/`Long`/`Example` fields)
- [ ] **CLI help (syntax topics)** — `cmd/mxcli/syntax/features_*.go` updated with new/changed MDL syntax; new `SyntaxFeature` entries added for new document types; `OR MODIFY` / `OR REPLACE` variants reflected in existing `Syntax` fields; accessible via `mxcli syntax <topic>` and REPL `help`
- [ ] **Syntax reference** — `docs/01-project/MDL_QUICK_REFERENCE.md` updated with new statement syntax
- [ ] **MDL examples** — working examples added to `mdl-examples/` for new commands
- [ ] **Site docs** — `docs-site/src/` pages added or updated for user-facing features

### Code quality
- [ ] Refactors are applied consistently across all relevant files (grep for the old pattern)
- [ ] Manually maintained lists (keyword lists, type mappings) are flagged as maintenance risks
- [ ] Design docs match the actual implementation — remove or update stale plans
- [ ] Numeric type conversions are bounds-checked — `float64→int` casts need overflow guards (`±2^53` for safe integer range); silent overflow produces garbage in serialized output
- [ ] `convert.go` updated when structs in `mdl/types/` gain or lose fields — `TestFieldCountDrift` will catch this at test time, but `convert.go` must be updated before merging
