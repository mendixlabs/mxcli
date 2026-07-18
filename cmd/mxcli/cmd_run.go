// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a Mendix app locally in a warm dev loop",
	Long: `Run a Mendix app with a warm, Docker-free dev loop (--local).

'mxcli run --local' keeps a mxbuild --serve process and a standalone Mendix
runtime hot. The first build is cold (~10-15s); after that a model change
rebuilds incrementally (~1s) and is applied without a full restart:

  - page / microflow / text change  -> hot reload_model (no restart)
  - entity / view / association      -> runtime restart (metamodel is
                                        reconciled only at startup)

The serve build reports which is needed, so the right action is chosen
automatically. With --watch, mxcli rebuilds and hot-applies on every change.

Requirements:
  - Mendix 11.x project (JDK 21; version-aware JDK selection is a follow-up)
  - A reachable PostgreSQL (the devcontainer provides one); the database must
    already exist. Defaults: 127.0.0.1:5432, user 'mendix', db from the project
    name. Override with --db-host/--db-name/--db-user/--db-password.

Examples:
  mxcli run --local -p app.mpr
  mxcli run --local -p app.mpr --watch
  mxcli run --local -p app.mpr --app-port 8081 --db-name myapp
`,
	Run: func(cmd *cobra.Command, args []string) {
		local, _ := cmd.Flags().GetBool("local")
		if !local {
			fmt.Fprintln(os.Stderr, "Error: only --local is supported for now (use 'mxcli docker run' for the container workflow)")
			os.Exit(1)
		}
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		watch, _ := cmd.Flags().GetBool("watch")
		ensureDB, _ := cmd.Flags().GetBool("ensure-db")
		setupOnly, _ := cmd.Flags().GetBool("setup")
		appPort, _ := cmd.Flags().GetInt("app-port")
		adminPort, _ := cmd.Flags().GetInt("admin-port")
		servePort, _ := cmd.Flags().GetInt("serve-port")
		dbHost, _ := cmd.Flags().GetString("db-host")
		dbName, _ := cmd.Flags().GetString("db-name")
		dbUser, _ := cmd.Flags().GetString("db-user")
		dbPassword, _ := cmd.Flags().GetString("db-password")
		screenshot, _ := cmd.Flags().GetBool("screenshot")
		screenshotPath, _ := cmd.Flags().GetString("screenshot-path")
		screenshotURLs, _ := cmd.Flags().GetStringArray("screenshot-url")
		screenshotUser, _ := cmd.Flags().GetString("screenshot-user")
		screenshotPassword, _ := cmd.Flags().GetString("screenshot-password")

		opts := docker.LocalRunOptions{
			ProjectPath:        projectPath,
			AppPort:            appPort,
			AdminPort:          adminPort,
			ServePort:          servePort,
			Watch:              watch,
			EnsureDB:           ensureDB,
			SetupOnly:          setupOnly,
			Screenshot:         screenshot,
			ScreenshotPath:     screenshotPath,
			ScreenshotURLs:     screenshotURLs,
			ScreenshotUser:     screenshotUser,
			ScreenshotPassword: screenshotPassword,
			DB: docker.DBConfig{
				Host:     dbHost,
				Name:     dbName,
				User:     dbUser,
				Password: dbPassword,
			},
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}

		if err := docker.RunLocal(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	runCmd.Flags().Bool("local", false, "Run locally without Docker (warm serve + standalone runtime)")
	runCmd.Flags().Bool("watch", false, "Rebuild and hot-apply on every project change")
	runCmd.Flags().Bool("ensure-db", false, "Provision the local Postgres + app database if missing (fresh-session bootstrap)")
	runCmd.Flags().Bool("setup", false, "Prepare prerequisites (cache MxBuild+runtime, ensure DB) and exit without booting — for a SessionStart hook")
	runCmd.Flags().Int("app-port", 0, "HTTP port for the app (default 8080)")
	runCmd.Flags().Int("admin-port", 0, "M2EE admin API port (default 8090)")
	runCmd.Flags().Int("serve-port", 0, "mxbuild --serve port (default 6543)")
	runCmd.Flags().String("db-host", "", "Database host:port (default 127.0.0.1:5432)")
	runCmd.Flags().String("db-name", "", "Database name (default derived from the project name)")
	runCmd.Flags().String("db-user", "", "Database user (default mendix)")
	runCmd.Flags().String("db-password", "", "Database password (default mendix)")
	runCmd.Flags().Bool("screenshot", false, "Capture a Playwright screenshot after boot and each applied change")
	runCmd.Flags().String("screenshot-path", "", "Screenshot output PNG (default <projectDir>/.mxcli/run-local.png)")
	runCmd.Flags().StringArray("screenshot-url", nil, "Page to screenshot: a full URL or a path relative to the app root, e.g. /p/customers (default the app root). Repeat for a multi-page set.")
	runCmd.Flags().String("screenshot-user", "", "Log in with this user before screenshotting (for pages behind login)")
	runCmd.Flags().String("screenshot-password", "", "Password for --screenshot-user")
	rootCmd.AddCommand(runCmd)
}
