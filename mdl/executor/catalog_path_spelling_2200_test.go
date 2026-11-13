// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ako/mxcli#2200 (mendixlabs/mxcli#1845): the catalog cache was matched on
// the exact text of -p. The same .mpr spelled another way (relative, "./",
// a non-clean path) read the cache as invalid — "MPR path changed" — and the
// query then ran on a narrower catalog built in memory, returning plausible
// but wrong results. The cache lives beside the .mpr, so what it must match is
// the file. The control is a different .mpr in the same folder.
func TestCatalogCache_AnySpellingOfTheSameMprIsValid(t *testing.T) {
	exec, cachePath := sourceCachedFixture(t)
	ctx := exec.newExecContext(context.Background())
	abs, err := filepath.Abs(ctx.MprPath)
	if err != nil {
		t.Fatal(err)
	}
	dir, base := filepath.Dir(abs), filepath.Base(abs)
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	for _, spelling := range []string{
		abs,
		base,
		"./" + base,
		filepath.Join(dir, "sub", "..", base),
		dir + string(filepath.Separator) + "." + string(filepath.Separator) + base,
	} {
		ctx.MprPath = spelling
		if valid, reason := isCacheValid(ctx, cachePath, "source"); !valid {
			t.Errorf("-p %q: cache invalid (%s), but it names the cached project", spelling, reason)
		}
	}

	// Control: another .mpr in the same folder is another project.
	other := filepath.Join(dir, "Other.mpr")
	if err := os.WriteFile(other, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx.MprPath = other
	if valid, _ := isCacheValid(ctx, cachePath, "fast"); valid {
		t.Error("a different .mpr in the same folder read the cache as its own")
	}
}
