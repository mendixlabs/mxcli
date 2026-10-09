// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeComposeName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"MyApp", "myapp"},
		{"my app", "my-app"},
		{"My  Cool__App!!", "my-cool__app"},
		{"  --Leading and trailing--  ", "leading-and-trailing"},
		{"_underscore-first", "underscore-first"},
		{"app.v2", "app-v2"},
		{"Ünïcode-Ñame", "n-code-ame"},
		{"123", "123"},
		{"", ""},
		{"!!!", ""},
		{strings.Repeat("a", 80), strings.Repeat("a", maxComposeNameBase)},
		{strings.Repeat("a", maxComposeNameBase-1) + "-bbb", strings.Repeat("a", maxComposeNameBase-1)},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := sanitizeComposeName(c.in); got != c.want {
				t.Errorf("sanitizeComposeName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestComposeProjectNameFor(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name string
		mpr  string
		want string
	}{
		{"folder wins over mpr base", filepath.Join(root, "Customer Portal", "app.mpr"), "mxcli-customer-portal"},
		{"folder with punctuation", filepath.Join(root, "MyApp_v2.0", "MyApp.mpr"), "mxcli-myapp_v2-0"},
		{"mpr base when folder is unusable", filepath.Join(root, "!!!", "Billing.mpr"), "mxcli-billing"},
		{"fallback when nothing usable", filepath.Join(root, "!!!", "???.mpr"), "mxcli-app"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ComposeProjectNameFor(c.mpr); got != c.want {
				t.Errorf("ComposeProjectNameFor(%q) = %q, want %q", c.mpr, got, c.want)
			}
		})
	}
}

func TestNormalizeLikeCompose(t *testing.T) {
	// Mirrors docker compose's own derivation of a project name from the
	// working directory: lowercase, drop invalid characters, trim leading _/-.
	cases := []struct{ in, want string }{
		{".docker", "docker"},
		{"docker", "docker"},
		{"My Docker.Dir", "mydockerdir"},
		{"_x-y_", "x-y_"},
		{"...", ""},
	}
	for _, c := range cases {
		if got := normalizeLikeCompose(c.in); got != c.want {
			t.Errorf("normalizeLikeCompose(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExistingComposeName(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"none", "services:\n  mendix:\n    name: not-top-level\n", "", false},
		{"plain", "name: foo\nservices:\n", "foo", true},
		{"double quoted", "# c\nname: \"mxcli-x\"\nservices:\n", "\"mxcli-x\"", true},
		{"single quoted with comment", "name: 'a-b'   # why\nservices:\n", "'a-b'", true},
		{"interpolated", "name: ${PROJ:-mine}\n", "${PROJ:-mine}", true},
		{"commented out", "# name: nope\nservices:\n", "", false},
		{"indented is not top level", "  name: nope\n", "", false},
		{"crlf", "name: foo\r\nservices:\r\n", "foo", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := existingComposeName([]byte(c.in))
			if got != c.want || ok != c.wantOK {
				t.Errorf("existingComposeName() = (%q, %v), want (%q, %v)", got, ok, c.want, c.wantOK)
			}
		})
	}
}

// initInto runs `docker init` for a fresh app folder named folder under root
// and returns the generated compose file's content.
func initInto(t *testing.T, root, folder string) (compose string, out string) {
	t.Helper()
	appDir := filepath.Join(root, folder)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(appDir, "app.mpr")
	if err := os.WriteFile(mpr, nil, 0644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Init(InitOptions{ProjectPath: mpr, Stdout: &buf}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(appDir, ".docker", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data), buf.String()
}

func TestInit_EachProjectGetsItsOwnComposeName(t *testing.T) {
	// Issue #1364: compose derived the project name from the ".docker" folder,
	// so every mxcli project was the compose project "docker" and their
	// containers and volumes collided.
	root := t.TempDir()
	a, _ := initInto(t, root, "AppOne")
	b, _ := initInto(t, root, "AppTwo")

	nameA, okA := existingComposeName([]byte(a))
	nameB, okB := existingComposeName([]byte(b))
	if !okA || !okB {
		t.Fatalf("generated compose files must carry a top-level name: (%q, %v) (%q, %v)", nameA, okA, nameB, okB)
	}
	if nameA == nameB {
		t.Errorf("two projects share compose project name %s", nameA)
	}
	if nameA != `"mxcli-appone"` || nameB != `"mxcli-apptwo"` {
		t.Errorf("names = %s, %s; want \"mxcli-appone\", \"mxcli-apptwo\"", nameA, nameB)
	}
	for _, c := range []string{a, b} {
		if strings.Contains(c, composeNamePlaceholder) {
			t.Error("template placeholder left in generated compose file")
		}
		if got := composeVersionOf(c); got < 3 {
			t.Errorf("generated template version = %d, want >= 3 (the name: line is part of v3)", got)
		}
	}
}

func composeVersionOf(content string) int { return parseTemplateVersion([]byte(content)) }

func TestInit_ReportsComposeProjectName(t *testing.T) {
	_, out := initInto(t, t.TempDir(), "Shop")
	if !strings.Contains(out, "mxcli-shop") {
		t.Errorf("init output should name the compose project; got:\n%s", out)
	}
}

// legacyProject lays out a project initialised by an older mxcli: a compose
// file with no name, so Compose named the stack after the ".docker" folder.
func legacyProject(t *testing.T, composeBody string) (mpr, dockerDir string) {
	t.Helper()
	appDir := filepath.Join(t.TempDir(), "LegacyApp")
	dockerDir = filepath.Join(appDir, ".docker")
	if err := os.MkdirAll(dockerDir, 0755); err != nil {
		t.Fatal(err)
	}
	mpr = filepath.Join(appDir, "app.mpr")
	if err := os.WriteFile(mpr, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dockerDir, "docker-compose.yml"), []byte(composeBody), 0644); err != nil {
		t.Fatal(err)
	}
	return mpr, dockerDir
}

const legacyCompose = "# mxcli-template-version: 2\nservices:\n  mendix:\n    image: x\n"

func readName(t *testing.T, dockerDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dockerDir, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	n, ok := existingComposeName(data)
	if !ok {
		t.Fatalf("regenerated compose file has no name:\n%s", data)
	}
	return n
}

func TestInit_ForceOnLegacyProjectKeepsOldStackName(t *testing.T) {
	// Renaming the stack would make Compose create new, empty volumes and
	// orphan the old Postgres data. A regeneration of a file that had no name
	// must therefore pin the name Compose was using: the folder name, "docker".
	mpr, dockerDir := legacyProject(t, legacyCompose)
	var buf bytes.Buffer
	if err := Init(InitOptions{ProjectPath: mpr, Force: true, Stdout: &buf}); err != nil {
		t.Fatal(err)
	}
	if got := readName(t, dockerDir); got != `"docker"` {
		t.Errorf("name after --force on a legacy project = %s, want \"docker\"", got)
	}
	out := buf.String()
	for _, want := range []string{"docker", "mxcli-legacyapp", "empty"} {
		if !strings.Contains(strings.ToLower(out), want) {
			t.Errorf("output should explain the kept name and how to opt in (missing %q):\n%s", want, out)
		}
	}
}

func TestInit_ForceOnLegacyCustomOutputDirKeepsFolderDerivedName(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "App")
	custom := filepath.Join(appDir, "My Stack.Dir")
	if err := os.MkdirAll(custom, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(custom, "docker-compose.yml"), []byte(legacyCompose), 0644)
	mpr := filepath.Join(appDir, "app.mpr")
	os.WriteFile(mpr, nil, 0644)

	var buf bytes.Buffer
	if err := Init(InitOptions{ProjectPath: mpr, OutputDir: custom, Force: true, Stdout: &buf}); err != nil {
		t.Fatal(err)
	}
	if got := readName(t, custom); got != `"mystackdir"` {
		t.Errorf("name = %s, want \"mystackdir\" (what Compose derived from the folder)", got)
	}
}

func TestInit_ForceOnLegacyHonoursComposeProjectNameInEnv(t *testing.T) {
	// The workaround for #1364 was COMPOSE_PROJECT_NAME in .env; --force
	// rewrites .env, so the value has to move into the compose file or the
	// stack is renamed behind the user's back.
	mpr, dockerDir := legacyProject(t, legacyCompose)
	os.WriteFile(filepath.Join(dockerDir, ".env"), []byte("APP_PORT=9000\nCOMPOSE_PROJECT_NAME=\"shop\"\n"), 0644)
	var buf bytes.Buffer
	if err := Init(InitOptions{ProjectPath: mpr, Force: true, Stdout: &buf}); err != nil {
		t.Fatal(err)
	}
	if got := readName(t, dockerDir); got != `"shop"` {
		t.Errorf("name = %s, want \"shop\" (from the old .env)", got)
	}
}

func TestInit_ForceCarriesEnvProjectNameOverExplicitName(t *testing.T) {
	// Compose prefers COMPOSE_PROJECT_NAME over name:, so a stack with both is
	// really named by .env. --force rewrites .env; the real name must survive.
	mpr, dockerDir := legacyProject(t, "# mxcli-template-version: 3\nname: \"mxcli-a\"\nservices:\n")
	os.WriteFile(filepath.Join(dockerDir, ".env"), []byte("COMPOSE_PROJECT_NAME=shop\n"), 0644)
	var buf bytes.Buffer
	if err := Init(InitOptions{ProjectPath: mpr, Force: true, Stdout: &buf}); err != nil {
		t.Fatal(err)
	}
	if got := readName(t, dockerDir); got != `"shop"` {
		t.Errorf("name = %s, want \"shop\" (the name Compose was really using)", got)
	}
	env, _ := os.ReadFile(filepath.Join(dockerDir, ".env"))
	if strings.Contains(string(env), "COMPOSE_PROJECT_NAME") {
		t.Error(".env should be the regenerated template")
	}
}

func TestInit_NewComposeFileHonoursEnvProjectName(t *testing.T) {
	// compose file deleted, .env (with the old workaround) still there
	mpr, dockerDir := legacyProject(t, legacyCompose)
	os.Remove(filepath.Join(dockerDir, "docker-compose.yml"))
	os.WriteFile(filepath.Join(dockerDir, ".env"), []byte("COMPOSE_PROJECT_NAME=shop\n"), 0644)
	var buf bytes.Buffer
	if err := Init(InitOptions{ProjectPath: mpr, Stdout: &buf}); err != nil {
		t.Fatal(err)
	}
	if got := readName(t, dockerDir); got != `"shop"` {
		t.Errorf("name = %s, want \"shop\"", got)
	}
}

func TestInit_ForceKeepsExplicitName(t *testing.T) {
	// A user who opted in (or chose their own name) keeps it across --force.
	mpr, dockerDir := legacyProject(t, "# mxcli-template-version: 3\nname: my-own\nservices:\n")
	var buf bytes.Buffer
	if err := Init(InitOptions{ProjectPath: mpr, Force: true, Stdout: &buf}); err != nil {
		t.Fatal(err)
	}
	if got := readName(t, dockerDir); got != "my-own" {
		t.Errorf("name = %s, want my-own", got)
	}
}

func TestInit_NoForceNeverRenames(t *testing.T) {
	// Without --force the compose file is left alone, and a deleted .env being
	// recreated must not introduce a project name either.
	mpr, dockerDir := legacyProject(t, legacyCompose)
	var buf bytes.Buffer
	if err := Init(InitOptions{ProjectPath: mpr, Stdout: &buf}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dockerDir, "docker-compose.yml"))
	if string(data) != legacyCompose {
		t.Errorf("compose file must be untouched without --force:\n%s", data)
	}
	env, _ := os.ReadFile(filepath.Join(dockerDir, ".env"))
	if strings.Contains(string(env), "COMPOSE_PROJECT_NAME") {
		t.Error(".env must not set COMPOSE_PROJECT_NAME (the compose file's name: is the single source)")
	}
}
