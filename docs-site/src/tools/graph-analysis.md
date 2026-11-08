# Graph Analysis

mxcli models a Mendix project as a **dependency graph** — documents (entities,
microflows, pages, …) are nodes and their references (`CATALOG.REFS`) are edges —
and runs topological analyses on top of it: god nodes, module coupling, community
detection, dependency cycles, layering, and centrality.

Use it to **understand an unfamiliar app and decide where to intervene**: what's
central and risky to change, which modules are entangled, what naturally belongs
together, and what it would take to split the app.

All of this is pure-Go (no external dependencies) and reads the catalog, so it
needs no Studio Pro and no cloud connectivity. It requires a **full** catalog
(the graph lives in the `refs` table, built by `refresh catalog full`).

## `mxcli graph-report` — the architecture map

A one-shot Markdown/JSON report rendered from the `CATALOG.graph_*` views:

```bash
mxcli graph-report -p app.mpr                 # markdown to stdout
mxcli graph-report -p app.mpr --top 25        # more rows per section
mxcli graph-report -p app.mpr --format json -o graph.json
mxcli graph-report -p app.mpr --include-framework   # keep System/Atlas/connectors
```

Sections: **god nodes** (degree centrality), **module coupling** (cross-module
"surprise edges"), **module cohesion** (intra/inter ratio), **dead documents**
(no inbound reference), **reference kinds** (the edge vocabulary), and **entity
hotspots** (entities used by the most flows). Framework/marketplace modules are
excluded by default — they dominate the raw top-N but aren't actionable.

Every section is a thin `SELECT` over a view, so it is reproducible directly:

```sql
select * from CATALOG.graph_god_nodes order by Degree desc limit 20;
```

## Community detection, cycles, layers, centrality

These need a graph algorithm, so they are computed on demand and stored:

```bash
mxcli -p app.mpr -c "refresh catalog communities"               # default resolution
mxcli -p app.mpr -c "refresh catalog communities resolution 0.6"  # coarser (fewer, larger)
mxcli -p app.mpr -c "refresh catalog communities resolution 2.0"  # finer (more, smaller)
```

This runs **Leiden** community detection, **Tarjan** strongly-connected components
(cycles), topological **layering**, **PageRank**, and **betweenness**, then
populates these catalog objects (and fills the `PageRank`/`Betweenness` columns of
`graph_god_nodes`):

| Object | Contents |
|--------|----------|
| `CATALOG.communities` | community id per asset |
| `CATALOG.community_summary` | per-community size, dominant-module label, members |
| `CATALOG.graph_cycles` | **assets** in a dependency cycle (structural kinds only) |
| `CATALOG.graph_module_cycles` | **modules** in a dependency cycle (every kind) |
| `CATALOG.graph_layers` | topological layer sequence number per asset |
| `CATALOG.graph_centrality` | PageRank / betweenness per asset |
| `CATALOG.graph_module_dependencies` | directed module→module edges (kind + count) |
| `CATALOG.graph_integration_surface` | cross-community edges → integration mechanism |

The `resolution` knob selects granularity: high γ → fine **candidate modules**;
low γ → coarse **candidate apps**.

### Two cycle tables, two questions

`graph_cycles` and `graph_module_cycles` are not a detail view and a rollup of
each other. Modules A and B are in a cycle when A references B *and* B references
A — through any documents, which need form no cycle between themselves. That is
the ordinary shape, so `graph_cycles` is legitimately empty for a genuinely
tangled pair of modules. Reading it as "no circular dependencies" while
`graph_module_coupling` listed both directions is
[mendixlabs/mxcli#1060](https://github.com/mendixlabs/mxcli/issues/1060).

They also read different edges, on purpose:

| | edge set | asks |
|---|---|---|
| `graph_cycles` | structural kinds only | which documents are tangled together |
| `graph_module_cycles` | every kind, like `graph_module_coupling` | which modules cannot be extracted independently |

The asset graph excludes navigational kinds (`layout`, `show_page`, `snippet_call`,
`datasource`, `widget`, `sync`, …) because they blur community and layer
clustering. A module cycle is a different claim: a page bound to a layout in
another module really is a dependency of that module, and on a blank Mendix app
`Administration → Atlas_Core` is a `layout` edge and nothing else. A module-cycle
table on the structural subset would answer "none" for exactly the pair that gets
reported.

`graph_analysis_scope` makes the split answerable instead of buried in source —
one row per reference kind with its edge count and whether it reaches the asset
graph:

```sql
select * from CATALOG.graph_analysis_scope order by InAssetGraph, Edges desc;
```

On a stock 11.14 app that is 110 of 316 edges outside it. Each
`graph_module_cycles` row also carries `RefKinds` — the kinds on that module's
edges *into the rest of the cycle*, so you know which reference to go and break.

```sql
select ModuleName, CycleSize, RefKinds from CATALOG.graph_module_cycles
  order by CycleSize desc, ModuleName;
```

### If one of these is empty

Empty means one of two very different things, and they used to look identical.
The tables above are filled by the pass, not by a build mode — so a catalog built
with `refresh catalog full` has all of them empty while `graph_module_coupling`,
a plain view over `refs`, answers normally. That asymmetry was
[mendixlabs/mxcli#1060](https://github.com/mendixlabs/mxcli/issues/1060).

A query now says which case you are in:

```
Warning: CATALOG.GRAPH_CYCLES requires refresh catalog communities (not run for this catalog)
```

No warning and no rows means the pass ran and genuinely found nothing.
`show catalog status` reports the same thing up front:

```
Graph analysis: ✓ Available (resolution 1)
Graph analysis: ✗ Not run (use refresh catalog communities)
```

The pass is **not** a build mode — it augments whatever mode is cached, so
`Build mode: full` says nothing about it. A later `refresh catalog full` used to
drop these tables silently; it now re-runs the pass at the same resolution, so the
graph survives a rebuild. To drop back, delete `.mxcli/catalog.db` and refresh.

### SHOW commands

```sql
list communities;                                   -- the community_summary listing
list community of Sales.Order;                      -- which community an asset is in
list community members of Sales.Order;              -- its co-clustered assets
```

## Two refactoring journeys

**Spaghetti → layered / modular app.**

```sql
select * from CATALOG.graph_cycles;                       -- tangled documents
select * from CATALOG.graph_module_cycles;                -- tangled modules
select Layer, AssetName from CATALOG.graph_layers
  order by Layer;                                          -- dependency depth
list communities;                                          -- cleaner module groupings
```

mxcli reports the *facts* (layer numbers, directed `graph_module_dependencies`);
your team decides what ordering is "correct" and enforces it with a Starlark rule.

**Monolith → multi-app (REST / OData / events).**

```bash
mxcli -p app.mpr -c "refresh catalog communities resolution 0.6"   # candidate apps
```

```sql
-- the contract list a split would require, classified by mechanism
select * from CATALOG.graph_integration_surface order by Edges desc;
```

Each crossing edge maps to its integration mechanism — `associate`→OData/shared
entity, `retrieve`→OData read, `call`→REST, `create/change`→event/REST write —
and `generalize` crossings are flagged as **blockers** (inheritance can't cross an
app boundary).

## Enforce your own architecture (Starlark)

mxcli ships the *facts*, not an opinion. Teams enforce their own layering /
allowed-dependency / no-cycle / coupling-budget policies via Starlark lint rules,
using these builtins (which read the graph tables):

`community_of`, `layer_of`, `cycles`, `module_cycles`, `module_dependencies`, `centrality`,
`god_nodes`, `integration_surface`, `refs_from`.

```python
RULE_ID = "ARCH900"
RULE_NAME = "No Payments→Reporting dependency"
DESCRIPTION = "Payments must not depend on Reporting"
CATEGORY = "architecture"
SEVERITY = "error"

def check():
    return [violation(message = "Payments must not depend on Reporting")
            for d in module_dependencies()
            if d.source_module == "Payments" and d.target_module == "Reporting"]
```

The graph builtins return empty when the community tables aren't built, so run
`refresh catalog communities` before `lint` in the same session. See
[Writing Custom Rules](custom-rules.md).

## Notes

- Quality depends on edge completeness. The `refs` graph captures control flow,
  CRUD, associations, generalization, widget datasources/actions, layout, and
  flow parameter/return types. References buried inside **expressions / XPath
  constraints** (and enum/constant usage) are not yet edges.
- Betweenness is O(V·E); it is skipped above ~6,000 nodes to keep the refresh
  fast (PageRank and communities still run).
- The Leiden implementation is deterministic and matches the reference
  `leidenalg` results.
