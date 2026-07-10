// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Docker build and deployment commands",
	Long: `Docker integration for building and deploying Mendix applications in containers.

Subcommands:
  run      Setup, build, and start in one command (recommended)
  init     Generate Docker Compose stack (docker-compose.yml, .env)
  build    Build a Portable App Distribution (PAD) package using MxBuild
  check    Check project for errors before building
  reload   Hot reload the running app (build + reload_model)
  up       Start the Docker Compose stack
  down     Stop the Docker Compose stack
  logs     View Mendix container logs
  status   Show container status
  shell    Open a shell in the Mendix container

Requirements:
  - Mendix 11.6.1 or later
  - MxBuild (auto-downloaded from CDN if not found)
  - JDK 21 (installed in devcontainers created by 'mxcli init')
  - Docker with Compose V2, or Podman 4.7+ with podman compose

Podman Support:
  mxcli auto-detects the container runtime (Docker or Podman).
  Override with: export MXCLI_CONTAINER_CLI=podman

Examples:
  # One command to setup, build, and start
  mxcli docker run -p app.mpr

  # Or step by step:
  mxcli docker init -p app.mpr
  mxcli docker build -p app.mpr
  mxcli docker up -p app.mpr --detach --wait
`,
}

var dockerRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Setup, build, and start in one command",
	Long: `Setup all dependencies, build the PAD package, and start the application.

This is the easiest way to get a Mendix app running in Docker. It handles:
1. Downloads MxBuild (if not cached)
2. Downloads the Mendix runtime (if not cached)
3. Initializes Docker stack (if not already done)
4. Builds the PAD package using MxBuild
5. Starts the Docker Compose stack
6. Waits for the runtime to report successful startup (with --wait)

All downloads are cached and reused across builds.

Examples:
  mxcli docker run -p app.mpr
  mxcli docker run -p app.mpr --wait
  mxcli docker run -p app.mpr --fresh --wait
  mxcli docker run -p app.mpr --skip-check
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		mxbuildPath, _ := cmd.Flags().GetString("mxbuild-path")
		skipCheck, _ := cmd.Flags().GetBool("skip-check")
		fresh, _ := cmd.Flags().GetBool("fresh")
		wait, _ := cmd.Flags().GetBool("wait")
		waitTimeout, _ := cmd.Flags().GetInt("wait-timeout")
		portOffset, _ := cmd.Flags().GetInt("port-offset")

		opts := docker.RunOptions{
			ProjectPath: projectPath,
			MxBuildPath: mxbuildPath,
			SkipCheck:   skipCheck,
			Fresh:       fresh,
			Wait:        wait,
			WaitTimeout: time.Duration(waitTimeout) * time.Second,
			PortOffset:  portOffset,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}

		if err := docker.Run(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var dockerBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build a Portable App Distribution package",
	Long: `Build a Portable App Distribution (PAD) package using MxBuild.

This command:
1. Detects the Mendix project version (requires >= 11.6.1)
2. Locates MxBuild and JDK 21 (auto-downloads MxBuild from CDN if not found)
3. Runs MxBuild with --target=portable-app-package
4. Applies version-aware patches to fix known PAD issues

MxBuild is cached at ~/.mxcli/mxbuild/{version}/ and reused across builds.
You can also pre-download with: mxcli setup mxbuild -p app.mpr

Patches applied:
  - [11.6.x] Fix missing execute permission on bin/start
  - [11.6.x] Fix Dockerfile CMD referencing start.sh instead of start
  - [all]    Replace deprecated openjdk:21 with eclipse-temurin:21-jre
  - [all]    Add HEALTHCHECK instruction

Examples:
  mxcli docker build -p app.mpr
  mxcli docker build -p app.mpr --mxbuild-path /path/to/mxbuild -o ./output
  mxcli docker build -p app.mpr --dry-run
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		mxbuildPath, _ := cmd.Flags().GetString("mxbuild-path")
		outputDir, _ := cmd.Flags().GetString("output")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		skipCheck, _ := cmd.Flags().GetBool("skip-check")
		noUpdateWidgets, _ := cmd.Flags().GetBool("no-update-widgets")

		opts := docker.BuildOptions{
			ProjectPath:       projectPath,
			MxBuildPath:       mxbuildPath,
			OutputDir:         outputDir,
			DryRun:            dryRun,
			SkipCheck:         skipCheck,
			SkipUpdateWidgets: noUpdateWidgets,
			Stdout:            os.Stdout,
		}

		if err := docker.Build(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var dockerCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check project for errors using mx",
	Long: `Run 'mx check' to validate the Mendix project before building.

This catches project errors (broken references, missing attributes, etc.)
early, before the slower MxBuild step. The 'docker build' command runs
this automatically unless --skip-check is used.

By default, 'mx update-widgets' runs before 'mx check' to normalize
pluggable widget definitions and prevent false CE0463 errors. Use
--no-update-widgets to skip this step.

The mx binary is located from the same directory as mxbuild.

Examples:
  mxcli docker check -p app.mpr
  mxcli docker check -p app.mpr --mxbuild-path /path/to/mendix
  mxcli docker check -p app.mpr --no-update-widgets
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		mxbuildPath, _ := cmd.Flags().GetString("mxbuild-path")
		noUpdateWidgets, _ := cmd.Flags().GetBool("no-update-widgets")

		opts := docker.CheckOptions{
			ProjectPath:       projectPath,
			MxBuildPath:       mxbuildPath,
			SkipUpdateWidgets: noUpdateWidgets,
			Stdout:            os.Stdout,
			Stderr:            os.Stderr,
		}

		if err := docker.Check(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var dockerInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate Docker Compose stack for a Mendix project",
	Long: `Generate a Docker Compose stack with PostgreSQL for running a Mendix application.

Creates a .docker/ directory next to the MPR file containing:
  - docker-compose.yml  (Mendix app + PostgreSQL with healthchecks)
  - .env.example        (documented environment variables)
  - .env                (copy of .env.example for local use)

The compose file references ./build as the build context, which is where
'mxcli docker build' outputs the PAD package.

Examples:
  mxcli docker init -p app.mpr
  mxcli docker init -p app.mpr --force
  mxcli docker init -p app.mpr -o /custom/docker/dir
  mxcli docker init -p app.mpr --port-offset 1   # Ports: 8081/8091/5433
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		outputDir, _ := cmd.Flags().GetString("output")
		force, _ := cmd.Flags().GetBool("force")
		portOffset, _ := cmd.Flags().GetInt("port-offset")

		opts := docker.InitOptions{
			ProjectPath: projectPath,
			OutputDir:   outputDir,
			Force:       force,
			PortOffset:  portOffset,
			Stdout:      os.Stdout,
		}

		if err := docker.Init(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var dockerUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the Docker Compose stack",
	Long: `Start the Mendix application and PostgreSQL containers.

Runs 'docker compose up --build' in the .docker/ directory.

With --wait, tails the container logs after starting in detached mode and
waits until the Mendix runtime reports successful startup (or timeout).

Examples:
  mxcli docker up -p app.mpr
  mxcli docker up -p app.mpr --detach
  mxcli docker up -p app.mpr --fresh   # Remove volumes first
  mxcli docker up -p app.mpr --detach --wait
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		detach, _ := cmd.Flags().GetBool("detach")
		fresh, _ := cmd.Flags().GetBool("fresh")
		wait, _ := cmd.Flags().GetBool("wait")
		waitTimeout, _ := cmd.Flags().GetInt("wait-timeout")

		opts := docker.RuntimeOptions{
			ProjectPath: projectPath,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}

		if err := docker.Up(opts, detach, fresh); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if wait && detach {
			if err := docker.WaitForReady(opts, time.Duration(waitTimeout)*time.Second); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

var dockerDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the Docker Compose stack",
	Long: `Stop and remove the Mendix application and PostgreSQL containers.

Examples:
  mxcli docker down -p app.mpr
  mxcli docker down -p app.mpr --volumes   # Also remove data volumes
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		volumes, _ := cmd.Flags().GetBool("volumes")

		opts := docker.RuntimeOptions{
			ProjectPath: projectPath,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}

		if err := docker.Down(opts, volumes); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var dockerLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View Mendix container logs",
	Long: `View logs from the Mendix application container.

Examples:
  mxcli docker logs -p app.mpr
  mxcli docker logs -p app.mpr --follow
  mxcli docker logs -p app.mpr --tail 50
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		follow, _ := cmd.Flags().GetBool("follow")
		tail, _ := cmd.Flags().GetInt("tail")

		opts := docker.RuntimeOptions{
			ProjectPath: projectPath,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}

		if err := docker.Logs(opts, follow, tail); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var dockerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show container status",
	Long: `Show the status of Docker Compose services.

Examples:
  mxcli docker status -p app.mpr
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		opts := docker.RuntimeOptions{
			ProjectPath: projectPath,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}

		if err := docker.Status(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var dockerReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Hot reload the running Mendix application",
	Long: `Build and reload the Mendix application model without restarting the container.

This uses the M2EE admin API reload_model action, which reloads the model
in ~100ms — much faster than a full container restart (~20s).

Modes:
  (default)      Build the PAD package, then call reload_model
  --model-only   Skip the build step, just call reload_model
  --css          Update styling only (instant, no build or model reload)

Examples:
  # Full reload: build + reload_model
  mxcli docker reload -p app.mpr

  # Model-only: skip build, just reload
  mxcli docker reload -p app.mpr --model-only

  # CSS hot reload: instant styling update
  mxcli docker reload -p app.mpr --css

  # Skip mx check for faster iteration
  mxcli docker reload -p app.mpr --skip-check
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		mxbuildPath, _ := cmd.Flags().GetString("mxbuild-path")
		skipCheck, _ := cmd.Flags().GetBool("skip-check")
		modelOnly, _ := cmd.Flags().GetBool("model-only")
		cssOnly, _ := cmd.Flags().GetBool("css")
		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetInt("port")
		token, _ := cmd.Flags().GetString("token")
		direct, _ := cmd.Flags().GetBool("direct")

		opts := docker.ReloadOptions{
			ProjectPath: projectPath,
			MxBuildPath: mxbuildPath,
			SkipCheck:   skipCheck,
			SkipBuild:   modelOnly,
			CSSOnly:     cssOnly,
			Host:        host,
			Port:        port,
			Token:       token,
			Direct:      direct,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}

		if err := docker.Reload(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var dockerShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Open a shell in the Mendix container",
	Long: `Open an interactive shell or execute a command in the Mendix container.

Examples:
  mxcli docker shell -p app.mpr
  mxcli docker shell -p app.mpr --exec "ls -la /mendix"
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		execCmd, _ := cmd.Flags().GetString("exec")

		opts := docker.RuntimeOptions{
			ProjectPath: projectPath,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}

		if err := docker.Shell(opts, execCmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	// Run command flags
	dockerRunCmd.Flags().String("mxbuild-path", "", "Path to MxBuild executable")
	dockerRunCmd.Flags().Bool("skip-check", false, "Skip 'mx check' pre-build validation")
	dockerRunCmd.Flags().Bool("fresh", false, "Remove volumes and rebuild from scratch")
	dockerRunCmd.Flags().BoolP("wait", "w", false, "Wait for runtime to start before returning")
	dockerRunCmd.Flags().Int("wait-timeout", 300, "Timeout in seconds for --wait (default: 300)")
	dockerRunCmd.Flags().Int("port-offset", 0, "Shift all ports by N (e.g., 1 → 8081/8091/5433) for running multiple projects")

	// Build command flags
	dockerBuildCmd.Flags().String("mxbuild-path", "", "Path to MxBuild executable")
	dockerBuildCmd.Flags().StringP("output", "o", "", "Output directory for PAD package")
	dockerBuildCmd.Flags().Bool("dry-run", false, "Detect tools and show patch plan without building")
	dockerBuildCmd.Flags().Bool("skip-check", false, "Skip 'mx check' pre-build validation")
	dockerBuildCmd.Flags().Bool("no-update-widgets", false, "Skip 'mx update-widgets' before check")

	// Check command flags
	dockerCheckCmd.Flags().String("mxbuild-path", "", "Path to MxBuild/Mendix installation (used to find mx)")
	dockerCheckCmd.Flags().Bool("no-update-widgets", false, "Skip 'mx update-widgets' before check")

	// Init command flags
	dockerInitCmd.Flags().StringP("output", "o", "", "Output directory (default: .docker/ next to MPR)")
	dockerInitCmd.Flags().Bool("force", false, "Overwrite existing files")
	dockerInitCmd.Flags().Int("port-offset", 0, "Shift all ports by N (e.g., 1 → 8081/8091/5433) for running multiple projects")

	// Up command flags
	dockerUpCmd.Flags().BoolP("detach", "d", false, "Run in background")
	dockerUpCmd.Flags().Bool("fresh", false, "Remove volumes and rebuild from scratch")
	dockerUpCmd.Flags().BoolP("wait", "w", false, "Wait for runtime to start (requires --detach)")
	dockerUpCmd.Flags().Int("wait-timeout", 300, "Timeout in seconds for --wait (default: 300)")

	// Down command flags
	dockerDownCmd.Flags().BoolP("volumes", "v", false, "Also remove data volumes")

	// Logs command flags
	dockerLogsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	dockerLogsCmd.Flags().Int("tail", 0, "Number of lines to show from end of logs")

	// Shell command flags
	dockerShellCmd.Flags().StringP("exec", "e", "", "Execute command instead of interactive shell")

	// Reload command flags
	dockerReloadCmd.Flags().String("mxbuild-path", "", "Path to MxBuild executable")
	dockerReloadCmd.Flags().Bool("skip-check", false, "Skip 'mx check' pre-build validation")
	dockerReloadCmd.Flags().Bool("model-only", false, "Skip build, just call reload_model")
	dockerReloadCmd.Flags().Bool("css", false, "CSS hot reload only (update_styling, no build)")
	dockerReloadCmd.Flags().String("host", "", "M2EE admin API host")
	dockerReloadCmd.Flags().Int("port", 0, "M2EE admin API port")
	dockerReloadCmd.Flags().String("token", "", "M2EE admin password")
	dockerReloadCmd.Flags().Bool("direct", false, "Connect via HTTP directly (bypass docker exec)")

	// Register all subcommands
	dockerCmd.AddCommand(dockerRunCmd)
	dockerCmd.AddCommand(dockerBuildCmd)
	dockerCmd.AddCommand(dockerCheckCmd)
	dockerCmd.AddCommand(dockerInitCmd)
	dockerCmd.AddCommand(dockerUpCmd)
	dockerCmd.AddCommand(dockerDownCmd)
	dockerCmd.AddCommand(dockerLogsCmd)
	dockerCmd.AddCommand(dockerStatusCmd)
	dockerCmd.AddCommand(dockerShellCmd)
	dockerCmd.AddCommand(dockerReloadCmd)
	rootCmd.AddCommand(dockerCmd)
}
