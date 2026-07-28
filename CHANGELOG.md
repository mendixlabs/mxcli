# Changelog

All notable changes to mxcli will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed

- **`ALTER PAGE … REPLACE` of a pluggable widget no longer triggers CE0463 (#112)** — replacing a pluggable widget (e.g. a combobox) emitted `Type` metadata (per-property `Caption`/`Category`) generated from the embedded template and installed `.mpk`, while untouched sibling widgets in the same page unit still carried metadata from the (older) widget version that authored them in Studio Pro. MxBuild flagged the mixed vintages as CE0463 ("The definition of this widget has changed") on the rebuilt widget — even for an identity rebuild that changed nothing. The replacement now grafts the stored widget's per-property display metadata (`Caption`, `Category`, `Description`) onto the freshly generated `Type` block, matched by property-key *path* (so nested object types that reuse a key, like a Maps widget's `markers/latitude` vs `dynamicMarkers/latitude`, stay independent) and only when the `WidgetId` matches — including pluggable widgets nested inside container replacements. Structural fields (`$ID`s, `ValueType`s) are never copied, so the new widget's `Object`↔`Type` cross-references stay intact. Verified against a Mendix 11.12.2 project: identity rebuild and datasource-XPath replace of a Combobox v2.8.1 both now pass `docker check` with zero errors (previously CE0463).

## [0.16.0] - 2026-07-12

Headline: **Pluggable chart authoring reaches round-trip fidelity**, plus in-place enum-caption editing, named layout placeholders, and a batch of new pre-build `check` heuristics. Charts gain widget-level datasource attributes, the `LINE`/`SCALECOLOR` object-list keywords, and a `DESCRIBE` that reconstructs them as executable MDL; workflows and widget-less pages now describe cleanly; view-entity OQL is validated before build; and several authoring mistakes are caught at `mxcli check` time instead of only by MxBuild.

### Added

- **Chart authoring expanded — object-lists, scale colours, and `DESCRIBE` round-trip** — building on the v0.15.0 chart-series work: widget-level datasource attributes for `PieChart`/`HeatMap` (item 1b), the `LINE` and `SCALECOLOR` object-list keywords, and a `DESCRIBE` that reconstructs pluggable-widget object-lists (series / line / scalecolor) as executable MDL, so an existing chart's syntax can be learned by describing it. Validated `LineChart` / `HeatMap` / `TimeSeries` / `BubbleChart` examples now run in the regular check-mdl suite.
- **`ALTER ENUMERATION … MODIFY VALUE <name> CAPTION '…'`** — edit an enumeration value's caption in place, without dropping and recreating the value.
- **Assign widgets to named layout placeholders** (#532) — a widget can be placed into a layout's named placeholder, not only the default content slot.
- **New authoring-time `check` heuristics** — caught at `mxcli check`, before a build round-trip:
  - `MDL043` — rejects an object/list-typed `declare` where a primitive variable is required.
  - `MDL032` — warns on a reserved OQL word used as a view-entity attribute name.
  - a derived-string view entity that MxBuild rejects with `CE6770` is now caught pre-build (reviving the OQL type-checker it depends on).
- **Workflow authoring skill (Bug 11a)** — added `.claude/skills/mendix/write-workflows.md` (synced to user projects and CLAUDE.md's "read first" list) documenting `CREATE` / `DROP` / `ALTER WORKFLOW`: the activity grammar (user task, multi-user task, decision, parallel split, jump, wait-for-timer/notification, boundary events), header options, and the two first-attempt gotchas (`PARAMETER $var: Entity`, body closes with `END WORKFLOW`). Workflow authoring already worked and built, but no skill documented it, so it read as read-only.
- **`DynamicClasses` documented across the styling surfaces** — the runtime-computed CSS-class property (a sibling of `Class`/`Style`) was wired and skill-documented but missing from every reference enumeration that lists its siblings. Added it to the `mxcli syntax page.styling` topic, `MDL_QUICK_REFERENCE.md` (styling table + ALTER PAGE `SET` properties), and the docs-site pages (`quick-reference`, `create-page`, `widget-types`, `alter-page`). Also demonstrated end-to-end in the `12-styling` doctype example (create-time, bulk `UPDATE WIDGETS`, and `ALTER PAGE ... SET DynamicClasses ON <container>`).

### Fixed

- **`DESCRIBE PAGE`/`DESCRIBE SNIPPET` of a widget-less page/snippet now re-parses (#626)** — an empty page described as `create page … ( … )` with no `{ }` body block, which the CREATE PAGE grammar rejects, so the DESCRIBE output failed to round-trip through `mxcli check`. Empty pages and snippets now emit an empty `{ }` body. Added an integration test (`TestRoundtripPage_DescribeReparses`) that actually re-parses DESCRIBE output (not just substring assertions); a full describe→check pass over the 57 `03-page-examples` pages is clean.
- **`DESCRIBE WORKFLOW` round-trips all activities as executable MDL (Bug 11b)** — on the default `modelsdk` engine, jump-to, wait-for-timer, and wait-for-notification activities (and the implicit start/end) decoded to `GenericWorkflowActivity` and rendered as non-executable `-- [Workflows$…]` comments, so `describe → exec` dropped them and their syntax couldn't be learned by describing an existing workflow. The reader now reconstructs these as typed activities (reading the jump target and timer delay from raw BSON), matching the legacy engine and the describe formatter, which already handled them. Round-trip verified: `describe → drop → exec → docker check` with zero workflow errors.
- **Containers now appear in the widget catalog** — `show widgets`, `update widgets`, and `CATALOG.widgets` queries dropped every `container` (`Forms$DivContainer` / `Pages$DivContainer`), so a `WHERE widgettype LIKE '%Container%'` filter silently matched nothing and containers couldn't be bulk-styled — even though they carry `Class` / `Style` / `DynamicClasses` / `DesignProperties` and can be clickable. The catalog now indexes user-authored containers and skips only the synthetic transparent `conditionalVisibilityWidget*` layout wrapper (matching how `DESCRIBE PAGE` already unwraps it). Other container types (LayoutGrid, TabContainer, …) were already indexed.
- **Widget action microflow parameter mappings persist (Bug 1)** — a widget action's microflow parameter mappings were dropped on write; they are now serialized and round-tripped.
- **Nanoflow client actions on widgets (Bug 2)** — the `modelsdk` engine now writes nanoflow client actions on widgets; previously only microflow actions serialized.
- **HeatMap scale colour persists via the `ColorValue` alias (Bug 10a class)** — a HeatMap's scale colour was not written; it now serializes through the `ColorValue` object-list alias.
- **`ALTER ENTITY … MODIFY ATTRIBUTE` constraints and `MOVE ENUMERATION` folder reads (Bug 12)** — the read paths for attribute constraints and an enumeration's folder are corrected so both operations round-trip.
- **View-entity OQL is read from the source document on the `modelsdk` engine (Mendix 11.x)** — so `DESCRIBE` and validation see the actual query text.
- **ELSIF arms preserved by lowering to nested `IfStmt` (#746)** — microflow expression `ELSIF` branches were dropped on round-trip; they are now retained.

## [0.15.0] - 2026-07-10

Headline: **A page-authoring fidelity wave on the `modelsdk` engine**, plus MCP pluggable-widget authoring (Phases 1–2), Playwright warm-session reuse, and new `check` heuristics for widget properties. A batch of numbered page bugs (DataView/DataGrid2/widget serialization and round-trip) are fixed, microflow round-trip gaps (#723) are closed, and several new authoring-time checks catch widget mistakes before they reach MxBuild.

### Added

- **`SHOW` / `DESCRIBE ICON COLLECTION`** — discover the icons in an icon collection (`CustomIcons$CustomIconCollection`, e.g. `Atlas_Core.Atlas_Filled`) so you can pick a valid name for a widget's `icon:` (icons have non-obvious names — it's `add`, not `plus`). `SHOW ICON COLLECTIONS` lists the collections (name, prefix, export level, icon count); `DESCRIBE ICON COLLECTION Module.Name` lists every icon with its ready-to-use `Module.Collection.IconName` reference. Read-only (icon collections ship with the theme/Atlas). Works on both engines. As a bonus, the `COLLECTION` keyword now accepts the plural, so `SHOW IMAGE COLLECTIONS` (and `ICON COLLECTIONS`) parse alongside the singular form.
- **Button icon (`icon:`) on action/link buttons** (#602) — `actionbutton`/`linkbutton` now accept `icon: 'Module.IconCollection.IconName'`, serialized as a `Forms$IconCollectionIcon` (the modern Atlas icon), so a link-mode button can carry a dedicated icon. Verified against a Studio-Pro-authored button; `describe` round-trips the `Icon:` clause and `mx check` is clean (an unknown icon name is correctly rejected with CE1613). Previously `icon:` was silently dropped (flagged by MDL-WIDGET07). Link render mode itself (`linkbutton`) already shipped.
- **REPL filesystem path completion for `EXECUTE SCRIPT`** — pressing Tab while typing the path argument of `execute script '<path>'` now completes against the filesystem (e.g. `execute script "mdl-`⇥ → `mdl-examples/`). Directories complete with a trailing `/` so you can keep tabbing to descend, hidden entries are offered only when the fragment starts with `.`, and completion works whether or not a project is connected (you often run a script to connect in the first place). Both single- and double-quoted paths are handled; keyword/object-name completion is unaffected.
- **Author Mendix Charts series via MDL** (Bug 9a) — SERIES chart types can now bind their data via MDL; object-list datasource sub-properties work, and the multi-widget docs are corrected.
- **DataGrid2 column binding to an associated attribute** (Bug 7) — a DataGrid2 column can now bind to an attribute reached over an association, not just a direct attribute of the grid's entity.
- **MCP pluggable-widget authoring** — the experimental MCP/PED backend can now author pluggable widgets against a running Studio Pro: Phase 1 accepts any registry-resolved pluggable widget via the shared `.def.json` registry; Phase 2 implements the expression, text-template, and action widget ops.
- **Playwright warm-session reuse and lifecycle control** — verify runs reuse a warm browser session across invocations, with new `open` / `status` / `close` session-lifecycle subcommands.
- **New authoring-time `check` heuristics for widgets** — `MDL-WIDGET07` warns on unrecognized built-in widget properties; `MDL-WIDGET08` flags invalid enum values on widget object-list sub-properties and rejects an association datasource on a `DataView`; `MDL-WIDGET09` rejects an invalid `DataView` database source.

### Changed

- **Go toolchain 1.26.4 → 1.26.5** for GO-2026-5856.

### Fixed

- **Page / widget serialization on the `modelsdk` engine** — a wave of numbered page bugs:
  - DataGrid2 column properties are now ordered by the widget template, fixing CE0463 on the modelsdk engine (Bug 6).
  - An association attribute is resolved correctly from a subclass context (Bug 3).
  - `DataView` "data from context over association" is supported, and an invalid association/database `DataView` source is now refused at both `check` and `exec` (Bug 5, `MDL-WIDGET09`).
  - Widget datasource `sort by … desc` is persisted and round-tripped by `DESCRIBE` (Bug 8).
  - `DynamicCellClass` is persisted and `dynamicclasses` is lowercased (Bug 10).
  - `DynamicText` contentparam over an association is persisted.
  - The `Visible` string/boolean conditional-visibility form and widget `DynamicClasses` expressions are persisted.
- **MDL parsing** — identifier quotes are stripped in expression contexts and in inline-bracket XPath (datasource `WHERE`).
- **OQL select-clause parser** was case-broken and is now case-insensitive (Bug 9b).
- **Microflow round-trip on the `modelsdk` engine** (#723) — execution flags (A1), flow-object box size (A2), and rule-based decisions (`IsRule`, A4) now read back correctly.
- **`docker` widget update** — the absolute `.mpr` path is passed to `mx update-widgets`, fixing a crash that left CE0463 unresolved.
- **MCP verify-on-timeout** — Studio Pro's `-32000` false failures are re-verified instead of reported as failures.
- **Executor** keeps the connection when a script reconnects internally.
- **Dependency bump** — `golang.org/x/crypto` → v0.52.0.

## [0.14.0] - 2026-07-06

Headline: **Mendix 11.12 support and `modelsdk`-engine parity.** This release makes mxcli's output load and build cleanly on Mendix 11.12 (strict `$ID`-first BSON ordering, and `CloseFormAction` / conditional-settings / number-filter serialization fixes), closes dozens of `DESCRIBE` and read-fidelity gaps on the now-default `modelsdk` engine, hardens OData import and publishing, and substantially expands the Starlark lint surface and the experimental MCP/PED backend. It also adds `CREATE` / `DROP JAVASCRIPT ACTION`, clickable containers, chart widgets, page-level CSS, and `linkbutton`.

### Added

- **`CREATE` / `DROP JAVASCRIPT ACTION`** — author JavaScript actions in MDL, mirroring `CREATE JAVA ACTION`, on both engines:

  ```sql
  create [or modify] javascript action Mod.Name(P: Type not null)
    returns Type
    [exposed as 'caption' in 'category']
    [platform Web|Native|Hybrid|All]   -- Web default
  as $$ <javascript> $$;
  drop javascript action Mod.Name;
  ```

  Each create writes the `JavaScriptActions$JavaScriptAction` unit plus `javascriptsource/<Module>/actions/<Name>.js` (BEGIN/END USER CODE markers), and the action is callable from nanoflows via `CALL JAVASCRIPT ACTION`. A JS action's BSON is structurally identical to a Java action but with `JavaScriptActions$` `$Type` names and a `Platform` field; the modelsdk engine encodes through the working Java gen path then rewrites the `$Type`s and injects `Platform`, so there's no generated-code divergence. `DESCRIBE` emits re-executable MDL. Verified end-to-end under both engines (`mx check` = 0). Ships with a synced user skill and a docs-site reference page.
- **Clickable `CONTAINER` — `OnClick:` / `Action:`** (#603) — a container's on-click action can now be set (`container c (OnClick: microflow Mod.Foo) { … }`, or the `Action:` alias), wired through both engines with a clean `DESCRIBE` roundtrip. Previously `OnClick:`/`Click:` errored in the parser and the one form that parsed (`Action:`) was silently dropped — the container always serialized `Forms$NoAction`. Non-clickable containers are unchanged (still `NoAction`).
- **`linkbutton` widget** — the documented `linkbutton (caption, action)` now builds (previously `exec` failed with "unsupported widget type: linkbutton" even though the grammar token and a `pages.LinkButton` stub existed). It serializes as a `Forms$ActionButton` with `RenderType: "Link"` — the modern toolbox "link button", not the legacy address-based `Forms$LinkButton` — so it reuses the proven action-button BSON with no CE0463 risk. Works in both `CREATE PAGE` and `ALTER PAGE INSERT`, and `DESCRIBE` round-trips the `linkbutton` keyword.
- **Chart widgets from bundled `.mpk` packages** (#679) — `ParseMPK` only read `WidgetFiles[0]`, so a package bundling several widgets registered only its first, and `Charts.mpk` (10 widgets, `AreaChart` first) left `BarChart`/`ColumnChart`/`PieChart`/`LineChart`/etc. invisible — `exec` failed "no definition for widget …" even after `widget init`. Every widget in a bundled package is now registered (`ParseMPKAll` / `ParseMPKWidget`); `widget extract`, `widget init`, and `refresh catalog` emit a def per bundled widget (`WidgetDefGeneratorVersion` 4→5, so existing projects regenerate). The SERIES chart types (Bar/Column/Area) are now authorable; a new `34-chart-widget-examples.mdl` documents them and the still-open gaps (per-series datasource binding, `LINE`/`SCALECOLOR` keywords).
- **Page-level CSS `Class` and `Style`** (#714) — `CREATE PAGE (Class: '…', Style: '…')` and `ALTER PAGE … SET (Class = …)` now write the page's `Forms$Appearance` class/style (previously rejected — `Class`/`Style` are reserved lexer tokens the generic header branch never matched). Wired through both engines with a `DESCRIBE` roundtrip.
- **Native `ListView` database datasource** — `listview (datasource: database from X where … sort by …)` now works on the default `modelsdk` engine (`Forms$ListViewXPathSource` with sort bar and search); previously only microflow sources were serialized, and a database source errored with "rerun with MXCLI_ENGINE=legacy". DataView/DataGrid2/Gallery database sources already worked.
- **`check --references` flags `System.owner` XPath refs on entities that don't store owner** (#641) — a retrieve/datasource constraint referencing `System.owner` / `changedBy` / `changedDate` / `createdDate` on an entity that doesn't store that member (which Studio Pro rejects with CE0161) is now caught at `mxcli check` time, with the exact `alter entity X add attribute owner: autoowner` fix hint. Fires against existing project entities (associations traversed via `/` are excluded, so a related entity's owner isn't false-flagged).
- **`check` heuristics for constructs MxBuild rejects** — several checks now catch at `mxcli check` time what previously only surfaced as a failed build round-trip:
  - `MDL041` — integer `div` (which yields Decimal) assigned to an `Integer`/`Long` target (CE0117). A rounding-function result (`round(...)`, `floor(...)`) into an Integer is deliberately not flagged. Wires the expression type-checker (`mdl/exprcheck`) into syntax-only `check` for the first time.
  - `MDL042` — `@caption` applied to a loop, which Mendix silently drops (for-loops have no caption); points to `@annotation`.
  - `MDL-WF01`/`WF02`/`WF03` and `MDL-BUTTON01` — workflow user-task-without-a-page (CE1834), single-outcome user task with a nested flow (CE1876), a decision/microflow outcome that isn't a valid enumeration identifier, and a DataGrid control-bar button passing the unbound `$currentObject` (CE1571). Workflows previously received no semantic validation at all. (Wave 1 of the MxBuild-gap-heuristics proposal.)
  - `MPR010` — an edit/new-form `DataView` containing input widgets but not wrapped in a layout grid (labels/inputs render misaligned). Available as both a `mxcli lint` rule and an authoring-time `mxcli check` warning; the built-in rule count is now 15.
- **Starlark lint expansion** — new query functions and flags for custom rules:
  - `constants()`, `scheduled_events()`, and `xpath_expressions()` query functions, plus `parse_xpath(expr)` to walk a parsed XPath/expression AST — enabling performance/security rules that inspect the parsed tree rather than raw strings.
  - `mxcli lint --rules/-r <IDs>` to isolate specific rules and `--modules/-m <names>` to scope to specific modules during rule development.
  - `mxcli lint` now honours `lint-config.yaml` (exclude modules, enabled flags, severity overrides, per-rule `options`) — previously only the REPL/script path did; adds a `get_option(key, default)` builtin.
  - Lint rules that need catalog depth self-declare it (#721): a rule using `refs_to`/`cycles`/… auto-upgrades the build to `full`/`communities` mode instead of silently returning empty results.
- **MCP/PED backend authoring surface** — the experimental backend for authoring against a running Studio Pro gained:
  - `GRANT` entity access rules over PED (#704) — access rules live on the domain-model document, so they're reachable even though the security documents are sealed (add/modify-only; a `GRANT` for a role that already has a rule, and `REVOKE`, are rejected honestly rather than faked).
  - `CREATE OR REPLACE NAVIGATION` web-profile authoring over PED (#699) — home page, login page, not-found page, and the menu tree.
  - Attribute default values over PED on Studio Pro 11.12+ (`ped_update_document` path-op on the `StoredValue`; project-version gated, since the PED server reports 1.0.0 on both 11.11 and 11.12).
  - Page-level `ALTER PAGE … SET (Title = …)` over MCP.
- **`docker init` detects a stale `docker-compose.yml`** — the compose template now carries a version stamp, and `docker up`/`down`/`logs`/`status`/`shell` warn when a project's compose predates the current template (so template fixes like the OQL live-preview flags reach projects that ran `init` before the fix), pointing at `mxcli docker init --force`.

### Changed

- **Nightly integration tests run against Mendix 11.12** (was 11.11), and the doctype `mx check` gate now runs every example through **both** engines (`modelsdk` default + `legacy`) so `modelsdk`-only serialization regressions are caught (#691). Marketplace-module test dependencies are now version-selected per script (e.g. External Database Connector 6.2.3 for ≤11.11, a slimmed 6.3.0 for 11.12+).
- **Dependency bump** — `actions/cache` 5 → 6.

### Fixed

#### Mendix 11.12 load & build compatibility

- **`$ID` must be the first property of every storage object** — Mendix 11.12's stricter streaming reader rejects a unit whose storage object doesn't lead with `$ID`, failing to load projects that ≤11.11 tolerated (`Expected '$ID' as the first property of a storage object, but got '$Type'/'Name'/…`). Fixed across three passes: `bson.M` (Go-map) storage objects converted to ordered `bson.D` (they only landed `$ID`-first by luck of map iteration); the already-ordered `bson.D` entity/attribute/access-rule/security serializers reordered to `$ID`-first; and every map-passthrough marshal site (settings raw-parts, ref-marking, domain-model `raw`) normalized via a non-sorting `HoistStorageID` that lifts `$ID`/`$Type` to the front without reordering the delicate widget maps a full sort would corrupt.
- **`CloseFormAction` page count wrote the wrong field name** — a CLOSE PAGE activity failed `mx check` on 11.12 with CE0117 (legacy engine); the count was written under `NumberOfPagesToClose` but the metamodel storage name is `NumberOfPages`. Now written correctly (the reader accepts both).
- **Conditional visibility/editability settings serialized `Attribute` as `null`** — a widget with a conditional setting failed to *load* on 11.12 (`StorageLoadException: '' is not a valid AttributeIdentifier`); Studio Pro writes the empty string `""` there. Now emits `""` not `null`.
- **`numberfilter` template had markerless empty arrays** — a `NUMBERFILTER` in a DataGrid column passed `check` but corrupted the `.mpr` on 11.12 (`WidgetProperty … does not contain a constructor with a parameter of type … WidgetValue`); its placeholder `Forms$ClientTemplate` blocks were authored with bare `[]` arrays missing the Mendix list marker int. Fixed in both engine copies of the template, with a guard test (`TestTemplates_NoMarkerlessEmptyArrays`) that rejects any markerless array in an embedded template.

#### `modelsdk` engine read & serialization parity

- **`DESCRIBE MICROFLOW` fidelity on the default engine** — a broad set of activities that rendered as `-- Empty action` or lost detail now round-trip, matching (and in a few cases exceeding) the legacy engine: loop `break`/`continue`; `EXPORT TO MAPPING` / `IMPORT FROM MAPPING` and single-object mapping cardinality; the retrieve `sort by` clause (read); inheritance-split variable and `@caption`; `download file`; legacy SOAP `call web service`; REST `body mapping from $var`; rule-based exclusive splits (were rendered as `if true`); the `NewCaseValue` branch case (a missing case dropped the entire then-body); `grant execute` on microflow/nanoflow allowed roles; and an unrecognized split case value that dropped a then-branch is now recovered by elimination.
- **Retrieve `sort by` columns dropped on write** (#727) — a database retrieve's sort clause was serialized to an empty list on the modelsdk engine (stored under the wrong key), so `DESCRIBE` emitted no sort. Now written under `Sortings`, mirroring the Sort list-operation.
- **`@annotation` on a microflow activity was silently dropped** on the modelsdk engine (no `Microflows$Annotation` case, and the linking `AnnotationFlow` was never emitted). Now serialized so notes round-trip.
- **External (OData) entities & associations serialized as plain persisted entities** (#718) — a regression from the default-engine switch: the modelsdk write adapters dropped external-entity serialization, so `CREATE EXTERNAL ENTITIES` produced plain entities and NPEs without their `from Service` link. The legacy serializer's logic is ported into the codec adapters (`Rest$OData*` gen types).
- **External OData string attributes lost `Length=0` (unlimited)** — the codec encoder only emits dirty properties, so an unlimited-length attribute omitted `Length` and Studio Pro applied its own default of 200 (20× CE6621). `SetLength` is now always called.
- **Caller-provided entity `$ID` was overwritten in `CreateEntity`** — the modelsdk `entityToGen` generated a fresh `$ID`, so an association wired to `entity.ID` (e.g. the primitive-collection NPE association in an external-entity import) dangled and the project failed to *open* (`KeyNotFoundException`). The caller's ID is now honoured.
- **Lossless OData reads + `call external action` write** — `ListConsumedODataServices` didn't read `HttpConfiguration` (a re-modify dropped the `ServiceUrl`, CE5111); `ListPublishedODataServices` surfaced only entity-set counts (an ALTER stripped the entity tree → NullReference on load); and a microflow `call external action` serialized with a nil action (CE0008). All fixed.
- **Page allowed roles not read** (#722) — on the modelsdk engine `SHOW ACCESS ON PAGE` and the `SHOW SECURITY MATRIX` page section reported "no module roles" for a role-restricted page (a security-audit hazard); `pageFromGen` never populated `Page.AllowedRoles`. This also caused `mxcli lint` to false-fire CE0557/MPR007 ("home page has no allowed roles") after a `GRANT VIEW ON PAGE` that had in fact persisted correctly (#696).
- **Cross-module associations invisible to `LIST`/`DESCRIBE`** — associations to an entity in another module live in the gen `CrossAssociations` collection, which `domainModelFromGen` didn't read; `DESCRIBE MODULE … WITH ALL` also skipped them on both engines. Now surfaced.
- **`System` module associations omitted** — the virtual System domain model built platform entities/attributes but not the platform associations (`UserRoles`, `Session_User`, `Workflow_*`, …), so they were missing from `SHOW`/`LIST ASSOCIATIONS` and `DESCRIBE MODULE System` on the modelsdk engine.

#### OData import & publishing

- **Published OData service missing `EdmType` (CE5016) and `IsMany` (CE5022)** — mxcli never wrote the exposed attribute's EDM type nor the exposed association end's multiplicity, so `mx check` flagged every exposed attribute and association on the modelsdk engine. Both are now derived and serialized (the legacy engine omitted them too, but its field order let the checker recompute — so it wasn't a canonical reference).
- **Re-importing external entities duplicated navigation associations** — running an import twice suffixed every nav association (`Friends`/`Friends2`/`Friends3`…); a triple TripPin import inflated 8 → 25. Existing associations are now recognized and skipped.
- **Consumed-OData Headers microflow written to the wrong slot** (#728, CE6808) — on Mendix 11.10+ the configuration and headers microflows are distinct fields (`ConfigurationEntityMicroflow` vs `HeaderListMicroflow`); mapping both to the configuration slot made Studio Pro demand a `ConsumedODataConfiguration` return type. They're now tracked separately, and the `configurationMicroflow` storage key is version-gated (introduced 10.12, renamed 11.10) so a setting no longer no-ops on 11.10+.
- **Unannotated OData entity capabilities** — an entity set without `Capabilities` annotations now defaults to read-only (Creatable/Updatable/Deletable = false), matching how Mendix reads the metadata; a `true` default disagreed with services like TripPin RESTier (26× CE6630). Only an explicit annotation turns a capability on.
- **OData primitive-collection NPEs on Mendix <11.0** — `Rest$ODataMappedPrimitiveCollectionValue` and friends are an 11.0 type that doesn't exist in the 10.x type cache, so writing them aborted the whole project load (`TypeCacheUnknownTypeException`). Pre-11 imports now omit primitive-collection properties (as Studio Pro does), keeping the rest of the external-entity import intact.

#### Microflows, retrieves & describe

- **Memory vs database retrieves conflated** (#726) — a retrieve-by-association (in-memory) and a database retrieve with an equivalent reverse-association XPath were indistinguishable, and re-running a script silently converted the memory retrieve into a database one. `from $var/Assoc` is now always an `AssociationRetrieveSource` and a database retrieve always renders as `from Entity where …` — except the one genuine case (a reverse `Reference` with owner `both` consumed as a list, which Mendix resolves to a single object) which stays a database source to avoid CE0100.
- **Quoted association/attribute names in a microflow `SET`/`Change` corrupted the `.mpr`** — `SET $x/Module."Assoc" = $y` (which the "always quote identifiers" guidance encourages) passed `check` and `exec` but carried the quotes into the member identifier, so Studio Pro failed to load (`StorageLoadException: … is not a valid AttributeIdentifier`). The target is now normalized (quotes stripped) at parse time, with a defense-in-depth guard that errors loudly on any future quote leak.
- **`DESCRIBE MICROFLOW` timed out on high-complexity flows** (#710) — the duplicate-output-variable check enumerated every execution path (O(2^branches)), so a McCabe-44 flow crossed the 300s timeout. Replaced with an O(V·E) reachability analysis giving the identical answer (a 120-diamond flow now completes instantly).
- **`describe` showed a non-default-language placeholder** (#702) — text extraction returned the first `Texts$Text` entry (often a placeholder in another language) and the page title hardcoded `en_US`, so a `describe`→`create` round-trip could overwrite the real caption. Translations are now selected by project default language → `en_US` → first non-empty.

#### Pages, MOVE, lint & MCP

- **Pop-up page `PopupWidth`/`PopupHeight` = 0 (auto-size) rejected** (#713) — `0` is Studio Pro's own default for an auto-sized pop-up, but mxcli rejected it as "must be positive" and both writers coerced `≤0` → 600, so an auto-size pop-up couldn't be authored. Only a negative value is now rejected.
- **`MOVE ENTITY` left a stale reference in inline view-entity OQL** — Mendix <11 stores a view entity's OQL both on the source document and inline on the entity; the modelsdk engine rewrote only the former, so `mx check` on 10.x aborted with CE0174. The inline copy is now rewritten too. Related: `MOVE ENTITY`/`MOVE ENUMERATION` no longer print spurious "could not update … no such file" warnings from trying to disk-load the virtual System domain model.
- **`MPR001` rejected the Mendix `ENUM_` naming prefix** (#715) — the enumeration naming rule rejected `ENUM_ShippingStatus` (which the Mendix best-practice `CONV004` rule *requires*) and its suggestion mangled the name; the two rules could not both be satisfied. `MPR001` now accepts the optional `ENUM_` prefix and preserves it in suggestions.
- **Chained XPath predicates mis-stripped in lint** — `[a = 1][b = 2]` was stripped to `a = 1][b = 2`; the bracket-matching now walks depth.
- **Java action `Enumeration` parameter/return types serialized as entity references** (#680) — `Enumeration(Module.Enum)` / `ENUM Module.Enum` on a Java or JavaScript action passed `check` but MxBuild rejected the model ("The selected entity … no longer exists"). The explicit-enum syntax now emits `CodeActions$EnumerationType`; a bare `Module.Name` still resolves as an entity (indistinguishable at parse time).
- **Enum `Value = 'Caption'` gave a cryptic parse error** — MDL enum values are `'Value ''Caption'''` with no `=`; the ANTLR error read like a quoting problem. A targeted hint now points at the `=`.
- **Duplicate identical widget-reference diagnostics** — a page referencing the same target from more than one widget (e.g. New + per-row Edit buttons to the same edit page) now yields a single diagnostic.
- **`mxcli oql` returned 0 rows against a docker-deployed runtime** — the OQL preview servlet only mounts when `mendix.running.locally.by.studiopro=true` is set, which a deployed `bin/start` doesn't; the compose command now sets it (and forces sane runtime ports), and the OQL client surfaces the "Action not found" preview error instead of swallowing it as an empty result.
- **MCP: context data view missing its entity (CE0488)** — a data view bound to a page parameter authored over MCP wrote only the source variable, not `entityRef`; both are now written. MCP page authoring was also migrated from the removed `pg_write_page` to `pg_patch_page` for Studio Pro 11.12 (#697).
- **`GetRawUnit` on v1 MPR files** (Mendix < 10.18) (#705) — the UUID string was passed directly to SQLite instead of being converted to a GUID blob, so every lookup failed with "no rows in result set".

#### Report & lint performance

- **`mxcli report --format json` hung for tens of minutes on large projects** (#720) — six lint rules called `GetMicroflow(id)` per microflow, and on the modelsdk backend each call re-decoded *every* microflow unit (O(N²), millions of parses on a 3259-microflow project). A per-run `FullMicroflow` cache loads all microflows once; report dropped from >40 min to ~5 s.

## [0.13.0] - 2026-06-20

Headline: **the roundtrip codec engine is now the default.** Reads and writes route through the new `modelsdk` codec engine — a Go-native, roundtrip-safe metamodel codec spanning 53 domains — replacing the legacy `sdk/mpr` write path. Legacy remains available as an explicit `--engine legacy` (or `MXCLI_ENGINE=legacy`) fallback for the few constructs the codec can't yet reproduce (e.g. SOAP), and refuses an op rather than dropping data where it can't. This release also lands an experimental **MCP/PED backend** for authoring against a running Studio Pro.

> **A big thank-you to [engalar](https://github.com/engalar).** The roundtrip codec engine and the expression type-checker that anchor this release are built on his contributions — his `modelsdk` codec work (the 53-domain, roundtrip-safe metamodel implementation) and `exprcheck` port were cherry-picked and adapted here. Much of what makes v0.13.0 possible is his. Thank you!

### Added

- **Experimental MCP/PED backend (`mxcli mcp`)** — author Mendix models against a *running* Studio Pro over the Model Edit Protocol (PED/MCP) transport, instead of writing the `.mpr` on disk. `mxcli mcp capabilities` reports what the connected Studio Pro version supports (a version-keyed capability registry), and CREATE/ALTER ops are gated on that model — unsupported constructs are refused with an actionable message rather than silently dropped. Covers entity create/update with NOT NULL / UNIQUE validation rules, ALTER ENTITY ADD ATTRIBUTE, ALTER STYLING design properties, page authoring (typed params, edit-button actions, design properties), folder placement, and business-event/workflow reads. Honoured in both `exec` and the interactive REPL (`--mcp`). Verified against Studio Pro 11.11.

- **Graph community detection & centrality (`refresh catalog communities`)** — a pure-Go (no CGO, no deps) graph engine over the refs graph: Leiden community detection, Tarjan cycles, topological layering, PageRank, and betweenness. `refresh catalog communities [resolution n]` computes them (in the full-refresh transaction) into new catalog tables/views — `communities` + `community_summary`, `graph_cycles`, `graph_layers`, `graph_centrality`, `graph_integration_surface` (cross-community edges → OData/REST/event mechanisms), `graph_module_dependencies` — and adds PageRank/betweenness columns to `graph_god_nodes`. Surfaced via `SHOW COMMUNITIES` / `SHOW COMMUNITY [MEMBERS] OF Module.Asset`, and exposed to Starlark lint rules (`community_of`, `layer_of`, `cycles`, `module_dependencies`, `centrality`, `god_nodes`, `integration_surface`, `refs_from`) so teams validate their own architecture guidelines. The native Leiden matches the `leidenalg` reference exactly (105 communities on Evora). Targets two refactoring journeys: spaghetti → layered/modular (cycles + layer sequence numbers) and monolith → multi-app (community cut → integration-contract list).
- **`mxcli graph-report` — architecture map from the dependency graph** — renders six analyses over new `CATALOG.graph_*` views: god nodes (degree centrality), cross-module coupling ("surprise edges"), module cohesion (intra/inter ratio), dead documents (no inbound edge), the reference-kind distribution, and entity hotspots (used by the most flows). Framework/marketplace modules are excluded by default (`--include-framework` to keep them); `--top N`, `--format markdown|json`, `-o file`. Each section is a thin `SELECT` over a `graph_*` view, so it's reproducible directly (`select * from CATALOG.graph_god_nodes`). Built on the now-substantially-complete `refs` graph; requires `refresh catalog full` (the command runs it). Also made `CATALOG.<name>` query translation generic (regex strip) so new catalog views work without a per-name allowlist.
- **Marketplace download & install** — the content API now returns a per-version `downloadUrl`, so the previously-parked install path is unblocked. `mxcli marketplace download <id> [--version X] [-o file]` fetches a content version's `.mpk` (two-step: MxToken-authed `303` on `marketplace.mendix.com` → public CDN, no token sent to the CDN). `mxcli marketplace install <id> -p app.mpr` is type-aware: widgets are copied into `widgets/`, new modules are imported via `mx module-import`, other types are downloaded with import instructions. Module **updates** are intentionally reported-not-applied — re-importing an existing module would discard local edits and change persistent-entity IDs (data loss); that path is left to Studio Pro pending an ID-preserving merge
- **Marketplace search caching** — the first `mxcli marketplace search` fetches the full catalog listing once and caches it under `~/.mxcli/marketplace-catalog-<profile>.json` (24h TTL, mode 0600); subsequent searches (any keyword) are served from the cache instantly. `--refresh` bypasses the cache and re-fetches. An interactive progress line ("Searching marketplace… N items scanned") shows during a fresh scan
- **`describe` auto-detects the document type** — the type is now optional for a qualified name: `mxcli describe MyModule.Customer` resolves the type itself (entity, microflow, page, snippet, enumeration, constant, java action, nanoflow, workflow, association incl. cross-module, …). Resolution prefers the catalog cache (O(1) lookup, no overhead vs. the explicit form) and falls back to a live project scan when the catalog is absent. An ambiguous name (e.g. an entity and a microflow sharing a name) is reported with its candidates. The explicit `describe <type> <name>` form is unchanged, and is still required for the forms that have no single qualified name (module, settings, navigation, module role)
- **Bare `describe Module.Name` works as MDL, not just as a CLI flag** — the auto-detect form is now part of the MDL grammar, so it parses and runs everywhere MDL does: the REPL, `exec` scripts, `check`, and the LSP (previously `describe Sales.Order` in the REPL was a parse error and only `mxcli describe Sales.Order` worked). The bare form resolves the type from the project's catalog `objects` index at execution time (built on demand, fresh — no staleness concern); all typed `describe <type> …` forms still take precedence, and an ambiguous or unknown name returns an actionable error
- **Pop-up page geometry** — `CREATE PAGE` and `ALTER PAGE` can now set a pop-up page's `width`, `height`, and `resizable` in the page header (#661). `DESCRIBE PAGE` round-trips them.
- **Compound (nested) design properties** — design properties on pages and snippets that nest (a group containing sub-properties) are now written and round-tripped by `DESCRIBE PAGE`/`DESCRIBE STYLING`, on both the codec engine and over MCP (#668)
- **Quoted identifiers in member lists and attribute refs** — names that collide with MDL reserved words can now be quoted in member lists and attribute references (#675), extending the reserved-word-quoting support to more positions (`DESCRIBE` emitters now quote reserved-word names in the remaining strict-identifier spots, #619)

### Changed

- **The codec engine (`modelsdk`) is the default; `sdk/mpr` is the explicit fallback** — all reads and writes now route through the roundtrip codec engine by default. The legacy path is reachable via `--engine legacy` or `MXCLI_ENGINE=legacy` for the constructs the codec can't yet reproduce (notably SOAP); where the codec path can't reproduce a construct it refuses the op rather than dropping data. This is the culmination of the Issue 7 parity effort that brought every document type — domain models, microflows, pages, workflows, security, REST/OData, agent-editor docs, settings, and more — to `mx check` parity on the codec path.

- **Catalog `objects` index includes associations** — the unified `objects` view now unions the `associations` table (`ObjectType = ASSOCIATION`), so it is a complete index for the cataloged document types and consumers no longer need a separate associations query. Catalog schema bumped to v3; cached `.mxcli/catalog.db` files rebuild automatically on the next `refresh catalog`.
- **Catalog indexes image collections, JavaScript actions, and data transformers** — these document types had no catalog table at all; they are now built (via the raw-unit surface, so no `CatalogReader`/backend change) into their own tables and unioned into `objects` (`IMAGE_COLLECTION`, `JAVASCRIPT_ACTION`, `DATA_TRANSFORMER`). `describe` auto-detect resolves image collections and data transformers by bare name. Catalog schema bumped to v4.
- **Catalog indexes agent-editor documents** — agents, AI models, knowledge bases, and consumed MCP services (one shared `CustomBlobDocuments$CustomBlobDocument` BSON wrapper, distinguished by `CustomDocumentType`) are now cataloged into their own tables and unioned into `objects` (`AGENT`, `AI_MODEL`, `KNOWLEDGE_BASE`, `CONSUMED_MCP_SERVICE`). The document name turned out to be a top-level wrapper field (not buried in the inner JSON blob), so this reads through the raw-unit surface with no `CatalogReader`/backend change, and `describe` auto-detect resolves all four by bare name. Catalog schema bumped to v5; this completes the `objects` index for the document types tracked in #658. (Verified against `test3-app`: 8 agent-editor docs across all four types.)

### Fixed

- **Page authoring fidelity** — several page constructs that were silently dropped or mis-stored are fixed: `DYNAMICTEXT` Attribute bindings are no longer dropped (#650); `ALTER PAGE` can set conditional `Visible`/`Editable` expressions without tripping CE0117 (#627); a ComboBox datasource property that was silently dropped is now caught at check time (#643); a quoted `where '<xpath>'` constraint is no longer mis-stored as CE0161 (#642); and gallery `DesktopColumns` + `class` are honoured on pluggable widgets.
- **`check` catches more page errors** — forward widget→page references (#674) and invalid static widget values (#672, #673) are now flagged at check time instead of surfacing later in Studio Pro.
- **`DROP MODULE` removes Java/JavaScript source directories** — dropping a module now also deletes its orphaned `javasource`/`javascriptsource` directories, on both engines.
- **Docker libSkiaSharp crash auto-handled** — `mxcli docker` auto-preloads the system libfreetype so the bundled `mx` no longer aborts with the `FT_Get_BDF_Property` symbol-lookup error, and reports a clear message when an M2EE call hits a stopped container.
- **`show context` now resolves its relationship sections** — the sections filtered the refs table on `TargetType`/`SourceType` using lowercase literals (`'entity'`, `'microflow'`, `'page'`) while those values are stored uppercase, so in case-sensitive SQLite "Entities Used", "Microflows Using This Entity", "Pages Displaying This Entity", "Related Entities" and the workflow context sections silently rendered empty. Now matched correctly. (`show callers|callees|references|impact` were unaffected and already pick up the expanded refs automatically.)
- **`catalog.refs` captures far more references** — the cross-reference index that powers `show callers|callees|references|impact` was missing whole categories (#663). Now added: nanoflow calls, consumed-REST-operation calls, and association-based retrieves from microflow actions; **nanoflows as reference sources** (previously only microflows were walked); **association references** (each association now links to both its FROM and TO entity — was an explicit `// Skipping for now` TODO); **page→layout references** (the emission was dead code gated behind an always-nil `LayoutCall`); **calculated-by** (entity→microflow for calculated attributes); **change/delete entity references** (resolved via lightweight intra-flow variable tracking); and **page- and snippet-widget references** — `datasource` (page/snippet→entity) and `action` (page/snippet→microflow/nanoflow), extracted from the existing raw-BSON widget walk and projected from `widgets_data` (new `MicroflowRef`/`NanoflowRef` columns; snippet widgets now populate the table too, with `ContainerType=SNIPPET`). On `MxGraphStudioDemo` the earlier slice took `associate` 0→104, `layout` 0→22; on `Evora-FactoryManagement` the full effort took `refs` from ~5.5k to 6,459. Re-run `refresh catalog full` to pick them up.
- **`catalog.activities` labels REST and other actions correctly instead of a generic `MicroflowAction`** — the `ActionType` column came from a hand-maintained type switch that only knew ~17 action types; every other parser-modelled action (REST call, REST operation call, web-service call, nanoflow call, JavaScript-action call, execute-database-query, transform-JSON, XML import/export, show-home-page, delete-object) silently collapsed into `ActionType = 'MicroflowAction'`, so e.g. `select … from CATALOG.activities where ActionType = 'RestCallAction'` returned nothing. The label is now derived from the concrete action type, so it stays correct for every action the parser models (including ones added later), and an action the parser doesn't model yet surfaces its real Mendix storage name rather than a generic bucket. On `MxGraphStudioDemo` the generic bucket dropped from 33 rows to 0, exposing RestCallAction/RestOperationCallAction/JavaScriptActionCallAction/NanoflowCallAction/DeleteObjectAction that were previously hidden. Re-run `refresh catalog full` to pick up the corrected labels.
- **`SHOW CATALOG TABLES` lists every catalog view** — the table list was hand-maintained and had drifted: the newly-cataloged document-type views (image collections, JavaScript actions, data transformers, agents, AI models, knowledge bases, consumed MCP services) and the pre-existing `navigation_profiles` view were all built and queryable but never shown. They are now listed, and a drift-guard test (`TestTables_CoversAllViews`) asserts every catalog VIEW appears in the list, so a future document type can't be silently omitted again.
- **`refresh catalog source` no longer O(N²) on large projects** — it resolved each document by re-reading and re-`bson.Unmarshal`ing *every* unit on *every* describe call, so a big app (#651: ~3.3k microflows, ~33k activities) took ~6 hours. The reader now builds a one-time `$Type + qualified-name → unit` index (decoding only the `Name` field, not the whole document), making `GetRawUnitByName` / `GetRawMicroflowByName` O(1); the shared backend means the index is built once across the parallel describe workers. The source phase also reports incremental progress every 2s instead of going silent for the whole build. GraphViewer's source build (993 microflows) dropped to ~3.5 min with live progress; cloud-portal-scale projects go from hours to minutes
- **Marketplace search now scans the whole catalog** — the Content API has no server-side search and caps `limit` at 100 per page, so `marketplace search` previously only filtered the first 100 items and silently missed matches further in (e.g. External Database Connector `219862`, Mendix Business Events `202649`). It now paginates via `offset`, fetching pages **concurrently** (first page alone so a common early match stays a single request; then bounded-parallel batches), and stops at `--limit` matches or end-of-catalog. Measured ~3m45s → ~44s on a slow link for a deep match; combined with the new cache, repeat searches are instant

## [0.12.0] - 2026-06-04

Headline: **one widget creation path.** The `datagrid`/`gallery`/`combobox`/`image` keywords and the `pluggablewidget '...'` form now build BSON through a single registry-driven engine, fed by widget definitions extracted from each project's installed `.mpk` files (`widget init`; auto-generated/refreshed on `exec`). The Mendix BSON *envelope* still comes from embedded `mendix-11.6` templates — full per-version, project-extracted templates remain tracked under #529.

### Added

- **Cross-version widget-envelope drift gate** — `make check-widget-versions` (script `scripts/check-widget-versions.sh`) runs a widget fixture through `exec` + `mx check` on multiple Mendix versions and fails if the CE0463 set differs between them (v0.12.0 Stream A). It drops each fixture's `create module` targets before exec so leftover/divergent reference-project state doesn't skew the comparison; the 11.10 libSkiaSharp crash is handled automatically via `scripts/mx-check.sh`. Fixture set: `03`, `30`, `31`, `32`. The gate surfaced one real 11.9→11.10 drift (textfilter `attrChoice`, #605, fixed above); after that fix all four fixtures pass with no cross-version drift

### Security

- **Go toolchain pinned to 1.26.4** — resolves two reachable standard-library vulnerabilities flagged by `govulncheck`: GO-2026-5039 (`net/textproto`, unescaped inputs in errors; reached via `mpk.ParseMPK`) and GO-2026-5037 (`crypto/x509`, inefficient candidate hostname parsing). `go.mod` now carries a `toolchain go1.26.4` directive and CI pins `go-version: '1.26.4'`, so every environment builds with the fixed stdlib

### Changed

- **ALTER STYLING design properties** — `ALTER STYLING` now writes design properties on pages and snippets, with correct value-type encoding (Option vs Custom; ToggleButtonGroup uses Option). `DESCRIBE STYLING` round-trips them. (#631)
- **Dependency bumps** — `chroma/v2` 2.24.1 → 2.26.1, `modernc.org/sqlite` 1.50.1 → 1.51.0, `mattn/go-runewidth` 0.0.23 → 0.0.24
- **DataGrid construction unified on the pluggable widget engine** — the `datagrid` MDL keyword now routes through the same registry-driven engine as the `pluggablewidget 'com.mendix.widget.web.datagrid.Datagrid'` form, so both produce equivalent BSON. The hand-coded keyword-path builder (`datagrid_builder.go` `BuildDataGrid2Widget` + ~30 helpers, ~990 lines) is deleted. Engine gained the column conventions the keyword path applied implicitly: CONTROLBAR→filtersPlaceholder routing, per-column filter-widget routing (`textfilter`/`numberfilter`/`datefilter`/`dropdownfilter`), object-list item property ordering, `Caption`/`Content` aliases with `CaptionParams`/`ContentParams` resolution, missing-Caption→attribute-name fallback, attribute-less columns default `sortable=false`, content-slot widgets auto-infer `ShowContentAs: customContent`, and the tooltip/exportValue empty-ClientTemplate conventions. (#529 Phase 4)
- **Catalog schema normalized** — every domain table (entities, microflows, pages, …) is now split into a `<name>_data` storage table plus a `<name>` view that joins `snapshots` to expose `ProjectName`, `SnapshotDate`, `SnapshotSource`, `SourceId`, `SourceBranch`, `SourceRevision`. Existing queries (`SELECT * FROM CATALOG.ENTITIES`, ad-hoc filters by `SnapshotSource`, the `objects` UNION view) keep working unchanged. Existing `.mxcli/catalog.db` files rebuild automatically on first open (schema version bumped to 2); cache metadata is cleared so the rebuild fires through `isCacheValid`. (#576)

### Fixed

- **DESCRIBE round-trips for pages and widgets** — `DESCRIBE` now emits re-executable MDL for several cases that previously broke a roundtrip: bare grant member names (#633), `microflow`/`nanoflow` (not `call_`) for widget actions (#634), an always-present java-action body (#637), quoted reserved-keyword DataGrid column names (#638), and widget-action microflow arguments as `Param: value` (#640)
- **Reserved-keyword names via quoting** — page/snippet parameter names (#114) and widget names (#619) that collide with MDL reserved words can now be expressed with quoting instead of being rejected
- **OQL against Mendix 11.11** — `mxcli oql` supports the new `/dev/preview_execute_oql` dev endpoint and surfaces its query errors (which arrive as HTTP 200 with an `{"error": ...}` body) instead of silently succeeding
- Filter widgets (`textfilter`/`numberfilter`/`datefilter`/`dropdownfilter`) with an explicit `attributes: [...]` list now emit `attrChoice="linked"` instead of `"auto"` (#605). `"auto"` is correct only for a *bare* filter inside a DataGrid column (it binds to the column's attribute); a filter with an explicit attributes list (e.g. inside a Gallery `filter` block) needs `"linked"`. Mendix 11.10+ flags `attrChoice="auto"` alongside a populated attributes list as definition drift (CE0463); Mendix 11.9 tolerated it. This was real 11.9→11.10 envelope drift that the v0.12.0 Stream A gate missed because its fixtures only exercised column-bound filters
- DataGrid column `ColumnWidth: manual` is honoured again — the Stream B engine consolidation dropped the keyword path's `ColumnWidth` → schema `width` mapping, leaving width at its `autoFill` default. A `Size:` value is only valid when `width=manual`, so under autoFill Studio Pro / `mx check` flagged CE0463. Restored as a `width ← ColumnWidth` column alias (caught by the new cross-version gate as `dgDyn` in fixture 31)
- Pluggable widget property conditional visibility (#574) — a TextTemplate property hidden by the widget's `editorConfig.js` under the current configuration now emits `TextTemplate: null` instead of the template's populated default, eliminating CE0463 ("the definition of this widget has changed"). Phase 1 hand-authors rules for VideoPlayer (`videoUrl`/`posterUrl` hidden when `type=expression`) and Timeline (`title`/`description`/`timeIndication` hidden when `customVisualization=true`); rules live in each widget's `.def.json` as a `propertyVisibility[]` block and ride the `generatorVersion` auto-refresh
- `mxcli exec` now generates **missing** widget definitions, not just refreshes stale ones — a project that has `.mpk` widgets installed but was never `widget init`-ed (no `.mxcli/widgets/`) previously failed the first widget build with "unsupported widget type: datagrid". `exec` now extracts the defs from the installed `.mpk` files on demand (matching `refresh catalog`), so it works without a separate `widget init` step
- Stale project-local widget definitions self-heal — `.def.json` files carry a `generatorVersion` stamp, and `mxcli exec` re-extracts any definition generated by an older engine before building widgets. Projects whose `.mxcli/widgets/` was generated before the v0.12.0 widget changes no longer emit CE0463 ("widget definition changed") on the next run without a manual `widget init --force`
- DataGrid filter widgets (`textfilter`/`numberfilter`/`datefilter`/`dropdownfilter`) default `attrChoice` to `auto` instead of `linked`/`custom`, so a filter placed inside a column body binds to the column's attribute automatically rather than failing `mx check` with CE0642 ("Property 'Attribute' is required")

## [0.11.0] - 2026-05-21

### Added

- **Pluggable widget property validation** — `mxcli check` flags unknown widget property keys as `MDL-WIDGET01`; respects MDL builtin property names (e.g. `Label`, `Caption`, `DataSource`) so they aren't reported as typos
- **`mxcli check --post-migration`** — scans for legacy native widgets in pages/snippets and reports `MDL-WIDGET02` with qualified module.document names; version-gated via the legacy-widget catalog
- **LSP widget integration** — completion suggests widget property keys inside `(...)` blocks; hover shows property descriptions extracted from `.mpk`; widget property typos surface as real-time diagnostics
- **Widget definition workflow** — `widget init --force` re-extracts existing `.def.json` files; `widget init` and `refresh catalog` auto-refresh stale definitions; `mxcli init` now runs `widget init` so new projects pick up widget defs automatically
- **Catalog: widget tables** — `widget_definitions` and `widget_definition_properties` queryable via `SELECT ... FROM CATALOG.widget_definitions`
- **`ALTER` for agent-editor documents** — `ALTER AGENT/MODEL/KNOWLEDGEBASE/MCPSERVICE` (#464)
- **Skill docs include MDL keyword routing** — generated widget skill files document object-list and child-slot keywords driven from `.def.json`

### Fixed

- DataGrid2 column `tooltip` / `exportValue` / `dynamicText` TextTemplate now matches Studio Pro's per-column-kind convention (CE0463 on attribute-bound columns, #578)
- DataGrid2 column `CaptionParams` / `ContentParams` / `ShowContentAs` / `Content` roundtrip (#547); `$localVar` references in column captions emit `Forms$PageVariable.LocalVariable`
- Pluggable widget engine wrote `CustomWidgets$AttributeRef` (not a registered Mendix type); now emits `DomainModels$AttributeRef` with the fully-qualified path so `mx update-widgets` no longer fails with `TypeCacheUnknownTypeException` (#64)
- Object-list item TextTemplate slots emit `null` when unset (Accordion `groups`, Maps `markers`, AreaChart `series`, PopupMenu items) instead of placeholder ClientTemplate that triggered CE0463 (#548)
- Pluggable widget CE0463 on Mendix 11.9 — `FormattingInfo TimeFormat` + `Selection` PascalCase normalization
- DataView `FormOrientation` / `LabelWidth` now controllable from MDL (#554)
- `ALTER PAGE` fixes: `INSERT`/`REPLACE` serializes DataGrid2 columns correctly; `set Title` actually updates the title (#561); column `SET` is case-insensitive and supports TextTemplate captions (#560); column inserts use the grid's data source as entity context
- Master-detail page round-trip — Gallery `ItemSelectionMode` + DataView selection-source described correctly
- `DesktopWidth` / `TabletWidth` / `PhoneWidth`: `AutoFill` now actually sets `Weight: -1` instead of dropping the override
- Pluggable widget validator respects MDL builtin property names (no false positives on `Label:`, `Caption:`, `DataSource:`, etc.)
- `mxcli check` detects custom-content column INSERT issues before MxBuild
- `--references` no longer flags `DROP + CREATE` of the same name as a conflict
- Reject Mendix reserved words on non-persistent entity attributes (#552)
- Cached catalog applies the current schema on load (no more "no such table" after schema bump)
- Nightly CE0117 on Mendix 10.24.19 — drop redundant `toString()` on string parameter

### Changed

- Test infrastructure: `TestMain` runs `widget init` on the shared source project so per-test copies inherit `.def.json` files; integration tests now exercise pluggable widget fixtures end-to-end
- Robust cleanup for doctype/mx-check tests eliminates ENOTEMPTY flake on CI
- `modernc.org/sqlite` bumped from 1.50.0 to 1.50.1

### Known limitations

- Two CE0463 cases remain for widgets with property-conditional TextTemplate visibility (VideoPlayer with `type='expression'`, Timeline with `customVisualization='true'`). Root cause and proposal in `docs/11-proposals/PROPOSAL_widget_property_visibility.md`; tracked under #574
- `pluggablewidget 'com.mendix.widget.web.datagrid.Datagrid'` form is less feature-complete than the `datagrid` keyword form (no CONTROLBAR/customContent/per-column filter routing). Tracked under #529 Phase 4

## [0.10.0] - 2026-05-12

### Added

- **Maven/JAR dependency management** — `CREATE/DROP/SHOW JAR DEPENDENCY` statements; `jar_dependencies` catalog table; skill and docs-site pages (MDL-JARDEP)
- **Object-list pluggable widget properties** — grammar keywords for object-list blocks, extraction to `.def.json` (Phase 1), and BSON serialization through the executor (Phase 1 Layer 3)
- **LEGACYDATAGRID grammar** — keyword dispatch table and `LEGACYDATAGRID` grammar rule (Phase 2 pluggable widget overhaul)
- **`AllowCreateChangeLocally` flag** — exposed on external OData entities (#534)
- **Catalog: contract_entities → external_entities link** — cross-reference between contract catalog and integration catalog
- **`not(expr)` grammar enforcement** — grammar now requires parenthesised form; bare `not expr` rejected with CE0117 diagnostic

### Fixed

- `mxcli fmt` exits 1 on unparseable input and pipes describe output correctly (#398)
- ALTER SNIPPET failing with "page not found" (#402)
- `SHOW CONTEXT OF` entity showing empty definition (#396)
- `CREATE ENTITY` rejects unknown attribute type names (#392)
- `CREATE ENUMERATION` rejects duplicate value names (#390)
- `DROP ENUMERATION` errors on ambiguous unqualified name (#391)
- `CREATE ASSOCIATION` rejects duplicates for cross-module associations (#389)
- `GRANT/REVOKE ON ENTITY` validates module roles (#399)
- Enum XPath comparisons stored as string literals instead of enum refs (#176)
- Catalog crash on duplicate OData contract entities/actions
- `CATALOG.JAR_DEPENDENCIES` missing from `Tables()` list
- Three nightly CI failures on Mendix 10.24
- DataGrid2 `WidgetObject` boolean defaults aligned with `PropertyType` schema
- `TextTemplate` translation defaults populated; `Editable=Always` set on filters
- Required `CustomWidget` envelope fields added to filter widgets
- `WidgetObject Properties` reordered to match `WidgetType PropertyTypes` order
- `AllowUpload` field added to `WidgetValueType` BSON (closes one CE0463 gap)
- Unique placeholder IDs for `TextTemplate` translations (#30)
- Two ALTER PAGE bugs caught in test feedback
- ComboBox CE0463 — guard auto-populate and null `selectAllButtonCaption`
- Grammar added as explicit dependency of `build`, `test`, and `release` targets

## [0.9.0] - 2026-05-08

### Added

- **Inheritance split and cast** — `CASE $var IS Module.SubType THEN ... END CASE` and `CAST $var AS Module.SubType` statements in microflow/nanoflow bodies; full BSON roundtrip with branch anchors, nested continuation cases, and merge emission (CE0079)
- **CREATE OR MODIFY** — Standardised `OR MODIFY` variant across all remaining document types so scripts are idempotent by default (#510)
- **MDL-DUPDEF** — `mxcli check` detects duplicate `CREATE` for the same qualified name and reports `MDL-DUPDEF`

### Fixed

- Catalog crash on duplicate business event channels (#533)
- `flowRefCollector` skipping EnumSplitStmt case and else bodies — impacted `show callers/callees` accuracy
- CE0079: inheritance split branches that continue after the split were missing their merge node
- Nested `traverseFlowUntilMerge` guard could cross an outer merge boundary (#528)
- Inheritance split: branch anchors, case order, nested continuation tails, and nodes outside cases all preserved
- List-typed Java action arguments not emitting the `empty` keyword (#521); broadened to cover all resolved `BasicParameterType` params
- REST mapping cardinality not roundtripping — `as list of` syntax now parsed and emitted (#519)
- Import mapping: `MinOccurs`/`MaxOccurs` not parsed on mapping elements; repeating Object root treated as list; `SingleObject` inferred when `JsonStructure` absent
- Microflow layout: spacing, branch heights, and loop containment improved
- `TEXTFILTER` inside `DATAGRID COLUMN` not wired to the column filter slot (#189)
- `SET $obj/Assoc` path target rejected and produced wrong BSON (#511)
- `SHOW WIDGETS WHERE … LIKE` silently degraded to equality match
- Reserved OData attribute names not renamed when importing entities (#526)
- Virtual `System.*` Java actions missing from `ListJavaActions` and catalog
- `ConcurrencyMode=Fixed` incorrectly marked as Creatable during OData import (#525)
- Reverse-Reference traversal through entity inheritance misclassified
- `mxcli check --references` reporting false positives on `System.*` references (#523)
- ANTLR4 version unpinned in CI caused flaky Maven Central lookup failures

### Changed

- Generated ANTLR parser removed from git; `make grammar` step added to CI (#514)
- `MDLParser.g4` split into domain-specific grammar files for maintainability (#515)

## [0.8.0] - 2026-05-05

### Added

- **CREATE/DROP NANOFLOW** — Full nanoflow authoring pipeline: grammar, AST, visitor, executor, BSON writer, CALL NANOFLOW statement, GRANT/REVOKE nanoflow access, and nanoflow ELK diagram support in VS Code preview
- **CALL JAVASCRIPT ACTION** — `call javascript action Module.ActionName(params)` fully supported in CREATE NANOFLOW/MICROFLOW bodies: grammar, parser, builder, serializer, and roundtrip
- **CASE/WHEN enum split** — Enum-value split statements with `CASE $var WHEN Module.Value THEN ... END CASE` syntax; replaces the earlier `split on enum` draft
- **CALL WEB SERVICE (SOAP)** — Legacy SOAP microflow call statement with unsupported-detail preservation as raw BSON
- **RENAME WORKFLOW / RENAME MODULE** — RENAME now covers workflows and modules with reference refactoring
- **Ellipsis placeholder expression** — `...` as a placeholder in microflow expressions
- **Add-to-list expressions** — `add expression to $list` syntax in microflow/nanoflow bodies
- **Free microflow annotations** — Unattached `@annotation` nodes in microflow bodies survive describe → exec round-trip
- **@anchor sequence flow annotation** — `@anchor(from: X, to: Y)` on microflow statements pins SequenceFlow attachment sides; split and loop forms supported; builder-default and layout-equivalent anchors suppressed from DESCRIBE output
- **OpenAPI import for REST clients** — `CREATE REST CLIENT` accepts `OpenAPI: 'path/or/url'` to auto-generate a consumed REST service from an OpenAPI 3.0 spec (#207)
- **DESCRIBE CONTRACT OPERATION FROM OPENAPI** — Preview OpenAPI-generated operations without writing to the project
- **mxcli catalog search** — Search Mendix Catalog for data sources and services (#213)
- **Local file metadata for OData clients** — `CREATE ODATA CLIENT` supports `file://` URLs and relative paths for `MetadataUrl` (#206)
- **CATALOG.ASSOCIATIONS table** — Query association metadata via `select ... from CATALOG.ASSOCIATIONS` (#419)
- **SET format = json** — Session-level `SET key = value` command; `SET format = json` applies to all subsequent output
- **Java action improvements** — DROP/RENAME updates source file references; `void` qualified name resolved as VoidType; explicit void returns parsed correctly
- **SHOW LANGUAGES** — Language listing with Languages array parsing and executor handler (#480)
- **VS Code extension** — LSP coverage extended to all document types (nanoflows, workflows, Java actions, JSON structures, import/export mappings)
- **LSP snippet completions** — `CREATE NANOFLOW`, `CALL MICROFLOW`, `CALL NANOFLOW`, `CALL JAVASCRIPT ACTION`, `CALL JAVA ACTION` snippets added
- **make check-mdl** — Fast doctype script syntax validation target; wired into CI
- **Nanoflow diff support** — `mxcli diff` detects and displays nanoflow changes
- **Nanoflow validation parity** — `mxcli check` runs full body validation on nanoflows via shared `validateFlowBody` helper

### Fixed

- SIGSEGV in `buildPublishedRestResourceDef` on malformed REST syntax (#429)
- nil panic in ALTER WORKFLOW when activity ref is missing or uses a keyword (#430)
- Single quotes not escaped in DESCRIBE ENTITY XPath output (#431)
- `diff-local` git-error propagation and regression tests (#424)
- DataGrid2 column name derivation for ALTER PAGE (#116)
- O(N²) `GetMicroflow`/`GetNanoflow` replaced with direct unit lookup (#397)
- `CALL MICROFLOW`/`CALL NANOFLOW` validates targets exist before writing model (#395)
- `mxcli new` exits 0 on download failure (#422)
- Reject obviously malformed `MetadataUrl` in CREATE ODATA CLIENT (#427)
- Rename commands reject collisions with existing names (#432)
- Exit codes and error messages for marketplace, eval list, widget init, TUI (#425)
- `connect`/`disconnect`/`status` registered in syntax registry (#441)
- `resolveSnippetRef` checks session cache before querying backend (#509)
- DESCRIBE WORKFLOW output was missing the `CREATE` keyword (#478)
- RENAME MODULE failed due to uppercase ObjectType comparison in visitor (#473)
- JSON structure qualified-name lookup through folder hierarchy (#508)
- Retry-style error handler tail now loops back to a merge before the source (#507)
- Cross-module associations preserved on CREATE object actions (#502)
- Negative annotation coordinates parsed correctly (#494)
- Multiple retrieve XPath predicates preserved (#500)
- Custom error handler routing, empty else branch preservation, and structured conditional emit (#366)
- Validation feedback targets preserved with fully-qualified association paths (#359)
- Mapping result range cardinality and explicit REST mapping output variables (#372)
- SNIPPETCALL on parameterised snippets no longer corrupts model
- SHOW_PAGE button actions no longer produce null `PageParameterMapping.Variable` (#295)
- `Forms$SnippetParameterMapping` used for snippet call parameter mappings
- Marketplace search applies client-side filtering (#479)
- Recursion depth limit added to EXECUTE SCRIPT (#472)
- `CATALOG.ASSOCIATIONS`/`CONSTANTS`/`OBJECTS` returning no rows (#419)

### Changed

- **MDL string literal escapes** — `\n`, `\r`, `\t`, `\\` inside single-quoted literals are now escape sequences. Scripts embedding raw backslash sequences must double the backslash.
- **CatalogDB/CatalogTx interfaces** — Catalog, Builder, and LintContext migrated to interface; SQLite implementation extracted to `catalogdb_sqlite.go`
- **LintReader interface** — `sdk/mpr` removed from linter and executor; all reads go through `LintReader`
- **Type-safe BSON helpers** — `bsonString`/`bsonBool` consolidated in `mdl/bsonutil` package

## [0.7.0] - 2026-04-21

### Added

- **Agent Editor** — CREATE/DROP Agent, Knowledge Base, Consumed MCP Service, and Model documents; read support for all four types; DESCRIBE MODULE WITH ALL includes agent-editor documents
- **Consumed REST Client v2** — Redesigned syntax with full mapping support, parameter support for SEND REST REQUEST, BODY JSON FROM clause roundtrip, and TRANSFORM microflow action (JSLT/XSLT, Mendix 11.9+)
- **Platform Authentication** — `mxcli auth login/logout/status/list` with PAT scheme for mendix.com; credentials stored at `~/.mxcli/auth.json` (mode 0600), MENDIX_PAT env override
- **Marketplace Browsing** — `mxcli marketplace search/info/versions` with `--min-mendix` compatibility filtering
- **Entity Event Handlers** — Full MDL support for before/after create/change/delete event handlers with entity parameter validation
- **System Attributes** — AutoOwner, AutoChangedBy, and other audit pseudo-types; ALTER ENTITY ADD/DROP ATTRIBUTE for system attributes
- **ALTER PUBLISHED REST SERVICE** — Full in-place modification of published REST services (#161)
- **GRANT/REVOKE ACCESS on PUBLISHED REST SERVICE** (#162)
- **GitHub Copilot support** — First-class Copilot integration in `mxcli init`
- **Unified --json output** — All commands support structured JSON output (#134); `mxcli check --format json/sarif` outputs structured results
- **OData TripPin bulk-import** — Executable bulk-import example with @Constant syntax for ServiceUrl
- **Backend Abstraction** — `ExecContext` with typed backend interfaces, dispatch registry replacing type-switch, mutation backends (`page_mutator`, `widget_builder`, `datagrid_builder`, `workflow_mutator`) decoupled from `sdk/mpr`
- **mdl/types package** — Shared types and utilities extracted from `sdk/mpr` (EDMX, AsyncAPI, ID, navigation, infrastructure, JSON utils)
- **bsonutil package** — BSON utility functions (IDToBsonBinary, BsonBinaryToID, NewIDBsonBinary)
- **Mock-based handler tests** — 189 tests across 33 files covering all executor command handlers
- **OperationRegistry extensibility** — Pluggable operation registry with ContainerSnippet constant

### Fixed

- REST client BASIC auth uses correct `Rest$ConstantValue` BSON key (#200)
- ConnectionIndex lost on roundtrip (int64 vs int32 type mismatch) (#204)
- OData: ByAssociation DataSource serialization for DataGrid 2, capability annotations for entity/association CRUD (#201), bulk-create NPEs for primitive collections, derived/abstract/contained entities, and navigation associations (#143)
- UUID v4 version/variant bits in `GenerateDeterministicID`; panic on invalid UUID in `IDToBsonBinary`
- Cascade-delete associations on DROP ENTITY and DROP ODATA CLIENT
- Reserved keywords now allowed as module names in CREATE MODULE
- Quoted identifiers accepted in CREATE MODULE
- Find, Filter, ListRange list operations parsed and rendered (#212)
- DESCRIBE REST CLIENT resolves constant credentials to literal values (#192)
- DESCRIBE microflow roundtrip issues; eliminate redundant Merge nodes when IF branch returns
- COLUMN name falls back to attribute + scope association lookup by module (#202)
- Schema-level external `<Annotations>` blocks parsed in OData $metadata
- OData ServiceUrl validated as constant reference
- Agent-editor commands conformed to backend abstraction

### Changed

- Executor fully decoupled from storage layer — all BSON writes go through mutation backends (PRs #225, #237, #238, #239)
- All executor handlers migrated to free functions using `ExecContext` (removed 233 unused wrapper methods)
- `show*` executor functions renamed to `list*` for consistency
- Type aliases added in `sdk/mpr` for backward compatibility after shared-type extraction

## [0.6.0] - 2026-04-09

### Added

- **RENAME** — Automatic reference refactoring when renaming entities, attributes, associations, and other elements
- **CREATE EXTERNAL ENTITIES** — Bulk import entities from OData contracts (#143)
- **@excluded Annotation** — Mark documents and microflow activities as excluded, with Excluded column in catalog and `[EXCLUDED]` indicator in LIST
- **LIST Alias** — LIST as alias for SHOW in MDL and CLI
- **ALTER WORKFLOW** — Full activity manipulation (INSERT, DROP, REPLACE) for workflow definitions
- **Primitive Page Parameters** — Support for String, Integer, and other primitive types in page parameters
- **DataGrid Column Targeting** — Addressable columns in ALTER PAGE via dotted refs (e.g., `DataGrid.ColumnName`)
- **diff-local --ref** — Accept git ranges directly via `--ref` for comparing arbitrary revisions
- **Virtual System Module** — Complete module listing including System module
- **PasswordPolicy.ValidatePassword** — Demo user password validation against project policy
- **Multiple XPath Predicates** — Support `[cond1][cond2]` in WHERE clauses
- **DESCRIBE Enhancements** — Missing types added to mxcli describe command, view entity Source object preservation
- **Proposals** — Bulk external action support from OData contracts, RENAME with reference refactoring

### Fixed

- INTO clause in CREATE EXTERNAL ENTITIES not routing to target module
- Mendix 11.9.0 integration test failures
- Demo user password updated to meet 12-char policy
- JSON number type inference and mxcli new locale duplicates
- BSON properties aligned with Mendix schema for mx diff compatibility
- View entity Source object ID preserved with CREATE OR MODIFY in DESCRIBE

### Changed

- Refactored large files: executor.go (4 files), init.go (3 files), tui/app.go (4 files), cmd_entities.go (3 files)
- Simplified diff-local to accept git ranges via `--ref` directly (removed `--base` flag)
- Pre-warmed name lookup maps to eliminate O(n²) BSON parsing in catalog source
- Updated CI to test against Mendix 11.9.0
- Documentation updates: LIST preferred over SHOW, execution modes, DataGrid column targeting, IMAGE datasource properties

## [0.5.0] - 2026-04-06

### Added

- **Import/Export Mappings** — CREATE/DESCRIBE/DROP IMPORT MAPPING and EXPORT MAPPING with JSON Structure integration, array mapping, and BSON roundtrip
- **IMPORT FROM MAPPING / EXPORT TO MAPPING** — Microflow actions for mapping-based data transformation
- **JSON Structure FOLDER** — FOLDER clause for organizing JSON Structures into folders
- **DESCRIBE NANOFLOW** — Display nanoflow activities, control flows, and return type
- **Pluggable Widget Engine v2** — Redesigned widget engine with 25+ new widget templates (accordion, maps, charts, timeline, etc.), filter widget migration, and `generateDefJSON` property mapping
- **WidgetDemo** — Baseline scripts and widget analysis tools for widget testing
- **mxcli new** — Create Mendix projects from scratch (downloads MxBuild, creates project, runs init, installs Linux mxcli binary)
- **setup mxcli** — Download platform-specific mxcli binary from GitHub releases
- **Podman Support** — Podman as Docker alternative with devcontainer configuration (#34)
- **Catalog Tables** — Import/export mapping catalog tables for project metadata queries
- **Project Tree** — Missing document types added to project tree and syntax highlighting
- **GRANT Additive** — GRANT is now additive with partial REVOKE for entity access
- **Version Pre-checks** — Executor commands validate Mendix version before BSON writes
- **SHOW FEATURES** — Display version registry feature availability
- **SHOW LANGUAGES** — Language listing and QUAL005 missing translations linter rule
- **Proposals** — Design proposals for i18n, workflow improvements, and multi-project tree view
- **BSON Tooling Guide** — Contributor documentation for BSON debugging workflow
- **CONTRIBUTING.md** — Rewritten with accurate project references

### Fixed

- CE1613 and Studio Pro crash from invalid CrossAssociation BSON (ParentConnection/ChildConnection fields) (#50)
- Import/export mapping BSON alignment with Studio Pro (JsonPath, ExposedName, ObjectHandling, array elements)
- Sort translation map iteration in all serializers for deterministic output
- Docker and diaglog tests cross-platform compatibility (macOS Unix socket paths)
- Roundtrip test stability with idempotency strategy
- Version gates for Mendix 10.24 nightly test failures and 11.0+-only MOVE commands
- Nanoflow BSON parsing for activities, flows, and return type
- mxcli new MPR filename detection from create-project
- Bun setup in nightly and release workflows for vscode-ext build
- Replace unreleased Mendix 11.9.0 with 11.8.0 in CI workflows

### Changed

- Redesigned import/export mapping syntax (v2) with comma separators
- Bumped dependencies: esbuild 0.28.0, typescript 6.0.2, sqlite 1.48.1, go-runewidth 0.0.22, @vscode/vsce 3.7.1
- Bumped CI actions: checkout v6, deploy-pages v5, upload-pages-artifact v4
- Bumped mdbook to v0.5.2 with musl for aarch64
- PR review checklist requires working MDL examples for syntax changes

## [0.4.0] - 2026-03-31

### Added

- **SEND REST REQUEST** — Microflow action for consumed REST services with full BSON serialization roundtrip
- **Pluggable Image Widget** — Full roundtrip support for `com.mendix.widget.web.image.Image` with Studio Pro-extracted templates
- **ALTER PAGE SET Url** — Change page URLs via MDL
- **ALTER PAGE SET Layout** — Switch page layout via MDL
- **ALTER ENTITY SET POSITION** — Set entity position in domain model diagrams
- **VISIBLE IF / EDITABLE IF** — Conditional visibility and editability with XPath expressions, plus TabletWidth/PhoneWidth properties
- **EXECUTE DATABASE QUERY** — Microflow action for static, dynamic, and parameterized SQL with runtime connection override
- **Contract Browsing** — SHOW/DESCRIBE CONTRACT ENTITIES/ACTIONS from cached OData $metadata, CONTRACT CHANNELS/MESSAGES from AsyncAPI
- **Integration Catalog** — 7 new catalog tables (rest_clients, rest_operations, published_rest_services, external_entities, external_actions, business_events, contract tables)
- **SHOW EXTERNAL ACTIONS / PUBLISHED REST SERVICES** — Integration pane commands
- **SHOW CONSTANT VALUES** — Display constant values and catalog tables
- **CREATE/DROP CONFIGURATION** — Configuration management with constant overrides
- **JavaScript Actions** — NDSL/MDL support for JavaScript action definitions
- **DROP/MOVE FOLDER** — Remove empty folders and reorganize project structure
- **GALLERY Columns** — DesktopColumns/TabletColumns/PhoneColumns properties
- **Forward-Reference Hints** — Helpful error messages when exec fails on later-defined objects
- **IMAGE FROM FILE** — Image collection syntax for file-based images
- **OpenSSF Baseline Level 1** — Security foundations and CodeQL fixes
- **Multi-Agent Merge Proposal** — Design proposal for parallel agent work on Mendix projects
- **Documentation Site** — mdBook-based site with tutorials, language reference, migration guide, and internals
- **Tool Integrations** — Added support for OpenCode, Mistral Vibe, and GitHub Copilot in `mxcli init`
- **TUI Enhancements** — Agent channel (Unix socket), UX improvements, auto-create module support
- **Custom Widget AIGC Skill** — Skill for AI-generated custom pluggable widgets
- **AI Issue Triage** — GitHub Actions workflow for automated issue classification
- **Daily Project Digest** — Scheduled workflow for project activity summaries

### Fixed

- Skip null TextTemplate in opTextTemplate to avoid CE0463 widget definition errors
- Set Editable to Conditional and fix Visible XPath expression serialization
- REST client BSON serialization field ordering and roundtrip correctness
- Image widget template extraction (imageObject defaults, Parameters version marker, Texts$Translation)
- Escape single quotes in page DESCRIBE output via `mdlQuote()`
- Resolve association/attribute and entity/enumeration ambiguity in MDL parser
- LSP diagnostics for editable `mendix-mdl://` documents
- Gallery CE0463 by re-extracting template and fixing augmentation
- DataGrid2 column name derivation from attribute or caption
- ComboBox association EntityRef via IndirectEntityRef with association path
- XPath tokens written unquoted to prevent CE0161
- Long type written as `DataTypes$LongType` instead of IntegerType
- Date as distinct type from DateTime throughout the pipeline
- MPR version detection using DB schema and `_FormatVersion` field
- Recurse into loop bodies when extracting catalog references
- CodeQL symlink path traversal alerts in tar extraction
- Multiple TUI data races and agent channel stability fixes

### Changed

- Bumped dependencies: pgx v5.9.1, zap v1.27.1, go-runewidth v0.0.21, cobra v1.10.2, mongo-driver v1.17.9, sqlite v1.48.0
- Refactored Visible/Editable syntax to `visible: [xpath]` and `editable: [xpath]`
- Used dedicated CWTest module in custom widget examples
- Always-quoted identifiers in MDL to prevent reserved keyword conflicts
- Added scope & atomicity and documentation sections to PR review checklist

## [0.3.0] - 2026-03-26

### Added

- **TUI** — Interactive terminal UI (`mxcli tui`) with yazi-style Miller columns, BSON/MDL preview, search, tabs, command palette (`:` key), session restore (`-c`), and mouse support
- **Workflows** — Full CREATE/DESCRIBE WORKFLOW support with activities (UserTask, Decision, CallMicroflow, CallWorkflow, Jump, WaitForTimer, ParallelSplit, BoundaryEvent), BSON round-trip, and ANNOTATION statements
- **Consumed REST Clients** — SHOW/DESCRIBE/CREATE consumed REST services with BSON writer and mx check validation
- **Image Collections** — SHOW/DESCRIBE/CREATE/DROP IMAGE COLLECTION with BSON writer and Kitty/iTerm2/Sixel inline image rendering in TUI
- **WHILE Loops** — WHILE loop support in microflows with examples
- **ALTER PAGE Variables** — ALTER PAGE ADD/DROP VARIABLE support (Phase 3)
- **XPath** — Dedicated XPath expression grammar, catalog table population, and skills reference
- **BSON Tools** — `bson dump --format ndsl`, `bson compare` with smart array matching, `bson discover` for field coverage analysis
- **Documentation Site** — mdBook-based site with full language reference, tutorials, and internals documentation
- **Anti-pattern Detection** — `mxcli check` detects nested loops and empty list anti-patterns (issue #21)
- **CREATE OR MODIFY** — Additive upsert for USER ROLE and DEMO USER
- **AI PR Review** — GitHub Actions workflow using GitHub Models API for automated pull request review
- **RETRIEVE FROM $Variable** — Support for in-memory and NPE list association traversal (issue #22)
- **Constants** — Constant syntax help topic, LSP snippet, and CREATE OR MODIFY examples
- **UnknownElement Fallback** — Table-driven parser registries with graceful fallback for unrecognized BSON types (issue #19)

### Fixed

- MPR corruption from dangling GUIDs after attribute drop/add (#4)
- BSON field ordering loss in ALTER PAGE operations (#3)
- ALTER PAGE SET Attribute property support (issue #10)
- ALTER PAGE REPLACE deep GUID regeneration for stale $ID fields (issue #9)
- Quoted identifiers not resolved in page widget references (issue #8)
- DATAGRID placeholder ID leak during template augmentation (issue #6)
- COMBOBOX association EntityRef via IndirectEntityRef with association path
- Page/layout unit type mismatch (Forms$ vs Pages$ prefix)
- VIEW entity types, constant value BSON, and test error detection
- False positive OQL type inference for CASE expressions
- RETRIEVE using DatabaseRetrieveSource for reverse Reference association traversal
- RETURNS Void treated as void return type like Nothing
- ANNOTATION keyword added to annotationName grammar rule
- System entity types and RETURN keyword formatting in microflows
- 10 CodeQL security alerts
- XPath token quoting for `[%CurrentDateTime%]` (#1)
- DROP MODULE/ROLE cascade-removes module roles from user roles
- Security script CE0066 entity access out-of-date errors
- Slow integration tests with build tags and TestMain (issue #16)
- Docker run failing on fresh projects (issue #13)

### Changed

- Aligned `mxcli check` and `mxcli lint` reporting with shared Violation format (issue #10)
- Promoted BSON commands from debug-only to release build
- Auto-discover `.mpr` file when `-p` is omitted
- Moved `bson/` and `tui/` packages under `cmd/mxcli/` for better encapsulation
- Consolidated show-describe proposals into `docs/11-proposals/` with archive
- Documented association ParentPointer/ChildPointer semantics in CLAUDE.md
- Normalized CRLF to LF in bug reports via `.gitattributes`

## [0.2.0] - 2026-03-15

### Added

- **CI/CD** — GitHub Actions workflow for build, test, and lint on push; release workflow for tagged versions
- **Makefile Lint Targets** — `make lint`, `make lint-go` (fmt + vet), `make lint-ts` (tsc --noEmit)
- **Playwright Testing** — Browser name config support, port-offset fixes, project directory CWD for session discovery
- **VS Code Extension** — Project tree auto-refresh via file watchers, association cardinality label fix

### Fixed

- Enum truncation, DROP+CREATE cache invalidation, duplicate variable detection, subfolder enum resolution
- IMPORT FK column NULL fallback and entity attribute validation
- Docker exec using host port instead of container-internal port
- AGGREGATE syntax in skills docs
- Association cardinality labels in domain model diagrams
- 3 MDL bugs and standardized enum DEFAULT syntax

### Changed

- Default to always-quoted identifiers in MDL to prevent reserved keyword conflicts
- Communication Style section in generated CLAUDE.md for human-readable change descriptions
- Shortened mxcli startup warning to single line
- Chromium system dependencies added to devcontainer Dockerfile

## [0.1.0] - 2026-03-13

First public release.

### Added

- **MDL Language** — SQL-like syntax (Mendix Definition Language) for querying and modifying Mendix projects
- **Domain Model** — CREATE/ALTER/DROP ENTITY, CREATE ASSOCIATION, attribute types, indexes, validation rules
- **Microflows & Nanoflows** — 60+ activity types, loops, error handling, expressions, parameters
- **Pages** — 50+ widget types, CREATE/ALTER PAGE/SNIPPET, DataGrid, DataView, ListView, pluggable widgets
- **Page Variables** — `variables: { $name: type = 'expression' }` in page/snippet headers for column visibility and conditional logic
- **Security** — Module roles, entity access rules, GRANT/REVOKE, UPDATE SECURITY reconciliation
- **Navigation** — Navigation profiles, menu items, home pages, login pages
- **Enumerations** — CREATE/ALTER/DROP ENUMERATION with localized values
- **Business Events** — CREATE/DROP business event services
- **Project Settings** — SHOW/DESCRIBE/ALTER for runtime, language, and theme settings
- **Database Connections** — CREATE/DESCRIBE DATABASE CONNECTION for Database Connector module
- **Full-text Search** — SEARCH across all strings, messages, captions, labels, and MDL source
- **Code Navigation** — SHOW CALLERS/CALLEES/REFERENCES/IMPACT/CONTEXT for cross-reference analysis
- **Catalog Queries** — SQL-based querying of project metadata via CATALOG tables
- **Linting** — 15 built-in rules + 27 Starlark rules across MDL, SEC, QUAL, ARCH, DESIGN, CONV categories
- **Report** — Scored best practices report with category breakdown (`mxcli report`)
- **Testing** — `.test.mdl` / `.test.md` test files with Docker-based runtime validation
- **Diff** — Compare MDL scripts against project state, git diff for MPR v2 projects
- **External SQL** — Direct queries against PostgreSQL, Oracle, SQL Server with credential isolation
- **Data Import** — IMPORT FROM external DB into Mendix app PostgreSQL with batch insert and ID generation
- **Connector Generation** — Auto-generate Database Connector MDL from external schema discovery
- **OQL** — Query running Mendix runtime via admin API
- **Docker Build** — `mxcli docker build` with PAD patching
- **VS Code Extension** — Syntax highlighting, diagnostics, completion, hover, go-to-definition, symbols, folding
- **LSP Server** — `mxcli lsp --stdio` for editor integration
- **Multi-tool Init** — `mxcli init` with support for Claude Code, Cursor, Continue.dev, Windsurf, Aider
- **Dev Container** — `mxcli init` generates `.devcontainer/` configuration for sandboxed AI agent development
- **MPR v1/v2** — Automatic format detection, read/write support for both formats
- **Fluent API** — High-level Go API (`api/` package) for programmatic model manipulation
