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

## Measurements (Studio Pro, 2026-11-23)

Two Studio Pro screenshots (classic domain model editor), measured against
models whose stored values are known:

- **TestApp, module `Mappings`, 100 % zoom** (`testdata/testapp`, ako/TestApp
  at the pinned submodule commit). Order (290, 150), Customer (620, 80), OrderLine
  (630, 250) and Product (640, 410) all map to the screen with one factor, 1.59
  screen px per model px, in x and y. (Item is ~100 px off, probably moved after
  the pinned commit, so it is left out.)
- **An app's `Fulfilment` module, 50 % zoom.** Its entities sit on columns 206–208
  screen px apart and rows 222–223 apart: 260 × 0.796 and 280 × 0.796. That is
  exactly `dmlayout.GridSlot` at half of the factor above, so this module is a
  never-laid-out mxcli model, the case MPR020 reports.

Two more shots, in Studio Pro 11.16.1's **beta** domain model viewer, of
[ako/PizzaV1](https://github.com/ako/PizzaV1) (`GitPizza.mpr`): Fulfilment at
75 % and Catalog at 100 %. Positions read from the repository fit them with one
factor per shot in x and y (1.044 and 1.392). Every Fulfilment and Catalog entity
is on a `GridSlot`, and on a copy of the project MPR020 reports Catalog,
Fulfilment, PizzaChat and Shop (Core and Simulation, whose entities are not on
the grid, are not reported).

What they settle (classic / beta where they differ):

| Quantity | Measured | `dmlayout` today |
|---|---|---|
| Box width | **fixed**, whatever the names: **150** classic, **158** beta; long names are cut off with "…" | `max(160, 8 × longest name + 24)`: covers both, too wide for long names, which only costs compactness |
| Box height | classic **`max(84, 36 + 16 × attributes)`** (84 for 1–3, 100 for 4, 132 for 6, ~855 for 51); beta **`max(~76, 28 + 16 × attributes)`** (76 for 3, 93 for 4, 141 for 7, 284 for 16) | `max(60, 34 + 16 × attributes)`: within a few px from 4 attributes up, **16–24 px short for 0–2** in both viewers |
| Label width | 83–93 px for 14–17 characters: about 5 px per character plus ~12 | not modelled |
| Anchor | `Order_Customer` stores from `100;46` / to `0;61`: the line leaves Order's right side 46 % down and reaches Customer's left side 61 % down | read and written, never computed |
| Cross-module line | Horizontal, from the entity's **left side** to the diagram's left edge; with *n* lines on one entity, the *i*-th is at **i / (n + 1)** of its height (mid-height for one) | not modelled |

Three facts about cross-module lines that change Phase 3:

1. **The target is drawn too, not only the holder.** `persistent_order` belongs to
   module ViewAssociations (`OrdersVE` → `Mappings.Order`), and the Mappings
   diagram draws it from Order's left side. So the entities needing a free band
   are every local entity at either end of a cross-module association, and the
   associations come from **every** module's `CrossAssociations`, not only this
   domain model's.
2. **The line runs through whatever is to its left.** In the Fulfilment shot,
   Coupon's line crosses DeliveryZone, OrderLine's crosses Order (its label lands
   on two other lines), and `Cart_ActiveOrder` runs through PaymentAttempt.
   That is #1929's rule 2, observed.
3. **Several lines on one entity are spaced evenly down its left side, at
   i / (n + 1) of its height.** Measured in the beta viewer to the pixel:
   Fulfilment.Order's three (`Order_PlacedBy`, `Cart_ActiveOrder`,
   `TrackView_Order`) at 25 / 50 / 75 %, Catalog.Product's two at 33 / 67 %,
   Catalog.Option's five at 17 … 83 %. The classic shot shows the same 25 / 50 %
   for Order. A line from an entity on the right runs across the whole diagram:
   Catalog.Ingredient's go through Allergen, Category, Product and ProductSize.

The same shots show rule 5. With 150-px boxes 260 px apart, the gap is 110 px.
Labels such as `PaymentAttempt_Order`, `OrderLogEntry_Order` and
`OrderLineModifier_OrderLine` do not fit: they are cut off or sit on a box, and
Studio Pro draws them with a red border. Red does not mean only that, though:
`OrderAccess_User` is red with room to spare, so the colour is not used as a
signal here. Many lines also converge on one corner of Order.

## Implementation Plan

Phase 0 and four phases, each its own PR and each shippable alone.

### Phase 0 — Correct the box estimate (measured above) — implemented

`boxSize` becomes width 160 and height `max(84, 40 + 16 × attributes)` (4 px over
the classic header for measurement error): the
larger of the classic and beta viewers in each dimension, since a project is
opened in either. The height is the safety fix: a 0–2-attribute entity is drawn
up to 24 px taller than estimated, so `Place` and `ResolveOverlaps` can leave it
touching the box below, and MPR016 can miss the overlap. A fixed width makes
`Plan` more compact for long names.
Both feed every placement path and MPR016/MPR020, so this is its own PR, with
tests at the measured sizes.

### Phase 1 — Label-sized gaps and target-aligned rows (rules 4, 5, 7) — implemented

1. **Gap per column boundary.** The gap between layers *i* and *i+1* becomes
   `max(columnGutter, labelWidth(longest association drawn between them) + markerAllowance)`.
   Here `labelWidth(n) = n × labelPxPerChar + 12` with `labelPxPerChar = 5.5`
   (measured about 5; see Measurements), and `markerAllowance = 40`, from #1929.
2. **Align rows to targets.** Walk the layers left to right, keeping the
   barycentre order. Each entity's desired top y is the top y of its
   *primary target*: the already-placed target with most edges to it, ties by
   name. Place it there if the box clears the entities already in its column.
   Otherwise take the nearest free y below. The same top y on both ends is what
   makes a line exactly horizontal once Phase 2 anchors it (see below).
3. Isolated band: unchanged.

Determinism (`TestPlan_IsDeterministic`) and no-overlap (`TestPlan_NoOverlap`)
still hold. Neither depends on the gap or the y rule.

**Follow-up — lookups beside what uses them (implemented).** Phase 1 left every
lookup in column 0, stacked from the top; with phase 3 pushing referrers down,
lookups such as Catalog's Category sat far above and columns away from their
only referrer. A lookup now takes the column just left of its nearest referrer
(the layering is otherwise unchanged) and, after placing, moves to the median
row of its referrers when that is free in its column, keeps its own
cross-module lines clear, stays out of those of the entities to its right, and
is closer than before. PizzaV1 Catalog: 8 of 10 lines straight (was 6), 160 px
shorter, no cross-module line blocked. The locality test now states its
contract as "only the new entity's column and the isolated band move".

**Follow-up — compaction (implemented).** Two refinements after looking at the
PizzaV1 renders:
- Unconnected entities no longer go in a band under the graph. They fill the
  free space to its right or below it, each at the candidate position (outside
  the graph's area, clear of every box, its cross-module lines clear, not in
  another's lines) that keeps the diagram closest to a landscape frame
  (`max(width, 1.5 × height)`); those with cross-module lines shortest first.
  Shop 3717 → 2408 px tall, PizzaChat 1216 → 728.
- A lookup alone in its column may share its nearest referrer's column, kept
  when it has fewer crossings, or as many and the diagram is narrower and no
  taller, and never when it adds a line through a box. Above or below is the
  swap search's call. Catalog: Allergen with Ingredient (above it: 1 crossing,
  against 3 below), crossings 6 → 2, 317 px narrower.

### Phase 2 — Recompute anchors (rules 4, 5) — implemented

`dmlayout.Anchors(dm, positions)` returns a connection-point pair per
association, and a full `mxcli layout` writes it into `ParentConnection` /
`ChildConnection` (`--only-overlapping` keeps the stored ones). For each
association between two different entities (self-associations keep what is
stored; generalizations have no stored points):

- **Side.** The side of each box that faces the other: top/bottom when the boxes
  overlap horizontally, left/right otherwise.
- **Straight where the boxes share a span.** Both ends at the same absolute
  coordinate: in the header (20 px below the lower top) when that is inside the
  shared span, which keeps the line straight whatever the heights; otherwise the
  middle of the span. This straightens a referrer *anywhere* beside a tall
  target, not only the one on its top row. The coordinate is taken on the
  longer box's whole-percent grid (Mendix stores whole numbers; a percent of an
  856-px box is 8.6 px), so the shorter box's rounding is worth half a pixel.
- **Not through a box.** A straight segment that would pass another box (a line
  between columns that are not adjacent) is not drawn straight.
- **Parallel associations.** The *k*-th association between the same two
  entities (by name) runs 20 px further along the span, so two associations
  are two lines with two labels.
- **Spreading.** The remaining ends on one side are spread evenly,
  `(i+1) × 100 / (k+1)`, in the order of their far ends, kept 5 % clear of the
  straight lines' ends on that side, and never on a corner (1–99).

Measured on ako/PizzaV1's Fulfilment: before, every referrer of Order met it at
one point; after, the lines to Coupon, DeliveryZone, Oven, OrderAccess,
OrderLine, OrderLogEntry, OrderLineModifier and both Courier associations are
straight. Lines that still cross a box are between non-adjacent columns
(Catalog's `Product_Allergen`, `RecipeItem_Ingredient`): that is ordering, phase 4.

### Phase 3 — Cross-module free band (rules 2, 3) — implemented

`dmlayout.CrossLinesFor(module, dm, allDomainModels)` collects, per entity, the
cross-module associations drawn at it: its own module's (the `ParentID`) and
every other module's whose `ChildRef` names it. `PlanWith(dm, cross)` places
each such entity, in its column, at the first y where each of its *n* lines (at
i / (n+1) of its height, ±10 px for the label) misses every box to its left:
its target's row when that is clear, otherwise the nearest clear y below,
otherwise under everything to its left. Column 0 starts at
`max(OriginX, longest cross-module name drawn from it + markers)`, the label
sitting between the canvas edge (x = 0, measured) and the box. An isolated
entity with a cross-module line gets a row of its own at the left of the band.
`Plan(dm)` is `PlanWith(dm, nil)`. `mxcli layout`, MPR020 and the `exec`
reminder all use the cross-aware plan, so `layout` still clears MPR020.

Measured on ako/PizzaV1 (Catalog, Fulfilment, Shop, PizzaChat, Core,
Simulation): **28 of 39** cross-module lines ran through a box after phase 2,
**0** after this. Straight in-module lines stay about the same (22 → 21 of 29).

The cost is height. A module where most entities carry cross-module lines
(Catalog: Product, Option, Ingredient, ProductSize, Allergen) becomes a
staircase, each column stepping below the one to its left; Shop grows from 2292
to 3717 px tall. Some in-module lines become long diagonals and two labels can
meet where lines cross. That trade is what #1929's priority order asks for
(rule 2 above rule 4 and rule 7). The crossings are phase 4's.

### Phase 4 — Crossing count, better ordering, lint (rule 1) — implemented

1. `dmlayout.Lines` / `Crossings(dm, pos, anchors, cross) (lines, boxes)`:
   segments over the estimated boxes, for associations (from their anchors;
   `StoredAnchors` reads them, with Studio Pro's 0;50 / 100;50 default),
   generalizations (centre to centre) and cross-module lines (left side to
   x = 0). Lines sharing an entity meet at its box and are not a crossing; a
   line along an edge does not count as through a box.
2. Ordering: **not** the barycentre sweeps first planned. They were built and
   measured, and never produced a better order, on PizzaV1 or on ~7,000 random
   models, because row alignment and the cross-module bands decide most
   positions. What did help is a neighbour-swap search: swap two entities in a
   column, keep the swap when the full placement, scored on the lines it
   actually draws (`lines + 2 × through a box`), is strictly better; repeat
   until nothing helps, within a budget of 8 swaps per entity. Strict
   improvement only, so never worse than the layering's order, and
   deterministic. PizzaV1 Catalog: 8 to 6; the other modules were already at
   their best.
3. `mxcli layout` prints `crossings A -> B (lines crossing lines …, lines
   through a box …; estimated)` for every module it changes. On PizzaV1:
   Catalog 30 → 6, Fulfilment 13 → 3, Shop 30 → 8 (grid as stored → laid out).
4. MPR021 `AssociationCrossesEntity` (warning): any of those lines through an
   entity it does not connect, against each box inset by 8 px so a line grazing
   an estimated edge is not reported. PizzaV1: 27 findings as stored, 1 after
   `mxcli layout` (Catalog's `Product_Allergen`, Allergen being used from
   columns 1 and 3). TestApp: 1, a real crossing in a hand-arranged diagram.

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

1. **Cross-module lines.** *Resolved* (see Measurements): left side to left edge,
   target entities included, *n* lines at i / (n + 1). *Open:* the order of the
   lines on one entity (Order's are not alphabetical). It does not change the
   band, which covers all of them.
2. **Label metrics.** *Resolved:* about 5 px per character plus ~12, measured in
   both shots; Phase 1 uses 5.5 per character.
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
