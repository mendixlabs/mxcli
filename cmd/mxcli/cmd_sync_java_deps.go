// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

var syncJavaDepsCmd = &cobra.Command{
	Use:   "sync-java-deps",
	Short: "Download the project's managed Java (JAR) dependencies into vendorlib/",
	Long: `Resolve every managed Java dependency the model declares and download it
into the project's vendorlib/ directory.

Declaring a dependency and resolving it are separate steps. MDL's
'ALTER MODULE X ADD JAR DEPENDENCY (…)' records the coordinate in the model —
'list jar dependencies' will report it — but nothing downloads the jar, and
MxBuild does not resolve it either: a full build produces a build.gradle with no
dependencies block. Studio Pro runs the resolution for you when you edit Module
Settings; headless, this command is that step.

Without it the failure is silent until runtime, as a missing-driver exception
from code that looks correctly configured.

Requires network access (Maven) and the mx binary for the project's version;
'mxcli setup mxbuild' fetches the latter.

Examples:
  mxcli sync-java-deps -p app.mpr
  mxcli sync-java-deps -p app.mpr --check   # report what is missing, download nothing
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectPath, _ := cmd.Flags().GetString("project")
		checkOnly, _ := cmd.Flags().GetBool("check")
		if projectPath == "" {
			return fmt.Errorf("--project (-p) is required")
		}

		deps, version, err := declaredJarDependencies(projectPath)
		if err != nil {
			return err
		}
		if len(deps) == 0 {
			fmt.Println("No managed Java dependencies declared.")
			return nil
		}

		missing := docker.UnvendoredJarDependencies(filepath.Dir(projectPath), deps)
		fmt.Printf("Declared: %d managed Java dependency/dependencies; %d not in vendorlib/\n",
			len(deps), len(missing))
		for _, m := range missing {
			fmt.Printf("  missing  %s\n", m)
		}
		if checkOnly {
			if len(missing) > 0 {
				// A non-zero exit makes this usable as a build-gate.
				os.Exit(1)
			}
			return nil
		}
		if len(missing) == 0 {
			return nil
		}

		fmt.Printf("Syncing against Mendix %s...\n", version)
		if err := docker.SyncJavaDependencies(projectPath, "", version, os.Stdout); err != nil {
			return err
		}
		if still := docker.UnvendoredJarDependencies(filepath.Dir(projectPath), deps); len(still) > 0 {
			// mx reports success even when a coordinate resolves to nothing, so
			// check the postcondition rather than trusting the exit code.
			return fmt.Errorf("sync finished but %d dependency/dependencies are still missing from vendorlib/: %v", len(still), still)
		}
		fmt.Println("All declared dependencies are in vendorlib/.")
		return nil
	},
}

// declaredJarDependencies reads the model's managed Java dependencies and the
// project's Mendix version.
func declaredJarDependencies(projectPath string) ([]docker.JarDependencyRef, string, error) {
	reader, err := openProjectReadOnly(projectPath)
	if err != nil {
		return nil, "", fmt.Errorf("opening project: %w", err)
	}
	defer func() { _ = reader.Disconnect() }()

	version := reader.ProjectVersion().ProductVersion

	// ListModuleSettings covers every module in one read; the dependency's owner
	// does not matter here, only whether the jar is on the classpath.
	all, err := reader.ListModuleSettings()
	if err != nil {
		return nil, version, fmt.Errorf("reading module settings: %w", err)
	}
	var out []docker.JarDependencyRef
	for _, ms := range all {
		if ms == nil {
			continue
		}
		for _, d := range ms.JarDependencies {
			out = append(out, docker.JarDependencyRef{
				Group: d.GroupID, Artifact: d.ArtifactID, Version: d.Version,
			})
		}
	}
	return out, version, nil
}

func init() {
	syncJavaDepsCmd.Flags().Bool("check", false, "Report missing dependencies and exit non-zero; download nothing")
	rootCmd.AddCommand(syncJavaDepsCmd)
}
