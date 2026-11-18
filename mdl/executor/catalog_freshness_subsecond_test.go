// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"os"
	"testing"
	"time"
)

// inSameSecond moves the project file's modification time by a fraction of a
// second without crossing a second boundary, as a write that follows a catalog
// build within the same second does. It fails the test if the second changed, so
// the test cannot pass for the wrong reason.
func inSameSecond(t *testing.T, mprPath string) {
	t.Helper()
	fi, err := os.Stat(mprPath)
	if err != nil {
		t.Fatal(err)
	}
	before := fi.ModTime()
	moved := before.Add(300 * time.Millisecond)
	if before.Nanosecond() > 500_000_000 {
		moved = before.Add(-300 * time.Millisecond)
	}
	if err := os.Chtimes(mprPath, moved, moved); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(mprPath)
	if err != nil {
		t.Fatal(err)
	}
	if after.ModTime().Unix() != before.Unix() || after.ModTime().Equal(before) {
		t.Fatalf("setup: mtime %v -> %v is not a change inside one second (file system precision?)",
			before, after.ModTime())
	}
}

// A project written in the same second as the catalog build left the cache
// looking fresh, so the next read answered from the catalog as it was before the
// write: `SnapshotDependents` after an `exec` returned no dependents about one
// run in six. The control is the unchanged file, which must stay fresh.
func TestCacheValid_WriteInTheSameSecondAsTheBuild(t *testing.T) {
	exec, cachePath := sourceCachedFixture(t)
	ctx := exec.newExecContext(context.Background())

	if ok, why := isCacheValid(ctx, cachePath, "fast"); !ok {
		t.Fatalf("control: an unchanged project must leave the cache valid, got %q", why)
	}
	if !catalogCurrent(ctx, "fast") {
		t.Fatal("control: an unchanged project must leave the in-memory catalog current")
	}

	inSameSecond(t, ctx.MprPath)

	if ok, _ := isCacheValid(ctx, cachePath, "fast"); ok {
		t.Error("isCacheValid: a project modified inside the second of the build was still treated as unchanged")
	}
	if catalogCurrent(ctx, "fast") {
		t.Error("catalogCurrent: a project modified inside the second of the build was still treated as unchanged")
	}
}
