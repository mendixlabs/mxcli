// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
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
	Long: `Find all references to the specified element (entity, microflow, page, etc.,
or an attribute Module.Entity.Attribute, an enumeration, or an enumeration
value Module.Enum.Value). Each (source, kind) is listed once.

Examples:
  mxcli refs -p app.mpr Module.Customer
  mxcli refs -p app.mpr Module.OrderPage
  mxcli refs -p app.mpr Module.Customer.Email
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
The summary counts distinct elements. For an enumeration, the uses of its values are
included. When nothing is found for an attribute or an enumeration value, the message
says which usage sites were checked: one named only through a variable in a free-text
expression ($Order/Total) is not resolved, so run 'mxcli search' before deleting.

Examples:
  mxcli impact -p app.mpr Module.Customer
  mxcli impact -p app.mpr Module.OrderStatus
  mxcli impact -p app.mpr Module.Customer.Email
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

		mdlCmd := "DESCRIBE STRUCTURE"
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
The MDL sources are written in mdl 1, each headed by "mdl 1;"; --mdl 0
writes them in the alpha language instead.

With --json the assembled context is wrapped in one JSON object,
{"name", "type", "depth", "context"}, where "context" is the markdown text
above; progress goes to stderr.

Examples:
  mxcli context -p app.mpr Module.ProcessOrder
  mxcli context -p app.mpr Module.Customer --depth 3
  mxcli context -p app.mpr Module.OrderPage
  mxcli context -p app.mpr Module.ImportCsvData
  mxcli context -p app.mpr Module.Customer --json | jq -r .context
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		depth, _ := cmd.Flags().GetInt("depth")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		mdlCmd := fmt.Sprintf("DESCRIBE CONTEXT OF %s", args[0])
		if depth > 0 {
			mdlCmd += fmt.Sprintf(" DEPTH %d", depth)
		}
		lang := mdlFlag(cmd)
		executeMDL(projectPath, mdlCmd, func(e *executor.Executor) { e.SetDescribeLanguage(lang) })
	},
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the project catalog using full-text search",
	Long: `Search the project catalog for matching strings and source code.

Searches across string literals (captions, labels, messages), the documentation
of project elements, and MDL source definitions. Requires at least a FULL
catalog build (done automatically).

Besides captions, labels and messages, it searches the documentation of every
element the catalog records (entities, attributes, constants, pages, microflows,
Java actions, REST services, ... and a module's domain model documentation).
Not searched: the documentation of an enumeration value (Mendix has no such
field) and the summary text of a remote service's contract.
A hit names the element (qualified name, type) and the field: the context
column says "documentation" for a documentation hit.

MDL source is searched only once the source index exists — build it with
  mxcli -p app.mpr -c "refresh catalog full source"
Until then a warning on stderr says only string literals were searched.

Output Formats:
  table   - Human-readable table (default)
  names   - Just qualified names, one per line (for piping)
  --json  - JSON array on stdout, progress on stderr (the same flag every
            query command takes). "--format json" is still accepted as a
            deprecated spelling of it.

Gating on a word:
  --fail-on-match   exit 1 when there is at least one hit, 0 when there is none
                    (results are printed as usual, JSON included). A query the
                    index cannot run, or any other error, exits 2 so that 1
                    always means "something matched".

Marketplace modules:
  --exclude-marketplace
                    leave out hits inside modules downloaded from the
                    Marketplace. Off by default: every module is searched.

Examples:
  mxcli search -p app.mpr "validation"
  mxcli search -p app.mpr "Customer" --json | jq '.[].qualifiedName'
  mxcli search -p app.mpr "Customer" --format names
  mxcli search -p app.mpr "error" -q --format names | xargs -I {} mxcli describe -p app.mpr microflow {}
  mxcli search -p app.mpr "TODO" --fail-on-match
  mxcli search -p app.mpr "TODO" --exclude-marketplace
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		format := resolveFormat(cmd, "table")
		quiet, _ := cmd.Flags().GetBool("quiet")
		failOnMatch, _ := cmd.Flags().GetBool("fail-on-match")
		excludeMarketplace, _ := cmd.Flags().GetBool("exclude-marketplace")

		// Under --fail-on-match, exit status 1 means "something matched", so a
		// search that could not run says so with 2 rather than with 1.
		errorExit := 1
		if failOnMatch {
			errorExit = 2
		}

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(errorExit)
		}

		// A JSON-format executor sends "Connected to:" and catalog progress to
		// stderr (ExecContext.progress). Set from the resolved format, not
		// from --json alone: `--format json` is the older spelling of the same
		// request and must be exactly as clean.
		exec, logger := newLoggedExecutor("subcommand")
		defer logger.Close()
		if isMachineReadableFormat(format) {
			exec.SetFormat(executor.FormatJSON)
		}
		if quiet {
			exec.SetQuiet(true)
		}
		defer exec.Close()

		// Connect to project
		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
				os.Exit(errorExit)
			}
		}

		// Execute search with format option
		query := strings.ReplaceAll(args[0], "'", "''")
		hits, err := exec.SearchWith(query, format, executor.SearchOptions{
			ExcludeMarketplace: excludeMarketplace,
			// A gate must not read "the index refused the query" as "no match".
			Strict: failOnMatch,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(errorExit)
		}
		if failOnMatch && hits > 0 {
			// os.Exit skips the deferred closes, and the log file would stay open.
			exec.Close()
			logger.Close()
			os.Exit(1)
		}
	},
}
