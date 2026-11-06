// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/internal/testutil"
)

func TestNewExplicitToolsReachCreateProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test executables use POSIX scripts")
	}
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("PATH", t.TempDir())
	modeler := filepath.Join(home, "Selected Beta", "modeler")
	if err := os.MkdirAll(modeler, 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"mx.deps.json": `{"targets":{"mx/99.98.97":{}}}`,
		"mx":           "#!/bin/sh\n[ \"$1\" = create-project ] || exit 90\necho selected-native-create\nexit 73\n",
		"mxbuild":      "#!/bin/sh\necho unexpected-build\nexit 74\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(modeler, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	result := runMainStreams(t, home, "new", "NativeApp", "--version", "99.98.97",
		"--mxbuild-path", filepath.Join("Selected Beta", "modeler", "mxbuild"), "--skip-init", "--skip-build", "--theme", "none", "--layout", "none")
	if result.exitCode == 0 || !strings.Contains(result.stdout, "selected-native-create") ||
		!strings.Contains(result.stderr, "Error creating project") {
		t.Fatalf("selected mx must reach create-project without CDN version resolution: exit %d\n%s\n%s", result.exitCode, result.stdout, result.stderr)
	}
	if strings.Contains(result.stdout+result.stderr, "unexpected-build") {
		t.Fatal("--skip-build invoked mxbuild")
	}
}

func TestNewDefaultToolsStillReachCachedCreateProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test executables use POSIX scripts")
	}
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("PATH", t.TempDir())
	modeler := filepath.Join(home, ".mxcli", "mxbuild", "99.98.97.1", "modeler")
	if err := os.MkdirAll(modeler, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modeler, "mx"), []byte("#!/bin/sh\necho default-cached-create\nexit 73\n"), 0755); err != nil {
		t.Fatal(err)
	}
	// A full version needs no CDN expansion. With no override, the existing
	// exact-version cache must still be used.
	result := runMainStreams(t, home, "new", "NativeApp", "--version", "99.98.97.1", "--theme", "none")
	if result.exitCode == 0 || !strings.Contains(result.stdout, "default-cached-create") {
		t.Fatalf("default cache resolution changed: exit %d\n%s\n%s", result.exitCode, result.stdout, result.stderr)
	}
}

func TestNewEmptyExplicitToolsFailBeforeFallback(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	result := runMainStreams(t, home, "new", "NativeApp", "--version", "99.98.97", "--mxbuild-path=", "--theme", "none")
	if result.exitCode == 0 || !strings.Contains(result.stderr, "--mxbuild-path must name an installed toolchain") {
		t.Fatalf("empty selection must not use default resolution: exit %d\n%s\n%s", result.exitCode, result.stdout, result.stderr)
	}
}

func TestNewExplicitToolsReachFirstBuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test executables use POSIX scripts")
	}
	fixture, err := filepath.Abs(filepath.Join("..", "..", "modelsdk", "mpr", "testdata", "v1-project", "App.mpr"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := openProjectReadOnly(fixture)
	if err != nil {
		t.Fatal(err)
	}
	version := reader.ProjectVersion().ProductVersion
	_ = reader.Disconnect()
	release := strings.Join(strings.Split(version, ".")[:3], ".")
	javaMajor, _ := docker.ProjectJavaMajor(fixture)
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("MXCLI_TEST_NEW_FIXTURE", fixture)
	record := filepath.Join(home, "first-build")
	t.Setenv("MXCLI_TEST_FIRST_BUILD_RECORD", record)
	stageRecord := filepath.Join(home, "created-stage")
	t.Setenv("MXCLI_TEST_NEW_STAGE_RECORD", stageRecord)
	creatorRecord := filepath.Join(home, "creator")
	t.Setenv("MXCLI_TEST_NEW_CREATOR_RECORD", creatorRecord)
	modeler := filepath.Join(home, "Selected Beta", "modeler")
	inner := filepath.Join(home, "Selected Beta", "inner")
	aliases := filepath.Join(home, "aliases")
	otherModeler := filepath.Join(aliases, "modeler")
	jdk := filepath.Join(home, "jdk")
	for _, dir := range []string{modeler, inner, otherModeler, filepath.Join(jdk, "bin")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("JAVA_HOME", jdk)
	files := map[string]string{
		filepath.Join(modeler, "mx.deps.json"): `{"targets":{"mx/` + release + `":{}}}`,
		filepath.Join(modeler, "mx"):           "#!/bin/sh\n[ \"$1\" = create-project ] || exit 90\nprintf '%s\\n' \"$0\" > \"$MXCLI_TEST_NEW_CREATOR_RECORD\"\nprintf '%s\\n' \"$PWD\" > \"$MXCLI_TEST_NEW_STAGE_RECORD\"\n/bin/cp \"$MXCLI_TEST_NEW_FIXTURE\" App.mpr\n",
		filepath.Join(modeler, "mxbuild"):      "#!/bin/sh\nprintf '%s\\n' \"$0\" \"$@\" > \"$MXCLI_TEST_FIRST_BUILD_RECORD\"\n",
		filepath.Join(jdk, "bin", "java"):      fmt.Sprintf("#!/bin/sh\necho 'openjdk version \"%d\"'\n", javaMajor),
	}
	// Both installations report the same release; their executable paths prove
	// which pair actually ran when a raw link/.. path is supplied.
	for _, name := range []string{"mx.deps.json", "mx", "mxbuild"} {
		files[filepath.Join(otherModeler, name)] = files[filepath.Join(modeler, name)]
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	selectedBuild, err := filepath.EvalSymlinks(filepath.Join(modeler, "mxbuild"))
	if err != nil {
		t.Fatal(err)
	}
	selectedCreator, err := filepath.EvalSymlinks(filepath.Join(modeler, "mx"))
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(aliases, "link")
	if err := os.Symlink(inner, link); err != nil {
		t.Fatal(err)
	}
	// filepath.Join would erase the link/.. sequence this regression exercises.
	rawRoot := link + string(filepath.Separator) + ".."
	for _, tc := range []struct{ name, requested, wantError, destination, toolPath string }{
		{"exact version", version, "", "missing", ""},
		{"release version", release, "", "missing", ""},
		{"wrong full build", release + ".999999999", "but the created project is", "missing", ""},
		{"wrong full build empty destination", release + ".999999999", "but the created project is", "empty", ""},
		{"wrong full build adopted brain", release + ".999999999", "but the created project is", "brain", ""},
		{"raw executable override", version, "", "missing", rawRoot + "/modeler/mxbuild"},
		{"raw modeler override", version, "", "missing", rawRoot + "/modeler"},
		{"raw installation override", version, "", "missing", rawRoot},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(record)
			_ = os.Remove(creatorRecord)
			toolPath := tc.toolPath
			if toolPath == "" {
				toolPath = filepath.Join(modeler, "mxbuild")
			}
			dest := filepath.Join(home, tc.name)
			before := map[string]string{}
			if tc.destination == "empty" {
				if err := os.MkdirAll(dest, 0755); err != nil {
					t.Fatal(err)
				}
			} else if tc.destination == "brain" {
				seedBrain(t, dest)
				before = snapshot(t, dest)
			}
			result := runMainStreams(t, home, "new", "NativeApp", "--version", tc.requested,
				"--mxbuild-path", toolPath, "--output-dir", dest,
				"--skip-init", "--theme", "none", "--layout", "none")
			got, err := os.ReadFile(record)
			if tc.wantError != "" {
				if result.exitCode == 0 || !strings.Contains(result.stderr, tc.wantError) || !os.IsNotExist(err) {
					t.Fatalf("wrong created version must fail before build: exit=%d record=%q err=%v\n%s\n%s", result.exitCode, got, err, result.stdout, result.stderr)
				}
				if tc.destination == "missing" {
					if _, err := os.Stat(dest); !os.IsNotExist(err) {
						t.Errorf("version rejection created the destination: %v", err)
					}
				} else if after := snapshot(t, dest); !maps.Equal(before, after) {
					t.Errorf("version rejection changed the destination: %d files before, %d after", len(before), len(after))
				}
				stage, err := os.ReadFile(stageRecord)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(strings.TrimSpace(string(stage))); !os.IsNotExist(err) {
					t.Errorf("version rejection left its staging directory: %v", err)
				}
				result = runMainStreams(t, home, "new", "NativeApp", "--version", version,
					"--mxbuild-path", filepath.Join(modeler, "mxbuild"), "--output-dir", dest,
					"--skip-init", "--theme", "none", "--layout", "none")
				if result.exitCode != 0 {
					t.Fatalf("corrected retry failed: exit=%d\n%s\n%s", result.exitCode, result.stdout, result.stderr)
				}
				for path, content := range before {
					got, err := os.ReadFile(filepath.Join(dest, path))
					if err != nil || string(got) != content {
						t.Errorf("corrected retry changed brain file %s: %v", path, err)
					}
				}
				return
			}
			creator, creatorErr := os.ReadFile(creatorRecord)
			if result.exitCode != 0 || err != nil || !strings.HasPrefix(string(got), selectedBuild+"\n--target=deploy\n") ||
				creatorErr != nil || strings.TrimSpace(string(creator)) != selectedCreator {
				t.Fatalf("selected pair was not used: exit=%d creator=%q err=%v build=%q err=%v\n%s\n%s", result.exitCode, creator, creatorErr, got, err, result.stdout, result.stderr)
			}
		})
	}
}

func TestNewInvalidExplicitToolsFailBeforeFallback(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	missing := filepath.Join(home, "missing", "mxbuild")
	result := runMainStreams(t, home, "new", "NativeApp", "--version", "99.98.97", "--mxbuild-path", missing, "--theme", "none")
	if result.exitCode == 0 || !strings.Contains(result.stderr, missing) || strings.Contains(result.stdout, "Resolved full version") {
		t.Fatalf("explicit path must fail before CDN/fallback: exit %d\n%s\n%s", result.exitCode, result.stdout, result.stderr)
	}
	if _, err := os.Stat(filepath.Join(home, "NativeApp")); !os.IsNotExist(err) {
		t.Fatalf("bad selection created a project directory: %v", err)
	}
}
