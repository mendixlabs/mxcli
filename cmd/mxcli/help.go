// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/syntax"
	"github.com/spf13/cobra"
)

var syntaxCmd = &cobra.Command{
	Use:   "syntax [topic [subtopic...]]",
	Short: "Show MDL syntax reference",
	Long: `Show MDL syntax reference from the feature registry.

Use --json for machine-readable output (optimized for LLM consumption).
Drill down with multiple arguments: mxcli syntax workflow user-task targeting

Top-level topics:
  domain-model    - Entities, associations, enumerations, constants, keywords, types
  microflow       - Microflow/nanoflow creation and activities
  page            - Pages, snippets, fragments, widgets
  security        - Roles, access control, demo users
  workflow        - Workflows, user tasks, decisions, parallel splits
  navigation      - Navigation profiles, menus, home pages
  settings        - Project settings
  integration     - OData, REST, SQL, OQL, XPath, Java actions, business events
  agents          - AI agent documents (Model, KB, Consumed MCP Service, Agent)
  errors          - Common validation errors and fixes
  structure       - SHOW STRUCTURE command
  move            - MOVE command for relocating documents
  search          - Full-text SEARCH command

Examples:
  mxcli syntax --json                          # Full index (LLM: cache this)
  mxcli syntax workflow --json                 # All workflow features
  mxcli syntax workflow user-task targeting     # Drill down to targeting
  mxcli syntax security entity-access           # Entity access rules
  mxcli syntax entity                           # Legacy alias → domain-model.entity
`,
	Run: func(cmd *cobra.Command, args []string) {
		jsonFlag, _ := cmd.Flags().GetBool("json")

		// No args: show full index (JSON) or help text
		if len(args) == 0 {
			if jsonFlag {
				syntax.WriteJSON(os.Stdout, syntax.All())
				return
			}
			cmd.Help()
			return
		}

		// Build registry path from args
		path := strings.ToLower(strings.Join(args, "."))

		// Apply aliases
		path = syntax.ResolveAlias(path)

		// Query registry
		if syntax.HasPrefix(path) {
			features := syntax.ByPrefix(path)
			if jsonFlag {
				syntax.WriteJSON(os.Stdout, features)
			} else {
				syntax.WriteText(os.Stdout, features)
			}
			return
		}

		fmt.Printf("Unknown topic: %s\n\n", path)
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(syntaxCmd)
}
