// SPDX-License-Identifier: Apache-2.0

package marketplace_test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/internal/auth"
	"github.com/JordtenBulte-OLC/mxcli/internal/marketplace"
)

// TestLive_DownloadFlow is a drift detector against the real marketplace API.
// It is gated on MENDIX_PAT: without a token it skips, so normal CI and
// contributors without credentials are unaffected. When a PAT IS present it
// verifies the two things the download feature depends on and that are owned by
// Mendix (so they can change under us):
//
//  1. the versions endpoint still returns a non-empty `downloadUrl` per version
//  2. that URL still 303-redirects to a real .mpk on the CDN (a valid zip)
//
// Content 20 ("Radiobutton List", a small widget) keeps the download tiny.
func TestLive_DownloadFlow(t *testing.T) {
	pat := os.Getenv("MENDIX_PAT")
	if pat == "" {
		t.Skip("set MENDIX_PAT to run the live marketplace download drift test")
	}

	ctx := context.Background()
	httpClient, err := auth.ClientFor(ctx, auth.ProfileDefault)
	if err != nil {
		t.Fatalf("auth client (MENDIX_PAT set but unusable): %v", err)
	}
	client := marketplace.New(httpClient)

	const contentID = 20
	list, err := client.Versions(ctx, contentID)
	if err != nil {
		t.Fatalf("versions(%d): %v", contentID, err)
	}
	if len(list.Items) == 0 {
		t.Fatalf("versions(%d) returned no items", contentID)
	}

	// Drift check 1: the downloadUrl field is still populated.
	var withURL *marketplace.Version
	for i := range list.Items {
		if list.Items[i].DownloadURL != "" {
			withURL = &list.Items[i]
			break
		}
	}
	if withURL == nil {
		t.Fatalf("no version of content %d exposes a downloadUrl — API shape may have changed (field renamed/removed?)", contentID)
	}

	// Drift check 2: the URL resolves to a real .mpk (a valid zip with package.xml).
	var buf bytes.Buffer
	filename, err := client.Download(ctx, withURL, &buf)
	if err != nil {
		t.Fatalf("download %s: %v", withURL.VersionNumber, err)
	}
	if filename == "" {
		t.Errorf("download returned an empty filename")
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("downloaded %d bytes that are not a valid zip/.mpk: %v", buf.Len(), err)
	}
	hasPackageXML := false
	for _, f := range zr.File {
		if f.Name == "package.xml" {
			hasPackageXML = true
			break
		}
	}
	if !hasPackageXML {
		t.Errorf("downloaded .mpk has no package.xml (entries: %d)", len(zr.File))
	}
}
