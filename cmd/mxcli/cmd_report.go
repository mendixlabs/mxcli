// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter/rules"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a best practices report for a Mendix project",
	Long: `Generate a scored report evaluating a Mendix project against best practice
conventions. The report includes category scores, recommendations, and
detailed findings.

Output formats:
  - markdown (default): Human-readable Markdown with tables and progress bars
  - json: Machine-readable structured output
  - html: Standalone HTML with embedded CSS

The report runs a FULL catalog refresh (required for comprehensive analysis)
and executes all built-in and Starlark lint rules.

Examples:
  mxcli report -p app.mpr
  mxcli report -p app.mpr --format json
  mxcli report -p app.mpr --format html --output report.html
  mxcli report -p app.mpr --format markdown --output report.md
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		format := resolveFormat(cmd, "markdown")
		outputPath, _ := cmd.Flags().GetString("output")
		excludeModules, _ := cmd.Flags().GetStringSlice("exclude")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		// Create executor and connect
		exec, logger := newLoggedExecutor("subcommand")
		defer logger.Close()
		defer exec.Close()

		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
				os.Exit(1)
			}
		}

		// Build FULL catalog (report needs comprehensive data)
		refreshCmd := "REFRESH CATALOG FULL"
		refreshProg, _ := visitor.Build(refreshCmd)
		for _, stmt := range refreshProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error building catalog: %v\n", err)
				os.Exit(1)
			}
		}

		// Get catalog from executor
		cat := exec.Catalog()
		if cat == nil {
			fmt.Fprintln(os.Stderr, "Error: catalog not built")
			os.Exit(1)
		}

		// Create lint context
		ctx := linter.NewLintContext(cat, exec.Backend())
		ctx.SetExcludedModules(excludeModules)

		// Create linter and register all rules
		lint := linter.New(ctx)

		// Built-in Go rules
		lint.AddRule(rules.NewNamingConventionRule())
		lint.AddRule(rules.NewEmptyMicroflowRule())
		lint.AddRule(rules.NewDomainModelSizeRule())
		lint.AddRule(rules.NewValidationFeedbackRule())
		lint.AddRule(rules.NewImageSourceRule())
		lint.AddRule(rules.NewEmptyContainerRule())
		lint.AddRule(rules.NewGallerySelectionListenerRule())
		lint.AddRule(rules.NewDataViewLayoutGridRule())
		lint.AddRule(rules.NewPageNavigationSecurityRule())
		lint.AddRule(rules.NewNoEntityAccessRulesRule())
		lint.AddRule(rules.NewWeakPasswordPolicyRule())
		lint.AddRule(rules.NewDemoUsersActiveRule())

		// MPR008 - requires BSON inspection
		lint.AddRule(rules.NewOverlappingActivitiesRule())

		// Convention rules (CONV011-CONV014)
		lint.AddRule(rules.NewNoCommitInLoopRule())
		lint.AddRule(rules.NewExclusiveSplitCaptionRule())
		lint.AddRule(rules.NewErrorHandlingOnCallsRule())
		lint.AddRule(rules.NewNoContinueErrorHandlingRule())

		// Load Starlark rules (includes CONV001-010, CONV015-017)
		projectDir := filepath.Dir(projectPath)
		lintRulesDir := filepath.Join(projectDir, ".claude", "lint-rules")
		if starlarkRules, err := linter.LoadStarlarkRulesFromDir(lintRulesDir); err == nil {
			for _, rule := range starlarkRules {
				lint.AddRule(rule)
			}
		}

		// Run all rules
		violations, err := lint.Run(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running linter: %v\n", err)
			os.Exit(1)
		}

		// Derive project name from path
		projectName := filepath.Base(projectPath)
		projectName = projectName[:len(projectName)-len(filepath.Ext(projectName))]

		// Build report. Categories are auto-detected from the rules that actually
		// ran (see linter.CategoriesFromRules); elementCounts sums COUNT(*) over
		// each category's "checkable universe" of catalog tables, so violation
		// density is normalized against project size instead of raw counts.
		elementCounts := categoryElementCounts(cat)

		if raw, _ := cmd.Flags().GetBool("raw"); raw {
			stats := linter.BuildRawStats(violations, lint.Rules(), elementCounts)
			payload := struct {
				Project    string                   `json:"project"`
				Categories []linter.RawCategoryStat `json:"categories"`
			}{Project: projectName, Categories: stats}

			var w io.Writer = os.Stdout
			if outputPath != "" {
				f, err := os.Create(outputPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
					os.Exit(1)
				}
				defer f.Close()
				w = f
			}
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			if err := enc.Encode(payload); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing raw stats: %v\n", err)
				os.Exit(1)
			}
			return
		}

		report := linter.BuildReport(
			projectName,
			time.Now().Format("2006-01-02 15:04:05"),
			violations,
			lint.Rules(),
			elementCounts,
		)

		// Format and output
		formatter := linter.GetReportFormatter(format)

		var writer *os.File
		if outputPath != "" {
			var err error
			writer, err = os.Create(outputPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
				os.Exit(1)
			}
			defer writer.Close()
		} else {
			writer = os.Stdout
		}

		if err := formatter.FormatReport(report, writer); err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting report: %v\n", err)
			os.Exit(1)
		}

		if outputPath != "" {
			fmt.Fprintf(os.Stderr, "Report written to %s\n", outputPath)
		}
	},
}

// categoryElementTables maps a report category (title-cased, matching
// linter.CategoriesFromRules output) to the catalog tables that represent its
// "checkable universe" — the elements its rules actually inspect. Used to
// normalize violation density (see normalizationBaseline in mdl/linter/report.go).
//
// This list is a best-effort approximation of what each category's rules look
// at (mdl/linter/rules/*.go and .claude/lint-rules/*.star); a category with no
// entry here (e.g. a custom Starlark CATEGORY) gets an elementsChecked of 0 and
// falls back to the flat, unnormalized penalty.
var categoryElementTables = map[string][]string{
	"Naming":       {"entities_data", "attributes_data", "associations_data", "microflows_data", "pages_data", "enumerations_data", "snippets_data"},
	"Security":     {"entities_data", "pages_data", "permissions", "role_mappings"},
	"Quality":      {"microflows_data", "activities_data", "entities_data", "attributes_data"},
	"Design":       {"entities_data", "pages_data", "widgets_data", "attributes_data"},
	"Correctness":  {"pages_data", "widgets_data", "microflows_data", "activities_data"},
	"Performance":  {"activities_data", "attributes_data", "entities_data"},
	"Architecture": {"modules_data", "entities_data", "microflows_data", "associations_data", "pages_data"},
	"Complexity":   {"microflows_data", "activities_data"},
}

// categoryElementCounts sums COUNT(*) over each category's checkable-universe
// tables in the catalog, so report.go can normalize each category's penalty by
// project size rather than scoring a 5-entity app and a 500-entity app the same
// way for an identical violation count.
func categoryElementCounts(cat *catalog.Catalog) map[string]int {
	counts := make(map[string]int, len(categoryElementTables))
	for category, tables := range categoryElementTables {
		total := 0
		for _, table := range tables {
			res, err := cat.Query(fmt.Sprintf("SELECT COUNT(*) FROM %s", table))
			if err != nil || len(res.Rows) == 0 {
				continue
			}
			if n, ok := res.Rows[0][0].(int64); ok {
				total += int(n)
			}
		}
		counts[category] = total
	}
	return counts
}
