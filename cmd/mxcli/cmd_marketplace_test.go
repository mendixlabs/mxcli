// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/internal/marketplace"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const sampleContentList = `{"items":[{"contentId":170,"publisher":"Mendix","type":"Module","categories":[{"name":"Utility"}],"supportCategory":"Platform","isPrivate":false,"latestVersion":{"name":"Community Commons","versionId":"0a03e65a","versionNumber":"11.5.0","minSupportedMendixVersion":"10.24.0","publicationDate":"2026-01-13T06:57:14.512Z"}}]}`

const sampleContent = `{"contentId":170,"publisher":"Mendix","type":"Module","categories":[{"name":"Utility"}],"supportCategory":"Platform","licenseUrl":"http://www.apache.org/licenses/LICENSE-2.0.html","isPrivate":false,"latestVersion":{"name":"Community Commons","versionId":"0a03e65a","versionNumber":"11.5.0","minSupportedMendixVersion":"10.24.0","publicationDate":"2026-01-13T06:57:14.512Z"}}`

const sampleVersions = `{"items":[{"name":"Community Commons","versionId":"0a03e65a","versionNumber":"11.5.0","minSupportedMendixVersion":"10.24.0","publicationDate":"2026-01-13T06:57:14.512Z","releaseNotes":"<p>upgraded guava</p>"}]}`

func resetMarketplaceFlags() {
	for _, c := range []*cobra.Command{marketplaceSearchCmd, marketplaceInfoCmd, marketplaceVersionsCmd, marketplaceDownloadCmd, marketplaceInstallCmd} {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
}

// runMarketplace invokes the marketplace subtree with the given handler
// used for every API call. Returns captured stdout+stderr.
func runMarketplace(t *testing.T, handler http.HandlerFunc, args ...string) (string, error) {
	t.Helper()
	// Isolate the catalog cache (~/.mxcli/...) into a temp HOME so tests never
	// read or write the real user cache.
	t.Setenv("HOME", t.TempDir())
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	origFactory := marketplaceClientFactory
	marketplaceClientFactory = func(_ context.Context, _ *cobra.Command) (*marketplace.Client, error) {
		return marketplace.NewWithBaseURL(ts.Client(), ts.URL), nil
	}
	t.Cleanup(func() { marketplaceClientFactory = origFactory })

	resetMarketplaceFlags()

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(append([]string{"marketplace"}, args...))
	err := rootCmd.ExecuteContext(context.Background())
	return out.String(), err
}

func TestMarketplaceSearch_TableOutput(t *testing.T) {
	out, err := runMarketplace(t, func(w http.ResponseWriter, r *http.Request) {
		// Search paginates the catalog (the API ignores ?search=): first page
		// is limit=100&offset=0, then client-side filtering.
		if !strings.Contains(r.URL.RawQuery, "offset=0") {
			t.Errorf("expected paginated request (offset=0), got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(sampleContentList))
	}, "search", "community")

	if err != nil {
		t.Fatalf("run: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "170") || !strings.Contains(out, "Community Commons") {
		t.Errorf("expected content in table, got: %s", out)
	}
	if !strings.Contains(out, "ID") || !strings.Contains(out, "PUBLISHER") {
		t.Errorf("expected table header, got: %s", out)
	}
}

func TestMarketplaceSearch_JSON(t *testing.T) {
	out, err := runMarketplace(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sampleContentList))
	}, "search", "community", "--json")

	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	items, ok := got["items"].([]any)
	if !ok || len(items) != 1 {
		t.Errorf("expected items[1], got: %v", got)
	}
}

func TestMarketplaceSearch_NoResults(t *testing.T) {
	out, err := runMarketplace(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"items":[]}`))
	}, "search", "nothing")

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No results") {
		t.Errorf("expected 'No results', got: %s", out)
	}
}

func TestMarketplaceInfo(t *testing.T) {
	out, err := runMarketplace(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/content/170" {
			t.Errorf("path: got %q, want /v1/content/170", r.URL.Path)
		}
		_, _ = w.Write([]byte(sampleContent))
	}, "info", "170")

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "170") || !strings.Contains(out, "Community Commons") {
		t.Errorf("expected content ID + name, got: %s", out)
	}
	if !strings.Contains(out, "Utility") {
		t.Errorf("expected category in detail: %s", out)
	}
}

func TestMarketplaceInfo_InvalidID(t *testing.T) {
	_, err := runMarketplace(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called for invalid ID")
	}, "info", "not-a-number")

	if err == nil {
		t.Fatal("expected error for invalid id")
	}
	if !strings.Contains(err.Error(), "invalid content id") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMarketplaceVersions(t *testing.T) {
	out, err := runMarketplace(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/content/170/versions" {
			t.Errorf("path: got %q, want /v1/content/170/versions", r.URL.Path)
		}
		_, _ = w.Write([]byte(sampleVersions))
	}, "versions", "170")

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "11.5.0") || !strings.Contains(out, "10.24.0") {
		t.Errorf("expected version numbers, got: %s", out)
	}
}

func TestMarketplaceVersions_MinMendixFilter(t *testing.T) {
	body := `{"items":[
		{"name":"X","versionId":"a","versionNumber":"3.0.0","minSupportedMendixVersion":"11.0.0","publicationDate":"2026-01-01T00:00:00Z"},
		{"name":"X","versionId":"b","versionNumber":"2.0.0","minSupportedMendixVersion":"10.24.0","publicationDate":"2026-01-01T00:00:00Z"},
		{"name":"X","versionId":"c","versionNumber":"1.0.0","minSupportedMendixVersion":"9.0.0","publicationDate":"2026-01-01T00:00:00Z"}
	]}`

	out, err := runMarketplace(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}, "versions", "170", "--min-mendix", "10.24.0")

	if err != nil {
		t.Fatal(err)
	}
	// Versions requiring Mendix > 10.24.0 must be filtered out.
	if strings.Contains(out, "3.0.0") {
		t.Errorf("3.0.0 (needs 11.0) should be filtered out:\n%s", out)
	}
	// Versions with minSupported <= 10.24.0 must remain.
	if !strings.Contains(out, "2.0.0") {
		t.Errorf("2.0.0 (needs 10.24.0) should remain:\n%s", out)
	}
	if !strings.Contains(out, "1.0.0") {
		t.Errorf("1.0.0 (needs 9.0) should remain:\n%s", out)
	}
}

func TestCompareSemverLike(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"10.24.0", "10.24.0", 0},
		{"10.24.0", "10.24.1", -1},
		{"10.24.1", "10.24.0", 1},
		{"10.0.0", "10", 0},       // missing components treated as 0
		{"10.24", "10.24.0", 0},   // ditto
		{"9.0.0", "10.0.0", -1},   // major difference
		{"11.0.0", "10.24.11", 1}, // two-digit minor
	}
	for _, tc := range cases {
		got := compareSemverLike(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestMarketplace_APIErrorSurfaces(t *testing.T) {
	_, err := runMarketplace(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}, "info", "170")

	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention 500: %v", err)
	}
}
