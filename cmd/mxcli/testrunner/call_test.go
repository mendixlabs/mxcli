// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCallCases_SingleObject(t *testing.T) {
	cases, err := ParseCallCases(strings.NewReader(`{"flow": "Mod.Flow"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].Flow != "Mod.Flow" {
		t.Fatalf("got %+v", cases)
	}
}

func TestParseCallCases_Array(t *testing.T) {
	cases, err := ParseCallCases(strings.NewReader(`[{"flow": "Mod.A"}, {"flow": "Mod.B", "name": "second"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[0].Flow != "Mod.A" || cases[1].Name != "second" {
		t.Fatalf("got %+v", cases)
	}
}

func TestParseCallCases_MissingFlowRejected(t *testing.T) {
	if _, err := ParseCallCases(strings.NewReader(`[{"flow": "Mod.A"}, {"name": "no flow here"}]`)); err == nil {
		t.Fatal("expected an error for a case with no \"flow\"")
	} else if !strings.Contains(err.Error(), "case 2") {
		t.Fatalf("error should name the case index, got: %v", err)
	}
}

func TestParseCallCases_InvalidJSON(t *testing.T) {
	if _, err := ParseCallCases(strings.NewReader(`not json`)); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

// touch sets both the content and the mtime of a file, since some filesystems
// have 1s mtime resolution and these tests compare times taken moments apart.
func touch(t *testing.T, path string, content []byte, when time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func TestCheckModelNotStale_NoDeploymentIsNotAnError(t *testing.T) {
	mpr := tempProject(t)
	if err := checkModelNotStale(mpr); err != nil {
		t.Fatalf("no deployment/model/model.mdp at all must not be treated as stale: %v", err)
	}
}

func TestCheckModelNotStale_FreshModelPasses(t *testing.T) {
	mpr := tempProject(t)
	dir := filepath.Dir(mpr)
	base := time.Now().Add(-time.Hour)
	touch(t, mpr, []byte("x"), base)
	touch(t, filepath.Join(dir, "deployment", "model", "model.mdp"), []byte("model"), base.Add(time.Minute))
	if err := checkModelNotStale(mpr); err != nil {
		t.Fatalf("the build is newer than every source file: %v", err)
	}
}

// TestCheckModelNotStale_EditedProjectFileRefused is the core safety property:
// a model edited after the running app's build must refuse a call rather than
// silently test the old model and report a stale green.
func TestCheckModelNotStale_EditedProjectFileRefused(t *testing.T) {
	mpr := tempProject(t)
	dir := filepath.Dir(mpr)
	base := time.Now().Add(-time.Hour)
	touch(t, filepath.Join(dir, "deployment", "model", "model.mdp"), []byte("model"), base)
	touch(t, mpr, []byte("edited after the build"), base.Add(time.Minute))

	err := checkModelNotStale(mpr)
	if err == nil {
		t.Fatal("expected a stale-model error")
	}
	if !strings.Contains(err.Error(), "App.mpr") {
		t.Fatalf("error should name the file that changed, got: %v", err)
	}
}

func TestCheckModelNotStale_EditedJavaSourceRefused(t *testing.T) {
	mpr := tempProject(t)
	dir := filepath.Dir(mpr)
	base := time.Now().Add(-time.Hour)
	touch(t, mpr, []byte("x"), base)
	touch(t, filepath.Join(dir, "deployment", "model", "model.mdp"), []byte("model"), base)
	touch(t, filepath.Join(dir, "javasource", "Mod", "MyJavaAction.java"), []byte("edited"), base.Add(time.Minute))

	if err := checkModelNotStale(mpr); err == nil {
		t.Fatal("a javasource edit after the build must also refuse")
	}
}
