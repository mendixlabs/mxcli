// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"encoding/json"
	"fmt"

	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

// moduleOverviewData is the JSON output schema for the module overview ELK diagram.
type moduleOverviewData struct {
	Format  string               `json:"format"`
	Type    string               `json:"type"`
	Modules []moduleOverviewNode `json:"modules"`
	Edges   []moduleOverviewEdge `json:"edges"`
}

type moduleOverviewNode struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	IsSystem       bool   `json:"isSystem"`
	EntityCount    int    `json:"entityCount"`
	MicroflowCount int    `json:"microflowCount"`
	PageCount      int    `json:"pageCount"`
}

type moduleOverviewEdge struct {
	Source string         `json:"source"`
	Target string         `json:"target"`
	Count  int            `json:"count"`
	Kinds  map[string]int `json:"kinds"`
}

// systemModuleNames is the set of well-known system/marketplace modules.
var systemModuleNames = map[string]bool{
	"System":               true,
	"Administration":       true,
	"Atlas_Core":           true,
	"Atlas_Web_Content":    true,
	"Atlas_Native_Content": true,
	"MxModelReflection":    true,
	"CommunityCommons":     true,
}

// ModuleOverview generates a JSON graph of all project modules and their
// cross-module dependencies, suitable for rendering with ELK.js.
func ModuleOverview(ctx *ExecContext) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Ensure catalog is built with full mode for refs
	if err := ensureCatalog(ctx, true); err != nil {
		return mdlerrors.NewBackend("build catalog", err)
	}

	// Get all module names
	moduleResult, err := ctx.Catalog.Query("select Name from modules")
	if err != nil {
		return mdlerrors.NewBackend("query modules", err)
	}

	moduleNames := make(map[string]bool)
	for _, row := range moduleResult.Rows {
		if name, ok := row[0].(string); ok {
			moduleNames[name] = true
		}
	}

	// Get entity counts per module
	entityCounts := make(map[string]int)
	result, err := ctx.Catalog.Query("select ModuleName, count(*) from entities GROUP by ModuleName")
	if err == nil {
		for _, row := range result.Rows {
			if name, ok := row[0].(string); ok {
				entityCounts[name] = toInt(row[1])
			}
		}
	}

	// Get microflow counts per module
	mfCounts := make(map[string]int)
	result, err = ctx.Catalog.Query("select ModuleName, count(*) from microflows GROUP by ModuleName")
	if err == nil {
		for _, row := range result.Rows {
			if name, ok := row[0].(string); ok {
				mfCounts[name] = toInt(row[1])
			}
		}
	}

	// Get page counts per module
	pageCounts := make(map[string]int)
	result, err = ctx.Catalog.Query("select ModuleName, count(*) from pages GROUP by ModuleName")
	if err == nil {
		for _, row := range result.Rows {
			if name, ok := row[0].(string); ok {
				pageCounts[name] = toInt(row[1])
			}
		}
	}

	// Build module nodes
	var modules []moduleOverviewNode
	for name := range moduleNames {
		modules = append(modules, moduleOverviewNode{
			ID:             name,
			Name:           name,
			IsSystem:       systemModuleNames[name],
			EntityCount:    entityCounts[name],
			MicroflowCount: mfCounts[name],
			PageCount:      pageCounts[name],
		})
	}

	// Sort modules alphabetically for deterministic output
	sortModuleNodes(modules)

	// Query cross-module dependency edges from REFS
	edgeResult, err := ctx.Catalog.Query(`
		select
			SUBSTR(SourceName, 1, INSTR(SourceName, '.') - 1) as SourceModule,
			SUBSTR(TargetName, 1, INSTR(TargetName, '.') - 1) as TargetModule,
			RefKind,
			count(*) as RefCount
		from refs
		where INSTR(SourceName, '.') > 0 and INSTR(TargetName, '.') > 0
		GROUP by SourceModule, TargetModule, RefKind
		having SourceModule != TargetModule
	`)
	if err != nil {
		return mdlerrors.NewBackend("query refs", err)
	}

	// Aggregate edges by source/target pair
	type edgeKey struct {
		source, target string
	}
	edgeMap := make(map[edgeKey]*moduleOverviewEdge)
	for _, row := range edgeResult.Rows {
		src, _ := row[0].(string)
		tgt, _ := row[1].(string)
		kind, _ := row[2].(string)
		count := toInt(row[3])

		if src == "" || tgt == "" {
			continue
		}

		key := edgeKey{src, tgt}
		edge, ok := edgeMap[key]
		if !ok {
			edge = &moduleOverviewEdge{
				Source: src,
				Target: tgt,
				Kinds:  make(map[string]int),
			}
			edgeMap[key] = edge
		}
		edge.Kinds[kind] += count
		edge.Count += count
	}

	// Collect edges into slice, filtering out edges referencing unknown modules
	var edges []moduleOverviewEdge
	for _, edge := range edgeMap {
		if moduleNames[edge.Source] && moduleNames[edge.Target] {
			edges = append(edges, *edge)
		}
	}

	// Sort edges for deterministic output
	sortModuleEdges(edges)

	data := moduleOverviewData{
		Format:  "elk",
		Type:    "module-overview",
		Modules: modules,
		Edges:   edges,
	}

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mdlerrors.NewBackend("marshal json", err)
	}

	fmt.Fprint(ctx.Output, string(out))
	return nil
}

// toInt converts an interface{} value to int.
func toInt(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case int32:
		return int(n)
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

// sortModuleNodes sorts module nodes alphabetically by name.
func sortModuleNodes(nodes []moduleOverviewNode) {
	for i := 0; i < len(nodes)-1; i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i].Name > nodes[j].Name {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}
}

// sortModuleEdges sorts edges by source then target.
func sortModuleEdges(edges []moduleOverviewEdge) {
	for i := 0; i < len(edges)-1; i++ {
		for j := i + 1; j < len(edges); j++ {
			if edges[i].Source > edges[j].Source ||
				(edges[i].Source == edges[j].Source && edges[i].Target > edges[j].Target) {
				edges[i], edges[j] = edges[j], edges[i]
			}
		}
	}
}

// --- Executor method wrapper for backward compatibility ---

func (e *Executor) ModuleOverview() error {
	return ModuleOverview(e.newExecContext(context.Background()))
}
