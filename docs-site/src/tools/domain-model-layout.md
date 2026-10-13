# Domain Model Layout

A domain model generated from an MDL script has to be *placed* somewhere, and
MDL says nothing about placement unless you write `@position` on every entity.
`mxcli layout` arranges a module from its association graph, so you do not have
to.

```bash
mxcli layout -p app.mpr --module Sales --dry-run   # list the moves
mxcli layout -p app.mpr --module Sales             # apply them
mxcli layout -p app.mpr                            # every module the project owns
```

## What it does

Entities are layered on the associations between them:

- an entity that references nothing else in the module is a **lookup**, and goes
  in the leftmost column;
- everything else sits one column past the furthest thing it references;
- entities with **no association at all** — non-persistent helpers, mostly — go
  in a band underneath, rather than being mixed in with the lookups.

Association lines then mostly run one way instead of crossing the diagram.

On a real 16-entity model the layering falls out of the associations with no
hints:

| column | entities |
|---|---|
| 1 | `Department`, `EmploymentType`, `MovementReason`, `PlanType`, `PlanningYear`, `Region` |
| 2 | `Team`, `GoalBucket` |
| 3 | `GoalChange`, `GoalRegionValue`, `PlanScope`, `CapTrackUser` |
| 4 | `Employee`, `ScopeMonth` |
| 5 | `EmployeeMonth`, `Movement` |

## It replaces positions you set by hand

This is the reason it is a command you run rather than something `exec` does on
its own. Inside the modules it touches, every entity is repositioned — including
any you arranged yourself in Studio Pro. Use `--dry-run` first; it prints every
move as `Module.Entity (x, y) -> (x, y)` and writes nothing.

Marketplace modules and `System` are never touched. Naming one explicitly is an
error rather than a silent skip:

```
$ mxcli layout -p app.mpr --module Administration
Administration comes from the Marketplace, and a module update replaces it —
pass --include-marketplace to lay it out anyway
```

## Repairing only the overlaps

A full layout is more than you want when the diagram is mostly fine. The default
grid gives every entity the same 280px row, but a box is as tall as its
attribute list, so an entity with about 30 or more attributes (fewer when the
one below it is long too) runs into the one below. `--only-overlapping` repairs
exactly that and leaves the rest alone:

```bash
mxcli layout -p app.mpr --module Sales --only-overlapping --dry-run
mxcli layout -p app.mpr --module Sales --only-overlapping
```

```
  Sales.Category overlaps Sales.Order
  Sales.Warehouse overlaps Sales.Invoice
  Sales.Employee overlaps Sales.Payment
  Sales.Ticket overlaps Sales.Category
  Sales.Category  (360, 380) -> (560, 250)
  Sales.Employee  (1140, 380) -> (1600, 420)
  Sales.Warehouse  (880, 380) -> (1080, 380)
Sales: 3 of 16 entities would move
Dry run: 3 entities would move. Re-run without --dry-run to apply.
```

`Ticket` collides with `Category`, which is itself moving away, so `Ticket` stays.

The rules:

- **A collision is an intersection of estimated boxes.** The estimate is the one
  the full layout uses: width from the longest name, height from the number of
  attributes. Boxes that only touch, or sit close, are not a collision. A
  domain-model annotation counts as something an entity can collide with.
- **The earlier entity in the model stays.** Of two colliding entities the one
  created first keeps its position, and the other moves (unless the first has
  itself been moved away by an earlier collision). An entity that collides
  with nothing is never moved, so a position you set by hand is kept unless its
  box collides with an entity that comes before it in the model.
- **A mover goes to the nearest free spot.** It is put at the closest position
  where its box, with a 40px margin, touches nothing else, and never above or to
  the left of the canvas origin. Annotations never move.
- **Running it again changes nothing.** The second run prints
  `no overlapping entities` and does not write.

`--dry-run` lists every collision found (`A overlaps B`, where `A` comes later in
the model) and every move; a collision with an entity that itself moves away
needs no move of its own. The box size is an estimate: Studio Pro's real
drawing is not known to mxcli, so an entity can still look crowded without
counting as a collision.

## Running it more than once

Positions are a function of the model alone, so the command is safe to leave in
a build script:

- **Idempotent.** A second run reports `already laid out` and does not write, so
  it produces no version-control noise.
- **Local.** Adding an entity and re-running moves only what the new
  relationships require — measured at 3 of 17 for one added entity with one
  association, not a reshuffle of the diagram.

## Flags

| Flag | Meaning |
|---|---|
| `--module <name>` | Lay out this module (repeatable). Default: every module the project owns. |
| `--dry-run` | Print the moves, write nothing. |
| `--include-marketplace` | Also lay out Marketplace modules. A module update replaces them, so this is normally pointless. |
| `--only-overlapping` | Move only entities whose boxes collide with another entity or an annotation, to the nearest free spot; keep every other position. |

## When you do want explicit positions

`@position(x, y)` on `CREATE ENTITY`, and `ALTER ENTITY … SET POSITION (x, y)`
to move one afterwards, both still work — and `describe entity` emits the stored
position, so arranging a model and describing it back is a way to capture a
layout into MDL.

Two things to know if you place entities yourself:

- The coordinate is the box's **top-left corner**.
- An entity created with no position takes the next slot in a wrapping grid.
  That is a default, not a layout: it keeps a large model on screen and keeps
  boxes of ordinary size apart, but it knows nothing about which entities are
  related, and an entity with many attributes is taller than a grid row. Run
  `mxcli layout --only-overlapping` to move just the ones that collide.

## Default position for an entity with no `@Position`

A wrapping grid (`mdl/dmlayout`), not the single 6,000px row it used to be.
