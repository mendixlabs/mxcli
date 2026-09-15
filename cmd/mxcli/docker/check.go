// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CheckOptions configures the mx check command.
type CheckOptions struct {
	// ProjectPath is the path to the .mpr file.
	ProjectPath string

	// MxBuildPath is an explicit path to the mxbuild executable (used to find mx).
	MxBuildPath string

	// SkipUpdateWidgets skips the 'mx update-widgets' step before checking.
	// By default, update-widgets runs first to normalize pluggable widget
	// definitions and prevent false CE0463 errors.
	SkipUpdateWidgets bool

	// Stdout for output messages.
	Stdout io.Writer

	// Stderr for error output.
	Stderr io.Writer
}

// copyFile copies a single file from src to dst, preserving the source file mode.
func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// Check runs 'mx check' on the project to validate it before building.
func Check(opts CheckOptions) error {
	w := opts.Stdout
	if w == nil {
		w = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	// Resolve mx binary
	projectVersion := ""
	if opts.ProjectPath != "" {
		if reader, err := openReadOnly(opts.ProjectPath); err == nil {
			projectVersion = reader.ProjectVersion().ProductVersion
			reader.Disconnect()
		}
	}

	mxPath, err := ResolveMxForVersion(opts.MxBuildPath, projectVersion)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Using mx: %s\n", mxPath)

	// Normalize pluggable widget definitions so `mx check` does not report false
	// CE0463 ("widget definition changed") errors. runUpdateWidgets preserves the
	// project's on-disk storage format; restore is deferred so the check below still
	// runs against the widget-normalized model.
	if !opts.SkipUpdateWidgets {
		restore := runUpdateWidgets(mxPath, opts.ProjectPath, w, stderr)
		defer restore()
	}

	// Run mx check
	fmt.Fprintf(w, "Checking project %s...\n", opts.ProjectPath)
	cmd := exec.Command(mxPath, "check", opts.ProjectPath)
	cmd.Stdout = w
	cmd.Stderr = stderr
	PrepareMxCommand(cmd)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("project check failed: %w", err)
	}

	fmt.Fprintln(w, "Project check passed.")
	return nil
}

// mxBinaryName returns the platform-specific mx binary name.
func mxBinaryName() string {
	if runtime.GOOS == "windows" {
		return "mx.exe"
	}
	return "mx"
}

func mxBinaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"mx.exe", "mx"}
	}
	return []string{"mx"}
}

// ResolveMx finds the mx executable.
// Priority: derive from mxbuild path > PATH lookup.
func ResolveMx(mxbuildPath string) (string, error) {
	return ResolveMxForVersion(mxbuildPath, "")
}

// ResolveMxForVersion finds the mx executable, preferring the project's exact
// Mendix version when multiple local installations or cached downloads exist.
func ResolveMxForVersion(mxbuildPath, preferredVersion string) (string, error) {
	if mxbuildPath != "" {
		// Resolve mxbuild first to handle directory paths
		resolvedMxBuild, err := resolveMxBuild(mxbuildPath, preferredVersion)
		if err == nil {
			// Look for mx in the same directory as mxbuild
			mxDir := filepath.Dir(resolvedMxBuild)
			candidate := filepath.Join(mxDir, mxBinaryName())
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}

			// Try deriving mx name from mxbuild name (e.g. mxbuild11.6.3 -> mx11.6.3)
			mxbuildBase := filepath.Base(resolvedMxBuild)
			suffix := strings.TrimPrefix(mxbuildBase, "mxbuild")
			if runtime.GOOS == "windows" {
				suffix = strings.TrimPrefix(mxbuildBase, "mxbuild")
				suffix = strings.TrimSuffix(suffix, ".exe")
				candidate = filepath.Join(mxDir, "mx"+suffix+".exe")
			} else {
				candidate = filepath.Join(mxDir, "mx"+suffix)
			}
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}

	// Try PATH
	if p, err := exec.LookPath("mx"); err == nil {
		return p, nil
	}

	if preferredVersion != "" {
		if studioProDir := ResolveStudioProDir(preferredVersion); studioProDir != "" {
			for _, name := range mxBinaryNames() {
				candidate := filepath.Join(studioProDir, "modeler", name)
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					return candidate, nil
				}
			}
		}
	}

	// Try OS-specific known locations (Studio Pro on Windows) before cached downloads.
	if matches := globVersionedMatches(mendixSearchPaths(mxBinaryName())); len(matches) > 0 {
		if exact := exactVersionedPath(matches, preferredVersion); exact != "" {
			return exact, nil
		}
		if newest := NewestVersionedPath(matches); newest != "" {
			return newest, nil
		}
	}

	if preferredVersion != "" {
		if p := CachedMxPath(preferredVersion); p != "" {
			return p, nil
		}
	}
	if p := AnyCachedMxPath(); p != "" {
		return p, nil
	}

	return "", fmt.Errorf("mx not found; specify --mxbuild-path pointing to Mendix installation directory")
}

// localMxForVersion returns a locally available mx binary that is EXACTLY the
// requested version, or "" if there is none.
//
// Unlike ResolveMxForVersion it never substitutes a different version. That
// substitution is reasonable when the project already exists and its version is a
// preference; it is wrong when the version IS the request — see
// ResolveMxForNewProject.
//
// Looks in the same places, minus the "any version will do" fallbacks: an
// exact-version Studio Pro install, an exact-version binary in the OS-specific
// install locations, then the exact-version download cache. PATH is deliberately
// excluded: an `mx` on PATH carries no version guarantee.
func localMxForVersion(version string) string {
	if version == "" {
		return ""
	}
	if studioProDir := ResolveStudioProDir(version); studioProDir != "" {
		for _, name := range mxBinaryNames() {
			candidate := filepath.Join(studioProDir, "modeler", name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	if matches := globVersionedMatches(mendixSearchPaths(mxBinaryName())); len(matches) > 0 {
		if exact := exactVersionedPath(matches, version); exact != "" {
			return exact
		}
	}
	return CachedMxPath(version)
}

// ResolveMxForNewProject finds the mx binary for use by mxcli new.
//
// The requested version is not a preference here, it is the definition of the
// output: `mx create-project` stamps the new project with the version of the binary
// that created it. So only an EXACT match may be reused; anything else is
// downloaded. Delegating to ResolveMxForVersion was wrong, because its last resort
// is AnyCachedMxPath() — with only 11.6.6 cached, `mxcli new --version 11.12.2`
// printed "Resolving MxBuild 11.12.2..." and then produced an 11.6.6 project, with
// no warning that the requested version had been ignored (mendixlabs/mxcli#808 era
// finding; the CDN had the version all along).
//
// On Windows and macOS an exact-version Studio Pro is still preferred, so those
// platforms do not download a Linux CDN binary that cannot execute.
func ResolveMxForNewProject(version string, progressWriter io.Writer) (string, error) {
	if mxPath := localMxForVersion(version); mxPath != "" {
		return mxPath, nil
	}
	// Not available locally at the requested version: download it (works on Linux;
	// on macOS/Windows this is only reached when Studio Pro is not installed).
	mxbuildPath, err := DownloadMxBuild(version, progressWriter)
	if err != nil {
		return "", err
	}
	return ResolveMx(mxbuildPath)
}

func CachedMxPath(version string) string {
	cacheDir, err := MxBuildCacheDir(version)
	if err != nil {
		return ""
	}
	return cachedBinaryPath(cacheDir, mxBinaryNames())
}

func AnyCachedMxPath() string {
	return anyCachedBinaryPath(mxBinaryNames())
}
