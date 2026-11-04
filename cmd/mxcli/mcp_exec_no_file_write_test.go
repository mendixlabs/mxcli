// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1684 (ako/mxcli#1833): "`mxcli exec <file.mdl> --mcp <url>`
// and `mxcli -c \"...\" --mcp <url>` do not send the writes through Studio Pro's
// MCP API. The command exits 0, prints `connect local '<app>.mpr'` and
// `Created constant: ...`, makes no `ped_*` tool call ... and writes the `.mpr`
// and `mprcontents/` on disk while Studio Pro has the app open." and "With
// `--mcp` given, mxcli also skips its own refusal while `<app>.mpr.lock` exists."
//
// These run the real main() in a child process (runMainStreams), with the flags
// exactly as the report gives them, against a fake Studio Pro and a project
// whose <app>.mpr.lock is present — Studio Pro has it open. The contract:
// a statement with a PED route reaches Studio Pro, one without is refused,
// and in neither case does a byte of the .mpr or mprcontents/ change. The
// controls at the bottom show the hash detects a write and the lock guard is
// live in the same setup, so a pass here is not an inert comparison.

const mcpProbeConstant = "create constant MyFirstModule.LiveProbe (Type: String, DefaultValue: 'x');"

// mcpLockedProject copies the v2 fixture into a fresh directory and puts
// Studio Pro's lock file beside it (lower-cased, as Studio Pro names it).
func mcpLockedProject(t *testing.T, lock bool) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../testdata/expr-checker")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	if lock {
		if err := os.WriteFile(filepath.Join(dst, "minimal.mpr.lock"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dst, "minimal.mpr")
}

// modelFingerprint hashes the .mpr and every file under mprcontents/ — the
// files Studio Pro owns while the project is open.
func modelFingerprint(t *testing.T, mpr string) string {
	t.Helper()
	files := []string{mpr}
	root := filepath.Join(filepath.Dir(mpr), "mprcontents")
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		sum := sha256.Sum256(data)
		h.Write([]byte(f + "\x00" + hex.EncodeToString(sum[:]) + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestMCP_ExecAndOneLinerNeverWriteTheProjectStudioProHasOpen(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	t.Setenv("MXCLI_ALLOW_STUDIO_PRO_OPEN", "")

	cases := []struct {
		name   string
		script bool   // exec <file> rather than -c
		stmt   string // the one statement run
		routed bool   // true: has a PED route; false: must be refused
	}{
		{"exec, create constant goes through PED", true, mcpProbeConstant, true},
		{"-c, create constant goes through PED", false, mcpProbeConstant, true},
		{"exec, a statement with no PED route is refused", true, "create image collection MyFirstModule.LiveProbeImages;", false},
		{"-c, a statement with no PED route is refused", false, "create image collection MyFirstModule.LiveProbeImages;", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, calls := fakeStudioPro(t)
			mpr := mcpLockedProject(t, true)
			dir := filepath.Dir(mpr)
			before := modelFingerprint(t, mpr)

			args := []string{"-p", mpr}
			if c.script {
				script := filepath.Join(t.TempDir(), "probe.mdl")
				if err := os.WriteFile(script, []byte(c.stmt+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				args = append([]string{"exec", script}, args...)
			} else {
				args = append(args, "-c", c.stmt)
			}
			args = append(args, "--mcp", srv.URL+"/mcp", "--mcp-dial", strings.TrimPrefix(srv.URL, "http://"), "--mcp-trace")
			res := runMainStreams(t, dir, args...)
			got := calls()

			if after := modelFingerprint(t, mpr); after != before {
				t.Errorf("the .mpr / mprcontents/ changed on disk with --mcp while Studio Pro has the project open "+
					"(exit %d, PED calls %v)\nstdout:\n%s\nstderr:\n%s", res.exitCode, got, res.stdout, res.stderr)
			}
			if c.routed {
				if res.exitCode != 0 {
					t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", res.exitCode, res.stdout, res.stderr)
				}
				if !contains(got, "ped_create_document") {
					t.Errorf("Studio Pro received %v, want a ped_create_document: the write did not go through PED", got)
				}
				if !strings.Contains(res.stderr, "→ ped_create_document") {
					t.Errorf("--mcp-trace lists no ped_create_document call:\n%s", res.stderr)
				}
				return
			}
			if res.exitCode == 0 {
				t.Errorf("a statement with no PED route exited 0 under --mcp; it must be refused\nstdout:\n%s\nstderr:\n%s",
					res.stdout, res.stderr)
			}
			if !strings.Contains(res.stderr, "not supported by the MCP backend") {
				t.Errorf("the refusal does not say the statement has no MCP route:\n%s", res.stderr)
			}
			if contains(got, "ped_create_document") {
				t.Errorf("Studio Pro received a create for a statement that was refused: %v", got)
			}
		})
	}
}

// Controls for the test above, in the same setup without --mcp. The first shows
// the fingerprint does detect this write (else "unchanged" proves nothing); the
// second that the lock guard fires here, so the --mcp runs above reached the
// project with the lock in place.
func TestMCP_ControlsWithoutMCP(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	t.Setenv("MXCLI_ALLOW_STUDIO_PRO_OPEN", "")

	t.Run("no lock: the write lands and the fingerprint sees it", func(t *testing.T) {
		mpr := mcpLockedProject(t, false)
		before := modelFingerprint(t, mpr)
		res := runMainStreams(t, filepath.Dir(mpr), "-p", mpr, "-c", mcpProbeConstant)
		if res.exitCode != 0 {
			t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", res.exitCode, res.stdout, res.stderr)
		}
		if modelFingerprint(t, mpr) == before {
			t.Fatal("a local write left the fingerprint unchanged — the --mcp test above could not detect a file write")
		}
	})
	t.Run("lock present: the local write is refused", func(t *testing.T) {
		mpr := mcpLockedProject(t, true)
		before := modelFingerprint(t, mpr)
		res := runMainStreams(t, filepath.Dir(mpr), "-p", mpr, "-c", mcpProbeConstant)
		if res.exitCode == 0 || !strings.Contains(res.stderr, "Studio Pro has this project open") {
			t.Fatalf("the .mpr.lock refusal did not fire (exit %d):\n%s", res.exitCode, res.stderr)
		}
		if modelFingerprint(t, mpr) != before {
			t.Fatal("the refused write changed the project")
		}
	})
}
