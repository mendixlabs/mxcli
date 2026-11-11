// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

// Build reuse (--reuse-build).
//
// A local test run injects the test microflows, builds with mxbuild and boots.
// For an unchanged project, unchanged test files and an unchanged mxcli the
// injected model is the same as last time, so the deployment that run left in
// <project dir>/deployment is already the build this run would produce. The
// constants of a run (--constant) are runtime configuration, not model, so they
// are not part of the key: a second run with other constants reuses the build too.
//
// The key is a content hash, never a timestamp: every file of the project folder
// (minus the folders that are outputs or caches), the generated endpoint and test
// microflow text, the mxcli executable and the mxbuild override. The record also
// pins the identity of deployment/model/model.mdp, so a build made by anything
// else in that folder (run --local, mxbuild by hand) is never taken for ours.
// Any doubt means a normal build.

const reuseRecordName = "test-build.json"

// reuseSkipDirs are not inputs of the build: outputs, caches and VCS state.
//
// theme-cache is a build output, not a source: mxbuild rewrites
// theme-cache/web/theme.compiled.css on every build with a fresh
// cache-busting query string on each font/asset url() even when no theme or
// model content changed (e.g. "Atlas_Styling.ttf?<nanosecond token>"). Without
// this exclusion the key never repeats across two separate 'mxcli test'
// invocations — only within one process's --watch loop, which never re-reads
// the tree from disk — so --reuse-build would silently rebuild every time on
// any app using the Atlas Core theme (effectively all of them) and never pay
// for itself. Found by running --reuse-build twice in a row against a real
// app and seeing it rebuild both times.
var reuseSkipDirs = map[string]bool{
	".git": true, ".mxcli": true, "deployment": true, ".mpr-snapshots": true,
	".mendix-cache": true, "node_modules": true, "theme-cache": true,
}

// reuseSkipAnywhere are skipped at any depth, not only at the top: a
// node_modules under themesource/ or a nested .git is tooling state, never a
// build input, and hashing it on every run costs time for nothing.
var reuseSkipAnywhere = map[string]bool{".git": true, "node_modules": true}

type reuseRecord struct {
	Key   string `json:"key"`
	Model string `json:"model"` // identity of deployment/model/model.mdp when recorded
}

func reuseRecordPath(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), ".mxcli", reuseRecordName)
}

func deploymentModelIdentity(projectPath string) string {
	fi, err := os.Stat(filepath.Join(filepath.Dir(projectPath), "deployment", "model", "model.mdp"))
	if err != nil || fi.Size() == 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d", fi.Size(), fi.ModTime().UnixNano())
}

// hashProjectTree hashes the content of every file under dir except reuseSkipDirs
// at the top level. Files are hashed in parallel; the combination is ordered by path.
func hashProjectTree(dir string) (string, error) {
	type item struct{ rel, sum string }
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			if rel != "." && (reuseSkipDirs[rel] || reuseSkipAnywhere[d.Name()]) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			t, lerr := os.Readlink(p)
			if lerr != nil {
				return lerr
			}
			paths = append(paths, rel+"\x00->"+t)
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	sums := make([]string, len(paths))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	work := make(chan int)
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range work {
				p := paths[idx]
				if len(p) > 0 && containsNul(p) {
					sums[idx] = p
					continue
				}
				f, oerr := os.Open(filepath.Join(dir, p))
				if oerr == nil {
					h := sha256.New()
					_, oerr = io.Copy(h, f)
					f.Close()
					if oerr == nil {
						sums[idx] = hex.EncodeToString(h.Sum(nil))
						continue
					}
				}
				mu.Lock()
				if firstErr == nil {
					firstErr = oerr
				}
				mu.Unlock()
			}
		}()
	}
	for i := range paths {
		work <- i
	}
	close(work)
	wg.Wait()
	if firstErr != nil {
		return "", firstErr
	}
	all := sha256.New()
	for i, p := range paths {
		fmt.Fprintf(all, "%s\x00%s\n", p, sums[i])
	}
	return hex.EncodeToString(all.Sum(nil)), nil
}

func containsNul(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return true
		}
	}
	return false
}

// testBuildKey is the key of the build a run with these generated microflows
// would produce from this project.
func testBuildKey(opts RunOptions, endpointMDL, flowsMDL string) (string, error) {
	tree, err := hashProjectTree(filepath.Dir(opts.ProjectPath))
	if err != nil {
		return "", err
	}
	exe, _ := os.Executable()
	exeID := ""
	if fi, err := os.Stat(exe); err == nil {
		exeID = fmt.Sprintf("%s:%d:%d", exe, fi.Size(), fi.ModTime().UnixNano())
	}
	h := sha256.New()
	fmt.Fprintf(h, "tree=%s\nexe=%s\nmxbuild=%s\nskipapp=%v\nendpoint=%s\nflows=%s\n",
		tree, exeID, opts.MxBuildPath, opts.SkipAppStartup, endpointMDL, flowsMDL)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// reusableBuild reports whether the deployment on disk is the build for key.
func reusableBuild(projectPath, key string) bool {
	data, err := os.ReadFile(reuseRecordPath(projectPath))
	if err != nil {
		return false
	}
	var rec reuseRecord
	if json.Unmarshal(data, &rec) != nil || rec.Key == "" || rec.Key != key {
		return false
	}
	id := deploymentModelIdentity(projectPath)
	return id != "" && id == rec.Model
}

// recordBuild notes that the deployment on disk is the build for key. Called
// after a boot that built it; failures are ignored (the next run just builds).
func recordBuild(projectPath, key string) {
	id := deploymentModelIdentity(projectPath)
	if id == "" || key == "" {
		return
	}
	data, _ := json.Marshal(reuseRecord{Key: key, Model: id})
	_ = os.MkdirAll(filepath.Dir(reuseRecordPath(projectPath)), 0o755)
	_ = os.WriteFile(reuseRecordPath(projectPath), data, 0o644)
}

// forgetBuild drops the record: the next run builds.
func forgetBuild(projectPath string) { _ = os.Remove(reuseRecordPath(projectPath)) }
