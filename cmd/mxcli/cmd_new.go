// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new <app-name>",
	Short: "Create a new Mendix project",
	Long: `Create a new Mendix project with all tooling configured.

This command performs the following steps:
  1. Downloads MxBuild for the specified Mendix version
  2. Creates a blank Mendix project using mx create-project
  3. Initializes AI tooling and devcontainer configuration (mxcli init)
  4. Downloads the correct mxcli binary for the devcontainer (linux)

Examples:
  mxcli new MyApp
  mxcli new MyApp --version 11.8.0
  mxcli new MyApp --version 10.24.0 --output-dir ./projects/my-app
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		appName := args[0]
		mendixVersion, _ := cmd.Flags().GetString("version")
		outputDir, _ := cmd.Flags().GetString("output-dir")
		skipInit, _ := cmd.Flags().GetBool("skip-init")

		if mendixVersion == "" {
			fmt.Fprintln(os.Stderr, "Error: --version is required (e.g., --version 11.8.0)")
			os.Exit(1)
		}

		// Resolve output directory
		if outputDir == "" {
			outputDir = appName
		}
		absDir, err := filepath.Abs(outputDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
			os.Exit(1)
		}

		// Check if directory already exists and has content
		if entries, err := os.ReadDir(absDir); err == nil && len(entries) > 0 {
			fmt.Fprintf(os.Stderr, "Error: directory %s already exists and is not empty\n", absDir)
			os.Exit(1)
		}

		// Step 1: Resolve mx binary.
		// On Windows and macOS, Studio Pro ships a native mx binary — prefer it.
		// CDN downloads contain Linux ELF binaries that cannot run on those platforms.
		// On Linux (CI, devcontainers), download mxbuild from CDN and derive mx.
		fmt.Printf("Step 1/4: Resolving MxBuild %s...\n", mendixVersion)
		mxPath, err := docker.ResolveMxForNewProject(mendixVersion, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not find mx binary for version %s: %v\n", mendixVersion, err)
			if runtime.GOOS == "darwin" {
				fmt.Fprintf(os.Stderr, "  On macOS, install Mendix Studio Pro %s from the Mendix Marketplace.\n", mendixVersion)
			}
			os.Exit(1)
		}

		// Step 2: Create project
		fmt.Printf("\nStep 2/4: Creating Mendix project '%s'...\n", appName)
		if err := os.MkdirAll(absDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
			os.Exit(1)
		}

		mxCmd := exec.Command(mxPath, "create-project", "--app-name", appName)
		mxCmd.Dir = absDir
		mxCmd.Stdout = os.Stdout
		mxCmd.Stderr = os.Stderr
		docker.PrepareMxCommand(mxCmd)
		if err := mxCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating project: %v\n", err)
			os.Exit(1)
		}

		// Clean up duplicate locale files that mx create-project generates.
		// MxBuild's AtlasPlugin.LoadTranslations crashes with "An item with the same
		// key has already been added" when duplicate translation.json files exist.
		if removed := cleanupDuplicateLocaleFiles(absDir); removed > 0 {
			fmt.Printf("  Cleaned %d duplicate locale file(s)\n", removed)
		}

		// Verify .mpr was created — mx create-project names the file after --app-name
		mprPath := filepath.Join(absDir, appName+".mpr")
		if _, err := os.Stat(mprPath); os.IsNotExist(err) {
			// Fallback: check for App.mpr (default when --app-name is not used)
			fallback := filepath.Join(absDir, "App.mpr")
			if _, err := os.Stat(fallback); err == nil {
				mprPath = fallback
			} else {
				// Last resort: find any .mpr file
				matches, _ := filepath.Glob(filepath.Join(absDir, "*.mpr"))
				if len(matches) > 0 {
					mprPath = matches[0]
				} else {
					fmt.Fprintf(os.Stderr, "Error: mx create-project did not produce an .mpr file in %s\n", absDir)
					os.Exit(1)
				}
			}
		}
		fmt.Printf("  Created %s\n", mprPath)

		// Step 3: Initialize tooling
		if !skipInit {
			fmt.Printf("\nStep 3/4: Initializing AI tooling...\n")
			initCmd.Run(initCmd, []string{absDir})
		} else {
			fmt.Printf("\nStep 3/4: Skipped (--skip-init)\n")
		}

		// Step 4: Ensure correct mxcli binary for devcontainer
		fmt.Printf("\nStep 4/4: Setting up mxcli binary...\n")
		mxcliBinPath := filepath.Join(absDir, "mxcli")
		if runtime.GOOS != "linux" {
			// Running on Windows/macOS — download the Linux binary for devcontainer
			tag := mxcliReleaseTag()
			fmt.Printf("  Downloading Linux mxcli (%s) for devcontainer...\n", tag)
			if err := downloadMxcliBinary("mendixlabs/mxcli", tag, "linux", "amd64", mxcliBinPath, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error: could not download Linux mxcli binary for devcontainer: %v\n", err)
				fmt.Fprintln(os.Stderr, "  Run 'mxcli setup mxcli --output ./mxcli' inside the project directory to fix this.")
				os.Exit(1)
			}
		} else {
			// Running on Linux — copy ourselves
			self, err := os.Executable()
			if err == nil {
				selfBytes, err := os.ReadFile(self)
				if err == nil {
					if err := os.WriteFile(mxcliBinPath, selfBytes, 0755); err != nil {
						fmt.Fprintf(os.Stderr, "  Warning: could not copy mxcli binary: %v\n", err)
					} else {
						fmt.Printf("  Copied mxcli to %s\n", mxcliBinPath)
					}
				}
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: could not copy mxcli binary: %v\n", err)
			}
		}

		fmt.Printf("\n✓ Project '%s' created at %s\n", appName, absDir)
		fmt.Println("\nNext steps:")
		fmt.Println("  1. Open the project folder in VS Code")
		fmt.Println("  2. Reopen in Dev Container when prompted")
		fmt.Printf("  3. Run './mxcli -p %s' to start working\n", filepath.Base(mprPath))
	},
}

// cleanupDuplicateLocaleFiles removes duplicate locale files that mx create-project
// generates in themesource/atlas_core/. MxBuild crashes when multiple translation.json
// files map to the same locale key (e.g., "en-US").
//
// Studio Pro-created projects have locale files only at:
//
//	themesource/atlas_core/locales/<locale>/translation.json
//
// mx create-project additionally creates duplicates in nested subdirectories
// (e.g., locales/en-US/atlas_core/locales/en-US/translation.json).
// We keep only the top-level files and remove any deeper duplicates.
func cleanupDuplicateLocaleFiles(projectDir string) int {
	localesDir := filepath.Join(projectDir, "themesource", "atlas_core", "locales")
	if _, err := os.Stat(localesDir); os.IsNotExist(err) {
		return 0
	}

	removed := 0
	// Walk locale directories (en-US, nl-NL, etc.)
	entries, err := os.ReadDir(localesDir)
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		localeDir := filepath.Join(localesDir, entry.Name())
		// Check for nested subdirectories that duplicate the locale
		subEntries, err := os.ReadDir(localeDir)
		if err != nil {
			continue
		}
		for _, sub := range subEntries {
			if sub.IsDir() {
				// Any subdirectory under a locale dir is a duplicate tree
				dupPath := filepath.Join(localeDir, sub.Name())
				if err := os.RemoveAll(dupPath); err == nil {
					removed++
				}
			}
		}
	}
	return removed
}

func init() {
	newCmd.Flags().String("version", "", "Mendix version (e.g., 11.8.0) — required")
	newCmd.Flags().String("output-dir", "", "Output directory (default: ./<app-name>)")
	newCmd.Flags().Bool("skip-init", false, "Skip AI tooling initialization (mxcli init)")

	rootCmd.AddCommand(newCmd)
}
