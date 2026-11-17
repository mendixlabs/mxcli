---
title: mxcli check — mine Mendix's diagnostics catalog to close the check↔mxbuild gap
status: draft
date: 2026-07-18
---

# Proposal: mine Mendix's diagnostics catalog for `mxcli check`

**Status:** Draft
**Date:** 2026-07-18
**Relates to:** `PROPOSAL_check_mxbuild_gap_heuristics.md` (one *tactical* slice of this —
6 specific constructs), `PROPOSAL_expression_type_checking.md` (Tier B enabler),
`PROPOSAL_mxcli_dev_warm_loop.md` (this catalog fronts the warm loop's build).

## Problem Statement

`mxcli check` reports "Check passed" for many constructs that the real MxBuild
(`docker build` / `mx check`) then rejects. Each miss costs a full build round-trip
(~30–60 s) to discover — and in the warm-dev-loop world (`PROPOSAL_mxcli_dev_warm_loop.md`)
it's the difference between an instant gate and a ~0.8 s build. The gap is currently
closed **reactively**, one rule at a time, whenever someone hits a build error.

Mendix already ships the complete list of things it will reject: a **diagnostics catalog**
embedded in `Mendix.Modeler.Texts.dll`. This proposal mines it into a **target list** so
`check`/`lint` coverage becomes a measured, prioritised program instead of a reactive one.

## BSON Structure

Not applicable — this is a static-analysis/tooling proposal. It reads Mendix's build-tool
assemblies (design-time), not app `.mpr` documents.

## Research findings (verified this session)

**Where the catalog lives.** `Mendix.Modeler.Texts.dll` embeds GNU-gettext `.po` resources
named `problem-descriptions_<locale>.po` (en-US primary, plus deprecated/test variants and
5 translations). They *are* the catalog.

**Format — already structured, not free text.**
- `msgid` = `CODE__SUMMARY`, e.g. `CE0001__ASSOCIATION_BETWEEN_PERSISTABLE_...`.
- `msgstr` = a **parameterised template** with named `{PLACEHOLDER}` tokens (and
  pluralisation tokens like `{S_IF_PLURAL}`), not runtime-assembled fragments.
- **Severity is encoded in the prefix**: `CE` = Consistency Error, `CW` = Warning,
  `CI` = Info. No inference needed.
- Each entry carries a `# LOCATION: <SourceFile>.cs` comment and an optional
  `# NOTE: MX_DOCS_CAT:<category>` docs-deep-link tag.

**Scale (Mendix 11.12.1, primary en-US PO):** **1318 codes** — 1265 `CE` + 53 `CW`
(`CI` codes live mostly in the deprecated/test POs); **727/1318 are parameterised**.
Present identically in 11.6.3 and 11.12.1.

**Static-checkability tiering** (heuristic, by message + source subsystem):

| Tier | Share | Nature | `mxcli check` reach |
|------|-------|--------|---------------------|
| A | ~60% (791) | structural / reference / required-property / naming | direct — model already parsed |
| B | ~26% (338) | expression / type-inference / XPath | needs a type-inference pass (`PROPOSAL_expression_type_checking.md`) |
| C | ~9% (120) | cross-document / datasource / client-compat | whole-model graph analysis |
| D | ~5% (69) | runtime / platform / tooling state | **not** statically checkable — skip |

Subsystem split (by source file): Microflows 364, Domain model 275, Pages/Widgets 248,
Integration/REST/OData 129, Security 29, Workflows 15, Navigation 7.

**mxcli already names a share of this by hand, and that share is computed, not quoted.**
`make diag-coverage` (`scripts/diagcoverage/`) reads the code IDs from the installed MxBuild's
`Mendix.Modeler.Texts.dll`, compares them with the codes named in mxcli's message strings and
lint rules, and ranks the codes it does not name by how many bug-fix records
(`.claude/skills/fix-issue/findings/`) hit them. A figure written here would be stale the next
time anyone adds a check. "Named" is a proxy for "covered": a check that does not name its
code is declared in `scripts/diagcoverage/covered-as.tsv`, with the fixture that shows it.
The approach works; the catalog turns "port as we hit them" into an ordered backlog.

**Is there a cheaper Mendix-native shortcut?** Not established. `mx check` (consistency
only, no build) was measured **once, at ~22 s** (project size and warm/cold state not
recorded), *slower* than an incremental build, because both cold-start the .NET model
engine and load the full `.mpr`. A later measurement on one small project (mxcli v0.32.0,
Mendix 11.16.1, ako/mxcli#1258) gave `mxcli docker check` 7.6 s and Mendix CLI 2's
`mendix check` (an in-process checker that emits CE codes such as CE0109/CE0844) 1.2 s.
Two single-project numbers do not settle it, but they show 22 s is not a constant.
`mendix check` also reported false errors on some real apps (tester report), so it is a
candidate *pre-filter* for the harvest loop, not a replacement oracle: any finding must be
confirmed against MxBuild before a rule is ported.

The justification for a Go-native `check` therefore rests on more than speed: it needs no
.NET runtime or mxbuild download, runs in the pre-write path before any model exists on
disk, and can be wired into `exec`. A faster Mendix-native checker would change the cost of
the *harvest loop*, not the case for porting.

## Spike results: Mendix CLI 2 `mendix check` as a fast oracle (2026-11-16)

Harness: `mdl-examples/check-gap/run-parity.sh` (fixtures beside it). Mendix 11.16.1
MxBuild, Mendix CLI 2 (11.16 build), Linux x64, one clean project from `mxcli new`
(0 errors on both checkers), single runs, no cold/warm split.

| fixture | `mxcli check -p` | `mx check` (MxBuild) | `mendix check` (CLI 2) |
|---|---|---|---|
| duplicate variable | rejected | CE0111 | CE0111 |
| wrong parameter name | rejected | CE1613 | CE0115, CE7710 |
| undefined variable | **passed** | CE0109 | CE0109, CE7823 |
| `DECLARE $X Integer = 'abc'` | **passed** | CE0117 | CE0844 |
| branch with no return | **passed** | CE0109 | CE0109, CE7823 |
| Decimal returned from String microflow | **passed** | CE0117 | CE0844 |
| unknown function `replace` | **passed** | CE0117 | CE7811 |
| commit of non-persistable entity (control) | passed | none | none |

- **Detection.** Both Mendix checkers flag all 7 broken fixtures and agree on the control.
  `mxcli check -p` misses 5 of 7, all Tier B (expression and flow typing).
- **Speed.** `mendix check` 1.3-1.7 s, `mx check` 6.3-8.5 s on the clean project (one 18.3 s outlier on the first fixture of a later run, so run-to-run spread is wide; 4.0 s on
  a 28-error project); `mxcli docker check` 7.5 s. None is near the 22 s above.
- **More specific.** Where `mx check` reports the generic CE0117 "Error(s) in
  expression", `mendix check` names the cause (CE0844 type mismatch, CE7811 unknown
  function). That is the better porting target.
- **Codes differ for the same finding.** On a 28-error project (`testdata/testapp-views`),
  all 28 "... no longer exists" findings are CE1613 in `mx check` (11.14.0 and 11.16.1
  alike) and CE7710 in `mendix check`. The harness must compare messages, not only codes.
- **The 6 extras are artifacts of a stripped project.** On `testdata/testapp-views/TestApp.mpr`
  (a Studio Pro 11.14.0 project holding only `MyFirstModule` and `System`: no Atlas_Core, no
  theme, no Administration module) `mendix check` reports 34 errors to MxBuild 11.16.1's 28.
  The 28 "selected … no longer exists" findings are CE7710 in CLI 2 and CE1613 in MxBuild,
  the same count. The 6 left over, classified by a control each:
  - **3x CE0136** (retrieve without an entity) are a cascade of the unresolved entity
    `Administration.Account`: they sit on the same three retrieve activities MxBuild reports
    as CE1613. Defining the entity removes all three, and the retrieve errors with them.
  - **2x CE0535** (grid column weights "add up to -1") are a cascade of the unresolved layout
    `Atlas_Core.Atlas_TopBar`. The stored columns are identical to what mxcli writes for
    `DesktopWidth: AutoFill` (Weight, TabletWeight, PhoneWeight and PreviewWidth all -1), and
    that shape is clean in both checkers on a healthy project, in a theme-less project when
    the layout resolves, and for a lone 6-wide column. Repointing the page at a layout that
    exists clears both; CE6858 ("update Atlas UI to 2.4 to use Layout Grid on Native pages")
    appears instead, for every page with a layout grid in a project without Atlas.
  - **1x CE6083** (design property "Row gap" unsupported by the theme) survives the repoint,
    and `mendix check` also raises it for a new page written into this project. MxBuild is
    silent on all of them. It is clean in both checkers on a healthy project, where the Atlas
    theme defines the property. The likely reason is that this project has no theme to
    compare against (inferred, not measured). MxBuild does raise CE6083 for real projects:
    10 bug-fix records name it.
  None is a check to port, and none is a CLI 2 false positive on a healthy project. Defining
  the missing entity also made `mendix check` raise CE8412 (the demo users' entity must be
  `System.User`): a result of the control, not of the project.
- **MxBuild reports errors in stages; `mendix check` does not.** Measured on MxBuild 11.16.1
  with six single defects and every ordered pair of them (30 documents, both orders) in one
  project, plus a reference defect:

  | stage | what is reported | what it hides |
  |---|---|---|
  | 0 | a reference that does not resolve (CE1613) | **every other error in every document**, a page's CE0149 included |
  | 1 | a duplicate variable name (CE0111) | every other error in the same microflow |
  | 2 | an undefined variable (CE0109) | the stage 3 group, in the same microflow |
  | 3 | CE0117 (wrong type), CE0068 (return in a loop), CE0038 (declare without a value), CE0067 (no final return) | nothing: all four are reported together |

  With one wrong-parameter call (CE1613) in 13 documents, MxBuild reported those 13 errors and
  nothing from the other 36 documents; `mendix check` reported 115 errors in 50. Dropping the
  reference defect made the undefined variable, the wrong type and the page's CE0149 appear
  together. No pair differed by order, and no two defects share a code. `mendix check` reported
  both defects in all 30 pair documents (judged by its own codes: CE0844 is its name for
  MxBuild's CE0117). Not measured: Studio Pro itself (the same consistency checker, so likely
  the same), documents other than microflows and pages, and the order among codes outside these
  six. Reproduce it with `make mask-matrix` (`scripts/maskmatrix`). What follows from it:
  - **Report every real error.** `mxcli check` reporting more than MxBuild is not a mismatch:
    the rest are real, only hidden until the earlier stage is fixed. Copying the staging would
    send an author through one fix-and-rebuild loop per stage.
  - **One defect per fixture**, and a control with it removed. A fixture holding two defects
    can look as if one is absent (the `captrack-19` rule stood down for exactly this reason).
  - **The bug records skew early.** A later-stage code shows up in a report only after the
    earlier ones were fixed, so the field ranking in `make diag-coverage` under-counts it.
  - **Run the oracle on a project whose references resolve**, or stage 0 hides the rest.
- **Conclusion.** `mendix check` works as a first-pass oracle: run it, then confirm any
  code it reports against `mx check` before porting. Run it on a project whose references
  resolve (no CE7710 / CE1613): on one that does not, it reports cascades MxBuild suppresses. It is a proprietary binary, so the
  harness takes its path as an option and nothing is committed.

## Extraction methodology

Reproducible, ~1 min, no Studio Pro. The **script is ours; its output is Mendix's corpus**
(see § Open questions on IP). Committed as `scripts/extract-diagnostics-catalog.sh`:

```bash
# 1. stream just the one assembly from the Mendix CDN (no full mxbuild download)
curl -s "https://cdn.mendix.com/runtime/mxbuild-${VERSION}.tar.gz" \
  | tar -xz --occurrence=1 modeler/Mendix.Modeler.Texts.dll

# 2. extract the embedded PO resources (needs mono-utils → monodis)
monodis --manifest   Mendix.Modeler.Texts.dll   # lists the .po resources
monodis --mresources Mendix.Modeler.Texts.dll   # writes each resource to a file

# 3. parse problem-descriptions_en-US.po into structured rows:
#    code, prefix→severity, slug, message-template, {params}, source .cs, MX_DOCS_CAT
#    then tier each by static-checkability. (Python parser in the script.)
```

`msgid` → `(prefix, number, slug)`; prefix → severity; `{...}` tokens → parameter list;
`# LOCATION:`/`# NOTE:` comments → source file + docs category. Output: `catalog.csv` /
`catalog.json` (local only). Re-run per Mendix version to refresh.

## Representative examples (verbatim, small fair-use sample)

```
CE0001  error  An association between a persistable entity and a non-persistable entity
               must start in the non-persistable entity and have owner 'Default'.
CE0002  error  Owner must be 'Default' for self-referential associations.
CE0006  error  An XPath constraint on an access rule on {DESCRIPTION} is not allowed,
               because it is not persistable.
CE0190  error  Scheduled events of type Legacy are removed; right click for possible
               remedies.                        [# NOTE: MX_DOCS_CAT:ScheduledEventMigration]
CE0529  error  The selected page '{PAGE_NAME}' contains {A_IF_SINGULAR}required
               parameter{S_IF_PLURAL} and can not be used as home page.
CE0572  error  Widget{S_IF_PLURAL} {WIDGETS} {IS_OR_ARE} not supported in React client.
CW0001  warn   Property 'Sort order' of the {CONTAINER} has no effect when a page is
               used for selecting.
CW0040  warn   Action activity that has a side effect on the client is not recommended
               here because the microflow is used as a data source for
               {CONTAINER_TYPE_AND_NAME}.
CI1561  info   Path parameter 'x' does not appear in the path ''.
```

Plus the production-security rule this session actually hit and fixed with one `grant`:
*"At least one allowed role must be selected if the page is used from navigation, a
button, or a URL."*

## Proposed approach

1. **Ship the catalog as a versioned data asset** — `sdk/versions/diagnostics-{version}.json`,
   produced by the extraction script per Mendix version. Lets `check`/`lint` emit
   **Studio-Pro-identical codes + messages + severities**, a UX win even before new rules.
   (Subject to the IP decision below — may ship code IDs + our own message text rather than
   Mendix's verbatim strings.)
2. **Port by tier then frequency.** Tier A first (cheapest, highest hit-rate), Tier B behind
   the type-inference work, Tier C case-by-case, Tier D never. Route `CE`→`check` (hard),
   `CW`→`lint` (advisory) — severity is free from the prefix.
3. **Harvest-against-the-oracle loop.** Use `mx check` (the full 1318-code truth; ~22 s in
   one measurement, see above) as an **offline oracle** over a corpus of deliberately-broken projects; diff its findings
   against `mxcli check`; the misses are the ranked backlog; port; re-run to confirm parity.
   This is the build tool for the porting effort, and reuses the "run mxbuild over broken
   projects and harvest output" fallback from the very first spike as a *deliberate* tool.
   Open spike: run Mendix CLI 2's `mendix check` over the same fixtures as a fast
   pre-filter and record its agreement with `mx check` per code (false-positive and
   false-negative rate) before trusting it for anything.
4. `PROPOSAL_check_mxbuild_gap_heuristics.md` is the first tactical instance; this proposal
   is the strategic program it fits into.

## Implementation Plan

| File | Change |
|------|--------|
| `scripts/diagcoverage/` | New (done): `make diag-coverage`, per-code coverage from the installed MxBuild, IDs only, ranked by bug-record frequency. No monodis. |
| `scripts/extract-diagnostics-catalog.sh` | New: the reproducible extractor (stream DLL → `monodis --mresources` → parse+tier). Committed here. |
| `sdk/versions/diagnostics-{version}.json` | Generated catalog data asset per version (IP decision pending). |
| `mdl/linter/rules/`, `mdl/executor/validate_*.go` | Port Tier-A codes, prioritised by frequency; cite the `CE`/`CW` code in each. |
| `mdl/executor/…` (oracle harness) | Offline `mx check` vs `mxcli check` parity report over a broken-project corpus → ranked backlog. |
| `mdl-examples/check-gap/` | Broken-project fixtures feeding the oracle harness. |

## Version Compatibility

- Codes are stable across 11.6.3 / 11.12.1; the extractor is version-parameterised, so the
  catalog refreshes per Mendix release. Register generated assets alongside the existing
  `sdk/versions/mendix-{9,10,11}.yaml`.
- New/removed codes between versions are themselves a useful signal (what Studio Pro started
  or stopped checking).

## Test Plan

- Extractor: unit-test the PO parser against a small committed fixture PO (our own, not
  Mendix's), asserting code/severity/params/location parsing.
- Oracle harness (gated, needs mxbuild): for each broken fixture, assert `mxcli check` now
  reports the same code `mx check` does — parity regression per ported rule.
- Each ported rule: a `mdl-examples/check-gap/` fixture + a `validate_*`/lint test.

## Open Questions

1. **IP / licensing (blocking the data-asset shape).** The message *text* is Mendix's
   proprietary copyrighted content extracted from their DLL. Committing all 1318 verbatim
   into the repo is redistribution. Options: (a) ship only **code IDs + severity + source +
   our own paraphrased messages**; (b) generate the full catalog **locally at build time**
   from the user's own licensed mxbuild (never committed); (c) confirm redistribution is
   permitted. Default: **(b)** — commit the extractor, not the corpus.
2. **`monodis` dependency.** The message extractor needs `mono-utils`. Alternatives:
   `ilspycmd` (dotnet tool) or a small pure-Go PE manifest-resource reader to drop the mono
   dep. The code IDs alone need none of it: the PO resources are plain ASCII in the DLL, so
   `scripts/diagcoverage` finds them by scanning the bytes. Only the message text and the
   tiering still need the extractor.
3. **Tiering is heuristic.** The A/B/C/D split is keyword+subsystem-based; the real
   per-rule trigger predicate lives in Mendix's `.cs` (the `LOCATION` breadcrumb) and must
   be reverse-engineered per rule. The catalog gives the *target + message*, not the
   predicate — sizing must account for that.
4. **Fast oracle.** The spike above supports `mendix check` as a first-pass oracle with
   `mx check` confirming. Still open: a corpus larger than 8 fixtures, and cold/warm
   timings. The 6 extras on a broken project are classified above.
5. **Overlap with `PROPOSAL_check_mxbuild_gap_heuristics.md`.** Fold that proposal's 6 cases
   in as the first harvested batch, or keep it separate and cross-link? (Recommend: keep it
   as the tactical pilot; this proposal owns the methodology + backlog.)
