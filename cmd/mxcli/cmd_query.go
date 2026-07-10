// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var callersCmd = &cobra.Command{
	Use:   "callers <qualified-name>",
	Short: "Find callers of a microflow",
	Long: `Find all microflows that call the specified microflow.

Use --transitive to find the full call chain (indirect callers).

Examples:
  mxcli callers -p app.mpr Module.ValidateOrder
  mxcli callers -p app.mpr Module.ProcessOrder --transitive
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		transitive, _ := cmd.Flags().GetBool("transitive")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		mdlCmd := fmt.Sprintf("SHOW CALLERS OF %s", args[0])
		if transitive {
			mdlCmd += " TRANSITIVE"
		}

		executeMDL(projectPath, mdlCmd)
	},
}

var calleesCmd = &cobra.Command{
	Use:   "callees <qualified-name>",
	Short: "Find callees of a microflow",
	Long: `Find all microflows called by the specified microflow.

Use --transitive to find the full call chain (indirect callees).

Examples:
  mxcli callees -p app.mpr Module.ProcessOrder
  mxcli callees -p app.mpr Module.MainFlow --transitive
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		transitive, _ := cmd.Flags().GetBool("transitive")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		mdlCmd := fmt.Sprintf("SHOW CALLEES OF %s", args[0])
		if transitive {
			mdlCmd += " TRANSITIVE"
		}

		executeMDL(projectPath, mdlCmd)
	},
}

var refsCmd = &cobra.Command{
	Use:   "refs <qualified-name>",
	Short: "Find references to an element",
	Long: `Find all references to the specified element (entity, microflow, page, etc.).

Examples:
  mxcli refs -p app.mpr Module.Customer
  mxcli refs -p app.mpr Module.OrderPage
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		mdlCmd := fmt.Sprintf("SHOW REFERENCES TO %s", args[0])
		executeMDL(projectPath, mdlCmd)
	},
}

var impactCmd = &cobra.Command{
	Use:   "impact <qualified-name>",
	Short: "Show impact of changing an element",
	Long: `Analyze the impact of changing an element by showing all elements that reference it.

Examples:
  mxcli impact -p app.mpr Module.Customer
  mxcli impact -p app.mpr Module.OrderStatus
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		mdlCmd := fmt.Sprintf("SHOW IMPACT OF %s", args[0])
		executeMDL(projectPath, mdlCmd)
	},
}

var structureCmd = &cobra.Command{
	Use:   "structure",
	Short: "Show compact project structure overview",
	Long: `Show a compact, token-efficient overview of the project structure.

Depth levels:
  1  Module summary with element counts
  2  Elements with signatures (default)
  3  Full types on attributes and named parameters

By default, system and marketplace modules are excluded.
Use --all to include them.

Examples:
  mxcli structure -p app.mpr
  mxcli structure -p app.mpr -d 1
  mxcli structure -p app.mpr -d 3 -m CRM
  mxcli structure -p app.mpr --all
`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		depth, _ := cmd.Flags().GetInt("depth")
		module, _ := cmd.Flags().GetString("module")
		all, _ := cmd.Flags().GetBool("all")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		mdlCmd := "SHOW STRUCTURE"
		if depth != 2 {
			mdlCmd += fmt.Sprintf(" DEPTH %d", depth)
		}
		if module != "" {
			mdlCmd += fmt.Sprintf(" IN %s", module)
		}
		if all {
			mdlCmd += " ALL"
		}
		executeMDL(projectPath, mdlCmd)
	},
}

var contextCmd = &cobra.Command{
	Use:   "context <qualified-name>",
	Short: "Assemble context for an element (for LLM consumption)",
	Long: `Assemble relevant context information about an element for LLM consumption.

Supported element types:
  microflows     definition, entities used, pages shown, called microflows, callers
  nanoflows      same as microflows
  entities       definition, microflows using it, pages displaying it, related entities
  pages          definition, entities used, microflows called, what shows it
  workflows      definition, activities, user tasks, called microflows, callers
  enumerations   definition and usage
  snippets       definition, pages that use it
  java actions   definition, microflows that call it
  OData clients  service info, external entities
  OData services service info, published entities

Use --depth to control how deep to traverse call chains (default: 2).

Examples:
  mxcli context -p app.mpr Module.ProcessOrder
  mxcli context -p app.mpr Module.Customer --depth 3
  mxcli context -p app.mpr Module.OrderPage
  mxcli context -p app.mpr Module.ImportCsvData
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		depth, _ := cmd.Flags().GetInt("depth")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		mdlCmd := fmt.Sprintf("SHOW CONTEXT OF %s", args[0])
		if depth > 0 {
			mdlCmd += fmt.Sprintf(" DEPTH %d", depth)
		}
		executeMDL(projectPath, mdlCmd)
	},
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the project catalog using full-text search",
	Long: `Search the project catalog for matching strings and source code.

Searches across string literals (captions, labels, messages) and MDL source
definitions. Requires at least a FULL catalog build (done automatically).

Output Formats:
  table   - Human-readable table (default)
  names   - Just qualified names, one per line (for piping)
  json    - JSON output

Examples:
  mxcli search -p app.mpr "validation"
  mxcli search -p app.mpr "Customer" --format names
  mxcli search -p app.mpr "error" -q --format names | xargs -I {} mxcli describe -p app.mpr microflow {}
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		format := resolveFormat(cmd, "table")
		quiet, _ := cmd.Flags().GetBool("quiet")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		exec, logger := newLoggedExecutor("subcommand")
		defer logger.Close()
		if quiet {
			exec.SetQuiet(true)
		}
		defer exec.Close()

		// Connect to project
		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
				os.Exit(1)
			}
		}

		// Execute search with format option
		query := strings.ReplaceAll(args[0], "'", "''")
		if err := exec.Search(query, format); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}
