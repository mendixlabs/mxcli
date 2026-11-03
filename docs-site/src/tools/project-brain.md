# Project Brain

`mxcli brain` records the project knowledge mxcli **cannot** compute. Two halves:

- **Decisions** — why a pattern was chosen here, which marketplace version broke
  what, what a recurring mxbuild error means in *this* app.
- **Open questions** — what is not decided yet, recorded so it is not silently
  forgotten and rediscovered expensively later.
- **The plan** — the requirements being built from, grouped into slices, when the
  source is a specification document, a prototype or a conversation.

The plan half exists because that source leaves **no trace in git**. A Word
document, a Figma file and a chat window are not an issue and not a commit
message, so hours of work can end with nothing recording what it was for — and a
session resuming tomorrow has no idea what it was building towards.

It is opt-in. A project without `docs/brain/` never hears about it. `mxcli`'s
bootstrap interview asks for requirements by default.

## The rule

**Anything mxcli can answer does not belong in the brain.** Entities,
microflows, pages, bindings, references and callers are all queryable. A note
that transcribes any of them will disagree with the project the moment someone
edits the model — and it will disagree silently, because nothing checks prose.

The brain stores only the negative space: the reason, the constraint, the
history that no query can reach.

## Layout

```
docs/brain/
  README.md              what the folder is
  project.md             cross-cutting decisions
  modules/Sales.md       decisions anchored to the Sales module
  modules/Finance.md     …
  plan/01-accounts.md    requirements for one deliverable slice
  plan/02-approvals.md   …
```

Committed, and reviewed in a pull request like any other change.

A shard is re-rendered from its entries on every write, so the title and the
preamble are regenerated and hand-edits to them do not survive. **YAML
frontmatter is preserved**, because mxcli does not own it — it is where markdown
tooling keeps per-file metadata (Foam and Obsidian tags, a docs site's nav
weight), and discarding it would quietly break any of those. It counts toward
the shard's cap, since a session loading the shard loads it too. A shard with no
frontmatter never grows an empty block.

The split is not cosmetic. A single file would make the size cap a project-wide
budget — recording a `Sales` decision would compete with a `Finance` one — and
every session would load every module's decisions. With one file per module, a
session loads `project.md` plus the shards for the modules it is touching.

`mxcli brain brief` assembles that set, so the saving does not depend on anyone
judging it correctly:

```bash
mxcli brain brief --slice 07-planning -p app.mpr   # project + that slice's
                                                   # modules + its plan
mxcli brain brief --module Sales -p app.mpr        # project + Sales, no plan
```

Which modules a slice needs is derived from its requirements' anchors — you do
not name them, because that is the thing the brief is being read to find out.
The pack goes to stdout and its size to stderr, so it pipes straight into a
prompt; `--json` returns the shards separately with their paths.

This is worth little in one long session, where the store is read once and then
cached. It is worth a large fraction of the context when each slice runs in its
own session or sub-agent and the pack is re-read from a cold start every time.

## Anchors

An entry's anchors are what make it routable and checkable.

| Anchor | Names |
|---|---|
| `@Sales` | a module |
| `@Sales.Order` | a document: entity, microflow, page, workflow, … |
| `@Sales.Order.Status` | a member: an attribute |

The **first** anchor decides the file: `@Sales.Order` puts the entry in
`modules/Sales.md`. An entry with no anchor is cross-cutting and goes to
`project.md`.

There is no index to maintain — the module prefix *is* the file name.

## Workflow

An agent captures; a person promotes.

```bash
mxcli brain init -p app.mpr

mxcli brain capture "Orders are committed by Finance, not Sales" \
  -a @Sales.Order -a @Finance.ACT_Post -p app.mpr

mxcli brain staged -p app.mpr      # review the queue
mxcli brain promote a1b2c3 -p app.mpr
```

Captures go to `.mxcli/brain/staged.jsonl`, which `mxcli init` git-ignores — so
nothing reaches a pull request until someone has looked at it. `mxcli lint`
prints a one-line reminder when the queue is not empty.

The queue is deliberately not sharded: routing it would force the file decision
before a human has looked at the entry, which is the decision promotion exists
to make.

## Checking

```bash
mxcli brain check -p app.mpr             # every shard
mxcli brain check --changed -p app.mpr   # only shards this branch touched
```

Two independent things are checked.

**Does each anchor still resolve?** Three outcomes, one of which is a failure:

| Outcome | Meaning | Fails |
|---|---|---|
| resolved | the anchor names something that is there | no |
| **not found** | the anchor names nothing — the entry is stale | **yes** |
| not indexable | the target exists, but its document type is not in the catalog's index | no |

The third state matters. The catalog's `objects` view covers the describable
document types, not all of them. Without this distinction an anchor to, say, a
scheduled event would read as *missing*, and `check` would demand edits to
entries that are perfectly current. mxcli separates the two with a
type-agnostic unit lookup that cannot miss a kind, because it never asks what
kind anything is.

**Is each entry in the right shard?** A separate axis, not a fourth anchor
state: every anchor can resolve and the entry still be in the wrong file. At
least one anchor must belong to the shard the entry sits in; anchors into other
modules are reported but do not fail, because a fact like "`Sales.Order` is
committed by `Finance.ACT_Post`" genuinely spans two modules.

Misfiling is only decided when something resolved. An entry whose anchors are
all *not indexable* is left alone — there is no evidence about where it belongs.

## Requirements and slices

When the source of truth lives outside git, record it before building:

```bash
mxcli brain capture "Orders must be approvable by a manager" \
  --slice 02-approvals -a @Sales.ACT_Order_Approve -p app.mpr
```

`--slice` is the only signal needed: it files the entry in `plan/02-approvals.md`
and makes it a requirement. Slices sort by name, so a numeric prefix is how a
roadmap is sequenced — that is your choice, not a field mxcli maintains.

### A requirement's anchor points forward

| | Anchor points at | An anchor that does not resolve means |
|---|---|---|
| decision | what exists | the decision is **stale** — `check` fails |
| requirement | what is intended | **not built yet** — normal, `check` passes |

Same syntax, opposite meaning. Anchoring a requirement at a microflow that does
not exist yet is correct: it is the forward reference that later becomes the
progress signal.

This is not a theoretical distinction. Recorded as an ordinary entry, a single
unbuilt requirement takes `mxcli brain check` to exit 1 — which is what made
requirements a separate kind rather than more entries in the same files.

### Progress is derived

```bash
mxcli brain plan -p app.mpr
```

```
SLICE           BUILT  PLANNED   UNANCHORED
01-accounts         1        0
02-approvals        0        1            1

1 of 3 requirements built, across 2 slice(s).
```

A requirement is **built** when its anchors resolve against the model. Nothing in
the file says "done": create the microflow a requirement points at and the count
moves on the next run, with the plan file untouched.

That is the reason this belongs in mxcli rather than in a hand-kept markdown
checklist. A status column is wrong the moment someone builds something, and
nothing tells you; a derived one cannot be.

A requirement with **no anchor** is counted apart as *unanchored* rather than
silently called planned — it cannot be measured until you say what will
implement it.

Misfiling is not checked for slices: a slice spans modules by design.

### Before there is a project

The brain does not need a model to exist: capture requirements from a
specification first, create the app afterwards. Name the brain's directory with
`-C`; no store command opens, or needs, the project.

```bash
mxcli brain init -C ./App
mxcli brain capture "A planner can create a visit" -s 01-scheduling -a @Scheduling.Visit -C ./App
mxcli brain plan -C ./App
```

`-p app.mpr` still works and means the directory that `.mpr` is — or will be —
in. The two are alternatives; passing both is refused.

`plan` and `check` then report what is true: nothing is built.

```
No model at /ws/App.mpr yet: 1 anchor(s) not checked, so nothing counts as built.

SLICE            BUILT  PLANNED   UNANCHORED
01-scheduling        0        1

0 of 1 requirements built, across 1 slice(s).
```

Every anchor is **unverifiable** — neither *not found* (that would report the
whole store as stale) nor built. `check` exits 0, and `--json` carries
`"model_absent": true` and an `unverifiable` count. Only a *missing* `.mpr`
means this: one that exists but will not open is still an error.

### Then create the project in the same directory

```bash
mxcli new App --version 11.12.1
```

`mxcli new` refuses a non-empty directory with one exception: a directory
holding **only** a brain — `docs/brain/` written by mxcli and its queue in
`.mxcli/brain/` — is adopted. The project is created around it, the store and
the queue are left exactly as they were, and from then on `-C ./App` finds the
one `.mpr` in the directory and counts the same requirements against it. Any
other file in the directory, or a `docs/brain/` mxcli did not write, is still
refused. A file the project would overwrite is reported before anything moves.

So the brain lives where the project will live, not in a workspace above it:
the directory is the project root from the first requirement onwards.

## Open questions

An open question is a decision that has not been made yet.

```bash
mxcli brain capture "Do approvers see rejected orders?" --open -a @Sales.Order -p app.mpr
```

Its **anchors are not checked**. A question may name something that does not
exist — often the question is precisely whether it should — so the staleness
rule that keeps decisions honest would report every question as a defect.
Measured: the identical anchor takes `brain check` to exit 1 as a decision and
exit 0 as a question.

`--open` combines with `--slice`. A question about a slice's scope is filed with
that slice and counted apart from its requirements: an unanswered question is
not outstanding scope, so it never inflates the slice's numbers.

Answering it converts it in place:

```bash
mxcli brain resolve <id> "Yes, for 30 days
Agreed with the product owner; drives the overview filter and the access rule."
```

The entry keeps its **id** and its position in the file, and the question
survives as the answer's context. From that moment its anchors are checked like
any other decision — which is the whole point of the transition, and is asserted
by a test.

Both `brain check` and `mxcli lint` report unanswered questions until one is
resolved. A question nobody answers is the one kind of entry that gets more
expensive the longer it sits.

## When to capture a decision

Capture needs a trigger, not good intentions. The reliable one:

> **You have had to correct the same thing twice.**

The second correction is the signal — it will happen a third time, to someone
else. The other trigger is choosing between real alternatives where the losing
one would look reasonable to the next person: record the choice *and* what ruled
the other out, or it gets re-litigated.

## Size

```bash
mxcli brain show -p app.mpr
```

```
SHARD              ENTRIES    LINES     HEADROOM
project.md               3       28      92/120
Administration.md        2       16     224/240
```

Each shard has a line budget and `promote` refuses rather than exceeding it,
naming the shard and its occupancy. `project.md` is the tightest: it is the only
file loaded every session, so every line in it is charged to every session.

A plan slice gets far more room — it holds source material and is read when
planning rather than loaded every session. It is still a budget, and that is the
point: **a slice too long to read is a slice that should be split.** Here the cap
does not merely bound context cost, it enforces the slicing discipline.

When a promotion is refused the answer is to condense or drop, not to raise the
cap. The cap is what stops the store becoming a file nobody reads.

Sizes are computed on every run and are deliberately not written into any
committed file, including the store's own `README.md` — a figure in prose is
stale the next time anyone promotes.

## Renaming

`mxcli rename` rewrites matching anchors in `docs/brain/` and in the staged
queue, and reports the count:

```
Renamed entity: Sales.Order → Sales.PurchaseOrder
Updated 2 brain anchor(s): @Sales.Order -> @Sales.PurchaseOrder
```

Renaming a module also moves `modules/<Old>.md` to `modules/<New>.md`, so its
entries do not immediately read as misfiled. Entry ids are not re-derived: an id
is a handle (`brain promote <id>`, prose that cites one), and invalidating every
reference *to* an entry in order to fix that entry's references to the model
would trade one dangling pointer for several.

It happens at the rename because it cannot be done afterwards. A decision's
anchor points backward, so a stale one shows up as `NOT FOUND`; a requirement's
points forward, so a stale one merely counts as `PLANNED` — indistinguishable
from not built yet. Measured on a real project, a refactor moved the reported
progress from 65/65 to 63/65 and the number was the only symptom.

## Commands

| Command | Does |
|---|---|
| `brain init` | Creates `docs/brain/`. Refuses a `docs/brain/` it did not write |
| `brain capture "<text>" [-a @Anchor]…` | Queues an entry. Never commits |
| `brain staged [--since <id>] [--slice <n>] [--fail-if-empty]` | Lists the queue with the shard each entry would land in |
| `brain promote <id> [--to <shard>]` | Writes it into its shard |
| `brain drop <id>` | Removes it from the queue or from its shard |
| `brain capture "<text>" --slice <name> [-a @Anchor]…` | Queues a **requirement** of that slice |
| `brain capture "<text>" --open [-a @Anchor]…` | Queues an **open question**; anchors not checked |
| `brain resolve <id> "<answer>"` | Answers it, turning it into a decision in place |
| `brain plan [--slice <name>]` | Each slice's requirements counted against the model |
| `brain brief --slice <name> \| --module <M>` | The reading pack: exactly the shards that work needs |
| `brain check [--changed]` | Anchors resolve, entries filed correctly, plus slice progress |
| `brain show [<shard>]` | Entries, lines and headroom per shard |

Dropping the last entry from a module shard deletes the file, so the directory
does not accumulate husks that read as "this module has decisions".

Every command takes `--json`, so a dispatcher running one agent per slice can
act on the answers rather than read them. `brain staged --since <id>` is the
slice boundary: note `last_id` before handing a slice off, pass it back
afterwards, and `--fail-if-empty` exits 1 on a slice that recorded nothing.

`--since` rather than `--slice`, because `capture --slice` is what makes an entry
a *requirement* — a decision found while building a slice carries no slice at
all, and a slice's findings are mostly decisions. The queue is append-only, so
its own order is the honest boundary.
