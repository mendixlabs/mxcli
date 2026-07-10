// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/internal/marketplace"
)

func TestStartSearchProgress_Gating(t *testing.T) {
	t.Run("nil for --json", func(t *testing.T) {
		c := marketplace.New(http.DefaultClient)
		if startSearchProgress(c, os.Stderr, true) != nil {
			t.Error("expected no progress for --json output")
		}
		if c.OnProgress != nil {
			t.Error("OnProgress must stay unset for --json")
		}
	})
	t.Run("nil for non-terminal writer", func(t *testing.T) {
		c := marketplace.New(http.DefaultClient)
		// A bytes.Buffer is not an *os.File, so it's never an interactive terminal.
		if startSearchProgress(c, &bytes.Buffer{}, false) != nil {
			t.Error("expected no progress for a non-terminal writer (pipe/buffer)")
		}
		if c.OnProgress != nil {
			t.Error("OnProgress must stay unset for a non-terminal writer")
		}
	})
}

func TestResolveVersion(t *testing.T) {
	versions := []marketplace.Version{
		{VersionNumber: "7.0.3", DownloadURL: "u3"},
		{VersionNumber: "7.0.2", DownloadURL: "u2"},
	}
	t.Run("latest (empty) picks first", func(t *testing.T) {
		v, err := resolveVersion(versions, "")
		if err != nil || v.VersionNumber != "7.0.3" {
			t.Fatalf("got %v, %v; want 7.0.3", v, err)
		}
	})
	t.Run("specific match", func(t *testing.T) {
		v, err := resolveVersion(versions, "7.0.2")
		if err != nil || v.VersionNumber != "7.0.2" {
			t.Fatalf("got %v, %v; want 7.0.2", v, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		if _, err := resolveVersion(versions, "9.9.9"); err == nil {
			t.Fatal("expected error for missing version")
		}
	})
	t.Run("empty list", func(t *testing.T) {
		if _, err := resolveVersion(nil, ""); err == nil {
			t.Fatal("expected error for empty version list")
		}
	})
}

// marketplaceTestHandler serves the content/versions/download/CDN endpoints for a
// single content item, building absolute URLs from the request host so the
// download redirect points back at the same test server.
func marketplaceTestHandler(contentType, filename, mpkBody string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		switch {
		case strings.HasSuffix(r.URL.Path, "/versions"):
			fmt.Fprintf(w, `{"items":[{"name":"X","versionId":"v1","versionNumber":"1.0.0","minSupportedMendixVersion":"10.0.0","publicationDate":"2026-01-01T00:00:00Z","downloadUrl":%q}]}`, base+"/v1/versions/v1/download")
		case strings.HasSuffix(r.URL.Path, "/download"):
			http.Redirect(w, r, base+"/cdn/"+filename, http.StatusSeeOther)
		case strings.HasPrefix(r.URL.Path, "/cdn/"):
			_, _ = w.Write([]byte(mpkBody))
		case strings.HasPrefix(r.URL.Path, "/v1/content/"):
			fmt.Fprintf(w, `{"contentId":42,"publisher":"Mendix","type":%q,"isPrivate":false}`, contentType)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestMarketplaceDownload_WritesFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.mpk")
	out, err := runMarketplace(t, marketplaceTestHandler("Module", "DatabaseConnector-v1.0.0.mpk", "PK\x03\x04mpk"),
		"download", "42", "-o", dest)
	if err != nil {
		t.Fatalf("download failed: %v\n%s", err, out)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("output not written: %v", err)
	}
	if string(got) != "PK\x03\x04mpk" {
		t.Errorf("content = %q, want the mpk body", string(got))
	}
	// No temp .part file left behind in the dest directory.
	entries, _ := os.ReadDir(filepath.Dir(dest))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestMarketplaceInstall_WidgetRouting(t *testing.T) {
	projDir := t.TempDir()
	mpr := filepath.Join(projDir, "app.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runMarketplace(t, marketplaceTestHandler("Widget", "RadioButtonList.mpk", "PK\x03\x04widget"),
		"install", "42", "-p", mpr)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(projDir, "widgets", "RadioButtonList.mpk"))
	if err != nil {
		t.Fatalf("widget not placed in widgets/: %v\noutput: %s", err, out)
	}
	if string(got) != "PK\x03\x04widget" {
		t.Errorf("widget content = %q", string(got))
	}
}
