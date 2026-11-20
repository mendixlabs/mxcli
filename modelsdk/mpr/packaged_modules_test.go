// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A package whose model cannot be read leaves its module out of every
// listing; it must say so, or the module looks unseen again (ako/mxcli#1780).
func TestPackagedModules_UnreadablePackageIsReported(t *testing.T) {
	src := filepath.Join("testdata", "v1-project", "App.mpr")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	mprPath := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mprPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "modules", "Broken.mxmodule"), []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	var notice bytes.Buffer
	PackagedModuleNotice = &notice
	t.Cleanup(func() { PackagedModuleNotice = nil })

	r, err := Open(mprPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	stored, err := r.listStoredUnitsByType("Projects$ModuleImpl")
	if err != nil {
		t.Fatal(err)
	}
	all, err := r.ListModules()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(stored)+1 { // + System
		t.Errorf("ListModules = %d modules, want the %d stored ones and System", len(all), len(stored))
	}
	pms := r.PackagedModules()
	if len(pms) != 1 || pms[0].Name != "Broken" || pms[0].Err == nil {
		t.Fatalf("PackagedModules = %+v, want Broken with its error", pms)
	}
	if !strings.Contains(notice.String(), "modules/Broken.mxmodule") {
		t.Errorf("notice = %q, want a line naming the package", notice.String())
	}
}
