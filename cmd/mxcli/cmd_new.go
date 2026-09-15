// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/cmd/mxcli/theme"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new <app-name>",
	Short: "Create a new Mendix project",
	Long: `Create a new Mendix project with all tooling configured.

This command performs the following steps:
  1. Downloads MxBuild for the specified Mendix version
  2. Creates a blank Mendix project using mx create-project
  3. Applies mxcli's default styling (--theme, see 'mxcli theme list')
  4. Creates a layout the project owns and moves its pages onto it (--layout none)
  5. Initializes AI tooling and devcontainer configuration (mxcli init)
  6. Runs one build so generated sources are settled (--skip-build to skip)
  7. Links this mxcli into the project (or downloads a Linux build on macOS/Windows)

Step 4 exists because Atlas's layouts live in a Marketplace module, and Mendix's
own guidance is not to edit them: an update replaces the module and the edit is
gone. A generated app therefore starts with a layout it can change.

The project is created in a short temporary directory and moved into place, so
--output-dir may be as deep as your filesystem allows and a failure never leaves
partial output behind. Mendix tooling itself refuses paths over 259 characters,
so a project deeper than that is created but reported as one Studio Pro on
Windows may not open.

Examples:
  mxcli new MyApp
  mxcli new MyApp --version 11.8.0
  mxcli new MyApp --version 10.24.0 --output-dir ./projects/my-app
  mxcli new MyApp --version 11.8.0 --theme none
  mxcli new MyApp --version 11.8.0 --layout none
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		appName := args[0]
		mendixVersion, _ := cmd.Flags().GetString("version")
		outputDir, _ := cmd.Flags().GetString("output-dir")
		skipInit, _ := cmd.Flags().GetBool("skip-init")
		themeName, _ := cmd.Flags().GetString("theme")
		skipBuild, _ := cmd.Flags().GetBool("skip-build")
		layoutMode, _ := cmd.Flags().GetString("layout")

		if mendixVersion == "" {
			fmt.Fprintln(os.Stderr, "Error: --version is required (e.g., --version 11.8.0)")
			os.Exit(1)
		}

		// Validate the theme before downloading ~800MB of MxBuild: a typo should
		// fail in a second, not after the slowest step in the command.
		if themeName != theme.NoneName {
			if _, err := theme.Get("", themeName); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
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
		fmt.Printf("Step 1/6: Resolving MxBuild %s...\n", mendixVersion)
		mxPath, err := docker.ResolveMxForNewProject(mendixVersion, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not find mx binary for version %s: %v\n", mendixVersion, err)
			if runtime.GOOS == "darwin" {
				fmt.Fprintf(os.Stderr, "  On macOS, install Mendix Studio Pro %s from the Mendix Marketplace.\n", mendixVersion)
			}
			os.Exit(1)
		}

		// Step 2: Create project.
		//
		// Created in a SHORT staging directory and moved into place afterwards.
		// MxToolset refuses to extract past a 259-character full path and aborts
		// PART WAY THROUGH when it hits that, so pointing it straight at a deep
		// output directory left the user with a few hundred orphaned files and no
		// .mpr (issue #825). Staging removes the limit from the equation entirely —
		// the destination may be as deep as the filesystem allows — and means the
		// destination is only ever written to after creation has succeeded.
		fmt.Printf("\nStep 2/6: Creating Mendix project '%s'...\n", appName)
		if err := os.MkdirAll(absDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
			os.Exit(1)
		}

		stageDir, cleanupStage, err := stagedProjectDirs()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer cleanupStage()

		mxCmd := exec.Command(mxPath, "create-project", "--app-name", appName)
		mxCmd.Dir = stageDir
		mxCmd.Stdout = os.Stdout
		mxCmd.Stderr = os.Stderr
		docker.PrepareMxCommand(mxCmd)
		if err := mxCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating project: %v\n", describeCreateProjectFailure(stageDir, err))
			os.Exit(1)
		}

		// Measure the template's deepest path before moving, so the portability
		// warning uses this version's real number rather than a constant that
		// drifts (181 characters on 11.13.0, 182 on 11.12.0).
		longest, longestPath := longestRelativePath(stageDir)
		if err := moveProject(stageDir, absDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error moving project into place: %v\n", err)
			os.Exit(1)
		}
		warnIfPathTooLongForStudioPro(os.Stdout, absDir, longest, longestPath)

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

		// The project is stamped with the version of the binary that created it, not
		// with --version. Resolving the wrong binary therefore yields a project at a
		// version the user never asked for, and every later step (init, mxbuild,
		// runtime) silently follows it. Check the postcondition rather than trusting
		// the resolution: a mismatch here means the model is wrong, so fail loudly
		// instead of handing back something that merely looks finished.
		if reader, err := openProjectReadOnly(mprPath); err == nil {
			created := reader.ProjectVersion().ProductVersion
			_ = reader.Disconnect()
			if created != "" && created != mendixVersion {
				fmt.Fprintf(os.Stderr,
					"Error: requested Mendix %s but the created project is %s.\n",
					mendixVersion, created)
				fmt.Fprintf(os.Stderr,
					"  mx create-project stamps the project with the version of the binary that ran it (%s).\n", mxPath)
				fmt.Fprintf(os.Stderr,
					"  Run 'mxcli setup mxbuild --version %s' and try again.\n", mendixVersion)
				os.Exit(1)
			}
			fmt.Printf("  Mendix version: %s\n", created)
		}

		// Step 3: Default styling. A blank Atlas app is unmistakably a blank Atlas
		// app; a generated one should look like a product on first boot. This
		// writes files under theme/ only — the model is untouched, so the theme
		// can be re-applied, swapped or removed at any point.
		if themeName != theme.NoneName {
			fmt.Printf("\nStep 3/7: Applying '%s' styling...\n", themeName)
			res, err := theme.Apply(absDir, themeName, theme.Options{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error applying theme: %v\n", err)
				os.Exit(1)
			}
			for _, f := range res.Files {
				fmt.Printf("  %-9s %s\n", f.Action, f.Path)
			}
		} else {
			fmt.Printf("\nStep 3/7: Skipped styling (--theme none)\n")
		}

		// Step 4: A layout the project owns. Every generated app used to start
		// on Atlas_Core.Atlas_TopBar — a layout in a Marketplace module, which
		// Mendix's own guidance says not to edit because an update replaces the
		// module. Scaffolding one inverts that from an obstacle you discover
		// into a starting point you already have.
		//
		// Best-effort, like the first build: a project without it is a perfectly
		// good project, so a failure is a warning rather than an exit.
		if layoutMode != LayoutNone {
			fmt.Printf("\nStep 4/7: Creating a layout the project owns...\n")
			if mod, err := scaffoldProjectLayout(mprPath, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: could not create the project layout: %v\n", err)
				fmt.Fprintln(os.Stderr, "  The project is usable and keeps Atlas's layouts. See 'mxcli syntax layout'.")
			} else {
				fmt.Printf("  Pages now render inside %s.%s — edit it with 'mxcli syntax layout.alter'\n",
					mod, scaffoldLayoutName)
			}
		} else {
			fmt.Printf("\nStep 4/7: Skipped the project layout (--layout none)\n")
		}

		// Step 5: Initialize tooling
		if !skipInit {
			fmt.Printf("\nStep 5/7: Initializing AI tooling...\n")
			initCmd.Run(initCmd, []string{absDir})
		} else {
			fmt.Printf("\nStep 5/7: Skipped (--skip-init)\n")
		}

		// Align the project's Java version with what mxcli can build and run
		// BEFORE the first build, or that build is the thing that fails: Mendix
		// 11.14's create-project writes JavaVersion = 25 and mxcli launches the
		// runtime on JDK 21 (ako/mxcli-maintenance #1). Best-effort, like the
		// steps around it — but note this one decides whether step 6 can succeed
		// at all, so a failure here is worth saying out loud.
		if lowered, err := alignJavaVersion(mprPath, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not align the project's Java version: %v\n", err)
			fmt.Fprintln(os.Stderr, "  If the first build fails with 'release version NN not supported',")
			fmt.Fprintln(os.Stderr, "  run: mxcli -p <project>.mpr -c \"alter settings MODEL JavaVersion = '21'\"")
		} else if lowered {
			fmt.Println()
		}

		// Step 5: Settle the sources MxBuild generates. The template ships the JS
		// and Java action stubs in a slightly older shape and the first build
		// rewrites all of them — 48 tracked files in a blank Mendix 11.12 app — so
		// without this the project goes dirty the first time anyone builds it, with
		// changes nobody wrote (mxcli-todo #7). Doing it here puts the settled form
		// in the first commit. Best-effort: this is a nicety, not a precondition
		// for a usable project, so a missing JDK or a build failure is a warning.
		if skipBuild {
			fmt.Printf("\nStep 6/7: Skipped first build (--skip-build)\n")
		} else {
			fmt.Printf("\nStep 6/7: Running the first build (settles generated sources)...\n")
			if err := docker.SettleGeneratedSources(mprPath, mxPath, mendixVersion, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: could not run the first build: %v\n", err)
				fmt.Fprintln(os.Stderr, "  The project is usable. Note that the first build will rewrite the")
				fmt.Fprintln(os.Stderr, "  generated action stubs under javascriptsource/ and javasource/ —")
				fmt.Fprintln(os.Stderr, "  that diff is build output, so commit it and move on.")
			}
		}

		// Step 6: Ensure correct mxcli binary for devcontainer
		fmt.Printf("\nStep 7/7: Setting up mxcli binary...\n")
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
			// Running on Linux — link ourselves into the project. Prefer a hard link:
			// it shares the inode (no ~111MB duplicated per project on the same
			// filesystem), is a real ELF the devcontainer can exec, and survives the
			// original binary being moved/removed (unlike a symlink). Fall back to a
			// full copy across filesystems (EXDEV) or when linking isn't possible.
			self, err := os.Executable()
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: could not locate mxcli binary: %v\n", err)
			} else if resolved, rerr := filepath.EvalSymlinks(self); rerr == nil {
				self = resolved
				_ = os.Remove(mxcliBinPath) // os.Link fails if the target already exists
				if err := os.Link(self, mxcliBinPath); err == nil {
					fmt.Printf("  Linked mxcli to %s (shared inode, no copy)\n", mxcliBinPath)
				} else if selfBytes, rerr := os.ReadFile(self); rerr == nil {
					if werr := os.WriteFile(mxcliBinPath, selfBytes, 0o755); werr != nil {
						fmt.Fprintf(os.Stderr, "  Warning: could not copy mxcli binary: %v\n", werr)
					} else {
						fmt.Printf("  Copied mxcli to %s\n", mxcliBinPath)
					}
				} else {
					fmt.Fprintf(os.Stderr, "  Warning: could not read mxcli binary: %v\n", rerr)
				}
			} else {
				fmt.Fprintf(os.Stderr, "  Warning: could not resolve mxcli binary path: %v\n", rerr)
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
	newCmd.Flags().Bool("skip-build", false,
		"Skip the first build (leaves generated action stubs to be rewritten by the next build)")
	newCmd.Flags().String("theme", theme.DefaultName,
		"Default styling to apply ('none' to keep plain Atlas; see 'mxcli theme list')")
	newCmd.Flags().String("layout", "app-default",
		"Create a layout the project owns and move its pages onto it ('none' to keep Atlas's)")

	rootCmd.AddCommand(newCmd)
}
