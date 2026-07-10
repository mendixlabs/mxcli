// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// gitFailBackend returns an ExecContext backed by a MockBackend that reports
// MPR v2 with a non-empty ContentsDir — enough to reach findChangedMxunitFiles.
func gitFailBackend(contentsDir string) *ExecContext {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		VersionFunc:     func() types.MPRVersion { return 2 },
		ContentsDirFunc: func() string { return contentsDir },
	}
	return &ExecContext{
		Context: context.Background(),
		Backend: mb,
		Output:  &bytes.Buffer{},
		Cache:   &executorCache{},
	}
}

// TestDiffLocal_GitError_ReturnsError is a regression test for issue #424:
// when git is not available or the project is not in a git repo, DiffLocal
// must return a non-nil error so the CLI can exit with code 1.
//
// The test replaces execCommand with a stub that simulates "git not a repo"
// (exit 128) so no real git process is spawned.
func TestDiffLocal_GitError_ReturnsError(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	// Simulate `git diff` failing with exit 128 (not a git repo).
	execCommand = func(name string, args ...string) *exec.Cmd {
		// Use the "false" command (always exits 1) or a helper binary that
		// exits with a specific code. exec.Command("sh", "-c", "exit 128")
		// produces the same *ExitError that real git produces.
		return exec.Command("sh", "-c", "exit 128")
	}

	ctx := gitFailBackend("/tmp/mprcontents")
	err := diffLocal(ctx, "HEAD", DiffOptions{})
	if err == nil {
		t.Fatal("diffLocal must return a non-nil error when git fails (issue #424: exit 0 on git error)")
	}

	// Error message should mention "git diff" so callers can surface a useful message.
	if !strings.Contains(err.Error(), "git diff") {
		t.Errorf("error message should mention 'git diff', got: %q", err.Error())
	}
}

// TestDiffLocal_NoChanges_ReturnsNil verifies that when git succeeds but
// reports no changed files, DiffLocal returns nil (exit 0 is correct).
func TestDiffLocal_NoChanges_ReturnsNil(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	// Simulate `git diff` succeeding with empty output (no changes).
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 0")
	}

	ctx := gitFailBackend("/tmp/mprcontents")
	err := diffLocal(ctx, "HEAD", DiffOptions{})
	if err != nil {
		t.Errorf("diffLocal must return nil when git reports no changes; got: %v", err)
	}
}
