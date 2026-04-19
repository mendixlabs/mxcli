// SPDX-License-Identifier: Apache-2.0

// mxcli is a command-line interface for working with Mendix projects using MDL syntax.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	mprbackend "github.com/mendixlabs/mxcli/mdl/backend/mpr"
	"github.com/mendixlabs/mxcli/mdl/diaglog"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/repl"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var (
	version   = "0.1.0"
	Version   = ""
	BuildTime = ""
)

const warningBanner = "WARNING: This is a vibe-coded PoC, alpha quality, use with caution.\n"

func main() {
	// Show warning banner unless --quiet, -q, --help, -h, or --version is passed
	if !shouldSuppressWarning() {
		fmt.Fprint(os.Stderr, warningBanner)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// shouldSuppressWarning checks if the warning should be suppressed
func shouldSuppressWarning() bool {
	// Check environment variable first (best for automated/CI usage)
	if os.Getenv("MXCLI_QUIET") != "" {
		return true
	}

	for _, arg := range os.Args[1:] {
		switch arg {
		case "-q", "--quiet", "-h", "--help", "--version", "-v":
			return true
		case "help", "version", "changelog":
			return true
		}
	}
	return false
}

// discoverProjectPath looks for a single .mpr file in the current directory.
// Returns the filename if exactly one is found, otherwise returns "".
func discoverProjectPath() string {
	entries, err := os.ReadDir(".")
	if err != nil {
		return ""
	}
	var mprFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".mpr") {
			mprFiles = append(mprFiles, e.Name())
		}
	}
	if len(mprFiles) == 1 {
		return mprFiles[0]
	}
	return ""
}

var rootCmd = &cobra.Command{
	Use:   "mxcli",
	Short: "Mendix CLI - Work with Mendix projects using MDL syntax",
	Long: `mxcli is a command-line interface for working with Mendix projects.

It supports MDL (Mendix Definition Language), a SQL-like syntax for
reading and modifying Mendix domain models.

Examples:
  # Get started with Claude Code in a Mendix project
  mxcli init /path/to/mendix-project; claude

  # Start interactive REPL
  mxcli

  # Execute MDL file
  mxcli exec script.mdl

  # Execute MDL commands directly
  mxcli -c "CONNECT LOCAL 'app.mpr'; SHOW ENTITIES;"

  # Connect to project and show entities
  mxcli -p app.mpr -c "SHOW ENTITIES"
`,
	Version: version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			if discovered := discoverProjectPath(); discovered != "" {
				_ = cmd.Flags().Set("project", discovered)
				fmt.Fprintf(os.Stderr, "Using project: %s\n", discovered)
			}
		}
		globalJSONFlag, _ = cmd.Flags().GetBool("json")
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Get flags
		commands, _ := cmd.Flags().GetString("command")
		projectPath, _ := cmd.Flags().GetString("project")

		if commands != "" {
			// Execute commands from -c flag
			exec, logger := newLoggedExecutor("batch")
			defer logger.Close()
			defer exec.Close()

			// Auto-connect if project specified
			if projectPath != "" {
				commands = fmt.Sprintf("CONNECT LOCAL '%s'; %s", projectPath, commands)
			}

			prog, errs := visitor.Build(commands)
			if len(errs) > 0 {
				for _, err := range errs {
					fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
				}
				os.Exit(1)
			}

			if err := exec.ExecuteProgram(prog); err != nil {
				if errors.Is(err, executor.ErrExit) {
					return
				}
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Start interactive REPL
			logger := diaglog.Init(version, "repl")
			defer logger.Close()

			r := repl.New(os.Stdin, os.Stdout)
			r.SetLogger(logger)
			defer r.Close()

			// Auto-connect if project specified
			if projectPath != "" {
				if err := r.ExecuteString(fmt.Sprintf("CONNECT LOCAL '%s';", projectPath)); err != nil {
					fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
				}
			}

			// Detect if stdin is a terminal or a pipe
			var err error
			if fi, statErr := os.Stdin.Stat(); statErr == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
				// Piped input: use quiet mode (no banner, no prompts)
				err = r.Run()
			} else {
				// Terminal: use readline with history, autocomplete
				err = r.RunWithReadline()
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

// globalJSONFlag is set by PersistentPreRun when --json is passed.
var globalJSONFlag bool

// resolveFormat returns the effective output format for a command.
// If the global --json flag is set and the command has a --format flag, it returns "json".
// Otherwise it returns the command's --format flag value (or the provided default).
func resolveFormat(cmd *cobra.Command, defaultFormat string) string {
	if globalJSONFlag {
		return "json"
	}
	if cmd.Flags().Lookup("format") != nil {
		f, _ := cmd.Flags().GetString("format")
		return f
	}
	return defaultFormat
}

// newLoggedExecutor creates an executor with diagnostics logging attached.
// The caller must call logger.Close() when done (safe on nil).
func newLoggedExecutor(mode string) (*executor.Executor, *diaglog.Logger) {
	logger := diaglog.Init(version, mode)
	exec := executor.New(os.Stdout)
	exec.SetBackendFactory(func() backend.FullBackend { return mprbackend.New() })
	exec.SetLogger(logger)
	if globalJSONFlag {
		exec.SetFormat(executor.FormatJSON)
	}
	return exec, logger
}

// executeMDL is a helper to execute MDL commands with a project.
func executeMDL(projectPath, mdlCmd string) {
	exec, logger := newLoggedExecutor("subcommand")
	defer logger.Close()
	defer exec.Close()

	fullCmd := fmt.Sprintf("CONNECT LOCAL '%s'; %s", projectPath, mdlCmd)
	prog, errs := visitor.Build(fullCmd)
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		}
		os.Exit(1)
	}

	if err := exec.ExecuteProgram(prog); err != nil {
		if errors.Is(err, executor.ErrExit) {
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	if Version != "" {
		version = Version
	}
	if BuildTime != "" {
		rootCmd.Version = version + " (" + BuildTime + ")"
	} else {
		rootCmd.Version = version
	}

	// Global flags
	rootCmd.PersistentFlags().StringP("project", "p", "", "Path to Mendix project (.mpr file)")
	rootCmd.PersistentFlags().Bool("json", false, "Output in JSON format")
	rootCmd.Flags().StringP("command", "c", "", "Execute MDL command(s) and exit")

	// Check command flags
	checkCmd.Flags().BoolP("references", "r", false, "Validate references against the project")
	checkCmd.Flags().String("format", "text", "Output format: text, json, sarif")

	// Diff command flags
	diffCmd.Flags().StringP("format", "f", "unified", "Output format: unified, side, struct")
	diffCmd.Flags().BoolP("color", "", false, "Use colored output")
	diffCmd.Flags().IntP("width", "w", 120, "Terminal width for side-by-side format")

	// Diff-local command flags
	diffLocalCmd.Flags().StringP("ref", "r", "HEAD", "Git ref or range (e.g., HEAD, main, main..feature)")
	diffLocalCmd.Flags().StringP("format", "f", "unified", "Output format: unified, side, struct")
	diffLocalCmd.Flags().BoolP("color", "", false, "Use colored output")
	diffLocalCmd.Flags().IntP("width", "w", 120, "Terminal width for side-by-side format")

	// Describe command flags
	describeCmd.Flags().StringP("format", "f", "mdl", "Output format: mdl, json, mermaid, elk")

	// Search command flags
	searchCmd.Flags().StringP("format", "f", "table", "Output format: table, names, json")
	searchCmd.Flags().BoolP("quiet", "q", false, "Suppress connection and status messages (for piping)")

	// Callers/callees command flags
	callersCmd.Flags().BoolP("transitive", "t", false, "Find transitive (indirect) callers")
	calleesCmd.Flags().BoolP("transitive", "t", false, "Find transitive (indirect) callees")

	// Structure command flags
	structureCmd.Flags().IntP("depth", "d", 2, "Detail level: 1=counts, 2=signatures, 3=types")
	structureCmd.Flags().StringP("module", "m", "", "Filter to specific module")
	structureCmd.Flags().Bool("all", false, "Include system/marketplace modules")

	// Context command flags
	contextCmd.Flags().IntP("depth", "d", 2, "Depth for call chain traversal")

	// Lint command flags
	lintCmd.Flags().StringP("format", "f", "text", "Output format: text, json, sarif")
	lintCmd.Flags().BoolP("color", "", false, "Use colored output")
	lintCmd.Flags().BoolP("list-rules", "l", false, "List available lint rules")
	lintCmd.Flags().StringSliceP("exclude", "e", nil, "Modules to exclude from linting")

	// Report command flags
	reportCmd.Flags().StringP("format", "f", "markdown", "Output format: markdown, json, html")
	reportCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
	reportCmd.Flags().StringSliceP("exclude", "e", nil, "Modules to exclude from report")

	// Test command flags
	testRunCmd.Flags().BoolP("list", "l", false, "List tests without executing")
	testRunCmd.Flags().StringP("junit", "j", "", "Write JUnit XML results to file")
	testRunCmd.Flags().BoolP("skip-build", "s", false, "Skip build step (reuse existing deployment)")
	testRunCmd.Flags().BoolP("verbose", "v", false, "Show all runtime log output")
	testRunCmd.Flags().BoolP("color", "", false, "Use colored output")
	testRunCmd.Flags().StringP("timeout", "t", "5m", "Timeout for runtime startup and test execution")

	// Eval command flags
	evalCheckCmd.Flags().StringP("test", "t", "", "Run only specific test ID")
	evalCheckCmd.Flags().BoolP("skip-mx-check", "", false, "Skip mx check validation")
	evalCheckCmd.Flags().StringP("output", "o", "", "Output directory for reports (default: no file output)")
	evalCheckCmd.Flags().StringP("mxcli-path", "", "", "Path to mxcli binary (default: self)")
	evalCheckCmd.Flags().BoolP("color", "", false, "Use colored output")
	evalCmd.AddCommand(evalCheckCmd)
	evalCmd.AddCommand(evalListCmd)

	// Add subcommands
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(describeCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(diffLocalCmd)
	rootCmd.AddCommand(callersCmd)
	rootCmd.AddCommand(calleesCmd)
	rootCmd.AddCommand(refsCmd)
	rootCmd.AddCommand(impactCmd)
	rootCmd.AddCommand(structureCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(lintCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(lspCmd)
	rootCmd.AddCommand(projectTreeCmd)
	rootCmd.AddCommand(diagCmd)
	rootCmd.AddCommand(testRunCmd)
	rootCmd.AddCommand(playwrightCmd)
	rootCmd.AddCommand(evalCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(fmtCmd)
}
