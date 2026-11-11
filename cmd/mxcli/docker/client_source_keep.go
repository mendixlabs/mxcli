// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// client_source_keep.go gives mxbuild --serve's incremental build back the
// client source its own previous build deleted (ako/mxcli#2150).
//
// On the Mendix versions where mxbuild writes web/dist itself, a build writes
// the client source the bundler reads — web/pages, layouts, nanoflows,
// snippets, widgets, package.json — and deletes it when the build ends. The
// next build in the same process regenerates only the documents that changed,
// so the bundler runs on almost nothing and dist/ comes out without a single
// page. serveRecovery answers that with a fresh serve process and a cold build
// on every change.
//
// Measured by driving mxbuild 11.16.0's /build API directly on a blank app:
//
//	build                               time    restartRequired  dist
//	1  cold                             12.1 s  true             47 files, 13 pages
//	2  same process, microflow change    1.9 s  false            4 files, 0 pages
//	1' fresh process, cold              12.4 s  true             complete
//	2' microflow change, source put back 2.3 s  false            complete
//	3' page change, source put back      2.3 s  false            complete, the edit in its page module
//	4' new page, source put back         2.5 s  false            complete, the new page in dist/pages
//
// Control: 4' against a cold build of the same model on a fresh process — the
// same file set, differing only in the build timestamp each module import
// carries ("./index.js?6393…").
//
// So the source is copied aside while a build runs and put back before the
// next. The 11.14 measurement in webclient_legacy_paths.go is not this one: it
// restored the bundler config and the empty directories, not what was in them.
//
// Nothing here names the directories. What is kept is whatever a build wrote
// under web/ that a successful build then removed, so a Mendix version that
// leaves its source in place (the rollup shape, 11.13 and earlier) keeps
// nothing and the keep switches itself off.
type clientSourceKeep struct {
	webDir string
	dir    string // where the copies live; outside web/, which mxbuild recreates

	// off is set once a successful build removed nothing it wrote: this
	// Mendix version needs no keep, and the sweeps would be pure cost.
	off bool

	// restored is what restore put back before the build now running, for
	// watch's stop to take out again when that build fails.
	restored []string

	// sweeps counts completed sweeps, so a test can wait for one instead of
	// for an amount of time.
	sweeps atomic.Int64
}

// clientSourcePoll is how often a running build's web/ is swept. The files
// live for about 1.5 s (measured on 11.14 and 11.16), and a sweep that misses
// one is caught when the build ends, by missingPageSource.
const clientSourcePoll = 50 * time.Millisecond

// newClientSourceKeep returns a keep for the deployment, or nil when there is
// nowhere to put the copies. Every method is a no-op on nil.
func newClientSourceKeep(deployDir string) *clientSourceKeep {
	dir, err := os.MkdirTemp("", "mxcli-client-source-")
	if err != nil {
		return nil
	}
	return &clientSourceKeep{webDir: filepath.Join(deployDir, "web"), dir: dir}
}

// Close removes the copies.
func (k *clientSourceKeep) Close() {
	if k != nil {
		_ = os.RemoveAll(k.dir)
	}
}

// reset empties the keep. A cold build writes every source file again, so
// before one the keep is cleared and whatever it held of deleted documents
// goes with it.
func (k *clientSourceKeep) reset() {
	if k == nil {
		return
	}
	_ = os.RemoveAll(k.dir)
	_ = os.MkdirAll(k.dir, 0o755)
	k.restored = nil
}

// files lists the kept files, relative, slash-separated.
func (k *clientSourceKeep) files() []string {
	if k == nil {
		return nil
	}
	var out []string
	_ = filepath.WalkDir(k.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if rel, rerr := filepath.Rel(k.dir, path); rerr == nil && !strings.HasSuffix(rel, keepTempSuffix) {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

// restore puts the kept source back into web/ where it is absent, and returns
// how many files it wrote.
func (k *clientSourceKeep) restore() int {
	if k == nil || k.off {
		return 0
	}
	k.restored = nil
	for _, rel := range k.files() {
		target := filepath.Join(k.webDir, filepath.FromSlash(rel))
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			continue
		}
		if err := copyFile(filepath.Join(k.dir, filepath.FromSlash(rel)), target); err == nil {
			k.restored = append(k.restored, rel)
		}
	}
	return len(k.restored)
}

// watch copies aside what the build about to run writes under web/, until the
// returned stop is called with whether that build succeeded.
//
// After a successful build, anything kept that is still in web/ was not
// removed by the build and is dropped from the keep: only what mxbuild deletes
// needs putting back, and holding more would resurrect a file the user
// deleted. After a failed one the keep is left as it was and what restore put
// back is taken out again, so a build that died before its own cleanup does
// not leave web/ looking like another client shape (planWebClient reads it).
func (k *clientSourceKeep) watch() (stop func(ok bool)) {
	if k == nil || k.off {
		return func(bool) {}
	}
	existing := k.present()
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			k.sweep(existing)
			select {
			case <-done:
				return
			case <-time.After(clientSourcePoll):
			}
		}
	}()
	return func(ok bool) {
		close(done)
		wg.Wait()
		k.sweep(existing)
		if !ok {
			for _, rel := range k.restored {
				_ = os.Remove(filepath.Join(k.webDir, filepath.FromSlash(rel)))
			}
			k.restored = nil
			return
		}
		k.restored = nil
		kept := k.files()
		for _, rel := range kept {
			if _, err := os.Lstat(filepath.Join(k.webDir, filepath.FromSlash(rel))); err == nil {
				_ = os.Remove(filepath.Join(k.dir, filepath.FromSlash(rel)))
			}
		}
		if len(k.files()) == 0 {
			k.off = true
			return
		}
		// A page bundled from source the sweeps did not catch would be left
		// out of the next build's bundle with nothing to say so. An empty keep
		// sends that build down serveRecovery's full-build path instead, which
		// fills the keep again.
		if len(k.missingPageSource()) > 0 {
			k.reset()
		}
	}
}

// present is the set of files under web/ now, dist/ excluded.
func (k *clientSourceKeep) present() map[string]bool {
	set := map[string]bool{}
	k.walkWeb(func(rel string, _ fs.FileInfo) { set[rel] = true })
	return set
}

// walkWeb visits the regular files under web/ outside dist/. Errors are
// skipped: mxbuild is deleting directories under the walk.
func (k *clientSourceKeep) walkWeb(visit func(rel string, info fs.FileInfo)) {
	_ = filepath.WalkDir(k.webDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(k.webDir, path)
		if rerr != nil {
			return nil
		}
		if d.IsDir() {
			if rel == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil {
			visit(filepath.ToSlash(rel), info)
		}
		return nil
	})
}

// keepTempSuffix marks a copy in progress, so a half-written one is never
// restored.
const keepTempSuffix = ".mxcli-part"

// sweep copies aside every file that appeared since the build started, and
// every kept file whose source changed. A file that was already in web/ and is
// not kept is one earlier builds left alone, so it is not copied.
func (k *clientSourceKeep) sweep(existing map[string]bool) {
	k.walkWeb(func(rel string, info fs.FileInfo) {
		kept := filepath.Join(k.dir, filepath.FromSlash(rel))
		have, err := os.Stat(kept)
		if err != nil && existing[rel] {
			return
		}
		if err == nil && have.Size() == info.Size() && have.ModTime().Equal(info.ModTime()) {
			return
		}
		src := filepath.Join(k.webDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
			return
		}
		part := kept + keepTempSuffix
		if err := copyFile(src, part); err != nil {
			_ = os.Remove(part)
			return
		}
		// A file mxbuild was still writing is copied again on the next sweep;
		// until then the previous copy stands rather than a truncated one.
		after, err := os.Stat(src)
		if err != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
			_ = os.Remove(part)
			return
		}
		_ = os.Chtimes(part, info.ModTime(), info.ModTime())
		_ = os.Rename(part, kept)
	})
	k.sweeps.Add(1)
}

// missingPageSource names the page modules in dist/pages that the keep holds
// no source for. The build that just ran bundled them from a file the sweeps
// missed, and the next build would silently leave them out — a page that 404s
// while the loop reports the change applied.
func (k *clientSourceKeep) missingPageSource() []string {
	if k == nil || k.off {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(k.webDir, "dist", "pages"))
	if err != nil {
		return nil
	}
	var missing []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		if _, err := os.Stat(filepath.Join(k.dir, "pages", e.Name())); err != nil {
			missing = append(missing, e.Name())
		}
	}
	return missing
}
