// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr/version"
)

// BuildOptions configures the docker build command.
type BuildOptions struct {
	// ProjectPath is the path to the .mpr file.
	ProjectPath string

	// MxBuildPath is an explicit path to the mxbuild executable.
	MxBuildPath string

	// OutputDir is the output directory for the PAD package. Defaults to a directory next to the .mpr.
	OutputDir string

	// DryRun only performs detection steps without running MxBuild.
	DryRun bool

	// SkipCheck skips the 'mx check' pre-build validation.
	SkipCheck bool

	// SkipUpdateWidgets skips the 'mx update-widgets' step before checking.
	SkipUpdateWidgets bool

	// Stdout for output messages.
	Stdout io.Writer
}

// Build runs MxBuild to create a Portable App Distribution package and applies patches.
func Build(opts BuildOptions) error {
	w := opts.Stdout
	if w == nil {
		w = os.Stdout
	}

	// Step 1: Detect version
	fmt.Fprintln(w, "Detecting project version...")
	reader, err := mpr.Open(opts.ProjectPath)
	if err != nil {
		return fmt.Errorf("opening project: %w", err)
	}
	pv := reader.ProjectVersion()
	reader.Close()

	fmt.Fprintf(w, "  Mendix version: %s\n", pv.ProductVersion)

	if !pv.IsAtLeastFull(11, 6, 1) {
		return fmt.Errorf("portable app distribution requires Mendix >= 11.6.1, found %s", pv.ProductVersion)
	}

	// Step 2: Resolve MxBuild
	fmt.Fprintln(w, "Resolving MxBuild...")
	mxbuildPath, err := resolveMxBuild(opts.MxBuildPath, pv.ProductVersion)
	if err != nil {
		// Auto-download fallback
		fmt.Fprintln(w, "  MxBuild not found locally, downloading from CDN...")
		mxbuildPath, err = DownloadMxBuild(pv.ProductVersion, w)
		if err != nil {
			return fmt.Errorf("downloading mxbuild: %w", err)
		}
	}
	fmt.Fprintf(w, "  MxBuild: %s\n", mxbuildPath)

	// Step 2b: Ensure PAD runtime files are linked (only needed for CDN downloads,
	// Studio Pro installations already have runtime files in place).
	if ResolveStudioProDir(pv.ProductVersion) == "" {
		if err := ensurePADFiles(pv.ProductVersion, w); err != nil {
			fmt.Fprintf(w, "  Warning: %v\n", err)
		}
	}

	// Step 3: Resolve JDK 21
	fmt.Fprintln(w, "Resolving JDK 21...")
	javaHome, err := resolveJDK21()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  JAVA_HOME: %s\n", javaHome)
	fmt.Fprintf(w, "  Java: %s\n", javaVersionString(javaHome))

	// Step 4: Pre-build check
	if !opts.SkipCheck {
		fmt.Fprintln(w, "Checking project for errors...")
		mxPath, err := ResolveMxForVersion(opts.MxBuildPath, pv.ProductVersion)
		if err != nil {
			fmt.Fprintf(w, "  Skipping check: %v\n", err)
		} else {
			// Run update-widgets before check to prevent false CE0463 errors
			if !opts.SkipUpdateWidgets {
				fmt.Fprintln(w, "  Updating widget definitions...")
				uwCmd := exec.Command(mxPath, "update-widgets", updateWidgetsPathArg(opts.ProjectPath))
				uwCmd.Stdout = w
				uwCmd.Stderr = os.Stderr
				PrepareMxCommand(uwCmd)
				if err := uwCmd.Run(); err != nil {
					fmt.Fprintf(w, "  Warning: update-widgets failed (continuing): %v\n", err)
				}
			}

			cmd := exec.Command(mxPath, "check", opts.ProjectPath)
			cmd.Stdout = w
			cmd.Stderr = os.Stderr
			PrepareMxCommand(cmd)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("project has errors (fix them or use --skip-check to bypass): %w", err)
			}
			fmt.Fprintln(w, "  Project check passed.")
		}
	}

	// Dry-run: stop here and show what would happen
	if opts.DryRun {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Dry run — post-build steps:")
		fmt.Fprintln(w, "  - Extract PAD ZIP if produced by MxBuild")
		fmt.Fprintln(w, "  - Locate PAD output (Dockerfile or docker_compose/Default.yaml)")
		fmt.Fprintln(w, "  - Generate Dockerfile if not produced by MxBuild")
		if cached := CachedRuntimePath(pv.ProductVersion); cached != "" {
			fmt.Fprintf(w, "  - Runtime: already cached at %s\n", cached)
		} else {
			fmt.Fprintf(w, "  - Download Mendix runtime %s from CDN (if not in PAD)\n", pv.ProductVersion)
		}
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Patches that would be applied:")
		is116x := pv.MajorVersion == 11 && pv.MinorVersion == 6
		if is116x {
			fmt.Fprintln(w, "  - Set bin/start execute permission (11.6.x)")
			fmt.Fprintln(w, "  - Fix Dockerfile CMD (start.sh -> start) (11.6.x)")
		}
		fmt.Fprintln(w, "  - Remove config arg from Dockerfile CMD")
		fmt.Fprintln(w, "  - Replace deprecated openjdk base image")
		fmt.Fprintln(w, "  - Add HEALTHCHECK instruction")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Dry run complete. No changes made.")
		return nil
	}

	// Step 5: Run MxBuild
	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(filepath.Dir(opts.ProjectPath), ".docker", "build")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	fmt.Fprintf(w, "Running MxBuild (target=portable-app-package)...\n")
	fmt.Fprintf(w, "  Output: %s\n", outputDir)

	javaExePath := filepath.Join(javaHome, "bin", "java")

	cmd := exec.Command(mxbuildPath,
		"--target=portable-app-package",
		fmt.Sprintf("--java-home=%s", javaHome),
		fmt.Sprintf("--java-exe-path=%s", javaExePath),
		fmt.Sprintf("-o=%s", outputDir),
		opts.ProjectPath,
	)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	PrepareMxCommand(cmd)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mxbuild failed: %w", err)
	}

	// Step 5b: Extract PAD ZIP if MxBuild produced one
	if err := extractPADZip(outputDir, w); err != nil {
		return fmt.Errorf("extracting PAD zip: %w", err)
	}

	// Step 6: Locate PAD output directory
	padDir, err := findPADDir(outputDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  PAD output: %s\n", padDir)

	// Step 6a: Flatten PAD to build root for volume mounts
	// Docker-compose mounts ./build:/mendix, so PAD contents must be at the top of outputDir.
	if padDir != outputDir {
		fmt.Fprintln(w, "  Flattening PAD to build directory...")
		padDir, err = flattenPADDir(padDir, outputDir)
		if err != nil {
			return fmt.Errorf("flattening PAD: %w", err)
		}
	}

	// Step 6b: Generate Dockerfile if MxBuild didn't produce one
	if _, err := os.Stat(filepath.Join(padDir, "Dockerfile")); os.IsNotExist(err) {
		fmt.Fprintln(w, "Generating Dockerfile (not produced by MxBuild)...")
		if err := generateDockerfile(padDir); err != nil {
			return fmt.Errorf("generating Dockerfile: %w", err)
		}
		fmt.Fprintln(w, "  Dockerfile generated.")
	}

	// Step 6c: Inject runtime if not present in PAD
	if err := injectRuntime(padDir, pv.ProductVersion, w); err != nil {
		return fmt.Errorf("injecting runtime: %w", err)
	}

	// Step 7: Apply patches
	fmt.Fprintln(w, "Applying patches...")
	results := ApplyPatches(padDir, pv)
	for _, r := range results {
		if r.Error != nil {
			fmt.Fprintf(w, "  [error]   %s: %v\n", r.Description, r.Error)
		} else {
			fmt.Fprintf(w, "  [%s] %s\n", r.Status, r.Description)
		}
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Build complete.")
	return nil
}

// RunOptions configures the docker run command (build + start in one step).
type RunOptions struct {
	// ProjectPath is the path to the .mpr file.
	ProjectPath string

	// MxBuildPath is an explicit path to the mxbuild executable.
	MxBuildPath string

	// SkipCheck skips the 'mx check' pre-build validation.
	SkipCheck bool

	// Fresh removes volumes before starting.
	Fresh bool

	// Wait waits for the runtime to report successful startup.
	Wait bool

	// WaitTimeout is the timeout for waiting (default: 5 minutes).
	WaitTimeout time.Duration

	// Stdout for output messages.
	Stdout io.Writer

	// Stderr for error output.
	Stderr io.Writer

	// PortOffset shifts all default ports by N when initializing the Docker stack.
	PortOffset int
}

// Run orchestrates the full Docker workflow: setup, init, build, and start.
// This is the single-command equivalent of running setup mxbuild, setup mxruntime,
// docker init, docker build, and docker up separately.
func Run(opts RunOptions) error {
	w := opts.Stdout
	if w == nil {
		w = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	// Step 1: Detect version
	fmt.Fprintln(w, "Detecting project version...")
	reader, err := mpr.Open(opts.ProjectPath)
	if err != nil {
		return fmt.Errorf("opening project: %w", err)
	}
	pv := reader.ProjectVersion()
	reader.Close()
	fmt.Fprintf(w, "  Mendix version: %s\n", pv.ProductVersion)

	// Step 2 & 3: Ensure MxBuild and runtime are available.
	// On Windows, prefer Studio Pro installation directory — it ships both mxbuild.exe
	// and runtime/, so no CDN download is needed.
	studioProDir := ResolveStudioProDir(pv.ProductVersion)
	if studioProDir != "" {
		fmt.Fprintf(w, "Using Studio Pro installation: %s\n", studioProDir)
		// Point MxBuildPath so Build() uses Studio Pro's mxbuild.exe
		if opts.MxBuildPath == "" {
			opts.MxBuildPath = studioProDir
		}
	} else {
		fmt.Fprintln(w, "Ensuring MxBuild is available...")
		_, err = DownloadMxBuild(pv.ProductVersion, w)
		if err != nil {
			return fmt.Errorf("setting up mxbuild: %w", err)
		}

		fmt.Fprintln(w, "Ensuring Mendix runtime is available...")
		_, err = DownloadRuntime(pv.ProductVersion, w)
		if err != nil {
			return fmt.Errorf("setting up runtime: %w", err)
		}

		// Link PAD runtime files into mxbuild directory.
		// MxBuild's PAD builder expects template files at mxbuild/{ver}/runtime/pad/,
		// but they live in the separately downloaded runtime at runtime/{ver}/runtime/pad/.
		// Non-fatal: some Mendix versions don't include PAD files in the runtime archive.
		if err := ensurePADFiles(pv.ProductVersion, w); err != nil {
			fmt.Fprintf(w, "  Warning: %v\n", err)
		}
	}

	// Step 3c: Ensure demo users exist
	// Blank projects created by mx create-project have no demo users, which means
	// the app starts but login fails. Create a default admin if none exist.
	if err := ensureDemoUsers(opts.ProjectPath, w); err != nil {
		// Non-fatal: warn but continue — the app will start, just login won't work.
		fmt.Fprintf(w, "  Warning: could not ensure demo users: %v\n", err)
	}

	// Step 4: Initialize Docker stack (idempotent)
	dockerDir := filepath.Join(filepath.Dir(opts.ProjectPath), ".docker")
	composePath := filepath.Join(dockerDir, "docker-compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		fmt.Fprintln(w, "Initializing Docker stack...")
		initOpts := InitOptions{
			ProjectPath: opts.ProjectPath,
			PortOffset:  opts.PortOffset,
			Stdout:      w,
		}
		if err := Init(initOpts); err != nil {
			return fmt.Errorf("docker init: %w", err)
		}
	} else {
		fmt.Fprintln(w, "Docker stack already initialized.")
	}

	// Step 5: Build
	fmt.Fprintln(w, "")
	buildOpts := BuildOptions{
		ProjectPath: opts.ProjectPath,
		MxBuildPath: opts.MxBuildPath,
		SkipCheck:   opts.SkipCheck,
		Stdout:      w,
	}
	if err := Build(buildOpts); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}

	// Step 6: Start
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Starting containers...")
	rtOpts := RuntimeOptions{
		ProjectPath: opts.ProjectPath,
		Stdout:      w,
		Stderr:      stderr,
	}
	if err := Up(rtOpts, true, opts.Fresh); err != nil {
		return fmt.Errorf("docker up: %w", err)
	}

	// Step 7: Wait for ready (if requested)
	if opts.Wait {
		timeout := opts.WaitTimeout
		if timeout == 0 {
			timeout = 5 * time.Minute
		}
		if err := WaitForReady(rtOpts, timeout); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Containers started in background.")
		fmt.Fprintln(w, "  Check status:  mxcli docker status -p "+opts.ProjectPath)
		fmt.Fprintln(w, "  View logs:     mxcli docker logs -p "+opts.ProjectPath+" --follow")
		fmt.Fprintln(w, "  Wait for app:  mxcli docker up -p "+opts.ProjectPath+" -d --wait")
	}

	return nil
}

// findPADDir searches for a PAD output in the output directory tree.
// It recognizes:
//   - classic PAD output with a Dockerfile
//   - PAD output with docker_compose/Default.yaml
//   - newer PAD output already extracted at the build root (app/bin/etc/lib)
func findPADDir(outputDir string) (string, error) {
	// Check output dir itself
	if isPADDir(outputDir) {
		return outputDir, nil
	}

	// Check immediate subdirectories
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return "", fmt.Errorf("reading output directory: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			candidate := filepath.Join(outputDir, e.Name())
			if isPADDir(candidate) {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("no PAD output found in %s (looked for Dockerfile, docker_compose/Default.yaml, or extracted app/bin/etc/lib layout)", outputDir)
}

// isPADDir checks if a directory contains PAD output.
// Recognizes Dockerfile-based PAD, docker_compose-based PAD, and newer
// already-extracted PAD layout at the build root.
func isPADDir(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "docker_compose", "Default.yaml")); err == nil {
		return true
	}
	return hasExtractedPADLayout(dir)
}

func hasExtractedPADLayout(dir string) bool {
	requiredDirs := []string{
		filepath.Join(dir, "app"),
		filepath.Join(dir, "bin"),
		filepath.Join(dir, "etc"),
		filepath.Join(dir, "lib"),
	}
	for _, path := range requiredDirs {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return false
		}
	}

	requiredFiles := []string{
		filepath.Join(dir, "bin", "start"),
		filepath.Join(dir, "lib", "runtime", "launcher", "runtimelauncher.jar"),
	}
	for _, path := range requiredFiles {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return false
		}
	}

	return true
}

// flattenPADDir moves contents from a PAD subdirectory to the output root directory.
// This ensures PAD contents are at the top of the build dir for volume mounts
// (docker-compose mounts ./build:/mendix).
func flattenPADDir(padDir, outputDir string) (string, error) {
	entries, err := os.ReadDir(padDir)
	if err != nil {
		return "", fmt.Errorf("reading PAD dir: %w", err)
	}
	for _, e := range entries {
		src := filepath.Join(padDir, e.Name())
		dst := filepath.Join(outputDir, e.Name())
		// Remove existing destination (from previous build)
		os.RemoveAll(dst)
		if err := os.Rename(src, dst); err != nil {
			return "", fmt.Errorf("moving %s to build root: %w", e.Name(), err)
		}
	}
	// Remove the now-empty subdirectory
	os.Remove(padDir)
	return outputDir, nil
}

// generateDockerfile creates a Dockerfile when MxBuild doesn't produce one
// (MxBuild 11.6.3+ produces docker_compose/Default.yaml instead).
func generateDockerfile(padDir string) error {
	dockerfilePath := filepath.Join(padDir, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err == nil {
		// Dockerfile already exists
		return nil
	}

	content := `FROM eclipse-temurin:21-jre
WORKDIR /mendix
COPY ./app ./app
COPY ./bin ./bin
COPY ./etc ./etc
COPY ./lib ./lib
ENV MX_LOG_LEVEL=info
EXPOSE 8080 8090
HEALTHCHECK --interval=15s --timeout=5s --start-period=30s --retries=3 \
  CMD curl -f http://localhost:8080/ || exit 1
CMD ["./bin/start"]
`
	return os.WriteFile(dockerfilePath, []byte(content), 0644)
}

// injectRuntime downloads the Mendix runtime (if not present in the PAD) and copies it
// into the PAD lib/runtime/ directory so the start script can find runtimelauncher.jar.
func injectRuntime(padDir string, version string, w io.Writer) error {
	// Check if runtime is already present in the PAD
	launcherJar := filepath.Join(padDir, "lib", "runtime", "launcher", "runtimelauncher.jar")
	if _, err := os.Stat(launcherJar); err == nil {
		fmt.Fprintln(w, "  Runtime already present in PAD output.")
		return nil
	}

	fmt.Fprintln(w, "Runtime not found in PAD output, downloading...")
	cacheDir, err := DownloadRuntime(version, w)
	if err != nil {
		return fmt.Errorf("downloading runtime: %w", err)
	}

	// Copy runtime from cache into the PAD
	src := filepath.Join(cacheDir, "runtime")
	dst := filepath.Join(padDir, "lib", "runtime")
	fmt.Fprintf(w, "  Copying runtime to %s...\n", dst)
	if err := copyDir(src, dst); err != nil {
		return fmt.Errorf("copying runtime to PAD: %w", err)
	}

	return nil
}

// copyDir recursively copies a directory tree from src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Compute destination path
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		// Copy file
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}

// extractPADZip finds the latest .zip in the output directory and extracts it.
// MxBuild outputs a ZIP file (e.g., MyApp_1.0.0.1.zip) containing the PAD contents.
// If no ZIP is found, this is a no-op. ZIPs are always extracted even if a previous
// build's PAD output exists — the new ZIP represents the latest build.
func extractPADZip(outputDir string, w io.Writer) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil // empty dir, let findPADDir handle the error
	}

	// Find ZIP files first — if MxBuild produced new ZIPs, always extract them.
	var zips []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
			zips = append(zips, filepath.Join(outputDir, e.Name()))
		}
	}
	if len(zips) == 0 {
		return nil
	}

	// Use the latest (by name sort — typically includes version)
	sort.Strings(zips)
	zipPath := zips[len(zips)-1]
	fmt.Fprintf(w, "Extracting PAD package: %s\n", filepath.Base(zipPath))

	if err := extractZip(zipPath, outputDir); err != nil {
		return fmt.Errorf("extracting %s: %w", filepath.Base(zipPath), err)
	}

	// Clean up all ZIP files to prevent accumulation across rebuilds
	for _, z := range zips {
		os.Remove(z)
	}
	fmt.Fprintf(w, "  Cleaned up %d ZIP file(s)\n", len(zips))

	return nil
}

// extractZip extracts a ZIP archive to the target directory.
func extractZip(zipPath, targetDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		name := f.Name
		// Sanitize: skip entries with path traversal
		if strings.Contains(name, "..") {
			continue
		}

		target := filepath.Join(targetDir, filepath.FromSlash(name))

		// Ensure the target is within targetDir
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(targetDir)) {
			continue
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("creating parent directory for %s: %w", target, err)
		}

		// Extract file
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("opening %s in zip: %w", name, err)
		}

		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return fmt.Errorf("creating file %s: %w", target, err)
		}

		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			return fmt.Errorf("writing file %s: %w", target, err)
		}

		outFile.Close()
		rc.Close()
	}

	return nil
}

// ensurePADFiles ensures that PAD runtime template files are available in the mxbuild
// directory. MxBuild's PAD builder expects files at ~/.mxcli/mxbuild/{ver}/runtime/pad/
// but the runtime is downloaded separately to ~/.mxcli/runtime/{ver}/runtime/pad/.
// This creates a symlink from the mxbuild location to the runtime location.
func ensurePADFiles(productVersion string, w io.Writer) error {
	mxbuildDir, err := MxBuildCacheDir(productVersion)
	if err != nil {
		return err
	}
	runtimeDir, err := RuntimeCacheDir(productVersion)
	if err != nil {
		return err
	}

	mxbuildPAD := filepath.Join(mxbuildDir, "runtime", "pad")
	runtimePAD := filepath.Join(runtimeDir, "runtime", "pad")

	// Already exists (previous run, or bundled with mxbuild)
	if _, err := os.Stat(mxbuildPAD); err == nil {
		fmt.Fprintln(w, "  PAD runtime files already present.")
		return nil
	}

	// Check that the runtime PAD source exists; if not, the cached runtime is stale
	// (downloaded before PAD files were included). Invalidate and re-download.
	if _, err := os.Stat(runtimePAD); err != nil {
		fmt.Fprintf(w, "  PAD files missing from cached runtime, re-downloading...\n")
		os.RemoveAll(runtimeDir)
		if _, err := DownloadRuntime(productVersion, w); err != nil {
			return fmt.Errorf("re-downloading runtime for PAD files: %w", err)
		}
		// Check again — some versions simply don't ship PAD files.
		if _, err := os.Stat(runtimePAD); err != nil {
			return fmt.Errorf("runtime PAD files not found at %s (not included in this Mendix version)", runtimePAD)
		}
	}

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(mxbuildPAD), 0755); err != nil {
		return fmt.Errorf("creating runtime directory in mxbuild: %w", err)
	}

	// Symlink runtime/pad into mxbuild
	if err := os.Symlink(runtimePAD, mxbuildPAD); err != nil {
		return fmt.Errorf("symlinking PAD files: %w", err)
	}
	fmt.Fprintf(w, "  Linked PAD runtime files: %s -> %s\n", mxbuildPAD, runtimePAD)
	return nil
}

// ensureDemoUsers checks whether the project has demo users configured.
// If not, it enables demo users and creates a default admin user so the
// application is accessible after startup.
func ensureDemoUsers(projectPath string, w io.Writer) error {
	fmt.Fprintln(w, "Checking demo users...")

	reader, err := mpr.Open(projectPath)
	if err != nil {
		return fmt.Errorf("opening project: %w", err)
	}

	ps, err := reader.GetProjectSecurity()
	reader.Close()
	if err != nil {
		return fmt.Errorf("reading project security: %w", err)
	}

	// If demo users already exist, nothing to do
	if len(ps.DemoUsers) > 0 {
		fmt.Fprintf(w, "  Found %d demo user(s), skipping.\n", len(ps.DemoUsers))
		return nil
	}

	fmt.Fprintln(w, "  No demo users found, creating default admin...")

	writer, err := mpr.NewWriter(projectPath)
	if err != nil {
		return fmt.Errorf("opening project for writing: %w", err)
	}
	defer writer.Close()

	// Re-read security through writer's reader
	ps, err = writer.Reader().GetProjectSecurity()
	if err != nil {
		return fmt.Errorf("reading project security: %w", err)
	}

	// Enable demo users if not already enabled
	if !ps.EnableDemoUsers {
		if err := writer.SetProjectDemoUsersEnabled(ps.ID, true); err != nil {
			return fmt.Errorf("enabling demo users: %w", err)
		}
		fmt.Fprintln(w, "  Enabled demo users.")
	}

	// Pick the first user role that looks like an admin, or fall back to the first role
	roleName := "Administrator"
	if len(ps.UserRoles) > 0 {
		roleName = ps.UserRoles[0].Name
		for _, ur := range ps.UserRoles {
			if ur.Name == "Administrator" || ur.Name == "Admin" {
				roleName = ur.Name
				break
			}
		}
	}

	if err := writer.AddDemoUser(ps.ID, "admin", "Admin123!", "", []string{roleName}); err != nil {
		return fmt.Errorf("creating demo user: %w", err)
	}

	fmt.Fprintf(w, "  Created demo user: admin / Admin123! (role: %s)\n", roleName)
	return nil
}

// DescribePatches returns the list of patches that would be applied for a given version.
func DescribePatches(pv *version.ProjectVersion) []string {
	var patches []string
	is116x := pv.MajorVersion == 11 && pv.MinorVersion == 6
	patches = append(patches, "Set bin/start execute permission")
	if is116x {
		patches = append(patches, "Fix Dockerfile CMD start.sh -> start (11.6.x)")
	}
	patches = append(patches, "Remove config arg from Dockerfile CMD")
	patches = append(patches, "Replace deprecated openjdk base image")
	patches = append(patches, "Add HEALTHCHECK instruction")
	patches = append(patches, "Bind admin API to all interfaces")
	patches = append(patches, "Set runtime/admin ports (avoid port 0 startup crash)")
	return patches
}
