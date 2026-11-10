// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// The restore record lets a process other than the dev loop undo what
// --test-endpoint installed. The dev loop removes the endpoint on its own way
// out, but it does not always get one: on Windows `run stop` ends a detached run
// with `taskkill /F`, which no handler sees, and anywhere a crash or a SIGKILL
// skips it too. The project was then left carrying the MxTest module, an
// after-startup pointing at it and themesource/mxtest/, and the next build
// failed with CE1613 once the module was dropped (ako/mxcli#1917,
// mendixlabs/mxcli#1723).
//
// So what Remove needs is written to disk before anything is installed — the
// original after-startup, whether MxTest pre-existed, and the project file as it
// was — and whoever finds the record with its owner gone (`run stop`, or the
// next --test-endpoint run) performs the same removal.
const (
	restoreRecordName = "test-endpoint-restore.json"
	restoreMprName    = "test-endpoint-restore.mpr"
)

type restoreRecord struct {
	// PID is the dev loop that installed the endpoint. While it is alive the
	// endpoint is its to remove.
	PID           int    `json:"pid"`
	AfterStartup  string `json:"afterStartup"`
	CreatedMxTest bool   `json:"createdMxTest"`
	// TreeDigest pairs with the saved project file, which is only put back while
	// the document tree still matches it (see projectSnapshot.restore).
	TreeDigest string `json:"treeDigest,omitempty"`
}

func restoreRecordPath(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), ".mxcli", restoreRecordName)
}

func restoreMprPath(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), ".mxcli", restoreMprName)
}

// writeRestoreRecord saves st for pid. The project-file copy is written before
// the record that refers to it, and the record through a rename, so a reader
// never finds a record whose copy is missing or half written.
func writeRestoreRecord(projectPath string, pid int, st projectState) error {
	dir := filepath.Dir(restoreRecordPath(projectPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	rec := restoreRecord{PID: pid, AfterStartup: st.afterStartup, CreatedMxTest: st.createdMxTest}
	if st.snapshot.path != "" {
		if err := os.WriteFile(restoreMprPath(projectPath), st.snapshot.contents, 0o600); err != nil {
			return err
		}
		rec.TreeDigest = st.snapshot.treeDigest
	}
	body, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp := restoreRecordPath(projectPath) + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, restoreRecordPath(projectPath))
}

// readRestoreRecord returns the record and the project state it describes, or
// nil when there is none.
func readRestoreRecord(projectPath string) (*restoreRecord, projectState, error) {
	body, err := os.ReadFile(restoreRecordPath(projectPath))
	if os.IsNotExist(err) {
		return nil, projectState{}, nil
	}
	if err != nil {
		return nil, projectState{}, err
	}
	var rec restoreRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil, projectState{}, fmt.Errorf("reading %s: %w", restoreRecordPath(projectPath), err)
	}
	st := projectState{afterStartup: rec.AfterStartup, createdMxTest: rec.CreatedMxTest}
	if rec.TreeDigest != "" {
		// A missing copy only costs the byte-exact restore; the removal itself
		// does not depend on it.
		if data, err := os.ReadFile(restoreMprPath(projectPath)); err == nil {
			st.snapshot = projectSnapshot{path: projectPath, contents: data, treeDigest: rec.TreeDigest}
		}
	}
	return &rec, st, nil
}

func removeRestoreRecord(projectPath string) {
	os.Remove(restoreRecordPath(projectPath))
	os.Remove(restoreMprPath(projectPath))
}

// Seams for the tests: the removal shells out to mxcli, which a unit test
// cannot do (findMxcli resolves to the test binary).
var (
	pidAlive        = docker.PidAlive
	removeInstalled = func(projectPath string, st projectState, w io.Writer) error {
		if err := cleanupEndpoint(projectPath, st, &TestSuite{}, w); err != nil {
			return err
		}
		removeGeneratedJavaSource(projectPath, w)
		restoreProjectFile(st, nil, w)
		return nil
	}
)

// RestoreAbandonedEndpoint removes a test endpoint whose dev loop ended without
// removing it, and reports whether there was one. A record whose owner is still
// running is left alone: that endpoint is in use, and its owner removes it.
//
// On failure the record stays, so the next `run stop` or --test-endpoint run
// tries again, and the error says what is left in the project.
func RestoreAbandonedEndpoint(projectPath string, w io.Writer) (bool, error) {
	rec, st, err := readRestoreRecord(projectPath)
	if err != nil || rec == nil {
		return false, err
	}
	if rec.PID > 0 && pidAlive(rec.PID) {
		return false, nil
	}
	fmt.Fprintf(w, "Removing the test endpoint left in the project by run pid %d (its run ended without removing it)...\n", rec.PID)
	RemoveHandshake(projectPath)
	if err := removeInstalled(projectPath, st, w); err != nil {
		return true, fmt.Errorf("could not remove the test endpoint — check the after-startup microflow and the %s module before committing: %w", mxTestModule, err)
	}
	removeRestoreRecord(projectPath)
	fmt.Fprintln(w, "  test endpoint removed; project restored")
	return true, nil
}

// afterStartupOf reads the project's after-startup microflow. A seam: the
// warning below is tested without a project file.
var afterStartupOf = func(projectPath string) (string, error) {
	reader, err := modelsdk.Open(projectPath)
	if err != nil {
		return "", err
	}
	defer reader.Disconnect()
	ps, err := reader.GetProjectSettings()
	if err != nil || ps == nil || ps.Model == nil {
		return "", err
	}
	return ps.Model.AfterStartupMicroflow, nil
}

// LeftoverEndpointWarning says so when the project's after-startup microflow is
// still the test endpoint's and no run of this project is hosting it, and ""
// otherwise.
//
// --test-endpoint swaps the setting for the lifetime of its run, and puts it
// back when the run ends — but a run killed with the container (a cloud session
// being recycled) never gets to, and nothing else looks at the setting until a
// `run stop`. Meanwhile the project file carries the swap, and a commit takes it
// along. So `run` and `check -p` look, and say how to undo it: with a restore
// record, `run stop` restores exactly what was there; without one (the .mxcli/
// folder went too), only the user knows what the setting was.
func LeftoverEndpointWarning(projectPath string) string {
	rec, _, _ := readRestoreRecord(projectPath)
	if rec != nil && rec.PID > 0 && pidAlive(rec.PID) {
		return "" // a live run hosts it, and removes it when it ends
	}
	af, err := afterStartupOf(projectPath)
	if err != nil || af != endpointStartupFlow {
		return ""
	}
	if rec != nil {
		return fmt.Sprintf("the project's after-startup microflow is %s, the mxcli test endpoint's, left by a run (pid %d) "+
			"that ended without removing it. Do not commit it:\n"+
			"  mxcli run stop -p %s    # removes the endpoint and restores the after-startup microflow",
			endpointStartupFlow, rec.PID, projectPath)
	}
	return fmt.Sprintf("the project's after-startup microflow is %s, the mxcli test endpoint's, and no run of this "+
		"project is hosting it; nothing records what it replaced. Do not commit it: set your own back and drop the module:\n"+
		"  mxcli -p %s -c \"alter settings runtime (AfterStartupMicroflow: '<yours, or empty>')\"\n"+
		"  mxcli -p %s -c \"drop module %s\"",
		endpointStartupFlow, projectPath, projectPath, mxTestModule)
}
