// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/testrunner"
	"github.com/spf13/cobra"
)

var testRunCmd = &cobra.Command{
	Use:   "test <file|dir> [file|dir...]",
	Short: "Run MDL tests against a Mendix project",
	Long: `Run microflow tests defined in .test.mdl or .test.md files.

Tests use MDL syntax with javadoc-style annotations for expectations:

  /**
   * @test String concatenation
   * @expect $result = 'John Doe'
   */
  $result = CALL MICROFLOW MyModule.ConcatNames(
    FirstName = 'John', LastName = 'Doe'
  );
  /

The test runner:
1. Parses test files and extracts test blocks with @test/@expect annotations
2. Generates a TestRunner microflow
3. Injects it into the project as after-startup microflow
4. Builds and restarts the Mendix runtime in Docker
5. Captures structured log output to determine pass/fail
6. Restores original project settings

Supports two file formats:
  .test.mdl  — Pure MDL test blocks separated by /
  .test.md   — Markdown specification with embedded mdl-test code blocks

Examples:
  # Run tests from a test file
  mxcli test tests/microflows.test.mdl -p app.mpr

  # Run all tests in a directory
  mxcli test tests/ -p app.mpr

  # Output JUnit XML for CI
  mxcli test tests/ -p app.mpr --junit results.xml

  # List tests without executing
  mxcli test tests/ -p app.mpr --list

  # Skip build (reuse existing deployment)
  mxcli test tests/ -p app.mpr --skip-build

  # Verbose output (show all runtime logs)
  mxcli test tests/ -p app.mpr --verbose
`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		list, _ := cmd.Flags().GetBool("list")
		junitOutput, _ := cmd.Flags().GetString("junit")
		skipBuild, _ := cmd.Flags().GetBool("skip-build")
		verbose, _ := cmd.Flags().GetBool("verbose")
		color, _ := cmd.Flags().GetBool("color")
		timeoutStr, _ := cmd.Flags().GetString("timeout")

		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid timeout: %v\n", err)
			os.Exit(1)
		}

		if list {
			// Just list tests, no execution needed
			if err := testrunner.ListTests(args, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}

		// Execution requires a project
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required for test execution")
			os.Exit(1)
		}

		opts := testrunner.RunOptions{
			ProjectPath: projectPath,
			TestFiles:   args,
			SkipBuild:   skipBuild,
			Timeout:     timeout,
			JUnitOutput: junitOutput,
			Verbose:     verbose,
			Color:       color,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}

		result, err := testrunner.Run(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if !result.AllPassed() {
			os.Exit(1)
		}
	},
}
