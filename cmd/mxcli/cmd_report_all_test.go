// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#2443: `report all` writes every kind that can run into one folder,
// an overview linking them, and says why a kind that needs inputs was not
// generated.
func TestReportAllWritesEveryPageAndAnOverview(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the catalog of a real project")
	}
	work := t.TempDir()
	if err := copyTree(filepath.Join("..", "..", "testdata", "pedapp"), filepath.Join(work, "app")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(work, "reports")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "security-annotations.yaml"), []byte("summary: Reviewed by hand on Tuesday.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = reportAllCmd.Flags().Set("offline", "true")
	_ = reportCmd.Flags().Set("no-mx-check", "true")
	t.Cleanup(func() {
		_ = reportAllCmd.Flags().Set("offline", "false")
		_ = reportCmd.Flags().Set("no-mx-check", "false")
		if rk, ok := reportKinds["dependencies"]; ok {
			_ = rk.cmd.Flags().Set("offline", "false")
		}
	})
	if err := runReportAll(reportAllCmd, filepath.Join(work, "app", "PedApp.mpr"), out, map[string]bool{"map": true}, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.html", "quality.html", "quality.json", "architecture.html", "security.html", "security.json", "dependencies.html", "configuration.html", "configuration.json", "tests.html"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "map.html")); err == nil {
		t.Error("--skip map leaves the map out")
	}
	index, _ := os.ReadFile(filepath.Join(out, "index.html"))
	for _, want := range []string{`<a href="security.html">Security</a>`, "needs a journey log", "needs the deployed revision",
		`aria-current="page">Overview</a>`, "Not generated"} {
		if !strings.Contains(string(index), want) {
			t.Errorf("index lacks %q", want)
		}
	}
	sec, _ := os.ReadFile(filepath.Join(out, "security.html"))
	if !strings.Contains(string(sec), `aria-current="page">Security</a>`) || !strings.Contains(string(sec), "Reviewed by hand on Tuesday.") {
		t.Error("each page links the others, and takes <kind>-annotations.yaml from the folder")
	}
}
