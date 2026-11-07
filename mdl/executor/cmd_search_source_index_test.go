// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// searchCacheFixture writes a real catalog cache next to a stand-in .mpr, at
// the given build mode, so search's ensureCatalog loads it instead of
// building. sourceRows seeds CATALOG.SOURCE; the strings table always holds
// one caption containing "Invoice".
func searchCacheFixture(t *testing.T, mode string, sourceRows [][2]string) (*ExecContext, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	return searchCacheFixtureSeeded(t, mode, sourceRows, nil)
}

// searchCacheFixtureSeeded is searchCacheFixture plus a hook that writes more
// rows into the catalog before it is saved.
func searchCacheFixtureSeeded(t *testing.T, mode string, sourceRows [][2]string, seed func(catalog.CatalogDB)) (*ExecContext, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	mprPath := filepath.Join(dir, "app.mpr")
	if err := os.WriteFile(mprPath, []byte("stand-in"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(mprPath)
	if err != nil {
		t.Fatal(err)
	}

	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	db := cat.CatalogDB()
	if _, err := db.Exec(`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, ModuleName)
		VALUES ('Sales.Invoice_Overview', 'PAGE', 'Invoice overview', 'caption', 'Sales')`); err != nil {
		t.Fatalf("seed strings: %v", err)
	}
	if seed != nil {
		seed(db)
	}
	for _, r := range sourceRows {
		if _, err := db.Exec(`INSERT INTO source (QualifiedName, ObjectType, SourceText, ModuleName, ElementId)
			VALUES (?, 'MICROFLOW', ?, 'Sales', 'id-1')`, r[0], r[1]); err != nil {
			t.Fatalf("seed source: %v", err)
		}
	}
	if err := cat.SetCacheInfo(mprPath, fi.ModTime(), "11.0.0", mode, 0); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(dir, ".mxcli", "catalog.db")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := cat.SaveToFile(cachePath); err != nil {
		t.Fatalf("save cache: %v", err)
	}
	cat.Close()

	var diag bytes.Buffer
	ctx, out := newMockCtx(t, withQuiet())
	ctx.MprPath = mprPath
	ctx.Diagnostics = &diag
	t.Cleanup(func() {
		if ctx.Catalog != nil {
			ctx.Catalog.Close()
		}
	})
	return ctx, out, &diag
}

const sourceIndexHint = "refresh catalog full source"

// A full-mode catalog has no source index. A search there answered only from
// string literals and said nothing — an agent read the empty source section
// as "no MDL mentions this". The warning must name the command that builds it,
// and go to diagnostics so --format json stays parseable.
//
// Since ako/mxcli#2004 a search builds the source index itself, so only a
// NoBuild search can still meet a catalog without one; that is where the
// warning lives now (cmd_search_builds_source_index_test.go covers the build).
func TestSearch_WarnsWhenSourceIndexNotBuilt(t *testing.T) {
	for _, format := range []string{"json", "names", "table"} {
		t.Run(format, func(t *testing.T) {
			ctx, out, diag := searchCacheFixture(t, "full", nil)
			if _, err := searchWith(ctx, "Invoice", format, SearchOptions{NoBuild: true}); err != nil {
				t.Fatalf("search: %v", err)
			}
			if !strings.Contains(diag.String(), sourceIndexHint) {
				t.Errorf("expected a missing-source-index warning naming %q on diagnostics; got %q", sourceIndexHint, diag.String())
			}
			if strings.Contains(out.String(), sourceIndexHint) || strings.Contains(out.String(), "Warning") {
				t.Errorf("warning leaked into the results payload:\n%s", out.String())
			}
			if format == "json" {
				var v []map[string]any
				if err := json.Unmarshal(out.Bytes(), &v); err != nil {
					t.Errorf("json output is not pure JSON: %v\n%s", err, out.String())
				}
			}
		})
	}
}

// The warning is about the index, not about the result: a query that matches
// nothing anywhere on a full-mode catalog must still say the source was not
// searched (this is the case where "No matches found." is most misleading).
func TestSearch_WarnsOnNoMatchesWithoutSourceIndex(t *testing.T) {
	ctx, _, diag := searchCacheFixture(t, "full", nil)
	if _, err := searchWith(ctx, "Nonexistent", "table", SearchOptions{NoBuild: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diag.String(), sourceIndexHint) {
		t.Errorf("expected warning on diagnostics; got %q", diag.String())
	}
}

// An index that was built but matched nothing is a real answer. Keyed off the
// recorded build mode, not the row count: a source-mode catalog with zero
// source rows must not warn.
func TestSearch_NoWarningWhenSourceIndexBuilt(t *testing.T) {
	cases := map[string][][2]string{
		"built-empty":    nil,
		"built-matching": {{"Sales.ACT_Invoice_Create", "create microflow Sales.ACT_Invoice_Create () begin end;"}},
	}
	for name, rows := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, out, diag := searchCacheFixture(t, "source", rows)
			if err := search(ctx, "Invoice", "json"); err != nil {
				t.Fatal(err)
			}
			if diag.Len() != 0 {
				t.Errorf("source index is built; expected no warning, got %q", diag.String())
			}
			if strings.Contains(out.String(), sourceIndexHint) {
				t.Errorf("unexpected hint in output:\n%s", out.String())
			}
		})
	}
}
