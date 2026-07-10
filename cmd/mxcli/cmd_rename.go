// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var renameCmd = &cobra.Command{
	Use:   "rename <type> <qualified-name> <new-name>",
	Short: "Rename a project element and update all references",
	Long: `Rename an element and automatically update all cross-references.

Types:
  entity         Rename an entity
  microflow      Rename a microflow
  nanoflow       Rename a nanoflow
  page           Rename a page
  enumeration    Rename an enumeration
  association    Rename an association
  constant       Rename a constant
  module         Rename a module (updates all qualified names)

Use --dry-run to preview changes without modifying.

Example:
  mxcli rename -p app.mpr entity MyModule.Customer Client
  mxcli rename -p app.mpr microflow MyModule.ACT_Old ACT_New
  mxcli rename -p app.mpr page MyModule.OldPage NewPage
  mxcli rename -p app.mpr module OldModule NewModule
  mxcli rename -p app.mpr entity MyModule.Customer Client --dry-run
`,
	Args: cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")

		objectType := strings.ToUpper(args[0])
		qualifiedName := args[1]
		newName := args[2]

		var mdlCmd string
		dryRunSuffix := ""
		if dryRun {
			dryRunSuffix = " DRY RUN"
		}

		switch objectType {
		case "ENTITY":
			mdlCmd = fmt.Sprintf("RENAME ENTITY %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "MICROFLOW":
			mdlCmd = fmt.Sprintf("RENAME MICROFLOW %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "NANOFLOW":
			mdlCmd = fmt.Sprintf("RENAME NANOFLOW %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "PAGE":
			mdlCmd = fmt.Sprintf("RENAME PAGE %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "ENUMERATION":
			mdlCmd = fmt.Sprintf("RENAME ENUMERATION %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "ASSOCIATION":
			mdlCmd = fmt.Sprintf("RENAME ASSOCIATION %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "CONSTANT":
			mdlCmd = fmt.Sprintf("RENAME CONSTANT %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "MODULE":
			mdlCmd = fmt.Sprintf("RENAME MODULE %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		default:
			fmt.Fprintf(os.Stderr, "Unknown type: %s\n", args[0])
			fmt.Fprintln(os.Stderr, "Valid types: entity, microflow, nanoflow, page, enumeration, association, constant, module")
			os.Exit(1)
		}

		exec, logger := newLoggedExecutor("subcommand")
		defer logger.Close()
		defer exec.Close()

		// Connect
		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s' FOR WRITING", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

		// Execute rename
		renameProg, errs := visitor.Build(mdlCmd)
		if len(errs) > 0 {
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
			}
			os.Exit(1)
		}

		for _, stmt := range renameProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

func init() {
	renameCmd.Flags().Bool("dry-run", false, "Preview changes without modifying")
	rootCmd.AddCommand(renameCmd)
}
