// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
)

// ---------------------------------------------------------------------------
// execLint — not connected
// ---------------------------------------------------------------------------

func TestLint_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execLint(ctx, &ast.LintStmt{})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

// ---------------------------------------------------------------------------
// listLintRules — ShowRules happy path
//
// Although listLintRules itself only writes to ctx.Output, execLint currently
// checks ctx.Connected() before dispatching to the ShowRules branch, so this
// test still needs a connected backend. It prints built-in rules
// (ID + Name + Description + Category + Severity).
// ---------------------------------------------------------------------------

func TestLint_ShowRules(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execLint(ctx, &ast.LintStmt{ShowRules: true})
	assertNoError(t, err)
	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected output listing rules, got empty")
	}
	// listLintRules registers at least MPR001 (NamingConvention)
	assertContainsStr(t, out, "MPR001")
	assertContainsStr(t, out, "NamingConvention")
	assertContainsStr(t, out, "Built-in rules:")
}

// ---------------------------------------------------------------------------
// execLint — full lint path
//
// NOTE: The full lint path (ShowRules=false) requires ctx.Reader(),
// ctx.Catalog, buildCatalog, and the linter package pipeline. The current
// mock infrastructure does not expose catalog mocks, so only
// the ShowRules branch can be exercised. Expanding coverage requires
// building catalog mock support (tracked separately).
// ---------------------------------------------------------------------------
