// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/docker"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup development tools",
	Long: `Download and configure tools required for Mendix development.

Subcommands:
  mxbuild    Download MxBuild for the project's Mendix version
  mxruntime  Download the Mendix runtime for the project's Mendix version
  mxcli      Download an mxcli binary from GitHub releases

Examples:
  mxcli setup mxbuild -p app.mpr
  mxcli setup mxbuild --version 11.6.3
  mxcli setup mxruntime -p app.mpr
  mxcli setup mxruntime --version 11.6.3
  mxcli setup mxcli --os linux --arch amd64
`,
}

var setupMxBuildCmd = &cobra.Command{
	Use:   "mxbuild",
	Short: "Download MxBuild from the Mendix CDN",
	Long: `Download and cache MxBuild for a specific Mendix version.

The version is detected from the project file (--project) or specified
explicitly (--version). The binary is cached at ~/.mxcli/mxbuild/{version}/
and automatically found by 'mxcli docker build' and 'mxcli docker check'.

The Mendix CDN only publishes Linux MxBuild. On Windows and macOS that binary
cannot run natively, so this command prefers Studio Pro's bundled MxBuild and
refuses to download the Linux binary unless --force is given (useful for
pre-caching a Linux binary for a Docker/devcontainer build).

Examples:
  mxcli setup mxbuild -p app.mpr
  mxcli setup mxbuild --version 11.6.3
  mxcli setup mxbuild -p app.mpr --dry-run
  mxcli setup mxbuild --version 11.6.3 --force   # download Linux binary on Windows/macOS
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		versionStr, _ := cmd.Flags().GetString("version")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		force, _ := cmd.Flags().GetBool("force")

		// Determine version
		if versionStr == "" && projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: specify --project (-p) or --version")
			os.Exit(1)
		}

		if versionStr == "" {
			// Detect from project
			reader, err := mpr.Open(projectPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error opening project: %v\n", err)
				os.Exit(1)
			}
			pv := reader.ProjectVersion()
			reader.Close()
			versionStr = pv.ProductVersion
			fmt.Fprintf(os.Stdout, "Detected Mendix version: %s\n", versionStr)
		}

		// The Mendix CDN only publishes Linux mxbuild. On Windows/macOS a CDN
		// download cannot run natively (dotnet reports "Exec format error"), so
		// prefer Studio Pro's bundled mxbuild and otherwise refuse with guidance.
		// --force overrides this to pre-cache the Linux binary (e.g. for Docker).
		if !force {
			nativePath, guidance, err := docker.NativeMxBuildForSetup(runtime.GOOS, versionStr)
			if nativePath != "" {
				fmt.Fprintf(os.Stdout, "Using Studio Pro mxbuild for %s: %s\n", versionStr, nativePath)
				fmt.Fprintf(os.Stdout, "(No download needed. Re-run with --force to download the Linux CDN binary instead.)\n")
				return
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n  %s\n", err, guidance)
				os.Exit(1)
			}
		}

		if dryRun {
			url := docker.MxBuildCDNURL(versionStr, runtime.GOARCH)
			cacheDir, _ := docker.MxBuildCacheDir(versionStr)
			fmt.Fprintf(os.Stdout, "Dry run:\n")
			fmt.Fprintf(os.Stdout, "  Version:      %s\n", versionStr)
			fmt.Fprintf(os.Stdout, "  Architecture: %s\n", runtime.GOARCH)
			fmt.Fprintf(os.Stdout, "  URL:          %s\n", url)
			fmt.Fprintf(os.Stdout, "  Cache dir:    %s\n", cacheDir)

			if cached := docker.CachedMxBuildPath(versionStr); cached != "" {
				fmt.Fprintf(os.Stdout, "  Status:       already cached at %s\n", cached)
			} else {
				fmt.Fprintf(os.Stdout, "  Status:       not cached, would download\n")
			}
			return
		}

		path, err := docker.DownloadMxBuild(versionStr, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stdout, "\nMxBuild ready: %s\n", path)
	},
}

var setupMxRuntimeCmd = &cobra.Command{
	Use:   "mxruntime",
	Short: "Download the Mendix runtime from the Mendix CDN",
	Long: `Download and cache the Mendix runtime for a specific Mendix version.

The version is detected from the project file (--project) or specified
explicitly (--version). The runtime is cached at ~/.mxcli/runtime/{version}/
and automatically used by 'mxcli docker build' when the PAD output does not
include the runtime (MxBuild 11.6.3+).

Examples:
  mxcli setup mxruntime -p app.mpr
  mxcli setup mxruntime --version 11.6.3
  mxcli setup mxruntime -p app.mpr --dry-run
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		versionStr, _ := cmd.Flags().GetString("version")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Determine version
		if versionStr == "" && projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: specify --project (-p) or --version")
			os.Exit(1)
		}

		if versionStr == "" {
			// Detect from project
			reader, err := mpr.Open(projectPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error opening project: %v\n", err)
				os.Exit(1)
			}
			pv := reader.ProjectVersion()
			reader.Close()
			versionStr = pv.ProductVersion
			fmt.Fprintf(os.Stdout, "Detected Mendix version: %s\n", versionStr)
		}

		if dryRun {
			url := docker.RuntimeCDNURL(versionStr)
			cacheDir, _ := docker.RuntimeCacheDir(versionStr)
			fmt.Fprintf(os.Stdout, "Dry run:\n")
			fmt.Fprintf(os.Stdout, "  Version:   %s\n", versionStr)
			fmt.Fprintf(os.Stdout, "  URL:       %s\n", url)
			fmt.Fprintf(os.Stdout, "  Cache dir: %s\n", cacheDir)

			if cached := docker.CachedRuntimePath(versionStr); cached != "" {
				fmt.Fprintf(os.Stdout, "  Status:    already cached at %s\n", cached)
			} else {
				fmt.Fprintf(os.Stdout, "  Status:    not cached, would download\n")
			}
			return
		}

		path, err := docker.DownloadRuntime(versionStr, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stdout, "\nMendix runtime ready: %s\n", path)
	},
}

// mxcliBinaryURL returns the GitHub releases download URL for an mxcli binary.
// ver should be a release tag like "v0.4.0" or "nightly".
// targetOS is "linux", "darwin", or "windows". targetArch is "amd64" or "arm64".
func mxcliBinaryURL(repo, ver, targetOS, targetArch string) string {
	name := fmt.Sprintf("mxcli-%s-%s", targetOS, targetArch)
	if targetOS == "windows" {
		name += ".exe"
	}
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, ver, name)
}

// mxcliReleaseTag returns the release tag that matches the running binary's
// version string. Tagged releases use "vX.Y.Z"; nightly builds contain
// "nightly" and map to the "nightly" release tag.
func mxcliReleaseTag() string {
	v := version // package-level var set from ldflags
	if strings.Contains(v, "nightly") {
		return "nightly"
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	// Strip build metadata after first hyphen-with-commit (e.g. "v0.4.0-3-gabcdef" -> "v0.4.0")
	if idx := strings.IndexByte(v, '-'); idx > 0 {
		v = v[:idx]
	}
	return v
}

// downloadMxcliBinary downloads the mxcli binary for the given OS/arch from
// GitHub releases and writes it to outputPath with executable permissions.
func downloadMxcliBinary(repo, tag, targetOS, targetArch, outputPath string, w io.Writer) error {
	url := mxcliBinaryURL(repo, tag, targetOS, targetArch)
	fmt.Fprintf(w, "Downloading mxcli %s (%s/%s)...\n", tag, targetOS, targetArch)
	fmt.Fprintf(w, "  URL: %s\n", url)
	return downloadMxcliBinaryFromURL(url, outputPath, w)
}

// downloadMxcliBinaryFromURL fetches a binary from rawURL and writes it to
// outputPath with executable permissions (0755). Used by downloadMxcliBinary
// and directly by tests via an httptest.Server URL.
func downloadMxcliBinaryFromURL(rawURL, outputPath string, w io.Writer) error {
	resp, err := http.Get(rawURL)
	if err != nil {
		return fmt.Errorf("downloading mxcli: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading mxcli: HTTP %d from %s", resp.StatusCode, rawURL)
	}

	if resp.ContentLength > 0 {
		fmt.Fprintf(w, "  Size: %.1f MB\n", float64(resp.ContentLength)/(1024*1024))
	}

	f, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(outputPath)
		return fmt.Errorf("writing binary: %w", err)
	}

	fmt.Fprintf(w, "  Saved to %s\n", outputPath)
	return nil
}

var setupMxcliCmd = &cobra.Command{
	Use:   "mxcli",
	Short: "Download an mxcli binary from GitHub releases",
	Long: `Download an mxcli binary for a specific OS/architecture from GitHub releases.

By default, downloads the version matching the currently running binary for
linux/amd64 — the typical target for devcontainers.

Examples:
  mxcli setup mxcli                           # Linux amd64 binary to ./mxcli
  mxcli setup mxcli --output /usr/local/bin/mxcli
  mxcli setup mxcli --os darwin --arch arm64   # macOS Apple Silicon
  mxcli setup mxcli --tag v0.4.0               # Specific release
  mxcli setup mxcli --tag nightly              # Latest nightly build
`,
	Run: func(cmd *cobra.Command, args []string) {
		targetOS, _ := cmd.Flags().GetString("os")
		targetArch, _ := cmd.Flags().GetString("arch")
		output, _ := cmd.Flags().GetString("output")
		tag, _ := cmd.Flags().GetString("tag")
		repo, _ := cmd.Flags().GetString("repo")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if tag == "" {
			tag = mxcliReleaseTag()
		}

		if dryRun {
			url := mxcliBinaryURL(repo, tag, targetOS, targetArch)
			fmt.Fprintf(os.Stdout, "Dry run:\n")
			fmt.Fprintf(os.Stdout, "  Tag:    %s\n", tag)
			fmt.Fprintf(os.Stdout, "  OS:     %s\n", targetOS)
			fmt.Fprintf(os.Stdout, "  Arch:   %s\n", targetArch)
			fmt.Fprintf(os.Stdout, "  URL:    %s\n", url)
			fmt.Fprintf(os.Stdout, "  Output: %s\n", output)
			return
		}

		if err := downloadMxcliBinary(repo, tag, targetOS, targetArch, output, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stdout, "\nmxcli ready: %s\n", output)
	},
}

func init() {
	setupMxBuildCmd.Flags().String("version", "", "Mendix version to download (e.g., 11.6.3)")
	setupMxBuildCmd.Flags().Bool("dry-run", false, "Show what would be downloaded without downloading")
	setupMxBuildCmd.Flags().Bool("force", false, "Download the Linux CDN binary even on Windows/macOS (e.g. to pre-cache for Docker)")

	setupMxRuntimeCmd.Flags().String("version", "", "Mendix version to download (e.g., 11.6.3)")
	setupMxRuntimeCmd.Flags().Bool("dry-run", false, "Show what would be downloaded without downloading")

	setupMxcliCmd.Flags().String("os", "linux", "Target operating system (linux, darwin, windows)")
	setupMxcliCmd.Flags().String("arch", "amd64", "Target architecture (amd64, arm64)")
	setupMxcliCmd.Flags().String("output", "./mxcli", "Output file path")
	setupMxcliCmd.Flags().String("tag", "", "Release tag to download (default: match running version)")
	setupMxcliCmd.Flags().String("repo", "mendixlabs/mxcli", "GitHub repository")
	setupMxcliCmd.Flags().Bool("dry-run", false, "Show what would be downloaded without downloading")

	setupCmd.AddCommand(setupMxBuildCmd)
	setupCmd.AddCommand(setupMxRuntimeCmd)
	setupCmd.AddCommand(setupMxcliCmd)
	rootCmd.AddCommand(setupCmd)
}
