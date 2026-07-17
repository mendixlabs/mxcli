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

	"github.com/mendixlabs/mxcli/sdk/mpr"
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

// updateWidgetsPathArg returns an absolute form of the .mpr path for the
// `mx update-widgets` invocation. MxToolset's AddProjectDirAsAllowedPath computes
// Path.GetDirectoryName(mprFilePath) to whitelist the project directory; given a
// bare filename (e.g. "app.mpr", as passed by `mxcli docker build -p app.mpr` run
// from the project dir) that returns "" → null and the tool throws
// System.ArgumentNullException, silently skipping the widget migration. That in
// turn leaves CE0463 "widget definition changed" errors unresolved at check time.
// An absolute path always has a directory component. `mx check` is unaffected, so
// only the update-widgets arg is normalized. Falls back to the input if Abs fails.
func updateWidgetsPathArg(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// snapshotStorageFormat backs up the MPRv2 storage files (.mpr index + mprcontents/)
// to a temp directory and returns a restore function that puts them back, undoing
// any v2 -> v1 conversion performed by an intervening `mx update-widgets`. The
// restore function removes the temp directory and is safe to defer; it best-effort
// restores and never panics. mprPath and contentsDir come from an mpr.Reader on a
// project already known to be MPRv2.
func snapshotStorageFormat(mprPath, contentsDir string) (restore func(), err error) {
	tmp, err := os.MkdirTemp("", "mxcli-mpr-snapshot-*")
	if err != nil {
		return nil, err
	}

	mprBackup := filepath.Join(tmp, filepath.Base(mprPath))
	if err := copyFile(mprPath, mprBackup); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}

	contentsBackup := filepath.Join(tmp, "mprcontents")
	if err := copyDir(contentsDir, contentsBackup); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}

	restore = func() {
		defer os.RemoveAll(tmp)
		// Restore the v2 index file.
		_ = copyFile(mprBackup, mprPath)
		// update-widgets deletes mprcontents/; drop whatever is there now (nothing,
		// after a conversion) and restore the backed-up tree.
		_ = os.RemoveAll(contentsDir)
		_ = copyDir(contentsBackup, contentsDir)
	}
	return restore, nil
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
		if reader, err := mpr.Open(opts.ProjectPath); err == nil {
			projectVersion = reader.ProjectVersion().ProductVersion
			reader.Close()
		}
	}

	mxPath, err := ResolveMxForVersion(opts.MxBuildPath, projectVersion)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Using mx: %s\n", mxPath)

	// `mx update-widgets` rewrites an MPRv2 project into the self-contained MPRv1
	// storage format: it inlines every unit into the .mpr and deletes mprcontents/.
	// A `check` must not mutate the on-disk storage format — silently doing so
	// desyncs the working tree from a Git repository that tracks the mprcontents/
	// files and can leave Studio Pro unable to open the project. So when the project
	// is MPRv2, snapshot the .mpr + mprcontents/ before update-widgets and restore
	// them after the check. The check still runs against the widget-normalized model
	// (so CE0463 false positives are still suppressed); only the on-disk format is
	// preserved. MPRv1 projects are already single-file and need no protection.
	if !opts.SkipUpdateWidgets && opts.ProjectPath != "" {
		if reader, err := mpr.Open(opts.ProjectPath); err == nil {
			isV2 := reader.Version() == mpr.MPRVersionV2
			contentsDir := reader.ContentsDir()
			reader.Close()
			if isV2 {
				restore, snapErr := snapshotStorageFormat(opts.ProjectPath, contentsDir)
				if snapErr != nil {
					// Can't protect the format — skip update-widgets rather than risk
					// an unrecoverable v2 -> v1 conversion. A CE0463 false positive is
					// the lesser evil than a silent, unrestorable format change.
					fmt.Fprintf(w, "Warning: could not snapshot MPRv2 storage (skipping update-widgets to avoid a v2->v1 conversion): %v\n", snapErr)
					opts.SkipUpdateWidgets = true
				} else {
					defer restore()
				}
			}
		}
	}

	// Run mx update-widgets to normalize pluggable widget definitions.
	// This prevents false CE0463 ("widget definition changed") errors caused
	// by mismatch between widget Object properties and Type PropertyTypes.
	if !opts.SkipUpdateWidgets {
		fmt.Fprintf(w, "Updating widget definitions in %s...\n", opts.ProjectPath)
		uwCmd := exec.Command(mxPath, "update-widgets", updateWidgetsPathArg(opts.ProjectPath))
		uwCmd.Stdout = w
		uwCmd.Stderr = stderr
		PrepareMxCommand(uwCmd)
		if err := uwCmd.Run(); err != nil {
			// Non-fatal: warn and continue with check
			fmt.Fprintf(w, "Warning: update-widgets failed (continuing with check): %v\n", err)
		} else {
			fmt.Fprintln(w, "Widget definitions updated.")
		}
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

// ResolveMxForNewProject finds the mx binary for use by mxcli new.
// On Windows and macOS it prefers an installed Studio Pro to avoid downloading
// Linux CDN binaries that won't execute on those platforms. On Linux (and as a
// fallback) it downloads mxbuild from the CDN and derives mx from the same dir.
func ResolveMxForNewProject(version string, progressWriter io.Writer) (string, error) {
	// Fast path: Studio Pro or cached download already present
	if mxPath, err := ResolveMxForVersion("", version); err == nil {
		return mxPath, nil
	}
	// Slow path: download mxbuild from CDN (works on Linux; on macOS/Windows
	// this is only reached if Studio Pro is not installed)
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
