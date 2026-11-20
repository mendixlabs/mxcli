// SPDX-License-Identifier: Apache-2.0

//go:build integration

package docker

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCheckFindings_RealMx runs CheckFindings against a real mx on a scaffolded
// project: the report must read, its total must match the findings, and asking
// for warnings must add findings, not change how errors read. It skips when no
// mx is resolvable (the CI integration job installs one).
func TestCheckFindings_RealMx(t *testing.T) {
	mxPath, err := ResolveMx("")
	if err != nil {
		t.Skipf("mx not resolvable: %v", err)
	}
	dir := t.TempDir()
	scaffold := exec.Command(mxPath, "create-project")
	scaffold.Dir = dir
	if out, err := scaffold.CombinedOutput(); err != nil {
		t.Skipf("mx create-project failed: %v\n%s", err, out)
	}
	opts := CheckOptions{ProjectPath: filepath.Join(dir, "App.mpr")}

	errorsOnly, _ := CheckFindings(opts, CheckKinds{})
	if err := errorsOnly.Verify(); err != nil {
		t.Fatalf("errors-only report: %v", err)
	}
	if n := errorsOnly.Count("warning") + errorsOnly.Count("deprecation"); n != 0 {
		t.Errorf("%d warnings/deprecations without -w -d", n)
	}

	all, _ := CheckFindings(opts, CheckKinds{Warnings: true, Deprecations: true})
	if err := all.Verify(); err != nil {
		t.Fatalf("report with warnings: %v", err)
	}
	if all.Count("error") != errorsOnly.Count("error") {
		t.Errorf("asking for warnings changed the error count: %d vs %d", all.Count("error"), errorsOnly.Count("error"))
	}
	if len(all.Findings) <= len(errorsOnly.Findings) {
		t.Errorf("-w -d added no findings (%d vs %d); a fresh project has some", len(all.Findings), len(errorsOnly.Findings))
	}
}
