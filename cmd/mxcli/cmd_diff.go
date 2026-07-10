// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff <script.mdl>",
	Short: "Compare an MDL script against the current project state",
	Long: `Compare an MDL script file against the current state of a Mendix project.

Shows the differences between what the script would create/modify and what
currently exists in the project.

Output Formats:
  unified  - Traditional unified diff format (default)
  side     - Side-by-side comparison
  struct   - Structural changes summary

Examples:
  # Unified diff (default)
  mxcli diff -p app.mpr changes.mdl

  # Side-by-side diff
  mxcli diff -p app.mpr changes.mdl --format side

  # Structural diff
  mxcli diff -p app.mpr changes.mdl --format struct

  # With color output
  mxcli diff -p app.mpr changes.mdl --color
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		projectPath, _ := cmd.Flags().GetString("project")
		format, _ := cmd.Flags().GetString("format")
		useColor, _ := cmd.Flags().GetBool("color")
		width, _ := cmd.Flags().GetInt("width")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		// Read the script file
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}

		// Parse the script
		prog, errs := visitor.Build(string(content))
		if len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "Syntax errors found:\n")
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "  - %v\n", err)
			}
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

		// Run diff
		opts := executor.DiffOptions{
			Format:   executor.DiffFormat(format),
			UseColor: useColor,
			Width:    width,
		}

		if err := exec.DiffProgram(prog, opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var diffLocalCmd = &cobra.Command{
	Use:   "diff-local",
	Short: "Compare local changes against git",
	Long: `Compare local (uncommitted) changes in mxunit files against a git reference.

This command finds modified mxunit files in the mprcontents/ folder and shows
the differences as MDL. Only works with MPR v2 format (Mendix 10.18+).

The --ref flag accepts any git ref or range (e.g., HEAD, main, main..feature-branch).

Examples:
  # Show uncommitted changes vs HEAD
  mxcli diff-local -p app.mpr

  # Compare against a specific commit
  mxcli diff-local -p app.mpr --ref HEAD~1

  # Compare against a branch
  mxcli diff-local -p app.mpr --ref main

  # Compare two arbitrary revisions (git range syntax)
  mxcli diff-local -p app.mpr --ref main..feature-branch

  # Three-dot range (changes since common ancestor)
  mxcli diff-local -p app.mpr --ref main...feature-branch

  # With structural format
  mxcli diff-local -p app.mpr --format struct --color
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		ref, _ := cmd.Flags().GetString("ref")
		format, _ := cmd.Flags().GetString("format")
		useColor, _ := cmd.Flags().GetBool("color")
		width, _ := cmd.Flags().GetInt("width")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		// Default ref to HEAD
		if ref == "" {
			ref = "HEAD"
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

		// Run diff-local
		opts := executor.DiffOptions{
			Format:   executor.DiffFormat(format),
			UseColor: useColor,
			Width:    width,
		}

		if err := exec.DiffLocal(ref, opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}
