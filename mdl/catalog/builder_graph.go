// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/catalog/graph"
)

// graphRefKinds are the structural reference kinds used to build the analysis
// graph. Navigational kinds (layout/parameter-of-page/show_page) are excluded —
// they add UI coupling that blurs module/app clustering.
var graphRefKinds = []string{
	"call", "retrieve", "create", "change", "delete", "associate", "generalize",
	"parameter", "return",
	// Not "commit": its variable is resolved from a parameter, a create or a
	// retrieve, each of which already links the flow to the entity (directly,
	// or through the association an association retrieve names). Adding it
	// would mostly double existing edges' weight and shift communities and
	// centrality on every project that commits (ako/mxcli#963).
	// Entry points: something outside the call graph invokes these, so the
	// microflow they run is reachable even though nothing in the model calls it. Leaving one out does
	// not hide it from GRAPH_DEAD_ASSETS — that view asks only whether ANY refs
	// row targets the name, whatever its kind — but it does cut the microflow out
	// of the analysis graph, so communities, layers, cycles and centrality all
	// see the API and scheduling surface as unreachable roots.
	// schedule  a scheduled event runs it
	// publish   a published service runs it: a REST operation, an OData
	//           resource or action, a service's authentication microflow
	// event     an entity event handler runs it on every commit/delete — the
	//           entity rather than the platform, but reached the same way, and
	//           structural rather than UI coupling — or a business event
	//           service runs it when a subscribed event arrives
	"schedule",
	"publish",
	"event",
	// by_name  a constant's value names the flow (a module is told which
	//          microflow to run, e.g. OIDC's provisioning microflow)
	// queue    a task queue runs the flow a call placed in it
	"by_name",
	"queue",
}

// graphRefKindsSQL renders graphRefKinds as a quoted SQL IN list, so the schema
// and the edge loader below cannot disagree about what is in the analysis graph.
func graphRefKindsSQL() string {
	quoted := make([]string, len(graphRefKinds))
	for i, k := range graphRefKinds {
		quoted[i] = "'" + k + "'"
	}
	return strings.Join(quoted, ", ")
}

// betweennessNodeCap bounds the O(V*E) betweenness computation. Above it,
// betweenness is skipped (PageRank/communities still run) to keep the pass fast.
const betweennessNodeCap = 6000

// effectiveResolution normalises the Leiden resolution. One function so the
// value recorded in the cache is the value the pass actually used — recording
// the caller's 0 would make a later re-run at "the same resolution" a different
// run.
func effectiveResolution(r float64) float64 {
	if r <= 0 {
		return 1.0
	}
	return r
}

// AddGraphAnalysis runs the graph-analysis pass (communities/cycles/layers/
// centrality) on an already-built catalog WITHOUT re-parsing. The catalog must
// already contain the refs table (built in full or source mode). This lets
// `refresh catalog communities` *augment* the existing catalog instead of
// rebuilding it — a rebuild downgrades a source-mode catalog to full and drops
// the source FTS data. The graph rows reuse the catalog's existing SnapshotId so
// the snapshot-framed views resolve.
func (c *Catalog) AddGraphAnalysis(resolution float64) error {
	resolution = effectiveResolution(resolution)
	var snapID string
	if err := c.db.QueryRow("SELECT SnapshotId FROM snapshots ORDER BY rowid DESC LIMIT 1").Scan(&snapID); err != nil {
		return fmt.Errorf("catalog has no snapshot (build full first): %w", err)
	}
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	b := &Builder{
		catalog:         c,
		snapshot:        &Snapshot{ID: snapID},
		communitiesMode: true,
		resolution:      resolution,
		rows:            newRowOwners(),
	}
	b.tx = newBuilderTx(tx, b)
	if err := b.buildGraphAnalysis(); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Record that the pass ran. An empty graph_cycles_data means "no cycles"
	// only once this is set; before it, it means "never computed" — and the two
	// were indistinguishable (mendixlabs/mxcli#1060). Written after the commit
	// so the flag can never claim a pass that was rolled back.
	if err := c.SetMeta(MetaGraphAnalysis, time.Now().Format(time.RFC3339)); err != nil {
		return err
	}
	return c.SetMeta(MetaGraphResolution, strconv.FormatFloat(resolution, 'g', -1, 64))
}

// buildGraphAnalysis runs the pure-Go graph algorithms over the refs graph and
// writes communities/cycles/layers/centrality. Only runs in communities mode; it
// reads the refs table built earlier in the same transaction, so no re-parse.
func (b *Builder) buildGraphAnalysis() error {
	if !b.communitiesMode {
		return nil
	}
	edges, err := b.loadGraphEdges()
	if err != nil {
		return err
	}
	for _, tbl := range []string{"communities_data", "graph_cycles_data", "graph_module_cycles_data", "graph_layers_data", "graph_centrality_data"} {
		if _, err := b.tx.Exec("DELETE FROM " + tbl); err != nil {
			return err
		}
	}
	// The module pass runs on its own edge set (every reference kind), so it must
	// not be gated on the STRUCTURAL one being non-empty — a project whose only
	// cross-module references are navigational has an empty asset graph and a
	// perfectly real module cycle, which is #1060's own Administration/Atlas_Core
	// pair.
	projectID, snapshotID := b.snapshotMeta()
	if err := b.buildModuleCycles(projectID, snapshotID); err != nil {
		return err
	}
	if len(edges) == 0 {
		return nil
	}

	g := graph.New(edges)
	resolution := effectiveResolution(b.resolution)
	// Communities.
	comm := g.Communities(resolution)
	commStmt, err := b.tx.Prepare(
		`INSERT INTO communities_data (AssetName, ModuleName, CommunityId, ProjectId, SnapshotId) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	commSet := map[int]struct{}{}
	for id := 0; id < g.N(); id++ {
		name := g.Name(id)
		if _, err := commStmt.Exec(name, moduleOf(name), comm[id], projectID, snapshotID); err != nil {
			commStmt.Close()
			return err
		}
		commSet[comm[id]] = struct{}{}
	}
	commStmt.Close()

	// Cycles (SCCs of size > 1, or self-loops).
	cycStmt, err := b.tx.Prepare(
		`INSERT INTO graph_cycles_data (AssetName, ModuleName, CycleId, CycleSize, ProjectId, SnapshotId) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for ci, comp := range g.Cycles() {
		for _, v := range comp {
			name := g.Name(v)
			if _, err := cycStmt.Exec(name, moduleOf(name), ci, len(comp), projectID, snapshotID); err != nil {
				cycStmt.Close()
				return err
			}
		}
	}
	cycStmt.Close()

	// Layers (topological sequence number).
	layers := g.Layers()
	layStmt, err := b.tx.Prepare(
		`INSERT INTO graph_layers_data (AssetName, ModuleName, Layer, ProjectId, SnapshotId) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for id := 0; id < g.N(); id++ {
		name := g.Name(id)
		if _, err := layStmt.Exec(name, moduleOf(name), layers[id], projectID, snapshotID); err != nil {
			layStmt.Close()
			return err
		}
	}
	layStmt.Close()

	// Centrality: PageRank always; betweenness when the graph is small enough.
	pr := g.PageRank(0.85, 100)
	var bt []float64
	if g.N() <= betweennessNodeCap {
		bt = g.Betweenness()
	}
	cenStmt, err := b.tx.Prepare(
		`INSERT INTO graph_centrality_data (AssetName, PageRank, Betweenness, ProjectId, SnapshotId) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for id := 0; id < g.N(); id++ {
		betw := 0.0
		if bt != nil {
			betw = bt[id]
		}
		if _, err := cenStmt.Exec(g.Name(id), pr[id], betw, projectID, snapshotID); err != nil {
			cenStmt.Close()
			return err
		}
	}
	cenStmt.Close()

	b.report("Graph communities", len(commSet))
	return nil
}

// buildModuleCycles finds the modules that depend on each other, directly or
// through a chain of other modules.
//
// Deliberately NOT derived from the asset-level SCCs above. Module A and module
// B are in a cycle when A references B and B references A at all — through any
// documents, which need form no cycle between themselves. That is the ordinary
// shape of a module cycle, and reading graph_cycles for it is what produced
// mendixlabs/mxcli#1060: the asset table was legitimately empty while
// graph_module_coupling listed both directions of the same pair.
//
// Over every reference kind, matching graph_module_coupling — the table this one
// is read beside — rather than the structural subset the clustering uses. A page
// bound to a layout in another module really is a dependency of that module.
func (b *Builder) buildModuleCycles(projectID, snapshotID string) error {
	edges, kinds, err := b.loadModuleEdges()
	if err != nil {
		return err
	}
	if len(edges) == 0 {
		return nil
	}
	g := graph.New(edges)
	stmt, err := b.tx.Prepare(
		`INSERT INTO graph_module_cycles_data (ModuleName, CycleId, CycleSize, RefKinds, ProjectId, SnapshotId) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for ci, comp := range g.Cycles() {
		// The modules of this cycle, for deciding which of a module's outgoing
		// kinds are the ones keeping the cycle alive.
		inCycle := make(map[string]bool, len(comp))
		for _, v := range comp {
			inCycle[g.Name(v)] = true
		}
		for _, v := range comp {
			name := g.Name(v)
			var involved []string
			for k := range kinds[name] {
				// Only edges that stay inside the cycle: a module's references to
				// the world outside it are not what makes it circular.
				if kinds[name][k] != nil {
					for target := range kinds[name][k] {
						if inCycle[target] {
							involved = append(involved, k)
							break
						}
					}
				}
			}
			sort.Strings(involved)
			if _, err := stmt.Exec(name, ci, len(comp), strings.Join(involved, ","), projectID, snapshotID); err != nil {
				return err
			}
		}
	}
	return nil
}

// loadModuleEdges reads module→module edges (all reference kinds, cross-module
// only) plus, per source module, which kinds reach which target module.
func (b *Builder) loadModuleEdges() ([]graph.Edge, map[string]map[string]map[string]bool, error) {
	stmt, err := b.tx.Prepare(
		`SELECT substr(SourceName, 1, instr(SourceName, '.') - 1) AS SourceModule,
			substr(TargetName, 1, instr(TargetName, '.') - 1) AS TargetModule,
			RefKind
		 FROM refs
		 WHERE instr(SourceName, '.') > 0 AND instr(TargetName, '.') > 0
			AND substr(SourceName, 1, instr(SourceName, '.') - 1)
				!= substr(TargetName, 1, instr(TargetName, '.') - 1)`)
	if err != nil {
		return nil, nil, err
	}
	defer stmt.Close()
	rows, err := stmt.Query()
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	var edges []graph.Edge
	kinds := map[string]map[string]map[string]bool{}
	for rows.Next() {
		var src, tgt, kind string
		if err := rows.Scan(&src, &tgt, &kind); err != nil {
			return nil, nil, err
		}
		if key := src + "\x00" + tgt; !seen[key] {
			seen[key] = true
			edges = append(edges, graph.Edge{Source: src, Target: tgt})
		}
		if kinds[src] == nil {
			kinds[src] = map[string]map[string]bool{}
		}
		if kinds[src][kind] == nil {
			kinds[src][kind] = map[string]bool{}
		}
		kinds[src][kind][tgt] = true
	}
	return edges, kinds, rows.Err()
}

// loadGraphEdges reads the structural edge set from the refs table.
func (b *Builder) loadGraphEdges() ([]graph.Edge, error) {
	stmt, err := b.tx.Prepare(
		`SELECT SourceName, TargetName FROM refs
		 WHERE SourceName != '' AND TargetName != '' AND RefKind IN (` + graphRefKindsSQL() + `)`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var edges []graph.Edge
	for rows.Next() {
		var s, t string
		if err := rows.Scan(&s, &t); err != nil {
			return nil, err
		}
		edges = append(edges, graph.Edge{Source: s, Target: t})
	}
	return edges, rows.Err()
}
