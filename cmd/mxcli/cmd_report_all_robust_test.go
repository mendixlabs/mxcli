// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mp "github.com/mendixlabs/mxcli/internal/marketplace"
	"github.com/spf13/cobra"
)

// probeMarketplace stands in for the Marketplace: it records the context it
// was given, and panics when asked to.
type probeMarketplace struct {
	gotCtx *context.Context
	panics bool
}

func (f probeMarketplace) Catalog(ctx context.Context) ([]mp.Content, error) {
	*f.gotCtx = ctx
	if f.panics {
		panic("cannot create context from nil parent")
	}
	return nil, nil
}

func (f probeMarketplace) Versions(ctx context.Context, contentID int) ([]mp.Version, error) {
	return nil, nil
}

// runAllWith runs `report all` on a copy of pedapp (it has Marketplace
// modules) for architecture and dependencies only, with the Marketplace
// replaced by fake.
func runAllWith(t *testing.T, fake probeMarketplace) (string, error) {
	t.Helper()
	work := t.TempDir()
	if err := copyTree(filepath.Join("..", "..", "testdata", "pedapp"), filepath.Join(work, "app")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(work, "reports")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	old := newMarketplaceSource
	newMarketplaceSource = func(cmd *cobra.Command) (marketplaceSource, error) { return fake, nil }
	t.Cleanup(func() { newMarketplaceSource = old })
	skip := map[string]bool{"readiness": true}
	for _, k := range reportAllKinds {
		skip[k] = k != "architecture" && k != "dependencies"
	}
	return out, runReportAll(reportAllCmd, filepath.Join(work, "app", "PedApp.mpr"), out, skip, io.Discard)
}

// Under `report all` a kind's command is never executed by cobra, so its
// context was nil: the dependencies report passed it to the Marketplace and
// panicked with "cannot create context from nil parent", and the
// vulnerability check failed with "net/http: nil Context" (ako/mxcli#2500).
func TestReportAllGivesEachKindAContext(t *testing.T) {
	var got context.Context
	if _, err := runAllWith(t, probeMarketplace{gotCtx: &got}); err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("the dependencies report reached the Marketplace with a nil context")
	}
}

// A collector that panics is one failed page, not a lost run: the other
// reports and the overview are still written (ako/mxcli#2500).
func TestReportAllSurvivesAPanickingKind(t *testing.T) {
	var got context.Context
	out, err := runAllWith(t, probeMarketplace{gotCtx: &got, panics: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"architecture.html", "architecture.json", "index.html"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("missing %s after a kind panicked", f)
		}
	}
	index, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !strings.Contains(string(index), "panic") || !strings.Contains(string(index), "cannot create context from nil parent") {
		t.Error("the overview says the dependencies report panicked, and why")
	}
}
