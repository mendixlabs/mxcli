// SPDX-License-Identifier: Apache-2.0

//go:build integration

package docker

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#2288, against a real `mx update-widgets`: the scoped harvest still
// clears every CE0463 (the widget documents are what the resync is for), and
// writes nothing but Forms$ documents.
func TestFixWidgets_ScopedHarvestClearsCE0463_Issue2288(t *testing.T) {
	mxPath, err := ResolveMx("")
	if err != nil {
		t.Skipf("mx not resolvable: %v", err)
	}
	dir, err := os.MkdirTemp("", "fws")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	scaffold := exec.Command(mxPath, "create-project")
	scaffold.Dir = dir
	PrepareMxCommand(scaffold)
	if out, err := scaffold.CombinedOutput(); err != nil {
		t.Skipf("mx create-project failed: %v\n%s", err, out)
	}
	mprPath := filepath.Join(dir, "App.mpr")
	if storageVersion(t, mprPath) != types.MPRVersionV2 {
		t.Skip("the scaffolded project is not MPR v2; the harvest is not used")
	}
	mpk := filepath.Join(dir, "widgets", "com.mendix.widget.web.Datagrid.mpk")
	if _, err := os.Stat(mpk); err != nil {
		t.Skipf("no Data grid 2 package to make stale: %v", err)
	}
	addWidgetProperty(t, mpk, "Datagrid.xml")

	check := func() string {
		var out bytes.Buffer
		_ = Check(CheckOptions{ProjectPath: mprPath, Stdout: &out, Stderr: &out, SkipUpdateWidgets: true})
		return out.String()
	}
	before := len(ce0463Lines(check()))
	if before == 0 {
		t.Skip("no CE0463 after making Data grid 2 stale")
	}

	res, err := RunToolPreservingFormat(mxPath, mprPath, "update-widgets", io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("RunToolPreservingFormat: %v", err)
	}
	if after := ce0463Lines(check()); len(after) != 0 {
		t.Errorf("%d of %d CE0463 remain after the scoped harvest:\n%s", len(after), before, strings.Join(after, "\n"))
	}
	if res.UnitsWritten == 0 {
		t.Error("nothing was written, yet the project had stale widgets")
	}
	for _, u := range res.LeftAsStored {
		if strings.HasPrefix(u, "Forms$") {
			t.Errorf("a widget document was left as stored: %s", u)
		}
	}
	t.Logf("CE0463 %d -> 0; %d unit(s) written; left as stored: %v", before, res.UnitsWritten, res.LeftAsStored)
}
