// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mendixlabs/mxcli/mdl/types"
)

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

// updateWidgetsCmd runs the mx invocation. It is a package variable so tests can
// substitute a stub that simulates the v2 -> v1 conversion without needing mx.
var updateWidgetsCmd = func(mxPath, pathArg string, w, stderr io.Writer) error {
	cmd := exec.Command(mxPath, "update-widgets", pathArg)
	cmd.Stdout = w
	cmd.Stderr = stderr
	PrepareMxCommand(cmd)
	return cmd.Run()
}

// runUpdateWidgets runs `mx update-widgets` on the project — normalizing pluggable
// widget definitions so the caller's check/build does not report false CE0463
// ("widget definition changed") errors — while preserving an MPRv2 project's
// on-disk storage format.
//
// The protection is needed because `mx update-widgets` rewrites an MPRv2 project
// into the self-contained MPRv1 format: it inlines every unit into the .mpr (adding
// a Unit.Contents column) and deletes mprcontents/. A command that checks or builds
// must not mutate the source project's storage format — doing so silently desyncs
// the working tree from a Git repository that tracks the mprcontents/ files, breaks
// a running `mxcli run --local` watch loop, and has been observed to leave Studio
// Pro unable to open the project. So on a v2 project the .mpr + mprcontents/ are
// snapshotted first and put back afterwards. MPRv1 projects are already single-file
// and need no protection.
//
// The caller must `defer restore()` rather than calling it immediately: the check /
// MxBuild step has to run against the widget-normalized model, or the CE0463 false
// positives this step exists to suppress come straight back. Only the on-disk
// format is restored, once the caller is done with the model.
//
// restore is never nil, is safe to defer, and never panics.
//
// This lives on the operation, not on a call site, because it was previously
// implemented in `Check` only — `Build` carried its own bare invocation and kept
// converting projects (mendixlabs/mxcli#763, then #808).
func runUpdateWidgets(mxPath, projectPath string, w, stderr io.Writer) (restore func()) {
	restore = func() {}
	if projectPath == "" {
		return restore
	}

	if reader, err := openReadOnly(projectPath); err == nil {
		isV2 := reader.Version() == types.MPRVersionV2
		contentsDir := reader.ContentsDir()
		reader.Disconnect()
		if isV2 {
			_, snapRestore, snapErr := snapshotStorageFormat(projectPath, contentsDir)
			if snapErr != nil {
				// Can't protect the format — skip update-widgets rather than risk an
				// unrecoverable v2 -> v1 conversion. A CE0463 false positive is the
				// lesser evil compared to a silent, unrestorable format change.
				fmt.Fprintf(w, "Warning: could not snapshot MPRv2 storage (skipping update-widgets to avoid a v2->v1 conversion): %v\n", snapErr)
				return restore
			}
			restore = snapRestore
		}
	}

	fmt.Fprintf(w, "Updating widget definitions in %s...\n", projectPath)
	if err := updateWidgetsCmd(mxPath, updateWidgetsPathArg(projectPath), w, stderr); err != nil {
		// Non-fatal: warn and let the caller continue. The snapshot is still restored
		// by the returned func — a failed run may have converted the project first.
		fmt.Fprintf(w, "Warning: update-widgets failed (continuing): %v\n", err)
		return restore
	}
	fmt.Fprintln(w, "Widget definitions updated.")
	return restore
}

// snapshotStorageFormat backs up the MPRv2 storage files (.mpr index + mprcontents/)
// to a temp directory and returns a restore function that puts them back, undoing
// any v2 -> v1 conversion performed by an intervening `mx update-widgets`. The
// restore function removes the temp directory and is safe to defer; it best-effort
// restores and never panics. mprPath and contentsDir come from a backend on a
// project already known to be MPRv2.
func snapshotStorageFormat(mprPath, contentsDir string) (dir string, restore func(), err error) {
	tmp, err := os.MkdirTemp("", "mxcli-mpr-snapshot-*")
	if err != nil {
		return "", nil, err
	}

	mprBackup := filepath.Join(tmp, filepath.Base(mprPath))
	if err := copyFile(mprPath, mprBackup); err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
	}

	contentsBackup := filepath.Join(tmp, "mprcontents")
	if err := copyDir(contentsDir, contentsBackup); err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
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
	return tmp, restore, nil
}
