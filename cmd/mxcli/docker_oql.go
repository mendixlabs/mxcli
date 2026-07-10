// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

var oqlCmd = &cobra.Command{
	Use:   "oql [query]",
	Short: "Execute an OQL query against a running Mendix runtime",
	Long: `Execute an OQL query against a running Mendix application using the M2EE admin API.
The query runs in preview/rollback mode (read-only).

By default, when -p is set, the request is routed through "docker compose exec"
to reach the container's admin API (which binds to localhost inside the container).
Use --direct to bypass docker exec and connect via HTTP directly.

Connection settings are resolved in order: flags > environment variables > .docker/.env > defaults.

Examples:
  # Query with project path (reads .docker/.env for credentials)
  mxcli oql -p app.mpr "SELECT Name, Email FROM MyModule.Customer"

  # JSON output for piping
  mxcli oql -p app.mpr --json "SELECT count(c.ID) FROM MyModule.Order AS c" | jq '.[0]'

  # Direct HTTP connection (when admin API is reachable from host)
  mxcli oql --direct --host localhost --port 8090 --token 'AdminPassword1!' "SELECT 1"
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := args[0]
		projectPath, _ := cmd.Flags().GetString("project")
		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetInt("port")
		token, _ := cmd.Flags().GetString("token")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		direct, _ := cmd.Flags().GetBool("direct")

		opts := docker.OQLOptions{
			Host:        host,
			Port:        port,
			Token:       token,
			ProjectPath: projectPath,
			Direct:      direct,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}

		result, err := docker.ExecuteOQL(opts, query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if jsonOutput {
			if err := docker.FormatOQLJSON(os.Stdout, result); err != nil {
				fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
				os.Exit(1)
			}
		} else {
			docker.FormatOQLTable(os.Stdout, result)
		}

		fmt.Fprintf(os.Stderr, "(%d rows)\n", len(result.Rows))
	},
}

func init() {
	oqlCmd.Flags().String("host", "", "Mendix admin API host (default: localhost)")
	oqlCmd.Flags().Int("port", 0, "Mendix admin API port (default: 8090)")
	oqlCmd.Flags().String("token", "", "M2EE admin password")
	oqlCmd.Flags().BoolP("json", "j", false, "Output as JSON array")
	oqlCmd.Flags().Bool("direct", false, "Connect via HTTP directly (bypass docker exec)")

	rootCmd.AddCommand(oqlCmd)
}
