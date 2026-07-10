// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

// frameworkModules are excluded from the report by default — they dominate the
// raw top-N with "expected" infrastructure (System.User, Atlas layouts) that is
// not actionable when understanding an app. --include-framework keeps them.
var frameworkModules = []string{"System", "Atlas_Core", "Atlas_Web_Content"}

var graphReportCmd = &cobra.Command{
	Use:   "graph-report",
	Short: "Architecture map of a Mendix project (god nodes, coupling, cohesion, dead code)",
	Long: `Render the project's dependency graph as a high-level architecture map: the
most-depended-upon "god nodes", cross-module coupling ("surprise edges"), module
cohesion, dead (unreferenced) documents, the reference-kind distribution, and the
entities used by the most flows.

Every section is a thin SELECT over the CATALOG.graph_* views, so it is also
reproducible directly (e.g. select * from CATALOG.graph_god_nodes). The command
runs a FULL catalog refresh (the graph views read the refs table).

Framework / marketplace modules (System, Atlas_Core, Atlas_Web_Content) are
excluded by default; pass --include-framework to keep them, or --exclude to drop
more.

Examples:
  mxcli graph-report -p app.mpr
  mxcli graph-report -p app.mpr --top 25
  mxcli graph-report -p app.mpr --format json -o graph.json
  mxcli graph-report -p app.mpr --include-framework`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		format := resolveFormat(cmd, "markdown")
		outputPath, _ := cmd.Flags().GetString("output")
		top, _ := cmd.Flags().GetInt("top")
		includeFramework, _ := cmd.Flags().GetBool("include-framework")
		exclude, _ := cmd.Flags().GetStringSlice("exclude")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		exec, logger := newLoggedExecutor("subcommand")
		defer logger.Close()
		defer exec.Close()
		exec.SetQuiet(true)

		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
				os.Exit(1)
			}
		}
		// The graph_* views read the refs table, which only FULL refresh populates.
		refreshProg, _ := visitor.Build("REFRESH CATALOG FULL")
		for _, stmt := range refreshProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error building catalog: %v\n", err)
				os.Exit(1)
			}
		}
		cat := exec.Catalog()
		if cat == nil {
			fmt.Fprintln(os.Stderr, "Error: catalog not built")
			os.Exit(1)
		}

		// notIn builds a "<expr> NOT IN ('a','b',...)" filter for the framework
		// (+ user-excluded) modules, or "" when framework is included.
		notIn := func(expr string) string {
			if includeFramework {
				return ""
			}
			mods := append(append([]string{}, frameworkModules...), exclude...)
			quoted := make([]string, len(mods))
			for i, m := range mods {
				quoted[i] = "'" + strings.ReplaceAll(m, "'", "''") + "'"
			}
			return " WHERE " + expr + " NOT IN (" + strings.Join(quoted, ", ") + ")"
		}
		moduleOf := "substr(Entity, 1, instr(Entity, '.') - 1)"

		sections := []graphSection{
			{"God nodes — most depended-upon / highest fan-out",
				"SELECT Asset, ObjectType, InDegree, OutDegree, Degree FROM graph_god_nodes" +
					notIn("ModuleName") + " ORDER BY Degree DESC LIMIT ?"},
			{"Module coupling — cross-module edges (surprise edges)",
				"SELECT SourceModule, TargetModule, Edges, RefKinds FROM graph_module_coupling ORDER BY Edges DESC LIMIT ?"},
			{"Module cohesion — lowest first (most entangled)",
				"SELECT ModuleName, IntraEdges, InterEdges, CohesionPct FROM graph_module_cohesion" +
					notIn("ModuleName") + " ORDER BY CohesionPct ASC LIMIT ?"},
			{"Dead documents — referenceable but no inbound edge",
				"SELECT QualifiedName, ObjectType, ModuleName FROM graph_dead_assets" +
					notIn("ModuleName") + " ORDER BY ObjectType, QualifiedName LIMIT ?"},
			{"Reference kinds — edge vocabulary",
				"SELECT RefKind, SourceType, TargetType, Count, Pct FROM graph_refkind_distribution ORDER BY Count DESC LIMIT ?"},
			{"Entity hotspots — used by the most flows",
				"SELECT Entity, UsedByFlows, AcrossModules FROM graph_entity_hotspots" +
					notIn(moduleOf) + " ORDER BY UsedByFlows DESC LIMIT ?"},
		}

		results := make([]graphSectionResult, 0, len(sections))
		for _, s := range sections {
			res, err := cat.Query(strings.Replace(s.query, "?", fmt.Sprintf("%d", top), 1))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error querying %q: %v\n", s.title, err)
				os.Exit(1)
			}
			results = append(results, graphSectionResult{Title: s.title, Columns: res.Columns, Rows: res.Rows})
		}

		var out string
		if format == "json" {
			out = renderGraphJSON(projectPath, results)
		} else {
			out = renderGraphMarkdown(projectPath, top, includeFramework, results)
		}

		if outputPath != "" {
			if err := os.WriteFile(outputPath, []byte(out), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", outputPath, err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Wrote %s\n", outputPath)
			return
		}
		fmt.Print(out)
	},
}

type graphSection struct {
	title string
	query string
}

type graphSectionResult struct {
	Title   string
	Columns []string
	Rows    [][]any
}

func renderGraphMarkdown(projectPath string, top int, includeFramework bool, sections []graphSectionResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Graph report — %s\n\n", projectPath)
	scope := "framework modules excluded"
	if includeFramework {
		scope = "framework modules included"
	}
	fmt.Fprintf(&b, "Architecture map from `CATALOG.graph_*` (top %d per section, %s).\n", top, scope)
	for _, s := range sections {
		fmt.Fprintf(&b, "\n## %s\n\n", s.Title)
		if len(s.Rows) == 0 {
			b.WriteString("_(none)_\n")
			continue
		}
		b.WriteString("| " + strings.Join(s.Columns, " | ") + " |\n")
		b.WriteString("|" + strings.Repeat("---|", len(s.Columns)) + "\n")
		for _, row := range s.Rows {
			cells := make([]string, len(row))
			for i, v := range row {
				cells[i] = strings.ReplaceAll(cellString(v), "|", "\\|")
			}
			b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		}
	}
	return b.String()
}

func renderGraphJSON(projectPath string, sections []graphSectionResult) string {
	type jsonSection struct {
		Title string           `json:"title"`
		Rows  []map[string]any `json:"rows"`
	}
	out := struct {
		Project  string        `json:"project"`
		Sections []jsonSection `json:"sections"`
	}{Project: projectPath}
	for _, s := range sections {
		js := jsonSection{Title: s.Title, Rows: make([]map[string]any, 0, len(s.Rows))}
		for _, row := range s.Rows {
			m := make(map[string]any, len(s.Columns))
			for i, col := range s.Columns {
				if i < len(row) {
					m[col] = row[i]
				}
			}
			js.Rows = append(js.Rows, m)
		}
		out.Sections = append(out.Sections, js)
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	return string(data) + "\n"
}

func cellString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
