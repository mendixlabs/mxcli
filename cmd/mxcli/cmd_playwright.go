// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/playwright"
	"github.com/spf13/cobra"
)

var playwrightCmd = &cobra.Command{
	Use:   "playwright",
	Short: "Browser testing with playwright-cli",
	Long:  `Commands for running browser-based verification tests using playwright-cli.`,
}

var playwrightVerifyCmd = &cobra.Command{
	Use:   "verify <file|dir> [file|dir...]",
	Short: "Run playwright-cli test scripts against a running Mendix app",
	Long: `Run .test.sh scripts that use playwright-cli to verify a Mendix application.

Test scripts are plain bash files using playwright-cli commands. Each script
runs sequentially, and a non-zero exit code marks the script as failed.
On failure, a screenshot is automatically captured for debugging.

Script naming convention: tests/verify-<name>.test.sh

Examples:
  # Run all test scripts in a directory
  mxcli playwright verify tests/ -p app.mpr

  # Run a specific script
  mxcli playwright verify tests/verify-customers.test.sh

  # Output JUnit XML for CI
  mxcli playwright verify tests/ -p app.mpr --junit results.xml

  # List scripts without executing
  mxcli playwright verify tests/ --list

  # Verbose output (show script stdout/stderr)
  mxcli playwright verify tests/ -p app.mpr --verbose

  # Custom app URL
  mxcli playwright verify tests/ --base-url http://localhost:9090

  # Leave the browser open so the next run reuses a warm, logged-in session
  # (the generate -> verify -> fix loop)
  mxcli playwright verify tests/ -p app.mpr --keep-open
`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		list, _ := cmd.Flags().GetBool("list")
		junitOutput, _ := cmd.Flags().GetString("junit")
		verbose, _ := cmd.Flags().GetBool("verbose")
		color, _ := cmd.Flags().GetBool("color")
		skipHealth, _ := cmd.Flags().GetBool("skip-health-check")
		keepOpen, _ := cmd.Flags().GetBool("keep-open")
		timeoutStr, _ := cmd.Flags().GetString("timeout")
		projectPath, _ := cmd.Flags().GetString("project")

		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid timeout: %v\n", err)
			os.Exit(1)
		}

		baseURL := resolveBaseURL(cmd, projectPath)

		if list {
			if err := playwright.ListScripts(args, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}

		opts := playwright.VerifyOptions{
			ProjectPath:     projectPath,
			TestFiles:       args,
			BaseURL:         baseURL,
			Timeout:         timeout,
			JUnitOutput:     junitOutput,
			Color:           color,
			Verbose:         verbose,
			SkipHealthCheck: skipHealth,
			KeepOpen:        keepOpen,
			Stdout:          os.Stdout,
			Stderr:          os.Stderr,
		}

		result, err := playwright.Verify(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if !result.AllPassed() {
			os.Exit(1)
		}
	},
}

var playwrightOpenCmd = &cobra.Command{
	Use:   "open [url]",
	Short: "Open (or attach to) the browser session for interactive use",
	Long: `Launch a playwright-cli browser session, or attach to a live one on the same
origin, so you can drive the app across separate commands (and reuse it with
'mxcli playwright verify --keep-open').

The URL is resolved from the positional argument, else --base-url, else APP_PORT
in .docker/.env, else http://localhost:8080.

Examples:
  mxcli playwright open -p app.mpr
  mxcli playwright open http://localhost:9090`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		baseURL := resolveBaseURL(cmd, projectPath)
		if len(args) == 1 {
			baseURL = args[0]
		}
		if err := playwright.OpenSession(playwright.SessionOptions{
			BaseURL:     baseURL,
			ProjectPath: projectPath,
			Stdout:      os.Stdout,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var playwrightStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report whether a browser session is live and what page it is on",
	Run: func(cmd *cobra.Command, args []string) {
		if err := playwright.SessionStatus(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var playwrightCloseCmd = &cobra.Command{
	Use:   "close",
	Short: "Close the current browser session",
	Run: func(cmd *cobra.Command, args []string) {
		all, _ := cmd.Flags().GetBool("all")
		if err := playwright.CloseSession(all, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	playwrightVerifyCmd.Flags().BoolP("list", "l", false, "List test scripts without executing")
	playwrightVerifyCmd.Flags().StringP("junit", "j", "", "Write JUnit XML results to file")
	playwrightVerifyCmd.Flags().BoolP("verbose", "v", false, "Show script stdout/stderr")
	playwrightVerifyCmd.Flags().BoolP("color", "", false, "Use colored output")
	playwrightVerifyCmd.Flags().StringP("timeout", "t", "2m", "Timeout per script execution")
	playwrightVerifyCmd.Flags().StringP("base-url", "", "http://localhost:8080", "Mendix app base URL")
	playwrightVerifyCmd.Flags().BoolP("skip-health-check", "", false, "Skip app reachability check")
	playwrightVerifyCmd.Flags().BoolP("keep-open", "", false, "Leave the browser session open after the run so the next verify reuses it")

	playwrightOpenCmd.Flags().StringP("base-url", "", "http://localhost:8080", "Mendix app base URL")
	playwrightCloseCmd.Flags().BoolP("all", "", false, "Close all browser sessions")

	playwrightCmd.AddCommand(playwrightVerifyCmd)
	playwrightCmd.AddCommand(playwrightOpenCmd)
	playwrightCmd.AddCommand(playwrightStatusCmd)
	playwrightCmd.AddCommand(playwrightCloseCmd)
}

// resolveBaseURL resolves the Mendix app base URL for playwright commands:
// an explicit --base-url wins, otherwise APP_PORT from .docker/.env (relative to
// the project), otherwise the default localhost:8080. Shared by verify and open.
func resolveBaseURL(cmd *cobra.Command, projectPath string) string {
	baseURL, _ := cmd.Flags().GetString("base-url")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	if !cmd.Flags().Changed("base-url") && projectPath != "" {
		if port := readAppPort(projectPath); port != "" {
			baseURL = fmt.Sprintf("http://localhost:%s", port)
		}
	}
	return baseURL
}

// readAppPort reads APP_PORT from .docker/.env relative to the project file.
// Returns empty string if the file doesn't exist or APP_PORT is not set.
func readAppPort(projectPath string) string {
	envPath := filepath.Join(filepath.Dir(projectPath), ".docker", ".env")
	f, err := os.Open(envPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if strings.HasPrefix(line, "APP_PORT=") {
			return strings.TrimPrefix(line, "APP_PORT=")
		}
	}
	return ""
}
