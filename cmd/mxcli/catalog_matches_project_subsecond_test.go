// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// catalogMatchesProject decides whether `describe` may answer from the cache
// instead of scanning the project. It compared whole seconds, so a project
// modified within the second of the build still matched.
func TestCatalogMatchesProject_WriteInTheSameSecondAsTheBuild(t *testing.T) {
	mpr := filepath.Join(t.TempDir(), "app.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(mpr)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	if err := cat.SetCacheInfo(catalog.AbsMprPath(mpr), fi.ModTime(), "11.8.0", "fast", time.Second); err != nil {
		t.Fatal(err)
	}
	if !catalogMatchesProject(cat, mpr) {
		t.Fatal("control: an unchanged project must match its cache")
	}

	moved := fi.ModTime().Add(300 * time.Millisecond)
	if fi.ModTime().Nanosecond() > 500_000_000 {
		moved = fi.ModTime().Add(-300 * time.Millisecond)
	}
	if err := os.Chtimes(mpr, moved, moved); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Stat(mpr); got.ModTime().Unix() != fi.ModTime().Unix() || got.ModTime().Equal(fi.ModTime()) {
		t.Fatalf("setup: %v -> %v is not a change inside one second", fi.ModTime(), got.ModTime())
	}
	if catalogMatchesProject(cat, mpr) {
		t.Error("a project modified inside the second of the build still matched its cache")
	}
}
