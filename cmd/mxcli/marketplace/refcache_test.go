// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

// TestCacheReadyNeedsTheMarker guards the invariant the whole cache rests on: a
// directory is not an entry. A process killed while writing leaves a populated
// tree, and serving that would surface as invented diff findings — a wrong
// answer, not a slow one.
func TestCacheReadyNeedsTheMarker(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PackageRef.mpr"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if cacheReady(dir) {
		t.Error("a directory with content but no marker was treated as a complete entry")
	}
	if err := os.WriteFile(filepath.Join(dir, completeMarker), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !cacheReady(dir) {
		t.Error("a marked entry was not treated as complete")
	}
}

// TestCacheDisabledIsNeverReady checks the bypass actually bypasses. Without it,
// "is this a stale-cache problem?" can only be answered by deleting the cache,
// which destroys the evidence.
func TestCacheDisabledIsNeverReady(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, completeMarker), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !cacheReady(dir) {
		t.Fatal("precondition: entry should be ready before disabling")
	}

	t.Setenv("MXCLI_NO_REF_CACHE", "1")
	if cacheReady(dir) {
		t.Error("MXCLI_NO_REF_CACHE=1 did not disable the cache")
	}
	// "0" and "false" are the off switches for the off switch; a user who exports
	// MXCLI_NO_REF_CACHE=0 means "leave it on".
	for _, off := range []string{"0", "false", "FALSE"} {
		t.Setenv("MXCLI_NO_REF_CACHE", off)
		if !cacheReady(dir) {
			t.Errorf("MXCLI_NO_REF_CACHE=%q disabled the cache; it should not", off)
		}
	}
}

// TestSafeKeyCannotEscapeTheCache checks that a version string from the API
// cannot become a path. Nothing today produces one, which is exactly why it
// would go unnoticed.
func TestSafeKeyCannotEscapeTheCache(t *testing.T) {
	for _, in := range []string{"../../etc/passwd", "a/b", `c\d`, "..", "x\x00y"} {
		got := safeKey(in)
		if strings.ContainsAny(got, `/\`) || got == ".." || strings.Contains(got, "\x00") {
			t.Errorf("safeKey(%q) = %q, which is still a path", in, got)
		}
	}
	if safeKey("") != "unknown" {
		t.Errorf("safeKey(\"\") = %q, want %q", safeKey(""), "unknown")
	}
	// Ordinary keys must survive intact, or every lookup misses and the cache is
	// silently useless.
	if got := safeKey("11.12.1"); got != "11.12.1" {
		t.Errorf("safeKey(%q) = %q, want it unchanged", "11.12.1", got)
	}
	if got := safeKey("2059615c-c6f1-4103-aedb-14820c077a1c"); got != "2059615c-c6f1-4103-aedb-14820c077a1c" {
		t.Errorf("a version UUID was mangled: %q", got)
	}
}

// TestRefCacheKeyIncludesMendixVersion is the correctness guard, not a
// housekeeping one. A reference built at a different Mendix version reports the
// platform's own conversions as user edits, so serving one project's entry to
// another version would produce confident, wrong findings.
func TestRefCacheKeyIncludesMendixVersion(t *testing.T) {
	const versionID = "2059615c-c6f1-4103-aedb-14820c077a1c"

	a, err := refCacheDir(versionID, "11.12.1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := refCacheDir(versionID, "11.13.0")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Errorf("the same published version shares a cache entry across Mendix versions: %s", a)
	}

	// And two different published versions must not collide at one Mendix version.
	c, err := refCacheDir("11111111-2222-3333-4444-555555555555", "11.12.1")
	if err != nil {
		t.Fatal(err)
	}
	if a == c {
		t.Errorf("two published versions share a cache entry: %s", a)
	}
}

// TestCopyTreeRoundTrip covers what a served entry depends on: nested files
// arrive with their contents, and the cache's own bookkeeping does not leak into
// the project tree handed to mx.
func TestCopyTreeRoundTrip(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "out")

	mustWrite(t, filepath.Join(src, "PackageRef.mpr"), "mpr")
	mustWrite(t, filepath.Join(src, "mprcontents", "unit.mxunit"), "unit")
	mustWrite(t, filepath.Join(src, completeMarker), "")

	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	if got := readFile(t, filepath.Join(dst, "PackageRef.mpr")); got != "mpr" {
		t.Errorf("top-level file = %q, want %q", got, "mpr")
	}
	if got := readFile(t, filepath.Join(dst, "mprcontents", "unit.mxunit")); got != "unit" {
		t.Errorf("nested file = %q, want %q", got, "unit")
	}
	if _, err := os.Stat(filepath.Join(dst, completeMarker)); !os.IsNotExist(err) {
		t.Error("the cache marker was copied into the project tree")
	}
}

// TestPublishToCacheReplaces checks that a rebuilt entry replaces the old one
// rather than merging into it. A merge would mix two builds' files, and the
// result would be a project that never existed.
func TestPublishToCacheReplaces(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "entry")

	first := filepath.Join(root, "build-1")
	mustWrite(t, filepath.Join(first, "keep.txt"), "old")
	mustWrite(t, filepath.Join(first, "only-in-old.txt"), "gone")
	if err := publishToCache(first, cacheDir); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if !cacheReady(cacheDir) {
		t.Fatal("entry is not ready after publishing")
	}

	second := filepath.Join(root, "build-2")
	mustWrite(t, filepath.Join(second, "keep.txt"), "new")
	if err := publishToCache(second, cacheDir); err != nil {
		t.Fatalf("second publish: %v", err)
	}

	if got := readFile(t, filepath.Join(cacheDir, "keep.txt")); got != "new" {
		t.Errorf("file content = %q, want the rebuilt %q", got, "new")
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "only-in-old.txt")); !os.IsNotExist(err) {
		t.Error("a file from the previous entry survived the replacement")
	}
	if !cacheReady(cacheDir) {
		t.Error("the replaced entry lost its marker")
	}
}

// TestCachedReferenceMissIsEmpty covers the paths that must degrade to "build
// it", not to an error: no entry, and no version ID to key on.
func TestCachedReferenceMissIsEmpty(t *testing.T) {
	dest, mpk := t.TempDir(), filepath.Join(t.TempDir(), "pkg.mpk")

	if got := CachedReference("", "11.12.1", dest, mpk); got != "" {
		t.Errorf("an empty version ID returned %q, want a miss", got)
	}
	if got := CachedReference("no-such-version-id", "11.12.1", dest, mpk); got != "" {
		t.Errorf("an absent entry returned %q, want a miss", got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestPruneKeepsNewestAndBoundOfOne covers the bound that stops the cache
// filling a small container's disk. A 34 MB entry per reference and twelve
// references in a provisioning run is ~400 MB, and running out of disk part way
// through an update is worse than a slow update: `marketplace update` does not
// roll back.
//
// The bound-of-1 case is the one worth asserting: pruning happens after
// publishing, so an entry must survive its own prune, or the cache would never
// hold anything and every run would silently rebuild.
func TestPruneKeepsNewestAndBoundOfOne(t *testing.T) {
	root := t.TempDir()
	testutil.SetHome(t, root)
	refRoot := filepath.Join(root, ".mxcli", "marketplace-refs", "ref")

	// Three entries, oldest first, with distinct marker times.
	names := []string{"oldest", "middle", "newest"}
	for i, n := range names {
		dir := filepath.Join(refRoot, n)
		mustWrite(t, filepath.Join(dir, "project", "PackageRef.mpr"), n)
		mustWrite(t, filepath.Join(dir, completeMarker), "")
		when := time.Now().Add(time.Duration(i-len(names)) * time.Hour)
		if err := os.Chtimes(filepath.Join(dir, completeMarker), when, when); err != nil {
			t.Fatal(err)
		}
	}
	// An abandoned staging directory: never servable, so it goes regardless.
	mustWrite(t, filepath.Join(refRoot, "building-123", "junk"), "x")

	pruneRefCache(2)

	for _, keep := range []string{"newest", "middle"} {
		if _, err := os.Stat(filepath.Join(refRoot, keep)); err != nil {
			t.Errorf("%s was evicted; the newest entries must survive", keep)
		}
	}
	if _, err := os.Stat(filepath.Join(refRoot, "oldest")); !os.IsNotExist(err) {
		t.Error("oldest survived a bound of 2")
	}
	if _, err := os.Stat(filepath.Join(refRoot, "building-123")); !os.IsNotExist(err) {
		t.Error("an abandoned staging directory was left behind")
	}

	pruneRefCache(1)
	if _, err := os.Stat(filepath.Join(refRoot, "newest")); err != nil {
		t.Error("a bound of 1 evicted the newest entry, so nothing would ever be cached")
	}
}

// TestPruneDisabledByZero checks the escape hatch for anyone with disk to spare.
func TestPruneDisabledByZero(t *testing.T) {
	root := t.TempDir()
	testutil.SetHome(t, root)
	refRoot := filepath.Join(root, ".mxcli", "marketplace-refs", "ref")
	for _, n := range []string{"a", "b", "c"} {
		mustWrite(t, filepath.Join(refRoot, n, completeMarker), "")
	}

	pruneRefCache(0)

	for _, n := range []string{"a", "b", "c"} {
		if _, err := os.Stat(filepath.Join(refRoot, n)); err != nil {
			t.Errorf("entry %s was evicted with pruning disabled", n)
		}
	}

	t.Setenv("MXCLI_REF_CACHE_MAX", "0")
	if got := refCacheMaxEntries(); got != 0 {
		t.Errorf("MXCLI_REF_CACHE_MAX=0 gave a bound of %d, want 0 (disabled)", got)
	}
	t.Setenv("MXCLI_REF_CACHE_MAX", "not-a-number")
	if got := refCacheMaxEntries(); got != defaultRefCacheEntries {
		t.Errorf("a malformed bound gave %d, want the default %d", got, defaultRefCacheEntries)
	}
}

// TestModelOnlyCopyKeepsWhatIsRead pins the filter that makes a reference entry
// ~14 MB instead of ~34 MB. The .mpr and mprcontents/ are the model; everything
// else in a reference project is bulk nothing reads.
//
// The negative half matters as much as the positive: if widgets/ started being
// copied again the cache would quietly double in size, and if the .mpr stopped
// being copied every hit would serve an unreadable reference.
func TestModelOnlyCopyKeepsWhatIsRead(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "out")

	mustWrite(t, filepath.Join(src, "PackageRef.mpr"), "model")
	mustWrite(t, filepath.Join(src, "mprcontents", "nested", "unit.mxunit"), "unit")
	mustWrite(t, filepath.Join(src, "widgets", "big.mpk"), "bulk")
	mustWrite(t, filepath.Join(src, "themesource", "atlas", "style.scss"), "bulk")
	mustWrite(t, filepath.Join(src, "theme-cache", "compiled.css"), "bulk")
	mustWrite(t, filepath.Join(src, "javascriptsource", "a.js"), "bulk")

	if err := copyTreeModelOnly(src, dst); err != nil {
		t.Fatalf("copyTreeModelOnly: %v", err)
	}

	if got := readFile(t, filepath.Join(dst, "PackageRef.mpr")); got != "model" {
		t.Errorf(".mpr = %q, want %q — a served entry would be unreadable", got, "model")
	}
	if got := readFile(t, filepath.Join(dst, "mprcontents", "nested", "unit.mxunit")); got != "unit" {
		t.Errorf("mprcontents unit = %q, want %q", got, "unit")
	}
	for _, bulk := range []string{"widgets", "themesource", "theme-cache", "javascriptsource"} {
		if _, err := os.Stat(filepath.Join(dst, bulk)); !os.IsNotExist(err) {
			t.Errorf("%s/ was copied into the cache entry; nothing reads it", bulk)
		}
	}

	// And the unfiltered copy still takes everything — it is what seeds a blank
	// project, which mx module-import reads in full.
	full := filepath.Join(t.TempDir(), "full")
	if err := copyTree(src, full); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(full, "widgets", "big.mpk")); err != nil {
		t.Error("the unfiltered copy dropped widgets/; a blank project needs its whole tree")
	}
}
