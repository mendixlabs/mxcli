// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ako/mxcli#2033: `run --local --watch --test-endpoint` rebuilt right after the
// boot with nothing edited — build #2, ~30 s of 404s on dist/pages and a forced
// client reload. --test-endpoint adds the MxTest module; the boot build writes
// its theme scaffold, themesource/mxtest/ (settings.json, main.js, …), and
// those files are newer than the boot's baseline. Without --test-endpoint no
// module is new and the boot stays on build #1, the reporter's control.

func touch(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestWatchIgnoresThemeScaffoldTheBuildWrote(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	t0 := time.Now().Add(-time.Hour)
	touch(t, mpr, t0)
	existing := filepath.Join(dir, "themesource", "myfirstmodule", "web", "main.scss")
	touch(t, existing, t0)

	// The boot: baseline, then a build that scaffolds themesource/mxtest/.
	base := sourceMTime(mpr)
	before := themeModuleFolders(mpr)
	touch(t, filepath.Join(dir, "themesource", "mxtest", "settings.json"), t0.Add(time.Minute))
	touch(t, filepath.Join(dir, "themesource", "mxtest", "native", "main.js"), t0.Add(time.Minute))
	builtAt := t0.Add(2 * time.Minute)
	scaffold := buildScaffold{}
	scaffold.record(mpr, before, builtAt)

	// Control: the unfiltered signal sees the scaffold as a change — the defect.
	if !sourceMTime(mpr).After(base) {
		t.Fatal("control: the scaffold should read as a change to the unfiltered signal")
	}
	if got := sourceMTimeExcept(mpr, scaffold); got.After(base) {
		t.Errorf("the build's own scaffold reads as a change (%v after baseline %v)", got, base)
	}

	// An edit to an existing theme file made while the build ran still counts.
	touch(t, existing, t0.Add(time.Minute))
	if !sourceMTimeExcept(mpr, scaffold).After(base) {
		t.Error("an edit to an existing theme file during the build was ignored")
	}
	touch(t, existing, t0)

	// A later edit inside the new folder counts too.
	touch(t, filepath.Join(dir, "themesource", "mxtest", "settings.json"), builtAt.Add(time.Second))
	if !sourceMTimeExcept(mpr, scaffold).After(base) {
		t.Error("a later edit to the scaffolded folder was ignored")
	}
}

// A folder that existed before the build is never scaffold, whatever its mtime.
func TestBuildScaffoldRecordsOnlyNewFolders(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	touch(t, mpr, time.Now())
	touch(t, filepath.Join(dir, "themesource", "old", "settings.json"), time.Now())
	before := themeModuleFolders(mpr)
	touch(t, filepath.Join(dir, "themesource", "fresh", "settings.json"), time.Now())
	s := buildScaffold{}
	s.record(mpr, before, time.Now())
	if _, ok := s["old"]; ok || len(s) != 1 {
		t.Errorf("scaffold = %v, want only the folder the build created", s)
	}
}

// The readers of the run state — `run status`, `run wait` — compare the source
// against the build that covers it, and used the unfiltered signal: a hosted
// dev loop (`--test-endpoint`) reported "change pending" for as long as it ran,
// for the scaffold of its own MxTest module, and never built it, because the
// watcher (above) correctly does not count it. The scaffold the run recorded
// is in its state, so a reader filters exactly what the watcher filters.
func TestSourceMTimeOfRunStateIgnoresTheRecordedScaffold(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	t0 := time.Now().Add(-time.Hour)
	touch(t, mpr, t0)
	before := themeModuleFolders(mpr)
	touch(t, filepath.Join(dir, "themesource", "mxtest", "settings.json"), t0.Add(time.Minute))
	builtAt := t0.Add(2 * time.Minute)
	scaffold := buildScaffold{}
	scaffold.record(mpr, before, builtAt)

	rec := NewRunStateRecorder(mpr, os.Getpid(), false, true, nil, "")
	rec.Scaffold(scaffold)
	st := rec.State()

	if !SourceMTime(mpr).After(t0) {
		t.Fatal("control: the unfiltered signal should see the scaffold")
	}
	if got := SourceMTimeOf(mpr, &st); got.After(t0) {
		t.Errorf("a reader of the run state sees the run's own scaffold as a change (%v after %v)", got, t0)
	}
	if got := SourceMTimeOf(mpr, nil); !got.Equal(SourceMTime(mpr)) {
		t.Errorf("with no run state the signal is the unfiltered one, got %v", got)
	}

	// The record survives the state file, as `run status` reads it from disk.
	disk, err := ReadRunState(mpr)
	if err != nil || disk == nil {
		t.Fatalf("ReadRunState = %v, %v", disk, err)
	}
	if got := SourceMTimeOf(mpr, disk); got.After(t0) {
		t.Errorf("the scaffold did not survive the state file: %v", disk.Scaffold)
	}

	// A later edit to the scaffolded folder is still a change.
	touch(t, filepath.Join(dir, "themesource", "mxtest", "settings.json"), builtAt.Add(time.Second))
	if !SourceMTimeOf(mpr, disk).After(t0) {
		t.Error("a later edit to the scaffolded folder was ignored")
	}
}
