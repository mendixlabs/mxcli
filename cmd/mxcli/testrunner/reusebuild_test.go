// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestProject lays out <dir>/App.mpr, <dir>/somefile.txt and
// <dir>/deployment/model/model.mdp, and returns the .mpr path (what callers
// pass as projectPath).
func newTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("mpr"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "somefile.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	modelDir := filepath.Join(dir, "deployment", "model")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.mdp"), []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	return mpr
}

func TestHashProjectTree_StableWhenUnchanged(t *testing.T) {
	mpr := newTestProject(t)
	dir := filepath.Dir(mpr)
	h1, err := hashProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := hashProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("hash changed with no file change: %s vs %s", h1, h2)
	}
}

func TestHashProjectTree_ChangesOnContentEdit(t *testing.T) {
	mpr := newTestProject(t)
	dir := filepath.Dir(mpr)
	before, err := hashProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "somefile.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := hashProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("hash did not change after editing a tracked file")
	}
}

func TestHashProjectTree_IgnoresSkipDirs(t *testing.T) {
	mpr := newTestProject(t)
	dir := filepath.Dir(mpr)
	before, err := hashProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Changing the deployment output (a build artifact) must not change the key:
	// otherwise reuse would never hit, because a build always rewrites deployment/.
	if err := os.WriteFile(filepath.Join(dir, "deployment", "model", "model.mdp"), []byte("rebuilt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".mxcli"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mxcli", "test-build.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := hashProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("hash changed from edits inside a skip dir (deployment/.mxcli)")
	}
}

// TestHashProjectTree_IgnoresThemeCache guards the real-app finding: mxbuild
// rewrites theme-cache/web/theme.compiled.css with a fresh cache-busting
// token on every build even with no theme/model change, so counting it as an
// input would make the key never repeat across two separate 'mxcli test'
// invocations and defeat --reuse-build entirely.
func TestHashProjectTree_IgnoresThemeCache(t *testing.T) {
	mpr := newTestProject(t)
	dir := filepath.Dir(mpr)
	before, err := hashProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(dir, "theme-cache", "web")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "theme.compiled.css"), []byte("src: url(a.ttf?1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	after1, err := hashProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before != after1 {
		t.Fatal("hash changed when theme-cache/ was created")
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "theme.compiled.css"), []byte("src: url(a.ttf?2)"), 0o644); err != nil {
		t.Fatal(err)
	}
	after2, err := hashProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if after1 != after2 {
		t.Fatal("hash changed when theme-cache content changed (cache-busting token) — this is the bug that defeats --reuse-build on every real app")
	}
}

func TestTestBuildKey_ChangesWithGeneratedMDL(t *testing.T) {
	mpr := newTestProject(t)
	opts := RunOptions{ProjectPath: mpr}
	k1, err := testBuildKey(opts, "endpoint-v1", "flows-v1")
	if err != nil {
		t.Fatal(err)
	}
	k2, err := testBuildKey(opts, "endpoint-v1", "flows-v2")
	if err != nil {
		t.Fatal(err)
	}
	if k1 == k2 {
		t.Fatal("key did not change when the generated test flows changed (e.g. a test file edit)")
	}
}

func TestTestBuildKey_ChangesWithMxBuildPath(t *testing.T) {
	mpr := newTestProject(t)
	k1, err := testBuildKey(RunOptions{ProjectPath: mpr}, "ep", "fl")
	if err != nil {
		t.Fatal(err)
	}
	k2, err := testBuildKey(RunOptions{ProjectPath: mpr, MxBuildPath: "/other/mxbuild"}, "ep", "fl")
	if err != nil {
		t.Fatal(err)
	}
	if k1 == k2 {
		t.Fatal("key did not change when MxBuildPath changed: a different mxbuild could produce a different build")
	}
}

func TestReusableBuild_FalseWithNoRecord(t *testing.T) {
	mpr := newTestProject(t)
	if reusableBuild(mpr, "anykey") {
		t.Fatal("reusableBuild true with no record on disk")
	}
}

func TestReusableBuild_RoundTrip(t *testing.T) {
	mpr := newTestProject(t)
	key := "abc123"
	recordBuild(mpr, key)
	if !reusableBuild(mpr, key) {
		t.Fatal("reusableBuild false right after recordBuild with the same key")
	}
	if reusableBuild(mpr, "different-key") {
		t.Fatal("reusableBuild true for a key that was never recorded")
	}
}

// TestReusableBuild_StaleAfterIndependentRebuild is the core safety property:
// if anything else (run --local, mxbuild by hand) rebuilds deployment/ after
// we recorded a key, the recorded key must no longer be honored even though
// its string value is unchanged, because the model identity moved.
func TestReusableBuild_StaleAfterIndependentRebuild(t *testing.T) {
	mpr := newTestProject(t)
	dir := filepath.Dir(mpr)
	key := "abc123"
	recordBuild(mpr, key)
	if !reusableBuild(mpr, key) {
		t.Fatal("expected reusable right after recording")
	}

	// Simulate an independent rebuild: new content, and make sure the mtime
	// actually advances (some filesystems have 1s mtime resolution).
	mdp := filepath.Join(dir, "deployment", "model", "model.mdp")
	future := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(mdp, []byte("rebuilt-by-something-else"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(mdp, future, future); err != nil {
		t.Fatal(err)
	}

	if reusableBuild(mpr, key) {
		t.Fatal("reusableBuild true after deployment/model/model.mdp changed independently — a stale build would be reused")
	}
}

func TestReusableBuild_FalseWhenModelMissing(t *testing.T) {
	mpr := newTestProject(t)
	dir := filepath.Dir(mpr)
	key := "abc123"
	recordBuild(mpr, key)
	if err := os.Remove(filepath.Join(dir, "deployment", "model", "model.mdp")); err != nil {
		t.Fatal(err)
	}
	if reusableBuild(mpr, key) {
		t.Fatal("reusableBuild true with no deployment/model/model.mdp on disk")
	}
}

func TestForgetBuild_ClearsRecord(t *testing.T) {
	mpr := newTestProject(t)
	key := "abc123"
	recordBuild(mpr, key)
	if !reusableBuild(mpr, key) {
		t.Fatal("expected reusable right after recording")
	}
	forgetBuild(mpr)
	if reusableBuild(mpr, key) {
		t.Fatal("reusableBuild true after forgetBuild")
	}
}

func TestRecordBuild_NoopWithEmptyKeyOrMissingModel(t *testing.T) {
	mpr := newTestProject(t)
	dir := filepath.Dir(mpr)
	recordBuild(mpr, "") // must not create a record for an empty key
	if _, err := os.Stat(reuseRecordPath(mpr)); err == nil {
		t.Fatal("recordBuild wrote a record for an empty key")
	}

	emptyMpr := filepath.Join(t.TempDir(), "Empty.mpr")
	if err := os.WriteFile(emptyMpr, []byte("mpr"), 0o644); err != nil {
		t.Fatal(err)
	}
	recordBuild(emptyMpr, "somekey") // no deployment/model/model.mdp at all
	if _, err := os.Stat(reuseRecordPath(emptyMpr)); err == nil {
		t.Fatal("recordBuild wrote a record when deploymentModelIdentity was empty")
	}
	_ = dir
}

// A node_modules or .git below the top is tooling state, not a build input: it
// neither changes the key nor costs a hash.
func TestHashProjectTree_IgnoresNestedToolingDirs(t *testing.T) {
	mpr := newTestProject(t)
	dir := filepath.Dir(mpr)
	before, err := hashProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"themesource/mymodule/node_modules/pkg", "widgets/src/.git"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sub, "f"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	after, err := hashProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Error("a nested node_modules or .git changed the reuse key")
	}
	// Control: an ordinary nested file still does.
	if err := os.WriteFile(filepath.Join(dir, "themesource", "mymodule", "main.scss"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if again, _ := hashProjectTree(dir); again == after {
		t.Error("an ordinary nested file did not change the reuse key")
	}
}
