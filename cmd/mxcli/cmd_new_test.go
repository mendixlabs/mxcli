// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDownloadMxcliBinary_HTTP404ReturnsError verifies that a 404 from the
// release server is surfaced as an error. This exercises the path in
// cmd_new.go step 4 that must exit 1 when the download fails.
func TestDownloadMxcliBinary_HTTP404ReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	// Temporarily override the GitHub releases URL by using a repo path that
	// maps to our test server. We test the underlying helper directly.
	outPath := filepath.Join(t.TempDir(), "mxcli")
	err := downloadMxcliBinaryFromURL(ts.URL+"/mxcli-linux-amd64", outPath, os.Stdout)
	if err == nil {
		t.Fatal("expected error on HTTP 404, got nil")
	}
}

// TestDownloadMxcliBinary_SuccessWritesBinary verifies that a successful
// download writes the binary to the output path.
func TestDownloadMxcliBinary_SuccessWritesBinary(t *testing.T) {
	content := []byte("fake-binary-content")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer ts.Close()

	outPath := filepath.Join(t.TempDir(), "mxcli")
	err := downloadMxcliBinaryFromURL(ts.URL+"/mxcli-linux-amd64", outPath, os.Stdout)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content mismatch: got %q, want %q", got, content)
	}
}

func TestValidateNewProjectPathLength_Boundary(t *testing.T) {
	longestRelativePath := asciiRelativePath(t, 181)
	for _, test := range []struct {
		name          string
		outputLength  int
		wantError     bool
		projectedPath int
	}{
		{name: "259 succeeds", outputLength: 77, projectedPath: 259},
		{name: "260 fails", outputLength: 78, wantError: true, projectedPath: 260},
	} {
		t.Run(test.name, func(t *testing.T) {
			outputDir := absolutePathWithLength(t, test.outputLength)
			err := validateNewProjectPathLength(outputDir, "11.12.2", longestRelativePath)
			if (err != nil) != test.wantError {
				t.Fatalf("validate path: error=%v wantError=%v", err, test.wantError)
			}
			projected := filepath.Join(outputDir, filepath.FromSlash(longestRelativePath))
			if got := utf16PathLen(projected); got != test.projectedPath {
				t.Fatalf("projected path length = %d, want %d", got, test.projectedPath)
			}
			if test.wantError {
				message := err.Error()
				for _, expected := range []string{"260 UTF-16", "at most 259", "shorter --output-dir", "at most 77"} {
					if !strings.Contains(message, expected) {
						t.Errorf("error %q does not contain %q", message, expected)
					}
				}
			}
		})
	}
}

func TestValidateNewProjectPathLength_CountsSurrogatePairs(t *testing.T) {
	if got := utf16PathLen("A😀한"); got != 4 {
		t.Fatalf("UTF-16 length = %d, want 4", got)
	}
	longestRelativePath := filepath.ToSlash(filepath.Join("한글", "deep-😀.txt"))
	base := filepath.Join(volumeRoot(t), "프로젝트-😀")
	remaining := 260 - utf16PathLen(base) - utf16PathLen(filepath.FromSlash(longestRelativePath)) - 2
	outputDir := filepath.Join(base, strings.Repeat("x", remaining))
	if got := utf16PathLen(filepath.Join(outputDir, filepath.FromSlash(longestRelativePath))); got != 260 {
		t.Fatalf("projected Unicode path length = %d, want 260", got)
	}
	if err := validateNewProjectPathLength(outputDir, "11.12.2", longestRelativePath); err == nil {
		t.Fatal("expected a 260-unit Unicode path to be rejected")
	}
}

func TestLongestBlankProjectTemplatePath_FindsVersionSpecificArchive(t *testing.T) {
	mxPath := fakeMxWithCore(t,
		embeddedZip(t, map[string]string{
			"System.mpr": "system",
			"short.txt":  "short",
		}),
		embeddedZip(t, map[string]string{
			"StarterApp_Blank.mpr":    "project",
			"a.txt":                   "a",
			"nested/b.txt":            "b",
			"nested/longest-name.txt": "longest",
		}),
	)

	got, err := longestBlankProjectTemplatePath(mxPath)
	if err != nil {
		t.Fatalf("inspect embedded project: %v", err)
	}
	if got != "nested/longest-name.txt" {
		t.Fatalf("longest path = %q, want version-specific NewProject archive path", got)
	}
}

func TestLongestBlankProjectTemplatePath_InstalledMx(t *testing.T) {
	mxPath := os.Getenv("MXCLI_TEST_MX")
	if mxPath == "" {
		t.Skip("set MXCLI_TEST_MX to an installed mx binary")
	}
	got, err := longestBlankProjectTemplatePath(mxPath)
	if err != nil {
		t.Fatalf("inspect installed MxToolset: %v", err)
	}
	if got == "" {
		t.Fatal("installed blank project template had no files")
	}
	t.Logf("longest blank-project template path: %d UTF-16 units: %s", utf16PathLen(got), got)
}

func TestCreateMendixProjectWithRollback_FailureLeavesNoOutput(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "new", "nested")
	target := filepath.Join(parent, "project")
	forced := errors.New("forced extraction failure")

	_, err := createMendixProjectWithRollback(target, "Failure", "test", "fake-mx",
		func(projectDir string) error {
			if err := os.WriteFile(filepath.Join(projectDir, "partial.txt"), []byte("partial"), 0o644); err != nil {
				return err
			}
			return forced
		})
	if !errors.Is(err, forced) {
		t.Fatalf("error = %v, want forced extraction failure", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("failed creation left output behind: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(parent), "nested")); !os.IsNotExist(err) {
		t.Fatalf("failed creation left newly-created parent directories behind: %v", err)
	}
}

func TestCreateMendixProjectWithRollback_FailurePreservesExistingEmptyOutput(t *testing.T) {
	target := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(target, 0o750); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	forced := errors.New("forced extraction failure")

	_, err = createMendixProjectWithRollback(target, "Failure", "test", "fake-mx",
		func(projectDir string) error {
			if err := os.WriteFile(filepath.Join(projectDir, "partial.txt"), []byte("partial"), 0o644); err != nil {
				return err
			}
			return forced
		})
	if !errors.Is(err, forced) {
		t.Fatalf("error = %v, want forced extraction failure", err)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatalf("existing empty output was removed: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("existing empty output was replaced after failed creation")
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatalf("existing output is no longer empty: entries=%v err=%v", entries, err)
	}
}

func TestCreateMendixProjectWithRollback_FailurePreservesSymlinkOutput(t *testing.T) {
	root := t.TempDir()
	realOutput := filepath.Join(root, "real-output")
	if err := os.Mkdir(realOutput, 0o755); err != nil {
		t.Fatal(err)
	}
	outputLink := filepath.Join(root, "short-output")
	if err := os.Symlink(realOutput, outputLink); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	forced := errors.New("forced extraction failure")

	_, err := createMendixProjectWithRollback(outputLink, "Failure", "test", "fake-mx",
		func(projectDir string) error {
			if err := os.WriteFile(filepath.Join(projectDir, "partial.txt"), []byte("partial"), 0o644); err != nil {
				return err
			}
			return forced
		})
	if !errors.Is(err, forced) {
		t.Fatalf("error = %v, want forced extraction failure", err)
	}
	if info, err := os.Lstat(outputLink); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("output symlink was not preserved: info=%v err=%v", info, err)
	}
	entries, err := os.ReadDir(realOutput)
	if err != nil || len(entries) != 0 {
		t.Fatalf("symlink target is no longer empty: entries=%v err=%v", entries, err)
	}
}

func TestCreateMendixProjectWithRollback_SucceedsWithExistingEmptyOutput(t *testing.T) {
	target := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(target, 0o750); err != nil {
		t.Fatal(err)
	}

	created, err := createMendixProjectWithRollback(target, "Existing", "test", "fake-mx",
		func(projectDir string) error {
			if err := os.WriteFile(filepath.Join(projectDir, "Existing.mpr"), []byte("fake"), 0o644); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(projectDir, "complete.txt"), []byte("complete"), 0o644)
		})
	if err != nil {
		t.Fatalf("create into existing empty output: %v", err)
	}
	if created.mprPath != filepath.Join(target, "Existing.mpr") {
		t.Fatalf("mpr path = %q, want final output path", created.mprPath)
	}
	if data, err := os.ReadFile(filepath.Join(target, "complete.txt")); err != nil || string(data) != "complete" {
		t.Fatalf("completed project was not committed: data=%q err=%v", data, err)
	}
}

func TestCreateMendixProjectWithRollback_MissingMPRRollsBack(t *testing.T) {
	target := filepath.Join(t.TempDir(), "project")
	_, err := createMendixProjectWithRollback(target, "Missing", "test", "fake-mx",
		func(projectDir string) error {
			return os.WriteFile(filepath.Join(projectDir, "partial.txt"), []byte("partial"), 0o644)
		})
	if err == nil || !strings.Contains(err.Error(), "did not produce an .mpr") {
		t.Fatalf("error = %v, want missing .mpr error", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("missing .mpr left output behind: %v", err)
	}
}

func TestCreateMendixProjectWithRollback_RejectsNonEmptyOutputBeforeCreation(t *testing.T) {
	target := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	called := false

	_, err := createMendixProjectWithRollback(target, "Existing", "test", "fake-mx",
		func(string) error {
			called = true
			return nil
		})
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("error = %v, want non-empty output error", err)
	}
	if called {
		t.Fatal("creator ran for a non-empty output directory")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("existing file changed: data=%q err=%v", data, err)
	}
}

func fakeMxWithCore(t *testing.T, archives ...[]byte) string {
	t.Helper()
	dir := t.TempDir()
	mxPath := filepath.Join(dir, "mx")
	if err := os.WriteFile(mxPath, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("managed assembly prefix")
	for _, archive := range archives {
		data = append(data, archive...)
	}
	data = append(data, []byte("managed assembly suffix")...)
	if err := os.WriteFile(filepath.Join(dir, "Mendix.Modeler.Core.dll"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return mxPath
}

func embeddedZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func absolutePathWithLength(t *testing.T, length int) string {
	t.Helper()
	root := volumeRoot(t)
	relativeLength := length - utf16PathLen(root)
	result := filepath.Join(root, asciiRelativePath(t, relativeLength))
	if got := utf16PathLen(result); got != length {
		t.Fatalf("constructed path length = %d, want %d (%s)", got, length, result)
	}
	return result
}

func volumeRoot(t *testing.T) string {
	t.Helper()
	volume := filepath.VolumeName(t.TempDir())
	return volume + string(filepath.Separator)
}

func asciiRelativePath(t *testing.T, length int) string {
	t.Helper()
	if length < 1 {
		t.Fatalf("relative path length must be positive, got %d", length)
	}
	const componentLength = 80
	var components []string
	for length > componentLength {
		partLength := componentLength
		if length == componentLength+1 {
			partLength-- // leave one unit after the separator
		}
		components = append(components, strings.Repeat("d", partLength))
		length -= partLength + 1 // include the path separator
	}
	if length < 1 {
		t.Fatalf("cannot construct relative path with requested remainder %d", length)
	}
	components = append(components, strings.Repeat("f", length))
	return filepath.Join(components...)
}
