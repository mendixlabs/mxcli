// SPDX-License-Identifier: Apache-2.0

package main

// openProjectReadOnly replaced sdk/mpr.Open in this package (Phase 4a of
// docs/plans/2026-09-14-retire-legacy-engine.md). Its callers are read paths
// whose output does not distinguish a working reader from an empty one:
// `check --post-migration` prints "No legacy native widgets found" both when a
// project genuinely has none AND when the reader hands it zero pages to scan.
// A baseline diff agrees in both cases, so it cannot settle this on its own.
//
// These assert the reader returns real data, which is the part the commands
// silently depend on.

import (
	"os"
	"path/filepath"
	"testing"
)

func fixtureProject(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../testdata/expr-checker")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return filepath.Join(dst, "minimal.mpr")
}

func TestOpenProjectReadOnly_ReadsWhatTheCommandsNeed(t *testing.T) {
	b, err := openProjectReadOnly(fixtureProject(t))
	if err != nil {
		t.Fatalf("openProjectReadOnly: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	// setup / new read this to pick the mxbuild version. An empty ProductVersion
	// would send `setup mxbuild` to the wrong CDN URL, or to none at all.
	if pv := b.ProjectVersion(); pv.ProductVersion == "" {
		t.Error("ProjectVersion().ProductVersion is empty — setup would detect no version")
	}
	if v, err := b.GetMendixVersion(); err != nil || v == "" {
		t.Errorf("GetMendixVersion() = %q, %v — want a version", v, err)
	}

	// check --post-migration walks these two. Zero of both is exactly what a
	// broken reader looks like, and it prints the same clean bill of health.
	pages, err := b.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(pages) == 0 {
		t.Error("ListPages returned none — the legacy-widget scan would report a " +
			"clean project without having looked at anything")
	}
	if _, err := b.ListSnippets(); err != nil {
		t.Errorf("ListSnippets: %v", err)
	}

	// sync-java-deps reads this one.
	if _, err := b.ListModuleSettings(); err != nil {
		t.Errorf("ListModuleSettings: %v", err)
	}
}

func TestOpenProjectReadOnly_RefusesWhatIsNotAProject(t *testing.T) {
	if b, err := openProjectReadOnly(filepath.Join(t.TempDir(), "nope.mpr")); err == nil {
		_ = b.Disconnect()
		t.Error("a nonexistent project opened without error")
	}
}
