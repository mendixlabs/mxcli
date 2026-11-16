# Project Reports

`mxcli report <kind>` produces a report a team asks of any Mendix app. Bare
`mxcli report` is still the scored best-practices report ([mxcli report](mxcli-report.md));
the kinds below answer specific questions instead of scoring.

| Kind | Question it answers |
|---|---|
| `architecture` | How do the app's modules depend on each other; where are the layers, bounded contexts and real cycles; what is unused? |
| `tests` | What do the microflow tests and browser journeys check, did they pass, and which microflows do they reach, per module? |
| `security` | Who can read what, which client-callable flows trust the caller, which defaults are unsafe? |
| `performance` | Where does time go in the main journeys at production volume, and did a fix help? |
| `traceability` | Is each requirement in the specification built, and what proves it? |
| `map` | What does the app contain, and how does it fit together? (an interactive guide) |

## Shared shape

Every kind collects data first and renders a page from that data second:

```bash
# measure, keep the data
mxcli report architecture -p app.mpr --format json -o arch.json
# render, with a person's names and verdicts, without measuring again
mxcli report architecture --from arch.json --annotations notes.yaml -o architecture.html
```

| Flag | Meaning |
|---|---|
| `--format html\|markdown\|json` | `json` is the collected data; `html` is one standalone page (no scripts, no external requests). Without `--format`, the `-o` extension decides, else markdown. |
| `-o <file>` | Output file (default stdout). |
| `--annotations <file>` | YAML or JSON with names, statuses and notes, keyed by the keys the page prints (`cluster:…`, `finding:…`, `gap:…`). |
| `--from <report.json>` | Re-render a saved report. |

Every page ends with **How this was measured** and **What this does not cover**,
stored with the data, so a page re-rendered later still says what the measurement
could not see. Preconditions a run did not meet (an unrecorded database state,
leftover test-runner files) are shown at the top. Annotation keys that no longer
match anything are listed at the bottom.

```yaml
title: Architecture — Shop
summary: One paragraph shown first.
items:
  "cluster:Orders.Order": {name: "Order intake", note: "..."}
  "finding:constant-secret:Crypto.Key": {status: fixed, note: "rotated, env only"}
recommendations:
  - Move Sales.SUB_Total into Billing.
```

Marketplace modules, `System` and the test runner's `MxTest` are outside the app
unless `--include-marketplace`; `--modules` and `--exclude` choose by hand.

## architecture

Layered module map (domain dependencies only — menus, layouts and page links are
reported separately, because they put every screen module in one cycle), module
health with the role of low-cohesion modules (shell, admin UI, adapter), the
coupling matrix in layer order, bounded contexts (clusters × modules), god nodes,
bridges, entity hotspots, cycles with the edges that close them, and unused
documents — marked `verify` when the graph cannot see how they might be used (named
in a constant, placed in a layout, named in text).

```bash
mxcli report architecture -p app.mpr -o architecture.html
```

## tests

Results from `mxcli test --junit` (or `--run` to run them), static reach of each
test through the call graph, and — with journeys — runtime reach from the spans the
app exported while each journey ran. Coverage per module in five categories: both,
tests only, journeys only, startup or scheduled (not a gap), neither.

```bash
mxcli report tests -p app.mpr --run -o tests.html
mxcli report tests -p app.mpr --junit results.xml --journeys journeys.jsonl --spans spans.jsonl -o tests.html
```

Journeys need the app running with `mxcli run --local --trace-otlp http://127.0.0.1:4318`
and a collector listening: `--journeys-cmd "<command>"` runs the journey script with
one, or run `mxcli report collect-spans -o spans.jsonl` yourself. The journey script
appends one JSON line per journey (or step) to `$MXCLI_JOURNEY_LOG`:

```json
{"journey":"Place an order","start":"2026-11-12T10:00:01Z","end":"2026-11-12T10:00:07Z","status":"passed","checks":["order shows in list"]}
{"meta":true,"database":"fresh seed 2026-11-12"}
```

## security

A model review: findings by severity with fixes and statuses, a user role × entity
read grid with each access rule's XPath classified (own rows, via user, filtered,
all), client-callable flows triaged by whether they check the caller or rely on
access rules, unsafe defaults (secret-looking constant defaults, test switches
defaulting to true, demo users, guest access, strict mode, password policy), and
the lint security rules grouped with a verdict slot.

```bash
mxcli report security -p app.mpr -o security.html
```

## performance

Per journey step: browser, server and database time and query count (median of warm
runs), a flame chart, the most frequent statements, Postgres `auto_explain` plans
with JIT time, N+1 shapes from the model, and before/after with `--baseline`.

```bash
mxcli report performance -p app.mpr --journeys steps.jsonl --spans spans.jsonl \
  --plans postgresql.log --baseline before.json -o performance.html
```

The command does not change the database. Seeding, instrumenting Postgres and
restoring are a protocol, described in the `report-performance` skill.

## traceability

Every requirement in the [project brain](project-brain.md), per milestone, with a
derived status (built, partial, changed, not built, not traceable), the microflows
that implement it, the tests and journeys that reach them, and the decisions and
open questions that bear on it — plus three gap lists.

```bash
mxcli report traceability -p app.mpr --junit results.xml --journeys journeys.jsonl --spans spans.jsonl -o traceability.html
```

`partial` and `changed` come from a decision or open question whose body says
``Partly meets requirement `<id>`: …`` or ``Changes requirement `<id>`: …``
(`brain capture --partly-meets <id>:<reason>` / `--changes` write it). The status
is brain's: `brain plan --json` reports the same `status` per requirement, and
`brain check` fails on a verdict naming an unknown id, one on a requirement, and a
requirement that is both changed and partly met.
Milestones are part of the plan: `mxcli brain milestone "M1" --slice 01-accounts`
writes `milestone: "M1"` in the slice's frontmatter, and `--requirement <id>`
places one requirement apart from its slice. The older
`docs/brain/plan/traceability.json` is still read, with a warning, until
`mxcli brain milestone --import` moves it into the plan. `brain plan --json` lists
each requirement with each anchor's resolution, and each slice's milestone.

## map

One interactive page for someone who did not build the app: user roles and where
each starts, the systems around the app and what runs without a user; the module
map (domain use apart from screen links, the hub and weak links behind toggles);
inside each module its data model with other modules' entities as ghosts and the
flows that write each entity, entry points grouped by what starts them, and the call
closure of one entry point; page flows per role, with the pages a role may open but
no click path reaches; associations crossing module boundaries; naming prefixes,
shared flows and an entry-point × module matrix; published operations with their
call closure, consumed services, HTTP calls, queues, schedules, Java actions and
constants; and a searchable index of every document with "uses" and "used by".

```bash
mxcli report map -p app.mpr -o map.html
mxcli report map -p app.mpr --format json -o map.json   # the same data for another viewer
```

With a project brain that has a plan, the map adds a **Features** tab: one feature
per slice, with its requirements, milestone and status, and what its anchors reach
— flows (the anchored ones, their footprints and call closure), the pages and
entities those lead to, entry points started from outside the feature, and the
roles that may open its pages. **Reached by no feature** lists the pages, entry
points and persistent entities no requirement leads to. Features are derived from
the plan, so there is no feature map to maintain alongside it.

The page is self-contained and draws one diagram at a time. Module purposes come
from domain-model documentation, or from `module:<Name>` notes in `--annotations`.

Entry points include two kinds of reference the catalog records for this: `by_name`
(a constant whose value is a flow's qualified name) and `queue` (a task queue runs
the flow a call placed in it). Both also keep such flows off the architecture
report's unused list.

## Skills

Each kind has a skill with the judgement the command cannot make and the traps found
building the reports by hand: `report-architecture`, `report-tests`,
`report-security`, `report-performance`, `report-traceability`, `report-map`.
