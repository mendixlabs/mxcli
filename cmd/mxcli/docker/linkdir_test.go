// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

// errPrivilegeNotHeld is Windows' ERROR_PRIVILEGE_NOT_HELD (1314), what
// os.Symlink returns for an ordinary user without Developer Mode.
const errPrivilegeNotHeld = syscall.Errno(1314)

// newLinkFixture builds <root>/target/f.txt and returns the target, the link
// path <root>/cache/link (cache/ is created, link is not) and the cache dir.
// The root has a space in its name: mklink is driven through cmd.exe, whose
// quoting is the part most likely to break on such a path.
func newLinkFixture(t *testing.T) (target, link, cache string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "dir with space")
	target = filepath.Join(root, "target")
	cache = filepath.Join(root, "cache")
	for _, d := range []string{target, cache} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(target, "f.txt"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	return target, filepath.Join(cache, "link"), cache
}

func TestLinkDir_TargetContentsReadableThroughLink(t *testing.T) {
	target, link, _ := newLinkFixture(t)
	if err := linkDir(target, link); err != nil {
		t.Fatalf("linkDir: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(link, "f.txt"))
	if err != nil {
		t.Fatalf("reading through the link: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("read %q through the link, want %q", got, "payload")
	}
}

// ensureMxBuildRuntimeSibling's "already present" check is os.Stat(dst), which
// must follow the link to the directory it points at.
func TestLinkDir_StatFollowsLink(t *testing.T) {
	target, link, _ := newLinkFixture(t)
	if err := linkDir(target, link); err != nil {
		t.Fatalf("linkDir: %v", err)
	}
	fi, err := os.Stat(link)
	if err != nil {
		t.Fatalf("os.Stat(link): %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("os.Stat(link) is not a directory: mode %v", fi.Mode())
	}
}

// download.go and build.go os.RemoveAll the mxbuild/runtime cache directories,
// which contain this link. RemoveAll must delete the link itself, never the
// runtime it points at: following a junction here would wipe the downloaded
// runtime out of ~/.mxcli/runtime.
func TestLinkDir_RemoveAllRemovesLinkNotTarget(t *testing.T) {
	target, link, cache := newLinkFixture(t)
	if err := linkDir(target, link); err != nil {
		t.Fatalf("linkDir: %v", err)
	}
	if err := os.RemoveAll(cache); err != nil {
		t.Fatalf("RemoveAll(cache): %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("link still present after RemoveAll of its parent: %v", err)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Errorf("cache dir still present after RemoveAll: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(target, "f.txt"))
	if err != nil {
		t.Fatalf("RemoveAll followed the link and damaged the target: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("target file changed: %q", got)
	}
}

func TestLinkDir_FailsWhenLinkExists(t *testing.T) {
	target, link, _ := newLinkFixture(t)
	if err := os.Mkdir(link, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := linkDir(target, link); err == nil {
		t.Error("expected an error when the link path already exists")
	}
}

// On Windows an ordinary user cannot create a symlink (#1286). When this
// machine is such a one, prove linkDir still works, i.e. the junction fallback
// ran; when it is not (Developer Mode or admin), there is no fallback to prove.
func TestLinkDir_WindowsFallsBackToJunctionWithoutSymlinkPrivilege(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	target, link, cache := newLinkFixture(t)

	probe := filepath.Join(cache, "probe")
	err := os.Symlink(target, probe)
	if err == nil {
		t.Skip("os.Symlink succeeds here (Developer Mode or elevated): the junction fallback cannot be exercised")
	}
	if !errors.Is(err, errPrivilegeNotHeld) {
		t.Skipf("os.Symlink failed with %v, not ERROR_PRIVILEGE_NOT_HELD: the junction fallback cannot be exercised", err)
	}

	if err := linkDir(target, link); err != nil {
		t.Fatalf("linkDir without symlink privilege: %v", err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("os.Lstat(link): %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || fi.Mode()&os.ModeIrregular == 0 {
		t.Errorf("link is not a junction (Go reports one as ModeIrregular): mode %v", fi.Mode())
	}
	if _, err := os.ReadFile(filepath.Join(link, "f.txt")); err != nil {
		t.Errorf("reading through the junction: %v", err)
	}
}
