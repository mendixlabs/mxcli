// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr/version"
)

// newTestZipWriter wraps zip.NewWriter for test helpers.
func newTestZipWriter(f *os.File) *zip.Writer {
	return zip.NewWriter(f)
}

func TestPatchStartPermissions_Applied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod not supported on Windows")
	}
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	os.MkdirAll(binDir, 0755)
	os.WriteFile(filepath.Join(binDir, "start"), []byte("#!/bin/sh\n"), 0644)

	result := patchStartPermissions(dir)
	if result.Status != "applied" {
		t.Errorf("expected applied, got %s", result.Status)
	}

	info, _ := os.Stat(filepath.Join(binDir, "start"))
	if info.Mode()&0111 == 0 {
		t.Error("start should be executable after patch")
	}
}

func TestPatchStartPermissions_Skipped_AlreadyExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod not supported on Windows")
	}
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	os.MkdirAll(binDir, 0755)
	os.WriteFile(filepath.Join(binDir, "start"), []byte("#!/bin/sh\n"), 0755)

	result := patchStartPermissions(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped, got %s", result.Status)
	}
}

func TestPatchStartPermissions_Skipped_NoFile(t *testing.T) {
	dir := t.TempDir()
	result := patchStartPermissions(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped, got %s", result.Status)
	}
}

func TestPatchDockerfileCMD_Applied(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM openjdk:21
WORKDIR /mendix
CMD ["./bin/start.sh", "etc/Default"]
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	result := patchDockerfileCMD(dir)
	if result.Status != "applied" {
		t.Errorf("expected applied, got %s", result.Status)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if strings.Contains(string(content), "start.sh") {
		t.Error("Dockerfile should not contain start.sh after patch")
	}
	if !strings.Contains(string(content), `"./bin/start"`) {
		t.Error("Dockerfile should contain ./bin/start after patch")
	}
}

func TestPatchDockerfileCMD_Skipped_AlreadyFixed(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM openjdk:21
CMD ["./bin/start", "etc/Default"]
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	result := patchDockerfileCMD(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped, got %s", result.Status)
	}
}

func TestPatchDockerfileCMD_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM openjdk:21
CMD ["./bin/start.sh", "etc/Default"]
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	patchDockerfileCMD(dir)
	result := patchDockerfileCMD(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped on second run, got %s", result.Status)
	}
}

func TestPatchBaseImage_Applied_Openjdk21(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM openjdk:21
WORKDIR /mendix
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	result := patchBaseImage(dir)
	if result.Status != "applied" {
		t.Errorf("expected applied, got %s", result.Status)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if !strings.Contains(string(content), "eclipse-temurin:21-jre") {
		t.Error("should contain eclipse-temurin:21-jre")
	}
	if strings.Contains(string(content), "openjdk") {
		t.Error("should not contain openjdk after patch")
	}
}

func TestPatchBaseImage_Applied_Openjdk21Jdk(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM openjdk:21-jdk
WORKDIR /mendix
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	result := patchBaseImage(dir)
	if result.Status != "applied" {
		t.Errorf("expected applied, got %s", result.Status)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if !strings.Contains(string(content), "eclipse-temurin:21-jre") {
		t.Error("should contain eclipse-temurin:21-jre")
	}
}

func TestPatchBaseImage_Applied_Openjdk21Slim(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM openjdk:21-slim
WORKDIR /mendix
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	result := patchBaseImage(dir)
	if result.Status != "applied" {
		t.Errorf("expected applied, got %s", result.Status)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if !strings.Contains(string(content), "eclipse-temurin:21-jre") {
		t.Error("should contain eclipse-temurin:21-jre")
	}
}

func TestPatchBaseImage_Skipped_AlreadyPatched(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM eclipse-temurin:21-jre
WORKDIR /mendix
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	result := patchBaseImage(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped, got %s", result.Status)
	}
}

func TestPatchBaseImage_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM openjdk:21
WORKDIR /mendix
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	patchBaseImage(dir)
	result := patchBaseImage(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped on second run, got %s", result.Status)
	}
}

func TestPatchHealthcheck_Applied(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM openjdk:21
WORKDIR /mendix
EXPOSE 8080
CMD ["./bin/start", "etc/Default"]
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	result := patchHealthcheck(dir)
	if result.Status != "applied" {
		t.Errorf("expected applied, got %s", result.Status)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	s := string(content)
	if !strings.Contains(s, "HEALTHCHECK") {
		t.Error("should contain HEALTHCHECK after patch")
	}
	// HEALTHCHECK should appear before CMD
	healthIdx := strings.Index(s, "HEALTHCHECK")
	cmdIdx := strings.Index(s, "CMD")
	if healthIdx >= cmdIdx {
		t.Error("HEALTHCHECK should appear before CMD")
	}
}

func TestPatchHealthcheck_Skipped_AlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM openjdk:21
HEALTHCHECK --interval=15s CMD curl -f http://localhost:8080/ || exit 1
CMD ["./bin/start", "etc/Default"]
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	result := patchHealthcheck(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped, got %s", result.Status)
	}
}

func TestPatchHealthcheck_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM openjdk:21
CMD ["./bin/start", "etc/Default"]
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	patchHealthcheck(dir)
	result := patchHealthcheck(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped on second run, got %s", result.Status)
	}
}

func TestPatchAdminAddresses_Applied(t *testing.T) {
	dir := t.TempDir()
	etcDir := filepath.Join(dir, "etc")
	os.MkdirAll(etcDir, 0755)
	os.WriteFile(filepath.Join(etcDir, "Default"), []byte("# Default config\n"), 0644)

	result := patchAdminAddresses(dir)
	if result.Status != "applied" {
		t.Errorf("expected applied, got %s", result.Status)
	}

	content, _ := os.ReadFile(filepath.Join(etcDir, "Default"))
	s := string(content)
	if !strings.Contains(s, `addresses = ["*"]`) {
		t.Error("should contain admin.addresses after patch")
	}
	if !strings.Contains(s, "# Default config") {
		t.Error("should preserve existing content")
	}
}

func TestPatchAdminAddresses_Skipped_AlreadyPatched(t *testing.T) {
	dir := t.TempDir()
	etcDir := filepath.Join(dir, "etc")
	os.MkdirAll(etcDir, 0755)
	os.WriteFile(filepath.Join(etcDir, "Default"), []byte("admin.addresses = [\"*\"]\n"), 0644)

	result := patchAdminAddresses(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped, got %s", result.Status)
	}
}

func TestPatchAdminAddresses_Skipped_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	result := patchAdminAddresses(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped, got %s", result.Status)
	}
}

func TestApplyPatches_116x_Gets7Patches(t *testing.T) {
	dir := t.TempDir()

	// Create bin/start without execute permission
	binDir := filepath.Join(dir, "bin")
	os.MkdirAll(binDir, 0755)
	os.WriteFile(filepath.Join(binDir, "start"), []byte("#!/bin/sh\n"), 0644)

	// Create Dockerfile with all issues
	dockerfile := `FROM openjdk:21
WORKDIR /mendix
CMD ["./bin/start.sh", "etc/Default"]
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	// Create etc/Default for admin addresses patch
	etcDir := filepath.Join(dir, "etc")
	os.MkdirAll(etcDir, 0755)
	os.WriteFile(filepath.Join(etcDir, "Default"), []byte("# config\n"), 0644)

	pv := &version.ProjectVersion{
		ProductVersion: "11.6.1",
		MajorVersion:   11,
		MinorVersion:   6,
		PatchVersion:   1,
	}

	results := ApplyPatches(dir, pv)
	if len(results) != 7 {
		t.Errorf("expected 7 patches for 11.6.x, got %d", len(results))
	}

	applied := 0
	for _, r := range results {
		if r.Status == "applied" {
			applied++
		}
	}
	if applied != 7 {
		t.Errorf("expected 7 applied patches, got %d", applied)
	}
}

func TestApplyPatches_12x_Gets6Patches(t *testing.T) {
	dir := t.TempDir()

	// Create bin/start without execute permission
	binDir := filepath.Join(dir, "bin")
	os.MkdirAll(binDir, 0755)
	os.WriteFile(filepath.Join(binDir, "start"), []byte("#!/bin/sh\n"), 0644)

	// Create Dockerfile with base image issue and config arg
	dockerfile := `FROM openjdk:21
WORKDIR /mendix
CMD ["./bin/start", "etc/Default"]
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	// Create etc/Default for admin addresses patch
	etcDir := filepath.Join(dir, "etc")
	os.MkdirAll(etcDir, 0755)
	os.WriteFile(filepath.Join(etcDir, "Default"), []byte("# config\n"), 0644)

	pv := &version.ProjectVersion{
		ProductVersion: "12.0.0",
		MajorVersion:   12,
		MinorVersion:   0,
		PatchVersion:   0,
	}

	results := ApplyPatches(dir, pv)
	if len(results) != 6 {
		t.Errorf("expected 6 patches for 12.x, got %d", len(results))
	}
}

func TestFindPADDir_InRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM test"), 0644)

	padDir, err := findPADDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if padDir != dir {
		t.Errorf("expected %s, got %s", dir, padDir)
	}
}

func TestFindPADDir_InSubdir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "MyApp-portable")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "Dockerfile"), []byte("FROM test"), 0644)

	padDir, err := findPADDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if padDir != subdir {
		t.Errorf("expected %s, got %s", subdir, padDir)
	}
}

func TestFindPADDir_NotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := findPADDir(dir)
	if err == nil {
		t.Error("expected error for missing PAD output")
	}
}

func TestFindPADDir_DockerCompose_InRoot(t *testing.T) {
	dir := t.TempDir()
	composeDir := filepath.Join(dir, "docker_compose")
	os.MkdirAll(composeDir, 0755)
	os.WriteFile(filepath.Join(composeDir, "Default.yaml"), []byte("services:"), 0644)

	padDir, err := findPADDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if padDir != dir {
		t.Errorf("expected %s, got %s", dir, padDir)
	}
}

func TestFindPADDir_DockerCompose_InSubdir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "MyApp-portable")
	composeDir := filepath.Join(subdir, "docker_compose")
	os.MkdirAll(composeDir, 0755)
	os.WriteFile(filepath.Join(composeDir, "Default.yaml"), []byte("services:"), 0644)

	padDir, err := findPADDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if padDir != subdir {
		t.Errorf("expected %s, got %s", subdir, padDir)
	}
}

func TestFindPADDir_ExtractedLayoutInRoot(t *testing.T) {
	dir := t.TempDir()
	for _, subdir := range []string{
		filepath.Join(dir, "app"),
		filepath.Join(dir, "etc"),
		filepath.Join(dir, "bin"),
		filepath.Join(dir, "lib", "runtime", "launcher"),
	} {
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", subdir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "start"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write start script: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib", "runtime", "launcher", "runtimelauncher.jar"), []byte("fake"), 0644); err != nil {
		t.Fatalf("write launcher: %v", err)
	}

	padDir, err := findPADDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if padDir != dir {
		t.Errorf("expected %s, got %s", dir, padDir)
	}
}

func TestFindPADDir_PrefersDockerfile(t *testing.T) {
	dir := t.TempDir()
	// Both Dockerfile and docker_compose exist — Dockerfile wins
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM test"), 0644)
	composeDir := filepath.Join(dir, "docker_compose")
	os.MkdirAll(composeDir, 0755)
	os.WriteFile(filepath.Join(composeDir, "Default.yaml"), []byte("services:"), 0644)

	padDir, err := findPADDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if padDir != dir {
		t.Errorf("expected %s, got %s", dir, padDir)
	}
}

func TestGenerateDockerfile_CreatesWhenMissing(t *testing.T) {
	dir := t.TempDir()

	err := generateDockerfile(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		t.Fatalf("Dockerfile not created: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "eclipse-temurin:21-jre") {
		t.Error("Dockerfile should contain eclipse-temurin:21-jre")
	}
	if !strings.Contains(s, `CMD ["./bin/start"]`) {
		t.Error("Dockerfile should contain CMD with start script (no config arg)")
	}
	if !strings.Contains(s, "HEALTHCHECK") {
		t.Error("Dockerfile should contain HEALTHCHECK")
	}
}

func TestGenerateDockerfile_SkipsWhenExists(t *testing.T) {
	dir := t.TempDir()
	existing := "FROM custom:image\n"
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(existing), 0644)

	err := generateDockerfile(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if string(content) != existing {
		t.Error("existing Dockerfile should not be overwritten")
	}
}

func TestInjectRuntime_SkipsWhenPresent(t *testing.T) {
	dir := t.TempDir()
	// Create a fake runtime launcher jar
	launcherDir := filepath.Join(dir, "lib", "runtime", "launcher")
	os.MkdirAll(launcherDir, 0755)
	os.WriteFile(filepath.Join(launcherDir, "runtimelauncher.jar"), []byte("fake"), 0644)

	var buf strings.Builder
	err := injectRuntime(dir, "11.6.3", &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "already present") {
		t.Error("should report runtime already present")
	}
}

func TestExtractPADZip_ExtractsZip(t *testing.T) {
	dir := t.TempDir()

	// Create a test ZIP with a docker_compose/Default.yaml inside a subdirectory
	zipPath := filepath.Join(dir, "MyApp_1.0.0.1.zip")
	createTestZip(t, zipPath, map[string]string{
		"MyApp-portable/docker_compose/Default.yaml": "services:",
		"MyApp-portable/bin/start":                   "#!/bin/sh\n",
	})

	var buf strings.Builder
	err := extractPADZip(dir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify extraction happened
	if _, err := os.Stat(filepath.Join(dir, "MyApp-portable", "docker_compose", "Default.yaml")); err != nil {
		t.Error("docker_compose/Default.yaml should exist after extraction")
	}
	if !strings.Contains(buf.String(), "Extracting") {
		t.Error("should report extraction")
	}
}

func TestExtractPADZip_ExtractsEvenWhenPADExists(t *testing.T) {
	dir := t.TempDir()

	// Create already-extracted PAD content from a previous build
	composeDir := filepath.Join(dir, "docker_compose")
	os.MkdirAll(composeDir, 0755)
	os.WriteFile(filepath.Join(composeDir, "Default.yaml"), []byte("old"), 0644)

	// Create a new ZIP from a subsequent build — must be extracted
	zipPath := filepath.Join(dir, "MyApp_1.0.0.2.zip")
	createTestZip(t, zipPath, map[string]string{
		"MyApp-portable/docker_compose/Default.yaml": "new",
		"MyApp-portable/bin/start":                   "#!/bin/sh\nnew",
	})

	var buf strings.Builder
	err := extractPADZip(dir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Extracting") {
		t.Error("should extract ZIP even when previous PAD output exists")
	}

	// ZIP should be cleaned up
	if _, err := os.Stat(zipPath); err == nil {
		t.Error("ZIP should be deleted after extraction")
	}

	// New content should be extracted in the subdirectory
	content, _ := os.ReadFile(filepath.Join(dir, "MyApp-portable", "docker_compose", "Default.yaml"))
	if string(content) != "new" {
		t.Errorf("expected new content, got %s", string(content))
	}
}

func TestExtractPADZip_NoOpWhenNoZip(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder
	err := extractPADZip(dir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatchDockerfileCMDConfigArg_Applied(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM eclipse-temurin:21-jre
WORKDIR /mendix
CMD ["./bin/start", "etc/Default"]
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	result := patchDockerfileCMDConfigArg(dir)
	if result.Status != "applied" {
		t.Errorf("expected applied, got %s", result.Status)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	s := string(content)
	if strings.Contains(s, "etc/Default") {
		t.Error("should not contain etc/Default after patch")
	}
	if !strings.Contains(s, `CMD ["./bin/start"]`) {
		t.Error("should contain CMD [\"./bin/start\"] after patch")
	}
}

func TestPatchDockerfileCMDConfigArg_Skipped_NoConfigArg(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM eclipse-temurin:21-jre
CMD ["./bin/start"]
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	result := patchDockerfileCMDConfigArg(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped, got %s", result.Status)
	}
}

func TestPatchDockerfileCMDConfigArg_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM eclipse-temurin:21-jre
CMD ["./bin/start", "etc/Default"]
`
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644)

	patchDockerfileCMDConfigArg(dir)
	result := patchDockerfileCMDConfigArg(dir)
	if result.Status != "skipped" {
		t.Errorf("expected skipped on second run, got %s", result.Status)
	}
}

// createTestZip creates a ZIP file with the given files (path -> content).
func createTestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("creating zip file: %v", err)
	}
	defer f.Close()

	w := newTestZipWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("adding %s to zip: %v", name, err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("writing %s to zip: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
}

func TestFlattenPADDir_MovesContents(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "MyApp-portable")
	os.MkdirAll(filepath.Join(subdir, "bin"), 0755)
	os.MkdirAll(filepath.Join(subdir, "app"), 0755)
	os.MkdirAll(filepath.Join(subdir, "docker_compose"), 0755)
	os.WriteFile(filepath.Join(subdir, "bin", "start"), []byte("#!/bin/sh\n"), 0755)
	os.WriteFile(filepath.Join(subdir, "docker_compose", "Default.yaml"), []byte("services:"), 0644)

	result, err := flattenPADDir(subdir, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != dir {
		t.Errorf("expected result to be outputDir, got %s", result)
	}

	// Contents should be at the top level
	if _, err := os.Stat(filepath.Join(dir, "bin", "start")); err != nil {
		t.Error("bin/start should exist at top level after flatten")
	}
	if _, err := os.Stat(filepath.Join(dir, "docker_compose", "Default.yaml")); err != nil {
		t.Error("docker_compose/Default.yaml should exist at top level after flatten")
	}

	// Subdirectory should be removed
	if _, err := os.Stat(subdir); err == nil {
		t.Error("subdirectory should be removed after flatten")
	}
}

func TestFlattenPADDir_OverwritesOldContents(t *testing.T) {
	dir := t.TempDir()

	// Old build contents at top level
	os.MkdirAll(filepath.Join(dir, "bin"), 0755)
	os.WriteFile(filepath.Join(dir, "bin", "start"), []byte("old"), 0755)

	// New PAD in subdirectory
	subdir := filepath.Join(dir, "MyApp-portable")
	os.MkdirAll(filepath.Join(subdir, "bin"), 0755)
	os.WriteFile(filepath.Join(subdir, "bin", "start"), []byte("new"), 0755)

	_, err := flattenPADDir(subdir, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "bin", "start"))
	if string(content) != "new" {
		t.Errorf("expected new content, got %s", string(content))
	}
}

func TestDescribePatches_116x(t *testing.T) {
	pv := &version.ProjectVersion{MajorVersion: 11, MinorVersion: 6, PatchVersion: 1}
	patches := DescribePatches(pv)
	if len(patches) != 7 {
		t.Errorf("expected 7 patches for 11.6.x, got %d", len(patches))
	}
}

func TestDescribePatches_12x(t *testing.T) {
	pv := &version.ProjectVersion{MajorVersion: 12, MinorVersion: 0, PatchVersion: 0}
	patches := DescribePatches(pv)
	if len(patches) != 6 {
		t.Errorf("expected 6 patches for 12.x, got %d", len(patches))
	}
}

func TestPatchRuntimePorts_Applied(t *testing.T) {
	dir := t.TempDir()
	etcDir := filepath.Join(dir, "etc")
	os.MkdirAll(etcDir, 0755)
	os.WriteFile(filepath.Join(etcDir, "Default"), []byte("# root config\ninclude file(\"etc/configurations/Default.conf\")\n"), 0644)

	result := patchRuntimePorts(dir)
	if result.Status != "applied" {
		t.Fatalf("expected applied, got %s (%v)", result.Status, result.Error)
	}
	s := string(mustRead(t, filepath.Join(etcDir, "Default")))
	if !strings.Contains(s, "runtime.http { port = 8080 }") {
		t.Error("should set the runtime http port to 8080")
	}
	if !strings.Contains(s, "admin { port = 8090 }") {
		t.Error("should set the admin port to 8090")
	}
	if !strings.Contains(s, "# root config") {
		t.Error("should preserve existing content")
	}
}

func TestPatchRuntimePorts_Skipped_AlreadyPatched(t *testing.T) {
	dir := t.TempDir()
	etcDir := filepath.Join(dir, "etc")
	os.MkdirAll(etcDir, 0755)
	os.WriteFile(filepath.Join(etcDir, "Default"), []byte("# mxcli: runtime/admin ports\n"), 0644)
	if got := patchRuntimePorts(dir).Status; got != "skipped" {
		t.Errorf("expected skipped, got %s", got)
	}
}

func TestPatchRuntimePorts_Skipped_NoFile(t *testing.T) {
	if got := patchRuntimePorts(t.TempDir()).Status; got != "skipped" {
		t.Errorf("expected skipped, got %s", got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
