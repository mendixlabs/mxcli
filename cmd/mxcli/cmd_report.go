// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a best practices report for a Mendix project",
	Long: `Generate a scored report evaluating a Mendix project against best practice
conventions. The report includes category scores, recommendations, and
detailed findings.

For reports that answer one question rather than score, use a kind:
  mxcli report architecture   module layers, coupling, bounded contexts, cycles
  mxcli report tests          test results and measured microflow coverage
  mxcli report security       access grid, caller-trusting flows, unsafe defaults
  mxcli report performance    journeys at volume: time per step, plans, before/after
  mxcli report traceability   requirements per milestone: built, partial, proven

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
  mxcli report -p app.mpr --modules Sales,Orders

--modules scores only the named modules — a project's own modules, without the
Marketplace and platform modules beside them. Only findings located in a
selected module count; project-wide findings (project security, user-role
mappings) are left out, and the report names the selection.
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		format := resolveFormat(cmd, "markdown")
		outputPath, _ := cmd.Flags().GetString("output")
		excludeModules, _ := cmd.Flags().GetStringSlice("exclude")
		moduleFilter, _ := cmd.Flags().GetStringSlice("modules")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		// Create executor and connect
		// With -o the payload goes to a file and stdout is free for progress.
		// Without it the report itself is on stdout, so progress must not be
		// (the markdown/json/html payload would be corrupted by it).
		reportProgress := progressSink(format)
		if outputPath != "" {
			reportProgress = os.Stdout
		}
		exec, logger := newLoggedExecutorTo("subcommand", reportProgress)
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
		if len(moduleFilter) > 0 {
			ctx.SetIncludedModules(moduleFilter)
		}

		// The rule set and the config come from the same two helpers `mxcli
		// lint` uses. This command used to build both itself: its inline copy of
		// the built-in list had fallen a rule behind (MDL-FLOW01), and it never
		// read lint-config.yaml at all — so a rule a team had deliberately
		// accepted and disabled still scored against them, and the score could
		// not be moved by any configuration (ako/mxcli#525). A score computed
		// from a different rule set than the listing that explains it is worse
		// than no score.
		projectDir := filepath.Dir(projectPath)
		lint := linter.New(ctx)
		for _, rule := range projectLintRules(projectDir, os.Stderr) {
			lint.AddRule(rule)
		}
		if cfg, configPath := applyLintConfig(lint, projectDir, os.Stderr); cfg != nil && len(cfg.ExcludeModules) > 0 {
			ctx.SetExcludedModules(append(excludeModules, cfg.ExcludeModules...))
			// The exclude wins, as in `lint`: say so rather than score an
			// empty selection 100.
			if shadowed := intersect(moduleFilter, cfg.ExcludeModules); len(shadowed) > 0 {
				fmt.Fprintf(os.Stderr,
					"Warning: --modules names %s, but %s excluded by %s — no findings will be scored for %s. Remove it from excludeModules to score it.\n",
					strings.Join(shadowed, ", "), pluralIsAre(len(shadowed)), configPath, pluralItThem(len(shadowed)))
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

		// Build report
		// --modules: score the selection only (ako/mxcli#953).
		report := linter.BuildReport(
			projectName,
			time.Now().Format("2006-01-02 15:04:05"),
			linter.ScopeToModules(violations, moduleFilter),
		)
		report.Modules = moduleFilter

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
