// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#951 item 1: `docker check` must never modify the user's project.
//
// `mx update-widgets` rewrites the model (and turns an MPRv2 project into
// MPRv1), and `mx check` itself compiles the theme into theme-cache/ and writes
// deployment/sass/. Measured on an 11.13 project: an MPRv1 project's .mpr was
// rewritten permanently by a plain `docker check`, and theme-cache/ and
// deployment/ were touched on v1 and v2 alike, with or without
// --no-update-widgets. Check now runs both tools on a temporary copy.
//
// The stubs below do to the directory they are given what the real tools do,
// so these tests fail the moment a tool is pointed at the project itself.
package docker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// treeState records every file's content hash and mtime, and every directory,
// under root.
func treeState(t *testing.T, root string) map[string]string {
	t.Helper()
	state := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			state[rel+"/"] = "dir"
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		state[rel] = hex.EncodeToString(sum[:]) + " " + info.ModTime().UTC().Format(time.RFC3339Nano)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return state
}

func diffStates(before, after map[string]string) []string {
	var out []string
	for k, v := range before {
		if a, ok := after[k]; !ok {
			out = append(out, "removed "+k)
		} else if a != v {
			out = append(out, "changed "+k)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			out = append(out, "added "+k)
		}
	}
	return out
}

// addProjectDirs gives a fixture the directories mx touches or reads.
func addProjectDirs(t *testing.T, dir string) {
	t.Helper()
	for _, f := range []string{
		"widgets/Some.Widget.mpk",
		"theme-cache/web/theme.compiled.css",
		"theme/web/main.scss",
		"javasource/app/Action.java",
	} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("original "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// stubTools replaces mx resolution and both mx invocations with stubs that
// mutate the project they are pointed at the way the real tools do, and record
// the paths they were given.
func stubTools(t *testing.T) (seen *[]string) {
	t.Helper()
	var paths []string
	origResolve, origUW, origCheck := resolveMxForCheck, updateWidgetsCmd, mxCheckCmd
	t.Cleanup(func() { resolveMxForCheck, updateWidgetsCmd, mxCheckCmd = origResolve, origUW, origCheck })

	resolveMxForCheck = func(string, string) (string, error) { return "mx", nil }
	updateWidgetsCmd = func(_, mprPath string, w, _ io.Writer) error {
		paths = append(paths, "update-widgets "+mprPath)
		dir := filepath.Dir(mprPath)
		// Rewrites the model (inlining units on v2) and drops mprcontents/.
		if err := os.WriteFile(mprPath, []byte("rewritten by update-widgets"), 0o644); err != nil {
			return err
		}
		return os.RemoveAll(filepath.Join(dir, "mprcontents"))
	}
	mxCheckCmd = func(_, mprPath string, _ []string, w, _ io.Writer) error {
		paths = append(paths, "check "+mprPath)
		dir := filepath.Dir(mprPath)
		// Compiles the theme and writes the sass entry point.
		if err := os.MkdirAll(filepath.Join(dir, "theme-cache/web"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "theme-cache/web/theme.compiled.css"), []byte("recompiled"), 0o644); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dir, "deployment/sass"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "deployment/sass/main.scss"), []byte("x"), 0o644); err != nil {
			return err
		}
		// mx reports the project it loaded by path in some messages.
		fmt.Fprintf(w, "Loading %s\nThe app contains: 0 errors.\n", mprPath)
		return nil
	}
	return &paths
}

func TestCheck_DoesNotModifyProject(t *testing.T) {
	for _, tc := range []struct {
		name          string
		fixture       func(*testing.T) string
		skipUpdate    bool
		wantUWOnACopy bool
	}{
		{"v1", v1Fixture, false, true},
		{"v2", v2Fixture, false, true},
		{"v1 --no-update-widgets", v1Fixture, true, false},
		{"v2 --no-update-widgets", v2Fixture, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mprPath := tc.fixture(t)
			projectDir := filepath.Dir(mprPath)
			addProjectDirs(t, projectDir)
			seen := stubTools(t)

			tmpRoot := t.TempDir()
			t.Setenv("TMPDIR", tmpRoot)

			before := treeState(t, projectDir)
			var out bytes.Buffer
			if err := Check(CheckOptions{
				ProjectPath:       mprPath,
				SkipUpdateWidgets: tc.skipUpdate,
				Stdout:            &out,
				Stderr:            io.Discard,
			}); err != nil {
				t.Fatalf("Check: %v\n%s", err, out.String())
			}
			after := treeState(t, projectDir)
			if d := diffStates(before, after); len(d) > 0 {
				t.Errorf("docker check modified the project:\n  %s", strings.Join(d, "\n  "))
			}

			// Every tool ran, and none of them on the project itself.
			if len(*seen) == 0 {
				t.Fatal("no mx tool was invoked")
			}
			for _, s := range *seen {
				if strings.HasPrefix(strings.Fields(s)[1], projectDir+string(filepath.Separator)) {
					t.Errorf("mx was pointed at the project itself: %s", s)
				}
			}
			ranUW := strings.HasPrefix((*seen)[0], "update-widgets ")
			if ranUW != tc.wantUWOnACopy {
				t.Errorf("update-widgets ran = %v, want %v (%v)", ranUW, tc.wantUWOnACopy, *seen)
			}

			// The output names the project, not the copy, and says what happened.
			got := out.String()
			if strings.Contains(got, tmpRoot) {
				t.Errorf("output leaks the temporary copy's path:\n%s", got)
			}
			if !strings.Contains(got, "Loading "+mprPath) {
				t.Errorf("mx output not reported against the original path:\n%s", got)
			}
			if tc.wantUWOnACopy && !strings.Contains(got, "normalised on a temporary copy") {
				t.Errorf("output does not say widgets were normalised on a copy:\n%s", got)
			}

			// The copy is cleaned up.
			if entries, _ := os.ReadDir(tmpRoot); len(entries) != 0 {
				t.Errorf("temporary copy left behind in %s: %v", tmpRoot, entries)
			}
		})
	}
}

// TestCopyProjectForCheck_SkipsOutputs pins that build output, VCS metadata and
// caches are not copied: a large project's deployment/ or .git can dwarf the
// model, and mx check needs neither.
func TestCopyProjectForCheck_SkipsOutputs(t *testing.T) {
	src := t.TempDir()
	for _, f := range []string{
		"App.mpr", "mprcontents/ab/cd/x.mxunit", "widgets/W.mpk",
		"themesource/m/web/main.scss", "theme/web/main.scss", "javasource/a/B.java",
		"deployment/run/x.jar", ".git/HEAD", "releases/App.mda", "theme-cache/web/t.css",
		".mxcli/catalog.db", "theme/node_modules/pkg/index.js", ".mendix-cache/x",
	} {
		p := filepath.Join(src, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(f), 0o644)
	}
	dst := t.TempDir()
	if err := copyProjectTree(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"App.mpr", "mprcontents/ab/cd/x.mxunit", "widgets/W.mpk", "themesource/m/web/main.scss", "theme/web/main.scss", "javasource/a/B.java"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err != nil {
			t.Errorf("%s not copied: %v", f, err)
		}
	}
	for _, f := range []string{"deployment", ".git", "releases", "theme-cache", ".mxcli", "theme/node_modules", ".mendix-cache"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err == nil {
			t.Errorf("%s was copied; it is output, VCS metadata or a cache", f)
		}
	}
}

// TestCopyProjectForCheck_TempDirInsideProject: with $TMPDIR inside the project
// the walk meets its own copy; it must not copy the copy into itself.
func TestCopyProjectForCheck_TempDirInsideProject(t *testing.T) {
	proj := t.TempDir()
	mpr := filepath.Join(proj, "App.mpr")
	if err := os.WriteFile(mpr, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(proj, "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	work, cleanup, err := copyProjectToTemp(mpr)
	if err != nil {
		t.Fatalf("copyProjectToTemp: %v", err)
	}
	defer cleanup()
	if b, err := os.ReadFile(work); err != nil || string(b) != "model" {
		t.Fatalf("copy of the model = %q, %v", b, err)
	}
	filepath.WalkDir(filepath.Dir(work), func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(d.Name(), "mxcli-copy-") {
			t.Errorf("the copy contains a copy of itself: %s", p)
			return filepath.SkipDir
		}
		return nil
	})
}

func TestCopyProjectForCheck_MissingProject(t *testing.T) {
	if _, _, err := copyProjectToTemp(filepath.Join(t.TempDir(), "nope.mpr")); err == nil {
		t.Error("want an error for a project file that does not exist")
	}
}

// TestWithProjectCopy_WritesStayOnTheCopy: fn gets a copy it may write into, the
// original is untouched, and the copy is gone once fn returns.
func TestWithProjectCopy_WritesStayOnTheCopy(t *testing.T) {
	proj := t.TempDir()
	mpr := filepath.Join(proj, "App.mpr")
	if err := os.WriteFile(mpr, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	var seen string
	err := WithProjectCopy(mpr, func(work string) error {
		seen = work
		if work == mpr {
			t.Error("fn was handed the original project")
		}
		return os.WriteFile(filepath.Join(filepath.Dir(work), "theme-cache.txt"), []byte("mx wrote this"), 0o644)
	})
	if err != nil {
		t.Fatalf("WithProjectCopy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "theme-cache.txt")); err == nil {
		t.Error("a write into the copy reached the original project")
	}
	if _, err := os.Stat(seen); err == nil {
		t.Errorf("the copy %s was not removed", seen)
	}
	if err := WithProjectCopy(filepath.Join(proj, "nope.mpr"), func(string) error { return nil }); err == nil {
		t.Error("want an error for a project file that does not exist")
	}
}

// TestWithProjectCopy_SkipsNamedFoldersButNeverTheProject: a folder inside the
// project that the caller names is left out of the copy; naming the project's own
// folder (an output directory of ".") skips nothing.
func TestWithProjectCopy_SkipsNamedFoldersButNeverTheProject(t *testing.T) {
	proj := t.TempDir()
	mpr := filepath.Join(proj, "App.mpr")
	for p, content := range map[string]string{
		"App.mpr":             "model",
		"dist/Old.mpk":        "earlier package",
		"widgets/w.mpk":       "a widget",
		"userlib/dist/ok.jar": "a deeper folder of the same name",
	} {
		full := filepath.Join(proj, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	exists := func(work, rel string) bool {
		_, err := os.Stat(filepath.Join(filepath.Dir(work), filepath.FromSlash(rel)))
		return err == nil
	}

	err := WithProjectCopy(mpr, func(work string) error {
		if exists(work, "dist/Old.mpk") {
			t.Error("the skipped folder was copied")
		}
		if !exists(work, "widgets/w.mpk") || !exists(work, "userlib/dist/ok.jar") || !exists(work, "App.mpr") {
			t.Error("something outside the skipped folder was not copied")
		}
		return nil
	}, filepath.Join(proj, "dist"), filepath.Join(t.TempDir(), "elsewhere"))
	if err != nil {
		t.Fatal(err)
	}

	err = WithProjectCopy(mpr, func(work string) error {
		if !exists(work, "App.mpr") || !exists(work, "dist/Old.mpk") {
			t.Error("naming the project folder itself must skip nothing")
		}
		return nil
	}, proj)
	if err != nil {
		t.Fatal(err)
	}
}
