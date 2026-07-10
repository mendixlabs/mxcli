// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/evalrunner"
	"github.com/spf13/cobra"
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluate project against acceptance criteria",
	Long: `Evaluation framework for testing mxcli + Claude Code against structured test definitions.

Eval test files use Markdown with YAML frontmatter to define:
  - User prompt (what was asked)
  - Automated checks (entity_exists, page_exists, etc.)
  - Acceptance criteria (human-readable expectations)
  - Iteration scenarios (follow-up prompts)

Subcommands:
  check    Validate a project against eval criteria
  list     List all eval tests in a directory

Examples:
  # Validate a project against eval criteria
  mxcli eval check docs/14-eval/eval-1.md -p app.mpr

  # Run specific test from a directory
  mxcli eval check docs/14-eval/ -p app.mpr --test APP-001

  # List all eval tests
  mxcli eval list docs/14-eval/
`,
}

var evalCheckCmd = &cobra.Command{
	Use:   "check <file|dir>",
	Short: "Validate a project against eval criteria",
	Long: `Run automated checks from eval test definitions against a Mendix project.

Checks include:
  - entity_exists: Verify entity with matching name exists
  - entity_has_attribute: Verify entity has attribute with expected type
  - page_exists: Verify page with matching name exists
  - page_has_widget: Verify page contains expected widget type
  - microflow_exists: Verify microflow with matching name exists
  - navigation_has_item: Verify navigation menu has items
  - mx_check_passes: Run mx check validation
  - lint_passes: Run mxcli lint

Pattern matching: *.Book matches MyModule.Book, *Overview* matches Book_Overview.

Examples:
  mxcli eval check docs/14-eval/eval-1.md -p app.mpr
  mxcli eval check docs/14-eval/ -p app.mpr --test APP-001
  mxcli eval check docs/14-eval/eval-1.md -p app.mpr --skip-mx-check
  mxcli eval check docs/14-eval/eval-1.md -p app.mpr --output eval-results/
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		testID, _ := cmd.Flags().GetString("test")
		skipMxCheck, _ := cmd.Flags().GetBool("skip-mx-check")
		outputDir, _ := cmd.Flags().GetString("output")
		mxcliPath, _ := cmd.Flags().GetString("mxcli-path")
		color, _ := cmd.Flags().GetBool("color")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		// Resolve mxcli path: use self if not specified
		if mxcliPath == "" {
			self, err := os.Executable()
			if err == nil {
				mxcliPath = self
			} else {
				mxcliPath = "mxcli"
			}
		}

		// Parse eval tests
		path := args[0]
		var tests []*evalrunner.EvalTest

		fi, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if fi.IsDir() {
			tests, err = evalrunner.ParseEvalDir(path)
		} else {
			var test *evalrunner.EvalTest
			test, err = evalrunner.ParseEvalFile(path)
			if err == nil {
				tests = []*evalrunner.EvalTest{test}
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing eval tests: %v\n", err)
			os.Exit(1)
		}

		// Filter by test ID if specified
		if testID != "" {
			var filtered []*evalrunner.EvalTest
			for _, t := range tests {
				if t.ID == testID {
					filtered = append(filtered, t)
				}
			}
			if len(filtered) == 0 {
				fmt.Fprintf(os.Stderr, "Error: no eval test with ID %q found\n", testID)
				os.Exit(1)
			}
			tests = filtered
		}

		if len(tests) == 0 {
			fmt.Fprintln(os.Stderr, "Error: no eval tests found")
			os.Exit(1)
		}

		// Set up check options
		checkOpts := evalrunner.CheckOptions{
			ProjectPath: projectPath,
			MxCliPath:   mxcliPath,
			SkipMxCheck: skipMxCheck,
		}

		// Run checks for each test
		runStart := time.Now()
		summary := &evalrunner.RunSummary{
			Timestamp: runStart,
		}

		allPassed := true
		for _, test := range tests {
			result := runEvalTest(test, checkOpts, color)
			summary.Results = append(summary.Results, *result)

			if result.OverallScore < 1.0 {
				allPassed = false
			}
		}
		summary.Duration = time.Since(runStart)

		// Print summary if multiple tests
		if len(tests) > 1 {
			evalrunner.PrintSummary(os.Stdout, summary, color)
		}

		// Write reports if output directory specified
		if outputDir != "" {
			// Create timestamped subdirectory
			runDir := filepath.Join(outputDir, fmt.Sprintf("run-%s", runStart.Format("2006-01-02T15-04-05")))

			for _, result := range summary.Results {
				if err := evalrunner.WriteJSONReport(&result, runDir); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write JSON report: %v\n", err)
				}
			}

			if err := evalrunner.WriteRunSummary(summary, runDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write summary: %v\n", err)
			}

			if err := evalrunner.WriteMarkdownReport(summary, runDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write markdown report: %v\n", err)
			}

			fmt.Fprintf(os.Stdout, "Reports written to %s\n", runDir)
		}

		if !allPassed {
			os.Exit(1)
		}
	},
}

// runEvalTest runs all checks for a single eval test and returns the result.
func runEvalTest(test *evalrunner.EvalTest, opts evalrunner.CheckOptions, color bool) *evalrunner.EvalResult {
	start := time.Now()

	result := &evalrunner.EvalResult{
		TestID:    test.ID,
		Category:  test.Category,
		Title:     test.Title,
		Timestamp: start,
		Criteria:  test.Criteria,
		Initial: evalrunner.PhaseResult{
			Phase: "initial",
		},
	}

	// Run initial checks
	fmt.Fprintf(os.Stdout, "Running checks for %s...\n", test.ID)
	result.Initial.Checks = evalrunner.RunChecks(test.Checks, opts)
	result.Initial.ComputeScore()

	// Run iteration checks if present
	if test.Iteration != nil && len(test.Iteration.Checks) > 0 {
		iterResult := &evalrunner.PhaseResult{
			Phase: "iteration",
		}
		iterResult.Checks = evalrunner.RunChecks(test.Iteration.Checks, opts)
		iterResult.ComputeScore()
		result.Iteration = iterResult
	}

	result.Duration = time.Since(start)
	result.ComputeOverallScore()

	// Print result
	evalrunner.PrintResult(os.Stdout, result, color)

	return result
}

var evalListCmd = &cobra.Command{
	Use:   "list <file|dir>",
	Short: "List eval tests",
	Long: `List all evaluation tests found in a file or directory.

Shows test ID, category, number of checks, and whether an iteration scenario exists.

Examples:
  mxcli eval list docs/14-eval/
  mxcli eval list docs/14-eval/eval-1.md
`,
	Args: cobra.RangeArgs(0, 1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			_ = cmd.Help()
			return
		}
		path := args[0]
		var tests []*evalrunner.EvalTest
		var err error

		fi, statErr := os.Stat(path)
		if statErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", statErr)
			os.Exit(1)
		}

		if fi.IsDir() {
			tests, err = evalrunner.ParseEvalDir(path)
		} else {
			var test *evalrunner.EvalTest
			test, err = evalrunner.ParseEvalFile(path)
			if err == nil {
				tests = []*evalrunner.EvalTest{test}
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(tests) == 0 {
			fmt.Fprintln(os.Stdout, "No eval tests found.")
			return
		}

		fmt.Fprintf(os.Stdout, "%-12s %-15s %8s %10s  %s\n", "ID", "Category", "Checks", "Iteration", "Title")
		fmt.Fprintln(os.Stdout, strings.Repeat("-", 70))

		for _, t := range tests {
			iterStr := "—"
			if t.Iteration != nil {
				iterStr = fmt.Sprintf("%d checks", len(t.Iteration.Checks))
			}

			fmt.Fprintf(os.Stdout, "%-12s %-15s %8d %10s  %s\n",
				t.ID,
				t.Category,
				len(t.Checks),
				iterStr,
				t.Title,
			)
		}

		fmt.Fprintf(os.Stdout, "\n%d eval test(s) found.\n", len(tests))
	},
}
