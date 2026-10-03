// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/marketplace"
	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/spf13/cobra"
)

// buildLocalMPK writes a .mpk (a zip) with the given entries and returns its path.
func buildLocalMPK(t *testing.T, name string, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for entry, body := range entries {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

// runInstallFile runs `marketplace install` with a client factory that FAILS the
// test if it is ever called: a --file install must never construct a marketplace
// client, because that is what needs a PAT. The project directory holds a
// placeholder .mpr — the widget path only checks that the project exists.
func runInstallFile(t *testing.T, args ...string) (string, error) {
	t.Helper()
	testutil.SetHome(t, t.TempDir())

	origFactory := marketplaceClientFactory
	marketplaceClientFactory = func(_ context.Context, _ *cobra.Command) (*marketplace.Client, error) {
		t.Fatal("--file must not construct a marketplace client (that is what needs a PAT)")
		return nil, nil
	}
	t.Cleanup(func() { marketplaceClientFactory = origFactory })

	resetMarketplaceFlags()

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(append([]string{"marketplace", "install"}, args...))
	err := rootCmd.ExecuteContext(context.Background())
	return out.String(), err
}

func placeholderProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mpr := filepath.Join(dir, "app.mpr")
	if err := os.WriteFile(mpr, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	return mpr
}

const widgetPackageXML = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.mendix.com/package/1.0/">
  <clientModule name="Badge" version="3.1.0" xmlns="http://www.mendix.com/clientModule/1.0/">
    <widgetFiles><widgetFile path="Badge.xml"/></widgetFiles>
  </clientModule>
</package>`

func TestInstallFile_WidgetLandsInWidgetsDir(t *testing.T) {
	mpr := placeholderProject(t)
	src := buildLocalMPK(t, "Badge.mpk", map[string]string{
		"package.xml": widgetPackageXML,
		"Badge.xml":   "<widget/>",
	})

	out, err := runInstallFile(t, "--file", src, "-p", mpr)
	if err != nil {
		t.Fatalf("run: %v\noutput: %s", err, out)
	}
	dest := filepath.Join(filepath.Dir(mpr), "widgets", "Badge.mpk")
	got, rerr := os.ReadFile(dest)
	if rerr != nil {
		t.Fatalf("widget not placed at %s: %v\noutput: %s", dest, rerr, out)
	}
	want, _ := os.ReadFile(src)
	if !bytes.Equal(got, want) {
		t.Errorf("widget bytes differ from the source package")
	}
	if !strings.Contains(out, "Installed widget Badge.mpk") {
		t.Errorf("expected an install line naming the file, got: %s", out)
	}
	if !strings.Contains(out, "mxcli fix widgets") {
		t.Errorf("expected the fix-widgets hint, got: %s", out)
	}
}

func TestInstallFile_WidgetOverwritesOnReinstall(t *testing.T) {
	mpr := placeholderProject(t)
	dest := filepath.Join(filepath.Dir(mpr), "widgets", "Badge.mpk")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := buildLocalMPK(t, "Badge.mpk", map[string]string{"package.xml": widgetPackageXML})
	if out, err := runInstallFile(t, "--file", src, "-p", mpr); err != nil {
		t.Fatalf("run: %v\noutput: %s", err, out)
	}
	got, _ := os.ReadFile(dest)
	if string(got) == "stale" {
		t.Errorf("reinstall must overwrite the existing widget (the update path)")
	}
}

func TestInstallFile_RefusesContentIDAndFileTogether(t *testing.T) {
	mpr := placeholderProject(t)
	src := buildLocalMPK(t, "x.mpk", map[string]string{"package.xml": widgetPackageXML})
	_, err := runInstallFile(t, "20", "--file", src, "-p", mpr)
	if err == nil || !strings.Contains(err.Error(), "either a content id or --file") {
		t.Fatalf("expected the either/or refusal, got: %v", err)
	}
}

func TestInstallFile_RefusesVersionFlag(t *testing.T) {
	mpr := placeholderProject(t)
	src := buildLocalMPK(t, "x.mpk", map[string]string{"package.xml": widgetPackageXML})
	_, err := runInstallFile(t, "--file", src, "--version", "1.0.0", "-p", mpr)
	if err == nil || !strings.Contains(err.Error(), "--version") {
		t.Fatalf("expected --version to be refused with --file, got: %v", err)
	}
}

func TestInstallFile_MissingPackage(t *testing.T) {
	mpr := placeholderProject(t)
	_, err := runInstallFile(t, "--file", filepath.Join(t.TempDir(), "nope.mpk"), "-p", mpr)
	if err == nil || !strings.Contains(err.Error(), "package not found") {
		t.Fatalf("expected 'package not found', got: %v", err)
	}
}

func TestInstallFile_RefusesUnknownPackageKind(t *testing.T) {
	mpr := placeholderProject(t)
	src := buildLocalMPK(t, "odd.mpk", map[string]string{
		"package.xml": `<?xml version="1.0"?><package xmlns="http://www.mendix.com/package/1.0/"><somethingElse/></package>`,
	})
	_, err := runInstallFile(t, "--file", src, "-p", mpr)
	if err == nil || !strings.Contains(err.Error(), "neither a module nor a widget") {
		t.Fatalf("expected the unknown-kind refusal, got: %v", err)
	}
	if _, serr := os.Stat(filepath.Join(filepath.Dir(mpr), "widgets")); serr == nil {
		t.Errorf("a refused package must not create widgets/")
	}
}

func TestInstallFile_NoContentIDAndNoFile(t *testing.T) {
	mpr := placeholderProject(t)
	_, err := runInstallFile(t, "-p", mpr)
	if err == nil || !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected the usage error to mention --file, got: %v", err)
	}
}

// The online path must be untouched by the refactor: a content id still parses
// and still reaches the client factory (which this helper makes fatal).
func TestInstallFile_ContentIDStillUsesTheClient(t *testing.T) {
	mpr := placeholderProject(t)
	testutil.SetHome(t, t.TempDir())
	called := false
	origFactory := marketplaceClientFactory
	marketplaceClientFactory = func(_ context.Context, _ *cobra.Command) (*marketplace.Client, error) {
		called = true
		return nil, context.Canceled
	}
	t.Cleanup(func() { marketplaceClientFactory = origFactory })
	resetMarketplaceFlags()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"marketplace", "install", "20", "-p", mpr})
	_ = rootCmd.ExecuteContext(context.Background())
	if !called {
		t.Fatalf("a content-id install must still go through the marketplace client")
	}
}
