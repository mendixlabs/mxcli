// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Reference projects are expensive and immutable, which is the whole argument
// for caching them.
//
// Building one costs a `mx create-project` (~12s) plus a `mx module-import`
// (~7s) plus the package download (~5s), and `marketplace update` builds TWO —
// the version installed, to answer "has anyone edited this?", and the version
// being moved to. Measured on Administration (21 elements, the smallest module
// in a blank app), `diff` and `update` each took ~67s, almost none of it the
// download.
//
// Two caches, because they miss in different places:
//
//   - blankCache, keyed by Mendix version. Every reference build starts from the
//     same blank app, so a run updating six modules ran `mx create-project`
//     twelve times for twelve identical results. This one hits on the FIRST
//     build of every module after the first.
//   - refCache, keyed by (marketplace version UUID, Mendix version). The whole
//     finished reference. `diff` followed by `update` builds the same base
//     reference twice; so does re-running either. This one hits on repeats.
//
// What makes them safe to cache is that a reference is read-only once built:
// SnapshotModule reads it, PerformUpdate copies units out of it, and neither
// shells out to `mx` against it. The cached tree is never handed out directly
// for the same reason a template is not — callers get a copy.

// refCacheDisabled reports whether the caches are switched off. Set
// MXCLI_NO_REF_CACHE=1 to make every reference build from scratch, which is how
// you bisect a suspected stale-cache problem without deleting anything.
func refCacheDisabled() bool {
	v := os.Getenv("MXCLI_NO_REF_CACHE")
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// refCacheRoot returns ~/.mxcli/marketplace-refs, creating nothing.
func refCacheRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".mxcli", "marketplace-refs"), nil
}

// completeMarker names the file that makes an entry usable. An entry is a
// directory tree written by a process that may be killed halfway through, so
// presence of the directory proves nothing — the marker is written last, after
// the atomic rename, and its absence means "rebuild", never "use what is there".
const completeMarker = ".mxcli-complete"

// blankCacheDir is where a pristine `mx create-project` result for one Mendix
// version lives.
func blankCacheDir(mendixVersion string) (string, error) {
	root, err := refCacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "blank", safeKey(mendixVersion)), nil
}

// refCacheDir is where a finished reference project for one published version
// lives. The key is the marketplace version UUID, not the version NUMBER:
// numbers collide across content (a blank 11.12.1 app has Atlas_Web_Content
// 4.1.0, and Administration's content has also published a 4.1.0), so keying on
// the number would serve one module's reference for another's.
//
// The Mendix version is part of the key because a reference built at a
// different version reports Mendix's own conversions as user edits — the same
// reason PackageProject refuses a mismatch outright.
func refCacheDir(versionID, mendixVersion string) (string, error) {
	root, err := refCacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "ref", refEntryFormat+"_"+safeKey(mendixVersion)+"_"+safeKey(versionID)), nil
}

// refEntryFormat versions what an entry holds. Change it whenever isModelFile
// keeps something it did not before: an entry written under the old filter is
// complete by its own marker and would otherwise be served for ever, missing
// exactly what the new filter was extended for.
//
// "f2" added javascriptsource/ and javasource/ (mendixlabs/mxcli#1441). Entries
// with no prefix were model-only; they sit beside the new ones under ref/, are
// never looked up again, and age out through pruneRefCache.
const refEntryFormat = "f2"

// safeKey makes a path component out of a version string or UUID. Anything that
// is not plainly alphanumeric becomes '-', so a malformed version from the API
// cannot walk out of the cache directory.
//
// '.' has to survive (11.12.1 is a version), which is what makes ".." the case
// worth naming: sanitising character by character leaves it untouched, and
// filepath.Join then walks a level up. A key that is nothing but dots is not a
// key.
func safeKey(s string) string {
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if strings.Trim(b.String(), ".") == "" {
		return "unknown"
	}
	return b.String()
}

// defaultRefCacheEntries bounds the finished-reference cache.
//
// An entry is the model plus action sources (see isModelFile) — for
// Administration at 11.12.1, the 14 MB model and 1.6 MB of javascriptsource/
// measured, against 34 MB for the whole reference project.
// Twelve is what a six-module update sweep builds, base and target each, so the
// default holds a whole sweep at ~170 MB and the `update` that follows a `diff`
// still hits.
//
// It is bounded at all because running out of disk part way through an update is
// a far worse outcome than rebuilding a reference: `marketplace update` does not
// roll back, so a failed write leaves the module already dropped.
//
// The blank-project cache is deliberately NOT bounded: one entry per Mendix
// version, and it is the one that pays off on every single build.
const defaultRefCacheEntries = 12

// refCacheMaxEntries reads the bound, honouring MXCLI_REF_CACHE_MAX. 0 disables
// pruning for anyone with disk to spare.
func refCacheMaxEntries() int {
	if v := os.Getenv("MXCLI_REF_CACHE_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return defaultRefCacheEntries
}

// pruneRefCache keeps the newest max entries and removes the rest.
//
// Recency is taken from the entry's marker file, which is written when the entry
// is published and touched when it is served, so "newest" means "most recently
// useful" rather than "most recently built". Ties are broken by name so the
// result does not depend on directory order.
func pruneRefCache(max int) {
	if max <= 0 {
		return
	}
	root, err := refCacheRoot()
	if err != nil {
		return
	}
	refRoot := filepath.Join(root, "ref")
	entries, err := os.ReadDir(refRoot)
	if err != nil {
		return
	}

	type aged struct {
		name string
		mod  time.Time
	}
	var complete []aged
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(refRoot, e.Name())
		info, serr := os.Stat(filepath.Join(dir, completeMarker))
		if serr != nil {
			// An incomplete entry is never served, so it is pure waste: a build that
			// was killed, or a staging directory whose publish failed. Remove it
			// regardless of the bound.
			if strings.HasPrefix(e.Name(), "building-") {
				_ = os.RemoveAll(dir)
			}
			continue
		}
		complete = append(complete, aged{e.Name(), info.ModTime()})
	}
	if len(complete) <= max {
		return
	}
	sort.Slice(complete, func(i, j int) bool {
		if complete[i].mod.Equal(complete[j].mod) {
			return complete[i].name < complete[j].name
		}
		return complete[i].mod.After(complete[j].mod)
	})
	for _, e := range complete[max:] {
		_ = os.RemoveAll(filepath.Join(refRoot, e.name))
	}
}

// touchEntry records that an entry was used, so pruning evicts what nobody is
// asking for rather than what was simply built first.
func touchEntry(dir string) {
	now := time.Now()
	_ = os.Chtimes(filepath.Join(dir, completeMarker), now, now)
}

// cacheReady reports whether dir holds a complete cache entry.
func cacheReady(dir string) bool {
	if refCacheDisabled() {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, completeMarker))
	return err == nil
}

// publishToCache moves a freshly built tree into the cache and marks it
// complete.
//
// The build happens elsewhere and is renamed in, so a killed process leaves a
// stray temp directory rather than a half-populated entry that the next run
// would trust. A rename across filesystems is not possible, so the caller must
// build under the same root; buildDir is removed on success either way.
//
// Losing a race is not an error. Two mxcli processes may build the same
// reference at once; whoever renames second wins and the other's work is
// discarded, which costs time and never correctness.
func publishToCache(buildDir, cacheDir string) error {
	if refCacheDisabled() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		return err
	}
	// A previous complete entry is replaced rather than merged: a merge would mix
	// two builds' files, and the entry is cheap to rebuild.
	staging := cacheDir + ".old"
	_ = os.RemoveAll(staging)
	if _, err := os.Stat(cacheDir); err == nil {
		_ = os.Rename(cacheDir, staging)
	}
	if err := os.Rename(buildDir, cacheDir); err != nil {
		// Put back whatever was there; better a stale-but-complete entry than none.
		if _, serr := os.Stat(staging); serr == nil {
			_ = os.Rename(staging, cacheDir)
		}
		return fmt.Errorf("publish cache entry: %w", err)
	}
	_ = os.RemoveAll(staging)

	f, err := os.Create(filepath.Join(cacheDir, completeMarker))
	if err != nil {
		// Without the marker the entry is simply never used, so this is not fatal.
		return nil
	}
	_ = f.Close()
	return nil
}

// isModelFile reports whether a path inside a reference project is something a
// cached reference is read for: the model, and the action sources DESCRIBE
// reads beside it.
//
// A reference project is a whole blank Mendix app, and most of it is bulk that
// nothing here touches. Measured on Administration at 11.12.1, a 34 MB entry:
//
//	PackageRef.mpr      14 MB   read
//	widgets/           9.6 MB   never read
//	themesource/       6.4 MB   never read
//	theme-cache/       2.1 MB   never read (compiled CSS)
//	javascriptsource/  1.6 MB   READ — kept
//
// The .mpr is opened by SnapshotModule and PerformUpdate; PerformUpdate takes
// the module's bundled widgets from the .mpk rather than from the reference,
// deliberately, because the reference project's widgets/ also holds the blank
// template's copies.
//
// javascriptsource/ and javasource/ were once dropped here as "never read", and
// that was wrong: DESCRIBE JAVASCRIPT ACTION and DESCRIBE JAVA ACTION read
// <dir>/<Module>/actions/<Name>.{js,java} next to the .mpr, and print a
// "source not available" placeholder when it is missing. A cache miss compared
// the real source against the project's; a hit compared the placeholder, and
// every action in an untouched module came out "changed locally" — 44 of them
// on NanoflowCommons, enough to make `update --to` refuse (mendixlabs/mxcli#1441).
//
// THIS IS A CONSTRAINT ON FUTURE CHANGES. Anything that starts reading a sibling
// directory of the reference .mpr will see it on a cache miss and not on a hit
// — a difference that shows up as findings that come and go. Extend this filter
// in the same commit and bump refEntryFormat, or store the whole tree again.
//
// mprcontents/ is kept: `mx module-import` collapses the reference to MPR v1,
// but a reference built from a protected module's tree (moduletree.go) skips
// module-import and keeps whatever format the blank project was created in.
func isModelFile(rel string) bool {
	for _, dir := range []string{"mprcontents", "javascriptsource", "javasource"} {
		if rel == dir || strings.HasPrefix(rel, dir+string(filepath.Separator)) {
			return true
		}
	}
	return !strings.ContainsRune(rel, filepath.Separator) && strings.HasSuffix(rel, ".mpr")
}

// copyTree copies a directory tree. Used both to seed a build from the cache and
// to hand a caller its own copy, so the cached tree is never the one written to.
//
// Symlinks are copied as symlinks; a Mendix project has none, and following them
// would let a crafted package escape the destination.
func copyTree(src, dst string) error {
	return copyTreeFiltered(src, dst, nil)
}

// copyTreeModelOnly copies the model and the action sources DESCRIBE reads (see
// isModelFile), for the finished-reference cache.
// The blank-project cache deliberately does NOT use this: a blank app is the
// input to `mx module-import`, which reads the whole tree.
func copyTreeModelOnly(src, dst string) error {
	return copyTreeFiltered(src, dst, isModelFile)
}

func copyTreeFiltered(src, dst string, keep func(rel string) bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		// The marker is cache bookkeeping and has no business in a project tree.
		if rel == completeMarker {
			return nil
		}
		if keep != nil && !keep(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)

		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode()&os.ModeSymlink != 0:
			link, lerr := os.Readlink(path)
			if lerr != nil {
				return lerr
			}
			return os.Symlink(link, target)
		case !info.Mode().IsRegular():
			// Sockets, devices and friends are not part of a project; skipping them
			// is better than failing the whole copy over one.
			return nil
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
