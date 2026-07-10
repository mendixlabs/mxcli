// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"html/template"
	"math"
	"net/http"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/spf13/cobra"
)

// ModuleStats holds statistics for a module for visualization.
type ModuleStats struct {
	Name           string
	EntityCount    int
	MicroflowCount int
	NanoflowCount  int
	PageCount      int
	SnippetCount   int
	LayoutCount    int
	EnumCount      int
	TotalDocs      int
	Color          string
}

// TreemapRect represents a rectangle in the treemap.
type TreemapRect struct {
	X, Y, W, H float64
	Module     *ModuleStats
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start HTTP server with project visualization",
	Long: `Start an HTTP server that serves a visualization of the Mendix project.

The visualization shows a treemap of modules sized by the number of documents
(entities, microflows, nanoflows, pages, snippets, layouts, enumerations) in each module.

Examples:
  mxcli serve -p app.mpr
  mxcli serve -p app.mpr --port 8080
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		port, _ := cmd.Flags().GetInt("port")

		if projectPath == "" {
			fmt.Println("Error: --project (-p) is required")
			return
		}

		fmt.Printf("Starting visualization server for: %s\n", projectPath)
		fmt.Printf("Open http://localhost:%d in your browser\n", port)

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			serveTreemap(w, r, projectPath)
		})
		http.HandleFunc("/svg", func(w http.ResponseWriter, r *http.Request) {
			serveSVG(w, r, projectPath)
		})

		if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
			fmt.Printf("Server error: %v\n", err)
		}
	},
}

func buildCatalog(projectPath string) (*catalog.Catalog, error) {
	reader, err := mpr.Open(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open project: %w", err)
	}
	defer reader.Close()

	cat, err := catalog.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create catalog: %w", err)
	}

	builder := catalog.NewBuilder(cat, reader)
	if err := builder.Build(nil); err != nil {
		return nil, fmt.Errorf("failed to build catalog: %w", err)
	}

	return cat, nil
}

func serveTreemap(w http.ResponseWriter, r *http.Request, projectPath string) {
	cat, err := buildCatalog(projectPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer cat.Close()

	modules, err := getModuleStats(cat)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get module stats: %v", err), 500)
		return
	}

	width, height := 1200.0, 800.0
	rects := generateTreemap(modules, 0, 0, width, height)

	renderTreemapHTML(w, rects, width, height, projectPath)
}

func serveSVG(w http.ResponseWriter, r *http.Request, projectPath string) {
	cat, err := buildCatalog(projectPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer cat.Close()

	modules, err := getModuleStats(cat)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get module stats: %v", err), 500)
		return
	}

	width, height := 1200.0, 800.0
	rects := generateTreemap(modules, 0, 0, width, height)

	w.Header().Set("Content-Type", "image/svg+xml")
	renderSVG(w, rects, width, height)
}

func getModuleStats(cat *catalog.Catalog) ([]*ModuleStats, error) {
	moduleMap := make(map[string]*ModuleStats)

	// Helper to query counts by counting in Go (catalog doesn't support GROUP BY)
	queryModuleCounts := func(table string) map[string]int {
		counts := make(map[string]int)
		result, err := cat.Query(fmt.Sprintf("SELECT ModuleName FROM %s", table))
		if err != nil {
			return counts
		}
		// Find module name column index
		moduleIdx := -1
		for i, col := range result.Columns {
			if col == "ModuleName" {
				moduleIdx = i
				break
			}
		}
		if moduleIdx < 0 {
			return counts
		}
		// Count occurrences of each module
		for _, row := range result.Rows {
			name := fmt.Sprintf("%v", row[moduleIdx])
			counts[name]++
		}
		return counts
	}

	// Query all document types (use lowercase table names as that's what SQLite uses)
	entityCounts := queryModuleCounts("entities")
	microflowCounts := queryModuleCounts("microflows")
	nanoflowCounts := queryModuleCounts("nanoflows")
	pageCounts := queryModuleCounts("pages")
	snippetCounts := queryModuleCounts("snippets")
	layoutCounts := queryModuleCounts("layouts")
	enumCounts := queryModuleCounts("enumerations")

	// Collect all module names
	allModules := make(map[string]bool)
	for name := range entityCounts {
		allModules[name] = true
	}
	for name := range microflowCounts {
		allModules[name] = true
	}
	for name := range nanoflowCounts {
		allModules[name] = true
	}
	for name := range pageCounts {
		allModules[name] = true
	}
	for name := range snippetCounts {
		allModules[name] = true
	}
	for name := range layoutCounts {
		allModules[name] = true
	}
	for name := range enumCounts {
		allModules[name] = true
	}

	// Build module stats
	for name := range allModules {
		moduleMap[name] = &ModuleStats{
			Name:           name,
			EntityCount:    entityCounts[name],
			MicroflowCount: microflowCounts[name],
			NanoflowCount:  nanoflowCounts[name],
			PageCount:      pageCounts[name],
			SnippetCount:   snippetCounts[name],
			LayoutCount:    layoutCounts[name],
			EnumCount:      enumCounts[name],
		}
	}

	// Calculate totals and assign colors
	colors := []string{
		"#4e79a7", "#f28e2c", "#e15759", "#76b7b2", "#59a14f",
		"#edc949", "#af7aa1", "#ff9da7", "#9c755f", "#bab0ab",
		"#6b9ac4", "#d48c35", "#c94e4e", "#8bc9c5", "#6db86d",
	}

	var modules []*ModuleStats
	for _, m := range moduleMap {
		m.TotalDocs = m.EntityCount + m.MicroflowCount + m.NanoflowCount +
			m.PageCount + m.SnippetCount + m.LayoutCount + m.EnumCount
		if m.TotalDocs > 0 {
			modules = append(modules, m)
		}
	}

	// Sort by size descending
	sort.Slice(modules, func(i, j int) bool {
		return modules[i].TotalDocs > modules[j].TotalDocs
	})

	// Assign colors
	for i, m := range modules {
		m.Color = colors[i%len(colors)]
	}

	return modules, nil
}

func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

// generateTreemap creates a squarified treemap layout.
func generateTreemap(modules []*ModuleStats, x, y, w, h float64) []TreemapRect {
	if len(modules) == 0 {
		return nil
	}

	total := 0
	for _, m := range modules {
		total += m.TotalDocs
	}

	if total == 0 {
		return nil
	}

	return squarify(modules, x, y, w, h, float64(total))
}

func squarify(modules []*ModuleStats, x, y, w, h, total float64) []TreemapRect {
	if len(modules) == 0 {
		return nil
	}

	if len(modules) == 1 {
		return []TreemapRect{{X: x, Y: y, W: w, H: h, Module: modules[0]}}
	}

	vertical := w >= h
	var rects []TreemapRect
	remaining := modules

	for len(remaining) > 0 {
		row, rest := layoutRow(remaining, x, y, w, h, total, vertical)

		rowTotal := 0.0
		for _, m := range row {
			rowTotal += float64(m.TotalDocs)
		}

		if vertical {
			rowW := (rowTotal / total) * w
			rowY := y
			for _, m := range row {
				mH := (float64(m.TotalDocs) / rowTotal) * h
				rects = append(rects, TreemapRect{X: x, Y: rowY, W: rowW, H: mH, Module: m})
				rowY += mH
			}
			x += rowW
			w -= rowW
		} else {
			rowH := (rowTotal / total) * h
			rowX := x
			for _, m := range row {
				mW := (float64(m.TotalDocs) / rowTotal) * w
				rects = append(rects, TreemapRect{X: rowX, Y: y, W: mW, H: rowH, Module: m})
				rowX += mW
			}
			y += rowH
			h -= rowH
		}

		total -= rowTotal
		remaining = rest
		vertical = w >= h
	}

	return rects
}

func layoutRow(modules []*ModuleStats, x, y, w, h, total float64, vertical bool) ([]*ModuleStats, []*ModuleStats) {
	if len(modules) <= 1 {
		return modules, nil
	}

	side := w
	if !vertical {
		side = h
	}

	var row []*ModuleStats
	rowTotal := 0.0
	bestRatio := math.MaxFloat64

	for i, m := range modules {
		rowTotal += float64(m.TotalDocs)
		row = append(row, m)

		rowW := (rowTotal / total) * side
		if rowW == 0 {
			continue
		}

		worstRatio := 0.0
		for _, rm := range row {
			rmSize := float64(rm.TotalDocs) / rowTotal
			var ratio float64
			if vertical {
				ratio = aspectRatio(rowW, rmSize*h)
			} else {
				ratio = aspectRatio(rmSize*w, rowW)
			}
			if ratio > worstRatio {
				worstRatio = ratio
			}
		}

		if worstRatio > bestRatio && i > 0 {
			return row[:len(row)-1], modules[i:]
		}
		bestRatio = worstRatio
	}

	return row, nil
}

func aspectRatio(w, h float64) float64 {
	if w == 0 || h == 0 {
		return math.MaxFloat64
	}
	r := w / h
	if r < 1 {
		return 1 / r
	}
	return r
}

func renderSVG(w http.ResponseWriter, rects []TreemapRect, width, height float64) {
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="%v" height="%v" viewBox="0 0 %v %v">
<style>
.module { stroke: #fff; stroke-width: 2; }
.module:hover { opacity: 0.8; }
.label { font-family: sans-serif; font-size: 12px; font-weight: 600; fill: #fff; }
.stats { font-family: sans-serif; font-size: 10px; fill: rgba(255,255,255,0.9); }
</style>
`, width, height, width, height)

	for _, rect := range rects {
		m := rect.Module
		tooltip := fmt.Sprintf("%s\nEntities: %d\nMicroflows: %d\nNanoflows: %d\nPages: %d\nSnippets: %d\nLayouts: %d\nEnumerations: %d\nTotal: %d",
			m.Name, m.EntityCount, m.MicroflowCount, m.NanoflowCount, m.PageCount, m.SnippetCount, m.LayoutCount, m.EnumCount, m.TotalDocs)

		fmt.Fprintf(w, `<rect class="module" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"><title>%s</title></rect>
`, rect.X, rect.Y, rect.W, rect.H, m.Color, template.HTMLEscapeString(tooltip))

		// Only show labels if rectangle is large enough
		if rect.W > 60 && rect.H > 40 {
			fmt.Fprintf(w, `<text class="label" x="%.1f" y="%.1f">%s</text>
`, rect.X+8, rect.Y+20, template.HTMLEscapeString(m.Name))
			fmt.Fprintf(w, `<text class="stats" x="%.1f" y="%.1f">%d docs</text>
`, rect.X+8, rect.Y+35, m.TotalDocs)
		}
	}

	fmt.Fprintf(w, `</svg>`)
}

func renderTreemapHTML(w http.ResponseWriter, rects []TreemapRect, width, height float64, projectPath string) {
	// Extract project name from path
	projectName := projectPath
	if idx := strings.LastIndex(projectPath, "/"); idx >= 0 {
		projectName = projectPath[idx+1:]
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>%s - Project Visualization</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
        h1 { color: #333; margin-bottom: 5px; }
        .subtitle { color: #666; margin-bottom: 20px; }
        svg { background: white; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); display: block; }
        .module { stroke: #fff; stroke-width: 2; cursor: pointer; }
        .module:hover { opacity: 0.8; }
        .label { font-family: sans-serif; font-size: 12px; font-weight: 600; fill: #fff; pointer-events: none; }
        .stats { font-family: sans-serif; font-size: 10px; fill: rgba(255,255,255,0.9); pointer-events: none; }
        .legend { margin-top: 20px; display: flex; flex-wrap: wrap; gap: 10px; }
        .legend-item { display: flex; align-items: center; gap: 6px; font-size: 13px; }
        .legend-color { width: 14px; height: 14px; border-radius: 3px; }
    </style>
</head>
<body>
    <h1>%s</h1>
    <p class="subtitle">Module treemap - sized by document count (hover for details)</p>
    <svg width="%v" height="%v" viewBox="0 0 %v %v">
`, template.HTMLEscapeString(projectName), template.HTMLEscapeString(projectName), width, height, width, height)

	for _, rect := range rects {
		m := rect.Module
		tooltip := fmt.Sprintf("%s\nEntities: %d\nMicroflows: %d\nNanoflows: %d\nPages: %d\nSnippets: %d\nLayouts: %d\nEnumerations: %d\nTotal: %d",
			m.Name, m.EntityCount, m.MicroflowCount, m.NanoflowCount, m.PageCount, m.SnippetCount, m.LayoutCount, m.EnumCount, m.TotalDocs)

		fmt.Fprintf(w, `        <rect class="module" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"><title>%s</title></rect>
`, rect.X, rect.Y, rect.W, rect.H, m.Color, template.HTMLEscapeString(tooltip))

		if rect.W > 60 && rect.H > 40 {
			fmt.Fprintf(w, `        <text class="label" x="%.1f" y="%.1f">%s</text>
`, rect.X+8, rect.Y+20, template.HTMLEscapeString(m.Name))
			fmt.Fprintf(w, `        <text class="stats" x="%.1f" y="%.1f">%d docs</text>
`, rect.X+8, rect.Y+35, m.TotalDocs)
		}
	}

	fmt.Fprintf(w, `    </svg>
    <div class="legend">
`)

	for _, rect := range rects {
		m := rect.Module
		fmt.Fprintf(w, `        <div class="legend-item"><div class="legend-color" style="background:%s"></div>%s (%d)</div>
`, m.Color, template.HTMLEscapeString(m.Name), m.TotalDocs)
	}

	fmt.Fprintf(w, `    </div>
</body>
</html>`)
}

func init() {
	serveCmd.Flags().IntP("port", "", 8080, "Port to listen on")
}
