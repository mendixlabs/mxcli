---
title: Readable domain model layout — label-sized gaps, straight lines, anchors, cross-module bands
status: draft
date: 2026-11-23
related:
  - mendixlabs/mxcli#1929
  - mendixlabs/mxcli#1692
  - mendixlabs/mxcli#1479
  - PROPOSAL_entity_position.md
---

# Proposal: Readable domain model layout

**Status:** Draft
**Date:** 2026-11-23
**Issue:** mendixlabs/mxcli#1929 (domain model layout skill)
**Relates to:** mendixlabs/mxcli#1692 (crossing-minimising mode; only its SVG part
shipped, d8883466), mendixlabs/mxcli#1479 (top-left anchoring, overlap repair)

## Problem Statement

mendixlabs/mxcli#1929 attaches a `domain-model-layout` skill. It lays out a
domain model by hand through the Studio Pro MCP server (`ped_update_document`).
It follows seven rules in priority order:

1. no association line crosses a box or another line;
2. an entity with a cross-module association needs a free band to its left;
3. the left margin is sized to the longest cross-module label;
4. lines are horizontal or vertical where possible;
5. every line is at least as long as its label plus the two multiplicity markers;
6. an entity can sit inside a triangle of three others (a "pocket");
7. the diagram is compact.

`mxcli layout` (`mdl/dmlayout.Plan`) has the layer structure the skill builds by
hand. Lookups go on the left, referrers to the right, and isolated entities go in
a band underneath. It follows almost none of the rules, though:

| Rule | What `Plan` does today | Visible result |
|---|---|---|
| 2, 3 | Cross-module associations are dropped from the graph (`newGraph`) | A referrer in a right-hand column has boxes to its left at its own height. Its cross-module line runs through them. |
| 5 | `columnGutter = 90`, fixed | A 14-character name such as `Order_Customer` needs about 125 px, so the label is cut off or lands on a neighbour |
| 4 | Every column is stacked from `OriginY` on its own. The barycentre pass only *orders* a column; it does not *align* rows with targets. | Most lines are diagonal |
| 4, 5 | Anchors (`ParentConnection` / `ChildConnection`) are never touched | After a move, an anchor someone dragged in Studio Pro points the wrong way (e.g. a bottom anchor towards a box that is now to the right) |
| 1 | One barycentre sweep, no crossing count | No way to tell a good layout from a bad one, in `--dry-run` or in lint |

The people who benefit are agents and developers who generate a domain model
with MDL and then open it in Studio Pro. They currently get a diagram that has
to be rearranged by hand. #1929 shows that people are writing hand-placement
skills to fill this gap. Such a skill only works with Studio Pro open, and it
estimates the same box sizes `dmlayout` already estimates.

The aim: once `mxcli layout` follows the skill's rules, the skill can shrink to
"run `mxcli layout`, check `describe --format svg`". The SVG renderer has drawn
stored anchors since d8883466, so the agent *can* see the result. The skill
assumes it cannot.

## BSON Structure

No new document type and no new property. This proposal writes two properties
that mxcli already reads and writes on every association write.

| Property | Owner | Format | Notes |
|---|---|---|---|
| `Location` | `DomainModels$EntityImpl` | point | Top-left of the box (#1479). `Plan` writes it today. |
| `ParentConnection` | `DomainModels$Association` | string `"x;y"` | Anchor on the **FROM** entity (the reference holder; the Parent/Child inversion in CLAUDE.md applies). Percentages of the box: x 0 = left … 100 = right, y 0 = top … 100 = bottom. |
| `ChildConnection` | `DomainModels$Association` | string `"x;y"` | Anchor on the **TO** entity |

Constraints already measured and recorded in `sdk/domainmodel/connection.go`:

- Both components **must be integers**. `"0.5;50"` fails to load
  (StorageLoadException, 11.13.0). Rounding is part of the algorithm, not a
  formatting detail.
- No range check: negatives and values over 100 load. This proposal only ever
  writes points on the box edge (one component is 0 or 100).
- A blank 11.13 app stores `0;54` / `100;54` for
  `Administration.AccountPasswordData_Account`. The skill's claim that Studio Pro
  defaults to bottom-centre → top-centre does not match this measurement. It may
  describe what the MCP server writes.
- `DomainModels$CrossAssociation` has **no** connection properties. Writing them
  crashes Studio Pro (#50). A cross-module line can only be steered by moving its
  entity.
- `UpdateDomainModel` already round-trips stored anchors
  (`TestUpdateDomainModel_PreservesConnectionPoints`). The write goes through the
  existing domain-model write path, so it is reconciled and elided like any other
  write (ADR-0008). An unchanged layout must write nothing.

No GUID or `$ID` is touched: positions and anchors are presentation only.

## Proposed CLI Changes

No MDL syntax. The command and its flags stay as they are. The default `Plan`
gets better, and two outputs are added:

```bash
mxcli layout -p app.mpr --module Sales --dry-run
#   Sales.Order      (100, 100) -> (330, 100)
#   Sales.OrderLine  (100, 280) -> (560, 100)
#   anchors: 3 associations re-anchored
#   crossings: 4 -> 0   (estimated from box-size estimates)
# Sales: 2 of 3 entities would move
```

- `crossings: before -> after` is printed on every run, dry or not. It counts
  line × line and line × box intersections, using the same estimated boxes and
  stored or planned anchors.
- `--only-overlapping` keeps its contract: it moves only colliding boxes and
  **does not** re-anchor (see Open Questions).
- A new lint rule, **MPR021 `AssociationCrossesEntity`**, reports an association
  line that runs through an entity box. Like MPR016 it uses `Intersects`, not
  `TooClose`, so a finding is real and not just tight. Line × line crossings stay
  out of lint: they are a quality score, not a defect.

## Implementation Plan

Four phases, each its own PR and each shippable alone.

### Phase 1 — Label-sized gaps and target-aligned rows (rules 4, 5, 7)

1. **Gap per column boundary.** The gap between layers *i* and *i+1* becomes
   `max(columnGutter, labelWidth(longest association drawn between them) + markerAllowance)`.
   Here `labelWidth(n) = n × labelPxPerChar + 10` and `markerAllowance = 40`,
   both from #1929. The skill's 5.5 px/char is measured. `dmlayout` uses
   8 px/char for *box* text, so use a separate, generous `labelPxPerChar = 7`
   until it is calibrated (Open Questions).
2. **Align rows to targets.** Walk the layers left to right, keeping the
   barycentre order. Each entity's desired top y is the top y of its
   *primary target*: the already-placed target with most edges to it, ties by
   name. Place it there if the box clears the entities already in its column.
   Otherwise take the nearest free y below. The same top y on both ends is what
   makes a line exactly horizontal once Phase 2 anchors it (see below).
3. Isolated band: unchanged.

Determinism (`TestPlan_IsDeterministic`) and no-overlap (`TestPlan_NoOverlap`)
still hold. Neither depends on the gap or the y rule.

### Phase 2 — Recompute anchors (rules 4, 5)

`Plan` returns anchors as well as positions:

```go
type Layout struct {
    Entities map[model.ID]model.Point
    Anchors  map[model.ID]AnchorPair // association ID -> parent/child edge points
}
```

For each same-module association (not self-associations, which keep what is
stored):

- **Side.** The side of each box that faces the other box. Horizontally separated
  → left/right sides; horizontally overlapping → top/bottom.
- **Exact horizontals.** If both boxes share a top y, both ends are anchored the
  same pixel distance below the top (`headerAnchorPx = 20`, inside the header,
  which is the same height on every box). Converted per box:
  `y% = round(20 × 100 / estimatedHeight)`. A height estimate that is a few px
  off moves the point by about a pixel. Centre alignment would move it by half
  the error.
- **Exact verticals.** Solve `x_a + w_a × p = x_b + w_b × q` for integer
  percentages, if a solution exists inside both boxes.
- **Spreading.** When *k* lines leave one side, order them by the position of
  their far end (top to bottom, or left to right). Spread them evenly
  (`(i+1) × 100 / (k+1)`) so no two meet at one point and none cross at the side.
- Otherwise, the facing sides at 50 %.

`runLayout` writes `Anchors` into `Association.ParentConnection` /
`ChildConnection` before `UpdateDomainModel`.

### Phase 3 — Cross-module free band (rules 2, 3)

**Blocked on a Studio Pro measurement** (Open Questions 1). The design, if #1929's
description holds:

1. `newGraph` records which entities are `ParentID` of a `CrossModuleAssociation`
   (the "holders").
2. After Phases 1–2, each holder needs a clear horizontal band. The band runs from
   the diagram's left edge to the box's left side, at the box's vertical middle,
   with label height above and below. Nothing may intersect it: no box, no planned
   line.
3. A holder in layer 0 has the band by construction. Any other holder whose band
   is blocked moves down to the first y in its column where the band is clear.
   That keeps its layer, so its same-module lines still run the right way.
4. The left margin (`OriginX` for layer 0) becomes
   `max(OriginX, labelWidth(longest cross-module name in layer 0) + markerAllowance)`.

### Phase 4 — Crossing count, better ordering, lint (rule 1)

1. `dmlayout.Crossings(dm, layout) (lineLine, lineBox int)`: segment geometry over
   the estimated boxes and anchors. Generalizations count as lines between box
   centres.
2. Ordering: replace the single sweep with alternating down/up barycentre sweeps
   (fixed count, e.g. 4). Keep the ordering with the fewest crossings; ties go to
   the earlier one, so the result stays deterministic.
3. `--dry-run` / normal output prints `crossings: before -> after`.
4. MPR021 in `mdl/linter/rules/mpr021_association_crosses_entity.go`.

Pockets (rule 6) are out of scope. With estimated box sizes the clearance check
inside a triangle is too unreliable to do automatically, and the gain is small
next to Phases 1–4.

### Files to modify/create

| File | Change |
|---|---|
| `mdl/dmlayout/dmlayout.go` | Per-boundary gaps, target-aligned y, `Layout` return type, holder tracking in `newGraph` |
| `mdl/dmlayout/anchors.go` (new) | Side choice, header-offset horizontals, integer vertical solve, spreading |
| `mdl/dmlayout/crossings.go` (new) | Segment intersection over estimated boxes; `Crossings` |
| `mdl/dmlayout/*_test.go` | Tests listed below |
| `cmd/mxcli/cmd_layout.go` | Write anchors, print anchor and crossing summary, help text |
| `mdl/linter/rules/mpr021_association_crosses_entity.go` (+ test) | Phase 4 lint rule |
| `.claude/skills/mendix/…` (domain model skill) | "After a domain model change: `mxcli layout --module M`, then `describe --format svg`" |
| `docs-site/src/` (layout page) | Document the new output lines |

Callers of `Plan` (only `cmd_layout.go` today) adapt to the `Layout` return.
`Place` / `PlaceNear` / `ResolveOverlaps` are untouched.

## Version Compatibility

No version gating. `Location`, `ParentConnection` and `ChildConnection` exist in
every Mendix version mxcli supports, in the same `"x;y"` form. The
integer-only rule was measured on 11.13.0. Phase 2 always writes integers, so the
rule holds whether or not older versions are stricter.

## Test Plan

Unit tests in `mdl/dmlayout/` (no `.mpr` needed):

- `TestPlan_GapFitsLongestLabel`: a 30-character association name makes the gap
  ≥ its label width + 40. **Control:** the same model with a 3-character name
  keeps `columnGutter`.
- `TestPlan_ReferrerSharesTargetRow`: Order → Customer gives the same top y.
  **Control:** two referrers of one target — the second does not overlap the
  first.
- `TestAnchors_HorizontalIsExact`: boxes of different attribute counts on one row
  give the same absolute anchor y within 1 px, and both components are integers.
- `TestAnchors_SpreadOnOneSide`: three lines from one side get distinct points,
  ordered so they do not cross.
- `TestAnchors_SelfAssociationUntouched`.
- `TestCrossings_KnownFigures`: hand-built X shape = 1 crossing; parallel = 0;
  line through a box = 1 line-box.
- `TestPlan_CrossingsNotWorseThanBefore` on a representative fixture (e.g. a
  copy of a 20+-entity module), with the count asserted, not just "≤".
- Phase 3: `TestPlan_CrossModuleHolderHasFreeBand`. **Control:** with holder
  tracking stubbed out, the same fixture blocks the band. The test must fail
  against that stub (CLAUDE.md: fix proven to be the cause).
- Existing `TestPlan_*` stay green, notably determinism and no-overlap.

Backend: extend `association_connection_test.go`. A layout run writes the planned
anchors, and a second run writes **nothing**. Include a control that proves the
"nothing" check can fail.

Visual (`.claude/skills/verify-in-runtime.md` does not apply; this is the
editor, not the runtime):

- `describe --format svg` before/after on the fixture, attached to the PR.
- One Studio Pro screenshot of the same module after `mxcli layout`, for the
  Phase 1–2 PR and again for Phase 3. Valid BSON and a clean `mx check` say
  nothing about whether a label is readable.

## Open Questions

1. **Cross-module line geometry.** #1929 says Studio Pro draws a cross-module
   association from the left-middle of the holder to the diagram's left edge, and
   that it cannot be moved. Nothing in the repo confirms this: the SVG renderer
   does not draw cross-module associations at all. Before Phase 3, a Studio Pro
   screenshot is needed of a module where a holder sits right of another entity.
   Also needed: is an entity that is the *target* of another module's association
   drawn with a line in this module's diagram? If so, it needs a band too.
2. **Label metrics.** 5.5 px/char (#1929) vs a generous 7. Calibrate from one
   Studio Pro screenshot using the skill's method (screen px ÷ real px between two
   boxes whose positions are known).
3. **Anchors under `--only-overlapping`.** Re-anchoring only the associations of
   moved entities would fix the stale-anchor problem there too. But that mode
   exists to respect hand work, and a dragged anchor *is* hand work. Proposal:
   leave anchors alone in that mode; revisit if users report bent lines after a
   repair.
4. **Generalization lines.** Studio Pro draws them, but no anchors are stored for
   them. Phase 4 counts them as centre-to-centre lines. Is that how Studio Pro
   routes them?
5. **Should the #1929 skill ship?** Once Phases 1–3 land, a skill wrapping
   `mxcli layout` + SVG check covers the CLI case. The MCP-only hand-placement
   path (Studio Pro open, unsaved changes) remains the skill's own territory. That
   is a call for the issue author and maintainer, not this proposal.
