// SPDX-License-Identifier: Apache-2.0

package docker

// ensureDemoUsers had 0% coverage when this package moved off sdk/mpr
// (Phase 4a of docs/plans/2026-09-14-retire-legacy-engine.md) — a WRITE path
// whose port nothing exercised. The harvest write next door sits at ~77%, so
// the gap was specific rather than a general absence of tests here.
//
// The port also collapsed `writer.Reader().GetProjectSecurity()` into
// `writer.GetProjectSecurity()`, since the backend is both halves. That is
// exactly the kind of one-line change that compiles whatever it does.

import (
	"bytes"
	"strings"
	"testing"
)

// clearDemoUsers strips the fixture's demo users so the create path is reachable.
//
// The shared fixture ships with two, so a test that skipped when any existed
// would never run — the vacuous-green shape this repo has shipped before (#808).
// Setting up the precondition is the fix, not skipping past it.
func clearDemoUsers(t *testing.T, projectPath string) {
	t.Helper()
	b, err := openForWriting(projectPath)
	if err != nil {
		t.Fatalf("open for writing: %v", err)
	}
	defer func() { _ = b.Disconnect() }()
	ps, err := b.GetProjectSecurity()
	if err != nil {
		t.Fatalf("GetProjectSecurity: %v", err)
	}
	for _, u := range ps.DemoUsers {
		if err := b.RemoveDemoUser(ps.ID, u.UserName); err != nil {
			t.Fatalf("RemoveDemoUser(%s): %v", u.UserName, err)
		}
	}
}

func TestEnsureDemoUsers_CreatesAdminWhenNoneExist(t *testing.T) {
	p := v2Fixture(t)
	clearDemoUsers(t, p)

	b, err := openReadOnly(p)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	before, err := b.GetProjectSecurity()
	if err != nil {
		t.Fatalf("GetProjectSecurity: %v", err)
	}
	_ = b.Disconnect()
	if len(before.DemoUsers) != 0 {
		t.Fatalf("clearDemoUsers left %d behind — the create path is unreachable",
			len(before.DemoUsers))
	}

	var out bytes.Buffer
	if err := ensureDemoUsers(p, &out); err != nil {
		t.Fatalf("ensureDemoUsers: %v", err)
	}

	// Read back through a FRESH connection: asserting on the in-memory value the
	// writer holds would pass against a write that never reached disk.
	after, err := openReadOnly(p)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = after.Disconnect() })
	ps, err := after.GetProjectSecurity()
	if err != nil {
		t.Fatalf("GetProjectSecurity (read-back): %v", err)
	}

	if len(ps.DemoUsers) != 1 {
		t.Fatalf("got %d demo users after ensureDemoUsers, want 1", len(ps.DemoUsers))
	}
	if ps.DemoUsers[0].UserName != "admin" {
		t.Errorf("demo user is %q, want admin", ps.DemoUsers[0].UserName)
	}
	if !ps.EnableDemoUsers {
		t.Error("demo users were created but not enabled — the app would still be inaccessible")
	}
	if got := out.String(); !strings.Contains(got, "Created demo user") {
		t.Errorf("no creation reported on the writer; output was:\n%s", got)
	}
}

// The idempotence half, and the control for the test above: a second run must
// report a skip and leave the count alone. Without it, the first test passes
// just as well against an implementation that appends a user on every build.
func TestEnsureDemoUsers_SkipsWhenUsersExist(t *testing.T) {
	p := v2Fixture(t)
	clearDemoUsers(t, p)

	var first bytes.Buffer
	if err := ensureDemoUsers(p, &first); err != nil {
		t.Fatalf("first ensureDemoUsers: %v", err)
	}

	var second bytes.Buffer
	if err := ensureDemoUsers(p, &second); err != nil {
		t.Fatalf("second ensureDemoUsers: %v", err)
	}
	if got := second.String(); !strings.Contains(got, "skipping") {
		t.Errorf("second run did not report a skip; output was:\n%s", got)
	}

	b, err := openReadOnly(p)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	ps, err := b.GetProjectSecurity()
	if err != nil {
		t.Fatalf("GetProjectSecurity: %v", err)
	}
	if len(ps.DemoUsers) != 1 {
		t.Errorf("got %d demo users after two runs, want 1 — the second run added another",
			len(ps.DemoUsers))
	}
}
