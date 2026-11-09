// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#961 item 1: `docker build` must not modify the user's project; it
// writes only its output directory.
//
// Measured on the 11.14 testapp before the fix: an MPRv1 build rewrote the .mpr
// (update-widgets ran on the project, and only an MPRv2 project's storage was
// restored afterwards), every MPRv2 .mxunit was rewritten and put back with a
// new mtime, and mx check / MxBuild wrote theme-cache/, deployment/, 160
// javasource/ proxies, TestApp.launch, .classpath and .project into it. All three
// tools now run on one temporary copy.
package docker

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubMxBuild replaces MxBuild with a stub that writes into the project it is
// given what the real one does, records the model it saw, and writes a PAD
// marker to the output directory.
func stubMxBuild(t *testing.T) (sawModel *string, sawPath *string) {
	t.Helper()
	var model, path string
	orig := mxbuildCmd
	t.Cleanup(func() { mxbuildCmd = orig })
	mxbuildCmd = func(_, _, outputDir, mprPath string, w, _ io.Writer) error {
		path = mprPath
		b, err := os.ReadFile(mprPath)
		if err != nil {
			return err
		}
		model = string(b)
		dir := filepath.Dir(mprPath)
		for f, content := range map[string]string{
			"deployment/model/model.mdp":                                   "built",
			"javasource/app/proxies/Entity.java":                           "generated proxy",
			strings.TrimSuffix(filepath.Base(mprPath), ".mpr") + ".launch": "rewritten",
			".classpath":                         "eclipse",
			"theme-cache/web/theme.compiled.css": "recompiled by mxbuild",
		} {
			p := filepath.Join(dir, f)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return err
		}
		io.WriteString(w, "Building "+mprPath+"\n")
		return os.WriteFile(filepath.Join(outputDir, "Dockerfile"), []byte("FROM x"), 0o644)
	}
	return &model, &path
}

func TestBuildOnCopy_DoesNotModifyProject(t *testing.T) {
	for _, tc := range []struct {
		name        string
		fixture     func(*testing.T) string
		skipCheck   bool
		skipUpdate  bool
		dryRun      bool
		updateFirst bool
	}{
		{"v1", v1Fixture, false, false, false, false},
		{"v2", v2Fixture, false, false, false, false},
		{"v1 --no-update-widgets", v1Fixture, false, true, false, false},
		{"v2 --no-update-widgets", v2Fixture, false, true, false, false},
		{"v1 --update-widgets", v1Fixture, false, false, false, true},
		{"v2 --update-widgets", v2Fixture, false, false, false, true},
		{"v1 --skip-check", v1Fixture, true, false, false, false},
		{"v2 --skip-check", v2Fixture, true, false, false, false},
		{"v1 --dry-run", v1Fixture, false, false, true, false},
		{"v2 --dry-run", v2Fixture, false, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mprPath := tc.fixture(t)
			projectDir := filepath.Dir(mprPath)
			addProjectDirs(t, projectDir)
			if err := os.WriteFile(strings.TrimSuffix(mprPath, ".mpr")+".launch", []byte("original launch"), 0o644); err != nil {
				t.Fatal(err)
			}
			seen := stubTools(t)
			sawModel, sawPath := stubMxBuild(t)
			origModel, err := os.ReadFile(mprPath)
			if err != nil {
				t.Fatal(err)
			}

			tmpRoot := t.TempDir()
			t.Setenv("TMPDIR", tmpRoot)
			outputDir := filepath.Join(projectDir, ".docker", "build")

			before := treeState(t, projectDir)
			mxPath := "mx"
			if tc.skipCheck {
				mxPath = ""
			}
			var out bytes.Buffer
			if err := buildOnCopy(buildSteps{
				ProjectPath:        mprPath,
				MxPath:             mxPath,
				MxBuildPath:        "mxbuild",
				JavaHome:           "/jdk",
				OutputDir:          outputDir,
				SkipUpdateWidgets:  tc.skipUpdate,
				UpdateWidgetsFirst: tc.updateFirst,
				DryRun:             tc.dryRun,
			}, &out, io.Discard); err != nil {
				t.Fatalf("buildOnCopy: %v\n%s", err, out.String())
			}

			// Nothing but the output directory changed.
			after := treeState(t, projectDir)
			var changed []string
			for _, d := range diffStates(before, after) {
				if !strings.Contains(d, ".docker") {
					changed = append(changed, d)
				}
			}
			if len(changed) > 0 {
				t.Errorf("docker build modified the project:\n  %s", strings.Join(changed, "\n  "))
			}

			// No tool was pointed at the project itself.
			all := append([]string{}, *seen...)
			if *sawPath != "" {
				all = append(all, "mxbuild "+*sawPath)
			}
			for _, s := range all {
				if strings.HasPrefix(strings.Fields(s)[1], projectDir+string(filepath.Separator)) {
					t.Errorf("a tool was pointed at the project itself: %s", s)
				}
			}

			if tc.dryRun {
				if *sawPath != "" {
					t.Error("MxBuild ran on a dry run")
				}
			} else {
				if _, err := os.Stat(filepath.Join(outputDir, "Dockerfile")); err != nil {
					t.Errorf("the PAD was not written to the output directory: %v", err)
				}
				// MxBuild builds the project as stored; only --update-widgets
				// builds a widget-normalised model — on the copy, not the
				// project (ako/mxcli#1794).
				wantNormalised := !tc.skipCheck && tc.updateFirst
				if got := *sawModel == "rewritten by update-widgets"; got != wantNormalised {
					t.Errorf("MxBuild saw the normalised model = %v, want %v", got, wantNormalised)
				}
				if !wantNormalised && *sawModel != string(origModel) {
					t.Error("MxBuild did not see the project's model as stored")
				}
				if !strings.Contains(out.String(), "Building "+mprPath) {
					t.Errorf("MxBuild output not reported against the project's path:\n%s", out.String())
				}
			}

			if strings.Contains(out.String(), tmpRoot) {
				t.Errorf("output leaks the temporary copy's path:\n%s", out.String())
			}
			if !strings.Contains(out.String(), "temporary copy") {
				t.Errorf("output does not say the build runs on a copy:\n%s", out.String())
			}
			if entries, _ := os.ReadDir(tmpRoot); len(entries) != 0 {
				t.Errorf("temporary copy left behind in %s: %v", tmpRoot, entries)
			}
		})
	}
}

// TestBuildOnCopy_CheckFailureStopsBeforeMxBuild: a failing check still aborts
// the build, as it did when the check ran on the project.
func TestBuildOnCopy_CheckFailureStopsBeforeMxBuild(t *testing.T) {
	mprPath := v1Fixture(t)
	stubTools(t)
	mxCheckCmd = func(string, string, []string, io.Writer, io.Writer) error { return io.ErrUnexpectedEOF }
	_, sawPath := stubMxBuild(t)
	t.Setenv("TMPDIR", t.TempDir())
	err := buildOnCopy(buildSteps{ProjectPath: mprPath, MxPath: "mx", OutputDir: filepath.Join(t.TempDir(), "out")}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "project has errors") {
		t.Fatalf("err = %v, want the check failure", err)
	}
	if *sawPath != "" {
		t.Error("MxBuild ran after a failed check")
	}
}
