// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestDownloadMxcliBinary_HTTP404ReturnsError verifies that a 404 from the
// release server is surfaced as an error. fetchDevcontainerMxcli turns this
// error into a warning in step 7 of cmd_new.go.
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
