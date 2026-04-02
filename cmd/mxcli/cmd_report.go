// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/linter/rules"
	"github.com/mendixlabs/mxcli/mdl/visitor"
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
		format, _ := cmd.Flags().GetString("format")
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

		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", projectPath))
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
		ctx := linter.NewLintContext(cat)
		ctx.SetExcludedModules(excludeModules)

		// Set reader so rules that inspect raw BSON work
		if reader := exec.Reader(); reader != nil {
			ctx.SetReader(reader)
		}

		// Create linter and register all rules
		lint := linter.New(ctx)

		// Built-in Go rules
		lint.AddRule(rules.NewNamingConventionRule())
		lint.AddRule(rules.NewEmptyMicroflowRule())
		lint.AddRule(rules.NewDomainModelSizeRule())
		lint.AddRule(rules.NewValidationFeedbackRule())
		lint.AddRule(rules.NewImageSourceRule())
		lint.AddRule(rules.NewEmptyContainerRule())
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

		// Build report
		report := linter.BuildReport(
			projectName,
			time.Now().Format("2006-01-02 15:04:05"),
			violations,
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
